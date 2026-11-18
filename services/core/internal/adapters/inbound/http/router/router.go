// Package router builds the HTTP handler of the server. It is the only place
// that creates the Gin engine, so Gin does not appear in any other package of
// the process (structure.md section 7).
package router

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/handlers"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/middleware"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/request"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/response"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

const BasePath = "/api/v1"

type Deps struct {
	Health inbound.Health
	// Auth is nil only in tests of the health routes. Without it no
	// authentication route exists.
	Auth inbound.Auth
	// AllowedHosts is the exact Host allow-list. Empty denies everything.
	AllowedHosts []string
	// AllowedOrigins is the exact Origin allow-list of state-changing requests.
	AllowedOrigins []string
	Logger         *slog.Logger
}

func New(d Deps) http.Handler { return build(d) }

// build assembles the engine. afterRecovery handlers run directly after the
// Recovery middleware. Tests use it to prove that Recovery wraps everything
// below it.
func build(d Deps, afterRecovery ...gin.HandlerFunc) *gin.Engine {
	logger := d.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	// Nobody is a trusted proxy, and a request either matches a route exactly
	// or is a 404: forwarded headers and redirects never change the Host or the
	// path that a check saw.
	if err := engine.SetTrustedProxies(nil); err != nil {
		panic("router: " + err.Error())
	}
	engine.RedirectTrailingSlash = false
	engine.RedirectFixedPath = false
	engine.HandleMethodNotAllowed = true

	// Recovery is outermost, so no middleware below it can panic outside it. The
	// Host allow-list runs before routing, so an unknown path from a foreign
	// Host is still a 403.
	chain := []gin.HandlerFunc{middleware.Recovery(logger)}
	chain = append(chain, afterRecovery...)
	chain = append(chain,
		middleware.RequestID(),
		middleware.SecurityHeaders(),
		middleware.AccessLog(logger),
		middleware.AllowedHosts(d.AllowedHosts),
	)
	engine.Use(chain...)

	health := handlers.NewHealth(d.Health)
	v1 := engine.Group(BasePath)
	v1.GET("/health/live", health.Live)
	v1.GET("/health/ready", health.Ready)
	if d.Auth != nil {
		authRoutes(v1, d)
	}

	engine.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound)
	})
	allow := allowedMethods(engine)
	engine.NoMethod(func(c *gin.Context) {
		if m := allow(c.Request.URL.Path); m != "" {
			c.Header("Allow", m)
		}
		response.Error(c, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed)
	})
	return engine
}

// authRoutes registers the routes of docs/design/api/endpoint-specification.md
// section "Authentication and account", with the checks in the order of
// authentication-and-authorization.md section 2.
func authRoutes(v1 *gin.RouterGroup, d Deps) {
	h := handlers.NewAuth(d.Auth)
	var (
		perClient   = middleware.RateLimitClient(middleware.NewLimiter(30, time.Minute))
		perClientRd = middleware.RateLimitClient(middleware.NewLimiter(300, time.Minute))
		readLimit   = middleware.RateLimitSession(middleware.NewLimiter(300, time.Minute))
		writeLimit  = middleware.RateLimitSession(middleware.NewLimiter(60, time.Minute))
		reauthLimit = middleware.RateLimitSession(middleware.NewLimiter(30, time.Minute))
		jsonBody    = middleware.JSONBody(request.AuthBodyLimit)
		origin      = middleware.Origin(d.AllowedOrigins)
		anonCSRF    = middleware.CSRFAnon(d.Auth)
		session     = middleware.Session(d.Auth)
		sessionCSRF = middleware.CSRFSession()
		gate        = middleware.RecoveryGate(d.Auth)
	)

	// These two work while recovery is required.
	v1.GET("/session", perClientRd, h.State)
	v1.POST("/system/recovery/reconcile", jsonBody, perClient, origin, anonCSRF, h.Reconcile)

	g := v1.Group("", gate)
	g.POST("/setup/account", jsonBody, perClient, origin, anonCSRF, h.Setup)
	g.POST("/session/login", jsonBody, perClient, origin, anonCSRF, h.Login)
	g.POST("/recovery/reset", jsonBody, perClient, origin, anonCSRF, h.ResetPassword)

	g.GET("/sessions", session, readLimit, h.List)
	g.POST("/session/logout", session, writeLimit, origin, sessionCSRF, h.Logout)
	g.POST("/sessions/:sessionId/revoke", session, writeLimit, origin, sessionCSRF, h.Revoke)
	g.POST("/session/reauthenticate", jsonBody, session, reauthLimit, origin, sessionCSRF, h.Reauthenticate)
	g.POST("/account/password", jsonBody, session, writeLimit, origin, sessionCSRF, h.ChangePassword)
}

// allowedMethods returns a function that maps a request path to the Allow header
// of the routes that match it, parameters included.
func allowedMethods(engine *gin.Engine) func(path string) string {
	type route struct {
		segments []string
		method   string
	}
	var routes []route
	for _, r := range engine.Routes() {
		routes = append(routes, route{strings.Split(r.Path, "/"), r.Method})
	}
	return func(path string) string {
		parts := strings.Split(path, "/")
		var methods []string
		for _, r := range routes {
			if len(r.segments) != len(parts) {
				continue
			}
			match := true
			for i, seg := range r.segments {
				if !strings.HasPrefix(seg, ":") && seg != parts[i] || strings.HasPrefix(seg, ":") && parts[i] == "" {
					match = false
					break
				}
			}
			if match {
				methods = append(methods, r.method)
			}
		}
		sort.Strings(methods)
		return strings.Join(methods, ", ")
	}
}
