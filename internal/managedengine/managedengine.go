// Package managedengine is the container engine the app installs and runs
// itself on macOS when the computer offers none: Colima's, Lima's and the
// docker CLI's release binaries, pinned by version and SHA-256 sum, under
// the app's own directory, with no Homebrew and no administrator password.
// Colima runs a Docker Engine in a Lima virtual machine; the app starts it
// when it opens and stops it when it quits, drives it by its socket and
// never touches the person's own docker context.
package managedengine

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Versions of the three binaries the engine is made of. A bump changes the
// version, the sums and nothing else; the app installed earlier re-downloads
// at its next start when they differ from what installed.json records.
const (
	ColimaVersion = "0.10.3"
	LimaVersion   = "2.2.1"
	DockerVersion = "29.9.1"
)

// Asset is one release file: where it comes from, what it must hash to,
// and what it unpacks or installs as under the engine directory.
type Asset struct {
	Name   string
	URL    string
	SHA256 string
	// Kind says how the file lands: "binary" (one executable, installed as
	// bin/<Install>), "tar.gz" (unpacked under the engine directory as it
	// is: Lima's bin/ and share/), "docker-tgz" (Docker's static tarball,
	// whose docker/docker becomes bin/docker).
	Kind    string
	Install string
}

// assets lists the files for an architecture ("arm64" or "amd64"), each
// with the sum its publisher states, or, for Docker's static CLI, which
// publishes none, the sum of the file as fetched and recorded here.
func assets(goarch string) ([]Asset, error) {
	switch goarch {
	case "arm64":
		return []Asset{
			{Name: "colima-Darwin-arm64", URL: "https://github.com/abiosoft/colima/releases/download/v" + ColimaVersion + "/colima-Darwin-arm64",
				SHA256: "980ad8bf61a4ca370243f4cb41401a61276dcd2c2502bee7b9b86f9250169f34", Kind: "binary", Install: "colima"},
			{Name: "lima-" + LimaVersion + "-Darwin-arm64.tar.gz", URL: "https://github.com/lima-vm/lima/releases/download/v" + LimaVersion + "/lima-" + LimaVersion + "-Darwin-arm64.tar.gz",
				SHA256: "9e9eacce88f37e185c346bad73aa6136f738d8cdf8c3bb23cd42b071824bc66e", Kind: "tar.gz"},
			{Name: "docker-" + DockerVersion + "-aarch64.tgz", URL: "https://download.docker.com/mac/static/stable/aarch64/docker-" + DockerVersion + ".tgz",
				SHA256: "7f2c1637313578046e7dd61153e4e4cd1188d83a1e4b71825ab914776c208460", Kind: "docker-tgz", Install: "docker"},
		}, nil
	case "amd64":
		return []Asset{
			{Name: "colima-Darwin-x86_64", URL: "https://github.com/abiosoft/colima/releases/download/v" + ColimaVersion + "/colima-Darwin-x86_64",
				SHA256: "3082737fe8a98afda11cba7d9a20b6e56fe80c6153464beda04bec630758770b", Kind: "binary", Install: "colima"},
			{Name: "lima-" + LimaVersion + "-Darwin-x86_64.tar.gz", URL: "https://github.com/lima-vm/lima/releases/download/v" + LimaVersion + "/lima-" + LimaVersion + "-Darwin-x86_64.tar.gz",
				SHA256: "6b776e4bc41f358e6ee01fefcc78e2eb26f11635a0fcff632234af77a566c6c4", Kind: "tar.gz"},
			{Name: "docker-" + DockerVersion + "-x86_64.tgz", URL: "https://download.docker.com/mac/static/stable/x86_64/docker-" + DockerVersion + ".tgz",
				SHA256: "9fc063bfae58ecf3c46e7528beaad9814968f7e624e9ba04a7f5cba5becad26c", Kind: "docker-tgz", Install: "docker"},
		}, nil
	}
	return nil, fmt.Errorf("no managed engine for %s", goarch)
}

// pinnedAssets is assets; a test points it at a publisher of its own.
var pinnedAssets = assets

// Assets is the pinned file list for this build's architecture.
func Assets() ([]Asset, error) { return pinnedAssets(runtime.GOARCH) }

// Licences names each binary's licence, for the card that offers the click.
const Licences = "Colima (MIT), Lima (Apache-2.0) and the docker command (Apache-2.0)"

