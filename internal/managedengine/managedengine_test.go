package managedengine

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The pinned table names three files per architecture, each with a URL
// under its publisher and a 64-hex SHA-256, and nothing for another.
func TestThePinnedTableIsCompleteForBothArchitectures(t *testing.T) {
	for _, arch := range []string{"arm64", "amd64"} {
		list, err := assets(arch)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 3 {
			t.Fatalf("%s: %d assets, want 3", arch, len(list))
		}
		hosts := map[string]bool{}
		for _, a := range list {
			if len(a.SHA256) != 64 || strings.Trim(a.SHA256, "0123456789abcdef") != "" {
				t.Errorf("%s: %s has no SHA-256 sum: %q", arch, a.Name, a.SHA256)
			}
			if !strings.HasPrefix(a.URL, "https://") {
				t.Errorf("%s: %s is not fetched over https: %s", arch, a.Name, a.URL)
			}
			hosts[strings.SplitN(strings.TrimPrefix(a.URL, "https://"), "/", 2)[0]] = true
			if (a.Kind == "binary" || a.Kind == "docker-tgz") && a.Install == "" {
				t.Errorf("%s: %s installs as nothing", arch, a.Name)
			}
		}
		if !hosts["github.com"] || !hosts["download.docker.com"] {
			t.Errorf("%s: publishers %v, want GitHub's releases and Docker's downloads", arch, hosts)
		}
	}
	if _, err := assets("386"); err == nil {
		t.Error("an architecture the table does not cover was given assets")
	}
}

// macEngine is the engine as Default lays it out on macOS for an account.
func macEngine(account string) *Engine {
	home := "/Users/" + account
	return &Engine{Dir: home + "/Library/Application Support/agentry-desktop/engine", VMDir: home + "/" + vmDirName}
}

// The socket path rule: both of the engine's longest sockets, the Docker
// API's under Colima's home and ssh's control socket under Lima's, are
// held under what macOS can bind, and an engine where either is over is
// refused by name, before anything is downloaded or started.
func TestTheSocketPathIsHeldToTheLimit(t *testing.T) {
	e := macEngine("me")
	if err := e.CheckSocketPath(); err != nil {
		t.Errorf("a usual home refused: %v", err)
	}
	if got := e.Socket(); got != "/Users/me/.agentry-vm/colima/default/docker.sock" {
		t.Errorf("socket = %s", got)
	}
	if got := e.limaSocket(); got != "/Users/me/.agentry-vm/lima/colima/ssh.sock.1234567890123456" {
		t.Errorf("Lima's socket = %s", got)
	}
	// A long account name still fits.
	if err := macEngine(strings.Repeat("n", 40)).CheckSocketPath(); err != nil {
		t.Errorf("a 40-character account refused: %v", err)
	}
	// Lima's home under the engine directory, under Application Support,
	// is over the limit for every account: refused, naming Lima's socket.
	nested := &Engine{Dir: "/Users/me/Library/Application Support/agentry-desktop/engine"}
	err := nested.CheckSocketPath()
	if err == nil || !strings.Contains(err.Error(), "ssh.sock") || !strings.Contains(err.Error(), "103") {
		t.Errorf("Lima's sockets under Application Support: %v", err)
	}
	// Exactly at the limit is over it: the path and its NUL must fit.
	at := &Engine{Dir: "/e/engine", VMDir: "/" + strings.Repeat("v", maxSocketPath-len("/x/lima/colima/"+limaSSHSocket)) + "/x/" + vmDirName}
	if n := len(at.limaSocket()); n < maxSocketPath {
		t.Fatalf("the test's path is %d bytes, want at least %d", n, maxSocketPath)
	}
	if err := at.CheckSocketPath(); err == nil {
		t.Errorf("a %d-byte socket path accepted", len(at.limaSocket()))
	}
	long := macEngine(strings.Repeat("n", 60))
	if err := long.CheckSocketPath(); err == nil {
		t.Error("a 60-character account accepted")
	}
	if err := long.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "103") {
		t.Errorf("Install did not refuse the over-long home first: %v", err)
	}
	if err := long.Start(context.Background()); err == nil {
		t.Error("Start did not refuse the over-long home")
	}
}

