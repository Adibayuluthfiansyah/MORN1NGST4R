package db_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/recoverymarker"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// G-recovery (recovery-generation.md section 11): the recovery generation
// against a real database, a real marker file and a real "restore" of an older
// database.

type failingMarkers struct {
	outbound.RecoveryMarkerStore
	fail atomic.Bool
}

func (f *failingMarkers) Replace(ctx context.Context, m outbound.RecoveryMarker) error {
	if f.fail.Load() {
		return errors.New("injected marker failure")
	}
	return f.RecoveryMarkerStore.Replace(ctx, m)
}

func (f *failingMarkers) Advance(ctx context.Context, m outbound.RecoveryMarker) error {
	if f.fail.Load() {
		return errors.New("injected marker failure")
	}
	return f.RecoveryMarkerStore.Advance(ctx, m)
}

type failingRepo struct {
	outbound.RecoveryRepository
	failComplete atomic.Bool
}

func (f *failingRepo) CompleteReconcile(ctx context.Context, g uint64, r string) error {
	if f.failComplete.Load() {
		return errors.New("injected failure of phase 3")
	}
	return f.RecoveryRepository.CompleteReconcile(ctx, g, r)
}

type rstack struct {
	c       *pgtest.Cluster
	dir     string
	api     *pgxpool.Pool
	markers *failingMarkers
	repo    *failingRepo
	svc     *recovery.Service
}

// newStack builds the server side of the recovery code against the current
// database and the marker directory dir. Call it again after a restore.
func newStack(t *testing.T, c *pgtest.Cluster, dir string) *rstack {
	t.Helper()
	store, err := recoverymarker.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	api := c.Pool(t, persistence.RoleAPI, 4)
	s := &rstack{c: c, dir: dir, api: api, markers: &failingMarkers{RecoveryMarkerStore: store}, repo: &failingRepo{RecoveryRepository: persistence.NewRecovery(api)}}
	s.svc = recovery.NewService(recovery.Deps{Markers: s.markers, Repo: s.repo, NewID: newID, Now: time.Now})
	return s
}

func stateDir(t *testing.T) string { return filepath.Join(t.TempDir(), "state") }

func (s *rstack) markerPath() string { return filepath.Join(s.dir, "recovery-generation.json") }

func (s *rstack) mustCheck(t *testing.T) recovery.Status {
	t.Helper()
	st, err := s.svc.Check(t.Context())
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	return st
}

func (s *rstack) audit(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := s.api.Exec(t.Context(), `INSERT INTO morningstar.audit_events (id, actor_kind, action, outcome) VALUES (uuidv7(), 'system', 'test.event', 'success')`); err != nil {
			t.Fatal(err)
		}
	}
}

func (s *rstack) marker(t *testing.T) outbound.RecoveryMarker {
	t.Helper()
	m, err := s.markers.Read(t.Context())
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	return m
}

// installed runs the first run and seeds an account, like a working installation.
func installed(t *testing.T) (*pgtest.Cluster, *rstack, pgtest.Base) {
	t.Helper()
	c := pgtest.Get(t)
	c.Fresh(t)
	s := newStack(t, c, stateDir(t))
	if st := s.mustCheck(t); st.Required {
		t.Fatalf("first run: %+v", st)
	}
	b := c.SeedBaseWithoutRecovery(t)
	return c, s, b
}

func wantRequired(t *testing.T, s *rstack, reason recovery.Reason) recovery.Status {
	t.Helper()
	st := s.mustCheck(t)
	if !st.Required || st.Reason != reason {
		t.Fatalf("Status = %+v, want recovery required with %s", st, reason)
	}
	return st
}

// 10: first run initializes both halves, and a second start is normal.
func TestGRecoveryFirstRunInitializesBothHalves(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	dir := stateDir(t)
	s := newStack(t, c, dir)
	if st := s.mustCheck(t); st.Required {
		t.Fatalf("first run: %+v", st)
	}
	m := s.marker(t)
	if m.Generation != 1 || m.InstanceID == "" {
		t.Errorf("marker = %+v", m)
	}
	var instance string
	var gen int64
	var state string
	if err := c.Owner(t).QueryRow(t.Context(), `SELECT instance_id::text, generation, state FROM morningstar.recovery_state`).Scan(&instance, &gen, &state); err != nil {
		t.Fatal(err)
	}
	if instance != m.InstanceID || gen != 1 || state != "normal" {
		t.Errorf("recovery_state = %s %d %s, marker %+v", instance, gen, state, m)
	}
	if m.AuditWatermark < 1 {
		t.Errorf("watermark = %d, want the first-run audit event to be covered", m.AuditWatermark)
	}
	second := newStack(t, c, dir)
	if st := second.mustCheck(t); st.Required {
		t.Errorf("second start: %+v", st)
	}
	if g, ok := second.svc.Generation(); !ok || g != 1 {
		t.Errorf("Generation = %d, %v", g, ok)
	}
}

