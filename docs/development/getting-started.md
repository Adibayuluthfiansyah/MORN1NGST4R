# Getting started (development)

This page covers the first implementation phase: the Go server skeleton, the web shell, configuration loading and the two health endpoints. It describes what exists and what does not. Requirements and designs live in the documents named in `docs/README.md`. This page does not change them.

## What is implemented

| Area | State |
| --- | --- |
| `services/core` Go module, `cmd/server`, `internal/bootstrap`, `internal/config` | Implemented |
| Environment configuration with fail-closed validation | Implemented |
| Inbound HTTP adapter (Gin): request id, Host allow-list, access log, panic recovery, error envelope | Implemented |
| `GET /api/v1/health/live` and `GET /api/v1/health/ready` | Implemented |
| Dependency-direction test (gate G-arch) | Implemented |
| `internal/domain/scope` (M1): host, address, prefix and port parsing, rules, `RuleSet.Evaluate`. Standard library only, not called by any other package yet | Implemented |
| `internal/domain/actor`, `operation`, `approval` and `scope.Decide` (M2): actors, the canonical operation and its hash, approvals with their consumption state machine, the pure decision function and `AuthorizedOperation`. Standard library only, not called by any other package yet | Implemented |
| PostgreSQL (M3): per-user cluster script, 33 tables in 10 migrations, four login roles, append-only audit table, atomic approval consumption, recovery generation (marker and reconciliation), database readiness probe | Implemented. See the section "Database" below |
| Audit (M5): the `audit` domain (event model, action catalog, redaction, retry policy), the `AuditSink` port (`Require`, `Record`, `Gate`) and its PostgreSQL adapter with bounded retries and a persistent alert (`system_alerts`), the anonymous actor kind, a server-generated correlation id, and the coalescing of repeated rate-limited refusals | Implemented |
| Authentication and session (M4): one local account, Argon2id, server-side sessions with hashed tokens, HttpOnly cookies, CSRF, Origin and Host checks, login rate limit, bootstrap secret on a memory-backed file, one-time recovery code, re-authentication grants, recovery gating and the reconcile route (`GET /session`, `POST /setup/account`, `/session/login`, `/session/logout`, `GET /sessions`, `POST /sessions/{id}/revoke`, `/session/reauthenticate`, `/recovery/reset`, `/system/recovery/reconcile`, `/account/password`) | Implemented |
| `apps/web` Next.js App Router shell with a backend status line (5 s request timeout, re-checks every 5 s until the backend is ready) and a same-origin rewrite | Implemented |
| Redacting `log/slog` logger | Implemented (by key name only) |

## What is a placeholder or missing

| Area | State |
| --- | --- |
| Login, bootstrap and recovery screens | Not built (M13). Until then the routes are used with an HTTP client that keeps cookies and sends the CSRF header |
| `cmd/worker`, jobs, claiming, tool execution, sandbox, egress | Not created. They wait for G-recovery and G-consume |
| Domain packages other than `scope`, `actor`, `operation` and `approval`, the tables of the asset, evidence, finding, integration and AI slices (00008 to 00012) | Not created (M6 and later) |
| OpenAPI generation | Not done. Gate G-openapi has not passed. The health routes are not in `docs/design/api/openapi.yaml` and the web client is hand-written |
| Nonce Content-Security-Policy, login and bootstrap screens, design system | Not done (M13). Only `frame-ancestors 'none'` and three static headers are set |
| Tailwind, shadcn/ui, TanStack Query, Zustand, form libraries, ESLint, Playwright | Not installed. They are listed in `tech.md` and enter with the milestone that needs them |

## Prerequisites

- Go 1.27.4 (the recommended release toolchain, `tech.md` section 1). `go.mod` keeps the `go 1.26.0` floor. Builds use `GOTOOLCHAIN=local`, so no other toolchain is downloaded.
- Node.js 24.x and npm (D-66). `apps/web/package.json` declares `>=24.15.0 <25` and the lockfile installs unchanged with `npm ci`. The reference baseline is Node 24.22.3. Node 26 still runs the code but prints an `EBADENGINE` warning, so treat it as a local experiment until the M13 check.
- `make`, a C compiler for `go test -race`, and Python 3 for the validators.
- PostgreSQL 18 from your system package manager (`initdb`, `pg_ctl`, `postgres` and `psql` on the PATH). Nothing is downloaded or bundled (D-67). Without it the server still runs, but `/ready` answers 503 and the database tests are skipped.

## Commands

