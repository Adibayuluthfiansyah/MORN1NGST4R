// Package passwordhash implements outbound.PasswordHasher with Argon2id
// (D-68, P-48). The hash is a PHC string with its own parameters, so a later
// change of the defaults does not invalidate stored hashes.
package passwordhash

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

type Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
	SaltLen   int
	KeyLen    uint32
}

// Default holds the starting values of D-68. The M4 benchmark confirms them for
// the reference laptop.
var Default = Params{MemoryKiB: 65536, Time: 3, Threads: 4, SaltLen: 16, KeyLen: 32}

// Bounds on a stored hash. A row that an attacker changed must not make the
// server allocate gigabytes.
const (
	maxMemoryKiB = 262144
	maxTime      = 10
	maxThreads   = 16
	minSalt      = 8
	maxSalt      = 64
	minKey       = 16
	maxKey       = 64
)

type Hasher struct {
	params Params
	sem    chan struct{}
	dummy  string
	kdf    func(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte
}

var _ outbound.PasswordHasher = (*Hasher)(nil)

// New returns a Hasher that runs at most maxConcurrent hashes at once. Each hash
// holds MemoryKiB of memory, so the bound is the memory budget.
func New(p Params, maxConcurrent int) (*Hasher, error) {
	if maxConcurrent < 1 || p.Time < 1 || p.Threads < 1 || p.MemoryKiB < 8*uint32(p.Threads) ||
		p.MemoryKiB > maxMemoryKiB || p.Time > maxTime || p.Threads > maxThreads ||
		p.SaltLen < minSalt || p.SaltLen > maxSalt || p.KeyLen < minKey || p.KeyLen > maxKey {
		return nil, errors.New("passwordhash: invalid parameters")
	}
	h := &Hasher{params: p, sem: make(chan struct{}, maxConcurrent), kdf: argon2.IDKey}
	dummy, err := h.Hash(context.Background(), "morningstar dummy secret")
	if err != nil {
		return nil, err
	}
	h.dummy = dummy
	return h, nil
}

func (h *Hasher) acquire(ctx context.Context) error {
	select {
	case h.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Hasher) release() { <-h.sem }

var b64 = base64.RawStdEncoding

func (h *Hasher) Hash(ctx context.Context, secret string) (string, error) {
	salt := make([]byte, h.params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("passwordhash: %w", err)
	}
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer h.release()
	key := h.kdf([]byte(secret), salt, h.params.Time, h.params.MemoryKiB, h.params.Threads, h.params.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, h.params.MemoryKiB, h.params.Time, h.params.Threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

type parsed struct {
	memory, time uint32
	threads      uint8
	salt, key    []byte
}

func parse(phc string) (parsed, bool) {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return parsed{}, false
	}
	var p parsed
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return parsed{}, false
	}
	vals := [3]uint64{}
	for i, name := range [3]string{"m=", "t=", "p="} {
		v, ok := strings.CutPrefix(params[i], name)
		if !ok {
			return parsed{}, false
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || strconv.FormatUint(n, 10) != v {
			return parsed{}, false
		}
		vals[i] = n
	}
	if vals[0] < 8 || vals[0] > maxMemoryKiB || vals[1] < 1 || vals[1] > maxTime || vals[2] < 1 || vals[2] > maxThreads || vals[0] < 8*vals[2] {
		return parsed{}, false
	}
	p.memory, p.time, p.threads = uint32(vals[0]), uint32(vals[1]), uint8(vals[2])
	var err error
	if p.salt, err = b64.DecodeString(parts[4]); err != nil || len(p.salt) < minSalt || len(p.salt) > maxSalt {
		return parsed{}, false
	}
	if p.key, err = b64.DecodeString(parts[5]); err != nil || len(p.key) < minKey || len(p.key) > maxKey {
		return parsed{}, false
	}
	return p, true
}

func (h *Hasher) Verify(ctx context.Context, secret, phc string) (bool, error) {
	p, ok := parse(phc)
	if !ok {
		return false, h.SpendVerifyCost(ctx, secret)
	}
	if err := h.acquire(ctx); err != nil {
		return false, err
	}
	defer h.release()
	key := h.kdf([]byte(secret), p.salt, p.time, p.memory, p.threads, uint32(len(p.key)))
	return subtle.ConstantTimeCompare(key, p.key) == 1, nil
}

func (h *Hasher) SpendVerifyCost(ctx context.Context, secret string) error {
	p, _ := parse(h.dummy)
	if err := h.acquire(ctx); err != nil {
		return err
	}
	defer h.release()
	_ = h.kdf([]byte(secret), p.salt, p.time, p.memory, p.threads, uint32(len(p.key)))
	return nil
}
