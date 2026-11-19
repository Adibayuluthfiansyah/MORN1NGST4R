package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunRefusesInvalidConfiguration(t *testing.T) {
	err := run(context.Background(), []string{"MORNINGSTAR_HTTP_LISTEN=0.0.0.0:8080"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MORNINGSTAR_HTTP_LISTEN") {
		t.Errorf("run error = %v, want one naming MORNINGSTAR_HTTP_LISTEN", err)
	}
}

// With no database, the worker starts, claims nothing and keeps repeating the
// recovery check. It stops cleanly when it is cancelled.
func TestRunWaitsForTheDatabaseAndStopsWhenCancelled(t *testing.T) {
	var logs syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	environ := []string{
		"MORNINGSTAR_DB_SOCKET_DIR=" + t.TempDir(), // no database listens here
		"MORNINGSTAR_STATE_DIR=" + filepath.Join(t.TempDir(), "state"),
		"MORNINGSTAR_EVIDENCE_DIR=" + filepath.Join(t.TempDir(), "evidence"),
	}
	go func() { done <- run(ctx, environ, &logs) }()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "the recovery check failed") {
		if time.Now().After(deadline) {
			t.Fatalf("no recovery check was logged: %s", logs.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Contains(logs.String(), "worker starting") {
		t.Error("the worker started without a recovery check that passed")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v after cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not stop")
	}
}
