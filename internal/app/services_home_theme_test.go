package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func themeJS(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	ThemeScript().ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

// /theme.js puts the stored theme on the root element before the first
// paint, says nothing for System, is never cached, and is the one path the
// handler serves.
func TestThemeScriptPutsTheStoredThemeOnBeforeThePage(t *testing.T) {
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))

	w := themeJS(t, http.MethodGet, "/theme.js")
	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Fatalf("System: %d %q, want 200 and no statement", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
		t.Fatalf("content type %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache control %q", cc)
	}

	if err := settings.SetTheme("midnight"); err != nil {
		t.Fatal(err)
	}
	if got := themeJS(t, http.MethodGet, "/theme.js").Body.String(); got != "document.documentElement.dataset.theme = \"midnight\";\n" {
		t.Fatalf("midnight: %q", got)
	}

	for _, r := range [][2]string{{http.MethodGet, "/theme.json"}, {http.MethodGet, "/other.js"}, {http.MethodPost, "/theme.js"}} {
		if w := themeJS(t, r[0], r[1]); w.Code != http.StatusNotFound {
			t.Fatalf("%s %s: %d, want 404", r[0], r[1], w.Code)
		}
	}
}
