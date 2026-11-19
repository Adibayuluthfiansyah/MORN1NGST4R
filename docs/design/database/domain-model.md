# Domain model

Status: design proposal. The model maps the PRD, the steering documents and the decision records onto persistence. It does not change a decision. A table exists only where an entity has identity, history or an integrity rule. Everything else is a column or a value object.

## 1. Bounded domains

The domain names below follow the candidate list of the task. The "Application area" column is the owning module of `structure.md` section 5.2. Cross-domain access goes through application contracts (structure.md section 2), so a domain never joins another domain's tables in its repository.

| Domain | Table groups | Application area | Main requirements | Contents |
| --- | --- | --- | --- | --- |
| Identity and access | identity, policy | auth, identity | FR-AUTH-001, FR-AUTH-002, FR-POLICY-002 | The one account, its credential hashes, sessions, re-authentication grants, login attempts, bootstrap and recovery records, and the versioned policy values. Tables: `bootstrap_secrets`, `login_attempts`, `policy_settings`, `reauth_grants`, `recovery_codes`, `recovery_state`, `sessions`, `user_credentials`, `users`. |
| Programs and scope | scope | program, target, scope | FR-PROG-001, FR-PROG-002, FR-SCOPE-001 to FR-SCOPE-007 | Programs, rules references, immutable scope versions and rules, refused exclusions, targets, lab registrations and target authorization validation. Tables: `lab_registration_ranges`, `lab_registrations`, `program_rules_versions`, `programs`, `refused_exclusions`, `scope_rules`, `scope_versions`, `target_validation_targets`, `target_validations`, `targets`. |
| Assessments | assessment, execution | assessment, job | FR-ASSESS-001, FR-JOB-001, FR-FINDING-004 | Assessments, target sets, plan items, jobs, attempts and their histories. Tables: `assessment_targets`, `assessment_transitions`, `assessments`, `job_attempts`, `job_transitions`, `jobs`, `plan_items`. |
| Authorization and audit | authorization, audit | approval, scope (policy evaluator), audit | FR-APPROVAL-001 to FR-APPROVAL-006, FR-POLICY-001, FR-AUDIT-001 | Canonical operations, policy decisions, approvals, uses and history, audit events and system alerts. Tables: `approval_events`, `approval_uses`, `approvals`, `audit_events`, `operations`, `policy_decisions`, `system_alerts`. |
| Asset intelligence | asset | asset | FR-ASSET-001, FR-ASSET-002 | Assets, services, endpoints, parameters, observations and relationships. Tables: `asset_relationships`, `assets`, `endpoint_parameters`, `endpoints`, `observations`, `services`. |
| Evidence and findings | evidence, finding | evidence, finding, validation, reporting | FR-EVIDENCE-001 to FR-EVIDENCE-003, FR-FINDING-001 to FR-FINDING-003, FR-REPORT-001 to FR-REPORT-003 | Evidence metadata and integrity, findings and their lifecycle, validation, human verification and reports. Tables: `evidence_integrity_checks`, `evidence_items`, `evidence_status`, `finding_evidence`, `finding_transitions`, `finding_verification_evidence`, `finding_verifications`, `findings`, `report_findings`, `reports`, `validation_records`. |
| AI and integrations | integration, platform | ai, credential | FR-AI-001 to FR-AI-005, FR-TOOL-001, FR-RECON-001, FR-CRED-001 | Providers, enablements, transmissions, AI outputs, tool registrations, passive sources, credential references, integration checks and idempotency keys. Tables: `ai_outputs`, `ai_providers`, `ai_transmissions`, `credential_refs`, `idempotency_keys`, `integration_checks`, `passive_source_enablements`, `program_provider_enablements`, `tool_configs`. |

## 2. Table or not

