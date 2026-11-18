// Package server_test runs the composed server against a real PostgreSQL
// cluster: the database readiness probe (D-86, NFR-OBS-003) and the recovery
// check at startup.
package server_test

import (
	"bytes"
	"context"
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

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/bootstrap"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

// safeBuffer is a log sink that the server and the test can both use.
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

type running struct {
	url    string
	srv    *bootstrap.Server
	logs   *safeBuffer
	done   chan error
	cancel context.CancelFunc
	state  string
}

func startServer(t *testing.T, c *pgtest.Cluster, state string) running {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load([]string{"HOME=/home/test", "MORNINGSTAR_DB_SOCKET_DIR=" + c.SocketDir, "MORNINGSTAR_STATE_DIR=" + state, "MORNINGSTAR_EVIDENCE_DIR=" + filepath.Join(t.TempDir(), "evidence")})
	if err != nil {
		t.Fatal(err)
	}
	cfg.HTTP.Listen = netip.MustParseAddrPort(ln.Addr().String())
	cfg.HTTP.AllowedHosts = []string{ln.Addr().String()}
	cfg.HTTP.ShutdownTimeout = 2 * time.Second
	logs := &safeBuffer{}
	srv, err := bootstrap.NewServer(cfg, logs)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the server did not stop")
		}
	})
	return running{url: "http://" + ln.Addr().String(), srv: srv, logs: logs, done: done, cancel: cancel, state: state}
}

var client = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

func waitReady(t *testing.T, r running, wantCode int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var code int
	var body string
	for time.Now().Before(deadline) {
		resp, err := client.Get(r.url + "/api/v1/health/ready")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			code, body = resp.StatusCode, strings.TrimSpace(string(b))
			if code == wantCode {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("ready = %d %s, want %d", code, body, wantCode)
}

func newState(t *testing.T) string { return filepath.Join(t.TempDir(), "state") }

// Acceptance of M3: a failing database gives 503 not_ready on /ready while /live
// stays 200, and the server recovers when the database returns.
func TestReadinessFollowsTheDatabase(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	r := startServer(t, c, newState(t))

	waitReady(t, r, 200)
	if code, body := get(t, r.url+"/api/v1/health/ready"); code != 200 || body != `{"status":"ready"}` {
		t.Errorf("ready = %d %s", code, body)
	}
	if code, body := get(t, r.url+"/api/v1/health/live"); code != 200 || body != `{"status":"ok"}` {
		t.Errorf("live = %d %s", code, body)
	}

	c.Stop(t)
	waitReady(t, r, 503)
	if code, body := get(t, r.url+"/api/v1/health/ready"); code != 503 || body != `{"status":"not_ready"}` {
		t.Errorf("ready with the database down = %d %s, want 503 {\"status\":\"not_ready\"}", code, body)
	}
	if code, body := get(t, r.url+"/api/v1/health/live"); code != 200 || body != `{"status":"ok"}` {
		t.Errorf("live with the database down = %d %s, want 200", code, body)
	}

	c.Start(t)
	waitReady(t, r, 200)

	// The health response and the log never carry the socket path, the role or a driver message.
	for _, secret := range []string{c.SocketDir, "ms_api", "FATAL", "connect:"} {
		if strings.Contains(r.logs.String(), secret) {
			t.Errorf("the log contains %q", secret)
		}
	}
}

// The server starts when the database is down, answers live, and runs its
// recovery check when the database comes up.
func TestServerStartsWithTheDatabaseDown(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	c.Stop(t)
	r := startServer(t, c, newState(t))

	if code, _ := get(t, r.url+"/api/v1/health/live"); code != 200 {
		t.Errorf("live = %d, want 200", code)
	}
	if code, body := get(t, r.url+"/api/v1/health/ready"); code != 503 || body != `{"status":"not_ready"}` {
		t.Errorf("ready = %d %s, want 503 not_ready", code, body)
	}
	if _, ok := r.srv.Recovery().Generation(); ok {
		t.Error("claims would be allowed before the recovery check ran")
	}

	c.Start(t)
	waitReady(t, r, 200)
	pgtest.Eventually(t, 15*time.Second, func() bool {
		gen, ok := r.srv.Recovery().Generation()
		return ok && gen == 1
	})
}

// The first start of an installation initializes both halves, and a second
// start of the same installation is normal.
func TestServerRunsTheFirstRunAndLaterStartsAreNormal(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	state := newState(t)
	r := startServer(t, c, state)
	waitReady(t, r, 200)
	if gen, ok := r.srv.Recovery().Generation(); !ok || gen != 1 {
		t.Fatalf("Generation = %d, %v after the first run", gen, ok)
	}
	if _, err := os.Stat(filepath.Join(state, "recovery-generation.json")); err != nil {
		t.Errorf("the marker was not written: %v", err)
	}
	r.cancel()
	<-r.done

	second := startServer(t, c, state)
	waitReady(t, second, 200)
	if gen, ok := second.srv.Recovery().Generation(); !ok || gen != 1 {
		t.Errorf("second start: Generation = %d, %v", gen, ok)
	}
}

// With the marker gone from a populated installation the server still answers
// live and ready (the database is fine) and reports recovery required.
func TestServerReportsRecoveryRequired(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	state := newState(t)
	r := startServer(t, c, state)
	waitReady(t, r, 200)
	r.cancel()
	<-r.done
	if err := os.Remove(filepath.Join(state, "recovery-generation.json")); err != nil {
		t.Fatal(err)
	}

	second := startServer(t, c, state)
	waitReady(t, second, 200)
	st := second.srv.Recovery().Status()
	if !st.Required || st.Reason != recovery.ReasonMarkerMissing {
		t.Errorf("Status = %+v, want recovery required (marker_missing)", st)
	}
	if _, ok := second.srv.Recovery().Generation(); ok {
		t.Error("claims would be allowed while recovery is required")
	}
	if !strings.Contains(second.logs.String(), "recovery is required") {
		t.Error("the server did not log that recovery is required")
	}
}

// First run needs the marker. When it cannot be written the server must not serve.
func TestServerDoesNotServeWhenTheFirstRunMarkerCannotBeWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	c := pgtest.Get(t)
	c.Fresh(t)
	state := newState(t)
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, 0o500); err != nil { // readable, not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(state, 0o700) })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load([]string{"HOME=/home/test", "MORNINGSTAR_DB_SOCKET_DIR=" + c.SocketDir, "MORNINGSTAR_STATE_DIR=" + state, "MORNINGSTAR_EVIDENCE_DIR=" + filepath.Join(t.TempDir(), "evidence")})
	if err != nil {
		t.Fatal(err)
	}
	cfg.HTTP.Listen = netip.MustParseAddrPort(ln.Addr().String())
	cfg.HTTP.AllowedHosts = []string{ln.Addr().String()}
	srv, err := bootstrap.NewServer(cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Serve(t.Context(), ln); err == nil {
		t.Fatal("the server served although the first-run marker could not be written")
	}
	// Nothing was committed: the next start can still do the first run.
	var n int
	if err := c.Owner(t).QueryRow(t.Context(), `SELECT count(*) FROM morningstar.recovery_state`).Scan(&n); err != nil || n != 0 {
		t.Errorf("recovery_state rows = %d (%v), want 0", n, err)
	}
}
