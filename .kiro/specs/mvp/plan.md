# MORN1NGST4R MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The readiness gate below is closed by decision record 0003. Starting M1 still needs the owner to say go.

**Goal:** Deliver one complete, authorized, end-to-end assessment on a Linux localhost web application, in small milestones that each end in an independently tested deliverable.

**Architecture:** Go modular monolith (`services/core`) with separate `server` and `worker` processes, PostgreSQL 18, a Next.js App Router UI (`apps/web`), local filesystem evidence, and tools run only in a verified sandbox behind a verified egress path. Domain code is standard-library only. Dependency directions are `structure.md` 5.6.

**Tech Stack:** Pins in `.kiro/steering/tech.md` (Go floor, module path, Node 24 and npm are Decided in D-65 and D-66, other pins stay Proposed until a milestone adds the module): Go 1.27.4 with a `go 1.26.0` floor, Gin v1.12.0, pgx v5.13.0, sqlc v1.31.1, goose v3.30.1, oapi-codegen v2.8.2, Next.js 16.4.6, React 19.3.1, TypeScript 5.9.3.

**Spec:** `.kiro/steering/{product,structure,security,architecture,tech}.md`; owner decisions in `docs/decisions/0001-owner-security-decisions-round-2.md`, `0002-foundation-completion-decisions.md` and `0003-foundation-closure-decisions.md`; product requirements in `docs/PRD.md`; semantics in `docs/decisions/pending/S1-security-semantics.md`; threats in `docs/security/threat-model.md`; spike evidence in `docs/security/feasibility/`.

## Readiness gate (closed by decision record 0003)

The four gates that blocked M1 are closed by `docs/decisions/0003-foundation-closure-decisions.md`. Nothing in this plan has been implemented. Closing a gate means a decision exists, not that a control works.

| Gate | Closed by | State |
| --- | --- | --- |
| 1. Approve `security.md` as edited in Round 5 and S1 | D-64 | Closed |
| 2. Confirm interpretive choices 1 to 14 of record 0002 | D-64 | Closed. Choices 1, 6 and 10 are tested in M1. |
| 3. Confirm tech pins, Node and package manager | D-65, D-66 | Closed. Node and npm affect only M13. Other module pins stay Proposed until each milestone adds the module. |
| 4. Go module path and Go floor | D-65 | Closed. Module path `github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core`, `go 1.26.0`, no `toolchain` line. |

M1 and M2 need only these gates. Everything that touches the network, a sandbox, authentication or an intrusive operation has the extra gates listed per milestone. The decisions that stay open (O-07, O-13, O-22, O-32, O-33, O-35, O-36, O-38) are gates of later milestones and are listed in the milestone map.

## Source-of-truth hierarchy (D-63)

1. Approved owner and security decisions. 2. Product requirements (`product.md`, `docs/PRD.md`). 3. Architecture and technical design. 4. This plan. 5. Supporting notes and research. A lower document is corrected when it disagrees with a higher one.

## Global Constraints

- Domain packages import only the Go standard library and permitted domain packages (`structure.md` 5.6). A library enters a package group only when `tech.md` lists it.
- Fail closed. Unknown, ambiguous or unparseable input is denied. Deny always wins (D-57).
- Wildcard inclusion rules are refused through the MVP (D-35, D-73). Wildcard and CIDR exclusions are accepted (D-57). A refused exclusion is never dropped silently (choice 10 of record 0002).
- Defaults: an in-scope rule without a port covers 80 and 443. An exclusion without a port covers all ports (D-36, D-57). Minimum CIDR prefix /24 IPv4 and /48 IPv6 for in-scope rules and lab ranges, not for exclusions (D-38, D-47, choice 1).
- One approval use per (approval, job), consumed atomically at claim. Three execution attempts for approval-bound active jobs. An intrusive operation has exactly one launch (D-30, D-49, D-55).
- Revocation, scope and lab changes terminate affected work within 30 seconds, measured before it is relied on (D-48).
- Intrusive operations stay disabled until authentication and re-authentication are implemented and tested (D-58). Active operations without established numeric limits stay blocked (O-38, D-71). Password entry is required on every intrusive approval by default (D-69).
- No network-touching tool adapter before the egress path passes its regression gate. No tool execution without the sandbox self-test passing (D-31, D-41, D-42).
- Secrets: Secret Service only, fail closed. Never in argv, environment, standard input, logs or ordinary files (D-52, D-85). The bootstrap secret lives only on a memory-backed file (D-68).
- No commit, push or merge unless the owner asks for it in that request. The commit steps below are written for when the owner has asked.
- Every decision here is a requirement or a direction. Nothing in this plan is implemented, and no security control counts as verified until a test demonstrates it (`security.md` 18).
- Tests that need `internal/` access live inside the Go module (D-27). Security regression tests are required for every security-critical task (`security.md` 16).

