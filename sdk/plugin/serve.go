package plugin

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/michmich112/congee/sdk/plugin/pluginv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Serve listens on CONGEE_PLUGIN_SOCKET, dials CONGEE_PLUGIN_HOST_SOCKET, and serves Handler until ctx is done.
func Serve(ctx context.Context, h Handler) error {
	if h == nil {
		return fmt.Errorf("plugin: nil handler")
	}
	pluginSock := os.Getenv(EnvPluginSocket)
	hostSock := os.Getenv(EnvPluginHostSocket)
	if pluginSock == "" || hostSock == "" {
		return fmt.Errorf("plugin: %s and %s are required", EnvPluginSocket, EnvPluginHostSocket)
	}
	_ = os.Remove(pluginSock)

	hostConn, err := dialUnixRetry(ctx, hostSock)
	if err != nil {
		return fmt.Errorf("plugin: dial host: %w", err)
	}
	defer hostConn.Close()
	hc := &hostClient{c: pluginv1.NewHostClient(hostConn)}
	if hh, ok := h.(HandlerHost); ok {
		hh.SetHost(hc)
	}

	lis, err := net.Listen("unix", pluginSock)
	if err != nil {
		return fmt.Errorf("plugin: listen %s: %w", pluginSock, err)
	}
	if err := os.Chmod(pluginSock, 0o600); err != nil {
		_ = lis.Close()
		return err
	}
	gs := grpc.NewServer()
	registerPlugin(gs, h)

	errCh := make(chan error, 1)
	go func() {
		errCh <- gs.Serve(lis)
	}()
	select {
	case <-ctx.Done():
		gs.GracefulStop()
		<-errCh
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func dialUnixRetry(ctx context.Context, path string) (*grpc.ClientConn, error) {
	var last error
	backoff := 20 * time.Millisecond
	for {
		conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			return conn, nil
		}
		last = err
		select {
		case <-ctx.Done():
			if last != nil {
				return nil, last
			}
			return nil, ctx.Err()
		case <-time.After(backoff):
			if backoff < 2*time.Second {
				backoff *= 2
			}
		}
	}
}
