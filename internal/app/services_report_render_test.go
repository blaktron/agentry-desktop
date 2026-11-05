package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry/pkg/reportdesign"
	"github.com/google/uuid"
)

// standInRender replaces the render's container for a test: it records the
// design it was asked to lay out and answers r.
func standInRender(t *testing.T, r *run.ReportRender, err error) *[]reportdesign.Design {
	t.Helper()
	var got []reportdesign.Design
	saved := renderSample
	renderSample = func(_ context.Context, doc []byte, _ io.Writer) (*run.ReportRender, error) {
		d, perr := reportdesign.Parse(doc)
		if perr != nil {
			t.Errorf("the render was handed a document the core refuses: %v", perr)
		}
		got = append(got, d)
		return r, err
	}
	t.Cleanup(func() { renderSample = saved; cancelRender() })
	return &got
}

var renderedBytes = []byte("%PDF-1.7\n% the render\n%%EOF\n")

// TestRenderLaysOutTheEditorsDesignWithItsStoredImages: Render draws the
// design the editor holds, an unsaved change included, with the logo, the
// cover image and the custom cover the registry keeps for it, never images
// the page's copy carries; the view names the render's facts, and the
// viewer reads its bytes by the view's token alone.
func TestRenderLaysOutTheEditorsDesignWithItsStoredImages(t *testing.T) {
	f := newFakeLibrary(t, true)
	stored := builtin(t, "boardroom")
	stored.Logo = reportdesign.Logo{PNG: base64.StdEncoding.EncodeToString(testPNG(t, 120, 40))}
	stored.CoverImage = reportdesign.Image{JPEG: base64.StdEncoding.EncodeToString(testJPEG(t, 64, 48))}
	stored.CustomCover = reportdesign.CustomCover{Source: "#rect(width: 100%, height: 100%)\n", Images: []reportdesign.Image{}}
	stored.Cover.Style = "custom"
	if err := stored.Validate(); err != nil {
		t.Fatal(err)
	}
	id := f.add(stored, true)

	editing := throughThePage(t, f.stored(id).WithoutAssets())
	editing.Brand.Name = "Unsaved Ltd"
	editing.Logo = reportdesign.Logo{PNG: base64.StdEncoding.EncodeToString(testPNG(t, 30, 30))}
	got := standInRender(t, &run.ReportRender{PDF: renderedBytes, SHA256: sha256Hex(renderedBytes), Pages: 5, Cover: "band", Logo: true}, nil)

	v, err := renderReportDesign(context.Background(), f.srv.URL, "agy_test", id, editing)
	if err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 {
		t.Fatalf("%d renders", len(*got))
	}
	d := (*got)[0]
	if d.Brand.Name != "Unsaved Ltd" || d.Logo != stored.Logo || d.CoverImage != stored.CoverImage || d.CustomCover.Source != stored.CustomCover.Source {
		t.Fatalf("rendered %q with the stored images %v %v %v", d.Brand.Name, d.Logo == stored.Logo, d.CoverImage == stored.CoverImage, d.CustomCover.Source == stored.CustomCover.Source)
	}
	if v.Pages != 5 || v.Cover != "band" || v.CoverStyle != "custom" || !v.Fallback || !v.Logo || v.Bytes != len(renderedBytes) || v.Name != stored.Name || v.Token == "" {
		t.Fatalf("view %+v", v)
	}
	if f.stored(id).Brand.Name == "Unsaved Ltd" {
		t.Fatal("a render saved the editor's design")
	}

	b64, err := renderedPDF(v.Token)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := base64.StdEncoding.DecodeString(b64); !bytes.Equal(b, renderedBytes) {
		t.Fatal("the viewer was handed other bytes")
	}
	for _, bad := range []string{"", "0" + v.Token[1:], strings.ToUpper(v.Token)} {
		if bad == v.Token {
			continue
		}
		if _, err := renderedPDF(bad); err == nil {
			t.Fatalf("token %q read the render", bad)
		}
	}
	cancelRender()
	if _, err := renderedPDF(v.Token); err == nil {
		t.Fatal("a closed render was still read")
	}
}

