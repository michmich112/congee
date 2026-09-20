.PHONY: build run dev test test-integration test-perf lint ui-dev ui-build docker-build proto test-plugin-e2e

VERSION ?= 0.0.0-dev

build:
	mkdir -p bin && CGO_ENABLED=1 go build -ldflags "-X github.com/michmich112/congee/internal/version.Version=$(VERSION)" -o bin/congee ./cmd/congee

# Run relay from source plus Vite admin UI (HMR) in one terminal, with colored [relay]/[admin] prefixes.
# Loads ./.env automatically if present — see cmd/congee/main.go. Use CONGEE_ENV=dev so admin proxies to Vite.
dev:
	bash scripts/dev.sh

run: build
	./bin/congee

test:
	CGO_ENABLED=1 go test ./...
	cd sdk/plugin && go test ./...

test-integration:
	go run github.com/onsi/ginkgo/v2/ginkgo -r ./test/integration/...

test-perf:
	@echo "test-perf: placeholder (add benchmarks under test/performance/)"

proto:
	cd sdk/plugin && protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative pluginv1/plugin.proto

test-plugin-e2e: build
	mkdir -p bin && CGO_ENABLED=1 go build -o bin/congee-plugin-fixture ./cmd/congee-plugin-fixture
	cd test/plugin-e2e && npm install && node run.mjs

lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; fi

ui-dev:
	cd web/admin && npm run dev

ui-build:
	cd web/admin && npm ci && node ./node_modules/@sveltejs/kit/svelte-kit.js sync && npm run build

GIT_REVISION ?= $(shell git rev-parse HEAD 2>/dev/null)

docker-build:
	docker build -t congee:latest \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_REVISION=$(GIT_REVISION) \
		.
