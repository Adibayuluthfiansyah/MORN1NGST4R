package evidence_test

// The M7 additions of the evidence use case: the provenance links of a capture
// (TD-M6-05), the sweep of the directory and the integrity re-check (TD-M6-01),
// the alert of a failed check, and the purge (TD-M6-08).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	evidenceapp "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

func (e *env) withAlerts(t *testing.T) {
	e.deps.Alerts = persistence.NewAlerts(e.c.Pool(t, persistence.RoleAPI, 2))
}

func (e *env) secondAssessment(t *testing.T) string {
	t.Helper()
	var id string
	if err := e.c.Owner(t).QueryRow(t.Context(), `INSERT INTO morningstar.assessments (id, program_id, name, created_by) VALUES (uuidv7(), $1, 'second', $2) RETURNING id`, e.b.ProgramID, e.b.UserID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) jobAndDecision(t *testing.T) (job, decision string) {
	t.Helper()
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job = e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if err := e.c.Owner(t).QueryRow(t.Context(), `INSERT INTO morningstar.policy_decisions (id, program_id, assessment_id, point, job_id, outcome, scope_version_id, validation_id, policy_snapshot_hash)
		VALUES (uuidv7(), $1, $2, 'E3', $3, 'allow', $4, $5, $6) RETURNING id`, e.b.ProgramID, e.b.AssessmentID, job, e.b.ScopeID, e.b.ValidationID, e.b.PolicyHash).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	return job, decision
}

func TestACaptureCannotCarryTheProvenanceOfAnotherAssessment(t *testing.T) {
	e := newEnv(t)
	job, decision := e.jobAndDecision(t)

	in := e.input("linked")
	in.JobID, in.DecisionID = job, decision
	if _, err := e.svc.Capture(t.Context(), in); err != nil {
		t.Fatalf("a job and a decision of the same assessment: %v", err)
	}

	other := e.secondAssessment(t)
	cases := map[string]func(*inbound.CaptureInput){
		"job of another assessment":      func(in *inbound.CaptureInput) { in.AssessmentID, in.JobID = other, job },
		"decision of another assessment": func(in *inbound.CaptureInput) { in.AssessmentID, in.DecisionID = other, decision },
		"decision of another job": func(in *inbound.CaptureInput) {
			in.JobID, in.DecisionID = ids.New(), decision
		},
		"attempt without a job":  func(in *inbound.CaptureInput) { in.AttemptID = ids.New() },
		"attempt that is absent": func(in *inbound.CaptureInput) { in.JobID, in.AttemptID = job, ids.New() },
	}
	before := files(t, e.dir)
	for name, mod := range cases {
		in := e.input("refused " + name)
		mod(&in)
		_, err := e.svc.Capture(t.Context(), in)
		var v *inbound.ValidationError
		if !errors.As(err, &v) {
			t.Errorf("%s: err = %v, want a validation error", name, err)
		}
	}
	if after := files(t, e.dir); len(after) != len(before) {
		t.Errorf("a refused capture wrote a file: %v -> %v", before, after)
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_items`); n != 1 {
		t.Errorf("items = %d, want 1", n)
	}
}

func (e *env) age(t *testing.T, name string, by time.Duration) {
	t.Helper()
	old := time.Now().Add(-by)
	if err := os.Chtimes(filepath.Join(e.dir, name), old, old); err != nil {
		t.Fatal(err)
	}
}

func exists(dir, name string) bool {
	_, err := os.Lstat(filepath.Join(dir, name))
	return err == nil
}

var sweepPolicy = inbound.SweepPolicy{OrphanAge: time.Hour, RecheckAfter: time.Hour, Batch: 10}

func TestTheSweepRemovesOnlyOldOrphansAndTemporaryFiles(t *testing.T) {
	e := newEnv(t)
	kept := e.capture(t, "kept")
	e.age(t, kept.StorageKey(), 48*time.Hour)

	orphanName := ids.New()
	young := ids.New()
	for _, n := range []string{orphanName, young} {
		if err := os.WriteFile(filepath.Join(e.dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e.age(t, orphanName, 2*time.Hour)
	tmpOld, tmpYoung, stray := ".tmp-0a1b2c3d4e5f6071", ".tmp-1a1b2c3d4e5f6071", "notes.txt"
	for _, n := range []string{tmpOld, tmpYoung, stray} {
		if err := os.WriteFile(filepath.Join(e.dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e.age(t, tmpOld, 2*time.Hour)
	e.age(t, stray, 100*time.Hour)
	link := "link-to-kept"
	if err := os.Symlink(filepath.Join(e.dir, kept.StorageKey()), filepath.Join(e.dir, link)); err != nil {
		t.Fatal(err)
	}

	res, err := e.svc.Sweep(t.Context(), sweepPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if exists(e.dir, orphanName) || exists(e.dir, tmpOld) {
		t.Error("an old orphan or an old temporary file was left")
	}
	for _, n := range []string{kept.StorageKey(), young, tmpYoung, stray, link} {
		if !exists(e.dir, n) {
			t.Errorf("%s was removed", n)
		}
	}
	if res.OrphansRemoved != 1 || res.TemporaryRemoved != 1 || res.Unrecognized < 2 {
		t.Errorf("result = %+v", res)
	}
}

func TestTheSweepChecksDueItemsAndMarksASwappedFile(t *testing.T) {
	e := newEnv(t)
	e.withAlerts(t)
	e.svc = evidenceapp.New(e.deps)
	good := e.capture(t, "good")
	bad := e.capture(t, "bad")
	if err := os.WriteFile(e.path(bad), []byte("swapped"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := e.svc.Sweep(t.Context(), sweepPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if res.Checked != 2 || res.Failed != 1 {
		t.Fatalf("result = %+v", res)
	}
	if st := e.state(t, good); st.Integrity != evidence.IntegrityOK {
		t.Errorf("good: %+v", st)
	}
	if st := e.state(t, bad); st.Integrity != evidence.Compromised {
		t.Errorf("bad: %+v", st)
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_integrity_checks WHERE trigger = 'sweep' AND actor_kind = 'system'`); n != 2 {
		t.Errorf("sweep checks recorded = %d, want 2", n)
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.system_alerts WHERE kind = 'evidence_integrity'`); n != 1 {
		t.Errorf("evidence_integrity alerts = %d, want 1", n)
	}

	// A second pass inside the re-check window checks nothing, and a compromised
	// item is never checked again by the sweep.
	res, err = e.svc.Sweep(t.Context(), sweepPolicy)
	if err != nil || res.Checked != 0 {
		t.Errorf("second pass: %+v, %v", res, err)
	}
}

func TestTheSweepSkipsPurgedAndArchivedEvidence(t *testing.T) {
	e := newEnv(t)
	pending := e.capture(t, "pending")
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.evidence_status SET retention_state = 'purge_pending' WHERE evidence_id = $1`, pending.ID()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(e.path(pending)); err != nil {
		t.Fatal(err)
	}
	if res, err := e.svc.Sweep(t.Context(), sweepPolicy); err != nil || res.Checked != 0 || res.Failed != 0 {
		t.Errorf("an item that is being purged was checked: %+v, %v", res, err)
	}
	if st := e.state(t, pending); st.Integrity != evidence.Unchecked {
		t.Errorf("a purge in progress was marked: %+v", st)
	}
	it := e.capture(t, "x")
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.evidence_status SET retention_state = 'purged', purged_at = now(), purged_by = $2 WHERE evidence_id = $1`, it.ID(), e.b.UserID); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Sweep(t.Context(), sweepPolicy)
	if err != nil || res.Checked != 0 {
		t.Errorf("purged: %+v, %v", res, err)
	}
	e2 := newEnv(t)
	e2.capture(t, "y")
	if _, err := e2.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.programs SET status = 'archived', archived_at = now() WHERE id = $1`, e2.b.ProgramID); err != nil {
		t.Fatal(err)
	}
	res, err = e2.svc.Sweep(t.Context(), sweepPolicy)
	if err != nil || res.Checked != 0 {
		t.Errorf("archived: %+v, %v", res, err)
	}
}

// TD-M6-02: the worker composition builds the store and the repository from the
// same directory and limit, so what the worker captures is checked and served by
// the server.
func TestEvidenceCapturedWithTheWorkerRoleIsCheckedAndReadByTheServer(t *testing.T) {
	e := newEnv(t)
	pool := e.c.Pool(t, persistence.RoleWorker, 4)
	sink, err := persistence.NewAuditSink(pool, fastPolicy, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker := evidenceapp.New(evidenceapp.Deps{
		Store: e.store, Repo: persistence.NewEvidence(pool), Audit: sink, Tx: persistence.NewTransactor(pool),
		NewID: ids.New, Now: func() time.Time { return time.Now().UTC() },
	})
	who, err := audit.Worker("worker-test")
	if err != nil {
		t.Fatal(err)
	}
	in := e.input("captured by the worker")
	in.Actor = who
	it, err := worker.Capture(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.svc.Check(t.Context(), e.me, ids.New(), it.ID())
	if err != nil || c.Result != evidence.ResultMatch {
		t.Fatalf("Check = %+v, %v", c, err)
	}
	got, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), false)
	if err != nil || string(got.Data) != "captured by the worker" {
		t.Fatalf("Open = %q, %v", got.Data, err)
	}
}

func (e *env) withPurge(t *testing.T) {
	t.Helper()
	pool := e.c.Pool(t, persistence.RoleRetention, 4)
	sink, err := persistence.NewAuditSink(pool, fastPolicy, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.deps.Retention = persistence.NewRetention(pool)
	e.deps.RetentionTx = persistence.NewTransactor(pool)
	e.deps.RetentionAudit = sink
	e.svc = evidenceapp.New(e.deps)
}

func TestPurgeRemovesTheContentAndKeepsTheMetadata(t *testing.T) {
	e := newEnv(t)
	e.withPurge(t)
	it := e.capture(t, "purge me")

	if err := e.svc.Purge(t.Context(), e.me, ids.New(), it.ID()); err != nil {
		t.Fatal(err)
	}
	if exists(e.dir, it.StorageKey()) {
		t.Error("the file is still there")
	}
	rec, err := e.svc.Get(t.Context(), e.me, it.ID())
	if err != nil || rec.Status.Retention != evidence.RetentionPurged || rec.Item.SHA256() != it.SHA256() {
		t.Fatalf("metadata = %+v, %v", rec, err)
	}
	if n := e.events(t, "evidence.purge_requested", audit.Success); n != 1 {
		t.Errorf("purge_requested = %d", n)
	}
	if n := e.events(t, "evidence.purged", audit.Success); n != 1 {
		t.Errorf("purged = %d", n)
	}
	if _, err := e.svc.Check(t.Context(), e.me, ids.New(), it.ID()); !errors.Is(err, inbound.ErrEvidencePurged) {
		t.Errorf("Check: %v", err)
	}
	if _, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), true); !errors.Is(err, inbound.ErrEvidencePurged) {
		t.Errorf("Open: %v", err)
	}
	if err := e.svc.Purge(t.Context(), e.me, ids.New(), it.ID()); !errors.Is(err, inbound.ErrEvidencePurged) {
		t.Errorf("a second purge: %v", err)
	}
}

