package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence/sqlcgen"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// maxPageRows bounds a page whatever the caller asks for.
const maxPageRows = 201

// Evidence implements outbound.EvidenceRepository. It never updates or deletes
// an item or a check: the tables refuse it, and only the integrity columns of
// the status row change (migration 00017).
type Evidence struct{ base }

var _ outbound.EvidenceRepository = Evidence{}

func (d *DB) Evidence() outbound.EvidenceRepository { return NewEvidence(d.pool) }

// NewEvidence returns the repository over pool. The server and worker roles use it.
func NewEvidence(pool *pgxpool.Pool) outbound.EvidenceRepository { return Evidence{base{pool}} }

func pgTime(t pgtype.Timestamptz) time.Time { return t.Time.UTC() }

func pgTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func nullText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

func limitRows(n int) (int32, error) {
	if n < 1 {
		return 0, errors.New("persistence: the page limit must be positive")
	}
	return int32(min(n, maxPageRows)), nil
}

func (r Evidence) Assessment(ctx context.Context, assessmentID string) (outbound.EvidenceAssessment, error) {
	id, err := uuidOf(assessmentID)
	if err != nil {
		return outbound.EvidenceAssessment{}, outbound.ErrEvidenceNotFound
	}
	row, err := r.q(ctx).EvidenceAssessment(ctx, id)
	if noRows(err) {
		return outbound.EvidenceAssessment{}, outbound.ErrEvidenceNotFound
	}
	if err != nil {
		return outbound.EvidenceAssessment{}, wrap("read the assessment of evidence", err)
	}
	return outbound.EvidenceAssessment{ProgramID: row.ProgramID, RawCapture: row.RawCapture, ProgramArchived: row.ProgramArchived}, nil
}

func (r Evidence) Insert(ctx context.Context, it evidence.Item) error {
	id, err := uuidOf(it.ID())
	if err != nil {
		return err
	}
	program, err := uuidOf(it.ProgramID())
	if err != nil {
		return err
	}
	assessment, err := uuidOf(it.AssessmentID())
	if err != nil {
		return err
	}
	job, err := uuidOrNull(it.JobID())
	if err != nil {
		return err
	}
	attempt, err := uuidOrNull(it.AttemptID())
	if err != nil {
		return err
	}
	target, err := uuidOrNull(it.TargetID())
	if err != nil {
		return err
	}
	decision, err := uuidOrNull(it.DecisionID())
	if err != nil {
		return err
	}
	var args []byte
	if a := it.RedactedArgs(); len(a) > 0 {
		if args, err = json.Marshal(a); err != nil {
			return fmt.Errorf("persistence: encode the arguments of evidence: %w", err)
		}
	}
	sum := it.SHA256()
	q := r.q(ctx)
	if err := q.InsertEvidenceItem(ctx, sqlcgen.InsertEvidenceItemParams{
		Column1: id, Column2: program, Column3: assessment, JobID: job, AttemptID: attempt, TargetID: target,
		Kind: string(it.Kind()), StoreBackend: evidence.StoreBackend, StorageKey: it.StorageKey(), Sha256: sum[:],
		SizeBytes: it.Size(), ContentType: it.ContentType(), RedactionState: string(it.Redaction()), Sensitivity: string(it.Sensitivity()),
		CapturedAt: pgtype.Timestamptz{Time: it.CapturedAt(), Valid: true}, Origin: string(it.Origin()),
		ToolKey: nullText(it.ToolKey()), ToolVersion: nullText(it.ToolVersion()),
		ToolBinarySha256: nullBytes(it.ToolBinarySHA256()), OperationHash: nullBytes(it.OperationHash()),
		DecisionID: decision, RedactedArgs: args,
	}); err != nil {
		return wrap("insert evidence", err)
	}
	return wrap("insert the status of evidence", q.InsertEvidenceStatus(ctx, id))
}

