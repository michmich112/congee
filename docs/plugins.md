# Plugins

Congee can run **isolated plugin processes** beside the relay. Plugins speak gRPC + protobuf (`congee.plugin.v1`) over **Unix domain sockets**, reconnect after crashes, and never share the relay address space.

## Listen vs intercept

- **Listen** (`messages.observe`, `index.own` / `OnStoredEvent`) is fire-and-forget. After an in-process subscription match, the relay goroutine only does a non-blocking enqueue (`select` / default drop). A background worker performs gRPC. Queue-full or plugin-down drops never fail the client. `OnStoredEvent` drops are healed by plugin backfill; pure observe drops are best-effort (metrics).
- **Intercept** (`req.intercept`) is the **only** synchronous plugin call. It runs on `REQ` **before** `subs.Add`, with a deadline. Not ready, timeout, or RPC error → **fail-open passthrough**.
- Host matches **subscriptions** before enqueue/RPC. Empty `kinds` / `message_types` match nothing (opt-in traffic).

Host deadline is `min(plugins.intercept_timeout_ms, handshake intercept_deadline_ms)`. Default / example ceiling is **250ms** (Conduit handshake asks for 200ms).

## ABI

Schema: [`sdk/plugin/proto` via `sdk/plugin/pluginv1/plugin.proto`](../sdk/plugin/pluginv1/plugin.proto). Nested Go module `github.com/michmich112/congee/sdk/plugin`.

Regenerate:

```bash
make proto
```

Handshake `api_version` must be **1** or the host rejects the plugin.

`plugin.json` may include **hooks** — extra argv on the same `exec` binary:

```json
"hooks": {
  "install": ["--hook=install"],
  "launch": ["--hook=launch"]
}
```

The host runs **install** once after unpack (does not fail the install if the hook errors; it logs a warning). **launch** runs once before the long-lived Serve process. Timeouts are 15 minutes so a model download can finish. Conduit uses these hooks to fetch MiniLM ONNX and onnxruntime into `data/` (skipped when `CONDUIT_EMBEDDER=fake`). Failed downloads are reported; the plugin never panics.

Spawn env (set by the host):

| Variable | Meaning |
| --- | --- |
| `CONGEE_PLUGIN_SOCKET` | Plugin listens here (`plugin.sock`) |
| `CONGEE_PLUGIN_HOST_SOCKET` | Plugin dials the shared `host.sock` |
| `CONGEE_PLUGIN_DATA_DIR` | Per-plugin `data/` (indexes, secrets) |
| `CONGEE_PLUGIN_SETTINGS` | JSON settings snapshot |
| `CONGEE_PLUGIN_ID` | Config id |
| `CONGEE_PLUGIN_PACKAGE_DIR` | Installed package root (parent of `bin/`) |

The host creates **`host.sock` first**, then spawns the process. Plugin stdout/stderr are captured into zerolog with `plugin_id`.

State machine: `Starting` → `Ready` → `Degraded` → backoff. Only **Ready** may intercept.

## Package layout

Installed under `plugins.directory` (or `$CONGEE_DATA_DIR/plugins` / beside `config.json`):

```
<id>/
  plugin.json
  bin/…
  models/      # Conduit: minilm.onnx
  lib/         # Conduit: onnxruntime per GOOS_GOARCH
  ui/          # optional static Svelte build
  data/        # kept across upgrade/uninstall unless wipe
```

`plugin.json` `exec` keys are `GOOS_GOARCH` (for example `darwin_arm64`).

**Install:** admin `POST /api/plugins/install` with `{ "url", "sha256" }` (sha256 required) or `{ "path" }` for a local directory (dev). Upgrade replaces `bin/` + `ui/` and **keeps `data/`**. Uninstall SIGTERM, then deletes the package; `data/` stays unless `wipe_data`.

## Config

```json
"plugins": {
  "directory": "",
  "intercept_timeout_ms": 250,
  "items": []
}
```

Plugin DB passwords in settings are redacted in the config changelog. Conduit stores the Postgres password in `data/secrets.json` mode `0600`.

## Admin UI

Nav **Plugins**: expandable sidebar (Manage plus each installed plugin). Table kebab: Settings, enable/disable, uninstall. Detail page embeds `GET /plugin-ui/{id}/` in a sandboxed iframe (`allow-scripts allow-forms`, **no** `allow-same-origin`). Module assets send `Access-Control-Allow-Origin` for opaque origin `null`. The parent bridges `postMessage` `{ type: "congee:plugin-api" }` to `/api/plugins/{id}/*` only (settings via `GET/PUT /api/plugins/{id}/settings`), checks `event.origin` (including `"null"`), and never puts the admin token in the iframe. Theme: `{ type: "congee:theme", theme }`.

## Intercept actions

`passthrough` | `reshape_req` (replace filters) | `respond` (ordered event ids; host hydrates via `GetEventsByIDs` **preserving order** and still applying visibility). Live subscription filters may drop `search` so fan-out is not FTS-ranked.

## Metrics

`listen_enqueued`, `listen_dropped`, `intercept_n`, plus passthrough counters for not-ready / timeout / error on `GET /api/plugins`.

## Conduit marketplace plugin

Separate repo: `conduit-plugin`. Packages: `listing`, `embed`, `index`, `handler`. Default index is **Turso/libSQL** at `$CONGEE_PLUGIN_DATA_DIR/conduit-index.db`. Postgres is optional in the plugin UI (warns if the **relay** is already Postgres — split brain). Fake embedder: `CONDUIT_EMBEDDER=fake` (required to use the test bag-of-words model; otherwise a missing/unlinked ONNX build disables vector rank). Vector width is `embed_dim` (default 384). A verified OpenAI-compatible HTTP provider that returns that many floats offloads MiniLM after Test + Save. Changing `embed_dim` or the embedder rebuilds the index. On-device assets are downloaded by install/launch hooks into `data/models` and `data/lib/<goos>_<goarch>/` (URLs configurable in Indexes). Kinds come from `kinds.json`: NIP-15 `30017`/`30018`, NIP-99 `30402`/`30403`, NIP-09 kind `5`. Kind `34550` is a NIP-72 community definition, not a stall. Observe is **off**; indexing is `OnStoredEvent` + watermark backfill.

Local SDK development: in `conduit-plugin`, `go.work` uses `../congee/sdk/plugin`. From Congee, `go.work.example` can span both modules — do not commit a `go.work` that points at a missing sibling repo (breaks CI).

## E2E

```bash
make test-plugin-e2e
```

Uses nostr-tools against a live binary; writes [`test/plugin-e2e/report.md`](../test/plugin-e2e/report.md).
