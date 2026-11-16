package scope_test

import (
	"net/netip"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

func mustTarget(t *testing.T, s string, port uint16) scope.Target {
	t.Helper()
	tg, err := scope.ParseTarget(s, port)
	if err != nil {
		t.Fatalf("ParseTarget(%q, %d): %v", s, port, err)
	}
	return tg
}

// Decision 3 of the owner: a /0 prefix covers only its own address family.
// "::/0" never matches an IPv4 target and "0.0.0.0/0" never matches an IPv6
// target. A researcher who wants to deny everything writes both.
func TestZeroLengthPrefixCoversOnlyItsOwnFamily(t *testing.T) {
	v4Allow := inc(C, "203.0.113.0/24", "")
	v6Allow := inc(C, "2001:db8:1::/48", "")
	v4, v6 := "203.0.113.9", "2001:db8:1::7"

	cases := []struct {
		name   string
		rules  []spec
		target scope.Target
		allow  bool
		reason string
	}{
		{"::/0 exclusion does not match an IPv4 target", []spec{v4Allow, exc(C, "::/0", "")}, mustTarget(t, v4, 443), true, scope.ReasonInScope},
		{"0.0.0.0/0 exclusion does not match an IPv6 target", []spec{v6Allow, exc(C, "0.0.0.0/0", "")}, mustTarget(t, v6, 443), true, scope.ReasonInScope},
		{"::/0 exclusion matches an IPv6 target", []spec{v6Allow, exc(C, "::/0", "")}, mustTarget(t, v6, 443), false, scope.ReasonExcluded},
		{"0.0.0.0/0 exclusion matches an IPv4 target", []spec{v4Allow, exc(C, "0.0.0.0/0", "")}, mustTarget(t, v4, 443), false, scope.ReasonExcluded},
		{"both exclusions deny IPv4", []spec{v4Allow, v6Allow, exc(C, "0.0.0.0/0", ""), exc(C, "::/0", "")}, mustTarget(t, v4, 443), false, scope.ReasonExcluded},
		{"both exclusions deny IPv6", []spec{v4Allow, v6Allow, exc(C, "0.0.0.0/0", ""), exc(C, "::/0", "")}, mustTarget(t, v6, 443), false, scope.ReasonExcluded},
		// A mapped literal is an IPv4 target, so the IPv4 rule covers it and the IPv6 rule does not.
		{"mapped literal is an IPv4 target, covered by 0.0.0.0/0", []spec{v4Allow, exc(C, "0.0.0.0/0", "")}, mustTarget(t, "::ffff:203.0.113.9", 443), false, scope.ReasonExcluded},
		{"mapped literal is an IPv4 target, not covered by ::/0", []spec{v4Allow, exc(C, "::/0", "")}, mustTarget(t, "::ffff:203.0.113.9", 443), true, scope.ReasonInScope},
		// The mapped range written as an IPv6 exclusion is the whole IPv4 space.
		{"::ffff:0:0/96 exclusion covers every IPv4 target", []spec{v4Allow, exc(C, "::ffff:0:0/96", "")}, mustTarget(t, v4, 443), false, scope.ReasonExcluded},
		{"::ffff:0:0/96 exclusion does not touch IPv6", []spec{v6Allow, exc(C, "::ffff:0:0/96", "")}, mustTarget(t, v6, 443), true, scope.ReasonInScope},
		// A /0 exclusion of one family never matches a host name.
		{"a /0 exclusion does not match a host target", []spec{inc(D, "example.com", ""), exc(C, "0.0.0.0/0", ""), exc(C, "::/0", "")}, hostTarget("example.com", 443), true, scope.ReasonInScope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := build(t, tc.rules...).Evaluate(tc.target)
			if d.Allowed != tc.allow || d.Reason != tc.reason {
				t.Errorf("Evaluate = %+v, want allowed=%v reason=%q", d, tc.allow, tc.reason)
			}
		})
	}
}

// Decision 1 of the owner: ::/96 is refused as a whole and the IPv6 loopback
// is not an exception. ::1 is never a canonical target, so no rule set can
// allow it, and no inclusion that contains it can be built.
func TestLoopbackIPv6CanNeverBeAllowed(t *testing.T) {
	loop := scope.Target{Addr: netip.MustParseAddr("::1"), Port: 443}
	cases := []struct {
		name  string
		rules []spec
	}{
		{"no rules", nil},
		{"exclusion ::/0", []spec{exc(C, "::/0", "")}},
		{"inclusion of an unrelated range", []spec{inc(C, "2001:db8:1::/48", "")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := build(t, tc.rules...).Evaluate(loop)
			if d.Allowed || d.Reason != scope.ReasonInvalidTarget {
				t.Errorf("Evaluate(::1) = %+v, want a denial as invalid_target", d)
			}
		})
	}
	// No inclusion that would cover ::1 can be created.
	for _, v := range []string{"::/48", "::/49", "::/64", "::/80", "::/96", "::/112", "::/127", "::1/128"} {
		if r, err := scope.NewRule(C, false, v, "", defMin); err == nil {
			t.Errorf("inclusion %q covering ::1 was accepted as %v", v, r.Prefix())
		}
	}
	if r, err := scope.NewRule(I, false, "::1", "", defMin); err == nil {
		t.Errorf("ip rule ::1 was accepted as %v", r.Addr())
	}
	if _, err := scope.NewRule(I, true, "::1", "", defMin); err == nil {
		t.Error("ip exclusion ::1 was accepted. Refused exclusions are reported, not kept")
	}
}

// Targets inside an embedded range are never canonical, so a rule set cannot
// allow them whatever it contains.
func TestEmbeddedRangeTargetsAreDeniedAsInvalid(t *testing.T) {
	allowEverythingPossible := build(t, inc(C, "2001:db8:1::/48", ""), inc(D, "example.com", ""), inc(I, "203.0.113.7", ""))
	for _, s := range []string{"::2", "::", "64:ff9b::1", "64:ff9b:1::1", "2002::1", "2001::1", "2001:0:ffff:ffff:ffff:ffff:ffff:ffff", "::ffff:ffff"} {
		tg := scope.Target{Addr: netip.MustParseAddr(s), Port: 443}
		if d := allowEverythingPossible.Evaluate(tg); d.Allowed || d.Reason != scope.ReasonInvalidTarget {
			t.Errorf("Evaluate(%s) = %+v, want invalid_target", s, d)
		}
	}
}
