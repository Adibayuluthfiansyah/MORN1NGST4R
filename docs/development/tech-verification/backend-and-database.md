# Technology verification: backend and database

Status: research record. Nothing here is a decision. Rows marked PROPOSED need owner approval.

## 1. Header

- Fetch date (UTC, from `date -u +%F`): **2026-11-15**. Every source in section 9 was fetched on this date.
- Method: read-only research using `web_fetch` against primary sources only (project sites and documentation, official GitHub repositories and `raw.githubusercontent.com` files, the Go module proxy `proxy.golang.org`, `pkg.go.dev`, `go.dev`, `postgresql.org`, the official `postgres/postgres` GitHub mirror, `cheatsheetseries.owasp.org`, `spec.openapis.org`). No secondary aggregator (endoflife.date, blogs) was used. No package was downloaded or installed. No existing file was edited.
- Terminology: "latest release" means the newest stable tag. "Supported release" means a release the owner of the project says it still maintains. The two are different and are stated separately wherever a primary source gives a support policy.
- Host environment (read-only, NOT a baseline):
  - `go version`: go1.27.3 linux/amd64. `go env`: GOOS=linux, GOARCH=amd64, GOTOOLCHAIN=auto, GOFLAGS empty.
  - `psql --version`: PostgreSQL 18.6. This is one minor behind the current 18.7 (section 2.2).
  - Kernel: Linux 7.2.11-zen1-1-zen x86_64. RAM from `free -m`: 7713 MiB total, 2720 MiB available at fetch time.
- Text extraction caveat: the `web_fetch` HTML extractor dropped the "The Locking Clause" section of `https://www.postgresql.org/docs/18/sql-select.html`. That passage was read from the official SGML source `doc/src/sgml/ref/select.sgml` on branch `REL_18_STABLE` of the `postgres/postgres` GitHub mirror (S28). The rendered page and the SGML are the same documentation.

### 1.1 What was NOT verified (summary)

Full list in section 8. The main gaps:

- Actual idle resident memory of a PostgreSQL instance. The documentation gives parameter defaults, not RSS.
- The release date of PostgreSQL 18.7, and whether PostgreSQL 19 exists as a release.
- The date of the Go 1.28 release (needed to know when Go 1.26 loses support).
- Gin's formal support or security policy. Its README states only a Go version prerequisite.
- Which PostgreSQL majors sqlc and goose officially support.
- OpenAPI 3.1 support in ogen. Request-validation behavior of the oapi-codegen Gin middleware.
- Whether testcontainers-go documents a Docker-API runtime requirement (inferred only from its go.mod).
- The SPDX variant of golangci-lint (`GPL-3.0-only` or `GPL-3.0-or-later`). The LICENSE file is the GNU GPL version 3 text.

## 2. Technology tables

All "Primary-source URLs" cells refer to the source register in section 9 (ID, URL, fetch date 2026-11-15).

### 2.1 Go toolchain

| Field | Value |
| --- | --- |
| Name / role | Go. Backend language and toolchain. |
| Latest stable verified | go1.27.4, released 2026-11-10 (S1, S2). Go 1.27.0 was released 2026-08-19. |
| Support window | "Each major Go release is supported until there are two newer major releases." Supported now: Go 1.27 (go1.27.4) and Go 1.26 (go1.26.11, released 2026-11-10). Go 1.26.0 was released 2026-02-10. Go 1.25 (1.25.0 released 2025-08-12) is no longer supported because 1.26 and 1.27 are both newer (derived from the policy text, S2). `go.dev/dl/?mode=json` lists only 1.27.4 and 1.26.11 as stable (S1). Go 1.26 loses support when Go 1.28 is released. That date was NOT VERIFIED. |
| License | Not fetched (BSD-style Go license). NOT VERIFIED. |
| Minimum requirement | The highest `go` directive among the verified dependencies is `go 1.26.0` (goose, ogen, huma, river, testcontainers-go, golangci-lint, x/crypto). Gin, pgx and oapi-codegen need `go 1.25.0` (section 4). |
| Trade-offs | Go 1.27 is about three months old. Go 1.26 is supported and still receives fixes. Both satisfy every `go` directive verified here. The host runs go1.27.3, one patch behind. |
| Verdict | PROPOSE toolchain go1.27.4 (latest stable, supported). PROPOSE `go 1.26.0` in `go.mod` as the module floor. Reason: goose requires it, and it keeps the Go 1.26 line usable. Go 1.26.11 is an equally supported fallback. |

### 2.2 PostgreSQL

| Field | Value |
| --- | --- |
| Name / role | PostgreSQL. Primary datastore, job queue, audit store. |
| Latest stable verified | PostgreSQL 18, current minor 18.7 (S3). First release 2025-09-25. The release date of 18.7 was NOT VERIFIED. |
| Support window | "The PostgreSQL Global Development Group supports a major version for 5 years after its initial release." Final release dates from the versioning page (S3): 18 on 2030-11-14, 17 on 2029-11-08, 16 on 2028-11-09, 15 on 2027-11-11, 14 on 2026-11-12. Current minors listed: 18.7, 17.12, 16.16, 15.20, 14.25. On the fetch date the page still shows 14 as "Supported: Yes", but its listed final release date (2026-11-12) has passed. Treat 14 as end of life. The page recommends: "always run the current minor release associated with their major version." |
| License | Not fetched. NOT VERIFIED. |
| Minimum requirement | pgx supports PostgreSQL 14 and higher (S10). River is tested against "the three most recent major versions of PostgreSQL" (S33), which on the S3 table are 18, 17 and 16. |
| Trade-offs | Major upgrades need dump/restore or `pg_upgrade` (S3). Choosing 18 gives the longest window (to 2030-11-14) and native `uuidv7()` (S27). The host has 18.6, so a minor upgrade is due. |
| Verdict | PROPOSE PostgreSQL 18, current minor (18.7 on the fetch date). Do not target 14 (end of life). Do not target a PostgreSQL 19 release: its existence as a stable release is NOT VERIFIED. |

### 2.3 Gin

| Field | Value |
| --- | --- |
| Name / role | `github.com/gin-gonic/gin`. HTTP framework. Inbound adapter only. |
| Latest stable verified | v1.12.0, tagged 2026-02-28 (S4). It is the newest entry in `@v/list`, so no later stable tag exists (S5). |
| Support window | No support or security policy was found in the README (S6). NOT VERIFIED. |
| License | MIT, from `LICENSE` at v1.12.0 (S7). |
| Minimum requirement | `go 1.25.0` in go.mod at v1.12.0 (S8). The README states "Gin requires Go version 1.25 or above" (S6). |
| Trade-offs | The README example says "Server will listen on 0.0.0.0:8080" for `r.Run()` (S6). The default does not bind loopback. `security.md` section 8 requires an explicit listen address. Use `http.Server{Addr: "127.0.0.1:<port>"}` and never bare `Run()`. |
| Verdict | KEEP v1.12.0. |

