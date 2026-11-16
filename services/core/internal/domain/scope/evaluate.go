package scope

import (
	"net/netip"
	"strings"
)

// Reasons of a Decision. They are stable codes, safe to store and to show.
const (
	ReasonInScope        = "in_scope"         // an inclusion matched and no exclusion did
	ReasonExcluded       = "excluded"         // an exclusion matched
	ReasonNoMatchingRule = "no_matching_rule" // default deny
	ReasonInvalidTarget  = "invalid_target"   // the target is not canonical
	ReasonInvalidRuleSet = "invalid_rule_set" // a rule is not valid, so nothing is allowed
)

// Target is a host or an address with a port, in canonical form. Exactly one
// of Host and Addr is set. A hand-built Target that is not canonical is
// denied. Build one with ParseTarget.
type Target struct {
	Host Host
	Addr netip.Addr
	Port uint16
}

// Decision is the result of an evaluation.
type Decision struct {
	Allowed bool
	Reason  string
}

func deny(reason string) Decision { return Decision{Allowed: false, Reason: reason} }

// ParseTarget builds the canonical Target for a host name or an IP literal and
// a port. An address is recognised by its form: it contains a colon, or it is
// a canonical four-part IPv4 address. Everything else is read as a host name,
// so "127.1" and "2130706433" are ambiguous and refused. Port 0 is invalid.
func ParseTarget(value string, port uint16) (Target, error) {
	if port == 0 {
		return Target{}, invalidPorts("port 0")
	}
	if value == "" {
		return Target{}, ambiguous("empty target")
	}
	if strings.Contains(value, ":") {
		a, err := ParseAddr(value)
		if err != nil {
			return Target{}, err
		}
		return Target{Addr: a, Port: port}, nil
	}
	// A value without a colon that netip reads is a canonical four-part IPv4
	// address: no zone, no mapped form and no embedded range are possible.
	if a, err := netip.ParseAddr(value); err == nil {
		return Target{Addr: a, Port: port}, nil
	}
	h, err := ParseHost(value)
	if err != nil {
		return Target{}, err
	}
	return Target{Host: h, Port: port}, nil
}

// canonical reports whether the target is exactly what ParseTarget would
// build: one of Host and Addr, in canonical form, with a port.
func (t Target) canonical() bool {
	if t.Port == 0 {
		return false
	}
	hasHost, hasAddr := t.Host != "", t.Addr.IsValid()
	switch {
	case hasHost == hasAddr:
		return false
	case hasHost:
		h, err := ParseHost(string(t.Host))
		return err == nil && h == t.Host
	default:
		// A mapped form and a zone do not survive the round trip, and the
		// embedded-IPv4 ranges are refused by ParseAddr.
		a, err := ParseAddr(t.Addr.String())
		return err == nil && a == t.Addr
	}
}

// RuleSet is the rules of one program or assessment.
type RuleSet []Rule

// Evaluate decides whether the set allows the target. The order is fixed:
//
//  1. A target that is not canonical is denied (invalid_target).
//  2. A set with an invalid rule is denied (invalid_rule_set). An unreadable
//     rule could be a lost exclusion, so nothing is allowed.
//  3. If any exclusion matches, the target is denied (excluded).
//  4. If any inclusion matches, the target is allowed (in_scope).
//  5. Otherwise the target is denied (no_matching_rule).
//
// It reads only its arguments. It resolves nothing, so a host target is
// matched against domain and wildcard rules only and an address target against
// ip and cidr rules only.
func (rs RuleSet) Evaluate(t Target) Decision {
	if !t.canonical() {
		return deny(ReasonInvalidTarget)
	}
	for _, r := range rs {
		if !r.valid() {
			return deny(ReasonInvalidRuleSet)
		}
	}
	included := false
	for _, r := range rs {
		if !r.matches(t) {
			continue
		}
		if r.exclude {
			return deny(ReasonExcluded)
		}
		included = true
	}
	if included {
		return Decision{Allowed: true, Reason: ReasonInScope}
	}
	return deny(ReasonNoMatchingRule)
}

// matches reports whether the rule covers the target, ports included.
func (r Rule) matches(t Target) bool {
	if !r.ports.Contains(t.Port) {
		return false
	}
	switch r.kind {
	case KindDomain:
		return t.Host != "" && t.Host == r.host
	case KindWildcard:
		// One or more labels below the base, never the base itself. The dot
		// is part of the suffix, so "evil-example.com" does not match
		// "*.example.com".
		h := string(t.Host)
		return h != "" && strings.HasSuffix(h, "."+string(r.host))
	case KindIP:
		return t.Addr.IsValid() && t.Addr == r.addr
	case KindCIDR:
		return t.Addr.IsValid() && r.prefix.Contains(t.Addr)
	}
	return false
}
