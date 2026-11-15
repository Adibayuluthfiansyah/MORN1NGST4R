package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"spike/egress"
)

// E4: redirects. Manual loop, every hop fully evaluated.
func e4() {
	c := egress.NewChecker("E4")
	cx, cancel := ctx()
	defer cancel()
	dns := must(egress.NewDNSServer())
	defer dns.Close()
	res := dns.Resolver() // nxdomain.test has no rule -> NXDOMAIN

	f1 := must(egress.StartFixture("P1", "tcp", "127.0.0.1:0", nil, false))
	defer f1.Close()
	f2 := must(egress.StartFixture("P2", "tcp", "127.0.0.1:0", nil, false))
	defer f2.Close()
	p1, p2 := f1.Port(), f2.Port()
	c.Obs("fixtures: P1=127.0.0.1:%d (lab-listed)  P2=127.0.0.1:%d (NOT lab-listed in the main policy)", p1, p2)

	lab := egress.NewPolicy(true, p1)
	cl := &egress.Client{Policy: lab, Resolver: res}
	start := func(code int, loc string) string {
		return fmt.Sprintf("http://127.0.0.1:%d/r?code=%d&loc=%s", p1, code, url.QueryEscape(loc))
	}

	type tc struct {
		label    string
		code     int
		loc      string
		follow   bool // expected: the redirect target is contacted
		p2Target bool
	}
	cases := []tc{
		{"abs URL, metadata address (301)", 301, "http://169.254.169.254:80/latest/meta-data/", false, false},
		{"abs URL, private address (302)", 302, "http://10.0.0.5:80/", false, false},
		{"scheme-relative //host:port, listed port (307)", 307, fmt.Sprintf("//127.0.0.1:%d/ok", p1), true, false},
		{"scheme-relative //host:port, unlisted port (308)", 308, fmt.Sprintf("//127.0.0.1:%d/ok", p2), false, true},
		{"backslash form http:\\\\host (302)", 302, fmt.Sprintf("http:\\\\127.0.0.1:%d/ok", p1), false, false},
		{"relative path /ok (301)", 301, "/ok", true, false},
		{"relative path /ok (302)", 302, "/ok", true, false},
		{"relative path /ok (307)", 307, "/ok", true, false},
		{"relative path /ok (308)", 308, "/ok", true, false},
		{"relative path /ok (303)", 303, "/ok", true, false},
		{"different port, same host, not listed (302)", 302, fmt.Sprintf("http://127.0.0.1:%d/ok", p2), false, true},
		{"userinfo trick http://127.0.0.1:P1@10.0.0.5/ (302)", 302, fmt.Sprintf("http://127.0.0.1:%d@10.0.0.5/", p1), false, false},
		{"unparseable http://[::1/ (302)", 302, "http://[::1/", false, false},
		{"bad escape http://%zz/ (302)", 302, "http://%zz/", false, false},
		{"escaped host %31%32%37.0.0.1 (302)", 302, fmt.Sprintf("http://%%31%%32%%37.0.0.1:%d/ok", p1), false, false},
		{"IPv4 shorthand 127.1 (302)", 302, fmt.Sprintf("http://127.1:%d/ok", p1), false, false},
		{"hex IPv4 0x7f.0.0.1 (302)", 302, fmt.Sprintf("http://0x7f.0.0.1:%d/ok", p1), false, false},
		{"empty Location (302)", 302, "", false, false},
		{"ftp scheme (302)", 302, fmt.Sprintf("ftp://127.0.0.1:%d/", p1), false, false},
		{"file scheme (302)", 302, "file:///etc/passwd", false, false},
		{"javascript: scheme (302)", 302, "javascript:alert(1)", false, false},
		{"name that fails to resolve (302)", 302, fmt.Sprintf("http://nxdomain.test:%d/ok", p1), false, false},
		{"whitespace in Location (302)", 302, fmt.Sprintf("http://127.0.0.1:%d/ok x", p1), false, false},
	}
	runCases := func(prefix string, cl *egress.Client, verbose bool) {
		for _, k := range cases {
			beforeP2 := f2.Conns()
			beforeP1 := f1.Conns()
			r := cl.Fetch(cx, start(k.code, k.loc))
			if verbose {
				hopLines(c, r)
			}
			followed := false
			for _, h := range r.Hops[1:] {
				if h.Decision == "ALLOW" && h.Remote != "" {
					followed = true
				}
			}
			p2hit := f2.Conns() != beforeP2
			okFinal := !k.follow || r.Final
			c.Check(prefix+k.label, followed == k.follow && !p2hit && okFinal && f1.Conns() >= beforeP1,
				"redirect target contacted=%v (expected %v), P2 contacted=%v, err=%v", followed, k.follow, p2hit, short(r.Err))
		}
	}
	runCases("lab-on/", cl, true)
	// Same table, but without http.Client: Transport.RoundTrip, so our own parser sees every Location.
	c.Obs("--- same table via Transport.RoundTrip (no http.Client); note which Location values http.Client turns into its own errors above ---")
	raw := &egress.Client{Policy: lab, Resolver: res, RawRoundTrip: true}
	runCases("lab-on-roundtrip/", raw, false)

	// loop
	r := cl.Fetch(cx, fmt.Sprintf("http://127.0.0.1:%d/loop/a", p1))
	hopLines(c, r)
	last := r.Hops[len(r.Hops)-1]
	c.Check("lab-on/loop-a-b-a", !r.Final && strings.Contains(last.Reason, "loop") && len(r.Hops) == 3, "hops=%d last=%q", len(r.Hops), last.Reason)

	// same different-port redirect, once P2 is explicitly listed
	both := &egress.Client{Policy: egress.NewPolicy(true, p1, p2), Resolver: res}
	r = both.Fetch(cx, start(302, fmt.Sprintf("http://127.0.0.1:%d/ok", p2)))
	hopLines(c, r)
	c.Check("lab-on-P2-listed/different-port-is-new-target-and-followed", r.Final && strings.Contains(r.Body, "fixture=P2"), "body=%q", strings.TrimSpace(r.Body))

	// CONTROL: default net/http client
	p2Before := f2.Conns()
	def := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true, Proxy: nil}}
	resp, err := def.Get(start(308, fmt.Sprintf("//127.0.0.1:%d/ok", p2)))
	body := ""
	if err == nil {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		body = strings.TrimSpace(string(b))
	}
	c.Obs("CONTROL default http.Client follows //127.0.0.1:P2 -> err=%v body=%q P2.conns %d -> %d", err, body, p2Before, f2.Conns())
	c.Check("control-default-client-follows-unlisted-port", err == nil && f2.Conns() == p2Before+1, "default client has no policy and followed the redirect")
	resp, err = def.Get(start(302, fmt.Sprintf("http:\\\\127.0.0.1:%d/ok", p1)))
	if resp != nil {
		resp.Body.Close()
	}
	c.Obs("CONTROL default http.Client on backslash form -> err=%v (Go's parser treats http:\\\\host as opaque; a browser would treat it as http://host/ — parsers disagree, so the spike refuses it)", err)

	// lab OFF: the first request is denied, nothing contacted
	off := &egress.Client{Policy: egress.NewPolicy(false), Resolver: res}
	n1, n2 := f1.Conns(), f2.Conns()
	r = off.Fetch(cx, start(302, "/ok"))
	hopLines(c, r)
	c.Check("lab-off/first-hop-denied-no-connect", !r.Final && f1.Conns() == n1 && f2.Conns() == n2, "err=%v", r.Err)
	c.Verdict("every hop is canonicalized and fully evaluated before it is followed; ambiguous Locations are refused")
}

func short(err error) string {
	if err == nil {
		return "<nil>"
	}
	m := err.Error()
	if len(m) > 110 {
		m = m[:110] + "..."
	}
	return m
}
