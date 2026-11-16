package scope_test

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// The fuzz targets below check invariants. Where a stdlib or an independent
// table gives an oracle, they use it instead of the package's own helpers.
// Seeds include valid and borderline inputs so that the corpus reaches the
// success paths of the address and prefix parsers.

func FuzzParseAddr(f *testing.F) {
	for _, s := range []string{
		"127.0.0.1", "203.0.113.7", "2001:db8::7", "2001:DB8:0:0:0:0:0:7", "::ffff:127.0.0.1", "::ffff:7f00:1",
		"::1", "::", "::2", "::ffff:0:1.2.3.4", "64:ff9b::1", "64:ff9a::1", "2002::1", "2003::1", "2001::1", "2001:1::1",
		"::ffff:0:0:0", "::ffff:0:ffff:ffff", "::fffe:ffff:ffff:ffff", "::ffff:1:0:0", "::1%eth0", "[::1]", "0x7f.0.0.1", "127.1", "1.2.3.4.", "", "1.2.3.0/24",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		a, err := scope.ParseAddr(s) // must not panic
		if err != nil {
			if !errors.Is(err, scope.ErrAmbiguous) || a.IsValid() {
				t.Fatalf("ParseAddr(%q) = %v, %v", s, a, err)
			}
			return
		}
		if a.Is4In6() || a.Zone() != "" {
			t.Fatalf("ParseAddr(%q) = %v is not canonical", s, a)
		}
		// Independent oracles: the stdlib agrees on the address, and no accepted
		// address lies in an embedded range of S1.1 step 7.
		std, err := netip.ParseAddr(s)
		if err != nil || std.Unmap() != a {
			t.Fatalf("ParseAddr(%q) = %v but netip says %v, %v", s, a, std, err)
		}
		if inEmbedded(a) {
			t.Fatalf("ParseAddr(%q) = %v is inside an embedded range", s, a)
		}
		again, err := scope.ParseAddr(a.String())
		if err != nil || again != a {
			t.Fatalf("not idempotent: %q -> %v -> %v, %v", s, a, again, err)
		}
	})
}

func FuzzParseInclusionPrefix(f *testing.F) {
	for _, s := range []string{
		"203.0.113.0/24", "203.0.113.0/25", "203.0.113.7/32", "198.51.0.0/16", "0.0.0.0/0", "2001:db8:1::/48",
		"2001:db8::/32", "2001:db8:1::7/128", "::ffff:203.0.113.0/120", "::/48", "::/64", "64:ff9b::/48", "2002::/48",
		"2001::/48", "2003::/48", "::ffff:0:0:0/96", "::ffff:0:0:0/80", "::1/128", "10/8", "203.0.113.5/24", "203.0.113.0/024", "", "/", "::/0",
	} {
		f.Add(s, uint8(24), uint8(48))
	}
	f.Add("2000::/3", uint8(1), uint8(1))
	f.Add("203.0.113.0/24", uint8(0), uint8(48))
	f.Add("203.0.113.0/24", uint8(23), uint8(48))
	f.Add("2001:db8::/47", uint8(24), uint8(47))
	f.Fuzz(func(t *testing.T, s string, v4, v6 uint8) {
		// A value outside the floor never builds a usable minimum.
		min, merr := scope.NewPrefixMin(int(v4), int(v6))
		p, err := scope.ParseInclusionPrefix(s, min) // must not panic
		valid := v4 >= scope.MinPrefixLenV4 && v4 <= 32 && v6 >= scope.MinPrefixLenV6 && v6 <= 128
		if valid != (merr == nil) {
			t.Fatalf("NewPrefixMin(%d, %d) error = %v, want an error only outside the floor", v4, v6, merr)
		}
		if !valid {
			if !errors.Is(err, scope.ErrMinimumUnset) || p.IsValid() {
				t.Fatalf("ParseInclusionPrefix(%q, %+v) = %v, %v, want ErrMinimumUnset", s, min, p, err)
			}
			return
		}
		if err != nil {
			if p.IsValid() {
				t.Fatalf("ParseInclusionPrefix(%q) returned %v with %v", s, p, err)
			}
			return
		}
		if p != p.Masked() || p.Addr().Is4In6() || p.Addr().Zone() != "" {
			t.Fatalf("ParseInclusionPrefix(%q) = %v is not canonical", s, p)
		}
		if (p.Addr().Is4() && p.Bits() < min.V4()) || (p.Addr().Is6() && p.Bits() < min.V6()) {
			t.Fatalf("ParseInclusionPrefix(%q, %+v) = %v is below the minimum", s, min, p)
		}
		if p.Addr().Is6() && overlapsEmbedded(p) {
			t.Fatalf("ParseInclusionPrefix(%q) = %v overlaps an embedded range", s, p)
		}
		again, err := scope.ParseInclusionPrefix(p.String(), min)
		if err != nil || again != p {
			t.Fatalf("not idempotent: %q -> %v -> %v, %v", s, p, again, err)
		}
	})
}

