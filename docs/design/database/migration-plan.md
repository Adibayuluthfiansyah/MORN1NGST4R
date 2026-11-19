# Migration plan

Status: design proposal. This is a specification. No migration file exists or is authorized by this task. Migrations are Goose files `services/core/migrations/NNNNN_name.sql`, sequential, forward only (D-80). A released migration is never edited. A correction is a new migration. **M3 update (2026-11-18):** the M3 slices are implemented in `services/core/migrations` (00001 to 00007, 00013 to 00015). The result, the rulings and the differences from this design are in `.kiro/specs/mvp/plan.md`, section "Milestone M3". **M6 update (2026-11-18):** slice 00009 is implemented, and a new slice 00017 holds its indexes, triggers and grants, because 00013 to 00015 were applied before 00009 existed and are never edited. 00009 depends on 00007 only: evidence holds no reference to the asset tables of 00008 (M20). The deviations are in `.kiro/specs/mvp/plan.md`, section "Milestone M6". **M7 update:** slices 00018 (job lifecycle) and 00019 (retention) are implemented, and the deviations are in section "Milestone M7".

## Strategy

- **Ordering.** Slices follow the milestone order of the plan: the M3 slices (00001 to 00007) create everything the claim, approval and audit gates need. Later slices are created at the start of the milestone that owns them.
- **Foreign key placement rule (DB-21).** A foreign key is created in the migration that creates the later of its two tables. A circular reference is created `DEFERRABLE INITIALLY DEFERRED` in the later migration. This is how the dependency order stays acyclic.
- **Indexes, triggers and grants are separate migrations** (00013 to 00015) so each can be reviewed alone. After the first release an index on a table with data is created `CONCURRENTLY` in its own non-transactional migration (`-- +goose NO TRANSACTION`).
- **Transactions.** Each migration runs in one transaction unless stated. PostgreSQL DDL is transactional, so a failed migration leaves no partial schema.
- **Locking.** Goose records the version table and serializes concurrent runs with an advisory lock on the migration connection. Only the migration role runs migrations, and only while the server and worker are stopped (the Makefile target stops them first, DB-22).
- **Rollback.** Forward only. The "rollback consideration" column states what a manual recovery would need. Production recovery is a restore from backup (see `data-lifecycle.md`).
- **Seed data.** Migrations insert no rows. Default policy values are written by the application at first start through the policy use case so they are audited (DB-20).

## Ordered migrations

| ID | Name | Milestone | Depends on | Transactional | Data-loss risk if reverted |
| --- | --- | --- | --- | --- | --- |
| 00001 | `schema_and_roles` | M3 | none | yes | none |
| 00002 | `identity_and_access` | M3 (behavior in M4) | 00001 | yes | Account data |
| 00003 | `policy` | M3 (behavior in M16) | 00002 | yes | Policy history |
| 00004 | `programs_and_scope` | M3 (behavior in M16 to M18) | 00002, 00003 | yes | Program and scope history |
| 00005 | `assessments_and_plans` | M3 (behavior in M21) | 00004 | yes | Assessment history |
| 00006 | `authorization_and_execution` | M3 (behavior in M2, M7, M19) | 00005 | yes | Approval and job history |
| 00007 | `audit` | M3 (behavior in M5) | 00006 | yes | Audit history |
| 00008 | `asset_intelligence` | M20 | 00007 | yes | Inventory |
| 00009 | `evidence` | M6 | 00007 | yes | Evidence metadata. Files are untouched. |
| 00010 | `findings_and_reports` | M22 and M14 | 00009 | yes | Finding and verification history |
| 00011 | `ai_and_integrations` | M10, M12, M25 | 00010 | yes | Configuration and transmission records |
| 00012 | `platform` | M16 | 00002 | yes | None of value |
| 00013 | `operational_indexes` | M3 and later slices | the table slices | yes | none |
| 00014 | `integrity_triggers` | M3 and later slices | 00013 | yes | none |
| 00015 | `grants` | M3 and later slices | 00014 | yes | none |
| 00016 | `audit_sink` | M5 | 00015 | yes | none |
| 00017 | `evidence_integrity` | M6 | 00009, 00015 | yes | none |
| 00018 | `job_lifecycle` | M7 | 00006, 00015, 00016 | yes | none |
| 00019 | `retention` | M7 | 00002, 00017, 00018 | yes | none |

