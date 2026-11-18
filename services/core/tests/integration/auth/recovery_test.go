package auth_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (b *browser) reset(code, newPass string) *reply {
	b.state()
	r := b.do("POST", "/api/v1/recovery/reset", map[string]any{"recoveryCode": code, "newPassword": newPass})
	return r
}

func canonical(code string) string { return strings.ToUpper(strings.ReplaceAll(code, "-", "")) }

func TestRecoveryCodeResetsThePasswordOnceAndRevokesEverySession(t *testing.T) {
	_, a, b, code := fresh(t)
	if len(canonical(code)) != 32 {
		t.Fatalf("recovery code %q is not 160 bits", code)
	}
	other := newBrowser(a)
	other.login(username, goodPass)

	nb := newBrowser(a)
	if r := nb.reset("AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA", otherPass); r.status != 401 || r.code() != "recovery_invalid" {
		t.Errorf("wrong code = %d %s", r.status, r.body)
	}
	if r := nb.reset(code, "short"); r.status != 422 {
		t.Errorf("a weak new password = %d %s", r.status, r.body)
	}

	r := nb.reset(strings.ToLower(strings.ReplaceAll(code, "-", " ")), otherPass)
	if r.status != 200 || r.str("recoveryCode") == "" || r.str("recoveryCode") == code {
		t.Fatalf("reset = %d %s", r.status, r.body)
	}
	newCode := r.str("recoveryCode")
	if nb.cookies["ms_session"] == "" {
		t.Error("the reset issued no new session")
	}
	if r := other.do("GET", "/api/v1/sessions", nil); r.status != 401 {
		t.Errorf("an older session survived the reset: %d", r.status)
	}
	if r := b.do("GET", "/api/v1/sessions", nil); r.status != 401 {
		t.Errorf("the session of the setup survived the reset: %d", r.status)
	}
	if r := newBrowser(a).login(username, goodPass); r.status != 401 {
		t.Errorf("the old password still works: %d", r.status)
	}
	if r := newBrowser(a).login(username, otherPass); r.status != 200 {
		t.Errorf("the new password does not work: %d %s", r.status, r.body)
	}
	if r := newBrowser(a).reset(code, "yet another long phrase 7"); r.status != 401 || r.code() != "recovery_invalid" {
		t.Errorf("a used code = %d %s", r.status, r.code())
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.recovery_codes WHERE used_at IS NOT NULL`); got != "1" {
		t.Errorf("%s used codes", got)
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.recovery_codes WHERE used_at IS NULL AND revoked_at IS NULL`); got != "1" {
		t.Errorf("%s live codes, want exactly one", got)
	}
	if r := newBrowser(a).reset(newCode, "yet another long phrase 7"); r.status != 200 {
		t.Errorf("the new code = %d %s", r.status, r.body)
	}
	if a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'auth.recovery_reset' AND outcome = 'success'`) != "2" ||
		a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'auth.recovery_reset' AND outcome = 'denied'`) != "2" {
		t.Error("the resets and the refusals are not audited")
	}
}

// The answer does not say whether a code exists, how it is malformed, or why.
func TestRecoveryAnswersAreIdenticalWhateverIsWrong(t *testing.T) {
	_, a, _, _ := fresh(t)
	nb := newBrowser(a)
	answers := map[string]*reply{
		"wrong":     nb.reset("AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA", otherPass),
		"malformed": nb.reset("not a code", otherPass),
		"empty":     nb.reset("", otherPass),
	}
	strip := func(r *reply) string {
		m := r.json()
		delete(m["error"].(map[string]any), "requestId")
		return fmt.Sprint(m)
	}
	for name, r := range answers {
		if r.status != 401 || r.code() != "recovery_invalid" || strip(r) != strip(answers["wrong"]) {
			t.Errorf("%s = %d %s", name, r.status, r.body)
		}
	}
}