### 2.4 pgx

| Field | Value |
| --- | --- |
| Name / role | `github.com/jackc/pgx/v5`. PostgreSQL driver, `pgxpool`, LISTEN/NOTIFY client. |
| Latest stable verified | v5.13.0, 2026-11-11 (S9). v5.12.0 is 2026-10-31 (S11). The GitHub releases API still returned v5.11.0 (published 2026-09-07) as "latest" and as the only entry (S12). v5.12.0 and v5.13.0 exist as module tags with CHANGELOG entries but no GitHub Release object. Treat the proxy and CHANGELOG as authoritative. |
| Support window | README: "pgx supports the same versions of Go and PostgreSQL that are supported by their respective teams ... This means pgx supports Go 1.25 and higher and PostgreSQL 14 and higher." (S10). That sentence lags the Go policy, since Go 1.25 is no longer supported by the Go team. "pgx follows semantic versioning for the documented public API on stable releases." No patch-support policy for old minor versions is stated. |
| License | MIT, from `LICENSE` (S13). |
| Minimum requirement | `go 1.25.0` (S14). |
| Known issues | CHANGELOG (S15): v5.12.0 fixes SQL injection in simple-protocol batches (GHSA-j38q-h3ph-jjpq, high; extended-protocol modes unaffected) and a `require_auth` / `channel_binding=require` bypass in v5.9.0 to v5.11.0 when the server identity is not verified (GHSA-95fj-3fxw-g62g, high). v5.13.0 fixes a panic in `Rows.Values` on malformed server responses (GHSA-w9rp-vfrh-q8vv). v5.9.2 fixed SQL injection through dollar-quoted strings in simple protocol (GHSA-j88v-2chj-qfwx). The advisory pages themselves were NOT fetched. |
| Trade-offs | Do not use `QueryExecModeSimpleProtocol`. v5.11.0 changed connection-string parsing to match libpq. Check any DSN built by the app. |
| Verdict | PROPOSE v5.13.0. Hard floor v5.12.0 because of the two high-severity fixes. |

### 2.5 sqlc

| Field | Value |
| --- | --- |
| Name / role | `github.com/sqlc-dev/sqlc`. Code generator (build-time tool). Not linked into the product. |
| Latest stable verified | v1.31.1, 2026-04-22 (S16). |
| Support window | None found. NOT VERIFIED. |
| License | MIT, `LICENSE` at v1.31.1 (S17). |
| Minimum requirement | `go 1.26.0`, `toolchain go1.26.2` in its go.mod (S18). That matters only when building it from source. sqlc's own go.mod depends on pgx v5.9.2. Docs: "pgx/v5 is supported starting from v1.18.0"; set `sql_package: "pgx/v5"` (S19). Supported PostgreSQL majors: NOT VERIFIED. |
| Trade-offs | It is five months old. A newer tag would need re-checking before an upgrade. |
| Verdict | PROPOSE v1.31.1. Pin the tool version in the repo, for example with a `go tool` directive. |

### 2.6 Goose

| Field | Value |
| --- | --- |
| Name / role | `github.com/pressly/goose/v3`. Schema migrations. |
| Latest stable verified | v3.30.1, 2026-11-12 (S20, S21). Previous: v3.30.0 on 2026-11-10 and v3.29.0 on 2026-10-11 (S21, S22). |
| Support window | None found. NOT VERIFIED. |
| License | MIT, `LICENSE` at v3.30.1 (S23). |
| Minimum requirement | `go 1.26.0` (S24). It depends on pgx v5.12.0. Docs say it works with your own `*sql.DB` (S25). With pgx this means the `pgx/v5/stdlib` adapter. README: pgx's `database/sql` interface is available. That adapter was not tested here. |
| Known issues | v3.30.1 changelog (S21): fixes a build failure when `mfridman/interpolate` is upgraded, and a lock edge case where a long-lived locker could stop waiting. "Programs that migrate once at startup were not affected." v3.30.0 adds a container image and fixes version sorting for unpadded SQL migration versions. |
| Verdict | PROPOSE v3.30.1. The release is three days old on the fetch date. v3.29.0 (one month old) is a conservative fallback. No primary source calls v3.30.1 unsafe. |

### 2.7 OpenAPI generator (recommended candidate)

| Field | Value |
| --- | --- |
| Name / role | `github.com/oapi-codegen/oapi-codegen/v2`. Spec-first server and client generator. |
| Latest stable verified | v2.8.2, 2026-10-31 (S29). |
| Support window | None found. NOT VERIFIED. |
| License | Apache-2.0 (LICENSE text "Apache License Version 2.0, January 2004", S30). |
| Minimum requirement | `go 1.25.0` (S31). README: needs Go 1.25+ to build; generated Gin server code needs Go 1.25+ (S32). |
| Capabilities | README (S32): supports OpenAPI 3.0 and 3.1, with 3.1 support described as "initial". Supported servers include Gin (`gin-server`), plus a "strict server" mode. It does not support OpenAPI 2.0. It uses `kin-openapi v0.142.0` (S31). |
| Verdict | PROPOSE v2.8.2 for spec-first generation. See section 3.2. |

### 2.8 Supporting libraries and tools

