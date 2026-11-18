package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

type fakeSessions struct {
	outbound.SessionStore
	rec     outbound.SessionRecord
	found   bool
	touched int
}

func (f *fakeSessions) Lookup(context.Context, []byte) (outbound.SessionRecord, bool, error) {
	return f.rec, f.found, nil
}

func (f *fakeSessions) Touch(context.Context, string, time.Duration, time.Duration) error {
	f.touched++
	return nil
}

func counterRandom() func(int) []byte {
	var n byte
	return func(size int) []byte {
		n++
		b := make([]byte, size)
		for i := range b {
			b[i] = n
		}
		return b
	}
}

func newService(now *time.Time, sessions outbound.SessionStore) *Service {
	return New(Deps{
		Sessions: sessions,
		Now:      func() time.Time { return *now },
		Random:   counterRandom(),
	})
}

func liveRecord(now time.Time) outbound.SessionRecord {
	return outbound.SessionRecord{
		ID: "s1", UserID: "u1", CSRFHash: []byte("x"), Now: now,
		IdleExpiresAt: now.Add(time.Minute), AbsoluteExpiresAt: now.Add(time.Hour),
	}
}

func TestAuthenticateBuildsAHumanActor(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := newService(&now, &fakeSessions{rec: liveRecord(now), found: true})
	token, _ := newToken(counterRandom())
	p, err := s.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if p.Actor().Kind() != actor.KindHuman || p.Actor().ID() != "u1" {
		t.Fatalf("actor = %v %q", p.Actor().Kind(), p.Actor().ID())
	}
	if p.UserID() != "u1" || p.SessionID() != "s1" {
		t.Fatal("principal identity wrong")
	}
}

func TestAuthenticateRefusesEveryNonLiveSession(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token, _ := newToken(counterRandom())
	cases := map[string]func(*outbound.SessionRecord){
		"revoked":       func(r *outbound.SessionRecord) { r.Revoked = true },
		"disabled":      func(r *outbound.SessionRecord) { r.UserDisabled = true },
		"idle expiry":   func(r *outbound.SessionRecord) { r.IdleExpiresAt = r.Now },
		"abs expiry":    func(r *outbound.SessionRecord) { r.AbsoluteExpiresAt = r.Now },
		"idle past":     func(r *outbound.SessionRecord) { r.IdleExpiresAt = r.Now.Add(-time.Second) },
		"absolute past": func(r *outbound.SessionRecord) { r.AbsoluteExpiresAt = r.Now.Add(-time.Second) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			rec := liveRecord(now)
			mutate(&rec)
			fs := &fakeSessions{rec: rec, found: true}
			p, err := newService(&now, fs).Authenticate(context.Background(), token)
			if !errors.Is(err, inbound.ErrSessionExpired) || p != nil {
				t.Fatalf("got %v, %v", p, err)
			}
			if fs.touched != 0 {
				t.Fatal("a dead session was touched")
			}
		})
	}
}

func TestAuthenticateRefusesUnknownAndMalformedTokens(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token, _ := newToken(counterRandom())
	s := newService(&now, &fakeSessions{found: false})
	for _, tok := range []string{"", "short", strings.Repeat("!", 43), token} {
		if p, err := s.Authenticate(context.Background(), tok); !errors.Is(err, inbound.ErrUnauthenticated) || p != nil {
			t.Fatalf("%q: got %v, %v", tok, p, err)
		}
	}
}

func TestAuthenticateRefusesAnIdentityThatIsNotAValidActor(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rec := liveRecord(now)
	rec.UserID = "bad id with spaces"
	token, _ := newToken(counterRandom())
	p, err := newService(&now, &fakeSessions{rec: rec, found: true}).Authenticate(context.Background(), token)
	if !errors.Is(err, inbound.ErrUnauthenticated) || p != nil {
		t.Fatalf("got %v, %v", p, err)
	}
}

