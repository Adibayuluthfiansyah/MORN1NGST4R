# Constraints and indexes

Status: design proposal. These are specifications, not executable SQL. Names follow `<table>_<kind>_<detail>`: `_pk`, `_fk_<column>`, `_uq_<detail>`, `_ck_<detail>`, `_idx`.

## Rules that apply to every table

- Primary key `id uuid` unless the table is a pure link table with a composite key.
- Every foreign key is `ON DELETE RESTRICT ON UPDATE RESTRICT` (DB-05). A circular reference is a nullable column set by UPDATE and its foreign key is created in the migration of the later table (DB-21). The three that a CHECK ties to a state (`assessments.current_validation_id`, `jobs.approval_use_id`, `findings.verification_id`) are `DEFERRABLE INITIALLY DEFERRED`.
- Program-owned tables carry `program_id`, and every reference between two program-owned tables is a composite foreign key `(program_id, x_id) -> parent (program_id, id)` so a row can never point at another program's row (DB-09).
- Each state column has a named CHECK listing its values (DB-02). Transition tables carry the legal pairs as CHECKs (DB-11).
- Immutable and append-only tables have a `BEFORE UPDATE OR DELETE` trigger that raises, and the runtime roles have no UPDATE or DELETE grant (DB-17).
- Query patterns for every index are stated. No speculative index is listed (DB-19).

## Concurrency-sensitive constraints

| ID | Property | Mechanism | Table |
| --- | --- | --- | --- |
| CC-01 | One use per approval and job | `UNIQUE (approval_id, job_id)` | approval_uses |
| CC-02 | Use count never exceeds the bound | Conditional `UPDATE approvals SET uses_consumed = uses_consumed + 1 WHERE id = $1 AND status = 'approved' AND expires_at > now() AND uses_consumed < max_uses AND revoked_at IS NULL`, zero rows means refusal. `CHECK (uses_consumed BETWEEN 0 AND max_uses)` backs it. | approvals |
| CC-03 | Two workers cannot claim one job | `SELECT ... FOR UPDATE SKIP LOCKED` in the claim transaction, plus `claim_epoch` fencing on every later write | jobs |
| CC-04 | A retry waits for confirmed termination, and a job has one live attempt | `UNIQUE (job_id) WHERE termination_confirmed_at IS NULL` | job_attempts |
| CC-05 | Launch count bound | `UPDATE jobs SET execution_attempts = execution_attempts + 1 WHERE id = $1 AND claim_epoch = $2 AND execution_attempts < max_execution_attempts` in the same transaction as the attempt insert. `CHECK (execution_attempts <= max_execution_attempts)`. | jobs, job_attempts |
| CC-06 | An intrusive job launches once | `CHECK (risk_class <> 'intrusive' OR max_execution_attempts = 1)` and `approvals` CHECK on intrusive `max_uses = 1` | jobs, approvals |
| CC-07 | One current scope version per program | `UNIQUE (program_id) WHERE superseded_at IS NULL`, publication locks the current row `FOR UPDATE` | scope_versions |
| CC-08 | One current validation per assessment | `UNIQUE (assessment_id) WHERE status = 'current'` | target_validations |
| CC-09 | One account | `UNIQUE ((true)) WHERE status = 'active'` and bootstrap locks | users |
| CC-10 | A finding becomes verified only by a human with a validation record | CHECKs on finding_transitions and findings, `finding_verifications.verified_by NOT NULL REFERENCES users` | finding_transitions, findings, finding_verifications |
| CC-11 | Idempotent request replay | `UNIQUE (user_id, key_hash, method, route)` | idempotency_keys |
| CC-12 | Duplicate findings merge | `UNIQUE (assessment_id, finding_key)` | findings |
| CC-13 | Idempotent result persistence per attempt | UNIQUE on observation identity per attempt | observations |
| CC-14 | A re-authentication grant is used once | Conditional `UPDATE reauth_grants SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL AND expires_at > now() AND session_id = $2 AND purpose = $3 AND subject_hash = $4` in the guarded action's transaction | reauth_grants |
| CC-15 | Claims and launches happen only under the current recovery generation | The claim and launch transactions join `recovery_state` and require `state = 'normal'` and the generation that the process read from the external marker (DB-28) | jobs, recovery_state |


## Domain: identity

### `users`

- Primary key: `(id)`
- `c1`: CHECK status IN (active, disabled)
- `c2`: UNIQUE (username)
- `c3`: CHECK username = lower(username) AND length(username) BETWEEN 3 AND 64
- `c4`: Single account (DB-07): CREATE UNIQUE INDEX users_single_active ON users ((true)) WHERE status = 'active'

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `users_single_active` | `UNIQUE ((true)) WHERE status = 'active'` | Enforces D-60 at the database |

### `user_credentials`

- Primary key: `(id)`
- Foreign key `user_credentials_fk_user_id`: `(user_id)` -> `users (id)`, RESTRICT
- `c1`: CHECK algorithm IN (argon2id)
- `c2`: CHECK (is_current AND superseded_at IS NULL) OR (NOT is_current AND superseded_at IS NOT NULL)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `user_credentials_current_uq` | `UNIQUE (user_id) WHERE is_current` | One current hash per user |

### `recovery_codes`

- Primary key: `(id)`
- Foreign key `recovery_codes_fk_user_id`: `(user_id)` -> `users (id)`, RESTRICT
- `c1`: CHECK NOT (used_at IS NOT NULL AND revoked_at IS NOT NULL)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `recovery_codes_active_uq` | `UNIQUE (user_id) WHERE used_at IS NULL AND revoked_at IS NULL` | One usable code per user |

### `bootstrap_secrets`

- Primary key: `(id)`
- Foreign key `bootstrap_secrets_fk_consumed_user_id`: `(consumed_user_id)` -> `users (id)`, RESTRICT, optional
- `c1`: CHECK (consumed_at IS NULL) = (consumed_user_id IS NULL)
- `c2`: CHECK NOT (consumed_at IS NOT NULL AND superseded_at IS NOT NULL)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `bootstrap_secrets_active_uq` | `UNIQUE ((true)) WHERE consumed_at IS NULL AND superseded_at IS NULL` | One live secret at a time |

### `sessions`

- Primary key: `(id)`
- Foreign key `sessions_fk_user_id`: `(user_id)` -> `users (id)`, RESTRICT
- `c1`: UNIQUE (token_hash)
- `c2`: CHECK length(token_hash) = 32 AND length(csrf_token_hash) = 32
- `c3`: CHECK absolute_expires_at > created_at
- `c4`: CHECK (revoked_at IS NULL) = (revoked_reason IS NULL)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `sessions_user_active_idx` | `(user_id) WHERE revoked_at IS NULL` | List and revoke active sessions |
| `sessions_expiry_idx` | `(absolute_expires_at)` | Cleanup of expired sessions |

### `reauth_grants`

- Primary key: `(id)`
- Foreign key `reauth_grants_fk_user_id`: `(user_id)` -> `users (id)`, RESTRICT
- Foreign key `reauth_grants_fk_session_id`: `(session_id)` -> `sessions (id)`, RESTRICT
- `c1`: CHECK purpose IN (approval_intrusive, policy_widening, scope_widening, lab_registration, exclusion_acknowledgement, raw_evidence_capture, provider_enablement, tool_registration, passive_source_enablement, verification, password_change)
- `c2`: CHECK expires_at - created_at <= interval '120 seconds' -- single-use grants only, lifetime proposed in DQ-04
- `c3`: CHECK length(subject_hash) = 32
- `c4`: CHECK expires_at > created_at

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `reauth_grants_lookup_idx` | `(session_id, purpose, subject_hash) WHERE consumed_at IS NULL` | Find an unconsumed grant for the action |

