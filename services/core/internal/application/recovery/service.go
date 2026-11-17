package recovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

var (
	// ErrNotRequired is returned by Reconcile when the installation does not
	// need a reconciliation.
	ErrNotRequired = errors.New("recovery is not required")
	// ErrNotNormal is returned when an operation needs a normal installation.
	ErrNotNormal = errors.New("the installation is not in the normal recovery state")
	// ErrMarkerWrite is returned when the recovery marker could not be written.
	// While it holds, claims are refused (section 4).
	ErrMarkerWrite = errors.New("the recovery marker could not be written")
)

// Deps are the collaborators of a Service.
type Deps struct {
	Markers outbound.RecoveryMarkerStore
	Repo    outbound.RecoveryRepository
	// NewID returns a new UUID for an installation.
	NewID  func() string
	Now    func() time.Time
	Logger *slog.Logger
}

// Status is the recovery state of the installation as the last check saw it.
type Status struct {
	// Required is true when the reconciliation procedure must run. Every claim
	// and every authenticated route except the session and the reconcile route
	// must then be refused.
	Required bool
	Reason   Reason
	// MarkerGeneration and DatabaseGeneration are 0 when that half is absent.
	MarkerGeneration   uint64
	DatabaseGeneration uint64
	DetectedAt         time.Time
}

// Service runs the startup decision and the reconciliation.
type Service struct {
	markers outbound.RecoveryMarkerStore
	repo    outbound.RecoveryRepository
	newID   func() string
	now     func() time.Time
	logger  *slog.Logger

	mu          sync.Mutex
	normal      bool
	instanceID  string
	generation  uint64
	markerFault bool
	status      Status
}

// NewService returns a Service. Every dependency except Logger is required.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	return &Service{markers: d.Markers, repo: d.Repo, newID: d.NewID, now: d.Now, logger: d.Logger}
}

// Check runs the startup decision (section 5). It returns Required status for
// every result that is not normal. It returns an error when the stores fail,
// and the caller must not serve in that case.
func (s *Service) Check(ctx context.Context) (Status, error) {
	// Leftover temporary files are harmless and removed. A failure to remove one
	// does not stop the start.
	if err := s.markers.Cleanup(ctx); err != nil {
		s.logger.Warn("recovery marker cleanup failed")
	}
	in, err := s.inputs(ctx)
	if err != nil {
		return Status{}, err
	}
	d := Decide(in)
	switch d.Action {
	case ActionFirstRun:
		return s.firstRun(ctx)
	case ActionResumePhase2:
		row := in.Facts.Row
		if err := s.writeMarker(ctx, row.InstanceID, row.Generation, true); err != nil {
			return Status{}, err
		}
		fallthrough
	case ActionResumePhase3:
		row := in.Facts.Row
		if err := s.repo.CompleteReconcile(ctx, row.Generation, string(d.Reason)); err != nil {
			return Status{}, fmt.Errorf("recovery: complete the interrupted reconciliation: %w", err)
		}
		s.logger.Warn("recovery: an interrupted reconciliation was completed", "generation", row.Generation)
		return s.setNormal(row.InstanceID, row.Generation), nil
	case ActionNormal:
		return s.setNormal(in.Facts.Row.InstanceID, in.Facts.Row.Generation), nil
	}
	return s.setRequired(in, d.Reason), nil
}

func (s *Service) inputs(ctx context.Context) (Input, error) {
	var in Input
	m, err := s.markers.Read(ctx)
	switch {
	case err == nil:
		in.Marker = &m
	case errors.Is(err, outbound.ErrMarkerMissing):
	case errors.Is(err, outbound.ErrMarkerInvalid):
		in.MarkerInvalid = true
	default:
		return Input{}, fmt.Errorf("recovery: read the marker: %w", err)
	}
	in.Facts, err = s.repo.Facts(ctx)
	if err != nil {
		return Input{}, fmt.Errorf("recovery: read the database state: %w", err)
	}
	return in, nil
}

func (s *Service) firstRun(ctx context.Context) (Status, error) {
	id := s.newID()
	err := s.repo.InitializeFirstRun(ctx, id, func(ctx context.Context, maxSeq uint64) error {
		return s.markers.Replace(ctx, outbound.RecoveryMarker{InstanceID: id, Generation: 1, AuditWatermark: maxSeq, WrittenAt: s.now()})
	})
	if err != nil {
		return Status{}, fmt.Errorf("recovery: first run: %w", err)
	}
	s.logger.Info("recovery: first run initialized", "generation", 1)
	return s.setNormal(id, 1), nil
}

