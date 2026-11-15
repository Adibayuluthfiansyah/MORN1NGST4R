package egress

import (
	"net/netip"
	"testing"
)

func TestCheckAddr(t *testing.T) {
	off := NewPolicy(false)
	off.LocalAddrs = func() []netip.Addr { return []netip.Addr{netip.MustParseAddr("8.8.4.4")} }
	lab := NewPolicy(true, 8080)
	lab.LocalAddrs = off.LocalAddrs
	cases := []struct {
		p    *Policy
		addr string
		port int
		want bool
	}{
		{off, "127.0.0.1", 8080, false},
		{off, "::1", 8080, false},
		{lab, "127.0.0.1", 8080, true},
		{lab, "127.0.0.9", 8080, true},
		{lab, "::1", 8080, true},
		{lab, "127.0.0.1", 9999, false}, // port not listed
		{lab, "169.254.169.254", 8080, false},
		{lab, "fd00:ec2::254", 8080, false},
		{lab, "0.0.0.0", 8080, false},
		{lab, "::", 8080, false},
		{lab, "10.0.0.5", 80, false},
		{lab, "192.168.1.1", 80, false},
		{lab, "fc00::1", 80, false},
		{lab, "fe80::1", 80, false},
		{lab, "224.0.0.1", 80, false},
		{lab, "ff02::1", 80, false},
		{lab, "192.0.2.1", 80, false},
		{lab, "100.64.0.1", 80, false},
		{lab, "240.0.0.1", 80, false},
		{lab, "::ffff:10.0.0.5", 80, false},   // 4in6 evaluated as 10.0.0.5
		{lab, "::ffff:127.0.0.1", 8080, true}, // 4in6 loopback evaluated as loopback
		{lab, "8.8.4.4", 443, false},          // assigned to a (faked) local interface
		{lab, "8.8.8.8", 443, true},           // evaluation only, never dialed
		{lab, "8.8.8.8", 80, true},
		{lab, "8.8.8.8", 8080, false}, // lab ports do not extend to public addresses
		{lab, "2001:4860:4860::8888", 443, true},
		{lab, "2001:db8::1", 443, false},
	}
	for _, c := range cases {
		d := c.p.CheckAddr(netip.MustParseAddr(c.addr), c.port)
		if d.Allowed != c.want {
			t.Errorf("%s:%d lab=%v got %v (%s) want %v", c.addr, c.port, c.p.Lab, d.Allowed, d.Reason, c.want)
		}
	}
}

func TestCanonicalHost(t *testing.T) {
	bad := []string{"", "127.1", "0x7f.1", "2130706433", "0177.0.0.1", "fe80::1%eth0", "a b", "exa\\mple.com", "ex%41mple.com", "[::1]", "-a.test", "a..test", "münchen.test"}
	for _, h := range bad {
		if _, _, _, err := CanonicalHost(h); err == nil {
			t.Errorf("host %q should be refused", h)
		}
	}
	good := map[string]string{"Example.TEST.": "example.test", "127.0.0.1": "127.0.0.1", "::1": "::1", "A-b.test": "a-b.test"}
	for in, want := range good {
		got, _, _, err := CanonicalHost(in)
		if err != nil || got != want {
			t.Errorf("host %q got %q err %v want %q", in, got, err, want)
		}
	}
}
