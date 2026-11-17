package recoverymarker_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/recoverymarker"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const instance = "0196a5c0-1111-7222-8333-444455556666"

func newStore(t *testing.T) (*recoverymarker.Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	s, err := recoverymarker.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func marker(gen, wm uint64) outbound.RecoveryMarker {
	return outbound.RecoveryMarker{InstanceID: instance, Generation: gen, AuditWatermark: wm, WrittenAt: time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)}
}

func TestReadOfAMissingMarker(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Read(t.Context()); !errors.Is(err, outbound.ErrMarkerMissing) {
		t.Errorf("error = %v, want ErrMarkerMissing", err)
	}
}

func TestWriteThenReadRoundTripsWithOwnerOnlyModes(t *testing.T) {
	s, dir := newStore(t)
	if err := s.Replace(t.Context(), marker(1, 7)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.InstanceID != instance || got.Generation != 1 || got.AuditWatermark != 7 {
		t.Errorf("read %+v", got)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "recovery-generation.json"): 0o600, filepath.Join(dir, "recovery-generation.lock"): 0o600} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != want {
			t.Errorf("%s mode = %o, want %o", path, st.Mode().Perm(), want)
		}
	}
}

func TestNewRefusesAWideDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := recoverymarker.New(dir); err == nil {
		t.Error("a directory with mode 755 was accepted")
	}
}

func TestNewRefusesASymlinkedDirectory(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := recoverymarker.New(link); err == nil {
		t.Error("a symlinked state directory was accepted")
	}
}

