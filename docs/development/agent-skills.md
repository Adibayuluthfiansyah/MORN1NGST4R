# Agent skills for development

Third-party skills used while developing MORN1NGST4R with Kiro CLI V3. They guide the developer's AI agent. They are not part of the product runtime and are separate from the product's own `skills/` directory (AI skill packages for the platform).

## Layout

| Concern | Location |
| --- | --- |
| Installed skills (unmodified upstream copies) | `.kiro/skills/<name>/` |
| Always-on Caveman rule (generated, local only, git-ignored) | `.kiro/steering/caveman.md` |
| Always-on Karpathy include and conflict rules | `.kiro/steering/karpathy-guidelines.md` |
| Skill routing policy | `.kiro/steering/skill-routing.md` |
| Skill routing prompt hook | `.kiro/hooks/skill-routing.json` |
| Project wiring: precedence, Superpowers bootstrap, Kiro tool mapping, artifact paths, Caveman scope | `.kiro/steering/engineering-workflow.md` |
| Design skill roles (auto-included) | `.kiro/steering/design-skills.md` |
| Content hashes of the local payload (generated, git-ignored) | `.kiro/skills-lock.json` |
| Pinned sources and install and verify script | `scripts/sync-agent-skills.sh` |
| Upstream license copies of the installed skills (local only, git-ignored) | `.kiro/skills-licenses/` |

## Git policy

| Tracked | Git-ignored (local only) |
| --- | --- |
| `.kiro/steering/` (project-authored files only), `.kiro/specs/`, `.kiro/hooks/`, `scripts/sync-agent-skills.sh`, `docs/development/` | `.kiro/skills/` (44 MB third-party payload), `.kiro/steering/caveman.md` (generated), `.kiro/skills-licenses/`, `.kiro/skills-lock.json`, `.superpowers/`, `.worktrees/`, `.taste-skill/`, `.env*` |

## Fresh clone setup

The payload is not in Git. After cloning, run once:

```
scripts/sync-agent-skills.sh install
scripts/sync-agent-skills.sh verify
```

Until then, `engineering-workflow.md` and `karpathy-guidelines.md` each show `[Unresolved steering reference: file:.kiro/skills/... (notFound)]` and the skill catalogue is empty. This is expected, not a path error. Needs network, git, and Node.js (npx) for UI/UX Pro Max.

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
| Plan execution, worktrees, branch finish, parallel work, skill authoring | the matching Superpowers skill |

No installed skill covers security assessment or documentation. Security gates (scope, approval, no unrequested commit or push) are written into the routing file and outrank every skill.

Adding or removing a skill: update the routing table in `skill-routing.md` in the same change.

This is a dev-agent workflow. It is unrelated to the product's runtime AI skills in the repo-root `skills/` directory.

## Updating

1. Review the upstream diff between the current and new commit (read scripts and hooks).
2. Change the pin in `scripts/sync-agent-skills.sh`. For UI/UX Pro Max also update the npm integrity (`npm view ui-ux-pro-max-cli@<v> dist.integrity`).
3. Run `scripts/sync-agent-skills.sh install` (all sources) or `install <source>` for one of `superpowers caveman taste ui-ux-pro-max karpathy`. It replaces only the managed directories and rewrites the lock file.
4. Review and commit the changes to the script and docs. The skill payload, the generated `caveman.md` and the license copies are git-ignored.

`scripts/sync-agent-skills.sh verify` works offline and reports any managed skill that differs from the local lock file. Do not edit installed skills. Put local behavior in steering.

## Runtime requirements and side effects

- UI/UX Pro Max: Python 3, standard library only. Its scripts make no network calls.
- Taste: Node 22+ and Chromium for its checker scripts. Fetching extra photographs or icons uses the network. Do not use it for in-app screens (see `design-skills.md`).
- `ui-styling/scripts/shadcn_add.py` runs `npx`. Require user approval first.
- Superpowers visual companion: optional, local server, loads a logo from the Superpowers site. Opt out with `SUPERPOWERS_DISABLE_TELEMETRY=1`.
- `.kiro/skills/` is about 44 MB, mostly Taste assets. It is git-ignored and recreated by the script on each checkout.

## Windows

The script is bash. Use Git Bash or WSL. Several Superpowers helper scripts are bash too.