| Candidate from the task | Decision | Where it lives |
| --- | --- | --- |
| Users, authentication credentials | Tables | `users`, `user_credentials` (hash only, history kept) |
| Sessions | Table | `sessions` (token and CSRF token hashed) |
| Authentication events | Audit events plus an operational counter | `audit_events` (auth.*), `login_attempts` for the rate limit |
| Re-authentication events | Table plus audit | `reauth_grants` (single use, bound to an action), `audit_events` auth.reauthenticated |
| Bootstrap and recovery records | Tables | `bootstrap_secrets`, `recovery_codes` (hashes only) |
| Programs, targets | Tables | `programs`, `program_rules_versions`, `targets` |
| Scope rules and exclusions | One table, exclusion is a flag | `scope_rules.exclude` (D-57). Refused exclusions are their own table because they carry a lifecycle (D-64 choice 10). |
| Scope versions and snapshots | Immutable header plus copied rules | `scope_versions`, `scope_rules` (DB-10) |
| Lab registrations and lifecycle | Immutable row with revocation, plus ranges. No separate lifecycle table | `lab_registrations`, `lab_registration_ranges`, `audit_events` (a change is a new row, D-47) |
| Target authorization attestations | Tables | `target_validations`, `target_validation_targets` |
| Assessment runs and plans | Tables | `assessments`, `assessment_targets`, `assessment_transitions`, `plan_items` |
| Jobs, attempts, transitions | Tables | `jobs`, `job_attempts`, `job_transitions` |
| Execution policy references | Table | `policy_decisions` (E1 to E4), referenced by attempts and audit |
| Approvals, usage, revocations, history | Tables. Revocation is columns plus an event, not a table | `approvals`, `approval_uses`, `approval_events` |
| Approval-bound operation snapshots | Content-addressed table | `operations` (hash covers target, parameters, class, destinations, limits) |
| Audit events | One append-only table | `audit_events`, `system_alerts` |
| Assets, endpoints, services | Tables | `assets`, `services`, `endpoints`, `endpoint_parameters` |
| Discovery observations and provenance | One append-only table | `observations` (origin, tool, job, attempt, evidence, scope version) |
| Asset relationships | Table | `asset_relationships` |
| Technology fingerprints | Observation payload, not a table | `observations.observation_kind = technology` |
| Evidence metadata, hashes, references | Tables | `evidence_items` (immutable), `evidence_integrity_checks`, `evidence_status`, `finding_evidence` |
| Findings, validation, human verification, transitions | Tables | `findings`, `finding_transitions`, `validation_records`, `finding_verifications`, `finding_verification_evidence` |
| Finding observations | Not a separate table. A repeated observation adds evidence to the existing finding by `finding_key` (D-75) | `findings.finding_key`, `finding_evidence` |
| Report status | Derived view, not a column | `finding_report_status` over `report_findings` and `reports` (DB-18) |
| Reports | Tables | `reports`, `report_findings` |
| AI provider configurations and capability settings | One table. Adapter-fixed capabilities live in code | `ai_providers`, `program_provider_enablements` |
| Tool configurations, integration metadata | Tables | `tool_configs`, `passive_source_enablements`, `integration_checks` |
| User-defined policy settings | Versioned table | `policy_settings` |
| Provider API keys, tool credentials | Not stored. Opaque references only | `credential_refs` (Secret Service key, D-52) |
| Skills, tool manifests | Not in the database | Repository `skills/` and manifests with SHA-256 (D-82). Only enablements are stored. |
| Severity, CVSS, bounty status | Not modeled. Not in the PRD | Deferred (DQ-10) |

## 3. State machines

Every state column has a named CHECK. Every state with history has a transition table whose CHECK lists the legal pairs (DB-11). "Actor" is who may cause the transition. A human is the authenticated session user. `worker` and `system` are internal actors. AI and tool output are never actors of an HTTP call.

