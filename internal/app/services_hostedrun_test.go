package app

// In Agentry, through the bound surface: the availability rule against a
// real /meta, a real token list and a real release; a run created, streamed,
// cancelled and downloaded against a fake runs extension; the artefacts
// under the chosen folder; and no docker anywhere on the path.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
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
	// others are more of the account's runs, listed before run: a knowledge
	// build, a draft or a chat the app must not take for a run.
	others []*hostedrun.Run
	// outputs are the run's files; the registry serves them the way a
	// pushed run's are, as one output.tar holding them all.
	outputs map[string]string
	// eventsRead counts the requests to the run's events route, which
	// the app must no longer make.
	eventsRead int
	// lostOutput makes the output route answer 404, as the registry does
	// for a bundle it can no longer open.
	lostOutput bool
	// noReceipt finishes the run the way a run pushed by an older host did:
	// with no receipt kept.
	noReceipt bool
	created   *hostedrun.CreateRequest
	refuse    func(w http.ResponseWriter)
	// attach says the status route offers knowledgeAttach; packs are the
	// account's packs in Agentry, as GET /knowledge/packs lists them.
	attach bool
	packs  []map[string]any
	// vaultSets and vaultDeletes record the vault's two write routes;
	// vaultAnswer, when set, answers a set instead.
	vaultSets    []vaultSet
	vaultDeletes []string
	vaultAnswer  func(w http.ResponseWriter, body []byte)

	// sessions is the status route's sessions line and sessionAllowed the
	// caller's; nil leaves them out, as a registry from before sessions
	// does.
	sessions       *hostedrun.SessionTerms
	sessionAllowed *bool
	// frames are a session's display events in order (sequence numbers
	// from 1), which the stream writes after the `after` it is asked for;
	// streamAfters records each stream's `after`, and sessionPosts every
	// post to a session route by route, whole.
	frames       []string
	streamAfters []string
	sessionPosts map[string][]string
	// tokensGone answers as the registry will once it stops serving a hosted
	// run's token counts: no token counts or dollar cost in a run's usage,
	// no token cap, no rates. False answers as today's routes do, with all
	// of them, which this app no longer reads.
	tokensGone bool
	// sessionRefuse, when set, answers a session post instead of 202.
	sessionRefuse func(w http.ResponseWriter, route string) bool
	cancelled     int
	// streamError, when set, is the class of the `error` event every
	// stream answers with: the registry cannot show this session.
	streamError string
	// noHosts makes the status route report no sandbox host answering.
	noHosts bool
	// failGets makes that many reads of the run answer 503, as a registry
	// the app has lost touch with does.
	failGets int
	// listDelay holds each answer to the runs list, so two attaches are in
	// flight at once.
	listDelay time.Duration
	// onCreate runs as the create is answered: what else happens in the
	// app while the registry creates the run.
	onCreate func()
	// earns is the Publisher Earnings program's answer for acme, whose
	// reporter is public and read by someone else.
	earns bool
	// uploads counts the input uploads, and creates the create posts, each
	// body whole (a start waiting for its connections makes the same create
	// again and uploads nothing again).
	uploads int
	creates []string
	// intake is the release's intake block as manifest YAML, "" for none (a
	// repository question).
	intake string
	// targetHosts is the release's capabilities.targetHosts, nil for none;
	// its host questions go in intake.
	targetHosts []string
	// browser is the release's capabilities.browser as YAML lines under
	// capabilities, "" for none; its site questions go in intake.
	browser string
	// routes, when set, adds routes of another extension to the fake (the
	// schedules extension's).
	routes func(mux *http.ServeMux)
}

