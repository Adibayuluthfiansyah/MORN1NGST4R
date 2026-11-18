package recovery_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

var errBoom = errors.New("injected failure")

const (
	actor   = "0196a5c0-2222-7222-8333-444455556666"
	request = "0196a5c0-3333-7222-8333-444455556666"
	newInst = "0196a5c0-4444-7222-8333-444455556666"
)

type fakeMarkers struct {
	m         *outbound.RecoveryMarker
	invalid   bool
	failWrite error
	writes    []outbound.RecoveryMarker
	cleaned   int
}

func (f *fakeMarkers) Read(context.Context) (outbound.RecoveryMarker, error) {
	switch {
	case f.invalid:
		return outbound.RecoveryMarker{}, outbound.ErrMarkerInvalid
	case f.m == nil:
		return outbound.RecoveryMarker{}, outbound.ErrMarkerMissing
	}
	return *f.m, nil
}

func (f *fakeMarkers) Replace(_ context.Context, m outbound.RecoveryMarker) error {
	if f.failWrite != nil {
		return f.failWrite
	}
	f.writes = append(f.writes, m)
	f.m, f.invalid = &m, false
	return nil
}

func (f *fakeMarkers) Advance(ctx context.Context, m outbound.RecoveryMarker) error {
	if f.invalid {
		return outbound.ErrMarkerInvalid
	}
	if f.m == nil {
		return outbound.ErrMarkerMissing
	}
	return f.Replace(ctx, m)
}

func (f *fakeMarkers) Cleanup(context.Context) error { f.cleaned++; return nil }

type fakeRepo struct {
	row                       *outbound.RecoveryRow
	users                     bool
	maxSeq                    uint64
	failBegin, failComplete   error
	began                     []outbound.ReconcileInput
	completed                 []string
	failFirstRunBeforeCommit  bool
	firstRunSeq               uint64
	completeGen               []uint64
	beginCalledWithGeneration uint64
}

func (f *fakeRepo) Facts(context.Context) (outbound.RecoveryFacts, error) {
	var r *outbound.RecoveryRow
	if f.row != nil {
		c := *f.row
		r = &c
	}
	return outbound.RecoveryFacts{Row: r, HasUsers: f.users, MaxAuditSeq: f.maxSeq}, nil
}

func (f *fakeRepo) InitializeFirstRun(ctx context.Context, id string, before func(context.Context, uint64) error) error {
	if err := before(ctx, f.firstRunSeq); err != nil {
		return err // nothing is committed
	}
	f.row = &outbound.RecoveryRow{InstanceID: id, Generation: 1}
	return nil
}

func (f *fakeRepo) BeginReconcile(_ context.Context, in outbound.ReconcileInput) (outbound.ReconcileCounts, error) {
	if f.failBegin != nil {
		return outbound.ReconcileCounts{}, f.failBegin
	}
	if f.row != nil && f.row.Generation != in.ExpectedGeneration {
		return outbound.ReconcileCounts{}, outbound.ErrRecoveryChanged
	}
	f.began = append(f.began, in)
	f.row = &outbound.RecoveryRow{InstanceID: pick(f.row, in.InstanceID), Generation: in.NewGeneration, Reconciling: true, LastReason: in.Reason}
	f.maxSeq++ // phase 1 writes an audit event
	return outbound.ReconcileCounts{}, nil
}

func pick(r *outbound.RecoveryRow, id string) string {
	if r != nil {
		return r.InstanceID
	}
	return id
}

func (f *fakeRepo) MaxAuditSeq(context.Context) (uint64, error) { return f.maxSeq, nil }

func (f *fakeRepo) CompleteReconcile(_ context.Context, gen uint64, reason string) error {
	if f.failComplete != nil {
		return f.failComplete
	}
	f.completed = append(f.completed, reason)
	f.completeGen = append(f.completeGen, gen)
	f.row.Reconciling = false
	f.maxSeq++
	return nil
}

func reconcile(svc *recovery.Service, actorID, requestID string) (inbound.ReconcileCounts, error) {
	return svc.Reconcile(context.Background(), inbound.ReconcileRequest{ActorUserID: actorID, RequestID: requestID})
}

