package auth

import (
	"context"
	"errors"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// verifyPassword checks a password for user, or spends the same work for an
// unknown or disabled user. The caller has already reserved an attempt.
func (s *Service) verifyPassword(ctx context.Context, user outbound.User, known bool, password string) (bool, error) {
	if !known || user.Disabled {
		return false, s.Hasher.SpendVerifyCost(ctx, password)
	}
	phc, found, err := s.Accounts.CurrentPasswordHash(ctx, user.ID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, s.Hasher.SpendVerifyCost(ctx, password)
	}
	return s.Hasher.Verify(ctx, password, phc)
}

func (s *Service) Login(ctx context.Context, in inbound.LoginInput) (inbound.LoginResult, error) {
	if in.Username == "" || identity.ValidatePasswordInput(in.Password) != nil {
		return inbound.LoginResult{}, validationError(inbound.FieldError{Field: "username", Code: "required"}, inbound.FieldError{Field: "password", Code: "required"})
	}
	n, err := s.Accounts.CountUsers(ctx)
	if err != nil {
		return inbound.LoginResult{}, s.mapStoreError(err)
	}
	if n == 0 {
		return inbound.LoginResult{}, inbound.ErrSetupUnavailable
	}
	var user outbound.User
	var known bool
	if username, err := identity.NormalizeUsername(in.Username); err == nil {
		if user, known, err = s.Accounts.UserByUsername(ctx, username); err != nil {
			return inbound.LoginResult{}, s.mapStoreError(err)
		}
	}
	userID := ""
	if known {
		userID = user.ID
	}
	allowed, err := s.Attempts.Reserve(ctx, outbound.AttemptLogin, in.ClientKey, userID, attemptWindow, identity.MaxFailedInWindow)
	if err != nil {
		return inbound.LoginResult{}, s.mapStoreError(err)
	}
	if !allowed {
		s.auditDenied(ctx, "auth.login", in.RequestID, map[string]any{"reason": "rate_limited"})
		return inbound.LoginResult{}, rateLimited()
	}
	ok, err := s.verifyPassword(ctx, user, known, in.Password)
	if err != nil {
		return inbound.LoginResult{}, err
	}
	if !ok {
		s.auditDenied(ctx, "auth.login", in.RequestID, map[string]any{"reason": "invalid_credentials"})
		return inbound.LoginResult{}, inbound.ErrInvalidCredentials
	}

	var out inbound.LoginResult
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		session, sessionID, err := s.issueSession(ctx, user.ID, in.UserAgent)
		if err != nil {
			return err
		}
		if err := s.Attempts.Settle(ctx, outbound.AttemptLogin, in.ClientKey, user.ID); err != nil {
			return err
		}
		if err := s.auditUser(ctx, "auth.login", user.ID, in.RequestID, map[string]any{"sessionId": sessionID}); err != nil {
			return err
		}
		summary := inbound.UserSummary{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName}
		out = inbound.LoginResult{User: summary, Session: session, State: inbound.SessionState{
			Kind: inbound.StateAuthenticated, CSRFToken: session.CSRFToken, User: &summary, SessionID: sessionID,
			IdleExpiresAt: s.Now().Add(identity.SessionIdleTTL), AbsoluteExpiresAt: session.ExpiresAt,
		}}
		return nil
	})
	return out, s.mapStoreError(err)
}

// Reauthenticate turns a fresh password entry into a single-use grant for one
// exact action (AUTH-07, D-69). It always asks for the password: there is no
// reusable window.
func (s *Service) Reauthenticate(ctx context.Context, p inbound.Principal, in inbound.ReauthInput) (inbound.ReauthGrant, error) {
	purpose, ok := identity.ParsePurpose(in.Purpose)
	if !ok || identity.ValidatePasswordInput(in.Password) != nil {
		return inbound.ReauthGrant{}, validationError(inbound.FieldError{Field: "purpose", Code: "invalid"})
	}
	subject, err := identity.SubjectHash(purpose, in.Subject)
	if err != nil {
		return inbound.ReauthGrant{}, inbound.ErrReauthSubjectInvalid
	}
	user, found, err := s.Accounts.UserByID(ctx, p.UserID())
	if err != nil {
		return inbound.ReauthGrant{}, s.mapStoreError(err)
	}
	allowed, err := s.Attempts.Reserve(ctx, outbound.AttemptReauthenticate, in.ClientKey, p.UserID(), attemptWindow, identity.MaxFailedInWindow)
	if err != nil {
		return inbound.ReauthGrant{}, s.mapStoreError(err)
	}
	if !allowed {
		s.auditDenied(ctx, "auth.reauthenticated", in.RequestID, map[string]any{"reason": "rate_limited"})
		return inbound.ReauthGrant{}, rateLimited()
	}
	match, err := s.verifyPassword(ctx, user, found, in.Password)
	if err != nil {
		return inbound.ReauthGrant{}, err
	}
	if !match {
		s.auditDenied(ctx, "auth.reauthenticated", in.RequestID, map[string]any{"reason": "invalid_credentials"})
		return inbound.ReauthGrant{}, inbound.ErrInvalidCredentials
	}
	var out inbound.ReauthGrant
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		row, err := s.Reauth.Insert(ctx, outbound.NewGrant{
			ID: s.NewID(), UserID: p.UserID(), SessionID: p.SessionID(), Purpose: string(purpose), SubjectHash: subject[:], TTL: identity.ReauthGrantTTL,
		})
		if err != nil {
			return err
		}
		if err := s.Attempts.Settle(ctx, outbound.AttemptReauthenticate, in.ClientKey, p.UserID()); err != nil {
			return err
		}
		out = inbound.ReauthGrant{ID: row.ID, Purpose: row.Purpose, ExpiresAt: row.ExpiresAt}
		return s.auditUser(ctx, "auth.reauthenticated", p.UserID(), in.RequestID, map[string]any{"purpose": string(purpose)})
	})
	return out, s.mapStoreError(err)
}

