# 0005: M1 scope engine closure decisions

Status: recorded on 2026-11-17 at the project owner's instruction to close milestone M1. This record continues `0004-health-endpoints-mvp-decision.md`. It adds D-87 to D-91. It supersedes no earlier decision. It narrows the wording of D-38 and of choice 1 of record 0003 as described in section 3.

## 1. Authority and scope

The owner reviewed the M1 scope engine (`services/core/internal/domain/scope`) and approved the decisions below. Nothing here was approved by an AI agent for itself. The record adds no milestone and does not change the order of the build sequence in `.kiro/specs/mvp/plan.md`. It adds acceptance requirements to M5, M9, M16, M18, M19 and M21 and records that the M1 deliverable is implemented.

## 2. Decision register entries

| ID | Decision | Supersedes | Source |
| --- | --- | --- | --- |
| D-87 | The in-scope CIDR minimum has an absolute floor of /24 for IPv4 and /48 for IPv6. A policy may make the minimum stricter and may loosen it again, never below the floor. The floor is a constant of the `scope` package. `PrefixMin` has no exported fields and its only constructors refuse a value below the floor, above the address length, zero, partial or negative. Changing the floor needs a new decision record and a code change. | Narrows the words "changed only through explicit policy" in D-38 to "changed only through explicit policy and never below the floor" | Owner instruction of 2026-11-17 |
| D-88 | The IPv4-translated range `::ffff:0:0:0/96` (SIIT) joins the embedded-IPv4 ranges of S1.1 step 7. A literal inside it is ambiguous. An inclusion prefix that overlaps it in either direction is refused. An exclusion prefix equal to or inside it is refused. No other range changes. | Extends the range list of S1.1 step 7 and plan Task 1.3 | Owner instruction of 2026-11-17 |
| D-89 | A scope decision is never readiness. A target is not declared ready on the result of `RuleSet.Evaluate` alone. Before target authorization validation (M18) records an allow and before an assessment becomes Ready, starts or resumes (M21), the special-address policy of M9 and M17 must be evaluated for every address in the target set, for all special addresses and not only SIIT. M18 and M21 reuse that one policy and do not define a list of their own. When the policy is unavailable or fails, readiness fails closed. | None | Owner instruction of 2026-11-17 |
| D-90 | The CIDR minimum is a versioned policy value. Making it stricter is a narrowing change. Loosening it, down to the floor at most, is a widening change that only the program owner can make by an explicit action with password re-entry (D-69). Every change creates a new policy version and an audit event. Every change makes target authorization validations stale and approvals bound to the earlier policy snapshot unusable until they are requested again. Nothing loosens the minimum automatically. | None. Applies choice 3 of record 0003, which lists the CIDR minimum as a widening-policy item | Owner instruction of 2026-11-17 |
| D-91 | `scope.Decision` must be able to name the rule that decided. The obligation is recorded and its implementation is deferred. It must be finished before M16 starts and at the latest before M18 can be accepted. | None | Owner instruction of 2026-11-17 |

D-87 to D-91 are entered in the register in `architecture.md` section 19. Identifiers are never reused or renumbered.

## 3. The decisions

### 3.1 CIDR minimum floor (D-87)

1. The floor is `MinPrefixLenV4 = 24` and `MinPrefixLenV6 = 48`.
2. `NewPrefixMin(v4, v6)` accepts v4 from 24 to 32 and v6 from 48 to 128. Anything else returns `ErrInvalidMinimum` and an unusable value.
3. The zero value of `PrefixMin` is unset. An inclusion parsed with it returns `ErrMinimumUnset`. A missing minimum never means no minimum.
4. The check is repeated inside `IsSet`, so a value built in the package without the constructor is still refused below the floor.
5. The floor applies to every in-scope CIDR, including lab ranges (record 0003, choice 1). M17 must build lab range prefixes with the same type and must not add a second minimum.
6. Exclusions have no minimum (record 0003, choice 1, D-57).
7. Why: D-38 said the minimum is changed through explicit policy. A policy value that can be set to /8 makes one mistaken or tampered edit authorize a whole network. The floor keeps the worst case at the size the owner chose in Round 2 and kept in Round 3.

### 3.2 SIIT (D-88)

