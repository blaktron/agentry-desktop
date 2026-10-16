package selfupdate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
)

// sumsFor writes a SHA256SUMS body over the given files, in the shape the
// release's `sha256sum *` produces.
func sumsFor(files map[string][]byte) string {
	var b strings.Builder
	for name, body := range files {
		d := sha256.Sum256(body)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(d[:]), name)
	}
	return b.String()
}

func TestArtifactForPicksWhatEachPlatformInstalls(t *testing.T) {
	files := map[string][]byte{
		"agentry-desktop-0.2.0-windows-amd64-setup.exe": []byte("setup"),
		"agentry-desktop-0.2.0-windows-amd64.exe":       []byte("bare exe"),
		"agentry-desktop-0.2.0-windows-amd64.zip":       []byte("zip"),
		"agentry-desktop-0.2.0-darwin-universal.zip":    []byte("bundle zip"),
		"agentry-desktop-0.2.0-darwin-universal.dmg":    []byte("dmg"),
		"agentry-desktop-0.2.0-linux-amd64":             []byte("bin"),
		"agentry-desktop-0.2.0-linux-amd64.deb":         []byte("deb"),
	}
	sums := sumsFor(files)

	t.Run("windows installs from the per-user setup, not the bare exe or zip", func(t *testing.T) {
		got, err := selfupdate.ArtifactFor(sums, "0.2.0", "windows", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "agentry-desktop-0.2.0-windows-amd64-setup.exe" {
			t.Errorf("picked %q", got.Name)
		}
		d := sha256.Sum256(files[got.Name])
		if got.SHA256 != hex.EncodeToString(d[:]) {
			t.Error("the digest is not the one the checksums give")
		}
	})

	t.Run("macos installs from the zip, not the dmg", func(t *testing.T) {
		got, err := selfupdate.ArtifactFor(sums, "0.2.0", "darwin", "arm64")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "agentry-desktop-0.2.0-darwin-universal.zip" {
			t.Errorf("picked %q; the dmg needs mounting and the zip does not", got.Name)
		}
	})

	t.Run("linux is the package manager's", func(t *testing.T) {
		_, err := selfupdate.ArtifactFor(sums, "0.2.0", "linux", "amd64")
		if !errors.Is(err, selfupdate.ErrNotApplicable) {
			t.Fatalf("err = %v, want ErrNotApplicable", err)
		}
	})
}

// The name is looked up in the signed checksums, never taken from anywhere
// else, so a release can only offer a file it actually signed.
func TestArtifactForRefusesWhatTheChecksumsDoNotName(t *testing.T) {
	sums := sumsFor(map[string][]byte{"agentry-desktop-0.2.0-linux-amd64": []byte("bin")})

	if _, err := selfupdate.ArtifactFor(sums, "0.2.0", "darwin", "arm64"); err == nil {
		t.Fatal("a release with nothing for this platform still offered something")
	}
	// A version other than the one being offered must not match either: the
	// checksums of release 0.1.0 do not authorise installing 0.2.0.
	if _, err := selfupdate.ArtifactFor(sums, "0.1.0", "linux", "amd64"); !errors.Is(err, selfupdate.ErrNotApplicable) {
		t.Fatalf("err = %v", err)
	}
	sums2 := sumsFor(map[string][]byte{"agentry-desktop-0.1.0-darwin-universal.zip": []byte("old")})
	if _, err := selfupdate.ArtifactFor(sums2, "0.2.0", "darwin", "arm64"); err == nil {
		t.Fatal("the checksums of another release were accepted for this one")
	}
}

func TestArtifactForRefusesADigestThatIsNotOne(t *testing.T) {
	sums := "not-a-digest  agentry-desktop-0.2.0-darwin-universal.zip\n"
	if _, err := selfupdate.ArtifactFor(sums, "0.2.0", "darwin", "arm64"); err == nil || !strings.Contains(err.Error(), "not a SHA-256") {
		t.Fatalf("err = %v, want the malformed digest refused", err)
	}
}

// serveFiles is a channel serving a downloads directory.
func serveFiles(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[strings.TrimPrefix(r.URL.Path, "/downloads/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadWritesOnlyWhatTheChecksumsSigned(t *testing.T) {
	body := []byte("the real installer bytes")
	name := "agentry-desktop-0.2.0-darwin-universal.zip"
	files := map[string][]byte{name: body}
	srv := serveFiles(t, files)
	art, err := selfupdate.ArtifactFor(sumsFor(files), "0.2.0", "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	path, err := selfupdate.Download(context.Background(), srv.Client(), srv.URL+"/downloads/manifest.json", art, dir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Error("the downloaded file is not what was served")
	}
	if filepath.Base(path) != name {
		t.Errorf("written as %q", filepath.Base(path))
	}
}

// The case the whole design exists for: the channel serves something other
// than what the release signed. It must be refused AND removed — a rejected
// download left on disk is one a later step could mistake for a release.
func TestDownloadRefusesAndRemovesBytesTheChecksumsDoNotMatch(t *testing.T) {
	name := "agentry-desktop-0.2.0-darwin-universal.zip"
	art, err := selfupdate.ArtifactFor(sumsFor(map[string][]byte{name: []byte("what was signed")}), "0.2.0", "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	srv := serveFiles(t, map[string][]byte{name: []byte("something else entirely")})
	dir := t.TempDir()

	_, err = selfupdate.Download(context.Background(), srv.Client(), srv.URL+"/downloads/manifest.json", art, dir)
	if err == nil {
		t.Fatal("bytes the release did not sign were accepted")
	}
	if !strings.Contains(err.Error(), "not the file this release signed") {
		t.Errorf("message %q does not say what happened", err)
	}
	left, _ := os.ReadDir(dir)
	for _, e := range left {
		t.Errorf("a rejected download was left behind: %s", e.Name())
	}
}

func TestDownloadRefusesAnArtifactFromAnotherHost(t *testing.T) {
	name := "agentry-desktop-0.2.0-darwin-universal.zip"
	srv := serveFiles(t, map[string][]byte{name: []byte("x")})
	art := selfupdate.Artifact{Name: "https://elsewhere.example.test/evil.zip", SHA256: strings.Repeat("0", 64)}

	_, err := selfupdate.Download(context.Background(), srv.Client(), srv.URL+"/downloads/manifest.json", art, t.TempDir())
	if err == nil {
		t.Fatal("an artifact named on another host was fetched")
	}
}

func TestDownloadRefusesAnArtifactNameThatIsAPath(t *testing.T) {
	srv := serveFiles(t, map[string][]byte{"x": []byte("x")})
	for _, name := range []string{"../escape.zip", "sub/dir.zip", "/abs.zip"} {
		if _, err := selfupdate.Download(context.Background(), srv.Client(), srv.URL+"/downloads/manifest.json",
			selfupdate.Artifact{Name: name, SHA256: strings.Repeat("0", 64)}, t.TempDir()); err == nil {
			t.Errorf("the artifact name %q was fetched", name)
		}
	}
}

func TestDownloadRefusesAChannelThatAnswersAnError(t *testing.T) {
	srv := serveFiles(t, map[string][]byte{})
	art := selfupdate.Artifact{Name: "agentry-desktop-0.2.0-darwin-universal.zip", SHA256: strings.Repeat("0", 64)}

	_, err := selfupdate.Download(context.Background(), srv.Client(), srv.URL+"/downloads/manifest.json", art, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want the 404 reported", err)
	}
}
