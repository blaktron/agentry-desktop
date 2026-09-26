package main

// In Agentry, through the bound surface (agentry-desktop#77): the
// availability rule against a real /meta, a real token list and a real
// release; a run created, streamed, cancelled and downloaded against a
// fake runs extension; the artefacts under the chosen folder; and no
// docker anywhere on the path.

import (
	"archive/tar"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

const hostedTestToken = "agy_desktoptoken0000"

// hostedEnvelope is a result.json as a pushed run's outputs carry it. Its
// receipt parts differ from the registry's on purpose: they are never
// trusted, since the agent could have written them.
const hostedEnvelope = `{"agentry":"1","status":"ok","exitClass":"ok","attestation":{"state":"verified","keyId":"k1"},` +
	`"capabilities_enforced":{"network.egress":"llm-proxy allowlist: api.example.com"},"harness":{"adapter":"docker-agent","model":"m"},"result":"the summary"}`

// hostedFake is a registry with the runs extension: the core routes the
// availability rule reads (/meta, /user/tokens, /resolve) and the account
// side of x-runs.yaml.
type hostedFake struct {
	mu sync.Mutex

	extensions []string
	scopes     []string
	// tokensStatus lets a test refuse the token list, which is the
	// "scopes unreadable" branch of the rule.
	tokensStatus int
	modes        []string
	secrets      []string
	enabled      bool

	run *hostedrun.Run
	// outputs are the run's files; the registry serves them the way a
	// pushed run's are, as one output.tar holding them all.
	outputs map[string]string
	// eventsRead counts the requests to the run's events route, which
	// the app must no longer make.
	eventsRead int
	// lostOutput makes the output route answer 404, as the registry does
	// for a bundle it can no longer open.
	lostOutput bool
	// noReceipt finishes the run the way one pushed before
	// agentry-enterprise#121 did: with no receipt kept.
	noReceipt bool
	created   *hostedrun.CreateRequest
	refuse    func(w http.ResponseWriter)
}

func (f *hostedFake) start(t *testing.T) string {
	t.Helper()
	if f.extensions == nil {
		f.extensions = []string{"billing", "runs"}
	}
	if f.scopes == nil {
		f.scopes = []string{"agent:read", "user:profile", "run:read", "run:write"}
	}
	if f.modes == nil {
		f.modes = []string{"autonomous"}
	}
	if f.tokensStatus == 0 {
		f.tokensStatus = http.StatusOK
	}
	if f.outputs == nil {
		f.outputs = map[string]string{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, map[string]any{
			"version": "1", "apiVersion": "v1", "features": map[string]bool{}, "extensions": f.extensions,
		})
	})
	mux.HandleFunc("GET /api/v1/user/tokens", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+hostedTestToken || f.tokensStatus != http.StatusOK {
			w.WriteHeader(f.tokensStatus)
			return
		}
		hostedJSON(w, http.StatusOK, []map[string]any{
			{"id": "00000000-0000-0000-0000-0000000000a1", "name": "another device", "prefix": "agy_other000", "scopes": []string{"agent:read"},
				"expiresAt": "2027-01-01T00:00:00Z", "createdAt": "2026-01-01T00:00:00Z"},
			{"id": "00000000-0000-0000-0000-0000000000a2", "name": "Agentry Desktop", "prefix": hostedTestToken[:12], "scopes": f.scopes,
				"expiresAt": "2027-01-01T00:00:00Z", "createdAt": "2026-01-01T00:00:00Z"},
		})
	})
	mux.HandleFunc("GET /api/v1/resolve", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, map[string]any{
			"archiveUrl": "/archive.tar.zst", "attestationUrl": "/att", "digest": "sha256:" + strings.Repeat("ab", 32),
			"manifest": hostedManifestDoc(t, f.modes, f.secrets), "namespace": "acme", "name": "reporter",
			"version": "1.0.0", "yanked": false,
		})
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, r *http.Request) {
		// The registry names where its subscription is managed
		// (agentry-enterprise#41); the app never builds that path.
		hostedJSON(w, http.StatusOK, map[string]any{
			"tier": "standard", "balance": 40, "allowance": 50, "submissionCost": 10,
			"periodEnd": "2026-10-11T00:00:00Z", "privateAgents": map[string]any{"max": 10, "used": 1},
			"upgrade": map[string]any{"available": true, "manage": false, "url": "http://" + r.Host + "/settings/subscription"},
		})
	})
	mux.HandleFunc("GET /api/v1/x/runs/status", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, map[string]any{
			"enabled": f.enabled, "workersOnline": 0, "hostsOnline": 1, "queued": 0,
			"caller": map[string]any{
				"tier": "standard", "caps": map[string]any{"timeoutSeconds": 300, "maxTurns": 20, "maxTokens": 20000},
				"holdCredits": 20, "inputTokensPerCredit": 50000, "outputTokensPerCredit": 25000,
			},
		})
	})
	mux.HandleFunc("POST /api/v1/x/runs/inputs", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		hostedJSON(w, http.StatusCreated, map[string]any{"id": "input-" + name, "name": name, "bytes": r.ContentLength})
	})
	mux.HandleFunc("POST /api/v1/x/runs/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.refuse != nil {
			f.refuse(w)
			return
		}
		var req hostedrun.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.created = &req
		f.run = &hostedrun.Run{
			ID: "22222222-2222-2222-2222-222222222222", Ref: req.Ref, Digest: "sha256:abc", Mode: "autonomous",
			Status: hostedrun.StatusQueued, QueuePosition: 3, HoldCredits: 20,
			Caps: hostedrun.Caps{TimeoutSeconds: 300, MaxTurns: 20, MaxTokens: 20000}, CreatedAt: time.Now().UTC(),
		}
		hostedJSON(w, http.StatusCreated, f.run)
	})
	mux.HandleFunc("GET /api/v1/x/runs/", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		items := []*hostedrun.Run{}
		if f.run != nil {
			items = append(items, f.run)
		}
		hostedJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/x/runs/{id}", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, f.run)
	})
	mux.HandleFunc("POST /api/v1/x/runs/{id}/cancel", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.run.CancelRequested = true
		hostedJSON(w, http.StatusOK, f.run)
	})
	mux.HandleFunc("GET /api/v1/x/runs/{id}/outputs/{name...}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		files, lost := maps.Clone(f.outputs), f.lostOutput
		f.mu.Unlock()
		if lost || r.PathValue("name") != hostedrun.PushedOutput {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		names := slices.Sorted(maps.Keys(files))
		tw := tar.NewWriter(w)
		for _, n := range names {
			_ = tw.WriteHeader(&tar.Header{Name: n, Mode: 0o600, Size: int64(len(files[n])), Typeflag: tar.TypeReg})
			_, _ = tw.Write([]byte(files[n]))
		}
		_ = tw.Close()
	})
	mux.HandleFunc("GET /api/v1/x/runs/{id}/events", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.eventsRead++
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// finish moves the fake's run to finished with a receipt and its outputs.
func (f *hostedFake) finish(outcome, exitClass string, settled int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	f.run.Status = hostedrun.StatusFinished
	f.run.Outcome, f.run.ExitClass, f.run.FinishedAt = outcome, exitClass, &now
	f.run.SettledCredits = &settled
	f.run.Usage = &hostedrun.Usage{InputTokens: 800, OutputTokens: 200, Turns: 3, WallMS: 9000}
	// A pushed run: the registry keeps the host's cut of the CLI's stdout
	// envelope as the receipt (agentry-enterprise#121); a run pushed before
	// that (noReceipt) has none.
	f.run.Receipt = hostedrun.Receipt{CapabilitiesEnforced: map[string]string{"runtime": "runsc: the agent container"}}
	f.run.Receipt.Attestation.State = "unsigned"
	f.run.Receipt.Harness.Adapter = "docker-agent"
	if f.noReceipt {
		f.run.Receipt = hostedrun.Receipt{}
	}
	f.run.Outputs = []hostedrun.Artefact{{Name: hostedrun.PushedOutput, Bytes: 4096}}
}

// set moves the fake's run to a status.
func (f *hostedFake) set(status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.run.Status, f.run.QueuePosition = status, 0
}

func hostedJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// hostedManifestDoc is the resolve result's manifest — the typed struct's
// own JSON, which is what a real registry serves.
func hostedManifestDoc(t *testing.T, modes, secrets []string) map[string]any {
	t.Helper()
	y := "schemaVersion: \"2\"\nname: reporter\nnamespace: acme\nversion: 1.0.0\nsummary: \"hosted run fixture\"\n" +
		"modes: [" + strings.Join(modes, ", ") + "]\nentrypoints:\n  instruction: SKILL.md\ncapabilities:\n" +
		"  network:\n    egress: [api.example.com]\n"
	if len(secrets) > 0 {
		y += "  secrets: [" + strings.Join(secrets, ", ") + "]\n"
	}
	y += "publish:\n  include: [agent.yaml, SKILL.md]\n"
	m, err := manifest.Parse([]byte(y))
	if err != nil {
		t.Fatalf("fixture manifest: %v", err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// signIn points the app's state at temp dirs and stores a token for reg in
// the settings file (the keychain has no entry for a 127.0.0.1 port). It
// returns the temporary HOME and runner home, so a test can prove a hosted
// run wrote nothing into either.
func signIn(t *testing.T, reg string) (home, runnerHome string) {
	t.Helper()
	home, runnerHome = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTRY_RUNNER_HOME", runnerHome)
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(settings.Registry{URL: settings.Normalize(reg), Token: hostedTestToken})
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	return home, runnerHome
}

// newRunAPI is a RunAPI with an app context and no webview: the emitter is
// replaced for the test's life, so the events the panel would receive are
// collected instead.
func newRunAPI(t *testing.T) (*RunAPI, *emitted) {
	t.Helper()
	e := &emitted{}
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if len(data) != 1 {
			return
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		switch name {
		case eventHostedRun:
			if v, ok := data[0].(*HostedRunView); ok {
				e.runs = append(e.runs, v)
			}
		case eventRunLog:
			if line, ok := data[0].(string); ok {
				e.log = append(e.log, line)
			}
		}
	}
	t.Cleanup(func() { emitEvent = old })
	return &RunAPI{app: &App{ctx: context.Background()}}, e
}

// emitted collects what the panel would have received: the run-log lines
// the local engine's view shows, and the hosted run's states.
type emitted struct {
	mu   sync.Mutex
	log  []string
	runs []*HostedRunView
}

func (e *emitted) lines() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.log...)
}

func (e *emitted) states() []*HostedRunView {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]*HostedRunView(nil), e.runs...)
}

