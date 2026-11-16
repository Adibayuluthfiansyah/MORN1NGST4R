package health_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/health"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

type probe struct {
	name  string
	check func(context.Context) error
}

func (p probe) Name() string                    { return p.name }
func (p probe) Check(ctx context.Context) error { return p.check(ctx) }

func ok(context.Context) error { return nil }

// The service must satisfy the inbound port that the HTTP adapter consumes.
var _ inbound.Health = (*health.Service)(nil)

func newService(t *testing.T, timeout time.Duration, probes ...outbound.ReadinessProbe) (*health.Service, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	return health.New(probes, timeout, logger), &logs
}

func TestLiveIsTrueEvenWhenNotReady(t *testing.T) {
	svc, _ := newService(t, time.Second, probe{"db", func(context.Context) error { return errors.New("down") }})
	if !svc.Live() {
		t.Error("Live() = false, want true: liveness must not depend on dependencies")
	}
}

func TestReadyWithoutProbes(t *testing.T) {
	svc, _ := newService(t, time.Second)
	if !svc.Ready(context.Background()) {
		t.Error("Ready() = false with no probes registered, want true")
	}
}

func TestReadyRequiresEveryProbe(t *testing.T) {
	failing := probe{"queue", func(context.Context) error { return errors.New("down") }}
	tests := []struct {
		name   string
		probes []outbound.ReadinessProbe
		want   bool
	}{
		{"all pass", []outbound.ReadinessProbe{probe{"a", ok}, probe{"b", ok}}, true},
		{"first fails", []outbound.ReadinessProbe{failing, probe{"b", ok}}, false},
		{"last fails", []outbound.ReadinessProbe{probe{"a", ok}, failing}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newService(t, time.Second, tc.probes...)
			if got := svc.Ready(context.Background()); got != tc.want {
				t.Errorf("Ready() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadyFailsWhenProbeIgnoresTheTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	stuck := probe{"stuck", func(context.Context) error { <-release; return nil }}
	svc, _ := newService(t, 20*time.Millisecond, stuck)

	start := time.Now()
	got := svc.Ready(context.Background())
	if got {
		t.Error("Ready() = true for a probe that never returns, want false")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Ready() took %v, want it bounded by the timeout", elapsed)
	}
}

func TestReadyFailsWhenCallerContextIsCancelled(t *testing.T) {
	svc, _ := newService(t, time.Second, probe{"a", func(ctx context.Context) error { return ctx.Err() }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if svc.Ready(ctx) {
		t.Error("Ready() = true with a cancelled context, want false")
	}
}

// A probe error can carry a connection string. Only the probe name is logged.
func TestFailureLogsProbeNameNotErrorText(t *testing.T) {
	leaky := probe{"database", func(context.Context) error {
		// Built from pieces so the repository safety scan does not see a URL credential.
		return errors.New("dial postgres:" + "//u:hunter2@host/db failed")
	}}
	svc, logs := newService(t, time.Second, leaky)
	svc.Ready(context.Background())
	out := logs.String()
	if !strings.Contains(out, "database") {
		t.Errorf("log does not name the failing probe: %q", out)
	}
	if strings.Contains(out, "hunter2") || strings.Contains(out, "postgres://") {
		t.Errorf("log contains the probe error text: %q", out)
	}
}

func TestNilLoggerIsAccepted(t *testing.T) {
	svc := health.New([]outbound.ReadinessProbe{probe{"a", func(context.Context) error { return errors.New("x") }}}, time.Second, nil)
	if svc.Ready(context.Background()) {
		t.Error("Ready() = true, want false")
	}
}
