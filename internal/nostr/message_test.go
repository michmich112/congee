package nostr

import (
	"encoding/json"
	"testing"
)

func TestParseMessageEVENT(t *testing.T) {
	raw := `["EVENT",{"id":"` + repeat("a", 64) + `","pubkey":"` + repeat("b", 64) + `","created_at":1,"kind":1,"tags":[],"content":"x","sig":"` + repeat("c", 128) + `"}]`
	msg, err := ParseMessage([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	em, ok := msg.(*EventMessage)
	if !ok || em.Event.Kind != 1 {
		t.Fatalf("%#v", msg)
	}
}

func TestParseMessageREQ(t *testing.T) {
	raw := `["REQ","mysub",{"kinds":[1,2]}]`
	msg, err := ParseMessage([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	rm, ok := msg.(*ReqMessage)
	if !ok || rm.SubID != "mysub" || len(rm.Filters) != 1 || len(rm.Filters[0].Kinds) != 2 {
		t.Fatalf("%#v", msg)
	}
}

func TestParseMessageCLOSE(t *testing.T) {
	raw := `["CLOSE","s"]`
	msg, err := ParseMessage([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	cm, ok := msg.(*CloseMessage)
	if !ok || cm.SubID != "s" {
		t.Fatalf("%#v", msg)
	}
}

func TestPeekClientCommand(t *testing.T) {
	cmd, err := PeekClientCommand([]byte(`["REQ","x",{}]`))
	if err != nil || cmd != "REQ" {
		t.Fatalf("got %q %v", cmd, err)
	}
	cmd, err = PeekClientCommand([]byte(`["COUNT","sub",{}]`))
	if err != nil || cmd != "COUNT" {
		t.Fatalf("got %q %v", cmd, err)
	}
	if _, err := PeekClientCommand([]byte(`not json`)); err == nil {
		t.Fatal("expected error")
	}
	if _, err := PeekClientCommand([]byte(`[]`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestMarshalRelayMessages(t *testing.T) {
	ev := &Event{ID: repeat("1", 64), PubKey: repeat("2", 64), CreatedAt: 1, Kind: 1, Content: "c"}
	b, err := MarshalRelayEvent("sub", ev)
	if err != nil {
		t.Fatal(err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil || len(arr) != 3 {
		t.Fatalf("%s", b)
	}
	if _, err := MarshalRelayOK(ev.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := MarshalRelayEOSE("sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := MarshalRelayClosed("sub", "reason"); err != nil {
		t.Fatal(err)
	}
	if _, err := MarshalRelayNotice("n"); err != nil {
		t.Fatal(err)
	}
}
