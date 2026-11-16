# 0004: Health and readiness endpoints in the MVP

Status: recorded on 2026-11-16 at the project owner's instruction "Health Endpoints MVP Decision & Documentation Alignment". This record continues `0003-foundation-closure-decisions.md`. It supersedes one sentence of D-80 and nothing else.

## 1. Authority and scope

The owner asked for the two health endpoints built in the Phase 1 scaffold to be kept as MVP functionality. The owner also asked for D-80 to be superseded for health endpoints and for the affected documents to be aligned. Nothing was approved by an AI agent for itself. This record changes documentation only. It starts no milestone and adds no code.

## 2. Decision register entry

| ID | Decision | Supersedes | Source |
| --- | --- | --- | --- |
| D-86 | Health and readiness endpoints are part of the MVP. Metrics stay post-MVP. The endpoints return fixed, minimal status words, expose nothing about the host or its dependencies, and stay behind the Host allow-list and the loopback-only binding. | The sentence "A health endpoint and metrics are post-MVP" in D-80, for health endpoints only. The health part of `NFR-OBS-002`, `DEF-05` and the O-14 resolution text in `architecture.md` | Owner instruction of 2026-11-16 |

D-80 is otherwise unchanged. Its logging decision (`log/slog` with a redacting handler, no telemetry), its metrics decision and every other convention in it stay in force. Identifiers are never reused or renumbered, so D-80 keeps its number and this record adds D-86. D-86 is entered in the register in `architecture.md` section 19, and D-80 there carries a supersession note. `scripts/validate-foundation.py` accepts decision identifiers defined in records 0004 and later and checks that they continue the register without gaps.

## 3. The decision

1. Liveness and readiness endpoints are included in the MVP.
2. Metrics endpoints remain post-MVP. No metrics endpoint is part of this decision.
3. Health responses carry fixed, minimal status information. They never carry sensitive information.
4. Health endpoints do not bypass the application security architecture. They are served by the same server, behind the same Host allow-list, on the same loopback-only listener.
5. The only authentication exception is the two routes named in section 4. It is explicit and it is not a precedent for other routes.

## 4. Endpoint contract

Both routes sit under the API base path `/api/v1`. They are given the contract identifiers HLT-01 and HLT-02 in this record. They are not in the OpenAPI document or in the 118-endpoint inventory of `docs/design/api/`, and no client or server code is generated for them (gate G-openapi has not passed). The Phase 1 web client is hand-written.

| ID | Method and path | Route class |
| --- | --- | --- |
| HLT-01 | `GET /api/v1/health/live` | Public (no session). Host allow-list applies. |
| HLT-02 | `GET /api/v1/health/ready` | Public (no session). Host allow-list applies. |

### Liveness (HLT-01)

- Meaning: the application process is alive and its HTTP stack answers.
- It does not imply that the database, the worker or any other dependency is available. It never depends on one.
- Answer: `200` with `{"status":"ok"}`. The use case can return `503` with `{"status":"not_live"}`. The Phase 1 service always reports live, so this answer is not produced today.

### Readiness (HLT-02)

- Meaning: the process finished initialization and is ready to accept requests, and every registered readiness probe passes within its timeout.
- Phase 1 state: no probe is registered. Readiness therefore confirms process initialization only. It does not confirm database readiness or the availability of any dependency, and no document or response may claim that it does. The first probe, the database, belongs to milestone M3 and is not implemented.
- Answers: `200` with `{"status":"ready"}`, or `503` with `{"status":"not_ready"}` and a `Retry-After` header.
- A failing probe is logged by its fixed name only. The error text is never logged or returned, because it can carry a connection string.

### Common behavior

