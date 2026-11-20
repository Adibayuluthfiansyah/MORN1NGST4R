# Agent skills for development

Third-party skills used while developing MORN1NGST4R with Kiro CLI V3, plus a local-only OpenCode adaptation of nine of them. They guide the developer's AI agent. They are not part of the product runtime and are separate from the product's own `skills/` directory (AI skill packages for the platform).

## Layout

| Concern | Location |
| --- | --- |
| Installed skills (unmodified upstream copies) | `.kiro/skills/<name>/` |
| OpenCode project skills (adapted, local only, git-ignored) | `.opencode/skills/<skill-id>/` |
| OpenCode project instructions | `AGENTS.md` |
| OpenCode project config (skill permission allowlist) | `.opencode/opencode.json` |
| OpenCode provenance, pins and licenses (local only, git-ignored) | `.opencode/skills/third-party-notices/NOTICE.md` |
| Always-on Caveman rule (generated, local only, git-ignored) | `.kiro/steering/caveman.md` |
| Always-on Karpathy include and conflict rules | `.kiro/steering/karpathy-guidelines.md` |
| Skill routing policy | `.kiro/steering/skill-routing.md` |
| Skill routing prompt hook | `.kiro/hooks/skill-routing.json` |
| Project wiring: precedence, Superpowers bootstrap, Kiro tool mapping, artifact paths, Caveman scope | `.kiro/steering/engineering-workflow.md` |
| Design skill roles (auto-included) | `.kiro/steering/design-skills.md` |
| Content hashes of the local payload (generated, git-ignored) | `.kiro/skills-lock.json` |
| Pinned sources and install and verify script | `scripts/sync-agent-skills.sh` |
| Upstream license copies of the installed skills (local only, git-ignored) | `.kiro/skills-licenses/` |

## OpenCode project skills

OpenCode loads project skills from `.opencode/skills/<skill-id>/SKILL.md`. The directory name is the skill ID. Frontmatter `name` is display only, and `description` is what OpenCode advertises for model-driven selection. Supporting files are referenced relative to `SKILL.md`. There is no `load_resource` tool, routing hook, or session bootstrap: the model calls the `skill` tool by ID.

Nine skills are adapted from the Kiro payload for local OpenCode use:

`brainstorming`, `writing-plans`, `executing-plans`, `test-driven-development`, `systematic-debugging`, `verification-before-completion`, `requesting-code-review`, `receiving-code-review`, `karpathy-guidelines`.

The project config `.opencode/opencode.json` is the V2 permission allowlist for these nine IDs. Its ordered rules are a wildcard `skill` deny followed by one `skill` allow per ID. A `deny` hides matching skills from the model and rejects loading; an `allow` advertises and loads the skill without approval. The allowlist does not force the model to invoke a skill. OpenCode merges this project config with the global config; it does not replace the global model, provider, or other non-conflicting settings.

The copies are not verbatim. The adaptations:

- Remove Kiro and Claude Code host coupling: `superpowers:` prefixes, `load_resource`, `invoke_sub_agent`, `CLAUDE_*`, hooks, and the session bootstrap.
- Remove Git mutation, worktree, branch-finishing, and continuous-execution instructions. The owner performs all Git operations.
- Replace `docs/superpowers/` artifacts with `.kiro/specs/<feature>/design.md` and `.kiro/specs/<feature>/plan.md`. `.kiro/specs/mvp/plan.md` remains authoritative and is never overwritten.
- Replace generic commands with `make test`, `make test-go`, `make test-web`, `make test-db`, `make validate`, `make build-go`, and `make build-web`. `make test` runs `make test-go` and `make test-web`; `make test-db` is separate and is never implied by `make test`.
- Add project precedence, fail-closed security, scope and approval rules, milestone gates, and the human-only finding rule: AI findings are candidates, only a human may mark a product finding Verified with a validation record and linked evidence that pass integrity checks, and evidence alone never changes a finding's status.
- Apply the scope rule everywhere: every target, including self-owned systems and local labs, must be explicitly registered and evaluated against the applicable scope and address rules. No self-owned or local-lab exemption.
- Keep Critical and Important findings with the owner: an agent cannot waive, defer, or silently accept one. It stops and reports; only the owner may authorize risk acceptance or deferral.

`subagent-driven-development`, `dispatching-parallel-agents`, `writing-skills`, and the UI/design skills are deliberately not migrated. The Kiro copies stay unchanged in `.kiro/skills/`.

`AGENTS.md` at the repository root carries the project instructions OpenCode needs: the D-63 hierarchy, security and scope invariants, owner approvals, milestone workflow, debt classes, verification commands, and Git ownership. It summarizes only the rules the agent needs and references the authoritative `.kiro/steering/` and `docs/` files instead of copying whole sections. A summary never overrides the source.