// Section 3: anything that is not a regular, owner-only, well-formed file with
// a matching check value is marker_invalid.
func TestReadRefusesAnInvalidMarker(t *testing.T) {
	valid := func(t *testing.T) ([]byte, string, *recoverymarker.Store) {
		s, dir := newStore(t)
		if err := s.Replace(t.Context(), marker(2, 9)); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "recovery-generation.json")
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b, path, s
	}
	tests := []struct {
		name   string
		damage func(t *testing.T, b []byte, path string)
	}{
		{"garbage", func(t *testing.T, _ []byte, p string) { write(t, p, []byte("not json")) }},
		{"empty", func(t *testing.T, _ []byte, p string) { write(t, p, nil) }},
		{"generation edited", func(t *testing.T, b []byte, p string) { write(t, p, edit(t, b, "generation", 5)) }},
		{"watermark edited", func(t *testing.T, b []byte, p string) { write(t, p, edit(t, b, "audit_watermark", 1)) }},
		{"check removed", func(t *testing.T, b []byte, p string) { write(t, p, edit(t, b, "check", "")) }},
		{"unknown field", func(t *testing.T, b []byte, p string) { write(t, p, edit(t, b, "extra", 1)) }},
		{"version 2", func(t *testing.T, b []byte, p string) { write(t, p, edit(t, b, "version", 2)) }},
		{"instance not a uuid", func(t *testing.T, b []byte, p string) { write(t, p, edit(t, b, "instance_id", "x")) }},
		{"generation zero", func(t *testing.T, b []byte, p string) { write(t, p, edit(t, b, "generation", 0)) }},
		{"negative watermark", func(t *testing.T, b []byte, p string) { write(t, p, edit(t, b, "audit_watermark", -1)) }},
		{"too large", func(t *testing.T, _ []byte, p string) { write(t, p, make([]byte, 64<<10)) }},
		{"mode 0644", func(t *testing.T, _ []byte, p string) {
			if err := os.Chmod(p, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"mode 0660", func(t *testing.T, _ []byte, p string) {
			if err := os.Chmod(p, 0o660); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink", func(t *testing.T, b []byte, p string) {
			other := p + ".real"
			write(t, other, b)
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(other, p); err != nil {
				t.Fatal(err)
			}
		}},
		{"directory", func(t *testing.T, _ []byte, p string) {
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, path, s := valid(t)
			tc.damage(t, b, path)
			if _, err := s.Read(t.Context()); !errors.Is(err, outbound.ErrMarkerInvalid) {
				t.Errorf("error = %v, want ErrMarkerInvalid", err)
			}
		})
	}
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

func edit(t *testing.T, b []byte, key string, value any) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m[key] = value
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Within one instance and generation the watermark never goes down. A lower
// generation is refused. A higher generation replaces the watermark, because a
// reconciliation restarts it from the database it reconciled.
func TestWriteKeepsTheHighestWatermarkAndNeverLowersTheGeneration(t *testing.T) {
	s, _ := newStore(t)
	ctx := t.Context()
	for _, m := range []outbound.RecoveryMarker{marker(1, 10), marker(1, 4)} {
		if err := s.Replace(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := s.Read(ctx); got.AuditWatermark != 10 {
		t.Errorf("watermark = %d after a lower write, want 10", got.AuditWatermark)
	}
	if err := s.Replace(ctx, marker(2, 3)); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(ctx); got.Generation != 2 || got.AuditWatermark != 3 {
		t.Errorf("after a new generation: %+v, want generation 2 and watermark 3", got)
	}
	if err := s.Replace(ctx, marker(1, 99)); !errors.Is(err, outbound.ErrMarkerStale) {
		t.Errorf("a lower generation: error = %v, want ErrMarkerStale", err)
	}
	other := marker(2, 1) // not a higher generation than the marker on disk
	other.InstanceID = "0196a5c0-aaaa-7222-8333-444455556666"
	if err := s.Replace(ctx, other); !errors.Is(err, outbound.ErrMarkerStale) {
		t.Errorf("another instance: error = %v, want ErrMarkerStale", err)
	}
}

// G-recovery 12: writers that run at the same time never lower the watermark.
func TestConcurrentWritersNeverLowerTheWatermark(t *testing.T) {
	s, dir := newStore(t)
	if err := s.Replace(t.Context(), marker(1, 1)); err != nil {
		t.Fatal(err)
	}
	// Each goroutine has its own store on the same directory, like the server and the worker.
	var wg sync.WaitGroup
	for i := 1; i <= 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st, err := recoverymarker.New(dir)
			if err != nil {
				t.Error(err)
				return
			}
			for j := 0; j < 25; j++ {
				if err := st.Replace(context.Background(), marker(1, uint64(i*100+j))); err != nil {
					t.Error(err)
					return
				}
			}
		}(i)
	}
	// A reader sees only complete, valid markers and a watermark that does not go down.
	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		var last uint64
		for {
			select {
			case <-stop:
				return
			default:
			}
			m, err := s.Read(context.Background())
			if err != nil {
				t.Errorf("a reader saw %v", err)
				return
			}
			if m.AuditWatermark < last {
				t.Errorf("the watermark went down from %d to %d", last, m.AuditWatermark)
				return
			}
			last = m.AuditWatermark
		}
	}()
	wg.Wait()
	close(stop)
	<-readerDone
	got, err := s.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.AuditWatermark != 8*100+24 {
		t.Errorf("final watermark = %d, want %d", got.AuditWatermark, 8*100+24)
	}
}

// Advance only updates a marker that exists. A marker that vanished while the
// process runs must be detected, so Advance never creates or repairs one.
func TestAdvanceNeverCreatesOrRepairsTheMarker(t *testing.T) {
	s, dir := newStore(t)
	path := filepath.Join(dir, "recovery-generation.json")
	if err := s.Advance(t.Context(), marker(1, 5)); !errors.Is(err, outbound.ErrMarkerMissing) {
		t.Errorf("Advance without a marker: error = %v, want ErrMarkerMissing", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Advance created the marker")
	}
	if err := s.Replace(t.Context(), marker(1, 5)); err != nil {
		t.Fatal(err)
	}
	if err := s.Advance(t.Context(), marker(1, 9)); err != nil {
		t.Fatalf("Advance on a valid marker: %v", err)
	}
	if got, _ := s.Read(t.Context()); got.AuditWatermark != 9 {
		t.Errorf("watermark = %d, want 9", got.AuditWatermark)
	}
	if err := s.Advance(t.Context(), marker(1, 2)); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(t.Context()); got.AuditWatermark != 9 {
		t.Errorf("watermark = %d after a lower advance, want 9", got.AuditWatermark)
	}
	// A damaged marker stays damaged.
	damaged := []byte("not json")
	write(t, path, damaged)
	if err := s.Advance(t.Context(), marker(1, 12)); !errors.Is(err, outbound.ErrMarkerInvalid) {
		t.Errorf("Advance on a damaged marker: error = %v, want ErrMarkerInvalid", err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(damaged) {
		t.Error("Advance rewrote a damaged marker")
	}
}

func TestCleanupRemovesLeftoverTemporaryFilesOnly(t *testing.T) {
	s, dir := newStore(t)
	if err := s.Replace(t.Context(), marker(1, 1)); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".recovery-generation.json.tmp-123")
	write(t, tmp, []byte("half"))
	keep := filepath.Join(dir, "unrelated.txt")
	write(t, keep, []byte("x"))
	if err := s.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("the temporary file is still there")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("Cleanup removed an unrelated file")
	}
	if _, err := s.Read(t.Context()); err != nil {
		t.Errorf("the marker is damaged: %v", err)
	}
}

// A temporary file that an interrupted write left behind is not a marker.
func TestALeftoverTemporaryFileIsIgnoredByRead(t *testing.T) {
	s, dir := newStore(t)
	write(t, filepath.Join(dir, ".recovery-generation.json.tmp-9"), []byte(`{"version":1}`))
	if _, err := s.Read(t.Context()); !errors.Is(err, outbound.ErrMarkerMissing) {
		t.Errorf("error = %v, want ErrMarkerMissing", err)
	}
}

func TestWriteRefusesInvalidMarkers(t *testing.T) {
	s, _ := newStore(t)
	bad := []outbound.RecoveryMarker{
		{InstanceID: "x", Generation: 1},
		{InstanceID: instance, Generation: 0},
		{InstanceID: instance, Generation: 1 << 63},
		{InstanceID: instance, Generation: 1, AuditWatermark: 1 << 63},
	}
	for i, m := range bad {
		m.WrittenAt = time.Now()
		if err := s.Replace(t.Context(), m); err == nil {
			t.Errorf("case %d: an invalid marker was written", i)
		}
	}
}
