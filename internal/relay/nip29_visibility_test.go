package relay

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// visibilityStoreStub implements storage.Store for NIP-29 visibility tests; only
// GetLatestGroupMetadata39000 and IsGroupMember are non-trivial.
type visibilityStoreStub struct {
	md       *nostr.Event
	mdErr    error
	memberFn func(ctx context.Context, relayPubkey, groupID, memberPubkey string) (bool, error)
}

func (s *visibilityStoreStub) SaveEvent(ctx context.Context, ev *nostr.Event) error {
	_, _ = ctx, ev
	return nil
}
func (s *visibilityStoreStub) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	_, _ = ctx, filters
	return nil, nil
}
func (s *visibilityStoreStub) DeleteEvent(ctx context.Context, id string) error {
	_, _ = ctx, id
	return nil
}
func (s *visibilityStoreStub) CountEvents(ctx context.Context, filters []nostr.Filter) (int, error) {
	_, _ = ctx, filters
	return 0, nil
}
func (s *visibilityStoreStub) HasEventID(ctx context.Context, id string) (bool, error) {
	_, _ = ctx, id
	return false, nil
}
func (s *visibilityStoreStub) SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error) {
	_, _, _ = ctx, searchQuery, constraints
	return nil, nil
}
func (s *visibilityStoreStub) SaveAuditEntry(ctx context.Context, e storage.AuditEntry) error {
	_, _ = ctx, e
	return nil
}
func (s *visibilityStoreStub) HasAuditDuplicate(ctx context.Context, e storage.AuditEntry) (bool, error) {
	_, _ = ctx, e
	return false, nil
}
func (s *visibilityStoreStub) QueryAuditLog(ctx context.Context, q storage.AuditQuery) ([]storage.AuditEntry, error) {
	_, _ = ctx, q
	return nil, nil
}
func (s *visibilityStoreStub) CountAuditLog(ctx context.Context, q storage.AuditQuery) (int64, error) {
	_, _ = ctx, q
	return 0, nil
}
func (s *visibilityStoreStub) ListDistinctAuditKinds(ctx context.Context, scanLimit int) ([]int, error) {
	_, _ = ctx, scanLimit
	return nil, nil
}
func (s *visibilityStoreStub) PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error) {
	_, _ = ctx, olderThanUnix
	return 0, nil
}
func (s *visibilityStoreStub) SaveWSConnectionSession(ctx context.Context, sess storage.WSConnectionSession) (int64, error) {
	_, _ = ctx, sess
	return 0, nil
}
func (s *visibilityStoreStub) QueryWSConnectionSessions(ctx context.Context, q storage.WSConnectionSessionQuery) ([]storage.WSConnectionSession, error) {
	_, _ = ctx, q
	return nil, nil
}
func (s *visibilityStoreStub) CountWSConnectionSessions(ctx context.Context) (int64, error) {
	_ = ctx
	return 0, nil
}
func (s *visibilityStoreStub) GetWSConnectionSessionByID(ctx context.Context, id int64) (*storage.WSConnectionSession, error) {
	_, _ = ctx, id
	return nil, nil
}
func (s *visibilityStoreStub) PurgeWSConnectionSessionsBefore(ctx context.Context, olderThanUnix int64) (int64, error) {
	_, _ = ctx, olderThanUnix
	return 0, nil
}
func (s *visibilityStoreStub) SaveConfigChange(ctx context.Context, c storage.ConfigChange) error {
	_, _ = ctx, c
	return nil
}
func (s *visibilityStoreStub) QueryConfigChangelog(ctx context.Context, limit int) ([]storage.ConfigChange, error) {
	_, _ = ctx, limit
	return nil, nil
}
func (s *visibilityStoreStub) EventIDPrefixExists(ctx context.Context, prefix string, groupID string, requireSameH bool) (bool, error) {
	_, _, _, _ = ctx, prefix, groupID, requireSameH
	return false, nil
}
func (s *visibilityStoreStub) GetLatestGroupMetadata39000(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error) {
	_, _, _ = ctx, relayPubkey, groupID
	return s.md, s.mdErr
}
func (s *visibilityStoreStub) GetLatestGroupAdmins39001(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error) {
	_, _, _ = ctx, relayPubkey, groupID
	return nil, nil
}
func (s *visibilityStoreStub) IsGroupMember(ctx context.Context, relayPubkey, groupID, memberPubkey string) (bool, error) {
	if s.memberFn != nil {
		return s.memberFn(ctx, relayPubkey, groupID, memberPubkey)
	}
	_, _, _, _ = ctx, relayPubkey, groupID, memberPubkey
	return false, nil
}
func (s *visibilityStoreStub) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	_ = ctx
	return storage.AdminStorageSnapshot{}, nil
}
func (s *visibilityStoreStub) UpsertRelayMetricBucket(ctx context.Context, b storage.RelayMetricBucket) error {
	_, _ = ctx, b
	return nil
}
func (s *visibilityStoreStub) QueryRelayMetricBuckets(ctx context.Context, q storage.RelayMetricBucketQuery) ([]storage.RelayMetricBucket, error) {
	_, _ = ctx, q
	return nil, nil
}
func (s *visibilityStoreStub) PurgeRelayMetricBucketsBefore(ctx context.Context, cutoffStartUnixExclusive int64) (int64, error) {
	_, _ = ctx, cutoffStartUnixExclusive
	return 0, nil
}

