package scope_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appscope "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const (
	programID    = "0198a2b4-0001-7222-8333-444455556666"
	assessmentID = "0198a2b4-0002-7222-8333-444455556666"
	jobID        = "0198a2b4-0003-7222-8333-444455556666"
	operationID  = "0198a2b4-0004-7222-8333-444455556666"
	approvalID   = "0198a2b4-0005-7222-8333-444455556666"
	scopeV1      = "0198a2b4-0006-7222-8333-444455556666"
	scopeV2      = "0198a2b4-0007-7222-8333-444455556666"
	validationID = "0198a2b4-0008-7222-8333-444455556666"
	userID       = "0198a2b4-0009-7222-8333-444455556666"
)

var now = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

type inputs struct {
	in  outbound.ScopeInput
	err error
}

func (i *inputs) Load(context.Context, outbound.JobRecord) (outbound.ScopeInput, error) {
	return i.in, i.err
}

type decisions struct {
	rows []outbound.PolicyDecision
	err  error
}

func (d *decisions) Insert(_ context.Context, p outbound.PolicyDecision) error {
	if d.err != nil {
		return d.err
	}
	d.rows = append(d.rows, p)
	return nil
}

type sink struct{ recorded []audit.Event }

func (s *sink) Require(context.Context, audit.Event) error { return nil }
func (s *sink) Record(_ context.Context, e audit.Event) error {
	s.recorded = append(s.recorded, e)
	return nil
}
func (s *sink) Gate(context.Context) error { return nil }