| Entity | State set | Legal transitions (from -> to) | Actor | API |
| --- | --- | --- | --- | --- |
| assessments | draft, ready, running, paused, completed, cancelled | draft->ready, ready->draft (a stale validation or target change), ready->running, running->paused, paused->running, running->completed, paused->completed, draft/ready/running/paused->cancelled | human | ASM-07 to ASM-12 |
| jobs.state | queued, awaiting_approval, running, succeeded, failed, cancelled | create->queued or awaiting_approval, awaiting_approval->queued, queued->running (claim), running->succeeded, running->failed, running->queued (retry, only while attempts remain and termination is confirmed), queued->failed, queued/awaiting_approval/running->cancelled | human (create, cancel), worker (claim, finish), system (reaper) | JOB-01, JOB-04, WRK-01 to WRK-07 |
| jobs.control_state | none, stop_requested, stop_confirmed, stop_failed | none->stop_requested (cancel, revocation, scope or lab change, stale validation, assessment cancel), stop_requested->stop_confirmed or stop_failed (worker only) | human or system requests, worker confirms | JOB-04, APR-06, SCP-02, LAB-04, ASM-12 |
| job_attempts.state | launching, running, succeeded, failed, timed_out, killed, cancelled, lost | launching->running->(succeeded, failed, timed_out, killed, cancelled), running->lost (lease expiry). Termination confirmation is a separate timestamp. | worker | WRK-03, WRK-05, WRK-07 |
| approvals | pending, approved, rejected, revoked (expiry derived) | pending->approved, pending->rejected, approved->revoked | human only | APR-04 to APR-06 |
| lab_registrations | active, expired, revoked | active->expired (sweep or E-check), active->revoked | human, system (expiry) | LAB-01, LAB-04, WRK-08 |
| target_validations | current, stale, expired, invalidated | current->stale (scope, rules, lab or target-set change), current->expired (24 h), current->invalidated (manual) | system as a consequence of a human change | VAL-01, SCP-02, PRG-06, LAB-01, LAB-04, ASM-05 |
| refused_exclusions | open, acknowledged, replaced | open->acknowledged (human, re-auth), open->replaced (a supported exclusion denies at least as much) | human | SCP-07, SCP-02 |
| plan_items | proposed, accepted, rejected, converted, superseded | proposed->accepted or rejected, accepted->converted (job created), proposed or accepted->superseded | human | PLN-01 to PLN-04, JOB-01 |
| findings | candidate, needs_review, inconclusive, false_positive, not_reproducible, awaiting_human_verification, verified | D-75 table, see `finding_transitions` constraints. Automation: create->candidate, candidate->needs_review only. Verified only by a user from awaiting_human_verification. | human, automation (two transitions only) | FND-01, FND-04 to FND-07 |
| reports | draft, final | draft->final (needs a generated file) | human | RPT-05, RPT-06 |
| evidence_status.integrity_state | unchecked, ok, compromised, missing | unchecked->ok, any->compromised, any->missing. compromised is sticky (DQ-01). | worker, system, human-triggered check | EVD-03, EVD-05, sweeps |
| evidence_status.retention_state | active, purge_pending, purged | active->purge_pending->purged, never backwards | human via the retention role | `Service.Purge` of `internal/application/evidence` (M7). No route or command offers it yet: a later milestone calls it |
| recovery_state | normal, reconciling | normal->reconciling (phase 1), reconciling->normal (phase 3). generation only increases. See `recovery-generation.md`. | human (AUTH-10) and start-up resume | AUTH-10 |
| sessions | active, revoked (expiry derived) | active->revoked | human, system | AUTH-04, AUTH-06, AUTH-08, AUTH-09 |

## 4. Protocols

### 4.1 Scope publication and invalidation (DB-10)

1. Publishing locks the current `scope_versions` row of the program `FOR UPDATE` (CC-07). The new version number is `previous + 1`.
2. The scope engine canonicalizes every submitted rule. A rule outside the grammar is refused. A refused exclusion becomes a `refused_exclusions` row and blocks readiness until acknowledged or replaced (D-64 choice 10). Wildcard inclusion is refused (CHECK on `scope_rules`).
3. Insert the new header and the full rule list. Set `superseded_at` on the previous header. `widening` is computed by comparing the canonical sets. A widening version consumes a re-authentication grant (CHECK).
4. In the same transaction mark every `current` validation of the program `stale` with reason `scope_change`, set `jobs.control_state = stop_requested` for queued and running jobs whose `scope_version_id` is superseded (index `jobs_scope_idx`), and move `ready` assessments back to `draft`.
5. The transaction commits the **intent**. The worker observes `stop_requested` and stops the process group, then writes `stop_confirmed` (DB-12). The 30-second objective is measured at the worker, not promised by the commit (D-48, O-35).
6. Existing jobs keep their original `scope_version_id`. Their history is not rewritten. A new job must be created and evaluated under the new version.

### 4.2 Approval, claim and consumption (DB-13)

1. A plan item has a canonical `operations` row. Creating it computes the hash over assessment, target, tool, typed parameters, class, declared destinations and limits.
2. A human requests an approval for that exact operation (`approvals.operation_id`). The approval screen renders only from the operation row (D-45).
3. Approval records the human, the time, the expiry, `max_uses`, `max_attempts`, the validation record and the scope version in force. An intrusive approval consumes a re-authentication grant, has `max_uses = 1` and `max_attempts = 1` (CHECKs).
4. A job for a risky operation must reference an approved approval (`CHECK`) and is created `queued`. The use is **not** consumed at creation.
5. A worker claims the job and consumes the use atomically (see DB-13). Two workers cannot both succeed: the first conditional update wins and the second finds zero rows or a locked job.
6. A retry claim finds `jobs.approval_use_id` already set and does not touch `approvals.uses_consumed`. The attempt counter is separate (`jobs.execution_attempts`, CC-05).
7. Expiry stops **new** launches only. A running attempt is not touched by expiry and is bounded by its approved duration (D-48). Revocation sets `control_state = stop_requested` on dependent jobs in the same transaction.

### 4.3 Attempts, stop and retry (DB-12, DB-14)