// vaultSet is one PUT as the vault received it: the name, the body whole.
type vaultSet struct {
	Name string
	Body string
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
	mux.HandleFunc("GET /api/v1/resolve", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if strings.Contains(r.URL.RawQuery, "missing") {
			hostedJSON(w, http.StatusNotFound, map[string]any{"type": "https://agentry.dev/problems/not-found", "title": "Not found"})
			return
		}
		hostedJSON(w, http.StatusOK, map[string]any{
			"archiveUrl": "/archive.tar.zst", "attestationUrl": "/att", "digest": "sha256:" + strings.Repeat("ab", 32),
			"manifest": hostedManifestDoc(t, f.modes, f.secrets, f.targetHosts, f.browser, f.intake), "namespace": "acme", "name": "reporter",
			"version": "1.0.0", "yanked": false,
		})
	})
	mux.HandleFunc("GET /api/v1/x/billing/earnings/publishers/acme", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"handle": "acme", "earns": f.earns})
	})
	mux.HandleFunc("GET /api/v1/agents/acme/reporter", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"namespace": "acme", "name": "reporter", "visibility": "public"})
	})
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"handle": "pat", "displayName": "Pat", "email": "pat@example.com"})
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, r *http.Request) {
		// The registry names where its subscription is managed; the app
		// never builds that path.
		hostedJSON(w, http.StatusOK, map[string]any{
			"tier": "standard", "balance": 40, "allowance": 50, "submissionCost": 10,
			"periodEnd": "2026-10-11T00:00:00Z", "privateAgents": map[string]any{"max": 10, "used": 1},
			"upgrade": map[string]any{"available": true, "manage": false, "url": "http://" + r.Host + "/settings/subscription"},
		})
	})
	mux.HandleFunc("GET /api/v1/x/runs/status", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		st := map[string]any{
			"enabled": f.enabled, "workersOnline": 0, "hostsOnline": map[bool]int{false: 1, true: 0}[f.noHosts], "queued": 0,
			"caller": map[string]any{
				"tier": "standard", "caps": map[string]any{"timeoutSeconds": 300, "maxTurns": 20},
				"holdCredits": 20,
			},
		}
		if !f.tokensGone {
			c := st["caller"].(map[string]any)
			c["caps"].(map[string]any)["maxTokens"] = 20000
			c["inputTokensPerCredit"], c["outputTokensPerCredit"] = 50000, 25000
			st["inputTokensPerCredit"], st["outputTokensPerCredit"] = 50000, 25000
		}
		if f.attach {
			st["knowledgeAttach"] = map[string]any{"enabled": true, "maxPacks": 5}
		}
		if f.sessions != nil {
			st["sessions"] = f.sessions
		}
		if f.sessionAllowed != nil {
			st["caller"].(map[string]any)["session"] = map[string]any{"allowed": *f.sessionAllowed}
		}
		hostedJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("GET /api/v1/knowledge/packs", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		packs := f.packs
		if packs == nil {
			packs = []map[string]any{}
		}
		hostedJSON(w, http.StatusOK, packs)
	})
	mux.HandleFunc("POST /api/v1/x/runs/inputs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.uploads++
		f.mu.Unlock()
		name := r.URL.Query().Get("name")
		hostedJSON(w, http.StatusCreated, map[string]any{"id": "input-" + name, "name": name, "bytes": r.ContentLength})
	})
	mux.HandleFunc("PUT /api/v1/x/vault/me/secrets/{name}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+hostedTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.vaultSets = append(f.vaultSets, vaultSet{Name: r.PathValue("name"), Body: string(body)})
		if f.vaultAnswer != nil {
			f.vaultAnswer(w, body)
			return
		}
		hostedJSON(w, http.StatusCreated, map[string]any{"name": r.PathValue("name")})
	})
	// A key saved for a source's host, recorded among the vault's sets as
	// "source-key:<host>".
	mux.HandleFunc("PUT /api/v1/x/vault/me/source-keys/{host}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+hostedTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.vaultSets = append(f.vaultSets, vaultSet{Name: "source-key:" + r.PathValue("host"), Body: string(body)})
		if f.vaultAnswer != nil {
			f.vaultAnswer(w, body)
			return
		}
		hostedJSON(w, http.StatusCreated, map[string]any{"host": r.PathValue("host")})
	})
	mux.HandleFunc("DELETE /api/v1/x/vault/me/secrets/{name}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.vaultDeletes = append(f.vaultDeletes, r.PathValue("name"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/x/runs/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		f.creates = append(f.creates, string(body))
		if f.refuse != nil {
			f.refuse(w)
			return
		}
		if f.onCreate != nil {
			f.onCreate()
		}
		var req hostedrun.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.created = &req
		f.run = &hostedrun.Run{
			ID: "22222222-2222-2222-2222-222222222222", Ref: req.Ref, Digest: "sha256:abc", Mode: "autonomous",
			Status: hostedrun.StatusQueued, QueuePosition: 3, HoldCredits: 20,
			Caps: hostedrun.Caps{TimeoutSeconds: 300, MaxTurns: 20}, CreatedAt: time.Now().UTC(),
		}
		if req.Mode == hostedrun.ModeInteractive {
			// A session holds nothing and carries its record.
			f.run.Mode, f.run.HoldCredits = hostedrun.ModeInteractive, 0
			f.run.Session = &hostedrun.Session{FreeMinutes: 10, CreditsPerMinute: 50, PaidThrough: 10, IdleSeconds: 300}
		}
		for _, id := range req.Knowledge {
			for _, p := range f.packs {
				if p["id"] == id {
					f.run.Knowledge = append(f.run.Knowledge, hostedrun.Attached{Pack: id, Name: p["name"].(string)})
				}
			}
		}
		hostedJSON(w, http.StatusCreated, f.served(f.run))
	})
	mux.HandleFunc("GET /api/v1/x/runs/", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		delay := f.listDelay
		f.mu.Unlock()
		time.Sleep(delay)
		f.mu.Lock()
		defer f.mu.Unlock()
		items := []map[string]any{}
		for _, o := range f.others {
			items = append(items, f.served(o))
		}
		if f.run != nil {
			items = append(items, f.served(f.run))
		}
		hostedJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/x/runs/{id}", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failGets > 0 {
			f.failGets--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		hostedJSON(w, http.StatusOK, f.served(f.run))
	})
	mux.HandleFunc("POST /api/v1/x/runs/{id}/cancel", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.cancelled++
		f.run.CancelRequested = true
		hostedJSON(w, http.StatusOK, f.served(f.run))
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
	// The session's stream, as the registry's session stream writes it: the
	// state whenever it changes, each frame after `after` once and in order,
	// and `done` once the run is over.
	mux.HandleFunc("GET /api/v1/x/runs/{id}/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+hostedTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		f.streamAfters = append(f.streamAfters, r.URL.Query().Get("after"))
		f.mu.Unlock()
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f.mu.Lock()
		class := f.streamError
		f.mu.Unlock()
		if class != "" {
			_, _ = io.WriteString(w, "event: error\ndata: {\"class\":\""+class+"\"}\n\n")
			return
		}
		fl, _ := w.(http.Flusher)
		lastState := ""
		for {
			f.mu.Lock()
			st, _ := json.Marshal(map[string]any{"status": f.run.Status, "startedAt": f.run.StartedAt, "session": f.run.Session})
			frames := slices.Clone(f.frames)
			over := f.run.Terminal()
			f.mu.Unlock()
			if string(st) != lastState {
				_, _ = io.WriteString(w, "event: state\ndata: "+string(st)+"\n\n")
				lastState = string(st)
			}
			for after < len(frames) {
				after++
				_, _ = io.WriteString(w, "id: "+strconv.Itoa(after)+"\nevent: frame\ndata: "+frames[after-1]+"\n\n")
			}
			if over {
				_, _ = io.WriteString(w, "event: done\ndata: {}\n\n")
				fl.Flush()
				return
			}
			fl.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	})
	for _, route := range []string{"turns", "files", "end"} {
		mux.HandleFunc("POST /api/v1/x/runs/{id}/session/"+route, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			defer f.mu.Unlock()
			if r.Header.Get("Authorization") != "Bearer "+hostedTestToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if f.sessionPosts == nil {
				f.sessionPosts = map[string][]string{}
			}
			f.sessionPosts[route] = append(f.sessionPosts[route], string(body))
			if f.sessionRefuse != nil && f.sessionRefuse(w, route) {
				return
			}
			w.WriteHeader(http.StatusAccepted)
		})
	}
	if f.routes != nil {
		f.routes(mux)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// frame adds one display event to the session's stream.
func (f *hostedFake) frame(ev string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = append(f.frames, ev)
}

// finish moves the fake's run to finished with a receipt and its outputs.
func (f *hostedFake) finish(outcome, exitClass string, settled int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	f.run.Status = hostedrun.StatusFinished
	f.run.Outcome, f.run.ExitClass, f.run.FinishedAt = outcome, exitClass, &now
	f.run.SettledCredits = &settled
	f.run.Usage = &hostedrun.Usage{Turns: 3, WallMS: 9000}
	// A pushed run: the registry keeps the host's cut of the CLI's stdout
	// envelope as the receipt; a run pushed before that (noReceipt) has
	// none.
	f.run.Receipt = hostedrun.Receipt{CapabilitiesEnforced: map[string]string{"runtime": "runsc: the agent container"}}
	for i, k := range f.run.Knowledge {
		// The host's CLI names each pack it mounted.
		f.run.Knowledge[i].Digest = "sha256:" + strings.Repeat("2b", 32)
		f.run.Receipt.CapabilitiesEnforced["knowledge"] = k.Name + " mounted read-only at /knowledge/" + k.Name + " (sha256:…, 4 pages, 6903 bytes, scan: a scan record this client has not confirmed with the registry); tools knowledge_list, knowledge_search and knowledge_read, on the tools container only"
	}
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

// served is a run as the fake's registry answers for it. Today's routes
// carry the usage's token counts and dollar cost and the caps' token
// ceiling, which this app no longer decodes; tokensGone leaves them out.
func (f *hostedFake) served(r *hostedrun.Run) map[string]any {
	b, _ := json.Marshal(r)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if f.tokensGone {
		return m
	}
	if caps, ok := m["caps"].(map[string]any); ok {
		caps["maxTokens"] = 20000
	}
	if u, ok := m["usage"].(map[string]any); ok {
		u["inputTokens"], u["outputTokens"], u["totalTokens"], u["costUsd"] = 800, 200, 1000, 0.0182
	}
	return m
}

func hostedJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// hostedManifestDoc is the resolve result's manifest — the typed struct's
// own JSON, which is what a real registry serves.
func hostedManifestDoc(t *testing.T, modes, secrets, targetHosts []string, browser string, intake ...string) map[string]any {
	t.Helper()
	y := "schemaVersion: \"2\"\nname: reporter\nnamespace: acme\nversion: 1.0.0\nsummary: \"hosted run fixture\"\n" +
		"modes: [" + strings.Join(modes, ", ") + "]\nentrypoints:\n  instruction: SKILL.md\ncapabilities:\n" +
		"  network:\n    egress: [api.example.com]\n"
	if len(secrets) > 0 {
		y += "  secrets: [" + strings.Join(secrets, ", ") + "]\n"
	}
	if len(targetHosts) > 0 {
		y += "  targetHosts: [" + strings.Join(targetHosts, ", ") + "]\n"
	}
	y += browser
	y += "publish:\n  include: [agent.yaml, SKILL.md]\n" + strings.Join(intake, "")
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
	// settings.TokenFor reads the OS keychain before the settings file, by
	// host and port: a token an earlier run left there for 127.0.0.1 and
	// this server's port would be read instead of this one. The keychain is
	// held in memory for the test.
	keyring.MockInit()
	home, runnerHome = t.TempDir(), t.TempDir()
	setUserHome(t, home)
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
		case eventHostedSessionEvent:
			if v, ok := data[0].(SessionEventView); ok {
				e.session = append(e.session, v)
			}
		case eventHostedSessionState:
			if v, ok := data[0].(HostedSessionStateView); ok {
				e.sessionStates = append(e.sessionStates, v)
			}
		}
	}
	t.Cleanup(func() { emitEvent = old })
	r := &RunAPI{app: &App{ctx: context.Background()}}
	// Cleanups run last-registered first, so this one runs before
	// emitEvent is put back: a follow still going when the test ends is
	// stopped and waited for, or its next report reaches the real Wails
	// runtime, which exits the test binary on a context that is not its
	// own. It does not run before a cleanup registered after it (fastPoll's):
	// a test whose follow may still be going when it ends calls fastPoll
	// first.
	t.Cleanup(func() {
		if cur := r.hostedCurrent(); cur != nil && cur.done != nil {
			cur.cancel()
			select {
			case <-cur.done:
			case <-time.After(10 * time.Second):
				t.Error("a run In Agentry's follow did not stop within 10s of the test's end")
			}
		}
	})
	return r, e
}

