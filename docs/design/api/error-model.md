# Error model

Status: design proposal.

## Shape

Every error response is `application/json` with this body and the same `X-Request-Id` header. There is no other error format. The two health probes (`docs/decisions/0004-health-endpoints-mvp-decision.md`) answer with a plain status document such as `{"status":"not_ready"}` on both 200 and 503. That is a status document, not an error response. Every other failure of those routes uses this envelope.

```json
{
  "error": {
    "code": "validation_failed",
    "message": "One or more fields are invalid.",
    "requestId": "018f0000-0000-7000-8000-000000000002",
    "retryable": false,
    "fieldErrors": [ { "field": "name", "code": "required", "message": "name is required" } ],
    "details": {}
  }
}
```

- `code`: stable, lower snake case, never removed or reused in `v1`. Clients branch on it.
- `message`: short, safe, fixed text per code. No user input is echoed except a field name.
- `requestId`: the request identifier, also written to the audit trail and the log.
- `retryable`: true only for `rate_limited`, `database_unavailable` and `provider_unreachable`. It never means a security denial can be retried into success.
- `fieldErrors`: present for `validation_failed` and `scope_rule_refused`. Each item has the JSON pointer-style field name, a code and a fixed message.
- `details`: only stable reason codes, for example the `reasons` of a policy denial. Never free text from a tool, a target or a provider.

## Never exposed

Stack traces, SQL text or constraint names, filesystem paths, secrets or hashes, internal tokens, session identifiers, sensitive provider details (endpoint userinfo, model responses), raw tool output, resolved addresses of non-lab targets, and the existence of a resource in another program. A missing resource and a resource the session may not see both return `not_found`.

## Status mapping

| HTTP | Meaning | Codes |
| --- | --- | --- |
| 400 | Malformed request | `invalid_request` |
| 401 | Authentication | `bootstrap_secret_invalid`, `invalid_credentials`, `recovery_invalid`, `session_expired`, `unauthenticated` |
| 403 | Authorization and browser protection | `csrf_invalid`, `forbidden`, `host_denied`, `origin_denied`, `policy_denied`, `reauth_invalid`, `reauth_required` |
| 404 | Not found or not visible | `not_found`, `source_unknown`, `tool_unknown` |
| 405 | Method not allowed | `method_not_allowed` |
| 409 | State conflict | `account_exists`, `alert_cause_unresolved`, `approval_exhausted`, `approval_expired`, `approval_required`, `approval_revoked`, `asset_not_in_scope`, `attestation_text_outdated`, `conflict_state`, `disclosure_mismatch`, `duplicate_finding`, `evidence_integrity_failed`, `evidence_required`, `exclusion_unacknowledged`, `finding_not_verified`, `idempotency_conflict`, `jobs_still_running`, `numeric_limits_missing`, `operation_hash_mismatch`, `operation_not_approvable`, `program_archived`, `program_has_active_assessments`, `provider_not_enabled`, `recovery_not_required`, `report_not_generated`, `scope_decision_stale`, `setup_unavailable`, `target_in_use`, `target_set_hash_mismatch`, `tool_not_verified`, `validation_required`, `validation_stale`, `version_conflict` |
| 410 | Gone | `evidence_purged` |
| 413 | Too large | `payload_too_large` |
| 415 | Wrong media type | `unsupported_media_type` |
| 422 | Validation or ambiguity | `endpoint_invalid`, `lab_ports_required`, `lab_range_forbidden`, `operation_invalid`, `policy_value_invalid`, `reauth_subject_invalid`, `scope_rule_refused`, `target_ambiguous`, `target_invalid`, `target_not_in_assessment`, `target_not_in_scope`, `target_not_registered`, `validation_failed` |
| 429 | Rate limit | `rate_limited` |
| 500 | Internal | `internal_error` |
| 502 | Upstream provider | `provider_unreachable` |
| 503 | Dependency unavailable | `credential_store_unavailable`, `database_unavailable`, `isolation_unavailable`, `recovery_required` |

## Catalog