## Review Focus

These inputs are implied by the spec and are exercised by named tests in the tasks that own them.

1. Legacy and numeric host spellings (`127.1`, `2130706433`, `0x7f.0.0.1`, `0177.0.0.1`, fullwidth digits) must be ambiguous and denied. Owner: Task 1.2.
2. A name with trailing dots, upper case, a leading wildcard, an underscore or a non-ASCII byte must not reach a comparison in non-canonical form. Owner: Task 1.2.
3. An exclusion must beat a matching inclusion for every rule kind, port and IP form, and adding an exclusion must never turn a denial into an allow. Owner: Task 1.4 and 1.5.
4. Port edge cases (`0`, `65536`, `08080`, duplicates, no port) and a redirect to another port. Owner: Task 1.3 and 1.4.
5. Hostile approval and job races: two workers claiming one job, a retry claim, a revocation racing a claim. Owner: M2 and M3.

## File Structure (first milestones)

| Path | Responsibility |
| --- | --- |
| `services/core/go.mod` | Module `github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core`, `go 1.26.0` floor, no `toolchain` line, no dependencies at first |
| `services/core/internal/domain/scope/host.go` | `Host` canonical ASCII name value type and `ParseHost` |
| `services/core/internal/domain/scope/addr.go` | `Addr`, `Prefix` parsing with `net/netip`, canonical forms, embedded-IPv4 rejection |
| `services/core/internal/domain/scope/port.go` | `PortSet` parsing and default sets |
| `services/core/internal/domain/scope/rule.go` | `Rule`, `RuleKind` (domain, wildcard, ip, cidr), registration validation |
| `services/core/internal/domain/scope/evaluate.go` | `RuleSet.Evaluate(Target) Decision` (names and literals only, no resolution) |
| `services/core/internal/domain/scope/*_test.go` | Table-driven and fuzz tests |
| `services/core/tests/architecture/deps_test.go` | Dependency-direction test over `go list -json` |
| `Makefile` | `make test` target only at first |

---

## Milestone map

Milestone IDs are stable. M0 and M16 to M25 were added in Round 6 to close PM-01 to PM-06. The IDs are not in build order. The dependency column is binding and the recommended order follows the table.