| Name | Role | Latest stable (date) | License (source) | Min Go | Verdict |
| --- | --- | --- | --- | --- | --- |
| `log/slog` (stdlib) | Structured logging | in Go 1.21 and later (S34, S35) | Go license (not fetched) | Go 1.21 | KEEP stdlib. See 3.3. |
| `crypto/sha256` (stdlib) | Evidence SHA-256 | stdlib (S36) | Go license (not fetched) | any | KEEP. See 3.6. |
| `golang.org/x/crypto` | `argon2`, `scrypt`, `bcrypt` | v0.59.0, 2026-11-11 (S37) | BSD-3-Clause text (S38) | `go 1.26.0` (S39) | PROPOSE v0.59.0 for `argon2`. See 3.7. |
| `github.com/google/uuid` | UUID v7 generation | v1.6.0, 2024-01-23 (S40) | BSD-3-Clause text (S41) | no `go` directive in go.mod (S42) | PROPOSE v1.6.0. See 3.5. |
| `github.com/stretchr/testify` | Test assertions | v1.12.1, 2026-08-17 (S43) | MIT (S44) | `go 1.17` (S45) | PROPOSE v1.12.1 (optional). |
| `github.com/riverqueue/river` | Queue library (alternative) | v0.51.2, 2026-11-09 (S46) | MPL-2.0 (S47) | `go 1.26.0`, toolchain go1.26.6 (S48) | Not the default. See 3.4. |
| `github.com/testcontainers/testcontainers-go` | Integration DB (container) | v0.45.0, 2026-11-05 (S49) | MIT (S50) | `go 1.26.0`, toolchain go1.26.10 (S51) | Optional only. See 3.6. |
| `github.com/fergusstrange/embedded-postgres` | Integration DB (embedded) | v1.34.0, 2026-03-17 (S52) | MIT (S53) | `go 1.18` (S54) | Not recommended. See 3.6. |
| `github.com/fe3dback/go-arch-lint` | Architecture lint | v1.19.1, 2026-10-29 (S55) | MIT (S56) | `go 1.25.0` (S57) | Alternative. See 3.6. |
| `github.com/golangci/golangci-lint/v2` | Linter runner (includes depguard, sloglint, testifylint) | v2.15.0, 2026-10-16 (S58) | GPL-3.0 text (S59) | `go 1.26.0` (S60) | Optional dev tool. See 3.6. |
| `github.com/ogen-go/ogen` | OpenAPI generator (alternative) | v1.25.0, 2026-11-13 (S61) | Apache-2.0 (S62) | `go 1.26.0` (S63) | Not the default. See 3.2. |
| `github.com/swaggo/swag` | Annotation to Swagger 2.0 | v1.16.6, 2025-07-28 (S64); v2 only `v2.0.0-rc6`, 2026-09-13 (S65) | MIT (S66) | `go 1.18` (S67) | Not recommended. See 3.2. |
| `github.com/danielgtaylor/huma/v2` | Framework layer with OpenAPI 3.1 | v2.40.0, 2026-10-27 (S68) | MIT (S69) | `go 1.26.0` (S70) | Not the default. See 3.2. |

## 3. Open decisions

### 3.1 O-01 PostgreSQL provisioning for a single-user local install

Verified facts (PostgreSQL major 18 documentation unless noted):

- Defaults (S71, S72): `shared_buffers` is "typically 128 megabytes (128MB), but might be less if your kernel settings will not support it (as determined during initdb)". It is set at server start. `max_connections` defaults to "typically 100". `work_mem` is 4MB. `maintenance_work_mem` is 64MB. `temp_buffers` is 8MB. `listen_addresses` defaults to `localhost`. If it is empty, "only Unix-domain sockets can be used". The documentation states no idle resident-set size. Idle RAM use is therefore NOT VERIFIED. The 128MB figure is the configured size of the main shared buffer area, not a measured idle footprint.
- Unix socket (S72): `unix_socket_directories` default is "normally /tmp". `unix_socket_permissions` default is 0777, "meaning anyone can connect". The documentation names 0770 and 0700 ("only user") as reasonable alternatives. The setting is "independent of" pg_hba.conf authentication. For sockets only write permission matters.
- Peer authentication (S73): the server takes the client's OS user name from the kernel and uses it as the database user name, with an optional `map`. It is supported on local connections only, on Linux and most BSDs.
- `initdb` (S74): the new cluster "will only be accessible by the cluster owner by default". `initdb` refuses to run as root and the server cannot run as root. `--auth-local` sets the method for Unix-socket (`local`) lines. "Do not use trust unless you trust all local users on your system. trust is the default for ease of installation." The default `password_encryption` is `scram-sha-256` (S72).

| Option | Needs root? | Needs container runtime? | Notes |
| --- | --- | --- | --- |
| A. User-provided system PostgreSQL | Installing the package usually needs root (package manager). NOT VERIFIED for any specific distribution. | No | Least code. The app cannot guarantee socket permissions, `pg_hba.conf` or `listen_addresses`. `security.md` section 15 requires owner-only data access. The system cluster is typically shared. |
| B. App-managed local instance (`initdb` / `pg_ctl`, per-user data dir, unix socket only) | No, provided the PostgreSQL server binaries are already installed. Installing them may need root. Source of binaries NOT VERIFIED. | No | The app can set `listen_addresses=''`, a 0700 socket directory and `unix_socket_permissions=0700`. It owns the data directory permissions. It adds code for lifecycle, upgrades and version checks. |
| C. Container | Depends on the runtime. NOT VERIFIED. | Yes | Adds a runtime dependency. Not needed for a localhost MVP. |
| D. Remote or hosted | No | No | Conflicts with local-first and `security.md` section 8. It would need TLS and separate security review. |

Security implications of unix-socket-only access with peer authentication (items 2 and 3 are inference from the cited documentation, not quoted facts):

1. Removing TCP removes network exposure. `listen_addresses=''` is a documented way to do it.
2. Peer authentication identifies the OS user, not the process. Any process running as the same OS user can connect as the mapped database role. The tool sandbox in `security.md` section 10 must therefore be unable to reach the socket path, or must run as another uid. Otherwise "deny tools access to application credentials" is not met for the database.
3. `security.md` section 13 requires different database roles for the server and worker (INSERT and SELECT on audit tables) and for retention purges. Peer authentication with an `ident` map lets one OS user map to several roles, so it does not separate privileges between processes that share a uid. Separation then needs distinct OS users or password (SCRAM) authentication over the socket, with the password held in the credential store.
4. With the default 0777 socket permission, other local users can reach the socket and only pg_hba.conf stops them. Set 0700.

PROPOSED default (not decided): **B**, with the app locating the PostgreSQL 18 binaries by absolute path and checking the version. A "bring your own DSN" path (A) stays available as an explicit, documented alternative. The settings the app should apply: `listen_addresses=''`, a socket directory owned by the user with mode 0700, `unix_socket_permissions=0700`, `initdb --auth-local=scram-sha-256` or peer with distinct roles per the point above, and a reduced `shared_buffers` and `max_connections` if the benchmark shows memory pressure. The reduced values have no primary source and must be benchmarked on the 8 GB laptop. Owner decision needed on who ships the PostgreSQL binaries (user package versus bundled), because that is where B needs root or adds supply-chain work.

### 3.2 O-04 OpenAPI strategy for Gin

Verified candidates (details in section 2.7 and 2.8):

