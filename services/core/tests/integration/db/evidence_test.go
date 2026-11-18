package db_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

type evEnv struct {
	*env
	repo outbound.EvidenceRepository
	tx   outbound.Transactor
	pool *pgxpool.Pool
}

func newEvEnv(t *testing.T) *evEnv {
	t.Helper()
	e := newEnv(t)
	return &evEnv{env: e, repo: persistence.NewEvidence(e.api), tx: persistence.NewTransactor(e.api), pool: e.api}
}

var evBase = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

// item builds the metadata of content, captured n seconds after evBase.
func (e *evEnv) item(t *testing.T, content string, n int, mod ...func(*evidence.NewItem)) evidence.Item {
	t.Helper()
	in := evidence.NewItem{
		ID: newID(), ProgramID: e.b.ProgramID, AssessmentID: e.b.AssessmentID, Kind: evidence.KindResponse, ContentType: "text/plain",
		Origin: evidence.Observed, Redaction: evidence.Redacted, SHA256: evidence.DigestOf([]byte(content)), Size: int64(len(content)),
		CapturedAt: evBase.Add(time.Duration(n) * time.Second),
	}
	for _, m := range mod {
		m(&in)
	}
	it, err := evidence.New(in)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func (e *evEnv) insert(t *testing.T, it evidence.Item) {
	t.Helper()
	if err := e.tx.InTx(t.Context(), func(ctx context.Context) error { return e.repo.Insert(ctx, it) }); err != nil {
		t.Fatal(err)
	}
}

func (e *evEnv) check(t *testing.T, it evidence.Item, o evidence.Observation, trigger evidence.CheckTrigger) (evidence.CheckRecord, evidence.Status, error) {
	t.Helper()
	c, err := evidence.NewCheck(newID(), it, o, trigger, evidence.CheckActor{Kind: evidence.ActorUser, UserID: e.b.UserID})
	if err != nil {
		t.Fatal(err)
	}
	var (
		stored evidence.CheckRecord
		st     evidence.Status
	)
	err = e.tx.InTx(t.Context(), func(ctx context.Context) error {
		var err error
		stored, st, err = e.repo.RecordCheck(ctx, c)
		return err
	})
	return stored, st, err
}

func good(it evidence.Item) evidence.Observation {
	return evidence.Observation{Found: true, Safe: true, Size: it.Size(), Digest: it.SHA256()}
}

func bad(it evidence.Item) evidence.Observation {
	return evidence.Observation{Found: true, Safe: true, Size: it.Size(), Digest: evidence.DigestOf([]byte("something else"))}
}

// stranger seeds a second user with a program and an assessment.
func (e *evEnv) stranger(t *testing.T) (userID, assessmentID string) {
	t.Helper()
	conn := e.c.Owner(t)
	var program string
	must := func(dst *string, sql string, args ...any) {
		t.Helper()
		if err := conn.QueryRow(t.Context(), sql, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	must(&userID, `INSERT INTO morningstar.users (id, username, status) VALUES (uuidv7(), 'stranger', 'disabled') RETURNING id`)
	must(&program, `INSERT INTO morningstar.programs (id, owner_user_id, name) VALUES (uuidv7(), $1, 'other') RETURNING id`, userID)
	must(&assessmentID, `INSERT INTO morningstar.assessments (id, program_id, name, created_by) VALUES (uuidv7(), $1, 'other', $2) RETURNING id`, program, userID)
	return userID, assessmentID
}

func TestEvidenceIsStoredWithAnUncheckedStatusAndReadBackWhole(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "body", 0, func(in *evidence.NewItem) {
		in.ToolKey, in.ToolVersion = "httpx", "1.6.0"
		in.OperationHash = pgtest.Random(32)
		in.RedactedArgs = []string{"-u", "https://lab.example"}
		in.Sensitivity = evidence.Normal
	})
	e.insert(t, it)

	rec, err := e.repo.Get(t.Context(), e.b.UserID, it.ID())
	if err != nil {
		t.Fatal(err)
	}
	if rec.Item.SHA256() != it.SHA256() || rec.Item.Size() != 4 || rec.Item.ToolKey() != "httpx" || rec.Item.Sensitivity() != evidence.Normal ||
		len(rec.Item.RedactedArgs()) != 2 || !rec.Item.CapturedAt().Equal(it.CapturedAt()) || rec.Item.ProgramID() != e.b.ProgramID {
		t.Errorf("item = %+v", rec.Item)
	}
	if rec.Status.Integrity != evidence.Unchecked || rec.Status.Retention != evidence.RetentionActive || rec.Status.LastCheckedAt != nil || rec.ProgramArchived {
		t.Errorf("status = %+v", rec)
	}
}

func TestAnItemAndItsStatusAreStoredTogetherOrNotAtAll(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "x", 0)
	boom := errors.New("the audit write failed")
	err := e.tx.InTx(t.Context(), func(ctx context.Context) error {
		if err := e.repo.Insert(ctx, it); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var n int
	if err := e.c.Owner(t).QueryRow(t.Context(), `SELECT (SELECT count(*) FROM morningstar.evidence_items) + (SELECT count(*) FROM morningstar.evidence_status)`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows after a rollback = %d, %v", n, err)
	}
}

func TestANameIsStoredOnce(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "x", 0)
	e.insert(t, it)
	err := e.tx.InTx(t.Context(), func(ctx context.Context) error { return e.repo.Insert(ctx, it) })
	if !errors.Is(err, outbound.ErrUniqueViolation) {
		t.Fatalf("err = %v, want a unique violation", err)
	}
}

func TestAnotherOwnersEvidenceCannotBeReachedByItsId(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "x", 0)
	e.insert(t, it)
	if _, _, err := e.check(t, it, good(it), evidence.TriggerManual); err != nil {
		t.Fatal(err)
	}
	other, otherAssessment := e.stranger(t)

	if _, err := e.repo.Get(t.Context(), other, it.ID()); !errors.Is(err, outbound.ErrEvidenceNotFound) {
		t.Errorf("Get: err = %v", err)
	}
	if _, err := e.repo.List(t.Context(), other, e.b.AssessmentID, outbound.EvidenceFilter{}, outbound.EvidencePageQuery{Limit: 10}); !errors.Is(err, outbound.ErrEvidenceNotFound) {
		t.Errorf("List of another owner's assessment: err = %v", err)
	}
	if _, err := e.repo.ListChecks(t.Context(), other, it.ID(), outbound.EvidencePageQuery{Limit: 10}); !errors.Is(err, outbound.ErrEvidenceNotFound) {
		t.Errorf("ListChecks: err = %v", err)
	}
	// The stranger's own assessment is reachable and holds none of this evidence.
	got, err := e.repo.List(t.Context(), other, otherAssessment, outbound.EvidenceFilter{}, outbound.EvidencePageQuery{Limit: 10})
	if err != nil || len(got) != 0 {
		t.Errorf("List of one's own empty assessment = %v, %v", got, err)
	}
	// An owner who is not a user at all.
	if _, err := e.repo.Get(t.Context(), "not-a-uuid", it.ID()); !errors.Is(err, outbound.ErrEvidenceNotFound) {
		t.Errorf("a malformed owner: err = %v", err)
	}
}

func TestEvidenceIsListedInPagesWithFilters(t *testing.T) {
	e := newEvEnv(t)
	app := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, app, "low_impact", 1)
	var items []evidence.Item
	for i := range 5 {
		kind := evidence.KindResponse
		if i%2 == 1 {
			kind = evidence.KindNote
		}
		it := e.item(t, "c", i, func(in *evidence.NewItem) {
			in.Kind = kind
			if i == 4 {
				in.JobID = job
			}
		})
		e.insert(t, it)
		items = append(items, it)
	}
	ids := func(recs []evidence.Record) (out []string) {
		for _, r := range recs {
			out = append(out, r.Item.ID())
		}
		return
	}
	want := func(idx ...int) (out []string) {
		for _, i := range idx {
			out = append(out, items[i].ID())
		}
		return
	}
	same := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	ctx := t.Context()
	list := func(f outbound.EvidenceFilter, q outbound.EvidencePageQuery) []evidence.Record {
		t.Helper()
		recs, err := e.repo.List(ctx, e.b.UserID, e.b.AssessmentID, f, q)
		if err != nil {
			t.Fatal(err)
		}
		return recs
	}

	if got := ids(list(outbound.EvidenceFilter{}, outbound.EvidencePageQuery{Limit: 50})); !same(got, want(0, 1, 2, 3, 4)) {
		t.Errorf("ascending = %v", got)
	}
	if got := ids(list(outbound.EvidenceFilter{}, outbound.EvidencePageQuery{Limit: 50, Descending: true})); !same(got, want(4, 3, 2, 1, 0)) {
		t.Errorf("descending = %v", got)
	}
	// Pages follow the cursor without gaps or repeats, in both directions.
	for _, desc := range []bool{false, true} {
		var seen []string
		var after *outbound.EvidenceCursor
		for range 10 {
			page := list(outbound.EvidenceFilter{}, outbound.EvidencePageQuery{Limit: 2, After: after, Descending: desc})
			if len(page) == 0 {
				break
			}
			seen = append(seen, ids(page)...)
			last := page[len(page)-1].Item
			after = &outbound.EvidenceCursor{At: last.CapturedAt(), ID: last.ID()}
		}
		w := want(0, 1, 2, 3, 4)
		if desc {
			w = want(4, 3, 2, 1, 0)
		}
		if !same(seen, w) {
			t.Errorf("paged (descending=%v) = %v, want %v", desc, seen, w)
		}
	}
	if got := ids(list(outbound.EvidenceFilter{Kind: evidence.KindNote}, outbound.EvidencePageQuery{Limit: 50})); !same(got, want(1, 3)) {
		t.Errorf("kind filter = %v", got)
	}
	if got := ids(list(outbound.EvidenceFilter{JobID: job}, outbound.EvidencePageQuery{Limit: 50})); !same(got, want(4)) {
		t.Errorf("job filter = %v", got)
	}
	if _, _, err := e.check(t, items[2], bad(items[2]), evidence.TriggerManual); err != nil {
		t.Fatal(err)
	}
	if got := ids(list(outbound.EvidenceFilter{Integrity: evidence.Compromised}, outbound.EvidencePageQuery{Limit: 50})); !same(got, want(2)) {
		t.Errorf("integrity filter = %v", got)
	}
	if got := list(outbound.EvidenceFilter{}, outbound.EvidencePageQuery{Limit: 3}); len(got) != 3 {
		t.Errorf("limit 3 gave %d rows", len(got))
	}
	if _, err := e.repo.List(ctx, e.b.UserID, e.b.AssessmentID, outbound.EvidenceFilter{}, outbound.EvidencePageQuery{Limit: 0}); err == nil {
		t.Error("a limit of zero was accepted")
	}
}

