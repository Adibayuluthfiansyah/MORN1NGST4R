package auth

import (
	"context"
	"strings"
	"unicode"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

type principal struct {
	userID, sessionID string
	csrfHash          []byte
	act               actor.Actor
}

func (p *principal) UserID() string                  { return p.userID }
func (p *principal) SessionID() string               { return p.sessionID }
func (p *principal) Actor() actor.Actor              { return p.act }
func (p *principal) ValidCSRF(presented string) bool { return csrfMatches(presented, p.csrfHash) }

// Authenticate resolves a session cookie. It is the only place that builds the
// human actor: every Principal in the process comes from here.
func (s *Service) Authenticate(ctx context.Context, sessionToken string) (inbound.Principal, error) {
	rec, _, err := s.lookup(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if err := s.Sessions.Touch(ctx, rec.ID, identity.SessionIdleTTL, identity.SessionTouchEvery); err != nil {
		return nil, s.mapStoreError(err)
	}
	human, err := actor.New(actor.KindHuman, rec.UserID)
	if err != nil {
		return nil, inbound.ErrUnauthenticated
	}
	return &principal{userID: rec.UserID, sessionID: rec.ID, csrfHash: rec.CSRFHash, act: human}, nil
}

func (s *Service) lookup(ctx context.Context, sessionToken string) (outbound.SessionRecord, []byte, error) {
	raw, ok := decodeToken(sessionToken)
	if !ok {
		return outbound.SessionRecord{}, nil, inbound.ErrUnauthenticated
	}
	rec, found, err := s.Sessions.Lookup(ctx, tokenHash(raw))
	if err != nil {
		return outbound.SessionRecord{}, nil, s.mapStoreError(err)
	}
	if !found {
		return outbound.SessionRecord{}, nil, inbound.ErrUnauthenticated
	}
	if rec.Revoked || rec.UserDisabled || !rec.Now.Before(rec.IdleExpiresAt) || !rec.Now.Before(rec.AbsoluteExpiresAt) {
		return outbound.SessionRecord{}, nil, inbound.ErrSessionExpired
	}
	return rec, raw, nil
}

// issueSession creates a session with a new token. It always issues a new
// token, so a token an attacker planted before the login is never reused.
func (s *Service) issueSession(ctx context.Context, userID, userAgent string) (inbound.IssuedSession, string, error) {
	token, raw := newToken(s.Random)
	csrf, csrfHash := csrfFor(raw)
	id := s.NewID()
	if err := s.Sessions.Insert(ctx, outbound.NewSession{
		ID: id, UserID: userID, TokenHash: tokenHash(raw), CSRFHash: csrfHash,
		IdleTTL: identity.SessionIdleTTL, AbsoluteTTL: identity.SessionAbsoluteTTL, UserAgent: storableText(userAgent, 200),
	}); err != nil {
		return inbound.IssuedSession{}, "", err
	}
	return inbound.IssuedSession{Token: token, CSRFToken: csrf, ExpiresAt: s.Now().Add(identity.SessionAbsoluteTTL)}, id, nil
}

// storableText makes client input safe to store and show: valid UTF-8, no
// control characters, at most n bytes and never cut inside a rune.
func storableText(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}

func (s *Service) SessionState(ctx context.Context, c inbound.Cookies) (inbound.SessionState, error) {
	st := s.Recovery.State()
	anon := func(state inbound.StateKind) inbound.SessionState {
		out := inbound.SessionState{Kind: state}
		payload, ok := s.parseAnon(c.Anon)
		if ok {
			out.CSRFToken = s.anonCSRF(payload)
			return out
		}
		out.NewAnonToken = s.newAnonToken()
		payload, _ = s.parseAnon(out.NewAnonToken)
		out.CSRFToken = s.anonCSRF(payload)
		return out
	}
	if st.Required {
		out := anon(inbound.StateRecoveryRequired)
		out.Recovery = &inbound.RecoveryInfo{Reason: st.Reason, MarkerGeneration: st.MarkerGeneration, DatabaseGeneration: st.DatabaseGeneration, DetectedAt: st.DetectedAt}
		return out, nil
	}
	if !st.Normal {
		return inbound.SessionState{}, inbound.ErrDatabaseUnavailable
	}
	n, err := s.Accounts.CountUsers(ctx)
	if err != nil {
		return inbound.SessionState{}, s.mapStoreError(err)
	}
	if n == 0 {
		return anon(inbound.StateSetupRequired), nil
	}
	if c.Session != "" {
		rec, raw, err := s.lookup(ctx, c.Session)
		switch {
		case err == nil:
			u, found, err := s.Accounts.UserByID(ctx, rec.UserID)
			if err != nil {
				return inbound.SessionState{}, s.mapStoreError(err)
			}
			if found {
				csrf, _ := csrfFor(raw)
				return inbound.SessionState{
					Kind: inbound.StateAuthenticated, CSRFToken: csrf, SessionID: rec.ID,
					User:          &inbound.UserSummary{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName},
					IdleExpiresAt: rec.IdleExpiresAt, AbsoluteExpiresAt: rec.AbsoluteExpiresAt,
				}, nil
			}
		case err == inbound.ErrDatabaseUnavailable:
			return inbound.SessionState{}, err
		}
	}
	return anon(inbound.StateAnonymous), nil
}

func (s *Service) Logout(ctx context.Context, p inbound.Principal, requestID string) error {
	return s.mapStoreError(s.Tx.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.Sessions.Revoke(ctx, p.SessionID(), p.UserID(), "logout"); err != nil {
			return err
		}
		return s.auditUser(ctx, "auth.logout", p.UserID(), requestID, map[string]any{"sessionId": p.SessionID()})
	}))
}

func view(r outbound.SessionRecord, current string) inbound.SessionView {
	return inbound.SessionView{ID: r.ID, CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt, IdleExpiresAt: r.IdleExpiresAt,
		AbsoluteExpiresAt: r.AbsoluteExpiresAt, Current: r.ID == current, UserAgent: r.UserAgent}
}

func (s *Service) ListSessions(ctx context.Context, p inbound.Principal, limit int) ([]inbound.SessionView, error) {
	if limit < 1 || limit > 200 {
		return nil, validationError(inbound.FieldError{Field: "limit", Code: "out_of_range"})
	}
	recs, err := s.Sessions.ListLive(ctx, p.UserID(), limit)
	if err != nil {
		return nil, s.mapStoreError(err)
	}
	out := make([]inbound.SessionView, 0, len(recs))
	for _, r := range recs {
		out = append(out, view(r, p.SessionID()))
	}
	return out, nil
}

// RevokeSession revokes one session of the caller. A session of another user
// and a session that does not exist give the same answer.
func (s *Service) RevokeSession(ctx context.Context, p inbound.Principal, sessionID, requestID string) (inbound.SessionView, error) {
	var out inbound.SessionView
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		rec, found, err := s.Sessions.Find(ctx, sessionID, p.UserID())
		if err != nil {
			return err
		}
		if !found {
			return inbound.ErrNotFound
		}
		revoked, err := s.Sessions.Revoke(ctx, sessionID, p.UserID(), "user_revoked")
		if err != nil {
			return err
		}
		if !revoked {
			return inbound.ErrConflictState
		}
		out = view(rec, p.SessionID())
		return s.auditUser(ctx, "auth.session_revoked", p.UserID(), requestID, map[string]any{"sessionId": sessionID})
	})
	return out, s.mapStoreError(err)
}
