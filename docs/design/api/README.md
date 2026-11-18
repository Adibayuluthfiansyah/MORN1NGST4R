# API contract design

Status: design proposal for owner review. **No route, handler, service, repository or frontend code exists.** The OpenAPI file is a contract to implement against, not a record of an implementation. Design decisions carry `API-nn` IDs. Open questions carry `DQ-nn` IDs and are listed in `../design-consistency-review.md`. Approved decisions (`D-nn`) are cited and not changed.

## Contents

| File | Content |
| --- | --- |
| `conventions.md` | Base path, naming, pagination, filtering, sorting, idempotency, request ids, envelopes, formats, limits, timeouts |
| `endpoint-inventory.md` | Every endpoint by domain, deferred endpoints, internal worker use cases |
| `endpoint-specification.md` | Per-endpoint specification (authentication, authorization, CSRF, schemas, status and error codes, idempotency, audit, rate limit, security notes, PRD ids) |
| `authentication-and-authorization.md` | Session, cookie, CSRF, Origin, Host, CORS, rate limits, re-authentication, bootstrap, recovery, the authorization chain |
| `endpoint-classification.md` | Class and first milestone of every endpoint, deferred endpoints with reasons |
| `error-model.md` | The error shape, the code catalog and handling rules |
| `database-mapping.md` | Endpoint to PRD, entities, authorization checks, audit event, transaction boundary, idempotency |
| `openapi.yaml` | OpenAPI 3.1 contract |

## Summary

- 118 endpoints in OpenAPI across 14 domains, 140 schemas, 3 deferred endpoints (not in OpenAPI), 9 internal worker use cases that are never routes.
- Route classes: public pre-session (5), authenticated read (53), authenticated state change (47), state change that consumes a re-authentication grant (13). Counts by domain are in the inventory, counts by scope class in `endpoint-classification.md`.
- Authentication is the server-side session only. No API token exists (D-60). The human is the only actor. AI, the worker and tools cannot reach the API (API-17).
- Every non-GET route needs the CSRF token and an allow-listed Origin. Every state-changing route has exactly one audit event.

## Design decisions