| Code | HTTP | Message meaning | Used by |
| --- | --- | --- | --- |
| `invalid_request` | 400 | The request could not be parsed or a header is malformed. | AUTH-02, AUTH-03, AUTH-07, AUTH-08, AUTH-10, AUTH-09 and 55 more |
| `unauthenticated` | 401 | No valid session. | AUTH-04, AUTH-05, AUTH-06, AUTH-07, AUTH-09, POL-01 and 110 more |
| `session_expired` | 401 | The session expired or was revoked. | AUTH-04, AUTH-05, AUTH-06, AUTH-07, AUTH-09, POL-01 and 110 more |
| `invalid_credentials` | 401 | Credentials were not accepted. The message never says which part failed. | AUTH-03, AUTH-07, AUTH-10, AUTH-09 |
| `recovery_invalid` | 401 | The recovery code was not accepted. | AUTH-08 |
| `forbidden` | 403 | The session may not perform this action on this resource. | AUTH-04, AUTH-05, AUTH-06, AUTH-07, AUTH-09, POL-01 and 110 more |
| `csrf_invalid` | 403 | The CSRF token is missing or wrong. | AUTH-02, AUTH-03, AUTH-04, AUTH-06, AUTH-07, AUTH-08 and 60 more |
| `origin_denied` | 403 | The Origin header is missing or not allowed. | AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06, AUTH-07 and 114 more |
| `host_denied` | 403 | The Host header is not allowed. | AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06 and 115 more |
| `reauth_required` | 403 | A fresh password entry bound to this action is required. | AUTH-09, POL-02, SCP-02, SCP-07, LAB-01, ASM-06 and 7 more |
| `reauth_invalid` | 403 | The re-authentication grant is expired, consumed or bound to another action. | AUTH-09, POL-02, SCP-02, SCP-07, LAB-01, ASM-06 and 7 more |
| `policy_denied` | 403 | The policy engine denied the operation. Reasons are stable codes. | JOB-01, FND-13 |
| `not_found` | 404 | The resource does not exist or is not visible to this session. | AUTH-06, POL-02, POL-03, PRG-03, PRG-04, PRG-05 and 97 more |
| `method_not_allowed` | 405 | The method is not allowed for this path. The response carries an `Allow` header that lists the permitted methods. | Router level: a known path called with a method it does not serve. No inventory endpoint returns it by design. |
| `conflict_state` | 409 | The resource is not in a state that allows this transition. | AUTH-02, AUTH-06, AUTH-08, AUTH-10, AUTH-09, POL-02 and 55 more |
| `version_conflict` | 409 | If-Match or expected version did not match. | PRG-04, TGT-04, SCP-02, ASM-04, AI-05 |
| `idempotency_conflict` | 409 | The Idempotency-Key was reused with a different request. | PRG-01, PRG-06, TGT-01, SCP-02, LAB-01, VAL-01 and 10 more |
| `program_archived` | 409 | The program is archived and read-only. | PRG-04, PRG-05, PRG-06, TGT-01, TGT-04, TGT-05 and 42 more |
| `validation_stale` | 409 | The target authorization validation is stale or expired. | ASM-07, ASM-08, ASM-10, APR-01, APR-04, JOB-01 |
| `exclusion_unacknowledged` | 409 | A refused exclusion is open. | VAL-01, ASM-07 |
| `approval_required` | 409 | The operation needs an approved approval. | JOB-01, FND-13 |
| `approval_expired` | 409 | The approval expired. Expiry stops new launches only. | JOB-01 |
| `approval_revoked` | 409 | The approval was revoked. | JOB-01 |
| `approval_exhausted` | 409 | The approval has no remaining uses. | JOB-01 |
| `operation_hash_mismatch` | 409 | The operation shown is not the operation stored. | APR-04 |
| `evidence_integrity_failed` | 409 | The evidence hash check failed or the content is missing. | EVD-05, FND-06 |
| `evidence_purged` | 410 | The evidence content was purged. | EVD-03, EVD-05 |
| `payload_too_large` | 413 | The body exceeds the route limit. | AUTH-02, AUTH-03, AUTH-07, AUTH-08, AUTH-10, AUTH-09 and 55 more |
| `unsupported_media_type` | 415 | Only application/json is accepted on this route. | AUTH-02, AUTH-03, AUTH-07, AUTH-08, AUTH-10, AUTH-09 and 55 more |
| `validation_failed` | 422 | One or more fields are invalid. fieldErrors lists them. | AUTH-02, AUTH-03, AUTH-07, AUTH-08, AUTH-10, AUTH-09 and 55 more |
| `target_ambiguous` | 422 | The target has more than one canonical form or cannot be parsed. It is denied. | TGT-01 |
| `rate_limited` | 429 | Too many requests. Retry-After is set. | AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06 and 115 more |
| `recovery_required` | 503 | The installation is in the recovery_required state after a detected restore. Only GET /session and the reconcile route work. | AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06, AUTH-07 and 113 more |
| `internal_error` | 500 | An unexpected failure. Nothing internal is exposed. | AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06 and 115 more |
| `database_unavailable` | 503 | The database is unavailable. No operation was started. | AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06 and 115 more |
| `credential_store_unavailable` | 503 | The Secret Service is unavailable. Credential features are disabled. | AI-02, AI-10 |
| `isolation_unavailable` | 503 | A required isolation layer is unavailable. No tool runs. | TLS-03 |
| `setup_unavailable` | 409 | Endpoint-specific condition, see the endpoint specification. | AUTH-02, AUTH-03, AUTH-08 |
| `bootstrap_secret_invalid` | 401 | Endpoint-specific condition, see the endpoint specification. | AUTH-02 |
| `account_exists` | 409 | Endpoint-specific condition, see the endpoint specification. | AUTH-02 |
| `reauth_subject_invalid` | 422 | Endpoint-specific condition, see the endpoint specification. | AUTH-07 |
| `policy_value_invalid` | 422 | Endpoint-specific condition, see the endpoint specification. | POL-02 |
| `program_has_active_assessments` | 409 | Endpoint-specific condition, see the endpoint specification. | PRG-05 |
| `target_invalid` | 422 | Endpoint-specific condition, see the endpoint specification. | TGT-01 |
| `target_in_use` | 409 | Endpoint-specific condition, see the endpoint specification. | TGT-05 |
| `scope_rule_refused` | 422 | Endpoint-specific condition, see the endpoint specification. | SCP-02 |
| `target_set_hash_mismatch` | 409 | Endpoint-specific condition, see the endpoint specification. | SCP-07 |
| `lab_range_forbidden` | 422 | Endpoint-specific condition, see the endpoint specification. | LAB-01 |
| `lab_ports_required` | 422 | Endpoint-specific condition, see the endpoint specification. | LAB-01 |
| `target_not_in_scope` | 422 | Endpoint-specific condition, see the endpoint specification. | LAB-01, VAL-01 |
| `attestation_text_outdated` | 409 | Endpoint-specific condition, see the endpoint specification. | VAL-01 |
| `jobs_still_running` | 409 | Endpoint-specific condition, see the endpoint specification. | ASM-11 |
| `target_not_registered` | 422 | Endpoint-specific condition, see the endpoint specification. | ASM-05 |
| `target_not_in_assessment` | 422 | Endpoint-specific condition, see the endpoint specification. | PLN-01 |
| `operation_invalid` | 422 | Endpoint-specific condition, see the endpoint specification. | PLN-01 |
| `operation_not_approvable` | 409 | Endpoint-specific condition, see the endpoint specification. | APR-01 |
| `numeric_limits_missing` | 409 | Endpoint-specific condition, see the endpoint specification. | APR-01 |
| `asset_not_in_scope` | 409 | Endpoint-specific condition, see the endpoint specification. | AST-06 |
| `scope_decision_stale` | 409 | Endpoint-specific condition, see the endpoint specification. | AST-06 |
| `duplicate_finding` | 409 | Endpoint-specific condition, see the endpoint specification. | FND-01 |
| `validation_required` | 409 | Endpoint-specific condition, see the endpoint specification. | FND-06 |
| `evidence_required` | 409 | Endpoint-specific condition, see the endpoint specification. | FND-06 |
| `finding_not_verified` | 409 | Endpoint-specific condition, see the endpoint specification. | RPT-04 |
| `report_not_generated` | 409 | Endpoint-specific condition, see the endpoint specification. | RPT-06, RPT-07 |
| `endpoint_invalid` | 422 | Endpoint-specific condition, see the endpoint specification. | AI-02 |
| `provider_unreachable` | 502 | Endpoint-specific condition, see the endpoint specification. | AI-06 |
| `disclosure_mismatch` | 409 | Endpoint-specific condition, see the endpoint specification. | AI-07, TLS-06 |
| `provider_not_enabled` | 409 | Endpoint-specific condition, see the endpoint specification. | AI-10 |
| `tool_unknown` | 404 | Endpoint-specific condition, see the endpoint specification. | TLS-02 |
| `tool_not_verified` | 409 | Endpoint-specific condition, see the endpoint specification. | TLS-04 |
| `source_unknown` | 404 | Endpoint-specific condition, see the endpoint specification. | TLS-06 |
| `recovery_not_required` | 409 | Endpoint-specific condition, see the endpoint specification. | AUTH-10 |
| `alert_cause_unresolved` | 409 | Endpoint-specific condition, see the endpoint specification. | INT-02 |