// emitted collects what the panel would have received: the run-log lines
// the local engine's view shows, and the hosted run's states.
type emitted struct {
	mu   sync.Mutex
	log  []string
	runs []*HostedRunView
	// session and sessionStates are a hosted session's events and states.
	session       []SessionEventView
	sessionStates []HostedSessionStateView
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
		{"a declared secret, no vault", func(f *hostedFake) { f.secrets = []string{"PAGERDUTY_TOKEN"} }, true, false, "keeps no secrets"},
		{"a declared secret, the vault and secret:write", func(f *hostedFake) {
			f.secrets, f.extensions = []string{"PAGERDUTY_TOKEN"}, []string{"billing", "runs", "vault"}
			f.scopes = []string{"agent:read", "user:profile", "run:read", "run:write", "secret:write"}
		}, true, true, ""},
		{"a declared secret, the vault, a token from before", func(f *hostedFake) {
			f.secrets, f.extensions = []string{"PAGERDUTY_TOKEN"}, []string{"billing", "runs", "vault"}
		}, true, false, "secret:write"},
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
			// reports one host and no worker.
			if v.RunnersOnline != 1 {
				t.Fatalf("runners online = %d, want the one host", v.RunnersOnline)
			}
			if v.HoldCredits != 20 || v.Tier != "standard" || v.Caps == nil || v.Caps.TimeoutSeconds != 300 || v.Caps.MaxTurns != 20 {
				t.Fatalf("the hold and caps come from the registry: %+v", v)
			}
			if !v.BalanceKnown || v.Balance != 40 {
				t.Fatalf("balance = %d (known %v)", v.Balance, v.BalanceKnown)
			}
			if v.MaxInputFiles != hostedrun.MaxInputFiles || v.MaxInputBytes != hostedrun.MaxInputBytes {
				t.Fatalf("the input limits are hostedrun's: %+v", v)
			}
		})
	}
}