| Candidate | Style | Version, last release | License | Official Gin support | OpenAPI version | Activity |
| --- | --- | --- | --- | --- | --- | --- |
| oapi-codegen | Spec-first generator | v2.8.2, 2026-10-31 | Apache-2.0 | Yes. `gin-server` generator, generated code Go 1.25+ (S32) | 3.0 and 3.1 ("initial" 3.1 support). No 2.0 (S32) | Releases in the last month (proxy). Repo activity metrics not fetched. |
| ogen | Spec-first generator | v1.25.0, 2026-11-13 | Apache-2.0 | Not listed. It generates its own static router and `Server` interface (S61 README). Mounting under Gin is an inference only. | "OpenAPI v3 Code Generator". 3.1 NOT VERIFIED. | Release two days before fetch. |
| swaggo/swag v1 | Annotation based | v1.16.6, 2025-07-28 | MIT | Yes, through `swaggo/gin-swagger` (S64 README) | Swagger 2.0 only ("generate Swagger 2.0 docs") | Repo `pushed_at` 2026-09-30, 488 open issues, not archived (S65 API). v2 (OpenAPI 3.1) exists only as `v2.0.0-rc6`, a pre-release. |
| huma v2 | Framework layer | v2.40.0, 2026-10-27 | MIT | A `humagin` adapter package exists in the repo (S68); `go.mod` imports gin v1.12.0 | "Described by OpenAPI 3.1" (S68 README) | Release in the last month. |

OpenAPI specification versions published on `spec.openapis.org` (S73b): v3.2.1, v3.2.0, v3.1.2, v3.1.1, v3.1.0, v3.0.4 and earlier 3.0.x, v2.0.

Trade-offs:

- Spec-first generation matches `structure.md` section 11: "Backend owns business contracts", OpenAPI documents the API, and a TypeScript client is generated from it. The generated `ServerInterface` keeps handlers thin. Generated code belongs in `adapters/inbound/http`.
- Annotation-based tools (swag) derive the spec from comments, so the spec can drift from the code, and the stable line emits only Swagger 2.0 (OpenAPI 2.0), which oapi-codegen explicitly does not support (S32). Its OpenAPI 3.1 line is a release candidate. Not recommended.
- A framework layer (huma) takes over request parsing, validation and error shape. That adds a second framework on top of the owner-chosen Gin.
- ogen would run its own router next to Gin. No primary source supports Gin directly.

PROPOSED default: **spec-first with oapi-codegen v2.8.2 and the `gin-server` (or strict-server) generator.** Author the spec as OpenAPI 3.0.4. Reason: 3.1 support in oapi-codegen is described as "initial", so 3.0 is the lower-risk path. Move to 3.1 once the generated output for the real spec is reviewed. Request validation with the oapi-codegen Gin middleware was NOT VERIFIED and must be checked before relying on it for `security.md` section 11 input validation. Handler-level validation stays required either way.

### 3.3 O-14 Logging

Verified: the `log/slog` package was added in Go 1.21 ("The new log/slog package provides structured logging with levels", S34). The package documentation describes `Logger`, `Handler` and key-value attributes (S35). The Go 1.21 notes state library changes are "made with the Go 1 promise of compatibility in mind" (S34). The package is part of the standard library for every Go release recommended here (1.26 and 1.27). The `golangci-lint` linter list includes `sloglint` and `loggercheck` (S61b). pgx lists a third-party `pgx-slog` tracer adapter (S10). Integration of Gin's request logger with slog was NOT VERIFIED.

Trade-offs: slog adds no dependency and has a handler interface, which is the hook for the redaction that `security.md` section 13 requires (no secrets, cookies or authorization headers in logs). Third-party loggers (zap, zerolog) add dependencies for speed the MVP does not need. No benchmark was run.

PROPOSED default: `log/slog` from the standard library with a custom redacting `Handler`.

### 3.4 O-12 PostgreSQL-backed queue

Documentation facts, all from PostgreSQL 18 documentation (page and major version in each line):

1. **SKIP LOCKED** (docs/18 `sql-select.html`, section "The Locking Clause", read from `REL_18_STABLE` SGML, S28): "With SKIP LOCKED, any selected rows that cannot be immediately locked are skipped. Skipping locked rows provides an inconsistent view of the data, so this is not suitable for general purpose work, but can be used to avoid lock contention with multiple consumers accessing a queue-like table." `NOWAIT` and `SKIP LOCKED` apply only to row-level locks. The ROW SHARE table lock "is still taken in the ordinary way". With a `LIMIT`, "locking stops once enough rows have been returned" and rows skipped by `OFFSET` are locked. A READ COMMITTED `SELECT` with `ORDER BY` and a locking clause "might block trying to obtain a lock on one or more of the rows", so rows can come back out of order. `SKIP LOCKED` is not allowed with `FETCH ... WITH TIES`. `FOR UPDATE` requires `UPDATE` privilege as well as `SELECT` (docs/18 `sql-select.html`, Description).
2. **LISTEN/NOTIFY** (docs/18 `sql-notify.html`, S75): "In the default configuration it must be shorter than 8000 bytes." A `NOTIFY` inside a transaction is "not delivered until and unless the transaction is committed". A listening session that is inside a transaction receives the event only after that transaction ends. Identical channel and payload within one transaction are delivered once. Order is the commit order across transactions. The notification queue is 8GB in a standard installation. If it fills, transactions calling `NOTIFY` fail at commit. A transaction that executed `NOTIFY` cannot be prepared for two-phase commit. The text describes delivery only to sessions that "have previously executed LISTEN". It does not promise delivery to a disconnected listener, so a `NOTIFY` is a wake-up hint and the worker needs a polling fallback (inference).
3. **INSERT ... RETURNING** (docs/18 `sql-insert.html`, S76): "Use of the RETURNING clause requires SELECT privilege on all columns mentioned in RETURNING." `ON CONFLICT` forms also need `SELECT` on columns they read. This matters for any role limited to INSERT.
4. **READ COMMITTED re-evaluation** (docs/18 `transaction-iso.html`, section 13.2.1, S77): `UPDATE`, `DELETE` and `SELECT FOR UPDATE` find only rows committed at command start. If a target row was changed by a concurrent transaction, the second command waits. If the first commits, the second ignores the row when it was deleted, "otherwise it will attempt to apply its operation to the updated version of the row. The search condition of the command (the WHERE clause) is re-evaluated to see if the updated version of the row still matches." For `SELECT FOR UPDATE`, "it is the updated version of the row that is locked and returned". The page also warns that an updating command "can see an inconsistent snapshot" of other rows. Consequence for a claim query (inference from this text): keep the state predicate (for example `state = 'queued'`) in the same statement that takes the lock, so it is re-checked against the committed row version.

Candidates:

| | Custom (`FOR UPDATE SKIP LOCKED` with leases and fencing) | River (`riverqueue/river`) |
| --- | --- | --- |
| License | none (project code) | MPL-2.0 (S47). Compatibility with Apache-2.0 distribution is for the owner to review. Not a legal verification. |
| Version, maintenance | n/a | v0.51.2, 2026-11-09. Every release is v0.x, so there is no v1 stability promise. More than 100 tags in the proxy list (S78). |
| pgx v5 | Native | Yes. `riverdriver/riverpgxv5` (S33). go.mod requires pgx v5.11.0. |
| Go / PostgreSQL | any | `go 1.26.0`, toolchain go1.26.6. "River is tested using the three most recent major versions of PostgreSQL" (S33). |
| Schema | Owned by the project and by Goose | River's own tables plus leader election, created by its CLI `river migrate-up` (S33). It says to use one migration tool when a database is shared. How River migrations fit into Goose was NOT VERIFIED. |
| Fit with `security.md` | Approval use can be consumed in the same transaction that claims the job, with DB constraints. That is the exact requirement in section 4 ("consumed atomically when the worker claims the job", "database constraints must prevent a second use"). | The jobs table is River's. Whether a caller can run its own statements inside River's claim transaction was NOT VERIFIED. The claim path is internal to the library. |

The lease and fencing design is not taken from any documentation. It is a design proposal: a `lease_expires_at` column and a monotonically increasing `fencing_token` set at claim. Every completion, heartbeat and result write includes `WHERE id = $1 AND fencing_token = $2`. The approval-use row is inserted in the claim transaction under a unique constraint on the job id. Retries share the use.

PROPOSED default: **custom queue behind the outbound `JobQueue` port**, using `FOR UPDATE SKIP LOCKED`, leases, fencing tokens and polling, with `NOTIFY` only as a wake-up hint. Reason: the approval-consumption and revalidation rules need control of the claim transaction, and the job state belongs to the project's own `job` domain and migrations. River remains a documented alternative if the owner prefers less code and accepts the pre-1.0 and schema-ownership trade-offs. Only River was verified as a library. A second library (`vgarvardt/gue/v5`, v5.9.0 dated 2025-09-29 on the proxy) was found but its license, go.mod and status were NOT VERIFIED, so it is not a candidate.

### 3.5 O-20 ID format

Verified:

- PostgreSQL 18 (docs/18 `functions-uuid.html`, S27) provides `gen_random_uuid()` and `uuidv4()` (version 4, random) and `uuidv7([shift interval])` ("Generates a version 7 (time-ordered) UUID. The timestamp is computed using UNIX timestamp with millisecond precision + sub-millisecond timestamp + random").
- PostgreSQL 17 (docs/17 `functions-uuid.html`, S27b) lists only `gen_random_uuid()` (version 4). `uuidv7()` is therefore not available before 18 per the documentation fetched. PostgreSQL 16 was not checked.
- `google/uuid` v1.6.0 (2024-01-23) exports `NewV7`, `NewV7FromReader`, `NewV6`, `NewRandom` and `New` (S79). License text is BSD-3-Clause (S41). Its go.mod has no `go` directive (S42). No release after v1.6.0 exists on the proxy as of the fetch date, so the library has been unchanged for about 22 months. That is a maintenance signal, not a defect.
- Sequences used for identity columns are not rolled back (docs/18 `transaction-iso.html`, note in section 13.2, S77).
- ULID: no PostgreSQL native type and no library were verified. NOT VERIFIED. RFC 9562 text and how pgx maps `uuid` (for example `pgtype.UUID`) were NOT fetched.

| Option | Facts and notes |
| --- | --- |
| UUID v4 | Native `uuid` type, `gen_random_uuid()`. Random, so no ordering. Index-locality effects were NOT VERIFIED from a primary source. |
| UUID v7 | Native `uuid` type. Time-ordered. `NewV7` in google/uuid or `uuidv7()` on PostgreSQL 18. The embedded millisecond timestamp reveals creation time (inference from the PostgreSQL text). |
| ULID | NOT VERIFIED. |
| bigint identity | Native. Guessable and enumerable (inference). The ID exists only after insert. |

PROPOSED default: **UUID v7, generated in the application** (`google/uuid` v1.6.0 `NewV7`) and stored in a native `uuid` column. Reasons: the domain can create an ID before persistence, it does not depend on the PostgreSQL major (17 lacks `uuidv7()`), and it keeps ordering. Per `structure.md` section 5.6 the domain must not import the library, so an ID generator belongs behind a port or in an adapter and the domain holds only a value type. PostgreSQL 18 `uuidv7()` is an acceptable default expression if the baseline stays at 18. Because v7 IDs expose creation time, authorization must never rely on ID secrecy.

### 3.6 O-18 Test tooling

Verified: `go test` is the standard runner. `testify` v1.12.1 (MIT, `go 1.17`) is a dependency of Gin and pgx already (go.mod files S8, S14). The golangci-lint linter list includes `depguard` ("checks if package imports are in a list of acceptable packages"), `testifylint` and `sloglint` (S61b). golangci-lint v2.15.0 bundles `depguard/v2 v2.2.1` (S60). Its license file is the GNU GPL version 3 text (S59). It runs as a separate dev tool and is not linked into the product, but the owner should confirm that is acceptable for an Apache-2.0 project. `go-arch-lint` v1.19.1 is MIT with `go 1.25.0`. Its configuration format was not read, so its capabilities are NOT VERIFIED beyond existence and license.

Integration-test PostgreSQL options:

| Option | Verified facts | Resource and risk notes |
| --- | --- | --- |
| Local instance (temp cluster via `initdb`/`pg_ctl`) | pgx's own test suite uses per-checkout PostgreSQL 14 to 18 clusters (S10). `initdb` runs unprivileged and defaults to an owner-only data directory (S74). | No new Go dependency, no network download, no container runtime. Each cluster allocates its shared memory per the defaults in 3.1. Lower them in test clusters. |
| testcontainers-go v0.45.0 | MIT. `go 1.26.0`. Imports `moby/moby/client` (S51), which implies a Docker-API runtime. An explicit documentation statement was NOT VERIFIED. Tested against the two latest Go releases (S80). | Needs a container runtime and a large dependency tree. Not suited to a "no root, no runtime" developer setup. |
| embedded-postgres v1.34.0 | MIT. Downloads PostgreSQL binaries from Maven (`https://repo1.maven.org/maven2` by default) into `$USER_HOME/.embedded-postgres-go/`. Default version 18.3.0 (older than 18.7). Default password `postgres`, default port 5432 (S81). | A runtime binary download without a digest check described in the README is a supply-chain concern under `security.md` section 14. It also pins an older PostgreSQL than the proposed baseline. |

