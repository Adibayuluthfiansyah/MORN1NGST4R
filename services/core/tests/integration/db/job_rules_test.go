package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// These tests are the database side of TD-M3-01 and TD-M3-02. They write as the
// worker role, because the rule is that a defect in the worker cannot break the
// lifecycle.

func (e *env) workerTx(t *testing.T, fn func(ctx context.Context, tx pgx.Tx) error) error {
	t.Helper()
	conn := e.c.Conn(t, persistence.RoleWorker)
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := fn(t.Context(), tx); err != nil {
		_ = tx.Rollback(t.Context())
		return err
	}
	return tx.Commit(t.Context())
}

func transition(ctx context.Context, tx pgx.Tx, job, from, to string, epoch int) error {
	_, err := tx.Exec(ctx, `INSERT INTO morningstar.job_transitions (id, job_id, from_state, to_state, control_state, claim_epoch, actor_kind) VALUES (uuidv7(), $1, $2, $3, 'none', $4, 'worker')`, job, from, to, epoch)
	return err
}

// claimRunning claims a job the way the worker does and returns the use id.
func (e *env) claimRunning(t *testing.T, a pgtest.Approval, job string) string {
	t.Helper()
	res, err := e.uses.Claim(t.Context(), e.claim(a, job))
	if err != nil {
		t.Fatal(err)
	}
	e.runJob(t, job)
	return res.UseID
}

func (e *env) decision(t *testing.T, job string) string {
	t.Helper()
	var id string
	if err := e.c.Owner(t).QueryRow(t.Context(), `INSERT INTO morningstar.policy_decisions (id, program_id, assessment_id, point, job_id, outcome, scope_version_id, validation_id, policy_snapshot_hash)
		VALUES (uuidv7(), $1, $2, 'E3', $3, 'allow', $4, $5, $6) RETURNING id`, e.b.ProgramID, e.b.AssessmentID, job, e.b.ScopeID, e.b.ValidationID, e.b.PolicyHash).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// launch raises the counter and inserts the attempt in one transaction, as the worker does.
func (e *env) launchAttempt(t *testing.T, job, use string, attemptNo, epoch int, counter int) error {
	t.Helper()
	decision := e.decision(t, job)
	return e.workerTx(t, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE morningstar.jobs SET execution_attempts = $2 WHERE id = $1`, job, counter); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO morningstar.job_attempts (id, job_id, attempt_no, claim_epoch, approval_use_id, decision_id, worker_instance) VALUES (uuidv7(), $1, $2, $3, $4, $5, 'w1')`,
			job, attemptNo, epoch, use, decision)
		return err
	})
}

func TestTheDatabaseRefusesAnIllegalJobTransition(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)

	for name, to := range map[string]string{"queued to succeeded": "succeeded", "queued to awaiting approval": "awaiting_approval"} {
		err := e.workerTx(t, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE morningstar.jobs SET state = $2, finished_at = CASE WHEN $2 = 'succeeded' THEN now() END WHERE id = $1`, job, to); err != nil {
				return err
			}
			return transition(ctx, tx, job, "queued", to, 0)
		})
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// A finished job does not move again, and a job is not created in a state that needs a history.
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.jobs SET state = 'failed', finished_at = now(), failure_class = 'x' WHERE id = $1`, job); err == nil {
		// The owner is refused by the transition parity (no history row), which is checked at commit. A direct statement commits at once.
		t.Error("the owner changed the state without a history row")
	}
	for _, state := range []string{"running", "succeeded", "failed", "cancelled"} {
		_, err := e.c.Owner(t).Exec(t.Context(), `INSERT INTO morningstar.jobs (id, program_id, assessment_id, job_kind, operation_id, risk_class, state, approval_id, scope_version_id, validation_id, finished_at, lease_owner, lease_expires_at, approval_use_id, failure_class)
			SELECT uuidv7(), program_id, assessment_id, job_kind, operation_id, risk_class, $2, approval_id, scope_version_id, validation_id,
			       CASE WHEN $2 IN ('succeeded','failed','cancelled') THEN now() END, CASE WHEN $2 = 'running' THEN 'w' END, CASE WHEN $2 = 'running' THEN now() + interval '1 minute' END,
			       NULL, 'policy_denial' FROM morningstar.jobs WHERE id = $1`, job, state)
		if err == nil {
			t.Errorf("a job was created %s", state)
		}
	}
}

