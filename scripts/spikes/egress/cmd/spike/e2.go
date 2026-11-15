package main

import (
	"fmt"
	"strings"

	"spike/egress"
)

// E2: mixed answers and CNAME chains.
func e2() {
	c := egress.NewChecker("E2")
	cx, cancel := ctx()
	defer cancel()
	dns := must(egress.NewDNSServer())
	defer dns.Close()
	res := dns.Resolver()

	f4 := must(egress.StartFixture("v4", "tcp", "127.0.0.1:0", nil, false))
	defer f4.Close()
	port := f4.Port()
	f6 := must(egress.StartFixture("v6", "tcp", fmt.Sprintf("[::1]:%d", port), nil, false))
	defer f6.Close()

	S := func(s egress.Step) []egress.Step { return []egress.Step{s} }
	dns.Set("ok-dual.test", S(egress.Step{A: []string{"127.0.0.1"}, AAAA: []string{"::1"}})...)
	dns.Set("mixed-a.test", S(egress.Step{A: []string{"127.0.0.1", "10.0.0.5"}})...)
	dns.Set("mixed-a-meta.test", S(egress.Step{A: []string{"127.0.0.1", "169.254.169.254"}})...)
	dns.Set("aaaa-bad.test", S(egress.Step{A: []string{"127.0.0.1"}, AAAA: []string{"fc00::5"}})...)        // only AAAA denied
	dns.Set("aaaa-meta.test", S(egress.Step{A: []string{"127.0.0.1"}, AAAA: []string{"fd00:ec2::254"}})...) // AAAA = metadata
	dns.Set("a-bad.test", S(egress.Step{A: []string{"10.0.0.5"}, AAAA: []string{"::1"}})...)                // only A denied
	dns.Set("mapped.test", S(egress.Step{A: []string{"127.0.0.1"}, AAAA: []string{"::ffff:10.0.0.5"}})...)  // 4in6 hides a private address
	dns.Set("ok-via-cname.test", S(egress.Step{CNAME: "hop1.test"})...)
	dns.Set("hop1.test", S(egress.Step{CNAME: "hop2.test"})...)
	dns.Set("hop2.test", S(egress.Step{A: []string{"127.0.0.1"}})...)
	dns.Set("www.test", S(egress.Step{CNAME: "cdn.test"})...)
	dns.Set("cdn.test", S(egress.Step{CNAME: "edge.test"})...)
	dns.Set("edge.test", S(egress.Step{A: []string{"10.0.0.9"}})...) // chain ends on a denied address
	dns.Set("www-meta.test", S(egress.Step{CNAME: "m1.test"})...)
	dns.Set("m1.test", S(egress.Step{A: []string{"169.254.169.254"}})...)
	dns.Set("www-mixed.test", S(egress.Step{CNAME: "m2.test"})...)
	dns.Set("m2.test", S(egress.Step{A: []string{"127.0.0.1", "192.168.1.10"}})...)

	lab := egress.NewPolicy(true, port)
	off := egress.NewPolicy(false)
	cl := &egress.Client{Policy: lab, Resolver: res}
	clOff := &egress.Client{Policy: off, Resolver: res}

	// cname chain as seen by the resolver (extra observation, separate lookups)
	if cn, err := res.LookupCNAME(cx, "www.test"); err == nil {
		c.Obs("LookupCNAME(www.test) -> %s (chain resolved by the mock; policy evaluates only the final addresses)", cn)
	}

	type tc struct {
		name      string
		wantAllow bool
	}
	cases := []tc{
		{"ok-dual.test", true}, {"ok-via-cname.test", true},
		{"mixed-a.test", false}, {"mixed-a-meta.test", false},
		{"aaaa-bad.test", false}, {"aaaa-meta.test", false}, {"a-bad.test", false}, {"mapped.test", false},
		{"www.test", false}, {"www-meta.test", false}, {"www-mixed.test", false},
	}
	for _, k := range cases {
		dns.Reset()
		conns := f4.Conns() + f6.Conns()
		t := lab.Evaluate(cx, res, k.name, port)
		c.Obs("%-18s allowed=%-5v reason=%q detail=%v", k.name, t.Allowed, t.Reason, t.Detail)
		r := cl.Fetch(cx, fmt.Sprintf("http://%s:%d/ok", k.name, port))
		contacted := f4.Conns()+f6.Conns() != conns
		c.Check("lab-on/"+k.name, t.Allowed == k.wantAllow && r.Final == k.wantAllow && k.wantAllow == contacted,
			"allow=%v fetch.final=%v contacted=%v A/AAAA queries=%d/%d dial-events=%d",
			t.Allowed, r.Final, contacted, dns.CountType(k.name, egress.TypeA), dns.CountType(k.name, egress.TypeAAAA), len(r.Stats.Snapshot()))
	}
	// the dual-stack control must have connected only to validated addresses
	d := cl.Fetch(cx, fmt.Sprintf("http://ok-dual.test:%d/ok", port))
	c.Obs("ok-dual.test dial events: %v", d.Stats.Snapshot())

	// lab off: everything denied, nothing contacted
	conns := f4.Conns() + f6.Conns()
	allDenied := true
	for _, k := range cases {
		r := clOff.Fetch(cx, fmt.Sprintf("http://%s:%d/ok", k.name, port))
		if r.Final {
			allDenied = false
			c.Obs("lab-off unexpectedly allowed %s", k.name)
		}
	}
	c.Check("lab-off/all-denied-no-connect", allDenied && f4.Conns()+f6.Conns() == conns, "11 names denied, connections unchanged at %d", conns)
	c.Obs("example lab-off reason: %s", strings.TrimPrefix(fmt.Sprint(clOff.Fetch(cx, fmt.Sprintf("http://ok-dual.test:%d/ok", port)).Err), "denied: "))
	c.Verdict("any denied address (A, AAAA, CNAME target, 4in6) denies the whole target before any dial")
}
