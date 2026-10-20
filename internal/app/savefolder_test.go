package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The Build tab's Save, Install and Submit write without a file dialog: Go
// makes a new folder under the default output folder, named for the
// QuickAgent, and admits it as picked.

func saveFolderAPI(t *testing.T) (*BuildAPI, string) {
	t.Helper()
	home := t.TempDir()
	setUserHome(t, home)
	t.Setenv(configdir.EnvVar, t.TempDir())
	app := &App{
		ctx:     context.Background(),
		confirm: func(context.Context, string, string) (bool, error) { return false, nil },
		pickDirectory: func(context.Context, string) (string, error) {
			t.Fatal("Save opened the directory picker")
			return "", nil
		},
	}
	return &BuildAPI{app: app}, home
}

func TestNewSaveFolderIsUnderDownloadsAgentryOutWithNoRunDefault(t *testing.T) {
	b, home := saveFolderAPI(t)
	dir, err := b.NewSaveFolder("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "Downloads", "agentry-out", "release-notes"); dir != want {
		t.Fatalf("folder %q, want %q", dir, want)
	}
	if !isEmptyDir(t, dir) {
		t.Fatalf("%s is not a new, empty folder", dir)
	}
	// Go chose it, so the session's Save writes into it.
	if err := b.Save("https://r.example", dir, saveFolderModel()); err != nil {
		t.Fatalf("Save into the folder Go made: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.yaml")); err != nil {
		t.Fatalf("no agent.yaml after Save: %v", err)
	}
}

func TestNewSaveFolderIsUnderTheSavedOutputFolder(t *testing.T) {
	b, _ := saveFolderAPI(t)
	saved := t.TempDir()
	st, _ := settings.Load()
	st.DefaultOutputDir = saved
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	dir, err := b.NewSaveFolder("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(saved, "release-notes"); dir != want {
		t.Fatalf("folder %q, want %q", dir, want)
	}
}

func TestNewSaveFolderNeverReusesAFolderThatExists(t *testing.T) {
	b, home := saveFolderAPI(t)
	root := filepath.Join(home, "Downloads", "agentry-out")
	held := filepath.Join(root, "release-notes")
	if err := os.MkdirAll(held, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(held, "agent.yaml"), []byte("name: other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An empty folder of the next name is taken too: it existed first.
	if err := os.Mkdir(filepath.Join(root, "release-notes-2"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := b.NewSaveFolder("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "release-notes-3"); dir != want {
		t.Fatalf("folder %q, want %q", dir, want)
	}
	if got, _ := os.ReadFile(filepath.Join(held, "agent.yaml")); string(got) != "name: other\n" {
		t.Fatalf("the existing QuickAgent was touched: %q", got)
	}
}

func TestNewSaveFolderKeepsAHostileNameToOneFolderInTheRoot(t *testing.T) {
	b, home := saveFolderAPI(t)
	root := filepath.Join(home, "Downloads", "agentry-out")
	for name, want := range map[string]string{
		"../../etc":     "etc",
		"a/b":           "a-b",
		`a\b`:           "a-b",
		"..":            "quickagent",
		"":              "quickagent",
		"/abs/path":     "abs-path",
		"Release Notes": "release-notes",
	} {
		dir, err := b.NewSaveFolder(name)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if filepath.Dir(dir) != root || !strings.HasPrefix(filepath.Base(dir), want) {
			t.Errorf("%q made %q, want a folder %q… directly in %s", name, dir, want, root)
		}
	}
}

// A folder Go did not make, beside the one it did, is still refused.
func TestNewSaveFolderAdmitsOnlyTheFolderItMade(t *testing.T) {
	b, home := saveFolderAPI(t)
	if _, err := b.NewSaveFolder("release-notes"); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(home, "Downloads", "agentry-out", "elsewhere")
	if err := b.Save("https://r.example", other, saveFolderModel()); err == nil || !strings.Contains(err.Error(), "not chosen through the file dialog") {
		t.Fatalf("Save into a folder Go did not make: err = %v, want the picker refusal", err)
	}
	if err := b.Save("https://r.example", filepath.Join(home, "Downloads", "agentry-out"), saveFolderModel()); err == nil || !strings.Contains(err.Error(), "not chosen through the file dialog") {
		t.Fatalf("Save into the output folder itself: err = %v, want the picker refusal", err)
	}
}

func saveFolderModel() build.Model {
	m := build.Default("example")
	m.Name, m.Summary = "release-notes", "Writes release notes from a changelog."
	return *m
}

func isEmptyDir(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) == 0
}
