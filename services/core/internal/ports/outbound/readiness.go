// Package outbound defines the contracts the application needs from the
// outside world. Adapters implement them.
package outbound

import "context"

// ReadinessProbe checks one dependency of the process, for example the
// database. Check must return when ctx is done.
type ReadinessProbe interface {
	// Name is a short fixed label. It is the only thing that is logged when
	// the check fails, because an error can carry a connection string.
	Name() string
	Check(ctx context.Context) error
}
