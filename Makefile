# shithub-cli Makefile
# See AGENTS.md for the engineering rules these targets enforce.

SHELL := /bin/sh

# Build metadata stamped into the binary. CI sets these; local builds get
# sensible fallbacks so `make build` always works.
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/tenseleyFlow/shithub-cli/internal/build.Version=$(VERSION) \
	-X github.com/tenseleyFlow/shithub-cli/internal/build.Commit=$(COMMIT) \
	-X github.com/tenseleyFlow/shithub-cli/internal/build.Date=$(DATE)

GO        ?= go
GOFLAGS   ?= -trimpath
BIN_DIR   := bin
BIN       := $(BIN_DIR)/shithub
PKG_MAIN  := ./cmd/shithub

.PHONY: help build install dev test test-race bench lint vet fmt fmt-check \
	spdx-check tidy clean ci install-tools release-snapshot

help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "Targets:\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: $(BIN) ## Build bin/shithub.

$(BIN): $(shell find . -name '*.go' -not -path './.docs/*' -not -path './vendor/*' 2>/dev/null) go.mod go.sum
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG_MAIN)

install: ## go install the binary into $GOBIN.
	$(GO) install $(GOFLAGS) -ldflags '$(LDFLAGS)' $(PKG_MAIN)

dev: ## Run shithub from source (passes ARGS through).
	$(GO) run $(GOFLAGS) -ldflags '$(LDFLAGS)' $(PKG_MAIN) $(ARGS)

test: ## Run unit tests.
	$(GO) test $(GOFLAGS) ./...

test-race: ## Run tests with the race detector.
	$(GO) test $(GOFLAGS) -race ./...

integration: ## Run end-to-end tests against a live shithub. Requires SHITHUB_INTEGRATION_HOST and SHITHUB_INTEGRATION_TOKEN env vars; see internal/integration/doc.go for the contract.
	@if [ -z "$$SHITHUB_INTEGRATION_HOST" ] || [ -z "$$SHITHUB_INTEGRATION_TOKEN" ]; then \
		echo "make integration: set SHITHUB_INTEGRATION_HOST and SHITHUB_INTEGRATION_TOKEN"; exit 2; \
	fi
	$(GO) test $(GOFLAGS) -tags=integration -count=1 ./internal/integration/...

bench: ## Run benchmarks.
	$(GO) test $(GOFLAGS) -bench=. -run=^$$ ./...

vet: ## go vet.
	$(GO) vet ./...

lint: ## golangci-lint run.
	golangci-lint run

fmt: ## Format with gofumpt + goimports.
	gofumpt -w .
	goimports -w .

fmt-check: ## Verify code is gofumpt + goimports clean (CI).
	@out=$$(gofumpt -l .); \
	if [ -n "$$out" ]; then \
		echo "gofumpt: files need formatting:"; echo "$$out"; exit 1; \
	fi
	@out=$$(goimports -l .); \
	if [ -n "$$out" ]; then \
		echo "goimports: files need formatting:"; echo "$$out"; exit 1; \
	fi

spdx-check: ## Verify SPDX headers on every .go file.
	@./scripts/check-spdx.sh

tidy: ## go mod tidy.
	$(GO) mod tidy

clean: ## Remove build artifacts.
	rm -rf $(BIN_DIR) dist

# CI runs the full verification pipeline. New checks belong here.
ci: fmt-check vet spdx-check lint test build ## Run the full CI check suite.

install-tools: ## Install development tools (gofumpt, goimports, golangci-lint, goreleaser).
	$(GO) install mvdan.cc/gofumpt@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint missing — install via your package manager or https://golangci-lint.run"; \
	}
	@command -v goreleaser >/dev/null 2>&1 || { \
		echo "goreleaser missing — install via your package manager or https://goreleaser.com"; \
	}

release-snapshot: ## Local cross-platform build via goreleaser.
	goreleaser release --snapshot --clean
