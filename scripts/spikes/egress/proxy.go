package egress

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Proxy is the minimal policy proxy: CONNECT and absolute-URI HTTP.
// It resolves once, evaluates every address, pins the validated address
// and enforces the port rule. It never follows redirects.
type Proxy struct {
	Policy   *Policy
	Resolver *net.Resolver

	mu        sync.Mutex
	Decisions []string
}

func (p *Proxy) logf(f string, a ...any) {
	line := fmt.Sprintf(f, a...)
	p.mu.Lock()
	p.Decisions = append(p.Decisions, line)
	p.mu.Unlock()
	fmt.Printf("[proxy] %s\n", line)
}

// Log returns a copy of the decision log.
func (p *Proxy) Log() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.Decisions...)
}

// Serve runs the proxy on ln until it is closed.
func (p *Proxy) Serve(ln net.Listener) *http.Server {
	srv := &http.Server{Handler: p, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	go srv.Serve(ln)
	return srv
}

var hopByHop = []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

func headerNames(h http.Header) []string {
	var n []string
	for k := range h {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	p.forward(w, r)
}

type countFirst struct {
	r     io.Reader
	n     atomic.Int64
	first atomic.Int32 // first byte + 1, 0 = none
}

func (c *countFirst) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	if n > 0 {
		c.first.CompareAndSwap(0, int32(b[0])+1)
		c.n.Add(int64(n))
	}
	return n, err
}

func (p *Proxy) connect(w http.ResponseWriter, r *http.Request) {
	host, ps, err := net.SplitHostPort(r.Host)
	port, perr := strconv.Atoi(ps)
	if err != nil || perr != nil {
		p.logf("CONNECT %q DENY malformed authority", r.Host)
		http.Error(w, "denied: malformed CONNECT authority", http.StatusBadRequest)
		return
	}
	t := p.Policy.Evaluate(r.Context(), p.Resolver, host, port)
	if !t.Allowed {
		p.logf("CONNECT %s:%d DENY %s", host, port, t.Reason)
		http.Error(w, "denied: "+t.Reason, http.StatusForbidden)
		return
	}
	st := &DialStats{}
	up, err := p.Policy.PinnedDial(t, st)(r.Context(), "tcp", r.Host)
	if err != nil {
		p.logf("CONNECT %s:%d ALLOW(addrs=%v) but dial failed: %v", host, port, t.Addrs, err)
		http.Error(w, "upstream dial failed", http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		up.Close()
		http.Error(w, "no hijack", 500)
		return
	}
	cc, brw, err := hj.Hijack()
	if err != nil {
		up.Close()
		return
	}
	p.logf("CONNECT %s:%d ALLOW addrs=%v remote=%s (request headers seen: %v)", host, port, t.Addrs, up.RemoteAddr(), headerNames(r.Header))
	cc.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	deadline := time.Now().Add(30 * time.Second)
	cc.SetDeadline(deadline)
	up.SetDeadline(deadline)
	var c2s io.Reader = cc
	if brw.Reader.Buffered() > 0 {
		c2s = io.MultiReader(brw.Reader, cc)
	}
	in := &countFirst{r: c2s}
	out := &countFirst{r: up}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(up, in)
		if tc, ok := up.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		io.Copy(cc, out)
		if tc, ok := cc.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	wg.Wait()
	cc.Close()
	up.Close()
	kind := "none"
	if f := in.first.Load(); f != 0 {
		kind = fmt.Sprintf("0x%02x", f-1)
		if f-1 == 0x16 {
			kind += " (TLS handshake record; contents not parsed, SNI not inspected)"
		}
	}
	p.logf("TUNNEL %s:%d closed c2s_bytes=%d s2c_bytes=%d first_client_byte=%s", host, port, in.n.Load(), out.n.Load(), kind)
}

func (p *Proxy) forward(w http.ResponseWriter, r *http.Request) {
	if !r.URL.IsAbs() || r.URL.Scheme != "http" {
		p.logf("%s %q DENY not an absolute-URI http proxy request (use CONNECT for https)", r.Method, r.RequestURI)
		http.Error(w, "denied: proxy accepts CONNECT or absolute-URI http only", http.StatusBadRequest)
		return
	}
	u, host, port, err := CanonicalURL(r.URL.String())
	if err != nil {
		p.logf("%s %q DENY %v", r.Method, r.RequestURI, err)
		http.Error(w, "denied: "+err.Error(), http.StatusBadRequest)
		return
	}
	t := p.Policy.Evaluate(r.Context(), p.Resolver, host, port)
	if !t.Allowed {
		p.logf("%s %s DENY %s", r.Method, u.String(), t.Reason)
		http.Error(w, "denied: "+t.Reason, http.StatusForbidden)
		return
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.URL = u
	// net/http's server already discarded the client's Host header for an
	// absolute-form request. The upstream Host is always the canonical
	// authority of the evaluated target.
	out.Host = u.Host
	for _, h := range hopByHop {
		out.Header.Del(h)
	}
	st := &DialStats{}
	tr := &http.Transport{Proxy: nil, DialContext: p.Policy.PinnedDial(t, st), DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{}, ResponseHeaderTimeout: 10 * time.Second}
	defer tr.CloseIdleConnections()
	resp, err := tr.RoundTrip(out)
	if err != nil {
		p.logf("%s %s ALLOW addrs=%v but upstream failed: %v", r.Method, u.String(), t.Addrs, err)
		http.Error(w, "upstream failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	p.logf("%s %s ALLOW addrs=%v upstream_host_sent=%s status=%d", r.Method, u.String(), t.Addrs, out.Host, resp.StatusCode)
	for _, h := range hopByHop {
		resp.Header.Del(h)
	}
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode) // redirects are passed through, never followed
	io.Copy(w, io.LimitReader(resp.Body, 1<<20))
}

// ProxyHeadersString helps tests print what a request carried.
func ProxyHeadersString(h http.Header) string { return strings.Join(headerNames(h), ",") }
