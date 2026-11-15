# Data lifecycle

Status: design proposal. Retention numbers are proposals (DQ-07). Nothing here deletes data today.

## 1. Distinctions

| Term | Meaning here | Reversible | Example |
| --- | --- | --- | --- |
| Soft state | A status column or timestamp marks the row inactive. The row stays and stays readable by its owner. | Yes | `targets.status = archived`, `approvals.status = revoked` |
| Archival | A program or assessment becomes read-only. Nothing is removed. | Yes, by a future restore action (DEF-03) | `programs.status = archived` |
| Purge of content | The stored bytes are removed. The metadata and hash stay so the audit story remains. | No | `evidence_status.retention_state = purged` |
| Permanent deletion | Rows are removed. Not available in the MVP. A later retention procedure deletes leaf rows first under `ms_retention` and audits the purge. | No | post-MVP |

## 2. Retention categories

| Class | Data | Rule |
| --- | --- | --- |
| RC-A | Authorization and audit history: audit events, approvals and their events and uses, policy decisions, scope versions and rules, validations, labs, finding history and verification, reports, policy versions, account and credential hash history | Kept indefinitely in the MVP. Never cascaded. Removed only by a future audited retention procedure. |
| RC-B | Security-operational state: sessions, re-authentication grants, login attempts, bootstrap secrets, idempotency keys, integration checks, system alerts | Purged by the sweep 30 days after expiry or clearance (proposed). Sessions and grants are useless after expiry. |
| RC-C | Assessment data: assessments, plan items, jobs, attempts, assets, observations, AI outputs | Kept until the human archives or purges the assessment. |
| RC-D | Evidence: content and metadata | Content kept until an explicit audited purge (D-77). Metadata kept after purge. No automatic age-based purge in the MVP. |
| RC-E | Configuration: providers, tools, credential references, enablements | Kept while configured. A removed provider or tool is disabled, not deleted, so audit rows keep a target. |

## 3. Behavior by event

| Event | Behavior | Why |
| --- | --- | --- |
| User deletion | Not supported. There is one account (D-60). A user row can be `disabled`. All foreign keys to `users` are RESTRICT, so a delete fails. | Approvals, verifications and audit rows name the user. |
| Password change or recovery reset | A new `user_credentials` row replaces the current one. All sessions are revoked. The old hash row stays as history (no reuse check is made from it). | Auditability of when the credential changed. |
| Program deletion | Not supported. Archive only. An archived program is read-only, its running work is stopped first, and its scope, approvals, findings and evidence stay. | Required: no cascading deletion of audit history. |
| Assessment archival | A completed or cancelled assessment is terminal and read-only through its state. Its jobs, evidence and findings stay. | Findings and evidence outlive the testing session. |
| Target archival | Archived targets stay for old validations and jobs. A new validation cannot include an archived target. | Historical authorization context must stay readable. |
| Scope change | A new version. The old version and its rules stay forever (RC-A). | DB-10. |
| Lab expiry or revocation | Status change only. | D-47. |
| Evidence purge | Human action under the retention role: content removed, `evidence_status` set to purged with actor and time, audit event, finding verifications that cite it keep the hash snapshot. A purged item cannot support a new verification. | D-77. |
| Report finalization | The report and its inclusion rows become immutable. | Reports are exports of record. |
| Idempotency and session cleanup | A sweep deletes expired rows older than 30 days (RC-B). | Operational data only. |
| Audit retention | Kept indefinitely in the MVP. A later procedure may delete rows older than the configured period under `ms_retention`, always writing a `retention.purged` audit event first. | SEC 9, SEC 13. |

## 4. Orphan detection

Run by the sweep (`WRK-08`) and reported as `system_alerts`. Nothing is deleted automatically.

| Check | Meaning | Action |
| --- | --- | --- |
| Evidence files with no `evidence_items` row | A write whose metadata transaction failed (P-37) | Report. Move to a quarantine directory only on human request. |
| `evidence_items` rows whose file is missing | Lost or deleted content | Record a `missing` check, alert, block use as proof. |
| Evidence files whose digest differs | Edit or corruption | Record `hash_mismatch`, mark compromised, alert. |
| `audit_events.entity_id` with no row of that `entity_type` | Soft-reference break (DB-16). Cannot happen without hard deletion. | Alert as a data-integrity failure. |
| Jobs `running` with an expired lease | Worker died | Reaper (WRK-07), attempt `lost`, no retry before termination is confirmed. |
| Jobs with `stop_requested` older than the objective | Stop not confirmed | Alert and keep new operations denied for that target. |
| Approvals `pending` older than 7 days | Forgotten requests (proposed) | Mark `expiry_observed` and show as stale in the UI. |
| Bootstrap secrets live while an account exists | Impossible by the single-account rule | Alert. |
| Observations with `origin = observed` and no attempt | Impossible by CHECK and FK | Alert as a data-integrity failure. |

## 5. Backup and recovery

- Backups: `pg_dump` of the `morningstar` database plus a copy of the evidence directory, both with owner-only permissions and never inside the repository (`.gitignore`, D-84). The two must come from the same moment. A restore re-hashes evidence and records the result.
- Backups hold target data, audit rows and password hashes, so they carry at least the permissions of the source (SEC 12). They hold no secret value, because secrets live in the Secret Service (D-52). A restore on another machine therefore loses provider keys and needs them re-entered. It also lacks the recovery marker, so it starts in `recovery_required` (`marker_missing`).
- **Restore detection and recovery (DB-27, DB-28, DQ-17).** A backup older than the latest approval use would bring back a consumed use as unconsumed and an older scope version as current. The database cannot detect its own restore. An external marker file outside the backup set holds a recovery generation and an audit watermark, and startup compares it with the restored database. A mismatch puts the installation in `recovery_required` and only the reconciliation (AUTH-10) can leave that state. The procedure, the invalidation list and the limits are in `recovery-generation.md`. The marker must never be part of a backup. A snapshot that restores the marker together with the database is not detected.
- Point-in-time recovery and replication are not designed for the MVP.
- Version upgrade: a new PostgreSQL major version needs a dump and restore into a new per-user cluster, with the recovery rule above.

## 6. Privacy and minimization

- `detail` and `payload` columns are size-limited and hold no raw request bodies, no cookies and no credentials.
- Evidence of record is redacted before it is written (D-77). Raw capture is a per-assessment widening policy and is marked in `evidence_items.redaction_state`.
- AI prompts and responses are not stored (D-81). `ai_transmissions` stores the digest and size only.
- Target data sent to a remote provider is recorded as a transmission with the program, never as content.

