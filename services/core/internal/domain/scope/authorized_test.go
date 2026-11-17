package scope_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// authorizedAt returns the AuthorizedOperation that Decide gives for a passive
// operation at the given point.
func authorizedAt(t *testing.T, p scope.Point) (scope.AuthorizedOperation, operation.Operation) {
	t.Helper()
	in := dcInput(t, operation.CategoryPassiveRecon)
	in.Point = p
	v := scope.Decide(in)
	if v.Outcome != scope.OutcomeAllow {
		t.Fatalf("Decide = %v %v", v.Outcome, v.Reasons)
	}
	return v.Authorized, in.Operation
}

// FR-POLICY-001 AC4 and the M2 acceptance: a launcher given no authorization
// value refuses to launch, and the zero value is that value.
func TestAZeroAuthorizedOperationIsRejected(t *testing.T) {
	var a scope.AuthorizedOperation
	if !a.IsZero() || a.DecisionID() != "" || !a.OperationHash().IsZero() || a.Point() != scope.PointInvalid || !a.ExpiresAt().IsZero() {
		t.Error("the zero AuthorizedOperation carries data")
	}
	op := dcOp(t, dcSpec(operation.CategoryPassiveRecon))
	for name, now := range map[string]time.Time{"now": dcNow, "zero time": {}, "far future": dcNow.Add(1000 * time.Hour)} {
		if err := a.Verify(op, "job-1", now); !errors.Is(err, scope.ErrZeroAuthorization) {
			t.Errorf("%s: Verify = %v, want ErrZeroAuthorization", name, err)
		}
	}
	if err := a.Verify(operation.Operation{}, "job-1", dcNow); !errors.Is(err, scope.ErrZeroAuthorization) {
		t.Errorf("zero operation: Verify = %v, want ErrZeroAuthorization", err)
	}
	// A deny never carries one.
	if !scope.Decide(scope.DecisionInput{}).Authorized.IsZero() {
		t.Error("a denial carried an authorization")
	}
}

func TestVerifyAcceptsTheMatchingOperationBeforeExpiry(t *testing.T) {
	a, op := authorizedAt(t, scope.PointE3)
	if a.Point() != scope.PointE3 {
		t.Fatalf("point = %v", a.Point())
	}
	if err := a.Verify(op, "job-1", dcNow); err != nil {
		t.Errorf("Verify at the decision time: %v", err)
	}
	if err := a.Verify(op, "job-1", a.ExpiresAt().Add(-time.Nanosecond)); err != nil {
		t.Errorf("Verify one nanosecond before expiry: %v", err)
	}
}

func TestVerifyRejectsAnExpiredAuthorization(t *testing.T) {
	a, op := authorizedAt(t, scope.PointE3)
	for _, at := range []time.Time{a.ExpiresAt(), a.ExpiresAt().Add(time.Nanosecond), a.ExpiresAt().Add(time.Hour), {}} {
		if err := a.Verify(op, "job-1", at); !errors.Is(err, scope.ErrAuthorizationExpired) {
			t.Errorf("Verify at %v = %v, want ErrAuthorizationExpired", at, err)
		}
	}
}

// P-04: the executor recomputes the hash from what it is about to run.
// Review finding I1: the authorization is for one job. Another job that runs
// the same operation under the same approval cannot present it.
func TestVerifyRejectsAnotherJob(t *testing.T) {
	a, op := authorizedAt(t, scope.PointE3)
	if a.Job() != "job-1" {
		t.Fatalf("Job() = %q", a.Job())
	}
	for _, j := range []approval.JobID{"job-2", "", "JOB-1", "job-1 "} {
		if err := a.Verify(op, j, dcNow); !errors.Is(err, scope.ErrAuthorizationMismatch) {
			t.Errorf("Verify for job %q = %v, want ErrAuthorizationMismatch", j, err)
		}
	}
}

// Review finding I2: an authorization lives much shorter than the 30 seconds
// that revocation and scope changes have to stop work in (security.md 3), so a
// decision made just before a revocation cannot be used for long after it.
func TestTheAuthorizationIsShortLived(t *testing.T) {
	if scope.AuthorizationTTL <= 0 || scope.AuthorizationTTL > 10*time.Second {
		t.Errorf("AuthorizationTTL = %v, want a few seconds, well below the 30 second stop objective", scope.AuthorizationTTL)
	}
}

func TestVerifyRejectsADifferentOperation(t *testing.T) {
	a, _ := authorizedAt(t, scope.PointE3)
	s := dcSpec(operation.CategoryPassiveRecon)
	s.Params = map[string]string{"path_depth": "2"} // one changed parameter
	if err := a.Verify(dcOp(t, s), "job-1", dcNow); !errors.Is(err, scope.ErrAuthorizationMismatch) {
		t.Errorf("a changed parameter: %v, want ErrAuthorizationMismatch", err)
	}
	s = dcSpec(operation.CategoryPassiveRecon)
	s.Target = "other.example.com"
	if err := a.Verify(dcOp(t, s), "job-1", dcNow); !errors.Is(err, scope.ErrAuthorizationMismatch) {
		t.Errorf("another target: %v, want ErrAuthorizationMismatch", err)
	}
	if err := a.Verify(operation.Operation{}, "job-1", dcNow); !errors.Is(err, scope.ErrAuthorizationMismatch) {
		t.Errorf("a zero operation: %v, want ErrAuthorizationMismatch", err)
	}
}

// Only the decision of the launch point E3 lets a process start. A decision
// of E1 or E2 says a job may be created or claimed.
func TestOnlyALaunchDecisionVerifies(t *testing.T) {
	for _, p := range []scope.Point{scope.PointE1, scope.PointE2} {
		a, op := authorizedAt(t, p)
		if err := a.Verify(op, "job-1", dcNow); !errors.Is(err, scope.ErrAuthorizationPoint) {
			t.Errorf("point %v: Verify = %v, want ErrAuthorizationPoint", p, err)
		}
	}
}

// Go cannot hide a type's zero value or restrict a function to one caller. What
// can be checked is that nothing outside the package can fill the value in.
func TestNoPublicWayToForgeAnAuthorization(t *testing.T) {
	typ := reflect.TypeOf(scope.AuthorizedOperation{})
	if typ.NumField() == 0 {
		t.Fatal("AuthorizedOperation has no fields to check")
	}
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			t.Errorf("AuthorizedOperation.%s is exported and can be set from outside the package", f.Name)
		}
	}
	// The verdict is the only other carrier. Its Authorized field has the
	// unexported type's zero value unless Decide set it, and an Allow without
	// one cannot be built by Decide.
	in := dcInput(t, operation.CategoryPassiveRecon)
	if v := scope.Decide(in); v.Outcome == scope.OutcomeAllow && v.Authorized.IsZero() {
		t.Error("Decide allowed without an authorization")
	}
}