Provenance, pinned commits, licenses, and adaptation notes are in `.opencode/skills/third-party-notices/NOTICE.md`. That file is git-ignored with the rest of `.opencode/skills/`.

### Local-only policy and fresh clones

`.opencode/skills/` is git-ignored. The adapted payload is local tooling and is not committed or redistributed. `AGENTS.md` and `.opencode/opencode.json` are tracked.

A fresh clone has the project instructions and permission allowlist but neither skill payload: both `.kiro/skills/` and `.opencode/skills/` are git-ignored. First restore the Kiro payload with `scripts/sync-agent-skills.sh install` (see below). Then recreate the adapted `.opencode/skills/` locally from `.kiro/skills/` using the adaptation rules above and the notice, or work without them. There is no sync script for the adapted copies. Neither payload is required for the application to build or run.

## Git policy

| Tracked | Git-ignored (local only) |
| --- | --- |
| `.kiro/steering/` (project-authored files only), `.kiro/specs/`, `.kiro/hooks/`, `scripts/sync-agent-skills.sh`, `docs/development/`, `AGENTS.md`, `.opencode/opencode.json` | `.kiro/skills/` (44 MB third-party payload), `.kiro/steering/caveman.md` (generated), `.kiro/skills-licenses/`, `.kiro/skills-lock.json`, `.opencode/skills/` (adapted payload), `.superpowers/`, `.worktrees/`, `.taste-skill/`, `.env*` |

## Fresh clone setup

The payload is not in Git. After cloning, run once:

```
scripts/sync-agent-skills.sh install
scripts/sync-agent-skills.sh verify
```

Until then, `engineering-workflow.md` and `karpathy-guidelines.md` each show `[Unresolved steering reference: file:.kiro/skills/... (notFound)]` and the skill catalogue is empty. This is expected, not a path error. The adapted `.opencode/skills/` copies are recreated only after this step, because their source is `.kiro/skills/`. Needs network, git, and Node.js (npx) for UI/UX Pro Max.

No custom agents are used. Steering `inclusion: always` replaces the upstream session-start hook, and one project hook (`skill-routing.json`) supports routing.

## Inventory

