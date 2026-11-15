# 0003: Foundation closure decisions (Round 6)

Status: recorded on 2026-11-16 at the project owner's instruction "Final foundation closure, repository hardening and push readiness". This record continues `0001-owner-security-decisions-round-2.md` and `0002-foundation-completion-decisions.md`.

## 1. Authority, method and limits

The owner asked for every outstanding foundation decision that can responsibly be resolved now to be resolved, with conservative defaults where evidence is incomplete, and for the M1 prerequisites to be closed. This record applies that instruction. Three rules governed how.

1. **Conservative direction.** Where the evidence did not settle a choice, the stricter and more fail-closed option was chosen. No security requirement was weakened. Where a number could not be established, a gate was written instead of a number (D-71).
2. **Decided is not implemented.** A decision in this record states a requirement or a direction. No application code exists. No control in `docs/security/threat-model.md` is "mitigated and tested". A decision that depends on a measurement or a test carries the exact gate that must pass first.
3. **Approval by instruction.** The owner's instruction is the approval for the items below that earlier records marked "awaiting owner review", including `security.md` as edited in Round 5 and S1. Nothing was approved by an AI agent for itself. The owner can reopen any item. A reopened item keeps its ID and gets a superseding entry.

Evidence for the technology facts is in `.kiro/steering/tech.md` and `docs/development/tech-verification/`. Evidence for the sandbox and egress facts is in `docs/security/feasibility/`. Nothing in this record adds new feasibility evidence.

## 2. Source-of-truth hierarchy (D-63)

When two documents disagree, the higher level wins. A lower-level document is corrected, and the history stays in the decision records.

| Level | Contents | Where |
| --- | --- | --- |
| 1 | Approved owner and security decisions | `docs/decisions/0001`, `0002`, `0003`, the Decided table in `.kiro/steering/architecture.md` section 19, and `.kiro/steering/security.md` |
| 2 | Product requirements | `.kiro/steering/product.md`, `docs/PRD.md` |
| 3 | Architecture and technical design | `.kiro/steering/architecture.md`, `structure.md`, `tech.md` |
| 4 | Implementation plan | `.kiro/specs/mvp/plan.md` |
| 5 | Supporting notes and research | `docs/security/threat-model.md`, `docs/security/feasibility/`, `docs/development/tech-verification/`, `docs/decisions/pending/` |

Two clarifications. A feasibility result or a threat model status never overrides a decision. A decision's status can be lowered only by a later decision. The pending folder holds proposals and records of applied proposals. A proposal in it is not in force.

## 3. Decision register entries

Source `R6` means the owner's Round 6 instruction. "Closes" lists the open items, proposals, conflicts and plan mismatches that the entry resolves. The full text and the gates follow in sections 4 to 6.