func newService(t *testing.T, m *fakeMarkers, r *fakeRepo) *recovery.Service {
	t.Helper()
	return recovery.NewService(recovery.Deps{
		Markers: m, Repo: r,
		NewID: func() string { return newInst },
		Now:   func() time.Time { return time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC) },
	})
}

// G-recovery 10: first run initializes both halves and a second start is normal.
func TestFirstRunInitializesBothHalvesAndASecondStartIsNormal(t *testing.T) {
	m, r := &fakeMarkers{}, &fakeRepo{firstRunSeq: 3}
	svc := newService(t, m, r)
	st, err := svc.Check(t.Context())
	if err != nil || st.Required {
		t.Fatalf("first Check = %+v, %v", st, err)
	}
	if r.row == nil || r.row.InstanceID != newInst || r.row.Generation != 1 {
		t.Errorf("row = %+v", r.row)
	}
	if m.m == nil || m.m.InstanceID != newInst || m.m.Generation != 1 || m.m.AuditWatermark != 3 {
		t.Errorf("marker = %+v", m.m)
	}
	if gen, ok := svc.Generation(); !ok || gen != 1 {
		t.Errorf("Generation = %d, %v", gen, ok)
	}
	r.maxSeq = 3
	svc2 := newService(t, m, r)
	st, err = svc2.Check(t.Context())
	if err != nil || st.Required {
		t.Errorf("second Check = %+v, %v", st, err)
	}
	if len(m.writes) != 1 {
		t.Errorf("the second start wrote the marker again: %d writes", len(m.writes))
	}
	if m.cleaned == 0 {
		t.Error("the start did not remove leftover temporary files")
	}
}

// Section 8: when the marker cannot be written the first run commits nothing,
// so the installation is not left half initialized.
func TestFirstRunWithAFailingMarkerWriteCommitsNothing(t *testing.T) {
	m, r := &fakeMarkers{failWrite: errBoom}, &fakeRepo{}
	svc := newService(t, m, r)
	if _, err := svc.Check(t.Context()); err == nil {
		t.Fatal("Check succeeded although the marker could not be written")
	}
	if r.row != nil {
		t.Error("the recovery_state row was committed")
	}
	if _, ok := svc.Generation(); ok {
		t.Error("claims would be allowed")
	}
}

func TestCheckReportsRecoveryRequiredWithTheGenerations(t *testing.T) {
	m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 3, AuditWatermark: 4}}
	r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 2}, maxSeq: 10}
	svc := newService(t, m, r)
	st, err := svc.Check(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Required || st.Reason != recovery.ReasonDatabaseOlder || st.MarkerGeneration != 3 || st.DatabaseGeneration != 2 || st.DetectedAt.IsZero() {
		t.Errorf("Status = %+v", st)
	}
	if _, ok := svc.Generation(); ok {
		t.Error("claims would be allowed while recovery is required")
	}
	if err := svc.AdvanceWatermark(t.Context()); err == nil {
		t.Error("the watermark was advanced while recovery is required")
	}
}

// A marker that cannot be read at all (other than missing or invalid) is an
// error of the store, never "normal".
func TestCheckFailsClosedOnAStoreError(t *testing.T) {
	r := &fakeRepo{}
	svc := recovery.NewService(recovery.Deps{Markers: erroringMarkers{}, Repo: r, NewID: func() string { return newInst }, Now: time.Now})
	if _, err := svc.Check(t.Context()); err == nil {
		t.Error("Check succeeded with a failing marker store")
	}
}

type erroringMarkers struct{}

func (erroringMarkers) Read(context.Context) (outbound.RecoveryMarker, error) {
	return outbound.RecoveryMarker{}, errBoom
}
func (erroringMarkers) Replace(context.Context, outbound.RecoveryMarker) error { return errBoom }
func (erroringMarkers) Advance(context.Context, outbound.RecoveryMarker) error { return errBoom }
func (erroringMarkers) Cleanup(context.Context) error                          { return nil }

func required(t *testing.T, m *fakeMarkers, r *fakeRepo) *recovery.Service {
	t.Helper()
	svc := newService(t, m, r)
	st, err := svc.Check(t.Context())
	if err != nil || !st.Required {
		t.Fatalf("Check = %+v, %v, want recovery required", st, err)
	}
	return svc
}

