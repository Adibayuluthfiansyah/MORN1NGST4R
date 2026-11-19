// Package evidence is the evidence use case (FR-EVIDENCE-001 to 003, D-77,
// P-37). It stores artifacts as redacted bytes with a SHA-256, and it checks the
// bytes against that hash every time they are read. A file that no longer
// matches, or that is not a plain owner-only file, is withheld and the item is
// marked compromised until a person decides about it.
//
// Nothing here executes a tool or decides that a finding is verified. The use
// case offers the integrity state to the modules that do (SupportsVerification).
package evidence

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

type Deps struct {
	Store outbound.EvidenceStore
	Repo  outbound.EvidenceRepository
	Audit outbound.AuditSink
	Tx    outbound.Transactor
	// Alerts raises the alert of a failed integrity check (domain-model.md 4.4).
	// Without it the finding is still stored and audited.
	Alerts outbound.SystemAlerts
	// Retention, RetentionTx and RetentionAudit run the purge with the retention
	// role. Without them Purge is unavailable.
	Retention      outbound.EvidenceRetention
	RetentionTx    outbound.Transactor
	RetentionAudit outbound.AuditSink

	NewID  func() string
	Now    func() time.Time
	Logger *slog.Logger
}

type Service struct{ Deps }

var (
	_ inbound.Evidence        = (*Service)(nil)
	_ inbound.EvidenceCapture = (*Service)(nil)
	_ inbound.EvidencePurge   = (*Service)(nil)
)

func New(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{Deps: d}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isUUID(s string) bool { return uuidPattern.MatchString(s) }

// mapError turns an error of a port into the error of the use case.
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, outbound.ErrStoreUnavailable):
		return inbound.ErrDatabaseUnavailable
	case errors.Is(err, outbound.ErrEvidenceNotFound):
		return inbound.ErrNotFound
	case errors.Is(err, outbound.ErrEvidencePurged):
		return inbound.ErrEvidencePurged
	}
	return err
}

func invalidField(field, code string) error {
	return &inbound.ValidationError{Fields: []inbound.FieldError{{Field: field, Code: code}}}
}

// ownerOf is the user whose evidence the request may reach.
func ownerOf(p inbound.Principal) (string, error) {
	if p == nil || !isUUID(p.UserID()) {
		return "", inbound.ErrUnauthenticated
	}
	return p.UserID(), nil
}

func userActor(p inbound.Principal) (audit.Actor, error) { return audit.FromActor(p.Actor()) }

// subject names the evidence in an audit event.
func subject(it evidence.Item) audit.Subject {
	return audit.Subject{ProgramID: it.ProgramID(), AssessmentID: it.AssessmentID(), JobID: it.JobID(), EntityType: "evidence", EntityID: it.ID()}
}

func (s *Service) event(who audit.Actor, action string, outcome audit.Outcome, reason, requestID string, it evidence.Item, detail map[string]any) (audit.Event, error) {
	return audit.New(s.NewID(), audit.Input{
		Actor: who, Action: action, Outcome: outcome, Reason: reason, RequestID: requestID, Subject: subject(it), Detail: detail,
	})
}

// require writes the record that the operation needs before it may take effect.
func (s *Service) require(ctx context.Context, who audit.Actor, action string, requestID string, it evidence.Item, detail map[string]any) error {
	return s.requireWith(ctx, s.Audit, who, action, requestID, it, detail)
}

func (s *Service) requireWith(ctx context.Context, sink outbound.AuditSink, who audit.Actor, action string, requestID string, it evidence.Item, detail map[string]any) error {
	e, err := s.event(who, action, audit.Success, "", requestID, it, detail)
	if err != nil {
		return err
	}
	return sink.Require(ctx, e)
}

// record writes the record of something that already happened or was refused.
// Whatever the result, the decision it records stands.
func (s *Service) record(ctx context.Context, who audit.Actor, action string, outcome audit.Outcome, reason, requestID string, it evidence.Item, detail map[string]any) {
	e, err := s.event(who, action, outcome, reason, requestID, it, detail)
	if err == nil {
		err = s.Audit.Record(ctx, e)
	}
	if err != nil {
		s.Logger.Error("evidence: an audit event was not recorded", "action", action, "outcome", string(outcome))
	}
}