// Nobody signed in: the mode is absent and the registry is asked nothing
// about runs at all.
func TestHostedAvailabilityWithoutSigningIn(t *testing.T) {
	f := &hostedFake{enabled: true}
	reg := f.start(t)
	setUserHome(t, t.TempDir())
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

// One In-Agentry run end to end through the bound surface: inputs uploaded,
// the run created with the intake answers, its states reported as it moves —
// queued, running, done with its outcome and credits — and the pushed run's
// output.tar unpacked into the chosen folder, the receipt read back; no log,
// no events route, no docker client anywhere near it.
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
	r.app.markPicked(inputDir, outDir) // what the native pickers record

	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "summarise INC-4821", []string{input}, outDir, map[string]string{"since": "yesterday"}, nil, false, false, nil)
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
	if final.Turns != 3 || final.WallMS != 9000 {
		t.Fatalf("usage = %+v", final)
	}
	if !final.Settled || final.SettledCredits != 2 || final.HoldCredits != 20 {
		t.Fatalf("credits = %+v", final)
	}
	// The receipt listed is the registry's, the host's cut of the CLI's
	// stdout, never the result.json in the outputs, which claims another
	// capability and a verified attestation: the agent could have written
	// it.
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
	// them, which is what makes it work where docker does not.
	for _, dir := range []string{filepath.Join(home, ".agentry"), runnerHome} {
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) > 0 {
			t.Fatalf("a hosted run wrote into %s: %v", dir, entries)
		}
	}
}

