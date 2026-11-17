// Package scope is the deterministic scope canonicalization and evaluation
// engine (milestone M1).
//
// It parses scope rules and targets into one canonical form and decides, for a
// target (a host name or an address, and a port), whether a set of rules
// allows it. RuleSet.Evaluate follows this order (S1.1, D-38). The first step
// that applies decides:
//
//  1. A target that is not canonical is denied (invalid_target).
//  2. A rule set that holds an invalid rule is denied (invalid_rule_set). An
//     unreadable rule could be a lost exclusion, so nothing is allowed.
//  3. If any exclusion matches, the target is denied (excluded). Exclusions
//     always win, whatever the order of the rules.
//  4. If any inclusion matches, the target is allowed (in_scope).
//  5. Otherwise the target is denied (no_matching_rule). The default is deny.
//
// Rule kinds are domain, wildcard, ip and cidr and nothing else (D-34). A
// wildcard inclusion is refused for the whole MVP and a wildcard exclusion is
// accepted (D-35, D-73, D-57). An inclusion without ports covers 80 and 443. An
// exclusion without ports covers every port (D-36). A rule built with a port
// list reads "1-65535" as every port, and a list of every port is the same set.
//
// CIDR rules have two explicit entry points. ParseInclusionPrefix needs a set
// PrefixMin and an unset one is an error (ErrMinimumUnset), never "no minimum".
// PrefixMin has no exported fields. DefaultPrefixMin is /24 IPv4 and /48 IPv6
// (D-38), which is also the floor (MinPrefixLenV4 and MinPrefixLenV6). NewPrefixMin
// accepts only equal or longer lengths, so a caller can tighten the minimum and
// can never lower it. ParseExclusionPrefix has no minimum (choice 1 of record
// 0002). A /0 prefix
// covers its own address family only: ::/0 does not match IPv4 and 0.0.0.0/0
// does not match IPv6.
//
// The whole of ::/96 is refused as an address, the IPv6 loopback ::1 included.
// So are the IPv4-translated range (SIIT, ::ffff:0:0:0/96, D-88), NAT64
// (64:ff9b::/96 and 64:ff9b:1::/48), 6to4 (2002::/16) and Teredo (2001::/32). A prefix that is equal to or inside one of these ranges is
// refused. An inclusion that overlaps one in either direction is refused too,
// because it would look as if it covered a range that is never a target. An
// exclusion that merely contains one is accepted, because it only denies more.
//
// The package is fail closed. Anything it cannot read exactly is refused with
// an error. It canonicalizes only what S1.1 says: case, one trailing dot, IPv6
// compression and case, and IPv4-mapped IPv6. Nothing is repaired, widened or
// guessed. An exclusion that cannot be read is returned as an
// *UnsupportedExclusionError that carries the original text, so the caller can
// record it and block readiness (choice 10).
//
// A Rule can be built only by NewRule. Rule.Value and Rule.Ports().String give
// the canonical text that NewRule reads back to an equal rule, which is what
// the data dictionary stores as canonical_value and ports_canonical.
//
// From milestone M2 the package also holds the policy decision function. Decide
// (decide.go) is pure. It takes the operation, the rules, the validation record,
// the policy state, the approval and the time, and returns allow, deny or
// require-approval with one stable reason. An allow carries an
// AuthorizedOperation (authorized.go): the decision ID, the hash of the
// operation and an expiry, with unexported fields. A tool executor calls Verify
// on it with the operation it is about to run. Only internal/application/scope
// may call Decide, which the architecture test checks (P-04). Decide takes no
// actor, because the order table does not let this package import actor. A
// scope decision is still not readiness (D-89).
//
// Boundaries (structure.md 5.6, D-73):
//
//   - The package imports the standard library, and from the net family
//     only net/netip, and from M2 on the operation and approval domain
//     packages (the order table of structure.md 5.6). The architecture test
//     refuses anything else, including indirect imports of the network, TLS or
//     randomness. There is no I/O, no environment and no network, and a name is
//     never resolved. The clock is never read: the package uses the types of
//     package time and every decision takes the current time as an argument.
//     A host target meets only domain and wildcard rules. An address target meets
//     only ip and cidr rules. A hostname exclusion therefore does not block the
//     same server reached by address (D-38, S1.1). No document may claim it does.
//   - Names are ASCII only. Any non-ASCII byte is ambiguous and denied, and so
//     is an underscore. This package does not convert international names, does
//     not apply the mixed-script rule and does not validate A-labels. A label
//     such as "xn--a" or "ab--cd" is accepted as plain ASCII here. IDNA
//     conversion and validation, and the mixed-script rule, are milestone M1b
//     in the application layer. Names from untrusted text must pass through M1b
//     before they reach this package.
//   - Special-purpose addresses (0.0.0.0, loopback, link-local, metadata and so
//     on) are valid inputs here. Denying them is the job of the egress tables
//     (M9, D-39, D-72) and the lab rules (M17). Evaluate is one layer of the
//     policy and never the only one.
//   - Inputs longer than any valid input are refused before anything is
//     allocated for them. The limits are the longest valid texts, not new
//     policy: a host of 253 octets and one dot, an address of 45 bytes, a prefix
//     of 49 bytes and a port list of 382103 bytes (the list of every port).
//   - Every error text is static. It never echoes the input.
//
// A decision of Evaluate is a scope result only. It is never readiness to
// test. Special-purpose addresses are valid here, so a literal such as
// 127.0.0.1 or fd00::1 can be in_scope. The readiness gates of M18 and M21 must
// apply the address policy of M9 and M17 as well, and fail closed when it is not
// available (D-89).
//
// Open and deferred items:
//
//   - F-4: Decision does not say which rule decided. Requirement: the audit
//     event of a scope evaluation (security.md 13) and the column
//     target_validation_targets.matched_scope_rule_id (data dictionary) must be
//     able to name the deciding rule, for an exclusion that denied and for an
//     inclusion that allowed. This package has no rule identifiers, so the
//     design must say how the caller supplies one. Deadline (D-91): implement
//     and test it before M16 starts, and at the latest before M18, which
//     persists matched_scope_rule_id. M5 reviews the field shape in the audit
//     event model. The audit of scope evaluations must not be built without it.
//   - The floor of PrefixMin (/24 IPv4, /48 IPv6) is a constant of this package
//     (D-87). A stored policy may be tightened and may be loosened again, never
//     below the floor, by an explicit change of the program owner (D-90, M16).
//     This package does not know policy versions.
package scope
