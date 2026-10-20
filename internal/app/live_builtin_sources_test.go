package app

// A built-in's repositories and web addresses, live: the bound methods
// driven as the Knowledge and Voice dialogs drive them, on this computer
// and In Agentry. Each is skipped unless AGENTRY_LIVE_ADDRESSES=1 and its
// variables are set, because each spends a model's tokens or the account's
// credits.
//
// On this computer, against the runner home and policy AGENTRY_RUNNER_HOME
// names (docker must see it), the model key the policy names and, for the
// voice, the person's own TypeSafe key ($TYPESAFE_API_KEY):
//
//	AGENTRY_LIVE_ADDRESSES=1 \
//	AGENTRY_LIVE_ADDRESSES_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_ADDRESSES_TOKEN=<a PAT with knowledge:read, knowledge:write, voice:read, voice:write> \
//	go test -run 'TestLive(Knowledge|Voice)From' -v -timeout 70m ./internal/app
//
// In Agentry, with a PAT that also carries run:read, run:write and
// user:profile:
//
//	AGENTRY_LIVE_ADDRESSES=1 AGENTRY_LIVE_ADDRESSES_REGISTRY=… AGENTRY_LIVE_ADDRESSES_TOKEN=… \
//	go test -run 'TestLiveHosted(Knowledge|Voice)From' -v -timeout 40m ./internal/app
//
// A pack a test makes is deleted, here and in Agentry. A voice made on this
// computer stays in the test's own runner home; one made In Agentry is the
// account's newest version, waiting for AI Review.

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
)

// The repository and the two pages the checks read: a small public
// repository at a tag, and two essays on a plain HTML site.
const liveRepo = "https://github.com/google/uuid@v1.6.0"

var liveVoiceURLs = []string{"https://paulgraham.com/greatwork.html", "https://paulgraham.com/hwh.html"}

func liveSources(t *testing.T) (reg, token string) {
	t.Helper()
	reg = strings.TrimRight(os.Getenv("AGENTRY_LIVE_ADDRESSES_REGISTRY"), "/")
	token = os.Getenv("AGENTRY_LIVE_ADDRESSES_TOKEN")
	if os.Getenv("AGENTRY_LIVE_ADDRESSES") != "1" || reg == "" || token == "" {
		t.Skip("set AGENTRY_LIVE_ADDRESSES=1, AGENTRY_LIVE_ADDRESSES_REGISTRY and _TOKEN to run the live checks of repositories and addresses")
	}
	return reg, token
}

func TestLiveKnowledgeFromARepository(t *testing.T) {
	reg, token := liveSources(t)
	name := "live-repo-" + time.Now().UTC().Format("20060102-150405")
	var mu sync.Mutex
	var log []string
	emit := emitEvent
	emitEvent = func(_ *App, ev string, data ...any) {
		if ev == eventKnowledgeLog && len(data) > 0 {
			if s, ok := data[0].(string); ok {
				mu.Lock()
				log = append(log, s)
				mu.Unlock()
			}
		}
	}
	t.Cleanup(func() { emitEvent = emit })
	env := knowledge.Env{
		Home: policy.Home, Keys: store.OSKeychain{},
		TokenFor: func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] },
		HTTP:     registry.HTTPClient,
	}
	app := &App{ctx: context.Background()}
	k := &KnowledgeAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env}}

	check := k.CheckSources(knowledge.Sources{Repos: []string{liveRepo}})
	if p := check.First(); p != "" || check.Lines[0].Host != "github.com" {
		t.Fatalf("the step's check: %+v", check)
	}
	res, err := k.Create(reg, name, nil, knowledge.Sources{Repos: []string{liveRepo}}, "", false)
	mu.Lock()
	t.Logf("build log:\n%s", strings.Join(log, "\n"))
	mu.Unlock()
	if err != nil {
		t.Fatalf("Create knowledge from a repository: %v", err)
	}
	t.Cleanup(func() { _ = env.Delete(context.Background(), reg, name, true) })
	t.Logf("made %s: %s, %d bytes; documents %d; fetched %+v; stopped %q; the Creator used %d in, %d out tokens",
		res.Name, kn.CountPages(res.Pages), res.Bytes, len(res.Documents), res.Fetched, res.Stopped, res.InputTokens, res.OutputTokens)
	if res.Pages == 0 || len(res.Fetched) != 1 || res.Fetched[0].Commit == "" || res.Fetched[0].Files == 0 {
		t.Fatalf("the pack: %+v", res)
	}
}

