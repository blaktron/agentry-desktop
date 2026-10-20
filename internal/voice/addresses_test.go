package voice

import (
	"context"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/voice/build"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
)

// The step reads the addresses with the CLI's own parse: an https page on a
// public host passes, and what the build would refuse is refused with its
// reason, the list held to its count.
func TestCheckAddressesReadsAsTheBuildWould(t *testing.T) {
	c := CheckAddresses([]string{" https://paulgraham.com/greatwork.html ", "", "http://example.com/post", "https://192.168.1.10/blog"})
	if c.Problem != "" || c.Max != build.MaxURLs || len(c.Lines) != 3 {
		t.Fatalf("check: %+v", c)
	}
	if l := c.Lines[0]; l.Problem != "" || l.Host != "paulgraham.com" || l.Shown != "https://paulgraham.com/greatwork.html" {
		t.Fatalf("a public page: %+v", l)
	}
	if c.Lines[1].Problem == "" || !strings.Contains(c.Lines[2].Problem, "not a public address") {
		t.Fatalf("refusals: %+v", c.Lines[1:])
	}
	if !strings.HasPrefix(c.First(), "http://example.com/post: ") {
		t.Fatalf("first: %q", c.First())
	}
	many := make([]string, build.MaxURLs+1)
	for i := range many {
		many[i] = "https://example.com/p" + strings.Repeat("x", i)
	}
	if c := CheckAddresses(many); !strings.Contains(c.Problem, "at most 20") {
		t.Fatalf("over the count: %q", c.Problem)
	}
}

// On this computer a voice from web addresses takes the person's own
// TypeSafe key: without it the build is refused, saying so, before anything
// runs; with it, the Voice Writer's build is handed the addresses, AI Review
// with that key, and web_read's scan with it for a page drawn by scripts.
func TestAVoiceFromAddressesTakesThePersonsTypeSafeKey(t *testing.T) {
	e, reg := testEnv(t, false)
	ctx := context.Background()
	home, _ := e.Home()
	t.Setenv(policy.HomeEnv, home)
	old := jevKey
	t.Cleanup(func() { jevKey = old })
	ran := false
	var seen build.Options
	fake := func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		ran, seen = true, o
		return &build.Result{Text: []byte("# Voice by the Writer\n\nKeep it plain.\n"), Pages: []*build.Page{{URL: o.URLs[0].Shown(), Sample: "greatwork.html"}}}, nil
	}
	urls := []string{"https://paulgraham.com/greatwork.html", "https://paulgraham.com/hwh.html"}

	jevKey = func(*policy.Policy) (string, error) { return "", drafter.ErrNoJevKey }
	if _, err := e.Build(ctx, BuildRequest{Registry: reg.URL, Job: build.Create, URLs: urls, Build: fake}); err == nil || err.Error() != NoReviewKey {
		t.Fatalf("addresses without the key: %v", err)
	}
	if ran {
		t.Fatal("the Voice Writer ran without the key AI Review needs")
	}

	jevKey = func(*policy.Policy) (string, error) { return "not-a-real-jev-key", nil }
	var log strings.Builder
	res, err := e.Build(ctx, BuildRequest{Registry: reg.URL, Job: build.Create, URLs: urls, Build: fake, Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen.URLs) != 2 || seen.URLs[1].Host != "paulgraham.com" || seen.Review == nil || seen.Web == nil ||
		seen.Web.Scan != toolsrv.WebScanJev || seen.Web.JevKey != "not-a-real-jev-key" {
		t.Fatalf("the Voice Writer was handed urls %v, review %v, web %+v", seen.URLs, seen.Review, seen.Web)
	}
	if len(res.Pages) != 1 || res.Pages[0].Sample != "greatwork.html" {
		t.Fatalf("pages: %+v", res.Pages)
	}
	if !strings.Contains(log.String(), "read by AI Review with your TypeSafe key") || strings.Contains(log.String(), "not-a-real") {
		t.Fatalf("log: %q", log.String())
	}

	if _, err := e.Build(ctx, BuildRequest{Registry: reg.URL, Job: build.Create, URLs: []string{"http://example.com/post"}, Build: fake}); err == nil || !strings.Contains(err.Error(), "the web addresses") {
		t.Fatalf("an address the build would refuse: %v", err)
	}
}
