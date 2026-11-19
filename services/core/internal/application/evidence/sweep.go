package evidence

import (
	"context"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

type (
	SweepPolicy = inbound.SweepPolicy
	SweepResult = inbound.SweepResult
)

// Sweep looks after the evidence directory and the evidence rows (TD-M6-01).
//
// A file without a row that is older than the floor is an orphan: the capture
// that wrote it did not commit. A temporary file of that age is the leftover of
// an interrupted write. Both are removed. An entry that is neither a published
// name nor a temporary name is never removed and never opened: it is counted,
// because nothing may be deleted on a guess. A published file with a row is
// never removed here.
//
// The integrity pass checks the items that were not checked for the period and
// marks a failure like any other check, with the trigger sweep and the system as
// the actor.
func (s *Service) Sweep(ctx context.Context, p SweepPolicy) (SweepResult, error) {
	var res SweepResult
	files, err := s.Store.List(ctx)
	if err != nil {
		return res, err
	}
	var keys []string
	for _, f := range files {
		if f.IsKey && !f.Special {
			keys = append(keys, f.Name)
		}
	}
	known, err := s.Repo.KnownKeys(ctx, keys)
	if err != nil {
		return res, mapError(err)
	}
	cutoff := s.Now().Add(-p.OrphanAge)
	for _, f := range files {
		switch {
		case f.Special || (!f.IsKey && !f.Temporary):
			res.Unrecognized++
		case f.Temporary:
			if f.ModTime.Before(cutoff) && s.Store.RemoveTemporary(ctx, f.Name) == nil {
				res.TemporaryRemoved++
			}
		case !known[f.Name] && f.ModTime.Before(cutoff):
			if s.Store.Remove(ctx, f.Name) == nil {
				res.OrphansRemoved++
			}
		}
	}

	due, err := s.Repo.DueForCheck(ctx, s.Now().Add(-p.RecheckAfter), max(p.Batch, 1))
	if err != nil {
		return res, mapError(err)
	}
	for _, rec := range due {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		c, err := s.verify(ctx, rec.Item, evidence.TriggerSweep, audit.System(), evidence.CheckActor{Kind: evidence.ActorSystem}, "")
		if err != nil {
			s.Logger.Warn("evidence: a sweep check did not complete", "evidenceId", rec.Item.ID())
			continue
		}
		res.Checked++
		if c.Result != evidence.ResultMatch {
			res.Failed++
		}
	}
	return res, nil
}
