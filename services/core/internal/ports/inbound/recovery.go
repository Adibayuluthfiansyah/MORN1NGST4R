package inbound

import (
	"context"
	"time"
)

type RecoveryState struct {
	// Normal is true once the startup check passed and claims are allowed.
	Normal             bool
	Required           bool
	Reason             string
	MarkerGeneration   uint64
	DatabaseGeneration uint64
	DetectedAt         time.Time
}

type ReconcileRequest struct {
	ActorUserID string
	RequestID   string
	// NewRecoveryCode replaces every open recovery code in the same transaction
	// that revokes the restored ones (TD-M3-04).
	NewRecoveryCodeID   string
	NewRecoveryCodeHash string
	// NewCredentialID and NewPasswordHash, when set, replace the current
	// password in the same transaction. The password of the restored database
	// is not trusted: it may be older than a rotation.
	NewCredentialID string
	NewPasswordHash string
}

type ReconcileCounts struct {
	Generation             uint64
	SessionsRevoked        int
	ApprovalsRevoked       int
	ApprovalsRejected      int
	ValidationsInvalidated int
	LabsRevoked            int
	JobsCancelled          int
}

// Recovery is the contract of the recovery use case that the authentication use
// case needs: the state for gating, and the reconciliation.
type Recovery interface {
	State() RecoveryState
	Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileCounts, error)
}
