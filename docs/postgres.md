# PostgreSQL storage

Congee can use **SQLite** or **PostgreSQL** for persistence. Set this in the JSON config:

```json
"database": {
  "type": "postgres",
  "dsn": "postgres://user:password@127.0.0.1:5432/congee?sslmode=disable"
}
```

- `database.type` must be `"postgres"` (or omit/`"turso"` for the default local backend; use `"sqlite"` for modernc SQLite without CGO).
- `database.dsn` is a standard PostgreSQL URL understood by the Bun `pgdriver` (TLS/query params as supported by the driver).

## Multi-instance fan-out

When using PostgreSQL, each relay process sends `NOTIFY new_event` with payload `{"id":"<hex>","origin":"<instance_id>"}` after a successful `SaveEvent`. Other instances `LISTEN` on the same channel, ignore their own `origin`, load the event by id, and broadcast it to active REQ subscriptions.

Give each relay process a **stable** origin id so local writes are not double-delivered to subscribers:

- Prefer **`relay.instance_id`** in the JSON config (auto-generated and persisted on first start when `CONGEE_INSTANCE_ID` is unset). You can change it from the admin UI (**Config → Storage**) when the environment variable is not set; a restart is required for the running PostgreSQL listener to pick up a new id.
- Optionally set **`CONGEE_INSTANCE_ID`** in the environment to force the id for that process; it overrides the config value at runtime and cannot be changed from the UI.

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

`TestPostgresManyTagsJSONBRoundTrip` saves a kind **5**-style event with many `e` tags and asserts `SaveEvent` + `QueryEvents` round-trips `event_tags.full_json` as a JSON **array** (guards against Bun double-encoding JSONB when using a plain `string` field).

```bash
TEST_POSTGRES_DSN='postgres://postgres:congee@127.0.0.1:5432/congee?sslmode=disable' go test ./internal/storage/postgres/ -count=1
```
