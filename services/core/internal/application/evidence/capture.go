package evidence

import (
	"context"
	"errors"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// removeTimeout bounds the removal of an orphan file after the caller gave up.
const removeTimeout = 5 * time.Second

// Capture stores one artifact (P-37). The order is the protocol: the bytes are
// redacted, the file is published without replacing anything, and the metadata
// and its audit record commit together. If the commit fails the file is
// removed, so a failed capture leaves no file that no row describes. If the
// removal fails too, the orphan is harmless: nothing points at it, and its name
// is an id that is never used again.
func (s *Service) Capture(ctx context.Context, in inbound.CaptureInput) (evidence.Item, error) {
	if in.Actor.Kind() == "" {
		return evidence.Item{}, invalidField("actor", "required")
	}
	if !isUUID(in.AssessmentID) {
		return evidence.Item{}, inbound.ErrNotFound
	}
	a, err := s.Repo.Assessment(ctx, in.AssessmentID)
	if err != nil {
		return evidence.Item{}, mapError(err)
	}
	if a.ProgramArchived {
		return evidence.Item{}, inbound.ErrProgramArchived
	}
	data, redaction, err := evidence.Prepare(in.Content, evidence.PrepareOptions{Raw: in.Raw, RawAllowed: a.RawCapture})
	if errors.Is(err, evidence.ErrRawNotAllowed) {
		return evidence.Item{}, inbound.ErrRawCaptureNotAllowed
	}
	if err != nil {
		return evidence.Item{}, err
	}
	if int64(len(data)) > s.Store.MaxBytes() {
		return evidence.Item{}, invalidField("content", "too_large")
	}
	sensitivity := in.Sensitivity
	if redaction == evidence.RawRetained {
		// Unredacted bytes are sensitive whatever the caller says.
		sensitivity = evidence.Sensitive
	}
	it, err := evidence.New(evidence.NewItem{
		ID: s.NewID(), ProgramID: a.ProgramID, AssessmentID: in.AssessmentID, JobID: in.JobID, AttemptID: in.AttemptID,
		TargetID: in.TargetID, DecisionID: in.DecisionID, Kind: in.Kind, ContentType: in.ContentType, Origin: in.Origin,
		Redaction: redaction, Sensitivity: sensitivity, SHA256: evidence.DigestOf(data), Size: int64(len(data)),
		CapturedAt: s.Now().UTC().Truncate(time.Microsecond),
		ToolKey:    in.ToolKey, ToolVersion: in.ToolVersion, ToolBinarySHA256: in.ToolBinarySHA256, OperationHash: in.OperationHash,
		RedactedArgs: in.RedactedArgs,
	})
	if err != nil {
		if f, ok := evidence.InvalidField(err); ok {
			return evidence.Item{}, invalidField(f, "invalid")
		}
		return evidence.Item{}, err
	}

	w, err := s.Store.Put(ctx, it.StorageKey(), data)
	if errors.Is(err, outbound.ErrEvidenceTooLarge) {
		return evidence.Item{}, invalidField("content", "too_large")
	}
	if err != nil {
		// A store that fails after it published (the sync of the directory) leaves a
		// complete file that no row describes. The key is new, so the file is ours,
		// unless the store said that the name was taken: that file is not ours.
		if !errors.Is(err, outbound.ErrEvidenceExists) {
			s.removeOrphan(it)
		}
		return evidence.Item{}, err
	}
	if w.Size != it.Size() || !w.Digest.Equal(it.SHA256()) {
		s.removeOrphan(it)
		return evidence.Item{}, errors.New("evidence: the store wrote other bytes than it was given")
	}
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Repo.Insert(ctx, it); err != nil {
			return err
		}
		return s.require(ctx, in.Actor, "evidence.captured", in.RequestID, it, map[string]any{
			"kind": string(it.Kind()), "origin": string(it.Origin()), "redaction": string(it.Redaction()),
			"sha256": it.SHA256().String(), "sizeBytes": it.Size(),
		})
	})
	if err != nil {
		s.removeOrphan(it)
		return evidence.Item{}, mapError(err)
	}
	return it, nil
}

func (s *Service) removeOrphan(it evidence.Item) {
	ctx, cancel := context.WithTimeout(context.Background(), removeTimeout)
	defer cancel()
	if err := s.Store.Remove(ctx, it.StorageKey()); err != nil {
		s.Logger.Error("evidence: the file of a failed capture was not removed", "evidenceId", it.ID())
	}
}
