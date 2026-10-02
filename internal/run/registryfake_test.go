package run

// A fake registry for this package's lifecycle tests: it serves one
// release (resolve + a real tar.zst archive built with pkg/archive) and
// 404s the attestation and signing-keys endpoints — the UNSIGNED
// provenance path (§8.8: no attestation recorded). Every route counts its
// hits so cache-reuse tests can prove "no re-download" instead of assuming
// it. Kept package-local rather than shared with internal/install on
// purpose: the install tests need DSSE signing fixtures this package's
// unsigned-path tests never touch, and a shared helper would couple the
// two suites to the union of both.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"

	"github.com/blaktron/agentry-desktop/internal/registry"
)

// runManifestYAML is the fixture release's agent.yaml: a runnable release
// declaring every capability class the compiler maps, plus one secret so
// profile resolution is observable (modeled on the CLI engine fixture).
const runManifestYAML = `schemaVersion: "2"
name: reporter
namespace: acme
modes: [autonomous]
version: 1.0.0
summary: "desktop run lifecycle fixture"
entrypoints:
  instruction: SKILL.md
capabilities:
  network:
    egress: [api.example.com]
  filesystem:
    read: workspace
    write: workspace
  exec: { commands: [echo] }
  secrets: [RUN_TEST_TOKEN]
  dataAccess: [code]
publish:
  include: [agent.yaml, SKILL.md]
`

// fakeRunRegistry is one test registry serving a single unsigned release.
type fakeRunRegistry struct {
	srv     *httptest.Server
	archive []byte
	digest  string

	// manifestDoc is the resolve result's manifest (defaults to the JSON
	// form of runManifestYAML).
	manifestDoc map[string]any
	namespace   string
	name        string
	version     string
	yanked      bool

	// corruptArchive flips one byte of the SERVED bytes, so the resolve
	// digest no longer matches what downloads.
	corruptArchive bool
	// attStatus is the attestation endpoint's status (default 404 —
	// unsigned; 500 exercises the unverifiable refusal).
	attStatus int
	// unscanned makes the resolve result say the version was admitted
	// without a scan (scanStatus skipped, no attestationUrl).
	unscanned bool
	// mcpScan, when set, is the answer to every declared server's
	// vulnerability lookup (/api/v1/mcp-servers/scan); unset, the route
	// 404s, which the runner reads as a server never seen.
	mcpScan map[string]any

	mu   sync.Mutex
	hits map[string]int
}

// hitCount returns how many requests a route has served.
func (f *fakeRunRegistry) hitCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

// count records one request to a route.
func (f *fakeRunRegistry) count(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hits == nil {
		f.hits = map[string]int{}
	}
	f.hits[path]++
}

