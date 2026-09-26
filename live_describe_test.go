package main

// Describe it, on this computer, live (#282): the bound methods driven as
// Settings and the Build page drive them, against this machine's OS
// keychain, TypeSafe, docker and model, with the native dialog answered
// by the test. Skipped unless both variables below are set:
//
//	AGENTRY_LIVE_DESCRIBE_JEV=<a TypeSafe Jev key> \
//	AGENTRY_LIVE_DESCRIBE_REGISTRY=https://dev.secureagentry.net \
//	go test -run TestLiveDescribe -v -timeout 30m .
//
// It uses the runner home and policy AGENTRY_RUNNER_HOME names (docker must
// see it) and the model key the policy's apiKeyEnv names in the
// environment. It saves a wrong key and tests it (TypeSafe's refusal), then
// saves the real key through StoreJevKey into the keychain entry
// provider:typesafe, tests it, and drafts with it, so the key reaches the
// run from the keychain and not from a variable. It will not run when the
// keychain already holds a key under that entry, so it never replaces one
// of the person's, and it forgets the key it saved at the end.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

func TestLiveDescribe(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("AGENTRY_LIVE_DESCRIBE_JEV"))
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_DESCRIBE_REGISTRY"), "/")
	if key == "" || reg == "" {
		t.Skip("set AGENTRY_LIVE_DESCRIBE_JEV and AGENTRY_LIVE_DESCRIBE_REGISTRY to run the live Describe it check")
	}
	if present, err := modelkey.Present(policy.JudgeKeyProvider); err != nil || present {
		t.Skipf("the keychain already holds %s (or cannot be asked: %v); the live check will not replace it", modelkey.Account(policy.JudgeKeyProvider), err)
	}
	// The keychain, not a variable, is what the draft must read.
	t.Setenv(policy.DefaultJudgeKeyEnv, "")
	t.Setenv(modelkey.EnvNoKeyring, "")

	var events []string
	emit := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		line := fmt.Sprint(data...)
		events = append(events, name+" "+line)
		if name == eventDraftLog {
			t.Log(line)
		}
	}
	t.Cleanup(func() { emitEvent = emit })
	var asked []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, _ string) (bool, error) {
		asked = append(asked, title)
		return true, nil
	}}
	r := &RunAPI{app: app}
	b := &BuildAPI{app: app, run: r}
	t.Cleanup(func() { _, _ = r.ForgetJevKey() })

	// A key TypeSafe does not know: saved, then refused in plain words.
	if _, err := r.StoreJevKey("jev-not-a-key-" + filepath.Base(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	tv, err := r.TestJevKey()
	if err != nil || tv.OK || !strings.Contains(tv.Message, "TypeSafe refused the key") || tv.From != "the keychain" {
		t.Fatalf("a wrong key: %+v %v", tv, err)
	}
	t.Logf("a wrong key: %s", tv.Message)

	// The real key, through the one call that carries it.
	v, err := r.StoreJevKey(key)
	if err != nil || !v.Stored || !v.Ready || v.Source != modelkey.SourceKeyring || v.EnvSet {
		t.Fatalf("store: %+v %v", v, err)
	}
	tv, err = r.TestJevKey()
	if err != nil || !tv.OK || tv.From != "the keychain" {
		t.Fatalf("the real key: %+v %v", tv, err)
	}
	t.Logf("the real key: %s", tv.Message)
	st, err := b.DescribeStatus()
	if err != nil || !st.Key.Ready || st.Key.EnvSet {
		t.Fatalf("status: %+v %v", st, err)
	}
	t.Logf("status: model %s; %s", st.ModelName, st.SearchSentence)

	dv, err := b.DescribeOnThisComputer(reg,
		"Check a CSV file I add for data-quality problems: missing values, duplicate rows, and columns whose values are not all the same format. Report the problems per column.",
		[]string{"It runs on its own, from the one file I add"})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if len(asked) != 1 || asked[0] != "Draft on this computer?" {
		t.Fatalf("dialogs %v", asked)
	}
	if dv.Model == nil || dv.Model.Name == "" || strings.TrimSpace(dv.Model.Instructions) == "" || dv.AIReview.Read == 0 {
		t.Fatalf("draft view: model %+v aiReview %+v", dv.Model, dv.AIReview)
	}
	t.Logf("draft: %s/%s — %s; AI Review read %d, held back %d (%d not scanned); decode notes %q; manifest error %q; %d matches, %d sources, %d open questions; in %s",
		dv.Model.Namespace, dv.Model.Name, dv.Model.Summary, dv.AIReview.Read, dv.AIReview.Withheld+dv.AIReview.NotScanned, dv.AIReview.NotScanned,
		dv.DecodeNotes, dv.ManifestError, len(dv.Matches), len(dv.Sources), len(dv.OpenQuestions), dv.OutDir)
	if c := b.Check(*dv.Model); c.First != nil {
		t.Logf("the form's checks: first problem %s: %s", c.First.Section, c.First.Problem)
	} else {
		t.Log("the form's checks: every step passes")
	}

	// The key is in nothing the page was given, the progress lines, the
	// draft's record or anything else in the run's folder.
	view, _ := json.Marshal(dv)
	if strings.Contains(string(view), key) {
		t.Fatal("the draft view carries the key")
	}
	for _, e := range events {
		if strings.Contains(e, key) {
			t.Fatal("a progress line carries the key")
		}
	}
	err = filepath.WalkDir(dv.OutDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, rerr := os.ReadFile(p) //nolint:gosec // the run's own folder
		if rerr == nil && strings.Contains(string(body), key) {
			t.Errorf("%s carries the key", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dv.OutDir, "draft.json")); err != nil {
		t.Fatalf("draft.json: %v", err)
	}

	// Forgotten: the entry and the policy's source go.
	f, err := r.ForgetJevKey()
	if err != nil || f.Stored || f.Ready {
		t.Fatalf("forget: %+v %v", f, err)
	}
}
