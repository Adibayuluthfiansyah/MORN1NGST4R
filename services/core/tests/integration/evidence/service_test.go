// Package evidence_test runs the evidence use case over a real PostgreSQL
// cluster, the real file store and the real audit sink (milestone M6).
package evidence_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/evidencefile"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	evidenceapp "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

const storeLimit = 4096

var fastPolicy = audit.RetryPolicy{MaxAttempts: 3, Delays: []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}, AttemptTimeout: 2 * time.Second}

type principal struct{ user string }

func (p principal) UserID() string        { return p.user }
func (p principal) SessionID() string     { return ids.New() }
func (p principal) ValidCSRF(string) bool { return true }
func (p principal) Actor() actor.Actor    { a, _ := actor.New(actor.KindHuman, p.user); return a }

// flaky wraps the audit sink so that a test can make a kind of write fail.
type flaky struct {
	outbound.AuditSink
	failRequire, failRecord atomic.Bool
}

func (f *flaky) Require(ctx context.Context, e audit.Event) error {
	if f.failRequire.Load() {
		return outbound.ErrAuditNotRecorded
	}
	return f.AuditSink.Require(ctx, e)
}

func (f *flaky) Record(ctx context.Context, e audit.Event) error {
	if f.failRecord.Load() {
		return outbound.ErrAuditNotRecorded
	}
	return f.AuditSink.Record(ctx, e)
}

type env struct {
	c     *pgtest.Cluster
	b     pgtest.Base
	dir   string
	store *evidencefile.Store
	svc   *evidenceapp.Service
	deps  evidenceapp.Deps
	sink  *flaky
	me    principal
}

func newEnv(t *testing.T) *env {
	t.Helper()
	c := pgtest.Get(t)
	c.Fresh(t)
	b := c.SeedBase(t)
	pool := c.Pool(t, persistence.RoleAPI, 6)
	dir := filepath.Join(t.TempDir(), "evidence")
	store, err := evidencefile.New(dir, storeLimit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sink, err := persistence.NewAuditSink(pool, fastPolicy, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := &flaky{AuditSink: sink}
	deps := evidenceapp.Deps{
		Store: store, Repo: persistence.NewEvidence(pool), Audit: f, Tx: persistence.NewTransactor(pool),
		NewID: ids.New, Now: func() time.Time { return time.Now().UTC() },
	}
	return &env{c: c, b: b, dir: dir, store: store, svc: evidenceapp.New(deps), deps: deps, sink: f, me: principal{user: b.UserID}}
}

func (e *env) who() audit.Actor {
	a, err := audit.User(e.b.UserID)
	if err != nil {
		panic(err)
	}
	return a
}

func (e *env) input(content string) inbound.CaptureInput {
	return inbound.CaptureInput{
		AssessmentID: e.b.AssessmentID, Kind: evidence.KindResponse, ContentType: "text/plain", Origin: evidence.Observed,
		Content: []byte(content), Actor: e.who(), RequestID: ids.New(),
	}
}

func (e *env) capture(t *testing.T, content string) evidence.Item {
	t.Helper()
	it, err := e.svc.Capture(t.Context(), e.input(content))
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func (e *env) path(it evidence.Item) string { return filepath.Join(e.dir, it.StorageKey()) }

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.c.Owner(t).QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (e *env) events(t *testing.T, action string, outcome audit.Outcome) int {
	t.Helper()
	return e.count(t, `SELECT count(*) FROM morningstar.audit_events WHERE action = $1 AND outcome = $2`, action, string(outcome))
}

func (e *env) state(t *testing.T, it evidence.Item) evidence.Status {
	t.Helper()
	rec, err := e.svc.Get(t.Context(), e.me, it.ID())
	if err != nil {
		t.Fatal(err)
	}
	return rec.Status
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, x := range es {
		out = append(out, x.Name())
	}
	return out
}

// secret is built at run time, so that this file carries no credential-shaped literal.
var secret = strings.Repeat("s3cr3t", 6)

func withSecret() string {
	return "GET /account HTTP/1.1\r\nHost: lab.example\r\nAuthorization: Bearer " + secret + "\r\n\r\nbody"
}

func TestCaptureStoresTheRedactedBytesAndTheirHash(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, withSecret())

	if it.Redaction() != evidence.Redacted || it.Sensitivity() != evidence.Sensitive || it.ProgramID() != e.b.ProgramID {
		t.Fatalf("item = %+v", it)
	}
	onDisk, err := os.ReadFile(e.path(it))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, []byte(secret)) {
		t.Fatal("the secret reached the disk")
	}
	if !bytes.Contains(onDisk, []byte("Host: lab.example")) || !bytes.Contains(onDisk, []byte("body")) {
		t.Errorf("redaction removed more than the secret: %q", onDisk)
	}
	if it.SHA256() != evidence.DigestOf(onDisk) || it.Size() != int64(len(onDisk)) {
		t.Errorf("the stored hash or size is not the one of the stored bytes")
	}
	if fi, _ := os.Stat(e.path(it)); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode())
	}
	st := e.state(t, it)
	if st.Integrity != evidence.Unchecked || st.Retention != evidence.RetentionActive {
		t.Errorf("status = %+v", st)
	}
	if n := e.events(t, "evidence.captured", audit.Success); n != 1 {
		t.Errorf("evidence.captured events = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.audit_events WHERE detail::text LIKE '%' || $1 || '%'`, secret); n != 0 {
		t.Errorf("the secret reached the audit trail")
	}
}

