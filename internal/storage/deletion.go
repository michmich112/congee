package storage

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/uptrace/bun"
)

// ErrDeletedEvent means a signed NIP-09 request already covers this event.
var ErrDeletedEvent = errors.New("invalid: event deleted by author")

func validHex32(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32
}

// CheckDeletion must run in the same write transaction as the event insert.
func CheckDeletion(ctx context.Context, tx bun.IDB, ev *nostr.Event, dTag string) error {
	if ev.Kind == 5 {
		return nil
	}
	var byID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(deleted_at) FROM deletion_ids WHERE pubkey = ? AND event_id = ?`, ev.PubKey, ev.ID).Scan(&byID); err != nil {
		return err
	}
	if byID.Valid {
		return ErrDeletedEvent
	}
	if nostr.IsReplaceable(ev.Kind) || nostr.IsAddressable(ev.Kind) {
		var byAddress sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(deleted_at) FROM deletion_addresses WHERE pubkey = ? AND kind = ? AND d_tag = ?`, ev.PubKey, ev.Kind, dTag).Scan(&byAddress); err != nil {
			return err
		}
		if byAddress.Valid && ev.CreatedAt <= byAddress.Int64 {
			return ErrDeletedEvent
		}
	}
	return nil
}

// ApplyDeletion records durable tombstones and removes matching stored events.
// It must run after the signed kind-5 event is inserted, in that transaction.
func ApplyDeletion(ctx context.Context, tx bun.IDB, ev *nostr.Event) error {
	if ev.Kind != 5 {
		return nil
	}
	for _, tag := range ev.Tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "e":
			id := tag[1]
			if !validHex32(id) {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO deletion_ids (pubkey, event_id, deleted_at) VALUES (?, ?, ?)
				ON CONFLICT (pubkey, event_id) DO UPDATE SET deleted_at = CASE WHEN excluded.deleted_at > deletion_ids.deleted_at THEN excluded.deleted_at ELSE deletion_ids.deleted_at END`, ev.PubKey, id, ev.CreatedAt); err != nil {
				return fmt.Errorf("store deletion id: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM event_tags WHERE event_id IN (SELECT id FROM events WHERE id = ? AND pubkey = ? AND kind <> 5)`, id, ev.PubKey); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE id = ? AND pubkey = ? AND kind <> 5`, id, ev.PubKey); err != nil {
				return err
			}
		case "a":
			kindText, rest, ok := strings.Cut(tag[1], ":")
			if !ok {
				continue
			}
			pubkey, dTag, ok := strings.Cut(rest, ":")
			if !ok || pubkey != ev.PubKey || !validHex32(pubkey) {
				continue
			}
			kind, err := strconv.Atoi(kindText)
			if err != nil || !(nostr.IsReplaceable(kind) || nostr.IsAddressable(kind)) || (nostr.IsReplaceable(kind) && dTag != "") {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO deletion_addresses (pubkey, kind, d_tag, deleted_at) VALUES (?, ?, ?, ?)
				ON CONFLICT (pubkey, kind, d_tag) DO UPDATE SET deleted_at = CASE WHEN excluded.deleted_at > deletion_addresses.deleted_at THEN excluded.deleted_at ELSE deletion_addresses.deleted_at END`, ev.PubKey, kind, dTag, ev.CreatedAt); err != nil {
				return fmt.Errorf("store deletion address: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM event_tags WHERE event_id IN (SELECT id FROM events WHERE pubkey = ? AND kind = ? AND d_tag = ? AND created_at <= ?)`, ev.PubKey, kind, dTag, ev.CreatedAt); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE pubkey = ? AND kind = ? AND d_tag = ? AND created_at <= ?`, ev.PubKey, kind, dTag, ev.CreatedAt); err != nil {
				return err
			}
		}
	}
	return nil
}
