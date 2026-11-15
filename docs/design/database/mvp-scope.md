# Database scope and complexity

Status: design proposal. The model has 59 tables and 631 columns. The count was not reduced. Each table is classified so that nobody implements all of them in M1 or M3 by default, and so that merging decisions are visible. A classification is not an instruction to merge or drop anything.

## Classes

| Class | Meaning | Tables |
| --- | --- | --- |
| MVP | Required for MVP | 9 |
| SEC | Required for a security invariant | 33 |
| LATER | Required by a later milestone (M20 or later) | 9 |
| DEF | Deferred | 1 |
| CONS | Candidate for consolidation | 7 |
| **Total** | | **59** |

Properties column: A auditability, Z authorization, I integrity, C concurrency. Precedence: DEF, then SEC, then CONS, then LATER (first needed in M20 or later), then MVP. A table in class CONS is still implemented unless the owner decides to merge it, because each one keeps a property listed in its reason.

## Per table

| Table | Domain | Class | First needed | PRD | Properties | Reason |
| --- | --- | --- | --- | --- | --- | --- |
| `users` | identity | SEC | M4 | FR-AUTH-001 | Z | The one account, with the single-account constraint (D-60, DB-07). |
| `user_credentials` | identity | SEC | M4 | FR-AUTH-001 | Z | Password hash history without plaintext (D-60). |
| `recovery_codes` | identity | SEC | M4 | FR-AUTH-001 | Z | One-time recovery code hash (D-68). |
| `bootstrap_secrets` | identity | SEC | M4 | FR-AUTH-001 | Z | Single-use first-run secret hash (D-68). |
| `sessions` | identity | SEC | M4 | FR-AUTH-001, FR-AUTH-002 | Z | Server-side sessions with hashed tokens (D-60). |
| `reauth_grants` | identity | SEC | M4 | FR-APPROVAL-005 | ZC | Single-use proof of fresh password entry bound to one action (D-58, D-69). |
| `login_attempts` | identity | CONS | M4 | FR-AUTH-001 | Z | Rate-limit counter. Could be counted from `audit_events` failures with an index, at the cost of coupling the limiter to the audit table. |
| `policy_settings` | policy | SEC | M16 | FR-POLICY-002 | AZ | The one versioned policy store with old and new values (D-53, D-64 choice 4). |
| `programs` | scope | MVP | M16 | FR-PROG-001 | Z | Every assessment belongs to a program (SEC 2). |
| `program_rules_versions` | scope | SEC | M16 | FR-PROG-002, FR-SCOPE-005 | AI | Stored program rules reference that validation binds to (D-61). |
| `scope_versions` | scope | SEC | M16 | FR-SCOPE-001, FR-SCOPE-006 | AZI | Immutable scope snapshots that every authorization references (DB-10). |
| `scope_rules` | scope | SEC | M16 | FR-SCOPE-001, FR-SCOPE-002 | ZI | The rules of a version, with exclusions and the wildcard CHECK (D-35, D-57). |
| `refused_exclusions` | scope | SEC | M16 | FR-SCOPE-001 | AZ | A refused exclusion is never dropped silently (D-64 choice 10). |
| `targets` | scope | MVP | M16 | FR-SCOPE-001 | Z | Registered target identities. Registration is not authorization. |
| `lab_registrations` | scope | SEC | M17 | FR-SCOPE-004 | AZ | Bounded lab exemption with expiry and revocation (D-47). |
| `lab_registration_ranges` | scope | CONS | M17 | FR-SCOPE-004 | Z | Could be a `cidr[]` column of `lab_registrations`. A child table allows a port set per range and a range index. |
| `target_validations` | scope | SEC | M18 | FR-SCOPE-005 | AZI | Target authorization validation bound to a target-set hash and rules version (D-61). |
| `target_validation_targets` | scope | CONS | M18 | FR-SCOPE-005 | AI | Per-target decision snapshot. Could be a `jsonb` column. Kept as rows so the decision stays queryable and keyed to the rule that matched. |
| `assessments` | assessment | MVP | M16 | FR-ASSESS-001 | AZ | The unit of authorized testing. |
| `assessment_targets` | assessment | MVP | M16 | FR-ASSESS-001 | Z | Target set of an assessment, part of the target-set hash. |
| `assessment_transitions` | assessment | CONS | M21 | FR-ASSESS-001 | AI | State history. Duplicates part of `audit_events`. Kept for the legal-pair CHECK (DB-11). |
| `plan_items` | assessment | MVP | M19 | FR-FINDING-004 | AZ | Planned operations, data and not executable (P-38). |
| `operations` | authorization | SEC | M19 | FR-APPROVAL-001, FR-APPROVAL-004 | ZI | Content-addressed canonical operation that an approval binds (D-51). |
| `policy_decisions` | authorization | SEC | M19 | FR-POLICY-001 | AZ | Decision identity for E1 to E4 so an authorization is traceable (P-04). |
| `approvals` | authorization | SEC | M19 | FR-APPROVAL-001 to FR-APPROVAL-006 | AZC | Human approval bound to one operation, with expiry and use bound (D-06, D-45, D-51). |
| `approval_events` | authorization | CONS | M19 | FR-APPROVAL-001 | A | Approval history. Overlaps `audit_events`. Kept because the history endpoint must not depend on audit retention. |
| `jobs` | execution | SEC | M7 | FR-JOB-001 | AZC | Persistent job with lease, fencing and control state (P-36, DB-12). |
| `approval_uses` | authorization | SEC | M7 | FR-APPROVAL-002 | ZC | One consumed use per approval and job (D-30, D-55). The concurrency constraint lives here. |
| `job_attempts` | execution | SEC | M7 | FR-JOB-001, FR-APPROVAL-002 | AZC | Launch accounting and confirmed termination (D-49, D-64 choice 12). |
| `job_transitions` | execution | MVP | M7 | FR-JOB-001 | AC | Legal-pair history of job state and control state. |
| `audit_events` | audit | SEC | M5 | FR-AUDIT-001 | A | Append-only audit trail (SEC 13). |
| `system_alerts` | audit | SEC | M5 | FR-AUDIT-001 | A | Persistent alert when audit retries are exhausted keeps new operations denied (D-48). |
| `assets` | asset | LATER | M20 | FR-ASSET-001 | Z | Inventory of hosts with scope decision and version. |
| `services` | asset | LATER | M20 | FR-ASSET-001 | - | Ports and services of an asset. |
| `endpoints` | asset | LATER | M20 | FR-ASSET-002 | - | Endpoint inventory. |
| `endpoint_parameters` | asset | LATER | M20 | FR-ASSET-002 | - | Parameter inventory. |
| `observations` | asset | LATER | M20 | FR-ASSET-001 | AIC | Append-only provenance with idempotent result persistence per attempt. |
| `asset_relationships` | asset | DEF | M20 revisit | none in FR-ASSET-001 | - | The product pillar names relationship mapping, but no functional requirement needs it. Trigger: a relationship view or analysis is specified. |
| `evidence_items` | evidence | SEC | M6 | FR-EVIDENCE-001 | AI | Immutable evidence metadata with SHA-256 and provenance (D-10, P-37). |
| `evidence_integrity_checks` | evidence | SEC | M6 | FR-EVIDENCE-002 | AI | Append-only verification history. |
| `evidence_status` | evidence | SEC | M6 | FR-EVIDENCE-002 | I | Mutable integrity and retention projection kept apart from the immutable metadata (P-37). |
| `findings` | finding | LATER | M22 | FR-FINDING-001 | AZ | Findings with the seven-state lifecycle and deduplication key (D-75). |
| `finding_transitions` | finding | SEC | M22 | FR-FINDING-001, FR-FINDING-003 | AZI | Legal transitions and the human-only Verified rule as CHECKs (D-75, SEC 5). |
| `finding_evidence` | finding | LATER | M22 | FR-FINDING-002 | I | Link between findings and evidence, owned by finding (P-23). |
| `validation_records` | finding | LATER | M22 | FR-FINDING-002 | AZ | Validation attempts and recommendations that never change status. |
| `finding_verifications` | finding | SEC | M22 | FR-FINDING-003 | AZ | Human verification record that can only reference a user (SEC 5). |
| `finding_verification_evidence` | finding | SEC | M22 | FR-FINDING-003 | AI | Evidence hashes and integrity checks relied on at verification. |
| `reports` | finding | MVP | M14 | FR-REPORT-001, FR-REPORT-002 | A | Generated reports with draft and final state. |
| `report_findings` | finding | SEC | M14 | FR-REPORT-001, FR-REPORT-002 | AZ | Inclusion rows. The CHECK keeps unverified items out of the confirmed section (D-75). |
| `credential_refs` | integration | CONS | M25 | FR-CRED-001 | A | Could be a column on `ai_providers` and `tool_configs`. A shared table gives one place to audit rotation and deletion. |
| `ai_providers` | integration | MVP | M12 | FR-AI-001 | Z | Provider configuration with locality classification (D-50, D-81). |
| `program_provider_enablements` | integration | SEC | M12 | FR-AI-002 | AZ | Per-program human enablement with the disclosure shown (D-46). |
| `ai_transmissions` | integration | SEC | M12 | FR-AI-004 | A | Record of each transmission, digest and size only (security.md 5). |
| `ai_outputs` | integration | LATER | M23 | FR-AI-005 | - | AI notes and drafts, labeled by origin. |
| `tool_configs` | integration | SEC | M10 | FR-TOOL-001 | ZI | Pinned binary digest and verification state (D-43). |
| `passive_source_enablements` | integration | SEC | M11 | FR-RECON-001 | AZ | Per-program human enablement of a passive source with disclosure (D-44). |
| `integration_checks` | integration | CONS | M10 | FR-TOOL-002, FR-CRED-001 | - | Self-test results. Could be `system_alerts` plus the audit trail. Kept for the status endpoint. |
| `idempotency_keys` | platform | MVP | M16 | NFR-SEC-001 | C | Prevents duplicate creation on retry (API-08). |
| `recovery_state` | identity | SEC | M3 | FR-APPROVAL-002, FR-APPROVAL-003, FR-SCOPE-006 | ZC | Database side of the recovery generation (DB-28). The external marker is the restore detector. |

