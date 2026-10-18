package deliverables

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FolderName is the folder inside the Documents folder the deliverables go
// to unless the person chose another.
const FolderName = "Agentry"

// DefaultRoot is the Deliverables folder when none was chosen: an Agentry
// folder inside the person's Documents folder.
func DefaultRoot() (string, error) {
	docs, err := DocumentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(docs, FolderName), nil
}

// DocumentsDir is the person's Documents folder: FOLDERID_Documents on
// Windows (it follows a redirected or renamed folder), XDG_DOCUMENTS_DIR on
// Linux and ~/Documents on macOS.
func DocumentsDir() (string, error) {
	return documentsDir(runtime.GOOS, os.Getenv, userHome)
}

func userHome() (string, error) { return os.UserHomeDir() }

var errNoDocuments = errors.New("this computer names no Documents folder for the person")

// unixDocuments is the Documents folder on a system with no known-folder
// API. On Linux it is XDG_DOCUMENTS_DIR, from the environment when set and
// absolute, else from the user-dirs.dirs file the desktop environment keeps
// (the file `xdg-user-dir DOCUMENTS` reads), else ~/Documents. A value of
// the file is "$HOME/…" or an absolute path; anything else is not used.
func unixDocuments(goos string, getenv func(string) string, home func() (string, error)) (string, error) {
	h, err := home()
	if err != nil || h == "" {
		return "", errNoDocuments
	}
	fallback := filepath.Join(h, "Documents")
	if goos != "linux" {
		return fallback, nil
	}
	if d := getenv("XDG_DOCUMENTS_DIR"); filepath.IsAbs(d) {
		return filepath.Clean(d), nil
	}
	cfg := getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(cfg) {
		cfg = filepath.Join(h, ".config")
	}
	if d := userDirsDocuments(filepath.Join(cfg, "user-dirs.dirs"), h); d != "" {
		return d, nil
	}
	return fallback, nil
}

// userDirsDocuments reads XDG_DOCUMENTS_DIR from a user-dirs.dirs file: a
// shell-style line, XDG_DOCUMENTS_DIR="$HOME/Documents". "" when the file or
// the line is not there or its value is not an absolute path once $HOME is
// put in.
func userDirsDocuments(file, home string) string {
	f, err := os.Open(file) //nolint:gosec // the desktop environment's own settings file
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 64<<10)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		v, ok := strings.CutPrefix(line, "XDG_DOCUMENTS_DIR=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		v = strings.Replace(v, "$HOME", home, 1)
		if !filepath.IsAbs(v) || strings.ContainsAny(v, "$`") {
			return ""
		}
		return filepath.Clean(v)
	}
	return ""
}