func privateGroupMetadata() *nostr.Event {
	return &nostr.Event{
		Kind: nostr.NIP29KindGroupMetadata,
		Tags: [][]string{{"private"}},
	}
}

func testVisibilityServer(t *testing.T, st storage.Store, relayID *relayidentity.Identity) *Server {
	t.Helper()
	cfg := minimalRelayCfg()
	cfg.NIPs.Enabled = []int{1, 11, 29}
	srv, err := NewServer(cfg, st, zerolog.Nop(), relayID)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func testRelayIdentity(t *testing.T) *relayidentity.Identity {
	t.Helper()
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "relay.secrets.json")
	if err := relayidentity.WriteTestSecrets(path, fmt.Sprintf("%x", priv.Serialize())); err != nil {
		t.Fatal(err)
	}
	id, err := relayidentity.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func registerTestConn(t *testing.T, srv *Server, id string) *Conn {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &Conn{
		ID:     id,
		server: srv,
		ctx:    ctx,
		cancel: cancel,
		send:   make(chan []byte, 4),
		log:    zerolog.Nop(),
	}
	srv.conns.Store(id, c)
	return c
}

func groupTaggedEvent() *nostr.Event {
	return &nostr.Event{
		ID:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Kind:    1,
		Content: "x",
		Tags:    [][]string{{"h", "grp1"}},
	}
}

func TestEventVisibleToSubscriptionMetadataErrorReturnsFalse(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{mdErr: errors.New("db down")}
	id := testRelayIdentity(t)
	srv := testVisibilityServer(t, st, id)
	ev := groupTaggedEvent()
	if srv.EventVisibleToSubscription("any-conn", ev) {
		t.Fatal("expected hidden when metadata fetch errors (privacy over availability)")
	}
}

func TestEventVisibleToSubscriptionMetadataDeadlineExceededReturnsFalse(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{mdErr: context.DeadlineExceeded}
	id := testRelayIdentity(t)
	srv := testVisibilityServer(t, st, id)
	ev := groupTaggedEvent()
	if srv.EventVisibleToSubscription("any-conn", ev) {
		t.Fatal("expected hidden when metadata fetch hits deadline")
	}
}

func TestEventVisibleToSubscriptionNilMetadataReturnsTrue(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{md: nil, mdErr: nil}
	id := testRelayIdentity(t)
	srv := testVisibilityServer(t, st, id)
	ev := groupTaggedEvent()
	if !srv.EventVisibleToSubscription("any-conn", ev) {
		t.Fatal("expected visible when metadata is nil (non-private)")
	}
}

func TestEventVisibleToSubscriptionPrivateNoConnFalse(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{md: privateGroupMetadata()}
	id := testRelayIdentity(t)
	srv := testVisibilityServer(t, st, id)
	ev := groupTaggedEvent()
	if srv.EventVisibleToSubscription("missing-conn", ev) {
		t.Fatal("expected hidden for private group without live connection")
	}
}

func TestEventVisibleToSubscriptionPrivateConnNoAuthFalse(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{md: privateGroupMetadata()}
	id := testRelayIdentity(t)
	srv := testVisibilityServer(t, st, id)
	_ = registerTestConn(t, srv, "cid-auth-none")
	ev := groupTaggedEvent()
	if srv.EventVisibleToSubscription("cid-auth-none", ev) {
		t.Fatal("expected hidden with connection but no NIP-42 pubkeys")
	}
}

func TestEventVisibleToSubscriptionMemberCheckErrorContinuesHidden(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{
		md: privateGroupMetadata(),
		memberFn: func(ctx context.Context, relayPubkey, groupID, memberPubkey string) (bool, error) {
			_, _, _, _ = ctx, relayPubkey, groupID, memberPubkey
			return false, errors.New("member lookup failed")
		},
	}
	id := testRelayIdentity(t)
	srv := testVisibilityServer(t, st, id)
	c := registerTestConn(t, srv, "cid-err")
	c.nip42AddPubkey("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	c.nip42AddPubkey("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	ev := groupTaggedEvent()
	if srv.EventVisibleToSubscription("cid-err", ev) {
		t.Fatal("expected hidden when all member checks error or non-member")
	}
}

func TestEventVisibleToSubscriptionMemberWhenAuthorized(t *testing.T) {
	t.Parallel()
	memberPK := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	st := &visibilityStoreStub{
		md: privateGroupMetadata(),
		memberFn: func(ctx context.Context, relayPubkey, groupID, memberPubkey string) (bool, error) {
			_, _, _ = ctx, relayPubkey, groupID
			return memberPubkey == memberPK, nil
		},
	}
	id := testRelayIdentity(t)
	srv := testVisibilityServer(t, st, id)
	c := registerTestConn(t, srv, "cid-ok")
	c.nip42AddPubkey(memberPK)
	ev := groupTaggedEvent()
	if !srv.EventVisibleToSubscription("cid-ok", ev) {
		t.Fatal("expected visible for authed group member")
	}
}

func TestEventVisibleToSubscriptionMemberCheckErrorContinuesToNextPubkey(t *testing.T) {
	t.Parallel()
	pkErr := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	pkOK := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	st := &visibilityStoreStub{
		md: privateGroupMetadata(),
		memberFn: func(ctx context.Context, relayPubkey, groupID, memberPubkey string) (bool, error) {
			_, _, _ = ctx, relayPubkey, groupID
			if memberPubkey == pkErr {
				return false, errors.New("transient member read")
			}
			if memberPubkey == pkOK {
				return true, nil
			}
			return false, nil
		},
	}
	id := testRelayIdentity(t)
	srv := testVisibilityServer(t, st, id)
	c := registerTestConn(t, srv, "cid-mixed")
	c.nip42AddPubkey(pkErr)
	c.nip42AddPubkey(pkOK)
	ev := groupTaggedEvent()
	if !srv.EventVisibleToSubscription("cid-mixed", ev) {
		t.Fatal("expected visible when a later authed pubkey passes membership after another errors")
	}
}

func TestEventVisibleToSubscriptionNIP29DisabledUsesTrue(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{md: privateGroupMetadata()}
	id := testRelayIdentity(t)
	cfg := minimalRelayCfg()
	cfg.NIPs.Enabled = []int{1, 11}
	srv, err := NewServer(cfg, st, zerolog.Nop(), id)
	if err != nil {
		t.Fatal(err)
	}
	ev := groupTaggedEvent()
	if !srv.EventVisibleToSubscription("missing", ev) {
		t.Fatal("without NIP-29 enabled, visibility should not gate")
	}
}