## Detail per migration

### 00001 `schema_and_roles`

- Purpose: Create the schema, the four login roles, helper functions (immutable-row, append-only) and the extension check.
- Milestone: M3
- Tables created: none
- Other objects: schema `morningstar`; roles `ms_migrator`, `ms_api`, `ms_worker`, `ms_retention` (NOLOGIN groups plus peer-mapped login roles, DB-06); functions `forbid_update_delete()`, `forbid_truncate()`; extensions: none required. `btree_gist` only if the lab range index needs it (verified at M3)
- Depends on: none
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop schema (empty). No data.

### 00002 `identity_and_access`

- Purpose: Account, credential hashes, recovery codes, bootstrap secrets, sessions, re-authentication grants, login attempts, and the database side of the recovery generation.
- Milestone: M3 (behavior in M4)
- Tables created: `users`, `user_credentials`, `recovery_codes`, `bootstrap_secrets`, `sessions`, `reauth_grants`, `login_attempts`, `recovery_state`
- Depends on: 00001
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop tables in reverse order. Loses the account.

### 00003 `policy`

- Purpose: Versioned policy settings.
- Milestone: M3 (behavior in M16)
- Tables created: `policy_settings`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `policy_settings(changed_by)` -> `users`; `policy_settings(reauth_grant_id)` -> `reauth_grants`
- Other objects: seed rows for the default policy values are inserted by the application at first start, not by the migration (DB-20)
- Depends on: 00002
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop table.

### 00004 `programs_and_scope`

- Purpose: Programs, rule versions, scope versions and rules, refused exclusions, targets, labs, validation records.
- Milestone: M3 (behavior in M16 to M18)
- Tables created: `programs`, `program_rules_versions`, `scope_versions`, `scope_rules`, `refused_exclusions`, `targets`, `lab_registrations`, `lab_registration_ranges`, `target_validations`, `target_validation_targets`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `lab_registrations(reauth_grant_id)` -> `reauth_grants`; `lab_registrations(registered_by)` -> `users`; `lab_registrations(revoked_by)` -> `users`; `program_rules_versions(recorded_by)` -> `users`; `programs(owner_user_id)` -> `users`; `refused_exclusions(acknowledged_by)` -> `users`; `refused_exclusions(reauth_grant_id)` -> `reauth_grants`; `scope_versions(created_by)` -> `users`; `scope_versions(reauth_grant_id)` -> `reauth_grants`; `target_validations(attested_by)` -> `users`; `targets(created_by)` -> `users`
- Depends on: 00002, 00003
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop tables in reverse order.

### 00005 `assessments_and_plans`

- Purpose: Assessments, target sets, state history, plan items.
- Milestone: M3 (behavior in M21)
- Tables created: `assessments`, `assessment_targets`, `assessment_transitions`, `plan_items`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `assessment_targets(added_by)` -> `users`; `assessment_targets(program_id)` -> `programs`; `assessment_targets(program_id, target_id)` -> `targets`; `assessment_targets(target_id)` -> `targets`; `assessment_transitions(actor_user_id)` -> `users`; `assessments(created_by)` -> `users`; `assessments(current_validation_id)` -> `target_validations`; `assessments(program_id)` -> `programs`; `assessments(program_id, current_validation_id)` -> `target_validations`; `lab_registrations(assessment_id)` -> `assessments`; `lab_registrations(program_id, assessment_id)` -> `assessments`; `plan_items(decided_by)` -> `users`; `plan_items(program_id)` -> `programs`; `plan_items(program_id, target_id)` -> `targets`; `plan_items(target_id)` -> `targets`; `target_validations(assessment_id)` -> `assessments`; `target_validations(program_id, assessment_id)` -> `assessments`
- Other objects: plan_items.operation_id and plan_items.ai_job_id foreign keys are created in 00006 (later tables)
- Depends on: 00004
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop tables in reverse order.

### 00006 `authorization_and_execution`

