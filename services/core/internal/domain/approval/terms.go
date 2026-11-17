package approval

import (
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// Terms are what a human grants in an approval: how long it lasts, how many jobs
// may use it, how many launches one job may make, and for an intrusive
// operation the re-authentication grant that the password entry produced.
type Terms struct {
	ExpiresAt     time.Time
	MaxUses       int
	MaxAttempts   int
	ReauthGrantID string
}

// DefaultTerms returns the starting values of S1.6 item 6 and D-45. They are
// starting values and not constants. A change of them is a policy change owned
// by the policy store (M16). Intrusive: 1 use, 1 attempt, 15 minutes.
// Low-impact: 5 uses, 3 attempts, 24 hours. A passive operation has none. The
// grant is left empty: the caller sets it after the password entry.
func DefaultTerms(class operation.RiskClass, now time.Time) (Terms, bool) {
	switch class {
	case operation.ClassIntrusive:
		return Terms{ExpiresAt: now.Add(15 * time.Minute), MaxUses: 1, MaxAttempts: 1}, true
	case operation.ClassLowImpact:
		return Terms{ExpiresAt: now.Add(24 * time.Hour), MaxUses: 5, MaxAttempts: maxAttempts}, true
	}
	return Terms{}, false
}

func (t Terms) validate(class operation.RiskClass, now time.Time) error {
	if !t.ExpiresAt.Round(0).After(now.Round(0)) || t.MaxUses < 1 || t.MaxAttempts < 1 || t.MaxAttempts > maxAttempts {
		return ErrBadTerms
	}
	if class == operation.ClassIntrusive {
		if t.MaxUses != 1 || t.MaxAttempts != 1 {
			return ErrBadTerms
		}
		if t.ReauthGrantID == "" {
			return ErrReauthRequired
		}
	}
	if t.ReauthGrantID != "" && !validID(t.ReauthGrantID) {
		return ErrBadTerms
	}
	return nil
}
