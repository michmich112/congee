# Environment variables

These variables are **outside** the JSON config file by design: they gate boot-time behavior or secrets that should not live in the mounted config file.

| Variable | Purpose | Default / notes |
|----------|---------|------------------|
| `CONGEE_ENV` | Runtime mode | `production` if unset or empty. Values `dev`, `development`, or `local` enable dev behavior (e.g. admin proxy to Vite, console logs). Values `prod` or `production` force production behavior. |
| `ENABLE_ADMIN_UI` | Admin HTTP server | Default `false`. When `true`, starts the admin API and UI on the port from JSON config. |
| `ADMIN_PASSWORD` | Admin authentication | Required when admin UI is enabled (plaintext comparison at boundary — use HTTPS in production). |
| `CONFIG_PATH` | JSON config file path | Default `./config.json`. |
| `CONGEE_INSTANCE_ID` | PostgreSQL multi-instance identity | Optional. When `database.type` is `postgres`, identifies this process in `LISTEN`/`NOTIFY` payloads so the relay does not re-broadcast its own writes. Default: `hostname` + UUID. |
| `TEST_POSTGRES_DSN` | Integration tests only | If set, enables PostgreSQL store/notifier tests (`go test`). Not used at runtime. |

All other relay behavior — listen ports, database DSN, logging level, audit retention, rate limits, connection limits, WebSocket compression, NIP-11 metadata, `nips.enabled`, shutdown timeouts, etc. — is configured in the JSON file referenced by `CONFIG_PATH`.

For PostgreSQL-specific settings and local Docker setup, see [docs/postgres.md](./postgres.md).

See `config.example.json` for the full schema of JSON fields and sensible defaults.
