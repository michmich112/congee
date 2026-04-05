.PHONY: build run test test-integration test-perf lint ui-dev ui-build docker-build

build:
	mkdir -p bin && go build -o bin/congee ./cmd/congee

run: build
	./bin/congee

test:
	go test ./...

test-integration:
	go run github.com/onsi/ginkgo/v2/ginkgo -r ./test/...

test-perf:
	@echo "test-perf: placeholder (add benchmarks under test/performance/)"

lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; fi

ui-dev:
	cd web/admin && npm run dev

ui-build:
	cd web/admin && npm ci && npx svelte-kit sync && npm run build

docker-build:
	docker build -t congee:latest .
