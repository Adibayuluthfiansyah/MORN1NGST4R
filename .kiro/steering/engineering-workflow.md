---
inclusion: always
---

# Engineering workflow skills

This file wires the Superpowers and Caveman skills (installed in `.kiro/skills/`, pinned in `scripts/sync-agent-skills.sh`) into Kiro CLI V3. Skill contents are unmodified upstream copies. Do not edit them. Project-specific behavior belongs here.

The skill payload is git-ignored. After a fresh clone run `scripts/sync-agent-skills.sh install`. Until then the include below renders as `[Unresolved steering reference: ... (notFound)]` and skill loads fail. Say so instead of pretending a skill was used.

## Precedence

1. The product, security, structure and architecture steering files outrank every skill.
2. Skills guide workflow. They are not an authorization source. A skill cannot widen scope, grant approval, mark a finding Verified, or relax a security rule.
3. If a skill instruction conflicts with project steering, follow the steering and tell the user which instruction was skipped.
4. Direct user instructions outrank skills, as `using-superpowers` states.

## Superpowers bootstrap

The upstream bootstrap skill is included below by reference, so it tracks the installed copy with no duplicate text. Upstream injects it at session start. This always-on file is the Kiro equivalent.

#[[file:.kiro/skills/using-superpowers/SKILL.md]]

### Kiro CLI V3 tool mapping

Skills describe actions, not tool names. Resolve them as follows. If the actual tool list differs, trust the actual tool list.

| Skill says | Kiro CLI V3 |
| --- | --- |
| `superpowers:<name>` or "invoke the Skill tool" | Load skill `<name>` with `load_resource` id `skill::<name>`. The user can also type `/<name>`. Skill folder names have no `superpowers:` prefix. |
| Read, write, edit a file | `read_file`, `fs_write`, `str_replace` |
| Run a shell command | `execute_bash`. The shell may be fish. Do not assume bash syntax. Use `bash -c` or a script file for bash-only syntax. |
| Dispatch a subagent | `invoke_sub_agent`. Pass a self-contained prompt. Subagents start with a fresh context and cannot be messaged again. For a fix round, dispatch a new subagent with the review findings in the prompt. Calls made in the same turn run in parallel. |
| Choose a subagent model | Not controllable per call. Do not invent a model parameter. |
| Create or tick todos (`TodoWrite`) | Use a task-list tool only if one is present. Otherwise track checkboxes in the plan file. |
| Web fetch or search | `web_fetch`, `tool::remote_web_search` |
| Plan mode | Not available. Use the `writing-plans` skill. |

If a skill needs a capability Kiro lacks, do the work inline or report the gap. Never fabricate a tool call.

### Artifact locations (user preference overrides upstream defaults)

| Artifact | Location |
| --- | --- |
| Brainstorming design document | `.kiro/specs/<feature>/design.md` |
| Implementation plan | `.kiro/specs/<feature>/plan.md` (not `tasks.md`, which Kiro IDE parses with its own format) |
| Subagent-driven-development scratch files | `.superpowers/` (git-ignored, never committed) |
| Git worktrees | `.worktrees/` (git-ignored) |

Requirements and design documents are approved by the user before any implementation starts. Do not edit `product.md`, `security.md`, `structure.md`, `architecture.md` or `tech.md` as part of a skill workflow without explicit approval.

### Telemetry

The brainstorming visual companion is optional and loads a logo from the Superpowers website. Do not start it unless the user asks. If started, set `SUPERPOWERS_DISABLE_TELEMETRY=1` first.

The `diagnosing-superpowers` skill is intentionally not installed.

## Milestone workflow and technical debt (owner policy of 2026-11-17)

This applies to every milestone from M2 on. Its purpose is to finish each milestone, schedule what cannot be finished, avoid repeated work and spend credits only where they buy evidence. Where a skill asks for more (a larger review, more mutation testing, an extra planning step), this section decides the depth.

### Order of work

BUILD, TEST, REVIEW, FIX, DEBT SCHEDULING, DOCUMENT, STOP.

