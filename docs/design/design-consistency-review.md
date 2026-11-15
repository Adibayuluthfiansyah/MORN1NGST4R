# Design consistency review

Status: review of the database design and the API design, written after both were complete. It lists what was checked, what was found and what remains for the owner. It changes no approved decision.

## 1. Method

The tables, the endpoints, the schemas and the OpenAPI file were written from one set of sources and then checked by `scripts/validate-design.py` (structure, references, ERD consistency, OpenAPI structure and the endpoint match) and read against `docs/PRD.md`, `security.md`, `architecture.md`, `0002` and `0003` and the threat model. The validator proves structural consistency only. It does not prove that the design is secure or that it will be implemented as written.

Size of the design: 59 tables, 631 columns, 15 migrations, 118 active endpoints (3 deferred, 9 internal worker use cases), 140 schemas, 72 error codes, 27 API decisions, 29 database decisions, 18 design questions (see `dq-decisions.md`: 10 Proposed, 2 Decided, 5 Deferred, 1 Owner Decision Required).

## 2. The fifteen checks

| # | Check | Result | Evidence |
| --- | --- | --- | --- |
| 1 | Every API resource maps to a valid domain concept | Pass | Every endpoint lists tables that exist in the dictionary (validator). `database-mapping.md` has one row per endpoint. |
| 2 | Every state-changing endpoint maps to an explicit state transition | Pass | Section 4 of this file. Every `W` and `RA` endpoint is listed with its transition or insert and its history table. The validator requires an audit event on each. |
| 3 | Every security-sensitive operation has an authorization rule | Pass | Every specification block has Authentication, Authorization, CSRF and Re-authentication rows (validator). Human-only actions are also refused at the database (CHECKs). |
| 4 | Every approval workflow maps to the approval data model | Pass | APR-01 to APR-07 and WRK-01 map to `operations`, `approvals`, `approval_events`, `approval_uses`, `policy_decisions` (mapping rows). The consumption protocol is DB-13 and CC-01, CC-02. |
| 5 | Every job operation maps to job and attempt entities | Pass | JOB-01 to JOB-06 and WRK-01 to WRK-07 map to `jobs`, `job_attempts`, `job_transitions`. DB-12 and DB-14 separate database state and process control. |
| 6 | Every finding transition maps to finding history | Pass | FND-01, FND-04 to FND-07 each insert a `finding_transitions` row. The legal pairs are CHECKs and a parity trigger keeps `findings.status` equal to the newest transition (DB-11). |
| 7 | Every evidence operation preserves integrity requirements | Pass | EVD-03 and EVD-05 re-hash and record `evidence_integrity_checks`. Metadata is immutable and the projection is separate (DB-23). Verification requires a `match` check (FND-06). |
| 8 | Every scope-dependent operation references the applicable scope version | Pass | `jobs`, `approvals`, `policy_decisions`, `target_validations`, `lab_registrations`, `assets` and `audit_events` store `scope_version_id` (DB-10). |
| 9 | Audit requirements appear in both database and API design | Pass | `audit_events` plus the catalog in `domain-model.md`. Every state-changing endpoint names one audit action. The validator checks that each endpoint action is in the catalog. |
| 10 | No endpoint allows AI to bypass human authorization | Pass by construction, not tested | There is no actor field in any request schema and no token authentication (validator). AI-10 creates a job whose output is data. Only human sessions reach approve, verify, scope, policy and lab routes. |
| 11 | No endpoint allows automated verification to become human verification | Pass by construction, not tested | FND-06 is the only route to Verified. `finding_verifications.verified_by` references `users` and the transition CHECK requires actor kind `user` for `verified`. `validation_records` from automation are recommendations only. |
| 12 | No schema silently permits cross-program resource access | Pass | Composite foreign keys through `program_id` (DB-09) and flat item routes with a server-derived program (API-05). |
| 13 | No deletion behavior destroys required audit history | Pass | No cascades, no DELETE routes, no hard deletion of auditable rows (DB-05, DB-26). |
| 14 | No API example contains actual target data or secrets | Pass | Examples use reserved names (`app.example.test`), documentation addresses (`203.0.113.0/24`) and `EXAMPLE-ONLY-NOT-A-SECRET`. The validator scans examples for secret-like text, home paths and non-reserved domains. |
| 15 | No undocumented state transition exists | Pass for the documented machines | Section 3 of `domain-model.md` lists 14 state machines. Transition tables carry the legal pairs. Entities with a status but no transition table (sessions, labs, validations, reports) have their transitions in that list and in section 4 below. |