// DownloadMB is about what the three files weigh together, for the card
// and the consent: 73 MB on Apple silicon, 63 MB on Intel, as fetched on
// 2026-10-16.
const DownloadMB = 75

// recordFile records what installed, at the engine directory's root.
const recordFile = "installed.json"

// record is installed.json.
type record struct {
	Colima string `json:"colima"`
	Lima   string `json:"lima"`
	Docker string `json:"docker"`
	Arch   string `json:"arch"`
}

func pinned() record {
	return record{Colima: ColimaVersion, Lima: LimaVersion, Docker: DockerVersion, Arch: runtime.GOARCH}
}

// Profile is the Colima profile the app runs; its socket is
// <colima home>/<profile>/docker.sock.
const Profile = "default"

// maxSocketPath is the size of a Unix socket address's path on macOS,
// its terminating NUL included: a path must be shorter than this.
const maxSocketPath = 104

// limaInstance is the Lima instance Colima runs its default profile as.
const limaInstance = "colima"

// limaSSHSocket is the longest socket Lima opens in an instance's
// directory: ssh's control socket, while ssh is connecting, carries a
// 16-character suffix. Lima refuses to create an instance whose directory
// would put this path at or over the limit, so it is what the limit is
// measured on.
const limaSSHSocket = "ssh.sock.1234567890123456"

// vmDirName is the directory the VM's state lives in: Colima's home
// (colima/, with the Docker API's socket) and Lima's (lima/, with the VM's
// disk and sockets). It is kept out of the engine directory, whose path
// under Application Support is too long for those sockets, as a directory
// of its own in the home directory.
const vmDirName = ".agentry-vm"

// Engine is the managed engine at one directory.
type Engine struct {
	// Dir is the engine directory: bin/ (colima, limactl, docker),
	// libexec/ and share/ (Lima's), cache/ (Colima's), docker-config/ and
	// installed.json.
	Dir string
	// VMDir holds the VM's state: colima/ (Colima's home) and lima/
	// (Lima's). Its base name is always vmDirName; empty is that name
	// under Dir.
	VMDir string
	// Client fetches the assets; nil is http.DefaultClient with a timeout.
	Client *http.Client
	// Command builds the commands run (colima start, stop, status, delete);
	// a test hands a stand-in. nil is exec.CommandContext.
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd
	// Ping asks the Docker API on a socket for /_ping; a test stubs it.
	Ping func(ctx context.Context, socket string) error
	// Log receives the install's and the engine's progress lines.
	Log io.Writer
}

// Default is the engine under the app's own directory,
// ~/Library/Application Support/agentry-desktop/engine on macOS, with the
// VM's state at ~/.agentry-vm, a path short enough for its sockets.
func Default() (*Engine, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("the application support directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("the home directory: %w", err)
	}
	return &Engine{Dir: filepath.Join(base, "agentry-desktop", "engine"), VMDir: filepath.Join(home, vmDirName)}, nil
}

// Supported says whether this build can manage an engine at all: macOS,
// on an architecture the pinned files cover.
func Supported() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := Assets()
	return err == nil
}

// Paths under the engine directory.
func (e *Engine) binDir() string     { return filepath.Join(e.Dir, "bin") }
func (e *Engine) colimaHome() string { return filepath.Join(e.vmDir(), "colima") }
func (e *Engine) limaHome() string   { return filepath.Join(e.vmDir(), "lima") }
func (e *Engine) colima() string     { return filepath.Join(e.binDir(), "colima") }

// VMDirPath is where the VM keeps its state: its disk, Colima's and Lima's
// homes.
func (e *Engine) VMDirPath() string { return e.vmDir() }

// vmDir is VMDir, or vmDirName under Dir.
func (e *Engine) vmDir() string {
	if e.VMDir != "" {
		return e.VMDir
	}
	return filepath.Join(e.Dir, vmDirName)
}

// limaSocket is the longest socket path Lima opens for the engine's VM.
func (e *Engine) limaSocket() string {
	return filepath.Join(e.limaHome(), limaInstance, limaSSHSocket)
}

// Docker is the docker CLI the engine is driven with.
func (e *Engine) Docker() string { return filepath.Join(e.binDir(), "docker") }

// DockerConfigDir is the docker configuration the engine's command reads
// (DOCKER_CONFIG): the engine's own, empty, so the person's config.json,
// whose credential helper may be another engine's (Docker Desktop's
// `desktop`, which fails every pull once that app is gone), is never
// consulted, and the context Colima writes lands here, not in theirs.
func (e *Engine) DockerConfigDir() string { return filepath.Join(e.Dir, "docker-config") }

