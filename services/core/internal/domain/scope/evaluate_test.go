package scope_test

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// rules builds a RuleSet from "kind:+value|ports" style specs, where + is an
// inclusion and - an exclusion. It fails the test on any refused rule.
type spec struct {
	kind    scope.RuleKind
	exclude bool
	value   string
	ports   string
}

func inc(kind scope.RuleKind, value, ports string) spec { return spec{kind, false, value, ports} }
func exc(kind scope.RuleKind, value, ports string) spec { return spec{kind, true, value, ports} }

func build(t *testing.T, specs ...spec) scope.RuleSet {
	t.Helper()
	var rs scope.RuleSet
	for _, s := range specs {
		r, err := scope.NewRule(s.kind, s.exclude, s.value, s.ports, defMin)
		if err != nil {
			t.Fatalf("NewRule(%v, exclude=%v, %q, %q): %v", s.kind, s.exclude, s.value, s.ports, err)
		}
		rs = append(rs, r)
	}
	return rs
}

func hostTarget(h string, port uint16) scope.Target {
	return scope.Target{Host: scope.Host(h), Port: port}
}

func addrTarget(a string, port uint16) scope.Target {
	return scope.Target{Addr: netip.MustParseAddr(a), Port: port}
}

type evalCase struct {
	name   string
	rules  []spec
	target scope.Target
	allow  bool
	reason string
}

func runEval(t *testing.T, cases []evalCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := build(t, tc.rules...).Evaluate(tc.target)
			if d.Allowed != tc.allow || d.Reason != tc.reason {
				t.Errorf("Evaluate = %+v, want allowed=%v reason=%q", d, tc.allow, tc.reason)
			}
		})
	}
}

const (
	D = scope.KindDomain
	W = scope.KindWildcard
	I = scope.KindIP
	C = scope.KindCIDR
)

func TestEvaluateDefaultDeny(t *testing.T) {
	runEval(t, []evalCase{
		{"no rules", nil, hostTarget("example.com", 443), false, scope.ReasonNoMatchingRule},
		{"no rules, address", nil, addrTarget("203.0.113.7", 443), false, scope.ReasonNoMatchingRule},
		{"only an unrelated rule", []spec{inc(D, "example.org", "")}, hostTarget("example.com", 443), false, scope.ReasonNoMatchingRule},
		{"only exclusions", []spec{exc(D, "example.org", "")}, hostTarget("example.com", 443), false, scope.ReasonNoMatchingRule},
	})
	if d := (scope.RuleSet)(nil).Evaluate(hostTarget("example.com", 443)); d.Allowed {
		t.Errorf("a nil RuleSet allowed a target: %+v", d)
	}
}

func TestEvaluateDefaultPorts(t *testing.T) {
	runEval(t, []evalCase{
		{"in-scope domain allows 443", []spec{inc(D, "example.com", "")}, hostTarget("example.com", 443), true, scope.ReasonInScope},
		{"in-scope domain allows 80", []spec{inc(D, "example.com", "")}, hostTarget("example.com", 80), true, scope.ReasonInScope},
		{"in-scope domain denies 8443", []spec{inc(D, "example.com", "")}, hostTarget("example.com", 8443), false, scope.ReasonNoMatchingRule},
		{"in-scope domain denies 22", []spec{inc(D, "example.com", "")}, hostTarget("example.com", 22), false, scope.ReasonNoMatchingRule},
		{"in-scope domain denies 8080", []spec{inc(D, "example.com", "")}, hostTarget("example.com", 8080), false, scope.ReasonNoMatchingRule},
		{"listed port is allowed", []spec{inc(D, "example.com", "8443")}, hostTarget("example.com", 8443), true, scope.ReasonInScope},
		{"listed port replaces the default", []spec{inc(D, "example.com", "8443")}, hostTarget("example.com", 443), false, scope.ReasonNoMatchingRule},
		{"two rules for one host add their ports", []spec{inc(D, "example.com", "8443"), inc(D, "example.com", "")}, hostTarget("example.com", 443), true, scope.ReasonInScope},
	})
}