// A test publisher serving the three files, with the sums the table must
// carry for them; the table is rewritten for the test.
func testPublisher(t *testing.T, tamper string) (*Engine, func()) {
	t.Helper()
	colima := []byte("#!/bin/sh\necho colima \"$@\"\n")
	// Lima's tarball unpacks at its root: ./bin, ./libexec, ./share.
	limaTar := tarGz(t, map[string]tarEntry{
		"./bin/limactl":                        {body: "#!/bin/sh\necho limactl\n", mode: 0o755},
		"./libexec/lima/lima-driver-x":         {body: "driver", mode: 0o755},
		"./share/lima/lima-guestagent.Linux-x": {body: "agent", mode: 0o644},
		"./share/doc/lima/README":              {body: "doc", mode: 0o644},
		"./share/doc/lima/templates":           {link: "../../lima/templates"},
		"./README-at-root":                     {body: "x", mode: 0o644},
	})
	dockerTar := tarGz(t, map[string]tarEntry{
		"docker/docker":        {body: "#!/bin/sh\necho docker\n", mode: 0o755},
		"docker/other":         {body: "x", mode: 0o644},
		"docker/../../escape":  {body: "x", mode: 0o644},
		"../../../etc/escaped": {body: "x", mode: 0o644},
	})
	files := map[string][]byte{"/colima": colima, "/lima.tar.gz": limaTar, "/docker.tgz": dockerTar}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == tamper {
			b = append([]byte("x"), b...)
		}
		_, _ = w.Write(b)
	}))
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	saved := pinnedAssets
	pinnedAssets = func(string) ([]Asset, error) {
		return []Asset{
			{Name: "colima", URL: srv.URL + "/colima", SHA256: sum(colima), Kind: "binary", Install: "colima"},
			{Name: "lima.tar.gz", URL: srv.URL + "/lima.tar.gz", SHA256: sum(limaTar), Kind: "tar.gz"},
			{Name: "docker.tgz", URL: srv.URL + "/docker.tgz", SHA256: sum(dockerTar), Kind: "docker-tgz", Install: "docker"},
		}, nil
	}
	e := &Engine{Dir: filepath.Join(shortTempDir(t), "engine"), Client: srv.Client(), Log: &bytes.Buffer{}}
	return e, func() { srv.Close(); pinnedAssets = saved }
}

