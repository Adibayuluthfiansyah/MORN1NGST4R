package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence/sqlcgen"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// The stores below join the transaction of their context. Attempts.Reserve is
// the exception: it always commits on its own.
type base struct{ pool *pgxpool.Pool }

func (b base) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(Querier(ctx, b.pool)) }

type (
	Accounts      struct{ base }
	Bootstraps    struct{ base }
	RecoveryCodes struct{ base }
	Sessions      struct{ base }
	Reauth        struct{ base }
	Audit         struct{ base }
	Attempts      struct {
		base
		tx *Transactor
	}
)

var (
	_ outbound.AccountStore      = Accounts{}
	_ outbound.BootstrapStore    = Bootstraps{}
	_ outbound.RecoveryCodeStore = RecoveryCodes{}
	_ outbound.SessionStore      = Sessions{}
	_ outbound.ReauthStore       = Reauth{}
	_ outbound.AttemptStore      = Attempts{}
	_ outbound.AuditLog          = Audit{}
)

func (d *DB) Accounts() outbound.AccountStore           { return Accounts{base{d.pool}} }
func (d *DB) Bootstraps() outbound.BootstrapStore       { return Bootstraps{base{d.pool}} }
func (d *DB) RecoveryCodes() outbound.RecoveryCodeStore { return RecoveryCodes{base{d.pool}} }
func (d *DB) Sessions() outbound.SessionStore           { return Sessions{base{d.pool}} }
func (d *DB) Reauth() outbound.ReauthStore              { return Reauth{base{d.pool}} }
func (d *DB) Audit() outbound.AuditLog                  { return Audit{base{d.pool}} }
func (d *DB) Attempts() outbound.AttemptStore {
	return Attempts{base{d.pool}, NewTransactor(d.pool)}
}

func uuidOf(s string) (pgtype.UUID, error) {
	if !validUUID(s) {
		return pgtype.UUID{}, errors.New("persistence: not a UUID")
	}
	return toUUID(s), nil
}

func uuidOrNull(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return uuidOf(s)
}

func wrap(what string, err error) error {
	if err == nil {
		return nil
	}
	if unavailable(err) {
		return fmt.Errorf("persistence: %s: %w", what, outbound.ErrStoreUnavailable)
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return fmt.Errorf("persistence: %s: %w", what, outbound.ErrUniqueViolation)
	}
	return fmt.Errorf("persistence: %s: %w", what, err)
}

func unavailable(err error) bool {
	var ce *pgconn.ConnectError
	var pe *pgconn.PgError
	var ne net.Error
	switch {
	case errors.As(err, &ce), errors.Is(err, context.DeadlineExceeded), pgconn.Timeout(err):
		return true
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, net.ErrClosed), errors.As(err, &ne):
		return true
	case errors.As(err, &pe):
		return strings.HasPrefix(pe.Code, "08") || strings.HasPrefix(pe.Code, "53") || strings.HasPrefix(pe.Code, "57P")
	}
	return false
}

func noRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func toUser(id, username, display string, disabled bool) outbound.User {
	return outbound.User{ID: id, Username: username, DisplayName: display, Disabled: disabled}
}

func (a Accounts) CountUsers(ctx context.Context) (int, error) {
	n, err := a.q(ctx).CountUsers(ctx)
	return int(n), wrap("count users", err)
}

func (a Accounts) SingleUser(ctx context.Context) (outbound.User, bool, error) {
	r, err := a.q(ctx).SingleUser(ctx)
	if noRows(err) {
		return outbound.User{}, false, nil
	}
	if err != nil {
		return outbound.User{}, false, wrap("read the account", err)
	}
	return toUser(r.ID, r.Username, r.DisplayName, r.Disabled), true, nil
}

func (a Accounts) UserByUsername(ctx context.Context, username string) (outbound.User, bool, error) {
	r, err := a.q(ctx).UserByUsername(ctx, username)
	if noRows(err) {
		return outbound.User{}, false, nil
	}
	if err != nil {
		return outbound.User{}, false, wrap("read the account", err)
	}
	return toUser(r.ID, r.Username, r.DisplayName, r.Disabled), true, nil
}

func (a Accounts) UserByID(ctx context.Context, id string) (outbound.User, bool, error) {
	u, err := uuidOf(id)
	if err != nil {
		return outbound.User{}, false, nil
	}
	r, err := a.q(ctx).UserByID(ctx, u)
	if noRows(err) {
		return outbound.User{}, false, nil
	}
	if err != nil {
		return outbound.User{}, false, wrap("read the account", err)
	}
	return toUser(r.ID, r.Username, r.DisplayName, r.Disabled), true, nil
}

