package turso

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

func signedDeletionFixture(t *testing.T, key *btcec.PrivateKey, kind int, at int64, tags [][]string, content string) *nostr.Event {
	t.Helper()
	ev := &nostr.Event{PubKey: hex.EncodeToString(key.PubKey().SerializeCompressed()[1:]), CreatedAt: at, Kind: kind, Tags: tags, Content: content}
	if err := ev.Sign(key); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestNIP09TombstonesSurviveRestart(t *testing.T) {
	skipNoDriver(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "deletions.db")
	open := func() *Store {
		st, err := Open(ctx, path, nil, zerolog.Nop())
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	st := open()
	owner, _ := btcec.PrivKeyFromBytes([]byte{1})
	other, _ := btcec.PrivKeyFromBytes([]byte{2})
	target := signedDeletionFixture(t, owner, 1, 100, nil, "target")
	if err := st.SaveEvent(ctx, target); err != nil {
		t.Fatal(err)
	}
	foreign := signedDeletionFixture(t, other, 5, 150, [][]string{{"e", target.ID}}, "foreign")
	if err := st.SaveEvent(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	if has, err := st.HasEventID(ctx, target.ID); err != nil || !has {
		t.Fatalf("foreign deletion removed target: %v %v", has, err)
	}
	request := signedDeletionFixture(t, owner, 5, 200, [][]string{{"e", target.ID}}, "own")
	if err := st.SaveEvent(ctx, request); err != nil {
		t.Fatal(err)
	}
	if has, err := st.HasEventID(ctx, target.ID); err != nil || has {
		t.Fatalf("own deletion left target: %v %v", has, err)
	}
	if err := st.SaveEvent(ctx, target); !errors.Is(err, storage.ErrDeletedEvent) {
		t.Fatalf("reimport deleted ID: %v", err)
	}
	future := signedDeletionFixture(t, owner, 1, 300, nil, "future exact ID")
	preemptive := signedDeletionFixture(t, owner, 5, 200, [][]string{{"e", future.ID}}, "exact ID")
	if err := st.SaveEvent(ctx, preemptive); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEvent(ctx, future); !errors.Is(err, storage.ErrDeletedEvent) {
		t.Fatalf("exact ID tombstone must not use address timestamp cutoff: %v", err)
	}
	addr := signedDeletionFixture(t, owner, 30402, 100, [][]string{{"d", "item"}}, "old")
	if err := st.SaveEvent(ctx, addr); err != nil {
		t.Fatal(err)
	}
	aTag := "30402:" + addr.PubKey + ":item"
	aRequest := signedDeletionFixture(t, owner, 5, 200, [][]string{{"a", aTag}}, "address")
	if err := st.SaveEvent(ctx, aRequest); err != nil {
		t.Fatal(err)
	}
	if has, err := st.HasEventID(ctx, addr.ID); err != nil || has {
		t.Fatalf("address deletion left target: %v %v", has, err)
	}
	if err := st.SaveEvent(ctx, addr); !errors.Is(err, storage.ErrDeletedEvent) {
		t.Fatalf("reimport deleted address: %v", err)
	}
	newAddr := signedDeletionFixture(t, owner, 30402, 201, [][]string{{"d", "item"}}, "new")
	if err := st.SaveEvent(ctx, newAddr); err != nil {
		t.Fatalf("post-deletion revision: %v", err)
	}
	oldRequest := signedDeletionFixture(t, owner, 5, 150, [][]string{{"a", aTag}}, "old request")
	if err := st.SaveEvent(ctx, oldRequest); err != nil {
		t.Fatal(err)
	}
	if has, err := st.HasEventID(ctx, newAddr.ID); err != nil || !has {
		t.Fatalf("older deletion removed newer revision: %v %v", has, err)
	}
	bad := signedDeletionFixture(t, owner, 5, 220, [][]string{{"e", newAddr.ID}}, "bad sig")
	bad.Sig = "00"
	if err := st.SaveEvent(ctx, bad); err == nil {
		t.Fatal("unsigned deletion accepted")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st = open()
	defer st.Close()
	if err := st.SaveEvent(ctx, target); !errors.Is(err, storage.ErrDeletedEvent) {
		t.Fatalf("ID tombstone after restart: %v", err)
	}
	if err := st.SaveEvent(ctx, addr); !errors.Is(err, storage.ErrDeletedEvent) {
		t.Fatalf("address tombstone after restart: %v", err)
	}
}