- Purpose: Operations, policy decisions, approvals and uses, jobs, attempts, job history.
- Milestone: M3 (behavior in M2, M7, M19)
- Tables created: `operations`, `policy_decisions`, `approvals`, `approval_events`, `approval_uses`, `jobs`, `job_attempts`, `job_transitions`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `approval_events(actor_user_id)` -> `users`; `approvals(assessment_id)` -> `assessments`; `approvals(decided_by)` -> `users`; `approvals(program_id)` -> `programs`; `approvals(program_id, assessment_id)` -> `assessments`; `approvals(program_id, scope_version_id)` -> `scope_versions`; `approvals(program_id, validation_id)` -> `target_validations`; `approvals(reauth_grant_id)` -> `reauth_grants`; `approvals(requested_by_user_id)` -> `users`; `approvals(revoked_by)` -> `users`; `approvals(scope_version_id)` -> `scope_versions`; `approvals(validation_id)` -> `target_validations`; `job_transitions(actor_user_id)` -> `users`; `jobs(assessment_id)` -> `assessments`; `jobs(created_by)` -> `users`; `jobs(plan_item_id)` -> `plan_items`; `jobs(program_id)` -> `programs`; `jobs(program_id, assessment_id)` -> `assessments`; `jobs(program_id, scope_version_id)` -> `scope_versions`; `jobs(scope_version_id)` -> `scope_versions`; `jobs(validation_id)` -> `target_validations`; `operations(assessment_id)` -> `assessments`; `operations(program_id)` -> `programs`; `operations(program_id, assessment_id)` -> `assessments`; `plan_items(ai_job_id)` -> `jobs`; `plan_items(assessment_id, operation_id)` -> `operations`; `plan_items(operation_id)` -> `operations`; `policy_decisions(assessment_id)` -> `assessments`; `policy_decisions(lab_registration_id)` -> `lab_registrations`; `policy_decisions(program_id)` -> `programs`; `policy_decisions(program_id, assessment_id)` -> `assessments`; `policy_decisions(program_id, scope_version_id)` -> `scope_versions`; `policy_decisions(scope_version_id)` -> `scope_versions`; `policy_decisions(validation_id)` -> `target_validations`
- Other objects: deferred foreign keys: assessments.current_validation_id, plan_items.operation_id, plan_items.ai_job_id, jobs.approval_use_id
- Depends on: 00005
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop tables in reverse order, then the deferred foreign keys.

### 00007 `audit`

- Purpose: Append-only audit events and system alerts.
- Milestone: M3 (behavior in M5)
- Tables created: `audit_events`, `system_alerts`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `audit_events(actor_user_id)` -> `users`; `audit_events(approval_id)` -> `approvals`; `audit_events(assessment_id)` -> `assessments`; `audit_events(decision_id)` -> `policy_decisions`; `audit_events(job_id)` -> `jobs`; `audit_events(program_id)` -> `programs`; `audit_events(scope_version_id)` -> `scope_versions`; `system_alerts(cleared_by)` -> `users`
- Depends on: 00006
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop tables. Loses the audit trail, so a rollback is allowed only before any data exists.

### 00008 `asset_intelligence`

- Purpose: Assets, services, endpoints, parameters, observations, relationships.
- Milestone: M20
- Tables created: `assets`, `services`, `endpoints`, `endpoint_parameters`, `observations`, `asset_relationships`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `asset_relationships(program_id)` -> `programs`; `assets(program_id)` -> `programs`; `assets(program_id, scope_version_id)` -> `scope_versions`; `assets(scope_version_id)` -> `scope_versions`; `endpoint_parameters(program_id)` -> `programs`; `endpoints(program_id)` -> `programs`; `observations(attempt_id)` -> `job_attempts`; `observations(job_id)` -> `jobs`; `observations(program_id)` -> `programs`; `observations(scope_version_id)` -> `scope_versions`; `services(program_id)` -> `programs`; `targets(program_id, promoted_from_asset_id)` -> `assets`; `targets(promoted_from_asset_id)` -> `assets`
- Other objects: deferred foreign keys: targets.promoted_from_asset_id, observations.evidence_id (added in 00009)
- Depends on: 00007
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop tables in reverse order.

### 00009 `evidence`

