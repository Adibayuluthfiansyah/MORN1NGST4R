package scope_test

import (
	"fmt"
	"math/rand/v2"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// All randomness is seeded, so a failure reproduces.
func newRand(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) }

// The type spec is declared in evaluate_test.go: the text of a rule as a person
// writes it. The reference matcher below reads this text. It never reads the
// parsed Rule, so it shares no parsing and no matching code with the package.

func (s spec) build() (scope.Rule, bool) {
	r, err := scope.NewRule(s.kind, s.exclude, s.value, s.ports, scope.DefaultPrefixMin())
	return r, err == nil
}

var (
	// Values are written in non-canonical ways on purpose (case, trailing dot,
	// mapped form) so that the reference has to canonicalize on its own.
	poolDomains = []string{
		"example.com", "Example.COM.", "a.example.com", "A.Example.com", "b.a.example.com", "x.example.com", "a.x.example.com",
		"a.b.x.example.com", "example.org", "evil-example.com", "example.com.evil.net", "co.uk", "shop.co.uk", "localhost", "a-b.example.net",
	}
	poolWildcards = []string{
		"*.example.com", "*.Example.COM.", "*.x.example.com", "*.a.example.com", "*.example.org", "*.co.uk", "*.com", "*.net",
	}
	poolIPs = []string{
		"203.0.113.0", "203.0.113.7", "203.0.113.8", "203.0.113.255", "203.0.114.0", "198.51.100.9", "198.51.0.1", "127.0.0.1",
		"10.0.0.1", "::ffff:203.0.113.7", "2001:db8::7", "2001:DB8:1::7", "2001:db8:2::1", "2001:db8:1:ffff::1",
	}
	poolCIDRs = []string{
		"203.0.113.0/24", "203.0.113.0/25", "203.0.113.128/25", "198.51.100.0/24", "198.51.0.0/16", "10.0.0.0/8", "0.0.0.0/0",
		"2001:db8:1::/48", "2001:db8::/32", "::/0", "203.0.113.7/32", "::ffff:203.0.113.0/120", "::ffff:0:0/96",
	}
	poolPorts = []string{"", "", "", "22", "443", "8443", "80,443", "22,8443", "1,65535", "1-65535"}

	targetHosts = []string{
		"example.com", "a.example.com", "b.a.example.com", "x.example.com", "a.x.example.com", "a.b.x.example.com", "example.org",
		"evil-example.com", "example.com.evil.net", "co.uk", "shop.co.uk", "localhost", "a-b.example.net", "other.test",
	}
	targetAddrs = []string{
		"203.0.113.0", "203.0.113.7", "203.0.113.8", "203.0.113.255", "203.0.114.0", "198.51.100.9", "198.51.0.1", "127.0.0.1",
		"10.0.0.1", "8.8.8.8", "2001:db8::7", "2001:db8:1::7", "2001:db8:2::1", "2001:db8:1:ffff::1", "2606:4700::1",
	}
	targetPorts = []uint16{1, 22, 80, 443, 8080, 8443, 65535}
)

func randomSpec(r *rand.Rand, exclude bool) spec {
	choose := func(p []string) string { return p[r.IntN(len(p))] }
	s := spec{exclude: exclude, ports: choose(poolPorts)}
	switch r.IntN(4) {
	case 0:
		s.kind, s.value = scope.KindDomain, choose(poolDomains)
	case 1:
		s.kind, s.value = scope.KindWildcard, choose(poolWildcards)
	case 2:
		s.kind, s.value = scope.KindIP, choose(poolIPs)
	default:
		s.kind, s.value = scope.KindCIDR, choose(poolCIDRs)
	}
	return s
}

// randomSpecs returns n specs that NewRule accepts, and the rules built from them.
func randomSpecs(r *rand.Rand, n int, exclude bool) ([]spec, scope.RuleSet) {
	var specs []spec
	var rs scope.RuleSet
	for len(rs) < n {
		s := randomSpec(r, exclude)
		if rule, ok := s.build(); ok {
			specs = append(specs, s)
			rs = append(rs, rule)
		}
	}
	return specs, rs
}

type pick struct {
	host string
	addr netip.Addr
	port uint16
}

func (p pick) target() scope.Target {
	return scope.Target{Host: scope.Host(p.host), Addr: p.addr, Port: p.port}
}

