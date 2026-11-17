package db_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// env is a fresh database with the base rows and a worker pool.
type env struct {
	c      *pgtest.Cluster
	b      pgtest.Base
	worker *pgxpool.Pool
	api    *pgxpool.Pool
	uses   outbound.ApprovalUses
	tx     outbound.Transactor
}

func newEnv(t *testing.T) *env {
	t.Helper()
	c := pgtest.Get(t)
	c.Fresh(t)
	b := c.SeedBase(t)
	pool := c.Pool(t, persistence.RoleWorker, 24)
	return &env{c: c, b: b, worker: pool, api: c.Pool(t, persistence.RoleAPI, 4), uses: persistence.NewApprovalUses(pool), tx: persistence.NewTransactor(pool)}
}

func (e *env) binding() approval.Binding {
	var p [32]byte
	copy(p[:], e.b.PolicyHash)
	return approval.Binding{ValidationID: e.b.ValidationID, ScopeVersionID: e.b.ScopeID, PolicySnapshot: p}
}

func (e *env) claim(a pgtest.Approval, job string) outbound.ApprovalClaim {
	var h [32]byte
	copy(h[:], a.OperationHash)
	return outbound.ApprovalClaim{
		UseID: newID(), ApprovalID: a.ID, JobID: job, ClaimEpoch: 1, Generation: 1,
		Binding: e.binding(), Operation: h,
	}
}

func (e *env) launch(a pgtest.Approval, job string) outbound.LaunchCheck {
	var h [32]byte
	copy(h[:], a.OperationHash)
	return outbound.LaunchCheck{ApprovalID: a.ID, JobID: job, Generation: 1, Binding: e.binding(), Operation: h}
}

func newID() string {
	// A random version-4 shaped identifier is enough for a test key.
	b := pgtest.Random(16)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, x := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hex[x>>4], hex[x&15])
	}
	return string(out)
}

// counts returns the counter of the approval and the number of its use rows,
// read as the schema owner.
func (e *env) counts(t *testing.T, approvalID string) (consumed, rows int) {
	t.Helper()
	conn := e.c.Owner(t)
	if err := conn.QueryRow(t.Context(), `SELECT uses_consumed, (SELECT count(*) FROM morningstar.approval_uses WHERE approval_id = $1) FROM morningstar.approvals WHERE id = $1`, approvalID).Scan(&consumed, &rows); err != nil {
		t.Fatal(err)
	}
	return consumed, rows
}

func reasonOf(t *testing.T, err error) approval.Reason {
	t.Helper()
	var d *approval.DenialError
	if !errors.As(err, &d) || !errors.Is(err, approval.ErrDenied) {
		t.Fatalf("error %v is not an approval denial", err)
	}
	return d.Reason
}

func TestClaimConsumesOneUse(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	claim := e.claim(a, job)

	res, err := e.uses.Claim(t.Context(), claim)
	if err != nil {
		t.Fatalf("Claim error: %v", err)
	}
	if res.Outcome != outbound.ClaimConsumed || res.UseID != claim.UseID {
		t.Errorf("result = %+v, want consumed with use %s", res, claim.UseID)
	}
	if consumed, rows := e.counts(t, a.ID); consumed != 1 || rows != 1 {
		t.Errorf("counter %d, rows %d, want 1 and 1", consumed, rows)
	}
}

// CC-02: two claims for the same (approval, job) leave exactly one use row, and
// the loser is told the use of the winner. max_uses is 1 on purpose: the loser
// must not be refused as exhausted.
func TestConcurrentClaimsForOneJobConsumeOneUse(t *testing.T) {
	e := newEnv(t)
	for round := 0; round < 5; round++ {
		a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 1})
		job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
		const claimers = 16
		var consumed, reused atomic.Int32
		useIDs := make(chan string, claimers)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < claimers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				res, err := e.uses.Claim(context.Background(), e.claim(a, job))
				if err != nil {
					t.Errorf("round %d: Claim error: %v", round, err)
					return
				}
				switch res.Outcome {
				case outbound.ClaimConsumed:
					consumed.Add(1)
				case outbound.ClaimReused:
					reused.Add(1)
				}
				useIDs <- res.UseID
			}()
		}
		close(start)
		wg.Wait()
		close(useIDs)
		if consumed.Load() != 1 || reused.Load() != claimers-1 {
			t.Fatalf("round %d: consumed %d, reused %d, want 1 and %d", round, consumed.Load(), reused.Load(), claimers-1)
		}
		first := ""
		for id := range useIDs {
			if first == "" {
				first = id
			} else if id != first {
				t.Fatalf("round %d: claimers got different use ids %s and %s", round, first, id)
			}
		}
		if c, rows := e.counts(t, a.ID); c != 1 || rows != 1 {
			t.Fatalf("round %d: counter %d, rows %d, want 1 and 1", round, c, rows)
		}
	}
}