// A run the registry kept no receipt for (one pushed by an older host) lists
// none, and a result.json in its outputs — claiming a verified attestation —
// is not read as one.
func TestAPushedRunWithNoReceiptListsNone(t *testing.T) {
	f := &hostedFake{enabled: true, noReceipt: true, outputs: map[string]string{"result.json": hostedEnvelope}}
	reg := f.start(t)
	signIn(t, reg)
	r, events := newRunAPI(t)
	fastPoll(t)
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, false, false, nil); err != nil {
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
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, false, false, nil); err != nil {
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
	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out1, nil, nil, false, false, nil)
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
	// SettingsAPI.OpenUpgrade for it when a person clicks, so what opens has
	// been checked against the registry's own origin first.

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
	start, err = r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out2, nil, nil, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start.Refusal != "run-active" {
		t.Fatalf("refusal = %+v", start)
	}
}

// Reopening the panel with an active run reattaches to it rather than
// starting a second, and Cancel reaches the registry.
func TestAttachAndCancel(t *testing.T) {
	f := &hostedFake{enabled: true}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)

	// Nothing active yet.
	attachDir := t.TempDir()

	r.app.markPicked(attachDir) // the folder the panel picked

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
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, false, false, nil); err == nil ||
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

// remotePack is one pack in Agentry as the core's list answers it.
func remotePack(id, name, state string) map[string]any {
	p := map[string]any{
		"id": id, "name": name, "state": state, "bytes": 6903, "files": 9, "pages": 4,
		"digest":     "sha256:" + strings.Repeat("2b", 32),
		"wrappedKey": map[string]any{"kind": "certificate", "recipient": "serial:42", "key": "AAAA"},
		"createdAt":  "2026-09-26T13:15:18Z", "updatedAt": "2026-09-26T13:17:30Z",
	}
	if state == "ready" {
		p["scan"] = map[string]any{"schema": "agentry.knowledge-scan/v1", "stages": []any{}, "read": map[string]any{"sources": 3, "wiki": 6, "bytes": 6903, "notRead": []any{}},
			"review": map[string]any{"configured": true, "provider": "typesafe", "sourceChunks": 1, "wikiChunks": 1}, "findings": []any{}}
	}
	return p
}

