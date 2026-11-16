package scope_test

import (
	"net/netip"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// embeddedTable is an independent copy of the ranges of S1.1 step 7. The tests
// and the fuzz oracles use it so that they do not share data with the package.
// It is the policy as approved. Do not extend it without a decision. The SIIT
// range was added by decision D-88 (record 0005).
var embeddedTable = []netip.Prefix{
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("::ffff:0:0:0/96"), // SIIT, RFC 2765 (D-88)
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2001::/32"),
}

func inEmbedded(a netip.Addr) bool {
	for _, r := range embeddedTable {
		if r.Contains(a) {
			return true
		}
	}
	return false
}

func overlapsEmbedded(p netip.Prefix) bool {
	for _, r := range embeddedTable {
		if p.Overlaps(r) {
			return true
		}
	}
	return false
}

// insideEmbedded reports whether p is equal to or inside a range.
func insideEmbedded(p netip.Prefix) bool {
	for _, r := range embeddedTable {
		if r.Contains(p.Addr()) && p.Bits() >= r.Bits() {
			return true
		}
	}
	return false
}

// lastAddr returns the highest address of an IPv6 prefix.
func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().As16()
	for i := p.Bits(); i < 128; i++ {
		b[i/8] |= 1 << (7 - uint(i%8))
	}
	return netip.AddrFrom16(b)
}

// parseLow reads an inclusion with the zero lengths that the unfloored entry
// point of the tests allows. The public API cannot build such a minimum (the
// floor of PrefixMin), but the overlap check must hold on its own.
func parseLow(s string) (netip.Prefix, error) { return scope.ParseInclusionUnfloored(s, 0, 0) }

// Positive and negative cases at both ends of every range, and next to it.
func TestEmbeddedRangeBoundaries(t *testing.T) {
	for _, r := range embeddedTable {
		first, last := r.Addr(), lastAddr(r)
		mid := first
		for i := 0; i < 5; i++ {
			mid = mid.Next()
		}
		t.Run(r.String(), func(t *testing.T) {
			// Inside: every address form of the range is ambiguous.
			for _, a := range []netip.Addr{first, last, mid} {
				if got, err := scope.ParseAddr(a.String()); err == nil {
					t.Errorf("ParseAddr(%v) inside %v = %v, want a refusal", a, r, got)
				}
			}
			// Next to it: accepted, unless the neighbour is inside another range.
			var neighbours []netip.Addr
			if before := first.Prev(); before.IsValid() {
				neighbours = append(neighbours, before)
			}
			if after := last.Next(); after.IsValid() {
				neighbours = append(neighbours, after)
			}
			for _, a := range neighbours {
				if inEmbedded(a) {
					continue
				}
				if _, err := scope.ParseAddr(a.String()); err != nil {
					t.Errorf("ParseAddr(%v) next to %v was refused: %v", a, r, err)
				}
				host := netip.PrefixFrom(a, 128)
				if _, err := scope.ParseInclusionPrefix(host.String(), scope.DefaultPrefixMin()); err != nil {
					t.Errorf("inclusion %v next to %v was refused: %v", host, r, err)
				}
				if _, err := scope.ParseExclusionPrefix(host.String()); err != nil {
					t.Errorf("exclusion %v next to %v was refused: %v", host, r, err)
				}
			}

			// Prefixes inside or equal: refused as inclusion and as exclusion.
			inside := []netip.Prefix{r, netip.PrefixFrom(first, 128), netip.PrefixFrom(last, 128)}
			if r.Bits() < 120 {
				inside = append(inside, netip.PrefixFrom(first, r.Bits()+8).Masked())
			}
			for _, p := range inside {
				if got, err := parseLow(p.String()); err == nil {
					t.Errorf("inclusion %v inside %v = %v, want a refusal", p, r, got)
				}
				if got, err := scope.ParseExclusionPrefix(p.String()); err == nil {
					t.Errorf("exclusion %v inside %v = %v, want a refusal", p, r, got)
				}
			}

			// A prefix that contains the range: an exclusion only denies more and
			// is accepted. An inclusion would look as if it covered the range and
			// is refused.
			if r.Bits() > 0 {
				wider := netip.PrefixFrom(first, r.Bits()-1).Masked()
				if _, err := scope.ParseExclusionPrefix(wider.String()); err != nil {
					t.Errorf("exclusion %v containing %v was refused: %v", wider, r, err)
				}
				if got, err := parseLow(wider.String()); err == nil {
					t.Errorf("inclusion %v containing %v = %v, want a refusal", wider, r, got)
				}
			}
		})
	}
}

// Prefix-length boundaries around each range, for the inclusion default minimum.
func TestInclusionBoundaryOfTheDefaultMinimum(t *testing.T) {
	for _, in := range []string{"2001:db8:1::/48", "2001:db8:1::/49", "203.0.113.0/24", "203.0.113.0/25"} {
		if _, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); err != nil {
			t.Errorf("%q at or above the minimum was refused: %v", in, err)
		}
	}
	for _, in := range []string{"2001:db8::/47", "2001:db8::/32", "203.0.112.0/23", "198.51.0.0/16"} {
		if _, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); err == nil {
			t.Errorf("%q below the minimum was accepted", in)
		}
	}
}