// CC-01 and CC-02: with max_uses 5 and 20 jobs that claim at once, exactly 5
// uses are consumed and 15 claims are refused as exhausted.
func TestConcurrentClaimsNeverExceedMaxUses(t *testing.T) {
	e := newEnv(t)
	for round := 0; round < 3; round++ {
		const maxUses, jobs = 5, 20
		a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: maxUses})
		ids := make([]string, jobs)
		for i := range ids {
			ids[i] = e.c.SeedJob(t, e.b, a, "low_impact", 1)
		}
		var consumed, exhausted atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for _, job := range ids {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				res, err := e.uses.Claim(context.Background(), e.claim(a, job))
				switch {
				case err == nil && res.Outcome == outbound.ClaimConsumed:
					consumed.Add(1)
				case err != nil && errors.Is(err, approval.ErrDenied) && reasonOf(t, err) == approval.ReasonExhausted:
					exhausted.Add(1)
				default:
					t.Errorf("round %d: unexpected result %+v, %v", round, res, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if consumed.Load() != maxUses || exhausted.Load() != jobs-maxUses {
			t.Fatalf("round %d: consumed %d, exhausted %d, want %d and %d", round, consumed.Load(), exhausted.Load(), maxUses, jobs-maxUses)
		}
		if c, rows := e.counts(t, a.ID); c != maxUses || rows != maxUses {
			t.Fatalf("round %d: counter %d, rows %d, want %d", round, c, rows, maxUses)
		}
	}
}

// The claim and the update of the job that records it commit together, and a
// rollback leaves nothing consumed (S1.6 item 2, CC-01).
func TestClaimJoinsTheTransactionOfTheContext(t *testing.T) {
	e := newEnv(t)
	t.Run("commit", func(t *testing.T) {
		a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
		job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
		claim := e.claim(a, job)
		err := e.tx.InTx(t.Context(), func(ctx context.Context) error {
			res, err := e.uses.Claim(ctx, claim)
			if err != nil {
				return err
			}
			_, err = persistence.Querier(ctx, e.worker).Exec(ctx, `UPDATE morningstar.jobs SET approval_use_id = $2, updated_at = now(), version = version + 1 WHERE id = $1`, job, res.UseID)
			return err
		})
		if err != nil {
			t.Fatalf("InTx error: %v", err)
		}
		var got string
		conn := e.c.Owner(t)
		if err := conn.QueryRow(t.Context(), `SELECT approval_use_id FROM morningstar.jobs WHERE id = $1`, job).Scan(&got); err != nil || got != claim.UseID {
			t.Errorf("job approval_use_id = %q (%v), want %s", got, err, claim.UseID)
		}
		if c, rows := e.counts(t, a.ID); c != 1 || rows != 1 {
			t.Errorf("counter %d, rows %d", c, rows)
		}
	})
	t.Run("rollback", func(t *testing.T) {
		a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
		job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
		boom := errors.New("later step failed")
		err := e.tx.InTx(t.Context(), func(ctx context.Context) error {
			if _, err := e.uses.Claim(ctx, e.claim(a, job)); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("InTx error = %v, want the callback error", err)
		}
		if c, rows := e.counts(t, a.ID); c != 0 || rows != 0 {
			t.Errorf("after rollback: counter %d, rows %d, want 0 and 0", c, rows)
		}
		// The use is still free for another claim.
		if _, err := e.uses.Claim(t.Context(), e.claim(a, job)); err != nil {
			t.Errorf("claim after rollback: %v", err)
		}
	})
	t.Run("panic rolls back", func(t *testing.T) {
		a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
		job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
		func() {
			defer func() { _ = recover() }()
			_ = e.tx.InTx(t.Context(), func(ctx context.Context) error {
				if _, err := e.uses.Claim(ctx, e.claim(a, job)); err != nil {
					return err
				}
				panic("boom")
			})
		}()
		if c, rows := e.counts(t, a.ID); c != 0 || rows != 0 {
			t.Errorf("after panic: counter %d, rows %d, want 0 and 0", c, rows)
		}
	})
}

// A claim that is made inside a transaction and loses a race for the same job
// must not leave the counter incremented (the exception block rolls it back).
func TestLosingClaimInATransactionLeavesTheCounterAlone(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 3})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)

	inFirst := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- e.tx.InTx(context.Background(), func(ctx context.Context) error {
			if _, err := e.uses.Claim(ctx, e.claim(a, job)); err != nil {
				return err
			}
			close(inFirst)
			<-release // hold the first transaction open, with its row lock
			return nil
		})
	}()
	<-inFirst

	second := make(chan outbound.ClaimResult, 1)
	go func() {
		res, err := e.uses.Claim(context.Background(), e.claim(a, job))
		if err != nil {
			t.Errorf("second claim error: %v", err)
		}
		second <- res
	}()
	// The second claim waits for the row lock. Give it time to block, then commit the first.
	time.Sleep(200 * time.Millisecond)
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first transaction: %v", err)
	}
	if res := <-second; res.Outcome != outbound.ClaimReused {
		t.Errorf("second claim outcome = %v, want reused", res.Outcome)
	}
	if c, rows := e.counts(t, a.ID); c != 1 || rows != 1 {
		t.Errorf("counter %d, rows %d, want 1 and 1", c, rows)
	}
}