// shortTempDir is a temporary directory whose path keeps the engine's
// socket under what macOS can bind: the test runner's own temporary
// directory lives under /var/folders there, which is too long.
func shortTempDir(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "agentry-me-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

type tarEntry struct {
	body string
	mode int64
	link string
}

func tarGz(t *testing.T, entries map[string]tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, en := range entries {
		if en.link != "" {
			if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeSymlink, Linkname: en.link}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: en.mode, Size: int64(len(en.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(en.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The install lands the three binaries executable, Lima's share directory
// beside them, nothing an archive entry would put elsewhere, and the
// record; Installed and Current then say so.
func TestInstallPutsTheBinariesInPlace(t *testing.T) {
	e, done := testPublisher(t, "")
	defer done()
	if e.Installed() {
		t.Fatal("installed before the install")
	}
	if err := e.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"bin/colima", "bin/limactl", "bin/docker"} {
		fi, err := os.Stat(filepath.Join(e.Dir, p))
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if fi.Mode()&0o111 == 0 {
			t.Errorf("%s is not executable: %v", p, fi.Mode())
		}
	}
	for _, p := range []string{"share/lima/lima-guestagent.Linux-x", "libexec/lima/lima-driver-x", "share/doc/lima/README"} {
		if _, err := os.Stat(filepath.Join(e.Dir, p)); err != nil {
			t.Errorf("Lima's %s: %v", p, err)
		}
	}
	for _, p := range []string{"share/doc/lima/templates", "README-at-root", "docker/other", "escape", "etc/escaped", "docker"} {
		if _, err := os.Stat(filepath.Join(e.Dir, p)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s landed: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Dir, vmDirName, "colima")); err != nil {
		t.Errorf("colima's home was not made: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(e.DockerConfigDir(), "config.json")); err != nil || strings.TrimSpace(string(b)) != "{}" {
		t.Errorf("the engine's own docker configuration: %q, %v", b, err)
	}
	entries, _ := os.ReadDir(e.Dir)
	for _, en := range entries {
		if strings.HasPrefix(en.Name(), "download-") {
			t.Errorf("the download directory %s was left", en.Name())
		}
	}
	if !e.Installed() || !e.Current() {
		t.Error("Installed/Current false after the install")
	}
	// A record from an older app: installed, not current.
	if err := os.WriteFile(filepath.Join(e.Dir, recordFile), []byte(`{"colima":"0.9.0","lima":"2.0.0","docker":"28.0.0","arch":"`+runtime.GOARCH+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !e.Installed() || e.Current() {
		t.Error("an older install should be installed and not current")
	}
}

// A file whose bytes do not hash to the pinned sum is refused, and nothing
// lands: not the file, not the others, not the record.
func TestATamperedDownloadIsRefused(t *testing.T) {
	for _, tamper := range []string{"/colima", "/lima.tar.gz", "/docker.tgz"} {
		e, done := testPublisher(t, tamper)
		err := e.Install(context.Background())
		done()
		if err == nil || !strings.Contains(err.Error(), "does not match its pinned SHA-256") {
			t.Fatalf("%s tampered: %v", tamper, err)
		}
		if _, err := os.Stat(filepath.Join(e.Dir, "bin")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s tampered: bin/ landed", tamper)
		}
		if _, err := os.Stat(filepath.Join(e.Dir, recordFile)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s tampered: the record was written", tamper)
		}
		if e.Installed() {
			t.Errorf("%s tampered: Installed", tamper)
		}
	}
}

// The commands: colima is run from the engine's own bin with COLIMA_HOME
// and LIMA_HOME under the VM directory and that bin first on PATH, the
// person's docker variables kept out; start waits for the socket, stop and
// remove run their commands, and Remove leaves nothing.
func TestTheEngineIsDrivenUnderItsOwnHome(t *testing.T) {
	e, done := testPublisher(t, "")
	defer done()
	e.VMDir = filepath.Join(shortTempDir(t), vmDirName)
	if err := e.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	var calls []string
	var envs [][]string
	e.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		calls = append(calls, filepath.Base(name)+" "+strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, "true")
		return cmd
	}
	pings := 0
	e.Ping = func(_ context.Context, socket string) error {
		pings++
		if socket != e.Socket() {
			t.Errorf("pinged %s, want %s", socket, e.Socket())
		}
		if pings < 2 {
			return errors.New("not yet")
		}
		return nil
	}
	t.Setenv("DOCKER_HOST", "unix:///somewhere/else.sock")
	t.Setenv("DOCKER_CONFIG", "/elsewhere/.docker")
	t.Setenv("COLIMA_HOME", "/elsewhere")
	t.Setenv("LIMA_HOME", "/elsewhere/lima")
	// The environment is built by env(); capture it through a command.
	cmd := e.command(context.Background(), "status")
	for _, kv := range cmd.Env {
		envs = append(envs, strings.SplitN(kv, "=", 2))
	}
	var sawHome, sawLima, sawPath, sawConfig bool
	for _, kv := range envs {
		switch kv[0] {
		case "DOCKER_CONFIG":
			sawConfig = true
			if kv[1] != e.DockerConfigDir() {
				t.Errorf("DOCKER_CONFIG = %s", kv[1])
			}
		case "COLIMA_HOME":
			sawHome = true
			if kv[1] != filepath.Join(e.VMDir, "colima") {
				t.Errorf("COLIMA_HOME = %s", kv[1])
			}
		case "LIMA_HOME":
			sawLima = true
			if kv[1] != filepath.Join(e.VMDir, "lima") {
				t.Errorf("LIMA_HOME = %s", kv[1])
			}
		case "PATH":
			sawPath = true
			if !strings.HasPrefix(kv[1], filepath.Join(e.Dir, "bin")+string(os.PathListSeparator)) {
				t.Errorf("PATH does not lead with the engine's bin: %s", kv[1])
			}
		case "COLIMA_CACHE_HOME":
			if kv[1] != filepath.Join(e.Dir, "cache") {
				t.Errorf("COLIMA_CACHE_HOME = %s", kv[1])
			}
		case "DOCKER_HOST", "DOCKER_CONTEXT":
			t.Errorf("%s reached the engine's command", kv[0])
		}
	}
	if !sawHome || !sawLima || !sawPath || !sawConfig {
		t.Error("COLIMA_HOME, LIMA_HOME, DOCKER_CONFIG or PATH missing from the command's environment")
	}
	if cmd.Path != "true" && filepath.Base(cmd.Path) != "true" {
		t.Errorf("the stand-in was not used: %s", cmd.Path)
	}
	calls = nil
	if err := e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "colima start default --vm-type vz --mount-type virtiofs --activate=false --save-config=false") {
		t.Errorf("start ran %q", calls)
	}
	if pings != 2 {
		t.Errorf("pinged %d times, want until it answered (2)", pings)
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !e.Running(context.Background()) {
		t.Error("Running false with a status that exits 0")
	}
	if err := e.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the engine directory was kept: %v", err)
	}
	if _, err := os.Stat(e.VMDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the VM directory was kept: %v", err)
	}
	want := []string{"colima stop default", "colima status default", "colima delete default --force --data"}
	if len(calls) != 4 || calls[1] != want[0] || calls[2] != want[1] || calls[3] != want[2] {
		t.Errorf("calls = %q", calls)
	}
	// Not installed: start refuses, stop and running say so, remove is fine.
	if err := e.Start(context.Background()); err == nil {
		t.Error("Start on nothing installed")
	}
	if e.Running(context.Background()) {
		t.Error("Running on nothing installed")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Errorf("Stop on nothing installed: %v", err)
	}
}

// Default is under the application support directory, and Supported is
// true only on macOS.
func TestDefaultIsUnderTheAppsOwnDirectory(t *testing.T) {
	e, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(e.Dir, filepath.Join("agentry-desktop", "engine")) {
		t.Errorf("Default dir = %s", e.Dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if e.VMDir != filepath.Join(home, ".agentry-vm") {
		t.Errorf("Default VM dir = %s", e.VMDir)
	}
	if Supported() != (runtime.GOOS == "darwin") {
		t.Errorf("Supported() = %v on %s", Supported(), runtime.GOOS)
	}
}

// Remove deletes only directories the engine names as its own: the engine
// directory and the VM directory, each by its base name.
func TestRemoveRefusesDirectoriesNotTheEngines(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "person")
	if err := os.MkdirAll(keep, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, e := range []*Engine{
		{Dir: keep, VMDir: filepath.Join(root, vmDirName)},
		{Dir: filepath.Join(root, "engine"), VMDir: keep},
		{Dir: filepath.Join(root, "engine"), VMDir: root},
	} {
		if err := e.Remove(context.Background()); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("Remove(%s, %s) = %v, want a refusal", e.Dir, e.VMDir, err)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("a refused Remove touched %s: %v", keep, err)
	}
}
