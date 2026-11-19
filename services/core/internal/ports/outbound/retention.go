package outbound

import (
	"context"
	"time"
)

// PurgedSessions is what a purge of sessions removed.
type PurgedSessions struct{ Grants, Sessions int64 }

// OperationalRetention purges the security-operational rows of class RC-B
// (data-lifecycle.md): sessions and re-authentication grants that ended, and
// login attempts. The database refuses a cutoff that is less than 30 days in the
// past and a caller that is not the retention role. An implementation joins the
// transaction of its context, so the purge and its audit event commit together.
type OperationalRetention interface {
	PurgeSessions(ctx context.Context, cutoff time.Time) (PurgedSessions, error)
	PurgeLoginAttempts(ctx context.Context, cutoff time.Time) (int64, error)
}