// randomPickFrom returns a target near one of the specs most of the time: the
// host or address that the rule text names, or one inside its range, with a
// port that is often 80 or 443. Without this the generator hits few matches.
func randomPickFrom(r *rand.Rand, specs []spec) pick {
	if len(specs) == 0 || r.IntN(10) < 3 {
		return randomPick(r)
	}
	s := specs[r.IntN(len(specs))]
	port := []uint16{80, 443, 443, 8443, 22}[r.IntN(5)]
	switch s.kind {
	case scope.KindDomain:
		return pick{host: strings.ToLower(strings.TrimSuffix(s.value, ".")), port: port}
	case scope.KindWildcard:
		base := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(s.value, "*."), "."))
		if r.IntN(4) == 0 {
			return pick{host: base, port: port} // the apex is never matched
		}
		return pick{host: []string{"www", "a.b", "x"}[r.IntN(3)] + "." + base, port: port}
	case scope.KindIP:
		if a, err := netip.ParseAddr(s.value); err == nil {
			return pick{addr: a.Unmap(), port: port}
		}
	case scope.KindCIDR:
		if p, err := netip.ParsePrefix(s.value); err == nil {
			a := p.Masked().Addr().Unmap()
			if r.IntN(2) == 0 && a.Is4() {
				b := a.As4()
				b[3] |= byte(r.IntN(4)) // a few addresses above the base, inside narrow prefixes only sometimes
				a = netip.AddrFrom4(b)
			}
			return pick{addr: a, port: port}
		}
	}
	return randomPick(r)
}

func randomPick(r *rand.Rand) pick {
	port := targetPorts[r.IntN(len(targetPorts))]
	if r.IntN(2) == 0 {
		return pick{host: targetHosts[r.IntN(len(targetHosts))], port: port}
	}
	return pick{addr: netip.MustParseAddr(targetAddrs[r.IntN(len(targetAddrs))]), port: port}
}

// ---- the reference matcher, written from the text of S1.1 and nothing else ----

func refPortMatch(s spec, port int) bool {
	switch {
	case port < 1 || port > 65535:
		return false
	case s.ports == "1-65535" || (s.ports == "" && s.exclude):
		return true
	case s.ports == "":
		return port == 80 || port == 443
	}
	for _, f := range strings.Split(s.ports, ",") {
		if n, _ := strconv.Atoi(f); n == port {
			return true
		}
	}
	return false
}

func refLabels(host string) []string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return strings.Split(host, ".")
}

// refAddrBits returns the 16 bytes of an address in the IPv6 form and the offset
// of the prefix length for it: an IPv4 address is read as ::ffff:a.b.c.d.
func refAddrBits(a netip.Addr) [16]byte { return a.Unmap().As16() }

func refCIDRMatch(value string, t netip.Addr) bool {
	p, err := netip.ParsePrefix(value)
	if err != nil {
		return false
	}
	base, bits := p.Addr(), p.Bits()
	if base.Is4In6() {
		base, bits = base.Unmap(), bits-96
	}
	if base.Is4() != t.Is4() {
		return false // a prefix covers only its own address family
	}
	b, a := refAddrBits(base), refAddrBits(t)
	n := bits
	if base.Is4() {
		n = bits + 96 // both sides are in the ::ffff:0:0/96 form
	}
	for i := 0; i < n; i++ {
		if (b[i/8]>>(7-uint(i%8)))&1 != (a[i/8]>>(7-uint(i%8)))&1 {
			return false
		}
	}
	return true
}

func refMatch(s spec, p pick) bool {
	if !refPortMatch(s, int(p.port)) {
		return false
	}
	switch s.kind {
	case scope.KindDomain:
		if p.host == "" {
			return false
		}
		return strings.Join(refLabels(s.value), ".") == p.host
	case scope.KindWildcard:
		if p.host == "" {
			return false
		}
		base := refLabels(strings.TrimPrefix(s.value, "*."))
		got := refLabels(p.host)
		if len(got) <= len(base) { // never the base itself, and only below it
			return false
		}
		for i := 1; i <= len(base); i++ {
			if got[len(got)-i] != base[len(base)-i] {
				return false
			}
		}
		return true
	case scope.KindIP:
		if !p.addr.IsValid() {
			return false
		}
		want, err := netip.ParseAddr(s.value)
		return err == nil && want.Unmap() == p.addr
	case scope.KindCIDR:
		return p.addr.IsValid() && refCIDRMatch(s.value, p.addr)
	}
	return false
}

func refAllowed(specs []spec, p pick) (allowed bool) {
	included := false
	for _, s := range specs {
		if !refMatch(s, p) {
			continue
		}
		if s.exclude {
			return false
		}
		included = true
	}
	return included
}