| ID | Decision | Closes | Source |
| --- | --- | --- | --- |
| D-63 | Source-of-truth hierarchy of section 2 | none directly. It is the rule used to resolve CF-01 to CF-06 | R6 |
| D-64 | `security.md` as edited in Round 5 and S1.1 to S1.6 are approved as the security specification. Interpretive choices 1 to 14 of record 0002 are decided as written there, with the clarifications in section 4. | Readiness gates 1 and 2 | R6 |
| D-65 | Go module path `github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core`. Module floor `go 1.26.0`, no `toolchain` line, builds use `GOTOOLCHAIN=local`. | Readiness gate 4, P-41 (modified) | R6 4.1 |
| D-66 | Frontend runtime Node.js 24.x with npm. Node 26 is not the baseline until a Next.js verification passes. | P-50 (modified), Node and package manager choice | R6 4.1 |
| D-67 | PostgreSQL 18 from user-installed binaries, a per-user cluster over a unix socket, no user-supplied DSN, no database password in the environment or in a file | O-01, O-39, O-43, P-42 (modified), P-18 | R6 4.2 |
| D-68 | Authentication details: first-run bootstrap secret on a memory-backed runtime file, hashed session tokens, Argon2id starting parameters, password recovery by a one-time recovery code through the web UI | O-42, O-37, P-48 | R6 4.3 |
| D-69 | Re-authentication: password entry on every intrusive approval and every widening policy change by default. An optional window of at most five minutes only by explicit policy change. Target authorization validation expires after 24 hours and the expiry stops new launches only. | P-39 (modified), P-40 | R6 3.1 |
| D-70 | Sandbox: required limits, a bounded job working directory, per-launch preflight, capability-defined baseline, static tool binaries only | O-41, P-63, P-64, P-51, CD-10, CD-13 | R6 4.5 |
| D-71 | Numeric request-rate, concurrency and duration limits are a pre-execution gate, not numbers. Heartbeat and kill timing are a measured gate. | O-38 (gated), O-35 (gated), O-06 | R6 3.1 |
| D-72 | The metadata never-authorize list adds three named addresses | O-34, P-58, CD-08 | R6 3.2 |
| D-73 | Host canonicalization: ASCII only in M1, strict IDNA and a single-script rule in M1b, wildcard inclusion refused through the MVP | O-24, O-33 (gated), O-32 (deferred), P-56, P-57, CD-07 | R6 3.1 |
| D-74 | Licensing: Apache-2.0 `LICENSE` added. Bubblewrap is used only as an external program. MPL public suffix data and GPL tools stay out of the repository. | O-40 | R6 4.6 |
| D-75 | Finding transition table, deduplication, report status and report format | O-11, O-27, P-10, P-55, C items 6 and 7, CF-01 | R6 5.1 |
| D-76 | Asset inventory is persistent and user-driven. Reproduction replay uses the first-party client. The controlled-test catalog stays the four classes of D-59. | O-30, O-31, O-29, C item 8, CF-02 | R6 5.1 |
| D-77 | Evidence storage defaults: redacted evidence of record, raw capture off by default, owner-only files, no encryption at rest in the MVP, retention by explicit audited purge | O-08, P-37 | R6 3.2 |
| D-78 | Accepted residual risks: same-user trust boundary, no audit hash chain in the MVP, no HTTPS path visibility at the proxy, no I/O limit | T-61, T-51, T-37, RR-01, RR-04, RR-07, RR-12 | R6 3.2 |
| D-79 | Localhost web controls: framing denial is required, Go validates Host, Origin and CSRF itself, and the serving path of P-49 is adopted with a verification gate | O-02, O-03, P-49, P-60, P-35 (framing), CD-06 | R6 3.2 |
| D-80 | Engineering conventions adopted: spec-first OpenAPI, custom PostgreSQL queue with a bounded passive retry policy, `log/slog`, test tooling, UUID v7, forward-only migrations, Makefile start-up, data location | O-04, O-12, O-14, O-18, O-19, O-20, O-21, O-23, P-43 to P-47, P-61, P-62 | R6 4 |
| D-81 | AI provider scope and egress: one loopback adapter and one OpenAI-compatible remote adapter, no provider enabled by default, a loopback model is remote unless forwarding is verified off, prompts are not stored | O-26, O-10, P-53, P-34, P-11, CD-09 | R6 3.1 |
| D-82 | Skills: `allowed-tools` is a request and never authority, bundled scripts are never run, the loader reads one configured directory | O-09, P-54 | R6 3.2 |
| D-83 | Plan and PRD reconciliation: milestones M0 and M16 to M24 added, design proposals P-01 to P-05, P-36 and P-38 adopted | PM-01 to PM-06, CF-03, CF-04, O-25 | R6 5 |
| D-84 | Repository hygiene policy: tracked and ignored categories, secrets scan, spike logs stay tracked | none | R6 6, 7 |
| D-85 | Credential store: `go-keyring` behind the `CredentialStore` port with a preflight, a gate for use outside a desktop session, headless Linux unsupported for credentials | O-17, P-59 | R6 4.4 |

## 4. Interpretive choices 1 to 14 of record 0002 (D-64)

Each row is the decision, why, what it means for M1, and what risk remains.

