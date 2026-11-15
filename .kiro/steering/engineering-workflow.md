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

## Caveman scope

The always-on `caveman.md` steering file applies to chat replies only. In this project:

- Specs, plans, ADRs, security documents, reports, commit messages, PR text and code comments are written in normal prose.
- Never compress away security requirements, scope or approval conditions, acceptance criteria, test names and assertions, or architectural decisions and their reasons.
- Verification claims quote the exact command and its actual result.
- Questions to the user, security warnings and irreversible-action confirmations use full sentences.
- "stop caveman" or "normal mode" turns it off for the session.
