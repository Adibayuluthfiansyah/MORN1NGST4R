package identity_test

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
)

func TestNormalizeUsername(t *testing.T) {
	good := map[string]string{"researcher": "researcher", "Research.er-1": "research.er-1", "abc": "abc", strings.Repeat("a", 64): strings.Repeat("a", 64)}
	for in, want := range good {
		got, err := identity.NormalizeUsername(in)
		if err != nil || got != want {
			t.Errorf("NormalizeUsername(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "ab", strings.Repeat("a", 65), "-abc", ".abc", "_abc", "a b c", "abc\n", "ünï", "abc@x", "a/b", "ａｂｃ", "abc\x00"} {
		if _, err := identity.NormalizeUsername(in); err == nil {
			t.Errorf("NormalizeUsername(%q) was accepted", in)
		}
	}
}

func TestValidateNewPassword(t *testing.T) {
	ok := []string{"correct horse battery", strings.Repeat("é", 12), "pass word 1234", strings.Repeat("a", 128)}
	for _, p := range ok {
		if err := identity.ValidateNewPassword("researcher", p); err != nil {
			t.Errorf("%q refused: %v", p, err)
		}
	}
	bad := []string{"", "short", strings.Repeat("a", 11), strings.Repeat("a", 129), "twelve chars\x00", "tab\tseparated pw", "\xff\xfe\xfdbad utf8 bytes", "researcherXXXX"[:0]}
	for _, p := range bad {
		if err := identity.ValidateNewPassword("researcher", p); err == nil {
			t.Errorf("%q accepted", p)
		}
	}
	if err := identity.ValidateNewPassword("long.username.here", "LONG.USERNAME.HERE"); err == nil {
		t.Error("a password equal to the username was accepted")
	}
}

func TestValidatePasswordInputBoundsWhatIsHashed(t *testing.T) {
	if identity.ValidatePasswordInput("") == nil || identity.ValidatePasswordInput(strings.Repeat("a", 4*identity.MaxPassword+1)) == nil || identity.ValidatePasswordInput("\xff") == nil {
		t.Error("an empty, oversized or invalid input was accepted")
	}
	if err := identity.ValidatePasswordInput("x"); err != nil {
		t.Error(err)
	}
}

func TestRecoveryCodeFormatAndCanonicalForm(t *testing.T) {
	raw := make([]byte, identity.RecoveryCodeBytes)
	for i := range raw {
		raw[i] = byte(i * 7)
	}
	shown, canonical, err := identity.FormatRecoveryCode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical) != 32 || len(shown) != 32+7 || strings.Count(shown, "-") != 7 {
		t.Fatalf("shown %q canonical %q", shown, canonical)
	}
	for _, in := range []string{shown, strings.ToLower(shown), " " + strings.ReplaceAll(shown, "-", " ") + " ", canonical} {
		got, ok := identity.CanonicalRecoveryCode(in)
		if !ok || got != canonical {
			t.Errorf("CanonicalRecoveryCode(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", canonical[:31], canonical + "A", strings.Replace(canonical, canonical[:1], "1", 1), strings.Repeat("A", 200), canonical[:31] + "!"} {
		if _, ok := identity.CanonicalRecoveryCode(in); ok {
			t.Errorf("CanonicalRecoveryCode(%q) was accepted", in)
		}
	}
	if _, _, err := identity.FormatRecoveryCode(raw[:19]); err == nil {
		t.Error("a short random input was accepted")
	}
	a, _, _ := identity.FormatRecoveryCode(make([]byte, 20))
	b, _, _ := identity.FormatRecoveryCode(append(make([]byte, 19), 1))
	if a == b {
		t.Error("two different random inputs gave the same code")
	}
}

func TestParsePurpose(t *testing.T) {
	for _, p := range []string{"approval_intrusive", "policy_widening", "scope_widening", "lab_registration", "exclusion_acknowledgement", "raw_evidence_capture", "provider_enablement", "tool_registration", "passive_source_enablement", "verification", "password_change"} {
		if got, ok := identity.ParsePurpose(p); !ok || string(got) != p {
			t.Errorf("ParsePurpose(%q) = %q, %v", p, got, ok)
		}
	}
	for _, p := range []string{"", "Approval_Intrusive", "approval", "approval_intrusive ", "approve"} {
		if _, ok := identity.ParsePurpose(p); ok {
			t.Errorf("ParsePurpose(%q) was accepted", p)
		}
	}
}

func TestSubjectHashBindsPurposeAndEveryField(t *testing.T) {
	base := map[string]any{"operationHash": "ab", "destinationsHash": "cd"}
	h, err := identity.SubjectHash(identity.PurposeApprovalIntrusive, base)
	if err != nil {
		t.Fatal(err)
	}
	same, _ := identity.SubjectHash(identity.PurposeApprovalIntrusive, map[string]any{"destinationsHash": "cd", "operationHash": "ab"})
	if h != same {
		t.Error("key order changed the hash")
	}
	other := map[string]struct {
		p identity.Purpose
		s map[string]any
	}{
		"other purpose":  {identity.PurposeScopeWidening, base},
		"other value":    {identity.PurposeApprovalIntrusive, map[string]any{"operationHash": "ab", "destinationsHash": "ce"}},
		"missing field":  {identity.PurposeApprovalIntrusive, map[string]any{"operationHash": "ab"}},
		"extra field":    {identity.PurposeApprovalIntrusive, map[string]any{"operationHash": "ab", "destinationsHash": "cd", "x": true}},
		"quote smuggled": {identity.PurposeApprovalIntrusive, map[string]any{"operationHash": "ab\",\"destinationsHash\":\"cd", "destinationsHash": ""}},
	}
	for name, o := range other {
		got, err := identity.SubjectHash(o.p, o.s)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got == h {
			t.Errorf("%s gave the same hash", name)
		}
	}
	for name, v := range map[string]any{"int and its string differ": 5, "bool": true} {
		a, _ := identity.SubjectHash(identity.PurposeVerification, map[string]any{"n": v})
		b, _ := identity.SubjectHash(identity.PurposeVerification, map[string]any{"n": "5"})
		if name == "int and its string differ" && a == b {
			t.Error("the integer 5 and the string \"5\" gave the same hash")
		}
	}
	if a, _ := identity.SubjectHash(identity.PurposeVerification, map[string]any{"n": float64(5)}); a != mustHash(t, identity.PurposeVerification, map[string]any{"n": 5}) {
		t.Error("a JSON number 5 and the integer 5 differ")
	}
}

func mustHash(t *testing.T, p identity.Purpose, s map[string]any) [32]byte {
	t.Helper()
	h, err := identity.SubjectHash(p, s)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSubjectHashRefusesWhatItCannotCanonicalize(t *testing.T) {
	bad := map[string]map[string]any{
		"nested object":   {"a": map[string]any{"b": 1}},
		"array":           {"a": []any{1}},
		"null":            {"a": nil},
		"fraction":        {"a": 1.5},
		"huge number":     {"a": float64(1 << 60)},
		"bad key":         {"Bad": "x"},
		"empty key":       {"": "x"},
		"key with quote":  {"a\"b": "x"},
		"long key":        {strings.Repeat("a", 33): "x"},
		"invalid utf8":    {"a": "\xff"},
		"nul":             {"a": "x\x00"},
		"long string":     {"a": strings.Repeat("x", 513)},
		"too many fields": manyKeys(17),
	}
	for name, s := range bad {
		if _, err := identity.SubjectHash(identity.PurposeVerification, s); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, err := identity.SubjectHash(identity.Purpose("nope"), map[string]any{}); err == nil {
		t.Error("an unknown purpose was accepted")
	}
	if _, err := identity.SubjectHash(identity.PurposePasswordChange, map[string]any{"a": "b"}); err == nil {
		t.Error("a password change with a subject was accepted")
	}
	got, err := identity.SubjectHash(identity.PurposePasswordChange, map[string]any{})
	if err != nil || got != sha256.Sum256([]byte("password_change\n{}")) {
		t.Errorf("the fixed password change subject = %x, %v", got, err)
	}
}

func manyKeys(n int) map[string]any {
	m := map[string]any{}
	for i := 0; i < n; i++ {
		m["k"+strings.Repeat("a", i)] = 1
	}
	return m
}

func TestAttemptsExhausted(t *testing.T) {
	for n, want := range map[int]bool{0: false, 4: false, 5: true, 6: true} {
		if got := identity.AttemptsExhausted(n); got != want {
			t.Errorf("AttemptsExhausted(%d) = %v", n, got)
		}
	}
}