func TestRawCaptureIsOffByDefaultAndNeedsTheAssessmentSetting(t *testing.T) {
	e := newEnv(t)

	in := e.input(withSecret())
	in.Raw = true
	if _, err := e.svc.Capture(t.Context(), in); !errors.Is(err, inbound.ErrRawCaptureNotAllowed) {
		t.Fatalf("raw without the setting: err = %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_items`); n != 0 || len(files(t, e.dir)) != 0 {
		t.Fatalf("a refused raw capture left %d rows and %v", n, files(t, e.dir))
	}

	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.assessments SET raw_evidence_capture = true WHERE id = $1`, e.b.AssessmentID); err != nil {
		t.Fatal(err)
	}
	// The setting alone changes nothing: a capture that did not ask for raw is redacted.
	it := e.capture(t, withSecret())
	if b, _ := os.ReadFile(e.path(it)); it.Redaction() != evidence.Redacted || bytes.Contains(b, []byte(secret)) {
		t.Fatalf("the setting alone kept the secret (%s)", it.Redaction())
	}
	// Asked for and allowed: raw, and marked sensitive.
	in.Sensitivity = evidence.Normal
	raw, err := e.svc.Capture(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(e.path(raw)); raw.Redaction() != evidence.RawRetained || raw.Sensitivity() != evidence.Sensitive || !bytes.Contains(b, []byte(secret)) {
		t.Fatalf("raw capture = %s, %s", raw.Redaction(), raw.Sensitivity())
	}
}

func TestACaptureThatCannotBeAuditedLeavesNothingBehind(t *testing.T) {
	e := newEnv(t)
	e.sink.failRequire.Store(true)
	if _, err := e.svc.Capture(t.Context(), e.input("x")); err == nil {
		t.Fatal("a capture without its audit record succeeded")
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_items`); n != 0 {
		t.Errorf("%d rows stayed", n)
	}
	if got := files(t, e.dir); len(got) != 0 {
		t.Errorf("an orphan file stayed: %v", got)
	}
}

// liar is a store that reports another hash than the one of the bytes it wrote.
type liar struct{ outbound.EvidenceStore }

func (l liar) Put(ctx context.Context, key string, content []byte) (outbound.EvidenceWritten, error) {
	w, err := l.EvidenceStore.Put(ctx, key, content)
	w.Digest[0] ^= 1
	return w, err
}

// lateFailure publishes the file and then fails, as a store does when the sync of the directory fails.
type lateFailure struct{ outbound.EvidenceStore }

func (l lateFailure) Put(ctx context.Context, key string, content []byte) (outbound.EvidenceWritten, error) {
	if _, err := l.EvidenceStore.Put(ctx, key, content); err != nil {
		return outbound.EvidenceWritten{}, err
	}
	return outbound.EvidenceWritten{}, errors.New("sync the directory failed")
}

func TestAPublishedFileOfAFailedStoreWriteIsNotLeftBehind(t *testing.T) {
	e := newEnv(t)
	d := e.deps
	d.Store = lateFailure{e.store}
	if _, err := evidenceapp.New(d).Capture(t.Context(), e.input(withSecret())); err == nil {
		t.Fatal("a capture whose store failed after publishing succeeded")
	}
	if got := files(t, e.dir); len(got) != 0 {
		t.Errorf("a file that no row describes stayed: %v", got)
	}
}