| Choice | Decision | Rationale | M1 impact | Residual risk |
| --- | --- | --- | --- | --- |
| 1 | The CIDR minimum (/24 IPv4, /48 IPv6) applies to in-scope rules and lab ranges. It does not apply to exclusions. An exclusion must still be a canonical prefix (no host bits, no shorthand). A `/0` exclusion is accepted and denies everything. | An exclusion only denies, so a broad one cannot widen scope (D-38, D-57). | `ParsePrefix` takes the minimum as a value. Exclusions pass a zero minimum. Tests in plan Tasks 1.3 and 1.4. | A mistaken broad exclusion makes an assessment unusable. That fails closed. |
| 2 | "Authenticated human owner or approver" means the single local researcher account of D-60. | The MVP has one account and no API tokens. | None. M2 defines the actor kinds. | One account approves everything (RR-02). |
| 3 | A policy change that "widens what can be tested or sent" is any change to scope, lab registrations, the CIDR minimum, approval defaults and attempt maximums, risk classification overrides, passive source enablement, remote provider enablement, retention of raw evidence, and the framing or CSP policy if it is ever made configurable. The list can only grow. | Matches `security.md` 3 and adds the two new items from D-77 and D-79. | None. M16 implements the list. | A future widening not on the list is missed. A review rule is in the plan: a new policy field must state whether it widens. |
| 4 | There is one canonical policy store. It is a set of versioned PostgreSQL tables written only by one persistence adapter behind one `PolicyStore` port owned by the `scope` application area. The runtime worker role has SELECT only. Each change stores old value, new value, actor, time and reason. | One writer is auditable. Versioned rows give the old and new value for free. | None. M3 creates the tables and M16 the port. | Same-user processes can still write the database (D-78). |
| 5 | The 30-second stop objective applies to revocation, scope edits and lab changes. It is an objective until measured. | The owner kept the 30 seconds for scope edits. | None. Gate G-revoke in M7. | A slow heartbeat misses the objective. Measurement is required before it is relied on. |
| 6 | Wildcard exclusions are accepted while wildcard inclusion is refused. | A wildcard exclusion denies more, never less (D-35, D-57). | `NewRule` accepts a wildcard exclusion and refuses a wildcard inclusion. Plan Task 1.4. | A wildcard exclusion over a public suffix denies a whole zone. That fails closed. |
| 7 | A lab registration still requires a matching in-scope rule and no exclusion. | Registration relaxes only the address denial (`security.md` 2, D-47). | None. M17 enforces it. | None beyond choice 11. |
| 8 | Numeric request-rate, concurrency and duration limits are not set. Low-impact and Intrusive operations and passive request rates stay blocked until the gate of D-71 passes. | No tool has been measured. | None. | Delay of the first active operation. |
| 9 | The DR-17 wording "credentials not sent without explicit user configuration" is superseded by D-50. Target credentials are never sent to an AI provider. | D-50 is stricter. | None. | None. |
| 10 | A refused exclusion is never dropped silently. The assessment is not ready until a human acknowledges it, bound to the canonical target-set hash, or replaces it with a supported exclusion that denies at least as much. | "Deny always wins" and "never silently widen scope". | `NewRule` returns a typed `ErrUnsupportedExclusion` that carries the original text. A new test asserts it is distinguishable from a generic error. M16 stores it and blocks readiness. Plan Task 1.4. | A human acknowledgement can be wrong. It is audited. |
| 11 | A lab registration whose address is assigned to any local interface lists ports. The application registers every listener and database endpoint it starts or depends on, including the web server and PostgreSQL. A tool operation does not start while that set is incomplete. | The researcher's own host is the most likely accidental target (T-11, T-12). | None. M9 and M17. | A listener started later and not registered. Connection-time recheck covers it. |
| 12 | An active operation needs a current target authorization validation. A retry launch waits until the previous attempt's process tree is confirmed terminated. No setting allows a second launch of an intrusive operation under one approval. | `security.md` 4 and D-49. | None. M2, M7, M19. | A confirmation of termination that is wrong. Tested with the kill and timing gates. |
| 13 | The default has no re-authentication window. Password entry is required for every intrusive approval and every widening policy change (D-69). Session tokens are stored only as hashes. A secret delivered through a file lives on a memory-backed filesystem inside the sandbox. | Stricter than the proposed five-minute window. | None. M4. | Approval fatigue grows with prompts (RR-02). |
| 14 | Metadata addresses are named individually, not by removing whole ranges (D-72). | Keeps IPv4 and IPv6 symmetric (D-54). | None. M9 tables. | A provider address not on the list. Research item for the table review in M9. |

M1 therefore depends on choices 1, 6 and 10. All three are decided and have tests in the plan.

## 5. Decision details and gates

### D-65 Go module path and toolchain

- Evidence: `git remote -v` shows `https://github.com/Adibayuluthfiansyah/MORN1NGST4R.git`, and the repository owner's Git identity is the same person. The owner identity is therefore unambiguous. The canonical module path follows the remote exactly, including case, so that a future `go get` resolves. Go module paths are case sensitive. The monorepo module sits in `services/core`, so the path ends in `/services/core`.
- If the repository is renamed or moved, the module path changes in one place and every import is rewritten mechanically. Nothing imports this module from outside the repository.
- Host evidence: `go version` reports go1.27.3. `tech.md` records go1.27.4 as the proposed release toolchain and `go 1.26.0` as the floor.
- Decision: `go.mod` carries `go 1.26.0` and no `toolchain` line. The Makefile exports `GOTOOLCHAIN=local` so that a build never downloads a toolchain silently. The owner installs go1.27.4 when convenient. It is not required for M1 because the host toolchain exceeds the floor.

### D-66 Frontend runtime

- Node.js 24.x, at least 24.15.0, is the baseline. npm is the package manager. The lockfile `package-lock.json` is committed once `apps/web` exists, and installs use `npm ci`.
- The development host runs Node 26.13.0. Node 26 is accepted for local experiments only. It becomes a supported runtime only after the M13 check shows Next.js, Vitest and jsdom behave on it. `engines` will state `>=24.15.0 <25` until then.
- This affects M13 only. Nothing in M1 to M12 depends on it.

### D-67 PostgreSQL provisioning and database access