- Purpose: Evidence metadata, integrity checks and status.
- Milestone: M6
- Tables created: `evidence_items`, `evidence_integrity_checks`, `evidence_status`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `evidence_integrity_checks(actor_user_id)` -> `users`; `evidence_items(assessment_id)` -> `assessments`; `evidence_items(attempt_id)` -> `job_attempts`; `evidence_items(decision_id)` -> `policy_decisions`; `evidence_items(job_id)` -> `jobs`; `evidence_items(program_id)` -> `programs`; `evidence_items(program_id, assessment_id)` -> `assessments`; `evidence_items(program_id, target_id)` -> `targets`; `evidence_items(target_id)` -> `targets`; `evidence_status(purged_by)` -> `users`; `observations(evidence_id)` -> `evidence_items`
- Depends on: 00007
- Note: the design listed 00008 as a dependency, but evidence holds no reference to the asset tables, so M6 builds this slice before M20. The deferred foreign key from `observations` to `evidence_items` moves to the slice of M20 (TD-M6-04). Indexes, triggers and grants are in 00017.
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop tables in reverse order.

### 00010 `findings_and_reports`

- Purpose: Findings, transitions, links, validation, verification, reports.
- Milestone: M22 and M14
- Tables created: `findings`, `finding_transitions`, `finding_evidence`, `validation_records`, `finding_verifications`, `finding_verification_evidence`, `reports`, `report_findings`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `finding_evidence(evidence_id)` -> `evidence_items`; `finding_evidence(linked_by_user_id)` -> `users`; `finding_evidence(program_id)` -> `programs`; `finding_evidence(program_id, evidence_id)` -> `evidence_items`; `finding_transitions(actor_user_id)` -> `users`; `finding_verification_evidence(evidence_id)` -> `evidence_items`; `finding_verification_evidence(integrity_check_id)` -> `evidence_integrity_checks`; `finding_verifications(reauth_grant_id)` -> `reauth_grants`; `finding_verifications(verified_by)` -> `users`; `findings(assessment_id)` -> `assessments`; `findings(asset_id)` -> `assets`; `findings(created_by_user_id)` -> `users`; `findings(endpoint_id)` -> `endpoints`; `findings(program_id)` -> `programs`; `findings(program_id, assessment_id)` -> `assessments`; `findings(program_id, asset_id)` -> `assets`; `findings(program_id, endpoint_id)` -> `endpoints`; `report_findings(included_by)` -> `users`; `report_findings(program_id)` -> `programs`; `reports(assessment_id)` -> `assessments`; `reports(created_by)` -> `users`; `reports(finalized_by)` -> `users`; `reports(program_id)` -> `programs`; `reports(program_id, assessment_id)` -> `assessments`; `validation_records(recorded_by_user_id)` -> `users`; `validation_records(replay_job_id)` -> `jobs`
- Other objects: view `finding_report_status`; deferred foreign key: findings.verification_id
- Depends on: 00009
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop view then tables in reverse order.

### 00011 `ai_and_integrations`

- Purpose: Credential references, providers, enablements, transmissions, AI outputs, tools, passive sources, integration checks.
- Milestone: M10, M12, M25
- Tables created: `credential_refs`, `ai_providers`, `program_provider_enablements`, `ai_transmissions`, `ai_outputs`, `tool_configs`, `passive_source_enablements`, `integration_checks`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `ai_outputs(assessment_id)` -> `assessments`; `ai_outputs(job_id)` -> `jobs`; `ai_outputs(program_id)` -> `programs`; `ai_outputs(program_id, assessment_id)` -> `assessments`; `ai_providers(created_by)` -> `users`; `ai_transmissions(audit_event_id)` -> `audit_events`; `ai_transmissions(job_id)` -> `jobs`; `ai_transmissions(program_id)` -> `programs`; `credential_refs(created_by)` -> `users`; `passive_source_enablements(enabled_by)` -> `users`; `passive_source_enablements(program_id)` -> `programs`; `passive_source_enablements(reauth_grant_id)` -> `reauth_grants`; `program_provider_enablements(enabled_by)` -> `users`; `program_provider_enablements(program_id)` -> `programs`; `program_provider_enablements(reauth_grant_id)` -> `reauth_grants`; `tool_configs(reauth_grant_id)` -> `reauth_grants`; `tool_configs(registered_by)` -> `users`
- Depends on: 00010
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop tables in reverse order. Secrets are not touched because they are not stored here.

