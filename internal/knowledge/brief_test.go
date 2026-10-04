package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// stubWeb replaces the person's keys as LocalWeb reads them for one test.
func stubWeb(t *testing.T, w *engine.WebRun, state drafter.SearchState, err error) *int {
	t.Helper()
	calls := 0
	old := localWeb
	localWeb = func(*policy.Policy, string) (*engine.WebRun, drafter.SearchState, error) {
		calls++
		return w, state, err
	}
	t.Cleanup(func() { localWeb = old })
	return &calls
}

// A brief alone builds a pack on this computer (agentry-desktop#498, plan
// knowledge-and-drafter.md D15): the Creator gets the brief and no
// documents, the log carries the plan's warning, and Add documents takes a
// brief alone too. Without the web asked for, no key is read.
func TestABriefAloneBuildsAPack(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	calls := stubWeb(t, &engine.WebRun{}, drafter.SearchOn, nil)
	var seen build.Options
	var log strings.Builder
	res, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "espresso", Brief: "  Espresso machine care, for a café's new staff.  ", Build: fakeCreator(t, &seen, nil), Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Brief != "Espresso machine care, for a café's new staff." || len(seen.Documents) != 0 || seen.Web != nil {
		t.Fatalf("the Creator ran with brief %q, documents %v, web %v", seen.Brief, seen.Documents, seen.Web)
	}
	if *calls != 0 || res.Searched || res.WithoutWeb != "" {
		t.Fatalf("a build without the web asked for its keys (%d) or said %+v", *calls, res)
	}
	if !strings.Contains(log.String(), NoDocumentsWarning) {
		t.Fatalf("the log did not warn that documents make the best knowledge: %q", log.String())
	}
	noPlaintextLeft(t, e)

	log.Reset()
	if _, err := e.Add(ctx, BuildRequest{Registry: reg.URL, Name: "espresso", Brief: "Add descaling.", Build: fakeCreator(t, &seen, nil), Log: &log}); err != nil {
		t.Fatal(err)
	}
	if seen.Brief != "Add descaling." || len(seen.Documents) != 0 {
		t.Fatalf("Add ran with brief %q, documents %v", seen.Brief, seen.Documents)
	}
	noPlaintextLeft(t, e)
}

// A build given neither documents nor a brief, or a brief past 8 KiB, is
// refused before the Creator runs.
func TestABuildNeedsDocumentsOrABriefWithinItsBound(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	ran := false
	never := func(context.Context, dockerx.Client, build.Options) (*build.Result, error) {
		ran = true
		return nil, errors.New("ran")
	}
	if _, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "empty", Brief: "   ", Build: never}); err == nil || !strings.Contains(err.Error(), "write a brief") {
		t.Fatalf("no documents and a blank brief: %v", err)
	}
	if _, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "long", Brief: strings.Repeat("a", build.MaxBriefBytes+1), Build: never}); err == nil || !strings.Contains(err.Error(), "8 KiB") {
		t.Fatalf("a brief past 8 KiB: %v", err)
	}
	if ran {
		t.Fatal("the Creator ran on a refused build")
	}
}

// Search the web (D16) gives the Creator the person's web configuration
// when this computer has the keys for it; without them the build runs
// without the web and says why, in the log and in the result, rather than
// failing.
func TestSearchTheWebUsesThePersonsKeysOrSaysWhyNot(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))

	w := &engine.WebRun{JevKey: "not-a-real-jev-key", Search: "brave", SearchKey: "not-a-real-search-key"}
	stubWeb(t, w, drafter.SearchOn, nil)
	var seen build.Options
	var log strings.Builder
	res, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "web-on", Brief: "Cold chain logistics.", Web: true, Build: fakeCreator(t, &seen, nil), Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Web != w || !res.Searched || res.WithoutWeb != "" {
		t.Fatalf("web = %v, result = %+v", seen.Web, res)
	}
	if !strings.Contains(log.String(), "AI Review reads every result first") || !strings.Contains(log.String(), "can raise what a build costs") {
		t.Fatalf("the log did not say how the web is searched: %q", log.String())
	}
	if strings.Contains(log.String(), "not-a-real") {
		t.Fatal("a key reached the log")
	}

	for _, c := range []struct {
		name  string
		state drafter.SearchState
		err   error
		want  string
	}{
		{"no-jev", "", drafter.ErrNoJevKey, "AI Review key"},
		{"no-provider", drafter.SearchNoProvider, nil, "no search provider"},
		{"no-key", drafter.SearchNoKey, nil, "key was found"},
	} {
		stubWeb(t, &engine.WebRun{JevKey: "not-a-real-jev-key"}, c.state, c.err)
		log.Reset()
		res, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: c.name, Brief: "Cold chain logistics.", Web: true, Build: fakeCreator(t, &seen, nil), Log: &log})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if seen.Web != nil || res.Searched || !strings.Contains(res.WithoutWeb, c.want) || !strings.Contains(log.String(), "builds without the web: ") {
			t.Fatalf("%s: web = %v, result = %+v, log %q", c.name, seen.Web, res, log.String())
		}
	}

	stubWeb(t, nil, "", errors.New("the keychain is locked"))
	if _, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "fault", Brief: "x", Web: true, Build: fakeCreator(t, &seen, nil)}); err == nil || !strings.Contains(err.Error(), "keychain") {
		t.Fatalf("a keychain fault: %v", err)
	}
}

// Under a sandboxed runtime with no name servers for the tools, the web is
// off before any key is read.
func TestASandboxedRuntimeWithoutResolversKeepsTheBuildOffTheWeb(t *testing.T) {
	calls := stubWeb(t, &engine.WebRun{}, drafter.SearchOn, nil)
	pol := policy.Default()
	pol.Docker.Runtime = "runsc"
	w, why, err := creatorWeb(pol, true)
	if err != nil || w != nil || !strings.Contains(why, "docker.resolvers") || *calls != 0 {
		t.Fatalf("web %v, why %q, err %v, keys read %d times", w, why, err, *calls)
	}
	pol.Docker.Resolvers = []string{"10.0.0.53"}
	if w, why, err := creatorWeb(pol, true); err != nil || w == nil || why != "" {
		t.Fatalf("with resolvers: web %v, why %q, err %v", w, why, err)
	}
}
