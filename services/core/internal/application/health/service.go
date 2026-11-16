// Package health implements the liveness and readiness use cases.
package health

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

var errProbePanicked = errors.New("readiness probe panicked")

// Service answers liveness and readiness questions.
type Service struct {
	probes  []outbound.ReadinessProbe
	timeout time.Duration
	logger  *slog.Logger
}

// New returns a Service. timeout bounds the whole readiness check. A nil
// logger discards log output.
func New(probes []outbound.ReadinessProbe, timeout time.Duration, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{probes: probes, timeout: timeout, logger: logger}
}

// Live reports that the process is running.
func (s *Service) Live() bool { return true }

// Ready reports whether every probe passes within the timeout. With no probe
// registered it is true: the process is initialized and has no dependency to
// check yet.
//
// Probes run one at a time in registration order. Every probe runs unless the
// context is done: once the caller cancels or the timeout expires, no further
// probe starts and the result is false. A probe that fails or panics makes the
// result false and does not stop the remaining probes.
func (s *Service) Ready(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	ready := true
	for i, p := range s.probes {
		if ctx.Err() != nil {
			s.logger.Warn("readiness check interrupted", "probes_not_run", len(s.probes)-i)
			return false
		}
		if err := s.run(ctx, p); err != nil {
			// Only the name is logged. The error text can hold a DSN.
			s.logger.Warn("readiness probe failed", "probe", p.Name())
			ready = false
		}
	}
	return ready
}

// run runs one probe in its own goroutine so that a probe that ignores its
// context cannot hold the caller past the deadline. The goroutine recovers a
// panic and reports it as an error, because an unrecovered panic in a
// goroutine ends the whole process and no middleware can catch it. The
// buffered channel lets the goroutine finish whenever the probe returns, even
// after run has given up waiting.
func (s *Service) run(ctx context.Context, p outbound.ReadinessProbe) error {
	done := make(chan error, 1)
	go func() {
		defer func() {
			// The panic value is dropped on purpose. It can hold a secret.
			if recover() != nil {
				done <- errProbePanicked
			}
		}()
		done <- p.Check(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
