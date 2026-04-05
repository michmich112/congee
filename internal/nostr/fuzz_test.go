package nostr

import (
	"encoding/json"
	"testing"
)

func FuzzParseMessage(f *testing.F) {
	f.Add([]byte(`["EVENT",{"id":"","pubkey":"` + repeat("a", 64) + `","created_at":1,"kind":1,"tags":[],"content":"","sig":"` + repeat("b", 128) + `"}]`))
	f.Add([]byte(`["REQ","sub",{"kinds":[1]}]`))
	f.Add([]byte(`["CLOSE","sub"]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseMessage(data)
	})
}

func FuzzFilterMatches(f *testing.F) {
	ev := Event{
		ID:        repeat("1", 64),
		PubKey:    repeat("2", 64),
		CreatedAt: 10,
		Kind:      1,
		Tags:      [][]string{{"e", repeat("3", 64)}},
		Content:   "x",
	}
	f.Add([]byte(`{"kinds":[1],"ids":["` + repeat("1", 64) + `"]}`))
	f.Fuzz(func(t *testing.T, filterJSON []byte) {
		var fl Filter
		if err := json.Unmarshal(filterJSON, &fl); err != nil {
			return
		}
		_ = fl.Matches(&ev)
	})
}