PROPOSED default (conservative):

- Unit tests with `go test` and the standard library. Optionally `testify` v1.12.1.
- Architecture rules: a standard-library test in `services/core/tests/architecture/` that runs `go list -json ./...` and asserts the dependency matrix of `structure.md` section 5.6. It adds no dependency. This was not prototyped, so the effort is unmeasured. `depguard` through golangci-lint is an optional second layer. `go-arch-lint` is an alternative if its configuration can express the domain dependency order. That is NOT VERIFIED.
- Database integration tests against a temporary local cluster (same mechanism as option B in 3.1). testcontainers-go only as an optional CI add-on.

### 3.7 Evidence integrity and password hashing

Evidence integrity (S36): `crypto/sha256` "implements the SHA224 and SHA256 hash algorithms as defined in FIPS 180-4". `sha256.New()` returns a `hash.Hash` for streaming a file. `sha256.Sum256` hashes a byte slice. This meets the SHA-256 requirement of `security.md` section 9 with no third-party dependency.

Password hashing:

- `golang.org/x/crypto` v0.59.0 (2026-11-11), BSD-3-Clause text, `go 1.26.0` (S37 to S39).
- `argon2.IDKey(password, salt, time, memory, threads, keyLen)`. The package documentation recommends Argon2id and gives two RFC 9106 Section 4 options: (1) time=1, memory=2*1024*1024 KiB (2 GiB), threads=4; (2) "If much less memory is available", time=3, memory=64*1024 KiB (64 MiB), threads=4 (S82).
- OWASP Password Storage Cheat Sheet (S83). "Use one of the following minimum configurations" for **Argon2id**, pasted exactly:
  - m=47104 (46 MiB), t=1, p=1 (Do not use with Argon2i)
  - m=19456 (19 MiB), t=2, p=1 (Do not use with Argon2i)
  - m=12288 (12 MiB), t=3, p=1
  - m=9216 (9 MiB), t=4, p=1
  - m=7168 (7 MiB), t=5, p=1
  - "These configuration settings provide an equal level of defense, and the only difference is a trade off between CPU and RAM usage."
- OWASP for **scrypt** (used "when the former [Argon2id] is not available"):
  - N=2^17 (128 MiB), r=8 (1024 bytes), p=1
  - N=2^16 (64 MiB), r=8 (1024 bytes), p=2
  - N=2^15 (32 MiB), r=8 (1024 bytes), p=3
  - N=2^14 (16 MiB), r=8 (1024 bytes), p=5
  - N=2^13 (8 MiB), r=8 (1024 bytes), p=10
- OWASP for **bcrypt**: "should only be used for password storage in legacy systems where Argon2 and scrypt are not available". Work factor "a minimum of 10". bcrypt has a 72-byte maximum input length for most implementations, so enforce a 72-byte maximum password length. OWASP warns against naive pre-hashing (null-byte and password-shucking issues).
- OWASP on work factor: "As a general rule, calculating a hash should take less than one second", and a very high factor can enable denial of service by exhausting CPU through many login attempts.

PROPOSED default: **Argon2id via `golang.org/x/crypto/argon2` with m=65536 KiB (64 MiB), t=3, p=4** (the second RFC 9106 option from the package documentation). It exceeds every OWASP minimum above. Benchmark on the target laptop and keep the time under one second. Never go below the OWASP minimum m=19456, t=2, p=1. Store the parameters with each hash and re-hash at the next login when the stored parameters are below the current policy. Use a random salt and constant-time comparison. Memory-bound DoS is limited by the rate limiting that `security.md` section 8 already requires. A 64 MiB hash on a laptop with 2.7 GiB free at fetch time is acceptable for one local account, but it should run under a concurrency limit. scrypt is the documented fallback and bcrypt is legacy-only. Whether `x/crypto/argon2` offers a PHC string encoder was NOT VERIFIED, so the app likely needs its own encoding of parameters.

## 4. Minimum Go version per library (from `go.mod` at the release tag)

| Module | Tag | `go` directive | Source |
| --- | --- | --- | --- |
| gin | v1.12.0 | 1.25.0 | S8 |
| pgx/v5 | v5.13.0 | 1.25.0 | S14 |
| sqlc | v1.31.1 | 1.26.0 (toolchain go1.26.2) | S18 |
| goose/v3 | v3.30.1 | 1.26.0 | S24 |
| oapi-codegen/v2 | v2.8.2 | 1.25.0 | S31 |
| ogen | v1.25.0 | 1.26.0 | S63 |
| swag | v1.16.6 | 1.18 | S67 |
| huma/v2 | v2.40.0 | 1.26.0 | S70 |
| river | v0.51.2 | 1.26.0 (toolchain go1.26.6) | S48 |
| golang.org/x/crypto | v0.59.0 | 1.26.0 | S39 |
| google/uuid | v1.6.0 | no directive | S42 |
| testify | v1.12.1 | 1.17 | S45 |
| testcontainers-go | v0.45.0 | 1.26.0 (toolchain go1.26.10) | S51 |
| embedded-postgres | v1.34.0 | 1.18 | S54 |
| go-arch-lint | v1.19.1 | 1.25.0 | S57 |
| golangci-lint/v2 | v2.15.0 | 1.26.0 | S60 |

## 5. Compatibility matrix

Only what the documents above state. "go.mod floor" means the declared minimum. Nothing here was build-tested.

| Item | Go | PostgreSQL | Other |
| --- | --- | --- | --- |
| Go 1.26.11 / 1.27.4 | n/a | n/a | Both supported by Go (S1, S2). Meet every floor in section 4. Go 1.25 is out of support. |
| pgx v5.13.0 | `go 1.25.0` floor | 14 and higher (README, S10); tested on 14 to 18 | Requires `golang.org/x/text`, `puddle/v2` (S14). |
| sqlc v1.31.1 | `go 1.26.0` to build from source | NOT VERIFIED | `sql_package: "pgx/v5"` supported since sqlc v1.18.0 (S19). Its own go.mod uses pgx v5.9.2. |
| goose v3.30.1 | `go 1.26.0` floor | NOT VERIFIED | `database/sql` based (S25). go.mod requires pgx v5.12.0 (S24). |
| Gin v1.12.0 | `go 1.25.0` floor | n/a | Use with stdlib `http.Server` bound to loopback. |
| oapi-codegen v2.8.2 | `go 1.25.0` floor | n/a | Gin generator, generated code needs Go 1.25+ (S32). |
| River v0.51.2 (if chosen) | `go 1.26.0` | Three newest majors (S33) | pgx v5.11.0 in go.mod. |
| PostgreSQL majors | n/a | 18 (to 2030-11-14), 17, 16, 15 supported. 14 final release 2026-11-12 | `uuidv7()` is in 18 only among 17 and 18 (S27, S27b). |