func (a Accounts) InsertUser(ctx context.Context, id, username string) error {
	u, err := uuidOf(id)
	if err != nil {
		return err
	}
	return wrap("insert the account", a.q(ctx).InsertUser(ctx, sqlcgen.InsertUserParams{Column1: u, Username: username}))
}

func (a Accounts) CurrentPasswordHash(ctx context.Context, userID string) (string, bool, error) {
	u, err := uuidOf(userID)
	if err != nil {
		return "", false, nil
	}
	h, err := a.q(ctx).CurrentPasswordHash(ctx, u)
	if noRows(err) {
		return "", false, nil
	}
	return h, err == nil, wrap("read the credential", err)
}

func (a Accounts) SetPassword(ctx context.Context, userID, credentialID, phc string) error {
	u, err := uuidOf(userID)
	if err != nil {
		return err
	}
	c, err := uuidOf(credentialID)
	if err != nil {
		return err
	}
	q := a.q(ctx)
	if err := q.SupersedeCredentials(ctx, u); err != nil {
		return wrap("supersede the credential", err)
	}
	if err := q.InsertCredential(ctx, sqlcgen.InsertCredentialParams{Column1: c, Column2: u, PhcHash: phc}); err != nil {
		return wrap("insert the credential", err)
	}
	return wrap("record the password change", q.TouchPasswordChanged(ctx, u))
}

func (b Bootstraps) LockLive(ctx context.Context) (outbound.BootstrapSecretRow, bool, error) {
	r, err := b.q(ctx).LockLiveBootstrapSecret(ctx)
	if noRows(err) {
		return outbound.BootstrapSecretRow{}, false, nil
	}
	if err != nil {
		return outbound.BootstrapSecretRow{}, false, wrap("lock the bootstrap secret", err)
	}
	return outbound.BootstrapSecretRow{ID: r.ID, Hash: r.SecretHash}, true, nil
}

func (b Bootstraps) Supersede(ctx context.Context) (int, error) {
	n, err := b.q(ctx).SupersedeBootstrapSecrets(ctx)
	return int(n), wrap("supersede bootstrap secrets", err)
}

func (b Bootstraps) Insert(ctx context.Context, id, hash string) error {
	u, err := uuidOf(id)
	if err != nil {
		return err
	}
	return wrap("insert the bootstrap secret", b.q(ctx).InsertBootstrapSecret(ctx, sqlcgen.InsertBootstrapSecretParams{Column1: u, SecretHash: hash}))
}

func (b Bootstraps) Consume(ctx context.Context, id, userID string) error {
	s, err := uuidOf(id)
	if err != nil {
		return err
	}
	u, err := uuidOf(userID)
	if err != nil {
		return err
	}
	n, err := b.q(ctx).ConsumeBootstrapSecret(ctx, sqlcgen.ConsumeBootstrapSecretParams{Column1: s, Column2: u})
	if err != nil {
		return wrap("consume the bootstrap secret", err)
	}
	if n != 1 {
		return errors.New("persistence: the bootstrap secret was not live")
	}
	return nil
}

func (r RecoveryCodes) LockLive(ctx context.Context, userID string) (outbound.RecoveryCodeRow, bool, error) {
	u, err := uuidOf(userID)
	if err != nil {
		return outbound.RecoveryCodeRow{}, false, nil
	}
	row, err := r.q(ctx).LockLiveRecoveryCode(ctx, u)
	if noRows(err) {
		return outbound.RecoveryCodeRow{}, false, nil
	}
	if err != nil {
		return outbound.RecoveryCodeRow{}, false, wrap("lock the recovery code", err)
	}
	return outbound.RecoveryCodeRow{ID: row.ID, Hash: row.CodeHash}, true, nil
}

func (r RecoveryCodes) Insert(ctx context.Context, id, userID, hash string) error {
	i, err := uuidOf(id)
	if err != nil {
		return err
	}
	u, err := uuidOf(userID)
	if err != nil {
		return err
	}
	return wrap("insert the recovery code", r.q(ctx).InsertRecoveryCode(ctx, sqlcgen.InsertRecoveryCodeParams{Column1: i, Column2: u, CodeHash: hash}))
}

func (r RecoveryCodes) MarkUsed(ctx context.Context, id string) error {
	i, err := uuidOf(id)
	if err != nil {
		return err
	}
	n, err := r.q(ctx).MarkRecoveryCodeUsed(ctx, i)
	if err != nil {
		return wrap("use the recovery code", err)
	}
	if n != 1 {
		return outbound.ErrNotLive
	}
	return nil
}

func (r RecoveryCodes) RevokeLive(ctx context.Context, userID string) error {
	u, err := uuidOf(userID)
	if err != nil {
		return err
	}
	return wrap("revoke recovery codes", r.q(ctx).RevokeLiveRecoveryCodes(ctx, u))
}

