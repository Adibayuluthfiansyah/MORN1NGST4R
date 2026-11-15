package egress

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DialStats records what a pinned dialer did.
type DialStats struct {
	mu     sync.Mutex
	Events []string
}

func (s *DialStats) add(f string, a ...any) {
	s.mu.Lock()
	s.Events = append(s.Events, fmt.Sprintf(f, a...))
	s.mu.Unlock()
}
func (s *DialStats) Snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.Events...)
}

// PinnedDial returns a DialContext that ignores the (name) address it is
// given and dials only the validated addresses of t, sequentially. Control
// re-checks the real remote address of every connection attempt as
// defence in depth. There is no DNS lookup in this function.
func (p *Policy) PinnedDial(t *Target, st *DialStats) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !t.Allowed || len(t.Addrs) == 0 {
			return nil, fmt.Errorf("target not allowed")
		}
		var firstErr error
		for _, a := range t.Addrs {
			d := net.Dialer{
				Timeout: 3 * time.Second,
				Resolver: &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
					return nil, fmt.Errorf("DNS forbidden in pinned dial")
				}},
				ControlContext: func(ctx context.Context, network, address string, c syscall.RawConn) error {
					ap, err := netip.ParseAddrPort(address)
					if err != nil {
						return fmt.Errorf("control: unparseable %q", address)
					}
					ip := ap.Addr().Unmap()
					validated := false
					for _, v := range t.Addrs {
						if v == ip {
							validated = true
						}
					}
					if !validated || int(ap.Port()) != t.Port {
						st.add("control DENY %s (not the validated address)", address)
						return fmt.Errorf("control: %s is not a validated address", address)
					}
					if d := p.CheckAddr(ip, int(ap.Port())); !d.Allowed {
						st.add("control DENY %s (%s)", address, d.Reason)
						return fmt.Errorf("control: %s", d.Reason)
					}
					st.add("control ALLOW %s", address)
					return nil
				},
			}
			c, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(a, uint16(t.Port)).String())
			if err == nil {
				st.add("dial %s (requested %q) -> connected", netip.AddrPortFrom(a, uint16(t.Port)), addr)
				return c, nil
			}
			st.add("dial %s failed: %v", netip.AddrPortFrom(a, uint16(t.Port)), err)
			if firstErr == nil {
				firstErr = err
			}
		}
		return nil, firstErr
	}
}

// ---------------------------------------------------------- URL handling