// Consume implements inbound.Reauth for the use cases that guard an action. It
// runs in the caller's transaction, so the grant is spent only when the guarded
// change commits.
func (s *Service) Consume(ctx context.Context, p inbound.Principal, use inbound.ReauthUse) error {
	if use.GrantID == "" {
		return inbound.ErrReauthRequired
	}
	if _, ok := identity.ParsePurpose(string(use.Purpose)); !ok {
		return inbound.ErrReauthInvalid
	}
	ok, err := s.Reauth.Consume(ctx, use.GrantID, p.SessionID(), string(use.Purpose), use.SubjectHash[:])
	if err != nil {
		return s.mapStoreError(err)
	}
	if !ok {
		return inbound.ErrReauthInvalid
	}
	return nil
}

// ChangePassword replaces the password (AUTH-09). It needs the current password
// and a grant of purpose password_change. Every session of the account,
// including this one, is revoked and a new session is issued.
func (s *Service) ChangePassword(ctx context.Context, p inbound.Principal, in inbound.ChangePasswordInput) (inbound.IssuedSession, error) {
	user, found, err := s.Accounts.UserByID(ctx, p.UserID())
	if err != nil {
		return inbound.IssuedSession{}, s.mapStoreError(err)
	}
	if !found {
		return inbound.IssuedSession{}, inbound.ErrUnauthenticated
	}
	if identity.ValidateNewPassword(user.Username, in.NewPassword) != nil || identity.ValidatePasswordInput(in.CurrentPassword) != nil {
		return inbound.IssuedSession{}, validationError(inbound.FieldError{Field: "newPassword", Code: "invalid"})
	}
	if in.ReauthGrantID == "" {
		return inbound.IssuedSession{}, inbound.ErrReauthRequired
	}
	allowed, err := s.Attempts.Reserve(ctx, outbound.AttemptReauthenticate, in.ClientKey, p.UserID(), attemptWindow, identity.MaxFailedInWindow)
	if err != nil {
		return inbound.IssuedSession{}, s.mapStoreError(err)
	}
	if !allowed {
		s.auditDenied(ctx, "auth.password_changed", in.RequestID, map[string]any{"reason": "rate_limited"})
		return inbound.IssuedSession{}, rateLimited()
	}
	match, err := s.verifyPassword(ctx, user, true, in.CurrentPassword)
	if err != nil {
		return inbound.IssuedSession{}, err
	}
	if !match {
		s.auditDenied(ctx, "auth.password_changed", in.RequestID, map[string]any{"reason": "invalid_credentials"})
		return inbound.IssuedSession{}, inbound.ErrInvalidCredentials
	}
	phc, err := s.Hasher.Hash(ctx, in.NewPassword)
	if err != nil {
		return inbound.IssuedSession{}, err
	}
	fixed, err := identity.SubjectHash(identity.PurposePasswordChange, map[string]any{})
	if err != nil {
		return inbound.IssuedSession{}, err
	}
	var out inbound.IssuedSession
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Consume(ctx, p, inbound.ReauthUse{GrantID: in.ReauthGrantID, Purpose: identity.PurposePasswordChange, SubjectHash: fixed}); err != nil {
			return err
		}
		if err := s.Accounts.SetPassword(ctx, p.UserID(), s.NewID(), phc); err != nil {
			return err
		}
		if _, err := s.Sessions.RevokeAllExcept(ctx, p.UserID(), "", "password_changed"); err != nil {
			return err
		}
		if out, _, err = s.issueSession(ctx, p.UserID(), in.UserAgent); err != nil {
			return err
		}
		if err := s.Attempts.Settle(ctx, outbound.AttemptReauthenticate, in.ClientKey, p.UserID()); err != nil {
			return err
		}
		return s.auditUser(ctx, "auth.password_changed", p.UserID(), in.RequestID, nil)
	})
	if errors.Is(err, inbound.ErrReauthInvalid) {
		s.auditDenied(ctx, "auth.password_changed", in.RequestID, map[string]any{"reason": "reauth_invalid"})
	}
	return out, s.mapStoreError(err)
}