func TestRetryClaimReusesTheUseAndIntrusiveNeverRetries(t *testing.T) {
	e := newEnv(t)
	t.Run("low impact retry", func(t *testing.T) {
		a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxAttempts: 2})
		job := e.c.SeedJob(t, e.b, a, "low_impact", 2)
		first, err := e.uses.Claim(t.Context(), e.claim(a, job))
		if err != nil || first.Outcome != outbound.ClaimConsumed {
			t.Fatalf("first claim = %+v, %v", first, err)
		}
		again, err := e.uses.Claim(t.Context(), e.claim(a, job))
		if err != nil || again.Outcome != outbound.ClaimReused || again.UseID != first.UseID {
			t.Fatalf("retry claim = %+v, %v, want reused %s", again, err, first.UseID)
		}
		if c, rows := e.counts(t, a.ID); c != 1 || rows != 1 {
			t.Errorf("counter %d, rows %d, want 1 and 1", c, rows)
		}
		// All attempts of the job are used: the retry is refused.
		conn := e.c.Owner(t)
		if _, err := conn.Exec(t.Context(), `UPDATE morningstar.jobs SET execution_attempts = 2 WHERE id = $1`, job); err != nil {
			t.Fatal(err)
		}
		_, err = e.uses.Claim(t.Context(), e.claim(a, job))
		if got := reasonOf(t, err); got != approval.ReasonAttemptsExhausted {
			t.Errorf("reason = %s, want %s", got, approval.ReasonAttemptsExhausted)
		}
	})
	t.Run("intrusive", func(t *testing.T) {
		a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{Risk: "intrusive"})
		job := e.c.SeedJob(t, e.b, a, "intrusive", 1)
		if res, err := e.uses.Claim(t.Context(), e.claim(a, job)); err != nil || res.Outcome != outbound.ClaimConsumed {
			t.Fatalf("first claim = %+v, %v", res, err)
		}
		_, err := e.uses.Claim(t.Context(), e.claim(a, job))
		if got := reasonOf(t, err); got != approval.ReasonIntrusiveNoRetry {
			t.Errorf("reason = %s, want %s", got, approval.ReasonIntrusiveNoRetry)
		}
		if c, rows := e.counts(t, a.ID); c != 1 || rows != 1 {
			t.Errorf("counter %d, rows %d, want 1 and 1", c, rows)
		}
	})
}

