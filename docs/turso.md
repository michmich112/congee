# Turso (libSQL) storage

Congee can use **Turso/libSQL** for local on-disk event storage via [go-libsql](https://github.com/tursodatabase/go-libsql). Set this in the JSON config:

```json
"database": {
  "type": "turso",
  "dsn": "./congee.db",
  "meta_dsn": "./congee-meta.db"
}
```

- `database.type` must be `"turso"`.
- `database.dsn` is a **local file path** for the libSQL database (same layout as SQLite events schema).
- Operational metadata (`audit_log`, `config_changelog`, metrics, WS sessions) stays in the **SQLite meta sidecar** (`meta_dsn`), like PostgreSQL.

## Build requirements

The Turso driver uses CGO and links libSQL native libraries. Build with `CGO_ENABLED=1` and a C toolchain (gcc/clang). Supported platforms match go-libsql: linux/darwin amd64 and arm64.

The official Docker image enables CGO in the build stage. Local `make build` uses CGO by default on macOS and Linux when gcc is available.

## Migrating from SQLite

SQLite and libSQL share the same on-disk format for Congee's schema. When migrating **sqlite → turso** via the admin UI (**Config → Storage**), Congee uses SQLite's native **`VACUUM INTO`** command to copy the live source database to the target path atomically (WAL-safe). This is faster than row-by-row copy and preserves the full database file.

Requirements:

- Source must match the running relay's configured `database.type` and `database.dsn`.
- Target path should not already contain a populated database. Admin target preflight does **not** open/create a missing Turso path (opening libSQL would create an empty shell file and break `VACUUM INTO`). Empty zero-byte leftovers from an older preflight are removed automatically.
- Other migration pairs (e.g. postgres → turso) use the row-by-row admin migration tool.

## Out of scope (v1)

- Turso Cloud embedded replicas and remote sync
- Remote-only libsql wire protocol (no local file)

See also [environment-variables.md](environment-variables.md) and [postgres.md](postgres.md) for other backends.