func TestEvaluateExclusionPrecedence(t *testing.T) {
	runEval(t, []evalCase{
		// An exclusion with no ports covers every port, even one that an
		// inclusion lists.
		{"exclusion without ports beats a listed port", []spec{exc(D, "example.com", ""), inc(D, "example.com", "8443")}, hostTarget("example.com", 8443), false, scope.ReasonExcluded},
		{"exclusion without ports beats the default ports", []spec{exc(D, "example.com", ""), inc(D, "example.com", "")}, hostTarget("example.com", 443), false, scope.ReasonExcluded},
		{"order of rules does not matter", []spec{inc(D, "example.com", ""), exc(D, "example.com", "")}, hostTarget("example.com", 443), false, scope.ReasonExcluded},
		// An exclusion with ports covers only those ports.
		{"narrow exclusion hits its port", []spec{inc(D, "example.com", "22,443"), exc(D, "example.com", "22")}, hostTarget("example.com", 22), false, scope.ReasonExcluded},
		{"narrow exclusion leaves other ports", []spec{inc(D, "example.com", "22,443"), exc(D, "example.com", "22")}, hostTarget("example.com", 443), true, scope.ReasonInScope},
		// The S1.1 case: an exclusion with no ports plus an in-scope rule with 8443.
		{"exclusion plus in-scope 8443", []spec{exc(D, "example.com", ""), inc(D, "example.com", "8443")}, hostTarget("example.com", 8443), false, scope.ReasonExcluded},
		// Exclusion on the same host and another inclusion kind.
		{"exclusion of an IP beats an inclusion of the same IP", []spec{inc(I, "203.0.113.7", ""), exc(I, "203.0.113.7", "")}, addrTarget("203.0.113.7", 443), false, scope.ReasonExcluded},
		{"exclusion CIDR beats an inclusion IP inside it", []spec{inc(I, "203.0.113.7", ""), exc(C, "203.0.113.0/25", "")}, addrTarget("203.0.113.7", 443), false, scope.ReasonExcluded},
		{"exclusion CIDR /16 beats an inclusion /24 inside it", []spec{inc(C, "198.51.100.0/24", ""), exc(C, "198.51.0.0/16", "")}, addrTarget("198.51.100.9", 443), false, scope.ReasonExcluded},
		{"exclusion /0 denies everything", []spec{inc(C, "203.0.113.0/24", ""), exc(C, "0.0.0.0/0", "")}, addrTarget("203.0.113.9", 443), false, scope.ReasonExcluded},
		{"an IPv4 /0 exclusion does not touch an IPv6 target", []spec{inc(C, "2001:db8:1::/48", ""), exc(C, "0.0.0.0/0", "")}, addrTarget("2001:db8:1::7", 443), true, scope.ReasonInScope},
		{"IPv6 exclusion ::/0 denies IPv6", []spec{inc(C, "2001:db8:1::/48", ""), exc(C, "::/0", "")}, addrTarget("2001:db8:1::7", 443), false, scope.ReasonExcluded},
		{"CIDR exclusion leaves addresses outside it", []spec{inc(C, "203.0.113.0/24", ""), exc(C, "203.0.113.0/25", "")}, addrTarget("203.0.113.200", 443), true, scope.ReasonInScope},
	})
}