// A run In Agentry attaches the account's packs kept in Agentry: the
// availability says the registry takes them, the create carries each by the
// registry's id, and the result lists what the run was given and the
// receipt's knowledge line. A pack that is not in Agentry, or not scanned
// there yet, is refused by name before anything is uploaded or created.
func TestAHostedRunAttachesPacksKeptInAgentry(t *testing.T) {
	const ready, scanning = "e1f8fcda-25b8-44df-9ebd-d27298fa5f0a", "4cb3f36e-213f-4423-bccb-055d505845e0"
	f := &hostedFake{
		enabled: true, attach: true,
		scopes:  []string{"agent:read", "user:profile", "run:read", "run:write", "knowledge:read", "knowledge:write"},
		packs:   []map[string]any{remotePack(ready, "runbook", "ready"), remotePack(scanning, "rescanned", "scanning")},
		outputs: map[string]string{"result.json": hostedEnvelope},
	}
	reg := f.start(t)
	signIn(t, reg)
	fastPoll(t) // before newRunAPI, so its follow stops before the poll is put back
	r, events := newRunAPI(t)
	outDir := t.TempDir()
	r.app.markPicked(outDir)

	v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !v.KnowledgeAttach || v.MaxPacks != 5 || v.KnowledgeNote != "" {
		t.Fatalf("availability = %+v", v)
	}

	for name, want := range map[string]string{"elsewhere": "not kept in Agentry", "rescanned": "not ready in Agentry"} {
		_, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, []string{name}, false, false, nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("attaching %s: %v, want %q", name, err, want)
		}
	}
	f.mu.Lock()
	created := f.created
	f.mu.Unlock()
	if created != nil {
		t.Fatalf("a refused pack still created a run: %+v", created)
	}

	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, []string{"runbook", "runbook"}, false, false, nil)
	if err != nil || start.Refusal != "" {
		t.Fatalf("start: %+v %v", start, err)
	}
	f.mu.Lock()
	created = f.created
	f.mu.Unlock()
	if !slices.Equal(created.Knowledge, []string{ready}) {
		t.Fatalf("the create's knowledge = %v, want the registry's id once", created.Knowledge)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 1)
	final := waitForFinal(t, events)
	if len(final.Knowledge) != 1 || final.Knowledge[0].Name != "runbook" {
		t.Fatalf("the run's packs = %+v", final.Knowledge)
	}
	if !strings.Contains(final.KnowledgeLine, "mounted read-only at /knowledge/runbook") {
		t.Fatalf("the knowledge line = %q", final.KnowledgeLine)
	}

	// A run with none sends none. The first run's views go, so the wait
	// below is for this run's end.
	f.mu.Lock()
	f.run, f.created = nil, nil
	f.mu.Unlock()
	events.mu.Lock()
	events.runs = nil
	events.mu.Unlock()
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	created = f.created
	f.mu.Unlock()
	if created == nil || created.Knowledge != nil {
		t.Fatalf("a run with no packs sent %+v", created)
	}
	// Let the follow end with the test, so it emits into nothing after.
	f.finish(hostedrun.OutcomeSucceeded, "ok", 1)
	waitForFinal(t, events)
}

// A token minted before the app asked for knowledge:read cannot attach:
// the availability says why, rather than offering a step the registry
// would refuse.
func TestHostedKnowledgeNeedsTheTokensKnowledgeRead(t *testing.T) {
	f := &hostedFake{enabled: true, attach: true}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.KnowledgeAttach || !strings.Contains(v.KnowledgeNote, "knowledge:read") {
		t.Fatalf("availability = %+v", v)
	}
}

// isolateHostedKeychain gives the test an in-memory OS keychain, which the
// CLI's sources code saves and reads a source's key in.
func isolateHostedKeychain(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
}

// A run In Agentry given an input source sends it: its answer in intake as
// every answer, and its token, the one typed for this run or the key saved
// on this computer for its host, in the create's sourceTokens alone, never
// in intake; a public source sends no token. A token the registry echoes in
// a refusal is not shown back.
func TestARunInAgentryGivenASourceSendsItsToken(t *testing.T) {
	isolateHostedKeychain(t)
	f := &hostedFake{enabled: true, modes: []string{"autonomous"},
		intake: "intake:\n  - id: codebase\n    prompt: \"The repository\"\n    type: repository\n    required: false\n"}
	reg := f.start(t)
	signIn(t, reg)
	// Each start is its own app's: a start waits for none in progress.
	fresh := func() (*RunAPI, string) {
		r, _ := newRunAPI(t)
		out := t.TempDir()
		r.app.markPicked(out)
		return r, out
	}
	r, out := fresh()
	const typed = "ghp-typed-for-this-hosted-run-77a1"
	const saved = "ghp-saved-on-this-computer-3c9e"

	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, map[string]string{"codebase": "https://github.com/acme/app"}, nil, false, false,
		map[string]string{"codebase": typed}); err != nil {
		t.Fatal(err)
	}
	if err := run.SaveSourceKey("repository", "https://github.com/acme/app", saved); err != nil {
		t.Fatal(err)
	}
	r, out = fresh()
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, map[string]string{"codebase": "https://github.com/acme/app"}, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	r, out = fresh()
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, map[string]string{"codebase": "https://gitlab.com/acme/public"}, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	creates := slices.Clone(f.creates)
	f.mu.Unlock()
	if len(creates) != 3 {
		t.Fatalf("creates %d", len(creates))
	}
	for i, want := range []string{`"sourceTokens":{"codebase":"` + typed + `"}`, `"sourceTokens":{"codebase":"` + saved + `"}`, ""} {
		var body struct {
			Intake       map[string]string `json:"intake"`
			SourceTokens map[string]string `json:"sourceTokens"`
		}
		if err := json.Unmarshal([]byte(creates[i]), &body); err != nil {
			t.Fatal(err)
		}
		if want == "" && body.SourceTokens != nil || want != "" && !strings.Contains(creates[i], want) {
			t.Fatalf("create %d: %s, want %s", i, creates[i], want)
		}
		for _, v := range body.Intake {
			if strings.Contains(v, typed) || strings.Contains(v, saved) {
				t.Fatalf("create %d carries a token in intake: %s", i, creates[i])
			}
		}
	}

	// A token for a question that is no answered source is refused before
	// anything is sent.
	r, out = fresh()
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, map[string]string{"codebase": " "}, nil, false, false, map[string]string{"codebase": typed}); err == nil {
		t.Fatal("a token for an unanswered source was sent")
	}

	// A refusal that echoes the token does not show it.
	f.mu.Lock()
	f.refuse = func(w http.ResponseWriter) {
		hostedJSON(w, http.StatusUnprocessableEntity, map[string]any{"type": "https://agentry.dev/problems/validation", "title": "Validation failed", "detail": "bad token " + typed})
	}
	f.mu.Unlock()
	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, map[string]string{"codebase": "https://github.com/acme/app"}, nil, false, false, map[string]string{"codebase": typed})
	if err != nil || start == nil {
		t.Fatalf("the refusal: %+v %v", start, err)
	}
	if b, _ := json.Marshal(start); strings.Contains(string(b), typed) {
		t.Fatalf("the refusal shows the token: %s", b)
	}
}

