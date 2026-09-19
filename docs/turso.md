# Turso (libSQL) storage

Congee stores local files with **Turso/libSQL** via [go-libsql](https://github.com/tursodatabase/go-libsql). This is the default engine (`database.type` is `"turso"`). Set this in the JSON config:

```json
"database": {
  "type": "turso",
  "dsn": "./congee.db",
  "meta_dsn": "./congee-meta.db"
}
```

- `database.type` must be `"turso"` (or leftover `"sqlite"` / empty, which is rewritten to `"turso"` on boot and on admin config writes).
- `database.dsn` is a **local file path** for the libSQL events database.
- Operational metadata (`audit_log`, `config_changelog`, metrics, WS sessions) is also libSQL, in `meta_dsn` (`congee-meta.db`).

SQLite (modernc) is no longer a supported engine. Existing on-disk files keep the same path; Congee opens them with go-libsql.

## Build requirements

Every Congee binary requires CGO and a C toolchain (gcc/clang), including PostgreSQL-only hosts, because meta uses go-libsql. Build with `CGO_ENABLED=1`. Supported platforms match go-libsql: linux/darwin amd64 and arm64.

The official Docker image enables CGO in the build stage. `make build` and `make test` set `CGO_ENABLED=1`.

## Migrating leftover SQLite configs

SQLite and libSQL share the same on-disk format for Congee's schema. Do **not** copy the file. On first boot of this version, `database.type` of `""` or `"sqlite"` is set to `"turso"` and written back atomically (same DSN). If libsql cannot open a leftover WAL file, boot fails with a clear error.

Row-by-row `storage.Migrate` remains for **postgres ↔ turso** via the admin UI (**Config → Storage**).

## Out of scope (v1)

- Turso Cloud embedded replicas and remote sync
- Remote-only libsql wire protocol (no local file)

See also [environment-variables.md](environment-variables.md) and [postgres.md](postgres.md) for other backends.
