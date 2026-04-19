# Congee

Congee is a Nostr relay written in Go with SQLite (default) or PostgreSQL storage, plus an optional Svelte 5 admin UI.

## Documentation

- [Getting started](docs/getting-started.md) — prerequisites, build, run (`make dev`), and connecting a client
- [Environment variables](docs/environment-variables.md) — env-only settings, optional `.env` file, and JSON config
- [AGENTS.md](AGENTS.md) — project context and conventions for contributors and automation

Phase implementation checklists live in [`docs/plans/`](docs/plans/).

## Run with Docker (GHCR)

Images are built in CI and pushed to GitHub Container Registry. The `nightly` tag tracks the latest successful build on `main`. Replace `ghcr.io/michmich112/congee` with `ghcr.io/<github-owner>/<repo>` if you use a fork or a different registry path.

1. Copy [`config.example.json`](config.example.json) to a host file (for example `./config.json`) and adjust ports, NIPs, and metadata as needed.
2. Run the container with a **writable** config mount if you plan to save settings from the admin UI (`PUT /api/config` fails when the file is read-only).

```bash
docker run -d --name congee \
  -p 3334:3334 -p 3335:3335 \
  -v "$PWD/config.json:/config/config.json" \
  -v congee-data:/data \
  -e CONFIG_PATH=/config/config.json \
  -e ENABLE_ADMIN_UI=true \
  -e ADMIN_PASSWORD=your-secure-password \
  ghcr.io/michmich112/congee:nightly
```

The image sets `CONGEE_DATA_DIR=/data` by default, so SQLite uses `/data/congee.db` on the `congee-data` volume. Optional overrides: `CONGEE_RELAY_PORT`, `CONGEE_ADMIN_PORT` (see [environment variables](docs/environment-variables.md)). The second port in `-p host:container` must match the **container** listen ports (from JSON or those env vars).

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