func TestRecoveryAttemptsAreRateLimited(t *testing.T) {
	_, a, _, code := fresh(t)
	nb := newBrowser(a)
	for i := 0; i < 5; i++ {
		if r := nb.reset("AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAA"+string(rune('A'+i)), otherPass); r.status != 401 {
			t.Fatalf("attempt %d = %d", i, r.status)
		}
	}
	if r := nb.reset(code, otherPass); r.status != 429 {
		t.Errorf("the right code after 5 failures = %d, want 429", r.status)
	}
	if a.scalar(`SELECT count(*) FROM morningstar.recovery_codes WHERE used_at IS NOT NULL`) != "0" {
		t.Error("the code was spent while rate limited")
	}
}

func TestPasswordChangeNeedsAFreshGrantAndTheCurrentPassword(t *testing.T) {
	_, a, b, _ := fresh(t)
	other := newBrowser(a)
	other.login(username, goodPass)

	change := func(current, next, grant string) *reply {
		return b.post("/api/v1/account/password", map[string]any{"currentPassword": current, "newPassword": next, "reauthGrantId": grant})
	}
	mint := func(purpose string, subject map[string]any) string {
		r := b.reauth(goodPass, purpose, subject)
		if r.status != 200 {
			t.Fatalf("reauthenticate = %d %s", r.status, r.body)
		}
		return r.str("grantId")
	}
	if r := change(goodPass, otherPass, ""); r.status != 403 || r.code() != "reauth_required" {
		t.Errorf("no grant = %d %s", r.status, r.code())
	}
	if r := change(goodPass, otherPass, mint("verification", map[string]any{"x": "y"})); r.status != 403 || r.code() != "reauth_invalid" {
		t.Errorf("a grant of another purpose = %d %s", r.status, r.code())
	}
	grant := mint("password_change", map[string]any{})
	if r := change(wrongPass, otherPass, grant); r.status != 401 || r.code() != "invalid_credentials" {
		t.Errorf("wrong current password = %d %s", r.status, r.code())
	}
	if r := change(goodPass, "short", grant); r.status != 422 {
		t.Errorf("weak new password = %d", r.status)
	}
	if a.scalar(`SELECT count(*) FROM morningstar.reauth_grants WHERE consumed_at IS NOT NULL`) != "0" {
		t.Fatal("a refused change spent the grant")
	}
	old := b.cookies["ms_session"]
	r := change(goodPass, otherPass, grant)
	if r.status != 204 || b.cookies["ms_session"] == "" || b.cookies["ms_session"] == old {
		t.Fatalf("change = %d %s, cookie %q", r.status, r.body, b.cookies["ms_session"])
	}
	if r := other.do("GET", "/api/v1/sessions", nil); r.status != 401 {
		t.Errorf("another session survived the change: %d", r.status)
	}
	stale := newBrowser(a)
	stale.cookies["ms_session"] = old
	if r := stale.do("GET", "/api/v1/sessions", nil); r.status != 401 {
		t.Errorf("the session token from before the change still works: %d", r.status)
	}
	if r := newBrowser(a).login(username, goodPass); r.status != 401 {
		t.Errorf("the old password works: %d", r.status)
	}
	if r := newBrowser(a).login(username, otherPass); r.status != 200 {
		t.Errorf("the new password does not work: %d", r.status)
	}
	if a.scalar(`SELECT count(*) FROM morningstar.reauth_grants WHERE consumed_at IS NOT NULL`) != "1" {
		t.Error("the grant was not spent by the change")
	}
	if a.scalar(`SELECT count(*) FROM morningstar.user_credentials WHERE is_current`) != "1" || a.scalar(`SELECT count(*) FROM morningstar.user_credentials`) != "2" {
		t.Error("the credential history is wrong")
	}
	// The grant cannot be used again, from the new session either.
	b.state()
	if r := change(otherPass, "a third long passphrase 9", grant); r.status != 403 || r.code() != "reauth_invalid" {
		t.Errorf("a spent grant = %d %s", r.status, r.code())
	}
}

func removeMarker(t *testing.T, a *app) {
	t.Helper()
	if err := os.Remove(filepath.Join(a.state, "recovery-generation.json")); err != nil {
		t.Fatal(err)
	}
}

