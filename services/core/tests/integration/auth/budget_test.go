package auth_test

import "testing"

// Password guesses (login, re-authentication) share one budget, so the
// re-authentication route is not a second way to guess. Recovery-code guesses
// have their own budget, so a lockout of the password does not close the one
// route that gets the owner back in.
func TestRecoveryHasItsOwnAttemptBudget(t *testing.T) {
	_, a, _, code := fresh(t)
	attacker := newBrowser(a)
	for i := 0; i < 5; i++ {
		if r := attacker.login(username, wrongPass); r.status != 401 {
			t.Fatalf("bad login %d = %d %s", i+1, r.status, r.body)
		}
	}
	if r := attacker.login(username, goodPass); r.status != 429 {
		t.Fatalf("the correct password during a lockout = %d %s", r.status, r.body)
	}
	owner := newBrowser(a)
	r := owner.reset(code, otherPass)
	if r.status != 200 {
		t.Fatalf("the recovery code during a password lockout = %d %s", r.status, r.body)
	}
	if owner.cookies["ms_session"] == "" {
		t.Fatal("the reset issued no session")
	}
}

func TestBadRecoveryCodesDoNotLockThePassword(t *testing.T) {
	_, a, _, _ := fresh(t)
	attacker := newBrowser(a)
	const wrong = "AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA"
	for i := 0; i < 5; i++ {
		if r := attacker.reset(wrong, otherPass); r.status != 401 {
			t.Fatalf("bad code %d = %d %s", i+1, r.status, r.body)
		}
	}
	if r := attacker.reset(wrong, otherPass); r.status != 429 {
		t.Fatalf("the sixth bad code = %d %s", r.status, r.body)
	}
	if r := newBrowser(a).login(username, goodPass); r.status != 200 {
		t.Fatalf("the password after bad recovery codes = %d %s", r.status, r.body)
	}
}

// A User-Agent with invalid UTF-8 must not turn a correct login into a 500.
func TestLoginWithAnInvalidUserAgentSucceeds(t *testing.T) {
	_, a, _, _ := fresh(t)
	b := newBrowser(a)
	b.state()
	r := b.do("POST", "/api/v1/session/login", map[string]any{"username": username, "password": goodPass}, header("User-Agent", "bad\xff\xfeagent"), header("X-CSRF-Token", b.csrf))
	if r.status != 200 {
		t.Fatalf("login = %d %s", r.status, r.body)
	}
	if got := a.scalar("SELECT user_agent FROM morningstar.sessions ORDER BY created_at DESC LIMIT 1"); got != "badagent" {
		t.Fatalf("stored user agent %q", got)
	}
}

func TestPasswordGuessesShareOneBudgetAcrossLoginAndReauthentication(t *testing.T) {
	_, a, b, _ := fresh(t)
	for i := 0; i < 5; i++ {
		if r := b.post("/api/v1/session/reauthenticate", map[string]any{"password": wrongPass, "purpose": "password_change", "subject": map[string]any{}}); r.status != 401 {
			t.Fatalf("bad reauthentication %d = %d %s", i+1, r.status, r.body)
		}
	}
	if r := newBrowser(a).login(username, goodPass); r.status != 429 {
		t.Fatalf("login after five bad reauthentications = %d %s", r.status, r.body)
	}
}
