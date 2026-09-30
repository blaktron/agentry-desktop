package progress

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// job is a job's own progress as a flow declares it: the shared report and
// one fact of its own.
type job struct {
	Report
	Pages int `json:"pages"`
}

// A job's report is the flat JSON every progress dialog reads: the shared
// fields and the job's own, side by side, with no nesting.
func TestAJobsReportIsFlat(t *testing.T) {
	b, err := json.Marshal(job{Report: Report{StartedAt: 1, TimeoutSeconds: 2, MaxTurns: 3, Turns: 4, InputTokens: 5, OutputTokens: 6}, Pages: 7})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"startedAt":1,"timeoutSeconds":2,"maxTurns":3,"turns":4,"inputTokens":5,"outputTokens":6,"pages":7}`
	if string(b) != want {
		t.Fatalf("%s, want %s", b, want)
	}
}

// Start is now, under the job's budgets, with nothing used yet.
func TestStartIsNowUnderTheBudgets(t *testing.T) {
	before := time.Now().UnixMilli()
	r := Start(1800, 40)
	if r.StartedAt < before || r.StartedAt > time.Now().UnixMilli() || r.TimeoutSeconds != 1800 || r.MaxTurns != 40 || r.Turns != 0 || r.InputTokens != 0 || r.OutputTokens != 0 {
		t.Fatalf("start %+v", r)
	}
}

// The meter reports the engine's usage as it arrives, and the job's own
// facts when the job changes them; a change that changes nothing is not
// reported.
func TestTheMeterReportsUsageAndTheJobsOwnFacts(t *testing.T) {
	var got []job
	m := NewMeter(func(j job) { got = append(got, j) }, job{Report: Start(60, 10)})
	m.Emit()
	m.Usage(receipt.Usage{Turns: 2, InputTokens: 200, OutputTokens: 20})
	m.Set(func(j *job) bool { j.Pages = 3; return true })
	m.Set(func(j *job) bool { return false })
	if len(got) != 3 {
		t.Fatalf("reports %+v", got)
	}
	if got[0].Turns != 0 || got[1].Turns != 2 || got[1].InputTokens != 200 || got[1].OutputTokens != 20 || got[1].Pages != 0 {
		t.Fatalf("reports %+v", got)
	}
	if last := got[2]; last.Turns != 2 || last.Pages != 3 || last.TimeoutSeconds != 60 || last.MaxTurns != 10 || last.StartedAt != got[0].StartedAt {
		t.Fatalf("last report %+v", last)
	}
}

// With no one to report to, the meter does nothing, and the job's own
// change is not even asked for.
func TestAMeterWithNoReportDoesNothing(t *testing.T) {
	m := NewMeter[job](nil, job{})
	m.Emit()
	m.Usage(receipt.Usage{Turns: 1})
	m.Set(func(*job) bool { t.Fatal("a change was asked for with no one to report to"); return true })
}

// Reports from the engine's usage and from the job's own watcher are made
// under one lock, so each is whole and they arrive in the order they were
// made (-race holds the first half).
func TestReportsFromTwoGoroutinesArriveWholeAndInOrder(t *testing.T) {
	var mu sync.Mutex
	var got []job
	m := NewMeter(func(j job) { mu.Lock(); got = append(got, j); mu.Unlock() }, job{Report: Start(0, 0)})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 1; i <= 100; i++ {
			m.Usage(receipt.Usage{Turns: i, InputTokens: int64(i), OutputTokens: int64(i)})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 1; i <= 100; i++ {
			m.Set(func(j *job) bool { j.Pages = i; return true })
		}
	}()
	wg.Wait()
	turns, pages := 0, 0
	for _, g := range got {
		if g.Turns < turns || g.Pages < pages || int64(g.Turns) != g.InputTokens {
			t.Fatalf("a report went back or was torn: %+v after turns %d, pages %d", g, turns, pages)
		}
		turns, pages = g.Turns, g.Pages
	}
	if turns != 100 || pages != 100 {
		t.Fatalf("last report turns %d pages %d", turns, pages)
	}
}

// Reporting says whether anyone is told, so a job's own watcher can stay
// off when no one is.
func TestReportingSaysWhetherAnyoneIsTold(t *testing.T) {
	if NewMeter[job](nil, job{}).Reporting() {
		t.Fatal("a meter with no report says it is reporting")
	}
	if !NewMeter(func(job) {}, job{}).Reporting() {
		t.Fatal("a meter with a report says it is not reporting")
	}
}
