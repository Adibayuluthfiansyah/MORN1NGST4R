# Documentation inventory

Status: reviewed in the design hardening round (2026-11-16). The repository is public on purpose for Kiroween 2026 and stays public. The aim is a focused, legally correct repository, not hidden security reasoning.

## Categories

| Category | Meaning | Default decision |
| --- | --- | --- |
| A | Public product and design documentation | Public |
| B | Public engineering evidence (research, feasibility, verification) | Public, after the safety scan below |
| C | Local-only or stale working material | Not public unless it has a clear public purpose |
| D | Third-party material | Local only and git-ignored. Never committed. |

## Inventory

A path ending in `/` covers everything under it. "Action" says what this round did.

| Path | Cat | Public decision | Reason | Action |
| --- | --- | --- | --- | --- |
| `README.md` | A | Public | Entry point, states the Apache-2.0 scope and the third-party limit | Third-party section added earlier, unchanged now |
| `LICENSE` | A | Public | Canonical Apache License 2.0 (SHA-256 `c71d239d...`). GitHub detects it as Apache-2.0. | Verified, unchanged |
| `.gitignore` | A | Public | Keeps secrets, runtime data, skill payloads and local license copies out | Added `.kiro/skills-licenses/` |
| `docs/README.md` | A | Public | Documentation index | New |
| `docs/PRD.md` | A | Public | Product requirements | Milestones corrected, NFR-REL-004 added |
| `docs/decisions/0001-owner-security-decisions-round-2.md` | A | Public | Owner decision record | None |
| `docs/decisions/0002-foundation-completion-decisions.md` | A | Public | Owner decision record | None |
| `docs/decisions/0003-foundation-closure-decisions.md` | A | Public | Owner decision record | Note on the license move in D-74. Supersession pointer for health endpoints in D-80 |
| `docs/decisions/0004-health-endpoints-mvp-decision.md` | A | Public | Owner decision record D-86: health endpoints in the MVP | New |
| `docs/decisions/0005-m1-scope-engine-closure-decisions.md` | A | Public | Owner decision record D-87 to D-91: M1 closure (CIDR minimum floor, SIIT, readiness gate, CIDR minimum policy versions, matched rule identifier) | New |
| `docs/decisions/README.md` | A | Public | Index and source-of-truth hierarchy | None |
| `docs/decisions/pending/README.md` | A | Public | Status of proposals and applied proposals | None |
| `docs/decisions/pending/S1-security-semantics.md` | A | Public | Detailed security specification (approved, D-64) | None |
| `docs/decisions/pending/C-product-decisions.md` | B | Public | Decision history, explains why items were decided | None |
| `docs/decisions/pending/F-password-recovery-proposal.md` | B | Public | History of the recovery decision (D-68) | None |
| `docs/decisions/pending/B1-security-review.md` | B | Public | Review record of the applied security patch | None |
| `docs/decisions/pending/B1-security.md.diff` | B | Public | The applied patch, kept as a record | None |
| `docs/decisions/pending/E1-architecture-split-mapping.md` | C | Candidate to retire | A proposal to split `architecture.md` that was never applied and is stale against revision 8. Its header says so. | Kept. **OWNER decision**: remove from the public tree or regenerate |
| `docs/decisions/pending/E1-architecture-steering.draft.md` | C | Candidate to retire | Same proposal, draft steering text | Kept. **OWNER decision** as above |
| `docs/design/` | A | Public | Database design, API contract, recovery generation, DQ register, review | Hardened this round |
| `docs/security/threat-model.md` | A | Public | The authoritative threat model | T-73 added |
| `docs/security/feasibility/` | B | Public | Sandbox and egress spike results. Reference-host description and `/tmp` scratch names are kept because they limit the claims. No credentials, no user names, no real targets. | Scanned, none |
| `docs/development/tech-verification/` | B | Public | Primary-source version and behavior checks with fetch dates | Scanned, none |
| `docs/development/agent-skills.md` | A | Public | How the development agent skills are installed and kept out of Git | License paragraphs updated |
| `docs/development/getting-started.md` | A | Public | How to run and test the first scaffold, what is a placeholder, and the open deviations | New |
| `docs/development/documentation-inventory.md` | A | Public | This file | New |
| `.kiro/steering/product.md` | A | Public | Product definition | None |
| `.kiro/steering/security.md` | A | Public | Security standards | None |
| `.kiro/steering/architecture.md` | A | Public | Architecture and decision register | None |
| `.kiro/steering/structure.md` | A | Public | Repository structure | None |
| `.kiro/steering/tech.md` | A | Public | Technology decisions | None |
| `.kiro/steering/skill-routing.md` | A | Public | Project-authored workflow rule | None |
| `.kiro/steering/engineering-workflow.md` | A | Public | Project-authored workflow rule (refers to an installed skill by include only) | None |
| `.kiro/steering/design-skills.md` | A | Public | Project-authored rule | None |
| `.kiro/steering/karpathy-guidelines.md` | A | Public | Project-authored wrapper that includes the installed skill by reference. No upstream text is copied. | None |
| `.kiro/steering/caveman.md` | D | Local only, git-ignored | Third-party rule text generated by the sync script | Removed from the tracked tree |
| `.kiro/specs/mvp/plan.md` | A | Public | Implementation plan | Milestones M16, M19, M21 corrected, gates added |
| `.kiro/hooks/skill-routing.json` | A | Public | Project hook | None |
| `.kiro/skills/` | D | Local only, git-ignored | Third-party skill payload, recreated by the sync script | None |
| `.kiro/skills-lock.json` | C | Local only, git-ignored | Generated hashes of the local payload | None |
| `.kiro/skills-licenses/` | D | Local only, git-ignored | Upstream license copies of the installed third-party skills | All copies are here |
| `scripts/sync-agent-skills.sh` | A | Public | Pinned installer for local tooling. Writes license copies to the local directory. | Updated |
| `scripts/validate-foundation.py` | A | Public | Foundation checks | Updated |
| `scripts/validate-design.py` | A | Public | Design checks, OpenAPI results reported separately | Updated |
| `scripts/spikes/` | B | Public | Throwaway feasibility code and the `last-run` logs that the feasibility records cite. Synthetic addresses only. | Scanned, none |

## Safety scan of the public tree

Run on the tracked and untracked-but-not-ignored files of `docs/`, `.kiro/` (outside the ignored payload), `scripts/` and the root files.

| Check | Result |
| --- | --- |
| Credentials, tokens, private keys, passwords | None found (`scripts/validate-foundation.py` safety scan) |
| Local absolute paths and user names | None. The only occurrence of the owner name is the public GitHub module path and the git remote. |
| Real target data, session material, request and response dumps | None. Spike fixtures use `.test` names and RFC 1918, RFC 5737 and loopback addresses as test data. |
| Private IP addresses that expose a real network | None. The RFC 1918 addresses are fixtures that the denial tests need. |
| Host-specific operational details | The reference host (distribution, kernel, tool versions) is named in the feasibility records and the threat model because it bounds what was verified. It carries no identifier of the person or the network. |
| Accidental third-party skill content | None tracked. The generated Caveman rule and the skill payload are git-ignored. |

Findings that are not an exposure but remain decisions for the owner: the two stale E1 proposal files (category C), and whether the roughly 150 KB of feasibility logs under `scripts/spikes/*/last-run/` stay in the tree.