- Supported version: PostgreSQL 18 (the host has client tools 18.6). Binaries are installed by the user with the system package manager. The project does not bundle, download or embed a server.
- Per-user cluster: created by `initdb` under `$XDG_DATA_HOME/morningstar/pg` with owner-only permissions. It listens on a unix socket only (`listen_addresses = ''`) in a socket directory with mode 0700 owned by the user. `unix_socket_permissions` is 0700.
- Authentication: peer authentication through `pg_hba.conf`. A `pg_ident.conf` map lets the OS user connect as separate login roles (migration, api, worker, retention) so that grants limit mistakes and SQL defects. Peer authentication cannot separate two processes of the same OS user. That residual risk is accepted (D-78).
- No password exists, so no password appears in a DSN, in the environment, in argv or in a file. A user-supplied DSN is not supported in the MVP. This closes O-43.
- Procedure (implemented in M3 as Makefile targets, P-61): create the cluster, start it with `pg_ctl`, create roles and the database, run goose migrations as the migration role, stop with `pg_ctl stop`. Start-up and shutdown are explicit commands. No process manager.
- Backup and recovery: `pg_dump` and a copy of the evidence directory, both written with owner-only permissions. Backups hold target data and audit rows, so they inherit the access restrictions of the source (`security.md` 12). Backups are never committed (`.gitignore`, D-84). A restore procedure is an M3 deliverable.
- Idle memory and connection limits are not measured. M3 must measure them on the 8 GB reference laptop before the defaults are set.

### D-68 Authentication bootstrap and recovery

- Account: one local researcher account. Password hash: Argon2id, starting values m=65536 KiB, t=3, p=4, 16-byte salt, 32-byte key, PHC string. The values are benchmarked on the target laptop in M4 and the final values are recorded in `tech.md`. Concurrent hashes are bounded. Sessions are server side in PostgreSQL, tokens are stored only as hashes, and the cookie is HttpOnly. No API tokens exist in the MVP.
- Bootstrap (O-42): when no account exists the server creates a 256-bit random bootstrap secret, stores only its hash, and writes the secret to `$XDG_RUNTIME_DIR/morningstar/bootstrap-secret` with mode 0600 in a directory with mode 0700. `$XDG_RUNTIME_DIR` is memory backed. The server checks that the filesystem is `tmpfs` or `ramfs` and refuses account creation if it is not or if the variable is unset. The secret is never printed to a terminal, never logged and never put in a URL. The user reads the file and enters the value on the first-run page. The page is served only on the loopback listener with the Host, Origin and CSRF rules. Until an account exists, every other route returns a fixed "setup required" response. The secret is deleted when the account is created or when the server stops. A new one is created at the next start while no account exists. Attempts are rate limited and audited.
- Recovery (O-37): option B of `docs/decisions/pending/F-password-recovery-proposal.md`. At account creation the application shows a recovery code of at least 128 bits once and stores only its hash made with the password function. A reset needs the code and a new password, in the web UI on the loopback listener. The code is single use and a new one is shown after a reset. A reset revokes every session and is audited. Failed attempts are rate limited, audited and give no information about whether a code exists. A reset keeps intrusive operations disabled until the password has been entered once in the new session. A lost code means option A: export evidence through a documented offline procedure and re-initialize. That offline procedure is destructive and must be written and reviewed before the first release (gate G10 in the threat model). No command-line reset exists (D-18).
- Login and password re-entry rate limit: more than 5 failed attempts within a rolling 15 minutes are refused until the window clears. Each failure is audited. These are policy defaults that M4 tests, not measured values.
- Not implemented and not tested. G-auth is the gate.

### D-69 Re-authentication and validation expiry

- Default: password entry on every intrusive approval and on every policy change that widens what can be tested or sent. This is the "no window configured" case of `security.md` 4 and is stricter than the five-minute window of P-39.
- An optional window of at most five minutes can be enabled only by an explicit policy change that itself requires password entry. It is never enabled by default and never by AI output.
- Target authorization validation expires 24 hours after it was made, in addition to change-based staleness (D-61). Expiry stops new launches only, as with approval expiry (D-48).

### D-70 Sandbox required limits, preflight, baseline and distribution

- Required limits (O-41, P-63): wall-clock timeout, memory, process count, CPU time, file size, output size, and a bounded job working directory. The working directory is a memory-backed tmpfs with an explicit size that M8 must prove with a test on the baseline. If the baseline cannot provide it, no tool launches. Memory-backed pages are also charged to the memory cgroup, which bounds them in a second way.
- A per-job I/O limit is not required. Its absence is an accepted residual risk (availability only, D-78).
- Per-launch preflight (P-64): before every launch check the pinned digest of the sandbox helper and re-check the required layers. This is in addition to the start-up self-test with production arguments.
- Baseline (O-22 stays open until a baseline passes): defined by capability as in P-51. The reference host is the only tested platform. Any other host is unsupported until `scripts/spikes/` suites and the product self-test pass on it.
- Tool distribution: static binaries only. A dynamically linked tool is unsupported until its library tree is pinned. Bubblewrap comes from the distribution package. Its path, version and SHA-256 are recorded at start-up and pinned by the preflight.
- The spike facts that remain gaps are listed in section 7.

