# MORN1NGST4R: Technology Decisions

This file is the single source for technology choices, versions and their status. `product.md`, `structure.md`, `security.md` and `architecture.md` refer to it. It was populated in Round 5 at the owner's instruction. Before that it was empty by decision D1.

## How to read this file

- **Verified on 2026-11-15** means a primary source (project site, official repository, package registry, standards body) was fetched on that date. The full source list, quotes and the claims that could not be verified are in `docs/development/tech-verification/` (four files). A version or fact marked NOT VERIFIED there is not stated as fact here.
- Status values: **Decided** (an owner statement or a D- entry), **Conditional** (decided as a direction, mechanism depends on a result), **Proposed** (P- entry, not owner-confirmed), **Open** (no choice), **Requires validation** (a behavior that only a test can confirm).
- The technologies the owner named are Decided (D-62). Their **versions are Proposed** (P-41, P-50) except where Round 6 decided them (D-65 Go, D-66 Node and npm, D-67 PostgreSQL). Round 6 is `docs/decisions/0003-foundation-closure-decisions.md`. A version is a pin, not a promise to upgrade. Do not upgrade only because a newer release exists. Do not use a pre-release.
- A library enters a package group of `structure.md` 5.6 only when it is listed here with a version, a licence and a reason.
- Tested platform: one reference host (Omarchy, Arch family, kernel 7.2.11-zen1, x86_64, Go 1.27.3, bubblewrap 0.13.1, Node 26.13.0, PostgreSQL client tools 18.6). Nothing else was tested. The host's own versions are not a baseline.

## 1. Platform and runtime baseline

| Item | Choice | Status | Reason and trade-off | Primary sources (fetched 2026-11-15) |
| --- | --- | --- | --- | --- |
| Operating system | Linux x86_64, localhost web application | Decided (D-16, D-17, D-19, D-20) | Windows, macOS, native desktop and headless are future modes | owner decision |
| Linux baseline | Defined by capability: a host that passes the sandbox self-test and the egress checks. The reference host is the only tested one. | Proposed (P-51, O-22) | Honest about what was tested. Ubuntu 22.04 packages bubblewrap 0.6.1 per its package page, while the spike's self-test assumes 0.8 with no cited source, and the release that introduced each option is NOT VERIFIED. The Ubuntu 24.04 userns default is NOT VERIFIED. | `tech-verification/tools-ai-and-skills.md` 3.3 |
| Go | Module path `github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core` (from the repository remote). `go 1.26.0` module floor, no `toolchain` line, builds with `GOTOOLCHAIN=local`. go1.27.4 is the recommended release toolchain and is not required. The reference host has go1.27.3. | Decided (D-65). Pins of other modules stay Proposed (P-41). | Go supports 1.27 and 1.26. Go 1.25 is out of support. goose needs `go 1.26.0`. Go 1.26.11 is a supported fallback. | go.dev/dl/?mode=json, go.dev/doc/devel/release. Licence BSD-3-Clause, go.dev/LICENSE (read by the coordinator, see `tech-verification/addendum-coordinator-checks.md`) |
| Node.js | 24.x, at least 24.15.0 (24.22.3 on the fetch date). `engines` will be `>=24.15.0 <25`. Node 26 is for local experiments only until an M13 check passes. | Decided (D-66) | Node 24 is Maintenance LTS to 2028-04-30 and satisfies every `engines` range checked. Node 26 is Active LTS to 2029-04-30, but a Next.js statement that 26 is tested was not found. Node 20 is end of life. | nodejs.org/en/about/previous-releases, nodejs.org/dist/index.json, raw.githubusercontent.com/nodejs/Release/main/schedule.json. Licence MIT for the core, raw.githubusercontent.com/nodejs/node/main/LICENSE (coordinator addendum) |
| Package manager | npm (bundled with Node). `package-lock.json` is committed once `apps/web` exists and installs use `npm ci`. | Decided (D-66). pnpm is not adopted. | pnpm saves disk. Corepack ships with Node up to 24 but not from Node 25. | pnpm docs, see `tech-verification/frontend.md` 2.2 |
| PostgreSQL | 18, current minor (18.7), installed by the user with the system package manager. The project does not bundle a server. | Decided (D-67) | Supported until 2030-11-14. PostgreSQL 14 final release 2026-11-12 has passed. Do not target a PostgreSQL 19 release (not verified to exist). Licence: the PostgreSQL License (coordinator addendum). | postgresql.org/support/versioning/, postgresql.org/about/licence/ |
| Browsers | Latest evergreen browsers. Effective floor Firefox 128 because of Tailwind 4. | Proposed | Tailwind 4 needs Firefox 128, Next.js lists Firefox 111 | `tech-verification/frontend.md` 6 |
| Hardware target | Linux laptop, 8 GB RAM | Owner statement in the Round 5 instruction (not a register entry) | See section 15 | owner instruction |