func TestACheckMovesTheStateAndACompromisedItemStaysCompromised(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "x", 0)
	e.insert(t, it)

	c, st, err := e.check(t, it, good(it), evidence.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if st.Integrity != evidence.IntegrityOK || st.LastCheckedAt == nil || c.CheckedAt.IsZero() || !st.Integrity.SupportsVerification() {
		t.Fatalf("after a match: %+v, %+v", c, st)
	}

	c, st, err = e.check(t, it, bad(it), evidence.TriggerRead)
	if err != nil {
		t.Fatal(err)
	}
	if st.Integrity != evidence.Compromised || st.CompromisedAt == nil || c.Result != evidence.ResultHashMismatch || c.ObservedSHA256 == nil {
		t.Fatalf("after a mismatch: %+v, %+v", c, st)
	}
	first := *st.CompromisedAt

	// A later match does not clear it, and the time of the first finding is kept.
	_, st, err = e.check(t, it, good(it), evidence.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if st.Integrity != evidence.Compromised || st.Integrity.SupportsVerification() || !st.CompromisedAt.Equal(first) {
		t.Fatalf("after a later match: %+v", st)
	}
	rec, err := e.repo.Get(t.Context(), e.b.UserID, it.ID())
	if err != nil || rec.Status.Integrity != evidence.Compromised || rec.Status.LastCheckedAt == nil {
		t.Fatalf("stored status = %+v, %v", rec.Status, err)
	}

	checks, err := e.repo.ListChecks(t.Context(), e.b.UserID, it.ID(), outbound.EvidencePageQuery{Limit: 10, Descending: true})
	if err != nil || len(checks) != 3 {
		t.Fatalf("history = %v, %v", checks, err)
	}
	if checks[0].Result != evidence.ResultMatch || checks[1].Result != evidence.ResultHashMismatch || checks[2].Result != evidence.ResultMatch {
		t.Errorf("history order = %s, %s, %s", checks[0].Result, checks[1].Result, checks[2].Result)
	}
	if checks[2].Actor.Kind != evidence.ActorUser || checks[2].Actor.UserID != e.b.UserID || checks[2].Trigger != evidence.TriggerManual {
		t.Errorf("first check = %+v", checks[2])
	}
}

func TestAMissingFileMarksTheItemMissingAndAMatchDoesNotRestoreIt(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "x", 0)
	e.insert(t, it)
	_, st, err := e.check(t, it, evidence.Observation{}, evidence.TriggerSweep)
	if err != nil || st.Integrity != evidence.Missing {
		t.Fatalf("after a missing file: %+v, %v", st, err)
	}
	_, st, err = e.check(t, it, good(it), evidence.TriggerManual)
	if err != nil || st.Integrity != evidence.Missing {
		t.Fatalf("after the file came back the state must stay missing (domain-model.md section 3): %+v, %v", st, err)
	}
}