// The decision equals the reference decision.
func TestEvaluateAgreesWithAnIndependentReference(t *testing.T) {
	r := newRand(5)
	allowed, denied := 0, 0
	for i := 0; i < 6000; i++ {
		inc, incRules := randomSpecs(r, 1+r.IntN(5), false)
		exc, excRules := randomSpecs(r, r.IntN(3), true)
		specs, rs := append(inc, exc...), append(incRules, excRules...)
		p := randomPickFrom(r, specs)
		d := rs.Evaluate(p.target())
		if want := refAllowed(specs, p); d.Allowed != want {
			t.Fatalf("iteration %d: target %+v decided %+v, reference says allowed=%v\nrules: %+v", i, p, d, want, specs)
		}
		if d.Allowed {
			allowed++
		} else {
			denied++
		}
	}
	if allowed < 600 || denied < 600 {
		t.Fatalf("the generator is too weak: %d allowed, %d denied", allowed, denied)
	}
}

// Adding an exclusion never turns a denial into an allow (G-scope).
func TestAddingExclusionNeverAllows(t *testing.T) {
	r := newRand(1)
	for i := 0; i < 5000; i++ {
		_, base := randomSpecs(r, r.IntN(7), r.IntN(3) == 0)
		_, extra := randomSpecs(r, 1, true)
		p := randomPick(r)
		before := base.Evaluate(p.target())
		after := append(append(scope.RuleSet{}, base...), extra...).Evaluate(p.target())
		if !before.Allowed && after.Allowed {
			t.Fatalf("iteration %d: adding an exclusion allowed %+v\nbefore: %+v\nafter:  %+v", i, p, before, after)
		}
	}
}

// Once an exclusion matches, no inclusion added later can allow the target.
func TestAddingInclusionNeverOverridesAnExclusion(t *testing.T) {
	r := newRand(2)
	checked := 0
	for i := 0; i < 6000; i++ {
		baseSpecs, base := randomSpecs(r, 1+r.IntN(5), false)
		exclSpecs, excl := randomSpecs(r, 1, true)
		withExcl := append(append(scope.RuleSet{}, base...), excl...)
		p := randomPickFrom(r, append(append([]spec{}, baseSpecs...), exclSpecs...))
		if d := withExcl.Evaluate(p.target()); d.Reason != scope.ReasonExcluded {
			continue
		}
		checked++
		_, more := randomSpecs(r, 1+r.IntN(4), false)
		if got := append(append(scope.RuleSet{}, withExcl...), more...).Evaluate(p.target()); got.Allowed || got.Reason != scope.ReasonExcluded {
			t.Fatalf("iteration %d: an excluded target %+v became %+v after adding inclusions", i, p, got)
		}
	}
	if checked < 150 {
		t.Fatalf("only %d excluded cases were generated, the generator is too weak", checked)
	}
}

// The decision does not depend on the order of the rules.
func TestEvaluateIsIndependentOfRuleOrder(t *testing.T) {
	r := newRand(3)
	for i := 0; i < 4000; i++ {
		_, inc := randomSpecs(r, 1+r.IntN(4), false)
		_, exc := randomSpecs(r, r.IntN(3), true)
		rs := append(inc, exc...)
		p := randomPick(r)
		want := rs.Evaluate(p.target())
		shuffled := append(scope.RuleSet{}, rs...)
		r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		if got := shuffled.Evaluate(p.target()); got != want {
			t.Fatalf("iteration %d: order changed the decision for %+v: %+v vs %+v", i, p, got, want)
		}
	}
}

// With no inclusion at all, nothing is allowed. Default deny.
func TestWithoutInclusionsEverythingIsDenied(t *testing.T) {
	r := newRand(4)
	for i := 0; i < 3000; i++ {
		_, rs := randomSpecs(r, r.IntN(6), true)
		if d := rs.Evaluate(randomPick(r).target()); d.Allowed {
			t.Fatalf("iteration %d: %+v allowed with exclusions only", i, d)
		}
	}
}

// ---- normalization: structured random input that reaches every parser ----

func randomIPv4(r *rand.Rand) string {
	return fmt.Sprintf("%d.%d.%d.%d", r.IntN(256), r.IntN(256), r.IntN(256), r.IntN(256))
}

