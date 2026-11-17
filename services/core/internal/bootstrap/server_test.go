package bootstrap_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/bootstrap"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
)

// testConfig returns a valid configuration for the listener's address.
func testConfig(t *testing.T, ln net.Listener) config.Config {
	t.Helper()
	cfg, err := config.Load([]string{"HOME=/home/test"})
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	cfg.HTTP.Listen = netip.MustParseAddrPort(addr)
	cfg.HTTP.AllowedHosts = []string{addr}
	cfg.HTTP.ShutdownTimeout = 2 * time.Second
	return cfg
}

// newServer composes a server without a database: these tests are about the
// HTTP layer. The database wiring is tested in tests/integration/server.
func newServer(t *testing.T, cfg config.Config, w io.Writer) *bootstrap.Server {
	t.Helper()
	srv, err := bootstrap.NewServer(cfg, w, bootstrap.WithoutDatabase())
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

type running struct {
	url    string
	cancel context.CancelFunc
	done   chan error
	logs   *bytes.Buffer
}

func start(t *testing.T, mutate func(*config.Config)) running {
	t.Helper()
	ln := listen(t)
	cfg := testConfig(t, ln)
	if mutate != nil {
		mutate(&cfg)
	}
	var logs bytes.Buffer
	srv := newServer(t, cfg, &logs)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	return running{url: "http://" + ln.Addr().String(), cancel: cancel, done: done, logs: &logs}
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(body))
}

func TestServesHealthEndpointsOverHTTP(t *testing.T) {
	s := start(t, nil)
	if code, body := get(t, s.url+"/api/v1/health/live"); code != 200 || body != `{"status":"ok"}` {
		t.Errorf("live = %d %s", code, body)
	}
	if code, body := get(t, s.url+"/api/v1/health/ready"); code != 200 || body != `{"status":"ready"}` {
		t.Errorf("ready = %d %s", code, body)
	}
}

func TestHostAllowListIsEnforcedOnTheWire(t *testing.T) {
	s := start(t, nil)
	req, _ := http.NewRequest(http.MethodGet, s.url+"/api/v1/health/live", nil)
	req.Host = "evil.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

func TestOversizedHeadersAreRefused(t *testing.T) {
	s := start(t, nil)
	req, _ := http.NewRequest(http.MethodGet, s.url+"/api/v1/health/live", nil)
	req.Header.Set("X-Padding", strings.Repeat("a", 64<<10))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("status = %d, want 431", resp.StatusCode)
	}
}

func TestSlowHeaderConnectionIsClosed(t *testing.T) {
	s := start(t, func(c *config.Config) { c.HTTP.ReadHeaderTimeout = 150 * time.Millisecond })
	conn, err := net.Dial("tcp", strings.TrimPrefix(s.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET /api/v1/health/live HTTP/1.1\r\nHost: x\r\n")); err != nil { // header never finished
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	began := time.Now()
	_, err = io.ReadAll(conn)
	if elapsed := time.Since(began); elapsed > 2*time.Second {
		t.Errorf("connection stayed open for %v (err=%v), want it closed near the read-header timeout", elapsed, err)
	}
}

func TestGracefulShutdown(t *testing.T) {
	s := start(t, nil)
	if code, _ := get(t, s.url+"/api/v1/health/live"); code != 200 {
		t.Fatalf("not serving before shutdown: %d", code)
	}
	s.cancel()
	select {
	case err := <-s.done:
		if err != nil {
			t.Errorf("Serve returned %v after a clean shutdown, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the context was cancelled")
	}
	if conn, err := net.DialTimeout("tcp", strings.TrimPrefix(s.url, "http://"), time.Second); err == nil {
		conn.Close()
		t.Error("the port still accepts connections after shutdown")
	}
}

func TestListenAndServeRefusesANonLoopbackAddress(t *testing.T) {
	cfg, err := config.Load([]string{"HOME=/home/test"})
	if err != nil {
		t.Fatal(err)
	}
	// A hand-built configuration bypasses config.Load. The server must still
	// fail closed before it binds anything.
	cfg.HTTP.Listen = netip.MustParseAddrPort("0.0.0.0:0")
	srv := newServer(t, cfg, io.Discard)
	if err := srv.ListenAndServe(context.Background()); err == nil {
		t.Error("ListenAndServe succeeded on 0.0.0.0, want an error")
	}
}

func TestServeRefusesANonLoopbackListener(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("cannot listen on the wildcard address here: %v", err)
	}
	defer ln.Close()
	cfg, _ := config.Load([]string{"HOME=/home/test"})
	srv := newServer(t, cfg, io.Discard)
	if err := srv.Serve(context.Background(), ln); err == nil {
		t.Error("Serve accepted a wildcard listener, want an error")
	}
}

func TestAccessLogGoesToTheGivenWriterWithoutQueryValues(t *testing.T) {
	s := start(t, func(c *config.Config) { c.Log.Level = -4 }) // debug
	get(t, s.url+"/api/v1/health/live?token=abc")
	out := s.logs.String()
	if !strings.Contains(out, `"route":"/api/v1/health/live"`) {
		t.Errorf("no access record in the log: %s", out)
	}
	if strings.Contains(out, "abc") {
		t.Errorf("query value leaked: %s", out)
	}
}
