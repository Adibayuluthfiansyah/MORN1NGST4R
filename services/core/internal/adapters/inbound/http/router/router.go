// Package router builds the HTTP handler of the server. It is the only place
// that creates the Gin engine, so Gin does not appear in any other package of
// the process (structure.md section 7).
package router

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/handlers"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/middleware"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/response"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

// BasePath is the API base path (API-01).
const BasePath = "/api/v1"

// Deps are the collaborators of the router.
type Deps struct {
	Health inbound.Health
	// AllowedHosts is the exact Host allow-list. Empty denies everything.
	AllowedHosts []string
	// Logger receives the access log. Nil discards it.
	Logger *slog.Logger
}

// New returns the HTTP handler.
//
// Authentication, session, CSRF and Origin checks are not part of this phase
// (milestone M4). Until they exist the only routes are the two public health
// probes, which return a fixed status word and nothing else.
func New(d Deps) http.Handler {
	return build(d)
}

// build assembles the engine. afterRecovery handlers run directly after the
// Recovery middleware. Production code passes none. Tests use it to prove that
// Recovery wraps everything below it.
func build(d Deps, afterRecovery ...gin.HandlerFunc) *gin.Engine {
	logger := d.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	// Do not trust X-Forwarded-* from anyone, and never redirect: a request
	// either matches a route exactly or it is a 404.
	if err := engine.SetTrustedProxies(nil); err != nil {
		panic("router: " + err.Error()) // nil is always valid
	}
	engine.RedirectTrailingSlash = false
	engine.RedirectFixedPath = false
	engine.HandleMethodNotAllowed = true

	// The order matters.
	//   - Recovery is first, so no application middleware can panic outside
	//     it. It sets the request id and the headers itself when the panic
	//     came before the middleware that sets them.
	//   - The request id comes next so every later record has it.
	//   - The Host allow-list runs before routing, so an unknown path or
	//     method from a foreign Host is still a 403.
	chain := []gin.HandlerFunc{middleware.Recovery(logger)}
	chain = append(chain, afterRecovery...) // test seam, empty in production
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

	allow := allowedMethods(engine)
	engine.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound)
	})
	engine.NoMethod(func(c *gin.Context) {
		// Valid while every route is static. Revisit with the first
		// parameterised route.
		if m := allow[c.Request.URL.Path]; m != "" {
			c.Header("Allow", m)
		}
		response.Error(c, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed)
	})
	return engine
}

// allowedMethods maps each registered static path to its Allow header value.
func allowedMethods(engine *gin.Engine) map[string]string {
	byPath := make(map[string][]string)
	for _, r := range engine.Routes() {
		byPath[r.Path] = append(byPath[r.Path], r.Method)
	}
	out := make(map[string]string, len(byPath))
	for path, methods := range byPath {
		sort.Strings(methods)
		out[path] = strings.Join(methods, ", ")
	}
	return out
}
