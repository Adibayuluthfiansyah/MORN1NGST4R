# MORN1NGST4R development targets. Run `make help`.
# Go builds use the local toolchain only (D-65). A newer toolchain is never
# downloaded implicitly.
export GOTOOLCHAIN := local

CORE := services/core
WEB  := apps/web

.DEFAULT_GOAL := help
.PHONY: help test test-go test-db test-web deps-web build-go build-web run-server run-web validate db-init db-start db-bootstrap db-migrate db-stop db-status db-up sqlc

# Per-user PostgreSQL 18 cluster (D-67). MORNINGSTAR_PG_BASE moves it.
PGCLUSTER := scripts/db/pg-cluster.sh
# goose is run as a tool and pinned here. It is not a dependency of the server.
GOOSE := github.com/pressly/goose/v3/cmd/goose@v3.30.1

help: ## List the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

test: test-go test-web ## Run every automated test

test-go: ## go vet, then go test with the race detector (includes the architecture test)
	cd $(CORE) && go vet ./... && go test -race -count=1 ./...

test-db: ## Integration tests against a throwaway PostgreSQL 18 cluster and a headless Chromium (fails when either is missing)
	cd $(CORE) && MORNINGSTAR_REQUIRE_DB_TESTS=1 MORNINGSTAR_REQUIRE_BROWSER_TESTS=1 go test -race -count=1 ./tests/integration/...

test-web: ## Type check and Vitest
	cd $(WEB) && npm run typecheck && npm test

deps-web: ## Install the pinned web dependencies (npm ci, install scripts disabled)
	cd $(WEB) && npm ci

build-go: ## Build the server and worker binaries into services/core/bin/
	cd $(CORE) && go build -trimpath -o bin/server ./cmd/server
	cd $(CORE) && go build -trimpath -o bin/worker ./cmd/worker

build-web: ## Production build of the web UI (MORNINGSTAR_API_ORIGIN is read at build time)
	cd $(WEB) && npm run build

run-server: ## Start the API on 127.0.0.1:8080 (see .env.example for settings)
	cd $(CORE) && go run ./cmd/server

run-worker: ## Start the worker (jobs and maintenance). It waits until the server has initialized the installation
	cd $(CORE) && go run ./cmd/worker

run-web: ## Start the web UI dev server on 127.0.0.1:3000
	cd $(WEB) && npm run dev

db-init: ## Create the per-user PostgreSQL cluster (unix socket, peer authentication, no password)
	$(PGCLUSTER) init

db-start: ## Start the cluster
	$(PGCLUSTER) start

db-bootstrap: ## Create the four login roles and the database (once, after db-start)
	$(PGCLUSTER) bootstrap

db-migrate: ## Apply the migrations as ms_migrator over the socket
	cd $(CORE) && go run $(GOOSE) -dir migrations pgx "host=$$(cd ../.. && $(PGCLUSTER) paths | sed -n 's/^socket=//p') user=ms_migrator dbname=morningstar sslmode=disable" up

db-stop: ## Stop the cluster
	$(PGCLUSTER) stop

db-status: ## Show whether the cluster runs
	$(PGCLUSTER) status

db-up: db-init db-start db-bootstrap db-migrate ## First-time setup: init, start, bootstrap and migrate

sqlc: ## Regenerate the typed queries from migrations/ and queries/ (sqlc is run as a tool, pinned here)
	cd $(CORE) && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

validate: ## Run the documentation validators
	python3 scripts/validate-foundation.py
	python3 scripts/validate-design.py