func TestReconcilePassesTheNewCredentialToPhaseOneAndRefusesAHalfPair(t *testing.T) {
	m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 5, AuditWatermark: 40}}
	r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 3}, maxSeq: 10}
	svc := required(t, m, r)
	do := func(id, hash string) error {
		_, err := svc.Reconcile(t.Context(), inbound.ReconcileRequest{ActorUserID: actor, RequestID: request, NewCredentialID: id, NewPasswordHash: hash})
		return err
	}
	for name, c := range map[string][2]string{"hash without id": {"", "phc:new"}, "id without hash": {idA, ""}, "id not a uuid": {"not-a-uuid", "phc:new"}} {
		if err := do(c[0], c[1]); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if len(r.began) != 0 || !svc.State().Required {
		t.Fatalf("a malformed credential reached phase 1 (%d calls)", len(r.began))
	}
	if err := do(idA, "phc:new"); err != nil {
		t.Fatal(err)
	}
	if len(r.began) != 1 || r.began[0].NewCredentialID != idA || r.began[0].NewPasswordHash != "phc:new" {
		t.Fatalf("phase 1 input = %+v", r.began)
	}
}

func TestReconcileRunsAllThreePhases(t *testing.T) {
	m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 5, AuditWatermark: 40}}
	r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 3}, maxSeq: 10}
	svc := required(t, m, r)

	counts, err := reconcile(svc, actor, request)
	if err != nil || counts.Generation != 6 || svc.State().Required {
		t.Fatalf("Reconcile = %+v, %v", counts, err)
	}
	if len(r.began) != 1 || r.began[0].NewGeneration != 6 || r.began[0].ExpectedGeneration != 3 || r.began[0].OldMarkerGeneration != 5 {
		t.Errorf("phase 1 input = %+v, want new generation max(5,3)+1 = 6", r.began)
	}
	if r.began[0].Reason != string(recovery.ReasonDatabaseOlder) || r.began[0].ActorUserID != actor || r.began[0].RequestID != request {
		t.Errorf("phase 1 input = %+v", r.began[0])
	}
	last := m.writes[len(m.writes)-1]
	if last.Generation != 6 || last.InstanceID != idA || last.AuditWatermark != 11 {
		t.Errorf("marker = %+v, want generation 6, the database instance and the audit sequence after phase 1 (11)", last)
	}
	if len(r.completed) != 1 || r.completeGen[0] != 6 {
		t.Errorf("phase 3 = %v %v", r.completed, r.completeGen)
	}
	if gen, ok := svc.Generation(); !ok || gen != 6 {
		t.Errorf("Generation = %d, %v, want 6", gen, ok)
	}
	// The next start is normal.
	next := newService(t, m, r)
	if st, err := next.Check(t.Context()); err != nil || st.Required {
		t.Errorf("the start after the reconciliation = %+v, %v", st, err)
	}
}

// Section 6: the new instance id when there is no row and no valid marker.
func TestReconcileInstanceChoice(t *testing.T) {
	t.Run("no row and a valid marker keeps the marker instance", func(t *testing.T) {
		m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 2}}
		r := &fakeRepo{users: true}
		svc := required(t, m, r)
		if _, err := reconcile(svc, actor, request); err != nil {
			t.Fatal(err)
		}
		if r.began[0].InstanceID != idA || r.began[0].NewGeneration != 3 {
			t.Errorf("input = %+v", r.began[0])
		}
	})
	t.Run("no row and no valid marker creates an instance and starts above 1", func(t *testing.T) {
		m, r := &fakeMarkers{invalid: true}, &fakeRepo{users: true}
		svc := required(t, m, r)
		if _, err := reconcile(svc, actor, request); err != nil {
			t.Fatal(err)
		}
		if r.began[0].InstanceID != newInst || r.began[0].NewGeneration != 2 || r.began[0].Reason != string(recovery.ReasonMarkerInvalid) {
			t.Errorf("input = %+v", r.began[0])
		}
	})
	t.Run("a mismatch keeps the database instance", func(t *testing.T) {
		m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idB, Generation: 1}}
		r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 1}}
		svc := required(t, m, r)
		if _, err := reconcile(svc, actor, request); err != nil {
			t.Fatal(err)
		}
		if m.writes[len(m.writes)-1].InstanceID != idA {
			t.Errorf("marker = %+v", m.writes)
		}
	})
}

