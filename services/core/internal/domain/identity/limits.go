package identity

import "time"

// The starting values of DQ-04, DQ-05 and D-68. They are policy defaults that
// M4 tests, not measured numbers.
const (
	SessionIdleTTL     = 30 * time.Minute
	SessionAbsoluteTTL = 12 * time.Hour
	SessionTouchEvery  = time.Minute
	ReauthGrantTTL     = 120 * time.Second
	AnonTokenTTL       = 15 * time.Minute

	AttemptWindow     = 15 * time.Minute
	MaxFailedInWindow = 5
)

// AttemptsExhausted reports whether the failures counted in the window refuse
// the next attempt. The check runs before any password hashing.
func AttemptsExhausted(failures int) bool { return failures >= MaxFailedInWindow }