func TestCSRFIsDerivedAndChecked(t *testing.T) {
	_, raw := newToken(counterRandom())
	tok, hash := csrfFor(raw)
	tok2, _ := csrfFor(raw)
	if tok != tok2 {
		t.Fatal("derivation is not stable")
	}
	if !csrfMatches(tok, hash) {
		t.Fatal("own token refused")
	}
	other, _ := newToken(counterRandom())
	for _, bad := range []string{"", "x", other, tok + "A", strings.ToUpper(tok)} {
		if csrfMatches(bad, hash) {
			t.Fatalf("accepted %q", bad)
		}
	}
	other2 := append([]byte(nil), raw...)
	other2[0] ^= 0xFF
	tok3, _ := csrfFor(other2)
	if csrfMatches(tok3, hash) {
		t.Fatal("token of another session accepted")
	}
	if sessionTok, _ := newToken(func(int) []byte { return raw }); tok == sessionTok {
		t.Fatal("csrf token equals the session token")
	}
}

func TestPrincipalValidCSRFUsesStoredHash(t *testing.T) {
	_, raw := newToken(counterRandom())
	tok, hash := csrfFor(raw)
	p := &principal{csrfHash: hash}
	if !p.ValidCSRF(tok) || p.ValidCSRF("") {
		t.Fatal("ValidCSRF wrong")
	}
}

func TestAnonTokenExpiresAndRejectsTampering(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := newService(&now, nil)
	tok := s.newAnonToken()
	payload, ok := s.parseAnon(tok)
	if !ok {
		t.Fatal("fresh token refused")
	}
	csrf := s.anonCSRF(payload)
	if !s.ValidAnonCSRF(tok, csrf) || s.ValidAnonCSRF(tok, "") || s.ValidAnonCSRF(tok, csrf+"x") {
		t.Fatal("anon csrf check wrong")
	}
	now = now.Add(14*time.Minute + 59*time.Second)
	if _, ok := s.parseAnon(tok); !ok {
		t.Fatal("token refused before expiry")
	}
	now = now.Add(time.Second)
	if _, ok := s.parseAnon(tok); ok {
		t.Fatal("token accepted at expiry")
	}
	if s.ValidAnonCSRF(tok, csrf) {
		t.Fatal("csrf of an expired token accepted")
	}
	now = time.Unix(1_700_000_000, 0)
	i := strings.IndexByte(tok, '.')
	for _, bad := range []string{"", ".", tok[:i], tok[:i] + ".", tok[i+1:], tok[:len(tok)-1] + "A", strings.Repeat("a", 200)} {
		if _, ok := s.parseAnon(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
	other := New(Deps{Now: func() time.Time { return now }, Random: func(n int) []byte { return make([]byte, n) }})
	if _, ok := other.parseAnon(tok); ok {
		t.Fatal("token of another process key accepted")
	}
}

type fakeRecovery struct {
	inbound.Recovery
	state inbound.RecoveryState
}

func (f fakeRecovery) State() inbound.RecoveryState { return f.state }

func TestGateFollowsTheRecoveryState(t *testing.T) {
	cases := []struct {
		name  string
		state inbound.RecoveryState
		want  inbound.Gate
	}{
		{"normal", inbound.RecoveryState{Normal: true}, inbound.GateOpen},
		{"required", inbound.RecoveryState{Required: true}, inbound.GateRecoveryRequired},
		{"required wins over normal", inbound.RecoveryState{Required: true, Normal: true}, inbound.GateRecoveryRequired},
		{"not checked yet", inbound.RecoveryState{}, inbound.GateNotReady},
	}
	for _, c := range cases {
		s := New(Deps{Recovery: fakeRecovery{state: c.state}})
		if got := s.Gate(); got != c.want {
			t.Errorf("%s: gate = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStorableTextKeepsUTF8(t *testing.T) {
	if storableText("abc", 5) != "abc" || storableText("abcdef", 3) != "abc" {
		t.Fatal("ascii")
	}
	if got := storableText("aé", 2); got != "a" {
		t.Fatalf("cut inside a rune: %q", got)
	}
}

// A User-Agent is client input that is stored. PostgreSQL refuses invalid UTF-8,
// which would turn a correct login into a 500.
func TestStorableTextReturnsStorableText(t *testing.T) {
	got := storableText("bad\xff\xfeagent\x00\x1b[31m", 200)
	if !utf8.ValidString(got) || strings.ContainsAny(got, "\x00\x1b\xff") {
		t.Fatalf("not storable: %q", got)
	}
	if got != "badagent[31m" {
		t.Fatalf("got %q", got)
	}
}
