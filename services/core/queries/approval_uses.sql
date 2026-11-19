-- Queries of the approval consumption adapter
-- (internal/adapters/outbound/persistence/approval_uses.go). They call the SQL
-- functions of migration 00006, which hold the conditional update and the
-- unique (approval, job) insert (TD-M2-05, M3 part).

-- name: ConsumeApprovalUse :one
SELECT outcome::text AS outcome, coalesce(reason, '')::text AS reason, coalesce(use_id::text, '')::text AS use_id
FROM morningstar.consume_approval_use(
    sqlc.arg(use_id)::uuid, sqlc.arg(approval_id)::uuid, sqlc.arg(job_id)::uuid, sqlc.arg(claim_epoch)::integer,
    sqlc.arg(generation)::bigint, sqlc.arg(operation_hash)::bytea, sqlc.arg(scope_version_id)::uuid,
    sqlc.arg(policy_snapshot_hash)::bytea, sqlc.arg(validation_id)::uuid);

-- name: CheckApprovalLaunch :one
-- An empty reason means that the launch check passed.
SELECT coalesce(morningstar.check_approval_launch(
    sqlc.arg(approval_id)::uuid, sqlc.arg(job_id)::uuid, sqlc.arg(generation)::bigint, sqlc.arg(operation_hash)::bytea,
    sqlc.arg(scope_version_id)::uuid, sqlc.arg(policy_snapshot_hash)::bytea, sqlc.arg(validation_id)::uuid, sqlc.arg(claim_epoch)::integer), '')::text AS reason;