func FuzzParseExclusionPrefix(f *testing.F) {
	for _, s := range []string{
		"203.0.113.0/24", "0.0.0.0/0", "198.51.0.0/16", "::/0", "2001:db8::/32", "::ffff:0:0/96", "::ffff:198.51.0.0/112",
		"::ffff:0:0:0/96", "::ffff:0:0:0/95", "::ffff:0:0:0/104", "::/1", "::/95", "::/96", "::1/128", "2002::/16", "2002::/48", "2001::/16", "2001::/32", "64:ff9b::/95", "64:ff9b::/96",
		"10/8", "203.0.113.5/24", "", "/",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := scope.ParseExclusionPrefix(s) // must not panic
		if err != nil {
			if !errors.Is(err, scope.ErrAmbiguous) || p.IsValid() {
				t.Fatalf("ParseExclusionPrefix(%q) = %v, %v", s, p, err)
			}
			return
		}
		if p != p.Masked() || p.Addr().Is4In6() || p.Addr().Zone() != "" {
			t.Fatalf("ParseExclusionPrefix(%q) = %v is not canonical", s, p)
		}
		if p.Addr().Is6() && insideEmbedded(p) {
			t.Fatalf("ParseExclusionPrefix(%q) = %v is inside an embedded range", s, p)
		}
		again, err := scope.ParseExclusionPrefix(p.String())
		if err != nil || again != p {
			t.Fatalf("not idempotent: %q -> %v -> %v, %v", s, p, again, err)
		}
	})
}

func FuzzParseTarget(f *testing.F) {
	for _, s := range []string{
		"example.com", "Example.COM.", "a.b.example.com", "xn--fu-hia.de", "localhost", "203.0.113.7", "2001:db8::7",
		"::ffff:203.0.113.7", "::1", "127.1", "2130706433", "0x7f.0.0.1", "1.2.3.4.", "user@example.com", "example.com:443",
		"[::1]", "exämple.com", "a_b.example.com", "*.example.com", "", ":", "::",
	} {
		f.Add(s, uint16(443))
	}
	f.Add("example.com", uint16(0))
	f.Fuzz(func(t *testing.T, s string, port uint16) {
		tg, err := scope.ParseTarget(s, port) // must not panic
		if err != nil {
			if tg != (scope.Target{}) {
				t.Fatalf("ParseTarget(%q, %d) returned %+v with %v", s, port, tg, err)
			}
			if !errors.Is(err, scope.ErrAmbiguous) && !errors.Is(err, scope.ErrInvalidPorts) {
				t.Fatalf("ParseTarget(%q, %d) error %v has an unexpected class", s, port, err)
			}
			return
		}
		if port == 0 || tg.Port != port {
			t.Fatalf("ParseTarget(%q, %d) = %+v", s, port, tg)
		}
		hasHost, hasAddr := tg.Host != "", tg.Addr.IsValid()
		if hasHost == hasAddr {
			t.Fatalf("ParseTarget(%q) = %+v must have exactly one of host and address", s, tg)
		}
		// A target the parser built is canonical for the evaluator.
		if d := (scope.RuleSet{}).Evaluate(tg); d.Reason != scope.ReasonNoMatchingRule {
			t.Fatalf("ParseTarget(%q) = %+v is not canonical: %+v", s, tg, d)
		}
		// Reading the canonical text again gives the same target.
		text := string(tg.Host)
		if hasAddr {
			text = tg.Addr.String()
		}
		if again, err := scope.ParseTarget(text, port); err != nil || again != tg {
			t.Fatalf("not idempotent: %q -> %+v -> %+v, %v", s, tg, again, err)
		}
		if hasAddr {
			// Oracle: the stdlib reads the same address, and nothing is inside an embedded range.
			std, err := netip.ParseAddr(s)
			if err != nil || std.Unmap() != tg.Addr || inEmbedded(tg.Addr) {
				t.Fatalf("ParseTarget(%q) = %v but netip says %v, %v", s, tg.Addr, std, err)
			}
			return
		}
		// Oracle for hosts: no address spelling, and only canonical characters.
		if _, err := netip.ParseAddr(s); err == nil {
			t.Fatalf("ParseTarget(%q) read an address literal as the host %q", s, tg.Host)
		}
		h := string(tg.Host)
		if len(h) == 0 || len(h) > 253 || h != strings.ToLower(h) {
			t.Fatalf("host %q is not canonical", h)
		}
		for _, l := range strings.Split(h, ".") {
			if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
				t.Fatalf("host %q has a bad label %q", h, l)
			}
			for _, c := range []byte(l) {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					t.Fatalf("host %q has the byte %#x", h, c)
				}
			}
		}
		if last := h[strings.LastIndexByte(h, '.')+1:]; isNumberLike(last) {
			t.Fatalf("host %q ends in a numeric label %q", h, last)
		}
	})
}

