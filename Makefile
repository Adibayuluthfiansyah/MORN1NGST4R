# MORN1NGST4R development targets. Run `make help`.
# Go builds use the local toolchain only (D-65). A newer toolchain is never
# downloaded implicitly.
export GOTOOLCHAIN := local

CORE := services/core
WEB  := apps/web

.DEFAULT_GOAL := help
.PHONY: help test test-go test-web deps-web build-go build-web run-server run-web validate

help: ## List the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

test: test-go test-web ## Run every automated test

test-go: ## go vet, then go test with the race detector (includes the architecture test)
	cd $(CORE) && go vet ./... && go test -race -count=1 ./...

test-web: ## Type check and Vitest
	cd $(WEB) && npm run typecheck && npm test

deps-web: ## Install the pinned web dependencies (npm ci, install scripts disabled)
	cd $(WEB) && npm ci

build-go: ## Build the server binary into services/core/bin/
	cd $(CORE) && go build -trimpath -o bin/server ./cmd/server

build-web: ## Production build of the web UI (MORNINGSTAR_API_ORIGIN is read at build time)
	cd $(WEB) && npm run build

run-server: ## Start the API on 127.0.0.1:8080 (see .env.example for settings)
	cd $(CORE) && go run ./cmd/server

run-web: ## Start the web UI dev server on 127.0.0.1:3000
	cd $(WEB) && npm run dev

validate: ## Run the documentation validators
	python3 scripts/validate-foundation.py
	python3 scripts/validate-design.py
