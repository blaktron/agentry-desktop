package run

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blaktron/agentry/pkg/reportdesign"
)

// TestLiveRenderReportSample renders the sample report in two built-in
// designs and in one whose cover style is a custom cover, on this
// computer's container engine with the pinned runner image:
//
//	AGENTRY_LIVE_RENDER=1 [AGENTRY_LIVE_RENDER_OUT=<folder>] go test ./internal/run -run TestLiveRenderReportSample -v
//
// The runner image is built on first use, which takes minutes. With
// AGENTRY_LIVE_RENDER_OUT set, each PDF is written there for a look. The
// custom cover is drawn by a runner whose template runs a design's own
// cover, and falls back to the band style on one that does not:
// AGENTRY_LIVE_RENDER_CUSTOM=drawn demands the first, =fallback the second.
func TestLiveRenderReportSample(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_RENDER") != "1" {
		t.Skip("set AGENTRY_LIVE_RENDER=1 to render the sample on this computer's container engine")
	}
	out := os.Getenv("AGENTRY_LIVE_RENDER_OUT")
	custom, _ := reportdesign.Builtin("boardroom")
	custom.Name = "Own cover"
	custom.Cover.Style = "custom"
	custom.CustomCover = reportdesign.CustomCover{Source: "#set page(fill: rgb(\"#1b2a4a\"))\n#align(center + horizon, text(white, 40pt)[A cover of my own])\n", Images: []reportdesign.Image{}}
	cases := []struct {
		name   string
		design reportdesign.Design
	}{
		{name: "plain", design: mustBuiltin(t, "plain")},
		{name: "boardroom", design: mustBuiltin(t, "boardroom")},
		{name: "custom", design: custom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			logw := io.Discard
			if testing.Verbose() {
				logw = os.Stderr
			}
			start := time.Now()
			r, err := RenderReportSample(ctx, c.design.Canonical(), logw)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(r.PDF, []byte("%PDF-")) || r.Pages < 3 {
				t.Fatalf("a %d-page render of %d bytes", r.Pages, len(r.PDF))
			}
			t.Logf("%s: %d pages, %d bytes, cover %q, %s", c.name, r.Pages, len(r.PDF), r.Cover, time.Since(start).Round(time.Millisecond))
			if c.design.Cover.When != "never" && r.Cover == "" {
				t.Errorf("no cover drawn for a design whose cover is drawn %q", c.design.Cover.When)
			}
			if c.design.Cover.Style != "custom" && r.Cover != "" && r.Cover != c.design.Cover.Style && c.design.Cover.Style != "plate" {
				t.Errorf("the %s cover drawn for a design that asks for %s", r.Cover, c.design.Cover.Style)
			}
			if c.design.Cover.Style == "custom" {
				switch want := os.Getenv("AGENTRY_LIVE_RENDER_CUSTOM"); {
				case want == "drawn" && r.Cover != "custom":
					t.Errorf("the custom cover fell back to %q", r.Cover)
				case want == "fallback" && (r.Cover == "custom" || r.Cover == ""):
					t.Errorf("the custom cover was drawn as %q, not a fallback", r.Cover)
				}
			}
			if out != "" {
				if err := os.WriteFile(filepath.Join(out, c.name+".pdf"), r.PDF, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func mustBuiltin(t *testing.T, id string) reportdesign.Design {
	t.Helper()
	d, ok := reportdesign.Builtin(id)
	if !ok {
		t.Fatalf("no built-in %q", id)
	}
	return d
}