| ID | Milestone | Depends on | Requirements covered | Open gates |
| --- | --- | --- | --- | --- |
| M0 | Foundation and repository | none | NFR-MAINT-001 | none (complete when the foundation commit exists) |
| M1 | Scope canonicalization core (pure domain) | M0 | FR-SCOPE-001, FR-SCOPE-002 | none |
| M1b | Host conversion (IDNA) in the application layer | M1 | FR-SCOPE-007 | O-33 corpus |
| M2 | Policy and approval domain model | M1 | FR-POLICY-001, FR-APPROVAL-001 to 004, FR-APPROVAL-006 (domain parts), FR-SCOPE-004 to 006 (domain parts) | none |
| M3 | Persistence: PostgreSQL provisioning, schema, migrations, atomic consume | M2 | schema for FR-PROG, FR-ASSESS, FR-JOB, FR-APPROVAL | G-consume. Idle memory measured |
| M4 | Authentication and local session | M3 | FR-AUTH-001, FR-AUTH-002 (server side), FR-APPROVAL-005 (re-authentication backend) | G-auth. Argon2id benchmark |
| M5 | Audit event model and sink | M3 | FR-AUDIT-001, FR-AI-004 (audit part) | none |
| M6 | Evidence metadata and integrity | M3, M5 | FR-EVIDENCE-001, FR-EVIDENCE-002 | none |
| M7 | Worker and job lifecycle | M2, M3, M5 | FR-JOB-001, FR-APPROVAL-002 (claim), FR-APPROVAL-003, FR-SCOPE-006 (running work) | G-revoke (O-35) |
| M8 | Sandbox launcher and self-test | M7 | FR-TOOL-002 | G-sandbox on a named baseline (O-13, O-22) |
| M9 | Egress enforcement (first-party client, policy proxy) | M1, M8, M17 | FR-TOOL-003, FR-SCOPE-003 | G-egress (O-07). Address tables reviewed (D-72) |
| M10 | Tool adapter framework (manifest, hygiene, exact-bytes launch) | M2, M8, M9 | FR-TOOL-001, FR-APPROVAL-004 (manifest classes) | G-sandbox, G-egress |
| M11 | First adapters: Subfinder (passive), httpx (probing), Nuclei (Intrusive, optional) | M6, M10, M18, M19, M20 | FR-RECON-001, FR-PROBE-001, FR-APPROVAL-006, FR-TOOL-004 | O-38 numbers (D-71). Per-tool flag validation. D-58 for Nuclei |
| M12 | AI provider abstraction | M3, M5, M25 | FR-AI-001, FR-AI-002, FR-AI-003 | none |
| M13 | Frontend foundation: shell, login, bootstrap, security headers, generated client | M4 | FR-AUTH-002 (browser side), FR-EVIDENCE-003 | O-02, O-03 verification (D-79). Node and npm (D-66) |
| M14 | Reporting (Markdown) | M6, M22 | FR-REPORT-001 to 003 | none |
| M15 | End-to-end authorized lab assessment | M11, M14, M22b, M23, M24 | all MVP requirements, SM-01 to SM-09 | D-58 for any intrusive step |
| M16 | Program, target and scope management use cases and the policy store | M1, M2, M4, M5 | FR-PROG-001, FR-PROG-002, FR-POLICY-002, FR-SCOPE-001 (registration) | none |
| M17 | Lab registration and lifecycle | M2, M5, M16 | FR-SCOPE-004 | none |
| M18 | Target authorization validation | M4, M5, M16, M17 | FR-SCOPE-005, FR-SCOPE-006 (invalidation use case) | none |
| M19 | Approval application lifecycle (request, approve with re-authentication, revoke, consume) | M2, M3, M4, M5, M18 | FR-APPROVAL-001 to 003, FR-APPROVAL-005 | G-auth, G-consume |
| M20 | Asset and endpoint inventory | M1, M3, M16 | FR-ASSET-001, FR-ASSET-002 | none |
| M21 | Assessment management and test planning | M16, M18, M19, M20 | FR-ASSESS-001, FR-FINDING-004 | none |
| M22 | Finding lifecycle, validation records and human verification | M4, M5, M6, M20 | FR-FINDING-001, FR-FINDING-002 (records), FR-FINDING-003 | none |
| M22b | Validation replay through the first-party client | M9, M19, M22 | FR-FINDING-002 (replay) | G-egress |
| M23 | AI-assisted analysis and orchestration | M6, M12, M21, M22 | FR-AI-005, FR-FINDING-004 (AI proposals) | none |
| M24 | UI integration of the workflow screens | M13, M14, M16 to M23 | UX-001 to UX-011, FR-APPROVAL-005 (screen), FR-SCOPE-005 (screen) | none |
| M25 | Credential store (Secret Service adapter) | M5 | FR-CRED-001 | Use outside a desktop session verified (D-85) |

Recommended build order: M0, M1, M2, M3, M5, M4, M16, M17, M18, M19, M25, M6, M7, M20, M21, M12, M13, M8, M9, M10, M11, M22, M22b, M14, M23, M24, M15. M1b runs at any point after M1 and must finish before any non-ASCII name is accepted. The validator `scripts/validate-foundation.py` checks that this table is acyclic and that no milestone depends on a later-built one.

**What can be built first without any spike:** M1, M2, M3 (apart from measurements), M5, M16 to M21, M22 and M25 (its port). **Hard blockers for tool execution:** M8 on a named baseline, M9 passing its regression gate, the O-38 numbers, per-tool flag validation, and for intrusive classes M4 and M19 with password re-entry implemented and tested (D-58).

Behavior that no milestone may break: automated analysis cannot mark a finding Verified (M22). A report never presents an unverified candidate as confirmed (M14). No tool runs before the sandbox and approval gates pass (M10, M11). AI cannot bypass scope or approval (M12, M23). An evidence change is detected (M6). M1 contains no tool execution.

## Test gates