func toRecord(i sqlcgen.MorningstarEvidenceItem, s sqlcgen.MorningstarEvidenceStatus, archived bool, lastChecked pgtype.Timestamptz) (evidence.Record, error) {
	var args []string
	if len(i.RedactedArgs) > 0 {
		if err := json.Unmarshal(i.RedactedArgs, &args); err != nil {
			return evidence.Record{}, fmt.Errorf("persistence: decode the arguments of evidence: %w", err)
		}
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	it, err := evidence.New(evidence.NewItem{
		ID: i.ID.String(), ProgramID: i.ProgramID.String(), AssessmentID: i.AssessmentID.String(),
		JobID: uuidString(i.JobID), AttemptID: uuidString(i.AttemptID), TargetID: uuidString(i.TargetID), DecisionID: uuidString(i.DecisionID),
		Kind: evidence.Kind(i.Kind), ContentType: i.ContentType, Origin: evidence.Origin(i.Origin),
		Redaction: evidence.Redaction(i.RedactionState), Sensitivity: evidence.Sensitivity(i.Sensitivity),
		SHA256: evidence.DigestFromBytes(i.Sha256), Size: i.SizeBytes, CapturedAt: pgTime(i.CapturedAt),
		ToolKey: str(i.ToolKey), ToolVersion: str(i.ToolVersion),
		ToolBinarySHA256: nullBytes(i.ToolBinarySha256), OperationHash: nullBytes(i.OperationHash), RedactedArgs: args,
	})
	if err != nil {
		return evidence.Record{}, fmt.Errorf("persistence: a stored evidence row is not valid: %w", err)
	}
	return evidence.Record{
		Item: it,
		Status: evidence.Status{
			Integrity: evidence.IntegrityState(s.IntegrityState), Retention: evidence.Retention(s.RetentionState),
			LastCheckedAt: pgTimePtr(lastChecked), CompromisedAt: pgTimePtr(s.CompromisedAt), PurgedAt: pgTimePtr(s.PurgedAt),
		},
		ProgramArchived: archived,
	}, nil
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return u.String()
}

func (r Evidence) Get(ctx context.Context, ownerID, id string) (evidence.Record, error) {
	eid, err := uuidOf(id)
	if err != nil {
		return evidence.Record{}, outbound.ErrEvidenceNotFound
	}
	owner, err := uuidOf(ownerID)
	if err != nil {
		return evidence.Record{}, outbound.ErrEvidenceNotFound
	}
	row, err := r.q(ctx).GetEvidence(ctx, sqlcgen.GetEvidenceParams{Column1: eid, Column2: owner})
	if noRows(err) {
		return evidence.Record{}, outbound.ErrEvidenceNotFound
	}
	if err != nil {
		return evidence.Record{}, wrap("read evidence", err)
	}
	return toRecord(row.MorningstarEvidenceItem, row.MorningstarEvidenceStatus, row.ProgramArchived, row.LastCheckedAt)
}

func (r Evidence) List(ctx context.Context, ownerID, assessmentID string, f outbound.EvidenceFilter, q outbound.EvidencePageQuery) ([]evidence.Record, error) {
	aid, err := uuidOf(assessmentID)
	if err != nil {
		return nil, outbound.ErrEvidenceNotFound
	}
	owner, err := uuidOf(ownerID)
	if err != nil {
		return nil, outbound.ErrEvidenceNotFound
	}
	rows, err := limitRows(q.Limit)
	if err != nil {
		return nil, err
	}
	job, err := uuidOrNull(f.JobID)
	if err != nil {
		return nil, err
	}
	var at pgtype.Timestamptz
	var after pgtype.UUID
	if q.After != nil {
		if after, err = uuidOf(q.After.ID); err != nil {
			return nil, err
		}
		at = pgtype.Timestamptz{Time: q.After.At, Valid: true}
	}
	kind, state := nullText(string(f.Kind)), nullText(string(f.Integrity))
	qs := r.q(ctx)
	owned, err := qs.EvidenceAssessmentOwned(ctx, sqlcgen.EvidenceAssessmentOwnedParams{Column1: aid, Column2: owner})
	if err != nil {
		return nil, wrap("check the owner of an assessment", err)
	}
	if !owned {
		return nil, outbound.ErrEvidenceNotFound
	}
	var out []evidence.Record
	if q.Descending {
		list, err := qs.ListEvidenceDesc(ctx, sqlcgen.ListEvidenceDescParams{
			Column1: aid, Column2: owner, Kind: kind, IntegrityState: state, JobID: job, BeforeAt: at, BeforeID: after, MaxRows: rows,
		})
		if err != nil {
			return nil, wrap("list evidence", err)
		}
		for _, row := range list {
			rec, err := toRecord(row.MorningstarEvidenceItem, row.MorningstarEvidenceStatus, row.ProgramArchived, row.LastCheckedAt)
			if err != nil {
				return nil, err
			}
			out = append(out, rec)
		}
		return out, nil
	}
	list, err := qs.ListEvidenceAsc(ctx, sqlcgen.ListEvidenceAscParams{
		Column1: aid, Column2: owner, Kind: kind, IntegrityState: state, JobID: job, AfterAt: at, AfterID: after, MaxRows: rows,
	})
	if err != nil {
		return nil, wrap("list evidence", err)
	}
	for _, row := range list {
		rec, err := toRecord(row.MorningstarEvidenceItem, row.MorningstarEvidenceStatus, row.ProgramArchived, row.LastCheckedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// RecordCheck needs a transaction: the row lock on the status is what keeps two
// checks from overwriting each other's verdict.
func (r Evidence) RecordCheck(ctx context.Context, c evidence.CheckRecord) (evidence.CheckRecord, evidence.Status, error) {
	if !inTx(ctx) {
		return evidence.CheckRecord{}, evidence.Status{}, errors.New("persistence: an integrity check is recorded inside a transaction")
	}
	id, err := uuidOf(c.ID)
	if err != nil {
		return evidence.CheckRecord{}, evidence.Status{}, err
	}
	eid, err := uuidOf(c.EvidenceID)
	if err != nil {
		return evidence.CheckRecord{}, evidence.Status{}, err
	}
	user, err := uuidOrNull(c.Actor.UserID)
	if err != nil {
		return evidence.CheckRecord{}, evidence.Status{}, err
	}
	request, err := uuidOrNull(c.RequestID)
	if err != nil {
		return evidence.CheckRecord{}, evidence.Status{}, err
	}
	q := r.q(ctx)
	cur, err := q.LockEvidenceStatus(ctx, eid)
	if noRows(err) {
		return evidence.CheckRecord{}, evidence.Status{}, outbound.ErrEvidenceNotFound
	}
	if err != nil {
		return evidence.CheckRecord{}, evidence.Status{}, wrap("lock the status of evidence", err)
	}
	if evidence.Retention(cur.RetentionState) == evidence.RetentionPurged {
		return evidence.CheckRecord{}, evidence.Status{}, outbound.ErrEvidencePurged
	}
	next := evidence.NextState(evidence.IntegrityState(cur.IntegrityState), c.Result)

	var sum []byte
	if c.ObservedSHA256 != nil {
		sum = c.ObservedSHA256.Bytes()
	}
	at, err := q.InsertEvidenceCheck(ctx, sqlcgen.InsertEvidenceCheckParams{
		Column1: id, Column2: eid, Trigger: string(c.Trigger), Result: string(c.Result), ActorKind: string(c.Actor.Kind),
		ObservedSha256: sum, ObservedSize: c.ObservedSize, ActorUserID: user, RequestID: request,
	})
	if err != nil {
		return evidence.CheckRecord{}, evidence.Status{}, wrap("insert an integrity check", err)
	}
	st, err := q.UpdateEvidenceIntegrity(ctx, sqlcgen.UpdateEvidenceIntegrityParams{State: string(next), CheckID: id, EvidenceID: eid})
	if err != nil {
		return evidence.CheckRecord{}, evidence.Status{}, wrap("update the integrity state", err)
	}
	c.CheckedAt = pgTime(at)
	checkedAt := c.CheckedAt
	return c, evidence.Status{
		Integrity: evidence.IntegrityState(st.IntegrityState), Retention: evidence.Retention(st.RetentionState),
		LastCheckedAt: &checkedAt, CompromisedAt: pgTimePtr(st.CompromisedAt), PurgedAt: pgTimePtr(st.PurgedAt),
	}, nil
}

func (r Evidence) ListChecks(ctx context.Context, ownerID, evidenceID string, q outbound.EvidencePageQuery) ([]evidence.CheckRecord, error) {
	eid, err := uuidOf(evidenceID)
	if err != nil {
		return nil, outbound.ErrEvidenceNotFound
	}
	owner, err := uuidOf(ownerID)
	if err != nil {
		return nil, outbound.ErrEvidenceNotFound
	}
	rows, err := limitRows(q.Limit)
	if err != nil {
		return nil, err
	}
	var at pgtype.Timestamptz
	var after pgtype.UUID
	if q.After != nil {
		if after, err = uuidOf(q.After.ID); err != nil {
			return nil, err
		}
		at = pgtype.Timestamptz{Time: q.After.At, Valid: true}
	}
	qs := r.q(ctx)
	owned, err := qs.EvidenceOwned(ctx, sqlcgen.EvidenceOwnedParams{Column1: eid, Column2: owner})
	if err != nil {
		return nil, wrap("check the owner of evidence", err)
	}
	if !owned {
		return nil, outbound.ErrEvidenceNotFound
	}
	var out []evidence.CheckRecord
	if q.Descending {
		list, err := qs.ListEvidenceChecksDesc(ctx, sqlcgen.ListEvidenceChecksDescParams{Column1: eid, Column2: owner, BeforeAt: at, BeforeID: after, MaxRows: rows})
		if err != nil {
			return nil, wrap("list integrity checks", err)
		}
		for _, row := range list {
			out = append(out, toCheck(row.ID, row.EvidenceID, row.CheckedAt, row.Trigger, row.Result, row.ObservedSha256, row.ObservedSize, row.ActorKind, row.ActorUserID, row.RequestID))
		}
		return out, nil
	}
	list, err := qs.ListEvidenceChecksAsc(ctx, sqlcgen.ListEvidenceChecksAscParams{Column1: eid, Column2: owner, AfterAt: at, AfterID: after, MaxRows: rows})
	if err != nil {
		return nil, wrap("list integrity checks", err)
	}
	for _, row := range list {
		out = append(out, toCheck(row.ID, row.EvidenceID, row.CheckedAt, row.Trigger, row.Result, row.ObservedSha256, row.ObservedSize, row.ActorKind, row.ActorUserID, row.RequestID))
	}
	return out, nil
}

func toCheck(id, evidenceID string, at pgtype.Timestamptz, trigger, result string, sum []byte, size *int64, kind, user, request string) evidence.CheckRecord {
	c := evidence.CheckRecord{
		ID: id, EvidenceID: evidenceID, CheckedAt: pgTime(at), Trigger: evidence.CheckTrigger(trigger), Result: evidence.CheckResult(result),
		ObservedSize: size, Actor: evidence.CheckActor{Kind: evidence.ActorKind(kind), UserID: user}, RequestID: request,
	}
	if len(sum) == 32 {
		d := evidence.DigestFromBytes(sum)
		c.ObservedSHA256 = &d
	}
	return c
}
