package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"spike/egress"
)

// E5: HTTP/2 connection coalescing and per-connection address checks.
func e5() {
	c := egress.NewChecker("E5")
	cx, cancel := ctx()
	defer cancel()
	dir := must(egress.ScratchDir())
	dns := must(egress.NewDNSServer())
	defer dns.Close()
	res := dns.Resolver()
	pki := must(egress.NewPKI([]string{"a.test", "b.test"}, nil))
	caPath := must(egress.WriteScratch(dir, "e5-ca.pem", pki.CAPEM))

	srv := must(egress.StartFixture("h2", "tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{pki.Leaf}}, true))
	defer srv.Close()
	port := srv.Port()
	dns.Set("a.test", egress.Step{A: []string{"127.0.0.1"}})
	dns.Set("b.test", egress.Step{A: []string{"127.0.0.1"}})
	c.Obs("one certificate (SAN a.test, b.test); both names -> 127.0.0.1; HTTP/2 server on 127.0.0.1:%d", port)

	newTransport := func(pol *egress.Policy, log *[]string, mu *sync.Mutex) *http.Transport {
		return &http.Transport{
			Proxy:             nil,
			ForceAttemptHTTP2: true,
			TLSClientConfig:   &tls.Config{RootCAs: pki.Pool},
			// The per-connection check: runs for every NEW connection, with the origin the transport wants.
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, ps, _ := net.SplitHostPort(addr)
				p, _ := strconv.Atoi(ps)
				t := pol.Evaluate(ctx, res, host, p)
				mu.Lock()
				*log = append(*log, fmt.Sprintf("dial-hook origin=%s allowed=%v addrs=%v", addr, t.Allowed, t.Addrs))
				mu.Unlock()
				if !t.Allowed {
					return nil, fmt.Errorf("denied: %s", t.Reason)
				}
				return pol.PinnedDial(t, &egress.DialStats{})(ctx, network, addr)
			},
		}
	}

	// ---- Go transport, shared (pooled) between origins
	lab := egress.NewPolicy(true, port)
	var log []string
	var mu sync.Mutex
	tr := newTransport(lab, &log, &mu)
	cl := &http.Client{Transport: tr, Timeout: 8 * time.Second}
	get := func(u string) (string, string) {
		resp, err := cl.Get(u)
		if err != nil {
			return "", err.Error()
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.Proto, strings.TrimSpace(string(b))
	}
	allH2 := true
	for _, u := range []string{"a.test", "b.test", "a.test", "b.test"} {
		proto, body := get(fmt.Sprintf("https://%s:%d/ok", u, port))
		c.Obs("GET %s -> %s %s", u, proto, body)
		allH2 = allH2 && proto == "HTTP/2.0"
	}
	mu.Lock()
	hooks := append([]string(nil), log...)
	mu.Unlock()
	for _, h := range hooks {
		c.Obs("%s", h)
	}
	c.Obs("server accepted %d TCP connection(s): %v", srv.Conns(), srv.ConnFrom)
	c.Check("go-h2-negotiated", allH2, "all four responses were HTTP/2.0")
	coalesced := srv.Conns() == 1
	c.Obs("Go transport coalesced a.test and b.test onto one connection: %v", coalesced)
	c.Check("go-dial-hook-saw-every-origin", len(hooks) >= 2 && containsAll(hooks, "a.test:", "b.test:"),
		"dial hook ran %d times, once per distinct origin; no origin was served without a hook call (conns=%d)", len(hooks), srv.Conns())
	c.Check("go-no-origin-served-without-hook", int(srv.Conns()) == len(hooks), "server connections=%d dial-hook calls=%d", srv.Conns(), len(hooks))
	tr.CloseIdleConnections()

	// concurrent first use of both origins
	var log2 []string
	srvBefore := srv.Conns()
	tr2 := newTransport(lab, &log2, &mu)
	cl2 := &http.Client{Transport: tr2, Timeout: 8 * time.Second}
	var wg sync.WaitGroup
	for _, n := range []string{"a.test", "b.test", "a.test", "b.test"} {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if resp, err := cl2.Get(fmt.Sprintf("https://%s:%d/ok", n, port)); err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}(n)
	}
	wg.Wait()
	mu.Lock()
	c.Obs("concurrent: dial-hook calls=%d, new server connections=%d", len(log2), srv.Conns()-srvBefore)
	mu.Unlock()
	c.Check("go-concurrent-hook-matches-connections", int(srv.Conns()-srvBefore) == len(log2), "every connection had a hook call")
	tr2.CloseIdleConnections()

	// lab off
	var log3 []string
	before := srv.Conns()
	tr3 := newTransport(egress.NewPolicy(false), &log3, &mu)
	_, err := (&http.Client{Transport: tr3, Timeout: 5 * time.Second}).Get(fmt.Sprintf("https://a.test:%d/ok", port))
	c.Check("lab-off/go-denied-no-connect", err != nil && srv.Conns() == before, "err=%v", err)

	// ---- curl
	curlRun := func(extra ...string) (string, string) {
		args := []string{"-q", "--noproxy", "*", "-sS", "--http2", "--cacert", caPath,
			"--resolve", fmt.Sprintf("a.test:%d:127.0.0.1", port), "--resolve", fmt.Sprintf("b.test:%d:127.0.0.1", port),
			"-w", "%{url} http=%{http_version} new_connects=%{num_connects} remote=%{remote_ip}:%{remote_port} local_port=%{local_port}\\n"}
		for _, u := range extra {
			if strings.HasPrefix(u, "https://") {
				args = append(args, "-o", "/dev/null", u) // -o applies per URL
			} else {
				args = append(args, u)
			}
		}
		cmd := exec.CommandContext(cx, "curl", args...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		var out, errb strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &errb
		cmd.Run()
		return out.String(), errb.String()
	}
	before = srv.Conns()
	out, errs := curlRun(fmt.Sprintf("https://a.test:%d/ok", port), fmt.Sprintf("https://b.test:%d/ok", port), fmt.Sprintf("https://a.test:%d/ok", port))
	c.Obs("curl (one process, 3 URLs a,b,a):\n%s%s", out, errs)
	c.Obs("server accepted %d new connection(s) from curl", srv.Conns()-before)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	curlCoalesced := srv.Conns()-before == 1
	c.Check("curl-observed", len(lines) == 3 && strings.Contains(out, "http=2"), "curl negotiated HTTP/2 and completed 3 transfers; coalesced across names=%v", curlCoalesced)
	_, verbose := curlRun("-v", fmt.Sprintf("https://a.test:%d/ok", port), fmt.Sprintf("https://b.test:%d/ok", port))
	for _, l := range strings.Split(verbose, "\n") {
		if strings.Contains(l, "Re-using") || strings.Contains(l, "Reusing") || strings.Contains(l, "Established") || strings.Contains(l, "Connected to") || strings.Contains(l, "connection #") || strings.Contains(l, "ALPN") || strings.Contains(l, "Found bundle") || strings.Contains(l, "Hostname") || strings.Contains(l, "multiplexed") {
			c.Obs("curl -v: %s", strings.TrimSpace(l))
		}
	}
	c.Obs("CONCLUSION (what was and was not shown): Go net/http %s keys its HTTP/2 pool by origin host:port and did not coalesce a.test/b.test (coalesced=%v). curl %s (single process) coalesced=%v. This says nothing about browsers, nghttp2-based daemons, or other curl versions/builds.", goVersion(), coalesced, "8.23.0", curlCoalesced)
	c.Verdict(fmt.Sprintf("Go: per-origin connections, hook sees every origin (coalesced=%v); curl coalesced=%v (observed only)", coalesced, curlCoalesced))
}

func field(body, key string) string {
	i := strings.Index(body, key)
	if i < 0 {
		return ""
	}
	return strings.Fields(body[i+len(key):])[0]
}

func containsAll(list []string, subs ...string) bool {
	for _, s := range subs {
		found := false
		for _, l := range list {
			if strings.Contains(l, s) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