// TD-M3-06 (M4 part): while recovery is required only GET /session and the
// reconcile route work, and the reconcile route needs the password.
func TestRecoveryRequiredGatesEveryRouteAndReconcileNeedsThePassword(t *testing.T) {
	_, a, b, code := fresh(t)
	oldSession := b.cookies["ms_session"]
	a.stop()
	removeMarker(t, a)
	a = start(t, a.c, options{runtime: a.runtime, state: a.state})

	nb := newBrowser(a)
	s := nb.state()
	if s.status != 200 || s.str("state") != "recovery_required" || s.str("recovery", "reason") != "marker_missing" || s.str("csrfToken") == "" {
		t.Fatalf("state = %d %s", s.status, s.body)
	}
	if m := s.json()["recovery"].(map[string]any); m["markerGeneration"] != nil || m["databaseGeneration"] != float64(1) || m["detectedAt"] == nil {
		t.Errorf("recovery info = %v", m)
	}
	if nb.cookies["ms_anon"] == "" {
		t.Error("no pre-session cookie for the reconcile call")
	}
	nb.cookies["ms_session"] = oldSession
	gated := map[string]*reply{
		"login":    nb.do("POST", "/api/v1/session/login", map[string]any{"username": username, "password": goodPass}),
		"setup":    nb.do("POST", "/api/v1/setup/account", map[string]any{"bootstrapSecret": "x", "username": username, "password": goodPass}),
		"reset":    nb.do("POST", "/api/v1/recovery/reset", map[string]any{"recoveryCode": code, "newPassword": otherPass}),
		"sessions": nb.do("GET", "/api/v1/sessions", nil),
		"logout":   nb.do("POST", "/api/v1/session/logout", nil),
	}
	for name, r := range gated {
		if r.status != 503 || r.code() != "recovery_required" || r.header.Get("Retry-After") == "" {
			t.Errorf("%s = %d %s", name, r.status, r.body)
		}
	}
	if r := nb.do("GET", "/api/v1/health/live", nil); r.status != 200 {
		t.Errorf("liveness = %d", r.status)
	}

	reconcile := func(pass string, confirm bool) *reply {
		return nb.do("POST", "/api/v1/system/recovery/reconcile", map[string]any{"password": pass, "newPassword": otherPass, "confirmRevocation": confirm})
	}
	if r := reconcile(goodPass, false); r.status != 422 {
		t.Errorf("without the confirmation = %d %s", r.status, r.body)
	}
	if r := reconcile(wrongPass, true); r.status != 401 || r.code() != "invalid_credentials" {
		t.Errorf("a wrong password = %d %s", r.status, r.body)
	}
	if a.scalar(`SELECT count(*) FROM morningstar.recovery_state WHERE state = 'normal' AND generation = 1`) != "1" {
		t.Fatal("a refused reconciliation changed the recovery state")
	}
	for _, mod := range []mod{noHeader("Origin"), header("Origin", "http://evil.example"), header("X-CSRF-Token", "forged")} {
		if r := nb.do("POST", "/api/v1/system/recovery/reconcile", map[string]any{"password": goodPass, "newPassword": otherPass, "confirmRevocation": true}, mod); r.status != 403 {
			t.Errorf("a reconcile without the browser checks = %d", r.status)
		}
	}

	r := reconcile(goodPass, true)
	if r.status != 200 || r.json()["generation"] != float64(2) || r.json()["revokedSessions"].(float64) < 1 || r.str("recoveryCode") == "" {
		t.Fatalf("reconcile = %d %s", r.status, r.body)
	}
	newCode := r.str("recoveryCode")
	if r := reconcile(goodPass, true); r.status != 409 || r.code() != "recovery_not_required" {
		t.Errorf("a second reconcile = %d %s", r.status, r.code())
	}

	// The installation is normal again and nothing from before survived.
	if r := newBrowser(a).do("GET", "/api/v1/sessions", nil, header("Cookie", "ms_session="+oldSession)); r.status != 401 {
		t.Errorf("the old session works after the reconciliation: %d", r.status)
	}
	if r := newBrowser(a).login(username, otherPass); r.status != 200 {
		t.Errorf("login with the new password after the reconciliation = %d %s", r.status, r.body)
	}
	if r := newBrowser(a).login(username, goodPass); r.status != 401 {
		t.Errorf("the restored password works after the reconciliation = %d %s", r.status, r.body)
	}
	// TD-M3-04: the recovery code that the restored database held is revoked.
	if r := newBrowser(a).reset(code, otherPass); r.status != 401 || r.code() != "recovery_invalid" {
		t.Errorf("the old recovery code = %d %s", r.status, r.code())
	}
	if r := newBrowser(a).reset(newCode, otherPass); r.status != 200 {
		t.Errorf("the new recovery code = %d %s", r.status, r.body)
	}
	if a.scalar(`SELECT generation FROM morningstar.recovery_state`) != "2" {
		t.Error("the generation did not move")
	}
}

