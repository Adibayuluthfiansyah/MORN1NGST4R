package scope_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// The floor of PrefixMin: no caller can build a minimum that is shorter than
// /24 for IPv4 or /48 for IPv6. A policy may only tighten it (M1 follow-up of
// the review, item 1).

func TestPrefixMinFloorConstants(t *testing.T) {
	if scope.MinPrefixLenV4 != 24 || scope.MinPrefixLenV6 != 48 {
		t.Errorf("floor = /%d and /%d, want /24 and /48", scope.MinPrefixLenV4, scope.MinPrefixLenV6)
	}
}

func TestNewPrefixMin(t *testing.T) {
	cases := []struct {
		name   string
		v4, v6 int
		ok     bool
	}{
		// exactly the floor and stricter values
		{"exactly the floor", 24, 48, true},
		{"stricter in both families", 28, 56, true},
		{"strictest in both families", 32, 128, true},
		{"stricter IPv6 only", 24, 56, true},
		{"stricter IPv4 only", 28, 48, true},
		// below the floor, one step and far
		{"IPv4 one below the floor", 23, 48, false},
		{"IPv6 one below the floor", 24, 47, false},
		{"both one below the floor", 23, 47, false},
		{"the old lowest value", 1, 1, false},
		{"the whole internet", 0, 0, false},
		// zero and partial
		{"IPv4 only", 24, 0, false},
		{"IPv6 only", 0, 48, false},
		{"IPv4 zero", 0, 56, false},
		{"IPv6 zero", 28, 0, false},
		// negative
		{"IPv4 negative", -1, 48, false},
		{"IPv6 negative", 24, -1, false},
		{"both negative", -24, -48, false},
		{"extreme negative", math.MinInt, math.MinInt, false},
		// above the family length
		{"IPv4 above 32", 33, 48, false},
		{"IPv6 above 128", 24, 129, false},
		{"extreme positive", math.MaxInt, math.MaxInt, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := scope.NewPrefixMin(tc.v4, tc.v6)
			if tc.ok {
				if err != nil || !m.IsSet() || m.V4() != tc.v4 || m.V6() != tc.v6 {
					t.Errorf("NewPrefixMin(%d, %d) = %v/%v set=%v, %v, want it accepted unchanged", tc.v4, tc.v6, m.V4(), m.V6(), m.IsSet(), err)
				}
				return
			}
			if !errors.Is(err, scope.ErrInvalidMinimum) {
				t.Errorf("NewPrefixMin(%d, %d) error = %v, want ErrInvalidMinimum", tc.v4, tc.v6, err)
			}
			// The value that comes back with the error is unusable.
			if m.IsSet() || m.V4() != 0 || m.V6() != 0 {
				t.Errorf("NewPrefixMin(%d, %d) returned a usable value %v/%v with an error", tc.v4, tc.v6, m.V4(), m.V6())
			}
			if p, perr := scope.ParseInclusionPrefix("203.0.113.0/24", m); !errors.Is(perr, scope.ErrMinimumUnset) || p.IsValid() {
				t.Errorf("a failed minimum parsed an inclusion: %v, %v", p, perr)
			}
		})
	}
}

func TestZeroPrefixMinIsUnset(t *testing.T) {
	var z scope.PrefixMin
	if z.IsSet() || z.V4() != 0 || z.V6() != 0 {
		t.Errorf("zero PrefixMin = %v/%v set=%v, want unset", z.V4(), z.V6(), z.IsSet())
	}
	if p, err := scope.ParseInclusionPrefix("203.0.113.0/24", z); !errors.Is(err, scope.ErrMinimumUnset) || p.IsValid() {
		t.Errorf("ParseInclusionPrefix with the zero value = %v, %v, want ErrMinimumUnset", p, err)
	}
	if r, err := scope.NewRule(scope.KindCIDR, false, "203.0.113.0/24", "", z); !errors.Is(err, scope.ErrMinimumUnset) {
		t.Errorf("NewRule with the zero value = %+v, %v, want ErrMinimumUnset", r, err)
	}
}

