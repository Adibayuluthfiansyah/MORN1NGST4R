// Command worker is the background job process of MORN1NGST4R.
//
// It only loads the configuration, composes the worker through bootstrap and
// runs it until it receives SIGINT or SIGTERM. It exposes no route and holds no
// business rule.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/bootstrap"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Environ(), os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, environ []string, logOut io.Writer) error {
	cfg, err := config.Load(environ)
	if err != nil {
		return err
	}
	w, err := bootstrap.NewWorker(cfg, logOut)
	if err != nil {
		return err
	}
	return w.Run(ctx)
}
