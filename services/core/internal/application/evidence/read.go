package evidence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// maxCursor bounds the text that is decoded: a cursor is about 100 characters.
const maxCursor = 256

type cursorWire struct {
	T string `json:"t"`
	I string `json:"i"`
}

func encodeCursor(at time.Time, id string) string {
	b, _ := json.Marshal(cursorWire{T: at.UTC().Format(time.RFC3339Nano), I: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (*outbound.EvidenceCursor, error) {
	if s == "" {
		return nil, nil
	}
	bad := invalidField("cursor", "invalid")
	if len(s) > maxCursor {
		return nil, bad
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, bad
	}
	var w cursorWire
	if err := json.Unmarshal(raw, &w); err != nil || !isUUID(w.I) {
		return nil, bad
	}
	at, err := time.Parse(time.RFC3339Nano, w.T)
	if err != nil {
		return nil, bad
	}
	return &outbound.EvidenceCursor{At: at, ID: w.I}, nil
}

// pageQuery validates the paging input. The repository is asked for one row more
// than the page, to learn whether another page exists.
func pageQuery(in inbound.PageInput) (outbound.EvidencePageQuery, int, error) {
	limit := in.Limit
	switch {
	case limit == 0:
		limit = defaultLimit
	case limit < 1 || limit > maxLimit:
		return outbound.EvidencePageQuery{}, 0, invalidField("limit", "out_of_range")
	}
	after, err := decodeCursor(in.Cursor)
	if err != nil {
		return outbound.EvidencePageQuery{}, 0, err
	}
	return outbound.EvidencePageQuery{Limit: limit + 1, After: after, Descending: in.Descending}, limit, nil
}

func pageInfo(limit, got int, last func() string) inbound.PageInfo {
	p := inbound.PageInfo{Limit: limit}
	if got > limit {
		p.HasMore = true
		p.NextCursor = last()
	}
	return p
}

func validKind(k string) bool {
	switch evidence.Kind(k) {
	case evidence.KindRequest, evidence.KindResponse, evidence.KindRequestResponse, evidence.KindToolOutput, evidence.KindNote, evidence.KindReport:
		return true
	}
	return false
}

func validState(s string) bool {
	switch evidence.IntegrityState(s) {
	case evidence.Unchecked, evidence.IntegrityOK, evidence.Compromised, evidence.Missing:
		return true
	}
	return false
}

func (s *Service) List(ctx context.Context, p inbound.Principal, in inbound.EvidenceListInput) (inbound.EvidencePage, error) {
	owner, err := ownerOf(p)
	if err != nil {
		return inbound.EvidencePage{}, err
	}
	if !isUUID(in.AssessmentID) {
		return inbound.EvidencePage{}, inbound.ErrNotFound
	}
	var bad []inbound.FieldError
	if in.Kind != "" && !validKind(in.Kind) {
		bad = append(bad, inbound.FieldError{Field: "kind", Code: "invalid"})
	}
	if in.IntegrityState != "" && !validState(in.IntegrityState) {
		bad = append(bad, inbound.FieldError{Field: "integrityState", Code: "invalid"})
	}
	if in.JobID != "" && !isUUID(in.JobID) {
		bad = append(bad, inbound.FieldError{Field: "jobId", Code: "invalid"})
	}
	q, limit, perr := pageQuery(in.Page)
	if perr != nil {
		if ve, ok := perr.(*inbound.ValidationError); ok {
			bad = append(bad, ve.Fields...)
		}
	}
	if len(bad) > 0 {
		return inbound.EvidencePage{}, &inbound.ValidationError{Fields: bad}
	}
	recs, err := s.Repo.List(ctx, owner, in.AssessmentID, outbound.EvidenceFilter{
		Kind: evidence.Kind(in.Kind), Integrity: evidence.IntegrityState(in.IntegrityState), JobID: in.JobID,
	}, q)
	if err != nil {
		return inbound.EvidencePage{}, mapError(err)
	}
	info := pageInfo(limit, len(recs), func() string {
		it := recs[limit-1].Item
		return encodeCursor(it.CapturedAt(), it.ID())
	})
	if len(recs) > limit {
		recs = recs[:limit]
	}
	return inbound.EvidencePage{Items: recs, Page: info}, nil
}

func (s *Service) Get(ctx context.Context, p inbound.Principal, id string) (evidence.Record, error) {
	owner, err := ownerOf(p)
	if err != nil {
		return evidence.Record{}, err
	}
	if !isUUID(id) {
		return evidence.Record{}, inbound.ErrNotFound
	}
	rec, err := s.Repo.Get(ctx, owner, id)
	return rec, mapError(err)
}

func (s *Service) Checks(ctx context.Context, p inbound.Principal, id string, page inbound.PageInput) (inbound.EvidenceChecksPage, error) {
	owner, err := ownerOf(p)
	if err != nil {
		return inbound.EvidenceChecksPage{}, err
	}
	if !isUUID(id) {
		return inbound.EvidenceChecksPage{}, inbound.ErrNotFound
	}
	q, limit, err := pageQuery(page)
	if err != nil {
		return inbound.EvidenceChecksPage{}, err
	}
	checks, err := s.Repo.ListChecks(ctx, owner, id, q)
	if err != nil {
		return inbound.EvidenceChecksPage{}, mapError(err)
	}
	info := pageInfo(limit, len(checks), func() string {
		c := checks[limit-1]
		return encodeCursor(c.CheckedAt, c.ID)
	})
	if len(checks) > limit {
		checks = checks[:limit]
	}
	return inbound.EvidenceChecksPage{Items: checks, Page: info}, nil
}
