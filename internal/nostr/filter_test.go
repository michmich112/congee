package nostr

import (
	"encoding/json"
	"testing"
)

func TestFilterUnmarshal_Search(t *testing.T) {
	raw := `{"kinds":[1],"search":"orange apps"}`
	var f Filter
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	if !f.HasSearch() || f.SearchText() != "orange apps" {
		t.Fatalf("search: %+v", f.Search)
	}
	if len(f.Kinds) != 1 {
		t.Fatalf("kinds: %+v", f.Kinds)
	}
}

func TestFilterUnmarshal_SearchEmptyString(t *testing.T) {
	raw := `{"search":""}`
	var f Filter
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	if f.HasSearch() {
		t.Fatal("empty search should not be active")
	}
}

func TestFilterMarshal_Search(t *testing.T) {
	s := `a"b`
	f := Filter{Kinds: []int{1}, Search: &s}
	b, err := json.Marshal(&f)
	if err != nil {
		t.Fatal(err)
	}
	var g Filter
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	if g.SearchText() != s {
		t.Fatalf("got %q", g.SearchText())
	}
}

func TestFilterMatches_IgnoresSearch(t *testing.T) {
	s := "hello"
	ev := &Event{ID: repeat("a", 64), PubKey: repeat("b", 64), CreatedAt: 1, Kind: 1, Content: "hello world"}
	f := Filter{Kinds: []int{1}, Search: &s}
	if f.Matches(ev) {
		t.Fatal("Matches must ignore NIP-50 search (no live full-text on hot path)")
	}
}

func TestFilterUnmarshal_TagKeys(t *testing.T) {
	raw := `{"kinds":[1],"#e":["aaaabbbbccccddddeeeeffffaaaabbbbccccddddeeeeffffaaaabbbbccccdddd"],"#p":["111122223333444455556666777788889999aaaabbbbccccddddeeeeffff"]}`
	var f Filter
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Kinds) != 1 || f.Kinds[0] != 1 {
		t.Fatalf("kinds: %+v", f.Kinds)
	}
	if len(f.Tag["#e"]) != 1 {
		t.Fatalf("#e: %+v", f.Tag)
	}
}

func TestFilterMatches(t *testing.T) {
	eid := repeat("a", 64)
	pk := repeat("b", 64)
	ev := &Event{
		ID:        eid,
		PubKey:    pk,
		CreatedAt: 100,
		Kind:      1,
		Tags:      [][]string{{"e", repeat("c", 64)}, {"p", pk}},
		Content:   "hi",
		Sig:       repeat("d", 128),
	}
	since := int64(50)
	until := int64(150)
	f := Filter{
		IDs:     []string{eid},
		Authors: []string{pk},
		Kinds:   []int{1},
		Since:   &since,
		Until:   &until,
		Tag: map[string][]string{
			"#e": {repeat("c", 64)},
			"#p": {pk},
		},
	}
	if !f.Matches(ev) {
		t.Fatal("expected match")
	}
	f2 := Filter{Authors: []string{repeat("e", 64)}}
	if f2.Matches(ev) {
		t.Fatal("expected no match")
	}
}

func TestFilterMatches_SinceUntil(t *testing.T) {
	ev := &Event{ID: repeat("a", 64), PubKey: repeat("b", 64), CreatedAt: 10, Kind: 1}
	since := int64(11)
	f := Filter{Since: &since}
	if f.Matches(ev) {
		t.Fatal("created_at < since")
	}
	until := int64(9)
	f2 := Filter{Until: &until}
	if f2.Matches(ev) {
		t.Fatal("created_at > until")
	}
}
