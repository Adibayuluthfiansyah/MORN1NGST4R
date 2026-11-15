---
inclusion: always
---

# Coding conduct: Karpathy guidelines

The `karpathy-guidelines` skill (installed unmodified in `.kiro/skills/`, pinned in `scripts/sync-agent-skills.sh`) is included below by reference so it applies to every coding task without a duplicate copy. Loading the skill again adds nothing. The payload is git-ignored: after a fresh clone run `scripts/sync-agent-skills.sh install` first, otherwise the include shows as unresolved.

#[[file:.kiro/skills/karpathy-guidelines/SKILL.md]]

## How it fits with other rules

- Superpowers decides which workflow runs. These guidelines set conduct inside any workflow, including small edits where no workflow skill fires. Where a Superpowers skill is stricter (test-driven-development, verification-before-completion), the stricter skill governs.
- "Use judgment on trivial tasks" lowers the depth of the work. It does not skip the skill check required by `using-superpowers`.
- Project steering outranks these guidelines in these cases:
  - Input validation, scope checks, approval checks and fail-closed behavior at trust boundaries are required by `security.md`. They are not "error handling for impossible scenarios".
  - Boundaries required by `structure.md` and `architecture.md` (domain, ports, adapters, separate runtimes) are not speculative abstractions.
  - A security or scope defect noticed in adjacent code is reported to the user in the reply. Do not fix it silently and do not ignore it.
