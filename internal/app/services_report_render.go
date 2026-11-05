package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry/pkg/reportdesign"
	"github.com/google/uuid"
)

// Render: the editor's design laid out on the sample report by the runner
// image's own export_pdf on this computer (run.RenderReportSample), so the
// person sees the real pages where the preview is a likeness. The design
// rendered is the one the editor holds, unsaved changes included, with the
// images the registry keeps for it, read with include=assets as a run reads
// it: the page never sends images. The PDF stays in Go once its checks pass
// (the ledger's PDF under reports/, %PDF-, its SHA-256); the page gets a
// token and the facts, and the viewer reads the bytes by that token, so the
// page cannot hand the viewer bytes of its own. One render runs at a time:
// a new one stops the one running and waits for it to end, and only the
// last one is kept.

// ReportRenderView is a render as the page shows it.
type ReportRenderView struct {
	// Token names the render's PDF for ReportRenderPDF.
	Token  string `json:"token"`
	Name   string `json:"name"`
	Pages  int    `json:"pages"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
	// Cover is the cover style drawn before the report, "" for none, and
	// CoverStyle the one the design asks for.
	Cover      string `json:"cover"`
	CoverStyle string `json:"coverStyle"`
	// Fallback says the design's own cover was asked for and the built-in
	// Cover was drawn in its place.
	Fallback bool `json:"fallback"`
	Logo     bool `json:"logo"`
}

// reportRender is the app's one render: the one running, and the last one
// done.
type reportRender struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	// done is closed when the running render ends.
	done  chan struct{}
	token string
	pdf   []byte
}

var renders reportRender

// renderSample is run.RenderReportSample; a test stands in for the
// container.
var renderSample = run.RenderReportSample

// RenderReportDesign lays the sample report out in the design the editor
// holds for the library's design id, with that design's stored images.
func (s *SettingsAPI) RenderReportDesign(url, id string, d reportdesign.Design) (*ReportRenderView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return renderReportDesign(s.app.ctx, url, token, id, d)
}

func renderReportDesign(ctx context.Context, url, token, id string, d reportdesign.Design) (*ReportRenderView, error) {
	did, err := designID(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := takeRender(cancel)
	defer func() {
		renders.mu.Lock()
		if renders.done == done {
			renders.cancel, renders.done = nil, nil
		}
		renders.mu.Unlock()
		close(done)
	}()

	doc, err := renderableDesign(ctx, url, token, did, d)
	if err != nil {
		return nil, err
	}
	r, err := renderSample(ctx, doc.Canonical(), io.Discard)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("the render was stopped")
		}
		return nil, err
	}
	t, err := renderToken()
	if err != nil {
		return nil, err
	}
	renders.mu.Lock()
	defer renders.mu.Unlock()
	if renders.done != done {
		return nil, errors.New("the render was stopped")
	}
	renders.token, renders.pdf = t, r.PDF
	return renderView(t, doc, r), nil
}

// takeRender makes the caller's render the running one: a render already
// running is stopped, and waited for, first. It answers the channel the
// caller closes when its render ends.
func takeRender(cancel context.CancelFunc) chan struct{} {
	done := make(chan struct{})
	for {
		renders.mu.Lock()
		if renders.done == nil {
			renders.cancel, renders.done = cancel, done
			renders.token, renders.pdf = "", nil
			renders.mu.Unlock()
			return done
		}
		prev, stop := renders.done, renders.cancel
		renders.mu.Unlock()
		stop()
		<-prev
	}
}

// renderableDesign is the editor's design with the stored design's images,
// held to the core's rules and to the pinned CLI's copy of them, as a run's
// design is (pickedReportTheme).
func renderableDesign(ctx context.Context, url, token string, id uuid.UUID, d reportdesign.Design) (reportdesign.Design, error) {
	client, err := reportClient(url, token)
	if err != nil {
		return reportdesign.Design{}, err
	}
	rctx, cancel := context.WithTimeout(ctx, reportDesignReadTimeout)
	defer cancel()
	rec, err := readDesign(rctx, client, url, id)
	if err != nil {
		return reportdesign.Design{}, fmt.Errorf("your report design could not be read (%w)", err)
	}
	stored, err := reportdesign.Parse(rec.Design)
	if err != nil {
		return reportdesign.Design{}, fmt.Errorf("your report design is not one this app can draw (%w; update the app)", err)
	}
	d = withStoredImages(d, stored)
	if err := d.Validate(); err != nil {
		return reportdesign.Design{}, err
	}
	if _, err := toolsrv.StampTheme(d.Canonical()); err != nil {
		return reportdesign.Design{}, fmt.Errorf("your report design is not one this app can draw (%w; update the app)", err)
	}
	return d, nil
}

// withStoredImages is the editor's design with the images the registry
// keeps for it, whatever images the page's copy carried.
func withStoredImages(d, stored reportdesign.Design) reportdesign.Design {
	d = d.WithoutAssets()
	d.Logo, d.CoverImage, d.CustomCover = stored.Logo, stored.CoverImage, stored.CustomCover
	return d
}

func renderView(token string, d reportdesign.Design, r *run.ReportRender) *ReportRenderView {
	return &ReportRenderView{
		Token: token, Name: d.Name, Pages: r.Pages, Bytes: len(r.PDF), SHA256: r.SHA256,
		Cover: r.Cover, CoverStyle: d.Cover.Style, Logo: r.Logo,
		Fallback: d.Cover.Style == "custom" && r.Cover != "" && r.Cover != "custom",
	}
}

func renderToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ReportRenderPDF is the last render's PDF, base64, for the app's viewer:
// only the bytes that passed the render's checks, and only by the token its
// view named.
func (s *SettingsAPI) ReportRenderPDF(token string) (string, error) {
	return renderedPDF(token)
}

func renderedPDF(token string) (string, error) {
	renders.mu.Lock()
	defer renders.mu.Unlock()
	if token == "" || token != renders.token || renders.pdf == nil {
		return "", errors.New("this render is no longer held: render the design again")
	}
	return base64.StdEncoding.EncodeToString(renders.pdf), nil
}

// CancelReportRender stops the render that is running, if one is, and
// lets go of the last render's PDF.
func (s *SettingsAPI) CancelReportRender() { cancelRender() }

func cancelRender() {
	renders.mu.Lock()
	defer renders.mu.Unlock()
	if renders.cancel != nil {
		renders.cancel()
	}
	renders.token, renders.pdf = "", nil
}