func TestEvaluateWildcardExclusions(t *testing.T) {
	excl := exc(W, "*.x.example.com", "")
	runEval(t, []evalCase{
		{"wildcard exclusion does not cover its apex", []spec{inc(D, "x.example.com", ""), excl}, hostTarget("x.example.com", 443), true, scope.ReasonInScope},
		{"wildcard exclusion covers one label below", []spec{inc(D, "a.x.example.com", ""), excl}, hostTarget("a.x.example.com", 443), false, scope.ReasonExcluded},
		{"wildcard exclusion covers many labels below", []spec{inc(D, "a.b.x.example.com", ""), excl}, hostTarget("a.b.x.example.com", 443), false, scope.ReasonExcluded},
		{"wildcard exclusion does not cover a sibling", []spec{inc(D, "y.example.com", ""), excl}, hostTarget("y.example.com", 443), true, scope.ReasonInScope},
		{"wildcard exclusion does not cover a lookalike suffix", []spec{inc(D, "evil-x.example.com", ""), excl}, hostTarget("evil-x.example.com", 443), true, scope.ReasonInScope},
		{"wildcard exclusion does not cover a foreign suffix", []spec{inc(D, "a.x.example.com.evil.net", ""), excl}, hostTarget("a.x.example.com.evil.net", 443), true, scope.ReasonInScope},
		{"wildcard exclusion does not cover the parent", []spec{inc(D, "example.com", ""), excl}, hostTarget("example.com", 443), true, scope.ReasonInScope},
		{"exclusion of the apex domain does not cover subdomains", []spec{exc(D, "example.com", ""), inc(D, "a.example.com", "")}, hostTarget("a.example.com", 443), true, scope.ReasonInScope},
		{"wildcard exclusion with ports covers only them", []spec{inc(D, "a.x.example.com", "22,443"), exc(W, "*.x.example.com", "22")}, hostTarget("a.x.example.com", 443), true, scope.ReasonInScope},
		{"wildcard exclusion over a public suffix denies the zone", []spec{inc(D, "shop.co.uk", ""), exc(W, "*.co.uk", "")}, hostTarget("shop.co.uk", 443), false, scope.ReasonExcluded},
		{"exclusion /label boundary: base is not a substring match", []spec{inc(D, "notx.example.com", ""), exc(W, "*.x.example.com", "")}, hostTarget("notx.example.com", 443), true, scope.ReasonInScope},
	})
}

func TestEvaluateAddressesAndRanges(t *testing.T) {
	runEval(t, []evalCase{
		{"ip rule allows exactly that address", []spec{inc(I, "203.0.113.7", "")}, addrTarget("203.0.113.7", 443), true, scope.ReasonInScope},
		{"ip rule denies the next address", []spec{inc(I, "203.0.113.7", "")}, addrTarget("203.0.113.8", 443), false, scope.ReasonNoMatchingRule},
		{"ip rule denies the previous address", []spec{inc(I, "203.0.113.7", "")}, addrTarget("203.0.113.6", 443), false, scope.ReasonNoMatchingRule},
		{"IPv6 ip rule", []spec{inc(I, "2001:db8::7", "")}, addrTarget("2001:db8::7", 80), true, scope.ReasonInScope},
		{"IPv6 ip rule denies a neighbour", []spec{inc(I, "2001:db8::7", "")}, addrTarget("2001:db8::8", 80), false, scope.ReasonNoMatchingRule},
		{"cidr first address", []spec{inc(C, "203.0.113.0/24", "")}, addrTarget("203.0.113.0", 443), true, scope.ReasonInScope},
		{"cidr last address", []spec{inc(C, "203.0.113.0/24", "")}, addrTarget("203.0.113.255", 443), true, scope.ReasonInScope},
		{"cidr address below", []spec{inc(C, "203.0.113.0/24", "")}, addrTarget("203.0.112.255", 443), false, scope.ReasonNoMatchingRule},
		{"cidr address above", []spec{inc(C, "203.0.113.0/24", "")}, addrTarget("203.0.114.0", 443), false, scope.ReasonNoMatchingRule},
		{"cidr /25 upper half", []spec{inc(C, "203.0.113.0/25", "")}, addrTarget("203.0.113.128", 443), false, scope.ReasonNoMatchingRule},
		{"cidr port default", []spec{inc(C, "203.0.113.0/24", "")}, addrTarget("203.0.113.9", 8443), false, scope.ReasonNoMatchingRule},
		{"cidr listed port", []spec{inc(C, "203.0.113.0/24", "8443")}, addrTarget("203.0.113.9", 8443), true, scope.ReasonInScope},
		{"IPv6 cidr /48 inside", []spec{inc(C, "2001:db8:1::/48", "")}, addrTarget("2001:db8:1:ffff:ffff:ffff:ffff:ffff", 443), true, scope.ReasonInScope},
		{"IPv6 cidr /48 outside", []spec{inc(C, "2001:db8:1::/48", "")}, addrTarget("2001:db8:2::1", 443), false, scope.ReasonNoMatchingRule},
		{"IPv4 cidr never matches an IPv6 address", []spec{inc(C, "203.0.113.0/24", "")}, addrTarget("2001:db8::1", 443), false, scope.ReasonNoMatchingRule},
		{"IPv6 cidr never matches an IPv4 address", []spec{inc(C, "2001:db8:1::/48", "")}, addrTarget("203.0.113.9", 443), false, scope.ReasonNoMatchingRule},
	})
}

