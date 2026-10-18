package app

// Destinations live, as an end-to-end check: the bound methods driven as the
// page drives them, with the real engine, this machine's docker, a model that
// is a scripted stand-in on the LAN, and a reference receiver that verifies the
// signature and answers with the status a file names. Skipped unless
// AGENTRY_LIVE_DESTINATIONS=1:
//
//	AGENTRY_LIVE_DESTINATIONS=1 \
//	AGENTRY_LIVE_DESTINATIONS_URL=http://<this machine's LAN address>:<port>/hook \
//	AGENTRY_LIVE_DESTINATIONS_STATUS=<the file the receiver reads its answer from> \
//	AGENTRY_LIVE_DESTINATIONS_LOG=<the receiver's log> \
//	AGENTRY_LIVE_DESTINATIONS_SECRET=<the secret the receiver verifies with> \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> AGENTRY_RUNNER_HOME=<a scratch runner home> \
//	HOME=<a scratch home> DBUS_SESSION_BUS_ADDRESS=<a throwaway secret service's bus> \
//	go test -run TestLiveDestinations -v ./internal/app
//
// It refuses to run on the person's own settings.
//
// What it holds: a destination saved through the page's binding is the CLI's;
// a run given it sends a signed body the receiver verifies; a receiver that
// answers 500 leaves the run ok and the delivery failed, listed on the result
// with a retry on offer; the retry delivers it through the CLI's own step, and
// no credential comes back to the page.

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/destinations"
)

func TestLiveDestinations(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_DESTINATIONS") != "1" {
		t.Skip("runs a real run: set AGENTRY_LIVE_DESTINATIONS=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" || os.Getenv("HOME") == "" || os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Fatalf("set %s, %s, HOME and DBUS_SESSION_BUS_ADDRESS (a throwaway secret service) to scratch values", configdir.EnvVar, policy.HomeEnv)
	}
	url, statusFile, logFile, secret := os.Getenv("AGENTRY_LIVE_DESTINATIONS_URL"), os.Getenv("AGENTRY_LIVE_DESTINATIONS_STATUS"), os.Getenv("AGENTRY_LIVE_DESTINATIONS_LOG"), os.Getenv("AGENTRY_LIVE_DESTINATIONS_SECRET")
	if url == "" || statusFile == "" || logFile == "" || secret == "" {
		t.Fatal("name the receiver: AGENTRY_LIVE_DESTINATIONS_URL, _STATUS, _LOG and _SECRET")
	}
	reg := cmp.Or(os.Getenv("AGENTRY_LIVE_DESTINATIONS_REGISTRY"), "https://dev.secureagentry.net")
	ref := cmp.Or(os.Getenv("AGENTRY_LIVE_DESTINATIONS_REF"), "runs-checker-125725/hosted-smoke@0.1.0")
	old := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = old })

	app := &App{ctx: context.Background()}
	d := &DestinationsAPI{app: app}
	st, err := d.Save(destinations.Input{Name: "lan", URL: url, OnMyNetwork: true, Secret: secret, Authorization: "Bearer live-desktop"})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(st); strings.Contains(string(b), secret) || strings.Contains(string(b), "live-desktop") {
		t.Fatalf("the view carries a credential: %s", b)
	}

	status := func(code string) {
		if err := os.WriteFile(statusFile, []byte(code), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lines := func() []map[string]any {
		b, _ := os.ReadFile(logFile)
		var out []map[string]any
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var m map[string]any
			if json.Unmarshal([]byte(l), &m) == nil {
				out = append(out, m)
			}
		}
		return out
	}

	status("500")
	before := len(lines())
	r := &RunAPI{app: app}
	res, err := r.RunAgent(reg, ref, "Write this week's digest.", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil, []string{"lan"})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	t.Logf("status %s, exit %d, run %s, folder %s", res.Status, res.ExitCode, res.RunID, res.OutDir)
	if res.Status != "ok" || res.ExitCode != 0 {
		t.Fatalf("the run ended %q (exit %d): %s", res.Status, res.ExitCode, res.Error)
	}
	got := lines()
	if len(got) != before+1 || got[len(got)-1]["signature_ok"] != true || got[len(got)-1]["fresh"] != true || got[len(got)-1]["auth"] != true {
		t.Fatalf("the receiver's log after the run: %v", got[before:])
	}
	v, err := r.RunDeliverables(res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sent: %+v", v.Sent)
	if len(v.Sent) != 1 || v.Sent[0].Destination != "lan" || v.Sent[0].Failed == 0 || v.Sent[0].Error != "status" || !v.Sent[0].CanRetry || !v.Sent[0].OnMyNetwork {
		t.Fatalf("the result's sends: %+v", v.Sent)
	}

	status("200")
	v, err = r.RetryDelivery(res.RunID, "lan")
	if err != nil {
		t.Fatalf("RetryDelivery: %v", err)
	}
	t.Logf("after the retry: %+v", v.Sent)
	if len(v.Sent) != 1 || v.Sent[0].Failed != 0 || v.Sent[0].Delivered == 0 || v.Sent[0].CanRetry {
		t.Fatalf("the sends after the retry: %+v", v.Sent)
	}
	got = lines()
	if last := got[len(got)-1]; len(got) != before+2 || last["signature_ok"] != true || last["status"] != float64(200) {
		t.Fatalf("the receiver's log after the retry: %v", got[before:])
	}
	if b, _ := json.Marshal(v); strings.Contains(string(b), secret) || strings.Contains(string(b), "live-desktop") {
		t.Fatalf("the result carries a credential: %s", b)
	}
	entries, _ := os.ReadDir(res.OutDir)
	for _, e := range entries {
		if b, err := os.ReadFile(res.OutDir + "/" + e.Name()); err == nil && strings.Contains(string(b), secret) {
			t.Errorf("%s holds the secret", e.Name())
		}
	}
}