// TD-M3-04: a live bootstrap secret in a restored database is expired.
func TestReconciliationExpiresBootstrapSecretsAndRecoveryCodes(t *testing.T) {
	_, a, _, _ := fresh(t)
	a.stop()
	removeMarker(t, a)
	a = start(t, a.c, options{runtime: a.runtime, state: a.state})
	a.exec(`INSERT INTO morningstar.bootstrap_secrets (id, secret_hash) VALUES (uuidv7(), 'sha256:restored')`)
	b := newBrowser(a)
	b.state()
	if r := b.do("POST", "/api/v1/system/recovery/reconcile", map[string]any{"password": goodPass, "newPassword": otherPass, "confirmRevocation": true}); r.status != 200 {
		t.Fatalf("reconcile = %d %s", r.status, r.body)
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.bootstrap_secrets WHERE consumed_at IS NULL AND superseded_at IS NULL`); got != "0" {
		t.Errorf("%s live bootstrap secrets after the reconciliation", got)
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.recovery_codes WHERE used_at IS NULL AND revoked_at IS NULL`); got != "1" {
		t.Errorf("%s live recovery codes after the reconciliation, want the one new code", got)
	}
}

// NFR-REL-004 and TD-M3-04: restoring an older database must not bring back a
// recovery code that was already used.
func TestRestoredDatabaseDoesNotReactivateAConsumedRecoveryCode(t *testing.T) {
	c, a, _, code := fresh(t)
	c.Snapshot(t, "snap_before_reset")
	newBrowser(a).state() // the first request after the snapshot meets the closed connections
	if r := newBrowser(a).reset(code, otherPass); r.status != 200 {
		t.Fatalf("reset = %d %s", r.status, r.body)
	}
	rt, state := a.runtime, a.state
	a.stop()

	c.Restore(t, "snap_before_reset")
	if err := os.Remove(filepath.Join(state, "recovery-generation.json")); err != nil {
		t.Fatal(err)
	}
	a = start(t, c, options{runtime: rt, state: state})
	if got := a.scalar(`SELECT count(*) FROM morningstar.recovery_codes WHERE used_at IS NULL AND revoked_at IS NULL`); got != "1" {
		t.Fatalf("the restored database should hold the unused code again, found %s", got)
	}
	nb := newBrowser(a)
	if s := nb.state(); s.str("state") != "recovery_required" {
		t.Fatalf("state = %s", s.body)
	}
	if r := nb.reset(code, "an unrelated long phrase 3"); r.status != 503 {
		t.Errorf("the reactivated code works before the reconciliation: %d", r.status)
	}
	if r := nb.do("POST", "/api/v1/system/recovery/reconcile", map[string]any{"password": goodPass, "newPassword": otherPass, "confirmRevocation": true}); r.status != 200 {
		t.Fatalf("reconcile = %d %s", r.status, r.body)
	}
	if r := newBrowser(a).reset(code, "an unrelated long phrase 3"); r.status != 401 {
		t.Errorf("the reactivated code works after the reconciliation: %d %s", r.status, r.body)
	}
}