// A retry claim is refused when the approval changed since the first claim: a
// revoked approval does not let a job claim again.
func TestRetryClaimIsRefusedAfterRevocation(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxAttempts: 2})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 2)
	if _, err := e.uses.Claim(t.Context(), e.claim(a, job)); err != nil {
		t.Fatal(err)
	}
	e.revoke(t, a)
	_, err := e.uses.Claim(t.Context(), e.claim(a, job))
	if got := reasonOf(t, err); got != approval.ReasonRevoked {
		t.Errorf("reason = %s, want %s", got, approval.ReasonRevoked)
	}
}

func (e *env) revoke(t *testing.T, a pgtest.Approval) {
	t.Helper()
	tag, err := e.api.Exec(t.Context(), `UPDATE morningstar.approvals SET status = 'revoked', revoked_at = now(), revoked_by = $2, revoke_reason = 'test', updated_at = now(), version = version + 1 WHERE id = $1`, a.ID, e.b.UserID)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("revoke: %v (%d rows)", err, tag.RowsAffected())
	}
}

// Every refusal consumes nothing, and the reason is the stable code of the
// domain model (internal/domain/approval).
func TestRefusedClaimsConsumeNothing(t *testing.T) {
	e := newEnv(t)
	other := newID()
	tests := []struct {
		name   string
		spec   pgtest.ApprovalSpec
		mutate func(*outbound.ApprovalClaim)
		want   approval.Reason
	}{
		{"pending", pgtest.ApprovalSpec{Status: "pending"}, nil, approval.ReasonNotApproved},
		{"rejected", pgtest.ApprovalSpec{Status: "rejected"}, nil, approval.ReasonNotApproved},
		{"revoked", pgtest.ApprovalSpec{Status: "revoked"}, nil, approval.ReasonRevoked},
		{"expired", pgtest.ApprovalSpec{TTL: -time.Hour}, nil, approval.ReasonExpired},
		{"another operation", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.Operation[0] ^= 0xff }, approval.ReasonOperationMismatch},
		{"another scope version", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.Binding.ScopeVersionID = other }, approval.ReasonScopeChanged},
		{"another policy snapshot", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.Binding.PolicySnapshot[0] ^= 0xff }, approval.ReasonPolicyChanged},
		{"another validation", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.Binding.ValidationID = other }, approval.ReasonValidationChanged},
		{"unknown approval", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.ApprovalID = other }, approval.ReasonNotApproved},
		{"claim epoch zero", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.ClaimEpoch = 0 }, approval.ReasonInvalidInput},
		{"zero operation hash", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.Operation = [32]byte{} }, approval.ReasonInvalidInput},
		{"zero policy snapshot", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.Binding.PolicySnapshot = [32]byte{} }, approval.ReasonInvalidInput},
		{"empty validation", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.Binding.ValidationID = "" }, approval.ReasonInvalidInput},
		{"job id that is not a uuid", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.JobID = "1; DROP TABLE x" }, approval.ReasonInvalidInput},
		{"approval id that is not a uuid", pgtest.ApprovalSpec{}, func(c *outbound.ApprovalClaim) { c.ApprovalID = "x" }, approval.ReasonInvalidInput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := e.c.SeedApproval(t, e.b, tc.spec)
			job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
			claim := e.claim(a, job)
			if tc.mutate != nil {
				tc.mutate(&claim)
			}
			_, err := e.uses.Claim(t.Context(), claim)
			if err == nil {
				t.Fatal("the claim was accepted")
			}
			if got := reasonOf(t, err); got != tc.want {
				t.Errorf("reason = %s, want %s", got, tc.want)
			}
			if c, rows := e.counts(t, a.ID); c != 0 || rows != 0 {
				t.Errorf("a refused claim left counter %d and %d rows", c, rows)
			}
		})
	}
}

func TestExhaustedApprovalRefusesTheNextJob(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 1})
	j1, j2 := e.c.SeedJob(t, e.b, a, "low_impact", 1), e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if _, err := e.uses.Claim(t.Context(), e.claim(a, j1)); err != nil {
		t.Fatal(err)
	}
	_, err := e.uses.Claim(t.Context(), e.claim(a, j2))
	if got := reasonOf(t, err); got != approval.ReasonExhausted {
		t.Errorf("reason = %s, want %s", got, approval.ReasonExhausted)
	}
	if c, rows := e.counts(t, a.ID); c != 1 || rows != 1 {
		t.Errorf("counter %d, rows %d, want 1 and 1", c, rows)
	}
}