- Launch: first run the sandbox preflight and E3 outside the attempt transaction. A denial or a refused preflight writes a `policy_decisions` row and an audit event and creates no attempt, so it is not a launch (D-55). Then, in one transaction, increment `execution_attempts` conditionally and insert `job_attempts` (the unconfirmed-attempt unique index refuses a second live attempt). The process starts after the commit. If the spawn itself fails after the commit, the attempt is marked `failed` with `sandbox_refused` and still counts, because a consumed launch is never restored silently (D-55).
- Stop: a human or the system writes `control_state = stop_requested` and the reason. The worker kills the process group, verifies it is gone, writes `termination_confirmed_at` and `stop_confirmed`. If it cannot, it writes `stop_failed` and raises a `system_alerts` row. Only a confirmed attempt allows a retry.
- Lease loss: the reaper marks the attempt `lost`. An intrusive job fails (D-49). A retryable job is requeued only after the attempt's termination is confirmed and `execution_attempts < max_execution_attempts`.
- Fencing: every worker write is conditioned on `(job_id, claim_epoch, state = running)`, so a stale worker cannot commit (P-36).

### 4.4 Evidence write and verification (DB-23)

1. The worker streams the redacted bytes to a temporary file in the evidence directory, hashes while writing, syncs, publishes without overwriting, and then commits `evidence_items`, `evidence_status` and the audit event in one transaction (P-37). A failed commit leaves an orphan file that the sweep reports.
2. Reads re-hash. Outcomes are recorded in `evidence_integrity_checks`: `match`, `hash_mismatch`, `size_mismatch`, `missing`, `metadata_inconsistent`.
3. A failing check sets `evidence_status.integrity_state` to `compromised` or `missing`, raises `system_alerts` kind `evidence_integrity`, and writes an audit event. Evidence in those states cannot support a verification (`finding_verification_evidence.integrity_check_id` must reference a `match`).
4. A file with no row is an orphan, reported and never used. A row with no file is `missing`. A row whose size or digest differs is `compromised`. A row that disagrees with itself (for example a purged state with a readable file) is `metadata_inconsistent`.

### 4.5 Finding verification (DB-11)

A human calls the verify action. In one transaction: lock the finding, require status `awaiting_human_verification`, require a validation record of this finding, require every listed evidence item to have a fresh `match` check, insert `finding_verifications` (user only), insert `finding_verification_evidence` rows with the hashes, insert the `finding_transitions` row (actor kind `user`, validation id), set `findings.status` and `verification_id`. The CHECKs make a verified status without a verification row, or a verification by a non-user actor, impossible. Reopening inserts a transition, marks the verification superseded and removes the finding from confirmed sections of draft reports.

### 4.6 Audit writing (DB-15 to DB-17)

- A state change and its audit event commit in one transaction. A denial or a failed login has no state change, so its audit event commits in its own transaction (P-33a).
- If the audit write fails the operation does not start. Cancellation, revocation, scope narrowing and denials still take effect, the audit record is retried a bounded number of times from an in-process buffer, and an exhausted bound writes a `system_alerts` row and keeps denying new operations (D-48).
- `detail` is redacted before the insert and capped at 8 KiB. No secret, token, cookie, authorization header or raw request body is written.

### 4.7 Cross-program isolation (DB-09)

A target, assessment, asset, evidence item or finding cannot be linked to another program's row because the composite foreign keys include `program_id`. An approval cannot approve an operation of another assessment because the foreign key is `(assessment_id, operation_id)`. A lab registration cannot use another program's target. The API additionally derives the program from the loaded row and never accepts a client-supplied program for an item.

### 4.8 Recovery generation (DB-28)

Restore detection uses an external marker file and the `recovery_state` row. The comparison, the reconciliation phases, the crash behavior and the threat boundary are in `recovery-generation.md`. The claim and launch transactions require `recovery_state.state = normal` and the generation read from the marker (CC-15).

## 5. Audit event catalog

Every state-changing endpoint writes exactly one event with its action name, outcome `success`, `denied` or `failure`. Worker and system events are listed after the endpoint events. Read-only calls are not audited, except evidence content and report exports (`evidence.accessed`, `report.exported`) (DQ-13).

**How events are written (M5, FR-AUDIT-001, SEC 13).** One port writes them, `AuditSink`, with two semantics. `Require` is the record that an operation needs before it may start: inside the transaction of the operation it joins it, so a failed write rolls the operation back. `Record` is for an event whose operation has already taken effect or been denied (a denial, a cancellation, a revocation): it commits on its own, is retried within a bound and never changes the outcome of the operation. When the retries are exhausted the sink raises one persistent alert (`system_alerts`, kind `audit_retry_exhausted`), and `Gate` denies new operations until a human clears it. An alert that cannot be stored is kept in memory and `Gate` keeps denying until it is. The retry bound is 3 attempts, waits of 100 and 400 ms and 1 second per attempt, approved by the owner on 2026-11-18. The correlation id (`request_id`) is made by the server and a value that the caller sends is ignored. An unauthenticated caller is the actor kind `anonymous`. The refusals of one action for rate limiting are recorded once per attempt window (15 minutes) with a count of the refusals that the window suppressed. Refusals for a wrong credential are never coalesced. The fields of a scope evaluation (D-91) are in the detail: `decision`, `reasons` and `decidingRuleId` (null when no rule decided). An AI transmission (`ai.transmission`) carries `provider`, `bytes` and `contentDigest` and no content.

