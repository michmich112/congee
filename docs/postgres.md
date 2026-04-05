# PostgreSQL storage

Congee can use **SQLite** (default) or **PostgreSQL** for persistence. Set this in the JSON config:

```json
"database": {
  "type": "postgres",
  "dsn": "postgres://user:password@127.0.0.1:5432/congee?sslmode=disable"
}
```

- `database.type` must be `"postgres"` (or omit/`"sqlite"` for SQLite).
- `database.dsn` is a standard PostgreSQL URL understood by the Bun `pgdriver` (TLS/query params as supported by the driver).

## Multi-instance fan-out

When using PostgreSQL, each relay process sends `NOTIFY new_event` with payload `{"id":"<hex>","origin":"<instance_id>"}` after a successful `SaveEvent`. Other instances `LISTEN` on the same channel, ignore their own `origin`, load the event by id, and broadcast it to active REQ subscriptions.

Set a stable **`CONGEE_INSTANCE_ID`** per process so local writes are not double-delivered to subscribers. If unset, Congee uses `hostname` + a random UUID (unique per boot, not ideal for long-lived clustered identity).

## Admin migration API

With the admin UI enabled, `POST /api/migration/start` accepts JSON:

```json
{
  "source": { "type": "sqlite", "dsn": "./congee.db" },
  "target": { "type": "postgres", "dsn": "postgres://..." }
}
```

The response is **SSE** (`text/event-stream`): `progress` events carry `percent` and `message`; `done` or `error` complete the stream. Paths and DSNs are resolved on the **server** (the admin service host), not in the browser.

## Local PostgreSQL for development

Example Docker run:

```bash
docker run --rm -d --name congee-pg \
  -e POSTGRES_PASSWORD=congee \
  -e POSTGRES_DB=congee \
  -p 5432:5432 \
  postgres:16
```

DSN:

`postgres://postgres:congee@127.0.0.1:5432/congee?sslmode=disable`

Stop: `docker stop congee-pg`.

## Tests

Integration tests for the PostgreSQL store and notifier run only when **`TEST_POSTGRES_DSN`** is set to a reachable database (schema is created automatically). Without it, those tests are skipped; `go test ./...` still passes.

```bash
TEST_POSTGRES_DSN='postgres://postgres:congee@127.0.0.1:5432/congee?sslmode=disable' go test ./internal/storage/postgres/ -count=1
```
