// Package middleware holds the Gin middleware of the inbound HTTP adapter.
package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/request"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/response"
)

// RequestID assigns the request id (API-13). The server makes it: the id is the
// correlation id of the audit trail, so a value that the caller sends is never
// kept and never echoed (TD-M4-03).
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		request.SetID(c, request.NewID())
		c.Header(request.HeaderID, request.ID(c))
		c.Next()
	}
}

// setSecurityHeaders sets the headers of API-19 (docs/design/api/README.md):
// framing denial (D-79), no sniffing, no referrer, same-origin resources, and
// no caching. Headers are set, not added, so calling it twice is harmless.
// The web server sets its own policy for pages and skips /api/ paths, so a
// browser never receives two policies for one response.
func setSecurityHeaders(c *gin.Context) {
	h := c.Writer.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
}

// SecurityHeaders sets the security headers on every response.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		setSecurityHeaders(c)
		c.Next()
	}
}

// AccessLog writes one record per request. It records the route template, not
// the raw path, and never the query string, headers or body, so a secret in
// any of them cannot reach the log and an attacker chosen path cannot forge
// log content.
//
// A panic that passes through is logged as status 500 and then continues to
// Recovery, so a panicked request is still access-logged exactly once.
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		write := func(status int) {
			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			level := slog.LevelDebug
			switch {
			case status >= 500:
				level = slog.LevelError
			case status >= 400:
				level = slog.LevelInfo
			}
			logger.LogAttrs(c.Request.Context(), level, "request",
				slog.String("request_id", request.ID(c)),
				slog.String("method", c.Request.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
		}
		defer func() {
			if v := recover(); v != nil {
				// The request is failing. Log it as 500 and let Recovery,
				// which wraps this middleware, answer it. If the logger
				// itself panics, that panic replaces v and Recovery handles
				// it the same way.
				write(http.StatusInternalServerError)
				panic(v)
			}
			write(c.Writer.Status())
		}()
		c.Next()
	}
}

// Recovery turns a panic anywhere below it into the internal_error envelope.
// It must be the first (outermost) middleware, so that no application
// middleware can panic outside it.
//
// The panic value and the stack are not logged, because either can hold a
// secret or a path. Only the value's type is. If the response has already
// started, it is left as it is and only logged. If the request id or security
// headers were not set yet because the panic came first, Recovery sets them.
// Logging is protected too: a failing logger cannot make Recovery panic.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if c.Writer.Written() {
				c.Abort()
			} else {
				if request.ID(c) == "" {
					request.SetID(c, request.UnknownID)
					c.Header(request.HeaderID, request.UnknownID)
				}
				setSecurityHeaders(c)
				response.Error(c, http.StatusInternalServerError, response.CodeInternal)
			}
			func() {
				defer func() { _ = recover() }() // a broken logger must not undo the recovery
				logger.LogAttrs(c.Request.Context(), slog.LevelError, "panic recovered",
					slog.String("request_id", request.ID(c)),
					slog.String("panic_type", fmt.Sprintf("%T", v)),
				)
			}()
		}()
		c.Next()
	}
}

// AllowedHosts rejects a request whose Host header is not in the allow-list
// (security.md section 8). The comparison is exact on host:port after
// lower-casing. An empty list denies everything.
func AllowedHosts(hosts []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		allowed[strings.ToLower(h)] = struct{}{}
	}
	return func(c *gin.Context) {
		if _, ok := allowed[strings.ToLower(c.Request.Host)]; !ok {
			response.Error(c, http.StatusForbidden, response.CodeHostDenied)
			return
		}
		c.Next()
	}
}
