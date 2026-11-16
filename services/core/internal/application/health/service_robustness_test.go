package health_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/health"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// counting returns a probe that records how often it ran.
func counting(name string, n *atomic.Int32, err error) outbound.ReadinessProbe {
	return probe{name, func(context.Context) error { n.Add(1); return err }}
}

// An unrecovered panic in a probe goroutine would end the whole test binary,
// so these tests also prove that the process does not crash.
func TestPanickingProbeIsAnUnhealthyResultNotACrash(t *testing.T) {
	var after atomic.Int32
	panicker := probe{"database", func(context.Context) error { panic("panic-value-with-a-secret") }}
	svc, logs := newService(t, time.Second, panicker, counting("later", &after, nil))

	if svc.Ready(context.Background()) {
		t.Error("Ready() = true with a panicking probe, want false")
	}
	if after.Load() != 1 {
		t.Errorf("probe after the panicking one ran %d times, want 1 (every probe still runs)", after.Load())
	}
	out := logs.String()
	if !strings.Contains(out, "database") {
		t.Errorf("log does not name the panicking probe: %q", out)
	}
	if strings.Contains(out, "panic-value-with-a-secret") {
		t.Errorf("panic value leaked into the log: %q", out)
	}
}

func TestPanicWithAnErrorValueIsAlsoContained(t *testing.T) {
	svc, _ := newService(t, time.Second, probe{"a", func(context.Context) error { panic(errors.New("boom")) }})
	if svc.Ready(context.Background()) {
		t.Error("Ready() = true, want false")
	}
}

func TestPanicWithNilIsAlsoContained(t *testing.T) {
	// panic(nil) becomes a *runtime.PanicNilError since Go 1.21.
	svc, _ := newService(t, time.Second, probe{"a", func(context.Context) error { panic(nil) }})
	if svc.Ready(context.Background()) {
		t.Error("Ready() = true, want false")
	}
}

func TestNoProbeStartsWhenTheContextIsAlreadyCancelled(t *testing.T) {
	var ran atomic.Int32
	svc, logs := newService(t, time.Second, counting("a", &ran, nil), counting("b", &ran, nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if svc.Ready(ctx) {
		t.Error("Ready() = true with a cancelled context, want false (even for probes that would pass)")
	}
	if ran.Load() != 0 {
		t.Errorf("%d probes ran after cancellation, want 0", ran.Load())
	}
	if !strings.Contains(logs.String(), "readiness check interrupted") {
		t.Errorf("the interruption was not logged: %q", logs.String())
	}
}

func TestNoFurtherProbeStartsAfterTheDeadlineExpires(t *testing.T) {
	var second atomic.Int32
	honoursContext := probe{"slow", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}
	svc, _ := newService(t, 30*time.Millisecond, honoursContext, counting("second", &second, nil))

	start := time.Now()
	if svc.Ready(context.Background()) {
		t.Error("Ready() = true after the deadline expired, want false")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Ready() took %v, want it bounded by the timeout", elapsed)
	}
	if second.Load() != 0 {
		t.Errorf("the second probe ran %d times after the deadline, want 0", second.Load())
	}
}

func TestNoFurtherProbeStartsAfterCallerCancelsDuringAProbe(t *testing.T) {
	var second atomic.Int32
	started := make(chan struct{})
	first := probe{"first", func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() }}
	svc, _ := newService(t, time.Minute, first, counting("second", &second, nil))

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan bool, 1)
	go func() { result <- svc.Ready(ctx) }()
	<-started
	cancel()

	select {
	case ready := <-result:
		if ready {
			t.Error("Ready() = true after the caller cancelled, want false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ready() did not return after cancellation")
	}
	if second.Load() != 0 {
		t.Errorf("the second probe ran %d times after cancellation, want 0", second.Load())
	}
}

func TestProbesRunInRegistrationOrderAndAllRunDespiteAFailure(t *testing.T) {
	var mu sync.Mutex
	var order []string
	record := func(name string, err error) outbound.ReadinessProbe {
		return probe{name, func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return err
		}}
	}
	svc, _ := newService(t, time.Second, record("a", nil), record("b", errors.New("down")), record("c", nil))
	if svc.Ready(context.Background()) {
		t.Error("Ready() = true with a failing probe, want false")
	}
	if got := strings.Join(order, ","); got != "a,b,c" {
		t.Errorf("probe order = %q, want a,b,c", got)
	}
}

func TestReadyIsDeterministicAcrossRepeatedCalls(t *testing.T) {
	var ran atomic.Int32
	svc, _ := newService(t, time.Second, counting("a", &ran, nil))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 200; i++ {
		if !svc.Ready(context.Background()) {
			t.Fatalf("call %d: Ready() = false for a passing probe", i)
		}
		if svc.Ready(cancelled) {
			t.Fatalf("call %d: Ready() = true for a cancelled context", i)
		}
	}
}

// Goroutines of probes that return, fail, panic or honour the context must
// all finish. A probe that ignores its context cannot be stopped from outside
// and is the probe author's bug, so it is not part of this test.
func TestProbeGoroutinesDoNotLeak(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	probes := []outbound.ReadinessProbe{
		probe{"ok", func(context.Context) error { return nil }},
		probe{"fail", func(context.Context) error { return errors.New("x") }},
		probe{"panic", func(context.Context) error { panic("x") }},
		probe{"slow", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }},
	}
	svc := health.New(probes, 5*time.Millisecond, logger)

	runtime.GC()
	before := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		svc.Ready(context.Background())
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines: %d before, %d after the calls, want no growth", before, after)
	}
}
