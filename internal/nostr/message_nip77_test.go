package nostr_test

import (
	"encoding/json"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
)

func TestParseNEGOpen(t *testing.T) {
	raw := []byte(`["NEG-OPEN","s1",{"kinds":[1]},"6161"]`)
	msg, err := nostr.ParseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	open, ok := msg.(*nostr.NegOpenMessage)
	if !ok {
		t.Fatalf("want *NegOpenMessage, got %T", msg)
	}
	if open.SubID != "s1" || open.InitialHex != "6161" {
		t.Fatalf("unexpected open: %+v", open)
	}
}

func TestMarshalRelayNegErr(t *testing.T) {
	b, err := nostr.MarshalRelayNegErr("s1", "blocked: test")
	if err != nil {
		t.Fatal(err)
	}
	var arr []any
	if err := json.Unmarshal(b, &arr); err != nil {
		t.Fatal(err)
	}
	if arr[0] != "NEG-ERR" || arr[1] != "s1" {
		t.Fatalf("unexpected: %v", arr)
	}
}
