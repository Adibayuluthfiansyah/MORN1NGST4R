# Endpoint classification

Status: design proposal. It classifies every endpoint without changing its ID, method, path, authorization, CSRF, re-authentication, audit event or PRD traceability (those are in `endpoint-specification.md`).

## Classes

| Class | Meaning |
| --- | --- |
| Security-critical | Enforces or changes an authentication, authorization, scope, approval, verification, data-egress or integrity invariant. Every route that consumes a re-authentication grant is in this class. |
| MVP required | Needed by the MVP acceptance scenario and built before milestone M20. |
| Later milestone | Needed by the MVP acceptance scenario and built in milestone M20 or later (inventory, assessment lifecycle, jobs, findings, AI analysis). |
| Deferred | Not in the MVP. Not in `openapi.yaml`. The ID stays reserved. |
| Internal only | A worker use case that is never an HTTP route. Listed in `endpoint-inventory.md`. |

Precedence when more than one applies: Deferred, then Security-critical, then Later milestone, then MVP required. The milestone is the one that first needs the route (`.kiro/specs/mvp/plan.md`). No endpoint was removed to reduce the count. Two were deferred because their information is available elsewhere (see the reasons).

## Counts

| Class | Endpoints |
| --- | --- |
| MVP required | 46 |
| Security-critical | 49 |
| Later milestone | 23 |
| Deferred | 3 |
| Internal only | 9 |
| **Active in OpenAPI** | **118** |

## Per endpoint

