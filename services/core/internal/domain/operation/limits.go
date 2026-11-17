package operation

import "time"

// Limits are the three values that an approval binds (D-55): how many jobs may
// run at the same time under it, how long one launched operation may run, and
// the request rate that all of those jobs share.
//
// The package sets no default. The numbers come from the pre-execution gate of
// D-71 (O-38). Until a number exists the limit is zero and the operation is
// blocked.
type Limits struct {
	MaxConcurrency    int
	MaxDuration       time.Duration
	RequestsPerMinute int
}

// Established reports whether the limits are numbers that can be enforced: all
// three are positive, and the rate is high enough to give every job at least
// one request per minute.
func (l Limits) Established() bool {
	return l.MaxConcurrency > 0 && l.MaxDuration > 0 && l.RequestsPerMinute >= l.MaxConcurrency
}

// PerJobRequestsPerMinute is the request rate of one job: the approved rate
// divided by the number of jobs that may run together. The shares of
// MaxConcurrency jobs never add up to more than RequestsPerMinute, so jobs under
// one approval do not multiply the approved rate (FR-APPROVAL-006). It is 0 when
// the limits are not established.
func (l Limits) PerJobRequestsPerMinute() int {
	if !l.Established() {
		return 0
	}
	return l.RequestsPerMinute / l.MaxConcurrency
}

func (l Limits) valid() bool {
	return l.MaxConcurrency >= 0 && l.MaxDuration >= 0 && l.RequestsPerMinute >= 0
}