| Action | Source | Meaning |
| --- | --- | --- |
| `ai.provider_configured` | AI-02 | state change through the API |
| `ai.provider_disabled` | AI-08 | state change through the API |
| `ai.provider_enabled` | AI-07 | state change through the API |
| `ai.provider_tested` | AI-06 | state change through the API |
| `ai.provider_updated` | AI-05 | state change through the API |
| `ai.task_requested` | AI-10 | state change through the API |
| `approval.approved` | APR-04 | state change through the API |
| `approval.rejected` | APR-05 | state change through the API |
| `approval.requested` | APR-01 | state change through the API |
| `approval.revoked` | APR-06 | state change through the API |
| `assessment.cancelled` | ASM-12 | state change through the API |
| `assessment.completed` | ASM-11 | state change through the API |
| `assessment.created` | ASM-01 | state change through the API |
| `assessment.paused` | ASM-09 | state change through the API |
| `assessment.raw_capture_changed` | ASM-06 | state change through the API |
| `assessment.ready` | ASM-07 | state change through the API |
| `assessment.resumed` | ASM-10 | state change through the API |
| `assessment.started` | ASM-08 | state change through the API |
| `assessment.targets_set` | ASM-05 | state change through the API |
| `assessment.updated` | ASM-04 | state change through the API |
| `asset.promoted` | AST-06 | state change through the API |
| `auth.account_created` | AUTH-02 | state change through the API |
| `auth.login` | AUTH-03 | state change through the API |
| `auth.logout` | AUTH-04 | state change through the API |
| `auth.password_changed` | AUTH-09 | state change through the API |
| `auth.reauthenticated` | AUTH-07 | state change through the API |
| `auth.recovery_reset` | AUTH-08 | state change through the API |
| `auth.session_revoked` | AUTH-06 | state change through the API |
| `evidence.accessed` | EVD-05 | state change through the API |
| `evidence.integrity_checked` | EVD-03 | state change through the API |
| `finding.created` | FND-01 | state change through the API |
| `finding.evidence_linked` | FND-09 | state change through the API |
| `finding.reopened` | FND-07 | state change through the API |
| `finding.replay_requested` | FND-13 | state change through the API |
| `finding.review_outcome` | FND-05 | state change through the API |
| `finding.submitted` | FND-04 | state change through the API |
| `finding.validation_recorded` | FND-11 | state change through the API |
| `finding.verified` | FND-06 | state change through the API |
| `job.cancel_requested` | JOB-04 | state change through the API |
| `job.created` | JOB-01 | state change through the API |
| `lab.registered` | LAB-01 | state change through the API |
| `lab.revoked` | LAB-04 | state change through the API |
| `passive_source.disabled` | TLS-07 | state change through the API |
| `passive_source.enabled` | TLS-06 | state change through the API |
| `plan_item.created` | PLN-01 | state change through the API |
| `plan_item.rejected` | PLN-04 | state change through the API |
| `policy.changed` | POL-02 | state change through the API |
| `program.archived` | PRG-05 | state change through the API |
| `program.created` | PRG-01 | state change through the API |
| `program.rules_recorded` | PRG-06 | state change through the API |
| `program.updated` | PRG-04 | state change through the API |
| `report.created` | RPT-01 | state change through the API |
| `report.exported` | RPT-07 | state change through the API |
| `report.finalized` | RPT-06 | state change through the API |
| `report.findings_set` | RPT-04 | state change through the API |
| `report.generated` | RPT-05 | state change through the API |
| `scope.exclusion_acknowledged` | SCP-07 | state change through the API |
| `scope.version_published` | SCP-02 | state change through the API |
| `system.recovery_reconciled` | AUTH-10 | state change through the API |
| `system_alert.cleared` | INT-02 | state change through the API |
| `target.archived` | TGT-05 | state change through the API |
| `target.registered` | TGT-01 | state change through the API |
| `target.updated` | TGT-04 | state change through the API |
| `tool.enabled` | TLS-04 | state change through the API |
| `tool.registered` | TLS-02 | state change through the API |
| `tool.verified` | TLS-03 | state change through the API |
| `validation.recorded` | VAL-01 | state change through the API |
| `job.claimed` | WRK-01 | Job claim with the E2 decision id |
| `job.launched` | WRK-03 | Attempt launch with the E3 decision id |
| `job.succeeded` | WRK-04 | Results committed |
| `job.failed` | WRK-04, WRK-07 | Failure class recorded |
| `job.stop_confirmed` | WRK-05, WRK-06 | Process tree confirmed gone |
| `job.stop_failed` | WRK-06 | Stop could not be confirmed, alert raised |
| `job.lease_lost` | WRK-07 | Lease expired |
| `approval.use_consumed` | WRK-01 | One use consumed at claim |
| `approval.use_denied` | WRK-01 | Claim refused: expired, revoked, exhausted or mismatched. Not an attempt, not a use |
| `policy.decision_denied` | E1 to E4 | Every denial with reasons, written in its own transaction |
| `evidence.captured` | WRK-04 | Evidence metadata committed |
| `evidence.integrity_failed` | EVD-03, EVD-05, sweeps | Hash mismatch or missing content |
| `evidence.purge_requested` | retention procedure | The content of an item is marked for purge (M7). Added to the catalog in M7, with `evidence.purged`, because the purge procedure of D-77 writes them |
| `evidence.purged` | retention procedure | The content was removed and the item is purged (M7). Its metadata and hash stay |
| `retention.purged` | worker maintenance | Operational rows (sessions, re-authentication grants, login attempts) were purged past their age floor (M7, RC-B). Audit rows are not purged |
| `finding.auto_created` | WRK-04 | Automated Candidate |
| `finding.auto_submitted` | WRK-04 | Automated Needs Review |
| `ai.transmission` | WRK-09 | Provider, program, size, digest |
| `system.first_run` | start-up | The first run created the recovery state and the marker (M3). Added to the catalog in M5 because the code wrote it before the catalog listed it |
| `system.recovery_detected` | start-up | A restore or a marker problem was detected, the installation is in recovery_required |
| `audit.retry_exhausted` | audit sink | Persistent alert raised, new operations denied (D-48) |
| `system.restore_recovered` | AUTH-10 | Authority recorded in the restored database was revoked (DB-27, DB-28) |
| `assessment.validation_expired` | WRK-08 | Validation record expired |
| `approval.expiry_observed` | WRK-08 | Expiry noted, no process affected |
| `lab.expired` | WRK-08 | Lab registration expired |
| `auth.login_failed` | AUTH-03 | Failed login, own transaction |
| `auth.session_expired` | WRK-08 | Session cleanup |

