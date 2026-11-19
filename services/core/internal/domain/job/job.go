// Package job holds the lifecycle rules of a job and of its attempts: the legal
// transitions (domain-model.md section 3), the retry rules of D-80, D-49 and
// D-55, and the timing that the stop objective of D-48 depends on. It reads no
// clock and no store. The worker and the database apply these rules, and the
// database refuses a change that breaks them (DB-11).
package job

import (
	"errors"
	"time"
)

type State string

const (
	Queued           State = "queued"
	AwaitingApproval State = "awaiting_approval"
	Running          State = "running"
	Succeeded        State = "succeeded"
	Failed           State = "failed"
	Cancelled        State = "cancelled"
)

// Control is the stop request of a job, separate from its state.
type Control string

const (
	ControlNone          Control = "none"
	ControlStopRequested Control = "stop_requested"
	ControlStopConfirmed Control = "stop_confirmed"
	ControlStopFailed    Control = "stop_failed"
)

type AttemptState string

const (
	AttemptLaunching AttemptState = "launching"
	AttemptRunning   AttemptState = "running"
	AttemptSucceeded AttemptState = "succeeded"
	AttemptFailed    AttemptState = "failed"
	AttemptTimedOut  AttemptState = "timed_out"
	AttemptKilled    AttemptState = "killed"
	AttemptCancelled AttemptState = "cancelled"
	AttemptLost      AttemptState = "lost"
)

// Live says that the attempt may still have a process.
func (s AttemptState) Live() bool { return s == AttemptLaunching || s == AttemptRunning }

type Risk string

const (
	Passive   Risk = "passive"
	LowImpact Risk = "low_impact"
	Intrusive Risk = "intrusive"
	AI        Risk = "ai"
)

// NeedsApproval says that the job relies on an approval use (D-55).
func (r Risk) NeedsApproval() bool { return r == LowImpact || r == Intrusive }

// Terminal says that no transition leaves the state.
func (s State) Terminal() bool { return s == Succeeded || s == Failed || s == Cancelled }

// Valid reports whether s is one of the six states.
func (s State) Valid() bool {
	switch s {
	case Queued, AwaitingApproval, Running, Succeeded, Failed, Cancelled:
		return true
	}
	return false
}

// CanMove is the transition table of the job state machine. The empty state is
// the creation of the job. The same table is the CHECK of job_transitions and
// the parity trigger of the jobs table.
func CanMove(from, to State) bool {
	switch {
	case from == "":
		return to == Queued || to == AwaitingApproval
	case from == AwaitingApproval:
		return to == Queued || to == Cancelled
	case from == Queued:
		return to == Running || to == Failed || to == Cancelled
	case from == Running:
		return to == Succeeded || to == Failed || to == Queued || to == Cancelled
	}
	return false
}

// RetryPolicy is the value of jobs.retry_policy. A job created without a retry
// policy never retries after an attempt (D-80).
type RetryPolicy string

const (
	RetryNone    RetryPolicy = "none"
	RetryBounded RetryPolicy = "bounded"
)

// Cause is why an attempt ended without success.
type Cause string

const (
	// CauseLeaseLost: the worker stopped renewing its lease and the attempt was
	// reaped.
	CauseLeaseLost Cause = "lease_lost"
	// CauseExecution: the process ended with a failure, or the launch itself failed
	// after the attempt was counted.
	CauseExecution Cause = "execution_failed"
	// CauseTimeout: the process ran past its approved duration.
	CauseTimeout Cause = "timed_out"
	// CauseStopped: a person or the system asked the job to stop.
	CauseStopped Cause = "stopped"
	// CausePolicyDenial: a policy check refused the job. A denial is final.
	CausePolicyDenial Cause = "policy_denial"
)

// Action is what to do with a job after an attempt ended.
type Action int

const (
	// ActionFail: the job is failed. The zero value fails, so an input that was
	// never filled in does not retry.
	ActionFail Action = iota
	// ActionRequeue: the job goes back to queued for a retry claim.
	ActionRequeue
	// ActionWait: the job cannot be decided yet because the process tree of the
	// attempt is not confirmed gone. Nothing is requeued and nothing is failed.
	ActionWait
)

// RetryInput is everything that the retry rule reads.
type RetryInput struct {
	Policy RetryPolicy
	Risk   Risk
	Cause  Cause
	// Attempts is jobs.execution_attempts after the attempt that ended, and
	// MaxAttempts is jobs.max_execution_attempts.
	Attempts, MaxAttempts int
	Control               Control
	// TerminationConfirmed says that the process tree of the last attempt is
	// confirmed gone.
	TerminationConfirmed bool
}

