// Package bootstrap assembles the dependencies of each runtime.
//
// There is one composition per process. This file composes the server. The
// worker composition does not exist yet (milestone M7), so no worker
// dependency is created here.
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
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/router"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/health"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/logging"
)

const (
	// readinessTimeout bounds one readiness check.
	readinessTimeout = 2 * time.Second
	// maxHeaderBytes is far below the Go default of 1 MiB. No route takes a
	// large header.
	maxHeaderBytes = 16 << 10
)

// Server is the composed HTTP server process.
type Server struct {
	cfg    config.Config
	logger *slog.Logger
	http   *http.Server
}

// NewServer composes the server from a validated configuration. Log output
// goes to logOut.
func NewServer(cfg config.Config, logOut io.Writer) *Server {
	logger := logging.New(logOut, cfg.Log.Level, cfg.Log.Format)

	// No readiness probe is registered yet. The database probe arrives with
	// the persistence milestone (M3).
	healthSvc := health.New(nil, readinessTimeout, logger)

	handler := router.New(router.Deps{
		Health:       healthSvc,
		AllowedHosts: cfg.HTTP.AllowedHosts,
		Logger:       logger,
	})

	return &Server{
		cfg:    cfg,
		logger: logger,
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
	}
}

// ListenAndServe binds the configured loopback address and serves until ctx is
// cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := requireLoopback(s.cfg.HTTP.Listen.Addr()); err != nil {
		return err
	}
	// The address is set explicitly. Gin's own Run would listen on every
	// interface (tech.md section 2).
	ln, err := net.Listen("tcp", s.cfg.HTTP.Listen.String())
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return s.Serve(ctx, ln)
}

// Serve serves on ln until ctx is cancelled, then shuts down gracefully within
// the configured timeout. It returns nil after a clean shutdown. It refuses a
// listener that is not on a loopback address.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return errors.New("listener is not a TCP listener")
	}
	if err := requireLoopback(tcp.AddrPort().Addr()); err != nil {
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

func requireLoopback(a netip.Addr) error {
	a = a.Unmap()
	if !a.IsLoopback() {
		return fmt.Errorf("refusing to listen on %s: only loopback addresses are supported", a)
	}
	return nil
}