func TestAChangeOfStateNeedsItsHistoryRowFromTheSameTransaction(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)

	// No history row.
	err := e.workerTx(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE morningstar.jobs SET state = 'cancelled', finished_at = now(), updated_at = now() WHERE id = $1`, job)
		return err
	})
	if err == nil {
		t.Fatal("a job was cancelled without a history row")
	}
	// A history row of an earlier transaction does not count.
	if err := e.workerTx(t, func(ctx context.Context, tx pgx.Tx) error { return transition(ctx, tx, job, "queued", "cancelled", 0) }); err != nil {
		t.Fatal(err)
	}
	err = e.workerTx(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE morningstar.jobs SET state = 'cancelled', finished_at = now(), updated_at = now() WHERE id = $1`, job)
		return err
	})
	if err == nil {
		t.Fatal("an old history row justified a new change")
	}
	// The change and its row together are accepted.
	if err := e.workerTx(t, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE morningstar.jobs SET state = 'cancelled', finished_at = now(), updated_at = now() WHERE id = $1`, job); err != nil {
			return err
		}
		return transition(ctx, tx, job, "queued", "cancelled", 0)
	}); err != nil {
		t.Fatalf("the legal change was refused: %v", err)
	}
	// And a finished job does not move.
	err = e.workerTx(t, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE morningstar.jobs SET state = 'queued', finished_at = NULL WHERE id = $1`, job); err != nil {
			return err
		}
		return transition(ctx, tx, job, "cancelled", "queued", 0)
	})
	if err == nil {
		t.Error("a cancelled job was requeued")
	}
}

func TestTheEpochAndTheAttemptCounterOnlyGrow(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxAttempts: 3})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 3)
	use := e.claimRunning(t, a, job)
	if err := e.launchAttempt(t, job, use, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	for name, stmt := range map[string]string{
		"lower the counter": `UPDATE morningstar.jobs SET execution_attempts = 0 WHERE id = $1`,
		"lower the epoch":   `UPDATE morningstar.jobs SET claim_epoch = 0 WHERE id = $1`,
	} {
		if err := e.workerTx(t, func(ctx context.Context, tx pgx.Tx) error { _, err := tx.Exec(ctx, stmt, job); return err }); err == nil {
			t.Errorf("the worker could %s", name)
		}
	}
}

func TestAnAttemptBelongsToTheClaimThatHoldsARunningJob(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxAttempts: 3})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 3)

	// A queued job has no attempt, whoever asks.
	if err := e.launchAttempt(t, job, "", 1, 0, 1); err == nil {
		t.Fatal("an attempt was made for a job that is not running")
	}
	use := e.claimRunning(t, a, job)
	if err := e.launchAttempt(t, job, use, 1, 7, 1); err == nil {
		t.Error("an attempt was made under another epoch")
	}
	if err := e.launchAttempt(t, job, use, 1, 1, 0); err == nil {
		t.Error("an attempt was made without raising the counter")
	}
	if err := e.launchAttempt(t, job, use, 2, 1, 2); err == nil {
		t.Error("the first attempt was numbered 2")
	}
	if err := e.launchAttempt(t, job, newID(), 1, 1, 1); err == nil {
		t.Error("an attempt named another approval use")
	}
	if err := e.launchAttempt(t, job, use, 1, 1, 1); err != nil {
		t.Fatalf("the first attempt was refused: %v", err)
	}
	// The first attempt has not confirmed its termination: one live attempt at a time.
	if err := e.launchAttempt(t, job, use, 2, 1, 2); err == nil {
		t.Error("a second attempt started before the first was confirmed gone")
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.job_attempts SET state = 'failed', ended_at = now(), termination_confirmed_at = now() WHERE job_id = $1`, job); err != nil {
		t.Fatal(err)
	}
	if err := e.launchAttempt(t, job, use, 2, 1, 2); err != nil {
		t.Fatalf("the second attempt was refused after the first was confirmed: %v", err)
	}
}

func TestTheAttemptsOfAJobStayWithinTheApprovedNumber(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxAttempts: 2})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 2)
	use := e.claimRunning(t, a, job)
	for n := 1; n <= 2; n++ {
		if err := e.launchAttempt(t, job, use, n, 1, n); err != nil {
			t.Fatalf("attempt %d: %v", n, err)
		}
		if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.job_attempts SET state = 'failed', ended_at = now(), termination_confirmed_at = now() WHERE job_id = $1 AND attempt_no = $2`, job, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.launchAttempt(t, job, use, 3, 1, 3); err == nil {
		t.Error("a third attempt of a job that allows two")
	}
}

