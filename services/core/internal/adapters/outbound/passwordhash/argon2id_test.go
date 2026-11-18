package passwordhash

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var fast = Params{MemoryKiB: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func newFast(t *testing.T, concurrent int) *Hasher {
	t.Helper()
	h, err := New(fast, concurrent)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHashAndVerify(t *testing.T) {
	h := newFast(t, 2)
	phc, err := h.Hash(t.Context(), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=64,t=1,p=1$") || strings.Contains(phc, "correct") {
		t.Errorf("phc = %q", phc)
	}
	if ok, err := h.Verify(t.Context(), "correct horse battery", phc); err != nil || !ok {
		t.Errorf("right password: %v, %v", ok, err)
	}
	for _, wrong := range []string{"correct horse batterz", "", "Correct horse battery", "correct horse battery "} {
		if ok, err := h.Verify(t.Context(), wrong, phc); err != nil || ok {
			t.Errorf("%q accepted: %v, %v", wrong, ok, err)
		}
	}
	other, _ := h.Hash(t.Context(), "correct horse battery")
	if other == phc {
		t.Error("two hashes of one password are equal: the salt is not random")
	}
}

func TestDefaultParametersAreTheOnesOfD68(t *testing.T) {
	if Default != (Params{MemoryKiB: 65536, Time: 3, Threads: 4, SaltLen: 16, KeyLen: 32}) {
		t.Errorf("Default = %+v", Default)
	}
}

func TestNewRefusesBadParameters(t *testing.T) {
	bad := []Params{
		{MemoryKiB: 64, Time: 0, Threads: 1, SaltLen: 16, KeyLen: 32},
		{MemoryKiB: 64, Time: 1, Threads: 0, SaltLen: 16, KeyLen: 32},
		{MemoryKiB: 4, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32},
		{MemoryKiB: 1 << 30, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32},
		{MemoryKiB: 64, Time: 1, Threads: 1, SaltLen: 4, KeyLen: 32},
		{MemoryKiB: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 4},
	}
	for i, p := range bad {
		if _, err := New(p, 1); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	if _, err := New(fast, 0); err == nil {
		t.Error("a concurrency of 0 was accepted")
	}
}

// A stored hash is attacker-influenced data if the database is. A malformed one
// verifies as false at the cost of a real check, and a huge one is never run.
func TestVerifyRefusesMalformedAndHostileHashes(t *testing.T) {
	h := newFast(t, 1)
	var calls atomic.Int32
	real := h.kdf
	h.kdf = func(pw, salt []byte, tm, mem uint32, th uint8, kl uint32) []byte {
		calls.Add(1)
		if mem > maxMemoryKiB {
			t.Errorf("the kdf was called with %d KiB", mem)
		}
		return real(pw, salt, tm, mem, th, kl)
	}
	good, _ := h.Hash(t.Context(), "pw")
	salt, key := strings.Split(good, "$")[4], strings.Split(good, "$")[5]
	hostile := []string{
		"", "plain text", "$argon2i$v=19$m=64,t=1,p=1$" + salt + "$" + key, "$argon2id$v=18$m=64,t=1,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=4294967295,t=1,p=1$" + salt + "$" + key, "$argon2id$v=19$m=64,t=4294967295,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=64,t=1,p=255$" + salt + "$" + key, "$argon2id$v=19$m=64,t=0,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=064,t=1,p=1$" + salt + "$" + key, "$argon2id$v=19$m=64,t=1,p=1$$" + key, "$argon2id$v=19$m=64,t=1,p=1$" + salt + "$",
		"$argon2id$v=19$m=64,t=1,p=1$" + salt + "$" + key + "$extra", "$argon2id$v=19$m=64,t=1$" + salt + "$" + key,
		"$argon2id$v=19$m=64,t=1,p=1$!!!$" + key, "$argon2id$v=19$t=1,m=64,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=7,t=1,p=1$" + salt + "$" + key, "$argon2id$v=19$m=15,t=1,p=2$" + salt + "$" + key,
	}
	for _, phc := range hostile {
		calls.Store(0)
		ok, err := h.Verify(t.Context(), "pw", phc)
		if ok || err != nil {
			t.Errorf("Verify(%q) = %v, %v", phc, ok, err)
		}
		if calls.Load() != 1 {
			t.Errorf("Verify(%q) ran the kdf %d times, want exactly one (the dummy cost)", phc, calls.Load())
		}
	}
}

func TestSpendVerifyCostRunsOneHash(t *testing.T) {
	h := newFast(t, 1)
	var calls atomic.Int32
	real := h.kdf
	h.kdf = func(pw, salt []byte, tm, mem uint32, th uint8, kl uint32) []byte {
		calls.Add(1)
		return real(pw, salt, tm, mem, th, kl)
	}
	if err := h.SpendVerifyCost(t.Context(), "anything"); err != nil || calls.Load() != 1 {
		t.Errorf("err %v, kdf calls %d", err, calls.Load())
	}
}

func TestConcurrencyIsBoundedAndWaitingHonoursTheContext(t *testing.T) {
	h := newFast(t, 2)
	var active, peak atomic.Int32
	release := make(chan struct{})
	h.kdf = func(pw, salt []byte, tm, mem uint32, th uint8, kl uint32) []byte {
		n := active.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-release
		active.Add(-1)
		return make([]byte, kl)
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.Hash(context.Background(), "pw")
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	for active.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Two hashes run and the others wait. A waiter whose context ends gives up.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, "pw"); err == nil {
		t.Error("a waiting hash ignored its context")
	}
	close(release)
	wg.Wait()
	if peak.Load() > 2 {
		t.Errorf("%d hashes ran at once, want at most 2", peak.Load())
	}
}