// Matching never resolves a name (S1.2 does that later). A host target meets
// only domain rules and an address target meets only ip and cidr rules.
func TestEvaluateDoesNotCrossBetweenNamesAndAddresses(t *testing.T) {
	runEval(t, []evalCase{
		{"host target is not matched against a cidr rule", []spec{inc(C, "203.0.113.0/24", "")}, hostTarget("example.com", 443), false, scope.ReasonNoMatchingRule},
		{"host target is not matched against an ip rule", []spec{inc(I, "203.0.113.7", "")}, hostTarget("example.com", 443), false, scope.ReasonNoMatchingRule},
		{"address target is not matched against a domain rule", []spec{inc(D, "example.com", "")}, addrTarget("203.0.113.7", 443), false, scope.ReasonNoMatchingRule},
		// Residual risk of D-38, documented and tested so nothing claims otherwise:
		// a hostname exclusion does not block the same server reached by address.
		{"hostname exclusion does not block the address", []spec{exc(D, "example.com", ""), inc(I, "203.0.113.7", "")}, addrTarget("203.0.113.7", 443), true, scope.ReasonInScope},
		{"address exclusion does not block the hostname", []spec{exc(I, "203.0.113.7", ""), inc(D, "example.com", "")}, hostTarget("example.com", 443), true, scope.ReasonInScope},
	})
}

func TestEvaluateRefusesInvalidTargets(t *testing.T) {
	allowAll := []spec{inc(D, "example.com", ""), inc(C, "203.0.113.0/24", ""), inc(I, "2001:db8::7", "")}
	runEval(t, []evalCase{
		{"zero target", allowAll, scope.Target{}, false, scope.ReasonInvalidTarget},
		{"port zero, host", allowAll, hostTarget("example.com", 0), false, scope.ReasonInvalidTarget},
		{"port zero, address", allowAll, addrTarget("203.0.113.9", 0), false, scope.ReasonInvalidTarget},
		{"port only", allowAll, scope.Target{Port: 443}, false, scope.ReasonInvalidTarget},
		{"host and address together", allowAll, scope.Target{Host: "example.com", Addr: netip.MustParseAddr("203.0.113.9"), Port: 443}, false, scope.ReasonInvalidTarget},
		{"upper-case host", allowAll, hostTarget("Example.com", 443), false, scope.ReasonInvalidTarget},
		{"host with a trailing dot", allowAll, hostTarget("example.com.", 443), false, scope.ReasonInvalidTarget},
		{"host with a port inside", allowAll, hostTarget("example.com:443", 443), false, scope.ReasonInvalidTarget},
		{"host with an underscore", allowAll, hostTarget("a_b.example.com", 443), false, scope.ReasonInvalidTarget},
		{"non-ASCII host", allowAll, hostTarget("exämple.com", 443), false, scope.ReasonInvalidTarget},
		{"numeric host", allowAll, hostTarget("127.1", 443), false, scope.ReasonInvalidTarget},
		{"IP literal as a host", allowAll, hostTarget("203.0.113.9", 443), false, scope.ReasonInvalidTarget},
		{"wildcard as a host", allowAll, hostTarget("*.example.com", 443), false, scope.ReasonInvalidTarget},
		{"IPv4-mapped address is not canonical", allowAll, addrTarget("::ffff:203.0.113.9", 443), false, scope.ReasonInvalidTarget},
		{"address with a zone", allowAll, addrTarget("fe80::1%eth0", 443), false, scope.ReasonInvalidTarget},
		{"NAT64 address", allowAll, addrTarget("64:ff9b::cb00:7107", 443), false, scope.ReasonInvalidTarget},
		{"6to4 address", allowAll, addrTarget("2002::1", 443), false, scope.ReasonInvalidTarget},
		{"an invalid target is denied before the rules are read", []spec{exc(C, "0.0.0.0/0", "")}, scope.Target{}, false, scope.ReasonInvalidTarget},
	})
}