// A release that declares a browser is offered a run, a session and a
// schedule In Agentry, and its start sends the site as a plain answer: no
// source token, though a key is saved on this computer for its host, and an
// http address is taken (a site is not an https-only source).
func TestABrowserReleaseIsOfferedAndStartedInAgentry(t *testing.T) {
	isolateHostedKeychain(t)
	f := &hostedFake{
		enabled: true, modes: []string{"autonomous"},
		browser: "  browser:\n    sites: [site]\n",
		intake:  "intake:\n  - id: site\n    prompt: \"Which site should I test?\"\n    type: url\n    required: true\n",
	}
	reg := f.start(t)
	signIn(t, reg)
	if err := run.SaveSourceKey("url", "https://shop.example.com/", "ghp-saved-for-the-sites-host-5d1"); err != nil {
		t.Fatal(err)
	}
	r, _ := newRunAPI(t)
	v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !v.Offered || !v.Available || v.TargetHosts {
		t.Fatalf("availability = %+v", v)
	}
	for _, site := range []string{"https://shop.example.com/login", "http://shop.example.com/"} {
		r, _ := newRunAPI(t)
		out := t.TempDir()
		r.app.markPicked(out)
		if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "test it", nil, out, map[string]string{"site": site}, nil, false, false, nil); err != nil {
			t.Fatalf("start with %s: %v", site, err)
		}
	}
	f.mu.Lock()
	creates := slices.Clone(f.creates)
	f.mu.Unlock()
	if len(creates) != 2 {
		t.Fatalf("creates %d", len(creates))
	}
	for i, want := range []string{"https://shop.example.com/login", "http://shop.example.com/"} {
		var body struct {
			Intake       map[string]string `json:"intake"`
			SourceTokens map[string]string `json:"sourceTokens"`
		}
		if err := json.Unmarshal([]byte(creates[i]), &body); err != nil {
			t.Fatal(err)
		}
		if body.Intake["site"] != want || body.SourceTokens != nil || strings.Contains(creates[i], "ghp-saved") {
			t.Fatalf("create %d: %s", i, creates[i])
		}
	}
}