1. Range: `::ffff:0:0:0/96`, which is `0:0:0:0:ffff:0:0:0` to `0:0:0:0:ffff:0:ffff:ffff`. It is different from the IPv4-mapped range `::ffff:0:0/96`, which is still unmapped to IPv4 (S1.1 step 7).
2. Source and status: RFC 2765 (SIIT, February 2000) defined the IPv4-translated address form. RFC 6145 (April 2011) states that it obsoletes RFC 2765. RFC 7915 (June 2016) in turn states that it obsoletes RFC 6145. The IANA IPv6 Special-Purpose Address Registry (last updated 2025-10-09, read during the M1 review) does not list `::ffff:0:0:0/96`. The registry lists `::ffff:0:0/96` (IPv4-mapped), `64:ff9b::/96` and `64:ff9b:1::/48`. The RFC bodies were not read beyond the status headers. Refusal is a precaution against a historical embedded-IPv4 form, not a claim that the range is routable.
3. Behavior: `ParseAddr`, `ParseTarget`, `ParseInclusionPrefix`, `ParseExclusionPrefix`, `NewRule` (ip and cidr kinds) and `RuleSet.Evaluate` refuse the range consistently. A hand-built `Target` with a SIIT address is not canonical and is denied as `invalid_target`. An exclusion that merely contains the range, such as `::ffff:0:0:0/80`, is accepted because it only denies more.
4. The resolved-address check of M9 (S1.2) is separate. It must still deny any IPv6 address outside `2000::/3`, which includes this range.
5. No other range was changed or added.

### 3.3 Readiness gate (D-89)

1. `Evaluate` answers one question: do the scope rules allow this name or literal, and port. It resolves nothing. Special-purpose addresses such as `127.0.0.1`, `10.0.0.1`, `100.64.0.1`, `169.254.169.254` or `fd00::1` are valid scope inputs and can be `in_scope`. Denying them belongs to the address policy of M9 (D-39, D-72, S1.2) and the lab rules of M17 (D-47).
2. M18 gate: a target authorization validation records `allow` for an address target only when the scope decision allows and the address policy allows. The per-target record keeps the scope reason and the address-policy reason separate, so a reviewer can see which layer denied.
3. M21 gate: Ready, start and resume require a current validation (FR-SCOPE-005) that satisfied item 2, and re-check that the address policy is available.
4. One policy: M18 and M21 call a single address-policy port. The tables and the decision function are the M9 deliverable (generated from the IANA registries with a pinned hash, plus multicast and the named entries of D-72), with the lab exemption of M17. M18 and M21 do not hold their own list and must not contradict it.
5. Fail closed: if the port has no implementation yet, the table is missing or fails its hash check, or the call returns an error, the result is deny and the assessment is not ready. This is the state until M9 delivers the policy. It does not change the build order. M18 and M21 are tested with a test double.
6. Host name targets are not resolved by M18 or M21. Their resolved addresses are evaluated at connection time by M9. Readiness for a host name target still needs the policy to be available (item 5), and the interface must not say that the resolved addresses were checked.

### 3.4 CIDR minimum policy changes (D-90)

1. Setting keys `cidr_minimum_v4` and `cidr_minimum_v6` already exist in `policy_settings` (data dictionary, `constraints-and-indexes.md` c1). Values are validated with `NewPrefixMin`. The database also checks the floor (constraint c4).
2. Tightening is a narrowing change and needs no re-authentication. Loosening toward the floor is a widening change (record 0003, choice 3) and needs a fresh password entry by the program owner. In the MVP the program owner is the single local researcher account (D-60). An AI, worker, API-token or import actor is refused and audited.
3. Each change inserts a new `policy_settings` version, supersedes the previous one in the same transaction and writes a `policy.changed` audit event with old value, new value, actor, time and reason (FR-POLICY-002).
4. Every change, tightening or loosening, changes the policy snapshot hash. A target authorization validation bound to the earlier snapshot is stale, so the targets must be validated again (FR-SCOPE-006, M18). Existing CIDR rules are read again under the new minimum. A rule that the new minimum refuses makes the rule set invalid and everything is denied until the rule is replaced.
5. An approval bound to the earlier policy snapshot is not valid under the new one. A new approval must be requested. Nothing carries an approval over (M19).
6. No automatic loosening. A schema upgrade, a changed default, an expired value, a deleted row, an import or a stored value that `NewPrefixMin` refuses never replaces a stricter stored value with the default. A refused stored value is an invalid policy and in-scope CIDR rules are refused until a human repairs it. The initial default applies only when no version exists.

### 3.5 Matched rule identifier (D-91)

