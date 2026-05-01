# Congee — agent context

## What this project is

**Congee** is a [Nostr](https://github.com/nostr-protocol/nips) relay written in Go. It stores events in **SQLite** by default, with optional **PostgreSQL** for larger or multi-instance deployments. A **Svelte 5** admin UI runs on a separate HTTP port when `ENABLE_ADMIN_UI=true`.

Nostr clients connect over **WebSocket** and exchange JSON messages: `EVENT`, `REQ`, `CLOSE`, and relay replies such as `OK`, `EOSE`, `CLOSED`, `NOTICE`.

## Architecture (high level)

- `cmd/congee/` — entrypoint: config, storage, NIP loader, relay server, optional admin server.
- `internal/nostr/` — event, filter, message parsing, kind classification (NIP-01).
- `internal/storage/` — `Store` interface; SQLite and PostgreSQL implementations (Bun ORM).
- `internal/relay/` — HTTP/WebSocket relay, subscription manager, validation chain, hooks, rate limiting, NIP-11, health.
- `internal/relayidentity/` — relay secp256k1 secrets file (`relay.secrets.json`), derived pubkey / NIP-19 npub, NIP-11 pubkey reconciliation.
- `internal/nips/` — NIP registry and loader (validators, hooks, message handlers).
- `internal/audit/` — audit log writes and retention cleanup.
- `internal/admin/` — standalone admin HTTP server (API + static UI or dev proxy).
- `internal/config/` — JSON config load/validate, atomic writes, changelog.
- `web/admin/` — SvelteKit admin UI (Tailwind, shadcn-svelte).

### PostgreSQL: `event_tags.full_json` (JSONB)

Bun’s JSONB appender calls `json.Marshal` on the Go field. Use **`json.RawMessage`** (or raw `[]byte` with the correct appender) for `full_json`—not **`string` holding JSON text with `bun:",type:jsonb"`**—or the column stores a JSON **string** wrapping the array and `QueryEvents` / admin event loads can fail. Legacy rows are decoded via `storage.DecodeTagFullJSON`. Regression coverage: `TestPostgresManyTagsJSONBRoundTrip` in `internal/storage/postgres/postgres_test.go` (runs when `TEST_POSTGRES_DSN` is set).

See the main plan in `.cursor/plans/` and `docs/plans/` for phase-by-phase detail.

## Languages and tooling

- **Go** 1.24+
- **Node** 24+ for the admin UI
- **Svelte 5** (Runes) with **shadcn-svelte** and **Tailwind CSS**
- **Bun** ORM (`github.com/uptrace/bun`) for SQLite/PostgreSQL
- **gobwas/ws** for WebSocket upgrades
- **zerolog** for structured logging
- **btcec/v2** (Schnorr) for event ID and signature checks
- **Ginkgo / Gomega** for integration-style tests; **`testing`** for unit tests
- Benchmarks under `test/performance/` when applicable

## Development rules

1. **Git**: Prefer granular commits per logical change; messages should state *what* and *why* clearly.
2. **Go layout**: Application code lives under `internal/` (not intended as a public library).
3. **Storage**: All database access goes through the `Store` interface — no ad-hoc SQL in relay handlers.
4. **Admin**: Admin routes and server must respect `ENABLE_ADMIN_UI`; do not expose admin API when disabled.
5. **NIPs**: Implement NIPs by registering validators, post-store hooks, and message handlers via the NIP registry — avoid hard-coding optional behavior in core relay loops.
6. **NIP toggles**: Enabling/disabling optional NIPs updates config and requires a **relay restart**; no hot-reload of pipeline registration.
7. **Svelte**: Svelte 5 runes only; use shadcn-svelte patterns; Tailwind for styling.
8. **Environment vs JSON config**: Boot-time env vars are `CONGEE_ENV`, `ENABLE_ADMIN_UI`, `ADMIN_PASSWORD`, `CONFIG_PATH`, `RELAY_SECRETS_PATH`, `CONGEE_RELAY_PORT`, `CONGEE_ADMIN_PORT`, `CONGEE_DATA_DIR`, plus PostgreSQL `CONGEE_INSTANCE_ID` and test-only `TEST_POSTGRES_DSN` as documented. Optional port and SQLite data-dir vars override the loaded JSON before validation. Everything else belongs in the JSON config file. For local dev, an optional **`.env`** in the process working directory is loaded on startup (see `cmd/congee/main.go`); it does not override variables already set in the environment.
9. **Config format**: JSON (not YAML). Validate on load and before admin API writes.
10. **Audit & logs**: Use **full pubkeys** in logs (never truncate). Persist relay activity to `audit_log` with configurable retention.
11. **Logging style**: zerolog — production JSON, dev console when `CONGEE_ENV` is dev-like; lowercase terse messages; include `conn_id` on connection-scoped lines; `duration_ms` for DB/network; `.Err(err)` for errors.
12. **Config file writes**: Atomic — write temp file in same directory, then `os.Rename`; serialize concurrent writes with a mutex.
13. **SQLite**: WAL mode; all writes through a **single-writer goroutine** to avoid `SQLITE_BUSY`.
14. **Lint**: `go vet` and `golangci-lint` (see `Makefile`).

## Where to read next

- `README.md` — quick links
- `docs/getting-started.md` — build and run
- `docs/environment-variables.md` — env vars and config overview
- `docs/plans/phase*.md` — implementation steps per phase

## Agent instructions (follow fully)
- Do not make any assumptions (if you're thinking: "the user probably wants..." then you're doing it wrong)
- Ask any and all questions before continuing
- Create a branch for your changes
- Commit changes granularly with documentation on the request
- Refer to these rules always