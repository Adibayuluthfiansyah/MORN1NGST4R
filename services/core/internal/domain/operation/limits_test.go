package operation_test

import (
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// D-55 and D-71: an active operation with no established numeric limits is
// blocked. The package invents no default number.
func TestLimitsEstablished(t *testing.T) {
	ok := operation.Limits{MaxConcurrency: 2, MaxDuration: time.Minute, RequestsPerMinute: 60}
	if !ok.Established() {
		t.Fatal("complete limits are not established")
	}
	cases := map[string]operation.Limits{
		"zero":                 {},
		"no concurrency":       {MaxDuration: time.Minute, RequestsPerMinute: 60},
		"no duration":          {MaxConcurrency: 2, RequestsPerMinute: 60},
		"no rate":              {MaxConcurrency: 2, MaxDuration: time.Minute},
		"negative concurrency": {MaxConcurrency: -1, MaxDuration: time.Minute, RequestsPerMinute: 60},
		"negative duration":    {MaxConcurrency: 2, MaxDuration: -time.Second, RequestsPerMinute: 60},
		"negative rate":        {MaxConcurrency: 2, MaxDuration: time.Minute, RequestsPerMinute: -1},
		"rate below jobs":      {MaxConcurrency: 5, MaxDuration: time.Minute, RequestsPerMinute: 4},
	}
	for name, l := range cases {
		if l.Established() {
			t.Errorf("%s: Established() = true for %+v", name, l)
		}
		if l.PerJobRequestsPerMinute() != 0 {
			t.Errorf("%s: PerJobRequestsPerMinute() = %d, want 0 for limits that are not established", name, l.PerJobRequestsPerMinute())
		}
	}
	// The boundary: one request per minute for each job is enough.
	if l := (operation.Limits{MaxConcurrency: 5, MaxDuration: time.Second, RequestsPerMinute: 5}); !l.Established() || l.PerJobRequestsPerMinute() != 1 {
		t.Errorf("5 jobs and 5 requests per minute = %+v, established %v, per job %d", l, l.Established(), l.PerJobRequestsPerMinute())
	}
}

// FR-APPROVAL-006 AC1: jobs under one approval never exceed the approved
// request rate in aggregate.
func TestPerJobRateNeverAddsUpToMoreThanTheApprovedRate(t *testing.T) {
	for c := 1; c <= 40; c++ {
		for rpm := 0; rpm <= 300; rpm++ {
			l := operation.Limits{MaxConcurrency: c, MaxDuration: time.Hour, RequestsPerMinute: rpm}
			per := l.PerJobRequestsPerMinute()
			if !l.Established() {
				if per != 0 {
					t.Fatalf("c=%d rpm=%d: not established but per job = %d", c, rpm, per)
				}
				continue
			}
			if per < 1 || per*c > rpm {
				t.Fatalf("c=%d rpm=%d: per job = %d, aggregate %d", c, rpm, per, per*c)
			}
		}
	}
}
