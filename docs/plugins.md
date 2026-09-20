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

Import the nested module (not the relay):

```bash
go get github.com/michmich112/congee/sdk/plugin@v0.1.0
```

```go
import sdk "github.com/michmich112/congee/sdk/plugin"
```

This pulls only the nested module zip (gRPC ABI + `Serve`), not Turso, the admin UI, or `internal/`. Git tags are **`sdk/plugin/vX.Y.Z`**; a root Congee `v1.2.3` release does not version the SDK. Local ABI iteration uses `replace` or [`go.work.example`](../go.work.example).

`plugin.json` may include **hooks** — extra argv on the same `exec` binary:

```json
"hooks": {
  "install": ["--hook=install"],
  "launch": ["--hook=launch"],
  "uninstall": ["--hook=uninstall"]
}
```

The host runs **install** once after unpack (does not fail the install if the hook errors; it logs a warning). **launch** runs once before the long-lived Serve process. **uninstall** runs after SIGTERM and **before** the package directory is deleted (30s timeout). Conduit uses install/launch to fetch MiniLM ONNX, tokenizer, and onnxruntime into `data/` (skipped when `CONDUIT_EMBEDDER=fake`), and uninstall to delete those downloaded blobs. Failed downloads are reported; the plugin never panics.

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

Installed under `plugins.directory` (or `$CONGEE_DATA_DIR/plugins` / beside `config.json`). In Docker this is **`/data/plugins`**. All plugin-owned files stay under that tree (never `/usr` or the image root):

```
/data/plugins/
  host.sock
  <id>/
    plugin.json
    bin/…
    ui/          # optional static Svelte build
    data/        # CONGEE_PLUGIN_DATA_DIR — indexes, secrets, downloaded models/libs
      models/    # Conduit: MiniLM + tokenizer (install hook)
      lib/       # Conduit: onnxruntime per GOOS_GOARCH (install hook)
```

`plugin.json` `exec` keys are `GOOS_GOARCH` (for example `darwin_arm64`).

**Install:** admin `POST /api/plugins/install` with `{ "url", "sha256" }` (sha256 is the **archive** checksum) or `{ "path" }` for a local directory (dev). Upgrade replaces `bin/` + `ui/` and **keeps `data/`**.

**Uninstall:** `POST /api/plugins/{id}/uninstall` with `{ "wipe_data": false|true }`. The host SIGTERMs the process, runs `hooks.uninstall`, then deletes `bin/` + `ui/`. `wipe_data: false` keeps `data/` (index/secrets) after the hook has removed plugin-managed deps (models, runtime). `wipe_data: true` deletes the entire `plugins/<id>/` tree. The config item is always removed.

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

Separate repo: `conduit-plugin`. Packages: `listing`, `embed`, `index`, `handler`. Default index is **Turso/libSQL** at `$CONGEE_PLUGIN_DATA_DIR/conduit-index.db`. Postgres is optional in the plugin UI (warns if the **relay** is already Postgres — split brain).

Depends only on `github.com/michmich112/congee/sdk/plugin` (`go get …@v0.1.0`). Local ABI work: copy `go.work.example` (do not commit a `go.work` that points at a missing sibling checkout).

On-device MiniLM (384-d) is linked against onnxruntime 1.21.0 (ORT C API 21, matching `yalue/onnxruntime_go` v1.19.0). Install/launch hooks download weights, tokenizer, and `libonnxruntime` into `data/models` and `data/lib/<goos>_<goarch>/`. `CONDUIT_EMBEDDER=fake` skips download and uses a bag-of-words embedder for tests only. A verified OpenAI-compatible HTTP provider that returns `embed_dim` floats offloads MiniLM after Test + Save. Uninstall hook deletes downloaded blobs; `wipe_data` also drops the index.

Kinds come from embedded `kinds.json`: NIP-15 `30017`/`30018`, NIP-99 `30402`/`30403`, NIP-09 kind `5`. Kind `34550` is a NIP-72 community definition, not a stall. Observe is **off**; indexing is `OnStoredEvent` + watermark backfill.

Install a GitHub Release tarball (per `GOOS_GOARCH`) via admin `POST /api/plugins/install` with the **archive** SHA-256 from the release notes.

## E2E

```bash
make test-plugin-e2e
```

Uses nostr-tools against a live binary; writes [`test/plugin-e2e/report.md`](../test/plugin-e2e/report.md).
