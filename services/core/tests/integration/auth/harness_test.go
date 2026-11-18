// Package auth_test runs the composed server against a real PostgreSQL cluster
// and checks the authentication behavior over HTTP: gate G-auth of the plan.
package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/passwordhash"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/bootstrap"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

const (
	webOrigin  = "http://127.0.0.1:3000"
	goodPass   = "correct horse battery staple"
	otherPass  = "another long passphrase 42"
	username   = "researcher"
	wrongPass  = "definitely not the password"
	fastMemory = 64
)

var fastParams = passwordhash.Params{MemoryKiB: fastMemory, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type app struct {
	t        *testing.T
	c        *pgtest.Cluster
	url      string
	addr     string
	srv      *bootstrap.Server
	logs     *safeBuffer
	runtime  string
	state    string
	evidence string
	cancel   context.CancelFunc
	done     chan error
}

type options struct {
	runtime  string
	state    string
	evidence string
	origins  []string
	hosts    []string
	params   *passwordhash.Params
}

func memoryRuntime(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/dev/shm", "ms-rt-")
	if err != nil {
		t.Skipf("no tmpfs directory for the bootstrap secret: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func start(t *testing.T, c *pgtest.Cluster, o options) *app {
	t.Helper()
	if o.runtime == "" {
		o.runtime = memoryRuntime(t)
	}
	if o.evidence == "" {
		o.evidence = filepath.Join(t.TempDir(), "evidence")
	}
	if o.state == "" {
		o.state = filepath.Join(t.TempDir(), "state")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	environ := []string{"HOME=/home/test", "MORNINGSTAR_DB_SOCKET_DIR=" + c.SocketDir, "MORNINGSTAR_STATE_DIR=" + o.state, "MORNINGSTAR_EVIDENCE_DIR=" + o.evidence, "XDG_RUNTIME_DIR=" + o.runtime}
	if len(o.origins) > 0 {
		environ = append(environ, "MORNINGSTAR_HTTP_ALLOWED_ORIGINS="+strings.Join(o.origins, ","))
	}
	cfg, err := config.Load(environ)
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	cfg.HTTP.Listen = netip.MustParseAddrPort(addr)
	cfg.HTTP.AllowedHosts = []string{addr, "localhost:" + strings.Split(addr, ":")[1]}
	cfg.HTTP.AllowedHosts = append(cfg.HTTP.AllowedHosts, o.hosts...)
	cfg.HTTP.ShutdownTimeout = 3 * time.Second
	params := fastParams
	if o.params != nil {
		params = *o.params
	}
	logs := &safeBuffer{}
	srv, err := bootstrap.NewServer(cfg, logs, bootstrap.WithPasswordParams(params))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{t: t, c: c, url: "http://" + addr, addr: addr, srv: srv, logs: logs, runtime: o.runtime, state: o.state, evidence: o.evidence, cancel: cancel, done: make(chan error, 1)}
	go func() { a.done <- srv.Serve(ctx, ln); close(a.done) }()
	t.Cleanup(a.stop)
	a.waitStarted()
	return a
}

func (a *app) stop() {
	a.cancel()
	select {
	case <-a.done:
	case <-time.After(10 * time.Second):
		a.t.Error("the server did not stop")
	}
}

// waitStarted waits for the startup check, which also prepares the bootstrap secret.
func (a *app) waitStarted() {
	a.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		r := newBrowser(a).do("GET", "/api/v1/session", nil)
		if r.status == 200 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	a.t.Fatal("the server did not finish its startup check")
}

func (a *app) secretPath() string { return filepath.Join(a.runtime, "morningstar", "bootstrap-secret") }

func (a *app) secret() string {
	a.t.Helper()
	b, err := os.ReadFile(a.secretPath())
	if err != nil {
		a.t.Fatalf("no bootstrap secret file: %v", err)
	}
	return strings.TrimSpace(string(b))
}

var httpClient = &http.Client{
	Transport:     &http.Transport{DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Timeout:       30 * time.Second,
}

type reply struct {
	status int
	body   []byte
	header http.Header
}

func (r *reply) json() map[string]any {
	var m map[string]any
	_ = json.Unmarshal(r.body, &m)
	return m
}

func (r *reply) code() string {
	e, _ := r.json()["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func (r *reply) str(path ...string) string {
	var cur any = r.json()
	for _, p := range path {
		m, _ := cur.(map[string]any)
		cur = m[p]
	}
	s, _ := cur.(string)
	return s
}

// browser keeps cookies and the CSRF token like a web page would.
type browser struct {
	a       *app
	cookies map[string]string
	csrf    string
	origin  string
}

func newBrowser(a *app) *browser {
	return &browser{a: a, cookies: map[string]string{}, origin: webOrigin}
}

type mod func(*http.Request)

func header(k, v string) mod { return func(r *http.Request) { r.Header.Set(k, v) } }
func noHeader(k string) mod  { return func(r *http.Request) { r.Header.Del(k) } }

func (b *browser) do(method, path string, body any, mods ...mod) *reply {
	b.a.t.Helper()
	var rd io.Reader
	if body != nil {
		var buf bytes.Buffer
		switch v := body.(type) {
		case string:
			buf.WriteString(v)
		default:
			if err := json.NewEncoder(&buf).Encode(v); err != nil {
				b.a.t.Fatal(err)
			}
		}
		rd = &buf
	}
	req, err := http.NewRequest(method, b.a.url+path, rd)
	if err != nil {
		b.a.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != "GET" {
		req.Header.Set("Origin", b.origin)
		if b.csrf != "" {
			req.Header.Set("X-CSRF-Token", b.csrf)
		}
	}
	for k, v := range b.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	for _, m := range mods {
		m(req)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		b.a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	for _, c := range resp.Cookies() {
		if c.MaxAge < 0 || c.Value == "" {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c.Value
		}
	}
	return &reply{status: resp.StatusCode, body: data, header: resp.Header}
}

// state loads GET /session and keeps the CSRF token it returns.
func (b *browser) state() *reply {
	r := b.do("GET", "/api/v1/session", nil)
	if t := r.str("csrfToken"); t != "" {
		b.csrf = t
	}
	return r
}

func (b *browser) setup(secret, user, pass string) *reply {
	b.state()
	r := b.do("POST", "/api/v1/setup/account", map[string]any{"bootstrapSecret": secret, "username": user, "password": pass})
	if t := r.str("csrfToken"); t != "" {
		b.csrf = t
	}
	return r
}

func (b *browser) login(user, pass string) *reply {
	b.state()
	r := b.do("POST", "/api/v1/session/login", map[string]any{"username": user, "password": pass})
	if t := r.str("csrfToken"); t != "" {
		b.csrf = t
	}
	return r
}

func (b *browser) post(path string, body any, mods ...mod) *reply {
	return b.do("POST", path, body, mods...)
}

// fresh returns a cluster with a new database and an app with an account.
func fresh(t *testing.T) (*pgtest.Cluster, *app, *browser, string) {
	t.Helper()
	c := pgtest.Get(t)
	c.Fresh(t)
	a := start(t, c, options{})
	b := newBrowser(a)
	r := b.setup(a.secret(), username, goodPass)
	if r.status != 201 {
		t.Fatalf("setup = %d %s", r.status, r.body)
	}
	return c, a, b, r.str("recoveryCode")
}

func (a *app) scalar(sql string, args ...any) string {
	a.t.Helper()
	var v string
	if err := a.c.Owner(a.t).QueryRow(a.t.Context(), "SELECT ("+sql+")::text", args...).Scan(&v); err != nil {
		a.t.Fatalf("%s: %v", sql, err)
	}
	return v
}

func (a *app) exec(sql string, args ...any) {
	a.t.Helper()
	if _, err := a.c.Owner(a.t).Exec(a.t.Context(), sql, args...); err != nil {
		a.t.Fatalf("%s: %v", sql, err)
	}
}
