---
inclusion: always
---

# Skill routing

Applies to the development agent only. It is separate from the product's runtime AI skills in the repository-root `skills/` directory, which are never routing targets. If you are a dispatched subagent, skip this file and follow your prompt.

Already in context, never reload: `caveman` (style), `karpathy-guidelines` (conduct), `using-superpowers` (bootstrap). Project steering outranks every skill (see `engineering-workflow.md`).

## Protocol for every user request

1. State the outcome the user wants.
2. Classify it with the table below. Mixed request: take the union of the rows, process skills first.
3. Name the project rules that apply (`security.md`, `structure.md`, `product.md`, `architecture.md`).
4. Select the minimum skills from the matching rows. Never load a skill outside the matching rows. The "1% chance" rule in `using-superpowers` applies only inside the matching rows.
5. Load each selected skill with `load_resource` id `skill::<name>` (steering: `steering::<name>`), before any tool call that reads project files or changes anything, and before the first substantive answer. Issue independent loads in one turn. Instructions such as "one message", "do not write files" or "no commands" limit output and side effects, not loading: a skill load is read-only.
6. Check each load result. If a load failed or the skill does not exist, say so plainly. Never write "using X" for a skill that did not load. Continue only with that limit stated, or ask.
7. First line of the reply: `Route: <class> | loaded: <names or none> | not loaded: <every table skill for the class that was not loaded, with reason, or none>`. A table skill may be skipped only for a stated reason that is true (for example the user declined the workflow, or the skill needs a git range that does not exist).
8. Follow the loaded skills inside the hard limits below.
9. Before claiming a file change, fix, or result is done, load `verification-before-completion` if not yet loaded, run the proving command, and report its real output.

If the class is unclear, ask one focused question and end the turn. Take no read, write or command action before the answer, and do not choose an interpretation yourself, even if the session looks non-interactive. Load nothing until the user answers.

## Routing table

| Class | Examples | Load |
| --- | --- | --- |
| Q&A | explain, compare, look up | none |
| Small feature | one function or file, behavior and acceptance fully stated | `test-driven-development`, `verification-before-completion` |
| Feature with open requirements | new capability, behavior change, unclear scope | `brainstorming`. After the user approves the design: `writing-plans` |
| Architecture or spec | design, ADR, tradeoffs, no code yet | `brainstorming` |
| Execute an approved plan | the user says go on an existing plan | `using-git-worktrees` if isolation is wanted, then `executing-plans` (default) or `subagent-driven-development` (only if the user chose it), plus `test-driven-development`, `verification-before-completion` |
| Bug or failure | failing test, wrong output, crash, regression | `systematic-debugging`; when fixing add `test-driven-development`, `verification-before-completion` |
| Review a change | diff, PR, branch, file set | `requesting-code-review` |
| Review feedback received | comments to act on | `receiving-code-review` |
| Frontend design | design system, palette, type, layout, UX, accessibility | steering `design-skills`, `ui-ux-pro-max`. Add `design-system` for tokens or Tailwind theme, `ui-styling` for shadcn/Tailwind components, `design-taste-frontend` for art direction or polish after the system exists. New screen with open requirements: `brainstorming` first |
| Security analysis | vulnerability analysis, threat model, scope or approval logic | none required. Apply the security gates below. Add `systematic-debugging` for a failing behavior, `requesting-code-review` for a diff |
| Documentation | docs, README, ADR text, guides | none. Place files per `structure.md` section 14. Docs that cite commands or results: `verification-before-completion` |
| Skill work | create, edit, test a dev skill | `writing-skills` |
| Branch workflow | worktree, wrap up a branch | `using-git-worktrees` or `finishing-a-development-branch` |
| Parallel work | 2 or more independent tasks | `dispatching-parallel-agents` |

No installed skill covers security assessment or documentation. Do not imply one does.

## Hard limits (skills cannot relax these)

- No commit, push, merge, PR, branch or worktree deletion unless the user asks for it in that request, even where a skill says to commit often or finish the branch.
- Plan-execution skills say to keep going without asking. Stop and ask anyway for irreversible actions, security-sensitive actions, or anything outside the approved scope.
- Security: before any active testing, the target must be explicitly registered and evaluated against the applicable scope and address rules. This applies to every target, including self-owned systems and local labs; there is no self-owned or local-lab exemption. Ambiguous or unknown targets are not tested. Analysis of our own code needs no target. Findings from AI analysis are candidates, not Verified. Only a human may mark a product finding Verified, and only when its validation record and linked evidence pass the integrity checks. Evidence alone never changes a finding's status.
- Specs go in `.kiro/specs/<feature>/`. Docs go in `docs/`. Do not edit `product.md`, `security.md`, `structure.md`, `architecture.md`, `tech.md` or installed skills. Skill updates go through `scripts/sync-agent-skills.sh`.
- Frontend: this is an operator application. Use Taste only in the app-screen mode described in `design-skills.md`.
- If the user names a skill or says `/skill-name`, load it. It overrides the selection above but not these limits.
