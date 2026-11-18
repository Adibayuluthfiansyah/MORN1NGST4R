package outbound

import "errors"

// ErrStoreUnavailable marks a failure to reach the database. Use cases map it to
// a retryable "database unavailable" answer and never to a security decision.
var ErrStoreUnavailable = errors.New("the store is unavailable")

// ErrUniqueViolation marks a unique constraint that a write broke, for example
// the second account of a race for the single-account rule.
var ErrUniqueViolation = errors.New("a unique constraint was violated")

// ErrNotLive marks a conditional update that matched no live row, for example
// a recovery code that another request used first.
var ErrNotLive = errors.New("the row is not live")
