<p align="center">
  <img src="docs/assets/congee-logo.svg" alt="Congee logo" width="120" height="141" />
</p>

# Congee

Congee is a Nostr relay written in Go with Turso/libSQL (default), optional SQLite, or PostgreSQL storage, plus an optional Svelte 5 admin UI.

## Documentation

- [Getting started](docs/getting-started.md) — prerequisites, build, run (`make dev`), and connecting a client
- [Environment variables](docs/environment-variables.md) — env-only settings, optional `.env` file, and JSON config
- [AGENTS.md](AGENTS.md) — project context and conventions for contributors and automation

Phase implementation checklists live in [`docs/plans/`](docs/plans/).

## Run with Docker (GHCR)

Images are built in CI and pushed to GitHub Container Registry. The `nightly` tag tracks the latest successful build on `main`. Replace `ghcr.io/michmich112/congee` with `ghcr.io/<github-owner>/<repo>` if you use a fork or a different registry path.

1. Optional: seed [`config.example.json`](config.example.json) into the volume if you want non-default settings before the first start. Otherwise the relay creates **`/data/config/config.json`** with defaults on first boot (same directory holds **`relay.secrets.json`**).
2. Mount a **writable** `/data` volume so the database, config, and secrets persist (`PUT /api/config` fails when the config file is read-only).

```bash
docker run -d --name congee \
  -p 3334:3334 -p 3335:3335 \
  -v congee-data:/data \
  -e ENABLE_ADMIN_UI=true \
  -e ADMIN_PASSWORD=your-secure-password \
  ghcr.io/michmich112/congee:nightly
```

You do **not** need `CONFIG_PATH`, `RELAY_SECRETS_PATH`, or `CONGEE_DATA_DIR` unless you want non-default locations. The image sets `CONGEE_DATA_DIR=/data` by default, so the default Turso database uses `/data/congee.db`. Optional overrides: `CONGEE_RELAY_PORT`, `CONGEE_ADMIN_PORT` (see [environment variables](docs/environment-variables.md)). The second port in `-p host:container` must match the **container** listen ports (from JSON or those env vars).

**Binary version** (no relay start):

```bash
docker run --rm ghcr.io/michmich112/congee:nightly congee version
```

**OCI labels** (includes `org.opencontainers.image.revision` for the git commit used in CI):

```bash
docker image inspect ghcr.io/michmich112/congee:nightly --format '{{json .Config.Labels}}'
```

If your host is Apple Silicon and the published image is `linux/amd64` only, add `--platform linux/amd64` to `docker run` until multi-arch builds are enabled in CI.

## License

See repository license when added.
