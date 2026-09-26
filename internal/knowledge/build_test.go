package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
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
		!strings.Contains(res.Key, "keeps for your account") || res.Stopped != "" {
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
