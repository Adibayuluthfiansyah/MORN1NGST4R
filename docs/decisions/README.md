# Decision records

This folder holds the owner decision records and their index. The decision register itself (D-, P- and O- identifiers with their status) is in `.kiro/steering/architecture.md` section 19. Identifiers are never reused or renumbered.

| Record | Round | IDs | What it decides |
| --- | --- | --- | --- |
| `0001-owner-security-decisions-round-2.md` | 2 and 3 | D-30 to D-46 | Approval lifecycle, sandbox and egress direction, scope grammar, AI egress, localhost security mode |
| `0002-foundation-completion-decisions.md` | 5 | D-47 to D-62 | Conservative resolutions of the Round 4 findings, authentication and validation options, technology baseline |
| `0003-foundation-closure-decisions.md` | 6 | D-63 to D-85 | Approval of `security.md` and S1, interpretive choices 1 to 14, technology and provisioning, authentication bootstrap and recovery, sandbox limits, accepted residual risks, plan and PRD reconciliation, repository hygiene |
| `pending/` | | | Proposals, and records of proposals that were applied. A proposal there is not in force. |

## Source-of-truth hierarchy (D-63)

When two documents disagree, the higher level wins and the lower document is corrected. History stays in the decision records.

1. Approved owner and security decisions: the records above, the Decided table of `architecture.md` section 19, and `.kiro/steering/security.md`.
2. Product requirements: `.kiro/steering/product.md` and `docs/PRD.md`.
3. Architecture and technical design: `.kiro/steering/architecture.md`, `structure.md`, `tech.md`.
4. The implementation plan: `.kiro/specs/mvp/plan.md`.
5. Supporting notes and research: `docs/security/threat-model.md`, `docs/security/feasibility/`, `docs/development/tech-verification/`, `docs/decisions/pending/`.

A feasibility result or a threat model status never overrides a decision. A decision's status is lowered only by a later decision. A decision is not an implementation: nothing in these records means a control exists or has been tested.

## Validation

`python3 scripts/validate-foundation.py` checks the register, the references between documents, the milestone dependencies, the requirement-to-milestone mapping, the ignore rules and a repository safety scan. Run it before every foundation commit.