| Gate | Passes when | Blocks |
| --- | --- | --- |
| G-arch | The dependency-direction test passes in CI | every merge |
| G-scope | The M1 table, property and fuzz tests pass, including every S1.1 case | M2 onward |
| G-consume | A concurrency test with two workers shows exactly one consumption per (approval, job), and a revoked or expired approval is refused at claim and at launch | M7, any job creation |
| G-auth | Login, session, CSRF, Host and Origin and rate-limit tests pass, and intrusive approval is refused without fresh password entry | any intrusive operation |
| G-sandbox | The self-test passes on the baseline with production arguments, refuses on each simulated missing layer, and regression tests derived from the spike S-suite pass. The spike suite itself tests the Python spike launcher, not the product. | M10 and any tool launch |
| G-egress | Regression tests derived from the spike E-suite pass against the product's client and proxy, with per-job sockets and refusal (not neutralization) of Host overrides | M10 and any network-touching tool |
| G-revoke | Measured: queued and running work stops within 30 seconds after a revocation or scope or lab change | any approval-bound job |
| G-redact | A secret search over argv, environment, files, logs, evidence and reports finds none | every release |
| G-foundation | `python3 scripts/validate-foundation.py` exits 0: register IDs unique, references resolve, milestone dependencies acyclic, every requirement mapped to a milestone, ignore rules hold, safety scan clean | every foundation commit |

## Acceptance criteria for the MVP

The MVP is accepted when, on the baseline host, a researcher can register a program and a local lab target, validate authorization with an attestation, run passive reconnaissance with an enabled source, probe an in-scope lab host, collect and verify evidence, review a candidate finding, verify it as a human, and export a Markdown report, while every denial and approval is audited, and the gates above pass. No external target is involved.

---

## Milestone M1: Scope canonicalization core

Deliverable: a stdlib-only `scope` domain package that parses rules and targets into canonical form and evaluates them by name or literal address, with wildcard inclusion refused. No resolution, no I/O.

### Task 1.1: Module scaffold and architecture test

**Files:**
- Create: `services/core/go.mod`, `services/core/tests/architecture/deps_test.go`, `Makefile`

**Interfaces:**
- Produces: a module that builds with `go build ./...` and a test `TestDomainImportsOnlyStdlibAndAllowedDomains` that fails if any `internal/domain/...` package imports a non-standard-library path outside the allowed domain order of `structure.md` 5.6.

- [ ] **Step 1: Write the failing test** `TestDomainImportsOnlyStdlibAndAllowedDomains` in `services/core/tests/architecture/deps_test.go`. It runs `go list -json ./internal/domain/...` and asserts that every import is either standard library or inside `internal/domain`. With no domain package yet it must pass vacuously, so add `TestDomainPackagesExist` expecting `internal/domain/scope` to exist.
- [ ] **Step 2: Run** `cd services/core && go test ./tests/architecture/...`. Expected: FAIL on `TestDomainPackagesExist`.
- [ ] **Step 3: Create** `go.mod` with the module path `github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core` (D-65), `go 1.26.0` and no `toolchain` line, an empty `internal/domain/scope/doc.go` (`package scope`), and a `Makefile` that exports `GOTOOLCHAIN=local` with a target `test` running `cd services/core && go vet ./... && go test ./...`.
- [ ] **Step 4: Run** `make test`. Expected: PASS.
- [ ] **Step 5: Commit** only if the owner asked for commits.

### Task 1.2: Canonical host names

**Files:**
- Create: `services/core/internal/domain/scope/host.go`, `host_test.go`

**Interfaces:**
- Produces: `type Host string` (canonical lowercase ASCII, no trailing dot), `func ParseHost(s string) (Host, error)`, `var ErrAmbiguous = errors.New("ambiguous target")`.

- [ ] **Step 1: Write the failing table test** `TestParseHost` with these exact cases: `"Example.COM."` returns `"example.com"`; `"example.com.."`, `"."`, `""`, `"user@host"`, `"a_b.example.com"`, `"ex ample.com"`, a 64-octet label, a 254-octet name, `"exämple.com"` (non-ASCII), and the numeric forms `"127.1"`, `"2130706433"`, `"0x7f.0.0.1"`, `"0177.0.0.1"`, `"127.000.000.001"`, `"1.2.3.256"`, `"1.2.3.4.5"`, `"foo.0x7f"` all return `ErrAmbiguous`. A 253-octet name of valid labels returns itself.
- [ ] **Step 2: Run** `go test ./internal/domain/scope -run TestParseHost`. Expected: FAIL.
- [ ] **Step 3: Implement** `ParseHost(s string) (Host, error)`: reject empty, whitespace, control characters and any of `/ \ @ ? # % : [ ] * ,`; strip exactly one trailing dot; reject non-ASCII (conversion happens in the application layer, M1b); lowercase; labels only letters, digits, hyphen, at most 63 octets, name at most 253 octets; apply the "last label all digits, or `0x` plus hex" rule and accept a four-part canonical decimal only when it is an IP literal (which belongs to `Addr`, not `Host`), so `ParseHost` rejects every numeric form.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Add** `FuzzParseHost` asserting idempotence (`ParseHost(string(h))` equals `h`), that output never contains a forbidden character, and that it never panics. Run `go test -fuzz=FuzzParseHost -fuzztime=30s ./internal/domain/scope`. Expected: no failures.