func TestDefaultPrefixMinIsTheFloor(t *testing.T) {
	d := scope.DefaultPrefixMin()
	if !d.IsSet() || d.V4() != scope.MinPrefixLenV4 || d.V6() != scope.MinPrefixLenV6 {
		t.Errorf("DefaultPrefixMin() = %v/%v set=%v, want the floor", d.V4(), d.V6(), d.IsSet())
	}
	same, err := scope.NewPrefixMin(24, 48)
	if err != nil || same != d {
		t.Errorf("NewPrefixMin(24, 48) = %v, %v, want the default value", same, err)
	}
	var z scope.PrefixMin
	if d == z {
		t.Error("the default equals the unset value")
	}
}

// Every accepted minimum is enforced by the parser, per family.
func TestPrefixMinIsEnforcedPerFamily(t *testing.T) {
	cases := []struct {
		name         string
		v4, v6       int
		accept, deny []string
	}{
		{"floor", 24, 48,
			[]string{"203.0.113.0/24", "203.0.113.0/25", "2001:db8:1::/48", "2001:db8:1::/49"},
			[]string{"203.0.112.0/23", "198.51.0.0/16", "0.0.0.0/0", "2001:db8::/47", "2001:db8::/32", "2600::/8"}},
		{"stricter in both", 28, 56,
			[]string{"203.0.113.0/28", "2001:db8:0:100::/56"},
			[]string{"203.0.113.0/27", "203.0.113.0/24", "2001:db8:0:200::/55", "2001:db8:1::/48"}},
		{"stricter IPv6 only", 24, 56,
			[]string{"203.0.113.0/24", "2001:db8:0:100::/56"},
			[]string{"203.0.112.0/23", "2001:db8:1::/48"}},
		{"stricter IPv4 only", 28, 48,
			[]string{"203.0.113.0/28", "2001:db8:1::/48"},
			[]string{"203.0.113.0/24", "2001:db8::/47"}},
		{"strictest", 32, 128,
			[]string{"203.0.113.7/32", "2001:db8::7/128"},
			[]string{"203.0.113.0/31", "2001:db8::/127"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := scope.NewPrefixMin(tc.v4, tc.v6)
			if err != nil {
				t.Fatal(err)
			}
			for _, in := range tc.accept {
				if _, err := scope.ParseInclusionPrefix(in, m); err != nil {
					t.Errorf("%q refused under /%d and /%d: %v", in, tc.v4, tc.v6, err)
				}
			}
			for _, in := range tc.deny {
				if p, err := scope.ParseInclusionPrefix(in, m); !errors.Is(err, scope.ErrPrefixTooShort) || p.IsValid() {
					t.Errorf("%q under /%d and /%d = %v, %v, want ErrPrefixTooShort", in, tc.v4, tc.v6, p, err)
				}
			}
		})
	}
}

// A shorter prefix than the floor is refused whatever the caller passes,
// including through NewRule.
func TestNoPublicPathAcceptsAPrefixBelowTheFloor(t *testing.T) {
	below := []string{"203.0.112.0/23", "198.51.0.0/16", "10.0.0.0/8", "0.0.0.0/0", "2001:db8::/47", "2001:db8::/32", "2600::/8"}
	for _, in := range below {
		if p, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); !errors.Is(err, scope.ErrPrefixTooShort) {
			t.Errorf("ParseInclusionPrefix(%q) = %v, %v, want ErrPrefixTooShort", in, p, err)
		}
		if r, err := scope.NewRule(scope.KindCIDR, false, in, "", scope.DefaultPrefixMin()); !errors.Is(err, scope.ErrPrefixTooShort) {
			t.Errorf("NewRule(%q) = %+v, %v, want ErrPrefixTooShort", in, r, err)
		}
	}
}

// PrefixMin is opaque. A caller outside the package must not be able to set a
// field, so a literal cannot build a lowered or half-set value. This test fails
// if someone exports a field.
func TestPrefixMinHasNoExportedFields(t *testing.T) {
	typ := reflect.TypeOf(scope.PrefixMin{})
	if typ.NumField() == 0 {
		t.Fatal("PrefixMin has no fields to check")
	}
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			t.Errorf("PrefixMin.%s is exported. A caller could lower it", f.Name)
		}
	}
}
