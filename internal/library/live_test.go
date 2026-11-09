package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// libraryAt is validDoc at a version, with one more preset so the copies
// can be told apart.
func libraryAt(version int, extra string) string {
	doc := strings.Replace(validDoc, "version: 1\n", "version: "+strconv.Itoa(version)+"\n", 1)
	if extra == "" {
		return doc
	}
	return doc + strings.ReplaceAll(strings.SplitN(validDoc, "presets:\n", 2)[1], "id: example", "id: "+extra)
}

func serve(t *testing.T, body *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := body.Load().(string)
		if b == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(b))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A newer copy from the repository is used, kept, and read back on the
// next start without the network.
func TestANewerCopyIsUsedAndKept(t *testing.T) {
	var body atomic.Value
	big := builtin.version + 10
	body.Store(libraryAt(big, "another"))
	srv := serve(t, &body)
	cache := filepath.Join(t.TempDir(), "library.yaml")
	var told []int
	l := NewLive(LiveOptions{URL: srv.URL, Cache: cache, OnChange: func(c Library) { told = append(told, c.Version()) }})
	if l.Status().Origin != OriginBuiltin || l.Current().Version() != builtin.version {
		t.Fatalf("before a read: %+v", l.Status())
	}
	changed, err := l.Refresh(context.Background())
	if err != nil || !changed {
		t.Fatalf("refresh: %v %v", changed, err)
	}
	st := l.Status()
	if st.Origin != OriginRepository || st.Version != big || st.Presets != 2 || st.Checked.IsZero() || st.Error != "" {
		t.Fatalf("after a read: %+v", st)
	}
	if _, ok := l.Current().Lookup("another"); !ok {
		t.Fatal("the newer copy's preset is not offered")
	}
	if len(told) != 2 || told[1] != big {
		t.Fatalf("told %v", told)
	}
	again := NewLive(LiveOptions{Cache: cache})
	if again.Status().Origin != OriginRepository || again.Current().Version() != big {
		t.Fatalf("the kept copy was not read back: %+v", again.Status())
	}
}

// A copy that is not newer, fails the checks, names another schema, or
// does not come changes nothing, and the failure is said.
func TestACopyThatCannotBeUsedChangesNothing(t *testing.T) {
	var body atomic.Value
	srv := serve(t, &body)
	cache := filepath.Join(t.TempDir(), "library.yaml")
	l := NewLive(LiveOptions{URL: srv.URL, Cache: cache})
	for name, doc := range map[string]string{
		"the same version": libraryAt(builtin.version, "another"),
		"an older version": libraryAt(1, "another"),
	} {
		body.Store(doc)
		if changed, err := l.Refresh(context.Background()); changed || err != nil {
			t.Errorf("%s: changed %v, err %v", name, changed, err)
		}
	}
	for name, doc := range map[string]string{
		"a failed check": strings.Replace(libraryAt(builtin.version+1, ""), "version: 1.2.3", "version: latest", 1),
		"another schema": strings.Replace(libraryAt(builtin.version+1, ""), "schema: 1", "schema: 2", 1),
		"not yaml":       "<html>rate limited</html>",
		"nothing there":  "",
	} {
		body.Store(doc)
		changed, err := l.Refresh(context.Background())
		if changed || err == nil || l.Status().Error == "" {
			t.Errorf("%s: changed %v, err %v, status %+v", name, changed, err, l.Status())
		}
	}
	if l.Status().Origin != OriginBuiltin || l.Current().Version() != builtin.version {
		t.Fatalf("the library moved: %+v", l.Status())
	}
	if _, err := os.Stat(cache); err == nil {
		t.Fatal("a copy that was not used was kept")
	}
}

// A kept copy older than the built-in one is passed over, as is one that
// no longer passes the checks.
func TestABuiltinNewerThanTheKeptCopyWins(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.yaml")
	if err := os.WriteFile(old, []byte(libraryAt(1, "another")), 0o600); err != nil {
		t.Fatal(err)
	}
	if l := NewLive(LiveOptions{Cache: old}); l.Status().Origin != OriginBuiltin {
		t.Fatalf("an older kept copy was used: %+v", l.Status())
	}
	broken := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(broken, []byte(strings.Replace(libraryAt(builtin.version+5, ""), "https://example.com/src", "http://example.com/src", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if l := NewLive(LiveOptions{Cache: broken}); l.Status().Origin != OriginBuiltin {
		t.Fatalf("a kept copy that fails the checks was used: %+v", l.Status())
	}
}

func TestReleasesReadMainAndPrereleasesReadDev(t *testing.T) {
	for v, branch := range map[string]string{"0.7.31": "main", "1.0.0": "main", "0.7.32-dev.1": "dev", "0.0.0-dev": "dev", "": "dev"} {
		if got := URLFor(v); got != Repository+"/"+branch+"/"+Path {
			t.Errorf("%q reads %s", v, got)
		}
	}
	if !strings.HasPrefix(Repository, "https://") {
		t.Fatal("the repository is not read over https")
	}
}

// The file the app reads from the repository is this package's library:
// the path the address names is the file built in.
func TestThePathIsTheBuiltInFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", Path))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(builtinData) {
		t.Fatal("the repository path is not the embedded library")
	}
}
