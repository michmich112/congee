package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

func TestMsgIDFromCtxNilAndEmpty(t *testing.T) {
	t.Parallel()
	if got := MsgIDFromCtx(nil); got != "" {
		t.Fatalf("nil ctx: got %q", got)
	}
	if got := MsgIDFromCtx(context.Background()); got != "" {
		t.Fatalf("empty ctx: got %q", got)
	}
}

func TestWithMsgIDRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := WithMsgID(context.Background(), "abc123")
	if MsgIDFromCtx(ctx) != "abc123" {
		t.Fatalf("MsgIDFromCtx = %q", MsgIDFromCtx(ctx))
	}
}

func TestRelayLoggerNilConnIsNop(t *testing.T) {
	t.Parallel()
	l := relayLogger(nil, context.Background())
	if l.GetLevel() != zerolog.Disabled {
		t.Fatalf("expected disabled / nop logger level, got %v", l.GetLevel())
	}
	l = relayLogger(nil, WithMsgID(context.Background(), "ignored"))
	if l.GetLevel() != zerolog.Disabled {
		t.Fatalf("nil conn with msg ctx: expected nop level, got %v", l.GetLevel())
	}
}

func TestRelayLoggerConnWithAndWithoutMsgID(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	base := zerolog.New(&buf).With().Str("conn_id", "conn-x").Logger()

	c := &Conn{log: base}
	logNoMsg := relayLogger(c, context.Background())
	logNoMsg.Info().Msg("no-msg")
	line1 := buf.String()
	buf.Reset()

	var obj map[string]any
	if err := json.Unmarshal([]byte(line1), &obj); err != nil {
		t.Fatalf("line1 json: %v", err)
	}
	if _, ok := obj["msg_id"]; ok {
		t.Fatalf("did not expect msg_id on line1, got %v", obj)
	}

	logWithMsg := relayLogger(c, WithMsgID(context.Background(), "mid-9"))
	logWithMsg.Info().Msg("with-msg")
	line2 := buf.String()
	buf.Reset()
	obj = nil
	if err := json.Unmarshal([]byte(line2), &obj); err != nil {
		t.Fatalf("line2 json: %v", err)
	}
	if obj["msg_id"] != "mid-9" {
		t.Fatalf("msg_id field = %v", obj["msg_id"])
	}
}
