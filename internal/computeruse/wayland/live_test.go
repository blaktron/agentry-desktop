//go:build linux && cgo

package wayland

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// fileTokens keeps restore tokens in a JSON file.
type fileTokens struct {
	mu   sync.Mutex
	path string
}

func (f *fileTokens) Get(k string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := map[string]string{}
	if b, err := os.ReadFile(f.path); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m[k]
}

func (f *fileTokens) Set(k, v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := map[string]string{}
	if b, err := os.ReadFile(f.path); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	m[k] = v
	b, _ := json.Marshal(m)
	_ = os.WriteFile(f.path, b, 0o600)
}

// TestLiveWaylandAppScope opens app scope on a running desktop for the
// application named by AGENTRY_LIVE_APP_CLASS, captures its window into
// AGENTRY_LIVE_OUT and moves the pointer to the window's centre. It runs
// only with AGENTRY_LIVE_COMPUTER_USE=1 in a Wayland session.
func TestLiveWaylandAppScope(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_COMPUTER_USE") != "1" {
		t.Skip("set AGENTRY_LIVE_COMPUTER_USE=1 in a Wayland session")
	}
	out := os.Getenv("AGENTRY_LIVE_OUT")
	class := os.Getenv("AGENTRY_LIVE_APP_CLASS")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	b, err := Open(ctx, Options{Scope: computeruse.ScopeApp, Apps: []App{{Name: "Blender", Classes: []string{class}}},
		Tokens: &fileTokens{path: filepath.Join(out, "tokens.json")}, ScriptDir: filepath.Join(out, "kwin")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	t.Logf("session %q; enforced: %s", b.Session(), b.Enforced())
	ws, err := b.Windows(ctx)
	if err != nil || len(ws) != 1 {
		t.Fatalf("windows %+v %v", ws, err)
	}
	w := ws[0]
	t.Logf("window %+v", w)
	img, err := b.Capture(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.Create(filepath.Join(out, "capture.png"))
	_ = png.Encode(f, img)
	_ = f.Close()
	if err := b.Focus(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	centre := image.Pt(w.Size.X/2, w.Size.Y/2)
	if err := b.Move(ctx, w.ID, centre); err != nil {
		t.Fatal(err)
	}
	p, in, err := b.Cursor(ctx, w.ID)
	t.Logf("cursor %v in=%v err=%v", p, in, err)
	if !in || p != centre {
		t.Fatalf("the pointer is at %v, want %v", p, centre)
	}
}
