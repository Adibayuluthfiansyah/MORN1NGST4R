package audit

import (
	"errors"
	"sync"
	"time"
)

// ErrInvalidPolicy is returned for a retry policy that is not explicit or not
// bounded.
var ErrInvalidPolicy = errors.New("invalid audit retry policy")

// Hard limits of any policy. They keep a mistake in a value from becoming an
// unbounded retry.
const (
	MaxAttemptsLimit = 5
	MaxDelayLimit    = 2 * time.Second
	MaxTimeoutLimit  = 5 * time.Second
)

// RetryPolicy is the bound on the retries of one audit write (SEC 13, D-48).
// There is no default here: a sink refuses the zero value, and the composition
// root supplies the numbers. MaxAttempts counts the first attempt. Delays[i] is
// the wait after attempt i+1 fails.
type RetryPolicy struct {
	MaxAttempts    int
	Delays         []time.Duration
	AttemptTimeout time.Duration
}

func (p RetryPolicy) Validate() error {
	if p.MaxAttempts < 1 || p.MaxAttempts > MaxAttemptsLimit {
		return ErrInvalidPolicy
	}
	if len(p.Delays) != p.MaxAttempts-1 {
		return ErrInvalidPolicy
	}
	for _, d := range p.Delays {
		if d <= 0 || d > MaxDelayLimit {
			return ErrInvalidPolicy
		}
	}
	if p.AttemptTimeout <= 0 || p.AttemptTimeout > MaxTimeoutLimit {
		return ErrInvalidPolicy
	}
	return nil
}

// Delay is the wait after the given attempt (1 is the first) has failed. It is
// zero after the last attempt.
func (p RetryPolicy) Delay(attempt int) time.Duration {
	if attempt < 1 || attempt > len(p.Delays) {
		return 0
	}
	return p.Delays[attempt-1]
}

// WorstCase is the longest a write can take: every attempt runs to its timeout
// and every delay is waited.
func (p RetryPolicy) WorstCase() time.Duration {
	total := time.Duration(p.MaxAttempts) * p.AttemptTimeout
	for _, d := range p.Delays {
		total += d
	}
	return total
}

const coalescerKeys = 64

// Coalescer limits how often one repeated event is recorded. The first event of
// a key is recorded. The repeats inside the window are counted, and the next
// event after the window reports the count. It never refuses an event because
// it is full: past coalescerKeys keys every event is recorded.
type Coalescer struct {
	window time.Duration
	now    func() time.Time
	mu     sync.Mutex
	keys   map[string]*burst
}

type burst struct {
	start      time.Time
	suppressed int
}

func NewCoalescer(window time.Duration, now func() time.Time) *Coalescer {
	return &Coalescer{window: window, now: now, keys: map[string]*burst{}}
}

// Retract gives back an Admit that recorded: the write failed, so the next
// event of the key is recorded again and reports the suppressed count that the
// failed record would have carried.
func (c *Coalescer) Retract(key string, suppressed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if b, ok := c.keys[key]; ok {
		b.start = time.Time{}
		b.suppressed = suppressed
	}
}

// Admit reports whether to record an event of key, and how many repeats the
// window before it suppressed.
func (c *Coalescer) Admit(key string) (record bool, suppressed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.now()
	b, ok := c.keys[key]
	if !ok {
		if len(c.keys) < coalescerKeys {
			c.keys[key] = &burst{start: t}
		}
		return true, 0
	}
	if t.Sub(b.start) < c.window {
		b.suppressed++
		return false, 0
	}
	n := b.suppressed
	b.start, b.suppressed = t, 0
	return true, n
}