// The rule the panel asks about before it offers the mode.
func TestHostedAvailability(t *testing.T) {
	cases := []struct {
		name      string
		set       func(*hostedFake)
		offered   bool
		available bool
		says      string
	}{
		{"everything holds", func(f *hostedFake) {}, true, true, ""},
		{"a core registry", func(f *hostedFake) { f.extensions = []string{} }, false, false, ""},
		{"billing but no runs", func(f *hostedFake) { f.extensions = []string{"billing"} }, false, false, ""},
		{"interactive only", func(f *hostedFake) { f.modes = []string{"interactive"} }, false, false, ""},
		{"a declared secret", func(f *hostedFake) { f.secrets = []string{"PAGERDUTY_TOKEN"} }, true, false, "PAGERDUTY_TOKEN"},
		{"a token from before", func(f *hostedFake) { f.scopes = []string{"agent:read", "user:profile"} }, true, false, "run:read"},
		{"the token list refused", func(f *hostedFake) { f.tokensStatus = http.StatusForbidden }, true, false, "did not say what the stored token may do"},
		{"hosted runs off", func(f *hostedFake) { f.enabled = false }, true, false, "not taking hosted runs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &hostedFake{enabled: true}
			c.set(f)
			reg := f.start(t)
			signIn(t, reg)
			r, _ := newRunAPI(t)

			v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			if v.Offered != c.offered || v.Available != c.available {
				t.Fatalf("offered=%v available=%v, want %v/%v — %+v", v.Offered, v.Available, c.offered, c.available, v)
			}
			if c.says != "" && !strings.Contains(v.Reason, c.says) {
				t.Fatalf("reason = %q, want it to mention %q", v.Reason, c.says)
			}
			if !v.Offered {
				return
			}
			// A sandbox host counts as a runner as a worker does: the fake
			// reports one host and no worker (agentry-enterprise#83).
			if v.RunnersOnline != 1 {
				t.Fatalf("runners online = %d, want the one host", v.RunnersOnline)
			}
			if v.HoldCredits != 20 || v.Tier != "standard" || v.Caps == nil || v.Caps.MaxTokens != 20000 {
				t.Fatalf("the hold and caps come from the registry: %+v", v)
			}
			if !v.BalanceKnown || v.Balance != 40 {
				t.Fatalf("balance = %d (known %v)", v.Balance, v.BalanceKnown)
			}
			if v.MaxInputFiles != hostedrun.MaxInputFiles || v.MaxInputBytes != hostedrun.MaxInputBytes {
				t.Fatalf("the input limits are the plan's: %+v", v)
			}
		})
	}
}

