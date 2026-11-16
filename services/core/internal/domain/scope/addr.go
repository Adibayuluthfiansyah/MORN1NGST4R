package scope

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
)

// ErrPrefixTooShort means that a prefix is shorter than the configured
// minimum (D-38).
var ErrPrefixTooShort = errors.New("prefix shorter than the minimum")

// Longest possible inputs. Each is the longest text that any valid input can
// have, so a longer input is invalid whatever it contains and is refused before
// anything is allocated for it (review finding F-9). They are not new policy.
const (
	// An IPv6 literal with a dotted tail is the longest address form:
	// 0000:0000:0000:0000:0000:ffff:255.255.255.255. A zone would make it
	// longer, and zones are refused.
	maxAddrInput = 45
	// An address, "/" and up to three digits of prefix length.
	maxPrefixInput = maxAddrInput + len("/128")
)

// The floor of the in-scope CIDR minimum. No caller can build a PrefixMin that
// is shorter: a policy may tighten the minimum and never lower it. The values
// are D-38. Any change needs an owner decision and a decision record, not a
// configuration value.
const (
	MinPrefixLenV4 = 24
	MinPrefixLenV6 = 48
)

// ErrInvalidMinimum means that NewPrefixMin was given a length below the floor
// or above the length of its address family. That covers zero, negative and
// half-set pairs.
var ErrInvalidMinimum = errors.New("prefix minimum below the floor (/24 IPv4, /48 IPv6) or above the address length")

// PrefixMin holds the minimum prefix lengths of in-scope CIDR rules. Its fields
// are unexported, so the only way to build a usable value is DefaultPrefixMin
// or NewPrefixMin, and both enforce the floor. The caller passes the value in.
// It is never a package variable (D-38).
//
// The zero value means "not set", never "no minimum". An inclusion refuses it
// with ErrMinimumUnset. Exclusions have no minimum and use ParseExclusionPrefix,
// which takes none (choice 1 of record 0002).
type PrefixMin struct {
	v4 int
	v6 int
}

// DefaultPrefixMin returns the floor itself: /24 for IPv4 and /48 for IPv6.
func DefaultPrefixMin() PrefixMin { return PrefixMin{v4: MinPrefixLenV4, v6: MinPrefixLenV6} }

// NewPrefixMin returns an explicit minimum. Each length must be at least the
// floor of its family and at most the address length (32 and 128), so a policy
// can only tighten the default. Anything else returns ErrInvalidMinimum and an
// unset value.
func NewPrefixMin(v4, v6 int) (PrefixMin, error) {
	m := PrefixMin{v4: v4, v6: v6}
	if !m.IsSet() {
		return PrefixMin{}, ErrInvalidMinimum
	}
	return m, nil
}

// V4 returns the minimum IPv4 prefix length, or 0 when the value is unset.
func (m PrefixMin) V4() int { return m.v4 }

// V6 returns the minimum IPv6 prefix length, or 0 when the value is unset.
func (m PrefixMin) V6() int { return m.v6 }

// IsSet reports whether both lengths are within their family and not below the
// floor. The zero value is not set. The check repeats the rule of NewPrefixMin
// on purpose, so a value that bypasses the constructor inside the package is
// still refused.
func (m PrefixMin) IsSet() bool {
	return m.v4 >= MinPrefixLenV4 && m.v4 <= 32 && m.v6 >= MinPrefixLenV6 && m.v6 <= 128
}

// embeddedRanges are the IPv6 ranges in which an IPv4 address is embedded at a
// position that depends on the prefix (S1.1 step 7). A literal inside one is
// ambiguous, and so is the whole of ::/96, the IPv6 loopback ::1 included (owner
// decision: there is no exception). IPv4-mapped ::ffff:0:0/96 is not here: it
// is unmapped instead.
//
// The IPv4-translated range ::ffff:0:0:0/96 (SIIT) is here by decision D-88
// (record 0005). RFC 2765 defined it. RFC 6145 made RFC 2765 obsolete, and the
// IANA IPv6 special-purpose registry no longer lists the range. It is refused as
// a precaution. No other range may be added without a decision.
var embeddedRanges = []netip.Prefix{
	netip.MustParsePrefix("::/96"),           // IPv4-compatible (deprecated)
	netip.MustParsePrefix("::ffff:0:0:0/96"), // IPv4-translated, SIIT (RFC 2765, obsolete)
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"), // 6to4
	netip.MustParsePrefix("2001::/32"), // Teredo
}

// inEmbeddedRange reports whether a falls inside an embedded-IPv4 range.
func inEmbeddedRange(a netip.Addr) bool {
	for _, r := range embeddedRanges {
		if r.Contains(a) {
			return true
		}
	}
	return false
}