### `login_attempts`

- Primary key: `(id)`
- Foreign key `login_attempts_fk_user_id`: `(user_id)` -> `users (id)`, RESTRICT, optional
- `c1`: CHECK kind IN (login, reauthenticate, recovery, bootstrap)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `login_attempts_window_idx` | `(kind, source_key, attempted_at DESC)` | Count failures in a rolling 15 minutes |

### `recovery_state`

- Primary key: `(singleton)`
- `c1`: CHECK singleton
- `c2`: CHECK state IN (normal, reconciling)
- `c3`: CHECK generation >= 1
- `c4`: CHECK last_reason IS NULL OR last_reason IN (marker_missing, marker_invalid, marker_older, database_older, instance_mismatch, reconcile_interrupted, first_run)


## Domain: policy

### `policy_settings`

- Primary key: `(id)`
- Foreign key `policy_settings_fk_changed_by`: `(changed_by)` -> `users (id)`, RESTRICT
- Foreign key `policy_settings_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT, optional
- `c1`: CHECK setting_key IN (cidr_minimum_v4, cidr_minimum_v6, approval_defaults, attempt_maximum, risk_overrides, audit_retry_bound, login_rate_limit, evidence_retention, verification_requires_reauth)
- `c2`: UNIQUE (setting_key, version_no)
- `c3`: CHECK NOT widening OR reauth_grant_id IS NOT NULL
- `c4`: CHECK CASE setting_key WHEN 'cidr_minimum_v4' THEN jsonb_typeof(value) = 'number' AND (value #>> '{}')::numeric BETWEEN 24 AND 32 WHEN 'cidr_minimum_v6' THEN jsonb_typeof(value) = 'number' AND (value #>> '{}')::numeric BETWEEN 48 AND 128 ELSE true END -- the floor of /24 and /48 (D-87), second line of defence behind `scope.NewPrefixMin`. CASE guarantees that the cast runs only for the two keys

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `policy_settings_current_uq` | `UNIQUE (setting_key) WHERE superseded_at IS NULL` | One current version per key; also the lookup of current values |


## Domain: scope

### `programs`

- Primary key: `(id)`
- Foreign key `programs_fk_owner_user_id`: `(owner_user_id)` -> `users (id)`, RESTRICT
- `c1`: CHECK status IN (active, archived)
- `c2`: CHECK (status = archived) = (archived_at IS NOT NULL)
- `c3`: UNIQUE (id) -- supports composite foreign keys
- `c4`: UNIQUE (owner_user_id, lower(name)) WHERE status = active

### `program_rules_versions`

- Primary key: `(id)`
- Foreign key `program_rules_versions_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `program_rules_versions_fk_recorded_by`: `(recorded_by)` -> `users (id)`, RESTRICT
- `c1`: UNIQUE (program_id, version_no)
- `c2`: UNIQUE (program_id, id)
- `c3`: CHECK reference_url IS NOT NULL OR rules_text IS NOT NULL
- `c4`: CHECK length(content_sha256) = 32

### `scope_versions`

- Primary key: `(id)`
- Foreign key `scope_versions_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `scope_versions_fk_rules_version_id`: `(rules_version_id)` -> `program_rules_versions (id)`, RESTRICT, optional
- Foreign key `scope_versions_fk_supersedes_version_id`: `(supersedes_version_id)` -> `scope_versions (id)`, RESTRICT, optional
- Foreign key `scope_versions_fk_created_by`: `(created_by)` -> `users (id)`, RESTRICT
- Foreign key `scope_versions_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT, optional
- Foreign key `scope_versions_fk_program_id_rules_version_id`: `(program_id, rules_version_id)` -> `program_rules_versions (program_id, id)`, RESTRICT, optional
- Foreign key `scope_versions_fk_program_id_supersedes_version_id`: `(program_id, supersedes_version_id)` -> `scope_versions (program_id, id)`, RESTRICT, optional
- `c1`: UNIQUE (program_id, version_no)
- `c2`: UNIQUE (program_id, id)
- `c3`: CHECK length(content_hash) = 32
- `c4`: CHECK NOT widening OR reauth_grant_id IS NOT NULL
- `c5`: CHECK (version_no = 1) = (supersedes_version_id IS NULL)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `scope_versions_current_uq` | `UNIQUE (program_id) WHERE superseded_at IS NULL` | One current version per program and the lookup of it |

### `scope_rules`

- Primary key: `(id)`
- Foreign key `scope_rules_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `scope_rules_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT
- Foreign key `scope_rules_fk_program_id_scope_version_id`: `(program_id, scope_version_id)` -> `scope_versions (program_id, id)`, RESTRICT
- `c1`: CHECK kind IN (domain, wildcard, ip, cidr)
- `c2`: CHECK NOT (kind = wildcard AND NOT exclude) -- wildcard inclusion refused (D-35, D-73)
- `c3`: UNIQUE (scope_version_id, kind, exclude, canonical_value, ports_canonical)
- `c5`: CHECK length(canonical_value) BETWEEN 1 AND 253 OR kind IN (ip, cidr)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `scope_rules_version_idx` | `(scope_version_id, exclude)` | Load a version, exclusions first |

### `refused_exclusions`