// Nobody signed in: the mode is absent and the registry is asked nothing
// about runs at all.
func TestHostedAvailabilityWithoutSigningIn(t *testing.T) {
	f := &hostedFake{enabled: true}
	reg := f.start(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	r, _ := newRunAPI(t)

	v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.Offered || v.Available || v.SignedIn || v.Reason != "" {
		t.Fatalf("availability = %+v", v)
	}
}

// One In-Agentry run end to end through the bound surface: inputs
// uploaded, the run created with the intake answers, its states reported
// as it moves — queued, running, done with its outcome and credits — and
// the pushed run's output.tar unpacked into the chosen folder, the receipt
// read back; no log, no events route, no docker client anywhere near it
// (hosted runs in production M3, agentry-desktop#182).
func TestStartHostedRunReportsStatesAndDownloads(t *testing.T) {
	f := &hostedFake{
		enabled: true,
		outputs: map[string]string{
			"result.json":       hostedEnvelope,
			"reports/report.md": "# Report\n",
			"receipts/run.json": `{"agentry":"1"}`,
		},
	}
	reg := f.start(t)
	home, runnerHome := signIn(t, reg)
	r, events := newRunAPI(t)
	fastPoll(t)

	inputDir := t.TempDir()
	input := filepath.Join(inputDir, "incident.txt")
	if err := os.WriteFile(input, []byte("INC-4821"), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	r.app.markPicked(inputDir, outDir) // what the native pickers record (D-030)

	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "summarise INC-4821", []string{input}, outDir, map[string]string{"since": "yesterday"})
	if err != nil {
		t.Fatal(err)
	}
	if start.Refusal != "" || start.Run == nil || start.Run.Status != hostedrun.StatusQueued || start.Run.QueuePosition != 3 {
		t.Fatalf("start = %+v (run %+v)", start, start.Run)
	}
	if start.Run.HoldCredits != 20 {
		t.Fatalf("the hold is the registry's: %+v", start.Run)
	}
	f.mu.Lock()
	created := f.created
	f.mu.Unlock()
	if created == nil || created.Task != "summarise INC-4821" || created.Intake["since"] != "yesterday" || len(created.Inputs) != 1 {
		t.Fatalf("created = %+v", created)
	}

	// A runner takes it: the panel hears it is running.
	waitForStatus(t, events, hostedrun.StatusQueued)
	f.set(hostedrun.StatusRunning)
	running := waitForStatus(t, events, hostedrun.StatusRunning)
	if running.State != "running" || running.StatusLine != "running" || running.Final {
		t.Fatalf("running = %+v", running)
	}
	// The run finishes; the follow loop downloads and reports.
	f.finish(hostedrun.OutcomeSucceeded, "ok", 2)
	final := waitForFinal(t, events)
	if lines := events.lines(); len(lines) != 0 {
		t.Fatalf("a run In Agentry wrote to the run-log view: %q", lines)
	}
	f.mu.Lock()
	read := f.eventsRead
	f.mu.Unlock()
	if read != 0 {
		t.Fatalf("the app read the run's events route %d time(s)", read)
	}
	if final.State != "done — pass" || final.Outcome != hostedrun.OutcomeSucceeded || final.OutcomeTone != "ok" || final.ExitClass != "ok" {
		t.Fatalf("final = %+v", final)
	}
	if final.TotalTokens != 1000 || final.Turns != 3 || final.WallMS != 9000 {
		t.Fatalf("usage = %+v", final)
	}
	if !final.Settled || final.SettledCredits != 2 || final.HoldCredits != 20 {
		t.Fatalf("credits = %+v", final)
	}
	// The receipt listed is the registry's, the host's cut of the CLI's
	// stdout, never the result.json in the outputs, which claims another
	// capability and a verified attestation: the agent could have written
	// it (agentry-enterprise#121, #185).
	if !final.ReceiptKept || final.CapabilitiesEnforced["runtime"] != "runsc: the agent container" || final.CapabilitiesEnforced["network.egress"] != "" ||
		final.AttestationState != "unsigned" || final.Adapter != "docker-agent" {
		t.Fatalf("the receipt listed: %+v", final)
	}
	if final.ResultJSON != hostedEnvelope {
		t.Fatalf("result = %q", final.ResultJSON)
	}
	if final.OutDir != outDir || len(final.Outputs) != 3 {
		t.Fatalf("outputs = %+v (dir %s)", final.Outputs, final.OutDir)
	}
	for _, a := range final.Outputs {
		if !strings.HasPrefix(a.Path, outDir+string(filepath.Separator)) {
			t.Fatalf("%s landed at %s, outside %s", a.Name, a.Path, outDir)
		}
		if _, err := os.Stat(a.Path); err != nil {
			t.Fatalf("%s: %v", a.Name, err)
		}
	}
	if final.DownloadError != "" {
		t.Fatalf("download error: %s", final.DownloadError)
	}
	// The run lock stayed free the whole time: a hosted run takes no
	// docker lock, so a local run could have started beside it.
	if !r.mu.TryLock() {
		t.Fatal("the hosted run held the docker run lock")
	}
	r.mu.Unlock()

	// And nothing of this machine's run setup was touched: no operator
	// policy, no runner cache, no MCP profile — a hosted run needs none of
	// them, which is what makes it work where docker does not (§4.7).
	for _, dir := range []string{filepath.Join(home, ".agentry"), runnerHome} {
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) > 0 {
			t.Fatalf("a hosted run wrote into %s: %v", dir, entries)
		}
	}
}