- Only `GET` is routed. Another method on these paths returns `405 method_not_allowed` with an `Allow: GET` header. An unknown path returns `404 not_found`. Both use the standard error envelope of `docs/design/api/error-model.md`.
- The two status bodies above are status documents. They are not error envelopes. Every other failure on any route uses the error envelope.
- A request whose Host header is not in the allow-list gets `403 host_denied` before routing, including for unknown paths and wrong methods. The Host value is not echoed.
- Every response carries `X-Request-Id` and the headers of API-19 (`docs/design/api/README.md`): `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `Referrer-Policy: no-referrer` and `Cross-Origin-Resource-Policy: same-origin`. The web server sets its own framing policy for pages only and skips `/api/`, so a browser never receives two policies for one response. No response carries a `Server` or `X-Powered-By` header.
- Health requests are not audited per request, consistent with `NFR-OBS-002` on read-only polling. The access log records the method, the route template, the status, the duration and the request id. It never records the query string or any header value.

## 5. Security requirements

1. No stack trace, secret, environment value, internal path, version string, configuration value or infrastructure detail appears in any response body or header of these routes. A recovered panic returns `500 internal_error` and the panic value is not logged.
2. The Host allow-list stays mandatory and fail closed. An empty list denies every request. Its entries are loopback names and addresses only.
3. The server binds only an explicit loopback IP literal. A wildcard, LAN or hostname listen address is refused at configuration load and again at bind time. Remote access is not supported in the MVP.
4. Public access is limited to these two routes. They are public because the web UI and local supervisors need to learn whether the process is up before any account or session exists, and because the answer is a fixed word that discloses nothing. Every other route requires the authentication of milestone M4. Authentication, session, CSRF and Origin checks are not implemented yet, so today these two routes are the only routes.
5. A new field, a new status word or any dependency detail in a health response needs a new decision. Detailed diagnostics, if ever needed, belong behind authentication.
6. The Go Host allow-list validates the Host header that Go receives. For browser traffic through the Next.js rewrite, which Host Go receives has not been independently verified, so the allow-list must not be described as validating the browser's original Host header. This does not weaken requirement 2. It limits what is claimed. Verification is part of M13 (D-79, T-55, T-56).
7. Rate limiting for public routes (`docs/design/api/endpoint-inventory.md`, route class PUB) is not implemented in Phase 1. It arrives with the common rate-limit middleware of M4. Until then the loopback binding and the Host allow-list are the only controls in front of these routes.

## 6. Rationale

- A readiness signal is needed before M3. The web shell shows backend status, and the start-up targets and tests need a stable way to ask whether the server is up.
- The routes expose no data, and the Host allow-list and loopback binding limit who can ask. This keeps the information-disclosure risk to a fixed status word.
- D-80 deferred health together with metrics because no need was shown at that time. The need now exists and the cost is two small routes.

## 7. Consequences

- `NFR-OBS-002` is narrowed to metrics, and the new `NFR-OBS-003` holds the health requirement and its acceptance criteria.
- The error catalog gains `405 method_not_allowed`.
- The routes are outside the OpenAPI document until the document is validated (G-openapi). When it is, they are added there and the hand-written client is replaced by the generated one.
- The M3 deliverable gains a database readiness probe and its test. Until then readiness is weaker than its name suggests, and the contract above says so.
- Public routes must not grow. Each addition needs its own decision.

## 8. Alternatives considered

| Alternative | Why not |
| --- | --- |
| Keep health post-MVP and remove the Phase 1 routes | Removes a working, tested signal that the web shell and start-up targets already use. |
| One endpoint for liveness and readiness | A dependency outage would then read as a dead process. The two questions differ. |
| Authenticated health | The question is asked before an account or session can exist. |
| Detailed health (per-dependency status, versions, uptime) | A fingerprinting aid for any local process that can reach the port. Not needed. |
| Include a metrics endpoint | Out of scope. No metric has a requirement, and a metrics surface needs its own disclosure review. |
| Edit the D-80 text in record 0003 | History stays in the records. A supersession note points here instead. |

## 9. Affected decisions and documents

| Item | Effect |
| --- | --- |
| D-80 (record 0003 section 5, "Logging" bullet) | Superseded for health endpoints. A pointer to this record was added. The rest of D-80 stands. |
| `architecture.md` D-80 row, O-14 text (section 17) and O-14 resolution row (section 19) | Each keeps its original wording and carries a supersession note for health. D-86 is added to the register. |
| `docs/PRD.md` | `NFR-OBS-002` narrowed to metrics, `NFR-OBS-003` added, milestone map and status lines updated. |
| `.kiro/steering/tech.md` | Observability, deferred-choice and build-command text updated. |
| `.kiro/specs/mvp/plan.md` | Phase 1 scaffold recorded as existing preparatory work. M3 gains the database readiness probe. M1 is not started. |
| `docs/design/api/error-model.md` | `405 method_not_allowed` added. A note says that the two health status bodies are not error envelopes. |
| `docs/design/api/endpoint-inventory.md` | `DEF-05` narrowed to metrics. |

## 10. Implementation status (Phase 1 scaffold, 2026-11-16)

Implemented and tested on the reference host:

- Both routes, the Host allow-list, the error envelope, the request id, the access log, panic containment and the loopback-only binding, in `services/core`.
- Tests: the router tests cover the Host allow-list on every route, the envelope shape, the 404 and 405 answers, request id handling, panic containment and the absence of query and header values in the log. The bootstrap tests exercise real HTTP over a loopback listener. The architecture test enforces the dependency directions.
- `make test` exited 0. Gate G-arch is covered for the packages that exist.

Not verified:

- The Host that Go receives through the Next.js rewrite (see requirement 6 of section 5). One manual run succeeded with the default allow-list. That is one observation and not a test.

Not implemented:

- Any readiness probe. The database probe is M3.
- Authentication, sessions, CSRF and Origin checks (M4), and rate limiting.
- Inclusion in the OpenAPI document, generated clients, and any metrics endpoint.

Nothing in this record claims that a security control is verified beyond the tests above. Gate G-redact and the threat-model statuses are unchanged.

## 11. Open points for the owner

- None. `architecture.md` and `docs/security/threat-model.md` were aligned in a later consistency pass. No threat row changes status because of this record.