### D-71 Numeric limits gate

No request-rate, concurrency or duration number is invented. A Low-impact or Intrusive operation, and the request rate of a passive source, stays blocked until all of the following hold: the tool has been measured against the local lab in M10 or M11 and the numbers are recorded in its manifest, the approval record binds them (D-55), and the owner approves the default for that class through an explicit policy change. The 30-second stop objective stays an objective. Gate G-revoke requires a measurement under load. This is the owner's own fail-closed instruction (choice 8).

### D-72 Metadata deny list

The never-authorize list is `169.254.169.254` and `fd00:ec2::254` (D-39) plus `168.63.129.16` (Azure WireServer, a public address no registry list contains), `fd20:ce::254` (Google Cloud IPv6) and `100.100.100.200` (Alibaba Cloud). Each is a named single-address entry. A lab range that contains one is refused. The IANA registry tables are generated by a procedure with a pinned SHA-256 and a recorded version (D-39). Other provider addresses are a research item that must be closed in the M9 table review. `security.md` 2 is updated to name the list.

### D-73 Host canonicalization and wildcard policy

- M1: ASCII only. Any non-ASCII byte is ambiguous and denied.
- M1b: `golang.org/x/net/idna` with an explicitly built strict, non-transitional Lookup profile, hand-written WHATWG numeric-host rules, and a single-script-per-label rule built from the standard `unicode` script tables. A label that mixes scripts is refused by default with the audited override of D-37. The single-script rule is stricter than UTS #39 restriction levels and does not claim to implement them. Gate for M1b: the IdnaTestV2 conformance corpus, the S1.1 IDNA cases and a homograph corpus, with an independent oracle (O-33).
- Wildcard inclusion stays refused through the whole MVP. Explicit domain rules cover the MVP workflow. O-32 (public suffix source) stays open and is a post-MVP item. The vendored-list direction of P-56 is recorded as the intended route, subject to the MPL-2.0 acceptance in D-74.

### D-74 Licensing policy

- The product is Apache-2.0 (`product.md` 1). The repository now contains the canonical Apache License 2.0 text as `LICENSE`. The file is a byte-exact copy of the text published by the Apache Software Foundation in its `apache/.github` repository (11357 bytes, SHA-256 `c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4`). It matches the SPDX text of `Apache-2.0` after whitespace normalization (1581 words). `apache.org` itself was unreachable from the host when this was done, so the direct comparison with that URL was not made. The appendix placeholder `[yyyy] [name of copyright owner]` is part of the standard text and is unchanged. No copyright notice is added to source files yet.
- Bubblewrap (LGPL) is run only as an external program that the user installs. It is not linked, copied or redistributed by the project.
- MPL-2.0 public suffix data is not added to the repository. The question reopens with O-32.
- GPL-licensed development tools (golangci-lint) are not adopted and nothing GPL is added to the repository.
- Every dependency is recorded in `tech.md` with version, licence and reason before it is added. Third-party agent skills are local-only development tooling. They are not part of the product and not redistributed, and their license copies are in the git-ignored `.kiro/skills-licenses/`.
- This is an engineering policy and a record of evidence. It is not legal advice. Tool licences (Nmap Public Source License and others) need a separate review before any adapter.

### D-75 Findings, deduplication and reporting

- States are the seven of `product.md` 7: Candidate, Needs Review, Inconclusive, False Positive, Not Reproducible, Awaiting Human Verification, Verified. These are the lifecycle states (CF-01). Terms such as "validated" or "confirmed" in other documents are not extra states. They describe a validation record or the Verified state.
- Transition table. An automated actor (AI, worker, tool) may create a Candidate, move a Candidate to Needs Review, and attach a recommendation. Nothing else. Every other transition is made by the authenticated human and is audited with the reason:
  - Needs Review to Inconclusive, False Positive, Not Reproducible, or Awaiting Human Verification.
  - Awaiting Human Verification to Verified, or back to Needs Review.
  - Inconclusive, False Positive, Not Reproducible and Verified back to Needs Review (reopening), with a reason.
  - Verified is allowed only when a validation record exists, its linked evidence passes the integrity check, and the actor is the authenticated human. Verified may require password entry (decided at M22 as a policy field, default off).
- Deduplication: a finding key is a hash of the assessment, the canonical asset or endpoint, the vulnerability class and the affected parameter. A new observation with an existing key adds evidence to the existing finding. It never changes its state and never reopens it.
- Report status is separate from validation status. Report status values: Not reported, In draft, In final. A report has its own state: Draft, then Final, set by the human.
- A report states findings by validation status. Only Verified findings appear as confirmed. Unverified candidates appear only in a separately labeled section when the human includes them. They are never presented as confirmed.
- Report format: Markdown from `text/template`, with untrusted values only in code spans or fences and no live links (P-55). Other formats are post-MVP.

