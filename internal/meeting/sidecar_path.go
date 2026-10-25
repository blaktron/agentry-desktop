package meeting

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// sidecarDir is the sidecar's own folder, its libraries beside it.
const sidecarDir = "whisper"

// sidecarBeside is where a copy whose executable is in dir carries the
// sidecar, "" where it carries none. The sidecar is carried with the app,
// never fetched, and found nowhere a page or a setting can point:
//
//   - Linux packages: /usr/lib/agentry-desktop/whisper/whisper-cli for
//     /usr/bin/agentry-desktop.
//   - Any other Linux copy: whisper/whisper-cli beside the executable.
//   - Windows: bin\whisper\whisper-cli.exe beside agentry-desktop.exe.
//   - macOS: Contents/Resources/whisper/whisper-cli in the bundle.
func sidecarBeside(dir, goos string) string {
	dir = filepath.Clean(dir)
	switch goos {
	case "windows":
		return filepath.Join(dir, "bin", sidecarDir, "whisper-cli.exe")
	case "darwin":
		contents := filepath.Dir(dir)
		if filepath.Base(dir) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(filepath.Dir(contents), ".app") {
			return ""
		}
		return filepath.Join(contents, "Resources", sidecarDir, "whisper-cli")
	default:
		if dir == "/usr/bin" {
			return filepath.Join("/usr/lib/agentry-desktop", sidecarDir, "whisper-cli")
		}
		return filepath.Join(dir, sidecarDir, "whisper-cli")
	}
}

// SidecarPath is where this copy of the app carries the sidecar.
func SidecarPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return sidecarBeside(filepath.Dir(exe), runtime.GOOS)
}

// ErrNoSidecar is a copy of the app that carries no transcriber.
var ErrNoSidecar = errors.New("this copy of Agentry Desktop does not carry the transcriber")

// CheckSidecar says whether path is a sidecar that can run: a regular,
// executable file.
func CheckSidecar(path string) error {
	if path == "" {
		return ErrNoSidecar
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return ErrNoSidecar
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0 {
		return ErrNoSidecar
	}
	return nil
}