func TestReconcileIsRefusedWhenNothingIsRequired(t *testing.T) {
	m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 1}}
	r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 1}}
	svc := newService(t, m, r)
	if _, err := svc.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := reconcile(svc, actor, request); !errors.Is(err, inbound.ErrRecoveryNotRequired) {
		t.Errorf("error = %v, want ErrRecoveryNotRequired", err)
	}
	if len(r.began) != 0 {
		t.Error("phase 1 ran")
	}
}

func TestReconcileRefusesBadActorAndRequestIDs(t *testing.T) {
	m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 3}}
	r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 1}}
	svc := required(t, m, r)
	for _, tc := range [][2]string{{"", request}, {actor, ""}, {"x", request}, {actor, "'; DROP"}} {
		if _, err := reconcile(svc, tc[0], tc[1]); err == nil {
			t.Errorf("Reconcile(%q, %q) succeeded", tc[0], tc[1])
		}
	}
	if len(r.began) != 0 {
		t.Error("phase 1 ran")
	}
}

// G-recovery 6 and 9: a crash or a failure after each phase resumes correctly
// and never leaves the installation normal with the old authority.
func TestReconcileCrashAfterEachPhase(t *testing.T) {
	setup := func() (*fakeMarkers, *fakeRepo) {
		return &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 2, AuditWatermark: 40}},
			&fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 1}, maxSeq: 10}
	}
	t.Run("phase 1 fails: nothing changed, still required", func(t *testing.T) {
		m, r := setup()
		r.failBegin = errBoom
		svc := required(t, m, r)
		if _, err := reconcile(svc, actor, request); !errors.Is(err, errBoom) {
			t.Fatalf("error = %v", err)
		}
		if len(m.writes) != 0 || r.row.Generation != 1 || r.row.Reconciling {
			t.Errorf("state changed: marker writes %d, row %+v", len(m.writes), r.row)
		}
		if _, ok := svc.Generation(); ok {
			t.Error("claims allowed")
		}
		again := newService(t, m, r)
		if st, _ := again.Check(t.Context()); !st.Required {
			t.Error("the next start is not recovery_required")
		}
	})
	t.Run("crash after phase 1: the next start resumes phases 2 and 3", func(t *testing.T) {
		m, r := setup()
		svc := required(t, m, r)
		m.failWrite = errBoom // the marker write of phase 2 fails
		if _, err := reconcile(svc, actor, request); err == nil {
			t.Fatal("Reconcile succeeded without a marker write")
		}
		if _, ok := svc.Generation(); ok {
			t.Error("claims allowed after a failed phase 2")
		}
		// The marker holds 2 and the row holds 3 and reconciling: row 7.
		if !r.row.Reconciling || r.row.Generation != 3 || m.m.Generation != 2 {
			t.Fatalf("marker %+v, row %+v", m.m, r.row)
		}
		m.failWrite = nil
		next := newService(t, m, r)
		st, err := next.Check(t.Context())
		if err != nil || st.Required {
			t.Fatalf("Check = %+v, %v, want the reconciliation resumed", st, err)
		}
		if m.m.Generation != 3 || r.row.Reconciling || m.m.AuditWatermark != r.maxSeq-1 {
			t.Errorf("marker %+v, row %+v, max seq %d", m.m, r.row, r.maxSeq)
		}
		if gen, ok := next.Generation(); !ok || gen != 3 {
			t.Errorf("Generation = %d, %v, want 3", gen, ok)
		}
	})
	t.Run("crash after the marker write: the next start completes phase 3", func(t *testing.T) {
		m, r := setup()
		svc := required(t, m, r)
		r.failComplete = errBoom
		if _, err := reconcile(svc, actor, request); err == nil {
			t.Fatal("Reconcile succeeded without phase 3")
		}
		if _, ok := svc.Generation(); ok {
			t.Error("claims allowed before phase 3")
		}
		if m.m.Generation != r.row.Generation || !r.row.Reconciling {
			t.Fatalf("marker %+v, row %+v", m.m, r.row)
		}
		r.failComplete = nil
		next := newService(t, m, r)
		st, err := next.Check(t.Context())
		if err != nil || st.Required {
			t.Fatalf("Check = %+v, %v, want normal", st, err)
		}
		if r.row.Reconciling || len(r.completed) != 1 || r.completed[0] != string(recovery.ReasonReconcileInterrupted) {
			t.Errorf("row %+v, completed %v", r.row, r.completed)
		}
	})
	t.Run("crash after phase 1 with a marker one generation behind resumes", func(t *testing.T) {
		// The marker holds generation 1 (the old value) and the database holds 2, reconciling.
		m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 1, AuditWatermark: 9}}
		r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 2, Reconciling: true}, maxSeq: 12}
		svc := newService(t, m, r)
		st, err := svc.Check(t.Context())
		if err != nil || st.Required {
			t.Fatalf("Check = %+v, %v", st, err)
		}
		if m.m.Generation != 2 || m.m.AuditWatermark != 12 || r.row.Reconciling {
			t.Errorf("marker %+v, row %+v", m.m, r.row)
		}
		if gen, ok := svc.Generation(); !ok || gen != 2 {
			t.Errorf("Generation = %d, %v", gen, ok)
		}
	})
}

