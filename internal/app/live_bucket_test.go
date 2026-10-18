package app

// A bucket destination live, as an end-to-end check: the bound methods driven
// as the page drives them, with the real engine, this machine's docker, a model
// that is a scripted stand-in on the LAN, and a MinIO on the LAN. Skipped unless
// AGENTRY_LIVE_BUCKET=1:
//
//	AGENTRY_LIVE_BUCKET=1 \
//	AGENTRY_LIVE_BUCKET_ENDPOINT=http://<this machine's LAN address>:<port> \
//	AGENTRY_LIVE_BUCKET_NAME=<an existing bucket> \
//	AGENTRY_LIVE_BUCKET_KEY_ID=<the service's access key id> \
//	AGENTRY_LIVE_BUCKET_SECRET=<its secret access key> \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> AGENTRY_RUNNER_HOME=<a scratch runner home> \
//	HOME=<a scratch home> DBUS_SESSION_BUS_ADDRESS=<a throwaway secret service's bus> \
//	go test -run TestLiveBucket -v ./internal/app
//
// It refuses to run on the person's own settings.
//
// What it holds: a bucket saved through the page's binding is the CLI's; a key
// pair is never kept for another service; a run given it puts each deliverable
// under <prefix>/<run id>/ and the result lists the objects; a wrong secret
// leaves the run ok and the delivery failed, with a retry on offer, and the
// retry, with the right secret, delivers it; and no credential comes back to the
// page or lands in the run folder.

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/destinations"
)

func TestLiveBucket(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_BUCKET") != "1" {
		t.Skip("runs a real run: set AGENTRY_LIVE_BUCKET=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" || os.Getenv("HOME") == "" || os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Fatalf("set %s, %s, HOME and DBUS_SESSION_BUS_ADDRESS (a throwaway secret service) to scratch values", configdir.EnvVar, policy.HomeEnv)
	}
	endpoint, bucket, keyID, secret := os.Getenv("AGENTRY_LIVE_BUCKET_ENDPOINT"), os.Getenv("AGENTRY_LIVE_BUCKET_NAME"), os.Getenv("AGENTRY_LIVE_BUCKET_KEY_ID"), os.Getenv("AGENTRY_LIVE_BUCKET_SECRET")
	if endpoint == "" || bucket == "" || keyID == "" || secret == "" {
		t.Fatal("name the service: AGENTRY_LIVE_BUCKET_ENDPOINT, _NAME, _KEY_ID and _SECRET")
	}
	reg := cmp.Or(os.Getenv("AGENTRY_LIVE_BUCKET_REGISTRY"), "https://dev.secureagentry.net")
	ref := cmp.Or(os.Getenv("AGENTRY_LIVE_BUCKET_REF"), "runs-checker-125725/hosted-smoke@0.1.0")
	old := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = old })

	app := &App{ctx: context.Background()}
	d := &DestinationsAPI{app: app}
	in := destinations.Input{Name: "minio", Kind: "s3", Bucket: bucket, Prefix: "agentry/live", Endpoint: endpoint, OnMyNetwork: true, AccessKeyID: keyID, Secret: "not-the-secret"}
	st, err := d.Save(in)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(st); strings.Contains(string(b), secret) || strings.Contains(string(b), keyID) || strings.Contains(string(b), "not-the-secret") {
		t.Fatalf("the view carries a credential: %s", b)
	}
	if len(st.Destinations) != 1 || !st.Destinations[0].CanSend || st.Destinations[0].Kind != "s3" {
		t.Fatalf("view: %+v", st.Destinations)
	}
	// A pair is not kept for another service, nor for the tick taken off.
	other := in
	other.Endpoint, other.AccessKeyID, other.Secret = "http://203.0.113.9:9000", "", ""
	if _, err := d.Save(other); err == nil {
		t.Fatal("the saved key pair went to another service")
	}
	if !d.KeepsKeyPair(destinations.Input{Name: "minio", Kind: "s3", Bucket: "elsewhere", Endpoint: endpoint, OnMyNetwork: true}) {
		t.Error("the same service does not keep its pair")
	}

	r := &RunAPI{app: app}
	run := func() *RunResultView {
		res, err := r.RunAgent(reg, ref, "Write this week's digest.", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil, []string{"minio"})
		if err != nil {
			t.Fatalf("RunAgent: %v", err)
		}
		t.Logf("status %s, exit %d, run %s, folder %s", res.Status, res.ExitCode, res.RunID, res.OutDir)
		if res.Status != "ok" || res.ExitCode != 0 {
			t.Fatalf("the run ended %q (exit %d): %s", res.Status, res.ExitCode, res.Error)
		}
		return res
	}

	// A wrong secret: the run is ok, the delivery failed, a retry is offered.
	res := run()
	v, err := r.RunDeliverables(res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sent with a wrong secret: %+v", v.Sent)
	if len(v.Sent) != 1 || v.Sent[0].Kind != "s3" || v.Sent[0].Failed == 0 || v.Sent[0].Error != "status" || !v.Sent[0].CanRetry || !v.Sent[0].OnMyNetwork {
		t.Fatalf("the result's sends: %+v", v.Sent)
	}

	// The right secret, saved over the same service (the key id is kept), and the retry delivers.
	in.AccessKeyID, in.Secret = "", secret
	if _, err := d.Save(in); err != nil {
		t.Fatal(err)
	}
	v, err = r.RetryDelivery(res.RunID, "minio")
	if err != nil {
		t.Fatalf("RetryDelivery: %v", err)
	}
	t.Logf("after the retry: %+v", v.Sent)
	if len(v.Sent) != 1 || v.Sent[0].Failed != 0 || v.Sent[0].Delivered == 0 || v.Sent[0].CanRetry {
		t.Fatalf("the sends after the retry: %+v", v.Sent)
	}

	// A fresh run delivers on its own.
	res2 := run()
	v2, err := r.RunDeliverables(res2.RunID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sent by a fresh run: %+v", v2.Sent)
	if len(v2.Sent) != 1 || v2.Sent[0].Failed != 0 || v2.Sent[0].Delivered == 0 {
		t.Fatalf("a fresh run's sends: %+v", v2.Sent)
	}
	for _, view := range []any{v, v2} {
		if b, _ := json.Marshal(view); strings.Contains(string(b), secret) || strings.Contains(string(b), keyID) {
			t.Fatalf("a result carries a credential: %s", b)
		}
	}
	for _, dir := range []string{res.OutDir, res2.OutDir} {
		_ = filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			if b, err := os.ReadFile(p); err == nil && (strings.Contains(string(b), secret) || strings.Contains(string(b), keyID)) {
				t.Errorf("%s holds a credential", p)
			}
			return nil
		})
	}
	t.Logf("run folders: %s %s", res.OutDir, res2.OutDir)
}
