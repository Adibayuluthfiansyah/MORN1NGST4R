package audit

import (
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// StoredDetailLimit is the limit of the database: audit_events_ck_c5 checks
// octet_length(detail::text) <= 8192, and jsonb::text puts a space after every
// colon and comma. MaxDetailBytes is measured on the compact JSON that the
// domain builds. Compact JSON grows by at most half when jsonb prints it, so the
// limit leaves that room and a detail that audit.New accepts is never refused by
// the database (a refusal would raise the alert that closes the gate).
const (
	StoredDetailLimit = 8192
	MaxDetailBytes    = 4096
)

const (
	maxDepth     = 2
	maxEntries   = 24
	maxStringLen = 256
	redacted     = "[redacted]"
)

var keyPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9_]{0,47}$`)

// secretFragments are the parts of a key that mark a secret. A key is compared
// in lower case without underscores, so "csrfToken", "csrf_token" and
// "newPassword" are all caught. Its value is replaced and the event is still
// written: a caller mistake must not cost the record.
var secretFragments = []string{
	"password", "passwd", "secret", "token", "apikey", "authorization",
	"cookie", "credential", "recoverycode", "privatekey", "bearer",
	"passphrase", "pwd", "signingkey", "jwt",
}

// looksSecret reports a value that carries a credential whatever its key is:
// an authorization header value or a private key block.
func looksSecret(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(l, "bearer ") || strings.HasPrefix(l, "basic ") || strings.Contains(l, "-----begin")
}

func isSecretKey(k string) bool {
	n := strings.ToLower(strings.ReplaceAll(k, "_", ""))
	for _, f := range secretFragments {
		if strings.Contains(n, f) {
			return true
		}
	}
	return false
}

// cleanString makes untrusted text safe to store and show: valid UTF-8, no
// control or format characters (the right-to-left override and the zero width
// characters among them) and no line or paragraph separators, at most
// maxStringLen characters.
func cleanString(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	if utf8.RuneCountInString(s) > maxStringLen {
		s = string([]rune(s)[:maxStringLen])
	}
	return s
}

func sanitizeObject(m map[string]any, depth int) (map[string]any, error) {
	if depth > maxDepth || len(m) > maxEntries {
		return nil, invalid("detail")
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if !keyPattern.MatchString(k) {
			return nil, invalid("detail")
		}
		if isSecretKey(k) {
			out[k] = redacted
			continue
		}
		c, err := sanitizeValue(v, depth)
		if err != nil {
			return nil, err
		}
		out[k] = c
	}
	return out, nil
}

func sanitizeValue(v any, depth int) (any, error) {
	switch x := v.(type) {
	case nil, bool:
		return x, nil
	case string:
		if c := cleanString(x); !looksSecret(c) {
			return c, nil
		}
		return redacted, nil
	case int:
		return int64(x), nil
	case int8:
		return int64(x), nil
	case int16:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int64:
		return x, nil
	case uint:
		return uint64(x), nil
	case uint8:
		return uint64(x), nil
	case uint16:
		return uint64(x), nil
	case uint32:
		return uint64(x), nil
	case uint64:
		return x, nil
	case float64:
		// jsonb stores a number as a numeric, which can be much longer than the
		// text that was sent, so only whole numbers that a float holds exactly pass.
		if math.IsNaN(x) || math.IsInf(x, 0) || x != math.Trunc(x) || math.Abs(x) > 1<<53 {
			return nil, invalid("detail")
		}
		return int64(x), nil
	case []string:
		if depth+1 > maxDepth || len(x) > maxEntries {
			return nil, invalid("detail")
		}
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = cleanString(s)
		}
		return out, nil
	case []any:
		if depth+1 > maxDepth || len(x) > maxEntries {
			return nil, invalid("detail")
		}
		out := make([]any, len(x))
		for i, e := range x {
			c, err := sanitizeValue(e, depth+1)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case map[string]any:
		return sanitizeObject(x, depth+1)
	}
	return nil, invalid("detail")
}

var (
	reasonCode = reasonPattern
	providerID = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
	sha256Hex  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const maxReasons = 16

// ScopeEvaluation is the detail of the event of a scope evaluation (SEC 13).
// It has a field for the rule that decided, whether an exclusion denied or an
// inclusion allowed (D-91). The field is always present, and null when no rule
// decided, so the shape does not depend on the result. scope.Decision does not
// name its rule yet, so no producer fills DecidingRuleID before the work of
// D-91 is done.
type ScopeEvaluation struct {
	Allowed        bool
	Reasons        []string
	DecidingRuleID string
}

// Detail returns the detail map for Input.Detail.
func (s ScopeEvaluation) Detail() (map[string]any, error) {
	if len(s.Reasons) > maxReasons {
		return nil, invalid("detail")
	}
	for _, r := range s.Reasons {
		if !reasonCode.MatchString(r) {
			return nil, invalid("detail")
		}
	}
	var rule any
	if s.DecidingRuleID != "" {
		if !isUUID(s.DecidingRuleID) {
			return nil, invalid("detail")
		}
		rule = s.DecidingRuleID
	}
	decision := "deny"
	if s.Allowed {
		decision = "allow"
	}
	return map[string]any{
		"decision": decision, "reasons": append([]string(nil), s.Reasons...), "decidingRuleId": rule,
	}, nil
}

// AITransmission is the detail of an AI provider transmission (FR-AI-004): the
// provider, the size and the SHA-256 of the content. It has no field for the
// content.
type AITransmission struct {
	Provider      string
	Bytes         int64
	ContentSHA256 string
}

func (a AITransmission) Detail() (map[string]any, error) {
	if !providerID.MatchString(a.Provider) || a.Bytes < 0 || !sha256Hex.MatchString(a.ContentSHA256) {
		return nil, invalid("detail")
	}
	return map[string]any{"provider": a.Provider, "bytes": a.Bytes, "contentDigest": a.ContentSHA256}, nil
}