func randomIPv6(r *rand.Rand) string {
	hot := []string{"64:ff9b", "64:ff9b:1", "2002", "2001:0", "2001:db8", "0", "ffff"}
	var groups []string
	if r.IntN(3) == 0 {
		groups = strings.Split(hot[r.IntN(len(hot))], ":")
	}
	for len(groups) < 8 {
		g := strconv.FormatUint(uint64(r.IntN(0x10000)), 16)
		if r.IntN(4) == 0 {
			g = "0"
		}
		if r.IntN(3) == 0 {
			g = strings.ToUpper(g)
		}
		if r.IntN(6) == 0 {
			g = strings.Repeat("0", r.IntN(3)) + g
			if len(g) > 4 {
				g = g[len(g)-4:]
			}
		}
		groups = append(groups, g)
	}
	switch r.IntN(4) {
	case 0: // compress a run
		i := r.IntN(7)
		j := i + 1 + r.IntN(8-i-1)
		return strings.Join(groups[:i], ":") + "::" + strings.Join(groups[j:], ":")
	case 1: // mapped
		return "::ffff:" + randomIPv4(r)
	case 2: // dotted tail
		return strings.Join(groups[:6], ":") + ":" + randomIPv4(r)
	}
	return strings.Join(groups, ":")
}

func randomAddrText(r *rand.Rand) string {
	var s string
	if r.IntN(2) == 0 {
		s = randomIPv4(r)
	} else {
		s = randomIPv6(r)
	}
	switch r.IntN(12) {
	case 0:
		s += "%eth0"
	case 1:
		s = "[" + s + "]"
	case 2:
		s += "."
	case 3:
		s = " " + s
	}
	return s
}

func randomPrefixText(r *rand.Rand) string {
	a := netip.AddrFrom16(netip.MustParseAddr("::").As16())
	if r.IntN(2) == 0 {
		a = netip.MustParseAddr(randomIPv4(r))
	} else if v, err := netip.ParseAddr(randomIPv6(r)); err == nil {
		a = v
	}
	bits := r.IntN(a.BitLen() + 1)
	p := netip.PrefixFrom(a, bits)
	if r.IntN(4) != 0 {
		p = p.Masked() // mostly valid, sometimes with host bits
	}
	return p.String()
}

func TestNormalizationIsIdempotentAndReachesEveryParser(t *testing.T) {
	r := newRand(6)
	alphabet := []byte("abcXYZ019-._ :@/*%éé\x00[]")
	randomString := func() string {
		n := r.IntN(24)
		b := make([]byte, 0, n)
		for i := 0; i < n; i++ {
			b = append(b, alphabet[r.IntN(len(alphabet))])
		}
		return string(b)
	}
	counts := map[string]int{}
	min := scope.DefaultPrefixMin()
	for i := 0; i < 30000; i++ {
		s := randomString()
		if h, err := scope.ParseHost(s); err == nil {
			counts["host"]++
			if again, err := scope.ParseHost(string(h)); err != nil || again != h {
				t.Fatalf("ParseHost not idempotent for %q: %q -> %q, %v", s, h, again, err)
			}
		}
		if h, err := scope.ParseHost(targetHosts[r.IntN(len(targetHosts))] + "."); err == nil {
			counts["host from a pool"]++
			_ = h
		}

		a := randomAddrText(r)
		if v, err := scope.ParseAddr(a); err == nil {
			counts["addr"]++
			if again, err := scope.ParseAddr(v.String()); err != nil || again != v {
				t.Fatalf("ParseAddr not idempotent for %q: %v -> %v, %v", a, v, again, err)
			}
			if tg, err := scope.ParseTarget(a, 443); err != nil || tg.Addr != v {
				t.Fatalf("ParseTarget(%q) = %+v, %v, want the address %v", a, tg, err, v)
			}
		}

		p := randomPrefixText(r)
		if v, err := scope.ParseInclusionPrefix(p, min); err == nil {
			counts["inclusion prefix"]++
			if again, err := scope.ParseInclusionPrefix(v.String(), min); err != nil || again != v {
				t.Fatalf("ParseInclusionPrefix not idempotent for %q: %v -> %v, %v", p, v, again, err)
			}
		}
		if v, err := scope.ParseExclusionPrefix(p); err == nil {
			counts["exclusion prefix"]++
			if again, err := scope.ParseExclusionPrefix(v.String()); err != nil || again != v {
				t.Fatalf("ParseExclusionPrefix not idempotent for %q: %v -> %v, %v", p, v, again, err)
			}
		}
	}
	// If a parser is never reached with a valid input the properties above
	// prove nothing about it. These floors were chosen from the seeded run.
	for name, floor := range map[string]int{"host": 100, "host from a pool": 29000, "addr": 3000, "inclusion prefix": 500, "exclusion prefix": 1500} {
		if counts[name] < floor {
			t.Errorf("%s parser reached %d valid inputs, want at least %d (counts: %v)", name, counts[name], floor, counts)
		}
	}
}
