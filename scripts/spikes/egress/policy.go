// Package egress is a throw-away feasibility spike. It is NOT the product
// policy engine. It contains a tiny address/scope evaluator, a mock DNS
// server, local fixtures, a first-party HTTP client and a policy proxy.
package egress

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// DefaultPorts are the in-scope ports of a rule without a port.
var DefaultPorts = []int{80, 443}

var (
	metadata4 = netip.MustParseAddr("169.254.169.254")
	metadata6 = netip.MustParseAddr("fd00:ec2::254")
)

// Hand-written subset of the IANA special-purpose registries. The product
// must generate this list from the registries (see doc, "needed before
// production").
var reservedPrefixes = mustPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24",
	"192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
	"224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32",
	"::/8", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23",
	"2001:db8::/32", "2002::/16", "3fff::/20", "fc00::/7", "fe80::/10", "ff00::/8",
)

func mustPrefixes(s ...string) []netip.Prefix {
	var out []netip.Prefix
	for _, p := range s {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}

// Policy is the in-spike evaluator.
type Policy struct {
	Lab         bool
	LabPrefixes []netip.Prefix // loopback prefixes a lab may allow
	LabPorts    map[int]bool   // explicit lab ports (apply to lab prefixes only)
	LocalAddrs  func() []netip.Addr
}

// NewPolicy returns a policy. With lab=false nothing on loopback is allowed.
// With lab=true 127.0.0.0/8 and ::1 are allowed on the listed ports only.
func NewPolicy(lab bool, labPorts ...int) *Policy {
	p := &Policy{
		Lab:         lab,
		LabPrefixes: mustPrefixes("127.0.0.0/8", "::1/128"),
		LabPorts:    map[int]bool{},
		LocalAddrs:  interfaceAddrs,
	}
	for _, port := range labPorts {
		p.LabPorts[port] = true
	}
	return p
}

// WithLabPrefixes narrows (or changes) the lab prefixes; returns p.
func (p *Policy) WithLabPrefixes(s ...string) *Policy {
	p.LabPrefixes = mustPrefixes(s...)
	return p
}

func interfaceAddrs() []netip.Addr {
	var out []netip.Addr
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(ipn.IP); ok {
				out = append(out, ip.Unmap())
			}
		}
	}
	return out
}

// Decision is the outcome for one address or one target.
type Decision struct {
	Allowed bool
	Reason  string
}

func deny(f string, a ...any) Decision  { return Decision{false, fmt.Sprintf(f, a...)} }
func allow(f string, a ...any) Decision { return Decision{true, fmt.Sprintf(f, a...)} }

func inPrefixes(a netip.Addr, ps []netip.Prefix) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// CheckAddr evaluates one concrete address and port.
func (p *Policy) CheckAddr(a netip.Addr, port int) Decision {
	if !a.IsValid() {
		return deny("invalid address")
	}
	if a.Zone() != "" {
		return deny("zoned address is ambiguous")
	}
	a = a.Unmap() // ::ffff:10.0.0.5 is evaluated as 10.0.0.5
	if port < 1 || port > 65535 {
		return deny("port %d out of range", port)
	}
	// Never overridable, not even by a lab registration.
	if a == metadata4 || a == metadata6 {
		return deny("cloud metadata address")
	}
	if a.IsUnspecified() {
		return deny("unspecified address")
	}
	if a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() {
		return deny("link-local address")
	}
	if a.IsLoopback() {
		if !p.Lab {
			return deny("loopback address (lab mode off)")
		}
		if !inPrefixes(a, p.LabPrefixes) {
			return deny("loopback address outside lab prefixes")
		}
		if !p.LabPorts[port] {
			return deny("loopback lab port %d not listed", port)
		}
		return allow("lab loopback %s:%d", a, port)
	}
	if a.IsMulticast() {
		return deny("multicast address")
	}
	if a.IsPrivate() {
		return deny("private/unique-local address")
	}
	if inPrefixes(a, reservedPrefixes) {
		return deny("reserved/special-purpose address")
	}
	if !a.IsGlobalUnicast() {
		return deny("not a global unicast address")
	}
	if a.Is6() && !netip.MustParsePrefix("2000::/3").Contains(a) {
		return deny("outside 2000::/3")
	}
	for _, l := range p.LocalAddrs() {
		if l == a {
			return deny("assigned to a local interface")
		}
	}
	ok := false
	for _, dp := range DefaultPorts {
		if dp == port {
			ok = true
		}
	}
	if !ok {
		return deny("port %d not in default in-scope ports 80/443", port)
	}
	return allow("global unicast %s:%d", a, port)
}