// 10 and section 8: when the marker cannot be written at first run, nothing is committed.
func TestGRecoveryFirstRunWithAFailingMarkerLeavesTheDatabaseEmpty(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	s := newStack(t, c, stateDir(t))
	s.markers.fail.Store(true)
	if _, err := s.svc.Check(t.Context()); err == nil {
		t.Fatal("first run succeeded without a marker")
	}
	var n int
	if err := c.Owner(t).QueryRow(t.Context(), `SELECT (SELECT count(*) FROM morningstar.recovery_state) + (SELECT count(*) FROM morningstar.audit_events)`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows after a failed first run: %d (%v)", n, err)
	}
	s.markers.fail.Store(false)
	if st := s.mustCheck(t); st.Required {
		t.Errorf("the retry: %+v", st)
	}
}

// 1: an older dump restored while the marker is kept is detected (rows 10 and 6).
func TestGRecoveryRestoreOfAnOlderDumpIsDetected(t *testing.T) {
	t.Run("row 10: same generation, fewer audit events than the watermark", func(t *testing.T) {
		c, s, _ := installed(t)
		c.Snapshot(t, "snap_row10")
		s2 := newStack(t, c, s.dir)
		if st := s2.mustCheck(t); st.Required {
			t.Fatalf("after the snapshot: %+v", st)
		}
		s2.audit(t, 5)
		if err := s2.svc.AdvanceWatermark(t.Context()); err != nil {
			t.Fatal(err)
		}
		c.Restore(t, "snap_row10")
		wantRequired(t, newStack(t, c, s.dir), recovery.ReasonDatabaseOlder)
	})
	t.Run("row 6: the marker generation is ahead of the database", func(t *testing.T) {
		c, s, b := installed(t)
		c.Snapshot(t, "snap_row6")
		s2 := newStack(t, c, s.dir)
		s2.mustCheck(t)
		if _, err := s2.svc.Reconcile(t.Context(), b.UserID, newID()); err == nil {
			t.Fatal("Reconcile succeeded on a normal installation")
		}
		// Make it required first (marker removed), then reconcile to generation 2.
		if err := os.Remove(s2.markerPath()); err != nil {
			t.Fatal(err)
		}
		wantRequired(t, s2, recovery.ReasonMarkerMissing)
		if _, err := s2.svc.Reconcile(t.Context(), b.UserID, newID()); err != nil {
			t.Fatal(err)
		}
		if g := s2.marker(t).Generation; g != 2 {
			t.Fatalf("marker generation = %d, want 2", g)
		}
		c.Restore(t, "snap_row6")
		wantRequired(t, newStack(t, c, s.dir), recovery.ReasonDatabaseOlder)
	})
}

// 2: a populated database with the marker deleted.
func TestGRecoveryMissingMarker(t *testing.T) {
	_, s, _ := installed(t)
	if err := os.Remove(s.markerPath()); err != nil {
		t.Fatal(err)
	}
	wantRequired(t, s, recovery.ReasonMarkerMissing)
}

