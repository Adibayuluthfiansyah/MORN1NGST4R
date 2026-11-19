package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// Findings of the M3 review (fresh context). Each test pins one of them.

// The worker role may call the SQL functions directly, without the Go adapter.
// A NULL argument must never read as "no difference" (a comparison with NULL is
// NULL, which an IF treats as false).
func TestFunctionsRefuseNullArguments(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 3})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if _, err := e.uses.Claim(t.Context(), e.claim(a, job)); err != nil { // the job holds a use
		t.Fatal(err)
	}
	w := e.c.Conn(t, persistence.RoleWorker)

	// Argument slots: approval, job, generation, hash, scope, policy, validation, epoch.
	good := []any{a.ID, job, int64(1), a.OperationHash, e.b.ScopeID, e.b.PolicyHash, e.b.ValidationID, int32(1)}
	for i, name := range []string{"approval", "job", "generation", "operation hash", "scope version", "policy snapshot", "validation", "claim epoch"} {
		args := append([]any(nil), good...)
		args[i] = nil
		var reason *string
		if err := w.QueryRow(t.Context(), `SELECT morningstar.check_approval_launch($1::uuid, $2::uuid, $3::bigint, $4::bytea, $5::uuid, $6::bytea, $7::uuid, $8::integer)`, args...).Scan(&reason); err != nil {
			t.Fatalf("check_approval_launch with NULL %s: %v", name, err)
		}
		if reason == nil {
			t.Errorf("check_approval_launch allowed a launch with a NULL %s", name)
		}

		var outcome, why *string
		if i == 7 {
			continue // the epoch is not an argument of the claim function in this position
		}
		// consume_approval_use(use, approval, job, epoch, generation, hash, scope, policy, validation)
		cargs := []any{newID(), args[0], args[1], int32(1), args[2], args[3], args[4], args[5], args[6]}
		if err := w.QueryRow(t.Context(), `SELECT outcome, reason FROM morningstar.consume_approval_use($1::uuid, $2::uuid, $3::uuid, $4::integer, $5::bigint, $6::bytea, $7::uuid, $8::bytea, $9::uuid)`, cargs...).Scan(&outcome, &why); err != nil {
			// An error is a refusal as well, as long as nothing was consumed.
			continue
		}
		if outcome == nil || *outcome == "consumed" || *outcome == "reused" {
			t.Errorf("consume_approval_use accepted a NULL %s: %v", name, outcome)
		}
	}
	if c, rows := e.counts(t, a.ID); c != 1 || rows != 1 {
		t.Errorf("counter %d, rows %d, want 1 and 1", c, rows)
	}
}

// The functions take the job id from the caller. They must check that the job
// is the one that the approval was made for.
func TestClaimIsRefusedForAJobOfAnotherApproval(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 3})
	b := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 3})
	jobOfB := e.c.SeedJob(t, e.b, b, "low_impact", 1)

	_, err := e.uses.Claim(t.Context(), e.claim(a, jobOfB))
	if got := reasonOf(t, err); got != approval.ReasonInvalidInput {
		t.Errorf("reason = %s, want %s", got, approval.ReasonInvalidInput)
	}
	if c, rows := e.counts(t, a.ID); c != 0 || rows != 0 {
		t.Errorf("approval A: counter %d, rows %d, want 0 and 0", c, rows)
	}
	// A job that does not exist.
	_, err = e.uses.Claim(t.Context(), e.claim(a, newID()))
	if got := reasonOf(t, err); got != approval.ReasonInvalidInput {
		t.Errorf("unknown job: reason = %s, want %s", got, approval.ReasonInvalidInput)
	}
	// The launch check refuses the same pairing.
	if got := reasonOf(t, e.uses.CheckLaunch(t.Context(), e.launch(a, jobOfB))); got != approval.ReasonInvalidInput {
		t.Errorf("launch check: reason = %s, want %s", got, approval.ReasonInvalidInput)
	}
	// The worker cannot insert the use row itself either.
	w := e.c.Conn(t, persistence.RoleWorker)
	if _, err := w.Exec(t.Context(), `INSERT INTO morningstar.approval_uses (id, approval_id, job_id, claim_epoch) VALUES ($1, $2, $3, 1)`, newID(), a.ID, jobOfB); err == nil {
		t.Error("the worker inserted an approval_uses row")
	} else {
		wantCode(t, "worker INSERT into approval_uses", err, sqlstateNoRight)
	}
}

// The expiry is read from the clock at the time of the check, not from the start
// of the transaction (DB-08).
func TestExpiryIsReadAtTheTimeOfTheCheck(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)

	err := e.tx.InTx(t.Context(), func(ctx context.Context) error {
		// The transaction starts here. now() is fixed from now on.
		if _, err := persistence.Querier(ctx, e.worker).Exec(ctx, `SELECT now()`); err != nil {
			return err
		}
		e.c.ExpireNow(t, a.ID) // the approval expires after the transaction started
		time.Sleep(20 * time.Millisecond)
		_, err := e.uses.Claim(ctx, e.claim(a, job))
		if got := reasonOf(t, err); got != approval.ReasonExpired {
			t.Errorf("claim inside the transaction: reason = %s, want %s", got, approval.ReasonExpired)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// recovery_state: the generation never goes down, the instance never changes,
// and the state follows the reconciliation phases only.
func TestRecoveryStateCannotBeRewound(t *testing.T) {
	e := newEnv(t)
	owner := e.c.Owner(t)
	if _, err := owner.Exec(t.Context(), `UPDATE morningstar.recovery_state SET generation = 5, state = 'reconciling'`); err != nil {
		t.Fatalf("a reconciliation start is allowed: %v", err)
	}
	if _, err := owner.Exec(t.Context(), `UPDATE morningstar.recovery_state SET state = 'normal'`); err != nil {
		t.Fatalf("the completion is allowed: %v", err)
	}
	for name, stmt := range map[string]string{
		"lower the generation":                `UPDATE morningstar.recovery_state SET generation = 4`,
		"change the instance":                 `UPDATE morningstar.recovery_state SET instance_id = uuidv7()`,
		"start reconciling at the same level": `UPDATE morningstar.recovery_state SET state = 'reconciling'`,
		"raise the generation while normal":   `UPDATE morningstar.recovery_state SET generation = 6`,
		"delete the row":                      `DELETE FROM morningstar.recovery_state`,
		"truncate":                            `TRUNCATE morningstar.recovery_state CASCADE`,
	} {
		if _, err := owner.Exec(t.Context(), stmt); err == nil {
			t.Errorf("owner: %s was accepted", name)
		}
		if _, err := e.api.Exec(t.Context(), stmt); err == nil {
			t.Errorf("api: %s was accepted", name)
		}
	}
	// The completion keeps the generation.
	if _, err := owner.Exec(t.Context(), `UPDATE morningstar.recovery_state SET state = 'reconciling', generation = 6`); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(t.Context(), `UPDATE morningstar.recovery_state SET state = 'normal', generation = 7`); err == nil {
		t.Error("the completion changed the generation")
	}
	var gen int64
	if err := owner.QueryRow(t.Context(), `SELECT generation FROM morningstar.recovery_state`).Scan(&gen); err != nil || gen != 6 {
		t.Errorf("generation = %d (%v), want 6", gen, err)
	}
}
