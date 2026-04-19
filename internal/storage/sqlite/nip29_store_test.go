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

	md2 := &nostr.Event{
		ID:        strings.Repeat("4", 64),
		PubKey:    relayPK,
		CreatedAt: 400,
		Kind:      39000,
		Tags:      [][]string{{"d", gid}, {"name", "Newer"}},
		Content:   "",
		Sig:       sig,
	}
	if err := st.SaveEvent(ctx, md2); err != nil {
		t.Fatal(err)
	}
	got2, err := st.GetLatestGroupMetadata39000(ctx, relayPK, gid)
	if err != nil || got2 == nil || got2.ID != md2.ID {
		t.Fatalf("latest metadata: want newer id %s got %v err=%v", md2.ID, got2, err)
	}

	ad1 := &nostr.Event{
		ID:        strings.Repeat("6", 64),
		PubKey:    relayPK,
		CreatedAt: 450,
		Kind:      nostr.NIP29KindGroupAdmins,
		Tags:      [][]string{{"d", gid}, {"p", userPK, "admin"}},
		Content:   "",
		Sig:       sig,
	}
	if err := st.SaveEvent(ctx, ad1); err != nil {
		t.Fatal(err)
	}
	ga, err := st.GetLatestGroupAdmins39001(ctx, relayPK, gid)
	if err != nil || ga == nil || ga.ID != ad1.ID {
		t.Fatalf("admins: %v %v", ga, err)
	}
	ad2 := &nostr.Event{
		ID:        strings.Repeat("7", 64),
		PubKey:    relayPK,
		CreatedAt: 550,
		Kind:      nostr.NIP29KindGroupAdmins,
		Tags:      [][]string{{"d", gid}, {"p", userPK}},
		Content:   "",
		Sig:       sig,
	}
	if err := st.SaveEvent(ctx, ad2); err != nil {
		t.Fatal(err)
	}
	ga2, err := st.GetLatestGroupAdmins39001(ctx, relayPK, gid)
	if err != nil || ga2 == nil || ga2.ID != ad2.ID {
		t.Fatalf("latest admins: want %s got %v err=%v", ad2.ID, ga2, err)
	}

	rm := &nostr.Event{
		ID:        strings.Repeat("5", 64),
		PubKey:    relayPK,
		CreatedAt: 500,
		Kind:      9001,
		Tags:      [][]string{{"h", gid}, {"p", userPK}},
		Content:   "",
		Sig:       sig,
	}
	if err := st.SaveEvent(ctx, rm); err != nil {
		t.Fatal(err)
	}
	member2, err2 := st.IsGroupMember(ctx, relayPK, gid, userPK)
	if err2 != nil || member2 {
		t.Fatalf("after 9001: member=%v err=%v", member2, err2)
	}
}