// failRemove makes the removal of a file fail, as a crash between the two
// transactions of a purge would leave it.
type failRemove struct {
	outbound.EvidenceStore
	fail bool
}

func (f *failRemove) Remove(ctx context.Context, key string) error {
	if f.fail {
		return errors.New("disk")
	}
	return f.EvidenceStore.Remove(ctx, key)
}

func TestAPurgeThatWasInterruptedIsResumedByASecondCall(t *testing.T) {
	e := newEnv(t)
	fr := &failRemove{EvidenceStore: e.store, fail: true}
	e.deps.Store = fr
	e.withPurge(t)
	it := e.capture(t, "resume")

	if err := e.svc.Purge(t.Context(), e.me, ids.New(), it.ID()); err == nil {
		t.Fatal("the purge reported success although the file was not removed")
	}
	if st := e.state(t, it); st.Retention != evidence.RetentionPurgePending {
		t.Fatalf("state = %v, want purge_pending", st.Retention)
	}
	if !exists(e.dir, it.StorageKey()) {
		t.Fatal("the file was removed")
	}
	fr.fail = false
	if err := e.svc.Purge(t.Context(), e.me, ids.New(), it.ID()); err != nil {
		t.Fatal(err)
	}
	if st := e.state(t, it); st.Retention != evidence.RetentionPurged || exists(e.dir, it.StorageKey()) {
		t.Errorf("after the resume: %v", st.Retention)
	}
	if n := e.events(t, "evidence.purge_requested", audit.Success); n != 1 {
		t.Errorf("purge_requested = %d, want once", n)
	}
}

