package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/sources"
)

// The Documents step reads a build's repositories and addresses with the
// CLI's own parse: what it accepts the build fetches, what it refuses the
// build would refuse, and a host with a key saved on this computer is
// named so (the key itself never leaves the keychain).
func TestCheckSourceListReadsAsTheBuildWould(t *testing.T) {
	keyring.MockInit()
	t.Setenv("AGENTRY_NO_KEYRING", "")
	if err := keyring.Set(modelkey.Service, sources.KeyAccount("github.com"), "not-a-real-token"); err != nil {
		t.Fatal(err)
	}
	c := CheckSourceList(Sources{
		Repos: []string{" https://github.com/google/uuid@v1.6.0 ", "", "http://github.com/o/r", "https://me:pw@github.com/o/r", "https://10.1.2.3/o/r"},
		URLs:  []string{"https://arxiv.org/pdf/1706.03762", "ftp://example.com/a.pdf"},
	})
	if c.Problem != "" || c.MaxRepos != build.MaxRepos || c.MaxURLs != build.MaxURLs {
		t.Fatalf("check: %+v", c)
	}
	want := []struct {
		addr, kind, host string
		saved, bad       bool
	}{
		{"https://github.com/google/uuid@v1.6.0", "repository", "github.com", true, false},
		{"http://github.com/o/r", "repository", "", false, true},
		{"https://me:pw@github.com/o/r", "repository", "", false, true},
		{"https://10.1.2.3/o/r", "repository", "", false, true},
		{"https://arxiv.org/pdf/1706.03762", "url", "arxiv.org", false, false},
		{"ftp://example.com/a.pdf", "url", "", false, true},
	}
	if len(c.Lines) != len(want) {
		t.Fatalf("lines: %+v", c.Lines)
	}
	for i, w := range want {
		l := c.Lines[i]
		if l.Address != w.addr || l.Kind != w.kind || l.Host != w.host || l.Saved != w.saved || (l.Problem != "") != w.bad {
			t.Errorf("line %d: %+v, want %+v", i, l, w)
		}
		if strings.Contains(fmt.Sprintf("%+v", l), "not-a-real-token") {
			t.Fatal("a saved key reached the step's view")
		}
	}
	if !strings.Contains(c.Lines[3].Problem, "not a public address") {
		t.Errorf("a private literal: %q", c.Lines[3].Problem)
	}
	if first := c.First(); !strings.HasPrefix(first, "http://github.com/o/r: ") {
		t.Errorf("first problem: %q", first)
	}

	many := make([]string, build.MaxRepos+1)
	for i := range many {
		many[i] = fmt.Sprintf("https://github.com/o/r%d", i)
	}
	if c := CheckSourceList(Sources{Repos: many}); !strings.Contains(c.Problem, "at most 10 repositories") {
		t.Fatalf("over the count: %q", c.Problem)
	}
}

// A repository or an address counts as a document: a build given one and
// nothing else runs, and the Creator is handed the parsed sources, each
// repository's token the key saved on this computer for its host. The log
// says what is fetched and through what, and never the key.
func TestABuildFetchesItsSourcesWithTheSavedKey(t *testing.T) {
	keyring.MockInit()
	t.Setenv("AGENTRY_NO_KEYRING", "")
	if err := keyring.Set(modelkey.Service, sources.KeyAccount("github.com"), "not-a-real-token"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	var seen build.Options
	var log strings.Builder
	res, err := e.Create(ctx, BuildRequest{
		Registry: reg.URL, Name: "uuid",
		Sources: Sources{Repos: []string{"https://github.com/google/uuid@v1.6.0"}, URLs: []string{"https://arxiv.org/pdf/1706.03762"}},
		Build:   fakeCreator(t, &seen, nil), Log: &log,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen.Sources) != 2 || seen.Sources[0].ID != "repo-github.com-google-uuid" || seen.Sources[0].Ref != "v1.6.0" ||
		seen.Sources[0].Token != "not-a-real-token" || seen.Sources[0].TokenFrom != sources.TokenFromKeychain ||
		seen.Sources[1].Host != "arxiv.org" || seen.Sources[1].Token != "" {
		t.Fatalf("the Creator was handed sources %+v", seen.Sources)
	}
	if strings.Contains(log.String(), NoDocumentsWarning) {
		t.Fatalf("a build from sources was warned it has no documents: %q", log.String())
	}
	if !strings.Contains(log.String(), "with the key saved on this computer for github.com") || !strings.Contains(log.String(), "through a proxy to arxiv.org alone") {
		t.Fatalf("the log did not say what it fetches: %q", log.String())
	}
	if strings.Contains(log.String(), "not-a-real-token") || strings.Contains(fmt.Sprintf("%+v", res), "not-a-real-token") {
		t.Fatal("a saved key reached the log or the result")
	}
	noPlaintextLeft(t, e)
}

// A source the build would refuse is refused before the Creator runs.
func TestABuildRefusesASourceBeforeItRuns(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	ran := false
	never := func(context.Context, dockerx.Client, build.Options) (*build.Result, error) {
		ran = true
		return nil, errors.New("ran")
	}
	for _, s := range []Sources{{Repos: []string{"http://github.com/o/r"}}, {URLs: []string{"https://192.168.1.10/a.pdf"}}} {
		if _, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "refused", Sources: s, Build: never}); err == nil || !strings.Contains(err.Error(), "the build's sources") {
			t.Fatalf("%+v: %v", s, err)
		}
	}
	if err := CheckSources(nil, Sources{Repos: []string{"  "}}, ""); err == nil || !strings.Contains(err.Error(), "give a repository or a web address") {
		t.Fatalf("only blank sources: %v", err)
	}
	if ran {
		t.Fatal("the Creator ran on a refused build")
	}
}