func TestACaptureNeverRemovesAFileThatItDidNotPublish(t *testing.T) {
	e := newEnv(t)
	first := e.capture(t, "the first")
	d := e.deps
	d.NewID = func() string { return first.ID() } // an id that is already taken
	if _, err := evidenceapp.New(d).Capture(t.Context(), e.input("the second")); err == nil {
		t.Fatal("a capture under a taken name succeeded")
	}
	if b, err := os.ReadFile(e.path(first)); err != nil || string(b) != "the first" {
		t.Fatalf("the file of the first capture = %q, %v", b, err)
	}
}

func TestACaptureIsRefusedWhenTheStoreReportsOtherBytes(t *testing.T) {
	e := newEnv(t)
	d := e.deps
	d.Store = liar{e.store}
	if _, err := evidenceapp.New(d).Capture(t.Context(), e.input("x")); err == nil {
		t.Fatal("a capture whose stored hash differs from the expected one succeeded")
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_items`); n != 0 || len(files(t, e.dir)) != 0 {
		t.Errorf("a refused capture left %d rows and %v", n, files(t, e.dir))
	}
}

func TestCaptureRefusesWhatItCannotStoreSafely(t *testing.T) {
	e := newEnv(t)
	in := e.input(strings.Repeat("x", storeLimit+1))
	var verr *inbound.ValidationError
	if _, err := e.svc.Capture(t.Context(), in); !errors.As(err, &verr) {
		t.Errorf("too large: err = %v", err)
	}
	in = e.input("x")
	in.AssessmentID = ids.New()
	if _, err := e.svc.Capture(t.Context(), in); !errors.Is(err, inbound.ErrNotFound) {
		t.Errorf("unknown assessment: err = %v", err)
	}
	in = e.input("x")
	in.Kind = "screenshot"
	if _, err := e.svc.Capture(t.Context(), in); !errors.As(err, &verr) {
		t.Errorf("unknown kind: err = %v", err)
	}
	in = e.input("x")
	in.Actor = audit.Actor{}
	if _, err := e.svc.Capture(t.Context(), in); err == nil {
		t.Error("a capture without an actor was accepted")
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.programs SET status = 'archived', archived_at = now() WHERE id = $1`, e.b.ProgramID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Capture(t.Context(), e.input("x")); !errors.Is(err, inbound.ErrProgramArchived) {
		t.Errorf("archived program: err = %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_items`); n != 0 || len(files(t, e.dir)) != 0 {
		t.Errorf("refused captures left %d rows and %v", n, files(t, e.dir))
	}
}

func TestOpenReturnsVerifiedBytesAndRecordsTheAccessBeforeThem(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "the body")
	got, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), false)
	if err != nil || string(got.Data) != "the body" || got.ID != it.ID() {
		t.Fatalf("Open = %+v, %v", got, err)
	}
	if n := e.events(t, "evidence.accessed", audit.Success); n != 1 {
		t.Errorf("evidence.accessed events = %d", n)
	}
	if st := e.state(t, it); st.Integrity != evidence.IntegrityOK || st.LastCheckedAt == nil {
		t.Errorf("status = %+v", st)
	}
	checks, err := e.svc.Checks(t.Context(), e.me, it.ID(), inbound.PageInput{})
	if err != nil || len(checks.Items) != 1 || checks.Items[0].Trigger != evidence.TriggerRead || checks.Items[0].Result != evidence.ResultMatch {
		t.Errorf("history = %+v, %v", checks, err)
	}

	// Without the access record no byte leaves, and the integrity check is not recorded either.
	e.sink.failRequire.Store(true)
	got, err = e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), false)
	if err == nil || got.Data != nil {
		t.Fatalf("Open without its audit record = %+v, %v", got, err)
	}
	if n := e.events(t, "evidence.accessed", audit.Success); n != 1 {
		t.Errorf("evidence.accessed events = %d after a failed write", n)
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_integrity_checks`); n != 1 {
		t.Errorf("a check was recorded for an access that did not happen: %d", n)
	}
}

func TestATamperedFileIsDetectedMarkedAndWithheld(t *testing.T) {
	for name, tamper := range map[string]func(t *testing.T, e *env, it evidence.Item){
		"overwrite in place": func(t *testing.T, e *env, it evidence.Item) {
			if err := os.WriteFile(e.path(it), bytes.Repeat([]byte("z"), int(it.Size())), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"append": func(t *testing.T, e *env, it evidence.Item) {
			f, err := os.OpenFile(e.path(it), os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			_, _ = f.WriteString("!")
		},
		"swap for a symlink to a copy with the same bytes": func(t *testing.T, e *env, it evidence.Item) {
			b, _ := os.ReadFile(e.path(it))
			copyPath := filepath.Join(e.dir, "copy")
			if err := os.WriteFile(copyPath, b, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(e.path(it)); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("copy", e.path(it)); err != nil {
				t.Fatal(err)
			}
		},
		"hard link": func(t *testing.T, e *env, it evidence.Item) {
			if err := os.Link(e.path(it), filepath.Join(e.dir, "alias")); err != nil {
				t.Fatal(err)
			}
		},
		"removal": func(t *testing.T, e *env, it evidence.Item) {
			if err := os.Remove(e.path(it)); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			it := e.capture(t, "the original body")
			tamper(t, e, it)

			got, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), false)
			if !errors.Is(err, inbound.ErrEvidenceIntegrityFailed) || got.Data != nil {
				t.Fatalf("Open = %+v, %v", got, err)
			}
			st := e.state(t, it)
			if st.Integrity != evidence.Compromised && st.Integrity != evidence.Missing {
				t.Errorf("state = %s, want compromised or missing", st.Integrity)
			}
			if name != "removal" && (st.Integrity != evidence.Compromised || st.CompromisedAt == nil) {
				t.Errorf("state = %+v, want compromised", st)
			}
			if n := e.events(t, "evidence.integrity_failed", audit.Failure); n != 1 {
				t.Errorf("evidence.integrity_failed events = %d", n)
			}
			if n := e.events(t, "evidence.accessed", audit.Success); n != 0 {
				t.Errorf("a refused access was recorded as a success")
			}
			if n := e.events(t, "evidence.accessed", audit.Denied); n != 1 {
				t.Errorf("evidence.accessed denials = %d", n)
			}
		})
	}
}

func TestACompromiseIsStickyAndOnlyAnAcknowledgedReadGetsBytes(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "the original body")
	if err := os.WriteFile(e.path(it), []byte("the forgery  body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), false); !errors.Is(err, inbound.ErrEvidenceIntegrityFailed) {
		t.Fatal(err)
	}

	// The file is put back. The item stays compromised until a person decides.
	if err := os.WriteFile(e.path(it), []byte("the original body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), false); !errors.Is(err, inbound.ErrEvidenceIntegrityFailed) {
		t.Fatalf("a compromised item was served after its file was restored: %v", err)
	}
	c, err := e.svc.Check(t.Context(), e.me, ids.New(), it.ID())
	if err != nil || c.Result != evidence.ResultMatch {
		t.Fatalf("Check = %+v, %v", c, err)
	}
	if st := e.state(t, it); st.Integrity != evidence.Compromised || st.Integrity.SupportsVerification() {
		t.Fatalf("a matching check cleared the compromise: %+v", st)
	}

	got, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), true)
	if err != nil || string(got.Data) != "the original body" {
		t.Fatalf("an acknowledged read = %+v, %v", got, err)
	}
	var ack string
	if err := e.c.Owner(t).QueryRow(t.Context(), `SELECT detail->>'acknowledged' FROM morningstar.audit_events WHERE action = 'evidence.accessed' AND outcome = 'success'`).Scan(&ack); err != nil || ack != "true" {
		t.Errorf("the acknowledgement is not in the audit record: %q, %v", ack, err)
	}

	// An acknowledged read of the forged bytes returns them, and the record says so.
	if err := os.WriteFile(e.path(it), []byte("the forgery  body"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), true)
	if err != nil || string(got.Data) != "the forgery  body" {
		t.Fatalf("an acknowledged read of a forged file = %+v, %v", got, err)
	}
	// Nothing at all is returned for a file that is not safe to read, acknowledged or not.
	if err := os.Remove(e.path(it)); err != nil {
		t.Fatal(err)
	}
	if got, err = e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), true); !errors.Is(err, inbound.ErrEvidenceIntegrityFailed) || got.Data != nil {
		t.Fatalf("an acknowledged read of a missing file = %+v, %v", got, err)
	}
}

func TestAnAcknowledgementGivenInAdvanceDoesNotReleaseABrandNewFailure(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "the original body")
	if c, err := e.svc.Check(t.Context(), e.me, ids.New(), it.ID()); err != nil || c.Result != evidence.ResultMatch {
		t.Fatalf("Check = %+v, %v", c, err)
	}
	if err := os.WriteFile(e.path(it), []byte("the forgery  body"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The item was intact when the person asked, so the person has not seen the failure yet.
	got, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), true)
	if !errors.Is(err, inbound.ErrEvidenceIntegrityFailed) || got.Data != nil {
		t.Fatalf("a read that found the failure first = %+v, %v", got, err)
	}
	if st := e.state(t, it); st.Integrity != evidence.Compromised {
		t.Fatalf("state = %+v", st)
	}
	if n := e.events(t, "evidence.accessed", audit.Success); n != 0 {
		t.Errorf("the refused read was recorded as an access")
	}
	// Now the person knows, and the acknowledgement counts.
	got, err = e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), true)
	if err != nil || string(got.Data) != "the forgery  body" {
		t.Fatalf("an acknowledged read after the failure was shown = %+v, %v", got, err)
	}
}

func TestAnIntegrityVerdictStandsWhenAuditCannotBeWritten(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "the original body")
	if err := os.WriteFile(e.path(it), []byte("the forgery  body"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.sink.failRequire.Store(true)
	e.sink.failRecord.Store(true)
	c, err := e.svc.Check(t.Context(), e.me, ids.New(), it.ID())
	if err != nil || c.Result != evidence.ResultHashMismatch {
		t.Fatalf("Check = %+v, %v", c, err)
	}
	if st := e.state(t, it); st.Integrity != evidence.Compromised {
		t.Fatalf("the finding was lost with the audit write: %+v", st)
	}
}

func TestACheckThatCannotBeAuditedIsNotRecorded(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "x")
	e.sink.failRequire.Store(true)
	if _, err := e.svc.Check(t.Context(), e.me, ids.New(), it.ID()); err == nil {
		t.Fatal("a matching check without its audit record succeeded")
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_integrity_checks`); n != 0 {
		t.Errorf("%d checks recorded", n)
	}
	if st := e.state(t, it); st.Integrity != evidence.Unchecked {
		t.Errorf("state = %+v", st)
	}
}