### D-76 Assets, replay and the test catalog

- The inventory is persistent and user-driven (O-30). `product.md` calls it "continuous" in the sense of an inventory kept across runs with an append-only observation history. No scheduler, no scheduled discovery and no autonomous loop exist in the MVP. FR-ASSET-003 stays Won't.
- A discovered asset is an observation until scope evaluation passes (architecture E4). A discovery never becomes a target without evaluation.
- Validation and reproduction replay requests through the first-party HTTP client behind the same scope, policy and approval checks as any other operation (O-31). It is not a tool adapter.
- The controlled-test catalog is the four classes of D-59 and the tools of P-52. Nuclei is optional, Intrusive and stays disabled until D-58.

### D-77 Evidence storage defaults

- Evidence of record is the redacted capture. Credentials, cookies and authorization values are redacted before the bytes are written. A raw capture is not persisted by default. Persisting raw capture is a per-assessment human policy setting that widens exposure, so it needs password entry (choice 3).
- Files are write-once with the P-37 protocol, owner-only (directories 0700, files 0600), addressed by ID and never by a path built from input, hashed with SHA-256 and re-hashed on read.
- Encryption at rest is not part of the MVP. It relies on the host's disk encryption, which the documentation states. Retention is until a human purge. Purges run under the retention role and are audited. A configurable retention period is post-MVP.
- Residual risk: redaction can miss an unknown secret format (T-49).

### D-78 Accepted residual risks

The owner accepts these for the MVP. Each stays in the threat model with its status and the condition to revisit.

| Risk | Accepted because | Revisit when |
| --- | --- | --- |
| Same-user trust boundary (T-61, RR-01) | One user, one machine. Separate OS users are post-MVP. Grants still limit mistakes and defects. | Before any multi-user or remote mode, or gate G10 |
| No audit hash chain (T-51, RR-07) | Deferred past the MVP (C item 3). Runtime roles cannot update or delete audit rows. | Before the first release gate G10 review |
| The proxy cannot see HTTPS paths, inner Host or same-host redirects (T-37, RR-04) | Enforcement is on host and port. The first-party client does request-level checks. | When a tool needs path-level control |
| Hostname exclusions do not cover IP aliases (T-05) | Already accepted by D-38 | Never, unless the grammar changes |
| No per-job I/O limit (RR-12, partly) | Availability only | If a tool shows I/O starvation |

### D-79 Localhost web controls

- Framing denial is required on every response: `Content-Security-Policy: frame-ancestors 'none'` and `X-Frame-Options: DENY`. `security.md` 9 is updated.
- Go validates Host, Origin and the CSRF token itself with hand-written middleware and `golang.org/x/time/rate` (P-60). `http.CrossOriginProtection` alone is not used, because it accepts a request with no Origin.
- Serving path (O-02, O-03): `next build` then `next start -H 127.0.0.1 -p <port>` with same-origin `rewrites` to the Go API. The Host allow-list in Go contains the exact host and port of the web server and of the API. M13 acceptance includes a test that observes the Host and Origin that Go sees through the rewrite. If the rewrite does not preserve a usable Host, the fallback is a direct cross-origin call to the API with a single explicitly configured CORS origin, the same Origin and CSRF checks, and no wildcard. Both paths fail closed.

### D-80 Engineering conventions

- OpenAPI is spec first: author 3.0.4, generate the Gin server with oapi-codegen v2.8.2, generate the TypeScript client with openapi-typescript and openapi-fetch (P-43). Generated code is committed and reviewed.
- Queue: a custom PostgreSQL queue with `FOR UPDATE SKIP LOCKED`, leases and fencing (P-44, P-36). `LISTEN/NOTIFY` is only a wake-up hint. Passive jobs: at most three attempts in total, no automatic retry after cancellation, revocation or a policy denial, and a retry never reuses an approval use (D-55). Request rates of passive sources fall under D-71.
- Logging: `log/slog` with a redacting handler (P-45). No telemetry. A health endpoint and metrics are post-MVP.
- Tests: `go test`, an architecture test over `go list -json`, a temporary local PostgreSQL cluster for integration tests, Vitest and Testing Library for the frontend, Playwright later with Chromium only (P-46).
- Identifiers are UUID v7 generated in the application (P-47). Migrations are forward only. A migration is never edited after release. A correction is a new migration.
- Start-up is a Makefile with targets for the PostgreSQL cluster, the server and the worker. No launcher binary (P-61, D-18).
- Runtime data lives under `$XDG_DATA_HOME/morningstar`, owner-only. A root `data/` directory is for development fixtures only and is ignored (P-62).
- Read models for reporting are read ports owned by the `reporting` application area (O-19).
- Adopted design proposals for the packages built first: P-01, P-02, P-03, P-04, P-05, P-07, P-09, P-12, P-13, P-14, P-16, P-17, P-18, P-20, P-21, P-27 rules, P-32, P-36, P-38. P-04 is not a compile-time guarantee in Go, so the caller test of the architecture document stays required.

