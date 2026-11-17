package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// outDirOps is fakeMainOps with an output folder saved in Settings, and a
// run that writes its envelope into the folder it reports and records the
// folder it was given.
type outDirOps struct {
	*fakeMainOps
	saved string
	given string
}

func (o *outDirOps) outputDir() string { return o.saved }

func (o *outDirOps) run(_, _, _ string, _ []string, _ map[string]string, _, outDir string, _ func(string, bool)) (*RunResultView, error) {
	o.given = outDir
	dir := outDir
	if o.saved != "" && outDir == o.saved {
		dir = filepath.Join(outDir, "acme-pdf-summary-01JRUN")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"status":"ok","runId":"01JRUN","outDir":"`+filepath.ToSlash(dir)+`"}`), 0o600); err != nil {
		return nil, err
	}
	return &RunResultView{Status: "ok", RunID: "01JRUN", OutDir: dir}, nil
}

func TestAnAgentsRunGoesToTheSavedOutputFolder(t *testing.T) {
	own := filepath.Join(t.TempDir(), "subagents", "sa-000000000001", "out")
	for _, tc := range []struct {
		name, saved, want string
	}{
		{"saved", t.TempDir(), ""},
		{"none saved", "", own},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops := &outDirOps{fakeMainOps: &fakeMainOps{}, saved: tc.saved}
			want := tc.want
			if tc.saved != "" {
				want = tc.saved
			}
			env, err := appSubagents{ops: ops}.Run(context.Background(), persistent.SubagentRunSpec{Exact: "acme/pdf-summary@1.0.0", Task: "go", OutDir: own})
			if err != nil {
				t.Fatal(err)
			}
			if ops.given != want {
				t.Fatalf("the run was given %q, want %q", ops.given, want)
			}
			if env.RunID != "01JRUN" {
				t.Fatalf("the envelope read back: %+v", env)
			}
		})
	}
}
