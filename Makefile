.PHONY: build run dev test test-integration test-perf lint ui-dev ui-build docker-build

VERSION ?= 0.0.0-dev

build:
	mkdir -p bin && go build -ldflags "-X github.com/michmich112/congee/internal/version.Version=$(VERSION)" -o bin/congee ./cmd/congee

# Run relay from source (no bin/congee). Loads ./.env automatically if present — see cmd/congee/main.go.
dev:
	go run ./cmd/congee

run: build
	./bin/congee

test:
	go test ./...

test-integration:
	go run github.com/onsi/ginkgo/v2/ginkgo -r ./test/integration/...

test-perf:
	@echo "test-perf: placeholder (add benchmarks under test/performance/)"

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