func (s Sessions) Insert(ctx context.Context, n outbound.NewSession) error {
	id, err := uuidOf(n.ID)
	if err != nil {
		return err
	}
	u, err := uuidOf(n.UserID)
	if err != nil {
		return err
	}
	var agent *string
	if n.UserAgent != "" {
		agent = &n.UserAgent
	}
	return wrap("insert the session", s.q(ctx).InsertSession(ctx, sqlcgen.InsertSessionParams{
		Column1: id, Column2: u, TokenHash: n.TokenHash, CsrfTokenHash: n.CSRFHash,
		IdleSecs: n.IdleTTL.Seconds(), AbsoluteSecs: n.AbsoluteTTL.Seconds(), UserAgent: agent,
	}))
}

func ts(t pgtype.Timestamptz) time.Time { return t.Time }

func (s Sessions) Lookup(ctx context.Context, tokenHash []byte) (outbound.SessionRecord, bool, error) {
	r, err := s.q(ctx).LookupSession(ctx, tokenHash)
	if noRows(err) {
		return outbound.SessionRecord{}, false, nil
	}
	if err != nil {
		return outbound.SessionRecord{}, false, wrap("read the session", err)
	}
	return outbound.SessionRecord{
		ID: r.ID, UserID: r.UserID, CSRFHash: r.CsrfTokenHash, CreatedAt: ts(r.CreatedAt), LastSeenAt: ts(r.LastSeenAt),
		IdleExpiresAt: ts(r.IdleExpiresAt), AbsoluteExpiresAt: ts(r.AbsoluteExpiresAt), Revoked: r.Revoked,
		UserAgent: r.UserAgent, UserDisabled: r.UserDisabled, Now: ts(r.DbNow),
	}, true, nil
}

func (s Sessions) Touch(ctx context.Context, id string, idleTTL, every time.Duration) error {
	u, err := uuidOf(id)
	if err != nil {
		return err
	}
	return wrap("touch the session", s.q(ctx).TouchSession(ctx, sqlcgen.TouchSessionParams{Column1: u, IdleSecs: idleTTL.Seconds(), EverySecs: every.Seconds()}))
}

func (s Sessions) Revoke(ctx context.Context, id, userID, reason string) (bool, error) {
	i, err := uuidOf(id)
	if err != nil {
		return false, nil
	}
	u, err := uuidOf(userID)
	if err != nil {
		return false, nil
	}
	n, err := s.q(ctx).RevokeSession(ctx, sqlcgen.RevokeSessionParams{Column1: i, Column2: u, RevokedReason: &reason})
	return n == 1, wrap("revoke the session", err)
}

func (s Sessions) RevokeAllExcept(ctx context.Context, userID, exceptID, reason string) (int, error) {
	u, err := uuidOf(userID)
	if err != nil {
		return 0, err
	}
	except, err := uuidOrNull(exceptID)
	if err != nil {
		return 0, err
	}
	n, err := s.q(ctx).RevokeSessionsExcept(ctx, sqlcgen.RevokeSessionsExceptParams{Reason: reason, UserID: u, ExceptID: except})
	return int(n), wrap("revoke sessions", err)
}

func (s Sessions) Find(ctx context.Context, id, userID string) (outbound.SessionRecord, bool, error) {
	i, err := uuidOf(id)
	if err != nil {
		return outbound.SessionRecord{}, false, nil
	}
	u, err := uuidOf(userID)
	if err != nil {
		return outbound.SessionRecord{}, false, nil
	}
	r, err := s.q(ctx).FindSession(ctx, sqlcgen.FindSessionParams{Column1: i, Column2: u})
	if noRows(err) {
		return outbound.SessionRecord{}, false, nil
	}
	if err != nil {
		return outbound.SessionRecord{}, false, wrap("read the session", err)
	}
	return outbound.SessionRecord{
		ID: r.ID, UserID: r.UserID, CSRFHash: r.CsrfTokenHash, CreatedAt: ts(r.CreatedAt), LastSeenAt: ts(r.LastSeenAt),
		IdleExpiresAt: ts(r.IdleExpiresAt), AbsoluteExpiresAt: ts(r.AbsoluteExpiresAt), Revoked: r.Revoked,
		UserAgent: r.UserAgent, Now: ts(r.DbNow),
	}, true, nil
}

func (s Sessions) ListLive(ctx context.Context, userID string, limit int) ([]outbound.SessionRecord, error) {
	u, err := uuidOf(userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q(ctx).ListLiveSessions(ctx, sqlcgen.ListLiveSessionsParams{Column1: u, Limit: int32(limit)})
	if err != nil {
		return nil, wrap("list sessions", err)
	}
	out := make([]outbound.SessionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, outbound.SessionRecord{
			ID: r.ID, UserID: r.UserID, CSRFHash: r.CsrfTokenHash, CreatedAt: ts(r.CreatedAt), LastSeenAt: ts(r.LastSeenAt),
			IdleExpiresAt: ts(r.IdleExpiresAt), AbsoluteExpiresAt: ts(r.AbsoluteExpiresAt), Revoked: r.Revoked,
			UserAgent: r.UserAgent, Now: ts(r.DbNow),
		})
	}
	return out, nil
}

