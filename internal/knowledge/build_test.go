package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"

	"github.com/blaktron/agentry-desktop/internal/progress"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// fakeCreator stands in for the Creator run (pkg/knowledge/build is the
// CLI's, tested there): it writes a pack into the directory it is given,
// and a page into the run's reports, as the real run leaves every page it
// wrote in the clear there.
func fakeCreator(t *testing.T, seen *build.Options, stop error) Builder {
	return func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		*seen = o
		if _, err := os.Stat(filepath.Join(o.PackDir, kn.ManifestFile)); errors.Is(err, os.ErrNotExist) {
			if err := os.CopyFS(o.PackDir, os.DirFS(knowledgetest.Make(t, o.Name, nil).Dir)); err != nil {
				return nil, err
			}
		}
		if err := os.MkdirAll(filepath.Join(o.OutDir, "reports", "wiki"), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(o.OutDir, "reports", "wiki", "page.md"), []byte("a page in the clear"), 0o600); err != nil {
			return nil, err
		}
		p, err := kn.Load(o.PackDir)
		if err != nil {
			return nil, err
		}
		return &build.Result{
			Documents: []string{"guide.txt"}, Skipped: []string{"scan.bin"},
			Applied:  &kn.Result{Manifest: p.Manifest, Dropped: []kn.Drop{{Path: "wiki/bad.md", Reason: "no title"}}},
			Envelope: &receipt.Envelope{Usage: receipt.Usage{InputTokens: 1200, OutputTokens: 300}},
			RunErr:   stop,
		}, nil
	}
}

func documents(t *testing.T) []string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "guide.txt")
	if err := os.WriteFile(p, []byte("the guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{p}
}

// Create runs the Creator through the run seam against the policy's model,
// keeps the pack it made sealed, and leaves nothing in the clear: neither
// the pack's directory nor the run's reports, which held every page.
func TestCreateKeepsThePackSealedAndLeavesNoPageInTheClear(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	var seen build.Options
	var log strings.Builder
	res, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "handbook", Documents: documents(t), RunID: "run-1", Build: fakeCreator(t, &seen, nil), Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if seen.RunID != "run-1" || seen.Name != "handbook" || seen.Policy == nil || seen.PolicyFile == nil || seen.Budgets == nil || seen.Home != homeOf(t, e) {
		t.Fatalf("the Creator ran with %+v", seen)
	}
	if !strings.Contains(log.String(), "nothing it reads or writes is scanned") {
		t.Fatalf("the log did not say the build is not scanned: %q", log.String())
	}
	if res.Name != "handbook" || res.Pages != 1 || len(res.Skipped) != 1 || len(res.Dropped) != 1 || res.InputTokens != 1200 ||
		!strings.Contains(res.Key, "keeps for your account") || res.Stopped != "" || res.Brief {
		t.Fatalf("result = %+v", res)
	}
	l, err := e.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Packs) != 1 || l.Packs[0].ScanState != ScanNotScanned {
		t.Fatalf("kept = %+v", l.Packs)
	}
	noPlaintextLeft(t, e)

	if _, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "handbook", Documents: documents(t), Build: fakeCreator(t, &seen, nil)}); err == nil || !strings.Contains(err.Error(), "already kept") {
		t.Fatalf("a second pack of the name: %v", err)
	}
	if _, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "other", Build: fakeCreator(t, &seen, nil)}); err == nil {
		t.Fatal("a build with no documents passed")
	}
	fail := func(context.Context, dockerx.Client, build.Options) (*build.Result, error) {
		return nil, errors.New("docker is not running")
	}
	if _, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "other", Documents: documents(t), Build: fail}); err == nil {
		t.Fatal("a failed build kept something")
	}
	if _, err := e.Find(reg.URL, "other"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a failed build left a pack: %v", err)
	}
	noPlaintextLeft(t, e)
}

// Add opens the kept pack for the Creator, reseals it under the key it
// had, and says a stopped run's reason and that Agentry keeps the copy
// from before.
func TestAddResealsUnderTheSameKey(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	before := keep(t, e, reg.URL, "handbook")
	if _, err := e.Upload(ctx, reg.URL, "handbook"); err != nil {
		t.Fatal(err)
	}
	var seen build.Options
	res, err := e.Add(ctx, BuildRequest{Registry: reg.URL, Name: "handbook", Documents: documents(t), Build: fakeCreator(t, &seen, errors.New("the run's budget ran out"))})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Name != "" {
		t.Fatalf("add named a new pack %q", seen.Name)
	}
	if res.Stopped == "" || !res.AgentryStale {
		t.Fatalf("result = %+v", res)
	}
	after, err := e.Find(reg.URL, "handbook")
	if err != nil {
		t.Fatal(err)
	}
	if after.ID() != before.ID() || string(after.Key.WrappedKey.Key) != string(before.Key.WrappedKey.Key) {
		t.Fatal("add changed the pack's id or its wrapped key")
	}
	if _, err := e.Add(ctx, BuildRequest{Registry: reg.URL, Name: "missing", Documents: documents(t), Build: fakeCreator(t, &seen, nil)}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("add to a pack not kept: %v", err)
	}
	noPlaintextLeft(t, e)
}

