package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
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

func TestRunServesUntilCancelled(t *testing.T) {
	addr := freePort(t)
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx, []string{"MORNINGSTAR_HTTP_LISTEN=" + addr}, &logs) }()

	var resp *http.Response
	var err error
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/api/v1/health/ready")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not come up: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("ready status = %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v after cancellation, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancellation")
	}
}
