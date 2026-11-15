package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"spike/egress"
)

// E6: policy proxy driven by a Go client and by curl.
func e6() {
	c := egress.NewChecker("E6")
	cx, cancel := ctx()
	defer cancel()
	dir := must(egress.ScratchDir())
	dns := must(egress.NewDNSServer())
	defer dns.Close()
	res := dns.Resolver()
	pki := must(egress.NewPKI([]string{"allowed.test", "a.test", "b.test", "deny4.test", "mixed.test"}, nil))
	caPath := must(egress.WriteScratch(dir, "e6-ca.pem", pki.CAPEM))
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{pki.Leaf}}

	plain := must(egress.StartFixture("plain", "tcp", "127.0.0.1:0", nil, false))
	defer plain.Close()
	tlsF := must(egress.StartFixture("tls", "tcp", "127.0.0.1:0", tlsCfg, false))
	defer tlsF.Close()
	forb := must(egress.StartFixture("forbidden-port", "tcp", "127.0.0.1:0", nil, false))
	defer forb.Close()
	pp, pt, px := plain.Port(), tlsF.Port(), forb.Port()

	S := func(a ...string) egress.Step { return egress.Step{A: a} }
	dns.Set("allowed.test", S("127.0.0.1"))
	dns.Set("a.test", S("127.0.0.1"))
	dns.Set("b.test", S("127.0.0.1"))
	dns.Set("deny4.test", S("10.0.0.5"))
	dns.Set("mixed.test", S("127.0.0.1", "10.0.0.5"))

	lab := egress.NewPolicy(true, pp, pt) // forbidden-port fixture and the proxy's own port are NOT listed
	prx := &egress.Proxy{Policy: lab, Resolver: res}
	pln := must(net.Listen("tcp", "127.0.0.1:0"))
	srv := prx.Serve(pln)
	defer srv.Close()
	ppPort := pln.Addr().(*net.TCPAddr).Port
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", ppPort)
	c.Obs("proxy %s; lab ports=[%d plain, %d tls]; NOT listed: %d (forbidden fixture), %d (proxy's own port)", proxyURL, pp, pt, px, ppPort)

	pu := must(url.Parse(proxyURL))
	goGet := func(rawurl string, mod func(*http.Request, *tls.Config)) (int, string, error) {
		tc := &tls.Config{RootCAs: pki.Pool}
		tr := &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: tc, DisableKeepAlives: true}
		defer tr.CloseIdleConnections()
		cl := &http.Client{Transport: tr, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, _ := http.NewRequestWithContext(cx, "GET", rawurl, nil)
		if mod != nil {
			mod(req, tc)
		}
		resp, err := cl.Do(req)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return resp.StatusCode, strings.TrimSpace(string(b)), nil
	}
	conns := func() [3]int64 { return [3]int64{plain.Conns(), tlsF.Conns(), forb.Conns()} }

	// ---- Go client through the proxy
	dns.Reset()
	st, body, err := goGet(fmt.Sprintf("http://allowed.test:%d/ok", pp), nil)
	c.Check("go/plain-allowed", err == nil && st == 200 && strings.Contains(body, "host=allowed.test:"), "status=%d body=%q", st, body)
	c.Check("go/plain-allowed-one-resolution", dns.CountType("allowed.test", egress.TypeA) == 1 && dns.CountType("allowed.test", egress.TypeAAAA) == 1, "A=%d AAAA=%d", dns.CountType("allowed.test", egress.TypeA), dns.CountType("allowed.test", egress.TypeAAAA))
	dns.Reset()
	st, body, err = goGet(fmt.Sprintf("https://allowed.test:%d/ok", pt), nil)
	c.Check("go/https-allowed-via-CONNECT", err == nil && st == 200 && strings.Contains(body, "sni=allowed.test"), "status=%d body=%q err=%v", st, body, err)
	c.Check("go/https-allowed-one-resolution", dns.CountType("allowed.test", egress.TypeA) == 1, "A=%d", dns.CountType("allowed.test", egress.TypeA))

	b0 := conns()
	st, body, err = goGet(fmt.Sprintf("http://deny4.test:%d/ok", pp), nil)
	c.Check("go/plain-denied-address", err == nil && st == 403, "status=%d body=%q", st, body)
	_, _, err = goGet(fmt.Sprintf("https://deny4.test:%d/ok", pt), nil)
	c.Check("go/https-denied-address", err != nil && strings.Contains(err.Error(), "403") || err != nil && strings.Contains(err.Error(), "Forbidden"), "err=%v", err)
	st, body, err = goGet(fmt.Sprintf("http://127.0.0.1:%d/ok", px), nil)
	c.Check("go/plain-denied-port", err == nil && st == 403 && forb.Conns() == 0, "status=%d body=%q forbidden-fixture conns=%d", st, body, forb.Conns())
	_, _, err = goGet(fmt.Sprintf("https://127.0.0.1:%d/ok", px), nil)
	c.Check("go/https-denied-port", err != nil && forb.Conns() == 0, "err=%v forbidden-fixture conns=%d", err, forb.Conns())
	_, _, err = goGet(fmt.Sprintf("https://127.0.0.1:%d/ok", ppPort), nil)
	c.Check("go/connect-to-proxy-own-port-denied", err != nil, "err=%v (own listener is never an allowed target)", err)
	st, body, _ = goGet(fmt.Sprintf("http://mixed.test:%d/ok", pp), nil)
	c.Check("go/plain-mixed-answer-denied", st == 403, "status=%d body=%q", st, body)
	_, _, err = goGet(fmt.Sprintf("https://mixed.test:%d/ok", pt), nil)
	c.Check("go/https-mixed-answer-denied", err != nil, "err=%v", err)
	c.Check("go/denied-targets-never-contacted", conns() == [3]int64{b0[0], b0[1], b0[2]}, "fixture connections before=%v after=%v", b0, conns())

	// ---- what the proxy can and cannot see
	st, body, err = goGet(fmt.Sprintf("https://a.test:%d/ok", pt), func(r *http.Request, tc *tls.Config) { tc.ServerName = "b.test" })
	c.Obs("CONNECT a.test but TLS SNI=b.test and Host=a.test: status=%d body=%q err=%v", st, body, err)
	c.Check("go/https-sni-differs-from-connect-host-not-detectable", err == nil && strings.Contains(body, "sni=b.test") && strings.Contains(body, "host=a.test:"),
		"server saw SNI b.test; the proxy log shows only CONNECT a.test (SNI not inspected)")
	st, body, err = goGet(fmt.Sprintf("http://allowed.test:%d/ok", pp), func(r *http.Request, tc *tls.Config) { r.Host = "evil.test" })
	c.Obs("Go client with req.Host=evil.test: net/http rewrites the absolute-URI target to evil.test, so the proxy evaluated evil.test instead: status=%d body=%q", st, body)
	c.Check("go/req.Host-override-changes-evaluated-target", st == 403 && strings.Contains(body, "evil.test"), "the override is evaluated as the target, not smuggled behind an allowed one")
	// raw request whose Host header disagrees with the absolute-URI authority
	raw := must(net.Dial("tcp", pln.Addr().String()))
	fmt.Fprintf(raw, "GET http://allowed.test:%d/ok HTTP/1.1\r\nHost: evil.test\r\nConnection: close\r\n\r\n", pp)
	rb, _ := io.ReadAll(io.LimitReader(raw, 4096))
	raw.Close()
	c.Obs("raw absolute-URI request with mismatching 'Host: evil.test': response=%q", strings.ReplaceAll(string(rb), "\r\n", "|"))
	c.Check("raw/host-header-mismatch-neutralized", strings.Contains(string(rb), "host=allowed.test:") && !strings.Contains(string(rb), "host=evil.test"), "upstream saw Host=allowed.test:%d (the client's Host header is discarded by net/http's server and replaced by the evaluated authority)", pp)

	// E9 evidence: CONNECT relays arbitrary (non-HTTP) bytes to an allowed host:port
	tun := must(net.Dial("tcp", pln.Addr().String()))
	fmt.Fprintf(tun, "CONNECT 127.0.0.1:%d HTTP/1.1\r\nHost: 127.0.0.1:%d\r\n\r\nNOT-HTTP-AT-ALL\r\n\r\n", pp, pp)
	tun.SetReadDeadline(time.Now().Add(3 * time.Second))
	tb, _ := io.ReadAll(io.LimitReader(tun, 2048))
	tun.Close()
	c.Obs("E9 evidence: CONNECT 127.0.0.1:%d then non-HTTP bytes -> reply=%q", pp, strings.ReplaceAll(string(tb), "\r\n", "|"))
	c.Check("e9/connect-relays-arbitrary-bytes-to-allowed-port", strings.Contains(string(tb), "200 Connection Established") && strings.Contains(string(tb), "400 Bad Request"),
		"the proxy relayed bytes it cannot interpret; policy is host:port only, protocol is not enforced")
	st, body, err = goGet(fmt.Sprintf("https://allowed.test:%d/r?code=302&loc=%%2Fok", pt), nil)
	c.Obs("HTTPS same-host redirect (client does not follow here): status=%d", st)
	st, body, err = goGet(fmt.Sprintf("https://allowed.test:%d/r?code=302&loc=%s", pt, url.QueryEscape(fmt.Sprintf("https://deny4.test:%d/", pt))), nil)
	c.Obs("HTTPS redirect to a denied name, status seen by client=%d (the redirect itself passed through the tunnel unseen)", st)

	// ---- curl through the proxy (HTTPS_PROXY / http_proxy)
	curl := func(args ...string) (string, int) {
		a := append([]string{"-q", "-sS", "--cacert", caPath, "-w", "\\n[curl http_code=%{http_code} num_connects=%{num_connects} exit-info=%{exitcode} proxy_connect=%{http_connect}]"}, args...)
		cmd := exec.CommandContext(cx, "curl", a...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HTTPS_PROXY=" + proxyURL, "http_proxy=" + proxyURL}
		out, _ := cmd.CombinedOutput()
		code := 0
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		return strings.TrimSpace(string(out)), code
	}
	out, code := curl(fmt.Sprintf("https://allowed.test:%d/ok", pt))
	c.Check("curl/https-allowed", code == 0 && strings.Contains(out, "http_code=200"), "exit=%d out=%q", code, out)
	out, code = curl(fmt.Sprintf("http://allowed.test:%d/ok", pp))
	c.Check("curl/http-allowed", code == 0 && strings.Contains(out, "http_code=200"), "exit=%d out=%q", code, out)
	for _, u := range []string{
		fmt.Sprintf("https://deny4.test:%d/", pt), fmt.Sprintf("http://deny4.test:%d/", pp),
		fmt.Sprintf("https://127.0.0.1:%d/", px), fmt.Sprintf("http://127.0.0.1:%d/", px),
		fmt.Sprintf("https://mixed.test:%d/", pt), fmt.Sprintf("http://mixed.test:%d/", pp)} {
		out, code = curl(u)
		c.Check("curl/denied/"+u, code != 0 || strings.Contains(out, "http_code=403"), "exit=%d out=%q", code, strings.ReplaceAll(out, "\n", " "))
	}
	c.Check("curl/denied-never-contacted", forb.Conns() == 0, "forbidden fixture conns=%d", forb.Conns())
	// redirects over HTTPS: enforced per new CONNECT
	n := len(prx.Log())
	out, code = curl("-L", fmt.Sprintf("https://allowed.test:%d/r?code=302&loc=%s", pt, url.QueryEscape(fmt.Sprintf("https://deny4.test:%d/", pt))))
	c.Check("curl/https-redirect-to-denied-name-refused-at-new-CONNECT", code != 0, "exit=%d out=%q", code, strings.ReplaceAll(out, "\n", " "))
	for _, l := range prx.Log()[n:] {
		c.Obs("proxy log for that redirect: %s", l)
	}
	n = len(prx.Log())
	before := tlsF.Reqs()
	out, code = curl("-L", fmt.Sprintf("https://allowed.test:%d/r?code=302&loc=%%2Fok", pt))
	c.Obs("curl -L same-host redirect: exit=%d out=%q server requests +%d", code, strings.ReplaceAll(out, "\n", " "), tlsF.Reqs()-before)
	nc := 0
	for _, l := range prx.Log()[n:] {
		c.Obs("proxy log for same-host redirect: %s", l)
		if strings.HasPrefix(l, "CONNECT") {
			nc++
		}
	}
	c.Check("proxy-has-no-per-hop-visibility-for-https", tlsF.Reqs()-before == 2 && nc == 1, "server handled %d requests (302 + final) over %d CONNECT: the proxy cannot see the redirect or the path", tlsF.Reqs()-before, nc)
	out, code = curl("-H", "Host: evil.test", fmt.Sprintf("http://allowed.test:%d/ok", pp))
	c.Check("curl/host-header-override-neutralized", code == 0 && strings.Contains(out, "host=allowed.test:"), "out=%q", strings.ReplaceAll(out, "\n", " "))

	// ---- lab OFF
	offPrx := &egress.Proxy{Policy: egress.NewPolicy(false), Resolver: res}
	oln := must(net.Listen("tcp", "127.0.0.1:0"))
	osrv := offPrx.Serve(oln)
	defer osrv.Close()
	ou := must(url.Parse("http://" + oln.Addr().String()))
	b1 := conns()
	tr := &http.Transport{Proxy: http.ProxyURL(ou), TLSClientConfig: &tls.Config{RootCAs: pki.Pool}, DisableKeepAlives: true}
	r1, e1 := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get(fmt.Sprintf("http://allowed.test:%d/ok", pp))
	code1 := 0
	if r1 != nil {
		code1 = r1.StatusCode
		r1.Body.Close()
	}
	_, e2 := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get(fmt.Sprintf("https://allowed.test:%d/ok", pt))
	c.Check("lab-off/proxy-denies-everything-no-connect", e1 == nil && code1 == 403 && e2 != nil && conns() == b1, "plain status=%d https err=%v conns before=%v after=%v", code1, e2, b1, conns())

	for _, l := range prx.Log() {
		if strings.HasPrefix(l, "TUNNEL") {
			c.Obs("sample proxy log: %s", l)
			break
		}
	}
	c.Verdict("proxy enforces address/port/mixed-answer policy per CONNECT and per absolute-URI request; HTTPS redirects are only enforced per new CONNECT")
}