func TestLiveVoiceFromAddresses(t *testing.T) {
	reg, token := liveSources(t)
	home := t.TempDir()
	env := knowledge.Env{
		Home: func() (string, error) { return home, nil }, Keys: &knowledgeKeys{m: map[string]string{}},
		TokenFor: func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] },
		HTTP:     registry.HTTPClient,
	}
	var mu sync.Mutex
	var log []string
	emit := emitEvent
	emitEvent = func(_ *App, ev string, data ...any) {
		if ev == eventVoiceLog && len(data) > 0 {
			if s, ok := data[0].(string); ok {
				mu.Lock()
				log = append(log, s)
				mu.Unlock()
			}
		}
	}
	t.Cleanup(func() { emitEvent = emit })
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	v := &VoiceAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env, runnerHomeDir: home}}

	check, err := v.CheckAddresses(liveVoiceURLs)
	if err != nil || check.Check.First() != "" {
		t.Fatalf("the dialog's check: %+v, %v", check, err)
	}
	t.Logf("AI Review here: ready %v %s", check.ReviewReady, check.ReviewWhy)
	res, err := v.Write(reg, "create", nil, liveVoiceURLs, "")
	mu.Lock()
	t.Logf("Voice Writer log:\n%s", strings.Join(log, "\n"))
	mu.Unlock()
	if !check.ReviewReady {
		if err == nil || !strings.Contains(err.Error(), "TypeSafe key") {
			t.Fatalf("without the TypeSafe key the run was not refused for it: %+v, %v", res, err)
		}
		t.Skip("no TypeSafe key here: the refusal is checked, the run is not")
	}
	if err != nil {
		t.Fatalf("a voice from web addresses: %v", err)
	}
	t.Logf("kept %d bytes (%s), samples %v, pages %+v", res.Voice.Bytes, res.Voice.Digest, res.Samples, res.Pages)
	if res.Voice == nil || res.Voice.Bytes == 0 || len(res.Pages) != 2 {
		t.Fatalf("the voice: %+v", res)
	}
}

