package worker_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/recoverymarker"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/bootstrap"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

type logSink struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logSink) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// The production composition of the worker, over a real cluster: the roles and
// grants that it needs are enough, it refuses to claim what it cannot evaluate,
// and its maintenance purges what is past the floor.
func TestTheComposedWorkerRunsItsMaintenanceAndRefusesWhatItCannotEvaluate(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	state := filepath.Join(t.TempDir(), "state")
	evidenceDir := filepath.Join(t.TempDir(), "evidence")

	// The server role initializes the installation. The worker role cannot.
	store, err := recoverymarker.New(state)
	if err != nil {
		t.Fatal(err)
	}
	srv := recovery.NewService(recovery.Deps{Markers: store, Repo: persistence.NewRecovery(c.Pool(t, persistence.RoleAPI, 2)), NewID: ids.New, Now: time.Now})
	if st, err := srv.Check(t.Context()); err != nil || st.Required {
		t.Fatalf("first run: %+v, %v", st, err)
	}
	b := c.SeedBaseWithoutRecovery(t)

	// Seeds: a job that nothing can evaluate, an attempt older than the floor and
	// an orphan file older than its floor.
	a := c.SeedApproval(t, b, pgtest.ApprovalSpec{})
	jobID := c.SeedJob(t, b, a, "low_impact", 1)
	if _, err := c.Owner(t).Exec(t.Context(), `INSERT INTO morningstar.login_attempts (id, kind, source_key, succeeded, attempted_at) VALUES (uuidv7(), 'login', 'k', false, now() - interval '45 days')`); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(evidenceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(evidenceDir, ids.New())
	if err := os.WriteFile(orphan, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load([]string{
		"MORNINGSTAR_DB_SOCKET_DIR=" + c.SocketDir, "MORNINGSTAR_STATE_DIR=" + state, "MORNINGSTAR_EVIDENCE_DIR=" + evidenceDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	var logs logSink
	w, err := bootstrap.NewWorker(cfg, &logs)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run returned %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("the worker did not stop")
		}
	}()

	t.Cleanup(func() {
		if t.Failed() {
			t.Log(logs.String())
		}
	})
	count := func(sql string, args ...any) int {
		var n int
		if err := c.Owner(t).QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	until(t, 10*time.Second, "the maintenance purged the old attempt", func() bool {
		return count(`SELECT count(*) FROM morningstar.login_attempts`) == 0
	})
	until(t, 10*time.Second, "the sweep removed the orphan", func() bool {
		_, err := os.Lstat(orphan)
		return os.IsNotExist(err)
	})
	if n := count(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'retention.purged' AND outcome = 'success'`); n != 1 {
		t.Errorf("retention.purged events = %d, want 1", n)
	}

	// The queued job is refused: no source of the evaluator inputs exists, and the
	// runner that exists refuses. The job never runs.
	until(t, 15*time.Second, "the job left the queue", func() bool {
		return count(`SELECT count(*) FROM morningstar.jobs WHERE id = $1 AND state = 'queued'`, jobID) == 0
	})
	if n := count(`SELECT count(*) FROM morningstar.job_attempts WHERE job_id = $1`, jobID); n != 0 {
		t.Errorf("attempts = %d, want none", n)
	}
	if n := count(`SELECT count(*) FROM morningstar.jobs WHERE id = $1 AND state IN ('failed', 'cancelled')`, jobID); n != 1 {
		t.Errorf("the job did not end failed or cancelled")
	}
	if n := count(`SELECT count(*) FROM morningstar.policy_decisions WHERE job_id = $1 AND outcome = 'deny'`, jobID); n < 1 {
		t.Errorf("no denial was recorded for the job")
	}
	if !strings.Contains(logs.String(), "worker starting") {
		t.Errorf("logs: %s", logs.String())
	}
}