## 2. Backend stack

Gin, pgx, sqlc, goose and OpenAPI are Decided technologies (D-02, D-62). The other entries (oapi-codegen, the `x/` modules, google/uuid, testify) are Proposed choices (P-41, P-43, P-47, P-48). All versions are Proposed (P-41). All verified 2026-11-15.

| Technology | Pin | Licence | Why selected | Known trade-offs | Source |
| --- | --- | --- | --- | --- | --- |
| Gin | v1.12.0 | MIT | Owner choice. Stable latest. | `r.Run()` listens on 0.0.0.0 per its README, so the server must bind loopback explicitly with `http.Server` (`security.md` 8). No built-in OpenAPI. | proxy.golang.org/github.com/gin-gonic/gin/@latest |
| pgx | v5.13.0 (floor v5.12.0) | MIT | Owner choice. v5.12.0 fixes SQL injection in simple-protocol batches and a `require_auth` bypass. v5.13.0 fixes a panic on malformed server responses (per the repository CHANGELOG and release tags, advisory pages not fetched). | GitHub's releases API still shows v5.11.0 as latest, so use module tags and the CHANGELOG. | proxy.golang.org/github.com/jackc/pgx/v5/@latest, repository CHANGELOG |
| sqlc | v1.31.1 | MIT | Owner choice. Type-safe SQL. | Review generated output on each bump. | proxy.golang.org, github.com/sqlc-dev/sqlc |
| goose | v3.30.1 (fallback v3.29.0) | MIT | Owner choice. Explicit migrations. | v3.30.1 was three days old. | proxy.golang.org/github.com/pressly/goose/v3/@latest |
| oapi-codegen | v2.8.2 | Apache-2.0 | Official Gin server generator, spec-first | OpenAPI 3.1 support is "initial", so author the spec as 3.0.4 | proxy.golang.org, github.com/oapi-codegen/oapi-codegen |
| golang.org/x/crypto | v0.59.0 | BSD-3-Clause | Argon2id | No stated support window for `x/` modules. Track advisories. | proxy.golang.org/golang.org/x/crypto/@latest |
| golang.org/x/net | v0.62.0 | BSD-3-Clause | `idna` for host canonicalization (P-57) | Its `publicsuffix` snapshot is months old and is not used for security decisions | proxy.golang.org/golang.org/x/net/@latest |
| golang.org/x/time | v0.16.0 | BSD-3-Clause | `rate` token bucket for login limiting | Keyed limiting is hand-written | proxy.golang.org/golang.org/x/time/@latest |
| google/uuid | v1.6.0 | BSD-3-Clause | UUID v7 in the application (P-47) | Last release 2024-01-23 | proxy.golang.org/github.com/google/uuid/@latest |
| testify | v1.12.1, optional | MIT | Assertions | Optional | proxy.golang.org |

Not pinned: swag v2 (release candidate), River (v0.x, MPL-2.0), embedded-postgres (runtime download), ULID libraries, `99designs/keyring`.

## 3. Frontend stack

Next.js, React, TypeScript, TanStack Query, Tailwind, shadcn/ui, Zustand, React Hook Form and Zod are Decided technologies (D-02, D-62, `structure.md` 8). ESLint, Vitest, jsdom, Testing Library, Playwright and the OpenAPI client generators are Proposed (P-43, P-46, P-50). All versions are Proposed (P-50). Verified 2026-11-15 against registry.npmjs.org and the project sites.

