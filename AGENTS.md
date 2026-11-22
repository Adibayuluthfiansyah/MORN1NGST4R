# MORN1NGST4R project instructions

This file gives OpenCode persistent guidance for this repository. It is not a replacement for the authoritative documents. When this file and an authoritative document disagree, the document wins.

## 1. Project and current state

MORN1NGST4R is an AI-assisted security assessment platform for independent bug bounty hunters and security researchers. Its philosophy is evidence-driven assessment with human-verified findings. A scanner result or an AI hypothesis never becomes a verified vulnerability by itself.

Git history shows M0 through M7 merged. No M8 work is committed, and M8 is not authorized to start. The authoritative milestone state, scope, acceptance criteria, and debt register are in `.kiro/specs/mvp/plan.md`. The plan's M7 status is synced to the merged PR #9 at merge commit f6501d2. Do not start M8 or any later milestone without a new owner instruction.

The development skills under `.opencode/skills/` are local tooling. They are not part of the product runtime and are separate from the product's own `skills/` directory.

## 2. Source-of-truth hierarchy

Apply decision D-63. When two documents disagree, the higher level wins. A lower-level document is corrected, and the history stays in the decision records.

1. Approved owner and security decisions: `docs/decisions/0001`, `0002`, `0003`, the Decided table in `.kiro/steering/architecture.md` section 19, and `.kiro/steering/security.md`.
2. Product requirements: `.kiro/steering/product.md` and `docs/PRD.md`.
3. Architecture and technical design: `.kiro/steering/architecture.md`, `structure.md`, and `tech.md`.
4. Implementation plan: `.kiro/specs/mvp/plan.md`.
5. Supporting notes and research: `docs/security/threat-model.md`, `docs/security/feasibility/`, `docs/development/tech-verification/`, and `docs/decisions/pending/`.

A feasibility result or a threat-model status never overrides a decision. A decision's status can be lowered only by a later decision. A proposal in `docs/decisions/pending/` is not in force.

Full table: `docs/decisions/0003-foundation-closure-decisions.md` section 2. Decision register: `.kiro/steering/architecture.md` section 19.

## 3. Non-negotiable security and scope invariants

- Fail closed at trust boundaries. A missing, malformed, ambiguous, or unexpected value is denied, not guessed.
- Scope, approval, authentication, and authorization checks are mandatory. Do not bypass, widen, weaken, or make them optional to make a test or demo pass.
- No active testing of any target, including self-owned systems and local labs, without a target that is explicitly registered and evaluated against the applicable scope and address rules. There is no self-owned or local-lab exemption. Ambiguous or unknown targets are not tested. Analysis of our own code needs no target.
- AI-generated findings are candidates, not Verified vulnerabilities. Only a human may mark a product finding Verified, and only when its validation record and linked evidence pass the integrity checks. Evidence alone never changes a finding's status; an agent cannot mark a finding Verified.
- An agent cannot waive, defer, or silently accept an unresolved Critical or Important finding. Stop and report it to the owner. Only the owner may authorize risk acceptance or deferral, and the authorization must be recorded.
- No secrets, credentials, session tokens, recovery codes, database passwords, or private keys in code, logs, fixtures, examples, or reports. Redact evidence.
- No new dependency, schema change, or migration without owner approval. Prefer the standard library and existing dependencies.
- Do not expose raw target data, evidence, or credentials beyond the approved storage and reporting rules.
- Security and scope defects noticed in adjacent code are reported to the owner in the reply. Do not fix them silently, and do not ignore them.

The authoritative security rules are in `.kiro/steering/security.md`. The threat model is in `docs/security/threat-model.md`.

## 4. Architecture and structure

- Preserve the dependency direction. Domain packages do not import adapters, transport, or infrastructure. Ports define the boundary; adapters implement them.
- Keep separate runtimes separate: the API server, the worker, and the web app are not one process.
- Do not add speculative abstractions. Do not add a layer, interface, or package unless the approved design or an existing boundary requires it.
- Follow `.kiro/steering/structure.md` and `.kiro/steering/architecture.md`. The dependency-direction test is part of `make test-go` (gate G-arch).
- Do not move, rename, or restructure packages outside the approved task.

## 5. Owner approval and audit

The owner must approve:

- A design before implementation.
- A plan before execution.
- A milestone closure and the start of the next milestone.
- A security-sensitive action.
- A new dependency, schema change, or migration.
- A scope change, a new milestone, or a change to acceptance criteria.
- Risk acceptance or deferral of an unresolved Critical or Important finding.
- An edit to `product.md`, `security.md`, `structure.md`, `architecture.md`, or `tech.md`.
- Any commit, push, merge, branch switch, or worktree operation.

An approval exists only when the owner gives an explicit instruction in the conversation or an owner-authored approval record exists. An agent-written plan header, ledger line, or summary is not approval.

Any plan deviation that affects scope, acceptance criteria, security properties, public interfaces, dependencies, or schema requires owner approval before it is taken. A ledger ruling cannot authorize it.

