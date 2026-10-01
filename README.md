<p align="center">
  <img src="docs/assets/congee-logo.svg" alt="Congee logo" width="120" height="141" />
</p>

# Congee

Congee is a Nostr relay written in Go with Turso/libSQL (default) or PostgreSQL storage, plus an optional Svelte 5 admin UI.

## Documentation

- [Getting started](docs/getting-started.md) — prerequisites, build, run (`make dev`), and connecting a client
- [Environment variables](docs/environment-variables.md) — env-only settings, optional `.env` file, and JSON config
- [Plugins](docs/plugins.md) — gRPC plugin host, listen vs intercept, Conduit marketplace plugin
- [Plugin architecture](docs/plugin-architecture.md) — host vs plugin binary; intercept log is host-side
- [NIP-77 negentropy syncing](docs/nip77.md) — enable, limits, upstream pull, observability
- [AGENTS.md](AGENTS.md) — project context and conventions for contributors and automation

Phase implementation checklists live in [`docs/plans/`](docs/plans/).

## Run with Docker (GHCR)

Images are built in CI and pushed to GitHub Container Registry for `linux/amd64` and `linux/arm64`. Replace `ghcr.io/michmich112/congee` with `ghcr.io/<github-owner>/<repo>` if you use a fork or a different registry path.

The repo-root `VERSION` file is the release number (`X.Y.Z`). CI stamps that string into the binary and the admin UI. Channel builds append a suffix so the image tag and the version shown in the admin UI match.

| Image tags | Branch | Version shown in the image | GitHub release |
| --- | --- | --- | --- |
| `latest`, `X.Y.Z` | `main` | `X.Y.Z` | `vX.Y.Z` on the first push of that version |
| `rc`, `X.Y.Z-rc` | `rc` | `X.Y.Z-rc` | pre-release `vX.Y.Z-rc` on the first push of that version |
| `nightly`, `X.Y.Z-nightly` | `rc` | `X.Y.Z-nightly` | none (image tags only) |

Later pushes of the same `VERSION` move the image tags and do not open another GitHub release. Bump the number with `make bump-version PART=patch` (or `minor` / `major`), which updates `VERSION` and the admin package together.

1. Optional: seed [`config.example.json`](config.example.json) into the volume if you want non-default settings before the first start. Otherwise the relay creates **`/data/config/config.json`** with defaults on first boot (same directory holds **`relay.secrets.json`**).
2. Mount a **writable** `/data` volume so the libSQL databases, config, and secrets persist (`PUT /api/config` fails when the config file is read-only).

```bash
docker run -d --name congee \
  -p 3334:3334 -p 3335:3335 \
  -v congee-data:/data \
  -e ENABLE_ADMIN_UI=true \
  -e ADMIN_PASSWORD=your-secure-password \
  ghcr.io/michmich112/congee:nightly
```

You do **not** need `CONFIG_PATH`, `RELAY_SECRETS_PATH`, or `CONGEE_DATA_DIR` unless you want non-default locations. The image sets `CONGEE_DATA_DIR=/data` by default, so libSQL uses `/data/congee.db`. Optional overrides: `CONGEE_RELAY_PORT`, `CONGEE_ADMIN_PORT` (see [environment variables](docs/environment-variables.md)). The second port in `-p host:container` must match the **container** listen ports (from JSON or those env vars).

**Binary version** (no relay start):

```bash
docker run --rm ghcr.io/michmich112/congee:nightly congee version
```

**OCI labels** (includes `org.opencontainers.image.revision` for the git commit used in CI):

```bash
docker image inspect ghcr.io/michmich112/congee:nightly --format '{{json .Config.Labels}}'
```

Published images include `linux/amd64` and `linux/arm64`.

## License

Congee is licensed under the [MIT License](LICENSE).