// CanonicalURL parses raw into a canonical http(s) URL. Anything ambiguous
// is refused.
func CanonicalURL(raw string) (*url.URL, string, int, error) {
	if raw == "" {
		return nil, "", 0, fmt.Errorf("empty URL")
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c <= 0x20 || c == 0x7f || c == '\\' {
			return nil, "", 0, fmt.Errorf("control/space/backslash in URL (ambiguous)")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", 0, fmt.Errorf("unparseable: %v", err)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", 0, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Opaque != "" || u.Host == "" {
		return nil, "", 0, fmt.Errorf("opaque or host-less URL")
	}
	if u.User != nil {
		return nil, "", 0, fmt.Errorf("userinfo not allowed")
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, "", 0, fmt.Errorf("empty port")
	}
	host, port := u.Hostname(), 0
	if ps := u.Port(); ps != "" {
		n, err := strconv.Atoi(ps)
		if err != nil || n < 1 || n > 65535 {
			return nil, "", 0, fmt.Errorf("bad port %q", ps)
		}
		port = n
	} else if u.Scheme == "https" {
		port = 443
	} else {
		port = 80
	}
	canon, _, isIP, err := CanonicalHost(host)
	if err != nil {
		return nil, "", 0, err
	}
	hostport := canon
	if isIP && strings.Contains(canon, ":") {
		hostport = "[" + canon + "]"
	}
	if u.Port() != "" {
		hostport += ":" + strconv.Itoa(port)
	}
	u.Host = hostport
	u.Fragment = ""
	return u, canon, port, nil
}

// ResolveLocation resolves a Location header against cur. Forms that
// parsers disagree on (backslashes, whitespace, control characters,
// opaque URIs) are refused rather than guessed.
func ResolveLocation(cur *url.URL, loc string) (string, error) {
	if loc == "" {
		return "", fmt.Errorf("empty Location")
	}
	for i := 0; i < len(loc); i++ {
		c := loc[i]
		if c <= 0x20 || c == 0x7f || c == '\\' {
			return "", fmt.Errorf("control/space/backslash in Location (ambiguous)")
		}
	}
	ref, err := url.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("unparseable Location: %v", err)
	}
	if ref.Opaque != "" {
		return "", fmt.Errorf("opaque Location")
	}
	return cur.ResolveReference(ref).String(), nil
}

// ------------------------------------------------------------------ client

// Hop is one step of a fetch.
type Hop struct {
	N        int
	URL      string
	Decision string // ALLOW / DENY
	Reason   string
	Addrs    []string
	Remote   string // actual remote addr of the connection
	Status   int
	Location string
	Next     string // "followed", "refused: ...", "final"
}

func (h Hop) String() string {
	return fmt.Sprintf("hop%d %s -> %s (%s) addrs=%v remote=%s status=%d loc=%q next=%s",
		h.N, h.URL, h.Decision, h.Reason, h.Addrs, h.Remote, h.Status, h.Location, h.Next)
}

// Result of Fetch.
type Result struct {
	Hops  []Hop
	Body  string
	Final bool // a final non-redirect response was received
	Err   error
	Stats *DialStats
}

// Client is the first-party egress-enforcing HTTP client.
type Client struct {
	Policy   *Policy
	Resolver *net.Resolver
	RootCAs  *x509.CertPool
	MaxHops  int
	// RawRoundTrip bypasses http.Client and uses Transport.RoundTrip, so
	// the loop sees redirect responses that http.Client would turn into errors.
	RawRoundTrip bool
}

var redirectCodes = map[int]bool{301: true, 302: true, 303: true, 307: true, 308: true}

// Fetch performs GET with a manual redirect loop. Every hop is
// canonicalized, resolved once, fully evaluated and then dialed at the
// validated address. Connection reuse across hops is impossible (new
// transport per hop, keep-alives off).
func (c *Client) Fetch(ctx context.Context, raw string) *Result {
	res := &Result{Stats: &DialStats{}}
	max := c.MaxHops
	if max == 0 {
		max = 5
	}
	visited := map[string]bool{}
	cur := raw
	for n := 1; ; n++ {
		hop := Hop{N: n, URL: cur}
		u, host, port, err := CanonicalURL(cur)
		if err != nil {
			hop.Decision, hop.Reason, hop.Next = "DENY", err.Error(), "refused: not evaluable"
			res.Hops = append(res.Hops, hop)
			res.Err = fmt.Errorf("denied: %v", err)
			return res
		}
		hop.URL = u.String()
		if visited[hop.URL] {
			hop.Decision, hop.Reason, hop.Next = "DENY", "redirect loop", "refused: loop"
			res.Hops = append(res.Hops, hop)
			res.Err = fmt.Errorf("denied: redirect loop at %s", hop.URL)
			return res
		}
		visited[hop.URL] = true
		if n > max {
			hop.Decision, hop.Reason, hop.Next = "DENY", "too many redirects", "refused: hop limit"
			res.Hops = append(res.Hops, hop)
			res.Err = fmt.Errorf("denied: too many redirects")
			return res
		}
		t := c.Policy.Evaluate(ctx, c.Resolver, host, port)
		for _, a := range t.Addrs {
			hop.Addrs = append(hop.Addrs, a.String())
		}
		if !t.Allowed {
			hop.Decision, hop.Reason, hop.Next = "DENY", t.Reason, "refused: policy"
			res.Hops = append(res.Hops, hop)
			res.Err = fmt.Errorf("denied: %s", t.Reason)
			return res
		}
		hop.Decision, hop.Reason = "ALLOW", t.Reason

		tr := &http.Transport{
			Proxy:                 nil, // never honour proxy env vars
			DialContext:           c.Policy.PinnedDial(t, res.Stats),
			TLSClientConfig:       &tls.Config{RootCAs: c.RootCAs}, // ServerName = canonical host from URL
			ForceAttemptHTTP2:     true,
			DisableKeepAlives:     true,
			ResponseHeaderTimeout: 5 * time.Second,
		}
		cl := &http.Client{Transport: tr, Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		trace := &httptrace.ClientTrace{GotConn: func(i httptrace.GotConnInfo) { hop.Remote = i.Conn.RemoteAddr().String() }}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
		var resp *http.Response
		if c.RawRoundTrip {
			// No http.Client at all: our own code sees every Location value.
			rc, cancel := context.WithTimeout(req.Context(), 10*time.Second)
			defer cancel()
			resp, err = tr.RoundTrip(req.WithContext(rc))
		} else {
			resp, err = cl.Do(req)
		}
		if err != nil {
			tr.CloseIdleConnections()
			hop.Next = "request failed: " + err.Error()
			res.Hops = append(res.Hops, hop)
			res.Err = err
			return res
		}
		hop.Status = resp.StatusCode
		if redirectCodes[resp.StatusCode] {
			hop.Location = resp.Header.Get("Location")
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			tr.CloseIdleConnections()
			next, err := ResolveLocation(u, hop.Location)
			if err != nil {
				hop.Next = "refused: " + err.Error()
				res.Hops = append(res.Hops, hop)
				res.Err = fmt.Errorf("redirect not followed: %v", err)
				return res
			}
			hop.Next = "redirect to " + next + " (evaluated as next hop)"
			res.Hops = append(res.Hops, hop)
			cur = next
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		tr.CloseIdleConnections()
		hop.Next = "final"
		res.Hops = append(res.Hops, hop)
		res.Body, res.Final = string(b), true
		return res
	}
}
