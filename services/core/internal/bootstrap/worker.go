package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/evidencefile"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/recoverymarker"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/unavailable"
	evidenceapp "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/evidence"
	appjob "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/maintenance"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/retention"
	appscope "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/logging"
)

const (
	// workerMaxConns bounds the pool of the worker role. A job holds no
	// connection while its process runs.
	workerMaxConns = 6
	// retentionMaxConns bounds the pool of the retention role, which runs one
	// purge at a time.
	retentionMaxConns = 2
)

// Worker is the composed background process: the job worker, the maintenance of
// retention and evidence, and the recovery check that gates both.
type Worker struct {
	cfg    config.Config
	logger *slog.Logger

	worker      *persistence.DB
	retention   *persistence.DB
	recovery    *recovery.Service
	jobs        *appjob.Worker
	maintenance *maintenance.Service
	files       *evidencefile.Store
}

// NewWorker composes the worker from a validated configuration. It opens two
// pools, one as the worker role and one as the retention role, and nothing
// else. The sources of the evaluator inputs and the sandbox runner do not exist
// before milestones M8 and M16 to M19, so the worker is wired with adapters that
// refuse (package unavailable): it claims nothing that it could run.
func NewWorker(cfg config.Config, logOut io.Writer) (*Worker, error) {
	logger := logging.New(logOut, cfg.Log.Level, cfg.Log.Format)

	wdb, err := persistence.Open(context.Background(), cfg.Database.SocketDir, persistence.RoleWorker, workerMaxConns)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	rdb, err := persistence.Open(context.Background(), cfg.Database.SocketDir, persistence.RoleRetention, retentionMaxConns)
	if err != nil {
		wdb.Close()
		return nil, fmt.Errorf("database: %w", err)
	}
	closeAll := func() { rdb.Close(); wdb.Close() }

	markers, err := recoverymarker.New(cfg.State.Dir)
	if err != nil {
		closeAll()
		return nil, fmt.Errorf("recovery marker: %w", err)
	}
	rec := recovery.NewService(recovery.Deps{Markers: markers, Repo: wdb.Recovery(), NewID: ids.New, Now: time.Now, Logger: logger})

	wsink, err := wdb.AuditSink(auditRetryPolicy, logger)
	if err != nil {
		closeAll()
		return nil, fmt.Errorf("audit sink: %w", err)
	}
	rsink, err := rdb.AuditSink(auditRetryPolicy, logger)
	if err != nil {
		closeAll()
		return nil, fmt.Errorf("audit sink: %w", err)
	}
	files, err := evidencefile.New(cfg.Evidence.Dir, cfg.Evidence.MaxBytes)
	if err != nil {
		closeAll()
		return nil, fmt.Errorf("evidence store: %w", err)
	}
	evid := evidenceapp.New(evidenceapp.Deps{
		Store: files, Repo: wdb.Evidence(), Audit: wsink, Tx: wdb.Transactor(), Alerts: wdb.SystemAlerts(),
		NewID: ids.New, Now: time.Now, Logger: logger,
	})

	jobs := wdb.Jobs()
	evaluator := appscope.New(appscope.Deps{Inputs: unavailable.ScopeInputs{}, Decisions: jobs, Audit: wsink, NewID: ids.New, Now: time.Now, Logger: logger})
	jw, err := appjob.New(appjob.Deps{
		Store: jobs, Uses: wdb.ApprovalUses(), Tx: wdb.Transactor(), Evaluator: evaluator, Runner: unavailable.Runner{},
		Alerts: wdb.SystemAlerts(), Audit: wsink, Recovery: rec, Timing: job.DefaultTiming(), WorkerID: workerID(),
		NewID: ids.New, Now: time.Now, Logger: logger,
	})
	if err != nil {
		_ = files.Close()
		closeAll()
		return nil, err
	}

	ret := retention.New(retention.Deps{Repo: rdb.Retention(), Tx: rdb.Transactor(), Audit: rsink, NewID: ids.New})
	return &Worker{
		cfg: cfg, logger: logger, worker: wdb, retention: rdb, recovery: rec, jobs: jw, files: files,
		maintenance: &maintenance.Service{Evidence: evid, Purger: ret, Policy: maintenance.DefaultPolicy(), Logger: logger},
	}, nil
}

// workerID names this process in audit events and in the lease of a job. It has
// the pattern of an audit reference.
func workerID() string {
	return fmt.Sprintf("worker-%d-%s", os.Getpid(), ids.New()[:8])
}

// Close releases the pools and the evidence directory.
func (w *Worker) Close() {
	_ = w.files.Close()
	w.retention.Close()
	w.worker.Close()
}

// Run starts the job loop and the maintenance once the recovery check says that
// the installation is normal, and returns after ctx is done and both have
// stopped. The worker role cannot write the first-run or the reconciliation
// state, so a worker that starts before the server waits and repeats the check.
func (w *Worker) Run(ctx context.Context) error {
	defer w.Close()
	if err := w.waitForRecovery(ctx); err != nil {
		return err
	}
	w.logger.Info("worker starting")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); w.maintenance.Run(ctx) }()
	var err error
	go func() { defer wg.Done(); err = w.jobs.Run(ctx) }()
	wg.Wait()
	w.logger.Info("worker stopped")
	return err
}

func (w *Worker) waitForRecovery(ctx context.Context) error {
	for {
		cctx, cancel := context.WithTimeout(ctx, recoveryCheckTimeout)
		st, err := w.recovery.Check(cctx)
		cancel()
		switch {
		case err == nil && !st.Required:
			return nil
		case err == nil:
			w.logger.Error("recovery is required: the worker claims nothing until the reconciliation ran", "reason", string(st.Reason))
		default:
			// No error text: a driver error can carry the socket path.
			w.logger.Warn("the recovery check failed, and it will be repeated")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(recoveryRetryInterval):
		}
	}
}
