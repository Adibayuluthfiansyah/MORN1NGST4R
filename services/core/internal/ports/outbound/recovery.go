package outbound

import (
	"context"
	"errors"
	"time"
)

// RecoveryMarker is the content of the external recovery marker
// (docs/design/database/recovery-generation.md section 3). It lives outside
// PostgreSQL, so a restore of the database does not change it.
type RecoveryMarker struct {
	InstanceID string
	Generation uint64
	// AuditWatermark is the highest audit_events.seq that the marker has seen.
	AuditWatermark uint64
	WrittenAt      time.Time
}

var (
	// ErrMarkerMissing: there is no marker file.
	ErrMarkerMissing = errors.New("recovery marker is missing")
	// ErrMarkerInvalid: the marker is unreadable, unparseable, a symlink, not
	// owned by the running user, wider than mode 0600, or fails its check.
	ErrMarkerInvalid = errors.New("recovery marker is invalid")
	// ErrMarkerStale: a write would lower the generation of the marker on disk,
	// or would replace it with another installation's marker.
	ErrMarkerStale = errors.New("recovery marker write refused: it is older than the marker on disk")
)

// RecoveryMarkerStore reads and writes the marker file. A write is atomic (a
// temporary file, fsync, rename, fsync of the directory) and runs under an
// exclusive lock. Within one instance and generation a write keeps the highest
// audit watermark, so a concurrent write by the server and the worker never
// lowers it.
type RecoveryMarkerStore interface {
	Read(ctx context.Context) (RecoveryMarker, error)
	// Replace writes the marker whether or not one exists. The first run and
	// the reconciliation use it, because they create or repair the marker. It
	// still refuses to lower the generation of a valid marker (ErrMarkerStale).
	Replace(ctx context.Context, m RecoveryMarker) error
	// Advance records a new watermark in the marker that exists. It returns
	// ErrMarkerMissing or ErrMarkerInvalid and writes nothing when there is no
	// valid marker, so that a marker which vanished while the process runs is
	// detected and not silently recreated.
	Advance(ctx context.Context, m RecoveryMarker) error
	// Cleanup removes temporary files that an interrupted write left behind.
	Cleanup(ctx context.Context) error
}

// RecoveryRow is the database side of the recovery generation (table
// recovery_state, one row).
type RecoveryRow struct {
	InstanceID string
	Generation uint64
	// Reconciling is true while state = 'reconciling'.
	Reconciling bool
	LastReason  string
}

// RecoveryFacts are the database inputs of the startup decision.
type RecoveryFacts struct {
	// Row is nil when recovery_state has no row.
	Row      *RecoveryRow
	HasUsers bool
	// MaxAuditSeq is the highest audit_events.seq, or 0 when there is none.
	MaxAuditSeq uint64
}

// ReconcileInput is the input of phase 1 of a reconciliation (section 6).
type ReconcileInput struct {
	// ExpectedGeneration is the generation of the recovery_state row that the
	// decision was based on, or 0 when the decision saw no row. Phase 1 refuses
	// when the locked row differs, so two reconciliations cannot both run.
	ExpectedGeneration uint64
	// InstanceID is used when the row has to be created.
	InstanceID    string
	NewGeneration uint64
	Reason        string
	// OldMarkerGeneration is the generation read from the marker, or 0.
	OldMarkerGeneration uint64
	ActorUserID         string
	RequestID           string
	// RecoveryCodeID and RecoveryCodeHash, when set, are inserted as the live
	// recovery code of ActorUserID in the same transaction that revokes the
	// restored ones (TD-M3-04).
	RecoveryCodeID   string
	RecoveryCodeHash string
	// NewCredentialID and NewPasswordHash, when set, supersede the current
	// credential of ActorUserID in the same transaction.
	NewCredentialID string
	NewPasswordHash string
}

// ReconcileCounts is how many records phase 1 invalidated.
type ReconcileCounts struct {
	SessionsRevoked      int `json:"sessions_revoked"`
	ReauthGrantsConsumed int `json:"reauth_grants_consumed"`
	ApprovalsRevoked     int `json:"approvals_revoked"`
	ApprovalsRejected    int `json:"approvals_rejected"`
	ValidationsInvalid   int `json:"validations_invalidated"`
	LabsRevoked          int `json:"lab_registrations_revoked"`
	AssessmentsToDraft   int `json:"assessments_returned_to_draft"`
	AssessmentsPaused    int `json:"assessments_paused"`
	RawCaptureCleared    int `json:"raw_evidence_capture_cleared"`
	JobsCancelled        int `json:"jobs_cancelled"`
	JobsStopRequested    int `json:"jobs_stop_requested"`
	AttemptsLost         int `json:"attempts_lost"`
	RecoveryCodesRevoked int `json:"codes_invalidated"`
	BootstrapExpired     int `json:"bootstrap_expired"`
}

// ErrRecoveryChanged is returned when the recovery_state row is not the one the
// decision was based on.
var ErrRecoveryChanged = errors.New("recovery state changed while the decision was made")

// RecoveryRepository is the database side of the recovery procedure.
type RecoveryRepository interface {
	// Facts reads the inputs of the decision table.
	Facts(ctx context.Context) (RecoveryFacts, error)

	// InitializeFirstRun inserts the recovery_state row (generation 1, normal)
	// and an audit event in one transaction. beforeCommit is called inside the
	// transaction with the current maximum audit sequence. It writes the marker.
	// When it fails, nothing is committed (section 8).
	InitializeFirstRun(ctx context.Context, instanceID string, beforeCommit func(ctx context.Context, maxSeq uint64) error) error

	// BeginReconcile is phase 1: one transaction that sets the state to
	// reconciling with the new generation, invalidates the authority recorded in
	// the database, and writes the audit event and the alert. When any statement
	// fails, nothing is committed. It returns the generation now in the row.
	BeginReconcile(ctx context.Context, in ReconcileInput) (ReconcileCounts, error)

	// MaxAuditSeq returns the highest audit sequence.
	MaxAuditSeq(ctx context.Context) (uint64, error)

	// CompleteReconcile is phase 3: state = normal, reconciled_at, and the audit
	// event system.recovery_reconciled with the given reason. generation must be
	// the generation in the row.
	CompleteReconcile(ctx context.Context, generation uint64, reason string) error
}