// A revocation that commits before a claim stops it. A claim that commits first
// is consumed and the revocation then stops the launch (E3). Either order keeps
// the counter equal to the rows.
func TestRevocationAndClaimRaceKeepsTheCounterTrue(t *testing.T) {
	e := newEnv(t)
	for round := 0; round < 10; round++ {
		a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
		job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
		var wg sync.WaitGroup
		start := make(chan struct{})
		var claimErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, claimErr = e.uses.Claim(context.Background(), e.claim(a, job))
		}()
		go func() {
			defer wg.Done()
			<-start
			e.revoke(t, a)
		}()
		close(start)
		wg.Wait()
		consumed, rows := e.counts(t, a.ID)
		if consumed != rows {
			t.Fatalf("round %d: counter %d differs from %d rows", round, consumed, rows)
		}
		if claimErr == nil && consumed != 1 {
			t.Fatalf("round %d: the claim succeeded but %d uses exist", round, consumed)
		}
		if claimErr != nil {
			if got := reasonOf(t, claimErr); got != approval.ReasonRevoked || consumed != 0 {
				t.Fatalf("round %d: refused with %s and %d uses", round, got, consumed)
			}
		}
		// After both, the launch check refuses in either order.
		if err := e.uses.CheckLaunch(t.Context(), e.launch(a, job)); err == nil {
			t.Fatalf("round %d: the launch of a revoked approval was allowed", round)
		}
	}
}

func TestCheckLaunch(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)

	err := e.uses.CheckLaunch(t.Context(), e.launch(a, job))
	if got := reasonOf(t, err); got != approval.ReasonUseNotClaimed {
		t.Errorf("before the claim: reason = %s, want %s", got, approval.ReasonUseNotClaimed)
	}
	if _, err := e.uses.Claim(t.Context(), e.claim(a, job)); err != nil {
		t.Fatal(err)
	}
	if err := e.uses.CheckLaunch(t.Context(), e.launch(a, job)); err != nil {
		t.Errorf("after the claim: %v", err)
	}
	if c, rows := e.counts(t, a.ID); c != 1 || rows != 1 {
		t.Errorf("the check consumed something: counter %d, rows %d", c, rows)
	}
	other := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if got := reasonOf(t, e.uses.CheckLaunch(t.Context(), e.launch(a, other))); got != approval.ReasonUseNotClaimed {
		t.Errorf("another job: reason = %s, want %s", got, approval.ReasonUseNotClaimed)
	}

	bad := e.launch(a, job)
	bad.Operation[0] ^= 0xff
	if got := reasonOf(t, e.uses.CheckLaunch(t.Context(), bad)); got != approval.ReasonOperationMismatch {
		t.Errorf("another operation: reason = %s", got)
	}
	bad = e.launch(a, job)
	bad.Binding.ScopeVersionID = newID()
	if got := reasonOf(t, e.uses.CheckLaunch(t.Context(), bad)); got != approval.ReasonScopeChanged {
		t.Errorf("another scope: reason = %s", got)
	}

	e.revoke(t, a)
	if got := reasonOf(t, e.uses.CheckLaunch(t.Context(), e.launch(a, job))); got != approval.ReasonRevoked {
		t.Errorf("after revocation: reason = %s, want %s", got, approval.ReasonRevoked)
	}
}

func TestCheckLaunchRefusesAnExpiredApproval(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if _, err := e.uses.Claim(t.Context(), e.claim(a, job)); err != nil {
		t.Fatal(err)
	}
	// Expiry stops a new launch (D-48).
	e.c.Expire(t, a.ID)
	if got := reasonOf(t, e.uses.CheckLaunch(t.Context(), e.launch(a, job))); got != approval.ReasonExpired {
		t.Errorf("reason = %s, want %s", got, approval.ReasonExpired)
	}
}