// A build on this computer is given at least 60 minutes, whatever the runner
// policy's run budget (15 minutes by default); a policy that allows more keeps
// its own, and the turns and cost stay the policy's. The floor is the CLI's,
// the one `agentry knowledge create` builds under. No budget of a build counts
// tokens.
func TestCreatorBudgetsGiveABuildAtLeastSixtyMinutes(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	builtinNoLimit(t, false)
	pol := &policy.Policy{Budgets: policy.Budgets{TimeoutSeconds: 900, MaxTurns: 60, MaxCostUSD: 2}}
	if b, want := CreatorBudgets(pol), (spec.Budgets{TimeoutSeconds: 3600, MaxTurns: 60, MaxCostUSD: 2}); b.Run != want || b.Shown != want {
		t.Fatalf("default policy: %+v, want %+v", b, want)
	}
	pol.Budgets.TimeoutSeconds = 1800
	if b := CreatorBudgets(pol); b.Run.TimeoutSeconds != 3600 {
		t.Fatalf("a policy at the old 30 minutes: %+v", b)
	}
	pol.Budgets.TimeoutSeconds, pol.Budgets.MaxTurns = 7200, 0
	if b, want := CreatorBudgets(pol), (spec.Budgets{TimeoutSeconds: 7200, MaxTurns: 0, MaxCostUSD: 2}); b.Run != want {
		t.Fatalf("a policy of two hours: %+v, want %+v", b, want)
	}
	if b := CreatorBudgets(nil); b.Run.TimeoutSeconds != 3600 {
		t.Fatalf("no policy: %+v", b)
	}
}

// A build reports its progress: the budgets at its start, the harness's
// usage as it arrives, and the pages the Creator has written, counted in the
// run's reports as they appear; and it runs with the 60-minute
// floor.
func TestABuildReportsItsProgress(t *testing.T) {
	defer func(d time.Duration) { progressEvery = d }(progressEvery)
	progressEvery = 10 * time.Millisecond
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	var seen build.Options
	inner := fakeCreator(t, &seen, nil)
	writing := func(ctx context.Context, d dockerx.Client, o build.Options) (*build.Result, error) {
		wiki := filepath.Join(o.OutDir, "reports", "wiki", "topics")
		if err := os.MkdirAll(wiki, 0o700); err != nil {
			return nil, err
		}
		// A link, to a page or to a folder of pages, is never followed.
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "elsewhere.md"), []byte("x"), 0o600); err != nil {
			return nil, err
		}
		if err := os.Symlink(filepath.Join(outside, "elsewhere.md"), filepath.Join(wiki, "linked.md")); err != nil {
			return nil, err
		}
		if err := os.Symlink(outside, filepath.Join(wiki, "linked-dir")); err != nil {
			return nil, err
		}
		// An index and the log are the application's, never counted.
		for _, f := range []string{"index.md", "log.md"} {
			if err := os.WriteFile(filepath.Join(o.OutDir, "reports", "wiki", f), []byte("x"), 0o600); err != nil {
				return nil, err
			}
		}
		for i, name := range []string{"a.md", "b.md"} {
			if err := os.WriteFile(filepath.Join(wiki, name), []byte("---\ntitle: x\n---\n"), 0o600); err != nil {
				return nil, err
			}
			o.OnUsage(receipt.Usage{Turns: i + 1, InputTokens: int64(1000 * (i + 1)), OutputTokens: 100})
			time.Sleep(30 * time.Millisecond)
		}
		return inner(ctx, d, o)
	}
	var mu sync.Mutex
	var got []Progress
	var log strings.Builder
	_, err := e.Create(ctx, BuildRequest{
		Registry: reg.URL, Name: "handbook", Documents: documents(t), Build: writing, Log: &log,
		Progress: func(p Progress) { mu.Lock(); got = append(got, p); mu.Unlock() },
	})
	if err != nil {
		t.Fatal(err)
	}
	// By default a build on this computer has no timeout and runs to the
	// hidden failsafe.
	if seen.Budgets == nil || seen.Budgets.TimeoutSeconds != 0 || seen.Budgets.MaxTurns != settings.BuiltinFailsafeTurns {
		t.Fatalf("the Creator ran with budgets %+v", seen.Budgets)
	}
	// A build on this computer takes up to 1 GB.
	if seen.MaxBytes != kn.MaxLocalBytes {
		t.Fatalf("the Creator ran with a cap of %d bytes", seen.MaxBytes)
	}
	if !strings.Contains(log.String(), "A build takes a while") {
		t.Fatalf("the log did not say a build takes a while: %q", log.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) < 3 {
		t.Fatalf("progress = %+v", got)
	}
	first, last := got[0], got[len(got)-1]
	if first.StartedAt == 0 || first.TimeoutSeconds != 0 || first.MaxTurns != 0 || first.Turns != 0 || first.Pages != 0 {
		t.Fatalf("first progress = %+v", first)
	}
	// The fake leaves one more page in the reports after the two above.
	if last.Turns != 2 || last.InputTokens != 2000 || last.Pages != 3 || last.StartedAt != first.StartedAt {
		t.Fatalf("last progress = %+v", last)
	}
}