"Pass by construction, not tested" means the design makes the property hold if implemented as written. No code and no test exists.

## 3. Issues found and how they were resolved

| # | Issue | Resolution |
| --- | --- | --- |
| 1 | PRD and architecture talk about approval use, execution attempt and retry as if one counter. They are different (D-30, D-55). | Two separate records: `approval_uses` (one per approval and job) and `jobs.execution_attempts` with `job_attempts` rows (CC-01, CC-05, DB-13, DB-14). |
| 2 | "A retry launch waits until the previous process tree is confirmed terminated" (D-64 choice 12) had no data representation. | `termination_confirmed_at` on attempts and a partial unique index allowing one unconfirmed attempt per job (CC-04). |
| 3 | D-48 promises a 30-second stop but a database transaction cannot stop a process. | Separate `control_state` from `state` (DB-12). The API commits intent and the worker confirms. The 30 seconds remain an objective to be measured (O-35). |
| 4 | D-64 choice 4 left the policy store technology to this design. | Versioned `policy_settings` rows, one current row per key, widening changes consume a re-authentication grant (CHECK). |
| 5 | D-75 requires report status separate from validation status. A column on `findings` could drift from the report rows. | Derived view `finding_report_status` (DB-18). The API exposes `reportStatus` read-only. |
| 6 | The PRD lists "session revocation" and "re-authentication" but no listing or grant objects. | `sessions` list and revoke endpoints (AUTH-05, AUTH-06). `reauth_grants` with a two-step contract (API-24). |
| 7 | D-69 allows an optional re-authentication window, which conflicts with a single-use grant bound to one subject. | First answered with a `reusable` column. Superseded in the hardening round (issue 18): no reusable grant exists, the window is deferred (DQ-18). |
| 8 | A restore from backup could bring back unconsumed approval uses and old scope versions. | DB-27 restore-recovery rule and DQ-17. |
| 9 | The plan has M3 build schema for everything the claim and audit gates need, while assets, evidence and findings belong to later milestones. | Migration slices follow the milestones (00001 to 00007 for M3). Later slices are created at the start of their milestones (migration plan). |
| 10 | AI tasks produce output but are not tool operations. The approval model covers tool operations. | Job kind `ai_task` with risk class `ai` carries no approval and runs no tool. Transmission is gated by program enablement, disclosure and an audit record (DQ-09). |
| 11 | The PRD says evidence content must not execute when displayed, and downloads must be controlled. | EVD-05 and RPT-07 are attachments with `nosniff` and `no-store`, audited before the first byte (API-18). |
| 12 | Discovery must not become authorization (E4). Promoting an asset to a target needed an explicit act. | AST-06 re-evaluates under the current scope version and creates a target with origin `promoted_asset`. A target is not authorization either: validation and policy still apply. |
| 13 | The audit table must reference many entity types without a foreign key per type. | Typed foreign keys for the six common references and a soft `entity_type, entity_id` pair that cannot dangle because rows are never hard-deleted (DB-16). An orphan check verifies it. |
| 14 | Both `security.md` and the plan say the first release creates no API tokens. `Idempotency-Key` and CSRF tokens look similar. | They are not credentials. A request id key or a CSRF token authenticates nothing. The API has no bearer authentication (API-17). |
| 15 | `GET /session` is public but must give a CSRF token to login, setup and recovery. | Pre-session `ms_anon` cookie and token (API-04). A restart invalidates it. |
| 17 | A restore of an old backup also restores the evidence that a restore happened, so the first rule (DB-27, a database-side step) could not detect it. | External marker with a recovery generation and an audit watermark, startup reconciliation, crash-safe reconcile route AUTH-10, claim-time check CC-15 (DB-28, `database/recovery-generation.md`). |
| 18 | The re-authentication window (D-69) conflicted with single-use grants, and the first design added a `reusable` column. | The reusable grant is removed. The window is deferred (DQ-18). Every grant is single use (API-26). |
| 19 | `approvals.requested_by_kind` allowed a `system` requester, a path automation could use. | Column removed. Only a human requests an approval (DQ-12). |
| 20 | Labs (M17) and validation (M18) need an assessment record, but assessments were first built in M21. Approvals (M19) need plan items, which were in M21. | Plan corrected: assessment records and target sets move to M16, plan items and canonical operations to M19. M21 keeps the lifecycle transitions, status and job creation. |
| 21 | Persistent alerts (audit retry exhausted, restore recovered) could be raised but no route cleared them. | INT-02 clears an alert after its cause is resolved. |
| 22 | Two endpoints (policy value history, provider kinds) duplicated information available elsewhere. | Deferred with reasons: POL-03 (the audit trail) and AI-01 (the schema enum). Their IDs stay reserved. |
| 23 | The first design pass had no explicit statement that stored state never authorizes by itself. | DB-29: status columns are inputs to the policy engine at E1 to E3, never authorization. |
| 24 | DQ statuses were not recorded and some recommendations read as decisions. | `dq-decisions.md` gives each DQ exactly one status. Only DQ-02 and DQ-03 are Decided (D-47, D-75). |
| 16 | Repeated discovery runs must add observations, while a retried attempt must not duplicate its results (P-36). | Observation uniqueness is per attempt (CC-13), so reruns add rows and a retry of one attempt does not duplicate. |