## 6. Requirement to data mapping

Every functional requirement of `docs/PRD.md` and the endpoints and tables that serve it. A requirement with no endpoint is served by the worker or is excluded from the MVP.

| Requirement | Endpoints | Tables | Internal owner or note |
| --- | --- | --- | --- |
| FR-PROG-001 Program registration | PRG-01, PRG-02, PRG-03, PRG-04, PRG-05 | `programs`, `audit_events`, `assessments` |  |
| FR-PROG-002 Program rules reference and version | PRG-06, PRG-07 | `program_rules_versions`, `target_validations`, `audit_events` |  |
| FR-SCOPE-001 Scope rule registration | TGT-01, TGT-02, TGT-03, TGT-04, SCP-01, SCP-02, SCP-03, SCP-04, SCP-06, SCP-07 | `targets`, `audit_events`, `scope_versions`, `scope_rules`, `refused_exclusions`, `target_validations`, `jobs`, `approvals`, `reauth_grants` |  |
| FR-SCOPE-002 Deterministic scope evaluation | SCP-05, AST-06 | `scope_versions`, `scope_rules`, `assets`, `targets`, `policy_decisions`, `audit_events` |  |
| FR-SCOPE-003 Resolved-address and redirect evaluation | none | `policy_decisions`, `lab_registrations`, `audit_events` | WRK-03, internal egress client (M9) |
| FR-SCOPE-004 Lab authorization | LAB-01, LAB-02, LAB-03, LAB-04 | `lab_registrations`, `lab_registration_ranges`, `targets`, `scope_versions`, `target_validations`, `reauth_grants`, `audit_events`, `jobs` |  |
| FR-SCOPE-005 Target authorization validation | VAL-01, VAL-02, VAL-03, WRK-08 | `target_validations`, `target_validation_targets`, `assessments`, `scope_versions`, `program_rules_versions`, `audit_events`, `approvals`, `approval_events`, `lab_registrations`, `sessions`, `idempotency_keys`, `evidence_status` |  |
| FR-SCOPE-006 Invalidation on change | AUTH-10, TGT-05, SCP-02, ASM-05 | `recovery_state`, `users`, `user_credentials`, `sessions`, `approvals`, `approval_events`, `target_validations`, `lab_registrations`, `jobs`, `job_attempts`, `job_transitions`, `assessments`, `audit_events`, `system_alerts`, `login_attempts`, `targets`, `assessment_targets`, `scope_versions`, `scope_rules`, `refused_exclusions`, `reauth_grants` |  |
| FR-SCOPE-007 Host name conversion | none | `scope_rules`, `targets` | canonicalization inside SCP-02, TGT-01, SCP-05 |
| FR-ASSESS-001 Assessment management | ASM-01, ASM-02, ASM-03, ASM-04, ASM-05, ASM-07, ASM-08, ASM-09, ASM-10, ASM-11, ASM-12, ASM-13 | `assessments`, `assessment_transitions`, `audit_events`, `assessment_targets`, `targets`, `target_validations`, `jobs`, `refused_exclusions`, `job_transitions`, `system_alerts` |  |
| FR-POLICY-001 Policy evaluation at enforcement points | JOB-01 | `jobs`, `plan_items`, `approvals`, `policy_decisions`, `operations`, `audit_events` | E1 in JOB-01 and PLN-01, E2 in WRK-01, E3 in WRK-03, E4 in AST-06 |
| FR-POLICY-002 Policy change control | POL-01, POL-02 | `policy_settings`, `reauth_grants`, `audit_events` |  |
| FR-APPROVAL-001 Approval record and binding | APR-01, APR-02, APR-03, APR-04, APR-05, APR-07 | `approvals`, `approval_events`, `operations`, `target_validations`, `scope_versions`, `policy_decisions`, `audit_events`, `reauth_grants` |  |
| FR-APPROVAL-002 Approval consumption and attempts | WRK-01, WRK-03, WRK-07 | `jobs`, `approvals`, `approval_uses`, `policy_decisions`, `job_transitions`, `audit_events`, `job_attempts` |  |
| FR-APPROVAL-003 Expiry, revocation and running work | AUTH-10, ASM-12, APR-06, JOB-04, WRK-05, WRK-06, WRK-08 | `recovery_state`, `users`, `user_credentials`, `sessions`, `approvals`, `approval_events`, `target_validations`, `lab_registrations`, `jobs`, `job_attempts`, `job_transitions`, `assessments`, `audit_events`, `system_alerts`, `login_attempts`, `assessment_transitions`, `idempotency_keys`, `evidence_status` |  |
| FR-APPROVAL-004 Approval classes | none | `operations`, `approvals` | class assigned by the reviewed manifest in PLN-01 |
| FR-APPROVAL-005 Approval interface and re-authentication | AUTH-07, PLN-03, APR-03, APR-04 | `reauth_grants`, `login_attempts`, `audit_events`, `plan_items`, `operations`, `approvals`, `approval_events`, `target_validations` |  |
| FR-APPROVAL-006 Fan-out limits | APR-01 | `approvals`, `approval_events`, `operations`, `target_validations`, `scope_versions`, `policy_decisions`, `audit_events`, `jobs` | APR-01, limits in the operation |
| FR-AUTH-001 Local account and session | AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06, AUTH-08, AUTH-10, AUTH-09 | `sessions`, `users`, `user_credentials`, `recovery_codes`, `bootstrap_secrets`, `audit_events`, `login_attempts`, `recovery_state`, `approvals`, `approval_events`, `target_validations`, `lab_registrations`, `jobs`, `job_attempts`, `job_transitions`, `assessments`, `system_alerts`, `reauth_grants` |  |
| FR-AUTH-002 Localhost request protection | AUTH-01 | `sessions`, `users` |  |
| FR-JOB-001 Persistent job lifecycle | JOB-01, JOB-02, JOB-03, JOB-04, JOB-05, JOB-06, WRK-01, WRK-02, WRK-07 | `jobs`, `plan_items`, `approvals`, `policy_decisions`, `operations`, `audit_events`, `job_transitions`, `job_attempts`, `approval_uses` | JOB-01 to JOB-06, WRK-01 to WRK-07 |
| FR-TOOL-001 Tool adapter framework | TLS-01, TLS-02, TLS-03, TLS-04 | `tool_configs`, `reauth_grants`, `audit_events`, `integration_checks`, `operations` | TLS-01 to TLS-04 |
| FR-TOOL-002 Sandbox isolation | TLS-03, INT-01, WRK-03 | `tool_configs`, `integration_checks`, `audit_events`, `system_alerts`, `ai_providers`, `jobs`, `job_attempts`, `policy_decisions` | WRK-03, TLS-03, INT-01 |
| FR-TOOL-003 Egress enforcement | none | `policy_decisions`, `job_attempts` | WRK-03 |
| FR-TOOL-004 Template-based vulnerability checks | none | `operations`, `approvals` | PLN-01, APR-01, D-58 gate |
| FR-RECON-001 Passive reconnaissance | TLS-05, TLS-06, TLS-07 | `passive_source_enablements`, `tool_configs`, `reauth_grants`, `audit_events`, `observations`, `jobs` | TLS-05 to TLS-07, JOB-01, WRK-04 |
| FR-PROBE-001 HTTP probing | none | `observations`, `evidence_items`, `jobs` | JOB-01, WRK-04 |
| FR-ASSET-001 Asset inventory | AST-01, AST-02, AST-05, AST-06, WRK-04 | `assets`, `scope_versions`, `services`, `observations`, `targets`, `policy_decisions`, `audit_events`, `evidence_items`, `evidence_status`, `job_attempts`, `jobs`, `job_transitions` |  |
| FR-ASSET-002 Endpoint and parameter inventory | AST-03, AST-04 | `endpoints`, `services`, `endpoint_parameters` |  |
| FR-ASSET-003 Scheduled or continuous refresh | none | none | none |
| FR-EVIDENCE-001 Evidence collection | EVD-01, EVD-02, WRK-04 | `evidence_items`, `evidence_status`, `observations`, `job_attempts`, `jobs`, `job_transitions`, `audit_events` | EVD-01, EVD-02, WRK-04 |
| FR-EVIDENCE-002 Integrity verification | EVD-02, EVD-03, EVD-04 | `evidence_items`, `evidence_status`, `evidence_integrity_checks`, `system_alerts`, `audit_events` |  |
| FR-EVIDENCE-003 Safe display and download | EVD-05 | `evidence_items`, `evidence_status`, `evidence_integrity_checks`, `audit_events` |  |
| FR-FINDING-001 Finding lifecycle | FND-01, FND-02, FND-03, FND-04, FND-05, FND-07, FND-08 | `findings`, `finding_transitions`, `audit_events`, `finding_verifications` |  |
| FR-FINDING-002 Validation workflow | FND-05, FND-09, FND-10, FND-11, FND-12 | `findings`, `finding_transitions`, `audit_events`, `finding_evidence`, `evidence_items`, `validation_records` |  |
| FR-FINDING-003 Human verification | FND-06 | `findings`, `finding_transitions`, `finding_verifications`, `finding_verification_evidence`, `validation_records`, `evidence_items`, `evidence_integrity_checks`, `reauth_grants`, `audit_events` |  |
| FR-FINDING-004 Test planning | PLN-01, PLN-02, PLN-03, PLN-04 | `plan_items`, `operations`, `policy_decisions`, `audit_events` |  |
| FR-AI-001 Provider abstraction and configuration | AI-02, AI-03, AI-04, AI-05, AI-06 | `ai_providers`, `credential_refs`, `reauth_grants`, `audit_events`, `integration_checks` |  |
| FR-AI-002 Remote provider disclosure | AI-07, AI-08 | `program_provider_enablements`, `ai_providers`, `reauth_grants`, `audit_events` |  |
| FR-AI-003 Bounded AI proposals | AI-10 | `jobs`, `ai_providers`, `program_provider_enablements`, `audit_events`, `ai_outputs`, `plan_items`, `findings` | AI-10, AI-11, PLN-02 |
| FR-AI-004 AI auditability | AI-09, WRK-09 | `ai_transmissions`, `audit_events` | AI-09, WRK-09 |
| FR-AI-005 Basic AI-assisted analysis | AI-10, AI-11 | `jobs`, `ai_providers`, `program_provider_enablements`, `audit_events`, `ai_outputs` |  |
| FR-REPORT-001 Report generation | RPT-01, RPT-02, RPT-03, RPT-04, RPT-05 | `reports`, `audit_events`, `report_findings`, `findings`, `finding_verifications`, `evidence_items` |  |
| FR-REPORT-002 Report status | RPT-04, RPT-06 | `report_findings`, `findings`, `finding_verifications`, `audit_events`, `reports` |  |
| FR-REPORT-003 Export safety | RPT-05, RPT-07 | `reports`, `report_findings`, `evidence_items`, `audit_events` |  |
| FR-REPORT-004 Submission tracking | none | none | none |
| FR-AUDIT-001 Audit trail | INT-01, INT-02, AUD-01 | `integration_checks`, `system_alerts`, `tool_configs`, `ai_providers`, `audit_events` |  |
| FR-CRED-001 Credential protection | AI-02, INT-01 | `ai_providers`, `credential_refs`, `reauth_grants`, `audit_events`, `integration_checks`, `system_alerts`, `tool_configs` | AI-02, TLS-01 (status only) |