// A build stopped from the card (its context cancelled) keeps the pages the
// run wrote: the pack is sealed under a context that outlives the
// cancellation, so the registry's wrap still answers and the pages are not
// lost; a pack being extended is resealed the same way.
func TestAStoppedBuildIsStillKeptSealed(t *testing.T) {
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	var seen build.Options
	for _, add := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		inner := fakeCreator(t, &seen, context.Canceled)
		stopped := func(c context.Context, d dockerx.Client, o build.Options) (*build.Result, error) {
			cancel() // StopBuild, while the Creator runs
			return inner(c, d, o)
		}
		req := BuildRequest{Registry: reg.URL, Name: "handbook", Documents: documents(t), Build: stopped}
		var res *BuildResult
		var err error
		if add {
			res, err = e.Add(ctx, req)
		} else {
			res, err = e.Create(ctx, req)
		}
		if err != nil {
			t.Fatalf("add=%v: the stopped build was not kept: %v", add, err)
		}
		if res.Stopped == "" || res.Pages == 0 {
			t.Fatalf("add=%v: result = %+v", add, res)
		}
	}
}

// Past Agentry's 10 MB of documents, a build on this computer says first
// that it can take a very long time and that the pack stays
// here; under it, nothing is said.
func TestABuildPastAgentrysCapSaysSoFirst(t *testing.T) {
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	big := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", kn.MaxBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	var seen build.Options
	var log strings.Builder
	if _, err := e.Create(context.Background(), BuildRequest{Registry: reg.URL, Name: "big", Documents: []string{big}, Build: fakeCreator(t, &seen, nil), Log: &log}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "can take a very long time") || !strings.Contains(log.String(), "stay on this computer") {
		t.Fatalf("log = %q", log.String())
	}
	log.Reset()
	if _, err := e.Create(context.Background(), BuildRequest{Registry: reg.URL, Name: "small", Documents: documents(t), Build: fakeCreator(t, &seen, nil), Log: &log}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(log.String(), "can take a very long time") {
		t.Fatalf("a small build warned: %q", log.String())
	}
}

// A build's progress event is one flat object: the shared report's fields
// and the pages, in this order, which is what the page reads
// (knowledge:build-progress; the shared fields live in internal/progress).
func TestABuildsProgressIsOneFlatObjectWithItsPages(t *testing.T) {
	p := Progress{Report: progress.Report{StartedAt: 1, TimeoutSeconds: 2, MaxTurns: 3, Turns: 4, InputTokens: 5, OutputTokens: 6}, Pages: 7}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"startedAt":1,"timeoutSeconds":2,"maxTurns":3,"turns":4,"inputTokens":5,"outputTokens":6,"pages":7}`
	if string(b) != want {
		t.Fatalf("%s, want %s", b, want)
	}
}

// Run defaults' no-limit switch for the built-in agents (on by default): a
// build on this computer runs with no timeout and to the hidden failsafe of 200
// turns, for a local model that cannot draft a pack in the Creator's 60
// minutes; the cost cap stays the policy's, and the build's log and its
// building view say no limit rather than "up to 0 minutes" or the failsafe.
func TestByDefaultABuildHasNoTimeOrTurnLimitButTheHiddenFailsafe(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	pol := &policy.Policy{Budgets: policy.Budgets{TimeoutSeconds: 900, MaxTurns: 60, MaxCostUSD: 2}}
	if b, want := CreatorBudgets(pol), (spec.Budgets{TimeoutSeconds: 0, MaxTurns: settings.BuiltinFailsafeTurns, MaxCostUSD: 2}); b.Run != want || b.Shown != (spec.Budgets{MaxCostUSD: 2}) {
		t.Fatalf("by default: %+v, want %+v run and no limit shown", b, want)
	}

	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	var seen build.Options
	var log strings.Builder
	var reports []Progress
	if _, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "handbook", Documents: documents(t), RunID: "run-606",
		Build: fakeCreator(t, &seen, nil), Log: &log, Progress: func(p Progress) { reports = append(reports, p) }}); err != nil {
		t.Fatal(err)
	}
	if seen.Budgets == nil || seen.Budgets.TimeoutSeconds != 0 || seen.Budgets.MaxTurns != settings.BuiltinFailsafeTurns {
		t.Fatalf("the Creator ran under %+v, want no timeout and the failsafe's turns", seen.Budgets)
	}
	if l := log.String(); !strings.Contains(l, "no time or turn limit") || strings.Contains(l, "up to 0 minutes") || strings.Contains(l, "200") {
		t.Fatalf("the log did not say the build has no time or turn limit: %q", l)
	}
	if len(reports) == 0 || reports[0].TimeoutSeconds != 0 || reports[0].MaxTurns != 0 {
		t.Fatalf("the building view was told %+v, want no time or turn limit", reports)
	}
}

// builtinNoLimit turns Run defaults' no-limit switch for the built-in agents
// on or off in the isolated desktop.yaml.
func builtinNoLimit(t *testing.T, on bool) {
	t.Helper()
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.BuiltinNoLimit = &on
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
}
