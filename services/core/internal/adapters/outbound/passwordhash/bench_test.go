package passwordhash

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// BenchmarkHash measures one hash for parameter sets around Default, and the
// time for two hashes that run together, which is the concurrency bound of the
// server. Run it with: go test -run '^$' -bench Hash -benchtime 10x ./internal/adapters/outbound/passwordhash
func BenchmarkHash(b *testing.B) {
	sets := []Params{
		{MemoryKiB: 19456, Time: 2, Threads: 1, SaltLen: 16, KeyLen: 32},
		{MemoryKiB: 46080, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32},
		{MemoryKiB: 65536, Time: 3, Threads: 1, SaltLen: 16, KeyLen: 32},
		{MemoryKiB: 65536, Time: 3, Threads: 2, SaltLen: 16, KeyLen: 32},
		Default,
		{MemoryKiB: 131072, Time: 3, Threads: 4, SaltLen: 16, KeyLen: 32},
	}
	for _, p := range sets {
		name := fmt.Sprintf("m=%d,t=%d,p=%d", p.MemoryKiB, p.Time, p.Threads)
		b.Run(name+"/single", func(b *testing.B) {
			h, err := New(p, 2)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := h.Hash(context.Background(), "correct horse battery staple"); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(name+"/two-at-once", func(b *testing.B) {
			h, err := New(p, 2)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				var wg sync.WaitGroup
				for range 2 {
					wg.Go(func() { _, _ = h.Hash(context.Background(), "correct horse battery staple") })
				}
				wg.Wait()
			}
		})
	}
}
