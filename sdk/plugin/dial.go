package plugin

import (
	"context"

	"github.com/michmich112/congee/sdk/plugin/pluginv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DialPlugin connects to a plugin Unix socket.
func DialPlugin(ctx context.Context, socketPath string) (pluginv1.PluginClient, *grpc.ClientConn, error) {
	_ = ctx
	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return pluginv1.NewPluginClient(conn), conn, nil
}

// HostServer implements pluginv1.HostServer using a Host implementation.
type HostServer struct {
	pluginv1.UnimplementedHostServer
	H Host
}

func (s *HostServer) QueryEvents(ctx context.Context, req *pluginv1.QueryEventsRequest) (*pluginv1.QueryEventsResponse, error) {
	evs, err := s.H.QueryEvents(ctx, filtersFromProto(req.GetFilters()))
	if err != nil {
		return nil, err
	}
	return &pluginv1.QueryEventsResponse{Events: eventsToProto(evs)}, nil
}

func (s *HostServer) GetEventsByIDs(ctx context.Context, req *pluginv1.GetEventsByIDsRequest) (*pluginv1.GetEventsByIDsResponse, error) {
	evs, err := s.H.GetEventsByIDs(ctx, req.GetIds())
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetEventsByIDsResponse{Events: eventsToProto(evs)}, nil
}

func (s *HostServer) Log(ctx context.Context, req *pluginv1.LogRequest) (*pluginv1.LogResponse, error) {
	if err := s.H.Log(ctx, req.GetLevel(), req.GetMessage(), req.GetFields()); err != nil {
		return nil, err
	}
	return &pluginv1.LogResponse{}, nil
}