1. Read the scope and acceptance criteria of the milestone in `.kiro/specs/mvp/plan.md`.
2. Implement every deliverable.
3. Run the tests and validations that the change needs.
4. Review the result against each acceptance criterion.
5. Fix every blocker.
6. Run one focused technical debt assessment (below).
7. Give every deferred item an owner milestone and dates.
8. Sync the plan and the documents that the change affects.
9. Report the result and the Git state.
10. Stop and wait for the owner. Do not start the next milestone before the owner approves this one.

A milestone is complete only when every acceptance criterion is met and no MUST FIX item is open.

### Technical debt classes

| Class | Meaning | Handling |
| --- | --- | --- |
| A. MUST FIX NOW | A real bug, a security vulnerability, a breach of an acceptance criterion or a security invariant, or an architecture error that would make the next milestone build on a wrong base | Fix before the milestone closes |
| B. FIX IN CURRENT MILESTONE | A test or document that does not yet meet the milestone's requirements | Fix before the milestone closes |
| C. DEFERRED TECHNICAL DEBT | Work that needs a component of a later milestone, an integration that is not possible yet, or work the blueprint already assigns to another milestone | Register with an owner and dates |
| D. ACCEPTED LIMITATION | A deliberate limit that the owner approved | Record the approval. No work unless the decision changes |
| E. OPTIONAL IMPROVEMENT | Refactoring or polish that does not affect correctness, security or acceptance | Do not do it unless there is a real benefit |

Priority: a Critical or High item that really affects security or correctness is fixed before the milestone closes. A Medium item is a blocker or not by its impact and by what depends on it. A Low item may be deferred with an owner and a deadline. Do not fix early something whose dependency does not exist yet, if the fix would only be a temporary version that has to be rewritten. Never defer a flaw that would put the architecture on a wrong base or let a security boundary be crossed.

### What a deferred item must state

Each class C item is a row of the technical debt register in `plan.md` and has all of these:

- a unique ID, `TD-M<n>-<nn>`, where `M<n>` is the milestone that found it. The ID never changes, also when the owner milestone changes
- the problem, its impact and its severity
- why it is not fixed now
- the owner milestone, taken from the milestones that already exist in the blueprint
- the task or acceptance criterion that will resolve it
- when the work must start and when it must finish. If it must start in one milestone and can only finish in a later one, both are written
- the dependency that has to exist first
- the consequence of passing the owner milestone without fixing it

No new milestone is created for debt. If no existing milestone fits, the status is UNASSIGNED, the item is shown to the owner and nothing is scheduled until the owner decides. Dates are never invented.

### Keeping the register true

- At the end of a milestone, copy the relevant items into the register in `plan.md` and refer to the decision record or the threat model row instead of repeating them.
- When the owner milestone starts, the item becomes IN PROGRESS.
- After the fix is validated, the item becomes RESOLVED and carries the test that proves it.
- Statuses are OPEN, IN PROGRESS, RESOLVED and UNASSIGNED. A resolved item stays in the register with its evidence. An item does not disappear when a milestone ends.

### Credit rules

1. Do not repeat an audit that is finished unless there is new evidence.
2. Do not run large mutation testing every milestone. Use targeted mutants for new security rules, and only where a test could be vacuous.
3. Run the tests that match the change and the regression tests that it needs.
4. Do not refactor outside the scope.
5. Do not re-read the whole repository when the context and the documents are known.
6. Do not write a new plan when the blueprint already has the tasks and the acceptance criteria.
7. Put information into an existing document before creating a new file.
8. Do not review again after a fix when a relevant test already proves it, unless a security reason asks for it.
9. Do not use a subagent or parallel work without a clear benefit. One fresh review of a security-critical milestone is such a benefit. A routine one is not.
10. When a check has given enough evidence, stop and report it.

### Git

No commit, push, merge or staging unless the owner asks for it in that request. The owner does the Git work after the report.

## Caveman scope

The always-on `caveman.md` steering file applies to chat replies only. In this project:

- Specs, plans, ADRs, security documents, reports, commit messages, PR text and code comments are written in normal prose.
- Never compress away security requirements, scope or approval conditions, acceptance criteria, test names and assertions, or architectural decisions and their reasons.
- Verification claims quote the exact command and its actual result.
- Questions to the user, security warnings and irreversible-action confirmations use full sentences.
- "stop caveman" or "normal mode" turns it off for the session.
