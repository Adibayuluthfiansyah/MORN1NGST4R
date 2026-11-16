package scope_test

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

func TestParseAddrAccepts(t *testing.T) {
	tests := []struct{ in, want string }{
		{"203.0.113.5", "203.0.113.5"},
		{"127.0.0.1", "127.0.0.1"},
		{"2001:db8::7", "2001:db8::7"},
		{"2001:DB8::7", "2001:db8::7"}, // an upper-case literal is canonicalized
		{"2001:0db8:0000:0000:0000:0000:0000:0007", "2001:db8::7"},
		{"::ffff:127.0.0.1", "127.0.0.1"}, // IPv4-mapped is evaluated as IPv4
		{"::ffff:7f00:1", "127.0.0.1"},
		{"::FFFF:7F00:1", "127.0.0.1"},
		{"::ffff:203.0.113.5", "203.0.113.5"},
		{"2001:db8::1", "2001:db8::1"}, // 2001:db8::/32 is not the Teredo range 2001::/32
	}
	for _, tc := range tests {
		got, err := scope.ParseAddr(tc.in)
		if err != nil {
			t.Errorf("ParseAddr(%q) error: %v", tc.in, err)
			continue
		}
		if got.String() != tc.want || got.Is4In6() || got.Zone() != "" {
			t.Errorf("ParseAddr(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseAddrRefusesAsAmbiguous(t *testing.T) {
	tests := []struct{ name, in string }{
		{"empty", ""},
		{"leading space", " 1.2.3.4"},
		{"trailing space", "1.2.3.4 "},
		{"newline", "1.2.3.4\n"},
		{"NUL", "1.2.3.4\x00"},
		{"short IPv4", "127.1"},
		{"decimal IPv4", "2130706433"},
		{"hex IPv4", "0x7f.0.0.1"},
		{"octal IPv4", "0177.0.0.1"},
		{"leading zeros", "127.000.000.001"},
		{"leading zero in one octet", "127.0.0.01"},
		{"octet over 255", "1.2.3.256"},
		{"five parts", "1.2.3.4.5"},
		{"three parts", "1.2.3"},
		{"trailing dot", "1.2.3.4."},
		{"fullwidth digits", "１２７.0.0.1"},
		{"ideographic full stop", "127。0.0.1"},
		{"negative", "-1.2.3.4"},
		{"plus", "+1.2.3.4"},
		{"zone", "::1%eth0"},
		{"zone on link local", "fe80::1%eth0"},
		{"escaped zone", "fe80::1%25eth0"},
		{"empty zone", "::1%"},
		{"brackets", "[::1]"},
		{"brackets and port", "[::1]:80"},
		{"IPv4 and port", "1.2.3.4:80"},
		{"CIDR text", "1.2.3.0/24"},
		{"userinfo", "user@1.2.3.4"},
		{"hostname", "example.com"},
		{"wildcard", "*"},
		{"too many groups", "1:2:3:4:5:6:7:8:9"},
		{"two double colons", "1::2::3"},
		{"invalid hex", "2001:db8::g"},
		{"longer than any address", strings.Repeat("1", 46)},
		// ::/96 is refused as a whole. There is no exception for the loopback
		// (decision of the owner, S1.1 step 7). The dotted tails of ::/96 are
		// IPv4-compatible forms and are refused too.
		{"IPv6 loopback", "::1"},
		{"IPv6 loopback, long form", "0:0:0:0:0:0:0:1"},
		{"IPv6 loopback, dotted tail", "::0.0.0.1"},
		{"IPv6 loopback, upper-case padded", "0000:0000:0000:0000:0000:0000:0000:0001"},
		{"unspecified", "::"},
		{"unspecified, long form", "0:0:0:0:0:0:0:0"},
		{"IPv4-compatible", "::127.0.0.1"},
		{"IPv4-compatible hex", "::7f00:1"},
		{"in ::/96, low value", "::2"},
		{"in ::/96, first of the range", "::0.0.0.0"},
		{"in ::/96, last of the range", "::ffff:ffff"},
		{"NAT64", "64:ff9b::cb00:7107"},
		{"NAT64 dotted", "64:ff9b::203.0.113.7"},
		{"NAT64 local-use", "64:ff9b:1::1"},
		{"NAT64 local-use inside /48", "64:ff9b:1:ffff::1"},
		{"6to4", "2002::1"},
		{"6to4 with embedded address", "2002:cb00:7107::1"},
		{"Teredo", "2001::1"},
		{"Teredo upper end", "2001:0:ffff:ffff:ffff:ffff:ffff:ffff"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scope.ParseAddr(tc.in)
			if !errors.Is(err, scope.ErrAmbiguous) {
				t.Fatalf("ParseAddr(%q) = %v, %v, want ErrAmbiguous", tc.in, got, err)
			}
			if got.IsValid() {
				t.Errorf("ParseAddr(%q) returned a valid address %v together with an error", tc.in, got)
			}
		})
	}
}

// Special-purpose addresses are not judged by the scope engine. The never-authorize
// tables of M9 (D-39, D-72) and the lab rules of M17 deny them. These cases pin
// that the scope layer parses them, so nobody reads Evaluate as the only guard.
func TestScopeEngineDoesNotJudgeSpecialPurposeAddresses(t *testing.T) {
	for _, s := range []string{"0.0.0.0", "255.255.255.255", "224.0.0.1", "169.254.169.254", "100.64.0.1", "127.0.0.1", "10.0.0.1", "fe80::1", "ff02::1", "fd00:ec2::254"} {
		if _, err := scope.ParseAddr(s); err != nil {
			t.Errorf("ParseAddr(%q) error: %v. The scope layer is not where these are denied (M9, M17)", s, err)
		}
	}
}

// D-88 (record 0005): the IPv4-translated (SIIT) range ::ffff:0:0:0/96 of RFC 2765
// is in the list of embedded-IPv4 ranges of S1.1 step 7, like NAT64. RFC 2765 was
// made obsolete by RFC 6145 and the IANA IPv6 special-purpose registry no longer
// lists the range. Every entry point refuses it.
func TestSIITRangeIsRefusedEverywhere(t *testing.T) {
	// First address, the dotted form, an address in the middle, and the last.
	for _, in := range []string{
		"::ffff:0:0:0", "::ffff:0:0.0.0.0", "::ffff:0:1.2.3.4", "::ffff:0:102:304", "0:0:0:0:ffff:0:7f00:1",
		"::ffff:0:ffff:ffff", "::ffff:0:255.255.255.255",
	} {
		if a, err := scope.ParseAddr(in); !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("ParseAddr(%q) = %v, %v, want ErrAmbiguous", in, a, err)
		}
		if tg, err := scope.ParseTarget(in, 443); !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("ParseTarget(%q) = %+v, %v, want ErrAmbiguous", in, tg, err)
		}
		if r, err := scope.NewRule(scope.KindIP, false, in, "", scope.DefaultPrefixMin()); !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("NewRule(ip inclusion %q) = %+v, %v, want ErrAmbiguous", in, r, err)
		}
		if r, err := scope.NewRule(scope.KindIP, true, in, "", scope.DefaultPrefixMin()); err == nil {
			t.Errorf("NewRule(ip exclusion %q) = %+v, want a refusal", in, r)
		}
	}
	// Prefixes equal to or inside the range, and prefixes that contain it.
	for _, in := range []string{
		"::ffff:0:0:0/96", "::ffff:0:0:0/104", "::ffff:0:102:300/120", "::ffff:0:1.2.3.4/128",
		"::ffff:0:0:0/95", "::ffff:0:0:0/80", "::ffff:0:0:0/64",
	} {
		if p, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("ParseInclusionPrefix(%q) = %v, %v, want ErrAmbiguous", in, p, err)
		}
		if r, err := scope.NewRule(scope.KindCIDR, false, in, "", scope.DefaultPrefixMin()); !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("NewRule(cidr inclusion %q) = %+v, %v, want ErrAmbiguous", in, r, err)
		}
	}
	for _, in := range []string{"::ffff:0:0:0/96", "::ffff:0:0:0/104", "::ffff:0:102:300/120", "::ffff:0:1.2.3.4/128"} {
		if p, err := scope.ParseExclusionPrefix(in); !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("ParseExclusionPrefix(%q) = %v, %v, want ErrAmbiguous", in, p, err)
		}
		if r, err := scope.NewRule(scope.KindCIDR, true, in, "", scope.DefaultPrefixMin()); err == nil {
			t.Errorf("NewRule(cidr exclusion %q) = %+v, want a refusal", in, r)
		}
	}
	// An exclusion that merely contains the range only denies more.
	for _, in := range []string{"::ffff:0:0:0/95", "::ffff:0:0:0/80"} {
		if _, err := scope.ParseExclusionPrefix(in); err != nil {
			t.Errorf("ParseExclusionPrefix(%q) containing the range was refused: %v", in, err)
		}
	}
	// IPv4-mapped ::ffff:0:0/96 is a different range and is still unmapped.
	if a, err := scope.ParseAddr("::ffff:1.2.3.4"); err != nil || a.String() != "1.2.3.4" {
		t.Errorf("ParseAddr(::ffff:1.2.3.4) = %v, %v, want 1.2.3.4", a, err)
	}
	// The addresses just before and after the range are not in it.
	for _, in := range []string{"::fffe:ffff:ffff:ffff", "::ffff:1:0:0"} {
		if _, err := scope.ParseAddr(in); err != nil {
			t.Errorf("ParseAddr(%q) next to the SIIT range was refused: %v", in, err)
		}
	}
}

