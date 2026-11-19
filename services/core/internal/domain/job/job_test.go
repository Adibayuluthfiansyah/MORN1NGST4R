package job_test

import (
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
)

func TestTheTransitionTableIsTheOneOfTheDesign(t *testing.T) {
	all := []job.State{"", job.Queued, job.AwaitingApproval, job.Running, job.Succeeded, job.Failed, job.Cancelled}
	legal := map[[2]job.State]bool{
		{"", job.Queued}: true, {"", job.AwaitingApproval}: true,
		{job.AwaitingApproval, job.Queued}: true, {job.AwaitingApproval, job.Cancelled}: true,
		{job.Queued, job.Running}: true, {job.Queued, job.Failed}: true, {job.Queued, job.Cancelled}: true,
		{job.Running, job.Succeeded}: true, {job.Running, job.Failed}: true, {job.Running, job.Queued}: true, {job.Running, job.Cancelled}: true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := job.CanMove(from, to); got != legal[[2]job.State{from, to}] {
				t.Errorf("CanMove(%q, %q) = %v", from, to, got)
			}
		}
	}
	for _, s := range []job.State{job.Succeeded, job.Failed, job.Cancelled} {
		if !s.Terminal() {
			t.Errorf("%s is not terminal", s)
		}
	}
	if job.Running.Terminal() || job.State("paused").Valid() || !job.Queued.Valid() {
		t.Error("state predicates")
	}
}

func TestOnlyActiveRisksNeedAnApproval(t *testing.T) {
	for r, want := range map[job.Risk]bool{job.Passive: false, job.AI: false, job.LowImpact: true, job.Intrusive: true, "": false} {
		if got := r.NeedsApproval(); got != want {
			t.Errorf("%q: %v", r, got)
		}
	}
	if !job.AttemptLaunching.Live() || !job.AttemptRunning.Live() || job.AttemptLost.Live() || job.AttemptKilled.Live() {
		t.Error("attempt liveness")
	}
}

func retry(mod func(*job.RetryInput)) job.RetryInput {
	in := job.RetryInput{Policy: job.RetryBounded, Risk: job.Passive, Cause: job.CauseLeaseLost, Attempts: 1, MaxAttempts: 3, Control: job.ControlNone, TerminationConfirmed: true}
	if mod != nil {
		mod(&in)
	}
	return in
}