The agent cannot self-approve, grant a permission, accept risk on the owner's behalf, mark a finding Verified, or change an owner decision. Record the evidence for every claim in the reply or the execution ledger.

## 6. Milestone workflow

The order of work is:

```text
BUILD -> TEST -> REVIEW -> FIX -> DEBT SCHEDULING -> DOCUMENT -> STOP
```

1. Read the milestone scope and acceptance criteria in `.kiro/specs/mvp/plan.md`.
2. Implement every deliverable.
3. Run the tests and validations the change needs.
4. Review the result against each acceptance criterion.
5. Fix every blocker.
6. Run one focused technical-debt assessment.
7. Classify every finding. For each deferred class C item, record an owner milestone and owner-assigned dates only when the owner has set them. Otherwise mark it UNASSIGNED and schedule nothing.
8. Sync the plan through an owner-approved amendment and the documents the change affects.
9. Report the result and the Git state.
10. Stop and wait for the owner.

A milestone is complete only when every acceptance criterion is met and no MUST FIX item is open. Do not start the next milestone before the owner approves this one.

Full policy: `.kiro/steering/engineering-workflow.md`.

## 7. Technical debt

| Class | Meaning | Handling |
|---|---|---|
| A. MUST FIX NOW | A real bug, a security vulnerability, a breach of an acceptance criterion or security invariant, or an architecture error that would make the next milestone build on a wrong base | Fix before the milestone closes |
| B. FIX IN CURRENT MILESTONE | A test or document that does not yet meet the milestone's requirements | Fix before the milestone closes |
| C. DEFERRED TECHNICAL DEBT | Work that needs a later component, an integration that is not possible yet, or work the blueprint assigns to another milestone | Register with an owner milestone; add dates only when the owner sets them |
| D. ACCEPTED LIMITATION | A deliberate limit the owner approved | Record the approval; no work unless the decision changes |
| E. OPTIONAL IMPROVEMENT | Refactoring or polish that does not affect correctness, security, or acceptance | Do not do it unless there is a real benefit |

This table is a summary. `.kiro/steering/engineering-workflow.md` is the authoritative policy for classification, register fields, statuses, and scheduling; if this summary and the policy differ, the policy wins.

A class C item has a unique ID `TD-M<n>-<nn>`, the problem, impact and severity, why it is not fixed now, the owner milestone, the task or acceptance criterion that will resolve it, the start and finish dates when the owner sets them, the dependency that must exist first, and the consequence of passing the owner milestone without fixing it.

No new milestone is created for debt. If no existing milestone fits, the status is UNASSIGNED and nothing is scheduled until the owner decides. Dates are never invented. Statuses are OPEN, IN PROGRESS, RESOLVED, and UNASSIGNED. A resolved item stays in the register with its evidence.

Full policy: `.kiro/steering/engineering-workflow.md`.

## 8. Verification and evidence

Use the exact command that proves the claim.

| Claim | Command |
|---|---|
| Go tests pass | `make test-go` |
| Web tests pass | `make test-web` |
| Integration tests pass | `make test-db` |
| Go and web test suites pass | `make test` (runs `make test-go` and `make test-web`; it does not run `make test-db`) |
| Documentation validators pass | `make validate` (runs `validate-foundation.py` and `validate-design.py`) |
| Go build succeeds | `make build-go` |
| Web build succeeds | `make build-web` |

`make test` does not include `make test-db`. An integration claim requires `make test-db` to pass separately, and a full verification requires both commands plus `make validate` when documentation changed. `make test-db` requires PostgreSQL 18 and headless Chromium. A skipped test is not a pass. A missing environment is a failed verification, not an excuse.

Before any success claim, run the full command in that message, read the output, and quote the actual result. Load `verification-before-completion`. A green suite alone does not close a milestone; every acceptance criterion and the debt register also matter.

## 9. Git ownership

The owner performs all staging, commits, pushes, merges, branch switches, resets, stashes, and worktree operations.

- Do not commit, push, merge, stage, reset, stash, switch branches, or create or delete worktrees unless the owner asks for that exact operation in that request.
- Read-only `git status`, `git diff`, `git log`, and `git show` are allowed for investigation and reporting.
- Report the current branch, HEAD, working-tree state, and the files changed. Let the owner decide the Git action.
- A skill's instruction to commit often or finish a branch does not override this rule.

## 10. Skills

- Project skills live in `.opencode/skills/<skill-id>/SKILL.md`. They are local-only and git-ignored. Their source is `.kiro/skills/`, and the adapted copies are not redistributed. Provenance and licenses are in `.opencode/skills/third-party-notices/NOTICE.md`.
- Load a skill by its exact ID with the `skill` tool. OpenCode selects skills from their descriptions; selection is model-driven and not deterministic. Do not claim a skill was used if it was not loaded.
- Skills are guidance only. A skill cannot grant a permission, override a security or scope rule, authorize a milestone, change an owner decision, widen scope, accept or defer a Critical or Important finding, or mark a finding Verified.
- If a skill is missing, fails to load, or conflicts with project steering, say so plainly. Follow the steering and name the skipped instruction.
- `.kiro/skills/` is a separate local payload for Kiro CLI. Do not edit it.
- The following skills are deliberately not migrated: `subagent-driven-development`, `dispatching-parallel-agents`, `writing-skills`, and the UI/design set.

