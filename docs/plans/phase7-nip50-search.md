# Phase 7 — NIP-50 search

## Goals

- SQLite: FTS5 virtual table + triggers keeping content in sync; implement `SearchEvents`
- PostgreSQL: `tsvector` column + GIN + trigger; implement `SearchEvents`
- Extend `internal/nostr/filter.go` for `search` field on REQ filters
- Register NIP-50 in registry as optional; gate on `nips.enabled`
- Unit + integration tests: empty query, special chars, ranking smoke

## Validation

- REQ with `search` returns matching events when NIP-50 enabled
- Disabled NIP-50: search filter rejected or ignored per chosen behavior (document)
