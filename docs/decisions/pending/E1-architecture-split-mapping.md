# E1: Proposed split of `architecture.md` (mapping for review)

**Round 6 (2026-11-16): STALE.** `architecture.md` is now revision 8 (D-63 to D-85 added). This mapping and its ID registry were generated against revision 7 and must be regenerated before anyone uses them. Nothing has been moved.
Status: PROPOSAL. Nothing has been moved. `.kiro/steering/architecture.md` is unchanged by this document. Regenerated in Round 5 against revision 7 of `architecture.md` (SHA-256 prefix `7164ebcb5f1f1550`), because Rounds 2, 3 and 5 changed the decision register. Awaiting owner approval.
## Goal

Keep `.kiro/steering/architecture.md` as a concise steering entry point and move the detail to `docs/architecture/` and `docs/decisions/`, as `structure.md` section 14 describes. Today the file is 142,824 bytes (941 lines) and loads whole when it is included. Every D, P and O ID keeps its number and its row.

## What changed since the previous mapping

- The first mapping covered a 103,110-byte file with 36 pieces and 98 IDs. This one covers 142,824 bytes with 39 pieces and 169 IDs.
- New IDs since the first mapping: D-30 to D-62 (33 decided), P-39 to P-64 (26 proposed) and O-32 to O-43 (12 open). No ID was removed or renumbered. Existing rows gained appended notes. In Rounds 3 and 5 the text of D-30, D-32, D-33, D-35, D-38, D-45 and O-35 was amended, P-22 is marked superseded, and O-28 moved from the Open table to the Resolved table (its ID and row text are kept).
- New pieces: the revision logs for Round 2, Round 3 and Round 5, all going to `revision-log.md`.
- The steering draft (`E1-architecture-steering.draft.md`) gained invariants for D-47 to D-62 and file-map entries for `tech.md` and the threat model.
- Nothing was moved. The split itself still waits for owner approval.

## Rules for the move

1. Move, do not rewrite. Each block listed below moves verbatim to exactly one destination.
2. One source of truth. The steering file holds summaries and links only. It never restates a block that lives in `docs/`. The dependency table already lives only in `structure.md` 5.6.
3. IDs are never renumbered or removed. Each ID row lives in one registry file (decided, proposed or open). References in prose keep the same ID text, so a text search for an ID finds both its definition and its uses.
4. Relative links replace "section N" references. A reference such as "section 8" becomes a link to `05-policy-and-scope.md`. The list below counts every such reference in the current file (82 of them) with the block that contains it, so none is missed. References to `structure.md` or `security.md` sections are not architecture sections and are excluded by rule (see below).
5. Nothing is deleted. The previous revision stays in version control (once committed) and in the backup archive.

## Proposed file layout

| File | Contents (current sections) | Approx. bytes |
| --- | --- | --- |
| `.kiro/steering/architecture.md` | front, title | 1,595 |
| `docs/decisions/README.md` | §19, howto | 1,476 |
| `docs/architecture/01-vision-and-context.md` | §1, §2 | 7,067 |
| `docs/architecture/02-modular-monolith.md` | §3, §4 | 7,645 |
| `docs/architecture/03-backend-and-frontend.md` | §5, §6 | 5,374 |
| `docs/architecture/04-assessment-and-jobs.md` | §7, §14 | 6,778 |
| `docs/architecture/05-policy-and-scope.md` | §8 | 6,613 |
| `docs/architecture/06-ai-orchestration.md` | §9 | 3,760 |
| `docs/architecture/07-tool-adapters.md` | §10 | 4,669 |
| `docs/architecture/08-evidence-and-findings.md` | §11, §12 | 5,764 |
| `docs/architecture/09-persistence.md` | §13 | 3,615 |
| `docs/architecture/10-credentials-and-deployment.md` | §15, §16 | 9,128 |
| `docs/architecture/11-errors-audit-testing.md` | §17, §18 | 9,533 |
| `docs/decisions/decided.md` | §19 | 20,011 |
| `docs/decisions/proposed.md` | §19 | 18,798 |
| `docs/decisions/open.md` | §19 | 11,517 |
| `docs/decisions/not-adopted.md` | §19 | 1,083 |
| `docs/decisions/revision-log.md` | §19 | 8,058 |
| `docs/decisions/steering-alignment.md` | §19 | 5,801 |
| `docs/architecture/12-diagrams.md` | §20 | 4,539 |

The steering file is rewritten as a new, short document (8,227 bytes in the draft, see `E1-architecture-steering.draft.md`), so it is listed above only for its retained front matter and status. The retained front matter is not counted in the draft size. The rewritten file replaces it.

## Section-by-section mapping

Lines are 1-based. A block covers its first line up to, but not including, the first line of the next block. The last block ends at the last line of the file.

