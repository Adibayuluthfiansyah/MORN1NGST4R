package evidence

import (
	"context"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

// lookup resolves the item for a request that may reach its content: the item of
// the owner, whose content has not been purged.
func (s *Service) lookup(ctx context.Context, p inbound.Principal, id string) (owner string, who audit.Actor, rec evidence.Record, err error) {
	if owner, err = ownerOf(p); err != nil {
		return
	}
	if !isUUID(id) {
		err = inbound.ErrNotFound
		return
	}
	if who, err = userActor(p); err != nil {
		return
	}
	rec, err = s.Repo.Get(ctx, owner, id)
	err = mapError(err)
	return
}

func checkDetail(c evidence.CheckRecord, it evidence.Item) map[string]any {
	d := map[string]any{"trigger": string(c.Trigger), "result": string(c.Result), "expected": it.SHA256().String()}
	if c.ObservedSHA256 != nil {
		d["observed"] = c.ObservedSHA256.String()
	}
	return d
}

// recordFailure writes what a failed check found. The finding is already
// stored, and it stands whether or not these records and the alert are written.
func (s *Service) recordFailure(ctx context.Context, who audit.Actor, requestID string, it evidence.Item, c evidence.CheckRecord) {
	s.record(ctx, who, "evidence.integrity_failed", audit.Failure, string(c.Result), requestID, it, checkDetail(c, it))
	if s.Alerts == nil {
		return
	}
	if err := s.Alerts.Raise(ctx, "evidence_integrity", map[string]any{"evidenceId": it.ID(), "result": string(c.Result), "trigger": string(c.Trigger)}); err != nil {
		s.Logger.Error("evidence: an integrity alert was not raised", "evidenceId", it.ID())
	}
}

// recordCheck commits a check on its own.
func (s *Service) recordCheck(ctx context.Context, c evidence.CheckRecord) (stored evidence.CheckRecord, err error) {
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		stored, _, err = s.Repo.RecordCheck(ctx, c)
		return err
	})
	return stored, err
}

// verify hashes the stored content and records the result. A match is recorded
// together with its audit event, so that no check exists without one. A failed
// check is committed first and audited after, because a person who cannot be
// told must not be able to lose the finding by breaking audit.
func (s *Service) verify(ctx context.Context, it evidence.Item, trigger evidence.CheckTrigger, who audit.Actor, by evidence.CheckActor, requestID string) (evidence.CheckRecord, error) {
	obs, err := s.Store.Observe(ctx, it.StorageKey(), it.Size())
	if err != nil {
		return evidence.CheckRecord{}, err
	}
	c, err := evidence.NewCheck(s.NewID(), it, obs, trigger, by)
	if err != nil {
		return evidence.CheckRecord{}, err
	}
	c.RequestID = requestID

	if c.Result == evidence.ResultMatch {
		var stored evidence.CheckRecord
		err = s.Tx.InTx(ctx, func(ctx context.Context) error {
			var err error
			if stored, _, err = s.Repo.RecordCheck(ctx, c); err != nil {
				return err
			}
			return s.require(ctx, who, "evidence.integrity_checked", requestID, it, checkDetail(c, it))
		})
		return stored, mapError(err)
	}
	stored, err := s.recordCheck(ctx, c)
	if err != nil {
		return evidence.CheckRecord{}, mapError(err)
	}
	s.record(ctx, who, "evidence.integrity_checked", audit.Success, "", requestID, it, checkDetail(stored, it))
	s.recordFailure(ctx, who, requestID, it, stored)
	return stored, nil
}

// Check re-hashes the stored content and records the result (EVD-03).
func (s *Service) Check(ctx context.Context, p inbound.Principal, requestID, id string) (evidence.CheckRecord, error) {
	owner, who, rec, err := s.lookup(ctx, p, id)
	if err != nil {
		return evidence.CheckRecord{}, err
	}
	if rec.ProgramArchived {
		return evidence.CheckRecord{}, inbound.ErrProgramArchived
	}
	if rec.Status.Retention == evidence.RetentionPurged {
		return evidence.CheckRecord{}, inbound.ErrEvidencePurged
	}
	return s.verify(ctx, rec.Item, evidence.TriggerManual, who, evidence.CheckActor{Kind: evidence.ActorUser, UserID: owner}, requestID)
}

// Open returns the content of an item after a fresh check (EVD-05). The file is
// read into memory and hashed in one pass, and the caller gets those bytes and
// no others, so the bytes served are the bytes that were checked. No byte is
// returned before the access is recorded: a match commits its check and the
// access event together, and an acknowledged read of a failed item records the
// access first.
func (s *Service) Open(ctx context.Context, p inbound.Principal, requestID, id string, acknowledge bool) (inbound.EvidenceContent, error) {
	_, who, rec, err := s.lookup(ctx, p, id)
	if err != nil {
		return inbound.EvidenceContent{}, err
	}
	it := rec.Item
	denied := func(reason string, e error) (inbound.EvidenceContent, error) {
		s.record(ctx, who, "evidence.accessed", audit.Denied, reason, requestID, it, map[string]any{"integrityState": string(rec.Status.Integrity), "acknowledged": acknowledge})
		return inbound.EvidenceContent{}, e
	}
	if rec.Status.Retention == evidence.RetentionPurged {
		return denied("evidence_purged", inbound.ErrEvidencePurged)
	}
	// An acknowledgement counts only for a failure that was already recorded when the
	// request began. A person who sent it in advance has not seen a failure that this
	// read finds first.
	blocked := rec.Status.Integrity == evidence.Compromised || rec.Status.Integrity == evidence.Missing
	if blocked && !acknowledge {
		return denied("integrity_failed", inbound.ErrEvidenceIntegrityFailed)
	}
	obs, data, err := s.Store.Read(ctx, it.StorageKey(), it.Size())
	if err != nil {
		return inbound.EvidenceContent{}, err
	}
	c, err := evidence.NewCheck(s.NewID(), it, obs, evidence.TriggerRead, evidence.CheckActor{Kind: evidence.ActorUser, UserID: p.UserID()})
	if err != nil {
		return inbound.EvidenceContent{}, err
	}
	c.RequestID = requestID
	accessed := map[string]any{"acknowledged": acknowledge, "result": string(c.Result), "integrityState": string(rec.Status.Integrity)}

	if c.Result == evidence.ResultMatch {
		err = s.Tx.InTx(ctx, func(ctx context.Context) error {
			if _, _, err := s.Repo.RecordCheck(ctx, c); err != nil {
				return err
			}
			return s.require(ctx, who, "evidence.accessed", requestID, it, accessed)
		})
		if err != nil {
			return inbound.EvidenceContent{}, mapError(err)
		}
		return inbound.EvidenceContent{ID: it.ID(), Data: data}, nil
	}

	stored, err := s.recordCheck(ctx, c)
	if err != nil {
		return inbound.EvidenceContent{}, mapError(err)
	}
	s.recordFailure(ctx, who, requestID, it, stored)
	if !(acknowledge && blocked) || data == nil {
		return denied("integrity_failed", inbound.ErrEvidenceIntegrityFailed)
	}
	if err := s.require(ctx, who, "evidence.accessed", requestID, it, accessed); err != nil {
		return inbound.EvidenceContent{}, err
	}
	return inbound.EvidenceContent{ID: it.ID(), Data: data}, nil
}