### D-81 AI provider scope and egress

- One adapter for a loopback server and one generic OpenAI-compatible adapter for remote services. Each key is bound to one configured endpoint (P-53).
- No provider is enabled by default. A remote provider needs per-program human configuration and shows what will be sent (D-46, D-50).
- A loopback address does not prove local-only operation, because a loopback Ollama can forward to a cloud service. The adapter checks at start-up that cloud forwarding is off. If it cannot verify that, the provider is classified remote. This follows from CD-09 and is stricter than D-50.
- Prompts and responses are not stored by default. The audit keeps provider, program, size and digest (`security.md` 5). Proposals are stored as typed objects with their origin. Prompt retention is therefore a policy that does not exist yet and cannot widen silently.
- A trusted-LAN category does not exist (O-36 stays deferred).
- AI jobs run as audited worker jobs with no `ToolExecutor` (P-16, P-32).

### D-82 Skills

`allowed-tools` in a skill file is a request and never authority. Bundled scripts are never executed. The loader reads one configured directory and verifies a recorded SHA-256 for each skill. A hash mismatch fails closed. The manifest format is `SKILL.md` plus a project-owned manifest (P-54). The `skills/` directory is not created before M23.

### D-83 Plan and PRD reconciliation

- PM-01 to PM-06 are closed by new milestones (M16 to M23) in the plan. M0 records the foundation itself. M24 integrates the workflow screens. The milestone map and dependency order are in `.kiro/specs/mvp/plan.md`.
- CF-01: the seven states stay (D-75). CF-02: D-76. CF-03: `product.md` gets an interpretation section that adds approvals, authentication, audit and evidence integrity to the MVP list. CF-04: P-02 to P-05 are adopted (D-80). CF-05: S1 is approved (D-64). CF-06: the O-24 grammar is closed.
- The PRD requirement statuses were corrected to match. Requirements are "Decided" only where a decision exists. Nothing is "Implemented".
- Assessment lifecycle states (FR-ASSESS-001): Draft, Ready, Running, Paused, Completed, Cancelled. Ready needs a current target authorization validation record and no unacknowledged refused exclusion. Paused stops new launches and holds queued jobs, and running jobs stay under their approvals. Cancelled stops queued and running work. A human changes every state, and each change is audited.
- No numeric performance or usability target is set for the MVP (NFR-PERF-001, NFR-USE-001). Performance and memory are measured and recorded, not promised.

### D-84 Repository hygiene

- `.gitignore` covers secrets and local configuration, runtime data and assessment evidence, captured traffic and raw scanner output, logs and crash dumps, generated build and test output, agent skill payloads and caches, and developer environment files. Bare names that could match a source directory are anchored to the repository root.
- `.env.example` and `.env.*.example` stay trackable. The repository contains no `.env.example` yet because no application configuration exists.
- The spike logs in `scripts/spikes/*/last-run/` are curated evidence cited by the feasibility records. They stay tracked through a negation rule.
- `scripts/validate-foundation.py` checks the ignore rules, the register, the references, the milestone dependencies and a secrets and local-path scan. It is run before each foundation commit.
- Evidence, target data, database dumps and captures are never committed. If a sensitive file is found tracked, it is removed from the index only, the exposure is assessed, and rotation is recommended. Removing it from the last commit does not erase history.

### D-85 Credential store

- `github.com/zalando/go-keyring` v0.2.8 behind the `CredentialStore` port, with a bus-socket preflight so that `dbus-launch` never runs, and any error other than "not found" fails closed (P-59).
- Whether a worker started outside a desktop session can use the Secret Service is not verified. Gate: a test on a desktop session and on a session without a bus, before any credential feature is accepted. Until then credentials are unavailable and the features that need them (authenticated tool runs, remote AI keys) are disabled with a message that lists the available alternatives.
- Headless Linux is unsupported for credentials in the MVP. The headless mechanisms of `security.md` 7 apply only to a future headless mode (D-20).
- No plaintext fallback, no secret in argv, the environment, standard input, logs or ordinary files.

## 6. The sixteen owner questions of the threat model

