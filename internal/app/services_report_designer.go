// DesignerAPI: Design with AI on Settings → Report design. The built-in
// Report Designer writes a whole report design from a brief (Design), or a
// custom cover page for one of the account's designs (Cover), In Agentry or
// on this computer, chosen in the dialog as Edit with AI and Create
// knowledge choose: the account keeps one library, so the place is a choice
// of the run, not of the page.
//
//   - In Agentry it is a hosted run of kind design_build at the registry's
//     flat price, the person's images uploaded as its inputs. The service
//     saves what it made to the library as a new design, never the default,
//     and the dialog shows that design's card read from the library by the
//     id the run's record names; a run that saved nothing says why in plain
//     words, refunded.
//   - On this computer it is the CLI's own run against the person's model,
//     for no credits, written to a folder of its own under the default
//     output folder. The dialog shows the result, and only the person's yes
//     adds it to the library, as a new design under a free name, never the
//     default.
//
// Either way Use, the library's own button, is what makes a design the
// default. The images come only from the native picker, and a run on this
// computer takes the run lock.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/blaktron/agentry/pkg/reportdesign"

	"github.com/blaktron/agentry-desktop/internal/designer"
	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/voice"
)

// eventDesignerLog carries a Report Designer run on this computer, line by
// line.
const eventDesignerLog = "designer:log"

// eventDesignerHosted carries a Report Designer run In Agentry's every state
// change and its final word.
const eventDesignerHosted = "designer:hosted"

// designerRunRef names a Report Designer run as the run in flight.
const designerRunRef = "the Report Designer"

// DesignerAPI is Design with AI's bound surface.
type DesignerAPI struct {
	app *App
	run *RunAPI
	// build is the Report Designer seam a test substitutes; nil is the CLI's.
	build designer.Builder
	// hosted is the Report Designer run In Agentry this app follows.
	hosted hostedSlot

	mu sync.Mutex
	// pending is the last result made on this computer, kept until the
	// person saves or discards it.
	pending *pendingDesign
}

// pendingDesign is a result made on this computer that is not in the
// library: the registry it is for, and the design with its images.
type pendingDesign struct {
	token    string
	registry string
	design   reportdesign.Design
}

// DesignerRequest is what the dialog starts a run with. From is the id of a
// design of the library (a Design starts from it, a Cover is written for
// it); Logo, Images and Document are files the native picker returned. A
// Document is refused where the Report Designer reads none
// (designer.DocumentHere).
type DesignerRequest struct {
	Job      string   `json:"job"`
	Brief    string   `json:"brief"`
	From     string   `json:"from"`
	Logo     string   `json:"logo"`
	Images   []string `json:"images"`
	URLs     []string `json:"urls"`
	Document string   `json:"document"`
}

