// Package windowsengine is the container engine the app installs for the
// person on Windows when the computer offers none: Rancher Desktop with its
// dockerd (moby) engine, from SUSE's signed installer pinned by version and
// by the SHA-512 sum its publisher states, installed for this user alone
// with no administrator rights. Rancher Desktop needs Windows Subsystem for
// Linux 2, which its installer does not turn on; where WSL is missing the
// app asks Windows to install it, which takes administrator consent once
// and a restart. Rancher Desktop stays the person's own application: it
// is listed in Windows' installed apps and removed there.
package windowsengine

import (
	"bytes"
	"context"
	"crypto/sha512"
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
	"unicode/utf16"
)

// The pinned release. A bump changes these four and nothing else.
const (
	RancherVersion = "1.24.0"
	MSIName        = "Rancher.Desktop.Setup." + RancherVersion + ".msi"
	MSIURL         = "https://github.com/rancher-sandbox/rancher-desktop/releases/download/v" + RancherVersion + "/" + MSIName
	// MSISHA512 is the sum the release publishes beside the installer
	// (<installer>.sha512sum); it publishes no SHA-256.
	MSISHA512 = "6428f330cdf8dc9dbbc07bbde270046c9e86fb91e90392c0d25ac764e64eaa8f9d2b3e7faaffed669a48f7fd7445dadda18072dd51c905bf59df4787b70ed77e"
)

// DownloadMB is about what the installer weighs (670,932,992 bytes), for
// the consent and the card.
const DownloadMB = 640

// Licence is Rancher Desktop's.
const Licence = "Apache-2.0"

// WSL is where Windows Subsystem for Linux stands on this computer.
type WSL string

const (
	// WSLReady: WSL answers; Rancher Desktop can install.
	WSLReady WSL = "ready"
	// WSLMissing: WSL is not installed; installing it needs administrator
	// consent and a restart.
	WSLMissing WSL = "missing"
	// WSLRestartPending: the app installed WSL and Windows has not
	// restarted since, so its virtual machine platform is not on yet.
	WSLRestartPending WSL = "restartPending"
)

// ErrWSLMissing is the installer's refusal when WSL is not installed.
var ErrWSLMissing = errors.New("the Rancher Desktop installer needs Windows Subsystem for Linux 2, which is not installed")

// Installer installs and starts Rancher Desktop for this user.
type Installer struct {
	// StateDir is the app's own directory for it: the downloaded installer,
	// the install log, wsl-restart.json while a restart is owed, and
	// installed.json once the app has installed it.
	StateDir string
	// LocalAppData and ProgramFiles are where a per-user and a machine-wide
	// install of Rancher Desktop live.
	LocalAppData string
	ProgramFiles string
	// URL and SHA512 are the installer's; a test points them at its own.
	URL    string
	SHA512 string
	// Client fetches the installer; nil is http.DefaultClient with a
	// timeout.
	Client *http.Client
	// Command builds the commands run (wsl, powershell, msiexec, rdctl); a
	// test hands a stand-in. nil is exec.CommandContext.
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd
	// BootTime is when Windows last started; a test stubs it.
	BootTime func() (time.Time, error)
	// Log receives the install's progress lines.
	Log io.Writer
}

// Default is the installer with the app's own directory and this user's
// environment.
func Default() (*Installer, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("the application data directory: %w", err)
	}
	return &Installer{
		StateDir:     filepath.Join(base, "agentry-desktop", "windows-engine"),
		LocalAppData: os.Getenv("LOCALAPPDATA"),
		ProgramFiles: os.Getenv("ProgramFiles"),
		URL:          MSIURL,
		SHA512:       MSISHA512,
		BootTime:     bootTime,
	}, nil
}

// Supported says whether this build can install the engine: Windows on
// x86-64, the one platform Rancher Desktop's installer is published for.
func Supported() bool { return runtime.GOOS == "windows" && runtime.GOARCH == "amd64" }

// binRel is where Rancher Desktop keeps its Windows commands (rdctl,
// docker) under its install directory.
var binRel = filepath.Join("resources", "resources", "win32", "bin")

// installDirs are a per-user install's directory and a machine-wide one's.
func (i *Installer) installDirs() []string {
	var dirs []string
	if i.LocalAppData != "" {
		dirs = append(dirs, filepath.Join(i.LocalAppData, "Programs", "Rancher Desktop"))
	}
	if i.ProgramFiles != "" {
		dirs = append(dirs, filepath.Join(i.ProgramFiles, "Rancher Desktop"))
	}
	return dirs
}

// Rdctl is Rancher Desktop's control command: the first install found, or
// where a per-user install puts it.
func (i *Installer) Rdctl() string { return i.command("rdctl.exe") }