// TestRenderSaysWhenTheDesignsOwnCoverWasNotDrawn: Fallback is a design
// that asks for its own cover and got another drawn; a built-in style drawn
// as asked, and no cover at all, are not.
func TestRenderSaysWhenTheDesignsOwnCoverWasNotDrawn(t *testing.T) {
	custom := builtin(t, "boardroom")
	custom.Cover.Style = "custom"
	never := custom
	never.Cover.When = "never"
	cases := []struct {
		d     reportdesign.Design
		drawn string
		want  bool
	}{
		{custom, "band", true},
		{custom, "custom", false},
		{never, "", false},
		{builtin(t, "boardroom"), "split", false},
		{builtin(t, "plain"), "", false},
	}
	for _, c := range cases {
		v := renderView("t", c.d, &run.ReportRender{Cover: c.drawn})
		if v.Fallback != c.want {
			t.Errorf("%s asking %q, drawn %q: fallback %v", c.d.Name, c.d.Cover.Style, c.drawn, v.Fallback)
		}
	}
}

// TestRenderRefusesBeforeItStarts: a design the core refuses, a design the
// library does not hold and a malformed id never reach the container; a
// render's own failure is the page's to show.
func TestRenderRefusesBeforeItStarts(t *testing.T) {
	f := newFakeLibrary(t, true)
	id := f.add(builtin(t, "classic"), false)
	got := standInRender(t, &run.ReportRender{PDF: renderedBytes, SHA256: sha256Hex(renderedBytes)}, nil)
	pale := builtin(t, "classic")
	pale.Accent = "#f4f4f4"
	for name, call := range map[string]func() error{
		"pale": func() error {
			_, err := renderReportDesign(context.Background(), f.srv.URL, "agy_test", id, pale)
			return err
		},
		"missing": func() error {
			_, err := renderReportDesign(context.Background(), f.srv.URL, "agy_test", uuid.NewString(), builtin(t, "classic"))
			return err
		},
		"bad id": func() error {
			_, err := renderReportDesign(context.Background(), f.srv.URL, "agy_test", "../x", builtin(t, "classic"))
			return err
		},
	} {
		if err := call(); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("%d renders reached the container", len(*got))
	}

	standInRender(t, nil, errors.New("the design could not be laid out: export_pdf: Typst failed"))
	if _, err := renderReportDesign(context.Background(), f.srv.URL, "agy_test", id, builtin(t, "classic")); err == nil || !strings.Contains(err.Error(), "Typst failed") {
		t.Fatalf("a failed render: %v", err)
	}

}

// TestANewRenderStopsTheOneRunning: a render started while another runs
// stops it and waits for it, and only the newer one's PDF is kept.
func TestANewRenderStopsTheOneRunning(t *testing.T) {
	f := newFakeLibrary(t, true)
	id := f.add(builtin(t, "classic"), false)
	standInRender(t, nil, nil)
	started := make(chan struct{})
	first := true
	var mu sync.Mutex
	renderSample = func(ctx context.Context, _ []byte, _ io.Writer) (*run.ReportRender, error) {
		mu.Lock()
		blocking := first
		first = false
		mu.Unlock()
		if blocking {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &run.ReportRender{PDF: renderedBytes, SHA256: sha256Hex(renderedBytes), Pages: 1}, nil
	}
	var wg sync.WaitGroup
	var firstErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, firstErr = renderReportDesign(context.Background(), f.srv.URL, "agy_test", id, builtin(t, "classic"))
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first render never started")
	}
	v, err := renderReportDesign(context.Background(), f.srv.URL, "agy_test", id, builtin(t, "classic"))
	wg.Wait()
	if err != nil || v.Pages != 1 {
		t.Fatalf("the second render: %+v, %v", v, err)
	}
	if firstErr == nil || !strings.Contains(firstErr.Error(), "stopped") {
		t.Fatalf("the first render: %v", firstErr)
	}
	if _, err := renderedPDF(v.Token); err != nil {
		t.Fatal(err)
	}
	cancelRender()
	if _, err := renderedPDF(v.Token); err == nil {
		t.Fatal("a closed render was still read")
	}
}
