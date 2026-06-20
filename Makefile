# Makefile for the data-receiving Go microservice.
# Targets mirror the CI pipeline so `make ci` locally == green build.

BINARY      := data-receiving
PKG         := ./...
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -X main.version=$(VERSION)

# Source of truth for contracts lives in contracts/schemas. The schema validator
# embeds a copy under internal/schema/schemas (go:embed cannot reach ../../contracts).
CONTRACTS_SCHEMAS := contracts/schemas
EMBED_SCHEMAS     := internal/schema/schemas

.PHONY: all build run test test-integration test-pact vet lint fmt ci sync-schemas check-schemas clean tidy

all: build

build: ## Build the service binary into ./bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/data-receiving

run: ## Run locally (defaults to stdout mode, no external services)
	go run -ldflags "$(LDFLAGS)" ./cmd/data-receiving

test: ## Run unit + contract tests (no Docker required)
	go test -race $(PKG)

test-integration: ## Run integration tests (requires Docker / testcontainers)
	go test -race -tags=integration ./test/integration/...

# CDCT message pacts (CLAUDE.md §3.3). Requires the pact FFI native lib:
#   go run github.com/pact-foundation/pact-go/v2 install
# On Windows the linker needs an import library generated from the DLL:
#   cd <libdir> && gendef pact_ffi.dll && \
#     dlltool -d pact_ffi.def -l libpact_ffi.dll.a -D pact_ffi.dll
# Then point the linker/loader at it (libdir defaults to /usr/local/lib).
test-pact: ## Run consumer-driven contract tests (requires pact FFI lib)
	CGO_ENABLED=1 go test -tags=pact ./test/pact/...

vet:
	go vet $(PKG)

lint: ## Run golangci-lint (install: https://golangci-lint.run)
	golangci-lint run

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

# Copy the canonical contract schemas into the embed directory used by go:embed.
sync-schemas:
	@mkdir -p $(EMBED_SCHEMAS)
	@cp $(CONTRACTS_SCHEMAS)/*.schema.json $(EMBED_SCHEMAS)/
	@echo "synced schemas -> $(EMBED_SCHEMAS)"

# CI guard: fail if the embedded schemas drifted from contracts/schemas.
check-schemas: sync-schemas
	@git diff --exit-code -- $(EMBED_SCHEMAS) || \
		(echo "ERROR: $(EMBED_SCHEMAS) is out of sync with $(CONTRACTS_SCHEMAS). Run 'make sync-schemas'." && exit 1)

ci: vet lint test ## Everything CI runs

clean:
	rm -rf bin/ dist/
