# API conventions

Status: design proposal. Decision IDs refer to `README.md`. Numbers marked proposed are in DQ-06.

## Base, versioning and media types

- Base path `/api/v1` (API-01). Browsers call it same-origin through the Next.js rewrite (D-79). The Go server binds `127.0.0.1` explicitly (SEC 8).
- Requests and responses are `application/json; charset=utf-8`. Downloads are `application/octet-stream`. A request body with another media type gets 415 `unsupported_media_type`. `Accept` other than JSON or `*/*` on a JSON route gets 406 only if the client asks for something the route cannot produce (not modeled further).
- Methods: `GET` read, `POST` create or action, `PUT` replace a set or setting, `PATCH` partial update of a mutable resource, no `DELETE` (the model has no hard deletion, DB-05).

## Resource naming and actions (API-02, API-05)

- Collection under the parent: `GET|POST /programs/{programId}/targets`. Item flat: `GET|PATCH /targets/{targetId}`. Action: `POST /approvals/{approvalId}/approve`. Subresource collections of an item are nested one level: `/jobs/{jobId}/attempts`.
- JSON field names are lowerCamelCase of the column names (`created_at` is `createdAt`). State and enumeration values are the lower snake case strings of the CHECK constraints (`awaiting_human_verification`).
- The server never accepts: a program id for an item, an actor, a `status` on creation, a `version` set by the client other than through `If-Match`, or a `createdAt`.

## Pagination, filtering and sorting (API-06, API-07)

- `limit` (1 to 200, default 50), `cursor` (opaque, from `page.nextCursor`), `sort` (`createdAt` or `-createdAt`).
- Response: `{ "items": [ ... ], "page": { "limit": 50, "nextCursor": "...", "hasMore": true } }`. `nextCursor` is `null` on the last page. An empty list is `[]`.
- Filters are exact-match query parameters named in the endpoint specification. An unknown filter is 400 `invalid_request`. The audit list adds `from` and `to` (RFC 3339) and orders by `(occurredAt, seq)` descending (DB-15).

## Idempotency (API-08)

- Required on creation routes marked `K` in the specification. Header `Idempotency-Key: <16 to 64 of A-Za-z0-9_->`. Stored 24 hours in `idempotency_keys` as a hash with a hash of the canonical request and the result entity. The stored record holds no response body, so a replay returns the current representation of the result entity.
- `GET`, `PUT` and state-guarded actions (`S`) need no key: a repeated action after its transition returns 409 `conflict_state` and changes nothing.
- A replay never repeats a side effect (no second approval request, no second job, no second audit event except the original).

## Concurrency (API-09)

`GET` returns `version` and an `ETag: "<version>"` header. `PATCH` and `PUT` on mutable resources require `If-Match: "<version>"`. Mismatch is 409 `version_conflict`. Missing is 400 `invalid_request`.

## Request identifiers (API-13)

`X-Request-Id` (UUID) in and out. The id is in every error body, every audit event and every log line of the request.

## Validation behavior

- The server validates every field at the boundary: type, length, enumeration, format. The frontend validation is a convenience only (SEC 11).
- Security parsing is not done by the HTTP layer. Scope values, hosts and addresses are passed to the scope engine, which returns a canonical form or refuses. An ambiguous value is 422 `target_ambiguous` and is never repaired (D-34, S1.1).
- Unknown fields are rejected (API-23). Strings are NFC-normalized only where the model says so. Control characters are rejected in names, labels and reasons.
- `fieldErrors` use the JSON path of the field (for example `rules[2].value`).

## Limits and timeouts (API-11, API-12)

| Item | Value (proposed) |
| --- | --- |
| Body, authentication routes | 16 KiB |
| Body, default | 64 KiB |
| Body, `POST /programs/{programId}/scope-versions`, `PUT /reports/{reportId}/findings` | 1 MiB |
| Pagination `limit` | 1 to 200 |
| Server read-header, read, write, idle | 5 s, 15 s, 30 s, 60 s |
| Handler budget | 25 s. Anything longer is a job. |
| Download | streamed, write deadline per chunk |
| `Idempotency-Key` lifetime | 24 hours |
| Text fields | name 200, label 200, reason 1000, notes 8000, rationale 4000 characters |

## Response conventions

- 200 for reads and actions, 201 with the created resource for creation (a `Location` header names the item path), 204 for logout and password change.
- Errors follow `error-model.md`. A successful response never contains a stack trace, SQL, path or secret.
- Every authenticated response has `Cache-Control: no-store` and the headers of API-19.
- `Retry-After` accompanies 429 and 503.

## Long-running work

No request blocks for a tool run or an AI call. The request creates a job (`JOB-01`, `AI-10`) and returns it. The client polls `GET /jobs/{jobId}` (P-12). The 30-second stop objective concerns the worker's stop of a process, not the HTTP latency (DB-12).

## Compatibility rules

Within `v1`: fields and endpoints may be added, error codes may be added, enumerations may gain values (clients must tolerate unknown values). Removing or renaming a field, changing a type, tightening a limit that breaks a valid request, or changing the meaning of a state is a `v2` change. Deprecation is announced in the changelog one milestone ahead.