| Current block | Lines | Bytes | Destination | Treatment |
| --- | --- | --- | --- | --- |
| --- | 1-7 | 389 | `.kiro/steering/architecture.md` | front matter stays, rewritten (inclusion mode and description kept) |
| # MORN1NGST4R architecture | 7-11 | 1,206 | `.kiro/steering/architecture.md` | title and status stay, shortened |
| ## How to read this document | 11-21 | 1,431 | `docs/decisions/README.md` | verbatim: tag legend and ID rules (single source of truth) |
| ## 1. Architectural vision and principles | 21-42 | 3,269 | `docs/architecture/01-vision-and-context.md` | verbatim; steering keeps a 5-line summary and links here |
| ## 2. System context | 42-74 | 3,798 | `docs/architecture/01-vision-and-context.md` | verbatim |
| ## 3. Modular monolith | 74-84 | 1,857 | `docs/architecture/02-modular-monolith.md` | verbatim (already a pointer to structure.md 5.6) |
| ## 4. Component responsibilities and boundaries | 84-128 | 5,788 | `docs/architecture/02-modular-monolith.md` | verbatim |
| ## 5. Backend architecture and request flow | 128-145 | 2,500 | `docs/architecture/03-backend-and-frontend.md` | verbatim |
| ## 6. Frontend architecture and communication with the backend | 145-163 | 2,874 | `docs/architecture/03-backend-and-frontend.md` | verbatim |
| ## 7. Assessment lifecycle and job orchestration | 163-195 | 2,990 | `docs/architecture/04-assessment-and-jobs.md` | verbatim |
| ## 8. Scope and policy enforcement | 195-230 | 6,613 | `docs/architecture/05-policy-and-scope.md` | verbatim; steering keeps the key invariants and links here |
| ## 9. AI orchestration and provider abstraction | 230-251 | 3,760 | `docs/architecture/06-ai-orchestration.md` | verbatim |
| ## 10. Security tool adapter architecture | 251-272 | 4,669 | `docs/architecture/07-tool-adapters.md` | verbatim |
| ## 11. Evidence collection, integrity and storage | 272-288 | 3,113 | `docs/architecture/08-evidence-and-findings.md` | verbatim |
| ## 12. Finding validation and human verification | 288-304 | 2,651 | `docs/architecture/08-evidence-and-findings.md` | verbatim |
| ## 13. Persistence and database boundaries | 304-319 | 3,615 | `docs/architecture/09-persistence.md` | verbatim |
| ## 14. Queue and worker architecture | 319-340 | 3,788 | `docs/architecture/04-assessment-and-jobs.md` | verbatim |
| ## 15. Credential and secret management | 340-363 | 3,106 | `docs/architecture/10-credentials-and-deployment.md` | verbatim |
| ## 16. Deployment architecture: localhost web MVP and future modes | 363-415 | 6,022 | `docs/architecture/10-credentials-and-deployment.md` | verbatim |
| ## 17. Error handling, observability and auditability | 415-442 | 3,905 | `docs/architecture/11-errors-audit-testing.md` | verbatim |
| ## 18. Testing strategy and architectural boundaries | 442-483 | 5,628 | `docs/architecture/11-errors-audit-testing.md` | verbatim |
| ## 19. Decisions, trade-offs and open items | 483-485 | 45 | `docs/decisions/README.md` | section heading only: becomes the decision index |
| ### Decided (from steering and owner decisions) | 485-552 | 20,011 | `docs/decisions/decided.md` | verbatim table |
| ### Proposed (recommendations) | 552-621 | 18,798 | `docs/decisions/proposed.md` | verbatim table |
| ### Open (not decided) | 621-667 | 10,692 | `docs/decisions/open.md` | verbatim table |
| ### Resolved by the MVP decision | 667-674 | 825 | `docs/decisions/open.md` | verbatim table (kept beside Open) |
| ### Not adopted (reconsider only with a demonstrated requirement) | 674-686 | 1,083 | `docs/decisions/not-adopted.md` | verbatim table |
| ### Revision log: MVP platform decision | 686-699 | 912 | `docs/decisions/revision-log.md` | verbatim |
| ### Revision log: foundation audit (revision 3) | 699-711 | 1,418 | `docs/decisions/revision-log.md` | verbatim |
| ### Revision log: owner approvals, Round 1 (revision 4) | 711-724 | 1,428 | `docs/decisions/revision-log.md` | verbatim |
| ### Revision log: owner security decisions, Round 2 (revision 5) | 724-735 | 1,134 | `docs/decisions/revision-log.md` | verbatim |
| ### Revision log: owner decisions and B1 applied, Round 3 (revision 6) | 735-750 | 1,598 | `docs/decisions/revision-log.md` | verbatim |
| ### Revision log: Round 5 foundation completion (revision 7) | 750-762 | 1,568 | `docs/decisions/revision-log.md` | verbatim |
| ### Steering alignment: resolved by owner approval (Round 1) | 762-774 | 1,513 | `docs/decisions/steering-alignment.md` | verbatim |
| ### Steering alignment: pending owner approval (not edited here) | 774-798 | 4,288 | `docs/decisions/steering-alignment.md` | verbatim |
| ## 20. Diagrams | 798-800 | 17 | `docs/architecture/12-diagrams.md` | section heading |
| ### D1. System context | 800-833 | 1,080 | `docs/architecture/12-diagrams.md` | verbatim Mermaid |
| ### D2. Component architecture | 833-895 | 1,849 | `docs/architecture/12-diagrams.md` | verbatim Mermaid |
| ### D3. Assessment data flow | 895-941 | 1,593 | `docs/architecture/12-diagrams.md` | verbatim Mermaid |

## Dry-run result (done in a scratch directory, not in the repository)

- The file was cut into 39 pieces at fixed heading markers (headings inside Mermaid fences are ignored), written to 20 destination files, read back at recorded offsets, and reassembled in the original order.
- Result: **byte-identical to the original**. SHA-256 prefix of the original `7164ebcb5f1f1550`, of the reassembled bytes `7164ebcb5f1f1550`. Sizes: 142,824 and 142,824 bytes.
- The pieces partition the file: the first starts at line 1, each one starts where the previous one ends, and the last ends at the end of the file.
- Every piece appears in exactly one destination. No block is split across two files and no block is copied to two files.
- The check can be re-run after the real move: concatenate the destination files in the manifest order and compare with the pre-move file from version control.

