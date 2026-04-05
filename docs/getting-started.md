# Getting started

## Prerequisites

- **Go** 1.24 or newer
- **Node.js** 24 or newer (for the admin UI build)
- A Nostr client that supports `wss://` or `ws://` (for local testing, use `ws://`)

## Clone and build

```bash
git clone <repository-url> congee
cd congee
make build
```

This produces `bin/congee`.

## Configuration

Copy the example config and edit paths, ports, and database settings:

```bash
cp config.example.json config.json
```

The active config path defaults to `./config.json` and can be overridden with the `CONFIG_PATH` environment variable.

Optional **local** environment (admin UI, dev mode, secrets) can live in a **`.env`** file in the project root — see [environment-variables.md](environment-variables.md). Copy `.env.example` to `.env` and adjust. The relay loads `.env` automatically when you start it from that directory.

## Run the relay

**Development (no `bin/` build):** from the repo root, with optional `.env` picked up automatically:

```bash
make dev
```

**Production-style binary:**

```bash
make run
# or after make build:
./bin/congee
```

By default the relay listens on the port set in `config.json` (see `config.example.json` — typically `3334`).

## Admin UI (optional)

Set `ENABLE_ADMIN_UI=true` and `ADMIN_PASSWORD` (see [environment-variables.md](environment-variables.md)). With `CONGEE_ENV=dev` (or `development` / `local`), the admin server **proxies** the browser to the Vite dev server on `http://127.0.0.1:5173`. Start Vite in a second terminal, or the UI will not load:

```bash
make ui-dev   # terminal 2 — Vite on :5173
make dev      # terminal 1 — relay + admin (with .env as needed)
```

If Vite is not running but you have already run **`make ui-build`**, the admin server **falls back** to `web/admin/build` and the UI still works (you will see a short warning in relay logs on the first failed proxy attempt).

For production, build the UI and serve static files from `web/admin/build/`:

```bash
make ui-build
ENABLE_ADMIN_UI=true ADMIN_PASSWORD=secret CONGEE_ENV=production ./bin/congee
```

## Connect a Nostr client

1. Start Congee with your `config.json` and note the WebSocket URL (e.g. `ws://127.0.0.1:3334/` if using default port and no TLS).
2. In your Nostr client, add a custom relay with that URL.
3. Publish or subscribe to events as supported by your client and the relay’s enabled NIPs.

TLS termination is expected to be handled by a reverse proxy (Caddy, nginx, etc.) in production; the binary serves plain HTTP/WebSocket by default.

## Tests and lint

```bash
make test
make test-integration
make lint
```

## Docker

```bash
make docker-build
```

Mount your `config.json` (or config directory) into the container as documented in the `Dockerfile` comments.
