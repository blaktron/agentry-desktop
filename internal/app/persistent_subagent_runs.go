package app

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// maxSubagentRuns bounds the subagent runs an agent's page lists.
const maxSubagentRuns = 50

// PersistentSubagentRunView is one run an agent started of a subagent, as
// its page lists it: the subagent's exact reference, when it started and
// ended, how it ended, and the app's own run, whose page has its receipts.
type PersistentSubagentRunView struct {
	ID       string    `json:"id"`
	Subagent string    `json:"subagent"`
	Started  time.Time `json:"started"`
	Ended    time.Time `json:"ended,omitzero"`
	// Outcome is the run's status (ok, failed, …) or error when it did
	// not run; empty while it runs.
	Outcome string `json:"outcome,omitempty"`
	Error   string `json:"error,omitempty"`
	// RunID is the run's own id, which its page is opened by.
	RunID string `json:"runId,omitempty"`
	// Copied are the files it delivered, as copied into the agent's files.
	Copied []string `json:"copied,omitempty"`
}

// subagentLine is one line of a segment's receipts/subagents.ndjson, the
// record the supervisor keeps of each subagent run.
type subagentLine struct {
	Time     time.Time `json:"time"`
	Event    string    `json:"event"`
	Run      string    `json:"run"`
	Subagent string    `json:"subagent"`
	Outcome  string    `json:"outcome"`
	RunID    string    `json:"runId"`
	Copied   []string  `json:"copied"`
	Error    string    `json:"error"`
}

// subagentRunViews reads the record of every segment of the agent name
// into its runs, newest first, at most maxSubagentRuns. A record that does
// not read is skipped: the list is the page's, never a failure of it.
func subagentRunViews(home, name string) []PersistentSubagentRunView {
	files, _ := filepath.Glob(filepath.Join(persistent.Dir(home, name), "segments", "*", "receipts", "subagents.ndjson"))
	runs := map[string]*PersistentSubagentRunView{}
	for _, f := range files {
		readSubagentLines(f, runs)
	}
	out := make([]PersistentSubagentRunView, 0, len(runs))
	for _, r := range runs {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	if len(out) > maxSubagentRuns {
		out = out[:maxSubagentRuns]
	}
	return out
}

func readSubagentLines(path string, runs map[string]*PersistentSubagentRunView) {
	f, err := os.Open(path) //nolint:gosec // a segment's own receipt under the runner home
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var l subagentLine
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.Run == "" {
			continue
		}
		r := runs[l.Run]
		if r == nil {
			r = &PersistentSubagentRunView{ID: l.Run, Subagent: l.Subagent}
			runs[l.Run] = r
		}
		switch l.Event {
		case "started":
			r.Started = l.Time
		case "ended":
			r.Ended, r.Outcome, r.Error, r.RunID, r.Copied = l.Time, l.Outcome, l.Error, l.RunID, l.Copied
			if r.Started.IsZero() {
				r.Started = l.Time
			}
		}
	}
}
