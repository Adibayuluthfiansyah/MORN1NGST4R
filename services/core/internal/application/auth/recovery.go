package auth

import (
	"context"
	"errors"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// ResetPassword replaces the password with the one-time recovery code (AUTH-08,
// D-68). The answer never says whether a code exists. The code is consumed, every
// session is revoked, and a new code and a new session are issued in one
// transaction.
func (s *Service) ResetPassword(ctx context.Context, in inbound.RecoveryResetInput) (inbound.RecoveryResetResult, error) {
	canonical, shapeOK := identity.CanonicalRecoveryCode(in.RecoveryCode)
	user, found, err := s.Accounts.SingleUser(ctx)
	if err != nil {
		return inbound.RecoveryResetResult{}, s.mapStoreError(err)
	}
	if !found {
		return inbound.RecoveryResetResult{}, inbound.ErrSetupUnavailable
	}
	if identity.ValidateNewPassword(user.Username, in.NewPassword) != nil {
		return inbound.RecoveryResetResult{}, validationError(inbound.FieldError{Field: "newPassword", Code: "invalid"})
	}
	allowed, err := s.Attempts.Reserve(ctx, outbound.AttemptRecovery, in.ClientKey, user.ID, attemptWindow, identity.MaxFailedInWindow)
	if err != nil {
		return inbound.RecoveryResetResult{}, s.mapStoreError(err)
	}
	if !allowed {
		s.auditDenied(ctx, "auth.recovery_reset", in.RequestID, map[string]any{"reason": "rate_limited"})
		return inbound.RecoveryResetResult{}, rateLimited()
	}
	deny := func() (inbound.RecoveryResetResult, error) {
		s.auditDenied(ctx, "auth.recovery_reset", in.RequestID, map[string]any{"reason": "recovery_invalid"})
		return inbound.RecoveryResetResult{}, inbound.ErrRecoveryInvalid
	}

	live, haveLive, err := s.RecoveryCodes.LockLive(ctx, user.ID)
	if err != nil {
		return inbound.RecoveryResetResult{}, s.mapStoreError(err)
	}
	if !shapeOK || !haveLive {
		if err := s.Hasher.SpendVerifyCost(ctx, in.RecoveryCode); err != nil {
			return inbound.RecoveryResetResult{}, err
		}
		return deny()
	}
	match, err := s.Hasher.Verify(ctx, canonical, live.Hash)
	if err != nil {
		return inbound.RecoveryResetResult{}, err
	}
	if !match {
		return deny()
	}

	phc, err := s.Hasher.Hash(ctx, in.NewPassword)
	if err != nil {
		return inbound.RecoveryResetResult{}, err
	}
	shown, codeHash, err := s.newRecoveryCode(ctx)
	if err != nil {
		return inbound.RecoveryResetResult{}, err
	}

	var out inbound.RecoveryResetResult
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		locked, ok, err := s.RecoveryCodes.LockLive(ctx, user.ID)
		if err != nil {
			return err
		}
		if !ok || locked.ID != live.ID {
			return inbound.ErrRecoveryInvalid
		}
		if err := s.RecoveryCodes.MarkUsed(ctx, locked.ID); err != nil {
			if errors.Is(err, outbound.ErrNotLive) {
				return inbound.ErrRecoveryInvalid
			}
			return err
		}
		if err := s.Accounts.SetPassword(ctx, user.ID, s.NewID(), phc); err != nil {
			return err
		}
		if _, err := s.Sessions.RevokeAllExcept(ctx, user.ID, "", "recovery_reset"); err != nil {
			return err
		}
		if err := s.RecoveryCodes.Insert(ctx, s.NewID(), user.ID, codeHash); err != nil {
			return err
		}
		session, _, err := s.issueSession(ctx, user.ID, in.UserAgent)
		if err != nil {
			return err
		}
		if err := s.Attempts.Settle(ctx, outbound.AttemptRecovery, in.ClientKey, user.ID); err != nil {
			return err
		}
		out = inbound.RecoveryResetResult{RecoveryCode: shown, Session: session}
		return s.auditUser(ctx, "auth.recovery_reset", user.ID, in.RequestID, nil)
	})
	if errors.Is(err, inbound.ErrRecoveryInvalid) {
		return deny()
	}
	return out, s.mapStoreError(err)
}

