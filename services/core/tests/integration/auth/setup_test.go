package auth_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

func TestSessionStateBeforeSetup(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	a := start(t, c, options{})
	b := newBrowser(a)
	r := b.state()
	if r.status != 200 || r.str("state") != "setup_required" || r.str("csrfToken") == "" {
		t.Fatalf("state = %d %s", r.status, r.body)
	}
	if b.cookies["ms_anon"] == "" {
		t.Error("no pre-session cookie was issued")
	}
	if _, has := b.cookies["ms_session"]; has {
		t.Error("a session cookie exists before setup")
	}
	// Until an account exists, every authenticated route is 401 and login is refused.
	if r := b.do("GET", "/api/v1/sessions", nil); r.status != 401 || r.code() != "unauthenticated" {
		t.Errorf("sessions before setup = %d %s", r.status, r.code())
	}
	if r := b.login(username, goodPass); r.status != 409 || r.code() != "setup_unavailable" {
		t.Errorf("login before setup = %d %s", r.status, r.code())
	}
}

// D-68: the secret is a 256-bit random value on a memory-backed file, only its
// hash is stored, and it works once.
func TestBootstrapSetupWorksOnceAndStoresOnlyHashes(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	a := start(t, c, options{})

	secret := a.secret()
	if len(secret) < 40 {
		t.Errorf("secret %q is too short for 256 bits", secret)
	}
	st, err := os.Stat(a.secretPath())
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("secret file: %v %v", st, err)
	}
	if dir, _ := os.Stat(a.runtime + "/morningstar"); dir.Mode().Perm() != 0o700 {
		t.Errorf("secret directory mode %o", dir.Mode().Perm())
	}
	if got := a.scalar(`SELECT secret_hash FROM morningstar.bootstrap_secrets WHERE consumed_at IS NULL AND superseded_at IS NULL`); strings.Contains(got, secret) || !strings.HasPrefix(got, "sha256:") {
		t.Errorf("stored secret hash %q", got)
	}

	b := newBrowser(a)
	if r := b.setup("not-the-secret", username, goodPass); r.status != 401 || r.code() != "bootstrap_secret_invalid" {
		t.Errorf("wrong secret = %d %s", r.status, r.code())
	}
	if n := a.scalar(`SELECT count(*) FROM morningstar.users`); n != "0" {
		t.Errorf("%s users after a wrong secret", n)
	}
	for _, bad := range []map[string]any{
		{"bootstrapSecret": secret, "username": "x", "password": goodPass},
		{"bootstrapSecret": secret, "username": username, "password": "short"},
		{"bootstrapSecret": "", "username": username, "password": goodPass},
	} {
		if r := b.do("POST", "/api/v1/setup/account", bad); r.status != 422 || r.code() != "validation_failed" {
			t.Errorf("invalid input %v = %d %s", bad, r.status, r.code())
		}
	}

	r := b.setup(secret, username, goodPass)
	if r.status != 201 {
		t.Fatalf("setup = %d %s", r.status, r.body)
	}
	code := r.str("recoveryCode")
	token := b.cookies["ms_session"]
	if code == "" || token == "" || r.str("user", "username") != username {
		t.Fatalf("setup result %s", r.body)
	}
	if strings.Contains(string(r.body), token) {
		t.Error("the session token is in the response body")
	}

	// Nothing secret is stored in the clear.
	sum := sha256.Sum256(decode(t, token))
	if got := a.scalar(`SELECT encode(token_hash, 'hex') FROM morningstar.sessions`); got != hex.EncodeToString(sum[:]) {
		t.Errorf("stored token hash %s", got)
	}
	for _, q := range []string{`SELECT string_agg(phc_hash, ' ') FROM morningstar.user_credentials`, `SELECT string_agg(code_hash, ' ') FROM morningstar.recovery_codes`} {
		got := a.scalar(q)
		if !strings.HasPrefix(got, "$argon2id$") || strings.Contains(got, goodPass) || strings.Contains(got, strings.ReplaceAll(code, "-", "")) {
			t.Errorf("%s = %q", q, got)
		}
	}
	if a.scalar(`SELECT count(*) FROM morningstar.sessions WHERE encode(token_hash, 'escape') LIKE '%' || $1 || '%'`, token) != "0" {
		t.Error("the token is stored in the clear")
	}

	// The secret works once and its file is gone.
	if _, err := os.Stat(a.secretPath()); !os.IsNotExist(err) {
		t.Error("the bootstrap secret file survived setup")
	}
	if r := newBrowserWith(a).setup(secret, "second", otherPass); r.status != 409 || r.code() != "account_exists" {
		t.Errorf("second setup = %d %s", r.status, r.code())
	}
	if a.scalar(`SELECT count(*) FROM morningstar.users`) != "1" || a.scalar(`SELECT count(*) FROM morningstar.bootstrap_secrets WHERE consumed_at IS NOT NULL`) != "1" {
		t.Error("account or secret bookkeeping is wrong")
	}
	if a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'auth.account_created' AND outcome = 'success'`) != "1" ||
		a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'auth.account_created' AND outcome = 'denied'`) == "0" {
		t.Error("account creation and the wrong secret are not both audited")
	}
}

