# Recovery generation

Status: design proposal (DB-28, DQ-17). Not implemented, not tested, and not an approved owner decision. It replaces the one-line restore rule of the first design pass (DB-27). **M3 update (2026-11-18):** implemented and tested (gate G-recovery passed) for the M3 tables: `internal/application/recovery`, `internal/adapters/outbound/recoverymarker`, `internal/adapters/outbound/persistence/recovery.go`. Differences: the first run writes the marker inside the database transaction, `Advance` never recreates a missing marker, and the reconciliation takes a user id and a request id as input. Open parts are TD-M3-04 to TD-M3-06 in the plan.

## 1. Problem

A PostgreSQL backup holds the authority that existed when it was taken: sessions, approvals with their remaining uses, current validations, active lab registrations, queued jobs and policy values. Restoring an old backup brings that authority back. A consumed approval use becomes unconsumed again, a revoked approval becomes approved, an old scope version becomes current. A duplicate or an out-of-scope launch could follow.

## 2. Why the database cannot detect its own restore

Anything stored only in the database is restored with it. A flag such as `restored = true`, a counter, or an audit event written before the restore disappears or reverts. A restored database is internally consistent and has no way to know that time went backwards. The detector therefore needs a witness outside the database that a database restore does not touch. A database-only boolean is rejected as the detector.

## 3. Mechanism

**External marker.** A small file owned by the application and kept outside PostgreSQL and outside the documented backup set.

| Item | Rule |
| --- | --- |
| Path | `$XDG_STATE_HOME/morningstar/recovery-generation.json`, default `~/.local/state/morningstar/`. The directory has mode 0700 and the file mode 0600, both owned by the running user. |
| Not in the backup | The documented backup is `pg_dump` plus the evidence directory (`data-lifecycle.md`). The marker is never part of it. A restore procedure must not restore it. |
| Content | `{"version":1,"instance_id":"<uuid>","generation":<n>,"audit_watermark":<n>,"written_at":"<rfc3339>","check":"<sha256 of the other fields>"}` |
| Atomic update | Write a temporary file in the same directory, `fsync` it, rename over the marker, `fsync` the directory. Every writer first takes `flock` on `recovery-generation.lock` and keeps the maximum watermark. |
| Validation on read | Regular file, not a symlink, owner equals the running user, mode no wider than 0600, version 1, valid UUID, integers in range, and the check value matches. Anything else is `marker_invalid`. The check value detects accidental corruption. It does not authenticate the writer. |

**Database side.** The single-row table `recovery_state` holds `instance_id`, `generation`, `state` (`normal` or `reconciling`), `last_reason` and `reconciled_at`. It is the other input of the comparison, never the detector by itself.

**Recovery generation.** A monotonically increasing integer, starting at 1. It increases only inside a reconciliation (section 6). Normal operation never changes it.

**Audit watermark.** The highest `audit_events.seq` the marker has recorded. After a restore to an older state the database holds a lower maximum than the marker, even when no reconciliation ever happened, because the marker was written after later events. This closes the case "backup and restore inside one generation".

## 4. When the watermark advances

The marker watermark is advanced **synchronously, after the database commit and before the dependent action continues**, for these events: an approval use consumed at claim (before the process launch), an approval revoked, a scope version published, a lab registration revoked or expired, a validation marked stale, a session revoked by recovery, and a policy change that narrows authority. It is also advanced at clean shutdown and every 30 seconds while running (proposed). If a marker write fails the worker refuses new claims and the server raises a `system_alerts` row until a write succeeds.

Honest limit: the marker can lag the database. Events committed after the last marker write are not covered. A restore to a state that still contains those events is not detected. The synchronous rule covers the events that matter for replay (a consumed use before a launch, a revocation, a scope change). It does not cover every event.

## 5. Startup reconciliation

Run by the server and by the worker before they accept work. M is the marker, D the `recovery_state` row, `max_seq` the highest audit sequence in the database.

