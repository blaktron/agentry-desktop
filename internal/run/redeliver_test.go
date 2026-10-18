package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/send"
)

// TestRedeliverSendsAFinishedRunsFolderThroughTheCLIsStep: the same delivery
// step the command line runs, on this computer's engine (a fake here), with the
// credential on its stdin and the record added to the run folder's ledger.
func TestRedeliverSendsAFinishedRunsFolderThroughTheCLIsStep(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	t.Setenv(policy.HomeEnv, t.TempDir())
	t.Setenv(modelkey.EnvNoKeyring, "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	var st destination.Store
	if err := st.Upsert(destination.Destination{Name: "n8n", Kind: destination.KindWebhook, URL: "https://hooks.example.com/x"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if err := destination.SaveSecrets("n8n", "whsec_DESKTOP", ""); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	body := "# Brief\n"
	if err := os.MkdirAll(filepath.Join(dir, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reports", "brief.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	env := receipt.Envelope{Agentry: receipt.EnvelopeVersion, Status: "ok", RunID: "r1", Ref: "acme/weekly@1.0.0", OutDir: dir, Result: receipt.TextResult("done"),
		Deliverables: &receipt.Deliverables{Files: []receipt.Deliverable{{Path: "reports/brief.md", Title: "Brief", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(body)), Type: "md", Marked: true}}}}
	b, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	f := &dockerx.Fake{}
	var stdin string
	f.Foreground = map[string]*dockerx.FakeForeground{"-delivery": {Handler: func(_ context.Context, r dockerx.RunSpec, in io.Reader, _, _ io.Writer) (int, error) {
		raw, _ := io.ReadAll(in)
		stdin = string(raw)
		var work, jobDir string
		for _, m := range r.Mounts {
			switch m.Target {
			case "/work":
				work = m.Source
			case "/job":
				jobDir = m.Source
			}
		}
		job, err := send.ReadJob(jobDir)
		if err != nil || len(job.Files) != 1 || job.Files[0].Staged == "" || job.RunID != "r1" {
			t.Errorf("job %+v %v", job, err)
		}
		rep, _ := json.Marshal(send.Report{Schema: send.ReportSchema, Kind: "webhook", Status: send.StatusDelivered, Result: send.StatusDelivered, Files: []send.FileResult{{Path: "reports/brief.md", Status: send.StatusDelivered}}})
		_ = os.WriteFile(filepath.Join(work, send.ReportFile), rep, 0o644)
		return 0, nil
	}}}
	old := newDockerClient
	newDockerClient = func() dockerx.Client { return f }
	t.Cleanup(func() { newDockerClient = old })

	ds, err := Redeliver(context.Background(), dir, []string{"n8n"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 || ds[0].Status != receipt.DeliveryDelivered || ds[0].Destination != "n8n" {
		t.Fatalf("deliveries %+v", ds)
	}
	if !strings.Contains(stdin, "whsec_DESKTOP") {
		t.Errorf("the step's stdin: %q", stdin)
	}
	ledger, _ := os.ReadFile(filepath.Join(dir, "receipts", "deliveries.ndjson"))
	if !strings.Contains(string(ledger), `"destination":"n8n"`) || strings.Contains(string(ledger), "DESKTOP") {
		t.Errorf("ledger %q", ledger)
	}
	// Nothing is asked of an engine for a name that is not saved.
	f.Calls = nil
	if _, err := Redeliver(context.Background(), dir, []string{"nope"}, io.Discard); err == nil || len(f.Calls) != 0 {
		t.Errorf("an unsaved destination: %v, %d calls", err, len(f.Calls))
	}
	if _, err := Redeliver(context.Background(), dir, nil, io.Discard); err == nil {
		t.Error("no destination named was accepted")
	}
}

// TestARunNamingADestinationThatIsNotSavedIsRefusedBeforeAnythingStarts: the
// names are read first, so a name that is not saved, or has no signing secret,
// refuses the run before a registry is asked or an image is touched.
func TestARunNamingADestinationThatIsNotSavedIsRefusedBeforeAnythingStarts(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	t.Setenv(policy.HomeEnv, t.TempDir())
	f := &dockerx.Fake{}
	old := newDockerClient
	newDockerClient = func() dockerx.Client { return f }
	t.Cleanup(func() { newDockerClient = old })
	_, err := prepare(context.Background(), Request{Registry: "https://r.example", Ref: "acme/x@1.0.0", Destinations: []string{"nope"}}, io.Discard, "autonomous")
	if err == nil || !strings.Contains(err.Error(), "no destination is saved") {
		t.Fatalf("err = %v", err)
	}
	if len(f.Calls) != 0 {
		t.Errorf("the engine was asked: %d calls", len(f.Calls))
	}
}
