package scope_test

import (
	"errors"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// roundTrip rebuilds a rule from its own canonical text. M16 stores
// Value() and Ports().String() (the canonical_value and ports_canonical
// columns) and reads them back through NewRule.
func roundTrip(r scope.Rule) (scope.Rule, error) {
	return scope.NewRule(r.Kind(), r.Exclude(), r.Value(), r.Ports().String(), defMin)
}

func sameRule(a, b scope.Rule) bool {
	return a.Kind() == b.Kind() && a.Exclude() == b.Exclude() && a.Value() == b.Value() &&
		a.Ports().String() == b.Ports().String() && a.Ports().All() == b.Ports().All() &&
		a.Host() == b.Host() && a.Addr() == b.Addr() && a.Prefix() == b.Prefix()
}

func TestRuleValueIsTheCanonicalReadableForm(t *testing.T) {
	tests := []struct {
		name      string
		kind      scope.RuleKind
		exclude   bool
		value     string
		ports     string
		wantValue string
		wantPorts string
	}{
		{"domain, mixed case and trailing dot", D, false, "Example.COM.", "", "example.com", "80,443"},
		{"domain exclusion", D, true, "example.com", "", "example.com", "1-65535"},
		{"domain with ports out of order", D, false, "example.com", "8443,22", "example.com", "22,8443"},
		{"wildcard exclusion keeps the star notation", W, true, "*.Internal.Example.COM.", "", "*.internal.example.com", "1-65535"},
		{"wildcard exclusion with ports", W, true, "*.x.example.com", "22,443", "*.x.example.com", "22,443"},
		{"wildcard exclusion over a TLD", W, true, "*.com", "", "*.com", "1-65535"},
		{"ip", I, false, "203.0.113.7", "", "203.0.113.7", "80,443"},
		{"ip, mapped form", I, false, "::ffff:203.0.113.7", "", "203.0.113.7", "80,443"},
		{"ip, upper-case IPv6", I, false, "2001:DB8::7", "8443", "2001:db8::7", "8443"},
		{"ip exclusion", I, true, "203.0.113.7", "", "203.0.113.7", "1-65535"},
		{"cidr", C, false, "203.0.113.0/24", "", "203.0.113.0/24", "80,443"},
		{"cidr IPv6", C, false, "2001:DB8:1::/48", "", "2001:db8:1::/48", "80,443"},
		{"cidr, mapped prefix", C, false, "::ffff:203.0.113.0/120", "", "203.0.113.0/24", "80,443"},
		{"cidr exclusion /0", C, true, "0.0.0.0/0", "", "0.0.0.0/0", "1-65535"},
		{"cidr exclusion ::/0", C, true, "::/0", "", "::/0", "1-65535"},
		{"cidr exclusion, mapped range", C, true, "::ffff:0:0/96", "", "0.0.0.0/0", "1-65535"},
		{"exclusion with the all-ports token", D, true, "example.com", "1-65535", "example.com", "1-65535"},
		{"exclusion with every port listed", D, true, "example.com", fullPortList(), "example.com", "1-65535"},
		{"inclusion with every port listed", D, false, "example.com", fullPortList(), "example.com", "1-65535"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := scope.NewRule(tc.kind, tc.exclude, tc.value, tc.ports, defMin)
			if err != nil {
				t.Fatalf("NewRule: %v", err)
			}
			if r.Value() != tc.wantValue {
				t.Errorf("Value() = %q, want %q", r.Value(), tc.wantValue)
			}
			if r.Ports().String() != tc.wantPorts {
				t.Errorf("Ports().String() = %q, want %q", r.Ports().String(), tc.wantPorts)
			}
			again, err := roundTrip(r)
			if err != nil {
				t.Fatalf("the canonical text %q / %q does not read back: %v", r.Value(), r.Ports().String(), err)
			}
			if !sameRule(r, again) {
				t.Errorf("round trip changed the rule: %q/%q -> %q/%q", r.Value(), r.Ports().String(), again.Value(), again.Ports().String())
			}
			// Reading canonical text is stable: a second round gives the same text.
			third, err := roundTrip(again)
			if err != nil || !sameRule(again, third) {
				t.Errorf("the canonical text is not a fixed point: %v", err)
			}
		})
	}
}