// Reconcile starts the reconciliation after a detected restore (AUTH-10). It
// needs the password of the restored database, checked like a login and counted
// by the same rate limit, and a new password that replaces it in the same
// transaction. A restored hash may be older than a rotation, so it is never kept.
// The recovery code issued here replaces every code that the restored database
// held, and it is shown once.
func (s *Service) Reconcile(ctx context.Context, in inbound.ReconcileInput) (inbound.ReconcileResult, error) {
	if s.Gate() != inbound.GateRecoveryRequired {
		return inbound.ReconcileResult{}, inbound.ErrRecoveryNotRequired
	}
	if !in.ConfirmRevocation || identity.ValidatePasswordInput(in.Password) != nil {
		return inbound.ReconcileResult{}, validationError(inbound.FieldError{Field: "confirmRevocation", Code: "required"})
	}
	user, found, err := s.Accounts.SingleUser(ctx)
	if err != nil {
		return inbound.ReconcileResult{}, s.mapStoreError(err)
	}
	if identity.ValidateNewPassword(user.Username, in.NewPassword) != nil || in.NewPassword == in.Password {
		return inbound.ReconcileResult{}, validationError(inbound.FieldError{Field: "newPassword", Code: "invalid"})
	}
	userID := ""
	if found {
		userID = user.ID
	}
	allowed, err := s.Attempts.Reserve(ctx, outbound.AttemptReauthenticate, in.ClientKey, userID, attemptWindow, identity.MaxFailedInWindow)
	if err != nil {
		return inbound.ReconcileResult{}, s.mapStoreError(err)
	}
	if !allowed {
		s.auditDenied(ctx, "system.recovery_reconciled", in.RequestID, map[string]any{"reason": "rate_limited"})
		return inbound.ReconcileResult{}, rateLimited()
	}
	match, err := s.verifyPassword(ctx, user, found, in.Password)
	if err != nil {
		return inbound.ReconcileResult{}, err
	}
	if !match {
		s.auditDenied(ctx, "system.recovery_reconciled", in.RequestID, map[string]any{"reason": "invalid_credentials"})
		return inbound.ReconcileResult{}, inbound.ErrInvalidCredentials
	}
	phc, err := s.Hasher.Hash(ctx, in.NewPassword)
	if err != nil {
		return inbound.ReconcileResult{}, err
	}
	shown, codeHash, err := s.newRecoveryCode(ctx)
	if err != nil {
		return inbound.ReconcileResult{}, err
	}
	counts, err := s.Recovery.Reconcile(ctx, inbound.ReconcileRequest{
		ActorUserID: user.ID, RequestID: in.RequestID, NewRecoveryCodeID: s.NewID(), NewRecoveryCodeHash: codeHash,
		NewCredentialID: s.NewID(), NewPasswordHash: phc,
	})
	if err != nil {
		if errors.Is(err, inbound.ErrRecoveryNotRequired) || errors.Is(err, outbound.ErrRecoveryChanged) {
			return inbound.ReconcileResult{}, inbound.ErrConflictState
		}
		return inbound.ReconcileResult{}, s.mapStoreError(err)
	}
	if err := s.Attempts.Settle(ctx, outbound.AttemptReauthenticate, in.ClientKey, user.ID); err != nil {
		s.Logger.Warn("auth: the reconciliation attempt could not be settled")
	}
	return inbound.ReconcileResult{
		Generation: counts.Generation, RevokedSessions: counts.SessionsRevoked, RevokedApprovals: counts.ApprovalsRevoked + counts.ApprovalsRejected,
		StaleValidations: counts.ValidationsInvalidated, RevokedLabs: counts.LabsRevoked, CancelledJobs: counts.JobsCancelled, RecoveryCode: shown,
	}, nil
}
