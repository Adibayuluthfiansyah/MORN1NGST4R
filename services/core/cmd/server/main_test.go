package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func TestRunRefusesInvalidConfiguration(t *testing.T) {
	err := run(context.Background(), []string{"MORNINGSTAR_HTTP_LISTEN=0.0.0.0:8080"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "MORNINGSTAR_HTTP_LISTEN") {
		t.Errorf("run error = %v, want one naming MORNINGSTAR_HTTP_LISTEN", err)
	}
}

// With no database running, the server still serves: it answers live and
// reports not ready (D-86, NFR-OBS-003). The full database tests are in
// tests/integration/server.
func TestRunServesUntilCancelled(t *testing.T) {
	addr := freePort(t)
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	environ := []string{
		"MORNINGSTAR_HTTP_LISTEN=" + addr,
		"MORNINGSTAR_DB_SOCKET_DIR=" + t.TempDir(), // no database listens here
		"MORNINGSTAR_STATE_DIR=" + filepath.Join(t.TempDir(), "state"),
		"MORNINGSTAR_EVIDENCE_DIR=" + filepath.Join(t.TempDir(), "evidence"),
	}
	go func() { done <- run(ctx, environ, &logs) }()

	// No keep-alive: a second request would otherwise leave a connection that
	// the client dialed and never used, and graceful shutdown waits for it.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	var resp *http.Response
	var err error
	for i := 0; i < 50; i++ {
		resp, err = client.Get("http://" + addr + "/api/v1/health/ready")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not come up: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("ready status = %d, want 503 without a database", resp.StatusCode)
	}
	live, err := client.Get("http://" + addr + "/api/v1/health/live")
	if err != nil {
		t.Fatal(err)
	}
	live.Body.Close()
	if live.StatusCode != http.StatusOK {
		t.Errorf("live status = %d, want 200", live.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v after cancellation, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("run did not return after cancellation. Logs:\n%s", logs.String())
	}
}