| Technology | Pin | Licence | Constraint or note |
| --- | --- | --- | --- |
| Next.js (App Router) | 16.4.6 | MIT | Active LTS. `engines` node >=20.9.0. Next 15.5.x is Maintenance LTS only. |
| React, React DOM | 19.3.1 | MIT | Confirm the react-dom number at install |
| TypeScript | 5.9.3 | Apache-2.0 | Not 7.0.2: its peer ranges conflict with the chosen tooling |
| TanStack Query | 5.107.1 | MIT | Accepts React 19 |
| Tailwind CSS, @tailwindcss/postcss | 4.3.5 | MIT | Node 22.12 or later, inferred from a dependency (the package has no `engines` field) |
| shadcn/ui (CLI), radix-ui | 4.23.0, 1.8.1 | MIT | Whether Radix and shadcn inline styles survive a nonce-only `style-src` is not verified |
| Zustand | 5.1.0 | MIT | Only where needed |
| React Hook Form, @hookform/resolvers | 7.92.0, 7.0.6 | MIT | The resolvers peer range covers Zod 3.25 and 4 |
| Zod | 4.6.7 | MIT | |
| ESLint | 9.39.5 | MIT | Not 10.15.0: three plugin peers conflict |
| Vitest, jsdom, Testing Library | 5.1.0, 31.0.1, 16.3.3 (+ `@testing-library/dom` ^10) | MIT | jsdom needs Node ^24.15.0 |
| Playwright | 1.65.1, deferred, Chromium only | Apache-2.0 | Browser download size not measured |

Serving (O-02, O-03, Proposed P-49): `next build`, then `next start -H 127.0.0.1 -p <port>`, with same-origin `rewrites` to the Go API. The CLI page says `next dev` and `next start` default to 0.0.0.0, and another page says localhost, so always pass `-H 127.0.0.1`. Static export cannot do cookies, rewrites, headers or a nonce CSP, so it is rejected. A nonce-based CSP without `unsafe-inline` is configured in `proxy.ts` (Next.js docs). Go validates Host, Origin and CSRF itself. Whether Next.js forwards a usable Host header through rewrites is NOT VERIFIED (Requires validation).

## 4. Database and persistence

| Item | Choice | Status | Notes |
| --- | --- | --- | --- |
| Datastore | PostgreSQL 18 | Decided (D-01, D-02), version Proposed | One datastore for state, queue and audit |
| Provisioning (O-01) | Per-user cluster under `$XDG_DATA_HOME/morningstar/pg`, created and started by Makefile targets with user-installed binaries. Unix socket only in a directory with mode 0700, peer authentication, separate login roles through `pg_ident.conf`. No password, no DSN, a user-supplied DSN is not supported in the MVP. Backup is `pg_dump` plus the evidence directory, owner-only. | Decided (D-67, closes O-01, O-39, O-43) | No root and no container. Peer authentication identifies the OS user, not the process, so a sandboxed tool running as the same user must have no path to the socket. Idle memory is not measured (Requires validation). Who provides the binaries is open (O-39). |
| Access | pgx v5 pool, sqlc queries, goose migrations | Decided | Roles for migration, API and worker (P-13). Peer authentication cannot separate roles between processes of the same OS user, so append-only audit and role separation are a design aim, not a guarantee, until separate credentials or OS users exist (threat model T-61) |
| Identifiers | UUID v7 generated in the application, native `uuid` column | Proposed (P-47) | PostgreSQL 18 also has `uuidv7()` per its docs. App-side keeps PostgreSQL 17 usable. |
| Isolation | READ COMMITTED (the PostgreSQL default) | Proposed (P-27) | Keep every approval validity field on one row, because a concurrent UPDATE re-evaluates the WHERE clause only against the updated row (PostgreSQL docs, verified) |
| Data location | `$XDG_DATA_HOME/morningstar`, owner-only permissions | Proposed (P-62, O-23) | |

## 5. Queue and worker