func TestACheckRecordsItsActorAndTrigger(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "x")
	reqID := ids.New()
	c, err := e.svc.Check(t.Context(), e.me, reqID, it.ID())
	if err != nil || c.Result != evidence.ResultMatch || c.Trigger != evidence.TriggerManual || c.Actor.Kind != evidence.ActorUser || c.Actor.UserID != e.b.UserID || c.RequestID != reqID {
		t.Fatalf("Check = %+v, %v", c, err)
	}
	if n := e.events(t, "evidence.integrity_checked", audit.Success); n != 1 {
		t.Errorf("evidence.integrity_checked events = %d", n)
	}
	if st := e.state(t, it); st.Integrity != evidence.IntegrityOK || !st.Integrity.SupportsVerification() {
		t.Errorf("state = %+v", st)
	}
}

func TestPurgedAndArchivedEvidenceAreRefused(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "x")
	owner := e.c.Owner(t)
	if _, err := owner.Exec(t.Context(), `UPDATE morningstar.programs SET status = 'archived', archived_at = now() WHERE id = $1`, e.b.ProgramID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Check(t.Context(), e.me, ids.New(), it.ID()); !errors.Is(err, inbound.ErrProgramArchived) {
		t.Errorf("Check on an archived program: err = %v", err)
	}
	if _, err := owner.Exec(t.Context(), `UPDATE morningstar.evidence_status SET retention_state = 'purged', purged_at = now(), purged_by = $2 WHERE evidence_id = $1`, it.ID(), e.b.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(t.Context(), `UPDATE morningstar.programs SET status = 'active', archived_at = NULL WHERE id = $1`, e.b.ProgramID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Check(t.Context(), e.me, ids.New(), it.ID()); !errors.Is(err, inbound.ErrEvidencePurged) {
		t.Errorf("Check: err = %v", err)
	}
	if got, err := e.svc.Open(t.Context(), e.me, ids.New(), it.ID(), true); !errors.Is(err, inbound.ErrEvidencePurged) || got.Data != nil {
		t.Errorf("Open: %+v, %v", got, err)
	}
	if n := e.events(t, "evidence.accessed", audit.Denied); n != 1 {
		t.Errorf("a refused read of purged content was recorded %d times, want once", n)
	}
	if n := e.count(t, `SELECT count(*) FROM morningstar.evidence_integrity_checks`); n != 0 {
		t.Errorf("a purged item was checked: %d checks", n)
	}
	if rec, err := e.svc.Get(t.Context(), e.me, it.ID()); err != nil || rec.Status.Retention != evidence.RetentionPurged {
		t.Errorf("metadata of purged evidence stays readable: %+v, %v", rec.Status, err)
	}
}

func TestAnotherUserSeesNothing(t *testing.T) {
	e := newEnv(t)
	it := e.capture(t, "x")
	other := principal{user: ids.New()}
	if _, err := e.svc.Get(t.Context(), other, it.ID()); !errors.Is(err, inbound.ErrNotFound) {
		t.Errorf("Get: %v", err)
	}
	if _, err := e.svc.Check(t.Context(), other, ids.New(), it.ID()); !errors.Is(err, inbound.ErrNotFound) {
		t.Errorf("Check: %v", err)
	}
	if got, err := e.svc.Open(t.Context(), other, ids.New(), it.ID(), true); !errors.Is(err, inbound.ErrNotFound) || got.Data != nil {
		t.Errorf("Open: %+v, %v", got, err)
	}
	if _, err := e.svc.Checks(t.Context(), other, it.ID(), inbound.PageInput{}); !errors.Is(err, inbound.ErrNotFound) {
		t.Errorf("Checks: %v", err)
	}
	if _, err := e.svc.List(t.Context(), other, inbound.EvidenceListInput{AssessmentID: e.b.AssessmentID}); !errors.Is(err, inbound.ErrNotFound) {
		t.Errorf("List: %v", err)
	}
	for _, id := range []string{"", "not-a-uuid", "../etc/passwd", strings.ToUpper(it.ID())} {
		if _, err := e.svc.Get(t.Context(), e.me, id); !errors.Is(err, inbound.ErrNotFound) {
			t.Errorf("Get(%q): %v", id, err)
		}
	}
	if n := e.events(t, "evidence.accessed", audit.Success); n != 0 {
		t.Errorf("%d accesses were recorded for refused requests", n)
	}
}

func TestListsPageWithAnOpaqueCursor(t *testing.T) {
	e := newEnv(t)
	var made []evidence.Item
	for i := range 5 {
		made = append(made, e.capture(t, strings.Repeat("a", i+1)))
		time.Sleep(2 * time.Millisecond)
	}
	var seen []string
	cursor := ""
	for range 10 {
		page, err := e.svc.List(t.Context(), e.me, inbound.EvidenceListInput{AssessmentID: e.b.AssessmentID, Page: inbound.PageInput{Limit: 2, Cursor: cursor, Descending: true}})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range page.Items {
			seen = append(seen, r.Item.ID())
		}
		if page.Page.Limit != 2 || page.Page.HasMore != (page.Page.NextCursor != "") {
			t.Fatalf("page = %+v", page.Page)
		}
		if !page.Page.HasMore {
			break
		}
		cursor = page.Page.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("seen %d items: %v", len(seen), seen)
	}
	for i, it := range made {
		if seen[4-i] != it.ID() {
			t.Errorf("position %d = %s, want %s", 4-i, seen[4-i], it.ID())
		}
	}

	page, err := e.svc.List(t.Context(), e.me, inbound.EvidenceListInput{AssessmentID: e.b.AssessmentID})
	if err != nil || page.Page.Limit != 50 || len(page.Items) != 5 || page.Page.HasMore || page.Page.NextCursor != "" {
		t.Errorf("default page = %+v, %v", page.Page, err)
	}
	var verr *inbound.ValidationError
	for name, in := range map[string]inbound.EvidenceListInput{
		"limit 0":          {AssessmentID: e.b.AssessmentID, Page: inbound.PageInput{Limit: -1}},
		"limit over 200":   {AssessmentID: e.b.AssessmentID, Page: inbound.PageInput{Limit: 201}},
		"forged cursor":    {AssessmentID: e.b.AssessmentID, Page: inbound.PageInput{Cursor: "AAAA"}},
		"cursor not base":  {AssessmentID: e.b.AssessmentID, Page: inbound.PageInput{Cursor: "%%%"}},
		"unknown kind":     {AssessmentID: e.b.AssessmentID, Kind: "screenshot"},
		"unknown state":    {AssessmentID: e.b.AssessmentID, IntegrityState: "fine"},
		"job not a uuid":   {AssessmentID: e.b.AssessmentID, JobID: "x"},
		"huge cursor text": {AssessmentID: e.b.AssessmentID, Page: inbound.PageInput{Cursor: strings.Repeat("A", 5000)}},
	} {
		if _, err := e.svc.List(t.Context(), e.me, in); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := e.svc.List(t.Context(), e.me, inbound.EvidenceListInput{AssessmentID: "x"}); !errors.Is(err, inbound.ErrNotFound) {
		t.Errorf("a malformed assessment id: %v", err)
	}
}