// A release that declares a target host is offered nothing In Agentry, and a
// start made anyway is refused before anything is uploaded or created.
func TestATargetHostReleaseIsNeitherOfferedNorStartedInAgentry(t *testing.T) {
	f := &hostedFake{
		enabled: true, targetHosts: []string{"server"},
		intake: "intake:\n  - id: server\n    prompt: \"The server\"\n    type: host\n    required: true\n",
	}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.Offered || v.Available || !v.TargetHosts || (v.Session != nil && v.Session.Offered) || (v.Schedule != nil && v.Schedule.Offered) {
		t.Fatalf("availability = %+v", v)
	}
	input := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(input, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	r.app.markPicked(filepath.Dir(input), out)
	_, err = r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", []string{input}, out, map[string]string{"server": "admin@203.0.113.4"}, nil, false, false, nil)
	if err == nil || !strings.Contains(err.Error(), "only a run on this computer") {
		t.Fatalf("start: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploads != 0 || len(f.creates) != 0 {
		t.Fatalf("uploads %d, creates %d", f.uploads, len(f.creates))
	}
}

// tokenRecorder stands in for the network: it fails every request and
// records the ones that carried a credential.
type tokenRecorder struct {
	mu   sync.Mutex
	sent []string
}

func (t *tokenRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Authorization") != "" {
		t.mu.Lock()
		t.sent = append(t.sent, req.Method+" "+req.URL.String())
		t.mu.Unlock()
	}
	return nil, io.ErrUnexpectedEOF
}

// The hosted-run bindings hold the page's registry URL to Settings' rule
// before a token is read: with a token in the keychain for a real host, an
// http:// spelling of it is refused by the availability and the starts, and
// the attaches follow nothing, with no request carrying the token anywhere.
func TestAHostedRunSendsNoTokenOverPlainHTTPToARealHost(t *testing.T) {
	keyring.MockInit()
	setUserHome(t, t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	if err := keyring.Set(settings.KeyringService, "dev.secureagentry.net", hostedTestToken); err != nil {
		t.Fatal(err)
	}
	rec := &tokenRecorder{}
	old := http.DefaultTransport
	http.DefaultTransport = rec
	t.Cleanup(func() { http.DefaultTransport = old })
	r, _ := newRunAPI(t)
	const reg = "http://dev.secureagentry.net"
	dir := t.TempDir()
	r.app.markPicked(dir)

	if v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("the availability over plain http: %+v %v", v, err)
	}
	if v, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "", nil, dir, nil, nil, false, false, nil); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("a run started over plain http: %+v %v", v, err)
	}
	if v, err := r.StartHostedSession(reg, "acme/reporter@1.0.0", "", nil, dir, nil, nil, false, false, nil); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("a session started over plain http: %+v %v", v, err)
	}
	if v, err := r.AttachHostedRun(reg, dir); err != nil || v.Run != nil {
		t.Fatalf("an attach over plain http: %+v %v", v, err)
	}
	if v, err := r.AttachHostedRunAtStart(reg); err != nil || v.Run != nil {
		t.Fatalf("an attach at start over plain http: %+v %v", v, err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.sent) != 0 {
		t.Fatalf("the token was sent: %v", rec.sent)
	}
}

// A run In Agentry never sends an answer to a secret question, whatever the
// page gives it: the registry would keep it with the run, so it refuses one.
// Every other answer goes. A refusal naming a question comes back as the
// view's question, never as an error.
func TestARunInAgentrySendsNoSecretAnswer(t *testing.T) {
	yes := true
	f := &hostedFake{enabled: true, modes: []string{"autonomous", "interactive"}, sessions: sessionTerms, sessionAllowed: &yes,
		intake: "intake:\n  - id: topic\n    prompt: \"Topic\"\n  - id: api_token\n    prompt: \"Your token\"\n    type: secret\n"}
	reg := f.start(t)
	signIn(t, reg)
	fastPoll(t)
	fastSessionRetry(t)
	out := t.TempDir()
	const secret = "s3cr3t-answer-never-sent-91c2"
	answers := map[string]string{"topic": "llamas", "api_token": secret}
	// Each start is its own app's: a start waits for none in progress.
	r, _ := newRunAPI(t)
	r.app.markPicked(out)
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, answers, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	r, _ = newRunAPI(t)
	r.app.markPicked(out)
	if _, err := r.StartHostedSession(reg, "acme/reporter@1.0.0", "go", nil, out, answers, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	creates := slices.Clone(f.creates)
	f.mu.Unlock()
	if len(creates) != 2 {
		t.Fatalf("creates %d", len(creates))
	}
	for i, c := range creates {
		var body struct {
			Intake map[string]string `json:"intake"`
		}
		if err := json.Unmarshal([]byte(c), &body); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(c, secret) || body.Intake["topic"] != "llamas" {
			t.Fatalf("create %d: %s", i, c)
		}
	}
	if answers["api_token"] != secret {
		t.Fatal("the caller's answers were changed")
	}

	// A release that cannot be read sends no answers at all: its secret
	// questions are not known.
	r, _ = newRunAPI(t)
	r.app.markPicked(out)
	if _, err := r.StartHostedRun(reg, "acme/missing@1.0.0", "go", nil, out, answers, nil, false, false, nil); err == nil || !strings.Contains(err.Error(), "answers were not sent") || strings.Contains(err.Error(), secret) {
		t.Fatalf("an unreadable release: %v", err)
	}
	f.mu.Lock()
	n := len(f.creates)
	f.mu.Unlock()
	if n != 2 {
		t.Fatalf("an unreadable release's answers were sent: %d creates", n)
	}

	f.mu.Lock()
	f.refuse = func(w http.ResponseWriter) {
		hostedJSON(w, http.StatusUnprocessableEntity, map[string]any{"type": "https://agentry.dev/problems/validation", "title": "Validation failed",
			"detail": "The answer to api_token is a secret.", "question": "api_token"})
	}
	f.mu.Unlock()
	r, _ = newRunAPI(t)
	r.app.markPicked(out)
	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, map[string]string{"topic": "llamas"}, nil, false, false, nil)
	if err != nil || start == nil || start.Refusal != "validation" || start.Question != "api_token" {
		t.Fatalf("the refusal: %+v %v", start, err)
	}
}