// buildRunArchive produces a real deterministic tar.zst holding the given
// files, via the same pkg/archive builder every publish uses.
func buildRunArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	ws := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list, err := pkgarchive.WalkWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := pkgarchive.BuildArchive(ws, list, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// manifestDocFromYAML round-trips a manifest through the typed struct's
// own JSON encoding — exactly the document shape ManifestFromResolve
// re-decodes, so the fake serves what a real registry would.
func manifestDocFromYAML(t *testing.T, y string) map[string]any {
	t.Helper()
	m, err := manifest.Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
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

// startRunRegistry fills defaults, starts the server, and returns its URL.
func startRunRegistry(t *testing.T, f *fakeRunRegistry) string {
	t.Helper()
	if f.namespace == "" {
		f.namespace = "acme"
	}
	if f.name == "" {
		f.name = "reporter"
	}
	if f.version == "" {
		f.version = "1.0.0"
	}
	if f.manifestDoc == nil {
		f.manifestDoc = manifestDocFromYAML(t, runManifestYAML)
	}
	if f.archive == nil {
		f.archive = buildRunArchive(t, map[string]string{
			"agent.yaml": runManifestYAML,
			"SKILL.md":   "# Reporter\n\nWrite the report.\n",
		})
	}
	f.digest = registry.Digest(f.archive)
	if f.attStatus == 0 {
		f.attStatus = http.StatusNotFound
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/resolve", func(w http.ResponseWriter, _ *http.Request) {
		f.count("/api/v1/resolve")
		w.Header().Set("Content-Type", "application/json")
		doc := map[string]any{
			"archiveUrl": "/archive.tar.zst", "attestationUrl": "/att",
			"digest": f.digest, "manifest": f.manifestDoc,
			"namespace": f.namespace, "name": f.name, "version": f.version,
			"yanked": f.yanked, "scanStatus": "pipeline",
		}
		if f.unscanned {
			delete(doc, "attestationUrl")
			doc["scanStatus"] = "skipped"
		}
		_ = json.NewEncoder(w).Encode(doc)
	})
	mux.HandleFunc("/archive.tar.zst", func(w http.ResponseWriter, _ *http.Request) {
		f.count("/archive.tar.zst")
		data := f.archive
		if f.corruptArchive {
			data = append([]byte(nil), data...)
			data[len(data)/2] ^= 0xFF
		}
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/att", func(w http.ResponseWriter, _ *http.Request) {
		f.count("/att")
		w.WriteHeader(f.attStatus)
	})
	mux.HandleFunc("/.well-known/agentry/signing-keys.json", func(w http.ResponseWriter, _ *http.Request) {
		f.count("/.well-known/agentry/signing-keys.json")
		w.WriteHeader(http.StatusNotFound)
	})

	mux.HandleFunc("/api/v1/mcp-servers/scan", func(w http.ResponseWriter, _ *http.Request) {
		f.count("/api/v1/mcp-servers/scan")
		if f.mcpScan == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.mcpScan)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f.srv.URL
}

// isolateRunEnv points every state location a run touches at temp dirs:
// HOME (policy path, default out dir), AGENTRY_RUNNER_HOME (install cache
// + profiles), and AGENTRY_CONFIG_DIR (installed state, settings, pinned
// keys). Returns the runner home.
func isolateRunEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	return home
}

// withFakeDocker swaps the docker seam for a stand-in (a dockerx.Fake, or
// a client that fails as a stopped daemon does) for one test.
func withFakeDocker(t *testing.T, d dockerx.Client) {
	t.Helper()
	old := newDockerClient
	newDockerClient = func() dockerx.Client { return d }
	t.Cleanup(func() { newDockerClient = old })
}

// cataloguedPolicy is the default policy plus the one line the fixture
// needs: it declares exec: {commands: [echo]}, and the compiler refuses a
// command the policy does not catalogue (agentry-cli#100), so echo maps to
// busybox, the applet the runner image already has — the same line the
// CLI's own fixtures set.
func cataloguedPolicy() *policy.Policy {
	pol := policy.Default()
	pol.Exec.Packages = map[string]string{"echo": "busybox"}
	return pol
}

// saveCataloguedPolicy writes cataloguedPolicy into the isolated runner
// home, where Run reads the operator policy from.
func saveCataloguedPolicy(t *testing.T) {
	t.Helper()
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.Save(path, cataloguedPolicy()); err != nil {
		t.Fatal(err)
	}
}

// fixtureExecTag is the exec image the compiler names for the fixture
// under cataloguedPolicy: echo -> busybox, no mirror.
func fixtureExecTag() string {
	pol := cataloguedPolicy()
	return spec.ExecImageTag(runnerImageTag(pol), []string{"echo"}, []string{"busybox"}, pol.Exec.APKMirror)
}

// readyImagesFake returns a Fake whose image checks all pass for the
// default policy's pins plus this build's runner tag, so EnsureInstalled
// pulls and builds nothing; it also serves the package record the install
// step reads back from the fixture's exec image once the run has built it
// (agentry-cli#100) — that image is deliberately absent, so the build
// happens and the record is what the fake answers with afterwards.
func readyImagesFake() *dockerx.Fake {
	pol := cataloguedPolicy()
	return &dockerx.Fake{
		ExistingImages: map[string]bool{
			pol.Images.Gateway:            true,
			pol.Images.HarnessDockerAgent: true,
			runnerImageTag(pol):           true,
		},
		ImageFiles: map[string]string{fixtureExecTag() + ":" + gateway.ExecPackagesFile: "busybox\n"},
	}
}

// scriptHappyAgent scripts a clean run: probe passes, the agent makes two
// tool calls, answers, and reports usage (NDJSON shapes from the CLI
// engine tests).
func scriptHappyAgent(f *dockerx.Fake) {
	f.Foreground = map[string]*dockerx.FakeForeground{
		"-probe": {ExitCode: 0},
		"-agent": {Stdout: `{"type":"tool_call","tool":"fs_read"}` + "\n" +
			`{"type":"tool_call","tool":"write_report"}` + "\n" +
			`{"type":"agent_choice","message":{"content":"final answer"}}` + "\n" +
			`{"type":"stream_stopped","usage":{"input_tokens":100,"output_tokens":40,"cost":0.05}}` + "\n"},
	}
}
