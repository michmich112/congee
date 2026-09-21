package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/sdk/plugin/pluginv1"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type interceptStub struct {
	fn func(context.Context, *pluginv1.InterceptREQRequest) (*pluginv1.InterceptREQResponse, error)
}

func (s *interceptStub) Handshake(context.Context, *pluginv1.HandshakeRequest, ...grpc.CallOption) (*pluginv1.HandshakeResponse, error) {
	return &pluginv1.HandshakeResponse{}, nil
}
func (s *interceptStub) Health(context.Context, *pluginv1.HealthRequest, ...grpc.CallOption) (*pluginv1.HealthResponse, error) {
	return &pluginv1.HealthResponse{Ready: true}, nil
}
func (s *interceptStub) Observe(context.Context, *pluginv1.ObserveRequest, ...grpc.CallOption) (*pluginv1.ObserveResponse, error) {
	return &pluginv1.ObserveResponse{}, nil
}
func (s *interceptStub) OnStoredEvent(context.Context, *pluginv1.OnStoredEventRequest, ...grpc.CallOption) (*pluginv1.OnStoredEventResponse, error) {
	return &pluginv1.OnStoredEventResponse{}, nil
}
func (s *interceptStub) ApplySettings(context.Context, *pluginv1.ApplySettingsRequest, ...grpc.CallOption) (*pluginv1.ApplySettingsResponse, error) {
	return &pluginv1.ApplySettingsResponse{}, nil
}
func (s *interceptStub) AdminAction(context.Context, *pluginv1.AdminActionRequest, ...grpc.CallOption) (*pluginv1.AdminActionResponse, error) {
	return &pluginv1.AdminActionResponse{}, nil
}
func (s *interceptStub) Status(context.Context, *pluginv1.StatusRequest, ...grpc.CallOption) (*pluginv1.StatusResponse, error) {
	return &pluginv1.StatusResponse{Ready: true}, nil
}
func (s *interceptStub) InterceptREQ(ctx context.Context, in *pluginv1.InterceptREQRequest, _ ...grpc.CallOption) (*pluginv1.InterceptREQResponse, error) {
	return s.fn(ctx, in)
}

func readyInterceptInstance(m *Manager, stub *interceptStub) *instance {
	return &instance{
		id:       "fixture",
		mgr:      m,
		state:    stateReady,
		client:   stub,
		deadline: 80 * time.Millisecond,
	}
}

func sampleREQ() *nostr.ReqMessage {
	s := "shoes"
	return &nostr.ReqMessage{SubID: "sub-rpc", Filters: []nostr.Filter{{Kinds: []int{30402}, Search: &s}}}
}

func TestInterceptLogPluginPassthrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	in := readyInterceptInstance(m, &interceptStub{fn: func(context.Context, *pluginv1.InterceptREQRequest) (*pluginv1.InterceptREQResponse, error) {
		return &pluginv1.InterceptREQResponse{Action: pluginv1.InterceptAction_INTERCEPT_ACTION_PASSTHROUGH}, nil
	}})
	res := in.intercept(ctx, sampleREQ())
	if res.Action != InterceptPassthrough {
		t.Fatalf("action %v", res.Action)
	}
	got := waitInterceptLog(t, m, "fixture", 1).Entries[0]
	if got.Action != "passthrough" || got.Error != "" || got.SubID != "sub-rpc" {
		t.Fatalf("entry %+v", got)
	}
}

func TestInterceptLogPluginReshape(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	in := readyInterceptInstance(m, &interceptStub{fn: func(context.Context, *pluginv1.InterceptREQRequest) (*pluginv1.InterceptREQResponse, error) {
		return &pluginv1.InterceptREQResponse{
			Action:         pluginv1.InterceptAction_INTERCEPT_ACTION_RESHAPE_REQ,
			ReshapeFilters: []*pluginv1.Filter{{Kinds: []int32{30402}, Search: "x"}},
		}, nil
	}})
	res := in.intercept(ctx, sampleREQ())
	if res.Action != InterceptReshapeREQ {
		t.Fatalf("action %v", res.Action)
	}
	got := waitInterceptLog(t, m, "fixture", 1).Entries[0]
	if got.Action != "reshape_req" || len(got.ReshapeFilters) != 1 || got.ReshapeFilters[0].Kinds[0] != 30402 {
		t.Fatalf("entry %+v", got)
	}
}

func TestInterceptLogPluginRespond(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	in := readyInterceptInstance(m, &interceptStub{fn: func(context.Context, *pluginv1.InterceptREQRequest) (*pluginv1.InterceptREQResponse, error) {
		return &pluginv1.InterceptREQResponse{
			Action:              pluginv1.InterceptAction_INTERCEPT_ACTION_RESPOND,
			EventIds:            []string{"aa", "bb"},
			SubscriptionFilters: []*pluginv1.Filter{{Kinds: []int32{30402}}},
		}, nil
	}})
	res := in.intercept(ctx, sampleREQ())
	if res.Action != InterceptRespond {
		t.Fatalf("action %v", res.Action)
	}
	got := waitInterceptLog(t, m, "fixture", 1).Entries[0]
	if got.Action != "respond" || len(got.EventIDs) != 2 || got.EventIDs[0] != "aa" {
		t.Fatalf("ids %+v", got.EventIDs)
	}
	if len(got.SubscriptionFilters) != 1 || got.SubscriptionFilters[0].Kinds[0] != 30402 {
		t.Fatalf("sub filters %+v", got.SubscriptionFilters)
	}
}

func TestInterceptLogPluginRPCError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	in := readyInterceptInstance(m, &interceptStub{fn: func(context.Context, *pluginv1.InterceptREQRequest) (*pluginv1.InterceptREQResponse, error) {
		return nil, errors.New("boom")
	}})
	res := in.intercept(ctx, sampleREQ())
	if res.Action != InterceptPassthrough {
		t.Fatalf("action %v", res.Action)
	}
	got := waitInterceptLog(t, m, "fixture", 1).Entries[0]
	if got.Action != "passthrough" || got.Error != "rpc: boom" {
		t.Fatalf("entry %+v", got)
	}
}

func TestInterceptLogPluginTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	in := readyInterceptInstance(m, &interceptStub{fn: func(ctx context.Context, _ *pluginv1.InterceptREQRequest) (*pluginv1.InterceptREQResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	in.deadline = 20 * time.Millisecond
	res := in.intercept(ctx, sampleREQ())
	if res.Action != InterceptPassthrough {
		t.Fatalf("action %v", res.Action)
	}
	got := waitInterceptLog(t, m, "fixture", 1).Entries[0]
	if got.Action != "passthrough" || got.Error != "timeout" {
		t.Fatalf("entry %+v", got)
	}
}
