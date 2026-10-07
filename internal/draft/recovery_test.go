package draft

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/webscan"
)

// Replay the recorded Qwen revision at the desktop's runner boundary: a
// refused final message still opens in the Builder after normal validation
// and AI Review, without spending the one repair turn on its prose prefix.
func TestTheRecordedRevisionOpensInTheDesktopAfterAProsePreamble(t *testing.T) {
	isolate(t)
	saveKey(t)
	message, err := os.ReadFile("testdata/vultr-revision-last-message.txt")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Prepare("https://registry.example", "Revise the Vultr setup wizard.", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var runs []localrun.Run
	first := fakeRun(t, &runs, answer, nil)
	checks := &fakeChecker{answers: []*drafter.Check{passCheck()}}
	var refused *receipt.Envelope
	res, err := Run(context.Background(), Request{Plan: p, RunID: "replay-716", Namespace: "alice",
		Scanner: func(string) (webscan.Scanner, error) {
			return &fakeScan{res: webscan.Result{Verdict: webscan.Pass}}, nil
		},
		Runner: func(ctx context.Context, r localrun.Run) (*receipt.Envelope, error) {
			if len(runs) > 0 {
				t.Fatal("the recovered terminal object unnecessarily spent the repair turn")
			}
			env, err := first(ctx, r)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(r.OutDir, "reports", "AGENT.md"), []byte("# Vultr setup\n\nRead the project and help with Vultr setup.\n"), 0600); err != nil {
				t.Fatal(err)
			}
			env.Result = receipt.TextResult(string(message))
			env.Status, env.ExitCode = "error", engine.ExitOutputSchema
			refused = env
			return env, engine.Classify(engine.ExitOutputSchema, errors.New("invalid character 'A'"))
		}, Checker: checks.build})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || !res.Draft.Passed() || res.Draft.IsRepaired() || len(checks.asked) != 1 {
		t.Fatalf("runs %d, validation %s, check calls %d", len(runs), res.Draft.Validation, len(checks.asked))
	}
	if res.Model.Name != "vultr-setup-wizard" || res.Model.Namespace != "alice" || !strings.Contains(res.Model.Instructions, "# Vultr setup") {
		t.Fatalf("recovered model %+v", res.Model)
	}
	if !strings.Contains(res.Draft.Notes, "50 characters") || !strings.Contains(string(res.Draft.Manifest), "optionalSecrets") {
		t.Fatalf("recovery notes %q, manifest %s", res.Draft.Notes, res.Draft.Manifest)
	}
	var raw string
	if err := json.Unmarshal(refused.Result, &raw); err != nil || raw != string(message) || refused.ExitCode != engine.ExitOutputSchema {
		t.Fatalf("original refused answer changed: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(res.OutDir, "draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	var written map[string]any
	if err := json.Unmarshal(b, &written); err != nil || written["recovered"] != true {
		t.Fatalf("recovery metadata absent: %v", err)
	}
}