| # | Condition | Result |
| --- | --- | --- |
| 1 | No marker, no `recovery_state` row, no users | First run. Initialize (section 8). Normal. |
| 2 | No marker, but a `recovery_state` row or a user exists | `recovery_required`, reason `marker_missing` |
| 3 | Marker unreadable, unparseable, wrong owner or mode, a symlink, or fails its check | `recovery_required`, reason `marker_invalid` |
| 4 | Marker valid, no `recovery_state` row | `recovery_required`, reason `database_older` (a database from before this mechanism, or an empty one) |
| 5 | Marker and row have different `instance_id` | `recovery_required`, reason `instance_mismatch` (another installation's database or marker) |
| 6 | `M.generation` greater than `D.generation` | `recovery_required`, reason `database_older` |
| 7 | `M.generation` less than `D.generation` and `D.state` is `reconciling` and `D.generation = M.generation + 1` | Crash during a reconciliation. Resume phase 2 and 3 automatically, audit it (reason `reconcile_interrupted`). |
| 8 | `M.generation` less than `D.generation` otherwise | `recovery_required`, reason `marker_older` (an old marker was restored, or marker and database were changed separately) |
| 9 | Generations equal and `D.state` is `reconciling` | Crash between phase 2 and phase 3. Complete phase 3 automatically, audit it. |
| 10 | Generations equal, state `normal`, and `max_seq` is less than `M.audit_watermark` | `recovery_required`, reason `database_older` |
| 11 | Generations equal, state `normal`, `max_seq` at or above the watermark | Normal. |

If the marker cannot be written at startup the process does not start serving. Every result other than "normal" blocks claims and every authenticated route except `GET /session` and the reconcile route (503 `recovery_required`). The worker claim transaction also joins `recovery_state` and requires `state = 'normal'` and the generation it read from the marker (CC-15), so a worker that started before the restore cannot claim afterwards.

## 6. Reconciliation (AUTH-10)

The operator opens the application, sees the recovery page (section 9), and submits the account password of the restored database, a new password and a confirmation. The old password is checked like a login and counts toward the rate limit, and only then is the new one hashed. The new password must differ from the old one and meet the password policy. Phase 1 replaces the current credential with the new one in the same transaction, because the restored hash may be older than a rotation and must not keep its authority (M4, owner decision of 2026-11-18). The new recovery code is issued in the same transaction.

**Phase 1, one database transaction.** Compute `newG = max(M.generation if valid, D.generation) + 1`. Set `recovery_state` to `reconciling`, `generation = newG`, the reason. Then invalidate authority recorded in the database:

| Authority | Action |
| --- | --- |
| Sessions | Revoke all (reason `recovery_reset`). Unconsumed `reauth_grants` are consumed. |
| Approvals | `approved` becomes `revoked`, `pending` becomes `rejected`, with `approval_events`. Approval use counters are not touched or restored. |
| Target validations | `current` becomes `invalidated`, reason `restore_recovery`. |
| Lab registrations | `active` becomes `revoked`. |
| Provider and passive-source enablements, tools | Enablements revoked, `tool_configs.enabled` and `ai_providers.enabled` set false, raw evidence capture set false. They are restored to the researcher's explicit choice later. |
| Assessments | `ready` returns to `draft`, `running` becomes `paused`, with `assessment_transitions`. |
| Jobs and attempts | Queued and awaiting jobs become `cancelled`. Running jobs get `control_state = stop_requested`. Live attempts become `lost` with termination unconfirmed. Nothing is retried until termination is confirmed (CC-04). |
| Audit and alerts | Insert `system.restore_recovered` with the old and new generation, the reason and the counts. Insert a `system_alerts` row of kind `restore_recovered`. |

**Phase 2, marker write.** Write the marker with `newG`, the same instance id and the current maximum audit sequence, using the atomic procedure of section 3.

**Phase 3, one database transaction.** Set `state = 'normal'`, `reconciled_at`, insert `system.recovery_reconciled`. The services resume.

If the audit insert in phase 1 fails, phase 1 rolls back completely and the installation stays in `recovery_required` (fail closed).

**What is not repaired.** Scope rules, program rules and policy values are restored as they were in the backup. They may be older or newer than what the researcher last had. The reconciliation cannot know. The alert and the recovery page tell the researcher to review the scope, the policy and the lab list before validating again. Every authorization that depends on them was invalidated, so nothing runs on them until a new validation and approvals exist.

## 7. Crash behavior

| Crash point | Next start |
| --- | --- |
| Before phase 1 commits | Nothing changed. Still `recovery_required`. |
| After phase 1, before the marker write | Row 7. Resume phase 2 and 3. All invalidation is already committed, so resuming grants nothing. |
| After the marker write, before phase 3 | Row 9. Complete phase 3. |
| After phase 3 | Normal. |
| Marker temporary file left behind | Ignored, removed at startup. The rename is the commit. |

A restored database that itself contains `state = reconciling` also resumes through rows 7 or 9, but resumption only writes the marker and the state. The invalidation it relies on is already in that database.

## 8. First-run initialization

When row 1 applies, the server generates `instance_id`, inserts `recovery_state` with generation 1 and `state = 'normal'`, writes the marker with generation 1 and the current watermark, and then creates the bootstrap secret (D-68). If the marker write fails the server does not create the bootstrap secret. First run is the only time the system creates both halves. After that, a missing half means `recovery_required`.

## 9. Operator-visible state and procedure

- `GET /session` returns `state: "recovery_required"` and a `recovery` object (reason, marker generation, database generation, detection time). Every other route returns 503 `recovery_required`.
- The web application shows one recovery page that explains the reason in plain words and lists what will be revoked (the table in section 6). The researcher confirms with the password.
- After success `GET /integrations/status` shows an open alert of kind `restore_recovered` until the researcher clears it (INT-02) after reviewing scope and policy.
- Restore procedure for the researcher: stop the server and worker, restore the database dump and the evidence directory, do not touch `~/.local/state/morningstar`, start the server, open the recovery page.

## 10. Threat boundary and limitations

This mechanism protects against an honest or accidental restore of the PostgreSQL database, a disk-level revert of the cluster directory only, and loss of the database. It does not protect against any of these, and nothing here claims it does:

- An attacker running as the same user, who can edit the marker and the database together.
- A malicious administrator or root.
- A restore that also restores the marker, such as a snapshot of the whole home directory or a virtual-machine snapshot revert.
- A change of the system clock.
- Events committed after the last marker write (section 4).

The marker is not cryptographically protected. An HMAC keyed from the Secret Service was considered and not adopted for the MVP: it makes startup depend on the keyring (a fail-closed dependency the MVP accepts only for credential features) and a same-user attacker can read the key anyway.

## 11. Required tests (gate G-recovery)

1. Restore an older dump with the marker kept: detected with `database_older` (rows 6 and 10).
2. Delete the marker with a populated database: `marker_missing`.
3. Corrupt the marker, change its mode, replace it with a symlink: `marker_invalid`.
4. Swap in another installation's marker: `instance_mismatch`.
5. Restore an old marker with a newer database: `marker_older`.
6. Crash after each phase of the reconciliation: correct resume, no authority granted.
7. A worker holding a claim loop before the restore cannot claim afterwards (CC-15).
8. After reconciliation no approval, validation, lab, session or queued job from the backup is usable, and no approval use count changed.
9. A failed audit insert leaves the installation in `recovery_required`.
10. First run initializes both halves and a second start is normal.
11. The marker watermark is advanced before a launch and a failed marker write stops claims.
12. A concurrent write by server and worker never lowers the watermark.
