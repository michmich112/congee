package nip29

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/rs/zerolog"
)

type hostStub struct {
	md       *nostr.Event
	mdErr    error
	memberFn func(ctx context.Context, groupID, memberPubkey string) (bool, error)
	rpk      string
}

func (s *hostStub) QueryEvents(context.Context, []nostr.Filter) ([]*nostr.Event, error) {
	return nil, nil
}
func (s *hostStub) CountEvents(context.Context, []nostr.Filter) (int, error) { return 0, nil }
func (s *hostStub) HasEventID(context.Context, string) (bool, error)         { return false, nil }
func (s *hostStub) SearchEvents(context.Context, string, nostr.Filter) ([]*nostr.Event, error) {
	return nil, nil
}
func (s *hostStub) EventIDPrefixExists(context.Context, string, string, bool) (bool, error) {
	return false, nil
}
func (s *hostStub) GetLatestGroupMetadata39000(_ context.Context, _ string) (*nostr.Event, error) {
	return s.md, s.mdErr
}
func (s *hostStub) GetLatestGroupAdmins39001(context.Context, string) (*nostr.Event, error) {
	return nil, nil
}
func (s *hostStub) IsGroupMember(ctx context.Context, groupID, memberPubkey string) (bool, error) {
	if s.memberFn != nil {
		return s.memberFn(ctx, groupID, memberPubkey)
	}
	return false, nil
}
func (s *hostStub) SaveEvent(context.Context, *nostr.Event) error  { return nil }
func (s *hostStub) DeleteEvent(context.Context, string) error      { return nil }
func (s *hostStub) RelayPubkey() string                            { return s.rpk }
func (s *hostStub) SignAsRelay(context.Context, *nostr.Event) error { return nil }
func (s *hostStub) Broadcast(context.Context, *nostr.Event) error  { return nil }
func (s *hostStub) Config() json.RawMessage                        { return json.RawMessage(`{}`) }
func (s *hostStub) Logger() zerolog.Logger                         { return zerolog.Nop() }

type stubConn struct {
	id  string
	pks []string
}

func (c stubConn) ID() string              { return c.id }
func (c stubConn) PeerIP() string          { return "127.0.0.1" }
func (c stubConn) HasAuth() bool           { return len(c.pks) > 0 }
func (c stubConn) AuthedPubkeys() []string { return c.pks }

func privateGroupMetadata() *nostr.Event {
	return &nostr.Event{
		Kind: nostr.NIP29KindGroupMetadata,
		Tags: [][]string{{"private"}},
	}
}

func groupTaggedEvent() *nostr.Event {
	return &nostr.Event{
		ID:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Kind:    1,
		Content: "x",
		Tags:    [][]string{{"h", "grp1"}},
	}
}