func TestPurgeRefusesAnotherOwnerAndAMissingRetentionRole(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "x")
	if err := e.svc.Purge(t.Context(), e.me, ids.New(), it.ID()); !errors.Is(err, evidenceapp.ErrPurgeUnavailable) {
		t.Errorf("without the retention role: %v", err)
	}
	e.withPurge(t)
	if err := e.svc.Purge(t.Context(), principal{user: ids.New()}, ids.New(), it.ID()); !errors.Is(err, inbound.ErrNotFound) {
		t.Errorf("another owner: %v", err)
	}
	if !exists(e.dir, it.StorageKey()) || e.state(t, it).Retention != evidence.RetentionActive {
		t.Error("a refused purge changed something")
	}
}

func TestAPurgeThatCannotBeAuditedChangesNothing(t *testing.T) {
	e := newEnv(t)
	e.withPurge(t)
	it := e.capture(t, "x")
	e.deps.RetentionAudit = refusing{}
	e.svc = evidenceapp.New(e.deps)
	if err := e.svc.Purge(t.Context(), e.me, ids.New(), it.ID()); err == nil {
		t.Fatal("a purge went through without its audit event")
	}
	if !exists(e.dir, it.StorageKey()) || e.state(t, it).Retention != evidence.RetentionActive {
		t.Error("the state or the file changed")
	}
}

type refusing struct{ outbound.AuditSink }

func (refusing) Require(context.Context, audit.Event) error { return outbound.ErrAuditNotRecorded }