PostgreSQL-backed queue behind the `JobQueue` port: **Decided as the medium** (D-62). A custom implementation with `SELECT ... FOR UPDATE SKIP LOCKED`, leases and fencing, and no queue library in the MVP: **Proposed** (P-44, O-12). Verified from the PostgreSQL docs: `SKIP LOCKED` "provides an inconsistent view of the data" and is meant for queue-like tables. `NOTIFY` payloads stay under 8000 bytes and are delivered after commit, so `LISTEN`/`NOTIFY` is only a wake-up hint and polling stays. `INSERT ... RETURNING` needs `SELECT` privilege on the returned columns. The lease and fencing design is a project proposal, not from documentation. Heartbeat and kill timing that fit inside the 30-second objective: **Open and Requires validation** (O-35). `server` and `worker` are separate processes (D-03). The worker has no network listener (P-17).

## 6. Evidence storage

| Item | Choice | Status |
| --- | --- | --- |
| Medium | Local filesystem | Decided (D-62) |
| Integrity | SHA-256 from the Go standard library (`crypto/sha256`) | Decided (D-10) |
| Write protocol | Temp file with `O_CREAT` with `O_EXCL`, hash, `fsync`, publish with `link` (never `rename` over an existing name), directory `fsync`. `os.Root` since Go 1.24. | Proposed (P-37), facts verified from man pages and go.dev |
| Encryption at rest, retention | | Open (O-08) |

## 7. Credentials

The Linux MVP uses the Secret Service only and fails closed without it. No plaintext file, environment variable or standard input carries a secret (D-52, Decided). Library: `github.com/zalando/go-keyring` v0.2.8 (MIT, 2026-03-23) with `godbus/dbus/v5` v5.2.4 (BSD-2-Clause), behind the `CredentialStore` port, with a bus-socket preflight so `dbus-launch` never runs (Decided as a direction, P-59, D-85). The Secret Service specification is "Draft, Secret Service 0.2" and describes a service in the user's login session. Whether a worker started outside a desktop session can use it is **Requires validation** (O-17, O-21). Password hashing: Argon2id through `x/crypto/argon2`, m=65536 KiB, t=3, p=4, 16-byte salt, 32-byte key, with concurrent hashes bounded. These are starting values (D-68). The final values come from the M4 benchmark and are recorded here. Those parameters exceed every OWASP Argon2id minimum set and need a benchmark on the target laptop. Sessions are stored in PostgreSQL and sent in an HttpOnly cookie (D-60). Cookies are not isolated by port (RFC 6265 section 8.5), so SameSite is not relied on and Go checks Host, Origin and CSRF itself, with hand-written code and `x/time/rate` (Proposed, P-60).

## 8. AI provider adapters

Provider-agnostic adapters behind a port: **Decided** (D-07, D-62). MVP scope **Proposed** (P-53, O-26): one local adapter for a loopback server, and one generic OpenAI-compatible adapter for remote services, each key bound to one configured endpoint. Verified: OpenAI, OpenRouter, DeepSeek, Gemini (beta layer), Anthropic (compatibility layer, "not a long-term or production-ready solution") and Ollama (`localhost:11434/v1`, binds 127.0.0.1 by default) and LM Studio (`localhost:1234/v1`) expose an OpenAI-shaped chat path with a bearer key. A loopback Ollama can forward to Ollama's cloud unless disabled, so loopback does not prove local-only. Data egress follows D-46 and D-50. Provider SDKs are not used. Sources: `tech-verification/tools-ai-and-skills.md` 4.

## 9. Security tool integration policy

