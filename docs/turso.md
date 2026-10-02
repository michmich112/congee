# Turso storage

Congee stores local files with [tursogo](https://turso.tech/database/tursogo) (`database.type` is `"turso"`). The file stays in WAL mode: one writer, and a pool of readers. Set this in the JSON config:

```json
"database": {
  "type": "turso",
  "dsn": "./congee.db",
  "meta_dsn": "./congee-meta.db"
}
```

- `database.type` must be `"turso"` (or leftover `"sqlite"` / empty, which is rewritten to `"turso"` on boot and on admin config writes).
- `database.dsn` is a **local file path** for the events database.
- Operational metadata (`audit_log`, `config_changelog`, metrics, WS sessions) is in `meta_dsn` (`congee-meta.db`).
- `database.analyze` defaults to false. When true, the process periodically runs `ANALYZE` so the admin dashboard can show on-disk size and approximate row counts. While it is false, the dashboard tells the operator to turn analysis on under Config → Storage. A restart applies the change.

## Upgrading an existing database

Stop the running relay before starting this version. The container entrypoint is still `congee`. Schema upgrade and the FTS5 removal run inside database open, before the relay listens. If that work fails, the process exits 1 and the container stays stopped.

On an events file that still has the SQLite FTS5 table `event_fts`:

1. The process checkpoints the WAL and copies the main file to `<db>.pre-v8.bak` (fsynced, and never overwritten).
2. It drops only `event_fts` and its three triggers, inside a transaction. Event rows and tag rows stay. A count mismatch rolls the drop back and the process exits.
3. It builds a Turso full-text index on `events.content`, checks that an existing row is searchable, then sets `user_version` to 8.

The meta file gets the same checkpoint and copy, at `<meta>.pre-v8.bak`, and the process exits if row counts change on the first open. Leave both backup files until you have a newer backup of the upgraded databases. The process does not delete them.

`internal/storage/ftsv7detach` exists only for this upgrade. Remove that package and the go-libsql dependency when a release no longer opens events files below schema version 8.

### Restore

1. Stop the process.
2. Move the live events file and any `-wal` / `-shm` aside.
3. Copy `<db>.pre-v8.bak` onto the live path. That file is the pre-upgrade database, FTS5 included.
4. Start the previous image, or start this image again so the upgrade runs on the restored copy.

Do not open the backup path while the live file is still in place.

Search after the upgrade still treats the query as one phrase. English stemming is gone: `running` does not match `run`. Tokens longer than 40 characters are not indexed.

## Build requirements

This release still requires CGO (`CGO_ENABLED=1`) and a C toolchain, because the v7 FTS5 upgrade uses go-libsql inside the same `congee` binary. Supported platforms remain linux/darwin amd64 and arm64.

Published tursogo builds omit the full-text index method. `make test`, CI, and the container image run `scripts/overlay-turso-fts.sh`, which replaces the embedded library with `turso_sync_sdk_kit` built with the `fts` feature. NIP-50 search uses that index (`fts_match` / `fts_score`).

The official Docker image enables CGO in the build stage. `make build` and `make test` set `CGO_ENABLED=1`. `ENTRYPOINT` is `congee`.

## Migrating leftover SQLite configs

On first boot, `database.type` of `""` or `"sqlite"` is set to `"turso"` and written back atomically (same DSN). This version opens that file with tursogo after the upgrade above.

Row-by-row `storage.Migrate` remains for **postgres ↔ turso** via the admin UI (**Config → Storage**). A replaceable event that loses NIP-01 ordering is skipped, not deleted from the destination.

## Out of scope

- MVCC and `BEGIN CONCURRENT`
- Turso Cloud sync
- Remote-only access with no local file

See also [environment-variables.md](environment-variables.md) and [postgres.md](postgres.md) for other backends.