## What the new steering file contains (draft in `E1-architecture-steering.draft.md`)

- Front matter (`inclusion: auto`, name, description) unchanged in meaning.
- A 5-line status and scope paragraph.
- The MVP delivery model in 6 lines (D-16 to D-22, D-29).
- Architectural invariants, each one line with its IDs and a link: policy is the only authorization boundary (D-05, D-28), approvals and their lifecycle (D-06, D-30, D-45), scope grammar and address policy (D-32, D-34 to D-40), AI proposes and cannot approve or verify (D-07, P-11), AI provider egress (D-46), findings stay Candidate until a human verifies (D-09), evidence integrity and untrusted tool output (D-10, P-37), tools run only through isolated adapters with no bypass and only through an approved network enforcement path (D-11, D-31, D-41 to D-44), no secrets in PostgreSQL or logs (D-08), dependency directions live in `structure.md` 5.6 (D-23), the localhost mode and untrusted loopback (D-33, P-19), and the PostgreSQL strategy is not assumed (D-22).
- A short table of the open decisions that currently block implementation, each with its ID and a link.
- A file map pointing to `docs/architecture/` and `docs/decisions/`.

It restates no table and no procedure.

## ID registry mapping

Totals: 62 decided, 64 proposed, 43 open or resolved (2 resolved: O-15 and O-28). All are defined today in section 19. After the move the registry files are `decided.md`, `proposed.md` and `open.md` (resolved items stay beside open ones).

"Referenced in" lists, in file order, the codes of the blocks where the ID appears outside its own row. Codes: `title` the title and status block, `how` the tag legend, a number is that section, `19dec`, `19pro`, `19opn`, `19res`, `19nad` the section 19 tables (decided, proposed, open, resolved, not adopted), `19rl1` to `19rl6` the six revision logs (MVP platform, foundation audit, Round 1, Round 2, Round 3, Round 5), `19sar` and `19sap` the steering alignment tables (resolved, pending), `20d1` to `20d3` the diagrams. After the move each reference becomes a link into the matching file.

