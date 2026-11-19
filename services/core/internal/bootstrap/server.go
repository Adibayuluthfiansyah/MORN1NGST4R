// Package bootstrap assembles the dependencies of each runtime.
//
// There is one composition per process. server.go composes the server and
// worker.go composes the worker. Neither creates a dependency of the other.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/router"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/bootstrapfile"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/evidencefile"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/passwordhash"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/recoverymarker"
	authapp "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/auth"
	evidenceapp "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/health"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/logging"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const (
	// readinessTimeout bounds one readiness check.
	readinessTimeout = 2 * time.Second
	// maxHeaderBytes is far below the Go default of 1 MiB. No route takes a
	// large header.
	maxHeaderBytes = 16 << 10
	// apiMaxConns bounds the server pool. The cluster allows 30 connections
	// (scripts/db/pg-cluster.sh), and the worker and the tools share them.
	apiMaxConns = 8
	// maxConcurrentHashes bounds the Argon2id memory: 2 hashes of 64 MiB each.
	maxConcurrentHashes = 2
	// recoveryCheckTimeout bounds the startup recovery check.
	recoveryCheckTimeout = 15 * time.Second
	// recoveryRetryInterval is how often the check repeats while the database
	// is down at start.
	recoveryRetryInterval = 5 * time.Second
)

// auditRetryPolicy is the bound on the retries of an audit write (SEC 13, D-48).
// The design leaves the number to "security policy" (threat-model C-33), and the
// owner approved these values on 2026-11-18 (TD-M5-02). Three attempts follow
// the default of D-30 for job attempts. The worst case of one write is 3.5
// seconds. Changing them changes only this value.
var auditRetryPolicy = audit.RetryPolicy{
	MaxAttempts:    3,
	Delays:         []time.Duration{100 * time.Millisecond, 400 * time.Millisecond},
	AttemptTimeout: time.Second,
}

// Server is the composed HTTP server process.
type Server struct {
	cfg    config.Config
	logger *slog.Logger
	http   *http.Server

	// db and recovery are nil when the server runs WithoutDatabase.
	db       *persistence.DB
	recovery *recovery.Service
	auth     *authapp.Service
	evidence *evidenceapp.Service
	files    *evidencefile.Store
}

// ServerOption changes how NewServer composes the server.
type ServerOption func(*serverOptions)

type serverOptions struct {
	database   bool
	hashParams passwordhash.Params
}

// WithPasswordParams replaces the Argon2id parameters. Only tests use it, to
// keep hashing fast. Production uses passwordhash.Default.
func WithPasswordParams(p passwordhash.Params) ServerOption {
	return func(o *serverOptions) { o.hashParams = p }
}

// WithoutDatabase composes a server that has no database, no readiness probe
// and no recovery check. It exists for the tests of the HTTP layer. The
// production composition (cmd/server) never uses it.
func WithoutDatabase() ServerOption { return func(o *serverOptions) { o.database = false } }

// NewServer composes the server from a validated configuration. Log output
// goes to logOut. By default it opens the database pool as the server role
// (lazily, so a database that is down does not stop the process), registers the
// database readiness probe (D-86) and prepares the recovery check. It fails when
// the recovery marker directory is unusable.
func NewServer(cfg config.Config, logOut io.Writer, opts ...ServerOption) (*Server, error) {
	o := serverOptions{database: true, hashParams: passwordhash.Default}
	for _, opt := range opts {
		opt(&o)
	}
	logger := logging.New(logOut, cfg.Log.Level, cfg.Log.Format)

	var (
		probes []outbound.ReadinessProbe
		db     *persistence.DB
		rec    *recovery.Service
		auth   *authapp.Service
		evid   *evidenceapp.Service
		files  *evidencefile.Store
	)
	if o.database {
		var err error
		if db, err = persistence.Open(context.Background(), cfg.Database.SocketDir, persistence.RoleAPI, apiMaxConns); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		markers, err := recoverymarker.New(cfg.State.Dir)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("recovery marker: %w", err)
		}
		probes = append(probes, db.Probe())
		rec = recovery.NewService(recovery.Deps{Markers: markers, Repo: db.Recovery(), NewID: ids.New, Now: time.Now, Logger: logger})
		hasher, err := passwordhash.New(o.hashParams, maxConcurrentHashes)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("password hashing: %w", err)
		}
		sink, err := db.AuditSink(auditRetryPolicy, logger)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("audit sink: %w", err)
		}
		auth = authapp.New(authapp.Deps{
			Hasher: hasher, Accounts: db.Accounts(), Bootstraps: db.Bootstraps(), RecoveryCodes: db.RecoveryCodes(),
			Sessions: db.Sessions(), Reauth: db.Reauth(), Attempts: db.Attempts(), Audit: sink,
			SecretFile: bootstrapfile.New(cfg.Auth.RuntimeDir), Tx: db.Transactor(), Recovery: rec,
			NewID: ids.New, Now: time.Now, Logger: logger,
		})
		if files, err = evidencefile.New(cfg.Evidence.Dir, cfg.Evidence.MaxBytes); err != nil {
			auth.Close()
			db.Close()
			return nil, fmt.Errorf("evidence store: %w", err)
		}
		evid = evidenceapp.New(evidenceapp.Deps{
			Store: files, Repo: db.Evidence(), Audit: sink, Tx: db.Transactor(), NewID: ids.New, Now: time.Now, Logger: logger,
		})
	}
	healthSvc := health.New(probes, readinessTimeout, logger)

	routerDeps := router.Deps{
		Health:         healthSvc,
		AllowedHosts:   cfg.HTTP.AllowedHosts,
		AllowedOrigins: cfg.HTTP.AllowedOrigins,
		Logger:         logger,
	}
	if auth != nil {
		routerDeps.Auth = auth
	}
	if evid != nil {
		routerDeps.Evidence = evid
	}
	handler := router.New(routerDeps)

	return &Server{
		cfg:      cfg,
		logger:   logger,
		db:       db,
		recovery: rec,
		auth:     auth,
		evidence: evid,
		files:    files,
		http: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			ReadTimeout:       cfg.HTTP.ReadTimeout,
			WriteTimeout:      cfg.HTTP.WriteTimeout,
			IdleTimeout:       cfg.HTTP.IdleTimeout,
			MaxHeaderBytes:    maxHeaderBytes,
			// net/http would answer "OPTIONS *" itself, before the handler,
			// which bypasses the Host allow-list. Route it like any request.
			DisableGeneralOptionsHandler: true,
			ErrorLog:                     slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		},
	}, nil
}