| Question | Answer | ID |
| --- | --- | --- |
| 1 Same-user separation | Accept the shared OS user as a documented risk. Use peer authentication with separate login roles for grants. | D-67, D-78 |
| 2 Metadata list | Add the three named addresses. Lab rules mean the wider list. | D-72 |
| 3 Mixed-script labels | Keep refusing, by a stricter single-script rule instead of UTS #39 levels. | D-73 |
| 4 Audit tamper evidence | Not required for the MVP. Accepted. | D-78 |
| 5 Resource limits | Six limits plus a bounded job working directory. No I/O limit. | D-70 |
| 6 Local model servers | Treated as remote unless cloud forwarding is verified off. | D-81 |
| 7 Framing denial | Add it to `security.md` 9. | D-79 |
| 8 Numeric limits and timing | A measured gate, no invented numbers. | D-71 |
| 9 Evidence capture | Redacted evidence of record, raw off by default, no encryption at rest, retention by purge. | D-77 |
| 10 HTTPS visibility | Accept the proxy limit. Use the first-party client for path-level control. | D-78 |
| 11 Tool distribution | Static binaries only. Pin bubblewrap by digest at start-up. | D-70 |
| 12 Baseline distributions | Defined by capability. Reference host only. No AppArmor profile in the MVP. | D-70 |
| 13 P-39 and P-40 | No window by default. 24-hour validation expiry. | D-69 |
| 14 Password recovery | One-time recovery code in the web UI. | D-68 |
| 15 Skills | `allowed-tools` ignored, scripts never run. | D-82 |
| 16 Documentation hygiene | CD-01 to CD-13 resolved or classified in the threat model. | D-83 |

## 7. What stays open, and the exact condition to revisit

None of these blocks M1. Each has a gate.

| Item | Why it stays open | Condition to close | Blocks |
| --- | --- | --- | --- |
| O-07 | The production egress path does not exist. The spike passed on one host. | G-egress passes against the product's client and proxy | M10, any network-touching tool |
| O-13, O-22 | Sandbox mechanism and baseline are proven only on the reference host. | G-sandbox passes on a named baseline | M10, any tool launch |
| O-32 | No public suffix source. Wildcard inclusion refused. | Post-MVP decision on source, licence and update strategy | Wildcard inclusion only |
| O-33 | IDNA behavior is unverified. | The M1b gate in D-73 | M1b |
| O-35 | Heartbeat and kill timing are not measured. | G-revoke measurement | Any approval-bound job |
| O-36 | A trusted-LAN category is deferred. | A separate owner decision | Nothing in the MVP |
| O-38 | No tool has been measured. | D-71 gate | Low-impact and Intrusive operations |

Spike facts that are not met as built and stay gaps (from `docs/security/feasibility/`):

- What the spikes verified, on one reference host only: the namespaces and capabilities, descriptor handling with a launcher that closes them, a seccomp filter on x86_64, cgroup memory, process and CPU limits through `systemd-run --user`, resolve-once pinning, a policy proxy through a Unix socket relay, and direct network access failing in a namespace without a route.
- What they did not verify: other distributions and kernels, an allow-list seccomp profile, a Go launcher, real security tools inside the sandbox, protected secret delivery by file or descriptor, routed namespaces, and behavior under load.
- Inherited descriptors: bubblewrap leaked 7 descriptors. The launcher must close them. A Go launcher is untested.
- Environment leakage: `--clearenv` left the parent environment readable in `/proc/1/environ`. The helper must start with an empty environment.
- Hash-then-execute by path failed the swap test. Only a sealed in-memory copy passed with `--ro-bind-data` ran the hashed bytes.
- Proxy socket: any process that can reach the path-based socket can use it. One socket per job is required and is untested.
- Active tunnels have no revocation hook. A revocation does not yet stop a running tunnel within 30 seconds.
- A Host header override is neutralized and not refused.
- Resource limits: no per-job disk quota on the host, no I/O limit, and `RLIMIT_NPROC` is unusable per job. D-70 requires a bounded working directory instead.

Tool execution stays disabled until all of these pass: G-sandbox, G-egress, G-revoke, the D-71 numbers, binary integrity with exact bytes, and per-tool argument validation. There is no unsandboxed exception (D-31).

## 8. M1 readiness

M1 needs only the readiness gates of the plan and the Go pin. All four gates are closed by this record: gate 1 (D-64), gate 2 (D-64), gate 3 (D-65, D-66, with Node affecting only M13), gate 4 (D-65). M1 is a pure standard-library domain package with no network, no sandbox, no authentication and no database. It contains no tool execution. The three interpretive choices it depends on (1, 6, 10) are decided and tested in the plan.

This record does not claim that M1 is built. It claims that no foundation decision or approval blocks starting it.

## 9. Documents changed by this round

`LICENSE`, `README.md`, `.gitignore`, `.kiro/steering/{architecture,security,tech,product}.md`, `.kiro/specs/mvp/plan.md`, `docs/PRD.md`, `docs/security/threat-model.md`, this record, `docs/decisions/README.md`, `docs/decisions/pending/README.md`, and `scripts/validate-foundation.py`. The pending proposals E1 (architecture split) were generated against architecture revision 7. They are stale against revision 8 and must be regenerated before use.
