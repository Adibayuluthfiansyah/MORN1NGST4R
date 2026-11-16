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
| `apps/web` Next.js App Router shell with a backend status line (5 s request timeout, re-checks every 5 s until the backend is ready) and a same-origin rewrite | Implemented |
| Redacting `log/slog` logger | Implemented (by key name only) |

## What is a placeholder or missing

| Area | State |
| --- | --- |
| Readiness probes | None registered. Readiness only says the process is initialized. The database probe comes with M3 |
| Authentication, sessions, CSRF, Origin checks | Not implemented (M4). The only routes are the two public health probes |
| `cmd/worker`, jobs, claiming, tool execution, sandbox, egress | Not created. They wait for G-recovery and G-consume |
| PostgreSQL, migrations, sqlc, and every domain package except `scope` | Not created (M2, M3 and later) |
| OpenAPI generation | Not done. Gate G-openapi has not passed. The health routes are not in `docs/design/api/openapi.yaml` and the web client is hand-written |
| Nonce Content-Security-Policy, login and bootstrap screens, design system | Not done (M13). Only `frame-ancestors 'none'` and three static headers are set |
| Tailwind, shadcn/ui, TanStack Query, Zustand, form libraries, ESLint, Playwright | Not installed. They are listed in `tech.md` and enter with the milestone that needs them |

## Prerequisites

- Go 1.27.4 (the recommended release toolchain, `tech.md` section 1). `go.mod` keeps the `go 1.26.0` floor. Builds use `GOTOOLCHAIN=local`, so no other toolchain is downloaded.
- Node.js 24.x and npm (D-66). `apps/web/package.json` declares `>=24.15.0 <25` and the lockfile installs unchanged with `npm ci`. The reference baseline is Node 24.22.3. Node 26 still runs the code but prints an `EBADENGINE` warning, so treat it as a local experiment until the M13 check.
- `make`, a C compiler for `go test -race`, and Python 3 for the validators.
- PostgreSQL is not needed yet.

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
```

Start the API and the UI in two terminals, then open `http://127.0.0.1:3000`. The page shows "Backend ready" when the browser can reach the API through the Next.js rewrite.

```
curl -s http://127.0.0.1:8080/api/v1/health/live    # {"status":"ok"}
curl -s http://127.0.0.1:8080/api/v1/health/ready   # {"status":"ready"}
```

## Configuration

The Go server reads only the process environment. It does not read `.env` files. `.env.example` lists every variable with its default.

| Variable | Default | Rule |
| --- | --- | --- |
| `MORNINGSTAR_HTTP_LISTEN` | `127.0.0.1:8080` | Loopback IP literal with a port. Wildcard, LAN, hostname and port 0 are refused |
| `MORNINGSTAR_HTTP_ALLOWED_HOSTS` | listen address and `localhost` on the same port | Exact `host:port` list, loopback names only |
| `MORNINGSTAR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `MORNINGSTAR_LOG_FORMAT` | `json` | `json` or `text` |
| `MORNINGSTAR_API_ORIGIN` (web) | `http://127.0.0.1:8080` | Loopback `http` origin with a port. Read by `next.config.ts` at `next build` or `next dev` start |

An unknown `MORNINGSTAR_*` variable stops the server, so a misspelled setting is never ignored. The one exception is `MORNINGSTAR_API_ORIGIN`: it belongs to the web UI, so the Go server accepts it and ignores its value. One shared environment can therefore serve both processes. A rejected value is quoted in the message. No secret is read from the environment. If you change the API port, set `MORNINGSTAR_API_ORIGIN` before `next build`, because Next.js fixes rewrite destinations at build time.

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

