# Phase 5 — Admin system

## Goals

- Standalone admin HTTP server on configurable port (from JSON), only when `ENABLE_ADMIN_UI=true`
- Password middleware (`ADMIN_PASSWORD`)
- Config GET/PUT with validation, atomic write, mutex, changelog DB row
- Audit query/filter/pagination
- NIP list + toggle optional (writes config; response flags restart required)
- `internal/config/changelog.go` helpers
- Svelte pages: dashboard, audit, config (+ changelog), nips
- `CONGEE_ENV`: dev → proxy Vite `:5173`; prod → `web/admin/build/` static
- Wire `main.go` to start/stop admin with relay on SIGTERM

## Admin API (`internal/admin/`)

1. **server.go** — Listen on `admin.port`; mount API routes; static or proxy per env.
2. **middleware.go** — Basic auth or `Authorization: Bearer` / header agreed in implementation — document in code; compare to `ADMIN_PASSWORD`.
3. **routes_config.go** — GET raw JSON; PUT body unmarshaled, validated via same as startup, atomic rename, `SaveConfigChange`.
4. **routes_audit.go** — List with filters + limit/offset or cursor.
5. **routes_nips.go** — GET all metadata + enabled state; PATCH toggle optional NIPs only.

## UI (`web/admin/`)

1. **+layout.svelte** — Nav, dark mode from system, auth gate.
2. **+page.svelte** — Dashboard stats (placeholders OK if APIs added incrementally).
3. **audit/+page.svelte** — Table + filters.
4. **config/+page.svelte** — JSON editor + changelog list.
5. **nips/+page.svelte** — Toggles + restart banner.

## Validation

- With `ENABLE_ADMIN_UI=true`, admin port serves UI and APIs
- Invalid config PUT returns 400 with error body
- Atomic config file behavior under concurrent PUT (serialized)