| Source | Pinned to | Installed skills |
| --- | --- | --- |
| [obra/superpowers](https://github.com/obra/superpowers) (MIT) | `22f0b46` | using-superpowers, brainstorming, writing-plans, executing-plans, test-driven-development, systematic-debugging, verification-before-completion, requesting-code-review, receiving-code-review, finishing-a-development-branch, subagent-driven-development, dispatching-parallel-agents, using-git-worktrees, writing-skills |
| [JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman) (Apache-2.0) | `b4ee71d` | caveman, plus steering rule |
| [nextlevelbuilder/ui-ux-pro-max-skill](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) (MIT) | repo `6ae86f6`, installer `ui-ux-pro-max-cli@2.15.0` | ui-ux-pro-max, ui-styling, design-system |
| [Leonxlnx/taste-skill](https://github.com/Leonxlnx/taste-skill) (MIT) | `f6b7657` | design-taste-frontend (upstream folder `skills/taste-skill`, renamed so the folder matches its `name:`) |
| [multica-ai/andrej-karpathy-skills](https://github.com/multica-ai/andrej-karpathy-skills) (MIT declared, no LICENSE file upstream) | `2c60614` (`main`) | karpathy-guidelines, plus always-on steering `karpathy-guidelines.md` that includes it by reference |

### License notes

- Nothing from these sources is committed or redistributed. The script writes the upstream license copies into the git-ignored `.kiro/skills-licenses/` for the local user.
- Karpathy Guidelines: upstream has no LICENSE file. MIT is only declared in its metadata. Treat it as local tooling only.
- UI/UX Pro Max: `ui-styling` declares `license: MIT` in its frontmatter but ships an Apache-2.0 `LICENSE.txt` with an unfilled copyright placeholder. This is unresolved upstream. This does not affect the project because the skill is never redistributed.

### Deliberately not installed

- `diagnosing-superpowers`: reads session logs and files GitHub issues.
- UI/UX Pro Max `brand`, `design`, `slides`, `banner-design`: logo, CIP and slide generators that need `GEMINI_API_KEY`, and they overlap with Taste's art-direction role.
- Caveman's other skills, CLI, proxy, MCP server and "Auto" cloud features: not needed for terse output, and some require an account or run a network proxy.
- Anti-AI-UI and similar: overlaps with Taste. Evaluate separately.

## Deviations from upstream install instructions

- Superpowers has no Kiro port. The bootstrap is delivered by steering (`#[[file:...]]` include of the installed `using-superpowers` skill) plus a tool-mapping table.
- UI/UX Pro Max's Kiro installer writes into `.kiro/steering/`. The script runs that installer (`--offline`, npm integrity pinned) and moves the output to `.kiro/skills/` so skills load on demand instead of as steering.
- Caveman's documented Kiro path is `npx skills add ... -g` (global, installs the selected skills) plus a steering file. The script copies the same files at a pinned commit, project-locally.
- Karpathy Guidelines has no Kiro adapter on upstream `main`. Two unmerged, unaffiliated PRs exist (#114 steering file, #129 skill under another name). Neither is used. The upstream `skills/karpathy-guidelines/SKILL.md` is already Agent Skills format and is copied unmodified. `scripts/sync-agent-skills.sh install karpathy` installs only this source.

## Skill routing

There are two separate routing systems. They share some skill names but do not share an enforcement mechanism.

### OpenCode routing (this project)

Policy: `AGENTS.md` section 10. OpenCode has no routing hook and no session bootstrap. The model sees the available skills' IDs and descriptions, then calls the `skill` tool with an exact ID. The routing table in `AGENTS.md` names the minimum required skills for each task class. It is guidance, not a deterministic dispatcher, and it cannot guarantee that the model follows it.

The project permission allowlist is `.opencode/opencode.json`. It uses the V2 `permissions` ruleset: a wildcard `{ "action": "skill", "resource": "*", "effect": "deny" }` followed by one `allow` rule per project skill. Rules run in order and the last matching rule wins. `deny` hides matching skills from the model and rejects loading; `allow` advertises the skill and loads it without approval. The allowlist does not force invocation.

OpenCode merges the project config with the global config; it does not replace the global file. Settings that do not conflict, such as the global model and providers, are preserved. Permission rules are combined in order: lower-priority configuration is loaded first, top-level rules next, and agent-specific rules last. A later matching rule wins, so verify the merged result instead of assuming the project file alone defines the policy. `opencode debug config` lists the configuration sources and the effective values.

After selecting skills, `AGENTS.md` requires this line:

```text
Skills: loaded: <names or none> | not loaded: <names with reason or none>
```

### Kiro routing (reference)

The following is the Kiro CLI V3 policy. It is separate from OpenCode; do not edit `.kiro/steering/skill-routing.md` or `.kiro/hooks/skill-routing.json` for OpenCode work.

Policy: `.kiro/steering/skill-routing.md` (always-on). Enforcement nudge: `.kiro/hooks/skill-routing.json`, one `UserPromptSubmit` hook with an `agent` action that appends a fixed reminder to every prompt (no script, no logic). Each request is classified, the matching skills are loaded with `load_resource` (`skill::<name>`), and the reply starts with `Route: <class> | loaded: ... | not loaded: ...` so a missed load is visible.

Why the hook exists: in fresh headless-session tests, steering alone loaded skills in 1 of 6 representative requests (the control with no routing file: 0 of 6). With the hook, all 18 runs (6 requests, 3 rounds) loaded the core skills for their class and the correct "none" cases loaded none. Optional extras in frontend runs varied between runs. Loading is not compliance: in a review test the agent loaded `requesting-code-review` but reviewed inline instead of dispatching a reviewer. Remove the hook and the policy still applies, but expect much weaker loading.

| Class | Skills loaded |
| --- | --- |
| Q&A, security analysis, documentation | none (documentation that cites commands: `verification-before-completion`) |
| Small feature | `test-driven-development`, `verification-before-completion` |
| Open-requirements feature, architecture | `brainstorming` (then `writing-plans` after design approval) |
| Bug | `systematic-debugging` (+ TDD and verification for the fix) |
| Review | `requesting-code-review` or `receiving-code-review` |
| Frontend | `design-skills` steering, `ui-ux-pro-max`, then `design-system`, `ui-styling`, `design-taste-frontend` as needed |
| Plan execution | `executing-plans` (adapted for OpenCode) |
| Worktrees, branch finish, parallel work, skill authoring | none. These skills are deliberately not migrated, and the owner performs every Git operation. Do not create a worktree, dispatch parallel agents, or invoke a branch-finishing skill. |

No installed skill covers security assessment or documentation. Security gates (scope, approval, no unrequested commit or push) come from `.kiro/steering/security.md` and `AGENTS.md`, and they outrank every skill. The routing file restates them for Kiro CLI; it is not their authority.

Adding or removing a skill: update the routing table in `skill-routing.md` in the same change. For OpenCode, update the `AGENTS.md` table and the `.opencode/opencode.json` allowlist together.

This is a dev-agent workflow. It is unrelated to the product's runtime AI skills in the repo-root `skills/` directory.

### Comparison

| Concern | Kiro CLI | OpenCode |
| --- | --- | --- |
| Routing policy | `.kiro/steering/skill-routing.md` (always-on steering) | `AGENTS.md` section 10 |
| Load call | `load_resource` with `skill::<name>` | `skill` tool with the exact ID |
| Enforcement nudge | `UserPromptSubmit` hook `.kiro/hooks/skill-routing.json` | No hook; permission allowlist only |
| Reply marker | `Route:` prefix with loaded/not-loaded lists | `Skills: loaded: ...` / `not loaded: ...` line |
| Determinism | Not deterministic; steering plus hook nudge | Not deterministic; instructions plus permissions |
| Available skills | Kiro payload, including skills not migrated to OpenCode | Only the nine IDs allowed by `.opencode/opencode.json` are advertised in this project |

### Verifying the OpenCode configuration

From the repository root:

```bash
python3 -m json.tool .opencode/opencode.json >/dev/null
opencode debug config
git diff --check
```

`opencode debug config` must list `.opencode/opencode.json` as a source and show the nine allow rules. Inspect every source for `permissions` rules, including `agents.<id>.permissions`: agent rules are appended after the top-level rules, and a later matching rule wins, so a global or agent rule can change the effective decision. This check confirms that every allowlisted ID matches a skill directory and that no extra skill is allowed:

```bash
python3 - <<'PY'
import json, os
cfg = json.load(open('.opencode/opencode.json'))
allowed = [r['resource'] for r in cfg['permissions'] if r['action'] == 'skill' and r['effect'] == 'allow']
dirs = {d for d in os.listdir('.opencode/skills') if os.path.exists(os.path.join('.opencode/skills', d, 'SKILL.md'))}
print('allowed:', allowed)
print('missing dirs:', sorted(set(allowed) - dirs))
print('unexpected dirs:', sorted(dirs - set(allowed)))
assert len(allowed) == 9
assert set(allowed) == dirs
PY
```

A successful parse, discovery, and ID match prove that the file is syntactically valid JSON, discovered, and consistent with the skill directories. They do not prove full conformance against a standards-compliant JSON Schema validator, and they do not show that the model will invoke a skill. The model still decides. The allowlist also cannot make a missing local skill payload appear; restore `.opencode/skills/` first (see Local-only policy and fresh clones).

## Updating

1. Review the upstream diff between the current and new commit (read scripts and hooks).
2. Change the pin in `scripts/sync-agent-skills.sh`. For UI/UX Pro Max also update the npm integrity (`npm view ui-ux-pro-max-cli@<v> dist.integrity`).
3. Run `scripts/sync-agent-skills.sh install` (all sources) or `install <source>` for one of `superpowers caveman taste ui-ux-pro-max karpathy`. It replaces only the managed directories and rewrites the lock file.
4. Review the changes to the script and docs and report them to the owner. The owner commits. The skill payload, the generated `caveman.md` and the license copies are git-ignored.
5. The OpenCode copies are not produced by the script. Re-adapt them manually against the same upstream pins and update `.opencode/skills/third-party-notices/NOTICE.md`; keep `.kiro/skills/` unchanged.

`scripts/sync-agent-skills.sh verify` works offline and reports any managed skill that differs from the local lock file. Do not edit installed skills. Put local behavior in steering.

## Runtime requirements and side effects

- UI/UX Pro Max: Python 3, standard library only. Its scripts make no network calls.
- Taste: Node 22+ and Chromium for its checker scripts. Fetching extra photographs or icons uses the network. Do not use it for in-app screens (see `design-skills.md`).
- `ui-styling/scripts/shadcn_add.py` runs `npx`. Require user approval first.
- Superpowers visual companion: optional, local server, loads a logo from the Superpowers site. Opt out with `SUPERPOWERS_DISABLE_TELEMETRY=1`.
- `.kiro/skills/` is about 44 MB, mostly Taste assets. It is git-ignored and recreated by the script on each checkout.

## Pending owner actions

This section records corrections that are outside the authorized file set for the post-migration corrective pass. The items previously listed here have been resolved by owner-approved reserved-document amendments:

- `.kiro/specs/mvp/plan.md` I-1 and I-2: the header now directs workers to the `executing-plans` skill and does not name `subagent-driven-development`; the M7 status records the merge in PR #9 at commit `f6501d2`. Applied.
- `.kiro/steering/skill-routing.md` line 50: now requires explicit registration and scope/address evaluation for every target, including self-owned systems and local labs, with no exemption; and requires a human, a validation record and linked evidence that pass integrity checks before a product finding can be marked Verified. Applied.
- `.kiro/steering/engineering-workflow.md` step 7: now reads "Give every deferred class C item an owner milestone and owner-assigned dates. If no owner milestone or date exists, mark it UNASSIGNED and schedule nothing. Never invent dates." Applied.

No unresolved items remain in this section.

## Windows

The script is bash. Use Git Bash or WSL. Several Superpowers helper scripts are bash too.