## 4. State-changing endpoints and their transitions

Every `W` and `RA` endpoint (and the public POST routes) with the state change it makes. "insert" means a new row in an immutable or append-only table.

| Endpoint | Change | History or audit |
| --- | --- | --- |
| AUTH-02 | `users`, `user_credentials`, `recovery_codes`, `sessions`: insert. `bootstrap_secrets`: live -> consumed | audit auth.account_created |
| AUTH-03 | `sessions`: insert. `login_attempts`: insert | audit auth.login (own transaction on failure) |
| AUTH-04, AUTH-06 | `sessions`: active -> revoked | audit auth.logout, auth.session_revoked |
| AUTH-07 | `reauth_grants`: insert | audit auth.reauthenticated |
| AUTH-08 | `recovery_codes`: live -> used, new insert. `user_credentials`: current -> superseded, insert. `sessions`: all -> revoked | audit auth.recovery_reset |
| AUTH-09 | `user_credentials`: current -> superseded, insert. Other `sessions` -> revoked. `reauth_grants`: consumed | audit auth.password_changed |
| POL-02 | `policy_settings`: current -> superseded, insert version | audit policy.changed |
| PRG-01, PRG-04, PRG-05 | `programs`: insert, update with version, active -> archived | audit program.created, program.updated, program.archived |
| PRG-06 | `program_rules_versions`: insert. Current `target_validations` -> stale when the version differs | audit program.rules_recorded |
| TGT-01, TGT-04, TGT-05 | `targets`: insert, label update, registered -> archived. Validations -> stale on archive. Dependent jobs `stop_requested` | audit target.* |
| SCP-02 | `scope_versions`: insert, previous -> superseded. `scope_rules`: insert. `refused_exclusions`: insert. Validations -> stale. Dependent jobs `stop_requested`. Ready assessments -> draft | audit scope.version_published, `assessment_transitions`, `job_transitions` |
| SCP-07 | `refused_exclusions`: open -> acknowledged | audit scope.exclusion_acknowledged |
| LAB-01, LAB-04 | `lab_registrations`: insert, active -> revoked. Validation -> stale. Dependent jobs `stop_requested` on revoke | audit lab.registered, lab.revoked |
| VAL-01 | `target_validations`: previous current -> stale, insert. `assessments.current_validation_id` set | audit validation.recorded |
| ASM-01 | `assessments`: insert (draft) | `assessment_transitions`, audit assessment.created |
| ASM-04, ASM-05, ASM-06 | `assessments` update. Target set diff in `assessment_targets`. Validation -> stale. Raw capture flag | audit assessment.updated, targets_set, raw_capture_changed |
| ASM-07 to ASM-12 | `assessments.status`: draft -> ready, ready -> running, running -> paused, paused -> running, running or paused -> completed, any open state -> cancelled. Cancel sets dependent jobs `stop_requested` | `assessment_transitions`, audit assessment.* |
| PLN-01, PLN-04 | `operations`: insert (content addressed). `plan_items`: insert, proposed -> rejected | audit plan_item.created, plan_item.rejected, `policy_decisions` (E1) |
| APR-01 | `approvals`: insert pending. `approval_events`: requested | audit approval.requested |
| APR-04, APR-05, APR-06 | `approvals`: pending -> approved, pending -> rejected, approved -> revoked. Revoke sets dependent jobs `stop_requested` | `approval_events`, `job_transitions`, audit approval.* |
| JOB-01, JOB-04 | `jobs`: insert queued or awaiting_approval, queued or awaiting_approval -> cancelled, running `control_state` -> stop_requested. `plan_items`: accepted -> converted | `job_transitions`, audit job.created, job.cancel_requested, `policy_decisions` (E1) |
| AST-06 | `targets`: insert (origin promoted_asset) | audit asset.promoted, `policy_decisions` (E4) |
| EVD-03 | `evidence_integrity_checks`: insert. `evidence_status` update. `system_alerts` on failure | audit evidence.integrity_checked |
| FND-01, FND-04 | `findings`: insert candidate. candidate -> needs_review | `finding_transitions`, audit finding.created, finding.submitted |
| FND-05 | needs_review -> inconclusive, false_positive, not_reproducible or awaiting_human_verification | `finding_transitions` (actor user, reason) |
| FND-06 | awaiting_human_verification -> verified. `finding_verifications` and `finding_verification_evidence`: insert | `finding_transitions` (actor user, validation id), audit finding.verified |
| FND-07 | Any outcome or verified -> needs_review. Verification marked superseded | `finding_transitions`, audit finding.reopened |
| FND-09, FND-11 | `finding_evidence`: insert. `validation_records`: insert (human, not a recommendation) | audit finding.evidence_linked, finding.validation_recorded |
| RPT-01, RPT-04, RPT-05, RPT-06 | `reports`: insert draft, file recorded, draft -> final. `report_findings`: replace set while draft. Confirmed section only for verified findings (CHECK) | audit report.* |
| AI-02, AI-05, AI-06 | `credential_refs` and `ai_providers`: insert, update. `integration_checks`: insert | audit ai.provider_* |
| AI-07, AI-08 | `program_provider_enablements`: insert, revoke | audit ai.provider_enabled, disabled |
| AI-10 | `jobs`: insert (`ai_task`, risk class ai) | audit ai.task_requested |
| TLS-02, TLS-03, TLS-04 | `tool_configs`: upsert (disabled, unverified), unverified -> verified or mismatch, verified -> enabled. `integration_checks`: insert | audit tool.* |
| TLS-06, TLS-07 | `passive_source_enablements`: insert, revoke | audit passive_source.enabled, disabled |