func TestARetryNeedsEveryCondition(t *testing.T) {
	if got := job.DecideRetry(retry(nil)); got.Action != job.ActionRequeue {
		t.Fatalf("the plain case = %+v", got)
	}
	for name, c := range map[string]struct {
		mod  func(*job.RetryInput)
		want job.Outcome
	}{
		"termination not confirmed":     {func(i *job.RetryInput) { i.TerminationConfirmed = false }, job.Outcome{Action: job.ActionWait}},
		"no retry policy":               {func(i *job.RetryInput) { i.Policy = job.RetryNone }, job.Outcome{job.ActionFail, job.FailureRetryDisabled}},
		"an unknown retry policy":       {func(i *job.RetryInput) { i.Policy = "always" }, job.Outcome{job.ActionFail, job.FailureRetryDisabled}},
		"attempts used up":              {func(i *job.RetryInput) { i.Attempts = 3 }, job.Outcome{job.ActionFail, job.FailureAttemptsExhausted}},
		"more attempts than allowed":    {func(i *job.RetryInput) { i.Attempts = 4 }, job.Outcome{job.ActionFail, job.FailureAttemptsExhausted}},
		"no attempt allowed at all":     {func(i *job.RetryInput) { i.MaxAttempts = 0 }, job.Outcome{job.ActionFail, job.FailureAttemptsExhausted}},
		"intrusive lease loss":          {func(i *job.RetryInput) { i.Risk = job.Intrusive }, job.Outcome{job.ActionFail, job.FailureLeaseExpired}},
		"intrusive failure":             {func(i *job.RetryInput) { i.Risk = job.Intrusive; i.Cause = job.CauseExecution }, job.Outcome{job.ActionFail, job.FailureIntrusiveNoRetry}},
		"stopped":                       {func(i *job.RetryInput) { i.Cause = job.CauseStopped }, job.Outcome{job.ActionFail, job.FailureStopped}},
		"a stop request in force":       {func(i *job.RetryInput) { i.Control = job.ControlStopRequested }, job.Outcome{job.ActionFail, job.FailureStopped}},
		"a confirmed stop":              {func(i *job.RetryInput) { i.Control = job.ControlStopConfirmed }, job.Outcome{job.ActionFail, job.FailureStopped}},
		"a failed stop":                 {func(i *job.RetryInput) { i.Control = job.ControlStopFailed }, job.Outcome{job.ActionFail, job.FailureStopped}},
		"policy denial":                 {func(i *job.RetryInput) { i.Cause = job.CausePolicyDenial }, job.Outcome{job.ActionFail, job.FailurePolicyDenial}},
		"timeout":                       {func(i *job.RetryInput) { i.Cause = job.CauseTimeout }, job.Outcome{job.ActionFail, job.FailureTimedOut}},
		"an unknown cause":              {func(i *job.RetryInput) { i.Cause = "weather" }, job.Outcome{job.ActionFail, job.FailureNotRetryable}},
		"a failed execution is retried": {func(i *job.RetryInput) { i.Cause = job.CauseExecution }, job.Outcome{Action: job.ActionRequeue}},
		"a low-impact job may retry":    {func(i *job.RetryInput) { i.Risk = job.LowImpact }, job.Outcome{Action: job.ActionRequeue}},
	} {
		if got := job.DecideRetry(retry(c.mod)); got != c.want {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
	}
	if got := job.DecideRetry(job.RetryInput{}); got.Action != job.ActionFail {
		t.Errorf("an empty input = %+v: it must fail", got)
	}
}

// A stop always wins, whatever else would allow a retry.
func TestAStopBeatsEveryOtherReasonToRetry(t *testing.T) {
	for _, risk := range []job.Risk{job.Passive, job.LowImpact, job.AI} {
		for _, cause := range []job.Cause{job.CauseLeaseLost, job.CauseExecution} {
			in := retry(func(i *job.RetryInput) { i.Risk, i.Cause, i.Control = risk, cause, job.ControlStopRequested })
			if got := job.DecideRetry(in); got.Action != job.ActionFail || got.FailureClass != job.FailureStopped {
				t.Errorf("%s %s: %+v", risk, cause, got)
			}
		}
	}
}

func TestTheDefaultTimingKeepsTheStopObjective(t *testing.T) {
	d := job.DefaultTiming()
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := d.ControlPoll + d.StopTimeout; got > job.StopObjective {
		t.Errorf("a stop on a live worker takes up to %v", got)
	}
	if got := d.LeaseTTL + d.Reap + d.StopTimeout; got > job.StopObjective {
		t.Errorf("the attempt of a dead worker is stopped after up to %v", got)
	}
	for name, mod := range map[string]func(*job.Timing){
		"zero lease":                     func(t *job.Timing) { t.LeaseTTL = 0 },
		"zero heartbeat":                 func(t *job.Timing) { t.Heartbeat = 0 },
		"zero control poll":              func(t *job.Timing) { t.ControlPoll = 0 },
		"zero reap":                      func(t *job.Timing) { t.Reap = 0 },
		"zero stop timeout":              func(t *job.Timing) { t.StopTimeout = 0 },
		"zero watermark":                 func(t *job.Timing) { t.Watermark = 0 },
		"a lease that one beat can miss": func(t *job.Timing) { t.LeaseTTL = t.Heartbeat * 2 },
		"a slow kill":                    func(t *job.Timing) { t.StopTimeout = 29 * time.Second },
		"a long lease":                   func(t *job.Timing) { t.LeaseTTL = 25 * time.Second },
	} {
		tm := job.DefaultTiming()
		mod(&tm)
		if err := tm.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