// A retry waits for the confirmed end of the previous process tree (D-80). The
// unique index job_attempts_unconfirmed_uq of 00013 says so in the database,
// even if the worker has a defect.
func TestALaterAttemptNeedsTheEarlierTreeConfirmedGone(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxAttempts: 2})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 2)
	use := e.claimRunning(t, a, job)
	if err := e.launchAttempt(t, job, use, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.job_attempts SET state = 'lost', ended_at = now() WHERE job_id = $1`, job); err != nil {
		t.Fatal(err)
	}
	if err := e.launchAttempt(t, job, use, 2, 1, 2); err == nil {
		t.Error("a second attempt started while the first tree was not confirmed gone")
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.job_attempts SET termination_confirmed_at = now() WHERE job_id = $1`, job); err != nil {
		t.Fatal(err)
	}
	if err := e.launchAttempt(t, job, use, 2, 1, 2); err != nil {
		t.Errorf("a second attempt after the confirmation: %v", err)
	}
}

// An intrusive operation has exactly one launch (D-49), and the database says so.
func TestAnIntrusiveJobCannotLaunchTwice(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{Risk: "intrusive"})
	job := e.c.SeedJob(t, e.b, a, "intrusive", 1)
	use := e.claimRunning(t, a, job)
	if err := e.launchAttempt(t, job, use, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.job_attempts SET state = 'failed', ended_at = now(), termination_confirmed_at = now() WHERE job_id = $1`, job); err != nil {
		t.Fatal(err)
	}
	if err := e.launchAttempt(t, job, use, 2, 1, 2); err == nil {
		t.Error("an intrusive job launched a second time")
	}
}

func TestTheClaimFunctionNeedsAQueuedJobAndTheNextEpoch(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 5})

	skipped := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	c := e.claim(a, skipped)
	c.ClaimEpoch = 3
	if got := reasonOf(t, e.tryClaim(t, c)); got != approval.ReasonInvalidInput {
		t.Errorf("a skipped epoch: %s", got)
	}
	stopped := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.jobs SET control_state = 'stop_requested', control_reason = 'test', control_requested_at = now() WHERE id = $1`, stopped); err != nil {
		t.Fatal(err)
	}
	if got := reasonOf(t, e.tryClaim(t, e.claim(a, stopped))); got != approval.ReasonInvalidInput {
		t.Errorf("a job with a stop request: %s", got)
	}
	running := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	e.claimRunning(t, a, running)
	again := e.claim(a, running)
	again.ClaimEpoch = 2
	if got := reasonOf(t, e.tryClaim(t, again)); got != approval.ReasonInvalidInput {
		t.Errorf("a job that is running: %s", got)
	}
	ok := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if _, err := e.uses.Claim(t.Context(), e.claim(a, ok)); err != nil {
		t.Errorf("the next epoch of a queued job: %v", err)
	}
	if c, rows := e.counts(t, a.ID); c != 2 || rows != 2 {
		t.Errorf("counter %d, rows %d, want 2 and 2: a refused claim consumed a use", c, rows)
	}
}

func (e *env) tryClaim(t *testing.T, c outbound.ApprovalClaim) error {
	t.Helper()
	_, err := e.uses.Claim(t.Context(), c)
	return err
}