- **MVP selection (Decided as a direction, P-52, D-76):** Subfinder v2.17.0 (MIT, passive), httpx v1.12.0 (MIT, probing), Nuclei v3.11.1 (MIT) as an optional third in the Intrusive class with nuclei-templates v10.5.2 (MIT) as a pinned local store. All are built without cgo per their release configuration (inferred, not checked with `ldd`), emit JSONL, and publish SHA-256 checksum files only. No signature was found and GitHub attestations were not verified. Verified 2026-11-15 from the official repositories.
- **Outside the MVP:** Nmap 7.993 (Nmap Public Source License, legal review before any adapter, not bundled), ffuf v2.3.0 (MIT), OWASP ZAP v2.17.0 (Apache-2.0, needs Java 17 or later). Port scanners are outside the MVP (D-41).
- **Per-tool flags (Requires validation):** the shared updater contacts a project service unless `-duc` is set. Nuclei needs `-ni` (interactsh) and `-dut` and must not use `-ai`, `-code` or `-headless`. Subfinder's proxy comes only from `-proxy` per source reading, its `-active` mode must not be used, and its API keys must not come from environment variables. Until protected-file delivery is verified (D-52), Subfinder runs only keyless sources. Whether each tool sends a host name or a resolved IP to a proxy is not verified, and it decides whether "connect only to an evaluated address" holds.
- **Provisioning:** user-installed binaries pinned by a manifest that records our own SHA-256 of the extracted binary, re-verified before each launch (Proposed). Execution uses a sealed in-memory copy of the hashed bytes (spike S11).
- **Policy (Decided):** D-41, D-42, D-43, D-44, `security.md` 10. Tool risk classes are D-59.
- **Sandbox: bubblewrap v0.13.1** (LGPL-2.0-or-later per the source header, LGPL-2.1-or-later per distribution pages), run as an external program. **Conditional** (D-42). Local spike PASS on the reference host only (`docs/security/feasibility/sandbox-spike.md`): six namespaces and `--disable-userns` work unprivileged; the launcher must start the helper with an empty environment and close inherited descriptors, and mount the root and `/dev` read-only; the seccomp filter denying `ptrace`, `bpf`, `io_uring`, `keyctl`, `perf_event_open`, `mount` and `unshare` of a user namespace was verified on x86_64 only; Landlock ABI 10 is available; cgroup v2 `cpu`, `memory` and `pids` are delegated through `systemd-run --user`; the `io` controller is not; `RLIMIT_NPROC` cannot be used per job; there is no per-job disk quota; the self-test takes about 1.3 s and the sandbox about 20 ms to start. Other distributions are NOT TESTED.
- **Required resource limits (Decided, D-70):** wall-clock timeout, memory, process count, CPU time, file size, output size and a size-bounded memory-backed job working directory. An unavailable limit means the operation does not run (D-42). M8 must prove the bounded working directory on the baseline, otherwise no tool launches. A per-job I/O limit is not required and its absence is an accepted residual risk. A per-launch preflight of the helper digest and the required layers runs before every launch. Only statically linked tool binaries are supported. Bubblewrap comes from the distribution package and its path, version and digest are recorded and pinned by the preflight.
- **Egress: first-party client and policy proxy** are **Conditional** (D-41). Local spike PASS on the reference host (`docs/security/feasibility/egress-spike.md`): resolve once and dial the validated IP; `Dialer.Control` runs for every attempt including Happy Eyeballs fallbacks; no HTTP/2 coalescing was observed in Go or curl 8.23.0; a fresh network namespace reaches a policy proxy through a Unix-socket relay (a small Go relay is preferred over socat). Gaps: the proxy sees only `CONNECT host:port` for HTTPS, a Host override is neutralized but not refused, any process that can reach the proxy socket can use it (use one socket per job), and a running tunnel has no revocation hook.

## 10. API and schema generation

OpenAPI documents the API (D-02, Decided). **Proposed (P-43, O-04):** spec-first. Author OpenAPI 3.0.4, generate the Gin server with oapi-codegen v2.8.2, and generate the TypeScript client with openapi-typescript 7.13.0 and openapi-fetch 0.17.0 (last repository release 2026-02-11). Fallback: Orval 8.48.0. Whether oapi-codegen's Gin middleware validation behaves as needed is Requires validation. Generated code is committed and reviewed.

## 11. Logging and observability

Structured logging with the standard library `log/slog` (Go 1.21 or later) and a redacting handler (Proposed, P-45, O-14). Audit goes through the `AuditSink` port (D-12, D-23). Secrets never reach logs (`security.md` 6, 13). No telemetry. Health and readiness endpoints are part of the MVP (D-86, `docs/decisions/0004-health-endpoints-mvp-decision.md`): fixed status words, behind the Host allow-list, no dependency detail, and readiness confirms process initialization only until a probe is registered. Metrics endpoints are post-MVP (D-80).