## What this means for the milestones

- **M1 creates no table.** It is a pure standard-library package (`.kiro/specs/mvp/plan.md`).
- **M3 creates only the slices 00001 to 00007** of `migration-plan.md`. The tables the approval claim, job accounting, audit and recovery need (classes SEC) cannot move. `plan_items` (first needed in M19) and `assessment_transitions` (M21) sit in slice 00005 and may move to a later slice if the foreign keys `jobs.plan_item_id` and `plan_items.ai_job_id` move with them. No table classified LATER or DEF is created in M3.
- **Deferred table.** `asset_relationships` (class DEF). Trigger: a relationship view or analysis is specified. Revisit at the start of M20. Only `observations.relationship_id` references it and would go with it.

## Possible over-modeling and duplication

| Candidate | What overlaps | Why it is kept | Safe to merge only if |
| --- | --- | --- | --- |
| `approval_events` with `audit_events` | Approval history is also an audit trail. | The approval history endpoint must not depend on audit retention, and approvals are the strongest authorization record. | The audit retention rule keeps approval events forever and the endpoint can read them. |
| `assessment_transitions` with `audit_events` | State history | The legal-pair CHECK (DB-11) needs a table to live on. | A different mechanism enforces legal assessment transitions. |
| `login_attempts` with `audit_events` | Failure counts are derivable from failed-login audit events. | The rate limiter should not read the audit table on every login. | An indexed audit query is fast enough and the limiter fails closed if the audit table is unavailable. |
| `lab_registration_ranges` with a `cidr[]` column | One list of ranges. | A per-range port set and a range index at E3. | Ports are always per registration, not per range. |
| `target_validation_targets` with a `jsonb` column | Per-target decision snapshot. | Rows stay queryable and carry foreign keys to the matching rule and lab. | Nothing queries a single target's history. |
| `credential_refs` with provider and tool columns | An opaque Secret Service key per owner. | One place to audit rotation and deletion. | Only providers and tools ever hold credentials and rotation is not audited separately. |
| `integration_checks` with `system_alerts` plus audit | Self-test results. | The status endpoint needs the latest result per subject. | The status endpoint reads the latest audit event per subject. |
| `program_provider_enablements` with `passive_source_enablements` | Same shape: program, subject, human, disclosure hash, grant. | They have different subjects (provider, source) and different foreign keys. | A single `program_enablements` table with a kind column keeps both foreign keys. |

Entities that must **not** be merged, because merging would weaken a property: `scope_versions` with `scope_rules` (scope history), `approvals` with `approval_uses` and `job_attempts` (approval lifecycle and execution accounting), `evidence_items` with `evidence_status` (evidence integrity), `finding_transitions` with `finding_verifications` (verification history), and `audit_events` with any domain table.