// With the SIIT range refused, no rule set can allow a SIIT target.
func TestEvaluateDeniesASIITTargetWhateverTheRules(t *testing.T) {
	rs := scope.RuleSet{}
	for _, v := range []string{"2001:db8::/48", "2600::/48", "203.0.113.0/24"} {
		r, err := scope.NewRule(scope.KindCIDR, false, v, "", scope.DefaultPrefixMin())
		if err != nil {
			t.Fatal(err)
		}
		rs = append(rs, r)
	}
	for _, in := range []string{"::ffff:0:1.2.3.4", "::ffff:0:0:0", "::ffff:0:ffff:ffff"} {
		// ParseTarget refuses it, so a caller cannot build the target.
		if _, err := scope.ParseTarget(in, 443); err == nil {
			t.Errorf("ParseTarget(%q) was accepted", in)
		}
		// A hand-built target is not canonical and is denied.
		a := netip.MustParseAddr(in)
		if d := rs.Evaluate(scope.Target{Addr: a, Port: 443}); d.Allowed || d.Reason != scope.ReasonInvalidTarget {
			t.Errorf("Evaluate(%v) = %+v, want invalid_target", a, d)
		}
	}
}

func TestInclusionPrefixAccepts(t *testing.T) {
	min := scope.DefaultPrefixMin()
	tests := []struct{ in, want string }{
		{"203.0.113.0/24", "203.0.113.0/24"},
		{"203.0.113.0/25", "203.0.113.0/25"},
		{"203.0.113.7/32", "203.0.113.7/32"},
		{"2001:db8:1::/48", "2001:db8:1::/48"},
		{"2001:db8:1:2::/64", "2001:db8:1:2::/64"},
		{"2001:DB8:1::/48", "2001:db8:1::/48"},
		{"::ffff:203.0.113.0/120", "203.0.113.0/24"}, // a mapped prefix is evaluated as IPv4
		{"::ffff:203.0.113.7/128", "203.0.113.7/32"},
		{"2001:db8:1::7/128", "2001:db8:1::7/128"},
	}
	for _, tc := range tests {
		got, err := scope.ParseInclusionPrefix(tc.in, min)
		if err != nil {
			t.Errorf("ParseInclusionPrefix(%q) error: %v", tc.in, err)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("ParseInclusionPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestInclusionPrefixRefuses(t *testing.T) {
	min := scope.DefaultPrefixMin()
	tooShort := []string{"0.0.0.0/0", "198.51.0.0/16", "203.0.112.0/23", "2001:db8::/32", "2001:db8::/47", "::/0", "::ffff:198.51.0.0/112", "::ffff:0:0/96"}
	for _, in := range tooShort {
		got, err := scope.ParseInclusionPrefix(in, min)
		if !errors.Is(err, scope.ErrPrefixTooShort) {
			t.Errorf("ParseInclusionPrefix(%q) = %v, %v, want ErrPrefixTooShort", in, got, err)
		}
	}
	ambiguous := []struct{ name, in string }{
		{"host bits set", "203.0.113.5/24"},
		{"host bits set IPv6", "2001:db8:1::1/48"},
		{"shorthand", "10/8"},
		{"three octets", "10.0.0/24"},
		{"length over 32", "1.2.3.0/33"},
		{"length over 128", "2001:db8::/129"},
		{"leading zero in length", "203.0.113.0/024"},
		{"negative length", "203.0.113.0/-1"},
		{"empty length", "203.0.113.0/"},
		{"no length", "203.0.113.0"},
		{"only a length", "/24"},
		{"space before slash", "203.0.113.0 /24"},
		{"space after slash", "203.0.113.0/ 24"},
		{"second slash", "203.0.113.0/24/24"},
		{"empty", ""},
		{"zone", "fe80::%eth0/64"},
		{"netmask", "203.0.113.0/255.255.255.0"},
		{"hostname", "example.com/24"},
		{"mapped prefix with host bits (/80)", "::ffff:0:0/80"},
		{"mapped prefix with host bits (/95)", "::ffff:0:0/95"},
		{"NAT64 range", "64:ff9b::/96"},
		{"inside NAT64", "64:ff9b::cb00:0/120"},
		{"NAT64 local-use", "64:ff9b:1::/48"},
		{"6to4", "2002::/48"},
		{"inside 6to4", "2002:cb00:7100::/48"},
		{"Teredo", "2001::/48"},
		{"::/96 range", "::/96"},
		{"inside ::/96", "::7f00:0/120"},
		{"the loopback prefix", "::1/128"},
	}
	for _, tc := range ambiguous {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scope.ParseInclusionPrefix(tc.in, min)
			if !errors.Is(err, scope.ErrAmbiguous) {
				t.Fatalf("ParseInclusionPrefix(%q) = %v, %v, want ErrAmbiguous", tc.in, got, err)
			}
			if got.IsValid() {
				t.Errorf("returned a valid prefix %v with an error", got)
			}
		})
	}
	// Shorthand is a syntax refusal in both modes.
	if _, err := scope.ParseExclusionPrefix("10/8"); !errors.Is(err, scope.ErrAmbiguous) {
		t.Errorf("ParseExclusionPrefix(10/8) = %v, want ErrAmbiguous", err)
	}
}

// A mapped address with a prefix length below 96 always has host bits set,
// because the ffff group sits at bits 80 to 95. The refusal therefore comes
// from the host-bits check and no separate "wider than the mapped range"
// branch is needed. This test proves it for every such length.
func TestMappedPrefixesShorterThan96AreAlwaysRefused(t *testing.T) {
	for _, base := range []string{"::ffff:0:0", "::ffff:203.0.113.0", "::ffff:ffff:ffff", "::ffff:0:1"} {
		for bits := 0; bits < 96; bits++ {
			in := base + "/" + itoa(bits)
			if p, err := scope.ParseExclusionPrefix(in); !errors.Is(err, scope.ErrAmbiguous) {
				t.Fatalf("ParseExclusionPrefix(%q) = %v, %v, want ErrAmbiguous", in, p, err)
			}
			if p, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); err == nil {
				t.Fatalf("ParseInclusionPrefix(%q) = %v, want an error", in, p)
			}
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// Choice 1 of record 0002: an exclusion has no minimum and may be broad. It
// must still be a canonical prefix.
func TestExclusionPrefixAccepts(t *testing.T) {
	tests := []struct{ in, want string }{
		{"0.0.0.0/0", "0.0.0.0/0"},
		{"198.51.0.0/16", "198.51.0.0/16"},
		{"203.0.113.0/24", "203.0.113.0/24"},
		{"2001:db8::/32", "2001:db8::/32"},
		{"::/0", "::/0"},
		{"2001::/16", "2001::/16"}, // contains the Teredo range, so it only denies more
		{"::/1", "::/1"},           // contains every embedded range
		{"::/95", "::/95"},         // contains ::/96
		{"64:ff9b::/95", "64:ff9b::/95"},
		{"::ffff:0:0/96", "0.0.0.0/0"}, // the mapped range is the whole IPv4 space
		{"::ffff:198.51.0.0/112", "198.51.0.0/16"},
	}
	for _, tc := range tests {
		got, err := scope.ParseExclusionPrefix(tc.in)
		if err != nil {
			t.Errorf("ParseExclusionPrefix(%q) error: %v", tc.in, err)
			continue
		}
		if got.String() != tc.want || got != got.Masked() {
			t.Errorf("ParseExclusionPrefix(%q) = %v, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExclusionPrefixRefuses(t *testing.T) {
	for _, in := range []string{
		"203.0.113.5/24", "10/8", "2001:db8::1/32", "1.2.3.0/33", "", "/", "203.0.113.0",
		"64:ff9b::/96", "2002::/16", "2001::/32", "::/96", "::/128", "::1/128", "::2/128", "::/112",
		"2002::/48", "64:ff9b:1::/48", "::7f00:0/120",
	} {
		if p, err := scope.ParseExclusionPrefix(in); !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("ParseExclusionPrefix(%q) = %v, %v, want ErrAmbiguous", in, p, err)
		}
	}
}

// A zero minimum must never mean "no minimum" (F-3 of the M1 review). It is
// the only unusable value that can be built from outside the package. Exclusions
// have their own entry point. The tests of NewPrefixMin in prefixmin_test.go
// cover every other broken value.
func TestInclusionPrefixNeedsASetMinimum(t *testing.T) {
	var unset scope.PrefixMin
	inputs := []string{"203.0.113.0/24", "2001:db8:1::/48", "0.0.0.0/0", "::/0", "203.0.113.7/32", "::ffff:203.0.113.0/120"}
	for _, in := range inputs {
		p, err := scope.ParseInclusionPrefix(in, unset)
		if !errors.Is(err, scope.ErrMinimumUnset) || p.IsValid() {
			t.Errorf("ParseInclusionPrefix(%q, unset) = %v, %v, want ErrMinimumUnset", in, p, err)
		}
	}
	// Every minimum at or above the floor is accepted.
	for _, args := range [][2]int{{24, 48}, {28, 56}, {32, 128}, {24, 56}, {28, 48}} {
		m, err := scope.NewPrefixMin(args[0], args[1])
		if err != nil {
			t.Fatalf("NewPrefixMin(%d, %d): %v", args[0], args[1], err)
		}
		if _, err := scope.ParseInclusionPrefix("203.0.113.7/32", m); err != nil {
			t.Errorf("minimum %d/%d refused: %v", args[0], args[1], err)
		}
	}
}

func TestInclusionPrefixAppliesEachFamilyMinimum(t *testing.T) {
	min, _ := scope.NewPrefixMin(28, 56)
	for _, in := range []string{"203.0.113.0/28", "2001:db8:0:100::/56"} {
		if _, err := scope.ParseInclusionPrefix(in, min); err != nil {
			t.Errorf("ParseInclusionPrefix(%q) error: %v", in, err)
		}
	}
	for _, in := range []string{"203.0.113.0/27", "2001:db8:0:200::/55"} {
		if _, err := scope.ParseInclusionPrefix(in, min); !errors.Is(err, scope.ErrPrefixTooShort) {
			t.Errorf("ParseInclusionPrefix(%q) = %v, want ErrPrefixTooShort", in, err)
		}
	}
}

// F-5 of the M1 review: an inclusion that overlaps an embedded-IPv4 range, in
// either direction, is refused. A prefix that contains a range would otherwise
// look as if it covered NAT64, 6to4 or Teredo, which are never canonical
// targets. An exclusion may contain a range because it only denies more.
func TestInclusionOverlappingAnEmbeddedRangeIsRefused(t *testing.T) {
	// Reachable with the default minimum, so through the public entry point.
	for _, in := range []string{
		// inside or equal
		"::/96", "::/120", "::1/128", "64:ff9b::/96", "64:ff9b::/112", "64:ff9b:1::/48", "64:ff9b:1::/64",
		"2002:cb00:7100::/48", "2001::/48", "2001:0:1::/48",
		// containing a range
		"::/48", "::/64", "::/80", "64:ff9b::/48", "64:ff9b::/64",
	} {
		if p, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); err == nil {
			t.Errorf("inclusion %q was accepted as %v. It overlaps an embedded range", in, p)
		} else if !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("inclusion %q: %v, want ErrAmbiguous", in, err)
		}
	}
	// Shorter prefixes stop at the floor. The overlap check still has to hold
	// on its own, so these go through the unfloored entry point of the tests.
	for _, in := range []string{
		"2002::/16", "2001::/32", "::/0", "::/1", "::/47", "2000::/3", "2000::/4", "2001::/16", "64:ff9b::/32", "64::/16",
	} {
		if p, err := scope.ParseInclusionUnfloored(in, 0, 0); !errors.Is(err, scope.ErrAmbiguous) {
			t.Errorf("unfloored inclusion %q = %v, %v, want ErrAmbiguous", in, p, err)
		}
		if p, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); err == nil {
			t.Errorf("inclusion %q was accepted as %v", in, p)
		}
	}
	// Prefixes that merely sit next to a range are fine.
	for _, in := range []string{"2001:db8::/48", "2003::/48", "2001:1::/48", "2001:ffff::/48", "64:ff9a::/48", "64:ff9b:2::/48", "2600::/48", "::1:0:0/112"} {
		if _, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); err != nil {
			t.Errorf("inclusion %q next to a range was refused: %v", in, err)
		}
	}
}

// ::1 is inside ::/96 and is refused everywhere. No inclusion that contains
// it is valid either, so a broad prefix cannot authorize the loopback.
func TestLoopbackIPv6IsRefusedEverywhere(t *testing.T) {
	if a, err := scope.ParseAddr("::1"); err == nil {
		t.Errorf("ParseAddr(::1) = %v, want ErrAmbiguous", a)
	}
	if p, err := scope.ParseExclusionPrefix("::1/128"); err == nil {
		t.Errorf("ParseExclusionPrefix(::1/128) = %v, want ErrAmbiguous", p)
	}
	for bits := 0; bits <= 128; bits++ {
		in := "::/" + itoa(bits)
		if bits == 0 {
			continue // ::/0 is a valid exclusion and refused as an inclusion below
		}
		if p, err := scope.ParseInclusionPrefix(in, scope.DefaultPrefixMin()); err == nil {
			t.Errorf("inclusion %q = %v contains ::1 and was accepted", in, p)
		}
		if p, err := scope.ParseInclusionUnfloored(in, 0, 0); err == nil {
			t.Errorf("unfloored inclusion %q = %v contains ::1 and was accepted", in, p)
		}
	}
	if p, err := scope.ParseInclusionUnfloored("::/0", 0, 0); err == nil {
		t.Errorf("unfloored inclusion ::/0 = %v contains ::1 and was accepted", p)
	}
	// ::/0 stays a valid exclusion: it denies everything in IPv6.
	if _, err := scope.ParseExclusionPrefix("::/0"); err != nil {
		t.Errorf("ParseExclusionPrefix(::/0): %v", err)
	}
}