- `services/core/internal/config`: defaults, overrides, 22 refusals (wildcard and LAN listen, IPv4-mapped IPv6, unknown variables, bad hosts, bad levels), and the web-only `MORNINGSTAR_API_ORIGIN` (accepted and ignored, near-miss names still refused).
- `services/core/internal/infrastructure/logging`: secret-looking keys are redacted, including inside groups. Values are not inspected.
- `services/core/internal/application/health`: readiness needs every probe, a stuck probe is bounded by the timeout, only the probe name is logged, a panicking probe is an unhealthy result and not a crash, no probe starts once the context is done, and probe goroutines do not accumulate.
- `services/core/internal/adapters/inbound/http/router`: Host allow-list on every route, error envelope shape, request id handling, the API-19 headers on every response, panic containment with Recovery as the outermost middleware (a panic before the request-id middleware still gets an id and headers, a response that already started is not corrupted, a failing logger cannot break a response), one access record per request including a panicked one, no query or header values in the access log.
- `services/core/internal/bootstrap` and `cmd/server`: real HTTP over a loopback listener, header size limit, slow-header timeout, graceful shutdown, refusal of a non-loopback listener, the server's logger redacts secret-named attributes, and 33 raw-request cases (absolute-form targets, duplicate, empty and malformed Host headers, HTTP/1.0, `OPTIONS *`, CONNECT, forwarding headers that must not be trusted, malformed junk). A denied Host never receives the health answer.
- `services/core/internal/domain/scope`: table, property and fuzz tests (`go test -fuzz=FuzzParseAddr` and the six other `Fuzz` targets) for hosts, addresses, prefixes, ports, rules and evaluation, including the `PrefixMin` floor (D-87) and the SIIT range (D-88).
- `services/core/tests/architecture`: dependency directions of `structure.md` 5.6 applied to the real packages for linux/amd64, test files included. Standard library versus third-party comes from `go list` metadata. Forbidden standard-library families match by prefix (`net/http/httputil`, `database/sql/driver`), process creation (`os/exec`, `syscall`, `os.StartProcess`) and plugin loading are refused outside a tools adapter, and source excluded by build constraints on every checked platform fails the test. An unknown internal package, an unreported import such as cgo, or an unlisted third-party import fails closed. The rule engine has its own regression tests.
- `apps/web`: the readiness client (strict parsing, no credentials, bounded by a timeout that also covers a stalled body, no timer left behind), the status component (a rejected or throwing check shows unreachable, sequential re-polling that stops on ready and on unmount), the API origin and header helpers, and `next.config.ts` itself (the rewrite target, the refusal of a non-loopback origin, the page headers and their exclusion of `/api/`).

## Deviations and open items for the owner

- Health endpoints. Kept in the MVP by D-86 (`docs/decisions/0004-health-endpoints-mvp-decision.md`), which supersedes D-80 for health endpoints. Metrics stay post-MVP.
- Milestone. The scaffold is recorded in the plan as existing preparatory work and is not a milestone. M1 (the `scope` package) is implemented. It decides scope only. It does not decide readiness (D-89), and `Decision` does not name the deciding rule yet (D-91). See `docs/decisions/0005-m1-scope-engine-closure-decisions.md`.
- `405 method_not_allowed` is in the error catalog of `docs/design/api/error-model.md`.
- Host behavior through the Next.js rewrite is unverified (see the section above). Closing it is part of M13.
- Toolchain selection. `mise.toml` pins Go 1.27.4 and Node 24.22.3. Install them once with `mise install go@1.27.4 node@24.22.3` and run commands through `mise exec -- make test`, or activate mise in your shell. mise is optional. Any installation of these versions works, and other installed versions are left alone.
- Go toolchain. `govulncheck` (run with `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`, not added to the module) reported GO-2026-6859 in `net/http` of go1.27.3. Under go1.27.4 it reports no vulnerability in the module graph.
- Indirect modules. Gin v1.12.0 pulls older `x/net`, `quic-go`, `x/crypto`, `x/sys` and `x/text`. govulncheck flagged `x/net` v0.51.0 (fixed in v0.60.0) and `quic-go` v0.59.0 (fixed in v0.59.1). `go.mod` now pins `x/net` v0.62.0 (the `tech.md` pin) and `quic-go` v0.59.1, with the others moved by the resolver. A rescan found nothing else in the module graph. Gin's dependency tree is large for a health endpoint (it includes `quic-go`, `mongo-driver` and `sonic`). Gin is the owner's decision (D-02), so this is recorded, not changed.
- Dependency licences. A scan of the license file headers of the Go modules in the build found only MIT, BSD-style and Apache-2.0 text. The npm tree includes optional platform packages under LGPL-3.0-or-later (`@img/sharp-libvips-linux-x64`, installed through `sharp`, a dependency of Next.js) and MPL-2.0 (`lightningcss-linux-x64-gnu`, installed through Vitest's Vite), plus `caniuse-lite` under CC-BY-4.0. They are installed on the developer machine and not redistributed by this repository. O-40 asks for an owner decision on MPL and LGPL material before any redistribution.
