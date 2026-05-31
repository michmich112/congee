package relay

import "context"

type relayInjectedKey struct{}

// withRelayInjected marks ctx as carrying a host-injected event (bypass EVENT pipeline stages).
func withRelayInjected(ctx context.Context) context.Context {
	return context.WithValue(ctx, relayInjectedKey{}, true)
}

func isRelayInjected(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(relayInjectedKey{}).(bool)
	return v
}