func testPlugin(t *testing.T, h *hostStub) *nip29 {
	t.Helper()
	p := New().(*nip29)
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEventVisibleMetadataErrorReturnsFalse(t *testing.T) {
	t.Parallel()
	h := &hostStub{mdErr: errors.New("db down"), rpk: stringsRepeat("r", 64)}
	p := testPlugin(t, h)
	ok, err := p.EventVisible(context.Background(), &plugin.ReqContext{}, groupTaggedEvent())
	if err != nil || ok {
		t.Fatal("expected hidden when metadata fetch errors (privacy over availability)")
	}
}

func TestEventVisibleMetadataDeadlineExceededReturnsFalse(t *testing.T) {
	t.Parallel()
	h := &hostStub{mdErr: context.DeadlineExceeded, rpk: stringsRepeat("r", 64)}
	p := testPlugin(t, h)
	ok, err := p.EventVisible(context.Background(), &plugin.ReqContext{}, groupTaggedEvent())
	if err != nil || ok {
		t.Fatal("expected hidden when metadata fetch hits deadline")
	}
}

func TestEventVisibleNilMetadataReturnsTrue(t *testing.T) {
	t.Parallel()
	h := &hostStub{md: nil, mdErr: nil, rpk: stringsRepeat("r", 64)}
	p := testPlugin(t, h)
	ok, err := p.EventVisible(context.Background(), &plugin.ReqContext{}, groupTaggedEvent())
	if err != nil || !ok {
		t.Fatal("expected visible when metadata is nil (non-private)")
	}
}

func TestEventVisiblePrivateNoConnFalse(t *testing.T) {
	t.Parallel()
	h := &hostStub{md: privateGroupMetadata(), rpk: stringsRepeat("r", 64)}
	p := testPlugin(t, h)
	ok, err := p.EventVisible(context.Background(), nil, groupTaggedEvent())
	if err != nil || ok {
		t.Fatal("expected hidden for private group without connection")
	}
}

func TestEventVisiblePrivateConnNoAuthFalse(t *testing.T) {
	t.Parallel()
	h := &hostStub{md: privateGroupMetadata(), rpk: stringsRepeat("r", 64)}
	p := testPlugin(t, h)
	rc := &plugin.ReqContext{Conn: stubConn{id: "cid-auth-none"}}
	ok, err := p.EventVisible(context.Background(), rc, groupTaggedEvent())
	if err != nil || ok {
		t.Fatal("expected hidden with connection but no NIP-42 pubkeys")
	}
}

func TestEventVisibleMemberCheckErrorContinuesHidden(t *testing.T) {
	t.Parallel()
	h := &hostStub{
		md:  privateGroupMetadata(),
		rpk: stringsRepeat("r", 64),
		memberFn: func(ctx context.Context, groupID, memberPubkey string) (bool, error) {
			_, _, _ = ctx, groupID, memberPubkey
			return false, errors.New("member lookup failed")
		},
	}
	p := testPlugin(t, h)
	rc := &plugin.ReqContext{Conn: stubConn{id: "cid-err", pks: []string{
		stringsRepeat("b", 64),
		stringsRepeat("c", 64),
	}}}
	ok, err := p.EventVisible(context.Background(), rc, groupTaggedEvent())
	if err != nil || ok {
		t.Fatal("expected hidden when all member checks error or non-member")
	}
}

func TestEventVisibleMemberWhenAuthorized(t *testing.T) {
	t.Parallel()
	memberPK := stringsRepeat("d", 64)
	h := &hostStub{
		md:  privateGroupMetadata(),
		rpk: stringsRepeat("r", 64),
		memberFn: func(ctx context.Context, groupID, memberPubkey string) (bool, error) {
			_, _ = ctx, groupID
			return memberPubkey == memberPK, nil
		},
	}
	p := testPlugin(t, h)
	rc := &plugin.ReqContext{Conn: stubConn{id: "cid-ok", pks: []string{memberPK}}}
	ok, err := p.EventVisible(context.Background(), rc, groupTaggedEvent())
	if err != nil || !ok {
		t.Fatal("expected visible for authed group member")
	}
}

func TestEventVisibleMemberCheckErrorContinuesToNextPubkey(t *testing.T) {
	t.Parallel()
	pkErr := stringsRepeat("e", 64)
	pkOK := stringsRepeat("f", 64)
	h := &hostStub{
		md:  privateGroupMetadata(),
		rpk: stringsRepeat("r", 64),
		memberFn: func(ctx context.Context, groupID, memberPubkey string) (bool, error) {
			_, _ = ctx, groupID
			if memberPubkey == pkErr {
				return false, errors.New("transient member read")
			}
			if memberPubkey == pkOK {
				return true, nil
			}
			return false, nil
		},
	}
	p := testPlugin(t, h)
	rc := &plugin.ReqContext{Conn: stubConn{id: "cid-mixed", pks: []string{pkErr, pkOK}}}
	ok, err := p.EventVisible(context.Background(), rc, groupTaggedEvent())
	if err != nil || !ok {
		t.Fatal("expected visible when a later authed pubkey passes membership after another errors")
	}
}

func TestEventVisibleNoHTagReturnsTrue(t *testing.T) {
	t.Parallel()
	h := &hostStub{md: privateGroupMetadata(), rpk: stringsRepeat("r", 64)}
	p := testPlugin(t, h)
	ev := &nostr.Event{Kind: 1, Content: "plain"}
	ok, err := p.EventVisible(context.Background(), nil, ev)
	if err != nil || !ok {
		t.Fatal("events without h tag should not be gated")
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = s[0]
	}
	return string(out)
}
