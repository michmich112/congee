.PHONY: build run dev test test-integration test-perf lint ui-dev ui-build docker-build proto test-plugin-e2e test-nip77-strfry-e2e bump-version check-version

VERSION ?= $(shell tr -d '[:space:]' < VERSION)

build:
	mkdir -p bin && CGO_ENABLED=1 go build -ldflags "-X github.com/michmich112/congee/internal/version.Version=$(VERSION)" -o bin/congee ./cmd/congee

# Run relay from source plus Vite admin UI (HMR) in one terminal, with colored [relay]/[admin] prefixes.
# Loads ./.env automatically if present — see cmd/congee/main.go. Use CONGEE_ENV=dev so admin proxies to Vite.
dev:
	bash scripts/dev.sh

run: build
	./bin/congee

.PHONY: overlay-turso-fts
overlay-turso-fts:
	./scripts/overlay-turso-fts.sh

test: overlay-turso-fts
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

test-nip77-strfry-e2e: build
	CGO_ENABLED=1 go test -tags e2e -timeout 10m -count=1 ./test/nip77-strfry-e2e/

lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; fi

ui-dev:
	cd web/admin && PUBLIC_CONGEE_VERSION=$(VERSION) npm run dev

check-version:
	bash scripts/check-version.sh

# PART=patch|minor|major. Updates VERSION and web/admin package.json / package-lock.json together.
bump-version:
	bash scripts/bump-version.sh $(PART)

ui-build:
	cd web/admin && npm ci && node ./node_modules/@sveltejs/kit/svelte-kit.js sync && npm run build

GIT_REVISION ?= $(shell git rev-parse HEAD 2>/dev/null)

docker-build:
	docker build -t congee:latest \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_REVISION=$(GIT_REVISION) \
		.