```
make help          # list the targets
make test          # Go and web tests
make test-go       # go vet and go test -race ./... in services/core
make deps-web      # npm ci in apps/web (install scripts are disabled by .npmrc)
make test-web      # next typegen, tsc --noEmit, Vitest
make run-server    # Go API on 127.0.0.1:8080
make run-web       # web dev server on 127.0.0.1:3000
make build-go      # services/core/bin/server
make build-web     # next build
make validate      # the two documentation validators
make db-up         # first time: create the cluster, start it, create the roles, apply the migrations
make db-start      # later sessions: start the cluster (make db-stop to stop it)
make test-db       # the database integration tests (they use a throwaway cluster and fail without PostgreSQL 18)
make sqlc          # regenerate the typed queries after a change in migrations/ or queries/
```

Start the API and the UI in two terminals, then open `http://127.0.0.1:3000`. The page shows "Backend ready" when the browser can reach the API through the Next.js rewrite.

```
curl -s http://127.0.0.1:8080/api/v1/health/live    # {"status":"ok"}
curl -s http://127.0.0.1:8080/api/v1/health/ready   # {"status":"ready"}
```

## Database

The database is a per-user PostgreSQL 18 cluster under `$XDG_DATA_HOME/morningstar` (default `~/.local/share/morningstar`). It listens on a unix socket only, in a directory with mode 0700, and accepts peer authentication only: the OS user maps to four login roles (`ms_migrator`, `ms_api`, `ms_worker`, `ms_retention`) and there is no password anywhere. The server connects as `ms_api` through `MORNINGSTAR_DB_SOCKET_DIR`. There is no connection string, host, port or password setting, and a `PGPASSWORD`, `PGOPTIONS` or similar variable makes the server refuse to start.

```
make db-up                                   # once
make run-server                              # the server runs the recovery check, then serves
curl -s http://127.0.0.1:8080/api/v1/health/ready   # {"status":"ready"} while the database answers
```

- The server starts even when the database is down. `/api/v1/health/live` is 200 and `/ready` is 503 until the database answers.
- On the first start of an empty database the server creates the recovery state and the marker file in `$XDG_STATE_HOME/morningstar` (`recovery-generation.json`, mode 0600, directory mode 0700). The marker lives outside the database on purpose. A database restore is detected by comparing the two (`docs/design/database/recovery-generation.md`). Keep the state directory out of any backup and out of a restore.
- To start over on purpose, stop the server, delete the database (or the whole cluster directory) and delete the state directory. A fresh database with the old marker is read as a restore and needs the reconciliation, which needs an account.
- `MORNINGSTAR_PG_BASE` moves the whole cluster tree for the scripts and the tests. The server ignores it.
- Migrations are forward only. `make db-migrate` runs goose as `ms_migrator`. A development database that applied the later migrations (00013 to 00015) before a slice with a lower number was added needs `goose -allow-missing`. A new database does not.

## Configuration

The Go server reads only the process environment. It does not read `.env` files. `.env.example` lists every variable with its default.

| Variable | Default | Rule |
| --- | --- | --- |
| `MORNINGSTAR_HTTP_LISTEN` | `127.0.0.1:8080` | Loopback IP literal with a port. Wildcard, LAN, hostname and port 0 are refused |
| `MORNINGSTAR_HTTP_ALLOWED_HOSTS` | listen address and `localhost` on the same port | Exact `host:port` list, loopback names only |
| `MORNINGSTAR_HTTP_ALLOWED_ORIGINS` | `http://127.0.0.1:3000,http://localhost:3000` | Exact `scheme://host:port` list of the web UI. Loopback `http` origins only. A state-changing request must send an Origin from this list |
| `MORNINGSTAR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `MORNINGSTAR_LOG_FORMAT` | `json` | `json` or `text` |
| `MORNINGSTAR_API_ORIGIN` (web) | `http://127.0.0.1:8080` | Loopback `http` origin with a port. Read by `next.config.ts` at `next build` or `next dev` start |

An unknown `MORNINGSTAR_*` variable stops the server, so a misspelled setting is never ignored. The one exception is `MORNINGSTAR_API_ORIGIN`: it belongs to the web UI, so the Go server accepts it and ignores its value. One shared environment can therefore serve both processes. A rejected value is quoted in the message. No secret is read from the environment. If you change the API port, set `MORNINGSTAR_API_ORIGIN` before `next build`, because Next.js fixes rewrite destinations at build time.

## Authentication and account