// Socket is where the engine serves the Docker API on this computer.
func (e *Engine) Socket() string {
	return filepath.Join(e.colimaHome(), Profile, "docker.sock")
}

// Endpoint is the socket as a docker endpoint.
func (e *Engine) Endpoint() string { return "unix://" + e.Socket() }

// Installed says whether the three binaries are in place and installed.json
// records them.
func (e *Engine) Installed() bool {
	_, err := e.readRecord()
	if err != nil {
		return false
	}
	for _, b := range []string{e.colima(), filepath.Join(e.binDir(), "limactl"), e.Docker()} {
		if fi, err := os.Stat(b); err != nil || fi.Mode()&0o111 == 0 {
			return false
		}
	}
	return true
}

// Current says whether what installed is the pinned set; false when the
// versions moved with the app, so the next start re-downloads.
func (e *Engine) Current() bool {
	rec, err := e.readRecord()
	return err == nil && rec == pinned()
}

func (e *Engine) readRecord() (record, error) {
	b, err := os.ReadFile(filepath.Join(e.Dir, recordFile)) //nolint:gosec // a fixed name under the app's own directory
	if err != nil {
		return record{}, err
	}
	var rec record
	if err := json.Unmarshal(b, &rec); err != nil {
		return record{}, err
	}
	return rec, nil
}

// CheckSocketPath refuses an engine whose sockets macOS could not bind,
// naming the path, before anything is downloaded: the Docker API's socket
// under Colima's home and the longest of Lima's under its own.
func (e *Engine) CheckSocketPath() error {
	for _, p := range []string{e.Socket(), e.limaSocket()} {
		if n := len(p); n >= maxSocketPath {
			return fmt.Errorf("the engine's socket path %s is %d bytes, and macOS allows %d at most; the app cannot run an engine under this home directory", p, n, maxSocketPath-1)
		}
	}
	return nil
}

// Install downloads the pinned files, refuses any whose sum differs, and
// puts the binaries in place. It may be run again over an older install:
// the files are replaced and installed.json rewritten. Nothing is run.
func (e *Engine) Install(ctx context.Context) error {
	if err := e.CheckSocketPath(); err != nil {
		return err
	}
	list, err := Assets()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.Dir, 0o700); err != nil {
		return fmt.Errorf("the engine directory: %w", err)
	}
	tmp, err := os.MkdirTemp(e.Dir, "download-")
	if err != nil {
		return fmt.Errorf("the download directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for _, a := range list {
		path := filepath.Join(tmp, a.Name)
		e.logf("Downloading %s…", a.Name)
		if err := e.fetch(ctx, a, path); err != nil {
			return err
		}
	}
	// Every file verified before any lands, so a refusal leaves the
	// install as it was. The record goes first: a copy that fails partway
	// (an update interrupted) then reads as not installed, never as a
	// mixed set of versions.
	if err := os.Remove(filepath.Join(e.Dir, recordFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace the install's record: %w", err)
	}
	if err := os.MkdirAll(e.binDir(), 0o700); err != nil {
		return fmt.Errorf("the engine's bin directory: %w", err)
	}
	for _, a := range list {
		path := filepath.Join(tmp, a.Name)
		switch a.Kind {
		case "binary":
			if err := install(path, filepath.Join(e.binDir(), a.Install)); err != nil {
				return err
			}
		case "tar.gz":
			if err := untar(path, e.Dir, func(name string) (string, bool) {
				// Lima's bin/, libexec/ and share/ as they are (limactl finds
				// its drivers and guest agents beside its own binary),
				// nothing else.
				if strings.HasPrefix(name, "bin/") || strings.HasPrefix(name, "libexec/") || strings.HasPrefix(name, "share/") {
					return name, true
				}
				return "", false
			}); err != nil {
				return fmt.Errorf("unpack %s: %w", a.Name, err)
			}
		case "docker-tgz":
			if err := untar(path, e.Dir, func(name string) (string, bool) {
				if name == "docker/docker" {
					return "bin/" + a.Install, true
				}
				return "", false
			}); err != nil {
				return fmt.Errorf("unpack %s: %w", a.Name, err)
			}
		default:
			return fmt.Errorf("asset %s: unknown kind %q", a.Name, a.Kind)
		}
	}
	// Colima honours COLIMA_HOME only once the directory exists.
	if err := os.MkdirAll(e.colimaHome(), 0o700); err != nil {
		return fmt.Errorf("colima's home: %w", err)
	}
	if err := e.writeDockerConfig(); err != nil {
		return err
	}
	b, _ := json.Marshal(pinned())
	if err := os.WriteFile(filepath.Join(e.Dir, recordFile), append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("record the install: %w", err)
	}
	e.logf("Installed Colima %s, Lima %s and the docker command %s under %s.", ColimaVersion, LimaVersion, DockerVersion, e.Dir)
	return nil
}

// fetch downloads one asset to path and refuses it unless its bytes hash
// to the pinned sum.
func (e *Engine) fetch(ctx context.Context, a Asset, path string) error {
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", a.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s answered %s", a.Name, a.URL, resp.Status)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // a fresh file in the download directory
	if err != nil {
		return err
	}
	h := sha256.New()
	_, cerr := io.Copy(io.MultiWriter(f, h), resp.Body)
	if err := f.Close(); err != nil && cerr == nil {
		cerr = err
	}
	if cerr != nil {
		return fmt.Errorf("download %s: %w", a.Name, cerr)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
		_ = os.Remove(path)
		return fmt.Errorf("%s does not match its pinned SHA-256 (got %s, want %s): refused, nothing installed", a.Name, got, a.SHA256)
	}
	return nil
}

