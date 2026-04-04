# Phase 6 — PostgreSQL + multi-instance

## Goals

- `internal/storage/postgres/`: Bun implementation of `Store`, migrations
- **notifier.go** — `LISTEN new_event` / `NOTIFY` with JSON payload `{id,origin}`; instance ID from hostname or UUID
- **migrator.go** — Copy SQLite ↔ PostgreSQL with progress callback
- **routes_migration.go** — Start migration + SSE progress
- **migration/+page.svelte** — UI for source/target + progress
- Tests: postgres store, notifier (where CI allows), migrator integrity

## PostgreSQL storage

- Same logical schema as SQLite; use appropriate types (`jsonb` for tag blobs if used)
- `SaveEvent` triggers `NOTIFY` after commit (in notifier adapter or store hook)

## Multi-instance fan-out

- Relay starts listener if store exposes `EventNotifier` with real implementation
- Ignore notifications where `origin == localInstanceID`
- On foreign: load event by id, run subscription manager dispatch

## Validation

- `go test` with docker-compose or `TEST_POSTGRES_DSN` optional skip
- Document local Postgres setup in getting-started or test README