Version selection (Go minimal version selection): goose asks for pgx v5.12.0, sqlc for v5.9.2 and River for v5.11.0. Requiring pgx v5.13.0 in the project `go.mod` satisfies all of them by the module system's rules. No build was run to confirm it.

## 6. Pins and "do not upgrade merely because newer exists"

| Technology | Pin | Reason |
| --- | --- | --- |
| Go toolchain | go1.27.4 (module floor `go 1.26.0`) | Latest stable and supported. Pin the toolchain in `go.mod` or CI. Do not move to Go 1.28 until it is released and the dependency set supports it. |
| PostgreSQL | 18, current minor (18.7 on the fetch date) | Longest support window. Stay on 18 for the MVP. Take minors, which the versioning policy recommends. Do not adopt 19 until it is a stable release. |
| Gin | v1.12.0 | Only stable latest. Re-check on each upgrade. |
| pgx | v5.13.0 (hard floor v5.12.0) | Security fixes in 5.12.0 and 5.13.0. This is a security-driven pin, not "newest for its own sake". |
| sqlc | v1.31.1 | Latest stable. Generated output must be reviewed on any bump. |
| goose | v3.30.1 (fallback v3.29.0) | Latest. It is three days old. Take the fallback if a regression appears. |
| oapi-codegen | v2.8.2 | Latest stable. 3.1 support is "initial", so author the spec as 3.0.4. |
| x/crypto | v0.59.0 | Latest at the fetch date. No support window is stated for `x/` modules. Track advisories. |
| google/uuid | v1.6.0 | Latest. Nothing newer exists. |
| testify | v1.12.1 | Latest, optional. |
| Do not pin | swag `v2.0.0-rc6`, River (pre-1.0, if the custom queue is chosen), embedded-postgres (runtime download, 18.3.0 default) | Pre-release, schema ownership conflict, supply-chain concern. |

## 7. Do not take from this record

- No version here has been build-tested together. The matrix lists declared floors only.
- PROPOSED items are recommendations for the owner. None is a decision.
- No finding in this record concerns a target system. This is analysis of dependencies.

## 8. Claims attempted and NOT VERIFIED

1. Idle resident memory (RSS) of a PostgreSQL instance. Only parameter defaults were verified.
2. Release date of PostgreSQL 18.7. The release notes page was fetched but its date did not appear in the extracted text.
3. Whether PostgreSQL 19 is released, and its date.
4. The date of Go 1.28, and therefore the exact end of support for Go 1.26.
5. The license text of the Go project and of PostgreSQL (not fetched).
6. Gin formal support or security policy.
7. Which PostgreSQL majors sqlc and goose officially support.
8. Whether `pgx/v5/stdlib` works with goose v3.30.1 in this project (docs say goose accepts any `*sql.DB`).
9. ogen support for OpenAPI 3.1. Any official Gin integration for ogen (none listed).
10. Request-validation behavior and maintenance of `oapi-codegen/gin-middleware`.
11. GitHub repository activity metrics (commits, open issues) for oapi-codegen, ogen, huma and River. Only release dates and, for swag, `pushed_at` were fetched.
12. The text of the pgx security advisories (IDs were taken from the CHANGELOG only).
13. Whether River exposes a hook to run custom statements, such as approval consumption, inside its claim transaction.
14. How River's migrations can be combined with Goose.
15. Licensing compatibility of MPL-2.0 (River) and GPL-3.0 tools (golangci-lint) with an Apache-2.0 project. This is a legal question.
16. SPDX variant of golangci-lint (`-only` or `-or-later`).
17. testcontainers-go documented container-runtime requirement. Whether rootless operation works.
18. go-arch-lint configuration capabilities. Whether depguard can express the full domain dependency order of `structure.md`.
19. `vgarvardt/gue/v5` license, go.mod, maintenance status.
20. ULID support in PostgreSQL or Go libraries. RFC 9562 text. How pgx and sqlc map the `uuid` type. UUID index-locality claims.
21. PostgreSQL 16 documentation on `uuidv7()` (only 17 and 18 were checked).
22. Whether `x/crypto/argon2` provides a PHC string encoder.
23. Whether any package manager installs PostgreSQL without root on a specific Linux distribution, and where users would obtain server binaries for option B.
24. Integration of Gin's request logger with `log/slog`.
25. Go 1.27.4 and 1.26.11 release contents (the page extract showed dates only for these two).

## 9. Source register (all fetched 2026-11-15)

