# Environment variables

These variables are **outside** the JSON config file by design: they gate boot-time behavior or secrets that should not live in the mounted config file.

## `.env` file (local development)

If a file named **`.env`** exists in the **current working directory** when the binary starts, Congee loads it with [godotenv](https://github.com/joho/godotenv) before reading `CONFIG_PATH` or other settings. A missing `.env` is ignored (for example in production containers that inject env vars directly).

- Run from the repository root so `./.env` is found (`make dev` does this).
- Copy [`.env.example`](../.env.example) to `.env` and uncomment or set values. `.env` is gitignored.
- Variables **already set** in the parent environment are **not** replaced by `.env` (godotenv default).

| Variable | Purpose | Default / notes |
|----------|---------|------------------|
| `CONGEE_ENV` | Runtime mode | `production` if unset or empty. Values `dev`, `development`, or `local` enable dev behavior (e.g. admin proxy to Vite, console logs). Values `prod` or `production` force production behavior. |
| `ENABLE_ADMIN_UI` | Admin HTTP server | Default `false`. When `true`, starts the admin API and UI on the port from JSON config. |
| `ADMIN_PASSWORD` | Admin authentication | Required when admin UI is enabled (plaintext comparison at boundary — use HTTPS in production). |
| `CONFIG_PATH` | JSON config file path | Default `/data/config/config.json`. For local development from the repo, set `CONFIG_PATH=./config.json` (or another writable path) so the relay does not require a root-owned `/data` tree. |
| `RELAY_SECRETS_PATH` | Relay secp256k1 secrets file | Optional. Overrides the default path for `relay.secrets.json` (32-byte secret hex JSON). When unset, the file is `relay.secrets.json` **next to** the config file — with the default `CONFIG_PATH`, that is `/data/config/relay.secrets.json`. Created on first run if missing. |
| `CONGEE_RELAY_PORT` | Relay HTTP/WebSocket listen port | Optional. Default 3334. When set (decimal integer), overrides `relay.port` from JSON after load. Must be between 1 and 65535. |
| `CONGEE_ADMIN_PORT` | Admin HTTP listen port | Optional. Default 3335. When set, overrides `admin.port` from JSON after load. Must be between 1 and 65535. Ignored for binding when `ENABLE_ADMIN_UI` is not enabled, but the value still overrides the in-memory config. |
| `CONGEE_DATA_DIR` | Local Turso database directory | Optional. When set and `database.type` is empty, `sqlite`, or `turso`, sets `database.dsn` to `<dir>/congee.db` and `database.meta_dsn` to `<dir>/congee-meta.db` (after path cleaning). Empty/`sqlite` types are then rewritten to `turso` on boot. Ignored when `database.type` is `postgres`. The official container image sets this to `/data` by default; mount a volume there for persistence. |
| `CONGEE_INSTANCE_ID` | PostgreSQL multi-instance identity | Optional. When `database.type` is `postgres`, identifies this process in `LISTEN`/`NOTIFY` payloads so the relay does not re-broadcast its own writes. If unset, Congee generates a UUID on first start, writes it to `relay.instance_id` in the JSON config, and reuses it on later boots. Setting this variable overrides the config value for the running process and locks it from edits in the admin UI under **Config → Storage** (restart still applies after config changes elsewhere). |
| `TEST_POSTGRES_DSN` | Integration tests only | If set, enables PostgreSQL store/notifier tests (`go test`). Not used at runtime. |

Most relay behavior — logging level, audit retention, rate limits, connection limits, WebSocket compression, NIP-11 metadata, `nips.enabled`, shutdown timeouts, **plugins**, etc. — is configured in the JSON file referenced by `CONFIG_PATH`. Listen ports and the events/meta file paths can additionally be overridden at process start by the variables above (applied after JSON load, then the merged config is validated).

`nip11.icon_source` and `nip11.banner_source` choose how each image is published: `default` (built-in Congee art, the default for a new config), `upload` (a PNG, JPEG, or WebP stored in `nip11-assets/` beside the JSON config), or `url` (an absolute `http` or `https` address in `nip11.icon` or `nip11.banner`). In default and upload mode the NIP-11 document points at `GET /assets/icon` and `GET /assets/banner` on this relay. In url mode those routes return 404 and the document contains the external URL. Uploads are made under **Config → Relay** (`POST /api/relay-assets/icon` and `POST /api/relay-assets/banner`).

NIP-11 `self` is derived from the relay signing identity in `relay.secrets.json`. The optional `nip11.admin_pubkey` is a separate 32-byte hex administrator contact key, stored in lowercase and published as NIP-11 `pubkey`; leave it empty to omit the contact identity. The old `nip11.pubkey` config value was the relay's own key, so it is ignored (with one log warning on load) and removed on the next config save. It is not copied into `admin_pubkey`. The response derives `limitation.max_message_length`, `max_subscriptions`, `max_filters`, `max_subid_length`, and (when enabled) `default_limit` from enforced config values. `auth_required` is true only when NIP-42 is enabled and `nip42.require_auth` is `connect`; `protected_kinds` keeps ordinary requests open and reports `auth_required: false`. A legacy `nip42.send_challenge_on_connect` value loads as `protected_kinds`. Congee does not advertise a `max_limit` or `max_event_tags` cap because it does not enforce those caps.

Plugin subprocesses additionally receive `CONGEE_PLUGIN_SOCKET`, `CONGEE_PLUGIN_HOST_SOCKET`, `CONGEE_PLUGIN_DATA_DIR`, `CONGEE_PLUGIN_SETTINGS`, and `CONGEE_PLUGIN_ID` (set by the host; see [plugins.md](plugins.md)). Conduit tests use `CONDUIT_EMBEDDER=fake`. If that variable is unset, Conduit does not silently fall back to the fake embedder — vector rank stays off until ONNX loads.

JSON `database.meta_dsn` (optional) points at the local database file for operational metadata (`audit_log`, `config_changelog`, `relay_metric_buckets`, `ws_connection_sessions`). Meta uses the same tursogo driver as local events, including when events are in PostgreSQL. When omitted, Congee uses `congee-meta.db` beside `database.dsn` (or `./congee-meta.db` when `database.type` is `postgres`). This release still requires CGO because upgrading an events file from SQLite FTS5 uses go-libsql. See [turso.md](turso.md).

For PostgreSQL-specific settings and local Docker setup, see [docs/postgres.md](./postgres.md). For Turso/libSQL local storage, see [docs/turso.md](./turso.md).

See `config.example.json` for the full schema of JSON fields and sensible defaults.
