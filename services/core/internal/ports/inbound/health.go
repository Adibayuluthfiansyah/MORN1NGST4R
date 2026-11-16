// Package inbound defines the use-case contracts that inbound adapters call.
package inbound

import "context"

// Health reports whether the process is alive and whether it can serve work.
// The answers are booleans on purpose. A probe never returns detail that could
// describe the host, a dependency or a configuration value.
type Health interface {
	// Live reports that the process is running. It never depends on a
	// dependency.
	Live() bool
	// Ready reports that every registered dependency check passes.
	Ready(ctx context.Context) bool
}