### Task 1.3: Addresses, prefixes and ports

**Files:**
- Create: `services/core/internal/domain/scope/addr.go`, `port.go`, and tests

**Interfaces:**
- Produces: `func ParseAddr(s string) (netip.Addr, error)` (canonical, zone rejected, IPv4-mapped IPv6 unmapped to IPv4, NAT64, 6to4, Teredo and `::/96` embedded forms rejected as `ErrAmbiguous`); `func ParsePrefix(s string, min PrefixMin) (netip.Prefix, error)` (host bits set, `/0`, shorthand `10/8` and below the minimum rejected); `type PortSet`, `func ParsePorts(s string) (PortSet, error)`; `DefaultInScopePorts` (80, 443) and `AllPorts`.

- [ ] **Step 1: Write failing tests**: `::ffff:127.0.0.1` and `::ffff:7f00:1` parse to `127.0.0.1`; `::1%eth0`, `64:ff9b::cb00:7107`, `2002::1`, `::127.0.0.1` return `ErrAmbiguous`; `203.0.113.5/24` (host bits set), `0.0.0.0/0`, `198.51.0.0/16` (shorter than the /24 minimum) and `2001:db8::/32` (shorter than the /48 minimum) are rejected with the default minimum, `10/8` shorthand is rejected with every minimum, and with a zero minimum (the exclusion case, choice 1) `0.0.0.0/0`, `198.51.0.0/16` and `2001:db8::/32` are accepted while host bits and shorthand are still rejected; `203.0.113.0/24`, `203.0.113.0/25` and `2001:db8:1::/48` are accepted; ports `0`, `65536`, `08080`, `80,80` are rejected and `8443` parses.
- [ ] **Step 2: Run** the tests. Expected: FAIL.
- [ ] **Step 3: Implement** with `net/netip` only. `PrefixMin` holds the IPv4 and IPv6 minimum lengths and defaults to 24 and 48. It is a value passed in, never a package variable, so only an explicit policy change can alter it. Exclusions pass a zero `PrefixMin`.
- [ ] **Step 4: Run** the tests. Expected: PASS.

### Task 1.4: Rules and evaluation

**Files:**
- Create: `services/core/internal/domain/scope/rule.go`, `evaluate.go`, tests

**Interfaces:**
- Consumes: `Host`, `netip.Addr`, `netip.Prefix`, `PortSet`.
- Produces: `type RuleKind int` with `KindDomain`, `KindWildcard`, `KindIP`, `KindCIDR`; `type Rule struct{ Kind RuleKind; Exclude bool; Host Host; Addr netip.Addr; Prefix netip.Prefix; Ports PortSet }`; `func NewRule(kind RuleKind, exclude bool, value string, ports string, min PrefixMin) (Rule, error)`; `type Target struct{ Host Host; Addr netip.Addr; Port uint16 }`; `type Decision struct{ Allowed bool; Reason string }`; `type UnsupportedExclusionError struct{ Input string }`; `func (rs RuleSet) Evaluate(t Target) Decision`.

- [ ] **Step 1: Write failing tests**: `NewRule(KindWildcard, false, "*.example.com", ...)` returns an error (inclusion refused, D-35). `NewRule(KindWildcard, true, "*.internal.example.com", ...)` succeeds (exclusion accepted). A CIDR exclusion `/16` succeeds and a `/16` inclusion fails. `Evaluate` cases: no rule matches then denied; an in-scope `example.com` allows `example.com:443` and denies `:8443`; an exclusion `example.com` with no ports denies `example.com:8443` even when an inclusion lists `8443`; an exclusion `*.x.example.com` does not cover `x.example.com`; a wildcard exclusion covers `a.b.x.example.com`; an in-scope `ip` rule allows only that address; an exclusion CIDR beats an inclusion IP inside it; a URL path, regex or scheme in a rule value returns an error. An exclusion whose syntax the grammar refuses returns an `*UnsupportedExclusionError` carrying the original text, and `errors.As` distinguishes it from the error for an unsupported inclusion, so the application layer can record it and block readiness (choice 10). A wildcard exclusion with a non-ASCII byte is refused as ambiguous in M1.
- [ ] **Step 2: Run** the tests. Expected: FAIL.
- [ ] **Step 3: Implement** `NewRule` and `Evaluate`. Decision order: exclusions first, then inclusions, default deny. Wildcard exclusion matches one or more labels below its base and never the apex. A zero-value `Target` is denied.
- [ ] **Step 4: Run** the tests. Expected: PASS.