// Target is the result of evaluating a (host, port) target.
type Target struct {
	Host    string // canonical host (name or IP literal)
	Port    int
	Addrs   []netip.Addr // unmapped; the only addresses that may be dialed
	Allowed bool
	Reason  string
	Detail  []string
	Queried bool // true when DNS was used
}

// Evaluate canonicalizes the host, resolves it ONCE (unless it is a literal),
// and evaluates every returned address. If any address is denied, the whole
// target is denied. Resolution failure fails closed.
func (p *Policy) Evaluate(ctx context.Context, r *net.Resolver, host string, port int) *Target {
	t := &Target{Port: port}
	canon, ip, isIP, err := CanonicalHost(host)
	if err != nil {
		t.Reason = "ambiguous host: " + err.Error()
		return t
	}
	t.Host = canon
	var addrs []netip.Addr
	if isIP {
		addrs = []netip.Addr{ip}
	} else {
		t.Queried = true
		got, err := r.LookupNetIP(ctx, "ip", canon)
		if err != nil {
			t.Reason = "resolution failed: " + err.Error()
			return t
		}
		addrs = got
	}
	if len(addrs) == 0 {
		t.Reason = "no addresses"
		return t
	}
	allOK := true
	for _, a := range addrs {
		a = a.Unmap().WithZone("")
		d := p.CheckAddr(a, port)
		t.Detail = append(t.Detail, fmt.Sprintf("%s=%v(%s)", a, d.Allowed, d.Reason))
		if !d.Allowed {
			allOK = false
			if t.Reason == "" {
				t.Reason = fmt.Sprintf("%s denied: %s", a, d.Reason)
			}
		}
		t.Addrs = append(t.Addrs, a)
	}
	if !allOK {
		t.Addrs = nil // nothing may be dialed
		return t
	}
	t.Allowed = true
	t.Reason = "all addresses allowed"
	return t
}

// CanonicalHost returns the canonical form of a host, or an error when the
// host is ambiguous. Numeric shorthand (127.1, 0x7f.1, 2130706433), zones,
// non-ASCII names and anything outside LDH labels are refused.
func CanonicalHost(h string) (canon string, ip netip.Addr, isIP bool, err error) {
	if h == "" {
		return "", ip, false, fmt.Errorf("empty host")
	}
	if strings.ContainsAny(h, "%[]\\/@ \t\r\n?#") {
		return "", ip, false, fmt.Errorf("forbidden character in host %q", h)
	}
	if strings.Contains(h, ":") {
		a, e := netip.ParseAddr(h)
		if e != nil {
			return "", ip, false, fmt.Errorf("unparseable IPv6 literal %q", h)
		}
		return a.String(), a, true, nil
	}
	if a, e := netip.ParseAddr(h); e == nil {
		return a.String(), a, true, nil
	}
	for i := 0; i < len(h); i++ {
		if h[i] >= 0x80 {
			return "", ip, false, fmt.Errorf("non-ASCII host (IDNA not implemented in spike)")
		}
	}
	h = strings.ToLower(strings.TrimSuffix(h, "."))
	if h == "" || len(h) > 253 {
		return "", ip, false, fmt.Errorf("bad host length")
	}
	labels := strings.Split(h, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return "", ip, false, fmt.Errorf("bad label %q", l)
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", ip, false, fmt.Errorf("bad character in label %q", l)
			}
		}
	}
	last := labels[len(labels)-1]
	if isNumericLike(last) {
		return "", ip, false, fmt.Errorf("numeric last label %q (IPv4 shorthand)", last)
	}
	return h, ip, false, nil
}

func isNumericLike(l string) bool {
	if strings.HasPrefix(l, "0x") {
		return true
	}
	for i := 0; i < len(l); i++ {
		if l[i] < '0' || l[i] > '9' {
			return false
		}
	}
	return true
}
