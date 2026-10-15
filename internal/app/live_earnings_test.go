package app

// Publisher Earnings' run-setup line and the star ask, live: the bound
// methods the run setup and a hosted run's result call, against a real
// registry. Skipped unless the variables are set, because it stars a
// QuickAgent for the token's account (and takes the star back):
//
//	AGENTRY_LIVE_EARNINGS_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_EARNINGS_TOKEN=<a PAT with the desktop's scopes> \
//	AGENTRY_LIVE_EARNINGS_REF=<ns>/<name>@<version: someone else's public
//	    QuickAgent, its publisher in the program, not starred by the account> \
//	AGENTRY_LIVE_EARNINGS_OWN_REF=<one of the account's own, optional> \
//	go test -run TestLiveEarnings -v ./internal/app
//
// It checks that the run setup carries the line for REF, that its result
// asks to be starred, that Star stars it (the desktop grant's agent:read is
// enough for PUT /user/stars) and the ask is then gone; and, for OWN_REF,
// that neither the line nor the ask is drawn. The star is removed at the end
// so the account is left as it was found.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveEarnings(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_EARNINGS_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_EARNINGS_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_EARNINGS_REF")
	if reg == "" || token == "" || ref == "" {
		t.Skip("set AGENTRY_LIVE_EARNINGS_REGISTRY, _TOKEN and _REF to check the line and the star ask")
	}
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(settings.Registry{URL: settings.Normalize(reg), Token: token})
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	r, _ := newRunAPI(t)

	avail, err := r.HostedAvailability(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !avail.Offered || !avail.PublisherEarns {
		t.Fatalf("the run setup of %s: offered %v, publisherEarns %v; want the line", ref, avail.Offered, avail.PublisherEarns)
	}
	t.Logf("the run setup of %s carries the line", ref)

	ask, err := r.StarAsk(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := manifest.ParseRef(ref)
	name := p.Namespace + "/" + p.Name
	if !ask.Ask || ask.Name != name {
		t.Fatalf("the result of a run of %s asks %+v; want an ask naming %s (is it starred already?)", ref, ask, name)
	}
	t.Cleanup(func() {
		client, err := registry.New(reg, token)
		if err != nil {
			t.Errorf("unstar %s: %v", name, err)
			return
		}
		if res, err := client.UnstarAgentWithResponse(context.Background(), p.Namespace, p.Name); err != nil || res.StatusCode() != 204 {
			t.Errorf("unstar %s: %v (%v)", name, err, res)
		}
	})
	if err := r.StarQuickAgent(reg, ask.Name); err != nil {
		t.Fatalf("star %s: %v", name, err)
	}
	after, err := r.StarAsk(reg, ref)
	if err != nil || after.Ask {
		t.Fatalf("still asked once starred: %+v, %v", after, err)
	}
	t.Logf("starred %s; the ask is gone", name)

	own := os.Getenv("AGENTRY_LIVE_EARNINGS_OWN_REF")
	if own == "" {
		return
	}
	ownAvail, err := r.HostedAvailability(reg, own)
	if err != nil {
		t.Fatal(err)
	}
	if ownAvail.PublisherEarns {
		t.Fatalf("the run setup of the account's own %s carries the line", own)
	}
	ownAsk, err := r.StarAsk(reg, own)
	if err != nil || ownAsk.Ask {
		t.Fatalf("the account's own %s asks to be starred: %+v, %v", own, ownAsk, err)
	}
	t.Logf("the account's own %s: no line, no ask", own)
}
