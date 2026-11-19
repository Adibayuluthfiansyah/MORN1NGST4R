package outbound

import (
	"context"
	"errors"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

var (
	// ErrNoJob: no job is ready to be claimed.
	ErrNoJob = errors.New("no job is ready")
	// ErrJobNotFound: the job does not exist.
	ErrJobNotFound = errors.New("the job does not exist")
	// ErrStaleClaim: the write names a claim that is not the current one of the
	// job: another worker holds it, the lease was lost, or the job moved on. A
	// stale worker is fenced out with this error (P-36). Nothing was written.
	ErrStaleClaim = errors.New("the claim of the job is not current")
	// ErrAttemptsExhausted: the job has no attempt left, or its counter changed.
	ErrAttemptsExhausted = errors.New("the job has no attempt left")
)

// JobRecord is the row of a job that the worker needs.
type JobRecord struct {
	ID, ProgramID, AssessmentID, Kind string
	// OperationID, ApprovalID, ApprovalUseID, ScopeVersionID and ValidationID are
	// empty when the column is null.
	OperationID, ApprovalID, ApprovalUseID, ScopeVersionID, ValidationID string
	ControlReason                                                        string
	Risk                                                                 job.Risk
	State                                                                job.State
	Control                                                              job.Control
	RetryPolicy                                                          job.RetryPolicy
	MaxAttempts, Attempts, ClaimEpoch                                    int
	LeaseOwner                                                           string
	LeaseExpiresAt                                                       time.Time
	AvailableAt                                                          time.Time
}

// Fence names the claim that a write belongs to. Every worker write is
// conditioned on it (P-36): the job must be running under this epoch and this
// lease owner.
type Fence struct {
	JobID  string
	Epoch  int
	Worker string
}

// TransitionActor says who caused a change of state. A user carries an id.
type TransitionActor struct {
	Kind   string // "user", "worker" or "system"
	UserID string
}

// ClaimJob moves a queued job to running under the next epoch. Epoch is the new
// one, and the job must be queued at Epoch-1 with no stop request.
type ClaimJob struct {
	Fence
	LeaseUntil time.Time
	// Generation is the recovery generation that the worker claims under. The
	// claim is refused when the database is at another one, for every kind of job.
	Generation uint64
	// UseID is the approval use that the claim consumed or found, or empty for a
	// job that needs none.
	UseID string
}

// MoveJob changes the state of a job and writes the history row in the same
// transaction (DB-11). From and To must be a legal pair. A worker move names its
// fence (Epoch above zero and Worker set), a move by a person or by the system
// leaves them empty and is conditioned on From only.
type MoveJob struct {
	JobID    string
	From, To job.State
	Epoch    int
	Worker   string
	// Control, when set, replaces control_state in the same statement.
	Control      job.Control
	FailureClass string
	Reason       string
	Actor        TransitionActor
	// AvailableAt, when set, is when a requeued job may be claimed again.
	AvailableAt time.Time
}

// CancelJob cancels a job that waits (queued or awaiting approval).
type CancelJob struct {
	JobID   string
	From    job.State
	Control job.Control
	Reason  string
	Actor   TransitionActor
}

// NewAttempt is the launch of a job: the counter is raised and the attempt is
// inserted in the one transaction of the caller.
type NewAttempt struct {
	ID         string
	Fence      Fence
	DecisionID string
	UseID      string
}

// Attempt is a row of job_attempts.
type Attempt struct {
	ID, JobID, Worker, ProcessRef string
	No, ClaimEpoch                int
	State                         job.AttemptState
	TerminationConfirmed          bool
	StartedAt                     time.Time
}

// AttemptUpdate changes an attempt. Zero fields stay as they are.
type AttemptUpdate struct {
	ID         string
	State      job.AttemptState
	ProcessRef string
	ErrorClass string
	ExitCode   *int
	// End sets ended_at. ConfirmTermination sets termination_confirmed_at.
	End, ConfirmTermination bool
}

// JobStore is the persistence of the queue, the lease and the attempts. Every
// method joins the transaction of its context (P-07), and every write of a
// running job is fenced. The transactions are READ COMMITTED, so the
// exclusivity of a claim comes from the row lock of LockNextQueued and from the
// conditional statements, never from the isolation level.
type JobStore interface {
	// LockNextQueued locks the next job that may be claimed (queued, no stop
	// request, available) and skips rows that another transaction holds.
	LockNextQueued(ctx context.Context, now time.Time) (JobRecord, error)
	// Lock reads a job and holds a row lock until the transaction ends.
	Lock(ctx context.Context, id string) (JobRecord, error)
	Claim(ctx context.Context, c ClaimJob) error
	Move(ctx context.Context, m MoveJob) error
	// Cancel cancels a job that is not running, in the columns that the API role
	// may change, so a person's cancellation does not need the rights of a worker.
	Cancel(ctx context.Context, c CancelJob) error
	// Heartbeat renews the lease and returns the control state of the job.
	Heartbeat(ctx context.Context, f Fence, until time.Time) (job.Control, error)
	// Control returns the control state of a job that the fence still holds.
	Control(ctx context.Context, f Fence) (job.Control, error)
	// SetControl records the outcome of a stop (stop_failed) on a job that the
	// fence still holds. A stop request is made by RequestStop.
	SetControl(ctx context.Context, f Fence, c job.Control) error
	// RequestStop sets stop_requested on a running job. It reports whether it
	// changed anything.
	RequestStop(ctx context.Context, jobID, reason string) (bool, error)
	// ExpiredLeases locks running jobs whose lease ended before now.
	ExpiredLeases(ctx context.Context, now time.Time, limit int) ([]JobRecord, error)
	// StopRequestedWaiting locks jobs that are not running and carry a stop request.
	StopRequestedWaiting(ctx context.Context, limit int) ([]JobRecord, error)
	// RunningCount counts the running jobs under an approval.
	RunningCount(ctx context.Context, approvalID string) (int, error)
	AddAttempt(ctx context.Context, a NewAttempt) (Attempt, error)
	UpdateAttempt(ctx context.Context, u AttemptUpdate) error
	// Attempts returns the attempts of a job, oldest first.
	Attempts(ctx context.Context, jobID string) ([]Attempt, error)
}

// PolicyDecision is a row of policy_decisions.
type PolicyDecision struct {
	ID, ProgramID, AssessmentID, OperationID, JobID string
	Point                                           scope.Point
	Outcome                                         scope.Outcome
	Reasons                                         []string
	ScopeVersionID, ValidationID, ApprovalID        string
	PolicySnapshot                                  [32]byte
}

// PolicyDecisions records every decision of the evaluator (E1 to E4).
type PolicyDecisions interface {
	Insert(ctx context.Context, d PolicyDecision) error
}

// ScopeInput is everything that the decision function reads about a job, loaded
// at the moment of the decision.
type ScopeInput struct {
	Operation  operation.Operation
	Rules      scope.RuleSet
	Policy     scope.PolicyState
	Validation scope.ValidationState
	// Approval is nil for a job that needs none.
	Approval *approval.Approval
}

// ScopeInputs loads the operation, the rules of the scope version in force, the
// policy state, the validation record and the approval of a job. It belongs to
// the program, validation and approval modules (TD-M2-06, TD-M2-07, TD-M2-08).
// An error, or a missing source, is a denial (S1.6): the evaluator never guesses.
type ScopeInputs interface {
	Load(ctx context.Context, j JobRecord) (ScopeInput, error)
}

// LaunchSpec is what the runner needs to preflight and start one attempt. It
// names the operation by id: the runner loads nothing from a string that an AI
// produced (security.md section 10).
type LaunchSpec struct {
	AttemptID   string
	JobID       string
	OperationID string
	Epoch       int
	// MaxDuration is the approved duration of the operation.
	MaxDuration time.Duration
	// Operation and Authorized are what the executor verifies before it starts a
	// process: the authorization is for this operation and this job (P-04).
	Operation  operation.Operation
	Authorized scope.AuthorizedOperation
}

// ProcessExit is how a process ended.
type ProcessExit struct {
	Code     int
	TimedOut bool
	// Err is set when the process could not be waited for.
	Err error
}

// Process is a started attempt.
type Process interface {
	// Ref names the process group, so that another worker can terminate it.
	Ref() string
	// Done delivers the exit once.
	Done() <-chan ProcessExit
	// Stop kills the process tree and returns only when it is confirmed gone.
	Stop(ctx context.Context) error
}

// AttemptRunner starts and stops the process of an attempt. The sandbox
// launcher of M8 implements it. Until then the worker has a runner that refuses.
type AttemptRunner interface {
	// Preflight refuses before an attempt exists (D-55): the sandbox self-test,
	// the egress path and the tool manifest. An error creates no attempt.
	Preflight(ctx context.Context, s LaunchSpec) error
	Start(ctx context.Context, s LaunchSpec) (Process, error)
	// Terminate kills the process tree of a lost attempt and reports whether it
	// is confirmed gone. ref is the reference that the worker recorded. It is
	// empty when the worker ended between the start of the process and that
	// record, and the runner then finds the tree by the attempt id. It returns
	// true only when no process of the attempt exists.
	Terminate(ctx context.Context, attemptID, ref string) (bool, error)
}

// ErrSandboxUnavailable is what a runner returns when it cannot isolate an attempt.
var ErrSandboxUnavailable = errors.New("the sandbox is not available")

// SystemAlerts raises an alert that a person has to clear.
type SystemAlerts interface {
	Raise(ctx context.Context, kind string, detail map[string]any) error
}
