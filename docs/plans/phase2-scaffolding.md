# Phase 2 — Project scaffolding

## Goals

- Go module, dependencies, `.gitignore`, `Makefile`, `Dockerfile`, `config.example.json`
- SvelteKit + Svelte 5 + Tailwind + shadcn-svelte skeleton in `web/admin/`
- Granular commits per logical step

## Steps

1. `go mod init github.com/michmich112/congee`
2. Add deps (pin compatible versions): `uptrace/bun`, `bun/dialect/sqlitedialect`, `bun/driver/sqliteshim`, `gobwas/ws`, `rs/zerolog`, `btcsuite/btcd/btcec/v2`, `onsi/ginkgo/v2`, `onsi/gomega`
3. `.gitignore`: `bin/`, `web/admin/node_modules/`, `web/admin/build/`, `web/admin/.svelte-kit/`, `*.db`, `*.db-wal`, `*.db-shm`, `dist/`, `.env`, `config.json` (optional: keep example only)
4. `config.example.json`: relay.port, admin.port, database (type, dsn/path), logging, audit retention, rate limits, connection limits, websocket compression, max subscription id length, nip11 block, nips.enabled array
5. `Makefile`: `build` → `mkdir -p bin && go build -o bin/congee ./cmd/congee`, `run`, `test`, `test-integration` (ginkgo), `test-perf` (placeholder or benchmarks), `lint` (go vet + golangci-lint if installed), `ui-dev`, `ui-build`, `docker-build`
6. `Dockerfile`: multi-stage — node:24 for `web/admin` npm ci + build; golang:1.24 build binary; minimal final image, `EXPOSE 3334 3335`, `ENTRYPOINT` binary, `VOLUME` hint for config
7. `web/admin`: SvelteKit, TypeScript, static adapter; add Tailwind; init shadcn-svelte; dark mode via `prefers-color-scheme`; placeholder layout and home page
8. `cmd/congee/main.go`: minimal `main` printing "congee" or no-op until Phase 4

## Validation

- `go build ./...` succeeds
- `make ui-build` produces `web/admin/build/`
- Docker image builds