// The reconciliation asks for the restored password and sets a new one in the
// same step, so a password that was rotated before the backup was taken does not
// come back. A refused request changes nothing.
func TestReconcileSetsTheNewPasswordAndKeepsTheOldOneOnRefusal(t *testing.T) {
	_, a, b, _ := fresh(t)
	oldSession := b.cookies["ms_session"]
	a.stop()
	removeMarker(t, a)
	a = start(t, a.c, options{runtime: a.runtime, state: a.state})
	hash := func() string {
		return a.scalar(`SELECT phc_hash FROM morningstar.user_credentials WHERE is_current`)
	}
	before := hash()

	nb := newBrowser(a)
	nb.state()
	reconcile := func(body map[string]any) *reply {
		return nb.do("POST", "/api/v1/system/recovery/reconcile", body)
	}
	field := func(r *reply) string {
		fe, _ := r.json()["error"].(map[string]any)["fieldErrors"].([]any)
		if len(fe) == 0 {
			return ""
		}
		f, _ := fe[0].(map[string]any)["field"].(string)
		return f
	}
	refused := map[string]struct {
		body   map[string]any
		status int
		field  string
	}{
		"no new password":   {map[string]any{"password": goodPass, "confirmRevocation": true}, 422, "newPassword"},
		"a weak new one":    {map[string]any{"password": goodPass, "newPassword": "short", "confirmRevocation": true}, 422, "newPassword"},
		"the same password": {map[string]any{"password": goodPass, "newPassword": goodPass, "confirmRevocation": true}, 422, "newPassword"},
		"a wrong old one":   {map[string]any{"password": wrongPass, "newPassword": otherPass, "confirmRevocation": true}, 401, ""},
		"no confirmation":   {map[string]any{"password": goodPass, "newPassword": otherPass, "confirmRevocation": false}, 422, "confirmRevocation"},
	}
	for name, c := range refused {
		r := reconcile(c.body)
		if r.status != c.status || (c.field != "" && field(r) != c.field) {
			t.Errorf("%s = %d %s", name, r.status, r.body)
		}
		if hash() != before {
			t.Fatalf("%s changed the password", name)
		}
	}
	if a.scalar(`SELECT count(*) FROM morningstar.recovery_state WHERE state = 'normal' AND generation = 1`) != "1" {
		t.Fatal("a refused reconciliation changed the recovery state")
	}

	r := reconcile(map[string]any{"password": goodPass, "newPassword": otherPass, "confirmRevocation": true})
	if r.status != 200 || r.str("recoveryCode") == "" {
		t.Fatalf("reconcile = %d %s", r.status, r.body)
	}
	if hash() == before {
		t.Fatal("the password was not replaced")
	}
	if got := a.scalar(`SELECT count(*) FILTER (WHERE is_current) || '/' || count(*) FROM morningstar.user_credentials`); got != "1/2" {
		t.Errorf("current/all credentials = %s, want 1/2", got)
	}
	if r := newBrowser(a).login(username, goodPass); r.status != 401 || r.code() != "invalid_credentials" {
		t.Errorf("the restored password = %d %s", r.status, r.body)
	}
	if r := newBrowser(a).login(username, otherPass); r.status != 200 {
		t.Errorf("the new password = %d %s", r.status, r.body)
	}
	if r := newBrowser(a).do("GET", "/api/v1/sessions", nil, header("Cookie", "ms_session="+oldSession)); r.status != 401 {
		t.Errorf("the old session works after the reconciliation: %d", r.status)
	}
	if a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'system.restore_recovered' AND outcome = 'success'`) != "1" {
		t.Error("the reconciliation was not audited")
	}
	// Every count of the event is a number: no key of the counts looks like a
	// secret to the redaction (the review of M5).
	if got := a.scalar(`SELECT count(*) FILTER (WHERE jsonb_typeof(c.value) <> 'number') || '/' || count(*) FROM morningstar.audit_events e, jsonb_each(e.detail->'counts') c WHERE e.action = 'system.restore_recovered'`); !strings.HasPrefix(got, "0/") || got == "0/0" {
		t.Errorf("non-numeric/all counts of the event = %s", got)
	}
}