// install puts one verified binary in place, executable.
func install(src, dst string) error {
	b, err := os.ReadFile(src) //nolint:gosec // a file this install just wrote
	if err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, b, 0o700); err != nil { //nolint:gosec // an executable the app runs
		return err
	}
	return os.Rename(tmp, dst)
}

// untar unpacks the regular files of a gzipped tar into dir, each at the
// path place gives it (false skips the entry), never outside dir, keeping
// the executable bit. Symlinks and anything else are skipped.
func untar(path, dir string, place func(name string) (string, bool)) error {
	f, err := os.Open(path) //nolint:gosec // a file this install just wrote
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	root := filepath.Clean(dir)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+h.Name)), "/")
		rel, ok := place(name)
		if !ok {
			continue
		}
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if dst != root && !strings.HasPrefix(dst, root+string(filepath.Separator)) {
			return fmt.Errorf("entry %q leaves the engine directory", h.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		mode := os.FileMode(0o600)
		if h.Mode&0o111 != 0 {
			mode = 0o700
		}
		out, err := os.OpenFile(dst+".new", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode) //nolint:gosec // under the engine directory, checked above
		if err != nil {
			return err
		}
		_, cerr := io.Copy(out, tr) //nolint:gosec // the archive is pinned by sum
		if err := out.Close(); err != nil && cerr == nil {
			cerr = err
		}
		if cerr != nil {
			return cerr
		}
		if err := os.Rename(dst+".new", dst); err != nil {
			return err
		}
	}
}

// writeDockerConfig makes the engine's own docker configuration directory
// with an empty config.json, when it is not there.
func (e *Engine) writeDockerConfig() error {
	if err := os.MkdirAll(e.DockerConfigDir(), 0o700); err != nil {
		return fmt.Errorf("the engine's docker configuration: %w", err)
	}
	path := filepath.Join(e.DockerConfigDir(), "config.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		return fmt.Errorf("the engine's docker configuration: %w", err)
	}
	return nil
}

// env is the environment the engine's commands run with: Colima's home
// and cache under the engine directory (Lima's under the home), the
// engine's own docker configuration, and the engine's own binaries first
// on PATH, so colima finds its limactl and docker.
func (e *Engine) env() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "COLIMA_HOME", "COLIMA_CACHE_HOME", "LIMA_HOME", "PATH", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG":
			continue
		}
		out = append(out, kv)
	}
	// Colima's cache (the VM's image, a few hundred MB) under the engine
	// directory too, so Remove leaves nothing in the person's caches; an
	// update keeps it, a fresh install fetches it again. Lima's home is
	// named beside Colima's rather than left under it as _lima, which
	// would lengthen its sockets' paths.
	return append(out,
		"COLIMA_HOME="+e.colimaHome(),
		"LIMA_HOME="+e.limaHome(),
		"COLIMA_CACHE_HOME="+filepath.Join(e.Dir, "cache"),
		"DOCKER_CONFIG="+e.DockerConfigDir(),
		"PATH="+e.binDir()+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
}