func TestLiveHostedKnowledgeFromARepository(t *testing.T) {
	reg, token := liveSources(t)
	name := "live-hosted-repo-" + time.Now().UTC().Format("20060102-150405")
	var mu sync.Mutex
	var views []*KnowledgeHostedBuildView
	emit := emitEvent
	emitEvent = func(_ *App, ev string, data ...any) {
		if ev != eventKnowledgeHosted || len(data) != 1 {
			return
		}
		if v, ok := data[0].(*KnowledgeHostedBuildView); ok {
			mu.Lock()
			views = append(views, v)
			mu.Unlock()
			t.Logf("build: %s %s %s", v.Status, v.State, v.StatusLine)
		}
	}
	t.Cleanup(func() { emitEvent = emit })
	home := t.TempDir()
	env := knowledge.Env{
		Home:     func() (string, error) { return home, nil },
		Keys:     &knowledgeKeys{m: map[string]string{}},
		TokenFor: func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] },
		HTTP:     registry.HTTPClient,
	}
	app := &App{ctx: context.Background()}
	k := &KnowledgeAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env}}

	avail, err := k.HostedAvailability(reg)
	if err != nil || !avail.Available || !avail.TakesSources {
		t.Fatalf("a build from a repository In Agentry is not offered here: %+v, %v", avail, err)
	}
	t.Logf("availability: holds %d; repositories up to %d, addresses up to %d", avail.HoldCredits, avail.MaxRepos, avail.MaxURLs)
	start, err := k.StartHostedBuild(reg, name, "", nil, knowledge.Sources{Repos: []string{liveRepo}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if start.Build == nil {
		t.Fatalf("the registry refused the build: %s %s", start.Refusal, start.Detail)
	}
	t.Logf("build %s created for %s, holding %d", start.Build.ID, start.Build.Name, start.Build.HoldCredits)
	t.Cleanup(func() {
		if err := env.Delete(context.Background(), reg, name, true); err != nil && !strings.Contains(err.Error(), "not") {
			t.Logf("cleanup: %v", err)
		}
	})
	final := waitHostedBuild(t, &mu, &views, 25*time.Minute)
	t.Logf("final: %s (%s); applied %v, %d pages, %d bytes; %d held, %d settled; repos %v; downloaded %v, scan %q",
		final.State, final.OutcomeLine, final.Applied, final.Pages, final.Bytes, final.HoldCredits, final.SettledCredits, final.Repos, final.Downloaded, final.ScanState)
	if final.Status != "finished" || !final.Applied || !final.Downloaded || len(final.Repos) != 1 || final.Repos[0] != liveRepo {
		t.Fatalf("the build did not land a pack from the repository here: %+v", final)
	}
}

func TestLiveHostedVoiceFromAddresses(t *testing.T) {
	reg, token := liveSources(t)
	finals := make(chan *VoiceHostedView, 4)
	emit := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if name == eventVoiceHosted && len(data) > 0 {
			if v, ok := data[0].(*VoiceHostedView); ok {
				t.Logf("voice: %s %s %s", v.Status, v.State, v.StatusLine)
				if v.Final {
					finals <- v
				}
			}
		}
	}
	t.Cleanup(func() { emitEvent = emit })
	home := t.TempDir()
	env := knowledge.Env{
		Home: func() (string, error) { return home, nil }, Keys: &knowledgeKeys{m: map[string]string{}},
		TokenFor: func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] },
		HTTP:     registry.HTTPClient,
	}
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	v := &VoiceAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env, runnerHomeDir: home}}

	view, err := v.View(reg)
	if err != nil || view.Account == nil {
		t.Fatalf("the account's voice: %+v, %v", view, err)
	}
	job := accountJob(view.Account)
	avail, err := v.HostedAvailability(reg, job)
	if err != nil || !avail.Available || !avail.TakesURLs {
		t.Fatalf("the Voice Writer In Agentry does not read addresses here: %+v, %v", avail, err)
	}
	note := ""
	if job == "edit" {
		note = "Draw on these two essays: plain words, short sentences."
	}
	start, err := v.HostedWithAI(reg, note, nil, liveVoiceURLs)
	if err != nil || start.Refusal != "" || start.Run == nil {
		t.Fatalf("start: %+v, %v", start, err)
	}
	var final *VoiceHostedView
	select {
	case final = <-finals:
	case <-time.After(25 * time.Minute):
		t.Fatal("the Voice Writer In Agentry did not finish")
	}
	t.Logf("%s %s: applied %v, %d bytes, settled %d of %d; urls %v; %s", job, final.ID, final.Applied, final.Bytes, final.SettledCredits, final.HoldCredits, final.URLs, final.PagesLine)
	if !final.Applied || !final.Settled || len(final.URLs) != 2 || final.PagesLine == "" {
		t.Fatalf("the Voice Writer In Agentry: %+v", final)
	}
}

// waitHostedBuild waits for a build In Agentry's last word.
func waitHostedBuild(t *testing.T, mu *sync.Mutex, views *[]*KnowledgeHostedBuildView, within time.Duration) *KnowledgeHostedBuildView {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		mu.Lock()
		for _, v := range *views {
			if v.Final {
				mu.Unlock()
				return v
			}
		}
		mu.Unlock()
	}
	t.Fatalf("the build did not end within %s", within)
	return nil
}