The account is created once, with a bootstrap secret that never leaves memory-backed storage.

- When the server starts and no account exists, it creates a 256-bit secret, stores only its hash in the database and writes the secret to `$XDG_RUNTIME_DIR/morningstar/bootstrap-secret` (file mode 0600, directory mode 0700). The directory must be on tmpfs or ramfs. On a normal Linux login session `XDG_RUNTIME_DIR` is `/run/user/<uid>`. If it is unset, relative or on disk, the server still runs but `POST /api/v1/setup/account` answers 409 `setup_unavailable`, and nothing is written. There is no setting that moves the file to disk.
- `GET /api/v1/session` reports `setup_required`, `anonymous` or `authenticated`, and hands out the CSRF token that the next request needs (`X-CSRF-Token`, together with an allow-listed `Origin` header). Anonymous callers get the token with an `ms_anon` cookie that lasts 15 minutes. Signed-in callers get the token of their session.
- `POST /api/v1/setup/account` takes the secret, a username and a password, creates the account, deletes the secret file, signs in and returns a one-time recovery code. The code is shown once. A restart replaces an unused secret with a new one.
- A password has 12 to 128 characters, valid UTF-8, no control characters and is not the username. A username has 3 to 64 characters from `a-z 0-9 . _ -`.
- Sessions idle out after 30 minutes and end after 12 hours. Logout, a password change and a recovery reset revoke them on the server.
- Five failed password guesses (login, re-authentication) in a rolling 15 minutes block the next one until the window clears. Recovery-code guesses and bootstrap-secret guesses have their own counts, so a locked password does not close the recovery route.
- Lost password: `POST /api/v1/recovery/reset` with the recovery code sets a new password, revokes every session and returns a new code. The old code is spent.
- After a database restore the server answers 503 `recovery_required` on every route except `GET /session` and `POST /api/v1/system/recovery/reconcile`. The reconcile call needs the password of the restored database and a new password, which replaces it in the same step. It revokes every session, consumes every open recovery code and bootstrap secret of the restored database and returns a new recovery code. If the call fails after the database step, the new password is already set: log in with it.
- The cookies are `HttpOnly`, `SameSite=Strict` and set no domain. `Secure` is set only when the connection uses TLS, which the MVP does not, so the cookies of a plain `http://localhost` session do not carry it.

## Audit

Every audit event is written by one adapter, `AuditSink`. An operation that needs its record before it may start (for example a login that creates a session) writes it inside its own transaction, so a failed write rolls the operation back and the API answers 503 `database_unavailable` or 500 `internal_error`. A denial, a logout and a session revocation take effect whatever audit does: their record is written on its own and retried up to three times (waits of 100 and 400 ms). Those numbers are provisional and wait for the owner. When the retries are exhausted the server raises one open alert, a row of `system_alerts` with kind `audit_retry_exhausted`, which survives a restart. The alert names the lost event by action and request id and holds no event detail. While it is open, `AuditSink.Gate` answers "degraded" and the use cases that start work (M7, M19, M21, TD-M5-01) must not start. No route calls it yet. Until the route `POST /system-alerts/{alertId}/clear` exists (M10, TD-M5-03), the owner of the database clears an alert after the cause is fixed: `UPDATE morningstar.system_alerts SET cleared_at = now(), cleared_by = '<user id>' WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL`. An unauthenticated caller is recorded with the actor kind `anonymous`, never `system`. The server makes the request id, and an `X-Request-Id` that a client sends is ignored.

## Health endpoints