| ID | Decision | Why |
| --- | --- | --- |
| API-01 | Base path `/api/v1`. The version is in the path. A breaking change makes `/api/v2`. Adding optional fields, endpoints or error codes is not breaking. | Simple for a generated client and for the Next.js rewrite. |
| API-02 | Resource names are plural nouns in kebab-case paths. JSON fields are lowerCamelCase. A security-sensitive lifecycle transition is an explicit action `POST /{resource}/{id}/{verb}` (approve, revoke, verify, reopen, start, pause, cancel). Pure data changes use `PATCH` or `PUT`. | An action endpoint states intent, takes its own audit event and its own state precondition. |
| API-03 | A single resource is returned as the bare object. A collection is `{ "items": [...], "page": {...} }`. An error is `{ "error": {...} }`. The request id is the `X-Request-Id` header and the error body. | No wrapper to unwrap in the generated client. Lists need metadata, objects do not. |
| API-04 | Authentication is the server-side session cookie `ms_session`. CSRF is a per-session token returned in the body of `GET /session` and sent in `X-CSRF-Token` with an allow-listed `Origin` on every non-GET request. Public state-changing routes use a short-lived pre-session cookie `ms_anon` and its token. | D-33, D-60. SameSite is not relied on (T-54, T-57). |
| API-05 | Collections are created and listed under their parent (`/programs/{programId}/targets`). Items are addressed flat (`/targets/{targetId}`). The server derives the program from the loaded row. A client never supplies a program for an item. | Closes BOLA: there is no second parent id to forge or mismatch. |
| API-06 | Pagination is keyset with an opaque `cursor`, `limit` default 50 and maximum 200. The order key is the sort field plus `id`. Offset pagination is not offered. | Stable under inserts, cheap on the indexes of the dictionary. |
| API-07 | Sorting is `sort=createdAt` or `sort=-createdAt` (default `-createdAt`) from a per-route allow-list. Filters are named query parameters with exact-match semantics, plus `q` (case-insensitive prefix) on asset lists only. | Predictable query plans. No free-form query language. |
| API-08 | Creation and long-running requests that are not naturally idempotent carry a required `Idempotency-Key` (16 to 64 URL-safe characters). The key is stored 24 hours with a hash of the canonical body. Same key and body returns the original status and the resource with `Idempotency-Replayed: true`. Same key with another body returns 409 `idempotency_conflict`. | A retried click must not create two programs or two jobs. |
| API-09 | Mutable resources carry `version`. `PATCH` and `PUT` on them require `If-Match: "<version>"`. A mismatch is 409 `version_conflict`. State-guarded actions use the state precondition instead. | Lost-update protection with the optimistic counter of the data dictionary. |
| API-10 | Timestamps are RFC 3339 in UTC with a `Z` suffix. UUIDs are lowercase canonical text, opaque to clients. Digests are lowercase hex SHA-256. | Unambiguous and matches `timestamptz` and `bytea` digests. |
| API-11 | Request size limits (proposed): 16 KiB on authentication routes, 64 KiB default, 1 MiB for scope versions and report inclusion. Larger bodies get 413. Downloads stream and have no body limit. | Request size limits are required (SEC 11). Numbers need review (DQ-06). |
| API-12 | Server timeouts (proposed): read-header 5 s, read 15 s, write 30 s, idle 60 s, handler budget 25 s. Downloads extend the write deadline per chunk. No request waits for a tool run. Long work is a job that the client polls (P-12). | Avoids slow-client exhaustion and keeps the API from owning process lifetime. |
| API-13 | Every request has an id. The server always generates it (a UUID) and ignores an `X-Request-Id` that the client sends, because the id is the correlation id of the audit trail and a caller must not choose it (M5, TD-M4-03). It is returned, written to the audit trail and the structured log. | Traceability of actor, operation and target. |
| API-14 | Rate-limit classes RL-AUTH, RL-READ, RL-WRITE, RL-PREVIEW, RL-DOWNLOAD, RL-AITEST with the numbers in `endpoint-specification.md`. Limits are keyed by session, and by client key for public routes. A refusal is 429 with `Retry-After`. | D-68 sets 5 failures per 15 minutes for authentication. The other numbers are proposals (DQ-06). |
| API-15 | Request pipeline order: Host check, size and media type, rate limit, session, Origin and CSRF, resource load and ownership, state precondition, re-authentication grant, policy engine, transaction. Any failure ends the request. Unknown, ambiguous and unowned resources are denied (404 or 403). | Fail closed, cheapest checks first, authorization before state so that state is not leaked. |
| API-16 | Worker operations are not routes. The worker has no listener (P-17). `endpoint-inventory.md` lists them as internal use cases (`WRK-nn`) so that nobody exposes them. | Task rule: internal routes must not be exposed. |
| API-17 | The only HTTP actor is the human session user. No request body or header carries an actor kind. There are no API tokens, service accounts or bearer credentials (D-60). An AI provider, the worker and a tool cannot call the API. | AI cannot approve, verify or change policy (SEC 5). |
| API-18 | Downloads are attachments: `Content-Disposition: attachment`, `Content-Type: application/octet-stream`, `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, and the audit event is written before the first byte. Evidence is re-hashed while streaming. | SEC 9, D-77. |
| API-19 | Every response carries `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cross-Origin-Resource-Policy: same-origin`, and authenticated responses `Cache-Control: no-store`. | D-79 framing denial and SEC 9. |
| API-20 | A policy denial returns 403 `policy_denied` with `error.details.reasons` as stable codes. Free text from tools, targets or providers is never in an error. | No leakage of untrusted or internal text. |
| API-21 | CORS is off by default (same-origin rewrite, D-79). The fallback is one exact configured origin with credentials, an explicit method and header list, and never `*`. | D-79. |
| API-22 | A route exists only if a PRD requirement and a milestone need it. Tables without a route (for example `policy_decisions`) are reached through other resources. Deferred routes are listed in the inventory and not in OpenAPI. | Task rule: do not expose a route because a table exists. |
| API-23 | Unknown JSON fields are rejected (`additionalProperties: false`). Optional fields are omitted, not sent as `null`, in requests. Responses use `null` only for documented nullable fields. Lists are never `null`. | Strict contracts catch client bugs and mass-assignment attempts. |
| API-25 | After a detected database restore the installation is in the state `recovery_required`. `GET /session` reports it. Every other route except the reconcile route returns 503 `recovery_required`. `POST /system/recovery/reconcile` needs the account password and starts a new recovery generation. | A restored database must not silently bring back old authority (DB-28, DQ-17). |
| API-26 | Every re-authentication grant is single use and bound to one action. There is no reusable or window grant in the MVP design (DQ-18 deferred). A grant never replaces an approval, never changes an approval use count, and cannot be issued or extended by an AI actor. | D-58, D-69. Re-authentication proves the human is present. Approval is a separate, exact-operation record. |
| API-27 | Each endpoint has a classification (MVP required, Security-critical, Later milestone, Deferred) and a first milestone in `endpoint-classification.md`. Deferred endpoints keep their ID and are not in `openapi.yaml`. | Task rule: deferred endpoints must not appear in the active specification. |
| API-24 | Re-authentication is a two-step contract: `POST /session/reauthenticate` returns a grant bound to the session, a purpose and a hash of the canonical subject. The action request carries `reauthGrantId`. The server consumes the grant in the action's transaction. A grant never travels in a URL. | D-58, D-69, S1.6. |

## OpenAPI validation status

The four results are reported separately by `scripts/validate-design.py` and must not be confused.

| Result | Status |
| --- | --- |
| YAML parse (Ruby's bundled YAML library) | PASS when the validator reports it |
| Structural checks written for this project (required top-level fields, operation IDs, responses, security, path parameters, endpoint list equals the inventory, request schemas reject unknown fields) | PASS when the validator reports it |
| Internal `$ref` resolution | PASS when the validator reports it |
| **Official OpenAPI 3.1 schema validation** | **NOT VALIDATED AGAINST OFFICIAL OPENAPI 3.1 SCHEMA.** No OpenAPI validator or copy of the official schema is installed in the working environment and installing one is not allowed in this phase. |

**Gate G-openapi (before any code generation, milestone M16):** validate `docs/design/api/openapi.yaml` against the official OpenAPI 3.1 JSON Schema (`https://spec.openapis.org/oas/3.1/schema/2022-10-07`) with a JSON Schema 2020-12 validator, or with an OpenAPI linter that bundles it, for example `npx @redocly/cli lint docs/design/api/openapi.yaml` or `openapi-generator validate -i docs/design/api/openapi.yaml`. Record the tool, its version, the command and the result in the plan. Do not generate server or client code until that gate passes. These commands were **not run** in this task.