- Primary key: `(id)`
- Foreign key `refused_exclusions_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `refused_exclusions_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT
- Foreign key `refused_exclusions_fk_acknowledged_by`: `(acknowledged_by)` -> `users (id)`, RESTRICT, optional
- Foreign key `refused_exclusions_fk_replaced_by_rule_id`: `(replaced_by_rule_id)` -> `scope_rules (id)`, RESTRICT, optional
- Foreign key `refused_exclusions_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT, optional
- Foreign key `refused_exclusions_fk_program_id_scope_version_id`: `(program_id, scope_version_id)` -> `scope_versions (program_id, id)`, RESTRICT
- `c1`: CHECK status IN (open, acknowledged, replaced)
- `c2`: CHECK length(original_text) <= 1000
- `c3`: CHECK (status <> acknowledged) OR (acknowledged_by IS NOT NULL AND acknowledged_at IS NOT NULL AND acknowledged_target_set_hash IS NOT NULL AND reauth_grant_id IS NOT NULL)
- `c4`: CHECK (status <> replaced) OR replaced_by_rule_id IS NOT NULL

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `refused_exclusions_open_idx` | `(program_id) WHERE status = open` | Block readiness while any is open |

### `targets`

- Primary key: `(id)`
- Foreign key `targets_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `targets_fk_promoted_from_asset_id`: `(promoted_from_asset_id)` -> `assets (id)`, RESTRICT, optional
- Foreign key `targets_fk_created_by`: `(created_by)` -> `users (id)`, RESTRICT
- Foreign key `targets_fk_program_id_promoted_from_asset_id`: `(program_id, promoted_from_asset_id)` -> `assets (program_id, id)`, RESTRICT, optional
- `c1`: CHECK kind IN (hostname, ip, cidr)
- `c2`: CHECK origin IN (human, promoted_asset)
- `c3`: CHECK (origin = promoted_asset) = (promoted_from_asset_id IS NOT NULL)
- `c4`: CHECK status IN (registered, archived)
- `c5`: UNIQUE (program_id, kind, canonical_value)
- `c6`: UNIQUE (program_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `targets_program_idx` | `(program_id, status)` | List targets of a program |

### `lab_registrations`

- Primary key: `(id)`
- Foreign key `lab_registrations_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `lab_registrations_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `lab_registrations_fk_target_id`: `(target_id)` -> `targets (id)`, RESTRICT
- Foreign key `lab_registrations_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT
- Foreign key `lab_registrations_fk_registered_by`: `(registered_by)` -> `users (id)`, RESTRICT
- Foreign key `lab_registrations_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT
- Foreign key `lab_registrations_fk_revoked_by`: `(revoked_by)` -> `users (id)`, RESTRICT, optional
- Foreign key `lab_registrations_fk_supersedes_lab_id`: `(supersedes_lab_id)` -> `lab_registrations (id)`, RESTRICT, optional
- Foreign key `lab_registrations_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `lab_registrations_fk_program_id_target_id`: `(program_id, target_id)` -> `targets (program_id, id)`, RESTRICT
- Foreign key `lab_registrations_fk_program_id_scope_version_id`: `(program_id, scope_version_id)` -> `scope_versions (program_id, id)`, RESTRICT
- `c1`: CHECK kind IN (address, cidr, hostname)
- `c2`: CHECK status IN (active, expired, revoked)
- `c3`: CHECK expires_at > created_at
- `c4`: CHECK NOT ports_required OR ports_canonical IS NOT NULL
- `c5`: CHECK (status = revoked) = (revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND revoke_reason IS NOT NULL)
- `c9`: UNIQUE (program_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `lab_registrations_active_idx` | `(assessment_id) WHERE status = active` | Find live labs at E2 and E3 and at expiry sweeps |

### `lab_registration_ranges`

- Primary key: `(id)`
- Foreign key `lab_registration_ranges_fk_lab_registration_id`: `(lab_registration_id)` -> `lab_registrations (id)`, RESTRICT
- `c1`: UNIQUE (lab_registration_id, range_cidr)
- `c2`: CHECK NOT (range_cidr && ANY (ARRAY['169.254.0.0/16','fe80::/10','224.0.0.0/4','ff00::/8','0.0.0.0/8','::/128','255.255.255.255/32','240.0.0.0/4','168.63.129.16/32','100.100.100.200/32','fd00:ec2::254/128','fd20:ce::254/128']::cidr[])) -- backstop for D-47 and D-72, the application generated tables stay authoritative
- `c3`: CHECK (family(range_cidr) = 4 AND masklen(range_cidr) >= 24) OR (family(range_cidr) = 6 AND masklen(range_cidr) >= 48) OR masklen(range_cidr) = max_masklen -- /24 and /48 minimum (D-38, D-47); single addresses allowed

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `lab_ranges_lookup_idx` | `USING gist (range_cidr inet_ops)` | Containment lookup at E3 (needs the btree_gist or built-in inet_ops, verified at M3) |

### `target_validations`

- Primary key: `(id)`
- Foreign key `target_validations_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `target_validations_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `target_validations_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT
- Foreign key `target_validations_fk_rules_version_id`: `(rules_version_id)` -> `program_rules_versions (id)`, RESTRICT
- Foreign key `target_validations_fk_attested_by`: `(attested_by)` -> `users (id)`, RESTRICT
- Foreign key `target_validations_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `target_validations_fk_program_id_scope_version_id`: `(program_id, scope_version_id)` -> `scope_versions (program_id, id)`, RESTRICT
- Foreign key `target_validations_fk_program_id_rules_version_id`: `(program_id, rules_version_id)` -> `program_rules_versions (program_id, id)`, RESTRICT
- `c1`: CHECK attestation_kind IN (standard, lab)
- `c2`: CHECK status IN (current, stale, expired, invalidated)
- `c3`: CHECK (status = current) = (invalidated_at IS NULL)
- `c4`: CHECK (status = current) OR invalidation_reason IS NOT NULL
- `c5`: CHECK invalidation_reason IS NULL OR invalidation_reason IN (scope_change, rules_change, lab_change, target_set_change, expiry, manual, restore_recovery)
- `c6`: CHECK expires_at > attested_at
- `c10`: UNIQUE (program_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `target_validations_current_uq` | `UNIQUE (assessment_id) WHERE status = current` | At most one current record per assessment, and its lookup |

### `target_validation_targets`

- Primary key: `(id)`
- Foreign key `target_validation_targets_fk_validation_id`: `(validation_id)` -> `target_validations (id)`, RESTRICT
- Foreign key `target_validation_targets_fk_target_id`: `(target_id)` -> `targets (id)`, RESTRICT
- Foreign key `target_validation_targets_fk_matched_scope_rule_id`: `(matched_scope_rule_id)` -> `scope_rules (id)`, RESTRICT, optional
- Foreign key `target_validation_targets_fk_lab_registration_id`: `(lab_registration_id)` -> `lab_registrations (id)`, RESTRICT, optional
- `c1`: CHECK decision IN (allow, deny)
- `c2`: UNIQUE (validation_id, target_id)


## Domain: assessment

### `assessments`

- Primary key: `(id)`
- Foreign key `assessments_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `assessments_fk_current_validation_id`: `(current_validation_id)` -> `target_validations (id)`, RESTRICT, DEFERRABLE INITIALLY DEFERRED, optional
- Foreign key `assessments_fk_created_by`: `(created_by)` -> `users (id)`, RESTRICT
- Foreign key `assessments_fk_program_id_current_validation_id`: `(program_id, current_validation_id)` -> `target_validations (program_id, id)`, RESTRICT, DEFERRABLE INITIALLY DEFERRED, optional
- `c1`: CHECK status IN (draft, ready, running, paused, completed, cancelled)
- `c2`: CHECK control_state IN (none, stop_requested)
- `c3`: UNIQUE (program_id, id)
- `c4`: CHECK status IN (draft, cancelled) OR current_validation_id IS NOT NULL -- ready, running, paused and completed need a validation record (D-83). The FK to target_validations is DEFERRABLE INITIALLY DEFERRED because that table references assessments

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `assessments_program_idx` | `(program_id, status, created_at DESC)` | List by program and state |

### `assessment_targets`

- Primary key: `(assessment_id, target_id)`
- Foreign key `assessment_targets_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `assessment_targets_fk_target_id`: `(target_id)` -> `targets (id)`, RESTRICT
- Foreign key `assessment_targets_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `assessment_targets_fk_added_by`: `(added_by)` -> `users (id)`, RESTRICT
- Foreign key `assessment_targets_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `assessment_targets_fk_program_id_target_id`: `(program_id, target_id)` -> `targets (program_id, id)`, RESTRICT

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `assessment_targets_active_idx` | `(assessment_id) WHERE removed_at IS NULL` | Current target set |

### `assessment_transitions`

- Primary key: `(id)`
- Foreign key `assessment_transitions_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `assessment_transitions_fk_actor_user_id`: `(actor_user_id)` -> `users (id)`, RESTRICT
- `c1`: CHECK legal pairs: (NULL,draft), (draft,ready), (ready,draft), (ready,running), (running,paused), (paused,running), (running,completed), (paused,completed), (draft,cancelled), (ready,cancelled), (running,cancelled), (paused,cancelled)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `assessment_transitions_idx` | `(assessment_id, created_at)` | History view |

### `plan_items`

- Primary key: `(id)`
- Foreign key `plan_items_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `plan_items_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `plan_items_fk_target_id`: `(target_id)` -> `targets (id)`, RESTRICT
- Foreign key `plan_items_fk_operation_id`: `(operation_id)` -> `operations (id)`, RESTRICT
- Foreign key `plan_items_fk_ai_job_id`: `(ai_job_id)` -> `jobs (id)`, RESTRICT, optional
- Foreign key `plan_items_fk_decided_by`: `(decided_by)` -> `users (id)`, RESTRICT, optional
- Foreign key `plan_items_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `plan_items_fk_program_id_target_id`: `(program_id, target_id)` -> `targets (program_id, id)`, RESTRICT
- Foreign key `plan_items_fk_assessment_id_operation_id`: `(assessment_id, operation_id)` -> `operations (assessment_id, id)`, RESTRICT
- `c1`: CHECK origin IN (human, ai)
- `c2`: CHECK status IN (proposed, accepted, rejected, converted, superseded)
- `c3`: CHECK (origin = ai) = (ai_job_id IS NOT NULL)
- `c4`: CHECK (status IN (accepted, rejected)) = (decided_by IS NOT NULL)
- `c8`: UNIQUE (assessment_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `plan_items_assessment_idx` | `(assessment_id, status)` | List plan items |


## Domain: authorization

### `operations`

- Primary key: `(id)`
- Foreign key `operations_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `operations_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `operations_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- `c1`: CHECK risk_class IN (passive, low_impact, intrusive)
- `c2`: CHECK length(operation_hash) = 32 AND length(manifest_digest) = 32
- `c3`: UNIQUE (assessment_id, operation_hash)
- `c4`: UNIQUE (assessment_id, id)

### `policy_decisions`

- Primary key: `(id)`
- Foreign key `policy_decisions_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `policy_decisions_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `policy_decisions_fk_operation_id`: `(operation_id)` -> `operations (id)`, RESTRICT, optional
- Foreign key `policy_decisions_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT, optional
- Foreign key `policy_decisions_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT
- Foreign key `policy_decisions_fk_validation_id`: `(validation_id)` -> `target_validations (id)`, RESTRICT, optional
- Foreign key `policy_decisions_fk_approval_id`: `(approval_id)` -> `approvals (id)`, RESTRICT, optional
- Foreign key `policy_decisions_fk_lab_registration_id`: `(lab_registration_id)` -> `lab_registrations (id)`, RESTRICT, optional
- Foreign key `policy_decisions_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `policy_decisions_fk_program_id_scope_version_id`: `(program_id, scope_version_id)` -> `scope_versions (program_id, id)`, RESTRICT
- `c1`: CHECK point IN (E1, E2, E3, E4)
- `c2`: CHECK outcome IN (allow, deny, require_approval)
- `c5`: UNIQUE (program_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `policy_decisions_job_idx` | `(job_id, decided_at)` | Decision trail of a job |

### `approvals`

- Primary key: `(id)`
- Foreign key `approvals_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `approvals_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `approvals_fk_operation_id`: `(operation_id)` -> `operations (id)`, RESTRICT
- Foreign key `approvals_fk_requested_by_user_id`: `(requested_by_user_id)` -> `users (id)`, RESTRICT
- Foreign key `approvals_fk_decided_by`: `(decided_by)` -> `users (id)`, RESTRICT, optional
- Foreign key `approvals_fk_validation_id`: `(validation_id)` -> `target_validations (id)`, RESTRICT
- Foreign key `approvals_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT
- Foreign key `approvals_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT, optional
- Foreign key `approvals_fk_revoked_by`: `(revoked_by)` -> `users (id)`, RESTRICT, optional
- Foreign key `approvals_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `approvals_fk_assessment_id_operation_id`: `(assessment_id, operation_id)` -> `operations (assessment_id, id)`, RESTRICT
- Foreign key `approvals_fk_program_id_validation_id`: `(program_id, validation_id)` -> `target_validations (program_id, id)`, RESTRICT
- Foreign key `approvals_fk_program_id_scope_version_id`: `(program_id, scope_version_id)` -> `scope_versions (program_id, id)`, RESTRICT
- `c1`: CHECK status IN (pending, approved, rejected, revoked)
- `c2`: CHECK risk_class IN (low_impact, intrusive) -- passive operations need no approval row
- `c3`: CHECK uses_consumed BETWEEN 0 AND max_uses
- `c4`: CHECK max_uses >= 1 AND max_attempts BETWEEN 1 AND 3
- `c5`: CHECK (risk_class <> intrusive) OR (max_uses = 1 AND max_attempts = 1) -- D-45, D-49
- `c6`: CHECK (status = pending) = (decided_by IS NULL AND decided_at IS NULL)
- `c7`: CHECK (status <> approved) OR expires_at IS NOT NULL
- `c8`: CHECK (risk_class <> intrusive) OR status <> approved OR reauth_grant_id IS NOT NULL
- `c9`: CHECK (status = revoked) = (revoked_at IS NOT NULL)
- `c14`: UNIQUE (assessment_id, id)
- `c15`: UNIQUE (reauth_grant_id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `approvals_operation_idx` | `(operation_id, status)` | Find the live approval for an operation at E1 and E3 |
| `approvals_assessment_idx` | `(assessment_id, created_at DESC)` | List approvals |

### `approval_events`

- Primary key: `(id)`
- Foreign key `approval_events_fk_approval_id`: `(approval_id)` -> `approvals (id)`, RESTRICT
- Foreign key `approval_events_fk_actor_user_id`: `(actor_user_id)` -> `users (id)`, RESTRICT, optional
- Foreign key `approval_events_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT, optional
- `c1`: CHECK event_type IN (requested, approved, rejected, revoked, use_consumed, use_denied, expiry_observed)
- `c2`: CHECK actor_kind IN (user, worker, system)
- `c3`: CHECK event_type NOT IN (approved, rejected, revoked) OR (actor_kind = user AND actor_user_id IS NOT NULL) -- only humans decide

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `approval_events_idx` | `(approval_id, created_at)` | Approval history |

### `approval_uses`

- Primary key: `(id)`
- Foreign key `approval_uses_fk_approval_id`: `(approval_id)` -> `approvals (id)`, RESTRICT
- Foreign key `approval_uses_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT
- `c1`: UNIQUE (approval_id, job_id) -- the concurrency constraint that forbids a second use for one job (security.md 4)
- `c2`: Deferred constraint trigger: the approval counter uses_consumed equals the number of approval_uses rows (DB-13)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `approval_uses_job_idx` | `(job_id)` | Find the use of a job |


## Domain: execution

### `jobs`

- Primary key: `(id)`
- Foreign key `jobs_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `jobs_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `jobs_fk_operation_id`: `(operation_id)` -> `operations (id)`, RESTRICT, optional
- Foreign key `jobs_fk_plan_item_id`: `(plan_item_id)` -> `plan_items (id)`, RESTRICT, optional
- Foreign key `jobs_fk_approval_id`: `(approval_id)` -> `approvals (id)`, RESTRICT, optional
- Foreign key `jobs_fk_approval_use_id`: `(approval_use_id)` -> `approval_uses (id)`, RESTRICT, DEFERRABLE INITIALLY DEFERRED, optional
- Foreign key `jobs_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT
- Foreign key `jobs_fk_validation_id`: `(validation_id)` -> `target_validations (id)`, RESTRICT, optional
- Foreign key `jobs_fk_created_by`: `(created_by)` -> `users (id)`, RESTRICT, optional
- Foreign key `jobs_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `jobs_fk_assessment_id_operation_id`: `(assessment_id, operation_id)` -> `operations (assessment_id, id)`, RESTRICT, optional
- Foreign key `jobs_fk_assessment_id_approval_id`: `(assessment_id, approval_id)` -> `approvals (assessment_id, id)`, RESTRICT, optional
- Foreign key `jobs_fk_program_id_scope_version_id`: `(program_id, scope_version_id)` -> `scope_versions (program_id, id)`, RESTRICT
- `c1`: CHECK job_kind IN (tool_operation, ai_task)
- `c2`: CHECK risk_class IN (passive, low_impact, intrusive, ai)
- `c3`: CHECK state IN (queued, awaiting_approval, running, succeeded, failed, cancelled)
- `c4`: CHECK control_state IN (none, stop_requested, stop_confirmed, stop_failed)
- `c5`: CHECK (job_kind = tool_operation) = (operation_id IS NOT NULL)
- `c6`: CHECK risk_class IN (passive, ai) OR approval_id IS NOT NULL -- risky job needs an approval reference (P-36)
- `c7`: CHECK state NOT IN (running, succeeded, failed) OR risk_class IN (passive, ai) OR approval_use_id IS NOT NULL OR failure_class = policy_denial
- `c8`: CHECK execution_attempts BETWEEN 0 AND max_execution_attempts
- `c9`: CHECK max_execution_attempts BETWEEN 1 AND 3
- `c10`: CHECK risk_class <> intrusive OR max_execution_attempts = 1
- `c11`: CHECK state <> running OR (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
- `c12`: CHECK (state IN (succeeded, failed, cancelled)) = (finished_at IS NOT NULL)
- `c13`: jobs.approval_use_id equals the approval_uses row for (approval_id, id): deferred constraint trigger. the FK jobs.approval_use_id is DEFERRABLE INITIALLY DEFERRED because a CHECK ties it to the job state and approval_uses references jobs
- `c18`: UNIQUE (assessment_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `jobs_claim_idx` | `(priority DESC, available_at, id) WHERE state = queued AND control_state = none` | The claim query with FOR UPDATE SKIP LOCKED |
| `jobs_lease_idx` | `(lease_expires_at) WHERE state = running` | Reaper finds expired leases |
| `jobs_assessment_idx` | `(assessment_id, state, created_at DESC)` | List jobs of an assessment |
| `jobs_approval_idx` | `(approval_id) WHERE state IN (queued, awaiting_approval, running)` | Revocation finds dependent jobs (30-second objective) |
| `jobs_scope_idx` | `(scope_version_id) WHERE state IN (queued, awaiting_approval, running)` | Scope change finds dependent jobs |

### `job_attempts`

- Primary key: `(id)`
- Foreign key `job_attempts_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT
- Foreign key `job_attempts_fk_approval_use_id`: `(approval_use_id)` -> `approval_uses (id)`, RESTRICT, optional
- Foreign key `job_attempts_fk_decision_id`: `(decision_id)` -> `policy_decisions (id)`, RESTRICT
- `c1`: UNIQUE (job_id, attempt_no)
- `c2`: CHECK attempt_no BETWEEN 1 AND 3
- `c3`: CHECK state IN (launching, running, succeeded, failed, timed_out, killed, cancelled, lost)
- `c4`: CHECK state IN (launching, running, lost) OR termination_confirmed_at IS NOT NULL -- a finished attempt has a confirmed tree end
- `c5`: CHECK (state IN (launching, running)) = (ended_at IS NULL)
- `c6`: CHECK octet_length(tool_binary_sha256) = 32 OR tool_binary_sha256 IS NULL

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `job_attempts_unconfirmed_uq` | `UNIQUE (job_id) WHERE termination_confirmed_at IS NULL` | A retry cannot start until the previous tree is confirmed terminated, and a job has one live attempt (D-64 choice 12) |

### `job_transitions`

- Primary key: `(id)`
- Foreign key `job_transitions_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT
- Foreign key `job_transitions_fk_actor_user_id`: `(actor_user_id)` -> `users (id)`, RESTRICT, optional
- `c1`: CHECK actor_kind IN (user, worker, system)
- `c2`: CHECK legal pairs: (NULL,queued), (NULL,awaiting_approval), (awaiting_approval,queued), (queued,running), (running,succeeded), (running,failed), (running,queued) when retry allowed, (queued,failed), (queued,cancelled), (awaiting_approval,cancelled), (running,cancelled)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `job_transitions_idx` | `(job_id, created_at)` | Execution history |


## Domain: audit

### `audit_events`

- Primary key: `(id)`
- Foreign key `audit_events_fk_actor_user_id`: `(actor_user_id)` -> `users (id)`, RESTRICT, optional
- Foreign key `audit_events_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT, optional
- Foreign key `audit_events_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT, optional
- Foreign key `audit_events_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT, optional
- Foreign key `audit_events_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT, optional
- Foreign key `audit_events_fk_decision_id`: `(decision_id)` -> `policy_decisions (id)`, RESTRICT, optional
- Foreign key `audit_events_fk_approval_id`: `(approval_id)` -> `approvals (id)`, RESTRICT, optional
- `c1`: UNIQUE (seq)
- `c2`: CHECK actor_kind IN (user, worker, system, ai, anonymous) (anonymous added by migration 00016)
- `c3`: CHECK outcome IN (success, denied, failure)
- `c4`: CHECK (actor_kind = user) = (actor_user_id IS NOT NULL)
- `c5`: CHECK octet_length(detail::text) <= 8192
- `c6`: Triggers: BEFORE UPDATE, DELETE and TRUNCATE raise unless current_user is the retention role and the row is past retention (DB-17)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `audit_events_time_idx` | `(occurred_at DESC, seq DESC)` | Audit list and keyset pagination |
| `audit_events_entity_idx` | `(entity_type, entity_id, occurred_at)` | History of one entity |
| `audit_events_action_idx` | `(action, occurred_at DESC)` | Filter by action |
| `audit_events_program_idx` | `(program_id, occurred_at DESC) WHERE program_id IS NOT NULL` | Program view |

### `system_alerts`

- Primary key: `(id)`
- Foreign key `system_alerts_fk_cleared_by`: `(cleared_by)` -> `users (id)`, RESTRICT, optional
- `c1`: CHECK kind IN (audit_retry_exhausted, evidence_integrity, sandbox_unavailable, credential_store_unavailable, restore_recovered)
- `c2`: CHECK (cleared_at IS NULL) = (cleared_by IS NULL)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `system_alerts_open_idx` | `(kind) WHERE cleared_at IS NULL` | Open alerts block new operations when kind = audit_retry_exhausted |
| `system_alerts_one_open_audit_alert` | UNIQUE `(kind) WHERE kind = audit_retry_exhausted AND cleared_at IS NULL` | At most one open audit alert, so an outage cannot fill the table with alerts (migration 00016) |


## Domain: asset

### `assets`

- Primary key: `(id)`
- Foreign key `assets_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `assets_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT, optional
- Foreign key `assets_fk_program_id_scope_version_id`: `(program_id, scope_version_id)` -> `scope_versions (program_id, id)`, RESTRICT, optional
- `c1`: CHECK kind IN (domain, ip)
- `c2`: CHECK scope_decision IN (unevaluated, in_scope, out_of_scope, ambiguous)
- `c3`: CHECK (scope_decision = unevaluated) = (scope_version_id IS NULL)
- `c4`: CHECK first_origin IN (observed, ai_inferred, human_entered)
- `c5`: UNIQUE (program_id, kind, canonical_value)
- `c6`: UNIQUE (program_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `assets_program_idx` | `(program_id, kind, canonical_value)` | Inventory browsing |
| `assets_scope_idx` | `(program_id, scope_decision)` | Filter unevaluated assets |

### `services`

- Primary key: `(id)`
- Foreign key `services_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `services_fk_asset_id`: `(asset_id)` -> `assets (id)`, RESTRICT
- Foreign key `services_fk_program_id_asset_id`: `(program_id, asset_id)` -> `assets (program_id, id)`, RESTRICT
- `c1`: CHECK transport IN (tcp, udp)
- `c2`: CHECK port BETWEEN 1 AND 65535
- `c3`: UNIQUE (asset_id, transport, port)
- `c4`: UNIQUE (program_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `services_asset_idx` | `(asset_id)` | Services of an asset |

### `endpoints`

- Primary key: `(id)`
- Foreign key `endpoints_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `endpoints_fk_service_id`: `(service_id)` -> `services (id)`, RESTRICT
- Foreign key `endpoints_fk_program_id_service_id`: `(program_id, service_id)` -> `services (program_id, id)`, RESTRICT
- `c1`: CHECK length(path_canonical) <= 2048
- `c2`: UNIQUE (service_id, method, path_canonical)
- `c3`: UNIQUE (program_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `endpoints_service_idx` | `(service_id, path_canonical)` | Endpoints of a service |

### `endpoint_parameters`

- Primary key: `(id)`
- Foreign key `endpoint_parameters_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `endpoint_parameters_fk_endpoint_id`: `(endpoint_id)` -> `endpoints (id)`, RESTRICT
- Foreign key `endpoint_parameters_fk_program_id_endpoint_id`: `(program_id, endpoint_id)` -> `endpoints (program_id, id)`, RESTRICT
- `c1`: CHECK location IN (query, body, header, cookie, path)
- `c2`: CHECK length(name) <= 256
- `c3`: UNIQUE (endpoint_id, location, name)

### `observations`

- Primary key: `(id)`
- Foreign key `observations_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `observations_fk_asset_id`: `(asset_id)` -> `assets (id)`, RESTRICT, optional
- Foreign key `observations_fk_service_id`: `(service_id)` -> `services (id)`, RESTRICT, optional
- Foreign key `observations_fk_endpoint_id`: `(endpoint_id)` -> `endpoints (id)`, RESTRICT, optional
- Foreign key `observations_fk_relationship_id`: `(relationship_id)` -> `asset_relationships (id)`, RESTRICT, optional
- Foreign key `observations_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT, optional
- Foreign key `observations_fk_attempt_id`: `(attempt_id)` -> `job_attempts (id)`, RESTRICT, optional
- Foreign key `observations_fk_evidence_id`: `(evidence_id)` -> `evidence_items (id)`, RESTRICT, optional
- Foreign key `observations_fk_scope_version_id`: `(scope_version_id)` -> `scope_versions (id)`, RESTRICT, optional
- Foreign key `observations_fk_program_id_asset_id`: `(program_id, asset_id)` -> `assets (program_id, id)`, RESTRICT, optional
- Foreign key `observations_fk_program_id_service_id`: `(program_id, service_id)` -> `services (program_id, id)`, RESTRICT, optional
- Foreign key `observations_fk_program_id_endpoint_id`: `(program_id, endpoint_id)` -> `endpoints (program_id, id)`, RESTRICT, optional
- `c1`: CHECK subject_type IN (asset, service, endpoint, relationship)
- `c2`: CHECK origin IN (observed, ai_inferred, human_entered)
- `c3`: CHECK num_nonnulls(asset_id, service_id, endpoint_id, relationship_id) = 1
- `c4`: CHECK octet_length(payload::text) <= 16384
- `c5`: CHECK origin <> observed OR job_id IS NOT NULL
- `c9`: UNIQUE (attempt_id, subject_type, observation_kind, md5(payload::text), coalesce(asset_id, service_id, endpoint_id, relationship_id)) -- idempotent result persistence per attempt

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `observations_subject_idx` | `(asset_id, observed_at DESC) WHERE asset_id IS NOT NULL` | Provenance view of an asset |
| `observations_job_idx` | `(job_id)` | Results of a job |

### `asset_relationships`

- Primary key: `(id)`
- Foreign key `asset_relationships_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `asset_relationships_fk_from_asset_id`: `(from_asset_id)` -> `assets (id)`, RESTRICT
- Foreign key `asset_relationships_fk_to_asset_id`: `(to_asset_id)` -> `assets (id)`, RESTRICT
- Foreign key `asset_relationships_fk_program_id_from_asset_id`: `(program_id, from_asset_id)` -> `assets (program_id, id)`, RESTRICT
- Foreign key `asset_relationships_fk_program_id_to_asset_id`: `(program_id, to_asset_id)` -> `assets (program_id, id)`, RESTRICT
- `c1`: CHECK relation IN (resolves_to, cname_of, subdomain_of, hosts, redirects_to)
- `c2`: CHECK from_asset_id <> to_asset_id
- `c3`: UNIQUE (from_asset_id, to_asset_id, relation)
- `c4`: UNIQUE (program_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `asset_relationships_to_idx` | `(to_asset_id)` | Reverse lookups |


## Domain: evidence

### `evidence_items`

- Primary key: `(id)`
- Foreign key `evidence_items_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `evidence_items_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `evidence_items_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT, optional
- Foreign key `evidence_items_fk_attempt_id`: `(attempt_id)` -> `job_attempts (id)`, RESTRICT, optional
- Foreign key `evidence_items_fk_target_id`: `(target_id)` -> `targets (id)`, RESTRICT, optional
- Foreign key `evidence_items_fk_decision_id`: `(decision_id)` -> `policy_decisions (id)`, RESTRICT, optional
- Foreign key `evidence_items_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `evidence_items_fk_program_id_target_id`: `(program_id, target_id)` -> `targets (program_id, id)`, RESTRICT, optional
- `c1`: UNIQUE (store_backend, storage_key)
- `c2`: CHECK length(sha256) = 32
- `c3`: CHECK size_bytes >= 0
- `c4`: CHECK kind IN (request, response, request_response, tool_output, note, report)
- `c5`: CHECK redaction_state IN (redacted, raw_retained)
- `c6`: CHECK sensitivity IN (normal, sensitive)
- `c7`: CHECK origin IN (observed, ai_inferred, human_entered)
- `c10`: UNIQUE (program_id, id)
- `c11`: Trigger: BEFORE UPDATE or DELETE raises (immutable metadata, P-37). Runtime roles have no UPDATE or DELETE grant.

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `evidence_assessment_idx` | `(assessment_id, captured_at DESC)` | List evidence |
| `evidence_job_idx` | `(job_id) WHERE job_id IS NOT NULL` | Evidence of a job |
| `evidence_sha_idx` | `(sha256)` | Find duplicates and orphan checks |

### `evidence_integrity_checks`

- Primary key: `(id)`
- Foreign key `evidence_integrity_checks_fk_evidence_id`: `(evidence_id)` -> `evidence_items (id)`, RESTRICT
- Foreign key `evidence_integrity_checks_fk_actor_user_id`: `(actor_user_id)` -> `users (id)`, RESTRICT, optional
- `c1`: CHECK trigger IN (read, manual, sweep, pre_verification)
- `c2`: CHECK result IN (match, hash_mismatch, size_mismatch, missing, metadata_inconsistent)
- `c3`: CHECK actor_kind IN (user, worker, system)
- `c4`: CHECK (result IN (match, hash_mismatch)) = (observed_sha256 IS NOT NULL)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `evidence_checks_idx` | `(evidence_id, checked_at DESC)` | Latest check and history |

### `evidence_status`

- Primary key: `(evidence_id)`
- Foreign key `evidence_status_fk_evidence_id`: `(evidence_id)` -> `evidence_items (id)`, RESTRICT
- Foreign key `evidence_status_fk_last_check_id`: `(last_check_id)` -> `evidence_integrity_checks (id)`, RESTRICT, optional
- Foreign key `evidence_status_fk_purged_by`: `(purged_by)` -> `users (id)`, RESTRICT, optional
- `c1`: CHECK integrity_state IN (unchecked, ok, compromised, missing)
- `c2`: CHECK retention_state IN (active, purge_pending, purged)
- `c3`: CHECK (retention_state = purged) = (purged_at IS NOT NULL AND purged_by IS NOT NULL)
- `c4`: CHECK (integrity_state <> compromised) OR compromised_at IS NOT NULL

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `evidence_status_state_idx` | `(integrity_state) WHERE integrity_state IN (compromised, missing)` | Find affected evidence |


## Domain: finding

### `findings`

- Primary key: `(id)`
- Foreign key `findings_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `findings_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `findings_fk_asset_id`: `(asset_id)` -> `assets (id)`, RESTRICT, optional
- Foreign key `findings_fk_endpoint_id`: `(endpoint_id)` -> `endpoints (id)`, RESTRICT, optional
- Foreign key `findings_fk_verification_id`: `(verification_id)` -> `finding_verifications (id)`, RESTRICT, DEFERRABLE INITIALLY DEFERRED, optional
- Foreign key `findings_fk_created_by_user_id`: `(created_by_user_id)` -> `users (id)`, RESTRICT, optional
- Foreign key `findings_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- Foreign key `findings_fk_program_id_asset_id`: `(program_id, asset_id)` -> `assets (program_id, id)`, RESTRICT, optional
- Foreign key `findings_fk_program_id_endpoint_id`: `(program_id, endpoint_id)` -> `endpoints (program_id, id)`, RESTRICT, optional
- Foreign key `findings_fk_id_verification_id`: `(id, verification_id)` -> `finding_verifications (finding_id, id)`, RESTRICT, DEFERRABLE INITIALLY DEFERRED, optional
- `c1`: CHECK status IN (candidate, needs_review, inconclusive, false_positive, not_reproducible, awaiting_human_verification, verified)
- `c2`: CHECK origin IN (ai, tool, human)
- `c3`: CHECK created_by_kind IN (user, ai, worker, tool)
- `c4`: CHECK (created_by_kind = user) = (created_by_user_id IS NOT NULL)
- `c5`: CHECK (status = verified) = (verification_id IS NOT NULL)
- `c6`: CHECK length(finding_key) = 32
- `c7`: CHECK asset_id IS NOT NULL OR endpoint_id IS NOT NULL
- `c8`: UNIQUE (assessment_id, finding_key)
- `c9`: UNIQUE (program_id, id)
- `c14`: Deferred constraint trigger: status equals the to_status of the newest finding_transitions row (DB-11)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `findings_assessment_idx` | `(assessment_id, status, created_at DESC)` | List and filter findings |
| `findings_asset_idx` | `(asset_id) WHERE asset_id IS NOT NULL` | Findings of an asset |

### `finding_transitions`

- Primary key: `(id)`
- Foreign key `finding_transitions_fk_finding_id`: `(finding_id)` -> `findings (id)`, RESTRICT
- Foreign key `finding_transitions_fk_actor_user_id`: `(actor_user_id)` -> `users (id)`, RESTRICT, optional
- Foreign key `finding_transitions_fk_validation_id`: `(validation_id)` -> `validation_records (id)`, RESTRICT, optional
- `c1`: CHECK actor_kind IN (user, ai, worker, tool)
- `c2`: CHECK (actor_kind = user) = (actor_user_id IS NOT NULL)
- `c3`: CHECK automated actors: actor_kind = user OR (from_status IS NULL AND to_status = 'candidate') OR (from_status = 'candidate' AND to_status = 'needs_review')
- `c4`: CHECK to_status <> 'verified' OR (actor_kind = user AND from_status = 'awaiting_human_verification' AND validation_id IS NOT NULL) -- only a human verifies (SEC 5, D-09)
- `c5`: CHECK legal pairs: (NULL,candidate), (candidate,needs_review), (needs_review,inconclusive), (needs_review,false_positive), (needs_review,not_reproducible), (needs_review,awaiting_human_verification), (awaiting_human_verification,verified), (awaiting_human_verification,needs_review), (inconclusive,needs_review), (false_positive,needs_review), (not_reproducible,needs_review), (verified,needs_review)
- `c6`: CHECK from_status IS NULL OR actor_kind <> user OR reason IS NOT NULL -- human transitions carry a reason
- `c7`: UNIQUE (finding_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `finding_transitions_idx` | `(finding_id, created_at)` | History view |

### `finding_evidence`

- Primary key: `(finding_id, evidence_id)`
- Foreign key `finding_evidence_fk_finding_id`: `(finding_id)` -> `findings (id)`, RESTRICT
- Foreign key `finding_evidence_fk_evidence_id`: `(evidence_id)` -> `evidence_items (id)`, RESTRICT
- Foreign key `finding_evidence_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `finding_evidence_fk_validation_id`: `(validation_id)` -> `validation_records (id)`, RESTRICT, optional
- Foreign key `finding_evidence_fk_linked_by_user_id`: `(linked_by_user_id)` -> `users (id)`, RESTRICT, optional
- Foreign key `finding_evidence_fk_program_id_finding_id`: `(program_id, finding_id)` -> `findings (program_id, id)`, RESTRICT
- Foreign key `finding_evidence_fk_program_id_evidence_id`: `(program_id, evidence_id)` -> `evidence_items (program_id, id)`, RESTRICT
- `c1`: CHECK relation IN (supports, reproduction, context)
- `c2`: CHECK linked_by_kind IN (user, ai, worker, tool)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `finding_evidence_evidence_idx` | `(evidence_id)` | Findings that use an evidence item |

### `validation_records`

- Primary key: `(id)`
- Foreign key `validation_records_fk_finding_id`: `(finding_id)` -> `findings (id)`, RESTRICT
- Foreign key `validation_records_fk_replay_job_id`: `(replay_job_id)` -> `jobs (id)`, RESTRICT, optional
- Foreign key `validation_records_fk_recorded_by_user_id`: `(recorded_by_user_id)` -> `users (id)`, RESTRICT, optional
- `c1`: CHECK kind IN (manual_review, reproduction_replay, automated_recommendation)
- `c2`: CHECK result IN (reproduced, not_reproduced, inconclusive, false_positive_suspected)
- `c3`: CHECK origin IN (human, ai, tool)
- `c4`: CHECK (origin = human) = (recorded_by_user_id IS NOT NULL)
- `c5`: CHECK origin = human OR is_recommendation
- `c6`: CHECK kind <> automated_recommendation OR is_recommendation
- `c7`: UNIQUE (finding_id, id)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `validation_records_finding_idx` | `(finding_id, created_at DESC)` | Validation history |

### `finding_verifications`

- Primary key: `(id)`
- Foreign key `finding_verifications_fk_finding_id`: `(finding_id)` -> `findings (id)`, RESTRICT
- Foreign key `finding_verifications_fk_verified_by`: `(verified_by)` -> `users (id)`, RESTRICT
- Foreign key `finding_verifications_fk_validation_id`: `(validation_id)` -> `validation_records (id)`, RESTRICT
- Foreign key `finding_verifications_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT, optional
- Foreign key `finding_verifications_fk_superseded_by_transition_id`: `(superseded_by_transition_id)` -> `finding_transitions (id)`, RESTRICT, optional
- Foreign key `finding_verifications_fk_finding_id_validation_id`: `(finding_id, validation_id)` -> `validation_records (finding_id, id)`, RESTRICT
- `c1`: UNIQUE (finding_id, id)
- `c3`: CHECK length(notes) >= 1
- `c4`: No actor-kind column: verified_by is NOT NULL and references users, so no AI, worker or tool identity can be stored

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `finding_verifications_finding_idx` | `(finding_id, verified_at DESC)` | Verification history |

### `finding_verification_evidence`

- Primary key: `(verification_id, evidence_id)`
- Foreign key `finding_verification_evidence_fk_verification_id`: `(verification_id)` -> `finding_verifications (id)`, RESTRICT
- Foreign key `finding_verification_evidence_fk_evidence_id`: `(evidence_id)` -> `evidence_items (id)`, RESTRICT
- Foreign key `finding_verification_evidence_fk_integrity_check_id`: `(integrity_check_id)` -> `evidence_integrity_checks (id)`, RESTRICT
- `c1`: CHECK length(sha256_at_verification) = 32
- `c2`: integrity_check_id must reference a check with result match for the same evidence (deferred trigger)

### `reports`

- Primary key: `(id)`
- Foreign key `reports_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `reports_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `reports_fk_created_by`: `(created_by)` -> `users (id)`, RESTRICT
- Foreign key `reports_fk_finalized_by`: `(finalized_by)` -> `users (id)`, RESTRICT, optional
- Foreign key `reports_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- `c1`: CHECK state IN (draft, final)
- `c2`: CHECK format IN (markdown)
- `c3`: CHECK (state = final) = (finalized_by IS NOT NULL AND finalized_at IS NOT NULL AND sha256 IS NOT NULL)
- `c5`: UNIQUE (program_id, id)
- `c6`: Trigger: a final report row and its report_findings rows cannot change

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `reports_assessment_idx` | `(assessment_id, created_at DESC)` | List reports |

### `report_findings`

- Primary key: `(report_id, finding_id)`
- Foreign key `report_findings_fk_report_id`: `(report_id)` -> `reports (id)`, RESTRICT
- Foreign key `report_findings_fk_finding_id`: `(finding_id)` -> `findings (id)`, RESTRICT
- Foreign key `report_findings_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `report_findings_fk_verification_id`: `(verification_id)` -> `finding_verifications (id)`, RESTRICT, optional
- Foreign key `report_findings_fk_included_by`: `(included_by)` -> `users (id)`, RESTRICT
- Foreign key `report_findings_fk_program_id_report_id`: `(program_id, report_id)` -> `reports (program_id, id)`, RESTRICT
- Foreign key `report_findings_fk_program_id_finding_id`: `(program_id, finding_id)` -> `findings (program_id, id)`, RESTRICT
- `c1`: CHECK section IN (confirmed, unverified_candidates)
- `c2`: CHECK section <> 'confirmed' OR (finding_status_at_inclusion = 'verified' AND verification_id IS NOT NULL) -- unverified items never appear as confirmed (D-75)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `report_findings_finding_idx` | `(finding_id)` | Derive the finding report status view |


## Domain: integration

### `credential_refs`

- Primary key: `(id)`
- Foreign key `credential_refs_fk_created_by`: `(created_by)` -> `users (id)`, RESTRICT
- `c1`: CHECK owner_kind IN (ai_provider, tool)
- `c2`: UNIQUE (secret_service_key)
- `c3`: CHECK secret_service_key ~ '^ms/[a-z_]+/[0-9a-f-]{36}$' -- fixed shape, never a value

### `ai_providers`

- Primary key: `(id)`
- Foreign key `ai_providers_fk_credential_ref_id`: `(credential_ref_id)` -> `credential_refs (id)`, RESTRICT, optional
- Foreign key `ai_providers_fk_created_by`: `(created_by)` -> `users (id)`, RESTRICT
- `c1`: CHECK adapter_kind IN (loopback, openai_compatible)
- `c2`: CHECK declared_locality IN (loopback, remote)
- `c3`: CHECK effective_locality IN (loopback, remote)
- `c4`: CHECK cloud_forwarding_check IN (unverified, off, on)
- `c5`: CHECK adapter_kind <> loopback OR declared_locality = loopback
- `c6`: CHECK effective_locality = remote OR (declared_locality = loopback AND cloud_forwarding_check = off)
- `c7`: UNIQUE (credential_ref_id)
- `c8`: CHECK length(endpoint_url) <= 2048 AND endpoint_url !~ '@' -- no userinfo credentials in URLs

### `program_provider_enablements`

- Primary key: `(provider_id)`
- Foreign key `program_provider_enablements_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `program_provider_enablements_fk_provider_id`: `(provider_id)` -> `ai_providers (id)`, RESTRICT
- Foreign key `program_provider_enablements_fk_enabled_by`: `(enabled_by)` -> `users (id)`, RESTRICT
- Foreign key `program_provider_enablements_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT
- `c1`: PRIMARY KEY (program_id, provider_id)
- `c2`: CHECK length(disclosure_hash) = 32

### `ai_transmissions`

- Primary key: `(id)`
- Foreign key `ai_transmissions_fk_provider_id`: `(provider_id)` -> `ai_providers (id)`, RESTRICT
- Foreign key `ai_transmissions_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `ai_transmissions_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT, optional
- Foreign key `ai_transmissions_fk_audit_event_id`: `(audit_event_id)` -> `audit_events (id)`, RESTRICT
- `c1`: CHECK locality_at_connect IN (loopback, remote)
- `c2`: CHECK length(content_digest) = 32
- `c3`: CHECK bytes_sent >= 0

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `ai_transmissions_program_idx` | `(program_id, created_at DESC)` | Disclosure review |

### `ai_outputs`

- Primary key: `(id)`
- Foreign key `ai_outputs_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `ai_outputs_fk_assessment_id`: `(assessment_id)` -> `assessments (id)`, RESTRICT
- Foreign key `ai_outputs_fk_job_id`: `(job_id)` -> `jobs (id)`, RESTRICT
- Foreign key `ai_outputs_fk_program_id_assessment_id`: `(program_id, assessment_id)` -> `assessments (program_id, id)`, RESTRICT
- `c1`: CHECK kind IN (analysis_note, report_draft)
- `c2`: CHECK origin = 'ai'
- `c3`: CHECK octet_length(body) <= 65536

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `ai_outputs_assessment_idx` | `(assessment_id, created_at DESC)` | List outputs |

### `tool_configs`

- Primary key: `(id)`
- Foreign key `tool_configs_fk_registered_by`: `(registered_by)` -> `users (id)`, RESTRICT
- Foreign key `tool_configs_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT
- `c1`: CHECK tool_key IN (subfinder, httpx, nuclei)
- `c2`: CHECK length(binary_sha256) = 32 AND length(manifest_digest) = 32
- `c3`: CHECK verification_state IN (unverified, verified, mismatch)
- `c4`: CHECK NOT enabled OR verification_state = verified
- `c5`: CHECK binary_path ~ '^/' AND binary_path !~ '[[:cntrl:]]'
- `c6`: UNIQUE (tool_key)

### `passive_source_enablements`

- Primary key: `(id)`
- Foreign key `passive_source_enablements_fk_program_id`: `(program_id)` -> `programs (id)`, RESTRICT
- Foreign key `passive_source_enablements_fk_enabled_by`: `(enabled_by)` -> `users (id)`, RESTRICT
- Foreign key `passive_source_enablements_fk_reauth_grant_id`: `(reauth_grant_id)` -> `reauth_grants (id)`, RESTRICT
- `c1`: CHECK length(disclosure_hash) = 32

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `passive_source_active_uq` | `UNIQUE (program_id, source_key) WHERE revoked_at IS NULL` | One live enablement per source |

### `integration_checks`

- Primary key: `(id)`
- `c1`: CHECK subject_kind IN (sandbox, egress, secret_service, database, ai_provider, tool)
- `c2`: CHECK result IN (pass, fail, unavailable)

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `integration_checks_idx` | `(subject_kind, subject_id, checked_at DESC)` | Latest status per subject |


## Domain: platform

### `idempotency_keys`

- Primary key: `(id)`
- Foreign key `idempotency_keys_fk_user_id`: `(user_id)` -> `users (id)`, RESTRICT
- `c1`: UNIQUE (user_id, key_hash, method, route)
- `c2`: CHECK length(key_hash) = 32 AND length(request_hash) = 32

| Index | Definition | Query pattern it serves |
| --- | --- | --- |
| `idempotency_expiry_idx` | `(expires_at)` | Cleanup |