### 00012 `platform`

- Purpose: Idempotency keys.
- Milestone: M16
- Tables created: `idempotency_keys`
- Cross-migration foreign keys created here (the referenced or referencing table is created here, DB-21): `idempotency_keys(user_id)` -> `users`
- Depends on: 00002
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop table.

### 00013 `operational_indexes`

- Purpose: Every non-key index and partial unique index listed in constraints-and-indexes.md, grouped by the slice that owns the table.
- Milestone: M3 and later slices
- Tables created: none
- Other objects: all indexes of the tables created so far
- Depends on: the table slices
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop indexes.

### 00014 `integrity_triggers`

- Purpose: Immutability and append-only triggers, deferred state-parity triggers (finding status, approval counter, job approval use), single-account guard.
- Milestone: M3 and later slices
- Tables created: none
- Other objects: triggers for every table whose kind is immutable, append-only or immutable-except-X; deferred constraint triggers described in constraints-and-indexes.md
- Depends on: 00013
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Drop triggers.

### 00015 `grants`

- Purpose: Least-privilege grants per role (P-13, DB-06).
- Milestone: M3 and later slices
- Tables created: none
- Other objects: GRANT/REVOKE statements per table and role (see the grants matrix below)
- Depends on: 00014
- Forward behavior: creates the objects above inside one transaction. Fails closed on an existing object (no `IF NOT EXISTS`, DB-22).
- Rollback consideration: Revoke grants (then nothing works).

### 00016 `audit_sink`

- Purpose: The anonymous actor kind and one open audit alert (M5, TD-M4-03, T-52).
- Milestone: M5
- Tables created: none
- Other objects: `audit_events_ck_c2` replaced by a check that also accepts `anonymous`. The unique partial index `system_alerts_one_open_audit_alert`.
- Depends on: 00015
- Forward behavior: runs in one transaction. Rows written before it keep their actor kind, because audit rows are append-only.
- Rollback consideration: none. Migrations are forward only.

### 00017 `evidence_integrity`

- Purpose: The indexes, triggers and grants of the evidence slice (M6, D-77, P-37, DB-11, DB-17).
- Milestone: M6
- Tables created: none
- Other objects: five indexes (`evidence_assessment_idx`, `evidence_job_idx`, `evidence_sha_idx`, `evidence_checks_idx`, `evidence_status_state_idx`). Items and checks are append-only (`forbid_update_delete`, `forbid_truncate`). `evidence_status` accepts an update of the integrity and retention columns only (`forbid_update_except`), cannot be deleted or truncated, and the function `evidence_status_guard` keeps a compromised item compromised (the time of the first mark stays) and makes a purge final. Grants: `ms_api` and `ms_worker` SELECT and INSERT on the three tables and UPDATE of `integrity_state`, `last_check_id`, `compromised_at` and `updated_at` on the status. `ms_retention` SELECT and UPDATE of the retention columns.
- Depends on: 00009, 00015
- Forward behavior: runs in one transaction.
- Rollback consideration: none. Migrations are forward only.

### 00018 `job_lifecycle`

- Purpose: The database rules of the job state machine and of its attempts (M7, TD-M3-01, TD-M3-02, DB-11, DB-12, D-49, D-55).
- Milestone: M7
- Tables created: none
- Other objects: the trigger `jobs_state_guard` (a job starts `queued` or `awaiting_approval` with no claim and no attempt, only the legal state pairs of the design are accepted, `claim_epoch` and `execution_attempts` only grow). The deferred constraint trigger `jobs_transition_parity` (a change of `jobs.state` needs a `job_transitions` row from the same transaction). The trigger `job_attempts_guard` (an attempt is inserted only for a running job with control `none` by the claim that holds it, the attempt number equals the counter, attempts stay below `max_execution_attempts` and an intrusive job has one, and the attempt names the approval use of its job). `consume_approval_use` is replaced: it locks the job and needs state `queued`, control `none` and the next epoch. `check_approval_launch` is dropped and created again with a trailing `p_claim_epoch`, and it needs the job running under that epoch with control `none`. EXECUTE stays with `ms_worker` only.
- Depends on: 00006, 00015, 00016
- Forward behavior: runs in one transaction. Existing job rows are not changed.
- Rollback consideration: none. Migrations are forward only.