| ID | URL |
| --- | --- |
| S1 | https://go.dev/dl/?mode=json |
| S2 | https://go.dev/doc/devel/release |
| S3 | https://www.postgresql.org/support/versioning/ |
| S4 | https://proxy.golang.org/github.com/gin-gonic/gin/@latest |
| S5 | https://proxy.golang.org/github.com/gin-gonic/gin/@v/list |
| S6 | https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/README.md |
| S7 | https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/LICENSE |
| S8 | https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/go.mod |
| S9 | https://proxy.golang.org/github.com/jackc/pgx/v5/@latest |
| S10 | https://raw.githubusercontent.com/jackc/pgx/v5.13.0/README.md |
| S11 | https://proxy.golang.org/github.com/jackc/pgx/v5/@v/v5.12.0.info |
| S12 | https://api.github.com/repos/jackc/pgx/releases/latest |
| S13 | https://raw.githubusercontent.com/jackc/pgx/v5.13.0/LICENSE |
| S14 | https://raw.githubusercontent.com/jackc/pgx/v5.13.0/go.mod |
| S15 | https://raw.githubusercontent.com/jackc/pgx/v5.13.0/CHANGELOG.md |
| S16 | https://proxy.golang.org/github.com/sqlc-dev/sqlc/@latest |
| S17 | https://raw.githubusercontent.com/sqlc-dev/sqlc/v1.31.1/LICENSE |
| S18 | https://raw.githubusercontent.com/sqlc-dev/sqlc/v1.31.1/go.mod |
| S19 | https://docs.sqlc.dev/en/v1.31.1/guides/using-go-and-pgx.html |
| S20 | https://proxy.golang.org/github.com/pressly/goose/v3/@latest |
| S21 | https://raw.githubusercontent.com/pressly/goose/v3.30.1/CHANGELOG.md |
| S22 | https://proxy.golang.org/github.com/pressly/goose/v3/@v/v3.30.0.info |
| S23 | https://raw.githubusercontent.com/pressly/goose/v3.30.1/LICENSE |
| S24 | https://raw.githubusercontent.com/pressly/goose/v3.30.1/go.mod |
| S25 | https://pressly.github.io/goose/ |
| S27 | https://www.postgresql.org/docs/18/functions-uuid.html |
| S27b | https://www.postgresql.org/docs/17/functions-uuid.html |
| S28 | https://raw.githubusercontent.com/postgres/postgres/REL_18_STABLE/doc/src/sgml/ref/select.sgml and https://www.postgresql.org/docs/18/sql-select.html |
| S29 | https://proxy.golang.org/github.com/oapi-codegen/oapi-codegen/v2/@latest |
| S30 | https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.2/LICENSE |
| S31 | https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.2/go.mod |
| S32 | https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.8.2/README.md |
| S33 | https://riverqueue.com/docs |
| S34 | https://go.dev/doc/go1.21 |
| S35 | https://pkg.go.dev/log/slog |
| S36 | https://pkg.go.dev/crypto/sha256 |
| S37 | https://proxy.golang.org/golang.org/x/crypto/@latest |
| S38 | https://raw.githubusercontent.com/golang/crypto/v0.59.0/LICENSE |
| S39 | https://raw.githubusercontent.com/golang/crypto/v0.59.0/go.mod |
| S40 | https://proxy.golang.org/github.com/google/uuid/@latest |
| S41 | https://raw.githubusercontent.com/google/uuid/v1.6.0/LICENSE |
| S42 | https://raw.githubusercontent.com/google/uuid/v1.6.0/go.mod |
| S43 | https://proxy.golang.org/github.com/stretchr/testify/@latest |
| S44 | https://raw.githubusercontent.com/stretchr/testify/v1.12.1/LICENSE |
| S45 | https://raw.githubusercontent.com/stretchr/testify/v1.12.1/go.mod |
| S46 | https://proxy.golang.org/github.com/riverqueue/river/@latest |
| S47 | https://raw.githubusercontent.com/riverqueue/river/v0.51.2/LICENSE |
| S48 | https://raw.githubusercontent.com/riverqueue/river/v0.51.2/go.mod |
| S49 | https://proxy.golang.org/github.com/testcontainers/testcontainers-go/@latest |
| S50 | https://raw.githubusercontent.com/testcontainers/testcontainers-go/v0.45.0/LICENSE |
| S51 | https://raw.githubusercontent.com/testcontainers/testcontainers-go/v0.45.0/go.mod |
| S52 | https://proxy.golang.org/github.com/fergusstrange/embedded-postgres/@latest |
| S53 | https://raw.githubusercontent.com/fergusstrange/embedded-postgres/v1.34.0/LICENSE |
| S54 | https://raw.githubusercontent.com/fergusstrange/embedded-postgres/v1.34.0/go.mod |
| S55 | https://proxy.golang.org/github.com/fe3dback/go-arch-lint/@latest |
| S56 | https://raw.githubusercontent.com/fe3dback/go-arch-lint/v1.19.1/LICENSE |
| S57 | https://raw.githubusercontent.com/fe3dback/go-arch-lint/v1.19.1/go.mod |
| S58 | https://proxy.golang.org/github.com/golangci/golangci-lint/v2/@latest |
| S59 | https://raw.githubusercontent.com/golangci/golangci-lint/v2.15.0/LICENSE |
| S60 | https://raw.githubusercontent.com/golangci/golangci-lint/v2.15.0/go.mod |
| S61 | https://proxy.golang.org/github.com/ogen-go/ogen/@latest and https://raw.githubusercontent.com/ogen-go/ogen/v1.25.0/README.md |
| S61b | https://golangci-lint.run/docs/linters/ |
| S62 | https://raw.githubusercontent.com/ogen-go/ogen/v1.25.0/LICENSE |
| S63 | https://raw.githubusercontent.com/ogen-go/ogen/v1.25.0/go.mod |
| S64 | https://proxy.golang.org/github.com/swaggo/swag/@latest and https://raw.githubusercontent.com/swaggo/swag/v1.16.6/README.md |
| S65 | https://proxy.golang.org/github.com/swaggo/swag/v2/@latest and https://api.github.com/repos/swaggo/swag |
| S66 | https://raw.githubusercontent.com/swaggo/swag/v1.16.6/license and https://api.github.com/repos/swaggo/swag/license |
| S67 | https://raw.githubusercontent.com/swaggo/swag/v1.16.6/go.mod |
| S68 | https://proxy.golang.org/github.com/danielgtaylor/huma/v2/@latest, https://raw.githubusercontent.com/danielgtaylor/huma/v2.40.0/README.md and https://raw.githubusercontent.com/danielgtaylor/huma/v2.40.0/adapters/humagin/humagin.go |
| S69 | https://raw.githubusercontent.com/danielgtaylor/huma/v2.40.0/LICENSE.md |
| S70 | https://raw.githubusercontent.com/danielgtaylor/huma/v2.40.0/go.mod |
| S71 | https://www.postgresql.org/docs/18/runtime-config-resource.html |
| S72 | https://www.postgresql.org/docs/18/runtime-config-connection.html |
| S73 | https://www.postgresql.org/docs/18/auth-peer.html |
| S73b | https://spec.openapis.org/oas/ |
| S74 | https://www.postgresql.org/docs/18/app-initdb.html |
| S75 | https://www.postgresql.org/docs/18/sql-notify.html |
| S76 | https://www.postgresql.org/docs/18/sql-insert.html |
| S77 | https://www.postgresql.org/docs/18/transaction-iso.html |
| S78 | https://proxy.golang.org/github.com/riverqueue/river/@v/list |
| S79 | https://pkg.go.dev/github.com/google/uuid@v1.6.0 |
| S80 | https://golang.testcontainers.org/system_requirements/ |
| S81 | https://raw.githubusercontent.com/fergusstrange/embedded-postgres/v1.34.0/README.md |
| S82 | https://pkg.go.dev/golang.org/x/crypto/argon2 |
| S83 | https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html |

Other fetches used for cross-reference: `https://proxy.golang.org/github.com/jackc/pgx/v5/@v/list`, `https://proxy.golang.org/github.com/pressly/goose/v3/@v/list`, `https://proxy.golang.org/github.com/sqlc-dev/sqlc/@v/list`, `https://www.postgresql.org/docs/release/18.7/`, `https://api.github.com/repos/jackc/pgx/releases?per_page=3`.