### Task 1.5: Property tests and S1.1 coverage check

**Files:**
- Create: `services/core/internal/domain/scope/property_test.go`

- [ ] **Step 1: Write** `TestAddingExclusionNeverAllows`: over random rule sets and targets, adding an exclusion never changes a denial into an allow. `TestNormalizationIdempotent` for hosts and prefixes.
- [ ] **Step 2: Run** `go test ./internal/domain/scope`. Expected: PASS. If a property fails, fix the code, not the test.
- [ ] **Step 3: Run** `go test -race ./...` and `go vet ./...`. Expected: no findings.
- [ ] **Step 4: Check coverage** against the S1.1 test list by hand and note any case not covered in the task output.
- [ ] **Step 5: Commit** only if the owner asked for commits.

**M1 acceptance:** G-arch and G-scope pass, every S1.1 case for names and literals has a test, wildcard inclusion is refused, exclusions are accepted, and the package imports only the standard library.

---

## Milestone definitions (M1b to M25)

Each of these is expanded into TDD tasks like M1 when its dependencies pass their gates. The definition fixes the deliverable, the interfaces others rely on, the acceptance tests and the stop conditions. Nothing below is implemented.

### M0: Foundation and repository
Deliverable: `LICENSE` (Apache-2.0), `.gitignore`, `README.md`, steering and decision documents, the threat model, the PRD, this plan, and `scripts/validate-foundation.py`. Acceptance: the validator exits 0, no application source exists, and the foundation commit contains no secret, evidence or local artifact. Evidence is the validator output in the foundation record.

### M1b: Host conversion (application layer)
Depends on: M1. Deliverable: `application/scope.CanonicalizeHost(input string) (scope.Host, error)` using `golang.org/x/net/idna` (P-57) with a strict, explicitly built, non-transitional Lookup profile, hand-written WHATWG numeric-host rules, and a single-script-per-label rule built from the standard `unicode` script tables (D-73). A mixed-script label is refused by default with an audited override. Acceptance: every S1.1 IDNA input (`fuß.de` becomes `xn--fu-hia.de`, ideographic full stop, soft hyphen, uppercase `XN--`, invalid `xn--`, length limits), the IdnaTestV2 corpus, a homograph corpus, and a cross-check that the domain `ParseHost` accepts the output. Stop if the library result differs from the S1.1 expectation. Gate O-33.

### M2: Policy and approval domain
Depends on: M1. Deliverable: pure `operation` (canonical form and hash), `actor` (human, system, ai, P-02), `approval` (record with expiry, uses, duration, concurrency, rate, attempt maximum, hash and destinations), the consumption state machine (claim, launch check, denial, retry claim), and the pure decision function returning `AuthorizedOperation` (P-03, P-04). Acceptance: expiry stops new launches only, revocation, an intrusive operation has one launch, a pre-launch denial is not an attempt, a consumed use is not restored, a retry claim reuses the record, a zero-value `AuthorizedOperation` is rejected, an AI actor cannot approve. No database.

### M3: Persistence
Depends on: M2. Deliverable: PostgreSQL 18 per-user cluster scripts (D-67), goose migrations (forward only), sqlc queries, login roles for migration, api, worker and retention, an append-only audit table with INSERT and SELECT only for the runtime roles, the policy store tables (D-64 choice 4), and the atomic conditional consume statement with a unique `(approval, job)` constraint. Acceptance: G-consume with real concurrent transactions, grant tests, a socket mode test, and a measurement of idle memory on the 8 GB laptop. Not covered: separation of two processes of one OS user (D-78).

### M4: Authentication and session
Depends on: M3. Deliverable: one local account, Argon2id (benchmarked), server-side sessions with hashed tokens, HttpOnly cookie, CSRF token, Host and Origin checks, login rate limiting, the bootstrap secret on a memory-backed file, the one-time recovery code, password re-entry for intrusive approval and widening policy changes (default every time). No API tokens. Acceptance: G-auth, a test that bootstrap is refused without a memory-backed location, a recovery reuse test, and browser-level tests for DNS rebinding and cookie sharing across localhost ports (unverified today, T-55, T-57).

### M5: Audit model
Depends on: M3. Deliverable: `audit` domain, `AuditSink` port and PostgreSQL adapter, bounded retries with a persistent alert, the events of `security.md` 13. Acceptance: an operation does not start when the record cannot be written, and denials and cancellations still take effect.