// A run the registry kept no receipt for (pushed before
// agentry-enterprise#121) lists none, and a result.json in its outputs —
// claiming a verified attestation — is not read as one.
func TestAPushedRunWithNoReceiptListsNone(t *testing.T) {
	f := &hostedFake{enabled: true, noReceipt: true, outputs: map[string]string{"result.json": hostedEnvelope}}
	reg := f.start(t)
	signIn(t, reg)
	r, events := newRunAPI(t)
	fastPoll(t)
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil); err != nil {
		t.Fatal(err)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 1)
	final := waitForFinal(t, events)
	if final.ReceiptKept || final.AttestationState != "" || len(final.CapabilitiesEnforced) != 0 || final.Adapter != "" {
		t.Fatalf("a receipt was read from the outputs: %+v", final)
	}
	if final.ResultJSON != hostedEnvelope || len(final.Outputs) != 1 {
		t.Fatalf("the outputs still came down: %+v", final)
	}
}

// A run whose output cannot be downloaded finishes with the error and no
// list of files: nothing came down, so nothing reads as saved.
func TestHostedRunWithALostOutputListsNothing(t *testing.T) {
	f := &hostedFake{enabled: true, lostOutput: true, outputs: map[string]string{"result.json": hostedEnvelope}}
	reg := f.start(t)
	signIn(t, reg)
	r, events := newRunAPI(t)
	fastPoll(t)
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil); err != nil {
		t.Fatal(err)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 1)
	final := waitForFinal(t, events)
	if final.DownloadError == "" || len(final.Outputs) != 0 {
		t.Fatalf("a lost output: error %q, outputs %+v", final.DownloadError, final.Outputs)
	}
	if final.State != "done — pass" || !final.Settled {
		t.Fatalf("the run itself still reads as it finished: %+v", final)
	}
}