// ParseAddr returns the canonical form of an IP address literal, or an error
// that wraps ErrAmbiguous. IPv4 must have exactly four decimal parts without
// leading zeros. A zone, brackets, a port and a prefix length are refused.
// IPv4-mapped IPv6 is returned as its IPv4 address. Every address of ::/96
// (the loopback ::1 and the unspecified :: included), SIIT
// (::ffff:0:0:0/96), NAT64, 6to4 and Teredo
// literals are ambiguous.
//
// Special-purpose addresses such as 0.0.0.0, 127.0.0.1 or 169.254.169.254 are
// valid here. Denying them is the job of the egress tables (M9, D-39, D-72) and
// of the lab rules (M17), not of this package.
func ParseAddr(s string) (netip.Addr, error) {
	if len(s) > maxAddrInput {
		return netip.Addr{}, ambiguous("longer than any address literal")
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, ambiguous("not a canonical address literal")
	}
	if a.Zone() != "" {
		return netip.Addr{}, ambiguous("address with a zone")
	}
	if a.Is4In6() {
		return a.Unmap(), nil
	}
	if a.Is6() && inEmbeddedRange(a) {
		return netip.Addr{}, ambiguous("address with an embedded IPv4 form")
	}
	return a, nil
}

// ParseInclusionPrefix returns the canonical form of an in-scope CIDR prefix.
// min must be fully set (ErrMinimumUnset otherwise) and a shorter prefix gives
// ErrPrefixTooShort. It refuses host bits set, shorthand such as "10/8", a
// zone, a length with a leading zero, and any prefix that overlaps an
// embedded-IPv4 range in either direction. A prefix that contains a range would
// look as if it covered SIIT, NAT64, 6to4 or Teredo, which are never targets. An
// IPv4-mapped prefix is returned as an IPv4 prefix.
func ParseInclusionPrefix(s string, min PrefixMin) (netip.Prefix, error) {
	if !min.IsSet() {
		return netip.Prefix{}, ErrMinimumUnset
	}
	return parseInclusion(s, min.v4, min.v6)
}

// parseInclusion applies the given lengths without judging them. Callers pass
// the lengths of a set PrefixMin. Tests call it with lowered lengths to keep the
// overlap check covered on its own.
func parseInclusion(s string, v4, v6 int) (netip.Prefix, error) {
	p, err := parsePrefix(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	if p.Addr().Is4() && p.Bits() < v4 || p.Addr().Is6() && p.Bits() < v6 {
		return netip.Prefix{}, ErrPrefixTooShort
	}
	if p.Addr().Is6() {
		for _, r := range embeddedRanges {
			if p.Overlaps(r) {
				return netip.Prefix{}, ambiguous("prefix overlaps an embedded-IPv4 range")
			}
		}
	}
	return p, nil
}

// ParseExclusionPrefix returns the canonical form of an out-of-scope CIDR
// prefix. An exclusion has no minimum: "0.0.0.0/0" and "::/0" are valid, each
// for its own address family only (choice 1 of record 0002). It refuses the
// same syntax errors as an inclusion. It refuses a prefix that is equal to or
// inside an embedded-IPv4 range. A wider prefix that contains a range is
// accepted, because an exclusion only denies more.
func ParseExclusionPrefix(s string) (netip.Prefix, error) {
	p, err := parsePrefix(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	if p.Addr().Is6() {
		for _, r := range embeddedRanges {
			if r.Contains(p.Addr()) && p.Bits() >= r.Bits() {
				return netip.Prefix{}, ambiguous("prefix inside an embedded-IPv4 range")
			}
		}
	}
	return p, nil
}

// parsePrefix reads the syntax that inclusions and exclusions share.
func parsePrefix(s string) (netip.Prefix, error) {
	if len(s) > maxPrefixInput {
		return netip.Prefix{}, ambiguous("longer than any prefix")
	}
	if strings.Count(s, "/") != 1 {
		return netip.Prefix{}, ambiguous("not an address and a prefix length")
	}
	addrPart, bitsPart, _ := strings.Cut(s, "/")
	if len(bitsPart) == 0 || len(bitsPart) > 3 || !isAllDigits(bitsPart) || (len(bitsPart) > 1 && bitsPart[0] == '0') {
		return netip.Prefix{}, ambiguous("invalid prefix length")
	}
	bits, _ := strconv.Atoi(bitsPart)

	a, err := netip.ParseAddr(addrPart)
	if err != nil {
		return netip.Prefix{}, ambiguous("not a canonical address literal")
	}
	if a.Zone() != "" {
		return netip.Prefix{}, ambiguous("address with a zone")
	}
	if bits > a.BitLen() {
		return netip.Prefix{}, ambiguous("prefix length too large")
	}
	p := netip.PrefixFrom(a, bits)
	if p.Masked() != p {
		return netip.Prefix{}, ambiguous("host bits set")
	}

	if a.Is4In6() {
		// A mapped prefix is its IPv4 prefix. Its length is at least 96: the
		// ffff group sits at bits 80 to 95, so a shorter length would leave
		// host bits set and was refused above. The test
		// TestMappedPrefixesShorterThan96AreAlwaysRefused proves it.
		return netip.PrefixFrom(a.Unmap(), bits-96), nil
	}
	return p, nil
}