### M6: Evidence metadata and integrity
Depends on: M3, M5. Deliverable: the write-once publish protocol (temp file, `O_EXCL`, hash, fsync, `link`, directory fsync), SHA-256 metadata, re-hash on read, owner-only permissions, redaction before write (D-77). Acceptance: a swap or overwrite is detected, an existing name is never replaced, raw capture is off by default.

### M7: Worker and job lifecycle
Depends on: M2, M3, M5. Deliverable: claim, lease, fencing, heartbeat, cancellation, the passive retry policy of D-80, and the 30-second stop objective measured (G-revoke). Acceptance: stale worker writes rejected, redelivery, a crash between claim and launch has a defined result, an intrusive job is failed on lease expiry, a retry waits for confirmed termination of the previous process tree.

### M8: Sandbox launcher
Depends on: M7. Deliverable: a Go launcher that reproduces the spike's argument set: empty environment for the helper, read-only root and `/dev`, closed descriptors, seccomp filter (x86_64), the required limits of D-70 including a size-bounded memory-backed working directory, a sealed in-memory copy of the verified tool bytes passed with `--ro-bind-data`, the per-launch preflight, and a start-up self-test that refuses and names the missing layer. Acceptance: G-sandbox on the named baseline, a test per simulated missing layer, regression tests derived from the spike S-suite. The spike suite tests the Python spike launcher, not the product.

### M9: Egress enforcement
Depends on: M1, M8, M17. Deliverable: first-party HTTP client (resolve once, dial the validated IP, per-hop redirect evaluation) and a policy proxy with one Unix socket per job, a relay inside the namespace, refusal of Host overrides, and a tunnel revocation hook. The path carries the registered initial target so that only that connection can use a lab exemption. Address deny tables generated from the IANA registries with a pinned SHA-256 and recorded version (D-39) plus the named metadata entries (D-72). Acceptance: G-egress.

### M10: Tool adapter framework
Depends on: M2, M8, M9. Deliverable: manifest schema (risk class, destinations, flags, secret delivery), per-parameter validation, option-injection guards, exact-bytes launch, JSONL parsing with strict schemas. Acceptance: S1.4 cases and a hostile-output corpus. No real tool yet.

### M11: First adapters
Depends on: M6, M10, M18, M19, M20. Deliverable: Subfinder (passive, `-duc`, no `-active`, keyless sources only until protected-file delivery is verified) after passive source enablement, then httpx (Low-impact) after the O-38 numbers exist. A lab test per tool records whether it sends a host name or an IP to the proxy. Nuclei (Intrusive, optional) only after G-auth and D-58. Acceptance: discovered assets are observations until scope evaluation passes, and each tool runs only through M10.

### M12: AI provider abstraction
Depends on: M3, M5, M25. Deliverable: provider port, a fake provider for tests, a loopback adapter, a generic OpenAI-compatible remote adapter, locality classification by connected address, a start-up check that cloud forwarding of a loopback model is off (otherwise remote), audited transmissions, typed proposals only. Acceptance: credentials never in prompts, a tunnel and a forwarding proxy are classified remote, re-resolution cannot change the classification, no provider is enabled by default.

### M13: Frontend foundation
Depends on: M4. Deliverable: `apps/web` with the generated client (P-43), nonce CSP in `proxy.ts`, framing denial, same-origin rewrites, login and bootstrap screens, no inline scripts, no `dangerouslySetInnerHTML`. Acceptance: Vitest, header tests, a frame-denial test, and a test of the Host and Origin that Go sees through the rewrite (D-79). Node 24 and npm (D-66).

### M14: Reporting
Depends on: M6, M22. Deliverable: Markdown report from `text/template`, untrusted values only in code spans or fences, report status separate from validation status, a confirmed section that contains only Verified findings, export audited and redacted by default. Acceptance: an unverified candidate never appears in the confirmed section, an escape fuzz test, and an export audit test.

### M15: End-to-end local lab assessment
Depends on: M11, M14, M22b, M23, M24. Deliverable: the acceptance scenario above against a local lab registered under D-47, with all gates green. Acceptance: that scenario passes end to end on the baseline host, every denial and approval in it is audited, and no external target is involved. Intrusive steps only when D-58 is satisfied.

### M16: Program, target and scope management
Depends on: M1, M2, M4, M5. Deliverable: application use cases to register a program with a rules reference and version, register targets, add and remove scope rules, record refused exclusions, and the single policy store port with audited changes and re-authentication for widening changes. Acceptance: a refused exclusion blocks readiness until acknowledged or replaced (choice 10), every policy change records old value, new value, actor, time and reason, an AI or worker actor is refused, a widening change without fresh password entry is refused.

