package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"spike/egress"
)

// E1: DNS pinning / rebinding.
func e1() {
	c := egress.NewChecker("E1")
	cx, cancel := ctx()
	defer cancel()

	dns := must(egress.NewDNSServer())
	defer dns.Close()
	res := dns.Resolver()

	pki := must(egress.NewPKI([]string{"rebind.test", "rebind-tls.test"}, nil))
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{pki.Leaf}}

	// Fixture A (the validated address) and B (the rebinding target) share a port.
	fa := must(egress.StartFixture("fixture-A", "tcp", "127.0.0.1:0", nil, false))
	defer fa.Close()
	port := fa.Port()
	fb := must(egress.StartFixture("fixture-B", "tcp", fmt.Sprintf("127.0.0.2:%d", port), nil, false))
	defer fb.Close()
	ta := must(egress.StartFixture("tls-A", "tcp", "127.0.0.1:0", tlsCfg, false))
	defer ta.Close()
	tport := ta.Port()
	tb := must(egress.StartFixture("tls-B", "tcp", fmt.Sprintf("127.0.0.2:%d", tport), tlsCfg, false))
	defer tb.Close()

	// Lab narrowed to 127.0.0.1 only, so 127.0.0.2 is a *denied* but reachable address.
	lab := egress.NewPolicy(true, port, tport).WithLabPrefixes("127.0.0.1/32")
	dns.Set("rebind.test", egress.Step{A: []string{"127.0.0.1"}}, egress.Step{A: []string{"127.0.0.2"}})
	dns.Set("rebind-tls.test", egress.Step{A: []string{"127.0.0.1"}}, egress.Step{A: []string{"127.0.0.2"}})
	url := fmt.Sprintf("http://rebind.test:%d/ok", port)
	c.Obs("mock DNS at %s; rebind.test: answer #1 -> 127.0.0.1 (lab-allowed), answer #2+ -> 127.0.0.2 (denied by policy but a live fixture)", dns.Addr())
	c.Obs("fixtures: A=127.0.0.1:%d B=127.0.0.2:%d (same port)", port, port)

	cl := &egress.Client{Policy: lab, Resolver: res, RootCAs: pki.Pool}

	// ---- (1) first-party client, request 1
	r1 := cl.Fetch(cx, url)
	hopLines(c, r1)
	c.Obs("request 1 body: %q", strings.TrimSpace(r1.Body))
	c.Obs("dial stats: %v", r1.Stats.Snapshot())
	c.Obs("DNS log after request 1: %v", dns.Log())
	c.Check("req1-allowed-and-reached-A", r1.Final && strings.Contains(r1.Body, "fixture=fixture-A") && fb.Conns() == 0,
		"final=%v A.conns=%d B.conns=%d", r1.Final, fa.Conns(), fb.Conns())
	c.Check("req1-one-lookup", dns.CountType("rebind.test", egress.TypeA) == 1 && dns.CountType("rebind.test", egress.TypeAAAA) == 1,
		"A queries=%d AAAA queries=%d (one resolution = one A + one AAAA; no second lookup at dial time)",
		dns.CountType("rebind.test", egress.TypeA), dns.CountType("rebind.test", egress.TypeAAAA))
	c.Check("req1-connected-to-validated-IP", len(r1.Hops) == 1 && strings.HasPrefix(r1.Hops[0].Remote, "127.0.0.1:"),
		"remote=%s; Host header seen by server: %s", r1.Hops[0].Remote, hostSeen(fa))

	// ---- (2) request 2 sees the rebound answer and is re-validated, not trusted
	before := dns.CountType("rebind.test", egress.TypeA)
	r2 := cl.Fetch(cx, url)
	hopLines(c, r2)
	c.Check("req2-rebound-answer-denied-no-connect", !r2.Final && r2.Err != nil && fb.Conns() == 0,
		"err=%v B.conns=%d (A queries %d -> %d: exactly one new lookup per request)", r2.Err, fb.Conns(), before, dns.CountType("rebind.test", egress.TypeA))

	// ---- (3) HTTPS: SNI/Host stay the canonical name; cert is verified against the name, not the IP
	dns.Reset()
	r3 := cl.Fetch(cx, fmt.Sprintf("https://rebind-tls.test:%d/ok", tport))
	hopLines(c, r3)
	c.Obs("https body: %q", strings.TrimSpace(r3.Body))
	c.Check("tls-sni-is-canonical-name", r3.Final && strings.Contains(r3.Body, "sni=rebind-tls.test") && tb.Conns() == 0,
		"final=%v tls-B.conns=%d; remote=%s", r3.Final, tb.Conns(), r3.Hops[0].Remote)

	// ---- (4) CONTROL: validate, then let net/http resolve again (the classic TOCTOU)
	dns.Reset()
	fa2c, fb2c := fa.Conns(), fb.Conns()
	t := lab.Evaluate(cx, res, "rebind.test", port)
	c.Obs("CONTROL step 1: validator lookup -> allowed=%v addrs=%v (A queries so far: %d)", t.Allowed, t.Addrs, dns.CountType("rebind.test", egress.TypeA))
	naive := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Resolver: res}).DialContext, // net/http resolves again at dial time
	}}
	resp, err := naive.Get(url)
	body := ""
	if err == nil {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		body = strings.TrimSpace(string(b))
	}
	c.Obs("CONTROL step 2: net/http did its own lookup -> A queries now %d; body=%q err=%v", dns.CountType("rebind.test", egress.TypeA), body, err)
	c.Check("control-naive-approach-is-rebindable", t.Allowed && strings.Contains(body, "fixture=fixture-B") && fb.Conns() == fb2c+1 && fa.Conns() == fa2c,
		"validated 127.0.0.1 but net/http connected to 127.0.0.2 (B.conns %d -> %d). This is the failure mode the pinned dial removes", fb2c, fb.Conns())

	// ---- (5) lab mode OFF: everything is denied, nothing is contacted
	dns.Reset()
	fa3 := fa.Conns()
	off := &egress.Client{Policy: egress.NewPolicy(false), Resolver: res, RootCAs: pki.Pool}
	r5 := off.Fetch(cx, url)
	hopLines(c, r5)
	c.Check("lab-off-denied-no-connect", !r5.Final && fa.Conns() == fa3 && fb.Conns() == fb2c+1,
		"err=%v A.conns unchanged=%d", r5.Err, fa.Conns())
	c.Verdict("pinned dial keeps validated IP across a changing DNS answer; one resolution per request")
}

func hostSeen(f *egress.Fixture) string {
	if len(f.Seen) == 0 {
		return "(none)"
	}
	s := f.Seen[len(f.Seen)-1]
	if i := strings.Index(s, "host="); i >= 0 {
		return strings.Fields(s[i:])[0]
	}
	return s
}
