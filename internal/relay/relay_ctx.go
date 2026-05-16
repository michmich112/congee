package relay

import (
	"context"

	"github.com/rs/zerolog"
)

// newMsgID is a short random id for one inbound client message (same entropy as conn_id; distinct field in logs).
func newMsgID() string { return newConnID() }

type msgIDCtxKey struct{}

// WithMsgID returns ctx annotated for one inbound WebSocket text message (after JSON parse).
func WithMsgID(ctx context.Context, msgID string) context.Context {
	return context.WithValue(ctx, msgIDCtxKey{}, msgID)
}

// MsgIDFromCtx returns the per-message correlation id, or empty if absent.
func MsgIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(msgIDCtxKey{}).(string)
	return v
}

// relayLogger returns the connection logger with optional msg_id from ctx for this dispatch.
func relayLogger(c *Conn, ctx context.Context) zerolog.Logger {
	if c == nil {
		return zerolog.Nop()
	}
	if id := MsgIDFromCtx(ctx); id != "" {
		return c.log.With().Str("msg_id", id).Logger()
	}
	return c.log
}