func op(t *testing.T, cat operation.Category, target string) operation.Operation {
	t.Helper()
	o, err := operation.New(operation.Spec{
		AssessmentID: assessmentID, ToolKey: "httpx", Kind: "probe", Category: cat, Target: target, Port: 443,
		Params: map[string]string{"path_depth": "1"}, Destinations: []string{target + ":443"},
		Limits: operation.Limits{MaxConcurrency: 2, MaxDuration: 10 * time.Minute, RequestsPerMinute: 60}, ManifestDigest: [32]byte{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func rules(t *testing.T, host string) scope.RuleSet {
	t.Helper()
	r, err := scope.NewRule(scope.KindDomain, false, host, "", scope.DefaultPrefixMin())
	if err != nil {
		t.Fatal(err)
	}
	return scope.RuleSet{r}
}

func record(risk job.Risk) outbound.JobRecord {
	return outbound.JobRecord{
		ID: jobID, ProgramID: programID, AssessmentID: assessmentID, OperationID: operationID, ApprovalID: approvalID, ScopeVersionID: scopeV1, ValidationID: validationID,
		Risk: risk, State: job.Queued, Control: job.ControlNone, MaxAttempts: 3,
	}
}

func policy(version string, snapshot byte) scope.PolicyState {
	return scope.PolicyState{ScopeVersionID: version, PolicySnapshot: [32]byte{snapshot}}
}

func newEvaluator(in *inputs, d *decisions, s *sink) *appscope.Evaluator {
	n := 0
	return appscope.New(appscope.Deps{
		Inputs: in, Decisions: d, Audit: s, Now: func() time.Time { return now },
		NewID: func() string { n++; return "0198a2b4-00a" + string(rune('0'+n)) + "-7222-8333-444455556666" },
	})
}

func passive(t *testing.T) *inputs {
	return &inputs{in: outbound.ScopeInput{
		Operation: op(t, operation.CategoryPassiveRecon, "app.example.com"), Rules: rules(t, "app.example.com"),
		Policy: policy(scopeV1, 9), Validation: scope.ValidationState{ID: validationID, Current: true},
	}}
}

// approved builds an approval of the operation under scope version 1, as a person made it.
func approved(t *testing.T, o operation.Operation, b approval.Binding) approval.Approval {
	t.Helper()
	human, err := actor.New(actor.KindHuman, userID)
	if err != nil {
		t.Fatal(err)
	}
	a, err := approval.Request(approvalID, o, human, b, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	terms, _ := approval.DefaultTerms(o.Class(), now)
	if a, err = a.Approve(human, terms, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAPassiveJobInScopeIsAllowedAndTheDecisionIsRecorded(t *testing.T) {
	d, s := &decisions{}, &sink{}
	e := newEvaluator(passive(t), d, s)
	res, err := e.Evaluate(t.Context(), appscope.Request{Point: scope.PointE2, Job: record(job.Passive)})
	if err != nil || !res.Allowed() || res.Verdict.Authorized.IsZero() {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if len(d.rows) != 1 || d.rows[0].ID != res.DecisionID || d.rows[0].Outcome != scope.OutcomeAllow || d.rows[0].Point != scope.PointE2 || d.rows[0].JobID != jobID {
		t.Fatalf("decisions = %+v", d.rows)
	}
	if d.rows[0].ScopeVersionID != scopeV1 || d.rows[0].PolicySnapshot != [32]byte{9} || len(s.recorded) != 0 {
		t.Errorf("decision = %+v, audited %d", d.rows[0], len(s.recorded))
	}
}

func TestATargetOutOfScopeIsDeniedAndRecorded(t *testing.T) {
	in := passive(t)
	in.in.Rules = rules(t, "other.example.com")
	d := &decisions{}
	res, err := newEvaluator(in, d, &sink{}).Evaluate(t.Context(), appscope.Request{Point: scope.PointE2, Job: record(job.Passive)})
	if err != nil || res.Allowed() || !res.Verdict.Authorized.IsZero() || len(res.Verdict.Reasons) != 1 {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if len(d.rows) != 1 || d.rows[0].Outcome != scope.OutcomeDeny {
		t.Fatalf("decisions = %+v", d.rows)
	}
}

func TestAMissingOrFailingSourceIsADenial(t *testing.T) {
	d := &decisions{}
	in := &inputs{err: errors.New("the policy store is down")}
	res, err := newEvaluator(in, d, &sink{}).Evaluate(t.Context(), appscope.Request{Point: scope.PointE2, Job: record(job.Passive)})
	if err != nil || res.Allowed() || res.Verdict.Reasons[0] != appscope.ReasonInputsUnavailable {
		t.Fatalf("result = %+v, %v", res, err)
	}
	// What the decision records is what the job names. Nothing is taken from a source that failed.
	if len(d.rows) != 1 || d.rows[0].ScopeVersionID != scopeV1 || d.rows[0].ValidationID != validationID || d.rows[0].PolicySnapshot != [32]byte{} {
		t.Errorf("decision = %+v", d.rows)
	}
	// Nothing at all is a denial too.
	res, err = newEvaluator(&inputs{}, &decisions{}, &sink{}).Evaluate(t.Context(), appscope.Request{Point: scope.PointE3, Job: record(job.Passive)})
	if err != nil || res.Allowed() {
		t.Fatalf("an empty source allowed: %+v, %v", res, err)
	}
}

func TestInputsThatAreNotAboutTheJobAreADenial(t *testing.T) {
	for name, edit := range map[string]func(*outbound.ScopeInput, *outbound.JobRecord){
		"another assessment": func(in *outbound.ScopeInput, j *outbound.JobRecord) {
			j.AssessmentID = "0198a2b4-0099-7222-8333-444455556666"
		},
		"another class": func(in *outbound.ScopeInput, j *outbound.JobRecord) { j.Risk = job.LowImpact },
	} {
		in := passive(t)
		j := record(job.Passive)
		edit(&in.in, &j)
		res, err := newEvaluator(in, &decisions{}, &sink{}).Evaluate(t.Context(), appscope.Request{Point: scope.PointE2, Job: j})
		if err != nil || res.Allowed() || res.Verdict.Reasons[0] != appscope.ReasonInputsMismatch {
			t.Errorf("%s: %+v, %v", name, res, err)
		}
	}
}

func TestADecisionThatCannotBeRecordedIsNotReturnedAsAnAnswer(t *testing.T) {
	d := &decisions{err: errors.New("disk full")}
	if res, err := newEvaluator(passive(t), d, &sink{}).Evaluate(t.Context(), appscope.Request{Point: scope.PointE2, Job: record(job.Passive)}); err == nil || res.Allowed() {
		t.Fatalf("result = %+v, %v: an unrecorded allow must not be usable", res, err)
	}
}

func TestTheEvaluatorDecidesOnlyAtTheWorkerPoints(t *testing.T) {
	for _, p := range []scope.Point{scope.PointE1, scope.PointInvalid, scope.Point(9)} {
		if _, err := newEvaluator(passive(t), &decisions{}, &sink{}).Evaluate(t.Context(), appscope.Request{Point: p, Job: record(job.Passive)}); err == nil {
			t.Errorf("point %v was accepted", p)
		}
	}
}

func TestAnApprovalBoundJobIsDeniedWhenWhatItWasApprovedForChanges(t *testing.T) {
	o := op(t, operation.CategoryHTTPProbe, "app.example.com")
	bound := approval.Binding{ValidationID: validationID, ScopeVersionID: scopeV1, PolicySnapshot: [32]byte{9}}
	ap := approved(t, o, bound)
	mk := func(mod func(*outbound.ScopeInput)) *inputs {
		in := outbound.ScopeInput{Operation: o, Rules: rules(t, "app.example.com"), Policy: policy(scopeV1, 9), Validation: scope.ValidationState{ID: validationID, Current: true}, Approval: &ap}
		if mod != nil {
			mod(&in)
		}
		return &inputs{in: in}
	}
	req := appscope.Request{Point: scope.PointE2, Job: record(job.LowImpact)}

	res, err := newEvaluator(mk(nil), &decisions{}, &sink{}).Evaluate(t.Context(), req)
	if err != nil || !res.Allowed() {
		t.Fatalf("the plain case: %+v, %v", res, err)
	}
	for name, c := range map[string]struct {
		mod    func(*outbound.ScopeInput)
		reason string
	}{
		"another scope version":                  {func(in *outbound.ScopeInput) { in.Policy = policy(scopeV2, 9) }, "approval_scope_changed"},
		"another policy snapshot":                {func(in *outbound.ScopeInput) { in.Policy = policy(scopeV1, 8) }, "approval_policy_changed"},
		"a validation that is no longer current": {func(in *outbound.ScopeInput) { in.Validation.Current = false }, scope.ReasonValidationNotCurrent},
		"no approval at all":                     {func(in *outbound.ScopeInput) { in.Approval = nil }, appscope.ReasonInputsMismatch},
	} {
		d := &decisions{}
		res, err := newEvaluator(mk(c.mod), d, &sink{}).Evaluate(t.Context(), req)
		if err != nil || res.Allowed() || !res.Verdict.Authorized.IsZero() || res.Verdict.Reasons[0] != c.reason {
			t.Errorf("%s: %+v, %v (want %s)", name, res, err, c.reason)
		}
		if len(d.rows) != 1 || d.rows[0].Outcome != scope.OutcomeDeny {
			t.Errorf("%s: decisions %+v", name, d.rows)
		}
	}
	// A revoked approval is refused at the claim and at the launch.
	human, _ := actor.New(actor.KindHuman, userID)
	revoked, err := ap.Revoke(human, "test", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []scope.Point{scope.PointE2, scope.PointE3} {
		in := mk(func(in *outbound.ScopeInput) { in.Approval = &revoked })
		res, err := newEvaluator(in, &decisions{}, &sink{}).Evaluate(t.Context(), appscope.Request{Point: p, Job: record(job.LowImpact)})
		if err != nil || res.Allowed() || res.Verdict.Reasons[0] != "approval_revoked" {
			t.Errorf("%v: %+v, %v", p, res, err)
		}
	}
}

func TestADenialIsAuditedWithTheNameOfItsDecision(t *testing.T) {
	in := passive(t)
	in.in.Rules = rules(t, "other.example.com")
	s := &sink{}
	e := newEvaluator(in, &decisions{}, s)
	req := appscope.Request{Point: scope.PointE3, Job: record(job.Passive)}
	res, _ := e.Evaluate(t.Context(), req)
	who, _ := audit.Worker("worker-1")
	e.AuditDenial(t.Context(), who, req, res, "")
	if len(s.recorded) != 1 {
		t.Fatalf("recorded %d events", len(s.recorded))
	}
	ev := s.recorded[0]
	if ev.Action() != "policy.decision_denied" || ev.Outcome() != audit.Denied || ev.Subject().DecisionID != res.DecisionID || ev.Subject().JobID != jobID {
		t.Errorf("event = %+v", ev)
	}
}