// CC-15: a claim made under another recovery generation, or while the database
// is being reconciled, is refused and consumes nothing.
func TestClaimIsRefusedUnderTheWrongRecoveryGeneration(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)

	stale := e.claim(a, job)
	stale.Generation = 2
	if _, err := e.uses.Claim(t.Context(), stale); !errors.Is(err, outbound.ErrRecoveryGeneration) {
		t.Errorf("generation 2 against 1: error = %v, want ErrRecoveryGeneration", err)
	}
	staleLaunch := e.launch(a, job)
	staleLaunch.Generation = 2
	if err := e.uses.CheckLaunch(t.Context(), staleLaunch); !errors.Is(err, outbound.ErrRecoveryGeneration) {
		t.Errorf("launch under generation 2: error = %v", err)
	}

	conn := e.c.Owner(t)
	if _, err := conn.Exec(t.Context(), `UPDATE morningstar.recovery_state SET state = 'reconciling', generation = generation + 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.uses.Claim(t.Context(), e.claim(a, job)); !errors.Is(err, outbound.ErrRecoveryGeneration) {
		t.Errorf("while reconciling: error = %v, want ErrRecoveryGeneration", err)
	}
	if c, rows := e.counts(t, a.ID); c != 0 || rows != 0 {
		t.Errorf("counter %d, rows %d, want 0 and 0", c, rows)
	}
}

// A reconciliation cannot start while a claim holds the shared lock on
// recovery_state, so a claim never straddles a generation change (CC-15).
func TestAClaimBlocksReconciliationUntilItCommits(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)

	inClaim := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- e.tx.InTx(context.Background(), func(ctx context.Context) error {
			if _, err := e.uses.Claim(ctx, e.claim(a, job)); err != nil {
				return err
			}
			close(inClaim)
			<-release
			return nil
		})
	}()
	<-inClaim

	api := e.c.Conn(t, persistence.RoleAPI)
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	tx, err := api.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `SELECT 1 FROM morningstar.recovery_state WHERE singleton FOR UPDATE`)
	_ = tx.Rollback(context.Background())
	// The lock wait must end by the timeout. A privilege error would also be an
	// error, and would prove nothing about the lock.
	if err == nil {
		t.Error("the exclusive lock on recovery_state was granted while a claim was open")
	} else if !errors.Is(err, context.DeadlineExceeded) && pgCode(err) != "57014" {
		t.Errorf("the lock attempt failed for another reason than waiting: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// The worker role has no UPDATE on approvals. A direct insert of a use without
// the counter is stopped at commit by the parity check (DB-13).
func TestWorkerCannotConsumeWithoutTheClaimFunction(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	w := e.c.Conn(t, persistence.RoleWorker)

	if _, err := w.Exec(t.Context(), `UPDATE morningstar.approvals SET uses_consumed = 1 WHERE id = $1`, a.ID); err == nil {
		t.Error("the worker updated approvals")
	}
	if _, err := w.Exec(t.Context(), `UPDATE morningstar.approvals SET max_uses = 100 WHERE id = $1`, a.ID); err == nil {
		t.Error("the worker raised max_uses")
	}
	if _, err := w.Exec(t.Context(), `INSERT INTO morningstar.approval_uses (id, approval_id, job_id, claim_epoch) VALUES ($1, $2, $3, 1)`, newID(), a.ID, job); err == nil {
		t.Error("a use without a counter increment was committed")
	}
	if c, rows := e.counts(t, a.ID); c != 0 || rows != 0 {
		t.Errorf("counter %d, rows %d, want 0 and 0", c, rows)
	}
}

// Two approval_uses rows for one (approval, job) are impossible (CC-02, c1).
func TestUseRowIsUniquePerApprovalAndJob(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 3})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if _, err := e.uses.Claim(t.Context(), e.claim(a, job)); err != nil {
		t.Fatal(err)
	}
	conn := e.c.Owner(t)
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context()) //nolint:errcheck
	if _, err := tx.Exec(t.Context(), `UPDATE morningstar.approvals SET uses_consumed = 2 WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO morningstar.approval_uses (id, approval_id, job_id, claim_epoch) VALUES ($1, $2, $3, 2)`, newID(), a.ID, job); err == nil {
		t.Error("a second use row for the same approval and job was accepted")
	}
}