// G-recovery 11: the watermark is advanced before a launch, and a failed marker
// write stops claims until a write succeeds.
func TestAdvanceWatermarkBeforeALaunchAndAFailedWriteStopsClaims(t *testing.T) {
	m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 1, AuditWatermark: 2}}
	r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 1}, maxSeq: 2}
	svc := newService(t, m, r)
	if _, err := svc.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.ClaimsAllowed(); err != nil {
		t.Fatalf("ClaimsAllowed = %v", err)
	}
	r.maxSeq = 9 // a claim was recorded in the audit log
	if err := svc.AdvanceWatermark(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.m.AuditWatermark != 9 {
		t.Errorf("watermark = %d, want 9", m.m.AuditWatermark)
	}
	m.failWrite = errBoom
	r.maxSeq = 12
	if err := svc.AdvanceWatermark(t.Context()); !errors.Is(err, recovery.ErrMarkerWrite) {
		t.Errorf("AdvanceWatermark = %v, want ErrMarkerWrite", err)
	}
	if err := svc.ClaimsAllowed(); !errors.Is(err, recovery.ErrMarkerWrite) {
		t.Errorf("ClaimsAllowed = %v, want ErrMarkerWrite", err)
	}
	m.failWrite = nil
	if err := svc.AdvanceWatermark(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.ClaimsAllowed(); err != nil {
		t.Errorf("ClaimsAllowed after a good write = %v", err)
	}
	if m.m.AuditWatermark != 12 {
		t.Errorf("watermark = %d, want 12", m.m.AuditWatermark)
	}
}

// A marker that disappears while the process runs is not recreated by the
// watermark advance. The advance fails, claims stop, and the next start finds
// the marker missing.
func TestAdvanceWatermarkDoesNotRecreateAMarkerThatVanished(t *testing.T) {
	m := &fakeMarkers{m: &outbound.RecoveryMarker{InstanceID: idA, Generation: 1, AuditWatermark: 2}}
	r := &fakeRepo{row: &outbound.RecoveryRow{InstanceID: idA, Generation: 1}, maxSeq: 2}
	svc := newService(t, m, r)
	if _, err := svc.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	m.m = nil // somebody deletes the file
	if err := svc.AdvanceWatermark(t.Context()); !errors.Is(err, recovery.ErrMarkerWrite) || !errors.Is(err, outbound.ErrMarkerMissing) {
		t.Fatalf("AdvanceWatermark = %v, want ErrMarkerWrite caused by ErrMarkerMissing", err)
	}
	if m.m != nil {
		t.Error("the marker was recreated")
	}
	if err := svc.ClaimsAllowed(); !errors.Is(err, recovery.ErrMarkerWrite) {
		t.Errorf("ClaimsAllowed = %v, want ErrMarkerWrite", err)
	}
}
