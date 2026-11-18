-- +goose Up
-- M5 audit sink. Two changes, both forward only.
-- 1. An unauthenticated caller is an anonymous actor and no longer a system actor, so a query for system actors does not return the denials of strangers (TD-M4-03).
-- 2. At most one open audit_retry_exhausted alert exists, so a long outage cannot fill the table with alerts (T-52). The alert is raised with ON CONFLICT DO NOTHING.
-- Rows written before this migration keep their actor kind: audit rows are append-only.

ALTER TABLE morningstar.audit_events DROP CONSTRAINT audit_events_ck_c2;
ALTER TABLE morningstar.audit_events ADD CONSTRAINT audit_events_ck_c2 CHECK (actor_kind IN ('user', 'worker', 'system', 'ai', 'anonymous'));

CREATE UNIQUE INDEX system_alerts_one_open_audit_alert ON morningstar.system_alerts (kind) WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL;