### 00019 `retention`

- Purpose: The audited purge of security-operational rows (M7, TD-M3-03, TD-M4-02, RC-B of `data-lifecycle.md`).
- Milestone: M7
- Tables created: none
- Other objects: the trigger `login_attempts_guard` replaces `login_attempts_no_update_delete`: update and truncate stay refused, and a delete is allowed only while the transaction-local setting `morningstar.retention_path` names the purge function. The SECURITY DEFINER functions `retention_purge_sessions(timestamptz)` and `retention_purge_login_attempts(timestamptz)`: each refuses a cutoff younger than 30 days, and the first never deletes a session or a re-authentication grant that an approval, a policy setting, a scope version, a lab registration or a refused exclusion refers to. EXECUTE is granted to `ms_retention` only. Grants: `ms_retention` INSERT on `audit_events`, so that the purge and its event commit together, and `ms_worker` SELECT on the column `users.id` only, because the worker runs the same startup recovery check as the server and that check asks whether an account exists. Audit rows are not purged: no retention number has been approved for them, and the guard of `audit_events` still refuses every update and delete (TD-M3-03, audit part).
- Depends on: 00002, 00017, 00018
- Forward behavior: runs in one transaction.
- Rollback consideration: none. Migrations are forward only.

## Grants matrix (migration 00015)

Roles: `ms_migrator` owns the schema and runs migrations. `ms_api` serves HTTP. `ms_worker` runs jobs. `ms_retention` purges under the retention policy. Peer authentication maps the one OS user to all four login roles through `pg_ident.conf` (D-67), so the grants limit mistakes and SQL defects, not a compromised same-user process (D-78).

| Table class | ms_api | ms_worker | ms_retention |
| --- | --- | --- | --- |
| Policy, scope, program, target, lab, validation tables | SELECT, INSERT, UPDATE on mutable columns only | SELECT only | none |
| approvals, approval_events | SELECT, INSERT, UPDATE (status, decision columns) | SELECT, UPDATE of uses_consumed only through the claim function | none |
| approval_uses, job_attempts, job_transitions, policy_decisions | SELECT | SELECT, INSERT | none |
| jobs | SELECT, INSERT, UPDATE (control_state, state for cancellation of queued jobs) | SELECT, UPDATE (lease, state, attempts) | none |
| audit_events | SELECT, INSERT | SELECT, INSERT | SELECT, INSERT (the event of its own purge, 00019). No role deletes audit rows: no number is approved (TD-M3-03) |
| evidence_items | SELECT, INSERT | SELECT, INSERT | SELECT |
| evidence_status, evidence_integrity_checks | SELECT, INSERT, UPDATE (status) | SELECT, INSERT, UPDATE (status) | SELECT, UPDATE (retention_state, purge columns) |
| findings and finding history | SELECT, INSERT, UPDATE (status, verification_id) | INSERT of automated transitions and candidates only | none |
| assets and observations | SELECT | SELECT, INSERT, UPDATE (projection columns) | none |
| Identity tables | SELECT, INSERT, UPDATE | none (`users.id` column SELECT only, for the recovery check, 00019) | none (sessions, grants and login attempts are purged only through the two functions of 00019) |

The worker role has no INSERT or UPDATE on scope, policy, approval decision columns, finding verification or user tables (P-32). The exact column lists are fixed in the M3 grant review.

## Environment notes

- Baseline: PostgreSQL 18 (D-67). The design uses only features available since PostgreSQL 14 (partial indexes, `num_nonnulls`, `cidr` operators, multirange types are not used), so PostgreSQL 17 still works.
- Database name `morningstar`, schema `morningstar`. Tests use a separate throwaway cluster and database created by the test helper (D-80), never the user database.
- Connections use the unix socket in a 0700 directory with peer authentication. No DSN, no password (D-67).