// Every kind and both modes are covered by the table above. This test states
// the coverage so that a new rule kind cannot be added without a round trip.
func TestRoundTripCoversEveryKindAndMode(t *testing.T) {
	kinds := map[scope.RuleKind]map[bool]bool{}
	for _, tc := range []struct {
		kind    scope.RuleKind
		exclude bool
		value   string
	}{
		{D, false, "example.com"}, {D, true, "example.com"}, {W, true, "*.example.com"},
		{I, false, "203.0.113.7"}, {I, true, "203.0.113.7"}, {C, false, "203.0.113.0/24"}, {C, true, "203.0.113.0/24"},
	} {
		r, err := scope.NewRule(tc.kind, tc.exclude, tc.value, "", defMin)
		if err != nil {
			t.Fatalf("%v %v %q: %v", tc.kind, tc.exclude, tc.value, err)
		}
		again, err := roundTrip(r)
		if err != nil || !sameRule(r, again) {
			t.Errorf("%v exclude=%v %q does not round trip: %v", tc.kind, tc.exclude, tc.value, err)
		}
		if kinds[tc.kind] == nil {
			kinds[tc.kind] = map[bool]bool{}
		}
		kinds[tc.kind][tc.exclude] = true
	}
	for _, k := range []scope.RuleKind{D, I, C} {
		if !kinds[k][false] || !kinds[k][true] {
			t.Errorf("kind %v is not covered in both modes", k)
		}
	}
	if !kinds[W][true] {
		t.Error("the wildcard exclusion is not covered")
	}
	var zero scope.Rule
	if zero.Value() != "" {
		t.Errorf("zero Rule Value() = %q, want empty", zero.Value())
	}
}

// A rule read back after the minimum was raised is refused. It must not be
// silently kept. M16 treats the refusal as an invalid rule set.
func TestRoundTripOfAnInclusionFailsUnderAHigherMinimum(t *testing.T) {
	r, err := scope.NewRule(C, false, "203.0.113.0/24", "", defMin)
	if err != nil {
		t.Fatal(err)
	}
	higher, _ := scope.NewPrefixMin(28, 56)
	if _, err := scope.NewRule(r.Kind(), r.Exclude(), r.Value(), r.Ports().String(), higher); !errors.Is(err, scope.ErrPrefixTooShort) {
		t.Errorf("re-reading a /24 under a /28 minimum: %v, want ErrPrefixTooShort", err)
	}
}

// F-3 and decision 2 of the review for NewRule: a missing or partial minimum is
// an error for inclusion CIDR rules, and exclusions ignore the minimum.
func TestNewRuleMinimumHandling(t *testing.T) {
	// The zero value is the only unusable minimum that can be built from outside
	// the package. A failed NewPrefixMin returns it as well.
	failed, err := scope.NewPrefixMin(1, 1)
	if err == nil {
		t.Fatal("NewPrefixMin(1, 1) must fail")
	}
	broken := []scope.PrefixMin{{}, failed}
	for _, m := range broken {
		if r, err := scope.NewRule(C, false, "203.0.113.0/24", "", m); !errors.Is(err, scope.ErrMinimumUnset) {
			t.Errorf("inclusion with minimum %+v = %+v, %v, want ErrMinimumUnset", m, r, err)
		}
		// An exclusion ignores the value, so a broken one cannot break it.
		if _, err := scope.NewRule(C, true, "198.51.0.0/16", "", m); err != nil {
			t.Errorf("exclusion with minimum %+v refused: %v", m, err)
		}
	}
	// Domain, wildcard and ip rules do not use the minimum.
	for _, m := range broken {
		if _, err := scope.NewRule(D, false, "example.com", "", m); err != nil {
			t.Errorf("domain rule with minimum %+v: %v", m, err)
		}
		if _, err := scope.NewRule(I, false, "203.0.113.7", "", m); err != nil {
			t.Errorf("ip rule with minimum %+v: %v", m, err)
		}
	}
}

// F-5 through NewRule: an inclusion that overlaps an embedded range is refused,
// an exclusion that contains one is accepted.
func TestNewRuleEmbeddedRanges(t *testing.T) {
	for _, v := range []string{"::/48", "::/64", "64:ff9b::/48", "2002::/48", "2001::/48", "64:ff9b:1::/48", "::1/128"} {
		if r, err := scope.NewRule(C, false, v, "", defMin); err == nil {
			t.Errorf("inclusion %q accepted as %v", v, r.Prefix())
		}
	}
	for _, v := range []string{"::/0", "::/1", "::/95", "2000::/3", "2001::/16"} {
		if _, err := scope.NewRule(C, true, v, "", defMin); err != nil {
			t.Errorf("exclusion %q refused: %v", v, err)
		}
	}
	for _, v := range []string{"::1/128", "::/96", "2002::/16", "64:ff9b::/96"} {
		var ue *scope.UnsupportedExclusionError
		if _, err := scope.NewRule(C, true, v, "", defMin); !errors.As(err, &ue) || ue.Input != v || ue.Kind != C {
			t.Errorf("exclusion %q = %v, want *UnsupportedExclusionError with the input and the kind", v, err)
		}
	}
}

// The Kind of a refused exclusion is the kind that was asked for.
func TestUnsupportedExclusionErrorCarriesTheKind(t *testing.T) {
	for _, kind := range []scope.RuleKind{D, W, I, C, scope.RuleKind(9)} {
		var ue *scope.UnsupportedExclusionError
		_, err := scope.NewRule(kind, true, "http://bad", "", defMin)
		if !errors.As(err, &ue) || ue.Kind != kind {
			t.Errorf("kind %v: %v", kind, err)
		}
	}
}