// DesignerAvailabilityView answers where the Report Designer may run for a
// job: on this computer always, with the model it would use and whether AI
// Review can read web pages there; In Agentry where the registry offers it,
// with its flat price and the balance it is paid from.
type DesignerAvailabilityView struct {
	// Model is the runner policy's model; ReviewReady says the person keeps
	// a TypeSafe key, which a run here with web addresses needs.
	Model       string `json:"model"`
	ReviewReady bool   `json:"reviewReady"`
	ReviewWhy   string `json:"reviewWhy,omitempty"`
	// The input limits on this computer.
	MaxBriefBytes  int   `json:"maxBriefBytes"`
	MaxReferences  int   `json:"maxReferences"`
	MaxCoverImages int   `json:"maxCoverImages"`
	MaxURLs        int   `json:"maxUrls"`
	MaxImageBytes  int64 `json:"maxImageBytes"`

	// In Agentry.
	Offered    bool   `json:"offered"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
	Remedy     string `json:"remedy,omitempty"`
	UpgradeURL string `json:"upgradeUrl,omitempty"`
	// Credits is the flat price; HoldCredits what this payer's run holds
	// (0 for a member of an unmetered organization).
	Credits      int64  `json:"credits"`
	HoldCredits  int64  `json:"holdCredits"`
	Balance      int64  `json:"balance"`
	BalanceKnown bool   `json:"balanceKnown"`
	Pool         string `json:"pool,omitempty"`
	// HostedMaxImages is how many images a run In Agentry takes beside a
	// Design's logo; HostedURLs whether it reads web addresses, at most
	// HostedMaxURLs.
	HostedMaxImages int  `json:"hostedMaxImages"`
	HostedURLs      bool `json:"hostedUrls"`
	HostedMaxURLs   int  `json:"hostedMaxUrls,omitempty"`

	// DocumentHere and DocumentInAgentry say a Design may start from a
	// document there, at most MaxDocumentBytes.
	DocumentHere      bool  `json:"documentHere"`
	DocumentInAgentry bool  `json:"documentInAgentry"`
	MaxDocumentBytes  int64 `json:"maxDocumentBytes"`
}

// Availability answers the dialog for a job at reg.
func (a *DesignerAPI) Availability(reg, job string) (*DesignerAvailabilityView, error) {
	if job != designer.JobDesign && job != designer.JobCover {
		return nil, fmt.Errorf("%q is not design or cover", job)
	}
	out := &DesignerAvailabilityView{
		Model: designer.Model(), MaxBriefBytes: designer.MaxBriefBytes, MaxReferences: designer.MaxReferences,
		MaxCoverImages: designer.MaxCoverImages, MaxURLs: designer.MaxURLs, MaxImageBytes: designer.MaxImageBytes,
		DocumentHere: designer.DocumentHere, DocumentInAgentry: false, MaxDocumentBytes: designer.MaxDocumentBytes,
	}
	if st, err := draft.GetStatus(); err == nil && st.Key.Ready() {
		out.ReviewReady = true
	} else {
		out.ReviewWhy = designer.NoReviewKey
	}
	url, token, err := storedCredential(reg)
	if err != nil {
		// Signed out the library is not open, and In Agentry is absent.
		return out, nil //nolint:nilerr // signed out is an answer, not a failure
	}
	ctx := a.app.ctx
	f := hostedrun.DesignFacts{SignedIn: true}
	if lib, err := reportDesignLibrary(ctx, url, token); err == nil {
		f.Library = lib.Available
		f.Full = lib.Available && len(lib.Designs) >= lib.Max
	}
	if exts, err := extensionsOf(ctx, url); err == nil {
		f.Runs = hasString(exts, "runs")
	}
	if !f.Library || !f.Runs {
		a.decide(out, f, job, url)
		return out, nil
	}
	f.Scopes, f.ScopesKnown = tokenScopes(ctx, url, token)
	client := hostedrun.New(url, token)
	if st, err := client.Status(ctx); err == nil {
		f.StatusKnown, f.Terms = true, st.DesignBuild
		if t := st.DesignBuild; t != nil {
			out.Credits, out.HoldCredits, out.HostedMaxImages = t.Credits, t.Credits, t.MaxImages
			if u, ok := t.TakesURLs(); ok {
				out.HostedURLs, out.HostedMaxURLs = true, u.Max
			}
		}
		if st.Caller != nil && st.Caller.DesignBuild != nil {
			out.HoldCredits = st.Caller.DesignBuild.HoldCredits
		}
	}
	if runs, err := client.List(ctx); err == nil {
		for i := range runs {
			if hostedrun.Active(runs[i].Status) {
				f.Active = &runs[i]
				break
			}
		}
	}
	if sub, err := fetchSubscription(ctx, url, token); err == nil && sub != nil {
		out.Balance, out.BalanceKnown, out.Pool = sub.Balance, true, sub.MemberOf
	}
	a.decide(out, f, job, url)
	return out, nil
}

func (a *DesignerAPI) decide(out *DesignerAvailabilityView, f hostedrun.DesignFacts, job, url string) {
	o := hostedrun.DecideDesign(f, job)
	out.Offered, out.Available, out.Reason, out.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
	if o.Upgrade {
		out.UpgradeURL = url + "/settings/subscription?upgrade=pro"
	}
}

// pickDesignerImage is the system's file picker for one image; a test
// replaces it.
var pickDesignerImage = func(ctx context.Context, title string) (string, error) {
	return nativeOpenFile(ctx, pickOptions{
		Title:   title,
		Filters: []fileFilter{{Name: "PNG or JPEG images", Pattern: "*.png;*.jpg;*.jpeg"}},
	})
}

// ChooseImage opens the system's file picker for one PNG or JPEG and
// returns its path ("" when cancelled), marked as picked: a run takes no
// image the page names itself.
func (a *DesignerAPI) ChooseImage(title string) (string, error) {
	p, err := pickDesignerImage(a.app.ctx, title)
	if err != nil || p == "" {
		return "", err
	}
	a.app.markPicked(p)
	return p, nil
}

// pickDesignerDocument is the system's file picker for one document; a test
// replaces it.
var pickDesignerDocument = func(ctx context.Context, title string) (string, error) {
	return nativeOpenFile(ctx, pickOptions{
		Title:   title,
		Filters: []fileFilter{{Name: "PDF or Word documents", Pattern: "*.pdf;*.docx"}},
	})
}

// ChooseDocument opens the system's file picker for the one document a
// Design starts from and returns its path ("" when cancelled), marked as
// picked, once its bytes say it is a PDF or a Word document within 20 MB.
func (a *DesignerAPI) ChooseDocument(title string) (string, error) {
	p, err := pickDesignerDocument(a.app.ctx, title)
	if err != nil || p == "" {
		return "", err
	}
	if _, err := designer.CheckDocument(p); err != nil {
		return "", err
	}
	a.app.markPicked(p)
	return p, nil
}

// DesignerAddressesView is the dialog's check of the web addresses: each
// line, and on this computer whether AI Review can read the pages.
type DesignerAddressesView struct {
	Check       *voice.AddressCheck `json:"check"`
	ReviewReady bool                `json:"reviewReady"`
	ReviewWhy   string              `json:"reviewWhy,omitempty"`
}

// CheckAddresses answers DesignerAddressesView with the CLI's own parse and
// the Report Designer's count. Answered from the key's presence alone: no
// key is read.
func (a *DesignerAPI) CheckAddresses(urls []string) (*DesignerAddressesView, error) {
	c := voice.CheckAddresses(urls)
	c.Max = designer.MaxURLs
	if c.Problem == "" && len(knowledge.TrimmedLines(urls)) > designer.MaxURLs {
		c.Problem = fmt.Sprintf("a design takes at most %d web addresses", designer.MaxURLs)
	}
	out := &DesignerAddressesView{Check: c}
	if st, err := draft.GetStatus(); err == nil && st.Key.Ready() {
		out.ReviewReady = true
	} else {
		out.ReviewWhy = designer.NoReviewKey
	}
	return out, nil
}

// check holds a request to the rules both places share, every image to the
// native picker, and the document to its place, before anything is read.
func (a *DesignerAPI) check(req DesignerRequest) error {
	if req.Document != "" {
		return designer.ErrNoDocument
	}
	if err := designer.Check(req.Job, req.Brief, strings.TrimSpace(req.From) != "", req.Logo, req.Images, req.URLs); err != nil {
		return err
	}
	if err := a.app.requirePicked("the logo", req.Logo); err != nil {
		return err
	}
	for _, p := range req.Images {
		if p == "" {
			return errors.New("an image with no file")
		}
		if err := a.app.requirePicked("the image", p); err != nil {
			return err
		}
	}
	return nil
}

// DesignerResultView is a design made on this computer, shown before it is
// saved: the design as the editor's preview draws it, what the run said of
// it, and where it was written. Token names it for SaveResult.
type DesignerResultView struct {
	Token     string            `json:"token"`
	Job       string            `json:"job"`
	Name      string            `json:"name"`
	View      *ReportDesignView `json:"view"`
	Looked    bool              `json:"looked"`
	Thumbnail bool              `json:"thumbnail"`
	Previews  int               `json:"previews"`
	Dir       string            `json:"dir"`
	Pages     []voice.Page      `json:"pages,omitempty"`
}

// RunHere runs the Report Designer on this computer against the runner
// policy's model and answers what it made, which is not in the library
// until SaveResult. It takes the run lock; its log streams as designer:log.
func (a *DesignerAPI) RunHere(reg string, req DesignerRequest) (*DesignerResultView, error) {
	if err := a.check(req); err != nil {
		return nil, err
	}
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, err
	}
	var from *reportdesign.Design
	if strings.TrimSpace(req.From) != "" {
		d, err := libraryDesign(a.app.ctx, url, token, req.From)
		if err != nil {
			return nil, err
		}
		from = &d
		if err := designer.CheckCoverImages(req.Job, d, len(req.Images)); err != nil {
			return nil, err
		}
	}
	home, err := a.run.knowledge().Home()
	if err != nil {
		return nil, err
	}
	root, err := saveRoot()
	if err != nil {
		return nil, err
	}
	var res *designer.Result
	err = a.run.asRun(jobBuild, "a run of the Report Designer", designerRunRef, eventDesignerLog, func(ctx context.Context, runID string, logw io.Writer) error {
		out, err := newFolderIn(root, "report-designer")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(logw, "Its reports, receipts and the design it makes are written to %s.\n", out)
		res, err = designer.Run(ctx, designer.Request{
			Job: req.Job, Brief: req.Brief, From: from, Logo: req.Logo, Images: req.Images, URLs: req.URLs,
			Home: home, OutDir: out, RunID: runID, Build: a.build, Log: logw,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	tok, err := renderToken()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.pending = &pendingDesign{token: tok, registry: url, design: res.Design}
	a.mu.Unlock()
	return &DesignerResultView{
		Token: tok, Job: req.Job, Name: res.Design.Name, View: designPreview(res.Design), Looked: res.Looked,
		Thumbnail: res.Thumbnail, Previews: res.Previews, Dir: res.Dir, Pages: res.Pages,
	}, nil
}

// designPreview is a design not in the library as the editor's preview
// draws it: no id, never the default.
func designPreview(d reportdesign.Design) *ReportDesignView {
	v := reportDesignView(designRecord{Design: d.Canonical()})
	v.ID, v.Default = "", false
	return v
}

// libraryDesign is a design of the library with its images, by id.
func libraryDesign(ctx context.Context, url, token, id string) (reportdesign.Design, error) {
	did, err := designID(id)
	if err != nil {
		return reportdesign.Design{}, err
	}
	client, err := reportClient(url, token)
	if err != nil {
		return reportdesign.Design{}, err
	}
	rec, err := readDesign(ctx, client, url, did)
	if err != nil {
		return reportdesign.Design{}, err
	}
	d, err := reportdesign.Parse(rec.Design)
	if err != nil {
		return reportdesign.Design{}, fmt.Errorf("this design is not one this app can read (%v); update the app", err)
	}
	return d, nil
}

// StopHere stops the Report Designer run on this computer, never a run or
// a build of another kind.
func (a *DesignerAPI) StopHere() error {
	r := a.run
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	if r.live == nil || r.live.ref != designerRunRef {
		return errors.New("the Report Designer is not running on this computer")
	}
	r.live.cancel()
	return nil
}

// SaveResult adds the result made on this computer that token names to the
// library at reg: a new design under its own name, or that name with a
// number when the library has one of that name, never the default. Its
// images go with it, each re-encoded by the core.
func (a *DesignerAPI) SaveResult(reg, token string) (*ReportDesignView, error) {
	url, cred, err := storedCredential(reg)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	p := a.pending
	a.mu.Unlock()
	if p == nil || token == "" || p.token != token {
		return nil, errors.New("that design is no longer here to save: run the Report Designer again")
	}
	if p.registry != url {
		return nil, fmt.Errorf("that design was made for your library at %s", originOf(p.registry))
	}
	ctx := a.app.ctx
	lib, err := reportDesignLibrary(ctx, url, cred)
	if err != nil {
		return nil, err
	}
	if !lib.Available {
		return nil, fmt.Errorf("%s keeps no report designs", originOf(url))
	}
	if len(lib.Designs) >= lib.Max {
		return nil, fmt.Errorf("your library holds %d report designs, the most it keeps: delete one, then save this one", len(lib.Designs))
	}
	d := p.design
	d.Name = freeDesignName(lib, d.Name)
	if err := d.Validate(); err != nil {
		return nil, err
	}
	client, err := reportClient(url, cred)
	if err != nil {
		return nil, err
	}
	v, err := createDesignDocument(ctx, client, url, d.Canonical(), false)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	if a.pending == p {
		a.pending = nil
	}
	a.mu.Unlock()
	if v.Default {
		// The registry never makes a design the default unasked; a record
		// that says so is not one to keep quiet about.
		return v, fmt.Errorf("the registry saved %s as your default, which was not asked: choose your default with Use", d.Name)
	}
	return v, nil
}

// DiscardResult forgets the result made on this computer that token names;
// its folder stays where the run wrote it.
func (a *DesignerAPI) DiscardResult(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending != nil && a.pending.token == token {
		a.pending = nil
	}
}

// freeDesignName is name, or name with the first number from 2 that no
// design of the library has, within the core's bound on a name.
func freeDesignName(lib *ReportDesignLibrary, name string) string {
	taken := func(n string) bool {
		for _, d := range lib.Designs {
			if strings.EqualFold(d.Name, n) {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	for i := 2; ; i++ {
		suffix := fmt.Sprintf(" (%d)", i)
		base := []rune(name)
		if room := reportdesign.MaxNameRunes - utf8.RuneCountInString(suffix); len(base) > room {
			base = base[:room]
		}
		if n := strings.TrimSpace(string(base)) + suffix; !taken(n) {
			return n
		}
	}
}

// DesignerHostedView is a Report Designer run In Agentry as the dialog shows
// it: where it stands while it moves, then the design it saved or why it
// saved none, and what it cost.
type DesignerHostedView struct {
	ID              string `json:"id"`
	Op              string `json:"op"`
	Status          string `json:"status"`
	State           string `json:"state"`
	StatusLine      string `json:"statusLine"`
	CancelRequested bool   `json:"cancelRequested"`
	OutcomeLine     string `json:"outcomeLine,omitempty"`
	OutcomeTone     string `json:"outcomeTone,omitempty"`
	Error           string `json:"error,omitempty"`
	HoldCredits     int64  `json:"holdCredits"`
	SettledCredits  int64  `json:"settledCredits"`
	Settled         bool   `json:"settled"`
	// Applied says the design it made is in the library now, never the
	// default, as DesignID and Name; Note says why not, when it is not.
	Applied  bool   `json:"applied"`
	DesignID string `json:"designId,omitempty"`
	Name     string `json:"name,omitempty"`
	Note     string `json:"note,omitempty"`
	Final    bool   `json:"final"`
	// URLs are the web addresses it was given; PagesLine what its record
	// says of their page reads, "" for a run given none.
	URLs      []string `json:"urls,omitempty"`
	PagesLine string   `json:"pagesLine,omitempty"`
}

// DesignerHostedStartView is a started run, or the registry's refusal.
type DesignerHostedStartView struct {
	Run     *DesignerHostedView `json:"run,omitempty"`
	Refusal string              `json:"refusal,omitempty"`
	Detail  string              `json:"detail,omitempty"`
}

func designerHostedView(r *hostedrun.Run, final bool) *DesignerHostedView {
	rv := hostedRunView(r, "", nil, "", "", final)
	out := &DesignerHostedView{
		ID: r.ID, Status: r.Status, State: rv.State, StatusLine: rv.StatusLine, CancelRequested: r.CancelRequested,
		OutcomeLine: rv.OutcomeLine, OutcomeTone: rv.OutcomeTone, Error: r.Error,
		HoldCredits: r.HoldCredits, SettledCredits: rv.SettledCredits, Settled: rv.Settled, Final: final,
	}
	if b := r.DesignBuild; b != nil {
		out.Op, out.Applied, out.DesignID, out.Name = b.Op, b.Applied, b.DesignID, b.Name
		out.Note = hostedrun.DesignNoteSentence(b.Note)
		if !b.Applied && b.Note != "" && out.Note == "" {
			out.Note = "The Report Designer saved no design. Its credits were refunded."
		}
		out.URLs, out.PagesLine = b.URLs, designerPagesLine(b)
	}
	if final && out.Applied {
		out.OutcomeLine = "The Report Designer saved " + designNameWords(out.Name) + " to your library as a new design. It is not your default: Use makes it so."
		out.OutcomeTone = "ok"
	}
	return out
}

// designerPagesLine is what a run given web addresses read of them: the
// page reads admitted against its terms, and the texts AI Review withheld.
func designerPagesLine(b *hostedrun.DesignBuild) string {
	if len(b.URLs) == 0 || b.Pages == 0 {
		return ""
	}
	s := plural(b.Pages, "page read", "page reads")
	if b.MaxPages > 0 {
		s += fmt.Sprintf(" of the %d it may make", b.MaxPages)
	}
	if b.Withheld > 0 {
		s += "; AI Review withheld " + plural(b.Withheld, "text", "texts") + " from the Report Designer"
	}
	return s + "."
}

// designNameWords is a design's name in the outcome's words.
func designNameWords(name string) string {
	if name == "" {
		return "its design"
	}
	return "“" + name + "”"
}

// StartHosted runs the Report Designer In Agentry: the logo and the images
// (files the native picker returned) are uploaded as the run's inputs, the
// web addresses sent as its urls, then the run is created and followed in
// the background. AI Review reads each page through the registry before the
// run sees it.
func (a *DesignerAPI) StartHosted(reg string, req DesignerRequest) (*DesignerHostedStartView, error) {
	if err := a.check(req); err != nil {
		return nil, err
	}
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, err
	}
	from := ""
	if strings.TrimSpace(req.From) != "" {
		id, err := designID(req.From)
		if err != nil {
			return nil, err
		}
		from = id.String()
		if req.Job == designer.JobCover {
			d, err := libraryDesign(a.app.ctx, url, token, from)
			if err != nil {
				return nil, err
			}
			if err := designer.CheckCoverImages(req.Job, d, len(req.Images)); err != nil {
				return nil, err
			}
		}
	}
	addrs := knowledge.TrimmedLines(req.URLs)
	if err := a.hosted.claim("a Report Designer run In Agentry is already in progress: wait for it to finish, or cancel it"); err != nil {
		return nil, err
	}
	claimed := true
	defer func() {
		if claimed {
			a.hosted.release()
		}
	}()
	ctx := a.app.ctx
	client := hostedrun.New(url, token)
	// The registry's own terms decide what it takes, read again here
	// before anything is uploaded.
	st, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	t := st.DesignBuild
	switch {
	case !t.Runs(req.Job):
		return nil, errors.New("this registry is not running the Report Designer for that job: run it on this computer")
	case t.MaxImages > 0 && len(req.Images) > t.MaxImages:
		return nil, fmt.Errorf("the Report Designer In Agentry takes at most %d images", t.MaxImages)
	case t.MaxBriefBytes > 0 && len(req.Brief) > t.MaxBriefBytes:
		return nil, fmt.Errorf("a brief In Agentry is at most %d bytes", t.MaxBriefBytes)
	}
	if len(addrs) > 0 {
		u, ok := t.TakesURLs()
		switch {
		case !ok:
			return nil, errors.New("this registry's Report Designer does not read web addresses In Agentry: run it on this computer, or leave them out")
		case len(addrs) > u.Max:
			return nil, fmt.Errorf("the Report Designer In Agentry takes at most %d web addresses", u.Max)
		}
	}
	var paths []string
	if req.Logo != "" {
		paths = append(paths, req.Logo)
	}
	paths = append(paths, req.Images...)
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.Size() > designer.MaxImageBytes {
			return nil, fmt.Errorf("the image %s is over %d MB", filepath.Base(p), designer.MaxImageBytes>>20)
		}
	}
	cr := hostedrun.CreateRequest{Kind: hostedrun.KindDesignBuild, Op: req.Job, Brief: strings.TrimSpace(req.Brief), From: from, URLs: addrs}
	if len(paths) > 0 {
		files, err := hostedrun.StageInputs(paths)
		if err != nil {
			return nil, err
		}
		ids, err := client.UploadInputs(ctx, files)
		if err != nil {
			if r := hostedrun.RefusalOf(err); r != nil {
				return designerRefusal(r), nil
			}
			return nil, err
		}
		cr.Inputs = ids
		if req.Logo != "" {
			cr.Logo = ids[0]
		}
	}
	created, err := client.Create(ctx, cr)
	if err != nil {
		if r := hostedrun.RefusalOf(err); r != nil {
			return designerRefusal(r), nil
		}
		return nil, err
	}
	claimed = false
	a.followHosted(url, token, created)
	return &DesignerHostedStartView{Run: designerHostedView(created, false)}, nil
}

func designerRefusal(r *hostedrun.Refusal) *DesignerHostedStartView {
	out := &DesignerHostedStartView{Refusal: r.Slug, Detail: r.Detail}
	if out.Detail == "" {
		out.Detail = r.Sentence()
	}
	return out
}

// followHosted watches the run in the background and reports every status
// change as designer:hosted. Its design lands in the library, not here: the
// dialog reads it from the library by the record's id when the run is over.
func (a *DesignerAPI) followHosted(reg, token string, started *hostedrun.Run) {
	followHostedRun(&a.hosted, reg, token, started, func(r *hostedrun.Run, final bool, lost error) {
		out := designerHostedView(r, final)
		if lost != nil {
			out.Error = "lost touch with the registry while the Report Designer ran (" + lost.Error() + "); open your library again to see what it saved"
		}
		emitEvent(a.app, eventDesignerHosted, out)
	})
}

// AttachHosted follows this account's Report Designer run In Agentry in
// flight, if it has one, so a library opened again shows it and its Cancel
// rather than offering a second. The URL is held to Settings' rule before
// its token is looked up.
func (a *DesignerAPI) AttachHosted(reg string) (*DesignerHostedStartView, error) {
	url, token, err := storedCredential(reg)
	if err != nil {
		return &DesignerHostedStartView{}, nil //nolint:nilerr // signed out follows nothing
	}
	client := hostedrun.New(url, token)
	if cur := a.hosted.current(); cur != nil {
		if cur.id == "" {
			// A start holds the slot; its own events report the run.
			return &DesignerHostedStartView{}, nil
		}
		if fresh, err := client.Get(a.app.ctx, cur.id); err == nil {
			return &DesignerHostedStartView{Run: designerHostedView(fresh, false)}, nil
		}
		return &DesignerHostedStartView{}, nil
	}
	if err := a.hosted.claim("busy"); err != nil {
		return &DesignerHostedStartView{}, nil //nolint:nilerr // a start took the slot first
	}
	runs, err := client.List(a.app.ctx)
	if err != nil {
		a.hosted.release()
		return &DesignerHostedStartView{}, nil //nolint:nilerr // no list, nothing to follow
	}
	for i := range runs {
		r := &runs[i]
		if r.IsDesignBuild() && hostedrun.Active(r.Status) {
			a.followHosted(url, token, r)
			return &DesignerHostedStartView{Run: designerHostedView(r, false)}, nil
		}
	}
	a.hosted.release()
	return &DesignerHostedStartView{}, nil
}

// CancelHosted asks the registry to cancel the Report Designer run In
// Agentry this app follows. What it held is settled as the registry settles
// a cancelled run.
func (a *DesignerAPI) CancelHosted() (*DesignerHostedView, error) {
	cur := a.hosted.current()
	if cur == nil || cur.id == "" {
		return nil, errors.New("no Report Designer run In Agentry is in progress")
	}
	_, token, err := storedCredential(cur.registry)
	if err != nil {
		return nil, fmt.Errorf("sign in to %s to cancel this run", regHost(cur.registry))
	}
	r, err := hostedrun.New(cur.registry, token).Cancel(a.app.ctx, cur.id)
	if err != nil {
		return nil, err
	}
	return designerHostedView(r, false), nil
}