// writeMarker writes the marker with the current highest audit sequence. A
// reconciliation creates or repairs the marker (replace). The watermark advance
// during normal operation only updates a marker that exists.
func (s *Service) writeMarker(ctx context.Context, instanceID string, generation uint64, replace bool) error {
	seq, err := s.repo.MaxAuditSeq(ctx)
	if err != nil {
		return fmt.Errorf("recovery: read the audit sequence: %w", err)
	}
	m := outbound.RecoveryMarker{InstanceID: instanceID, Generation: generation, AuditWatermark: seq, WrittenAt: s.now()}
	if replace {
		err = s.markers.Replace(ctx, m)
	} else {
		err = s.markers.Advance(ctx, m)
	}
	if err != nil {
		return fmt.Errorf("recovery: write the marker: %w", err)
	}
	return nil
}

func (s *Service) setNormal(instanceID string, generation uint64) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normal, s.instanceID, s.generation, s.markerFault = true, instanceID, generation, false
	s.status = Status{}
	return s.status
}

func (s *Service) setRequired(in Input, reason Reason) Status {
	st := Status{Required: true, Reason: reason, DetectedAt: s.now()}
	if in.Marker != nil {
		st.MarkerGeneration = in.Marker.Generation
	}
	if in.Facts.Row != nil {
		st.DatabaseGeneration = in.Facts.Row.Generation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normal, s.status = false, st
	s.logger.Error("recovery: the installation needs reconciliation", "reason", string(reason),
		"marker_generation", st.MarkerGeneration, "database_generation", st.DatabaseGeneration)
	return st
}

// Status returns the result of the last check or reconciliation.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Generation returns the recovery generation under which claims may be made. It
// reports false while the installation is not in the normal state.
func (s *Service) Generation() (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation, s.normal
}

// Reconcile runs the reconciliation (section 6) for the human user actorUserID
// who confirmed it. The caller has checked that user's password. Phase 1
// invalidates the authority recorded in the database, phase 2 writes the marker
// and phase 3 returns the state to normal. Any failure leaves the installation
// in recovery_required, or in a state that the next start resumes (section 7).
func (s *Service) Reconcile(ctx context.Context, actorUserID, requestID string) (Status, error) {
	if !validUUID(actorUserID) || !validUUID(requestID) {
		return Status{}, errors.New("recovery: the actor and the request must be UUIDs")
	}
	in, err := s.inputs(ctx)
	if err != nil {
		return Status{}, err
	}
	d := Decide(in)
	if d.Action != ActionRequired {
		return Status{}, ErrNotRequired
	}

	var markerGen, dbGen uint64
	if in.Marker != nil {
		markerGen = in.Marker.Generation
	}
	if in.Facts.Row != nil {
		dbGen = in.Facts.Row.Generation
	}
	newGen := max(markerGen, dbGen, 1) + 1
	instance := s.newID()
	switch {
	case in.Facts.Row != nil:
		instance = in.Facts.Row.InstanceID
	case in.Marker != nil:
		instance = in.Marker.InstanceID
	}

	counts, err := s.repo.BeginReconcile(ctx, outbound.ReconcileInput{
		ExpectedGeneration: dbGen, InstanceID: instance, NewGeneration: newGen, Reason: string(d.Reason),
		OldMarkerGeneration: markerGen, ActorUserID: actorUserID, RequestID: requestID,
	})
	if err != nil {
		return Status{}, fmt.Errorf("recovery: phase 1: %w", err)
	}
	s.logger.Warn("recovery: phase 1 committed", "reason", string(d.Reason), "new_generation", newGen, "counts", counts)

	if err := s.writeMarker(ctx, instance, newGen, true); err != nil {
		return Status{}, fmt.Errorf("phase 2: %w", err)
	}
	if err := s.repo.CompleteReconcile(ctx, newGen, string(d.Reason)); err != nil {
		return Status{}, fmt.Errorf("recovery: phase 3: %w", err)
	}
	return s.setNormal(instance, newGen), nil
}

// AdvanceWatermark writes the marker with the current highest audit sequence.
// The worker calls it after the database commit of a claim and before it
// launches the process, and after other events that must not be replayed
// (section 4). When the write fails it returns ErrMarkerWrite and ClaimsAllowed
// refuses until a write succeeds.
func (s *Service) AdvanceWatermark(ctx context.Context) error {
	s.mu.Lock()
	normal, instance, generation := s.normal, s.instanceID, s.generation
	s.mu.Unlock()
	if !normal {
		return ErrNotNormal
	}
	err := s.writeMarker(ctx, instance, generation, false)
	s.mu.Lock()
	s.markerFault = err != nil
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMarkerWrite, err)
	}
	return nil
}

// ClaimsAllowed returns nil when new claims may be made: the installation is
// normal and the last marker write succeeded.
func (s *Service) ClaimsAllowed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !s.normal:
		return ErrNotNormal
	case s.markerFault:
		return ErrMarkerWrite
	}
	return nil
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
