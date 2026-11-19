// Package unavailable holds the adapters of a capability that does not exist
// yet. Each one refuses, so the worker of milestone M7 is complete and
// fail-closed while the real adapters of later milestones are missing: the
// sandbox runner of M8 and the sources of the evaluator inputs of M16, M18 and
// M19 (TD-M7-01, TD-M7-02).
package unavailable

import (
	"context"
	"errors"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// ErrNoScopeInputs is what the source of the evaluator inputs returns. The
// evaluator turns it into a denial, so no job can be claimed or launched.
var ErrNoScopeInputs = errors.New("no source of the evaluator inputs is configured")

// ScopeInputs refuses every request.
type ScopeInputs struct{}

func (ScopeInputs) Load(context.Context, outbound.JobRecord) (outbound.ScopeInput, error) {
	return outbound.ScopeInput{}, ErrNoScopeInputs
}

// Runner refuses to start a process and reports no process as terminated.
type Runner struct{}

func (Runner) Preflight(context.Context, outbound.LaunchSpec) error {
	return outbound.ErrSandboxUnavailable
}

func (Runner) Start(context.Context, outbound.LaunchSpec) (outbound.Process, error) {
	return nil, outbound.ErrSandboxUnavailable
}

func (Runner) Terminate(context.Context, string, string) (bool, error) {
	return false, outbound.ErrSandboxUnavailable
}
