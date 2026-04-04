# Phase 3 — Nostr protocol + storage foundation

## Goals

- `internal/nostr/`: event ID, signatures, filters, messages, kinds
- `internal/storage/`: `Store` interface, models, SQLite (WAL + single-writer), notifier noop
- `internal/config/`: typed JSON config + validation
- Unit + fuzz tests; granular commits

## Nostr (`internal/nostr/`)

1. **event.go** — Event struct, JSON marshaling, ID = SHA-256 of canonical serialized form per NIP-01; verify Schnorr with `btcec/v2` + `schnorr`.
2. **filter.go** — Filter struct; `Matches(Event)` for ids, authors, kinds, `#e`/`#p`/generic tag maps, since/until/limit.
3. **message.go** — Parse client arrays: `EVENT`, `REQ`, `CLOSE`. Serialize relay: `EVENT`, `OK`, `EOSE`, `CLOSED`, `NOTICE`.
4. **kinds.go** — Classify kind: regular, replaceable, ephemeral, addressable per NIP-01.

## Storage (`internal/storage/`)

1. **store.go** — Interface: `SaveEvent` (atomic replaceable/addressable replacement), `QueryEvents`, `DeleteEvent`, `CountEvents`, `SearchEvents` (stub returns "not implemented"), `SaveAuditEntry`, `QueryAuditLog`, `PurgeAuditLog`, `SaveConfigChange`, `QueryConfigChangelog`.
2. **models.go** — Bun models: events, tags, audit_log, config_changelog; indexes as in main plan.
3. **sqlite/sqlite.go** — Open with WAL; channel + single goroutine for writes; reads concurrent. Bun + sqlitedialect + sqliteshim.
4. **sqlite/migrations.go** — Versioned schema; run on open.
5. **notifier.go** — `EventNotifier`: `Notify(eventID)`, `Listen() <-chan string`; SQLite returns no-op.

## Config (`internal/config/`)

1. **config_types.go** — Structs matching `config.example.json`.
2. **config.go** — Load from path (caller passes path from env); semantic validation (ports, durations, required fields, nip numbers exist in registry when validating enabled list).

## Tests

- Unit: event ID, sig verify, filter match, kinds, SQLite CRUD, replaceable behavior.
- Fuzz: `nostr.ParseMessage`, `filter.Matches` — no panics.

## Validation

- `go test ./...` passes
- SQLite file created and migrated on first open
