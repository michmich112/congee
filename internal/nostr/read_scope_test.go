package nostr

import (
	"encoding/json"
	"testing"
)

func TestReadScopeCannotBeSetOrTransmittedByClient(t *testing.T) {
	var filter Filter
	if err := json.Unmarshal([]byte(`{"kinds":[1059],"ReadScope":{"RecipientPubkeys":["attacker"]},"read_scope":{"excluded_kinds":[]}}`), &filter); err != nil {
		t.Fatal(err)
	}
	if filter.ReadScope != nil {
		t.Fatal("wire input set a server-owned read scope")
	}
	filter.ReadScope = &ReadScope{ExcludedKinds: []int{1059}, RecipientPubkeys: []string{"private-identity"}}
	data, err := json.Marshal(&filter)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"kinds":[1059]}` {
		t.Fatalf("server scope leaked to wire: %s", data)
	}
}

func TestFilterValuePreservesRecipientTagOnWire(t *testing.T) {
	filter := Filter{Kinds: []int{1059}, Tag: map[string][]string{"#p": {"recipient"}}, ReadScope: &ReadScope{ExcludedKinds: []int{1059}}}
	data, err := json.Marshal([]any{"NEG-OPEN", "s", filter, "61"})
	if err != nil {
		t.Fatal(err)
	}
	var frame []json.RawMessage
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatal(err)
	}
	var decoded Filter
	if err := json.Unmarshal(frame[2], &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Tag["#p"]) != 1 || decoded.Tag["#p"][0] != "recipient" || decoded.ReadScope != nil {
		t.Fatalf("recipient filter lost or scope leaked: %s", data)
	}
}
