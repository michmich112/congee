package relay

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

// corePostStoreDeletion applies NIP-09 semantics for stored kind-5 events: delete same-author
// targets referenced by "e" and "a" tags, then audit each successful deletion.
func corePostStoreDeletion(ctx context.Context, store storage.Store, c *Conn, ev *nostr.Event) {
	if ev == nil || ev.Kind != nostr.KindDeletion {
		return
	}
	log := relayLogger(c, ctx)
	for _, tag := range ev.Tags {
		if len(tag) < 2 || tag[1] == "" {
			continue
		}
		switch tag[0] {
		case "e":
			if err := deleteByEventID(ctx, store, c, ev, tag[1]); err != nil {
				log.Error().Err(err).Str("target_id", tag[1]).Str("deletion_id", ev.ID).
					Msg("kind-5 e-tag deletion failed")
			}
		case "a":
			if err := deleteByAddressable(ctx, store, c, ev, tag[1]); err != nil {
				log.Error().Err(err).Str("coordinate", tag[1]).Str("deletion_id", ev.ID).
					Msg("kind-5 a-tag deletion failed")
			}
		}
	}
}

func deleteByEventID(ctx context.Context, store storage.Store, c *Conn, deletion *nostr.Event, targetID string) error {
	targets, err := store.QueryEvents(ctx, []nostr.Filter{{IDs: []string{targetID}}})
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	target := targets[0]
	if target.PubKey != deletion.PubKey {
		return nil
	}
	return deleteTargetEvent(ctx, store, c, deletion, target)
}

func deleteByAddressable(ctx context.Context, store storage.Store, c *Conn, deletion *nostr.Event, coordinate string) error {
	kind, pubkey, dTag, ok := nostr.ParseAddressableCoordinate(coordinate)
	if !ok || pubkey != deletion.PubKey {
		return nil
	}
	targets, err := store.QueryEvents(ctx, []nostr.Filter{{
		Authors: []string{pubkey},
		Kinds:   []int{kind},
		Tag:     map[string][]string{"#d": {dTag}},
	}})
	if err != nil {
		return err
	}
	for _, target := range targets {
		if target.PubKey != deletion.PubKey {
			continue
		}
		if err := deleteTargetEvent(ctx, store, c, deletion, target); err != nil {
			return err
		}
	}
	return nil
}

func deleteTargetEvent(ctx context.Context, store storage.Store, c *Conn, deletion, target *nostr.Event) error {
	if err := store.DeleteEvent(ctx, target.ID); err != nil {
		return err
	}
	detail := fmt.Sprintf("deleted_event_id=%s deletion_id=%s conn_id=%s kind=%d",
		target.ID, deletion.ID, c.ID, target.Kind)
	return audit.Log(ctx, store, audit.ActionEventDeleted, detail, deletion.PubKey)
}