func newBrowserWith(a *app) *browser { return newBrowser(a) }

// DB-07: two requests with the right secret create one account.
func TestConcurrentSetupCreatesOneAccount(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	a := start(t, c, options{})
	secret := a.secret()
	var wg sync.WaitGroup
	results := make(chan int, 6)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := newBrowser(a)
			results <- b.setup(secret, username, goodPass).status
		}()
	}
	wg.Wait()
	close(results)
	created := 0
	for s := range results {
		if s == 201 {
			created++
		} else if s != 409 {
			t.Errorf("unexpected status %d", s)
		}
	}
	if created != 1 || a.scalar(`SELECT count(*) FROM morningstar.users`) != "1" {
		t.Errorf("%d accounts were created", created)
	}
}

// D-68: when the location is not memory backed, no secret exists and setup is refused.
func TestSetupIsRefusedWithoutAMemoryBackedLocation(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	disk, err := os.MkdirTemp(".", "disk-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(disk) })
	abs, _ := os.Getwd()
	abs += "/" + disk
	if err := os.Chmod(abs, 0o700); err != nil {
		t.Fatal(err)
	}
	a := start(t, c, options{runtime: abs})
	if _, err := os.Stat(abs + "/morningstar/bootstrap-secret"); !os.IsNotExist(err) {
		t.Fatal("the secret reached the disk")
	}
	b := newBrowser(a)
	if r := b.state(); r.str("state") != "setup_required" {
		t.Errorf("state = %s", r.body)
	}
	if r := b.setup("anything", username, goodPass); r.status != 409 || r.code() != "setup_unavailable" {
		t.Errorf("setup = %d %s", r.status, r.code())
	}
	if a.scalar(`SELECT count(*) FROM morningstar.users`) != "0" {
		t.Error("an account was created without a memory-backed secret")
	}
}

// D-68: a new secret is made at each start while no account exists, and an
// older one stops working. The file is removed when the server stops.
func TestBootstrapSecretIsReplacedAtStartAndRemovedAtStop(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	rt := memoryRuntime(t)
	a := start(t, c, options{runtime: rt})
	first := a.secret()
	a.stop()
	if _, err := os.Stat(a.secretPath()); !os.IsNotExist(err) {
		t.Error("the secret file survived the stop")
	}
	b := start(t, c, options{runtime: rt, state: a.state})
	second := b.secret()
	if first == second {
		t.Fatal("the secret was reused at the next start")
	}
	if r := newBrowser(b).setup(first, username, goodPass); r.status != 401 {
		t.Errorf("the old secret = %d %s", r.status, r.code())
	}
	if r := newBrowser(b).setup(second, username, goodPass); r.status != 201 {
		t.Errorf("the new secret = %d %s", r.status, r.body)
	}
}

// With an account, a restart leaves no live secret and no file.
func TestRestartWithAnAccountLeavesNoBootstrapSecret(t *testing.T) {
	_, a, _, _ := fresh(t)
	rt, state := a.runtime, a.state
	a.stop()
	b := start(t, a.c, options{runtime: rt, state: state})
	if _, err := os.Stat(b.secretPath()); !os.IsNotExist(err) {
		t.Error("a bootstrap secret file exists although an account exists")
	}
	if b.scalar(`SELECT count(*) FROM morningstar.bootstrap_secrets WHERE consumed_at IS NULL AND superseded_at IS NULL`) != "0" {
		t.Error("a live bootstrap secret exists although an account exists")
	}
}
