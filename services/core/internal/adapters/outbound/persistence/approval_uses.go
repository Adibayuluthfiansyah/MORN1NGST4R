package persistence

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence/sqlcgen"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// maxGeneration is the largest generation that fits the bigint column.
const maxGeneration = uint64(math.MaxInt64)

// ApprovalUses implements outbound.ApprovalUses with the SQL functions
// morningstar.consume_approval_use and morningstar.check_approval_launch
// (migration 00006). The worker role has no UPDATE on approvals, so these
// functions are its only way to consume a use (CC-02, DB-13).
type ApprovalUses struct {
	pool *pgxpool.Pool
}

// NewApprovalUses returns the adapter. pool is the worker pool.
func NewApprovalUses(pool *pgxpool.Pool) *ApprovalUses { return &ApprovalUses{pool: pool} }

var _ outbound.ApprovalUses = (*ApprovalUses)(nil)

// reasonRecoveryGeneration is the reason code of the SQL functions for CC-15.
const reasonRecoveryGeneration = "recovery_generation"

var knownReasons = map[string]approval.Reason{}

func init() {
	for _, r := range []approval.Reason{
		approval.ReasonNotApproved, approval.ReasonRevoked, approval.ReasonExpired, approval.ReasonExhausted,
		approval.ReasonOperationMismatch, approval.ReasonUseNotClaimed, approval.ReasonIntrusiveNoRetry,
		approval.ReasonAttemptsExhausted, approval.ReasonScopeChanged, approval.ReasonPolicyChanged,
		approval.ReasonValidationChanged, approval.ReasonInvalidInput,
	} {
		knownReasons[string(r)] = r
	}
}

func denial(r approval.Reason) error { return &approval.DenialError{Reason: r} }

// validUUID reports whether s is a canonical UUID. The database would refuse
// anything else, but a refusal made here never reaches the database.
func validUUID(s string) bool {
	var u pgtype.UUID
	return len(s) == 36 && u.Scan(s) == nil && u.Valid
}

func bindingValid(b approval.Binding) bool {
	return validUUID(b.ValidationID) && validUUID(b.ScopeVersionID) && b.PolicySnapshot != [32]byte{}
}

// Claim consumes one use for the job in one statement, or returns the use that
// the job already holds.
func (s *ApprovalUses) Claim(ctx context.Context, c outbound.ApprovalClaim) (outbound.ClaimResult, error) {
	if c.ClaimEpoch < 1 || !validUUID(c.UseID) || !validUUID(c.ApprovalID) || !validUUID(c.JobID) ||
		!bindingValid(c.Binding) || c.Operation == [32]byte{} || c.Generation > maxGeneration {
		return outbound.ClaimResult{}, denial(approval.ReasonInvalidInput)
	}
	row, err := sqlcgen.New(Querier(ctx, s.pool)).ConsumeApprovalUse(ctx, sqlcgen.ConsumeApprovalUseParams{
		UseID: toUUID(c.UseID), ApprovalID: toUUID(c.ApprovalID), JobID: toUUID(c.JobID),
		ClaimEpoch: int32(c.ClaimEpoch), Generation: int64(c.Generation), OperationHash: c.Operation[:],
		ScopeVersionID: toUUID(c.Binding.ScopeVersionID), PolicySnapshotHash: c.Binding.PolicySnapshot[:], ValidationID: toUUID(c.Binding.ValidationID),
	})
	if err != nil {
		return outbound.ClaimResult{}, fmt.Errorf("persistence: claim approval use: %w", err)
	}
	switch row.Outcome {
	case "consumed", "reused":
		if !validUUID(row.UseID) {
			return outbound.ClaimResult{}, errors.New("persistence: claim approval use: the function returned no use")
		}
		r := outbound.ClaimResult{Outcome: outbound.ClaimConsumed, UseID: row.UseID}
		if row.Outcome == "reused" {
			r.Outcome = outbound.ClaimReused
		}
		return r, nil
	case "refused":
		return outbound.ClaimResult{}, refusal(row.Reason)
	}
	return outbound.ClaimResult{}, errors.New("persistence: claim approval use: unknown outcome")
}

// CheckLaunch checks, immediately before a launch, that the job holds a use of
// a live approval. It consumes nothing.
func (s *ApprovalUses) CheckLaunch(ctx context.Context, c outbound.LaunchCheck) error {
	if c.ClaimEpoch < 1 || !validUUID(c.ApprovalID) || !validUUID(c.JobID) || !bindingValid(c.Binding) || c.Operation == [32]byte{} || c.Generation > maxGeneration {
		return denial(approval.ReasonInvalidInput)
	}
	reason, err := sqlcgen.New(Querier(ctx, s.pool)).CheckApprovalLaunch(ctx, sqlcgen.CheckApprovalLaunchParams{
		ApprovalID: toUUID(c.ApprovalID), JobID: toUUID(c.JobID), Generation: int64(c.Generation), ClaimEpoch: int32(c.ClaimEpoch), OperationHash: c.Operation[:],
		ScopeVersionID: toUUID(c.Binding.ScopeVersionID), PolicySnapshotHash: c.Binding.PolicySnapshot[:], ValidationID: toUUID(c.Binding.ValidationID),
	})
	if err != nil {
		return fmt.Errorf("persistence: check approval launch: %w", err)
	}
	if reason == "" {
		return nil
	}
	return refusal(reason)
}

// refusal maps a reason code of the SQL function to an error. A code that this
// package does not know is an error of the store, which the caller treats as a
// refusal too.
func refusal(reason string) error {
	if reason == reasonRecoveryGeneration {
		return outbound.ErrRecoveryGeneration
	}
	if r, ok := knownReasons[reason]; ok {
		return denial(r)
	}
	return errors.New("persistence: the function returned an unknown reason")
}

// toUUID converts a string that validUUID accepted.
func toUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		panic("persistence: toUUID on an unchecked value") // every caller checks validUUID first
	}
	return u
}