// 3: corrupt the marker, change its mode, replace it with a symlink.
func TestGRecoveryInvalidMarker(t *testing.T) {
	damage := map[string]func(t *testing.T, path string){
		"corrupt": func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte(`{"version":1,"instance_id":"x"}`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"mode": func(t *testing.T, p string) {
			if err := os.Chmod(p, 0o666); err != nil {
				t.Fatal(err)
			}
		},
		"symlink": func(t *testing.T, p string) {
			real := p + ".real"
			if err := os.Rename(p, real); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, p); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, f := range damage {
		t.Run(name, func(t *testing.T) {
			_, s, _ := installed(t)
			f(t, s.markerPath())
			wantRequired(t, s, recovery.ReasonMarkerInvalid)
		})
	}
}

// 4: another installation's marker.
func TestGRecoveryAnotherInstallationsMarker(t *testing.T) {
	_, s, _ := installed(t)
	otherDir := stateDir(t)
	other, err := recoverymarker.New(otherDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Replace(t.Context(), outbound.RecoveryMarker{InstanceID: newID(), Generation: 1, AuditWatermark: 1, WrittenAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// Swap the file in, as an administrator would.
	if err := os.Rename(filepath.Join(otherDir, "recovery-generation.json"), s.markerPath()); err != nil {
		t.Fatal(err)
	}
	wantRequired(t, s, recovery.ReasonInstanceMismatch)
}

// 5: an old marker with a newer database.
func TestGRecoveryOldMarkerWithANewerDatabase(t *testing.T) {
	_, s, b := installed(t)
	old, err := os.ReadFile(s.markerPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.markerPath()); err != nil {
		t.Fatal(err)
	}
	wantRequired(t, s, recovery.ReasonMarkerMissing)
	if _, err := s.svc.Reconcile(t.Context(), b.UserID, newID()); err != nil {
		t.Fatal(err)
	}
	// Generation 2 now. Put the generation 1 marker back.
	if err := os.WriteFile(s.markerPath(), old, 0o600); err != nil {
		t.Fatal(err)
	}
	wantRequired(t, newStack(t, s.c, s.dir), recovery.ReasonMarkerOlder)
}

// 6 and 9: crash after each phase, and a failed audit insert in phase 1.
func TestGRecoveryCrashAfterEachPhase(t *testing.T) {
	t.Run("phase 1 fails: the audit insert is refused, nothing is invalidated", func(t *testing.T) {
		c, s, b := installed(t)
		sess := c.SeedSession(t, b)
		ap := c.SeedApproval(t, b, pgtest.ApprovalSpec{})
		if err := os.Remove(s.markerPath()); err != nil {
			t.Fatal(err)
		}
		wantRequired(t, s, recovery.ReasonMarkerMissing)
		// The server may not insert into the audit log: phase 1 must fail as a whole.
		if _, err := c.Owner(t).Exec(t.Context(), `REVOKE INSERT ON morningstar.audit_events FROM ms_api`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.svc.Reconcile(t.Context(), b.UserID, newID()); err == nil {
			t.Fatal("Reconcile succeeded without an audit event")
		}
		var revoked *time.Time
		var status string
		var gen int64
		var state string
		owner := c.Owner(t)
		if err := owner.QueryRow(t.Context(), `SELECT revoked_at FROM morningstar.sessions WHERE id = $1`, sess).Scan(&revoked); err != nil || revoked != nil {
			t.Errorf("the session was revoked by a failed phase 1 (%v)", err)
		}
		if err := owner.QueryRow(t.Context(), `SELECT status FROM morningstar.approvals WHERE id = $1`, ap.ID).Scan(&status); err != nil || status != "approved" {
			t.Errorf("approval status = %q (%v), want approved", status, err)
		}
		if err := owner.QueryRow(t.Context(), `SELECT generation, state FROM morningstar.recovery_state`).Scan(&gen, &state); err != nil || gen != 1 || state != "normal" {
			t.Errorf("recovery_state = %d %s (%v), want 1 normal (rolled back)", gen, state, err)
		}
		// Still fail closed on the next start.
		if st := newStack(t, c, s.dir).mustCheck(t); !st.Required {
			t.Error("the installation is not recovery_required after a failed phase 1")
		}
	})
	t.Run("crash after phase 1: the next start resumes and grants nothing", func(t *testing.T) {
		c, s, b := installed(t)
		ap := c.SeedApproval(t, b, pgtest.ApprovalSpec{})
		s.audit(t, 3)
		if err := s.svc.AdvanceWatermark(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(s.markerPath()); err != nil {
			t.Fatal(err)
		}
		wantRequired(t, s, recovery.ReasonMarkerMissing)
		s.markers.fail.Store(true) // the process dies before the marker write
		if _, err := s.svc.Reconcile(t.Context(), b.UserID, newID()); err == nil {
			t.Fatal("Reconcile succeeded without a marker")
		}
		if _, ok := s.svc.Generation(); ok {
			t.Error("claims allowed after a failed phase 2")
		}
		// The next start: no marker and a reconciling row is recovery_required again (marker_missing),
		// and the second reconciliation completes. Nothing granted in between.
		s.markers.fail.Store(false)
		next := newStack(t, c, s.dir)
		wantRequired(t, next, recovery.ReasonMarkerMissing)
		if _, err := next.svc.Reconcile(t.Context(), b.UserID, newID()); err != nil {
			t.Fatal(err)
		}
		var status string
		if err := c.Owner(t).QueryRow(t.Context(), `SELECT status FROM morningstar.approvals WHERE id = $1`, ap.ID).Scan(&status); err != nil || status != "revoked" {
			t.Errorf("approval status = %q (%v), want revoked", status, err)
		}
	})
	t.Run("crash after phase 1 with a valid marker: the next start resumes phases 2 and 3 (row 7)", func(t *testing.T) {
		c, s, b := installed(t)
		ap := c.SeedApproval(t, b, pgtest.ApprovalSpec{})
		// Generation of the marker goes ahead of the database: database_older, required.
		if err := s.markers.RecoveryMarkerStore.Replace(t.Context(), outbound.RecoveryMarker{InstanceID: s.marker(t).InstanceID, Generation: 2, AuditWatermark: 1, WrittenAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		wantRequired(t, s, recovery.ReasonDatabaseOlder)
		s.markers.fail.Store(true)
		if _, err := s.svc.Reconcile(t.Context(), b.UserID, newID()); err == nil {
			t.Fatal("Reconcile succeeded without a marker write")
		}
		s.markers.fail.Store(false)
		// Marker generation 2, database generation 3 and reconciling: row 7.
		next := newStack(t, c, s.dir)
		if st := next.mustCheck(t); st.Required {
			t.Fatalf("the interrupted reconciliation was not resumed: %+v", st)
		}
		if g := next.marker(t).Generation; g != 3 {
			t.Errorf("marker generation = %d, want 3", g)
		}
		var status, state string
		if err := c.Owner(t).QueryRow(t.Context(), `SELECT (SELECT status FROM morningstar.approvals WHERE id = $1), (SELECT state FROM morningstar.recovery_state)`, ap.ID).Scan(&status, &state); err != nil || status != "revoked" || state != "normal" {
			t.Errorf("approval %q, state %q (%v)", status, state, err)
		}
	})
	t.Run("crash after the marker write: the next start completes phase 3 (row 9)", func(t *testing.T) {
		c, s, b := installed(t)
		if err := os.Remove(s.markerPath()); err != nil {
			t.Fatal(err)
		}
		wantRequired(t, s, recovery.ReasonMarkerMissing)
		s.repo.failComplete.Store(true)
		if _, err := s.svc.Reconcile(t.Context(), b.UserID, newID()); err == nil {
			t.Fatal("Reconcile succeeded without phase 3")
		}
		if _, ok := s.svc.Generation(); ok {
			t.Error("claims allowed before phase 3")
		}
		var state string
		if err := c.Owner(t).QueryRow(t.Context(), `SELECT state FROM morningstar.recovery_state`).Scan(&state); err != nil || state != "reconciling" {
			t.Fatalf("state = %q (%v), want reconciling", state, err)
		}
		next := newStack(t, c, s.dir)
		if st := next.mustCheck(t); st.Required {
			t.Fatalf("the start after the crash: %+v", st)
		}
		if err := c.Owner(t).QueryRow(t.Context(), `SELECT state FROM morningstar.recovery_state`).Scan(&state); err != nil || state != "normal" {
			t.Errorf("state = %q (%v), want normal", state, err)
		}
	})
}

// 8: after a reconciliation nothing from the backup is usable and no use count changed.
func TestGRecoveryNothingFromTheBackupSurvives(t *testing.T) {
	c, s, b := installed(t)
	sess := c.SeedSession(t, b)
	lab := c.SeedLab(t, b)
	c.SetAssessmentStatus(t, b, "running")
	if _, err := c.Owner(t).Exec(t.Context(), `UPDATE morningstar.assessments SET raw_evidence_capture = true WHERE id = $1`, b.AssessmentID); err != nil {
		t.Fatal(err)
	}

	used := c.SeedApproval(t, b, pgtest.ApprovalSpec{MaxUses: 3})
	usedJob := c.SeedJob(t, b, used, "low_impact", 1)
	live := c.SeedApproval(t, b, pgtest.ApprovalSpec{})
	liveJob := c.SeedJob(t, b, live, "low_impact", 1)
	pending := c.SeedApproval(t, b, pgtest.ApprovalSpec{Status: "pending"})

	worker := c.Pool(t, persistence.RoleWorker, 4)
	uses := persistence.NewApprovalUses(worker)
	e := &env{c: c, b: b, worker: worker, uses: uses}
	if _, err := uses.Claim(t.Context(), e.claim(used, usedJob)); err != nil {
		t.Fatal(err)
	}
	owner := c.Owner(t)
	var consumedBefore, rowsBefore int
	if err := owner.QueryRow(t.Context(), `SELECT uses_consumed, (SELECT count(*) FROM morningstar.approval_uses) FROM morningstar.approvals WHERE id = $1`, used.ID).Scan(&consumedBefore, &rowsBefore); err != nil {
		t.Fatal(err)
	}

	// The marker is lost, the installation needs reconciliation.
	if err := os.Remove(s.markerPath()); err != nil {
		t.Fatal(err)
	}
	wantRequired(t, s, recovery.ReasonMarkerMissing)
	st, err := s.svc.Reconcile(t.Context(), b.UserID, newID())
	if err != nil || st.Required {
		t.Fatalf("Reconcile = %+v, %v", st, err)
	}

	str := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if err := owner.QueryRow(t.Context(), sql, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return v
	}
	checks := []struct{ what, got, want string }{
		{"session", str(`SELECT revoked_reason FROM morningstar.sessions WHERE id = $1`, sess), "recovery_reset"},
		{"used approval", str(`SELECT status FROM morningstar.approvals WHERE id = $1`, used.ID), "revoked"},
		{"live approval", str(`SELECT status FROM morningstar.approvals WHERE id = $1`, live.ID), "revoked"},
		{"pending approval", str(`SELECT status FROM morningstar.approvals WHERE id = $1`, pending.ID), "rejected"},
		{"validation", str(`SELECT status || '/' || invalidation_reason FROM morningstar.target_validations WHERE id = $1`, b.ValidationID), "invalidated/restore_recovery"},
		{"lab registration", str(`SELECT status FROM morningstar.lab_registrations WHERE id = $1`, lab), "revoked"},
		{"assessment", str(`SELECT status FROM morningstar.assessments WHERE id = $1`, b.AssessmentID), "paused"},
		{"raw evidence capture", str(`SELECT raw_evidence_capture::text FROM morningstar.assessments WHERE id = $1`, b.AssessmentID), "false"},
		{"queued job", str(`SELECT state FROM morningstar.jobs WHERE id = $1`, liveJob), "cancelled"},
		{"approval events", str(`SELECT count(*)::text FROM morningstar.approval_events WHERE reason_code = 'recovery_reset'`), "3"},
		{"alert", str(`SELECT kind FROM morningstar.system_alerts`), "restore_recovered"},
		{"audit restore", str(`SELECT count(*)::text FROM morningstar.audit_events WHERE action = 'system.restore_recovered'`), "1"},
		{"audit completion", str(`SELECT count(*)::text FROM morningstar.audit_events WHERE action = 'system.recovery_reconciled'`), "1"},
		{"recovery state", str(`SELECT state || '/' || generation FROM morningstar.recovery_state`), "normal/2"},
	}
	for _, ck := range checks {
		if ck.got != ck.want {
			t.Errorf("%s = %q, want %q", ck.what, ck.got, ck.want)
		}
	}
	var consumedAfter, rowsAfter int
	if err := owner.QueryRow(t.Context(), `SELECT uses_consumed, (SELECT count(*) FROM morningstar.approval_uses) FROM morningstar.approvals WHERE id = $1`, used.ID).Scan(&consumedAfter, &rowsAfter); err != nil {
		t.Fatal(err)
	}
	if consumedAfter != consumedBefore || rowsAfter != rowsBefore {
		t.Errorf("use counters changed: %d/%d before, %d/%d after", consumedBefore, rowsBefore, consumedAfter, rowsAfter)
	}

	// Nothing from the backup can be claimed or launched, even under the new generation.
	gen, ok := s.svc.Generation()
	if !ok || gen != 2 {
		t.Fatalf("Generation = %d, %v", gen, ok)
	}
	claim := e.claim(live, liveJob)
	claim.Generation = gen
	if _, err := uses.Claim(t.Context(), claim); err == nil || !errors.Is(err, approval.ErrDenied) {
		t.Errorf("claim of a revoked approval under the new generation: %v", err)
	}
	launch := e.launch(used, usedJob)
	launch.Generation = gen
	if got := reasonOf(t, uses.CheckLaunch(t.Context(), launch)); got != approval.ReasonRevoked {
		t.Errorf("launch under a revoked approval: %s", got)
	}
}

// 7: a worker that read generation 1 before the restore cannot claim afterwards.
func TestGRecoveryAWorkerFromBeforeTheRestoreCannotClaim(t *testing.T) {
	c, s, b := installed(t)
	worker := c.Pool(t, persistence.RoleWorker, 4)
	uses := persistence.NewApprovalUses(worker)
	e := &env{c: c, b: b, worker: worker, uses: uses}

	ap := c.SeedApproval(t, b, pgtest.ApprovalSpec{MaxUses: 2})
	j1, j2 := c.SeedJob(t, b, ap, "low_impact", 1), c.SeedJob(t, b, ap, "low_impact", 1)
	if _, err := uses.Claim(t.Context(), e.claim(ap, j1)); err != nil {
		t.Fatalf("claim under generation 1 before the restore: %v", err)
	}

	if err := os.Remove(s.markerPath()); err != nil {
		t.Fatal(err)
	}
	wantRequired(t, s, recovery.ReasonMarkerMissing)
	if _, err := s.svc.Reconcile(t.Context(), b.UserID, newID()); err != nil {
		t.Fatal(err)
	}
	// The worker still holds generation 1.
	if _, err := uses.Claim(t.Context(), e.claim(ap, j2)); !errors.Is(err, outbound.ErrRecoveryGeneration) {
		t.Errorf("claim under the old generation: %v, want ErrRecoveryGeneration", err)
	}
	var n int
	if err := c.Owner(t).QueryRow(t.Context(), `SELECT count(*) FROM morningstar.approval_uses WHERE job_id = $1`, j2).Scan(&n); err != nil || n != 0 {
		t.Errorf("the stale worker consumed a use (%d rows, %v)", n, err)
	}
}

// 11: the watermark is advanced before a launch, and a failed marker write
// stops claims until a write succeeds.
func TestGRecoveryWatermarkIsAdvancedAndAFailedWriteStopsClaims(t *testing.T) {
	_, s, _ := installed(t)
	before := s.marker(t).AuditWatermark
	s.audit(t, 4) // the claim and its audit events
	if err := s.svc.AdvanceWatermark(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := s.marker(t).AuditWatermark; got != before+4 {
		t.Errorf("watermark = %d, want %d", got, before+4)
	}
	if err := s.svc.ClaimsAllowed(); err != nil {
		t.Errorf("ClaimsAllowed = %v", err)
	}

	// The marker directory becomes read-only: the write fails.
	if err := os.Chmod(s.dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.dir, 0o700) })
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	s.audit(t, 1)
	if err := s.svc.AdvanceWatermark(t.Context()); !errors.Is(err, recovery.ErrMarkerWrite) {
		t.Fatalf("AdvanceWatermark = %v, want ErrMarkerWrite", err)
	}
	if err := s.svc.ClaimsAllowed(); !errors.Is(err, recovery.ErrMarkerWrite) {
		t.Errorf("ClaimsAllowed = %v, want ErrMarkerWrite", err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.svc.AdvanceWatermark(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.svc.ClaimsAllowed(); err != nil {
		t.Errorf("ClaimsAllowed after a good write = %v", err)
	}
}

// A marker that is deleted while the server runs is not recreated by the
// watermark advance. Claims stop, and the next start reports marker_missing.
func TestGRecoveryAMarkerDeletedAtRuntimeIsNotRecreated(t *testing.T) {
	c, s, _ := installed(t)
	if err := os.Remove(s.markerPath()); err != nil {
		t.Fatal(err)
	}
	err := s.svc.AdvanceWatermark(t.Context())
	if !errors.Is(err, recovery.ErrMarkerWrite) || !errors.Is(err, outbound.ErrMarkerMissing) {
		t.Fatalf("AdvanceWatermark = %v, want ErrMarkerWrite caused by ErrMarkerMissing", err)
	}
	if _, err := os.Stat(s.markerPath()); !os.IsNotExist(err) {
		t.Error("the marker was recreated")
	}
	if err := s.svc.ClaimsAllowed(); !errors.Is(err, recovery.ErrMarkerWrite) {
		t.Errorf("ClaimsAllowed = %v", err)
	}
	wantRequired(t, newStack(t, c, s.dir), recovery.ReasonMarkerMissing)
}