// A refused create reads as a sentence with its slug — and, for credits,
// the way onward — rather than an error with JSON in it.
func TestStartHostedRunRendersRefusals(t *testing.T) {
	f := &hostedFake{enabled: true, refuse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/credits-exhausted","title":"Not enough credits for this run","status":402,` +
			`"detail":"a run holds 20 credits; this account has 4, and its allowance resets on 11 October 2026","balance":4,"hold":20,` +
			`"resetsAt":"2026-10-11T00:00:00Z","tier":"standard"}`))
	}}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)

	out1 := t.TempDir()
	r.app.markPicked(out1)
	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out1, nil)
	if err != nil {
		t.Fatalf("a refusal is a result, not an error: %v", err)
	}
	if start.Refusal != "credits-exhausted" || start.Balance != 4 || start.Hold != 20 || start.ResetsAt == "" {
		t.Fatalf("refusal = %+v", start)
	}
	if !strings.Contains(start.Detail, "a run holds 20 credits") || strings.Contains(start.Detail, "{") {
		t.Fatalf("detail = %q", start.Detail)
	}
	// No destination travels in the view: the panel asks
	// SettingsAPI.OpenUpgrade for it when a person clicks, so what opens
	// has been checked against the registry's own origin first
	// (agentry-enterprise#41).

	f.mu.Lock()
	f.refuse = func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/run-active","title":"A run is already active","status":409,` +
			`"detail":"One hosted run at a time per account: wait for it to finish, or cancel it."}`))
	}
	f.mu.Unlock()
	out2 := t.TempDir()
	r.app.markPicked(out2)
	start, err = r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start.Refusal != "run-active" {
		t.Fatalf("refusal = %+v", start)
	}
}