## 12. Testing strategy

Proposed (P-46, O-18): `go test` with the standard library, optional testify; architecture tests inside the Go module over `go list -json` for the dependency directions of `structure.md` 5.6; integration tests against a temporary local PostgreSQL cluster; fuzz tests with `go test -fuzz` for scope parsing; Vitest, Testing Library and jsdom for the frontend; Playwright later with Chromium only. The feasibility spikes are reproducible: `bash scripts/spikes/sandbox/run-all.sh` and `bash scripts/spikes/egress/run-all.sh`. Not adopted: testcontainers (optional CI only), embedded-postgres, golangci-lint (GPL-3.0, separate process only, O-40).

## 13. Build and development commands

Rows marked yes exist. Rows marked no are intended (Proposed).

| Command | Exists | Purpose |
| --- | --- | --- |
| `bash scripts/spikes/sandbox/run-all.sh` | yes | Throwaway reference suite for the sandbox spike, about 25 s. It tests the Python spike launcher, so it is not a product gate. |
| `bash scripts/spikes/egress/run-all.sh` | yes | Throwaway reference suite for the egress spike, about 40 s. The product needs its own regression tests ported from it. |
| `scripts/sync-agent-skills.sh verify` | yes | Verify installed agent skills |
| `python3 scripts/validate-foundation.py` | yes | Foundation checks: decision register, references, milestone dependencies, ignore rules, repository safety scan. Exit status 1 on any failure. |
| `make test` | yes | `go vet` and `go test -race ./...` in `services/core` (includes the architecture test), then `next typegen`, `tsc --noEmit` and Vitest in `apps/web` |
| `make run-server`, `make run-web` | yes | Go API on 127.0.0.1:8080 and web dev server on 127.0.0.1:3000 (Phase 1 scaffold) |
| `next build` then `next start -H 127.0.0.1 -p <port>` in `apps/web` | yes | Serve the UI. The API origin of the rewrite is fixed at build time |
| Makefile targets for the PostgreSQL cluster and the worker | no | Process start-up, no launcher (Proposed, P-61, O-21). Arrive with M3 and M7 |

## 14. Dependency and licence policy

- The project is Apache-2.0. Every dependency needs a licence review before it is added (`security.md` 14). Permissive licences (MIT, BSD, Apache-2.0, the PostgreSQL License) are the default.
- MPL-2.0 public suffix data and any generated table, GPL-3.0 development tools and LGPL helpers invoked as external programs need an owner decision (O-40). This is not legal advice.
- Pin exact versions. Use stable releases only. Record the primary source and the verification date for each pin, and monitor advisories.
- Prefer the standard library. Add a dependency only when it removes real risk or work, and list it here first.
- Tool binaries and templates are pinned by SHA-256 and re-verified before launch (D-43).

## 15. Resource constraints

Target: a Linux laptop with 8 GB RAM. The host reported 7.7 GB total. Argon2id at 64 MiB per hash is about 0.8% of 8 GB, so concurrent hashes are bounded. The sandbox start-up cost measured about 12 to 22 ms in process. PostgreSQL idle memory, the Next.js production server memory and Playwright browser downloads were **not measured** (Requires validation). PostgreSQL documents typical defaults of `shared_buffers` 128MB and `max_connections` 100, and a reduction for the laptop needs a benchmark. No container runtime is required.

## 16. Explicitly deferred technology choices

Decided in Round 6 and no longer deferred: Node and package manager (D-66), PostgreSQL provisioning (D-67), password recovery (D-68), evidence encryption and retention defaults (D-77), skill manifest direction (D-82), report format (D-75). Still deferred or gated: browser support policy, metrics endpoints, a second authentication factor, the public suffix source (O-32) and the MPL licence question (D-74), a trusted-LAN AI category (O-36), Windows, macOS and headless platform choices, native desktop frameworks (D-18), numeric active-operation limits (O-38, D-71), heartbeat and kill timing (O-35, D-71), report formats beyond Markdown, encryption at rest after the MVP.