func (e *Engine) command(ctx context.Context, args ...string) *exec.Cmd {
	build := e.Command
	if build == nil {
		build = exec.CommandContext
	}
	cmd := build(ctx, e.colima(), args...)
	cmd.Env = e.env()
	if e.Log != nil {
		cmd.Stdout, cmd.Stderr = e.Log, e.Log
	}
	return cmd
}

// startWait bounds how long the engine may take to answer after colima
// start returns; the first start creates the VM and pulls its image, so
// colima's own run is the long part and is bounded by ctx.
const startWait = 60 * time.Second

// Start starts the engine's VM and waits for its socket to answer. The
// virtual machine is Apple's (vz) with virtiofs mounts, the home directory
// shared; the person's docker context is left alone (--activate=false) and
// nothing is written to a configuration file beyond Colima's own.
func (e *Engine) Start(ctx context.Context) error {
	if !e.Installed() {
		return errors.New("the engine is not installed")
	}
	if err := e.CheckSocketPath(); err != nil {
		return fmt.Errorf("start Agentry's engine: %w", err)
	}
	// Colima honours COLIMA_HOME only once the directory exists; the
	// install made it and the docker configuration, and a start remakes
	// them should they have gone.
	if err := os.MkdirAll(e.colimaHome(), 0o700); err != nil {
		return fmt.Errorf("start Agentry's engine: its home: %w", err)
	}
	if err := e.writeDockerConfig(); err != nil {
		return fmt.Errorf("start Agentry's engine: %w", err)
	}
	e.logf("Starting Agentry's engine (Colima)…")
	cmd := e.command(ctx, "start", Profile, "--vm-type", "vz", "--mount-type", "virtiofs", "--activate=false", "--save-config=false")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start Agentry's engine: %w", err)
	}
	ping := e.Ping
	if ping == nil {
		return nil
	}
	deadline := time.Now().Add(startWait)
	for {
		if err := ping(ctx, e.Socket()); err == nil {
			e.logf("The engine answers at %s.", e.Socket())
			return nil
		} else if time.Now().After(deadline) || ctx.Err() != nil {
			return fmt.Errorf("the engine started but its socket %s does not answer: %w", e.Socket(), err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Stop stops the VM; the install stays.
func (e *Engine) Stop(ctx context.Context) error {
	if !e.Installed() {
		return nil
	}
	e.logf("Stopping Agentry's engine…")
	if err := e.command(ctx, "stop", Profile).Run(); err != nil {
		return fmt.Errorf("stop Agentry's engine: %w", err)
	}
	return nil
}

// Running asks colima whether the VM is up.
func (e *Engine) Running(ctx context.Context) bool {
	if !e.Installed() {
		return false
	}
	cmd := e.command(ctx, "status", Profile)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Run() == nil
}

// Remove deletes the VM with its data, then the VM directory and the whole
// engine directory, so nothing of the engine is left but the app. When the
// VM's deletion fails the directories still go, and the error says so: a
// host agent of the VM may be left running until the Mac restarts.
func (e *Engine) Remove(ctx context.Context) error {
	// Only ever the directories the app named: never a home, a root or
	// nothing.
	if d := filepath.Clean(e.Dir); d == "" || d == "." || d == string(filepath.Separator) || filepath.Base(d) != "engine" {
		return fmt.Errorf("refusing to remove %q: not an engine directory", e.Dir)
	}
	if d := filepath.Clean(e.vmDir()); filepath.Base(d) != vmDirName {
		return fmt.Errorf("refusing to remove %q: not the engine's VM directory", e.vmDir())
	}
	var deleteErr error
	if e.Installed() {
		e.logf("Removing Agentry's engine and its data…")
		if err := e.command(ctx, "delete", Profile, "--force", "--data").Run(); err != nil {
			deleteErr = fmt.Errorf("delete Agentry's engine's virtual machine: %w", err)
			e.logf("%v (removing the directory anyway)", deleteErr)
		}
	}
	if err := os.RemoveAll(e.vmDir()); err != nil {
		return fmt.Errorf("remove Agentry's engine's virtual machine directory: %w", err)
	}
	if err := os.RemoveAll(e.Dir); err != nil {
		return fmt.Errorf("remove Agentry's engine's directory: %w", err)
	}
	if deleteErr != nil {
		return fmt.Errorf("%w; the engine's directories were removed, and a process of its virtual machine may be left until this Mac restarts", deleteErr)
	}
	return nil
}

func (e *Engine) logf(format string, args ...any) {
	if e.Log != nil {
		_, _ = fmt.Fprintf(e.Log, format+"\n", args...)
	}
}
