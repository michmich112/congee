package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
)

func TestNIP29StoreQueries(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "nip29.db"), nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	relayPK := strings.Repeat("a", 64)
	userPK := strings.Repeat("b", 64)
	sig := strings.Repeat("c", 128)
	gid := "testgrp"

	evNote := &nostr.Event{
		ID:        strings.Repeat("1", 64),
		PubKey:    userPK,
		CreatedAt: 100,
		Kind:      1,
		Tags:      [][]string{{"h", gid}},
		Content:   "x",
		Sig:       sig,
	}
	if err := st.SaveEvent(ctx, evNote); err != nil {
		t.Fatal(err)
	}

	ok, err := st.EventIDPrefixExists(ctx, evNote.ID[:8], gid, true)
	if err != nil || !ok {
		t.Fatalf("prefix exists same h: ok=%v err=%v", ok, err)
	}
	ok, err = st.EventIDPrefixExists(ctx, evNote.ID[:8], gid, false)
	if err != nil || !ok {
		t.Fatalf("prefix exists: ok=%v err=%v", ok, err)
	}
	ok, err = st.EventIDPrefixExists(ctx, "ffffffff", gid, false)
	if err != nil || ok {
		t.Fatalf("missing prefix: ok=%v err=%v", ok, err)
	}

	put := &nostr.Event{
		ID:        strings.Repeat("2", 64),
		PubKey:    relayPK,
		CreatedAt: 200,
		Kind:      9000,
		Tags:      [][]string{{"h", gid}, {"p", userPK}},
		Content:   "",
		Sig:       sig,
	}
	if err := st.SaveEvent(ctx, put); err != nil {
		t.Fatal(err)
	}
	member, err := st.IsGroupMember(ctx, relayPK, gid, userPK)
	if err != nil || !member {
		t.Fatalf("member: %v %v", member, err)
	}

	md := &nostr.Event{
		ID:        strings.Repeat("3", 64),
		PubKey:    relayPK,
		CreatedAt: 300,
		Kind:      39000,
		Tags:      [][]string{{"d", gid}, {"name", "T"}},
		Content:   "",
		Sig:       sig,
	}
	if err := st.SaveEvent(ctx, md); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetLatestGroupMetadata39000(ctx, relayPK, gid)
	if err != nil || got == nil || got.Kind != 39000 {
		t.Fatalf("metadata: %v %v", got, err)
	}
}