Both routes are public and return one fixed status word, nothing about the host, versions, dependencies or configuration. They sit behind the Host allow-list, so a request with another Host header gets `403 host_denied` first. Unknown paths return the `not_found` envelope and wrong methods return `405 method_not_allowed` with an `Allow` header. Every response carries `X-Request-Id` and the headers of API-19: `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `Referrer-Policy: no-referrer` and `Cross-Origin-Resource-Policy: same-origin`. The web server sets its own framing policy for pages only and skips `/api/`, so a response proxied from the API carries Go's policy once.

| Route | 200 | 503 |
| --- | --- | --- |
| `/api/v1/health/live` | `{"status":"ok"}` | `{"status":"not_live"}` (not produced today) |
| `/api/v1/health/ready` | `{"status":"ready"}` | `{"status":"not_ready"}` with `Retry-After: 5` |

## The Host check and the Next.js rewrite

The Go server validates the Host header of the request it receives, against the allow-list above. Browser traffic to the UI on `127.0.0.1:3000` reaches Go through the Next.js rewrite. Which Host Go receives in that case has not been independently verified. It may be the API's own host and port (set by the proxy) or the browser's original Host. Do not read the allow-list as validating the browser's original Host header.

One manual run on the reference host (Next.js 16.4.6 with `next start`, API on another loopback port) was answered with 200 by the default allow-list, which contains only the API's own host and port. That is a single observation. It is not a test and not a verification. DNS-rebinding and Host-header protection for the web origin (T-55, T-56) is therefore not established until the M13 test of D-79 passes. Direct requests to the API port are checked as described. The loopback-only binding and the allow-list stay required (D-86).

## Tests

- `services/core/tests/integration/db` and `tests/integration/server` (need PostgreSQL 18, see `make test-db`): migrations, schema against the data dictionary, socket and role provisioning, grant matrix, append-only and column-limiting triggers, G-consume (real concurrent transactions, agreement with the M2 model), G-recovery (a real restore of an older database), the readiness probe against a stopped cluster.
- `services/core/tests/integration/auth` (needs PostgreSQL 18): setup and the bootstrap secret, login, the rate limit and its budgets, sessions, expiry and revocation, CSRF, re-authentication grants, recovery reset, password change, recovery gating and reconcile, restore of an older database, and secrets in logs, audit events and error responses. `browser_test.go` drives headless Chromium (DNS rebinding, cookie sharing between local ports). It skips when no Chromium is installed, and `MORNINGSTAR_REQUIRE_BROWSER_TESTS=1` (set by `make test-db`) turns that skip into a failure.
- `services/core/internal/application/auth`, `internal/domain/identity`, `internal/adapters/outbound/{passwordhash,bootstrapfile}`, `internal/adapters/inbound/http/{router,handlers}`: session checks, tokens, the order of the setup steps, password policy, the hash bounds, the secret file, the route table (Host, Origin, CSRF, session, gate, body rules, rate limit) and the cookie attributes. `go test -run '^$' -bench Hash ./internal/adapters/outbound/passwordhash` repeats the Argon2id benchmark.
- `services/core/tests/architecture`: also `TestHumanActorIsBuiltOnlyByTheAuthenticationUseCase` (TD-M2-01), and for M5 `TestEveryAuditWriteGoesThroughTheSinkAdapter` and `TestAuditCatalogMatchesTheDesignDocument`.
- `services/core/internal/domain/audit`, `internal/adapters/outbound/persistence` (`audit_internal_test.go`), `tests/integration/db/audit_sink_test.go` and `tests/integration/auth/audit_test.go` (need PostgreSQL 18): the event model, the retry bound, the persistent alert and the gate, and the three semantics through the server (an operation does not start, a denial applies, a revocation takes effect when the record cannot be written).
- `services/core/internal/adapters/outbound/{persistence,recoverymarker}`, `internal/application/recovery`, `internal/platform/posix`: connection settings, marker file (modes, symlinks, concurrent writers), the decision table and the crash-resume paths with injected failures.
- `services/core/internal/config`: defaults, overrides, 22 refusals (wildcard and LAN listen, IPv4-mapped IPv6, unknown variables, bad hosts, bad levels), and the web-only `MORNINGSTAR_API_ORIGIN` (accepted and ignored, near-miss names still refused).
- `services/core/internal/infrastructure/logging`: secret-looking keys are redacted, including inside groups. Values are not inspected.
- `services/core/internal/application/health`: readiness needs every probe, a stuck probe is bounded by the timeout, only the probe name is logged, a panicking probe is an unhealthy result and not a crash, no probe starts once the context is done, and probe goroutines do not accumulate.
- `services/core/internal/adapters/inbound/http/router`: Host allow-list on every route, error envelope shape, request id handling, the API-19 headers on every response, panic containment with Recovery as the outermost middleware (a panic before the request-id middleware still gets an id and headers, a response that already started is not corrupted, a failing logger cannot break a response), one access record per request including a panicked one, no query or header values in the access log.
- `services/core/internal/bootstrap` and `cmd/server`: real HTTP over a loopback listener, header size limit, slow-header timeout, graceful shutdown, refusal of a non-loopback listener, the server's logger redacts secret-named attributes, and 33 raw-request cases (absolute-form targets, duplicate, empty and malformed Host headers, HTTP/1.0, `OPTIONS *`, CONNECT, forwarding headers that must not be trusted, malformed junk). A denied Host never receives the health answer.
- `services/core/internal/domain/{actor,operation,approval}` and `scope/decide_test.go`: table, hostile-sequence and fuzz tests (`FuzzOperationHash`) for the operation hash, the approval lifecycle, the consumption state machine and the decision function.
- `services/core/tests/security`: the security regression tests of the policy core. `TestM2Acceptance` has one subtest for each M2 acceptance sentence.
- `services/core/internal/domain/scope`: table, property and fuzz tests (`go test -fuzz=FuzzParseAddr` and the six other `Fuzz` targets) for hosts, addresses, prefixes, ports, rules and evaluation, including the `PrefixMin` floor (D-87) and the SIIT range (D-88).
- `services/core/tests/architecture`: dependency directions of `structure.md` 5.6 applied to the real packages for linux/amd64, test files included. Standard library versus third-party comes from `go list` metadata. Forbidden standard-library families match by prefix (`net/http/httputil`, `database/sql/driver`), process creation (`os/exec`, `syscall`, `os.StartProcess`) and plugin loading are refused outside a tools adapter, and source excluded by build constraints on every checked platform fails the test. An unknown internal package, an unreported import such as cgo, or an unlisted third-party import fails closed. The rule engine has its own regression tests.
- `apps/web`: the readiness client (strict parsing, no credentials, bounded by a timeout that also covers a stalled body, no timer left behind), the status component (a rejected or throwing check shows unreachable, sequential re-polling that stops on ready and on unmount), the API origin and header helpers, and `next.config.ts` itself (the rewrite target, the refusal of a non-loopback origin, the page headers and their exclusion of `/api/`).

## Deviations and open items for the owner

- M4 decisions that wait for the owner (details in the plan, section "Milestone M4"): the password policy numbers, the Argon2id values for `tech.md`, and the default web origin `http://127.0.0.1:3000`. The owner approved the `newPassword` field and the extra `recoveryCode` field of the reconcile call and the 300 per minute limit of `GET /session` on 2026-11-18.
- A local process can lock the password for 15 minutes at a time with five wrong guesses. This is accepted (T-59). Recovery by code is the way back in.

