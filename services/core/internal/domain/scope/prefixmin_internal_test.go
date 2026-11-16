package scope

import "testing"

// isSet is the second line of defence behind NewPrefixMin. A value that is
// built inside the package, bypassing the constructor, is still refused below
// the floor.
func TestIsSetRefusesEveryValueOutsideTheFloor(t *testing.T) {
	cases := []struct {
		m    PrefixMin
		want bool
	}{
		{PrefixMin{}, false},
		{PrefixMin{v4: 24, v6: 48}, true},
		{PrefixMin{v4: 32, v6: 128}, true},
		{PrefixMin{v4: 23, v6: 48}, false},
		{PrefixMin{v4: 24, v6: 47}, false},
		{PrefixMin{v4: 1, v6: 1}, false},
		{PrefixMin{v4: 24}, false},
		{PrefixMin{v6: 48}, false},
		{PrefixMin{v4: -1, v6: 48}, false},
		{PrefixMin{v4: 24, v6: -1}, false},
		{PrefixMin{v4: 33, v6: 48}, false},
		{PrefixMin{v4: 24, v6: 129}, false},
	}
	for _, tc := range cases {
		if got := tc.m.IsSet(); got != tc.want {
			t.Errorf("%+v.IsSet() = %v, want %v", tc.m, got, tc.want)
		}
	}
}

func TestParseInclusionRefusesAnUnsetValueBuiltInsideThePackage(t *testing.T) {
	for _, m := range []PrefixMin{{v4: 1, v6: 1}, {v4: 23, v6: 48}, {v4: 24, v6: 47}, {v4: 24}} {
		if p, err := ParseInclusionPrefix("203.0.113.0/24", m); err != ErrMinimumUnset {
			t.Errorf("ParseInclusionPrefix with %+v = %v, %v, want ErrMinimumUnset", m, p, err)
		}
	}
}
