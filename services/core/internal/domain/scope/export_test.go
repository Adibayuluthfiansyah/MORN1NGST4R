package scope

import "net/netip"

// ParseInclusionUnfloored runs the inclusion parser with lengths that the
// public API refuses (the floor of PrefixMin). It exists only in tests, so the
// overlap check with embedded ranges keeps its own coverage for prefixes that
// the floor would stop first.
func ParseInclusionUnfloored(s string, v4, v6 int) (netip.Prefix, error) {
	return parseInclusion(s, v4, v6)
}