func TestConcurrentChecksNeverEraseACompromise(t *testing.T) {
	e := newEvEnv(t)
	for range 10 {
		it := e.item(t, "x", 0)
		e.insert(t, it)
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				o := good(it)
				if i == 3 {
					o = bad(it)
				}
				if _, _, err := e.check(t, it, o, evidence.TriggerRead); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		rec, err := e.repo.Get(t.Context(), e.b.UserID, it.ID())
		if err != nil || rec.Status.Integrity != evidence.Compromised {
			t.Fatalf("state = %+v, %v: a concurrent match erased the compromise", rec.Status, err)
		}
	}
}

func TestACheckNeedsATransactionAndRefusesAPurgedItem(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "x", 0)
	e.insert(t, it)
	c, err := evidence.NewCheck(newID(), it, good(it), evidence.TriggerManual, evidence.CheckActor{Kind: evidence.ActorSystem})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.repo.RecordCheck(t.Context(), c); err == nil {
		t.Error("a check was recorded outside a transaction")
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.evidence_status SET retention_state = 'purged', purged_at = now(), purged_by = $2 WHERE evidence_id = $1`, it.ID(), e.b.UserID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.check(t, it, good(it), evidence.TriggerManual); !errors.Is(err, outbound.ErrEvidencePurged) {
		t.Errorf("err = %v, want ErrEvidencePurged", err)
	}
	rec, err := e.repo.Get(t.Context(), e.b.UserID, it.ID())
	if err != nil || rec.Status.Retention != evidence.RetentionPurged || rec.Status.PurgedAt == nil {
		t.Errorf("status = %+v, %v", rec.Status, err)
	}
}

func TestTheAssessmentFactsForCaptureComeFromTheDatabase(t *testing.T) {
	e := newEvEnv(t)
	got, err := e.repo.Assessment(t.Context(), e.b.AssessmentID)
	if err != nil || got.ProgramID != e.b.ProgramID || got.RawCapture || got.ProgramArchived {
		t.Fatalf("assessment = %+v, %v", got, err)
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.assessments SET raw_evidence_capture = true WHERE id = $1`, e.b.AssessmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.programs SET status = 'archived', archived_at = now() WHERE id = $1`, e.b.ProgramID); err != nil {
		t.Fatal(err)
	}
	got, err = e.repo.Assessment(t.Context(), e.b.AssessmentID)
	if err != nil || !got.RawCapture || !got.ProgramArchived {
		t.Fatalf("assessment = %+v, %v", got, err)
	}
	if _, err := e.repo.Assessment(t.Context(), newID()); !errors.Is(err, outbound.ErrEvidenceNotFound) {
		t.Errorf("an unknown assessment: err = %v", err)
	}
}
