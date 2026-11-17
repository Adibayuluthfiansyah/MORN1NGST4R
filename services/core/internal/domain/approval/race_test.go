package approval_test

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// TestHostileSequences drives approvals through long random sequences of
// claims, retry claims, launches (allowed and denied), revocations and the
// passing of time, and checks the invariants of the schema after every step:
// the consumed uses never exceed the maximum and never decrease, a job holds at
// most one use, attempts never exceed the maximum, an intrusive job has at most
// one attempt, a revoked approval never gives anything again, and nothing is
// restored. It is the model the database constraints must agree with (M3).
func TestHostileSequences(t *testing.T) {
	const runs, steps = 300, 120
	jobs := []approval.JobID{"job-1", "job-2", "job-3", "job-4", "job-5", "job-6", "job-7", "job-8"}
	for run := 0; run < runs; run++ {
		rng := rand.New(rand.NewPCG(uint64(run), 7))
		intrusive := run%3 == 0
		var a approval.Approval
		if intrusive {
			a = approvedIntrusive(t)
		} else {
			a = approvedLow(t)
		}
		op := a.Operation()
		now := t0
		consumed, wasRevoked := 0, false
		attempts := map[approval.JobID]int{}
		other := spec(operation.CategoryHTTPProbe)
		other.Params = map[string]string{"path_depth": "9"}
		wrong := mustOp(t, other)

		for step := 0; step < steps; step++ {
			job := jobs[rng.IntN(len(jobs))]
			use := op
			if rng.IntN(10) == 0 {
				use = wrong // a changed operation
			}
			b := binding()
			changed := false
			if rng.IntN(12) == 0 {
				b.ScopeVersionID, changed = "scv-9", true // the scope version moved on
			}
			switch rng.IntN(6) {
			case 0, 1:
				next, u, err := a.Claim(job, 1+rng.IntN(5), use, b, now)
				if err == nil {
					if wasRevoked || use.Hash() != op.Hash() || a.Expired(now) || changed {
						t.Fatalf("run %d step %d: a claim was granted that must be denied", run, step)
					}
					a = next
					if u.JobID() != job {
						t.Fatalf("run %d: claim returned the use of another job", run)
					}
				} else if !next.Operation().IsZero() {
					t.Fatalf("run %d step %d: a denied claim returned a usable approval", run, step)
				}
			case 2, 3:
				next, err := a.AuthorizeLaunch(job, use, b, rng.IntN(3), rng.IntN(2) == 0, now)
				if err == nil {
					if wasRevoked || use.Hash() != op.Hash() || a.Expired(now) || changed {
						t.Fatalf("run %d step %d: a launch was granted that must be denied", run, step)
					}
					a = next
					attempts[job]++
				} else if !next.Operation().IsZero() {
					t.Fatalf("run %d step %d: a denied launch returned a usable approval", run, step)
				}
			case 4:
				if rng.IntN(15) == 0 && !wasRevoked {
					next, err := a.Revoke(human(t), "hostile run", now)
					if err != nil {
						t.Fatalf("run %d: revoke: %v", run, err)
					}
					a, wasRevoked = next, true
				}
			case 5:
				now = now.Add(time.Duration(rng.IntN(7*3600)) * time.Second)
			}

			// Invariants.
			uses := a.Uses()
			if a.UsesConsumed() != len(uses) || a.UsesConsumed() > a.MaxUses() {
				t.Fatalf("run %d step %d: %d uses consumed of %d", run, step, a.UsesConsumed(), a.MaxUses())
			}
			if a.UsesConsumed() < consumed {
				t.Fatalf("run %d step %d: a consumed use was restored (%d -> %d)", run, step, consumed, a.UsesConsumed())
			}
			consumed = a.UsesConsumed()
			seen := map[approval.JobID]bool{}
			for _, u := range uses {
				if seen[u.JobID()] {
					t.Fatalf("run %d step %d: job %s holds two uses", run, step, u.JobID())
				}
				seen[u.JobID()] = true
				if u.Attempts() > a.MaxAttempts() {
					t.Fatalf("run %d step %d: %d attempts of %d", run, step, u.Attempts(), a.MaxAttempts())
				}
				if intrusive && u.Attempts() > 1 {
					t.Fatalf("run %d step %d: an intrusive job launched %d times", run, step, u.Attempts())
				}
				if u.Attempts() != attempts[u.JobID()] {
					t.Fatalf("run %d step %d: attempts %d do not match the %d granted launches", run, step, u.Attempts(), attempts[u.JobID()])
				}
			}
			if wasRevoked && a.Status() != approval.StatusRevoked {
				t.Fatalf("run %d step %d: a revoked approval changed status to %v", run, step, a.Status())
			}
		}
		if intrusive && a.UsesConsumed() > 1 {
			t.Fatalf("run %d: an intrusive approval was used %d times", run, a.UsesConsumed())
		}
	}
}

// FR-APPROVAL-006 AC1: the per-job rates of every job that may run together
// add up to no more than the approved rate.
func TestFanOutNeverMultipliesTheApprovedRate(t *testing.T) {
	for conc := 1; conc <= 12; conc++ {
		for rpm := conc; rpm <= conc*7+3; rpm++ {
			s := spec(operation.CategoryHTTPProbe)
			s.Limits = operation.Limits{MaxConcurrency: conc, MaxDuration: time.Hour, RequestsPerMinute: rpm}
			op := mustOp(t, s)
			terms, _ := approval.DefaultTerms(op.Class(), t0)
			a, err := requested(t, op).Approve(human(t), terms, t0)
			if err != nil {
				t.Fatal(err)
			}
			per := a.Operation().Limits().PerJobRequestsPerMinute()
			if per < 1 || per*conc > rpm {
				t.Fatal(fmt.Sprintf("concurrency %d, rate %d: per-job rate %d adds up to %d", conc, rpm, per, per*conc))
			}
			// The launch check never admits more running jobs than the approval binds.
			a, _, err = a.Claim("job-1", 1, op, binding(), t0)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.CheckLaunch("job-1", op, binding(), conc, true, t0); err == nil {
				t.Fatalf("concurrency %d: a launch was admitted with %d jobs already running", conc, conc)
			}
		}
	}
}
