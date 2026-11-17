package scope

import (
	"errors"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// AuthorizationTTL is how long an AuthorizedOperation stays valid after the
// decision. It is short on purpose: the launch check E3 is the narrowest gap
// between a decision and an action (architecture.md section 8), and a decision
// made just before a revocation must not stay usable for the 30 seconds that
// revocation has to stop work in (security.md 3). The value is a choice of
// milestone M2 that the owner may change. An executor calls Verify immediately
// before it starts the process.
const AuthorizationTTL = 5 * time.Second

var (
	// ErrZeroAuthorization is returned for an AuthorizedOperation that Decide
	// never produced.
	ErrZeroAuthorization = errors.New("no authorization")
	// ErrAuthorizationExpired is returned when the authorization is not valid
	// any more.
	ErrAuthorizationExpired = errors.New("authorization expired")
	// ErrAuthorizationMismatch is returned when the operation to launch is not
	// the one that was authorized.
	ErrAuthorizationMismatch = errors.New("authorization is for another operation")
	// ErrAuthorizationPoint is returned when the decision was not made at the
	// launch point E3.
	ErrAuthorizationPoint = errors.New("authorization was not made at the launch point")
)

// Point is where in the flow a decision is made (P-05). The zero value is
// PointInvalid. E4, the promotion of a discovered asset, needs the asset domain
// and is not part of M2.
type Point int

const (
	PointInvalid Point = iota
	// PointE1 is job creation, or the conversion of a plan item into a job.
	PointE1
	// PointE2 is the claim of a job by a worker.
	PointE2
	// PointE3 is the check immediately before a process is launched.
	PointE3
)

// String returns the stable name of the point.
func (p Point) String() string {
	switch p {
	case PointE1:
		return "E1"
	case PointE2:
		return "E2"
	case PointE3:
		return "E3"
	}
	return "invalid"
}

// AuthorizedOperation is what a tool executor needs before it starts a process
// (P-04). It carries the decision ID, the hash of the operation, the job and an
// expiry. It is for one job: another job that runs the same operation under the
// same approval cannot present it.
// Its fields are unexported and Decide is the only function that fills them in,
// so nothing outside this package can build a non-zero value.
//
// Strength of the control, stated honestly: Go cannot hide the zero value, and
// `var a scope.AuthorizedOperation` compiles anywhere. The zero value is
// rejected by Verify. The architecture test asserts that Decide is called only
// from application/scope. Neither makes bypass impossible. They make accidental
// bypass hard and forgery visible, and E3 remains the real enforcement.
type AuthorizedOperation struct {
	decisionID string
	hash       operation.Hash
	job        approval.JobID
	expiresAt  time.Time
	point      Point
}

// IsZero reports whether Decide never produced the value.
func (a AuthorizedOperation) IsZero() bool { return a.hash.IsZero() }

func (a AuthorizedOperation) DecisionID() string            { return a.decisionID }
func (a AuthorizedOperation) OperationHash() operation.Hash { return a.hash }
func (a AuthorizedOperation) ExpiresAt() time.Time          { return a.expiresAt }
func (a AuthorizedOperation) Point() Point                  { return a.point }
func (a AuthorizedOperation) Job() approval.JobID           { return a.job }

// Verify is the check that a tool executor runs. It rejects the zero value, an
// operation other than the authorized one (the executor passes the operation it
// is about to run, so the hash is recomputed from it), another job, a decision
// that was not made at E3, and an authorization that is expired. The instant of
// expiry is already expired.
//
// Verify cannot see a revocation. The value is short-lived for that reason, and
// the executor records the launch through approval.AuthorizeLaunch in the same
// step, which is what counts the attempt.
func (a AuthorizedOperation) Verify(op operation.Operation, job approval.JobID, now time.Time) error {
	if a.IsZero() {
		return ErrZeroAuthorization
	}
	if op.IsZero() || op.Hash() != a.hash || job != a.job {
		return ErrAuthorizationMismatch
	}
	if a.point != PointE3 {
		return ErrAuthorizationPoint
	}
	if now.IsZero() || !now.Round(0).Before(a.expiresAt) {
		return ErrAuthorizationExpired
	}
	return nil
}