// isNumberLike is written for the oracle: all digits, or 0x and hex digits.
func isNumberLike(l string) bool {
	if l == "" {
		return false
	}
	digits := true
	for _, c := range []byte(l) {
		if c < '0' || c > '9' {
			digits = false
		}
	}
	if digits {
		return true
	}
	if strings.HasPrefix(l, "0x") {
		for _, c := range []byte(l[2:]) {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
		return true
	}
	return false
}

func FuzzNewRule(f *testing.F) {
	for _, k := range []uint8{1, 2, 3, 4} {
		for _, ex := range []bool{false, true} {
			f.Add(k, ex, "example.com", "")
			f.Add(k, ex, "*.example.com", "")
			f.Add(k, ex, "203.0.113.7", "8443,80")
			f.Add(k, ex, "203.0.113.0/24", "443")
			f.Add(k, ex, "2001:db8:1::/48", "")
			f.Add(k, ex, "::/0", "1-65535")
			f.Add(k, ex, "::ffff:203.0.113.0/120", "")
		}
	}
	f.Add(uint8(0), true, "example.com", "")
	f.Add(uint8(9), false, "example.com", "")
	f.Add(uint8(4), false, "::/48", "")
	f.Add(uint8(1), true, "https://example.com/x", "0")
	f.Fuzz(func(t *testing.T, kindN uint8, exclude bool, value, ports string) {
		kind := scope.RuleKind(kindN % 7)                            // 0 and 5, 6 are not kinds
		r, err := scope.NewRule(kind, exclude, value, ports, defMin) // must not panic
		var ue *scope.UnsupportedExclusionError
		if err != nil {
			if r.Kind() != 0 {
				t.Fatalf("NewRule returned a rule %v with %v", r, err)
			}
			switch {
			case exclude:
				if !errors.As(err, &ue) || ue.Input != value || ue.Ports != ports || ue.Kind != kind || !errors.Is(err, scope.ErrUnsupportedExclusion) {
					t.Fatalf("a refused exclusion must be an *UnsupportedExclusionError with its input: %v", err)
				}
			default:
				if errors.As(err, &ue) {
					t.Fatalf("a refused inclusion is an UnsupportedExclusionError: %v", err)
				}
				if kind == scope.KindWildcard && !errors.Is(err, scope.ErrWildcardInclusion) {
					t.Fatalf("a wildcard inclusion must give ErrWildcardInclusion, got %v", err)
				}
			}
			return
		}

		// An accepted rule is valid and well-formed.
		if kind < scope.KindDomain || kind > scope.KindCIDR {
			t.Fatalf("an unknown kind %v was accepted", kind)
		}
		if kind == scope.KindWildcard && !exclude {
			t.Fatalf("a wildcard inclusion was accepted: %q", value)
		}
		if r.Kind() != kind || r.Exclude() != exclude || r.Ports().IsEmpty() {
			t.Fatalf("bad rule %v", r)
		}
		target := scope.Target{Host: "example.com", Port: 443}
		if d := (scope.RuleSet{r}).Evaluate(target); d.Reason == scope.ReasonInvalidRuleSet {
			t.Fatalf("an accepted rule is invalid for the evaluator: %q", value)
		}
		// The canonical text reads back to the same rule.
		again, err := roundTrip(r)
		if err != nil || !sameRule(r, again) {
			t.Fatalf("round trip of %q/%q failed: %v", r.Value(), r.Ports().String(), err)
		}
		// Independent checks of the accepted prefix.
		if kind == scope.KindCIDR {
			p := r.Prefix()
			if p != p.Masked() {
				t.Fatalf("prefix %v has host bits", p)
			}
			switch {
			case !exclude && ((p.Addr().Is4() && p.Bits() < 24) || (p.Addr().Is6() && p.Bits() < 48)):
				t.Fatalf("inclusion %v is below the minimum", p)
			case !exclude && p.Addr().Is6() && overlapsEmbedded(p):
				t.Fatalf("inclusion %v overlaps an embedded range", p)
			case exclude && p.Addr().Is6() && insideEmbedded(p):
				t.Fatalf("exclusion %v is inside an embedded range", p)
			}
		}
		if kind == scope.KindIP && (r.Addr().Is4In6() || inEmbedded(r.Addr())) {
			t.Fatalf("ip rule %v is not canonical", r.Addr())
		}
		if (kind == scope.KindDomain || kind == scope.KindWildcard) && r.Host() != scope.Host(strings.ToLower(string(r.Host()))) {
			t.Fatalf("host %q is not lower case", r.Host())
		}
		// The default for missing ports.
		if ports == "" {
			want := "80,443"
			if exclude {
				want = "1-65535"
			}
			if r.Ports().String() != want {
				t.Fatalf("default ports = %q, want %q", r.Ports().String(), want)
			}
		}
	})
}
