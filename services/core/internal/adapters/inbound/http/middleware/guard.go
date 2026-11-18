package middleware

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/request"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/response"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

const principalKey = "principal"

func PrincipalOf(c *gin.Context) inbound.Principal {
	p, _ := c.Get(principalKey)
	pr, _ := p.(inbound.Principal)
	return pr
}

// RecoveryGate answers 503 while the installation needs recovery or has not
// finished its startup check. The routes that must work in that state are
// registered outside the gate.
func RecoveryGate(auth inbound.Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch auth.Gate() {
		case inbound.GateRecoveryRequired:
			response.Error(c, http.StatusServiceUnavailable, response.CodeRecoveryRequired)
		case inbound.GateNotReady:
			response.Error(c, http.StatusServiceUnavailable, response.CodeDatabaseUnavailable)
		default:
			c.Next()
		}
	}
}

// Limiter is a fixed-window counter per key. It is bounded: when it holds too
// many keys it refuses new ones, which fails closed.
type Limiter struct {
	mu      sync.Mutex
	now     func() time.Time
	limit   int
	window  time.Duration
	buckets map[string]*bucket
}

type bucket struct {
	start time.Time
	count int
}

const maxBuckets = 4096

func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{now: time.Now, limit: limit, window: window, buckets: make(map[string]*bucket)}
}

func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok || now.Sub(b.start) >= l.window {
		if !ok && len(l.buckets) >= maxBuckets {
			l.evict(now)
			if len(l.buckets) >= maxBuckets {
				return false, l.window
			}
		}
		b = &bucket{start: now}
		l.buckets[key] = b
	}
	if b.count >= l.limit {
		return false, b.start.Add(l.window).Sub(now)
	}
	b.count++
	return true, 0
}

func (l *Limiter) evict(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.start) >= l.window {
			delete(l.buckets, k)
		}
	}
}

func RateLimitClient(l *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ok, wait := l.Allow(request.ClientKey(c)); !ok {
			response.Detailed(c, http.StatusTooManyRequests, response.CodeRateLimited, nil, wait)
			return
		}
		c.Next()
	}
}

// RateLimitSession counts per session. It runs after Session.
func RateLimitSession(l *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ok, wait := l.Allow(PrincipalOf(c).SessionID()); !ok {
			response.Detailed(c, http.StatusTooManyRequests, response.CodeRateLimited, nil, wait)
			return
		}
		c.Next()
	}
}

// JSONBody refuses a body that is too large or not application/json before the
// handler reads it.
func JSONBody(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch request.LimitJSON(c, limit) {
		case request.BodyTooLarge:
			response.Error(c, http.StatusRequestEntityTooLarge, response.CodePayloadTooLarge)
		case request.BodyWrongMediaType:
			response.Error(c, http.StatusUnsupportedMediaType, response.CodeUnsupportedMediaType)
		default:
			c.Next()
		}
	}
}

// Session resolves the ms_session cookie. Everything below it sees a Principal.
func Session(auth inbound.Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(request.CookieSession)
		if err != nil || token == "" {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthenticated)
			return
		}
		p, err := auth.Authenticate(c.Request.Context(), token)
		if err != nil {
			switch {
			case errors.Is(err, inbound.ErrSessionExpired):
				response.Error(c, http.StatusUnauthorized, response.CodeSessionExpired)
			case errors.Is(err, inbound.ErrUnauthenticated):
				response.Error(c, http.StatusUnauthorized, response.CodeUnauthenticated)
			case errors.Is(err, inbound.ErrDatabaseUnavailable):
				response.Error(c, http.StatusServiceUnavailable, response.CodeDatabaseUnavailable)
			default:
				response.Error(c, http.StatusInternalServerError, response.CodeInternal)
			}
			return
		}
		c.Set(principalKey, p)
		c.Next()
	}
}

// Origin applies to state-changing requests. The Origin header is required and
// must equal an allow-list entry. A missing Origin, "null" and a cross-site
// Sec-Fetch-Site are refused. This is deliberately stricter than the standard
// library's CrossOriginProtection, which accepts a request without Origin (P-60).
func Origin(allowed []string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		set[strings.ToLower(o)] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := set[strings.ToLower(origin)]; !ok || origin == "" || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			response.Error(c, http.StatusForbidden, response.CodeOriginDenied)
			return
		}
		c.Next()
	}
}

// CSRFSession compares X-CSRF-Token with the token of the session.
func CSRFSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !PrincipalOf(c).ValidCSRF(c.GetHeader(request.HeaderCSRF)) {
			response.Error(c, http.StatusForbidden, response.CodeCSRFInvalid)
			return
		}
		c.Next()
	}
}

// CSRFAnon compares X-CSRF-Token with the token bound to the ms_anon cookie. It
// protects the public state-changing routes against login CSRF.
func CSRFAnon(auth inbound.Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		anon, _ := c.Cookie(request.CookieAnon)
		if anon == "" || !auth.ValidAnonCSRF(anon, c.GetHeader(request.HeaderCSRF)) {
			response.Error(c, http.StatusForbidden, response.CodeCSRFInvalid)
			return
		}
		c.Next()
	}
}