func (r Reauth) Insert(ctx context.Context, g outbound.NewGrant) (outbound.GrantRow, error) {
	id, err := uuidOf(g.ID)
	if err != nil {
		return outbound.GrantRow{}, err
	}
	u, err := uuidOf(g.UserID)
	if err != nil {
		return outbound.GrantRow{}, err
	}
	s, err := uuidOf(g.SessionID)
	if err != nil {
		return outbound.GrantRow{}, err
	}
	row, err := r.q(ctx).InsertReauthGrant(ctx, sqlcgen.InsertReauthGrantParams{
		Column1: id, Column2: u, Column3: s, Purpose: g.Purpose, SubjectHash: g.SubjectHash, TtlSecs: g.TTL.Seconds(),
	})
	if err != nil {
		return outbound.GrantRow{}, wrap("insert the grant", err)
	}
	return outbound.GrantRow{ID: row.ID, Purpose: row.Purpose, ExpiresAt: ts(row.ExpiresAt)}, nil
}

func (r Reauth) Consume(ctx context.Context, id, sessionID, purpose string, subjectHash []byte) (bool, error) {
	g, err := uuidOf(id)
	if err != nil {
		return false, nil
	}
	s, err := uuidOf(sessionID)
	if err != nil {
		return false, nil
	}
	n, err := r.q(ctx).ConsumeReauthGrant(ctx, sqlcgen.ConsumeReauthGrantParams{Column1: g, Column2: s, Purpose: purpose, SubjectHash: subjectHash})
	return n == 1, wrap("consume the grant", err)
}

func (a Attempts) Reserve(ctx context.Context, kind outbound.AttemptKind, sourceKey, userID string, window time.Duration, max int) (bool, error) {
	uid, err := uuidOrNull(userID)
	if err != nil {
		return false, err
	}
	allowed := false
	kinds := make([]string, 0, 2)
	for _, k := range kind.Budget() {
		kinds = append(kinds, string(k))
	}
	err = a.tx.InTx(withoutTx(ctx), func(ctx context.Context) error {
		q := a.q(ctx)
		if err := q.LockLoginAttempts(ctx); err != nil {
			return err
		}
		n, err := q.CountEffectiveFailures(ctx, sqlcgen.CountEffectiveFailuresParams{WindowSecs: window.Seconds(), Kinds: kinds, SourceKey: sourceKey, UserID: uid})
		if err != nil {
			return err
		}
		if int(n) >= max {
			return nil
		}
		allowed = true
		return a.insert(ctx, kind, sourceKey, uid, false)
	})
	return allowed, wrap("reserve an attempt", err)
}

func (a Attempts) Settle(ctx context.Context, kind outbound.AttemptKind, sourceKey, userID string) error {
	uid, err := uuidOrNull(userID)
	if err != nil {
		return err
	}
	return wrap("settle an attempt", a.insert(ctx, kind, sourceKey, uid, true))
}

func (a Attempts) insert(ctx context.Context, kind outbound.AttemptKind, sourceKey string, uid pgtype.UUID, ok bool) error {
	id, err := uuidOf(ids.New())
	if err != nil {
		return err
	}
	return a.q(ctx).InsertLoginAttempt(ctx, sqlcgen.InsertLoginAttemptParams{Column1: id, Kind: string(kind), SourceKey: sourceKey, Succeeded: ok, UserID: uid})
}

func (a Audit) Append(ctx context.Context, e outbound.AuditEvent) error {
	id, err := uuidOf(ids.New())
	if err != nil {
		return err
	}
	uid, err := uuidOrNull(e.ActorUserID)
	if err != nil {
		return err
	}
	req, err := uuidOrNull(e.RequestID)
	if err != nil {
		return err
	}
	var ref *string
	if e.ActorRef != "" {
		ref = &e.ActorRef
	}
	detail := []byte("{}")
	if e.Detail != nil {
		if detail, err = json.Marshal(e.Detail); err != nil {
			return wrap("encode the audit detail", err)
		}
	}
	return wrap("append the audit event", a.q(ctx).InsertAuditEvent(ctx, sqlcgen.InsertAuditEventParams{
		Column1: id, ActorKind: e.ActorKind, Action: e.Action, Outcome: e.Outcome, Column5: detail,
		ActorUserID: uid, ActorRef: ref, RequestID: req,
	}))
}
