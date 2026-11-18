package audit_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
)

const (
	eventID = "0198a2b4-1111-7222-8333-444455556666"
	userID  = "0198a2b4-2222-7222-8333-444455556666"
	reqID   = "0198a2b4-3333-4222-8333-444455556666"
	ruleID  = "0198a2b4-4444-7222-8333-444455556666"
)

func mustUser(t *testing.T) audit.Actor {
	t.Helper()
	a, err := audit.User(userID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func valid(t *testing.T) audit.Input {
	t.Helper()
	return audit.Input{Actor: mustUser(t), Action: "auth.login", Outcome: audit.Success, RequestID: reqID}
}

func detail(t *testing.T, e audit.Event) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.DetailJSON(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNewBuildsAnEventWithEveryField(t *testing.T) {
	in := valid(t)
	in.Reason = "invalid_credentials"
	in.Subject = audit.Subject{ProgramID: ruleID, EntityType: "session", EntityID: userID}
	in.Detail = map[string]any{"sessionId": userID, "count": 3, "ok": true}
	e, err := audit.New(eventID, in)
	if err != nil {
		t.Fatal(err)
	}
	if e.ID() != eventID || e.Action() != "auth.login" || e.Outcome() != audit.Success || e.RequestID() != reqID || e.Reason() != "invalid_credentials" {
		t.Fatalf("event = %+v", e)
	}
	if e.Actor().Kind() != audit.ActorUser || e.Actor().UserID() != userID || e.Subject().ProgramID != ruleID || e.Subject().EntityType != "session" {
		t.Fatalf("actor or subject wrong: %+v %+v", e.Actor(), e.Subject())
	}
	d := detail(t, e)
	if d["reason"] != "invalid_credentials" || d["sessionId"] != userID || d["count"] != float64(3) || d["ok"] != true {
		t.Fatalf("detail = %v", d)
	}
}

func TestNewRefusesAnInvalidEvent(t *testing.T) {
	for name, mod := range map[string]func(*audit.Input){
		"unknown action":       func(in *audit.Input) { in.Action = "auth.made_up" },
		"empty action":         func(in *audit.Input) { in.Action = "" },
		"action with spaces":   func(in *audit.Input) { in.Action = "auth.login " },
		"no outcome":           func(in *audit.Input) { in.Outcome = "" },
		"unknown outcome":      func(in *audit.Input) { in.Outcome = "maybe" },
		"zero actor":           func(in *audit.Input) { in.Actor = audit.Actor{} },
		"request id not uuid":  func(in *audit.Input) { in.RequestID = "not-a-uuid" },
		"request id uppercase": func(in *audit.Input) { in.RequestID = strings.ToUpper(reqID) },
		"bad reason":           func(in *audit.Input) { in.Reason = "Rate Limited!" },
		"reason twice":         func(in *audit.Input) { in.Reason = "x_y"; in.Detail = map[string]any{"reason": "other"} },
		"program id not uuid":  func(in *audit.Input) { in.Subject.ProgramID = "1" },
		"entity id not uuid":   func(in *audit.Input) { in.Subject.EntityID = "../etc" },
		"entity type bad":      func(in *audit.Input) { in.Subject.EntityType = "Session\n" },
		"bad detail key":       func(in *audit.Input) { in.Detail = map[string]any{"Bad Key": 1} },
		"unsupported value":    func(in *audit.Input) { in.Detail = map[string]any{"f": func() {}} },
		"nan":                  func(in *audit.Input) { in.Detail = map[string]any{"f": nan()} },
	} {
		in := valid(t)
		mod(&in)
		if _, err := audit.New(eventID, in); !errors.Is(err, audit.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := audit.New("not-a-uuid", valid(t)); !errors.Is(err, audit.ErrInvalid) {
		t.Errorf("event id: err = %v", err)
	}
}

func nan() float64 {
	var zero float64
	return zero / zero
}

func TestErrorsNameTheFieldAndNeverTheValue(t *testing.T) {
	in := valid(t)
	in.Action = "secret-value-here"
	_, err := audit.New(eventID, in)
	if err == nil || !strings.Contains(err.Error(), "action") || strings.Contains(err.Error(), "secret-value-here") {
		t.Fatalf("err = %v", err)
	}
}

func TestEveryDocumentedActionIsAccepted(t *testing.T) {
	actions := audit.Actions()
	if len(actions) < 90 {
		t.Fatalf("%d actions in the catalog", len(actions))
	}
	for _, a := range actions {
		in := valid(t)
		in.Action = string(a)
		if _, err := audit.New(eventID, in); err != nil {
			t.Errorf("%s: %v", a, err)
		}
	}
	actions[0] = "tampered"
	if audit.Actions()[0] == "tampered" {
		t.Fatal("Actions returned the internal slice")
	}
}

func TestActorKinds(t *testing.T) {
	if _, err := audit.User("not-a-uuid"); !errors.Is(err, audit.ErrInvalid) {
		t.Errorf("user with a bad id: %v", err)
	}
	if a := audit.Anonymous(); a.Kind() != audit.ActorAnonymous || a.UserID() != "" || a.Ref() != "" {
		t.Errorf("anonymous = %+v", a)
	}
	if a := audit.System(); a.Kind() != audit.ActorSystem || a.UserID() != "" {
		t.Errorf("system = %+v", a)
	}
	for name, ctor := range map[string]func(string) (audit.Actor, error){"worker": audit.Worker, "ai": audit.AI} {
		if _, err := ctor(""); !errors.Is(err, audit.ErrInvalid) {
			t.Errorf("%s with no reference: %v", name, err)
		}
		if _, err := ctor("bad ref\n"); !errors.Is(err, audit.ErrInvalid) {
			t.Errorf("%s with a bad reference: %v", name, err)
		}
		if a, err := ctor("worker-1"); err != nil || a.Ref() != "worker-1" || a.UserID() != "" {
			t.Errorf("%s = %+v, %v", name, a, err)
		}
	}
}

func TestFromActorKeepsTheKindsApart(t *testing.T) {
	human, _ := actor.New(actor.KindHuman, userID)
	if a, err := audit.FromActor(human); err != nil || a.Kind() != audit.ActorUser || a.UserID() != userID {
		t.Errorf("human = %+v, %v", a, err)
	}
	sys, _ := actor.New(actor.KindSystem, "scheduler")
	if a, err := audit.FromActor(sys); err != nil || a.Kind() != audit.ActorSystem || a.Ref() != "scheduler" || a.UserID() != "" {
		t.Errorf("system = %+v, %v", a, err)
	}
	ai, _ := actor.New(actor.KindAI, "provider-1")
	if a, err := audit.FromActor(ai); err != nil || a.Kind() != audit.ActorAI || a.Ref() != "provider-1" {
		t.Errorf("ai = %+v, %v", a, err)
	}
	if _, err := audit.FromActor(actor.Actor{}); !errors.Is(err, audit.ErrInvalid) {
		t.Errorf("zero actor: %v", err)
	}
	notUUID, _ := actor.New(actor.KindHuman, "researcher")
	if _, err := audit.FromActor(notUUID); !errors.Is(err, audit.ErrInvalid) {
		t.Errorf("human without a uuid: %v", err)
	}
}

func TestSecretKeysAreRedactedAtEveryDepth(t *testing.T) {
	in := valid(t)
	in.Detail = map[string]any{
		"password":      "hunter2 hunter2",
		"newPassword":   "another one",
		"csrfToken":     "tok",
		"recoveryCode":  "AAAA-BBBB",
		"apiKey":        "k",
		"authorization": "Bearer x",
		"nested":        map[string]any{"clientSecret": "s", "fine": "ok", "inner": map[string]any{"cookie": "c"}},
		"sessionId":     userID,
	}
	e, err := audit.New(eventID, in)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(e.DetailJSON())
	for _, leak := range []string{"hunter2", "another one", `"tok"`, "AAAA-BBBB", `"k"`, "Bearer", `"s"`, `"c"`} {
		if strings.Contains(raw, leak) {
			t.Errorf("detail %s contains %q", raw, leak)
		}
	}
	d := detail(t, e)
	if d["password"] != "[redacted]" || d["sessionId"] != userID {
		t.Errorf("detail = %v", d)
	}
	if n := d["nested"].(map[string]any); n["clientSecret"] != "[redacted]" || n["fine"] != "ok" {
		t.Errorf("nested = %v", n)
	}
}

func TestUntrustedValuesAreMadeSafe(t *testing.T) {
	in := valid(t)
	in.Detail = map[string]any{
		"target": "a\x00b\x1b[31m\r\ncd\xff\xfe" + strings.Repeat("x", 1000),
	}
	e, err := audit.New(eventID, in)
	if err != nil {
		t.Fatal(err)
	}
	got := detail(t, e)["target"].(string)
	if len([]rune(got)) > 256 || strings.ContainsAny(got, "\x00\x1b\r\n\xff") {
		t.Fatalf("value %q was not made safe", got)
	}
	if !strings.HasPrefix(got, "ab[31mcd") {
		t.Fatalf("value %q lost its content", got)
	}
}

func TestDetailLimits(t *testing.T) {
	deep := map[string]any{"a": map[string]any{"b": map[string]any{"c": map[string]any{"d": 1}}}}
	in := valid(t)
	in.Detail = deep
	if _, err := audit.New(eventID, in); !errors.Is(err, audit.ErrInvalid) {
		t.Errorf("nesting too deep: %v", err)
	}
	wide := map[string]any{}
	for i := 0; i < 40; i++ {
		wide["k"+string(rune('a'+i%26))+string(rune('a'+i/26))] = 1
	}
	in.Detail = wide
	if _, err := audit.New(eventID, in); !errors.Is(err, audit.ErrInvalid) {
		t.Errorf("too many keys: %v", err)
	}
	big := map[string]any{}
	for i := 0; i < 24; i++ {
		big["k"+string(rune('a'+i))] = map[string]any{"a": strings.Repeat("y", 256), "b": strings.Repeat("y", 256), "c": strings.Repeat("y", 256)}
	}
	in.Detail = big
	if _, err := audit.New(eventID, in); !errors.Is(err, audit.ErrInvalid) {
		t.Errorf("more than 8192 bytes of detail: %v", err)
	}
}

func TestAnEventCannotBeChangedAfterItIsBuilt(t *testing.T) {
	in := valid(t)
	in.Detail = map[string]any{"list": []any{"a", "b"}, "n": map[string]any{"x": 1}}
	e, err := audit.New(eventID, in)
	if err != nil {
		t.Fatal(err)
	}
	before := string(e.DetailJSON())

	in.Detail["list"].([]any)[0] = "changed"
	in.Detail["n"].(map[string]any)["x"] = 2
	in.Detail["added"] = true
	in.Actor = audit.Anonymous()
	in.Subject.ProgramID = ruleID
	e.DetailJSON()[0] = 'X'
	if string(e.DetailJSON()) != before {
		t.Fatalf("detail changed: %s", e.DetailJSON())
	}
	if e.Actor().Kind() != audit.ActorUser || e.Subject().ProgramID != "" {
		t.Fatal("the actor or the subject changed")
	}
}

func TestAnonymousEventsAreNotSystemEvents(t *testing.T) {
	in := valid(t)
	in.Actor = audit.Anonymous()
	in.Outcome = audit.Denied
	e, err := audit.New(eventID, in)
	if err != nil {
		t.Fatal(err)
	}
	if e.Actor().Kind() == audit.ActorSystem || e.Actor().Kind() != audit.ActorAnonymous {
		t.Fatalf("anonymous event has kind %q", e.Actor().Kind())
	}
}

func TestScopeEvaluationDetailNamesTheDecidingRule(t *testing.T) {
	cases := map[string]struct {
		in   audit.ScopeEvaluation
		want map[string]any
	}{
		"an exclusion denies": {audit.ScopeEvaluation{Allowed: false, Reasons: []string{"excluded"}, DecidingRuleID: ruleID}, map[string]any{"decision": "deny", "decidingRuleId": ruleID}},
		"an inclusion allows": {audit.ScopeEvaluation{Allowed: true, Reasons: []string{"in_scope"}, DecidingRuleID: ruleID}, map[string]any{"decision": "allow", "decidingRuleId": ruleID}},
		"no rule matched":     {audit.ScopeEvaluation{Allowed: false, Reasons: []string{"no_rule_matched"}}, map[string]any{"decision": "deny", "decidingRuleId": nil}},
	}
	for name, c := range cases {
		d, err := c.in.Detail()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		in := valid(t)
		in.Action = "policy.decision_denied"
		in.Detail = d
		e, err := audit.New(eventID, in)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		got := detail(t, e)
		if got["decision"] != c.want["decision"] || got["decidingRuleId"] != c.want["decidingRuleId"] {
			t.Errorf("%s: detail = %v", name, got)
		}
		if _, ok := got["decidingRuleId"]; !ok {
			t.Errorf("%s: the field decidingRuleId is missing, so the shape depends on the result", name)
		}
		if rs, _ := got["reasons"].([]any); len(rs) != 1 {
			t.Errorf("%s: reasons = %v", name, got["reasons"])
		}
	}
	for name, bad := range map[string]audit.ScopeEvaluation{
		"rule id not a uuid": {Reasons: []string{"x"}, DecidingRuleID: "rule-3"},
		"reason with spaces": {Reasons: []string{"not stable"}},
		"too many reasons":   {Reasons: make17()},
	} {
		if _, err := bad.Detail(); !errors.Is(err, audit.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func make17() []string {
	out := make([]string, 17)
	for i := range out {
		out[i] = "reason_code"
	}
	return out
}

func TestAITransmissionDetailCarriesNoContent(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	d, err := audit.AITransmission{Provider: "openrouter", Bytes: 1234, ContentSHA256: digest}.Detail()
	if err != nil {
		t.Fatal(err)
	}
	if d["provider"] != "openrouter" || d["bytes"] != int64(1234) || d["contentDigest"] != digest || len(d) != 3 {
		t.Fatalf("detail = %v", d)
	}
	for name, bad := range map[string]audit.AITransmission{
		"no provider":      {Bytes: 1, ContentSHA256: digest},
		"provider spaces":  {Provider: "a b", Bytes: 1, ContentSHA256: digest},
		"negative size":    {Provider: "p", Bytes: -1, ContentSHA256: digest},
		"short digest":     {Provider: "p", Bytes: 1, ContentSHA256: "abcd"},
		"uppercase digest": {Provider: "p", Bytes: 1, ContentSHA256: strings.ToUpper(digest)},
	} {
		if _, err := bad.Detail(); !errors.Is(err, audit.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRetryPolicyMustBeExplicitAndBounded(t *testing.T) {
	ok := audit.RetryPolicy{MaxAttempts: 3, Delays: []time.Duration{100 * time.Millisecond, 400 * time.Millisecond}, AttemptTimeout: time.Second}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if ok.Delay(1) != 100*time.Millisecond || ok.Delay(2) != 400*time.Millisecond || ok.Delay(3) != 0 || ok.Delay(0) != 0 {
		t.Errorf("delays %v %v %v %v", ok.Delay(0), ok.Delay(1), ok.Delay(2), ok.Delay(3))
	}
	if ok.WorstCase() != 3*time.Second+500*time.Millisecond {
		t.Errorf("worst case = %v", ok.WorstCase())
	}
	for name, p := range map[string]audit.RetryPolicy{
		"zero value":          {},
		"no attempts":         {MaxAttempts: 0, AttemptTimeout: time.Second},
		"too many attempts":   {MaxAttempts: 6, Delays: []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}, AttemptTimeout: time.Second},
		"delay count wrong":   {MaxAttempts: 3, Delays: []time.Duration{time.Millisecond}, AttemptTimeout: time.Second},
		"zero delay":          {MaxAttempts: 2, Delays: []time.Duration{0}, AttemptTimeout: time.Second},
		"negative delay":      {MaxAttempts: 2, Delays: []time.Duration{-time.Second}, AttemptTimeout: time.Second},
		"delay too long":      {MaxAttempts: 2, Delays: []time.Duration{time.Minute}, AttemptTimeout: time.Second},
		"no attempt timeout":  {MaxAttempts: 1},
		"attempt timeout big": {MaxAttempts: 1, AttemptTimeout: time.Minute},
	} {
		if err := p.Validate(); !errors.Is(err, audit.ErrInvalidPolicy) {
			t.Errorf("%s: %v", name, err)
		}
	}
	one := audit.RetryPolicy{MaxAttempts: 1, AttemptTimeout: time.Second}
	if err := one.Validate(); err != nil {
		t.Errorf("a single attempt needs no delays: %v", err)
	}
}

func TestCoalescerAdmitsTheFirstAndCountsTheRest(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := audit.NewCoalescer(15*time.Minute, func() time.Time { return now })
	if rec, n := c.Admit("auth.login"); !rec || n != 0 {
		t.Fatalf("first = %v, %d", rec, n)
	}
	for i := 0; i < 5; i++ {
		now = now.Add(time.Minute)
		if rec, _ := c.Admit("auth.login"); rec {
			t.Fatalf("repeat %d was recorded inside the window", i+1)
		}
	}
	if rec, n := c.Admit("auth.recovery_reset"); !rec || n != 0 {
		t.Fatalf("another key = %v, %d", rec, n)
	}
	now = now.Add(15 * time.Minute)
	if rec, n := c.Admit("auth.login"); !rec || n != 5 {
		t.Fatalf("after the window = %v, %d, want a record that reports 5 suppressed", rec, n)
	}
	if rec, n := c.Admit("auth.login"); rec || n != 0 {
		t.Fatalf("immediately after = %v, %d", rec, n)
	}
}

func TestCoalescerNeverDropsAnEventBecauseItIsFull(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := audit.NewCoalescer(time.Hour, func() time.Time { return now })
	for i := 0; i < 500; i++ {
		key := "k" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+i/676))
		if rec, _ := c.Admit(key); !rec {
			t.Fatalf("the first event of key %d was suppressed", i)
		}
	}
}

func TestMoreSecretKeysAndSecretLookingValuesAreRedacted(t *testing.T) {
	in := valid(t)
	in.Detail = map[string]any{
		"passphrase": "correct horse", "pwd": "x", "signingKey": "k", "jwt": "eyJabc",
		"note": "Bearer eyJhbGciOiJIUzI1NiJ9", "other": "bearer abc", "basic": "Basic dXNlcjpwYXNz",
		"pem": "-----" + "BEGIN PRIVATE KEY" + "-----\nabc", "fine": "a normal sentence",
	}
	e, err := audit.New(eventID, in)
	if err != nil {
		t.Fatal(err)
	}
	d := detail(t, e)
	for _, k := range []string{"passphrase", "pwd", "signingKey", "jwt", "note", "other", "basic", "pem"} {
		if d[k] != "[redacted]" {
			t.Errorf("%s = %v", k, d[k])
		}
	}
	if d["fine"] != "a normal sentence" {
		t.Errorf("fine = %v", d["fine"])
	}
}

func TestInvisibleControlsAreStrippedFromValues(t *testing.T) {
	in := valid(t)
	in.Detail = map[string]any{"target": "a\u202Eb\u200Bc\u2028d\uFEFFe\u2029f"}
	e, err := audit.New(eventID, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := detail(t, e)["target"]; got != "abcdef" {
		t.Fatalf("target = %q", got)
	}
}

func TestAReasonInTheDetailFollowsTheSameRule(t *testing.T) {
	in := valid(t)
	in.Detail = map[string]any{"reason": "free text with spaces"}
	if _, err := audit.New(eventID, in); !errors.Is(err, audit.ErrInvalid) {
		t.Errorf("a free text reason: %v", err)
	}
	in.Detail = map[string]any{"reason": "rate_limited"}
	if _, err := audit.New(eventID, in); err != nil {
		t.Errorf("a stable reason code: %v", err)
	}
}

// jsonb stores a number as a numeric, which can be far longer than the JSON text
// that was sent, so only whole numbers that a float holds exactly are accepted.
func TestNumbersAreWholeAndExact(t *testing.T) {
	for name, v := range map[string]float64{"fraction": 1.5, "huge": 1e300, "beyond exact": 9007199254740993.0 * 2} {
		in := valid(t)
		in.Detail = map[string]any{"n": v}
		if _, err := audit.New(eventID, in); !errors.Is(err, audit.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	in := valid(t)
	in.Detail = map[string]any{"n": float64(3)}
	e, err := audit.New(eventID, in)
	if err != nil || !strings.Contains(string(e.DetailJSON()), `"n":3`) {
		t.Fatalf("%s, %v", e.DetailJSON(), err)
	}
}

// The database limit is on jsonb::text, which is up to one and a half times the
// compact JSON that the domain measures. The domain limit leaves that room.
func TestTheDetailLimitLeavesRoomForTheDatabaseText(t *testing.T) {
	if audit.MaxDetailBytes*3/2 > audit.StoredDetailLimit {
		t.Fatalf("%d compact bytes can grow past %d stored bytes", audit.MaxDetailBytes, audit.StoredDetailLimit)
	}
}

// The Coalescer gives a refusal back when its record was not written, so a
// failed write does not silence the next ones.
func TestACoalescedBurstIsGivenBackWhenItsRecordFailed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := audit.NewCoalescer(15*time.Minute, func() time.Time { return now })
	if rec, _ := c.Admit("auth.login"); !rec {
		t.Fatal("first")
	}
	c.Admit("auth.login")
	c.Admit("auth.login")
	now = now.Add(16 * time.Minute)
	rec, n := c.Admit("auth.login")
	if !rec || n != 2 {
		t.Fatalf("after the window = %v, %d", rec, n)
	}
	c.Retract("auth.login", n)
	rec, n = c.Admit("auth.login")
	if !rec || n != 2 {
		t.Fatalf("after the failed write = %v, %d: the record and its count must come back", rec, n)
	}
}