## Handling rules

| Case | Handling |
| --- | --- |
| Invalid input | 400 `invalid_request` for a body that does not parse, 422 `validation_failed` with `fieldErrors` for a parsed body that breaks a rule. Unknown JSON fields are rejected (`additionalProperties: false`). |
| Authentication failure | 401 `unauthenticated` or `session_expired`. A login failure is always `invalid_credentials` with the same message and timing class. |
| Authorization failure | 403 `forbidden` for a known but forbidden action. 404 `not_found` for another owner's resource or an unknown ID. |
| Out-of-scope target | A scope preview returns decision `deny` inside a 200 result. An operation that would touch it returns 403 `policy_denied` with `details.reasons` such as `excluded`, `no_matching_rule`. It is audited. |
| Ambiguous target | 422 `target_ambiguous`. The input is denied and never repaired (D-34, S1.1). |
| Expired approval | 409 `approval_expired`. Expiry stops new launches only and never stops a launched process (D-48). |
| Revoked approval | 409 `approval_revoked`. |
| Conflicting state transition | 409 `conflict_state`. The body names no internal state beyond the current public status. |
| Rate limit | 429 `rate_limited` with `Retry-After`. |
| Resource not found | 404 `not_found`. |
| Method not allowed | 405 `method_not_allowed` with an `Allow` header, in the standard envelope. It is raised by the router before any handler, and a Host outside the allow-list is still denied first (403 `host_denied`). |
| Evidence integrity failure | 409 `evidence_integrity_failed`. The check result is recorded, the evidence is marked, an alert is raised, and the content is withheld unless the human acknowledges (DQ-15). |
| Database unavailable | 503 `database_unavailable`, `retryable: true`. No operation starts. There is no in-memory fallback (architecture section 17). |
| Internal failure | 500 `internal_error`. The cause is in the log under the same `requestId`, redacted. |
