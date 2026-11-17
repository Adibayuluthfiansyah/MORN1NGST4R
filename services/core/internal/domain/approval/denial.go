package approval

import "errors"

// Reason is a stable code for a denial. The codes are stored with the audit
// event (approval_events.reason_code) and returned in the details of an API
// error, so they never change. The first four match the error catalog of
// docs/design/api/error-model.md.
type Reason string

const (
	// ReasonNotApproved: the approval is pending, rejected or does not exist.
	ReasonNotApproved Reason = "approval_not_approved"
	// ReasonRevoked: the approval was revoked.
	ReasonRevoked Reason = "approval_revoked"
	// ReasonExpired: the approval expired. Expiry stops new launches only.
	ReasonExpired Reason = "approval_expired"
	// ReasonExhausted: every use of the approval is consumed.
	ReasonExhausted Reason = "approval_exhausted"
	// ReasonOperationMismatch: the operation is not the one that was approved.
	ReasonOperationMismatch Reason = "operation_mismatch"
	// ReasonUseNotClaimed: the job has not consumed a use of this approval.
	ReasonUseNotClaimed Reason = "use_not_claimed"
	// ReasonIntrusiveNoRetry: an intrusive operation has exactly one launch.
	ReasonIntrusiveNoRetry Reason = "intrusive_no_retry"
	// ReasonAttemptsExhausted: the job used every attempt of the approval.
	ReasonAttemptsExhausted Reason = "attempts_exhausted"
	// ReasonPreviousAttemptRunning: the process tree of the last attempt is not
	// confirmed gone.
	ReasonPreviousAttemptRunning Reason = "previous_attempt_running"
	// ReasonConcurrencyExceeded: as many jobs as approved already run.
	ReasonConcurrencyExceeded Reason = "concurrency_exceeded"
	// ReasonLimitsNotEstablished: the approved limits are not numbers (D-71).
	ReasonLimitsNotEstablished Reason = "limits_not_established"
	// ReasonScopeChanged: the scope version has changed since the approval was
	// made (FR-SCOPE-006).
	ReasonScopeChanged Reason = "approval_scope_changed"
	// ReasonPolicyChanged: the policy snapshot has changed since the approval
	// was made (D-90).
	ReasonPolicyChanged Reason = "approval_policy_changed"
	// ReasonValidationChanged: the target authorization validation is not the
	// one the approval was made on.
	ReasonValidationChanged Reason = "approval_validation_changed"
	// ReasonInvalidInput: a missing or malformed argument. It denies.
	ReasonInvalidInput Reason = "invalid_input"
)

// ErrDenied matches every *DenialError through errors.Is.
var ErrDenied = errors.New("approval denied")

// DenialError reports a denial with its stable reason. The text carries the
// reason code and nothing from the input.
type DenialError struct {
	Reason Reason
}

func (e *DenialError) Error() string { return "approval denied: " + string(e.Reason) }

// Is makes errors.Is(err, ErrDenied) true for every denial.
func (e *DenialError) Is(target error) bool { return target == ErrDenied }

// ReasonOf returns the reason of a denial. It reports false for any other error.
func ReasonOf(err error) (Reason, bool) {
	var de *DenialError
	if errors.As(err, &de) {
		return de.Reason, true
	}
	return "", false
}

func deny(r Reason) error { return &DenialError{Reason: r} }
