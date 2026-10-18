package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-desktop/internal/destinations"
)

// destinationsEnv isolates the CLI's configuration folder and gives the
// keychain a mock, never the person's.
func destinationsEnv(t *testing.T) {
	t.Helper()
	freshConfig(t)
	t.Setenv(modelkey.EnvNoKeyring, "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
}

func TestSettingsDestinationsSaveListAndRemoveNeverReturnACredential(t *testing.T) {
	destinationsEnv(t)
	d := &DestinationsAPI{app: &App{}}
	st, err := d.Save(destinations.Input{Name: "n8n", URL: "https://hooks.example.com/webhook/abc?token=URLTOKEN", Secret: "whsec_PAGE", Authorization: "Bearer PAGEAUTH"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(st)
	for _, bad := range []string{"PAGE", "URLTOKEN", "/webhook/"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("a view carries %q: %s", bad, b)
		}
	}
	if len(st.Destinations) != 1 || !st.Destinations[0].HasSecret || !st.Destinations[0].HasAuthorization {
		t.Fatalf("view: %+v", st.Destinations)
	}
	// The CLI's reader gets what the page saved.
	r, err := destination.Resolve([]string{"n8n"})
	if err != nil || r[0].Secret != "whsec_PAGE" || r[0].Authorization != "Bearer PAGEAUTH" {
		t.Fatalf("%+v %v", r, err)
	}
	if got := d.Check("https://10.0.0.5/x", false); got == "" {
		t.Error("a private address was accepted without the tick")
	}
	if got := d.Check("https://10.0.0.5/x", true); got != "" {
		t.Errorf("a private address on my network was refused: %s", got)
	}
	if _, err := d.SetDefault("acme/weekly", []string{"n8n"}); err != nil {
		t.Fatal(err)
	}
	if got := d.DefaultFor("acme/weekly@1.0.0"); len(got) != 1 {
		t.Errorf("default: %v", got)
	}
	st, err = d.Remove("n8n")
	if err != nil || len(st.Destinations) != 0 || destination.HasSecret("n8n") {
		t.Fatalf("%+v %v", st, err)
	}
	if _, err := d.Save(destinations.Input{Name: "bad", URL: "http://hooks.example.com/x", Secret: "s"}); err == nil {
		t.Error("an http address was saved without the tick")
	}
}

// sendRun is a run folder (run-1) whose envelope lists one file and records a
// send of it to "n8n" with the given status, and the ledger the given lines.
func sendRun(t *testing.T, envStatus string, ledger ...receipt.Delivery) (*RunAPI, string) {
	t.Helper()
	r, dir := deliverableRun(t, map[string][]byte{"reports/a.md": []byte("a")})
	var env map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, "result.json"))
	_ = json.Unmarshal(b, &env)
	env["deliveries"] = []receipt.Delivery{{Destination: "n8n", Kind: "webhook", Host: "hooks.example.com:443", Object: "reports/a.md", SHA256: sum([]byte("a")), Status: envStatus, Error: map[bool]string{true: "status", false: ""}[envStatus == "failed"]}}
	b, _ = json.Marshal(env)
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	var lines []string
	for _, l := range ledger {
		x, _ := json.Marshal(l)
		lines = append(lines, string(x))
	}
	if len(lines) > 0 {
		if err := os.MkdirAll(filepath.Join(dir, "receipts"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeRunFile(t, filepath.Join(dir, "receipts", "deliveries.ndjson"), strings.Join(lines, "\n")+"\n")
	}
	return r, dir
}

// saveN8n saves the destination "n8n" in the current configuration folder (each
// run helper starts a fresh one, so it is saved after the run is made).
func saveN8n(t *testing.T) *DestinationsAPI {
	t.Helper()
	keyring.MockInit()
	d := &DestinationsAPI{app: &App{}}
	if _, err := d.Save(destinations.Input{Name: "n8n", URL: "https://hooks.example.com/x", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestARunResultListsWhereItWasSentAndTheLatestWordWins(t *testing.T) {
	failed := receipt.Delivery{Destination: "n8n", Kind: "webhook", Host: "hooks.example.com:443", Object: "reports/a.md", SHA256: sum([]byte("a")), Status: "failed", Error: "status"}
	r, _ := sendRun(t, "failed", failed)
	d := saveN8n(t)
	v, err := r.RunDeliverables("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Sent) != 1 || v.Sent[0].Failed != 1 || v.Sent[0].Error != "status" || !v.Sent[0].CanRetry || v.Sent[0].Destination != "n8n" {
		t.Fatalf("sent: %+v", v.Sent)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "token") {
		t.Errorf("the view carries an address's token: %s", b)
	}
	// A retry is a later line: it is the outcome now, and nothing to retry.
	ok := failed
	ok.Status, ok.Error = "delivered", ""
	r, _ = sendRun(t, "failed", failed, ok)
	saveN8n(t)
	v, _ = r.RunDeliverables("run-1")
	if len(v.Sent) != 1 || v.Sent[0].Delivered != 1 || v.Sent[0].Failed != 0 || v.Sent[0].CanRetry {
		t.Fatalf("after the retry: %+v", v.Sent)
	}
	// A destination that is gone offers no retry.
	r, _ = sendRun(t, "failed", failed)
	if _, err := d.Remove("n8n"); err == nil {
		t.Fatal("removed a destination of another configuration folder")
	}
	if v, _ = r.RunDeliverables("run-1"); len(v.Sent) != 1 || v.Sent[0].CanRetry {
		t.Fatalf("a removed destination offers a retry: %+v", v.Sent)
	}
	// A run with none has an empty list, not null.
	r, _ = deliverableRun(t, map[string][]byte{"reports/a.md": []byte("a")})
	v, _ = r.RunDeliverables("run-1")
	if b, _ := json.Marshal(v); !strings.Contains(string(b), `"sent":[]`) {
		t.Errorf("a run with no sends: %s", b)
	}
}

func TestRetryDeliverySendsOnlyToADestinationThatFailedTheRun(t *testing.T) {
	failed := receipt.Delivery{Destination: "n8n", Kind: "webhook", Host: "hooks.example.com:443", Object: "reports/a.md", SHA256: sum([]byte("a")), Status: "failed", Error: "status"}
	r, dir := sendRun(t, "failed", failed)
	d := saveN8n(t)
	if _, err := d.Save(destinations.Input{Name: "other", URL: "https://hooks.example.com/other", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	var asked []string
	var inDir string
	old := redeliver
	t.Cleanup(func() { redeliver = old })
	redeliver = func(_ context.Context, runDir string, names []string, _ io.Writer) ([]receipt.Delivery, error) {
		asked, inDir = names, runDir
		ok := failed
		ok.Status, ok.Error = "delivered", ""
		line, _ := json.Marshal(ok)
		f, err := os.OpenFile(filepath.Join(runDir, "receipts", "deliveries.ndjson"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			return nil, err
		}
		_, _ = f.Write(append(line, '\n'))
		_ = f.Close()
		return []receipt.Delivery{ok}, nil
	}
	// A destination the run never failed is refused, and so is one not saved.
	if _, err := r.RetryDelivery("run-1", "other"); err == nil || len(asked) != 0 {
		t.Errorf("a destination the run has no failure for: %v %v", err, asked)
	}
	if _, err := r.RetryDelivery("run-1", "../../etc"); err == nil || len(asked) != 0 {
		t.Errorf("a path named as a destination: %v %v", err, asked)
	}
	v, err := r.RetryDelivery("run-1", "n8n")
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "n8n" || inDir != dir {
		t.Errorf("asked %v in %q, want n8n in %q", asked, inDir, dir)
	}
	if len(v.Sent) != 1 || v.Sent[0].Delivered != 1 || v.Sent[0].CanRetry {
		t.Errorf("after: %+v", v.Sent)
	}
	// Removed since the run: refused with a word about it, nothing sent.
	asked = nil
	r, _ = sendRun(t, "failed", failed)
	if _, err := r.RetryDelivery("run-1", "n8n"); err == nil || len(asked) != 0 {
		t.Errorf("a removed destination: %v %v", err, asked)
	}
}