// Docker is the docker command Rancher Desktop ships.
func (i *Installer) Docker() string { return i.command("docker.exe") }

func (i *Installer) command(name string) string {
	dirs := i.installDirs()
	for _, d := range dirs {
		p := filepath.Join(d, binRel, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if len(dirs) == 0 {
		return name
	}
	return filepath.Join(dirs[0], binRel, name)
}

// RancherInstalled says whether Rancher Desktop is installed, by the app
// or by the person, for this user or for the machine.
func (i *Installer) RancherInstalled() bool {
	for _, d := range i.installDirs() {
		if _, err := os.Stat(filepath.Join(d, binRel, "rdctl.exe")); err == nil {
			return true
		}
	}
	return false
}

// InstalledByApp says whether the app installed it, which is when the app
// also starts it when it opens.
func (i *Installer) InstalledByApp() bool {
	_, err := os.Stat(filepath.Join(i.StateDir, installedFile))
	return err == nil && i.RancherInstalled()
}

const (
	installedFile = "installed.json"
	restartFile   = "wsl-restart.json"
	logFile       = "msiexec.log"
)

// restartRecord is wsl-restart.json: when Windows had last started when
// the app installed WSL.
type restartRecord struct {
	Boot time.Time `json:"boot"`
}

// bootSlack absorbs the drift of a boot time computed from the uptime
// counter: two reads in one boot differ by milliseconds, two boots by far
// more.
const bootSlack = 2 * time.Minute

// WSLState reports where WSL stands. A restart is owed while the boot the
// app installed WSL in is still the current one; after it the record goes.
// Otherwise WSL is ready when `wsl --version` (WSL from the Store) or
// `wsl --status` (the in-box one) succeeds.
func (i *Installer) WSLState(ctx context.Context) WSL {
	if b, err := os.ReadFile(filepath.Join(i.StateDir, restartFile)); err == nil { //nolint:gosec // a fixed name under the app's own directory
		var rec restartRecord
		if json.Unmarshal(b, &rec) == nil && i.BootTime != nil {
			if now, err := i.BootTime(); err == nil {
				if d := now.Sub(rec.Boot); d < bootSlack && d > -bootSlack {
					return WSLRestartPending
				}
			}
		}
		_ = os.Remove(filepath.Join(i.StateDir, restartFile))
	}
	for _, arg := range []string{"--version", "--status"} {
		if i.cmd(ctx, "wsl.exe", arg).Run() == nil {
			return WSLReady
		}
	}
	return WSLMissing
}

// InstallWSLScript is the PowerShell the app runs to install WSL: Windows'
// own elevation prompt asks for administrator consent, and the exit code
// is wsl's.
const InstallWSLScript = `$p = Start-Process -FilePath 'wsl.exe' -ArgumentList '--install','--no-distribution' -Verb RunAs -Wait -PassThru; exit $p.ExitCode`

// InstallWSL asks Windows to install WSL with no Linux distribution of the
// person's (Rancher Desktop brings its own) and records that a restart is
// owed. Declining the consent prompt is an error that says so.
func (i *Installer) InstallWSL(ctx context.Context) error {
	i.logf("Installing Windows Subsystem for Linux: Windows asks for administrator consent")
	cmd := i.cmd(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", InstallWSLScript)
	out, err := cmd.CombinedOutput()
	i.logf("%s", strings.TrimSpace(decodeText(out)))
	if err != nil {
		return fmt.Errorf("no Windows Subsystem for Linux was installed (the consent was declined, or wsl --install failed): %w", err)
	}
	if err := os.MkdirAll(i.StateDir, 0o700); err != nil {
		return err
	}
	boot := time.Now()
	if i.BootTime != nil {
		if b, err := i.BootTime(); err == nil {
			boot = b
		}
	}
	raw, _ := json.Marshal(restartRecord{Boot: boot})
	return os.WriteFile(filepath.Join(i.StateDir, restartFile), raw, 0o600)
}

// msiPath is where the downloaded installer is kept.
func (i *Installer) msiPath() string { return filepath.Join(i.StateDir, MSIName) }

// Download fetches the installer into the state directory and keeps it only
// when its SHA-512 is the pinned one; a copy already there that matches is
// used as it is.
func (i *Installer) Download(ctx context.Context) (string, error) {
	dst := i.msiPath()
	if sum, err := fileSHA512(dst); err == nil && sum == i.SHA512 {
		i.logf("Rancher Desktop %s's installer is already here and matches its sum", RancherVersion)
		return dst, nil
	}
	if err := os.MkdirAll(i.StateDir, 0o700); err != nil {
		return "", err
	}
	i.logf("Downloading Rancher Desktop %s's installer (about %d MB) from %s", RancherVersion, DownloadMB, i.URL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := i.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("download Rancher Desktop's installer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download Rancher Desktop's installer: %s answered %s", i.URL, resp.Status)
	}
	part := dst + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // a fixed name under the app's own directory
	if err != nil {
		return "", err
	}
	h := sha512.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(part)
		return "", fmt.Errorf("download Rancher Desktop's installer: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != i.SHA512 {
		_ = os.Remove(part)
		return "", fmt.Errorf("the Rancher Desktop installer does not match the SHA-512 this app pins (got %s): nothing was installed", got)
	}
	if err := os.Rename(part, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// MSIArgs is the installer's command line: silent, no restart, a verbose
// log, and for this user alone (the installer's own default is the whole
// machine, which needs an administrator).
func MSIArgs(msi, log string) []string {
	return []string{"/i", msi, "/qn", "/norestart", "/l*v", log, "MSIINSTALLPERUSER=1"}
}

// Windows Installer's exit codes the install reads.
const (
	msiSuccess       = 0
	msiRebootNeeded  = 3010
	msiAnotherInFlux = 1618
)

// InstallMSI runs the installer for this user. Success is 0, or 3010 (a
// restart Windows wants for something else); 1618 is another install in
// progress; a refusal because WSL is missing is ErrWSLMissing, read from
// the log.
func (i *Installer) InstallMSI(ctx context.Context, msi string) error {
	log := filepath.Join(i.StateDir, logFile)
	i.logf("Installing Rancher Desktop %s for this user (no administrator rights)", RancherVersion)
	err := i.cmd(ctx, "msiexec.exe", MSIArgs(msi, log)...).Run()
	code := exitCodeOf(err)
	switch code {
	case msiSuccess, msiRebootNeeded:
		raw, _ := json.Marshal(map[string]string{"rancherDesktop": RancherVersion})
		return os.WriteFile(filepath.Join(i.StateDir, installedFile), raw, 0o600)
	case msiAnotherInFlux:
		return errors.New("another installation is in progress on this computer; try again when it is done")
	}
	b, _ := os.ReadFile(log) //nolint:gosec // the log this install wrote
	if strings.Contains(decodeText(b), "Windows Subsystem for Linux") {
		return ErrWSLMissing
	}
	return fmt.Errorf("the Rancher Desktop installer failed (exit %d); its log is %s", code, log)
}

// FirstStartArgs start Rancher Desktop the first time with the choices its
// first-run window would ask for: the dockerd (moby) engine, which serves
// the Docker API, Kubernetes off, in the background, and no window.
func FirstStartArgs() []string {
	return []string{"start", "--container-engine.name=moby", "--kubernetes.enabled=false", "--application.start-in-background", "--no-modal-dialogs"}
}

// StartArgs start it again with the settings it saved.
func StartArgs() []string {
	return []string{"start", "--application.start-in-background", "--no-modal-dialogs"}
}

// Start launches Rancher Desktop through rdctl. It returns once the
// application is launched; its engine answers a while later.
func (i *Installer) Start(ctx context.Context, first bool) error {
	args := StartArgs()
	if first {
		args = FirstStartArgs()
	}
	i.logf("Starting Rancher Desktop (%s)", strings.Join(args, " "))
	out, err := i.cmd(ctx, i.Rdctl(), args...).CombinedOutput()
	if s := strings.TrimSpace(decodeText(out)); s != "" {
		i.logf("%s", s)
	}
	if err != nil {
		return fmt.Errorf("start Rancher Desktop: %w", err)
	}
	return nil
}

func (i *Installer) cmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	if i.Command != nil {
		return i.Command(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...) //nolint:gosec // fixed programs, arguments built here
}

func (i *Installer) client() *http.Client {
	if i.Client != nil {
		return i.Client
	}
	return &http.Client{Timeout: 30 * time.Minute}
}

func (i *Installer) logf(format string, a ...any) {
	if i.Log != nil {
		_, _ = fmt.Fprintf(i.Log, format+"\n", a...)
	}
}

func fileSHA512(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // a fixed name under the app's own directory
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// exitCodeOf reads a command's exit code; a test maps it.
var exitCodeOf = exitCode

// exitCode is a command's exit code, -1 when it did not run.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// decodeText reads what wsl.exe and Windows Installer write, UTF-16LE with
// or without its byte-order mark, or plain bytes.
func decodeText(b []byte) string {
	if bytes.HasPrefix(b, []byte{0xFF, 0xFE}) {
		b = b[2:]
	} else if len(b) < 2 || b[1] != 0 || b[0] == 0 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for k := range u {
		u[k] = uint16(b[2*k]) | uint16(b[2*k+1])<<8
	}
	return string(utf16.Decode(u))
}