// Auth returns the authentication use case, or nil WithoutDatabase. Use cases of
// later milestones take their Principal and their re-authentication check from
// it.
func (s *Server) Auth() *authapp.Service { return s.auth }

// Transactor returns the transaction foundation of the server pool, or nil
// WithoutDatabase. Use cases that guard an action with a re-authentication grant
// run the change and Auth().Consume in one transaction.
func (s *Server) Transactor() outbound.Transactor {
	if s.db == nil {
		return nil
	}
	return s.db.Transactor()
}

// Recovery returns the recovery service, or nil WithoutDatabase.
func (s *Server) Recovery() *recovery.Service { return s.recovery }

// Evidence returns the evidence use case, or nil WithoutDatabase. Use cases that
// produce artifacts (tool results, notes) store them through its Capture.
func (s *Server) Evidence() *evidenceapp.Service { return s.evidence }

// Close releases the database pool. Serve calls it when it returns.
func (s *Server) Close() {
	if s.auth != nil {
		s.auth.Close()
	}
	if s.files != nil {
		_ = s.files.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
}

// ListenAndServe binds the configured loopback address and serves until ctx is
// cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := requireLoopback(s.cfg.HTTP.Listen.Addr()); err != nil {
		s.Close()
		return err
	}
	// The address is set explicitly. Gin's own Run would listen on every
	// interface (tech.md section 2).
	ln, err := net.Listen("tcp", s.cfg.HTTP.Listen.String())
	if err != nil {
		s.Close()
		return fmt.Errorf("listen: %w", err)
	}
	return s.Serve(ctx, ln)
}

// Serve serves on ln until ctx is cancelled, then shuts down gracefully within
// the configured timeout. It returns nil after a clean shutdown. It refuses a
// listener that is not on a loopback address.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	defer s.Close()
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return errors.New("listener is not a TCP listener")
	}
	if err := requireLoopback(tcp.AddrPort().Addr()); err != nil {
		_ = ln.Close()
		return err
	}

	// The recovery check runs before the server accepts a request. A database
	// that is down is not a reason to stay away: the server answers live and
	// reports not ready, and the check repeats until the database is back. Any
	// other failure (for example a marker that cannot be written at first run)
	// stops the start.
	var background sync.WaitGroup
	defer background.Wait()
	checkCtx, stopChecks := context.WithCancel(ctx)
	defer stopChecks()
	if err := s.startRecovery(checkCtx, &background); err != nil {
		_ = ln.Close()
		return err
	}

	s.logger.Info("server starting", "addr", ln.Addr().String())
	errCh := make(chan error, 1)
	go func() { errCh <- s.http.Serve(ln) }()

	select {
	case err := <-errCh:
		// Serve returns only on failure here. ErrServerClosed needs Shutdown.
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	s.logger.Info("server stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		_ = s.http.Close()
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// startRecovery runs the recovery check once. When the database is not
// reachable it starts a retry loop in the background and returns nil.
func (s *Server) startRecovery(ctx context.Context, background *sync.WaitGroup) error {
	if s.recovery == nil {
		return nil
	}
	err := s.checkRecovery(ctx)
	if err == nil {
		return nil
	}
	pingCtx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()
	if s.db.Probe().Check(pingCtx) == nil {
		return fmt.Errorf("recovery check: %w", err)
	}
	s.logger.Warn("the database is not reachable. The server is not ready, and the recovery check repeats")
	background.Add(1)
	go func() {
		defer background.Done()
		ticker := time.NewTicker(recoveryRetryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if err := s.checkRecovery(ctx); err == nil {
				return
			}
			// No error text: a driver error can carry the socket path.
			s.logger.Warn("the recovery check failed again, and it will be repeated")
		}
	}()
	return nil
}

func (s *Server) checkRecovery(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, recoveryCheckTimeout)
	defer cancel()
	st, err := s.recovery.Check(ctx)
	if err != nil {
		return err
	}
	if st.Required {
		s.logger.Error("recovery is required: claims and authenticated routes stay closed until the reconciliation ran", "reason", string(st.Reason))
		return nil
	}
	return s.auth.PrepareBootstrap(ctx)
}

func requireLoopback(a netip.Addr) error {
	a = a.Unmap()
	if !a.IsLoopback() {
		return fmt.Errorf("refusing to listen on %s: only loopback addresses are supported", a)
	}
	return nil
}
