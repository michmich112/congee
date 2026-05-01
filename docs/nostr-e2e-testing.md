# Nostr E2E testing strategy (Congee)

This document defines how to validate relay behavior **beyond** unit tests: against a **running** Congee instance using real WebSocket + JSON messages, optionally with external Nostr clients or small harnesses.

Related plans (under `docs/plans/` locally, if present): relay identity secrets, NIP-42, NIP-29.

## Layers

### 1. Automated in-repo (CI, default)

- **`go test ./...`** — unit tests for parsers, validators, storage, relayidentity, config.
- **`test/integration`** (Ginkgo) — binds an ephemeral relay port, opens WebSocket connections, sends `EVENT` / `REQ` / `AUTH`, asserts `OK` / `EOSE` / `CLOSED` / `NOTICE`. This is the **primary regression net** for NIP-42 and core relay flows.

**When to extend:** add a new Ginkgo `Describe` per NIP or per user journey (e.g. NIP-29 private group `REQ` after `AUTH`) so CI stays green without manual steps.

### 2. Local manual smoke (operator / developer)

**Goal:** eyeball compatibility with real client expectations and TLS/WebSocket edge cases.

1. Build: `go build -o congee ./cmd/congee`
2. Config: use a dev `config.json` (SQLite DSN under `/tmp` or `./data`), enable the NIPs under test in `nips.enabled`, set `nip42.relay_url` to `ws://127.0.0.1:<relay-port>/` (or `wss://` behind a reverse proxy).
3. Optional: `RELAY_SECRETS_PATH=/tmp/congee-test-relay.secrets.json` for a disposable identity file.
4. Run: `./congee` (or `CONFIG_PATH=./config.json ./congee` when not using the container default `/data/config/config.json`).
5. **Clients:** point [Damus](https://damus.io/), [Amethyst](https://github.com/vitorpamplona/amethyst), [Snort](https://snort.social/), or [noStrudel](https://nostrudel.ninja/) at `ws://127.0.0.1:<port>/` (dev only; browsers may need `nip11.cors_allow_any_origin` for NIP-11 fetches).

**Checklist (examples):**

| Feature | Action | Pass criterion |
|--------|--------|----------------|
| Relay identity | Open admin dashboard | `npub` + hex pubkey match `GET /api/relay-identity` |
| NIP-11 | `curl -H 'Accept: application/nostr+json' http://127.0.0.1:<port>/` | JSON includes `pubkey`, `supported_nips` |
| NIP-42 | Connect WS; if challenge on connect, sign 22242; `REQ` protected kinds | `CLOSED` → `auth-required:` then success after `AUTH` |
| NIP-29 | Publish `h`-tagged event after 9007 bootstrap | Stored; `previous` invalid id → `OK` false; restricted group requires membership |

### 3. Programmatic external harness (optional repo artifact)

**Goal:** a **small Nostr client** (second process) that speaks the same wire protocol as Congee’s tests but is easy to run from the shell or in a separate CI job.

**Options:**

- **Go** — new module under `test/nostr-e2e/` using `github.com/gobwas/ws` (already in tree) or `nhooyr.io/websocket`: subcommands like `auth-flow`, `group-post`, reading relay URL from `CONGEE_WS_URL`.
- **Node** — `nostr-tools` + `ws`: quick scripts in `test/nostr-e2e/js/` with `npm install` and `node smoke.mjs`.
- **CLI tools** — [nak](https://github.com/fiatjaf/nak) or similar for scripted `REQ`/`EVENT` if installed on the runner.

**Contract:** harness assumes Congee is already running (started by Makefile target or CI service container). Environment:

- `CONGEE_WS_URL` — e.g. `ws://127.0.0.1:3334/`
- Optional: path to a test nsec for signing client events (never commit real keys; generate ephemeral keys in script).

**Suggested Makefile target (when harness exists):**

```text
run-congee-e2e:
	# start congee in background with test config, wait for port, run harness, kill congee
```

## CI recommendation

- **Keep** fast `go test ./...` on every push.
- **Optional second job:** build Congee, start with a temp config, run `test/nostr-e2e` binary with `CONGEE_WS_URL` — catches wiring bugs that only appear in `main` + full server stack.

## Mapping to features

| Feature | Layer 1 (Ginkgo/unit) | Layer 2 (manual) | Layer 3 (harness) |
|---------|----------------------|------------------|-------------------|
| Relay identity | `internal/relayidentity` tests; integration reconcile | Admin dashboard identity card; NIP-11 `pubkey` | `GET /api/relay-identity` vs file on disk |
| NIP-42 | AUTH + gated `REQ` integration | Client signs 22242; subscribe to kind 4 | Script: challenge → AUTH → REQ |
| NIP-29 | SQLite store queries; extend Ginkgo for private `REQ` | Create group, post with `h`, test `previous` | Script: full group timeline |

## Security note

Do not commit `relay.secrets.json` or client `nsec` files. Use temp paths and ephemeral keys for local and CI runs.