### M17: Lab registration and lifecycle
Depends on: M2, M5, M16. Deliverable: lab registration bound to a program and an assessment with exact target kind, permitted range, ports for loopback and local-interface addresses, expiry and revocation that invalidate dependent jobs, never a replacement for scope validation. Acceptance: a public name that resolves into a lab range does not inherit it, the CIDR minimum applies, forbidden address classes are refused, every change is audited.

### M18: Target authorization validation
Depends on: M4, M5, M16, M17. Deliverable: the validation record (mechanical scope result, program-rule reference, human attestation), bound to the canonical target-set hash and the rules version, lab-specific wording, 24-hour expiry that stops new launches only. Acceptance: any scope, rules or lab change makes it stale, an AI attestation is refused, the interface does not call it proof of ownership.

### M19: Approval application lifecycle
Depends on: M2, M3, M4, M5, M18. Deliverable: request, approve (rendered only from the canonical operation, with password re-entry for intrusive operations), revoke and consume use cases. Acceptance: G-consume and G-auth, an approval cannot authorize an unrelated operation, an AI actor cannot approve.

### M20: Asset and endpoint inventory
Depends on: M1, M3, M16. Deliverable: domains, addresses, services, HTTP services, technologies, endpoints and parameters with origin and an append-only observation history (D-76). Acceptance: a repeated run adds an observation and overwrites nothing, a discovered asset is flagged as an observation until scope evaluation passes, no scheduler exists.

### M21: Assessment management and test planning
Depends on: M16, M18, M19, M20. Deliverable: assessment lifecycle (create, start, pause, complete), plan items with origin owned by the `assessment` domain (P-38), immutability after approval. Acceptance: a plan item cannot become a job without a policy decision and, where required, an approval.

### M22: Finding lifecycle, validation and human verification
Depends on: M4, M5, M6, M20. Deliverable: the seven states and the transition table of D-75, deduplication key, validation records linked to evidence, report status separate from validation status, the Verified transition for a human actor only. Acceptance: an AI, worker or tool actor cannot set Verified, a refused attempt is audited, Verified is refused when evidence fails its integrity check or no validation record exists, a duplicate observation never changes a state.

### M22b: Validation replay
Depends on: M9, M19, M22. Deliverable: replay of recorded requests through the first-party client behind the same scope, policy and approval checks. Acceptance: a replay without a current validation record or approval is denied, a failed reproduction is distinct from an inconclusive one.

### M23: AI-assisted analysis and orchestration
Depends on: M6, M12, M21, M22. Deliverable: audited AI worker jobs that produce typed proposals (asset analysis, test recommendations, evidence interpretation) with origin, no handle to executors. Acceptance: an adversarial AI contract test, AI output is labeled as a recommendation, a proposal never executes without policy and approval.

### M24: UI integration of the workflow screens
Depends on: M13, M14, M16 to M23. Deliverable: screens for programs, targets and scope, labs, validation, approvals, assessments, inventory, findings, evidence, AI disclosure and reports, built on the M13 foundation. Acceptance: the approval screen renders only from the canonical operation, strings are escaped and length capped, a framed page is refused, the UX requirements of the PRD are met.

### M25: Credential store
Depends on: M5. Deliverable: the `CredentialStore` port and a Secret Service adapter with a bus-socket preflight (D-85), typed errors, fail-closed behavior. Acceptance: tests on a desktop session and on a session without a bus, no `dbus-launch`, no plaintext fallback, a secret-canary search over argv, environment, files and logs finds none (G-redact).

## Self-review

- **Decision coverage:** D-47 to D-61 map to milestones as before. D-62 to D-85: D-63 to D-65 M0, M1. D-66 M13. D-67 M3. D-68 and D-69 M4 and M19. D-70 M8. D-71 M7, M10, M11. D-72 M9. D-73 M1, M1b. D-74 M0. D-75 M14 and M22. D-76 M11, M20, M22b. D-77 M6. D-78 accepted risks. D-79 M13. D-80 M3, M7. D-81 M12. D-82 M23. D-83 this plan. D-84 M0. D-85 M25.
- **Requirement coverage:** every FR of the PRD appears in the milestone map above. The validator checks this.
- **Proportion:** only M1 is expanded to test level. The rest are definitions on purpose, because their interfaces depend on earlier milestones.
- **Open items carried:** O-07, O-13, O-22, O-32, O-33, O-35, O-36 and O-38 each appear as a gate or note above. None blocks M1.
