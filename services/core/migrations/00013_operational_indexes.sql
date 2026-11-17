-- +goose Up
-- Slice 00013 operational_indexes (milestone M3 part). Every index names the query it serves (DB-19).
-- Later milestones add the indexes of their tables before the first release.

CREATE UNIQUE INDEX users_single_active ON morningstar.users ((true)) WHERE status = 'active'; -- Enforces D-60 at the database
CREATE UNIQUE INDEX user_credentials_current_uq ON morningstar.user_credentials (user_id) WHERE is_current; -- One current hash per user
CREATE UNIQUE INDEX recovery_codes_active_uq ON morningstar.recovery_codes (user_id) WHERE used_at IS NULL AND revoked_at IS NULL; -- One usable code per user
CREATE UNIQUE INDEX bootstrap_secrets_active_uq ON morningstar.bootstrap_secrets ((true)) WHERE consumed_at IS NULL AND superseded_at IS NULL; -- One live secret at a time
CREATE INDEX sessions_user_active_idx ON morningstar.sessions (user_id) WHERE revoked_at IS NULL; -- List and revoke active sessions
CREATE INDEX sessions_expiry_idx ON morningstar.sessions (absolute_expires_at); -- Cleanup of expired sessions
CREATE INDEX reauth_grants_lookup_idx ON morningstar.reauth_grants (session_id, purpose, subject_hash) WHERE consumed_at IS NULL; -- Find an unconsumed grant for the action
CREATE INDEX login_attempts_window_idx ON morningstar.login_attempts (kind, source_key, attempted_at DESC); -- Count failures in a rolling 15 minutes
CREATE UNIQUE INDEX policy_settings_current_uq ON morningstar.policy_settings (setting_key) WHERE superseded_at IS NULL; -- One current version per key; also the lookup of current values
CREATE UNIQUE INDEX scope_versions_current_uq ON morningstar.scope_versions (program_id) WHERE superseded_at IS NULL; -- One current version per program and the lookup of it
CREATE INDEX scope_rules_version_idx ON morningstar.scope_rules (scope_version_id, exclude); -- Load a version, exclusions first
CREATE INDEX refused_exclusions_open_idx ON morningstar.refused_exclusions (program_id) WHERE status = 'open'; -- Block readiness while any is open
CREATE INDEX targets_program_idx ON morningstar.targets (program_id, status); -- List targets of a program
CREATE INDEX lab_registrations_active_idx ON morningstar.lab_registrations (assessment_id) WHERE status = 'active'; -- Find live labs at E2 and E3 and at expiry sweeps
CREATE INDEX lab_ranges_lookup_idx ON morningstar.lab_registration_ranges USING gist (range_cidr inet_ops); -- Containment lookup at E3 (needs the btree_gist or built-in inet_ops, verified at M3)
CREATE UNIQUE INDEX target_validations_current_uq ON morningstar.target_validations (assessment_id) WHERE status = 'current'; -- At most one current record per assessment, and its lookup
CREATE INDEX assessments_program_idx ON morningstar.assessments (program_id, status, created_at DESC); -- List by program and state
CREATE INDEX assessment_targets_active_idx ON morningstar.assessment_targets (assessment_id) WHERE removed_at IS NULL; -- Current target set
CREATE INDEX assessment_transitions_idx ON morningstar.assessment_transitions (assessment_id, created_at); -- History view
CREATE INDEX plan_items_assessment_idx ON morningstar.plan_items (assessment_id, status); -- List plan items
CREATE INDEX policy_decisions_job_idx ON morningstar.policy_decisions (job_id, decided_at); -- Decision trail of a job
CREATE INDEX approvals_operation_idx ON morningstar.approvals (operation_id, status); -- Find the live approval for an operation at E1 and E3
CREATE INDEX approvals_assessment_idx ON morningstar.approvals (assessment_id, created_at DESC); -- List approvals
CREATE INDEX approval_events_idx ON morningstar.approval_events (approval_id, created_at); -- Approval history
CREATE INDEX approval_uses_job_idx ON morningstar.approval_uses (job_id); -- Find the use of a job
CREATE INDEX jobs_claim_idx ON morningstar.jobs (priority DESC, available_at, id) WHERE state = 'queued' AND control_state = 'none'; -- The claim query with FOR UPDATE SKIP LOCKED
CREATE INDEX jobs_lease_idx ON morningstar.jobs (lease_expires_at) WHERE state = 'running'; -- Reaper finds expired leases
CREATE INDEX jobs_assessment_idx ON morningstar.jobs (assessment_id, state, created_at DESC); -- List jobs of an assessment
CREATE INDEX jobs_approval_idx ON morningstar.jobs (approval_id) WHERE state IN ('queued', 'awaiting_approval', 'running'); -- Revocation finds dependent jobs (30-second objective)
CREATE INDEX jobs_scope_idx ON morningstar.jobs (scope_version_id) WHERE state IN ('queued', 'awaiting_approval', 'running'); -- Scope change finds dependent jobs
CREATE UNIQUE INDEX job_attempts_unconfirmed_uq ON morningstar.job_attempts (job_id) WHERE termination_confirmed_at IS NULL; -- A retry cannot start until the previous tree is confirmed terminated, and a job has one live attempt (D-64 choice 12)
CREATE INDEX job_transitions_idx ON morningstar.job_transitions (job_id, created_at); -- Execution history
CREATE INDEX audit_events_time_idx ON morningstar.audit_events (occurred_at DESC, seq DESC); -- Audit list and keyset pagination
CREATE INDEX audit_events_entity_idx ON morningstar.audit_events (entity_type, entity_id, occurred_at); -- History of one entity
CREATE INDEX audit_events_action_idx ON morningstar.audit_events (action, occurred_at DESC); -- Filter by action
CREATE INDEX audit_events_program_idx ON morningstar.audit_events (program_id, occurred_at DESC) WHERE program_id IS NOT NULL; -- Program view
CREATE INDEX system_alerts_open_idx ON morningstar.system_alerts (kind) WHERE cleared_at IS NULL; -- Open alerts block new operations when kind = audit_retry_exhausted

-- Partial unique indexes that the design lists as constraints.
CREATE UNIQUE INDEX programs_uq_c4 ON morningstar.programs (owner_user_id, lower(name)) WHERE status = 'active';