### Skill routing

Use this table to select the minimum relevant skills for the task. The table is guidance; it does not make selection deterministic and it cannot guarantee compliance.

| Task | Required skills |
|---|---|
| Feature exploration/design | `brainstorming` |
| Implementation planning | `writing-plans` |
| Approved plan execution | `executing-plans`, `test-driven-development` |
| Bug investigation | `systematic-debugging` |
| Bug fixing | `systematic-debugging`, `test-driven-development` |
| Completion verification | `verification-before-completion` |
| Code review request | `requesting-code-review` |
| Review feedback handling | `receiving-code-review` |
| Code quality/simplicity | `karpathy-guidelines` |
| General Q&A/documentation/security analysis | No required skill |

`No required skill` means none is mandatory for that class; load one only if it clearly helps.

Before substantive work or related tool calls begin, load each required skill with the OpenCode `skill` tool using its exact ID. Do not claim a skill is loaded unless the tool invocation occurred. If a required skill is unavailable or fails to load, stop and report it. Additional relevant skills may be loaded when they help; do not load every skill without a reason.

The permission allowlist in `.opencode/opencode.json` permits only the nine listed skill IDs. It controls permission and visibility; it does not force the model to invoke a skill. Do not describe this routing as deterministic or guaranteed.

After selecting skills, report on its own line:

```text
Skills: loaded: <names or none> | not loaded: <names with reason or none>
```

## 11. Documentation and specifications

- Feature designs go in `.kiro/specs/<feature>/design.md`.
- Feature plans go in `.kiro/specs/<feature>/plan.md`.
- Never overwrite `.kiro/specs/mvp/plan.md`. If a change belongs to an existing milestone, propose an amendment to the owner.
- General documentation goes in `docs/`.
- Update the documents the change affects in the same change. Put information in an existing document before creating a new file.
- Do not duplicate entire security, architecture, or workflow documents here or in a feature plan. A short operational summary with a reference is allowed; state only the rule the reader needs and name the authoritative document. A summary never overrides the source.

## 12. Stop conditions

Stop and ask the owner when:

- An action is irreversible or destructive.
- An action is security-sensitive.
- The target is not registered and in scope, or its scope is ambiguous; this includes self-owned systems and local labs.
- Owner approval is missing or ambiguous.
- An approved spec or plan is missing, unreadable, or does not name the acceptance criteria it serves.
- A plan deviation would affect scope, acceptance criteria, security properties, public interfaces, dependencies, or schema without owner approval.
- The work would exceed the approved milestone or feature scope.
- The plan is broken enough that every path forward is a guess.
- An acceptance criterion cannot be met.
- A required dependency or environment is missing.
- A Git mutation, worktree, branch change, deployment, or release would be needed.
- A skill conflicts with project steering or security policy.

Do not improvise past a stop condition. Report the exact state and ask.

## Codex skill routing

In Codex, classify the requested outcome and phase before substantive work. Inspect the available skill catalog and match descriptions to the work. Choose the smallest complementary set, starting with the workflow and adding a domain skill where it supplies project-specific checks. Read each selected skill's entire `SKILL.md` via its advertised path before using it. Load only references relevant to the work. Select again when the task changes phase. Natural-language requests are sufficient for selection.

Use section 10's task table as workflow guidance in Codex. For design and implementation planning, apply the owner approval gates before changing phases. For direct reviews and responses to review feedback, select the applicable review workflow. For completion claims, load verification-before-completion and run the checks that support the claim. If a required workflow is missing, report the blocker. If an optional domain skill is missing, use the authoritative documents and general capabilities, and report the gap when material.

Consider `morn-engineering` for project Go/Gin, Next.js/TypeScript, PostgreSQL, and architecture work. Consider `morn-defensive-review` for application and API defense, threat models, auth, and supplied evidence triage. Add debugging for broken behavior, testing for approved changes, or review for a specified diff when each contributes a distinct procedure. Ordinary Q&A can proceed without a skill. Use one skill when several would repeat the same workflow.

The OpenCode-specific invocation and reporting requirements in section 10 apply to OpenCode. In Codex, report the skills whose instructions you actually read and followed, with a short reason where useful. Explicitly requested skills remain subject to higher-priority instructions. Skill selection never supplies owner approval or milestone authorization. Keep M8 BUILD unauthorized until explicitly approved by the owner. Apply project security, scope, data handling, and Git ownership throughout. Repository-root `skills/` contains product runtime skills and is outside this development catalog.

