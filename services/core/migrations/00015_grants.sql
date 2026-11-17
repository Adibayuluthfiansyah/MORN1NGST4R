-- +goose Up
-- Slice 00015 grants (milestone M3 part). Least privilege per role (DB-06, P-13). The matrix is in migration-plan.md.
-- The exact column lists below are the M3 grant review. Differences from the matrix are listed in the M3 section of plan.md.

GRANT USAGE ON SCHEMA morningstar TO ms_api, ms_worker, ms_retention;
REVOKE ALL ON ALL TABLES IN SCHEMA morningstar FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA morningstar FROM PUBLIC;

-- ms_api
GRANT SELECT, INSERT, UPDATE ON morningstar.users TO ms_api;
GRANT SELECT, INSERT ON morningstar.user_credentials TO ms_api;
GRANT UPDATE (is_current, superseded_at) ON morningstar.user_credentials TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.recovery_codes TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.bootstrap_secrets TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.sessions TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.reauth_grants TO ms_api;
GRANT SELECT, INSERT ON morningstar.login_attempts TO ms_api;
GRANT SELECT, INSERT ON morningstar.recovery_state TO ms_api;
GRANT UPDATE (state, generation, last_reason, reconciled_at, updated_at) ON morningstar.recovery_state TO ms_api;
GRANT SELECT, INSERT ON morningstar.policy_settings TO ms_api;
GRANT UPDATE (superseded_at) ON morningstar.policy_settings TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.programs TO ms_api;
GRANT SELECT, INSERT ON morningstar.program_rules_versions TO ms_api;
GRANT SELECT, INSERT ON morningstar.scope_versions TO ms_api;
GRANT UPDATE (superseded_at) ON morningstar.scope_versions TO ms_api;
GRANT SELECT, INSERT ON morningstar.scope_rules TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.refused_exclusions TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.targets TO ms_api;
GRANT SELECT, INSERT ON morningstar.lab_registrations TO ms_api;
GRANT UPDATE (status, revoked_at, revoked_by, revoke_reason) ON morningstar.lab_registrations TO ms_api;
GRANT SELECT, INSERT ON morningstar.lab_registration_ranges TO ms_api;
GRANT SELECT, INSERT ON morningstar.target_validations TO ms_api;
GRANT UPDATE (status, invalidated_at, invalidation_reason) ON morningstar.target_validations TO ms_api;
GRANT SELECT, INSERT ON morningstar.target_validation_targets TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.assessments TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.assessment_targets TO ms_api;
GRANT SELECT, INSERT ON morningstar.assessment_transitions TO ms_api;
GRANT SELECT, INSERT, UPDATE ON morningstar.plan_items TO ms_api;
GRANT SELECT, INSERT ON morningstar.operations TO ms_api;
GRANT SELECT, INSERT ON morningstar.policy_decisions TO ms_api;
GRANT SELECT, INSERT ON morningstar.approvals TO ms_api;
GRANT UPDATE (status, decided_by, decided_at, expires_at, max_uses, max_attempts, reauth_grant_id, revoked_at, revoked_by, revoke_reason, updated_at, version) ON morningstar.approvals TO ms_api;
GRANT SELECT, INSERT ON morningstar.approval_events TO ms_api;
GRANT SELECT ON morningstar.approval_uses TO ms_api;
GRANT SELECT, INSERT ON morningstar.jobs TO ms_api;
GRANT UPDATE (state, control_state, control_reason, control_requested_at, finished_at, updated_at, version) ON morningstar.jobs TO ms_api;
GRANT SELECT ON morningstar.job_attempts TO ms_api;
-- The reconciliation after a restore marks live attempts lost (recovery-generation.md section 6).
GRANT UPDATE (state, ended_at, error_class) ON morningstar.job_attempts TO ms_api;
GRANT SELECT, INSERT ON morningstar.job_transitions TO ms_api;
GRANT SELECT, INSERT ON morningstar.audit_events TO ms_api;
GRANT SELECT, INSERT ON morningstar.system_alerts TO ms_api;
GRANT UPDATE (cleared_at, cleared_by) ON morningstar.system_alerts TO ms_api;

-- ms_worker
GRANT SELECT ON morningstar.recovery_state TO ms_worker;
GRANT SELECT ON morningstar.policy_settings TO ms_worker;
GRANT SELECT ON morningstar.programs TO ms_worker;
GRANT SELECT ON morningstar.program_rules_versions TO ms_worker;
GRANT SELECT ON morningstar.scope_versions TO ms_worker;
GRANT SELECT ON morningstar.scope_rules TO ms_worker;
GRANT SELECT ON morningstar.refused_exclusions TO ms_worker;
GRANT SELECT ON morningstar.targets TO ms_worker;
GRANT SELECT ON morningstar.lab_registrations TO ms_worker;
GRANT SELECT ON morningstar.lab_registration_ranges TO ms_worker;
GRANT SELECT ON morningstar.target_validations TO ms_worker;
GRANT SELECT ON morningstar.target_validation_targets TO ms_worker;
GRANT SELECT ON morningstar.assessments TO ms_worker;
GRANT SELECT ON morningstar.assessment_targets TO ms_worker;
GRANT SELECT ON morningstar.assessment_transitions TO ms_worker;
GRANT SELECT ON morningstar.plan_items TO ms_worker;
GRANT SELECT ON morningstar.operations TO ms_worker;
GRANT SELECT, INSERT ON morningstar.policy_decisions TO ms_worker;
GRANT SELECT ON morningstar.approvals TO ms_worker;
GRANT SELECT, INSERT ON morningstar.approval_events TO ms_worker;
GRANT SELECT ON morningstar.approval_uses TO ms_worker;
GRANT SELECT ON morningstar.jobs TO ms_worker;
GRANT UPDATE (state, control_state, control_reason, approval_use_id, lease_owner, lease_expires_at, heartbeat_at, claim_epoch, execution_attempts, available_at, failure_class, finished_at, updated_at, version) ON morningstar.jobs TO ms_worker;
GRANT SELECT, INSERT ON morningstar.job_attempts TO ms_worker;
GRANT UPDATE (state, process_group_ref, ended_at, termination_confirmed_at, exit_code, error_class, tool_version, tool_binary_sha256, sandbox_digest, argv_redacted) ON morningstar.job_attempts TO ms_worker;
GRANT SELECT, INSERT ON morningstar.job_transitions TO ms_worker;
GRANT SELECT, INSERT ON morningstar.audit_events TO ms_worker;
GRANT SELECT, INSERT ON morningstar.system_alerts TO ms_worker;

-- ms_retention
GRANT SELECT ON morningstar.audit_events TO ms_retention;

-- The worker consumes an approval use only through the claim function (CC-02).
GRANT EXECUTE ON FUNCTION morningstar.consume_approval_use(uuid, uuid, uuid, integer, bigint, bytea, uuid, bytea, uuid) TO ms_worker;
GRANT EXECUTE ON FUNCTION morningstar.check_approval_launch(uuid, uuid, bigint, bytea, uuid, bytea, uuid) TO ms_worker;