// Outcome is the result of DecideRetry. FailureClass is set when the action is
// ActionFail and is a stable code that jobs.failure_class stores.
type Outcome struct {
	Action       Action
	FailureClass string
}

const (
	FailureStopped           = "stopped"
	FailurePolicyDenial      = "policy_denial"
	FailureIntrusiveNoRetry  = "intrusive_no_retry"
	FailureRetryDisabled     = "retry_disabled"
	FailureAttemptsExhausted = "attempts_exhausted"
	FailureTimedOut          = "timed_out"
	FailureNotRetryable      = "not_retryable"
	FailureSandboxRefused    = "sandbox_refused"
	FailureLeaseExpired      = "lease_expired"
)

// DecideRetry is the rule for a job whose attempt ended without success.
//
// A stop request, a policy denial and a timeout are final (D-80). An intrusive
// operation has exactly one launch and fails when its lease expires (D-49). A
// retry needs a retry policy, an attempt left, and a process tree that is
// confirmed gone, and it never reuses an approval use (the claim reuses the
// record, D-55). Everything else fails.
func DecideRetry(in RetryInput) Outcome {
	switch {
	case in.Cause == CauseStopped || in.Control != ControlNone:
		return Outcome{ActionFail, FailureStopped}
	case in.Cause == CausePolicyDenial:
		return Outcome{ActionFail, FailurePolicyDenial}
	case in.Cause == CauseTimeout:
		return Outcome{ActionFail, FailureTimedOut}
	case in.Cause != CauseLeaseLost && in.Cause != CauseExecution:
		return Outcome{ActionFail, FailureNotRetryable}
	case in.Risk == Intrusive:
		if in.Cause == CauseLeaseLost {
			return Outcome{ActionFail, FailureLeaseExpired}
		}
		return Outcome{ActionFail, FailureIntrusiveNoRetry}
	case in.Policy != RetryBounded:
		return Outcome{ActionFail, FailureRetryDisabled}
	case in.MaxAttempts < 1 || in.Attempts >= in.MaxAttempts:
		return Outcome{ActionFail, FailureAttemptsExhausted}
	case !in.TerminationConfirmed:
		return Outcome{Action: ActionWait}
	}
	return Outcome{Action: ActionRequeue}
}

// Timing holds the numbers that the stop objective depends on. They are
// provisional until the owner sets them (O-35, gate G-revoke).
type Timing struct {
	// LeaseTTL is how long a lease lives after the last heartbeat.
	LeaseTTL time.Duration
	// Heartbeat is how often a worker renews the lease of a running job.
	Heartbeat time.Duration
	// ControlPoll is how often a worker looks for a stop request of its jobs.
	ControlPoll time.Duration
	// Reap is how often a worker looks for expired leases and stop requests of
	// queued jobs.
	Reap time.Duration
	// StopTimeout bounds the kill of a process tree and the check that it is gone.
	StopTimeout time.Duration
	// Watermark is how often the recovery marker is advanced while running.
	Watermark time.Duration
}

// StopObjective is the objective of D-48.
const StopObjective = 30 * time.Second

// DefaultTiming gives a worst case of ControlPoll plus StopTimeout (12 seconds)
// for a stop request on a live worker, and LeaseTTL plus Reap plus StopTimeout
// (26 seconds) for the attempt of a dead worker.
func DefaultTiming() Timing {
	return Timing{
		LeaseTTL: 12 * time.Second, Heartbeat: 4 * time.Second, ControlPoll: 2 * time.Second,
		Reap: 4 * time.Second, StopTimeout: 10 * time.Second, Watermark: 30 * time.Second,
	}
}

var ErrTiming = errors.New("job: the timing does not keep the stop objective")

// Validate refuses a timing that cannot keep the stop objective or that lets a
// healthy lease expire between two heartbeats.
func (t Timing) Validate() error {
	if t.LeaseTTL <= 0 || t.Heartbeat <= 0 || t.ControlPoll <= 0 || t.Reap <= 0 || t.StopTimeout <= 0 || t.Watermark <= 0 {
		return ErrTiming
	}
	if t.LeaseTTL < 2*t.Heartbeat+t.Heartbeat/2 {
		return ErrTiming
	}
	if t.ControlPoll+t.StopTimeout > StopObjective || t.LeaseTTL+t.Reap+t.StopTimeout > StopObjective {
		return ErrTiming
	}
	return nil
}