## 5. Requirement traceability

- Functional requirements in the PRD: 48. With an endpoint or an internal worker owner: 40. Excluded from the MVP (Won't): 8 (FR-SCOPE-003, FR-SCOPE-007, FR-APPROVAL-004, FR-TOOL-003, FR-TOOL-004, FR-PROBE-001, FR-ASSET-003, FR-REPORT-004).
- The full table, one row per requirement with endpoints and tables, is section 6 of `database/domain-model.md`. The endpoint to requirement mapping is `api/database-mapping.md`.
- Security controls to design elements: scope default deny and exclusions (`scope_rules` CHECKs, SCP-05, VAL-01), versioned scope (DB-10), human-only approval (approvals CHECKs, APR-04), re-authentication (`reauth_grants`, API-24), single account and recovery (DB-07, AUTH-02, AUTH-08), evidence integrity (DB-23, EVD-03), human verification (CC-10), audit (DB-15 to DB-17), credential references (DB-24), localhost browser protection (API-04, API-15, API-19, API-21), worker isolation of audit and policy writes (grants matrix).

## 6. Design questions and owner decisions

The 18 questions are recorded with exactly one status each in `dq-decisions.md`. Status summary: 2 Decided, 10 Proposed, 5 Deferred, 1 Owner Decision Required. A Proposed item is not approved. Only DQ-09 asks for an owner answer before its milestone. None blocks M1.

| ID | Title | Status | Milestone |
| --- | --- | --- | --- |
| DQ-01 | Clearing a compromised evidence state | Proposed | M6 |
| DQ-02 | Lab registration reuse | Decided | M17 |
| DQ-03 | Re-authentication for human verification | Decided | M22 |
| DQ-04 | Lifetime of a single-use re-authentication grant | Proposed | M4 |
| DQ-05 | Session expiry | Proposed | M4 |
| DQ-06 | Request limits and rate-limit numbers | Proposed | M4 and M16 |
| DQ-07 | Operational retention numbers | Proposed | M7 (sweeps) |
| DQ-08 | Program restore and permanent deletion | Deferred | M16 and G10 |
| DQ-09 | AI tasks and approval | Owner Decision Required | M12 (before the first remote adapter) |
| DQ-10 | Finding severity | Deferred | M22 |
| DQ-11 | Multi-user mode | Deferred | post-MVP |
| DQ-12 | Who may request an approval | Proposed | M19 |
| DQ-13 | Auditing reads of the audit trail | Proposed | M5 |
| DQ-14 | Manual asset entry | Deferred | M20 |
| DQ-15 | Downloading evidence that failed an integrity check | Proposed | M6 |
| DQ-16 | Who supplies a tool digest | Proposed | M10 |
| DQ-17 | Behavior after a database restore | Proposed | M3 and M4 |
| DQ-18 | Reusable re-authentication window | Deferred | M19 and M24 |

## 6a. Security invariants re-checked in the hardening round

| Invariant | Where it is enforced | Not enforced by the design alone |
| --- | --- | --- |
| Scope version references | `scope_versions`, `scope_rules`, composite foreign keys, `scope_version_id` on jobs, approvals, decisions, validations, assets (DB-10) | Evaluation under the right version at E1 to E3 |
| Target authorization | `target_validations` bound to the target-set hash, rules version and scope version; DB-29 | The attestation is a human claim (T-13) |
| Approval use accounting | CC-01, CC-02, `approval_uses`, retries reuse the original use (DB-13) | Concurrency tests (G-consume) |
| Job claims and fencing | CC-03, `claim_epoch`, CC-15 recovery generation | Worker implementation and timing tests |
| Retry behavior and process termination | CC-04, CC-05, CC-06, `termination_confirmed_at` | A false termination confirmation (worker logic) |
| Evidence integrity | Immutable `evidence_items`, append-only checks, separate `evidence_status`, DQ-01 and DQ-15 | Content of the evidence store is outside the database |
| Human-only verification | CHECKs on `finding_transitions`, `findings`, FK `finding_verifications.verified_by` to `users`, FND-06 only route | Session theft |
| Audit immutability | No UPDATE or DELETE grant, triggers, DB-17 | Migration role and owner can alter rows (D-78) |
| Session revocation | `sessions.revoked_at`, recovery and password-change revoke all | Browser cookie behavior (T-57) |
| Host, Origin and CSRF | Pipeline order in `authentication-and-authorization.md`, no API tokens | Next.js rewrite behavior is unverified (T-56) |
| Cross-program access | Composite foreign keys (DB-09), flat item routes (API-05) | Application bugs that skip the program check, covered by tests |
| Remote AI data disclosure | `program_provider_enablements`, `ai_transmissions`, `ai_providers.effective_locality`, DQ-09 | DQ-09 is an open owner question |
| Secret handling | Hashes only, Secret Service references only (DB-24), no secret in examples | Redaction quality (T-49) |
| Restored authority | DB-28, CC-15, AUTH-10 | Same-user attacker, whole-home snapshot restore |
| No state implies authorization | DB-29. Claim and launch re-evaluate the policy engine. | Implementation must not skip E2 and E3 |
| No AI approval or verification | No actor field in request schemas, human-only CHECKs, DQ-12 | None known |


## 7. Residual risks of the design

- The model is a paper design. Constraints that look sufficient here (for example the deferred parity triggers) can still have gaps when written as SQL. M3 must add tests that try to violate each CHECK and unique index (G-consume and the security regression tests).
- The database cannot stop a process. A recorded `stop_requested` is intent. The 30-second objective depends on worker timing that nobody has measured (O-35).
- One OS user owns the database, the worker and the API. Grants limit mistakes and SQL defects, not a compromised same-user process (D-78). Audit rows can be altered by the migration role and the owner.
- Redaction of evidence before storage is best effort. A missed secret format reaches disk and the `evidence_items` bytes (T-49).
- The browser behaviors that the session and CSRF design rely on are unverified (T-55 to T-57). The OpenAPI file has not been checked against the official 3.1 schema.
- Rate limits, timeouts and sizes are proposals. A wrong number is a usability or denial-of-service problem and not a correctness one.
- Copy-forward scope versions grow with each edit. Rule sets are small, so this is accepted, and a cap on versions per program is not designed.
- The `jsonb` columns (`operations.parameters`, `observations.payload`, `audit_events.detail`) hold data produced by untrusted tools. Size CHECKs bound them. Their contents are never interpreted as instructions.

## 8. Verdict

The design is consistent with itself and with the approved decisions as far as a structural validator and a document review can show. It is **not** complete in the sense of ready for unreviewed implementation: DQ-09 needs an owner answer before M12, the Proposed items need acceptance, the numbers need review, and the browser, concurrency and sandbox assumptions need tests. Implementation of M1 does not depend on any of these questions.

