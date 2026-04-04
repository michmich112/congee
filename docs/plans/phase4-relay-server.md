# Phase 4 — Relay server + pipeline

## Goals

- Full relay: WebSocket, registry, validation chain, hooks, subscriptions, limits, NIP-11, health, audit, main wiring
- NIP-01 registered via loader; integration + fuzz tests
- Granular commits

## Relay (`internal/relay/`)

1. **registry.go** — Map message type string → handler.
2. **middleware.go** — Ordered `EventValidator` chain; NIP-01 registers ID + signature.
3. **hooks.go** — Post-store hooks; NIP-01: audit + fan-out to subscriptions.
4. **limiter.go** — Per-IP and per-connection limits; max subs; max filters per REQ; max subscription ID length (default 64).
5. **relay.go** — HTTP server, WS upgrade (gobwas/ws), graceful shutdown, NIP-11 + health only on relay listener.
6. **handler.go** — Connection ID (short hex), reader/writer, buffered writes, ping/pong, backpressure policy, optional permessage-deflate.
7. **subscription.go** — Per-conn subs + broadcast; `sync.RWMutex`.
8. **nip11.go** — `Accept: application/nostr+json` on root GET; merge config + enabled NIPs.
9. **health.go** — `GET /health` when DB and listener ready.

## NIPs (`internal/nips/`)

1. **registry.go** — NIP metadata: number, description, GitHub URL, mandatory flag.
2. **loader.go** — Read `nips.enabled`, register components for enabled NIPs.

## Audit (`internal/audit/`)

1. **audit.go** — Write entries with connection id context.
2. **cleanup.go** — Periodic purge by retention.

## Entry (`cmd/congee/main.go`)

- Phase 4: load config, open storage, register NIPs, start relay, **graceful shutdown** (signal handling, drain). Do **not** start the admin server here unless a minimal stub is required to compile; Phase 5 wires `ENABLE_ADMIN_UI` and the real admin HTTP server into `main`.

## Tests

- Unit: limiter, validators, subscription manager, sub ID length.
- Ginkgo: EVENT/OK, REQ/EVENT/EOSE, CLOSE, replaceable, NIP-11, health.
- Fuzz/malformed: oversized, bad UTF-8, deep nesting; assert no panic, correct NOTICE/CLOSED/OK false.

## Validation

- `ginkgo` integration suite green
- Manual `wscat` or nostr client smoke test documented