- Health endpoints. Kept in the MVP by D-86 (`docs/decisions/0004-health-endpoints-mvp-decision.md`), which supersedes D-80 for health endpoints. Metrics stay post-MVP.
- Milestone. The scaffold is recorded in the plan as existing preparatory work and is not a milestone. M1 (the `scope` package) and M2 (the policy and approval domain) are implemented. It decides scope only. It does not decide readiness (D-89), and `Decision` does not name the deciding rule yet (D-91). See `docs/decisions/0005-m1-scope-engine-closure-decisions.md`.
- `405 method_not_allowed` is in the error catalog of `docs/design/api/error-model.md`.
- Host behavior through the Next.js rewrite is unverified (see the section above). Closing it is part of M13.
- Toolchain selection. `mise.toml` pins Go 1.27.4 and Node 24.22.3. Install them once with `mise install go@1.27.4 node@24.22.3` and run commands through `mise exec -- make test`, or activate mise in your shell. mise is optional. Any installation of these versions works, and other installed versions are left alone.
- Go toolchain. `govulncheck` (run with `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`, not added to the module) reported GO-2026-6859 in `net/http` of go1.27.3. Under go1.27.4 it reports no vulnerability in the module graph.
- Indirect modules. Gin v1.12.0 pulls older `x/net`, `quic-go`, `x/crypto`, `x/sys` and `x/text`. govulncheck flagged `x/net` v0.51.0 (fixed in v0.60.0) and `quic-go` v0.59.0 (fixed in v0.59.1). `go.mod` now pins `x/net` v0.62.0 (the `tech.md` pin) and `quic-go` v0.59.1, with the others moved by the resolver. A rescan found nothing else in the module graph. Gin's dependency tree is large for a health endpoint (it includes `quic-go`, `mongo-driver` and `sonic`). Gin is the owner's decision (D-02), so this is recorded, not changed.
- Dependency licences. A scan of the license file headers of the Go modules in the build found only MIT, BSD-style and Apache-2.0 text. The npm tree includes optional platform packages under LGPL-3.0-or-later (`@img/sharp-libvips-linux-x64`, installed through `sharp`, a dependency of Next.js) and MPL-2.0 (`lightningcss-linux-x64-gnu`, installed through Vitest's Vite), plus `caniuse-lite` under CC-BY-4.0. They are installed on the developer machine and not redistributed by this repository. O-40 asks for an owner decision on MPL and LGPL material before any redistribution.