func TestEvaluateRefusesAnInvalidRuleSet(t *testing.T) {
	good := build(t, inc(D, "example.com", ""))
	var zero scope.Rule // an unbuilt rule has no kind and no ports
	for name, rs := range map[string]scope.RuleSet{
		"only a zero rule":                  {zero},
		"zero rule after a good one":        append(append(scope.RuleSet{}, good...), zero),
		"zero rule before a good one":       append(scope.RuleSet{zero}, good...),
		"zero rule where an exclusion goes": append(scope.RuleSet{zero}, build(t, exc(D, "example.org", ""))...),
	} {
		t.Run(name, func(t *testing.T) {
			d := rs.Evaluate(hostTarget("example.com", 443))
			if d.Allowed || d.Reason != scope.ReasonInvalidRuleSet {
				t.Errorf("Evaluate = %+v, want a denial with %q: an unreadable rule could be a lost exclusion", d, scope.ReasonInvalidRuleSet)
			}
		})
	}
}

func TestEvaluateIsDeterministicAndDoesNotChangeTheRuleSet(t *testing.T) {
	rs := build(t, inc(D, "example.com", "443,8443"), exc(D, "example.com", "8443"), inc(C, "203.0.113.0/24", ""))
	before := make(scope.RuleSet, len(rs))
	copy(before, rs)
	snapshot := func(r scope.Rule) string {
		return r.Kind().String() + "|" + r.Value() + "|" + r.Ports().String() + "|" + map[bool]string{true: "x", false: "i"}[r.Exclude()]
	}
	targets := []scope.Target{hostTarget("example.com", 443), hostTarget("example.com", 8443), addrTarget("203.0.113.9", 80), {}}
	first := make([]scope.Decision, len(targets))
	for i, tg := range targets {
		first[i] = rs.Evaluate(tg)
	}
	for round := 0; round < 50; round++ {
		for i, tg := range targets {
			if got := rs.Evaluate(tg); got != first[i] {
				t.Fatalf("round %d target %d: %+v != %+v", round, i, got, first[i])
			}
		}
	}
	for i := range rs {
		if snapshot(rs[i]) != snapshot(before[i]) {
			t.Errorf("rule %d changed during evaluation", i)
		}
	}
}

func TestEvaluateAtPortBoundaries(t *testing.T) {
	all := build(t, inc(D, "example.com", "1,65535"))
	for port, want := range map[uint16]bool{1: true, 65535: true, 2: false, 65534: false, 0: false} {
		if d := all.Evaluate(hostTarget("example.com", port)); d.Allowed != want {
			t.Errorf("port %d: %+v, want allowed=%v", port, d, want)
		}
	}
	excl := build(t, inc(D, "example.com", "1,65535,443"), exc(D, "example.com", ""))
	for _, port := range []uint16{1, 443, 65535} {
		if d := excl.Evaluate(hostTarget("example.com", port)); d.Allowed || d.Reason != scope.ReasonExcluded {
			t.Errorf("port %d under an all-ports exclusion: %+v", port, d)
		}
	}
}