// Reopening the panel with an active run reattaches to it rather than
// starting a second (D11), and Cancel reaches the registry.
func TestAttachAndCancel(t *testing.T) {
	f := &hostedFake{enabled: true}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)

	// Nothing active yet.
	attachDir := t.TempDir()

	r.app.markPicked(attachDir) // the folder the panel picked (D-030)

	att, err := r.AttachHostedRun(reg, attachDir)
	if err != nil || att.Run != nil {
		t.Fatalf("attach with no run: %+v %v", att, err)
	}

	f.mu.Lock()
	f.run = &hostedrun.Run{
		ID: "33333333-3333-3333-3333-333333333333", Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusRunning,
		HoldCredits: 20, CreatedAt: time.Now().UTC(),
	}
	f.mu.Unlock()

	outDir := t.TempDir()
	r.app.markPicked(outDir)
	att, err = r.AttachHostedRun(reg, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if att.Run == nil || att.Run.ID != "33333333-3333-3333-3333-333333333333" || att.Run.Status != hostedrun.StatusRunning {
		t.Fatalf("attach = %+v", att)
	}
	// A second start is refused while one is being followed.
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil); err == nil ||
		!strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("a second start: %v", err)
	}
	cancelled, err := r.CancelHostedRun()
	if err != nil {
		t.Fatal(err)
	}
	if !cancelled.CancelRequested {
		t.Fatalf("cancel = %+v", cancelled)
	}
	f.finish(hostedrun.OutcomeInterrupted, "interrupted", 1)
	// The follow loop ends on its own once the run is over.
	deadline := time.Now().Add(10 * time.Second)
	for r.hostedCurrent() != nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if r.hostedCurrent() != nil {
		t.Fatal("the follow loop outlived the run")
	}
}

// fastPoll shortens the follow loop's re-read for the test's life.
func fastPoll(t *testing.T) {
	t.Helper()
	old := hostedPoll
	hostedPoll = 10 * time.Millisecond
	t.Cleanup(func() { hostedPoll = old })
}

// waitForStatus waits for the follow loop to report the run in a status.
func waitForStatus(t *testing.T, events *emitted, status string) *HostedRunView {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, v := range events.states() {
			if v.Status == status {
				return v
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the run was never reported %s", status)
	return nil
}

// waitForFinal waits for the follow loop's last word on the run.
func waitForFinal(t *testing.T, events *emitted) *HostedRunView {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, v := range events.states() {
			if v.Final {
				return v
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the run never reported a final state")
	return nil
}
