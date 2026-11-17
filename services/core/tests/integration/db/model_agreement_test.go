package db_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// The SQL claim function is the database form of CheckClaim and Claim of the M2
// model. This test builds the same scenario in both and requires the same
// verdict and the same reason code. A change to either side that breaks the
// agreement fails here (TD-M2-05, M3 part).

var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func modelOp(t *testing.T, cat operation.Category, target string) operation.Operation {
	t.Helper()
	op, err := operation.New(operation.Spec{
		AssessmentID: "asm-1", ToolKey: "httpx", Kind: "probe", Category: cat, Target: target, Port: 443,
		Params:         map[string]string{"path_depth": "1"},
		Destinations:   []string{target + ":443"},
		Limits:         operation.Limits{MaxConcurrency: 2, MaxDuration: 10 * time.Minute, RequestsPerMinute: 60},
		ManifestDigest: [32]byte{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func TestSQLClaimAgreesWithTheDomainModel(t *testing.T) {
	human, err := actor.New(actor.KindHuman, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	other := modelOp(t, operation.CategoryHTTPProbe, "other.example.com")

	type scenario struct {
		name string
		// class of the approved operation
		category operation.Category
		status   string        // DB status: approved, pending, revoked
		ttl      time.Duration // DB lifetime
		maxUses  int
		// priorJobs claim first (in both worlds) and leave the approval with fewer free uses.
		priorJobs int
		// retry: the job under test already holds a use.
		retry bool
		// the claim under test differs from the approval
		wrongOp     bool
		wrongScope  bool
		wrongPolicy bool
		wrongValid  bool
	}
	scenarios := []scenario{
		{name: "approved low impact", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 2},
		{name: "approved intrusive", category: operation.CategoryStateChangingReplay, status: "approved", maxUses: 1},
		{name: "pending", category: operation.CategoryHTTPProbe, status: "pending", maxUses: 2},
		{name: "revoked", category: operation.CategoryHTTPProbe, status: "revoked", maxUses: 2},
		{name: "expired", category: operation.CategoryHTTPProbe, status: "approved", ttl: -time.Hour, maxUses: 2},
		{name: "another operation", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 2, wrongOp: true},
		{name: "another scope version", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 2, wrongScope: true},
		{name: "another policy snapshot", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 2, wrongPolicy: true},
		{name: "another validation", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 2, wrongValid: true},
		{name: "last use free", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 2, priorJobs: 1},
		{name: "exhausted", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 2, priorJobs: 2},
		{name: "retry of a low impact job", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 1, retry: true},
		{name: "retry of an intrusive job", category: operation.CategoryStateChangingReplay, status: "approved", maxUses: 1, retry: true},
		{name: "retry of a job under a revoked approval", category: operation.CategoryHTTPProbe, status: "revoked", maxUses: 1, retry: true},
		{name: "wrong operation checked before exhaustion", category: operation.CategoryHTTPProbe, status: "approved", maxUses: 1, priorJobs: 1, wrongOp: true},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			e := newEnv(t) // a fresh database: an operation is unique per assessment
			op := modelOp(t, sc.category, "app.example.com")
			risk := op.Class().String()
			var hash [32]byte = op.Hash()

			// Database side. A revoked approval is first approved, used and then revoked
			// when the scenario needs prior claims, so that prior uses exist.
			spec := pgtest.ApprovalSpec{Risk: risk, MaxUses: sc.maxUses, OperationHash: hash[:], TTL: sc.ttl}
			finalStatus := sc.status
			if sc.status == "revoked" && (sc.retry || sc.priorJobs > 0) {
				spec.Status = "approved"
			} else {
				spec.Status = sc.status
			}
			dbApproval := e.c.SeedApproval(t, e.b, spec)

			// Model side.
			bind := e.binding()
			terms, ok := approval.DefaultTerms(op.Class(), t0)
			if !ok {
				t.Fatal("no default terms")
			}
			terms.MaxUses = sc.maxUses
			if sc.ttl < 0 {
				terms.ExpiresAt = t0.Add(time.Hour)
			}
			if op.Class() == operation.ClassIntrusive {
				terms.ReauthGrantID = "grant-1"
			}
			model, err := approval.Request("apr-1", op, human, bind, t0)
			if err != nil {
				t.Fatal(err)
			}
			if sc.status != "pending" {
				model, err = model.Approve(human, terms, t0)
				if err != nil {
					t.Fatal(err)
				}
			}
			now := t0.Add(time.Minute)
			if sc.ttl < 0 {
				now = t0.Add(2 * time.Hour) // after the expiry of the model approval
			}

			claimFor := func(job string, epoch int) (outbound.ApprovalClaim, approval.Binding, operation.Operation) {
				c := e.claim(dbApproval, job)
				c.ClaimEpoch = epoch
				b, o := bind, op
				if sc.wrongOp {
					c.Operation = other.Hash()
					o = other
				}
				if sc.wrongScope {
					c.Binding.ScopeVersionID = newID()
					b.ScopeVersionID = c.Binding.ScopeVersionID
				}
				if sc.wrongPolicy {
					c.Binding.PolicySnapshot[0] ^= 0xff
					b.PolicySnapshot = c.Binding.PolicySnapshot
				}
				if sc.wrongValid {
					c.Binding.ValidationID = newID()
					b.ValidationID = c.Binding.ValidationID
				}
				return c, b, o
			}
			// Prior claims use the right operation and bindings.
			prior := func(job string) {
				c := e.claim(dbApproval, job)
				if _, err := e.uses.Claim(t.Context(), c); err != nil {
					t.Fatalf("prior claim: %v", err)
				}
				model, _, err = model.Claim(approval.JobID(job), 1, op, bind, now)
				if err != nil {
					t.Fatalf("prior model claim: %v", err)
				}
			}

			for i := 0; i < sc.priorJobs; i++ {
				prior(e.c.SeedJob(t, e.b, dbApproval, risk, 1))
			}
			job := e.c.SeedJob(t, e.b, dbApproval, risk, 1)
			if sc.retry {
				prior(job)
			}
			if finalStatus == "revoked" {
				if spec.Status == "approved" {
					e.revoke(t, dbApproval) // the seed revoked it already otherwise
				}
				model, err = model.Revoke(human, "test", now)
				if err != nil {
					t.Fatal(err)
				}
			}

			claim, mb, mo := claimFor(job, 1)
			_, dbErr := e.uses.Claim(t.Context(), claim)
			modelErr := model.CheckClaim(approval.JobID(job), mo, mb, now)

			switch {
			case dbErr == nil && modelErr == nil:
			case dbErr == nil || modelErr == nil:
				t.Fatalf("database says %v, model says %v", dbErr, modelErr)
			default:
				var d *approval.DenialError
				var m *approval.DenialError
				if !errors.As(dbErr, &d) || !errors.As(modelErr, &m) {
					t.Fatalf("database %v, model %v: not denials", dbErr, modelErr)
				}
				if d.Reason != m.Reason {
					t.Errorf("database reason %s, model reason %s", d.Reason, m.Reason)
				}
			}
		})
	}
}
