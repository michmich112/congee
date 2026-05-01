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
| `CONGEE_DATA_DIR` | SQLite database directory | Optional. When set and `database.type` is empty or `sqlite`, sets `database.dsn` to `<dir>/congee.db` (after path cleaning). Ignored when `database.type` is `postgres`. The official container image sets this to `/data` by default; mount a volume there for persistence. |
| `CONGEE_INSTANCE_ID` | PostgreSQL multi-instance identity | Optional. When `database.type` is `postgres`, identifies this process in `LISTEN`/`NOTIFY` payloads so the relay does not re-broadcast its own writes. If unset, Congee generates a UUID on first start, writes it to `relay.instance_id` in the JSON config, and reuses it on later boots. Setting this variable overrides the config value for the running process and locks it from edits in the admin UI (restart still applies after config changes elsewhere). |
| `TEST_POSTGRES_DSN` | Integration tests only | If set, enables PostgreSQL store/notifier tests (`go test`). Not used at runtime. |

Most relay behavior — logging level, audit retention, rate limits, connection limits, WebSocket compression, NIP-11 metadata, `nips.enabled`, shutdown timeouts, etc. — is configured in the JSON file referenced by `CONFIG_PATH`. Listen ports and the SQLite file path can additionally be overridden at process start by the variables above (applied after JSON load, then the merged config is validated).

For PostgreSQL-specific settings and local Docker setup, see [docs/postgres.md](./postgres.md).

See `config.example.json` for the full schema of JSON fields and sensible defaults.