1. Requirement: the audit event of a scope evaluation (`security.md` 13) and the column `target_validation_targets.matched_scope_rule_id` need the identifier of the deciding rule, whether an exclusion denied or an inclusion allowed. Today `Decision` carries `Allowed` and `Reason` only.
2. The package has no rule identifiers. The design must say how the caller supplies one, for example an index or an identifier that the caller attaches to each rule. That design is not made here.
3. Deadline: implemented and tested before M16 starts, because M16 is the first use case that evaluates scope and records the result (target registration). M5 reviews the field shape while it defines the audit event. At the latest, M18 cannot be accepted without it, because M18 persists `matched_scope_rule_id`. The audit of scope evaluations is not built without it.
4. Tests required when implemented: an exclusion decides, an inclusion decides, nothing matches, and the identifier survives a rule set that holds several matching rules.

## 4. Clarifications of the M1 grammar approved in the M1 review

These were approved by the owner during the M1 review. They sharpen choices 1, 6 and 10 of record 0003 and D-38, D-57 and D-73. They have no identifier of their own.

1. `ParsePrefix` no longer exists. `ParseInclusionPrefix(s, PrefixMin)` and `ParseExclusionPrefix(s)` are separate entry points, so a forgotten minimum cannot become "no minimum".
2. A `/0` prefix covers its own address family only. `::/0` does not match IPv4 and `0.0.0.0/0` does not match IPv6.
3. The whole of `::/96` is refused as an address, including `::1`, with no loopback exception.
4. An inclusion prefix that overlaps an embedded-IPv4 range in either direction is refused. An exclusion prefix is refused only when it is equal to or inside a range.
5. `Rule` has unexported fields. `Rule.Value()` returns the canonical text that `NewRule` reads back to an equal rule, and a wildcard keeps its `*.` prefix.
6. `ParsePorts` accepts the exact text `1-65535` as every port. A list of every port is the same set.
7. Inputs longer than any valid input are refused before allocation: host 254, address 45, prefix 49, ports 382103 bytes.
8. The architecture test limits the scope package to `net/netip` from the `net` family, bans `crypto/tls`, `crypto/rand` and (in production code) `math/rand`, and checks indirect imports.

## 5. Implementation status of M1 (2026-11-17)

Implemented in `services/core/internal/domain/scope`: `ParseHost`, `ParseAddr`, `ParseInclusionPrefix`, `ParseExclusionPrefix`, `ParsePorts`, `PrefixMin`, `NewRule`, `ParseTarget` and `RuleSet.Evaluate`. The tests are table, property and fuzz tests, with an architecture test in `services/core/tests/architecture`.

Not implemented and not claimed:

- No application use case, persistence, audit, API or user interface uses the package yet. A scope control is not verified end to end.
- IDNA conversion, validation and the mixed-script rule are M1b. The package accepts ASCII names only.
- Wildcard inclusion stays refused until a public suffix source exists (O-32, D-73).
- The address policy (M9, M17), the matched rule identifier (D-91) and the policy store (M16) are not built.

## 6. Open points for the owner

- Build order. M17 (acceptance: forbidden address classes are refused) and M18 come before M9 in the recommended order, and the address tables are an M9 deliverable. D-89 handles readiness with a fail-closed port. M17 has the same need. This record does not reorder the plan. If M17 cannot be tested without the tables, the owner decides whether the pure table part moves earlier.
- The port that M18 and M21 call is named `AddressPolicy` here for reference. Its method set belongs to the M9 and M18 design and is not decided.

## 7. Affected documents

| Item | Effect |
| --- | --- |
| `.kiro/steering/architecture.md` | D-87 to D-91 added to the register. D-38 and P-24 carry a pointer. Status line and revision log updated. |
| `.kiro/specs/mvp/plan.md` | M1 recorded as implemented with its final interfaces. M5, M9, M16, M18, M19 and M21 gain acceptance items. The order and dependencies are unchanged. |
| `docs/PRD.md` | Status lines updated. Acceptance criteria added to FR-SCOPE-001, FR-SCOPE-005, FR-ASSESS-001 and FR-POLICY-002. |
| `docs/design/database/` | `policy_settings` values and the floor check, and the meaning of `matched_scope_rule_id` and `target_validation_targets.decision`. |
| `docs/security/threat-model.md` | Evidence text of T-01, T-10 and T-21, and the "what exists today" paragraph. No status changed. |
| `README.md`, `docs/README.md`, `docs/development/getting-started.md` | M1 recorded as implemented. |
| `scripts/validate-foundation.py` | Register end and the record list updated. |
