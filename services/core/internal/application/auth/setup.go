package auth

import (
	"context"
	"errors"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

func (s *Service) newRecoveryCode(ctx context.Context) (shown, hash string, err error) {
	shown, canonical, err := identity.FormatRecoveryCode(s.Random(identity.RecoveryCodeBytes))
	if err != nil {
		return "", "", err
	}
	hash, err = s.Hasher.Hash(ctx, canonical)
	return shown, hash, err
}

// Setup creates the one account with the bootstrap secret (AUTH-02). The cheap
// secret check runs before any password hashing, so an attacker without the
// secret cannot make the server spend 64 MiB per request.
func (s *Service) Setup(ctx context.Context, in inbound.SetupInput) (inbound.SetupResult, error) {
	username, uerr := identity.NormalizeUsername(in.Username)
	var fields []inbound.FieldError
	if uerr != nil {
		fields = append(fields, inbound.FieldError{Field: "username", Code: "invalid"})
	}
	if identity.ValidateNewPassword(username, in.Password) != nil {
		fields = append(fields, inbound.FieldError{Field: "password", Code: "invalid"})
	}
	if !validSecretShape(in.BootstrapSecret) {
		fields = append(fields, inbound.FieldError{Field: "bootstrapSecret", Code: "invalid"})
	}
	if len(fields) > 0 {
		return inbound.SetupResult{}, validationError(fields...)
	}

	n, err := s.Accounts.CountUsers(ctx)
	if err != nil {
		return inbound.SetupResult{}, s.mapStoreError(err)
	}
	if n > 0 {
		return inbound.SetupResult{}, inbound.ErrAccountExists
	}
	allowed, err := s.Attempts.Reserve(ctx, outbound.AttemptBootstrap, in.ClientKey, "", attemptWindow, identity.MaxFailedInWindow)
	if err != nil {
		return inbound.SetupResult{}, s.mapStoreError(err)
	}
	if !allowed {
		s.auditDenied(ctx, "auth.account_created", in.RequestID, map[string]any{"reason": "rate_limited"})
		return inbound.SetupResult{}, rateLimited()
	}

	live, found, err := s.Bootstraps.LockLive(ctx)
	if err != nil {
		return inbound.SetupResult{}, s.mapStoreError(err)
	}
	if !found {
		return inbound.SetupResult{}, inbound.ErrSetupUnavailable
	}
	if !secretMatches(in.BootstrapSecret, live.Hash) {
		s.auditDenied(ctx, "auth.account_created", in.RequestID, map[string]any{"reason": "bootstrap_secret_invalid"})
		return inbound.SetupResult{}, inbound.ErrBootstrapSecretInvalid
	}

	phc, err := s.Hasher.Hash(ctx, in.Password)
	if err != nil {
		return inbound.SetupResult{}, err
	}
	shown, codeHash, err := s.newRecoveryCode(ctx)
	if err != nil {
		return inbound.SetupResult{}, err
	}

	var out inbound.SetupResult
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		locked, found, err := s.Bootstraps.LockLive(ctx)
		if err != nil {
			return err
		}
		if !found || locked.ID != live.ID || !secretMatches(in.BootstrapSecret, locked.Hash) {
			return inbound.ErrConflictState
		}
		if count, err := s.Accounts.CountUsers(ctx); err != nil {
			return err
		} else if count > 0 {
			return inbound.ErrAccountExists
		}
		userID := s.NewID()
		if err := s.Accounts.InsertUser(ctx, userID, username); err != nil {
			return err
		}
		if err := s.Accounts.SetPassword(ctx, userID, s.NewID(), phc); err != nil {
			return err
		}
		if err := s.RecoveryCodes.Insert(ctx, s.NewID(), userID, codeHash); err != nil {
			return err
		}
		if err := s.Bootstraps.Consume(ctx, locked.ID, userID); err != nil {
			return err
		}
		session, _, err := s.issueSession(ctx, userID, in.UserAgent)
		if err != nil {
			return err
		}
		if err := s.Attempts.Settle(ctx, outbound.AttemptBootstrap, in.ClientKey, userID); err != nil {
			return err
		}
		if err := s.auditUser(ctx, "auth.account_created", userID, in.RequestID, nil); err != nil {
			return err
		}
		out = inbound.SetupResult{User: inbound.UserSummary{ID: userID, Username: username}, RecoveryCode: shown, Session: session}
		return nil
	})
	if err != nil {
		if errors.Is(err, outbound.ErrStoreUnavailable) {
			return inbound.SetupResult{}, inbound.ErrDatabaseUnavailable
		}
		return inbound.SetupResult{}, s.uniqueViolationAsExists(err)
	}
	if err := s.SecretFile.Remove(); err != nil {
		s.Logger.Warn("auth: the bootstrap secret file could not be removed")
	}
	return out, nil
}

// uniqueViolationAsExists turns the loss of a race for the single account into
// the ordinary "account exists" answer.
func (s *Service) uniqueViolationAsExists(err error) error {
	if errors.Is(err, outbound.ErrUniqueViolation) {
		return inbound.ErrAccountExists
	}
	return err
}