func TestParseTarget(t *testing.T) {
	ok := []struct {
		in   string
		port uint16
		host string
		addr string
	}{
		{"Example.COM.", 443, "example.com", ""},
		{"example.com", 80, "example.com", ""},
		{"203.0.113.7", 443, "", "203.0.113.7"},
		{"2001:DB8::7", 443, "", "2001:db8::7"},
		{"::ffff:203.0.113.7", 443, "", "203.0.113.7"},
		{"::ffff:7f00:1", 8080, "", "127.0.0.1"},
		{"2001:db8::1", 8080, "", "2001:db8::1"},
	}
	for _, tc := range ok {
		got, err := scope.ParseTarget(tc.in, tc.port)
		if err != nil {
			t.Errorf("ParseTarget(%q, %d) error: %v", tc.in, tc.port, err)
			continue
		}
		if string(got.Host) != tc.host || got.Port != tc.port || (tc.addr == "") == got.Addr.IsValid() || (tc.addr != "" && got.Addr.String() != tc.addr) {
			t.Errorf("ParseTarget(%q, %d) = %+v, want host %q addr %q", tc.in, tc.port, got, tc.host, tc.addr)
		}
		// What ParseTarget builds is always canonical for Evaluate.
		if d := build(t, inc(D, "example.com", ""), inc(I, "203.0.113.7", "443"), inc(I, "2001:db8::1", "8080")).Evaluate(got); d.Reason == scope.ReasonInvalidTarget {
			t.Errorf("ParseTarget(%q, %d) produced a target that Evaluate calls invalid", tc.in, tc.port)
		}
	}
	refused := []struct {
		name, in string
		port     uint16
		want     error
	}{
		{"short IPv4", "127.1", 443, scope.ErrAmbiguous},
		{"decimal IPv4", "2130706433", 443, scope.ErrAmbiguous},
		{"hex IPv4", "0x7f.0.0.1", 443, scope.ErrAmbiguous},
		{"octal IPv4", "0177.0.0.1", 443, scope.ErrAmbiguous},
		{"leading zeros", "127.000.000.001", 443, scope.ErrAmbiguous},
		{"fullwidth digits", "１２７.0.0.1", 443, scope.ErrAmbiguous},
		{"non-ASCII", "exämple.com", 443, scope.ErrAmbiguous},
		{"zone", "::1%lo", 443, scope.ErrAmbiguous},
		{"brackets", "[::1]", 443, scope.ErrAmbiguous},
		{"userinfo", "user@example.com", 443, scope.ErrAmbiguous},
		{"host and port", "example.com:443", 443, scope.ErrAmbiguous},
		{"underscore", "a_b.example.com", 443, scope.ErrAmbiguous},
		{"URL", "https://example.com", 443, scope.ErrAmbiguous},
		{"empty", "", 443, scope.ErrAmbiguous},
		{"NAT64", "64:ff9b::cb00:7107", 443, scope.ErrAmbiguous},
		{"wildcard", "*.example.com", 443, scope.ErrAmbiguous},
		{"port zero", "example.com", 0, scope.ErrInvalidPorts},
		{"port zero, address", "203.0.113.7", 0, scope.ErrInvalidPorts},
		{"IPv6 loopback", "::1", 443, scope.ErrAmbiguous},
		{"IPv6 loopback, long form", "0:0:0:0:0:0:0:1", 443, scope.ErrAmbiguous},
		{"IPv6 loopback, dotted tail", "::0.0.0.1", 443, scope.ErrAmbiguous},
		{"unspecified IPv6", "::", 443, scope.ErrAmbiguous},
		{"IPv4 with a trailing dot", "1.2.3.4.", 443, scope.ErrAmbiguous},
		{"IPv4 with a leading space", " 1.2.3.4", 443, scope.ErrAmbiguous},
		{"6to4", "2002::1", 443, scope.ErrAmbiguous},
		{"Teredo", "2001::1", 443, scope.ErrAmbiguous},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scope.ParseTarget(tc.in, tc.port)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ParseTarget(%q, %d) = %+v, %v, want %v", tc.in, tc.port, got, err, tc.want)
			}
			if got != (scope.Target{}) {
				t.Errorf("ParseTarget returned %+v together with an error", got)
			}
		})
	}
}