| ID | Title (first words of its row) | Registry file | Referenced in current sections |
| --- | --- | --- | --- |
| D-01 | Monorepo, modular monolith | `docs/decisions/decided.md` | 1 |
| D-02 | Go, Gin, PostgreSQL with pgx, sqlc, Goose, Next.js with TanStack Query, OpenAP | `docs/decisions/decided.md` | 19rl2 |
| D-03 | `server` and `worker` executables with single runtime responsibilities, bootst | `docs/decisions/decided.md` | 3,19rl1 |
| D-04 | Layering and dependency rules, thin handlers, no logic in repositories | `docs/decisions/decided.md` | 1 |
| D-05 | Backend policy engine is the mandatory boundary. Workers revalidate. | `docs/decisions/decided.md` | 1,7 |
| D-06 | Approval bound to exact target, operation, parameters, risk, limits, time, app | `docs/decisions/decided.md` | 8,19dec |
| D-07 | AI untrusted, provider independent, cannot approve or verify | `docs/decisions/decided.md` | 2 |
| D-08 | No plaintext secrets or silent fallback, platform credential stores | `docs/decisions/decided.md` | 2 |
| D-09 | Finding lifecycle states, validation and report status kept separate. Automati | `docs/decisions/decided.md` | 1,19rl2 |
| D-10 | SHA-256, provenance, redaction, access control for evidence | `docs/decisions/decided.md` | 11 |
| D-11 | Worker and process hardening requirements | `docs/decisions/decided.md` | 10 |
| D-12 | Audit event categories | `docs/decisions/decided.md` | 1 |
| D-13 | MVP exclusions | `docs/decisions/decided.md` | 1 |
| D-14 | Desktop and headless modes share core logic when they exist. Applies to the fu | `docs/decisions/decided.md` | 1,19rl1 |
| D-15 | Local-first | `docs/decisions/decided.md` | 1 |
| D-16 | The MVP is a local-first web application used in a browser on localhost. Front | `docs/decisions/decided.md` | title,1,3,6,16,19dec,19rl1,19rl2 |
| D-17 | Initial development and the primary MVP environment is Linux localhost | `docs/decisions/decided.md` | 1,4,18 |
| D-18 | Native desktop packaging is not an MVP requirement. No Electron, Tauri or othe | `docs/decisions/decided.md` | 1,3,6,14,16,19dec,19pro,19opn,19nad |
| D-19 | Windows and macOS support are future compatibility goals, not initial delivery | `docs/decisions/decided.md` | 4,10,15,16,18,19opn,19res,19nad |
| D-20 | Linux headless server deployment is a future deployment mode | `docs/decisions/decided.md` | 1,2,15,16,19pro,19opn,19res,19nad |
| D-21 | Frontend and backend boundaries stay clean so future deployment modes remain p | `docs/decisions/decided.md` | 1,6,16,19pro |
| D-22 | No PostgreSQL deployment strategy is assumed. Local installation, bundled life | `docs/decisions/decided.md` | title,13,16,19opn,19rl1,19rl2 |
| D-23 | Domain dependency order. Leaf packages `operation` and `actor`. Domain package | `docs/decisions/decided.md` | 3,4,19pro,19opn,19rl3,19rl6,19sar |
| D-24 | `internal/infrastructure` holds shared technical code (connection pool, transa | `docs/decisions/decided.md` | 3,4,19pro,19rl3,19sar |
| D-25 | The worker loop is an inbound adapter (`adapters/inbound/worker`) that calls a | `docs/decisions/decided.md` | 4,19pro,19opn,19rl3,19sar |
| D-26 | Outbound ports are organized by consuming application area, not as one shared  | `docs/decisions/decided.md` | 19pro,19rl3,19sar |
| D-27 | White-box, integration, security (internal access) and architecture tests live | `docs/decisions/decided.md` | 18,19rl3,19sar |
| D-28 | Target scope is validated before scheduling tool execution and again immediate | `docs/decisions/decided.md` | 19rl3,19sar |
| D-29 | `product.md` and `structure.md` state the Linux localhost MVP, with Windows, m | `docs/decisions/decided.md` | 16,19res,19rl3,19sar |
| D-30 | An approval use is consumed once per (approval, job), atomically in the job cl | `docs/decisions/decided.md` | title,7,8,14,17,19dec,19pro,19opn,19rl4,19rl5,19rl6,19sap |
| D-31 | No unsandboxed execution exception. Every tool execution requires isolation, i | `docs/decisions/decided.md` | 10,19opn,19sap |
| D-32 | A lab exemption for a denied address class applies only when the operation's o | `docs/decisions/decided.md` | title,8,19dec,19pro,19rl4,19rl5,19sap |
| D-33 | The localhost web application is its own security mode and does not inherit th | `docs/decisions/decided.md` | title,16,19pro,19rl4,19rl5,19sap |
| D-34 | Scope rule kinds in the MVP: domain, wildcard, IP and CIDR only. URL paths, re | `docs/decisions/decided.md` | 8,19opn,19sap |
| D-35 | A wildcard matches one or more labels below its base and never the apex. A mai | `docs/decisions/decided.md` | title,19opn,19rl5,19rl6 |
| D-36 | Default ports: an in-scope rule without a port covers 80 and 443, an exclusion | `docs/decisions/decided.md` |  |
| D-37 | Host canonicalization runs in the application layer with an explicitly configu | `docs/decisions/decided.md` |  |
| D-38 | Exclusions always win. Ambiguous, malformed or non-canonical targets are denie | `docs/decisions/decided.md` | title,8,19opn,19rl5 |
| D-39 | Denied addresses come from generated IANA special-purpose registry data plus e | `docs/decisions/decided.md` | 8,19pro,19opn |
| D-40 | Every redirect hop is a new target and repeats canonicalization, scope, port a | `docs/decisions/decided.md` | 8,19pro,19sap |
| D-41 | Egress enforcement direction: a first-party HTTP client for probing and replay | `docs/decisions/decided.md` | 8,19opn,19rl6 |
| D-42 | Preferred sandbox feasibility direction: rootless Linux namespaces with Bubble | `docs/decisions/decided.md` | 10,19pro,19opn,19rl6,19sap |
| D-43 | The six tool hygiene rules of S1.4 are approved: parameter schemas, option and | `docs/decisions/decided.md` | 10,19pro,19rl6,19sap |
| D-44 | Every tool manifest declares non-target destinations. Updates, telemetry, call | `docs/decisions/decided.md` | 8,10,19pro,19res,19rl5,19sap |
| D-45 | Approval record and screen bound to the canonical operation. AI agents and API | `docs/decisions/decided.md` | title,8,19pro,19opn,19rl5,19rl6,19sap |
| D-46 | Any AI provider endpoint that is not on the loopback interface is remote, incl | `docs/decisions/decided.md` | title,19dec,19pro,19opn,19rl4,19rl5,19rl6,19sap |
| D-47 | Lab authorization is evaluated before the general non-public address denial an | `docs/decisions/decided.md` | title,8,18,19rl6 |
| D-48 | Expiry prevents new launches only. A launched operation continues only within  | `docs/decisions/decided.md` | 7,19dec,19pro,19opn |
| D-49 | An intrusive operation has exactly one process launch: no automatic retry, no  | `docs/decisions/decided.md` | 7,17 |
| D-50 | Target credentials, cookies, access tokens and session secrets are never sent  | `docs/decisions/decided.md` | 19dec,19pro,19opn |
| D-51 | The approval record binds the canonical operation hash and the declared networ | `docs/decisions/decided.md` | 19dec |
| D-52 | The Linux MVP stores credentials in the Secret Service only. A missing Secret  | `docs/decisions/decided.md` | 2,10,15,16,19opn |
| D-53 | Security policy changes are made only by an authenticated human through one ca | `docs/decisions/decided.md` |  |
| D-54 | The application-owned endpoint set (listeners, wildcard binds, databases, per- | `docs/decisions/decided.md` | 8,19pro |
| D-55 | Approval consumption is separate from process attempts and is consumed atomica | `docs/decisions/decided.md` | 19dec,19pro,19opn |
| D-56 | Any scope, program-rules or lab change, widening or narrowing, invalidates the | `docs/decisions/decided.md` |  |
| D-57 | Out-of-scope rules and exclusions cover all ports unless narrowed. Exclusions  | `docs/decisions/decided.md` | 8,19dec,19opn |
| D-58 | Intrusive operations stay disabled until local authentication and re-authentic | `docs/decisions/decided.md` |  |
| D-59 | Approval class membership, strict mapping: passive reconnaissance is Passive.  | `docs/decisions/decided.md` | 19opn |
| D-60 | Authentication, option A: one local researcher account, memory-hard password h | `docs/decisions/decided.md` | 4,16,19dec,19pro,19opn |
| D-61 | Target authorization validation, option A: mechanical scope validation, a stor | `docs/decisions/decided.md` | 19pro,19res |
| D-62 | The technology baseline named by the owner is confirmed: Go, Gin, PostgreSQL,  | `docs/decisions/decided.md` | title,19pro,19rl6 |
| O-01 | PostgreSQL deployment strategy. Classified, none evaluated or chosen (D-22). F | `docs/decisions/open.md` | 2,13,16,18,19dec,19pro,19opn,19rl1,19rl6,19sap,20d1 |
| O-02 | How the Next.js app is served on localhost in the MVP Round 5: PROPOSED P-49. | `docs/decisions/open.md` | 2,6,16,19pro,19rl1,20d1 |
| O-03 | Browser to API path, session and CORS model Round 5: PROPOSED P-49 and P-60. | `docs/decisions/open.md` | 6,16,19dec,19pro,19opn,19rl1,20d1 |
| O-04 | OpenAPI direction. Gin has no built-in OpenAPI support, so a generator or anno | `docs/decisions/open.md` | 5,19pro,19rl2,19sap |
| O-05 | MVP tools, provisioning, version pinning, license review Round 5: PROPOSED P-5 | `docs/decisions/open.md` | 10,19pro,19sap |
| O-06 | Risk scale, which MVP operations are risky, approval durations and use bound ( | `docs/decisions/open.md` | 8,19pro,19rl2,19rl4 |
| O-07 | Mechanism for P-24: scope binding across redirects and DNS, and connecting to  | `docs/decisions/open.md` | 8,19dec,19pro,19rl2,19rl4 |
| O-08 | Evidence raw or redacted storage, encryption at rest, retention | `docs/decisions/open.md` | 11 |
| O-09 | Runtime skill manifest and loader format Round 5: PROPOSED P-54. | `docs/decisions/open.md` | 9,19pro |
| O-10 | LLM data egress policy and prompt retention (Round 2: remote provider definiti | `docs/decisions/open.md` | 9,19dec,19pro,19rl4,19sap |
| O-11 | Finding transition table, which transitions automation may apply, deduplicatio | `docs/decisions/open.md` | 12,19rl2,19sap |
| O-12 | Queue library or custom, cancellation signaling Round 5: PROPOSED P-44, a cust | `docs/decisions/open.md` | 7,14,19pro,19rl2,19sap |
| O-13 | Linux process-isolation mechanism for the worker and tools (Windows and macOS  | `docs/decisions/open.md` | 10,13,18,19dec,19pro,19rl1,19rl2,19rl4,19sap |
| O-14 | Logging library, error format, metrics, health, audit volume for read-only pol | `docs/decisions/open.md` | 17,19pro,19rl2,19sap |
| O-15 | Is macOS desktop in the MVP, and are Windows and macOS headless included? `P`  | `docs/decisions/open.md` (resolved) | 16,19rl1,19rl3,19sar |
| O-16 | Local authentication for the single-user localhost MVP (mechanism, session lif | `docs/decisions/open.md` | 4,5,6,16,19dec,19pro,19opn,19rl1,19rl2,19rl5 |
| O-17 | Which Linux credential sources the MVP implements. Rotation and entry flow. Ex | `docs/decisions/open.md` | 4,15,16,19pro,19rl1,19sap,20d1 |
| O-18 | Architecture-test tooling, integration-test PostgreSQL (depends on O-01) Round | `docs/decisions/open.md` | 16,18,19pro,19rl1,19rl3,19sap |
| O-19 | Read models for cross-domain queries | `docs/decisions/open.md` | 13 |
| O-20 | ID format, schema conventions, migration rollback Round 5: PROPOSED P-47. | `docs/decisions/open.md` | 13,16,19pro |
| O-21 | How `server`, `worker` and PostgreSQL are started and kept running locally (P- | `docs/decisions/open.md` | 14,15,16,19pro,19rl1 |
| O-22 | Supported Linux baseline for the MVP: distributions, minimum versions of Go, N | `docs/decisions/open.md` | 10,16,19dec,19pro,19opn,19rl1,19sap |
| O-23 | Where evidence and other runtime data live, and the ignore rules for it Round  | `docs/decisions/open.md` | 2,11,16,19pro,19rl1,19sap,20d1 |
| O-24 | Scope rule grammar and matching semantics: hostnames (case, trailing dot, IDN  | `docs/decisions/open.md` | 8,18,19rl2,19rl4,19sap |
| O-25 | Reduced by `AUD` A2: the module homes are approved (D-23 to D-25). Still open: | `docs/decisions/open.md` | 4,5,19rl3 |
| O-26 | MVP AI scope: which providers are implemented and what "basic AI-assisted anal | `docs/decisions/open.md` | 9,19pro,19sap |
| O-27 | Report format and content, redaction on export, and the values of "report stat | `docs/decisions/open.md` | 4,12,19pro,19opn,19rl6,19sap |
| O-28 | Meaning of "Validate target authorization" (`P` 6 step 4), and whether passive | `docs/decisions/open.md` (resolved) | 8,19dec,19rl4,19rl5,19rl6,19sap |
| O-29 | MVP controlled-test catalog: which active tests exist ("execute controlled tes | `docs/decisions/open.md` | 19rl6,19sap |
| O-30 | "Continuous asset and endpoint inventory" (`P` 4) in the MVP: scheduled re-sca | `docs/decisions/open.md` | 7,19sap |
| O-31 | How reproduction and validation replays requests (section 10) | `docs/decisions/open.md` | 10,19rl2 |
| O-32 | Public suffix data source and library: which one, license review, update strat | `docs/decisions/open.md` | 8,19dec,19pro,19opn,19rl4,19rl6 |
| O-33 | IDNA library and pinned profile, and validation of its behavior and of UTS #39 | `docs/decisions/open.md` | 8,19dec,19pro,19opn |
| O-34 | Cloud metadata address list: whether to name `100.100.100.200` and `168.63.129 | `docs/decisions/open.md` | 19dec,19pro |
| O-35 | Approval policy numbers the owner did not set: the stop time after a scope edi | `docs/decisions/open.md` | 19dec,19pro,19rl5,19rl6 |
| O-36 | A trusted-LAN AI provider category (D-46 treats every non-loopback endpoint as | `docs/decisions/open.md` | 19dec,19rl4 |
| O-37 | Password recovery for the single local account | `docs/decisions/open.md` | 16,19dec,19opn,19rl6 |
| O-38 | Numeric request-rate, concurrency and duration limits for each active operatio | `docs/decisions/open.md` | 19dec,19opn |
| O-39 | Who provides the PostgreSQL binaries for the per-user cluster (P-42): the user | `docs/decisions/open.md` | 19opn |
| O-40 | Licence acceptance: MPL-2.0 public suffix data and any generated table, GPL-3. | `docs/decisions/open.md` | 19pro,19opn |
| O-41 | Which sandbox resource limits are required (P-63), in particular whether a per | `docs/decisions/open.md` |  |
| O-42 | First-run account creation: how a one-time bootstrap code is delivered without | `docs/decisions/open.md` |  |
| O-43 | How a user-supplied PostgreSQL DSN (P-42) delivers its password without an env | `docs/decisions/open.md` | 19rl6 |
| P-01 | Domain-to-domain imports only for owner-defined ID types and value objects, al | `docs/decisions/proposed.md` | 3,18,19rl2 |
| P-02 | Explicit `Actor` (human, system, ai) on every use case. Requirement is decided | `docs/decisions/proposed.md` | 1,4,5,8,19rl2 |
| P-03 | Policy engine is a pure decision function in `domain/scope` (using `domain/app | `docs/decisions/proposed.md` | 4,8,19rl2 |
| P-04 | `ToolExecutor` accepts only an `AuthorizedOperation`, defined in `domain/scope | `docs/decisions/proposed.md` | 8,10,18,19pro,19rl2 |
| P-05 | Four enforcement points E1 to E4 | `docs/decisions/proposed.md` | 8,18 |
| P-06 | PostgreSQL-backed queue behind `JobQueue` | `docs/decisions/proposed.md` | 2,14,19dec,19nad,19rl1,19rl6 |
| P-07 | Transaction boundary at the use case through `Transactor`, with the rules in P | `docs/decisions/proposed.md` | 4,5,13,17,19rl2 |
| P-08 | Write-once evidence files, hash on write, re-hash on read, with the protocol i | `docs/decisions/proposed.md` | 2,11,16,19dec,19rl2,19rl6 |
| P-09 | `origin` on stored facts, redactor in logs, evidence and prompts | `docs/decisions/proposed.md` | 6,9,15 |
| P-10 | Automation limited to Candidate and Needs Review plus recommendations | `docs/decisions/proposed.md` | 1,12,19rl2 |
| P-11 | AI returns typed proposals only and has no tool handle | `docs/decisions/proposed.md` | 9 |
| P-12 | Polling for job progress in the MVP | `docs/decisions/proposed.md` | 6,17 |
| P-13 | Separate DB roles for migration, API and worker. Audit rows insertable and rea | `docs/decisions/proposed.md` | 2,13,16,17,19rl2 |
| P-14 | Tool capability manifest read by policy | `docs/decisions/proposed.md` | 7,10,14 |
| P-15 | Desktop launcher supervises server and worker, loopback only. **Deferred**: no | `docs/decisions/proposed.md` | 19rl1 |
| P-16 | AI tasks run as persistent audited worker jobs | `docs/decisions/proposed.md` | 9 |
| P-17 | MVP process model: `server` and `worker` run as two separate local processes n | `docs/decisions/proposed.md` | 14,16,19pro,19opn,19rl1 |
| P-18 | The application depends only on database connection settings, never on how Pos | `docs/decisions/proposed.md` | 2,13,15,16,19opn |
| P-19 | Localhost hardening baseline: bind the API and the UI server to loopback expli | `docs/decisions/proposed.md` | 1,2,5,6,16,18,19dec,19opn,19rl2,19rl4,19sap |
| P-20 | Frontend and backend meet only at the versioned REST API. Base URL and allowed | `docs/decisions/proposed.md` | 6,16 |
| P-21 | OS-specific code only under `platform/`, enforced by an architecture test. Cro | `docs/decisions/proposed.md` | 4,10,16,18 |
| P-22 | The MVP applies the `SEC` 15 Desktop requirements and `SEC` 7 Linux guidance.  | `docs/decisions/proposed.md` | 16,19dec,19rl1,19rl4,19rl6 |
| P-23 | Domain dependency order and two small leaf domain packages, `operation` and `a | `docs/decisions/proposed.md` | 3,5,11,12,18,19dec,19pro,19rl2,19rl3 |
| P-24 | Resolved network address is part of the target. Deny loopback, private, link-l | `docs/decisions/proposed.md` | 2,8,18,19dec,19opn,19rl2,19rl4,19rl6,19sap |
| P-25 | Tool invocation hygiene: per-parameter schema and option guard, secrets never  | `docs/decisions/proposed.md` | 2,10,15,18,19dec,19rl4,19sap |
| P-26 | Non-target destinations (updates, templates, callbacks, telemetry, passive sou | `docs/decisions/proposed.md` | 2,10,18,19dec,19rl4,19sap |
| P-27 | `internal/infrastructure` for the pool and transaction helper, and the transac | `docs/decisions/proposed.md` | 5,13,19dec,19pro,19rl3 |
| P-28 | Worker loop in a small inbound adapter that calls an `application/job` use cas | `docs/decisions/proposed.md` | 4,14,19dec,19rl3,20d2 |
| P-29 | `ports/outbound` organized by consuming application area | `docs/decisions/proposed.md` | 4,19dec,19rl3 |
| P-30 | Approvals have mandatory expiry, bounded use consumed in the claim transaction | `docs/decisions/proposed.md` | 8,18,19dec,19opn,19rl4,19rl6,19sap |
| P-31 | Scope edits (widening or narrowing), lab changes and approval revocation cance | `docs/decisions/proposed.md` | 7,8,18,19dec,19rl4,19rl6 |
| P-32 | AI jobs run in a composition with no `ToolExecutor`. Database grants back the  | `docs/decisions/proposed.md` | 8,9,14 |
| P-33 | Audit integrity: denials audited in their own transaction, an audit write fail | `docs/decisions/proposed.md` | 5,11,17,18,19rl6,19sap |
| P-34 | AI egress default: local providers only, per-provider per-program opt-in for c | `docs/decisions/proposed.md` | 2,9,14,17,19dec,19rl4,19rl6,19sap |
| P-35 | Output hardening: CSP, `nosniff`, attachment-only evidence downloads, framing  | `docs/decisions/proposed.md` | 6,19rl6 |
| P-36 | Queue fencing: writes conditioned on job, attempt and running state, unique re | `docs/decisions/proposed.md` | 14,17,18 |
| P-37 | Evidence write protocol: same-filesystem temp file, flush, no-replace publish, | `docs/decisions/proposed.md` | 11,19pro |
| P-38 | Plan items and AI proposals are owned by the `assessment` domain with an origi | `docs/decisions/proposed.md` | 4,7,19rl2 |
| P-39 | Re-authentication recency for intrusive approval: password entered within the  | `docs/decisions/proposed.md` | 16,19dec,19opn,19rl6 |
| P-40 | Target authorization validation expires after 24 hours in addition to change-b | `docs/decisions/proposed.md` | 19dec,19res |
| P-41 | Backend toolchain pins (verified 2026-11-15, see `tech.md`): Go 1.27.4 with a  | `docs/decisions/proposed.md` |  |
| P-42 | PostgreSQL 18 current minor, provisioned as a per-user cluster over a unix soc | `docs/decisions/proposed.md` | 19opn |
| P-43 | OpenAPI spec-first: author OpenAPI 3.0.4, generate the Gin server with oapi-co | `docs/decisions/proposed.md` | 19opn |
| P-44 | Custom PostgreSQL queue with `FOR UPDATE SKIP LOCKED`, leases and fencing, wit | `docs/decisions/proposed.md` | 19pro,19opn |
| P-45 | Logging with the standard library `log/slog` and a redacting handler (O-14) | `docs/decisions/proposed.md` | 19opn |
| P-46 | Test tooling: `go test`, a standard-library architecture test over `go list -j | `docs/decisions/proposed.md` | 19opn |
| P-47 | Identifiers: UUID v7 generated in the application with google/uuid v1.6.0 and  | `docs/decisions/proposed.md` | 19opn |
| P-48 | Password hashing: Argon2id through `golang.org/x/crypto/argon2` with m=65536 K | `docs/decisions/proposed.md` | 16,19dec,19opn |
| P-49 | Serving and browser path: `next build` then `next start -H 127.0.0.1`, same-or | `docs/decisions/proposed.md` | 19opn |
| P-50 | Frontend pins (verified 2026-11-15, see `tech.md`): Node 24.x at least 24.15.0 | `docs/decisions/proposed.md` |  |
| P-51 | Linux baseline defined by capability, not by a distribution list: x86_64, kern | `docs/decisions/proposed.md` | 19opn |
| P-52 | MVP tool integrations: Subfinder (passive), httpx (probing), and Nuclei as an  | `docs/decisions/proposed.md` | 19opn |
| P-53 | AI provider scope: one local adapter (loopback) and one generic OpenAI-compati | `docs/decisions/proposed.md` | 19opn |
| P-54 | Skill manifest: `SKILL.md` layout for instructions plus a project-owned manife | `docs/decisions/proposed.md` | 19opn |
| P-55 | Report format: Markdown generated from `text/template` with untrusted values o | `docs/decisions/proposed.md` | 19opn |
| P-56 | Public suffix data: vendor the unmodified list with a pinned SHA-256 and a sma | `docs/decisions/proposed.md` | 19opn |
| P-57 | Host name handling: `golang.org/x/net/idna` Lookup profile plus hand-written s | `docs/decisions/proposed.md` | 19opn |
| P-58 | Metadata deny list additions after the two decided addresses: `168.63.129.16`  | `docs/decisions/proposed.md` | 19opn |
| P-59 | Credential store: `go-keyring` v0.2.8 behind the `CredentialStore` port with a | `docs/decisions/proposed.md` | 19opn |
| P-60 | Web security in Gin: hand-written Host allow-list, Origin check, CSRF token, s | `docs/decisions/proposed.md` | 16,19opn |
| P-61 | Process start-up: a Makefile with targets for the PostgreSQL cluster, the serv | `docs/decisions/proposed.md` | 19opn |
| P-62 | Data location: a per-user directory under `$XDG_DATA_HOME/morningstar` for the | `docs/decisions/proposed.md` | 19opn |
| P-63 | Required resource limits for every sandboxed tool: wall-clock timeout, memory, | `docs/decisions/proposed.md` | 19opn |
| P-64 | Before every sandboxed launch run a cheap preflight that checks the sandbox he | `docs/decisions/proposed.md` | 19rl6 |

## In-text "section N" references that become links

The table lists each current block that contains such a reference, the reference text, and where its target moves. A reference is excluded when it follows a source tag or a file name (`SEC`, `S`, `P`, `MVP`, `AUD`, `structure.md`, `security.md`, `product.md`, `tech.md`), because it then points at another document, and when the number is followed by a dot and another digit (as in "section 5.6"). For a range such as "sections 8 to 12", the first and last sections are listed, and every section between them is also affected and needs a manual check when the links are written.

| Current block | References found | Target after the move |
| --- | --- | --- |
| MORN1NGST4R architecture | section 19; section 19 | README.md |
| How to read this document | Section 19; section 19; section 19 | README.md |
| 1. Architectural vision and principles | sections 8 to 12; section 19 | 05-policy-and-scope.md; 08-evidence-and-findings.md; README.md |
| 2. System context | section 20 | 12-diagrams.md |
| 4. Component responsibilities and boundaries | section 17; section 20 | 11-errors-audit-testing.md; 12-diagrams.md |
| 5. Backend architecture and request flow | section 8 | 05-policy-and-scope.md |
| 7. Assessment lifecycle and job orchestration | section 20; section 8; section 8 | 05-policy-and-scope.md; 12-diagrams.md |
| 8. Scope and policy enforcement | section 18; section 18 | 11-errors-audit-testing.md |
| 9. AI orchestration and provider abstraction | section 4 | 02-modular-monolith.md |
| 10. Security tool adapter architecture | section 11; section 12 | 08-evidence-and-findings.md |
| 13. Persistence and database boundaries | Section 16 | 10-credentials-and-deployment.md |
| 16. Deployment architecture: localhost web MVP | section 15; section 18; section 6; section 15; section 19 | 03-backend-and-frontend.md; 10-credentials-and-deployment.md; 11-errors-audit-testing.md; README.md |
| 17. Error handling, observability and auditabi | section 15; section 19 | 10-credentials-and-deployment.md; README.md |
| 18. Testing strategy and architectural boundar | section 19 | README.md |
| Open (not decided) | section 16; section 11; section 12; section 17; section 10 | 07-tool-adapters.md; 08-evidence-and-findings.md; 10-credentials-and-deployment.md; 11-errors-audit-testing.md |
| Revision log: MVP platform decision | Section 19; Section 19; Section 19; Section 19; Section 19; Section 19; Section 16; Section 2; Section 15; Sections 2, 15, 16; Sections 3 to 14, 17 | 01-vision-and-context.md; 02-modular-monolith.md; 04-assessment-and-jobs.md; 10-credentials-and-deployment.md; 11-errors-audit-testing.md; README.md |
| Revision log: foundation audit (revision 3) | Sections 8, 18, 19; Sections 3, 11, 19; Section 19; Sections 1, 3, 4, 12, 18, 19; Section 20; Section 19; Sections 16, 19 | 01-vision-and-context.md; 02-modular-monolith.md; 05-policy-and-scope.md; 08-evidence-and-findings.md; 10-credentials-and-deployment.md; 11-errors-audit-testing.md; 12-diagrams.md; README.md |
| Revision log: owner approvals, Round 1 (revisi | Section 3; Section 3; Section 19; Section 19 | 02-modular-monolith.md; README.md |
| Revision log: owner security decisions, Round  | Section 19; Section 19; Section 19; Section 19; sections 8 and 10; Sections 8, 10; Section 19 | 05-policy-and-scope.md; 07-tool-adapters.md; README.md |
| Revision log: owner decisions and B1 applied,  | section 19; Section 19; Section 19; Section 19; Section 19; Section 19; Section 19; Section 19 | README.md |
| Revision log: Round 5 foundation completion (r | Section 19; Section 19; Section 19; Section 19; section 4; sections 7, 14 and 16; section 8; sections 10 and 15; section 16; section 18; Sections 4, 7, 8, 10, 14, 15, 16, 18; Section 19 | 02-modular-monolith.md; 04-assessment-and-jobs.md; 05-policy-and-scope.md; 07-tool-adapters.md; 10-credentials-and-deployment.md; 11-errors-audit-testing.md; README.md |

## Checks to run after the real move

1. Concatenate the destination files in the manifest order and compare with the pre-move file. They must be identical, apart from the added file headers and link rewrites, which are listed in the move commit.
2. The set of defined IDs before and after must be equal (62 D, 64 P, 43 O including the resolved O-15 and O-28), and each ID must be defined exactly once.
3. Every ID referenced anywhere must resolve to a definition.
4. No file under `docs/` restates a table that is also in `.kiro/steering/`.
5. The steering file stays under the agreed size, and `inclusion: auto` still matches its description.

## Decisions requested

1. Approve the file layout and numbering, or edit the names.
2. Keep the registry as four files (decided, proposed, open, not-adopted), or merge them into one file per kind with a combined index.
3. Keep `inclusion: auto` for the concise steering file, or make it always-included now that it is short.
4. Approve moving the revision logs and the steering-alignment tables to `docs/decisions/`.
5. Confirm that Mermaid diagrams live in `docs/architecture/12-diagrams.md` and are linked, not embedded, from the steering file.