| ID | Method | Path | Class | Milestone | Redundancy or note |
| --- | --- | --- | --- | --- | --- |
| AUTH-01 | GET | `/session` | Security-critical | M4 |  |
| AUTH-02 | POST | `/setup/account` | Security-critical | M4 |  |
| AUTH-03 | POST | `/session/login` | Security-critical | M4 |  |
| AUTH-04 | POST | `/session/logout` | Security-critical | M4 |  |
| AUTH-05 | GET | `/sessions` | Security-critical | M4 | Needed for session revocation (SEC 8). Not redundant. |
| AUTH-06 | POST | `/sessions/{sessionId}/revoke` | Security-critical | M4 |  |
| AUTH-07 | POST | `/session/reauthenticate` | Security-critical | M4 |  |
| AUTH-08 | POST | `/recovery/reset` | Security-critical | M4 |  |
| AUTH-10 | POST | `/system/recovery/reconcile` | Security-critical | M4 |  |
| AUTH-09 | POST | `/account/password` | Security-critical | M4 |  |
| POL-01 | GET | `/policy/settings` | MVP required | M16 |  |
| POL-02 | PUT | `/policy/settings/{settingKey}` | Security-critical | M16 |  |
| POL-03 | GET | `/policy/settings/{settingKey}/versions` | Deferred | M16 | Redundant with the audit trail (every change records old and new value, D-53). Deferred. |
| PRG-01 | POST | `/programs` | MVP required | M16 |  |
| PRG-02 | GET | `/programs` | MVP required | M16 |  |
| PRG-03 | GET | `/programs/{programId}` | MVP required | M16 |  |
| PRG-04 | PATCH | `/programs/{programId}` | MVP required | M16 |  |
| PRG-05 | POST | `/programs/{programId}/archive` | Security-critical | M16 |  |
| PRG-06 | POST | `/programs/{programId}/rules-versions` | Security-critical | M16 |  |
| PRG-07 | GET | `/programs/{programId}/rules-versions` | MVP required | M16 | Needed to show rule versions (FR-PROG-002). |
| TGT-01 | POST | `/programs/{programId}/targets` | MVP required | M16 |  |
| TGT-02 | GET | `/programs/{programId}/targets` | MVP required | M16 |  |
| TGT-03 | GET | `/targets/{targetId}` | MVP required | M16 | Could be served by the list. Kept for deep links and for the ownership check path. |
| TGT-04 | PATCH | `/targets/{targetId}` | MVP required | M16 |  |
| TGT-05 | POST | `/targets/{targetId}/archive` | Security-critical | M16 |  |
| SCP-01 | GET | `/programs/{programId}/scope` | MVP required | M16 |  |
| SCP-02 | POST | `/programs/{programId}/scope-versions` | Security-critical | M16 |  |
| SCP-03 | GET | `/programs/{programId}/scope-versions` | MVP required | M16 |  |
| SCP-04 | GET | `/scope-versions/{scopeVersionId}` | MVP required | M16 | Overlaps SCP-01 plus SCP-03. Kept for stable links from audit events and jobs to one version. |
| SCP-05 | POST | `/programs/{programId}/scope-evaluations` | Security-critical | M16 |  |
| SCP-06 | GET | `/programs/{programId}/refused-exclusions` | MVP required | M16 |  |
| SCP-07 | POST | `/refused-exclusions/{refusedExclusionId}/acknowledge` | Security-critical | M16 |  |
| LAB-01 | POST | `/assessments/{assessmentId}/lab-registrations` | Security-critical | M17 |  |
| LAB-02 | GET | `/assessments/{assessmentId}/lab-registrations` | MVP required | M17 |  |
| LAB-03 | GET | `/lab-registrations/{labRegistrationId}` | MVP required | M17 | Same as TGT-03. |
| LAB-04 | POST | `/lab-registrations/{labRegistrationId}/revoke` | Security-critical | M17 |  |
| VAL-01 | POST | `/assessments/{assessmentId}/target-validations` | Security-critical | M18 |  |
| VAL-02 | GET | `/assessments/{assessmentId}/target-validations` | MVP required | M18 |  |
| VAL-03 | GET | `/target-validations/{validationId}` | MVP required | M18 | Needed: it returns the per-target results the list omits. |
| ASM-01 | POST | `/programs/{programId}/assessments` | MVP required | M16 |  |
| ASM-02 | GET | `/programs/{programId}/assessments` | MVP required | M16 |  |
| ASM-03 | GET | `/assessments/{assessmentId}` | MVP required | M16 |  |
| ASM-04 | PATCH | `/assessments/{assessmentId}` | MVP required | M16 |  |
| ASM-05 | PUT | `/assessments/{assessmentId}/targets` | Security-critical | M16 |  |
| ASM-06 | PUT | `/assessments/{assessmentId}/raw-evidence-capture` | Security-critical | M21 |  |
| ASM-07 | POST | `/assessments/{assessmentId}/ready` | Later milestone | M21 |  |
| ASM-08 | POST | `/assessments/{assessmentId}/start` | Later milestone | M21 |  |
| ASM-09 | POST | `/assessments/{assessmentId}/pause` | Later milestone | M21 |  |
| ASM-10 | POST | `/assessments/{assessmentId}/resume` | Later milestone | M21 |  |
| ASM-11 | POST | `/assessments/{assessmentId}/complete` | Later milestone | M21 |  |
| ASM-12 | POST | `/assessments/{assessmentId}/cancel` | Security-critical | M21 |  |
| ASM-13 | GET | `/assessments/{assessmentId}/status` | Later milestone | M21 | Computed view for the dashboard. Not redundant. |
| PLN-01 | POST | `/assessments/{assessmentId}/plan-items` | Security-critical | M19 |  |
| PLN-02 | GET | `/assessments/{assessmentId}/plan-items` | MVP required | M19 |  |
| PLN-03 | GET | `/plan-items/{planItemId}` | MVP required | M19 |  |
| PLN-04 | POST | `/plan-items/{planItemId}/reject` | MVP required | M19 |  |
| APR-01 | POST | `/plan-items/{planItemId}/approvals` | Security-critical | M19 |  |
| APR-02 | GET | `/assessments/{assessmentId}/approvals` | MVP required | M19 |  |
| APR-03 | GET | `/approvals/{approvalId}` | MVP required | M19 |  |
| APR-04 | POST | `/approvals/{approvalId}/approve` | Security-critical | M19 |  |
| APR-05 | POST | `/approvals/{approvalId}/reject` | Security-critical | M19 |  |
| APR-06 | POST | `/approvals/{approvalId}/revoke` | Security-critical | M19 |  |
| APR-07 | GET | `/approvals/{approvalId}/events` | MVP required | M19 | Could be embedded in APR-03. Kept because the history is append-only and can be long. |
| JOB-01 | POST | `/plan-items/{planItemId}/jobs` | Security-critical | M21 |  |
| JOB-02 | GET | `/assessments/{assessmentId}/jobs` | Later milestone | M21 |  |
| JOB-03 | GET | `/jobs/{jobId}` | Later milestone | M21 |  |
| JOB-04 | POST | `/jobs/{jobId}/cancel` | Security-critical | M21 |  |
| JOB-05 | GET | `/jobs/{jobId}/attempts` | Later milestone | M21 |  |
| JOB-06 | GET | `/jobs/{jobId}/transitions` | Later milestone | M21 | Could be embedded in JOB-03. Kept for the same reason. |
| AST-01 | GET | `/programs/{programId}/assets` | Later milestone | M20 |  |
| AST-02 | GET | `/assets/{assetId}` | Later milestone | M20 |  |
| AST-03 | GET | `/programs/{programId}/endpoints` | Later milestone | M20 |  |
| AST-04 | GET | `/endpoints/{endpointId}` | Later milestone | M20 |  |
| AST-05 | GET | `/assets/{assetId}/observations` | Later milestone | M20 |  |
| AST-06 | POST | `/assets/{assetId}/promote` | Security-critical | M20 |  |
| EVD-01 | GET | `/assessments/{assessmentId}/evidence` | MVP required | M6 |  |
| EVD-02 | GET | `/evidence/{evidenceId}` | MVP required | M6 |  |
| EVD-03 | POST | `/evidence/{evidenceId}/integrity-checks` | Security-critical | M6 |  |
| EVD-04 | GET | `/evidence/{evidenceId}/integrity-checks` | MVP required | M6 | Could be embedded in EVD-02. Kept because check history is append-only. |
| EVD-05 | GET | `/evidence/{evidenceId}/content` | Security-critical | M6 |  |
| FND-01 | POST | `/assessments/{assessmentId}/findings` | Later milestone | M22 |  |
| FND-02 | GET | `/assessments/{assessmentId}/findings` | Later milestone | M22 |  |
| FND-03 | GET | `/findings/{findingId}` | Later milestone | M22 |  |
| FND-04 | POST | `/findings/{findingId}/submit-for-review` | Security-critical | M22 |  |
| FND-05 | POST | `/findings/{findingId}/review-outcome` | Security-critical | M22 |  |
| FND-06 | POST | `/findings/{findingId}/verify` | Security-critical | M22 |  |
| FND-07 | POST | `/findings/{findingId}/reopen` | Security-critical | M22 |  |
| FND-08 | GET | `/findings/{findingId}/transitions` | Later milestone | M22 | Same as JOB-06. |
| FND-09 | POST | `/findings/{findingId}/evidence` | Later milestone | M22 |  |
| FND-10 | GET | `/findings/{findingId}/evidence` | Later milestone | M22 | Could be embedded in FND-03. Kept because evidence lists are large. |
| FND-11 | POST | `/findings/{findingId}/validations` | Security-critical | M22 |  |
| FND-12 | GET | `/findings/{findingId}/validations` | Later milestone | M22 |  |
| FND-13 | POST | `/findings/{findingId}/validations/replay` | Deferred | M22b |  |
| RPT-01 | POST | `/assessments/{assessmentId}/reports` | MVP required | M14 |  |
| RPT-02 | GET | `/assessments/{assessmentId}/reports` | MVP required | M14 |  |
| RPT-03 | GET | `/reports/{reportId}` | MVP required | M14 |  |
| RPT-04 | PUT | `/reports/{reportId}/findings` | Security-critical | M14 |  |
| RPT-05 | POST | `/reports/{reportId}/generate` | MVP required | M14 |  |
| RPT-06 | POST | `/reports/{reportId}/finalize` | MVP required | M14 |  |
| RPT-07 | GET | `/reports/{reportId}/content` | Security-critical | M14 |  |
| AI-01 | GET | `/ai/provider-kinds` | Deferred | M12 | Adapter kinds are an enum in the OpenAPI schema and fixed in code. A static list needs no route. Deferred. |
| AI-02 | POST | `/ai/providers` | Security-critical | M12 |  |
| AI-03 | GET | `/ai/providers` | MVP required | M12 |  |
| AI-04 | GET | `/ai/providers/{providerId}` | MVP required | M12 |  |
| AI-05 | PATCH | `/ai/providers/{providerId}` | MVP required | M12 |  |
| AI-06 | POST | `/ai/providers/{providerId}/tests` | Security-critical | M12 |  |
| AI-07 | POST | `/programs/{programId}/ai-providers/{providerId}/enable` | Security-critical | M12 |  |
| AI-08 | POST | `/programs/{programId}/ai-providers/{providerId}/disable` | MVP required | M12 |  |
| AI-09 | GET | `/programs/{programId}/ai-transmissions` | MVP required | M12 |  |
| AI-10 | POST | `/assessments/{assessmentId}/ai-tasks` | Security-critical | M23 |  |
| AI-11 | GET | `/assessments/{assessmentId}/ai-outputs` | Later milestone | M23 |  |
| TLS-01 | GET | `/tools` | MVP required | M10 |  |
| TLS-02 | PUT | `/tools/{toolKey}` | Security-critical | M10 |  |
| TLS-03 | POST | `/tools/{toolKey}/verify` | Security-critical | M10 |  |
| TLS-04 | POST | `/tools/{toolKey}/enable` | Security-critical | M10 |  |
| TLS-05 | GET | `/programs/{programId}/passive-sources` | MVP required | M11 |  |
| TLS-06 | POST | `/programs/{programId}/passive-sources/{sourceKey}/enable` | Security-critical | M11 |  |
| TLS-07 | POST | `/programs/{programId}/passive-sources/{sourceKey}/disable` | MVP required | M11 | Symmetry with TLS-06. Kept because revocation of a disclosure must exist. |
| INT-01 | GET | `/integrations/status` | MVP required | M10 |  |
| INT-02 | POST | `/system-alerts/{alertId}/clear` | Security-critical | M10 |  |
| AUD-01 | GET | `/audit-events` | MVP required | M5 |  |

## Internal use cases

| ID | Operation | Milestone |
| --- | --- | --- |
| WRK-01 | Claim a job | M7 |
| WRK-02 | Heartbeat and renew lease | M7 |
| WRK-03 | Record attempt launch (E3) | M8 |
| WRK-04 | Commit results | M11 |
| WRK-05 | Confirm termination | M7 |
| WRK-06 | Observe stop request and acknowledge | M7 |
| WRK-07 | Lease reaper | M7 |
| WRK-08 | Expiry and retention sweeps | M7 |
| WRK-09 | Write AI transmission record | M12 |

## Redundant or unnecessary CRUD

There is no generic CRUD. The model has no DELETE routes (DB-05), no PUT replacement of whole resources except sets and settings, and no routes for tables that are only reachable through another resource (`policy_decisions`, `approval_uses`, `job_transitions` through JOB-06, `finding_verification_evidence`, `target_validation_targets`). The candidates for later consolidation are the "get one" routes that duplicate an embedded representation (SCP-04, TGT-03, LAB-03, APR-07, JOB-06, FND-08, FND-10, EVD-04). They are kept because each carries a stable link target and an ownership check, and because the history endpoints are append-only and can be long.

