package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/michmich112/congee/sdk/plugin/pluginv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pluginServer struct {
	pluginv1.UnimplementedPluginServer
	h Handler
}

func (s *pluginServer) Handshake(ctx context.Context, req *pluginv1.HandshakeRequest) (*pluginv1.HandshakeResponse, error) {
	if req.GetApiVersion() != APIVersion {
		return nil, status.Errorf(codes.FailedPrecondition, "api_version %d unsupported (want %d)", req.GetApiVersion(), APIVersion)
	}
	res, err := s.h.Handshake(ctx, json.RawMessage(req.GetSettingsJson()))
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, status.Error(codes.Internal, "nil handshake")
	}
	return &pluginv1.HandshakeResponse{
		ApiVersion:           APIVersion,
		PluginId:             res.PluginID,
		Name:                 res.Name,
		Version:              res.Version,
		Capabilities:         append([]string(nil), res.Capabilities...),
		Subscriptions:        subsToProto(res.Subscriptions),
		InterceptDeadlineMs:  int32(res.InterceptDeadlineMs),
	}, nil
}

func (s *pluginServer) Health(ctx context.Context, _ *pluginv1.HealthRequest) (*pluginv1.HealthResponse, error) {
	ready, msg, err := s.h.Health(ctx)
	if err != nil {
		return nil, err
	}
	return &pluginv1.HealthResponse{Ready: ready, Message: msg}, nil
}

func (s *pluginServer) Observe(ctx context.Context, req *pluginv1.ObserveRequest) (*pluginv1.ObserveResponse, error) {
	msg := ObserveMessage{
		Type:    req.GetMessageType(),
		SubID:   req.GetSubId(),
		Filters: filtersFromProto(req.GetFilters()),
	}
	if req.Event != nil {
		ev := eventFromProto(req.Event)
		msg.Event = &ev
	}
	if err := s.h.Observe(ctx, msg); err != nil {
		return nil, err
	}
	return &pluginv1.ObserveResponse{}, nil
}

func (s *pluginServer) InterceptREQ(ctx context.Context, req *pluginv1.InterceptREQRequest) (*pluginv1.InterceptREQResponse, error) {
	res, err := s.h.InterceptREQ(ctx, Req{SubID: req.GetSubId(), Filters: filtersFromProto(req.GetFilters())})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return &pluginv1.InterceptREQResponse{Action: pluginv1.InterceptAction_INTERCEPT_ACTION_PASSTHROUGH}, nil
	}
	out := &pluginv1.InterceptREQResponse{
		ReshapeFilters:      filtersToProto(res.ReshapeFilters),
		EventIds:            append([]string(nil), res.EventIDs...),
		SubscriptionFilters: filtersToProto(res.SubscriptionFilters),
	}
	switch res.Action {
	case InterceptReshapeREQ:
		out.Action = pluginv1.InterceptAction_INTERCEPT_ACTION_RESHAPE_REQ
	case InterceptRespond:
		out.Action = pluginv1.InterceptAction_INTERCEPT_ACTION_RESPOND
	default:
		out.Action = pluginv1.InterceptAction_INTERCEPT_ACTION_PASSTHROUGH
	}
	return out, nil
}

func (s *pluginServer) OnStoredEvent(ctx context.Context, req *pluginv1.OnStoredEventRequest) (*pluginv1.OnStoredEventResponse, error) {
	if err := s.h.OnStoredEvent(ctx, eventFromProto(req.GetEvent()), req.GetStored()); err != nil {
		return nil, err
	}
	return &pluginv1.OnStoredEventResponse{}, nil
}

func (s *pluginServer) ApplySettings(ctx context.Context, req *pluginv1.ApplySettingsRequest) (*pluginv1.ApplySettingsResponse, error) {
	subs, err := s.h.ApplySettings(ctx, json.RawMessage(req.GetSettingsJson()))
	if err != nil {
		return nil, err
	}
	return &pluginv1.ApplySettingsResponse{Subscriptions: subsToProto(subs)}, nil
}

func (s *pluginServer) AdminAction(ctx context.Context, req *pluginv1.AdminActionRequest) (*pluginv1.AdminActionResponse, error) {
	payload, err := s.h.AdminAction(ctx, req.GetName(), json.RawMessage(req.GetPayloadJson()))
	if err != nil {
		return nil, err
	}
	return &pluginv1.AdminActionResponse{PayloadJson: string(payload)}, nil
}

func (s *pluginServer) Status(ctx context.Context, _ *pluginv1.StatusRequest) (*pluginv1.StatusResponse, error) {
	st, err := s.h.Status(ctx)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return &pluginv1.StatusResponse{}, nil
	}
	return &pluginv1.StatusResponse{Ready: st.Ready, Json: string(st.JSON)}, nil
}

type hostClient struct {
	c pluginv1.HostClient
}

func (h *hostClient) QueryEvents(ctx context.Context, filters []Filter) ([]Event, error) {
	if h == nil || h.c == nil {
		return nil, fmt.Errorf("plugin: host not connected")
	}
	res, err := h.c.QueryEvents(ctx, &pluginv1.QueryEventsRequest{Filters: filtersToProto(filters)})
	if err != nil {
		return nil, err
	}
	return eventsFromProto(res.GetEvents()), nil
}

func (h *hostClient) GetEventsByIDs(ctx context.Context, ids []string) ([]Event, error) {
	if h == nil || h.c == nil {
		return nil, fmt.Errorf("plugin: host not connected")
	}
	res, err := h.c.GetEventsByIDs(ctx, &pluginv1.GetEventsByIDsRequest{Ids: ids})
	if err != nil {
		return nil, err
	}
	return eventsFromProto(res.GetEvents()), nil
}

func (h *hostClient) Log(ctx context.Context, level, message string, fields map[string]string) error {
	if h == nil || h.c == nil {
		return fmt.Errorf("plugin: host not connected")
	}
	_, err := h.c.Log(ctx, &pluginv1.LogRequest{Level: level, Message: message, Fields: fields})
	return err
}

func registerPlugin(s grpc.ServiceRegistrar, h Handler) {
	pluginv1.RegisterPluginServer(s, &pluginServer{h: h})
}
