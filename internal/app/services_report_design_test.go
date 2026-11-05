package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry/pkg/reportdesign"
	"github.com/google/uuid"
)

// fakeDesign is one design of the fake registry's library, kept as the
// core keeps it: the document without its images, each image beside it.
type fakeDesign struct {
	id      uuid.UUID
	doc     reportdesign.Design
	logo    reportdesign.Logo
	cover   reportdesign.Image
	custom  reportdesign.CustomCover
	def     bool
	created time.Time
	updated time.Time
	seq     int
}

// fakeLibrary answers the library routes as the core does: the list, a
// create (409 past ten, 422 for a document the core refuses, the images
// carried in the document kept), a read (the images only on
// include=assets), a save (the stored images kept), a delete, the default,
// and each image's upload and removal. found=false is a registry that has
// no library. The fake records the last part name an upload sent
// and the last create's body.
type fakeLibrary struct {
	mu       sync.Mutex
	found    bool
	designs  map[uuid.UUID]*fakeDesign
	seq      int
	partName string
	lastPost string
	srv      *httptest.Server
}

// requests counts every request any fake registry of this file answered.
var requests atomic.Int64

func newFakeLibrary(t *testing.T, found bool) *fakeLibrary {
	t.Helper()
	f := &fakeLibrary{found: found, designs: map[uuid.UUID]*fakeDesign{}}
	mux := http.NewServeMux()
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		requests.Add(1)
		if !f.found {
			w.WriteHeader(http.StatusNotFound)
			return false
		}
		if r.Header.Get("Authorization") != "Bearer agy_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	refuse := func(w http.ResponseWriter, status int, detail string) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"type":"about:blank","title":"refused","status":` + strconv.Itoa(status) + `,"detail":` + strconv.Quote(detail) + `}`))
	}
	byID := func(w http.ResponseWriter, r *http.Request) *fakeDesign {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			refuse(w, http.StatusNotFound, "no such design")
			return nil
		}
		d := f.designs[id]
		if d == nil {
			refuse(w, http.StatusNotFound, "no such design")
		}
		return d
	}
	record := func(w http.ResponseWriter, r *http.Request, status int, d *fakeDesign) {
		doc := d.doc
		if r.URL.Query().Get("include") == "assets" {
			doc.Logo, doc.CoverImage, doc.CustomCover = d.logo, d.cover, d.custom
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id":"` + d.id.String() + `","default":` + strconv.FormatBool(d.def) +
			`,"hasLogo":` + strconv.FormatBool(d.logo != reportdesign.Logo{}) +
			`,"hasCoverImage":` + strconv.FormatBool(d.cover != reportdesign.Image{}) +
			`,"hasCustomCover":` + strconv.FormatBool(!d.custom.IsZero()) +
			`,"createdAt":"` + d.created.Format(time.RFC3339) + `","updatedAt":"` + d.updated.Format(time.RFC3339) +
			`","design":` + string(doc.Canonical()) + `}`))
	}
	list := func(w http.ResponseWriter) {
		type summary struct {
			ID             uuid.UUID `json:"id"`
			Name           string    `json:"name"`
			BasedOn        string    `json:"basedOn"`
			Version        int       `json:"version"`
			Default        bool      `json:"default"`
			HasLogo        bool      `json:"hasLogo"`
			HasCoverImage  bool      `json:"hasCoverImage"`
			HasCustomCover bool      `json:"hasCustomCover"`
			CreatedAt      time.Time `json:"createdAt"`
			UpdatedAt      time.Time `json:"updatedAt"`
		}
		out := struct {
			Default *uuid.UUID `json:"default,omitempty"`
			Designs []summary  `json:"designs"`
		}{Designs: []summary{}}
		ordered := make([]*fakeDesign, 0, len(f.designs))
		for _, d := range f.designs {
			ordered = append(ordered, d)
		}
		for i := range ordered {
			for j := i + 1; j < len(ordered); j++ {
				if ordered[j].seq < ordered[i].seq {
					ordered[i], ordered[j] = ordered[j], ordered[i]
				}
			}
		}
		for _, d := range ordered {
			if d.def {
				id := d.id
				out.Default = &id
			}
			out.Designs = append(out.Designs, summary{ID: d.id, Name: d.doc.Name, BasedOn: d.doc.BasedOn, Version: d.doc.Version, Default: d.def,
				HasLogo: d.logo != reportdesign.Logo{}, HasCoverImage: d.cover != reportdesign.Image{}, HasCustomCover: !d.custom.IsZero(),
				CreatedAt: d.created, UpdatedAt: d.updated})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
	// parse reads a design body's document as the core does: the document
	// held to every rule, images included.
	parse := func(w http.ResponseWriter, raw json.RawMessage) (reportdesign.Design, bool) {
		d, err := reportdesign.Parse(raw)
		if err != nil {
			refuse(w, http.StatusUnprocessableEntity, "design: "+err.Error())
			return d, false
		}
		return d, true
	}
	mux.HandleFunc("GET /api/v1/user/report-designs", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		list(w)
	})
	mux.HandleFunc("POST /api/v1/user/report-designs", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		raw, _ := io.ReadAll(r.Body)
		f.lastPost = string(raw)
		var body struct {
			Default bool            `json:"default"`
			Design  json.RawMessage `json:"design"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			refuse(w, http.StatusBadRequest, "not JSON")
			return
		}
		d, ok := parse(w, body.Design)
		if !ok {
			return
		}
		if len(f.designs) >= reportdesign.MaxLibrary {
			refuse(w, http.StatusConflict, "Your library holds ten designs; delete one first.")
			return
		}
		f.seq++
		now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC).Add(time.Duration(f.seq) * time.Minute)
		fd := &fakeDesign{id: uuid.New(), doc: d.WithoutAssets(), logo: d.Logo, cover: d.CoverImage, custom: d.CustomCover, created: now, updated: now, seq: f.seq}
		if body.Default {
			for _, o := range f.designs {
				o.def = false
			}
			fd.def = true
		}
		f.designs[fd.id] = fd
		record(w, r, http.StatusCreated, fd)
	})
	mux.HandleFunc("PUT /api/v1/user/report-designs/default", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var body struct {
			ID *uuid.UUID `json:"id"`
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			refuse(w, http.StatusBadRequest, "not JSON")
			return
		}
		if body.ID != nil && f.designs[*body.ID] == nil {
			refuse(w, http.StatusNotFound, "no such design")
			return
		}
		for id, d := range f.designs {
			d.def = body.ID != nil && id == *body.ID
		}
		list(w)
	})
	mux.HandleFunc("GET /api/v1/user/report-designs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if d := byID(w, r); d != nil {
			record(w, r, http.StatusOK, d)
		}
	})
	mux.HandleFunc("PUT /api/v1/user/report-designs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		d := byID(w, r)
		if d == nil {
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Design json.RawMessage `json:"design"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			refuse(w, http.StatusBadRequest, "not JSON")
			return
		}
		nd, ok := parse(w, body.Design)
		if !ok {
			return
		}
		d.doc = nd.WithoutAssets()
		d.updated = d.updated.Add(time.Minute)
		record(w, r, http.StatusOK, d)
	})
	mux.HandleFunc("DELETE /api/v1/user/report-designs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if d := byID(w, r); d != nil {
			delete(f.designs, d.id)
			w.WriteHeader(http.StatusNoContent)
		}
	})
	upload := func(w http.ResponseWriter, r *http.Request, maxUpload int) ([]byte, *fakeDesign) {
		d := byID(w, r)
		if d == nil {
			return nil, nil
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			refuse(w, http.StatusBadRequest, "Send the image as one multipart part named file.")
			return nil, nil
		}
		f.partName = hdr.Filename
		data, _ := io.ReadAll(file)
		if len(data) > maxUpload {
			refuse(w, http.StatusRequestEntityTooLarge, "This image is too large.")
			return nil, nil
		}
		return data, d
	}
	imageErr := func(w http.ResponseWriter, err error) {
		if errors.Is(err, reportdesign.ErrImageTooLarge) {
			refuse(w, http.StatusRequestEntityTooLarge, "This image is too large.")
			return
		}
		refuse(w, http.StatusUnsupportedMediaType, "An image is a PNG or a JPEG. SVG is not accepted.")
	}
	mux.HandleFunc("PUT /api/v1/user/report-designs/{id}/logo", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		data, d := upload(w, r, reportdesign.MaxLogoUploadBytes)
		if d == nil {
			return
		}
		logo, err := reportdesign.NormalizeLogo(data)
		if err != nil {
			imageErr(w, err)
			return
		}
		d.logo = logo
		r.URL.RawQuery = "include=assets"
		record(w, r, http.StatusOK, d)
	})
	mux.HandleFunc("DELETE /api/v1/user/report-designs/{id}/logo", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if d := byID(w, r); d != nil {
			d.logo = reportdesign.Logo{}
			r.URL.RawQuery = "include=assets"
			record(w, r, http.StatusOK, d)
		}
	})
	mux.HandleFunc("PUT /api/v1/user/report-designs/{id}/cover-image", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		data, d := upload(w, r, reportdesign.MaxCoverImageUploadBytes)
		if d == nil {
			return
		}
		img, err := reportdesign.NormalizeCoverImage(data)
		if err != nil {
			imageErr(w, err)
			return
		}
		d.cover = img
		r.URL.RawQuery = "include=assets"
		record(w, r, http.StatusOK, d)
	})
	mux.HandleFunc("DELETE /api/v1/user/report-designs/{id}/cover-image", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if d := byID(w, r); d != nil {
			d.cover = reportdesign.Image{}
			r.URL.RawQuery = "include=assets"
			record(w, r, http.StatusOK, d)
		}
	})
	mux.HandleFunc("DELETE /api/v1/user/report-designs/{id}/custom-cover", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if d := byID(w, r); d != nil {
			d.custom = reportdesign.CustomCover{}
			r.URL.RawQuery = "include=assets"
			record(w, r, http.StatusOK, d)
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// add puts a design in the fake's library straight, as the site or the CLI
// would have, and answers its id.
func (f *fakeLibrary) add(d reportdesign.Design, def bool) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC).Add(time.Duration(f.seq) * time.Minute)
	fd := &fakeDesign{id: uuid.New(), doc: d.WithoutAssets(), logo: d.Logo, cover: d.CoverImage, custom: d.CustomCover, def: def, created: now, updated: now, seq: f.seq}
	if def {
		for _, o := range f.designs {
			o.def = false
		}
	}
	f.designs[fd.id] = fd
	return fd.id.String()
}

// stored is a design of the fake, whole with its images.
func (f *fakeLibrary) stored(id string) reportdesign.Design {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := f.designs[uuid.MustParse(id)]
	doc := d.doc
	doc.Logo, doc.CoverImage, doc.CustomCover = d.logo, d.cover, d.custom
	return doc
}

// builtin is a built-in design, failing the test when there is none.
func builtin(t *testing.T, id string) reportdesign.Design {
	t.Helper()
	d, ok := reportdesign.Builtin(id)
	if !ok {
		t.Fatalf("no built-in %q", id)
	}
	return d
}

// throughThePage is a design as the page hands it back: through JSON, as
// Wails sends a binding's argument.
func throughThePage(t *testing.T, d reportdesign.Design) reportdesign.Design {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var back reportdesign.Design
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	return back
}

// The editor's choices are the core's: its ten kits in its order, each with
// both thumbnails from the pinned CLI, its families with the roles each may
// set, its closed lists, its tokens and its limits.
func TestReportDesignOptionsAreTheCores(t *testing.T) {
	o := reportDesignOptions()
	var ids []string
	for _, k := range o.Kits {
		ids = append(ids, k.ID)
		if !strings.HasPrefix(k.Cover, "data:image/png;base64,iVBORw0KGgo") || !strings.HasPrefix(k.Page, "data:image/png;base64,iVBORw0KGgo") {
			t.Fatalf("%s: thumbnails %q, %q", k.ID, k.Cover[:30], k.Page[:30])
		}
		want, _ := reportdesign.BuiltinDocument(k.ID)
		if !bytes.Equal(k.Design.Canonical(), want) {
			t.Fatalf("%s: the kit is not the core's document", k.ID)
		}
	}
	if got := strings.Join(ids, ","); got != strings.Join(reportdesign.BuiltinIDs(), ",") || len(ids) != 10 || o.DefaultKitID != "plain" {
		t.Fatalf("kits %s, default %s", got, o.DefaultKitID)
	}
	if len(o.Families) != len(reportdesign.Families()) || len(o.CodeFonts) != 4 || strings.Join(o.Papers, ",") != "a4,letter" {
		t.Fatalf("options %+v", o)
	}
	for _, f := range o.BodyFonts {
		if f == "Playfair Display" || f == "Fraunces" || f == "JetBrains Mono" {
			t.Fatalf("%s offered for body text", f)
		}
	}
	if strings.Join(o.Tokens, ",") != "title,author,date,page,pages,brand,section,confidentiality" {
		t.Fatalf("tokens %v", o.Tokens)
	}
	if len(o.CoverStyles) != 7 || o.CoverStyles[0].Name != "band" || !o.CoverStyles[0].AccentOnText || o.CoverStyles[1].Name != "split" || !o.CoverStyles[1].SecondaryOnText {
		t.Fatalf("cover styles %+v", o.CoverStyles)
	}
	if strings.Join(o.HeaderStyles, ",") != "none,rule,brandLine,titleAndPage,logoLeft" || strings.Join(o.CoverWhens, ",") != "long,always,never" {
		t.Fatalf("lists %+v %+v", o.HeaderStyles, o.CoverWhens)
	}
	if o.MaxName != 60 || o.MaxLine != 120 || o.MaxContactLines != 4 || o.SlotCount != 3 || o.MaxLibrary != 10 {
		t.Fatalf("limits %+v", o)
	}
}

// The page's checks are the core's: an edited kit is said as one; a colour
// the core refuses is refused with its words and the nearest shade that
// passes, and the shade it offers is saved by the core; whether a colour
// carries text decides the ratio it needs, for the accent and the secondary
// alike; the slots are expanded from the sample's facts and the design's
// brand, as the preview draws them.
func TestCheckReportDesignIsTheCoresVerdict(t *testing.T) {
	d := builtin(t, "classic")
	d.Name = "Ours"
	if c := checkReportDesign(d); c.Problem != nil || c.Unedited || c.Label != "Ours (from Classic)" {
		t.Fatalf("renamed: %+v", c)
	}
	d.Name = "Classic"
	d.Paper = "letter"
	if c := checkReportDesign(d); c.Label != "Classic, edited" {
		t.Fatalf("edited: %+v", c)
	}
	if c := checkReportDesign(builtin(t, "lumen")); !c.Unedited || c.Label != "Lumen" || c.Problem != nil {
		t.Fatalf("a kit: %+v", c)
	}

	// Classic sets small-capital headings in the accent: text, 4.5:1.
	d.Accent = "#7fa7d9"
	c := checkReportDesign(d)
	if c.Problem == nil || c.Problem.Field != "accent" || c.Accent.OK || !c.Accent.OnText || c.Accent.Need != 4.5 {
		t.Fatalf("pale accent on text: %+v", c)
	}
	if want := d.Validate().Error(); !strings.HasSuffix(want, c.Problem.Message) {
		t.Fatalf("the page's words %q are not the core's %q", c.Problem.Message, want)
	}
	if !strings.Contains(c.Problem.Message, c.Accent.Nearest) {
		t.Fatalf("the offered shade %s is not the one the core names: %s", c.Accent.Nearest, c.Problem.Message)
	}
	d.Accent = c.Accent.Nearest
	if c := checkReportDesign(d); c.Problem != nil || !c.Accent.OK {
		t.Fatalf("the offered shade must pass: %+v", c)
	}

	// The same pale accent on rules only (graphics) needs 3:1: no cover
	// drawn, a ruled title and rule headings.
	d.Accent, d.Headings, d.TitleBlock, d.Cover.When = "#7fa7d9", "rule", "ruled", "never"
	if c := checkReportDesign(d); c.Accent.OnText || c.Accent.Need != 3 {
		t.Fatalf("accent on graphics: %+v", c)
	}
	// A band cover drawn always puts the accent on text again.
	d.Cover.Style, d.Cover.When = "band", "always"
	if c := checkReportDesign(d); !c.Accent.OnText || c.Accent.Need != 4.5 {
		t.Fatalf("accent on a band cover: %+v", c)
	}

	// The secondary: none is not checked; on a split cover it carries the
	// brand, so it needs 4.5:1; a pale one is refused in the core's words.
	d = builtin(t, "boardroom")
	if c := checkReportDesign(d); !c.Secondary.Set || !c.Secondary.OnText || c.Secondary.Need != 4.5 || !c.Secondary.OK || c.SecondaryTint == "" {
		t.Fatalf("Boardroom's secondary: %+v", c)
	}
	d.Secondary = "#f2d27a"
	c = checkReportDesign(d)
	if c.Problem == nil || c.Problem.Field != "secondary" || c.Secondary.OK || !strings.Contains(c.Problem.Message, c.Secondary.Nearest) {
		t.Fatalf("pale secondary: %+v", c)
	}
	d.Secondary = ""
	if c := checkReportDesign(d); c.Secondary.Set || c.SecondaryTint != "" || c.Problem != nil || !c.Accent.OnText {
		t.Fatalf("no secondary: a split cover puts the accent on text: %+v", c)
	}

	d = builtin(t, "plain")
	d.Accent = "blue"
	if c := checkReportDesign(d); c.Accent.Valid || c.Problem == nil || c.Problem.Field != "accent" || c.Tint != "" {
		t.Fatalf("not hex: %+v", c)
	}
	d = builtin(t, "modern")
	if c := checkReportDesign(d); c.Tint == "" || c.Tint == d.Accent {
		t.Fatalf("tint %q", c.Tint)
	}
	d.Footer.Slots[0] = "line one\nline two"
	if c := checkReportDesign(d); c.Problem == nil || c.Problem.Field != "footer.slots.0" {
		t.Fatalf("two-line slot: %+v", c)
	}

	// The slots as the preview draws them: tokens from the sample and the
	// brand, anything else literal.
	d = builtin(t, "plain")
	d.Brand.Name, d.Brand.Confidentiality = "Acme & Co", "Internal"
	d.Header.Slots = [3]string{"{brand}", "{section}", "{title} <b>"}
	d.Footer.Slots = [3]string{"{confidentiality}", "{date}", "Page {page} of {pages} {nope}"}
	c = checkReportDesign(d)
	if strings.Join(c.Header, "|") != "Acme & Co|Summary|Quarterly review <b>" || strings.Join(c.Footer, "|") != "Internal|4 November 2026|Page 2 of 4 {nope}" {
		t.Fatalf("slots %v %v", c.Header, c.Footer)
	}
}

// The library and the editor: an empty library lists nothing and is not
// Plain's absence; each kit, loaded into the editor and sent back, is
// added as the core's own document; the list, the read with its images, a
// save that keeps the images, the default, a duplicate that carries the
// images, a delete, and the eleventh refused in the registry's words; a
// design a later core wrote is said, not shown.
func TestReportDesignLibraryAndEditor(t *testing.T) {
	f := newFakeLibrary(t, true)
	ctx := context.Background()
	url := f.srv.URL
	lib, err := reportDesignLibrary(ctx, url, "agy_test")
	if err != nil || !lib.Available || len(lib.Designs) != 0 || lib.Default != "" || lib.Max != 10 {
		t.Fatalf("empty: %+v, %v", lib, err)
	}
	var ids []string
	for i, id := range reportdesign.BuiltinIDs() {
		d := throughThePage(t, builtin(t, id))
		v, err := createReportDesign(ctx, url, "agy_test", d, i == 2)
		if err != nil || v.Design == nil || v.Label != d.Name || v.Logo != "" || v.CustomCover != nil {
			t.Fatalf("create %s: %+v, %v", id, v, err)
		}
		doc, _ := reportdesign.BuiltinDocument(id)
		if got := f.stored(v.ID); !bytes.Equal(got.Canonical(), doc) {
			t.Fatalf("create %s stored %s", id, got.Canonical())
		}
		if !strings.Contains(f.lastPost, `"default":`+strconv.FormatBool(i == 2)) {
			t.Fatalf("create %s sent %s", id, f.lastPost)
		}
		ids = append(ids, v.ID)
	}
	lib, err = reportDesignLibrary(ctx, url, "agy_test")
	if err != nil || len(lib.Designs) != 10 || lib.Default != ids[2] || !lib.Designs[2].Default || lib.Designs[0].Name != "Plain" || lib.Designs[9].BasedOn != "lumen" {
		t.Fatalf("ten: %+v, %v", lib, err)
	}
	if _, err := createReportDesign(ctx, url, "agy_test", builtin(t, "plain"), false); err == nil || !strings.Contains(err.Error(), "ten designs") {
		t.Fatalf("the eleventh: %v", err)
	}

	// A save keeps the stored images and sends none; the core's refusal
	// is given before anything is sent.
	before := requests.Load()
	bad := builtin(t, "modern")
	bad.Accent = "#9ec5ff"
	if _, err := saveReportDesign(ctx, url, "agy_test", ids[2], bad); err == nil || !strings.Contains(err.Error(), "accent") || requests.Load() != before {
		t.Fatalf("pale accent: %v (%d requests)", err, requests.Load()-before)
	}
	edited := builtin(t, "modern")
	edited.Name, edited.Brand.Name, edited.Footer.Slots[0] = "House", "Acme & Co <internal>", "{brand}"
	edited.Logo = reportdesign.Logo{PNG: "not sent"}
	v, err := saveReportDesign(ctx, url, "agy_test", ids[2], edited)
	if err != nil || v.Label != "House (from Modern)" || !v.Default || v.Design.Logo != (reportdesign.Logo{}) {
		t.Fatalf("save edited: %+v, %v", v, err)
	}
	if got := f.stored(ids[2]); !bytes.Equal(got.Canonical(), edited.WithoutAssets().Canonical()) {
		t.Fatalf("stored %s", got.Canonical())
	}

	// The read answers the images beside the document, never in it.
	withImages := builtin(t, "field")
	withImages.Name = "Field notes"
	withImages.Logo, _ = reportdesign.NormalizeLogo(testPNG(t, 120, 40))
	withImages.CoverImage, _ = reportdesign.NormalizeCoverImage(testJPEG(t, 300, 200))
	withImages.CustomCover = reportdesign.CustomCover{Source: "#let cover(meta, design) = [Hello]", Thumbnail: reportdesign.ImageOf(testPNG(t, 20, 28), "png")}
	fid := f.add(withImages, false)
	v, err = openReportDesign(ctx, url, "agy_test", fid)
	if err != nil || v.ID != fid || v.Design == nil || v.Design.Logo != (reportdesign.Logo{}) || v.Design.CoverImage != (reportdesign.Image{}) || !v.Design.CustomCover.IsZero() {
		t.Fatalf("open: %+v, %v", v, err)
	}
	if !strings.HasPrefix(v.Logo, "data:image/png;base64,iVBORw0KGgo") || !strings.HasPrefix(v.CoverImage, "data:image/jpeg;base64,/9j/") ||
		v.CustomCover == nil || !strings.HasPrefix(v.CustomCover.Thumbnail, "data:image/png;base64,") || v.CustomCover.SourceBytes != len(withImages.CustomCover.Source) {
		t.Fatalf("open's images: %q %q %+v", v.Logo[:30], v.CoverImage[:30], v.CustomCover)
	}
	if v.Label != "Field notes (from Field)" || v.Design.SHA256() != withImages.WithoutAssets().SHA256() {
		t.Fatalf("open: %+v", v)
	}

	// A duplicate carries every image and is named after the original,
	// never the default.
	if _, err := duplicateReportDesign(ctx, url, "agy_test", ids[0]); err == nil {
		t.Fatal("the library is full: an eleventh is refused")
	}
	lib, err = deleteReportDesign(ctx, url, "agy_test", ids[9])
	if err != nil || len(lib.Designs) != 10 {
		t.Fatalf("delete: %+v, %v", lib, err)
	}
	if lib, err = deleteReportDesign(ctx, url, "agy_test", ids[8]); err != nil || len(lib.Designs) != 9 {
		t.Fatalf("delete: %+v, %v", lib, err)
	}
	if _, err := deleteReportDesign(ctx, url, "agy_test", ids[8]); err == nil || !strings.Contains(err.Error(), "no such design") {
		t.Fatalf("delete again: %v", err)
	}
	copy, err := duplicateReportDesign(ctx, url, "agy_test", fid)
	if err != nil || copy.Default || copy.Design.Name != "Field notes copy" || copy.Logo == "" || copy.CoverImage == "" || copy.CustomCover == nil {
		t.Fatalf("duplicate: %+v, %v", copy, err)
	}
	want := withImages
	want.Name = "Field notes copy"
	if got := f.stored(copy.ID); got.SHA256() != want.SHA256() {
		t.Fatalf("the copy is not the original with its images: %s", got.Canonical())
	}
	if got := designCopyName(strings.Repeat("x", 60)); len([]rune(got)) != 60 || !strings.HasSuffix(got, " copy") {
		t.Fatalf("a long name's copy: %q", got)
	}

	// The default: another, then none.
	lib, err = setDefaultReportDesign(ctx, url, "agy_test", fid)
	if err != nil || lib.Default != fid {
		t.Fatalf("set default: %+v, %v", lib, err)
	}
	lib, err = setDefaultReportDesign(ctx, url, "agy_test", "")
	if err != nil || lib.Default != "" {
		t.Fatalf("no default: %+v, %v", lib, err)
	}
	if _, err := setDefaultReportDesign(ctx, url, "agy_test", "not-an-id"); err == nil || !strings.Contains(err.Error(), "not one of your report designs") {
		t.Fatalf("a bad id: %v", err)
	}

	// A later core's document is said, not shown.
	f.mu.Lock()
	f.designs[uuid.MustParse(fid)].doc.AgentryTheme = 4
	f.mu.Unlock()
	v, err = openReportDesign(ctx, url, "agy_test", fid)
	if err != nil || v.Design != nil || !strings.Contains(v.Unreadable, "agentryTheme") {
		t.Fatalf("a later document: %+v, %v", v, err)
	}
	if _, err := openReportDesign(ctx, url, "agy_wrong", fid); err == nil {
		t.Fatal("a refused credential must be an error")
	}
	old := newFakeLibrary(t, false)
	if lib, err := reportDesignLibrary(ctx, old.srv.URL, "agy_test"); err != nil || lib.Available {
		t.Fatalf("no library: %+v, %v", lib, err)
	}
}

// A local run takes the design the person picked, whole with its images,
// and the pinned CLI stamps it as the core hashes it, which is the digest
// its receipt names; with no pick it takes the account's default, and with
// no default it is Plain. A pick that cannot be read, that the library does
// not hold, that this build cannot draw, or that was made signed out
// refuses the run, since the person chose it. A default an account it
// cannot read is Plain, said in the run's log, as is a registry address
// Settings would not accept: how a PDF looks is no reason to stop a run the
// person chose no look for.
func TestALocalRunTakesThePickedDesign(t *testing.T) {
	f := newFakeLibrary(t, true)
	url := f.srv.URL
	ctx := context.Background()
	var log strings.Builder
	take := func(token, picked string) engine.ReportTheme {
		t.Helper()
		got, err := accountReportTheme(ctx, url, token, picked, &log)
		if err != nil {
			t.Fatalf("picked %q: %v", picked, err)
		}
		return got
	}
	refused := func(token, picked, want string) {
		t.Helper()
		got, err := accountReportTheme(ctx, url, token, picked, &log)
		if err == nil || got.Design != nil || !strings.Contains(err.Error(), "picked for this run cannot be used") || !strings.Contains(err.Error(), want) {
			t.Fatalf("picked %q: %+v, %v (want %q)", picked, got, err, want)
		}
	}

	if got := take("agy_test", ""); got.Design != nil || got.Name != "" || got.From != "" || log.Len() != 0 {
		t.Fatalf("no design: %+v %q, want the zero value (Plain by default)", got, log.String())
	}
	ours := builtin(t, "modern")
	ours.Name, ours.Accent, ours.Fonts.Body = "House", "#0f766e", "Lora"
	ours.Logo, _ = reportdesign.NormalizeLogo(testPNG(t, 120, 40))
	ours.CoverImage, _ = reportdesign.NormalizeCoverImage(testPNG(t, 300, 200))
	classic := f.add(builtin(t, "classic"), false)
	house := f.add(ours, false)
	if got := take("agy_test", ""); got.Design != nil || log.Len() != 0 {
		t.Fatalf("no default: %+v %q", got, log.String())
	}
	for id, d := range map[string]reportdesign.Design{classic: builtin(t, "classic"), house: ours} {
		got := take("agy_test", id)
		if !bytes.Equal(got.Design, d.Canonical()) || got.From != engine.ThemeFromLibraryFlag {
			t.Fatalf("%s: %+v", d.Name, got)
		}
		stamp, err := toolsrv.StampTheme(got.Design)
		if err != nil || stamp.SHA256 != d.SHA256() || stamp.Name != d.Name || stamp.BasedOn != d.BasedOn {
			t.Fatalf("%s: the pinned CLI stamps %+v, %v", d.Name, stamp, err)
		}
	}
	if _, err := setDefaultReportDesign(ctx, url, "agy_test", house); err != nil {
		t.Fatal(err)
	}
	if got := take("agy_test", ""); !bytes.Equal(got.Design, ours.Canonical()) || got.From != engine.ThemeFromAccount {
		t.Fatalf("the default: %+v", got)
	}
	if log.Len() != 0 {
		t.Fatalf("nothing to warn about yet: %q", log.String())
	}

	// The default, unreadable: Plain, said.
	if got := take("agy_wrong", ""); got.Design != nil || !strings.Contains(log.String(), "report designs could not be read") {
		t.Fatalf("unreadable: %+v, log %q", got, log.String())
	}
	// A pick that fails refuses the run, with nothing in the log.
	log.Reset()
	refused("agy_wrong", house, "could not be read")
	refused("agy_test", uuid.NewString(), "could not be read")
	refused("agy_test", "not-an-id", "not one of your report designs")
	f.mu.Lock()
	f.designs[uuid.MustParse(house)].doc.AgentryTheme = 4
	f.mu.Unlock()
	refused("agy_test", house, "not one this app can draw")
	if log.Len() != 0 {
		t.Fatalf("a refused pick writes no warning: %q", log.String())
	}
	// Signed out: nothing is asked; a pick is refused.
	before := requests.Load()
	if got := take("", ""); got.Design != nil || log.Len() != 0 || requests.Load() != before {
		t.Fatalf("signed out: %+v, log %q, %d requests", got, log.String(), requests.Load()-before)
	}
	refused("", house, "signed out")
	// An http:// spelling of a registry off this machine is never sent the
	// token: Plain, said, and no request.
	log.Reset()
	got, err := accountReportTheme(ctx, "http://registry.example", "agy_test", "", &log)
	if err != nil || got.Design != nil || !strings.Contains(log.String(), "not one Settings accepts") || strings.Contains(log.String(), "registry.example") {
		t.Fatalf("http off this machine: %+v, %v, log %q", got, err, log.String())
	}
	if requests.Load() != before {
		t.Fatal("no request may be made without a usable credential and address")
	}
}

// The images: an upload sends one multipart part, the file's bytes and not
// its name, and the core's re-encoding comes back as a data: URL beside the
// design; a file over the core's bound is refused before it is sent; a
// removal keeps the design; and Brand from logo reads the stored logo's two
// colours as shades that pass for text.
func TestTheReportImages(t *testing.T) {
	f := newFakeLibrary(t, true)
	url := f.srv.URL
	ctx := context.Background()
	id := f.add(builtin(t, "editorial"), true)
	did := uuid.MustParse(id)
	dir := t.TempDir()
	logoFile := filepath.Join(dir, "my-company-logo.png")
	if err := os.WriteFile(logoFile, testPNG(t, 120, 40), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := uploadReportImage(ctx, url, "agy_test", did, logoUpload, logoFile)
	if err != nil || !strings.HasPrefix(v.Logo, "data:image/png;base64,iVBORw0KGgo") || v.Design.Logo != (reportdesign.Logo{}) || f.partName != "image" {
		t.Fatalf("logo: %+v, %v (part %q)", v, err, f.partName)
	}
	if stored := f.stored(id); stored.Logo == (reportdesign.Logo{}) || v.Logo != "data:image/png;base64,"+stored.Logo.PNG {
		t.Fatal("the page's logo is not the core's")
	}
	s, err := suggestReportColours(ctx, url, "agy_test", id)
	if err != nil || s.Accent == "" || s.Secondary == "" || s.Accent == s.Secondary {
		t.Fatalf("suggest: %+v, %v", s, err)
	}
	for _, c := range []string{s.Accent, s.Secondary} {
		if ratio, err := reportdesign.Contrast(c, reportdesign.White); err != nil || ratio < reportdesign.TextContrast {
			t.Fatalf("%s does not pass for text (%v, %v)", c, ratio, err)
		}
	}

	coverFile := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(coverFile, testJPEG(t, 640, 480), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err = uploadReportImage(ctx, url, "agy_test", did, coverImageUpload, coverFile)
	if err != nil || !strings.HasPrefix(v.CoverImage, "data:image/jpeg;base64,/9j/") || v.Logo == "" {
		t.Fatalf("cover image: %+v, %v", v, err)
	}
	big := filepath.Join(dir, "big.png")
	if err := os.WriteFile(big, bytes.Repeat([]byte{0}, reportdesign.MaxLogoUploadBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	if _, err := uploadReportImage(ctx, url, "agy_test", did, logoUpload, big); err == nil || !strings.Contains(err.Error(), "at most 512 KB") || requests.Load() != before {
		t.Fatalf("too large: %v", err)
	}
	svg := filepath.Join(dir, "logo.svg")
	if err := os.WriteFile(svg, []byte("<svg/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := uploadReportImage(ctx, url, "agy_test", did, logoUpload, svg); err == nil || !strings.Contains(err.Error(), "SVG is not accepted") {
		t.Fatalf("svg: %v", err)
	}
	if _, err := uploadReportImage(ctx, url, "agy_test", did, logoUpload, dir); err == nil || !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("a folder: %v", err)
	}

	v, err = removeReportAsset(ctx, url, "agy_test", did, logoUpload)
	if err != nil || v.Logo != "" || v.CoverImage == "" || v.Label != "Editorial" {
		t.Fatalf("remove logo: %+v, %v", v, err)
	}
	if _, err := suggestReportColours(ctx, url, "agy_test", id); err == nil || !strings.Contains(err.Error(), "no logo") {
		t.Fatalf("suggest without a logo: %v", err)
	}
	v, err = removeReportAsset(ctx, url, "agy_test", did, coverImageUpload)
	if err != nil || v.CoverImage != "" {
		t.Fatalf("remove cover image: %+v, %v", v, err)
	}
	f.mu.Lock()
	f.designs[did].custom = reportdesign.CustomCover{Source: "#let cover(meta, design) = [Hi]"}
	f.mu.Unlock()
	v, err = removeReportAsset(ctx, url, "agy_test", did, customCoverAsset)
	if err != nil || v.CustomCover != nil {
		t.Fatalf("remove custom cover: %+v, %v", v, err)
	}
}

// The bindings take a page-supplied address through Settings' rule before
// a token is looked up or a picker opens: an http:// spelling of a registry
// off this machine is refused, whatever the keychain holds.
func TestReportDesignBindingsHoldTheAddressToSettingsRule(t *testing.T) {
	picked := false
	saved := pickReportImage
	pickReportImage = func(context.Context, reportAsset) (string, error) { picked = true; return "", nil }
	t.Cleanup(func() { pickReportImage = saved })
	s := &SettingsAPI{app: &App{ctx: context.Background()}}
	const bad = "http://registry.example"
	id := uuid.NewString()
	calls := map[string]func() error{
		"ReportDesignLibrary":     func() error { _, err := s.ReportDesignLibrary(bad); return err },
		"OpenReportDesign":        func() error { _, err := s.OpenReportDesign(bad, id); return err },
		"CreateReportDesign":      func() error { _, err := s.CreateReportDesign(bad, builtin(t, "plain"), false); return err },
		"SaveReportDesign":        func() error { _, err := s.SaveReportDesign(bad, id, builtin(t, "plain")); return err },
		"DeleteReportDesign":      func() error { _, err := s.DeleteReportDesign(bad, id); return err },
		"SetDefaultReportDesign":  func() error { _, err := s.SetDefaultReportDesign(bad, id); return err },
		"DuplicateReportDesign":   func() error { _, err := s.DuplicateReportDesign(bad, id); return err },
		"SuggestReportColours":    func() error { _, err := s.SuggestReportColours(bad, id); return err },
		"ChooseReportLogo":        func() error { _, err := s.ChooseReportLogo(bad, id); return err },
		"ChooseReportCoverImage":  func() error { _, err := s.ChooseReportCoverImage(bad, id); return err },
		"RemoveReportLogo":        func() error { _, err := s.RemoveReportLogo(bad, id); return err },
		"RemoveReportCoverImage":  func() error { _, err := s.RemoveReportCoverImage(bad, id); return err },
		"RemoveReportCustomCover": func() error { _, err := s.RemoveReportCustomCover(bad, id); return err },
	}
	for name, call := range calls {
		if err := call(); err == nil || !strings.Contains(err.Error(), "https") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if picked {
		t.Fatal("a picker opened for a refused address")
	}
}

// reportDesignFixture is the frontend tests' copy of the core's report
// designs (frontend/tests/reportDesign.test.ts): each kit's canonical
// document as text, the families, the closed lists, the tokens and the
// limits. It is held here to the core this app pins, so the frontend's round
// trip is tested against the documents the core ships.
// AGENTRY_UPDATE_FIXTURES=1 rewrites it after a core bump. The path is from
// this package, two levels below the root.
const reportDesignFixture = "../../frontend/tests/fixtures/reportDesigns.json"

func TestTheFrontendsReportDesignFixtureIsTheCores(t *testing.T) {
	o := reportDesignOptions()
	kits := map[string]string{}
	for _, id := range reportdesign.BuiltinIDs() {
		doc, _ := reportdesign.BuiltinDocument(id)
		kits[id] = string(doc)
	}
	want, err := json.MarshalIndent(map[string]any{
		"ids":             reportdesign.BuiltinIDs(),
		"kits":            kits,
		"families":        o.Families,
		"headingFonts":    o.HeadingFonts,
		"bodyFonts":       o.BodyFonts,
		"codeFonts":       o.CodeFonts,
		"papers":          o.Papers,
		"titleBlocks":     o.TitleBlocks,
		"headingStyles":   o.HeadingStyles,
		"headingScales":   o.HeadingScales,
		"margins":         o.Margins,
		"quoteStyles":     o.QuoteStyles,
		"pageNumbers":     o.PageNumbers,
		"headerStyles":    o.HeaderStyles,
		"coverStyles":     o.CoverStyles,
		"coverFacts":      o.CoverFacts,
		"coverWhens":      o.CoverWhens,
		"tokens":          o.Tokens,
		"maxName":         o.MaxName,
		"maxLine":         o.MaxLine,
		"maxContactLines": o.MaxContactLines,
		"slotCount":       o.SlotCount,
		"maxLibrary":      o.MaxLibrary,
		"defaultKitId":    o.DefaultKitID,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if os.Getenv("AGENTRY_UPDATE_FIXTURES") == "1" {
		if err := os.MkdirAll(filepath.Dir(reportDesignFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(reportDesignFixture, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(reportDesignFixture)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s is not the pinned core's report designs (%v): regenerate with AGENTRY_UPDATE_FIXTURES=1 go test ./internal/app -run %s", reportDesignFixture, err, t.Name())
	}
}

// testPNG is a w×h PNG, opaque, in two colours.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, twoColours(w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// testJPEG is a w×h JPEG in the same two colours.
func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, twoColours(w, h), &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func twoColours(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.NRGBA{R: 0x14, G: 0x5a, B: 0x32, A: 255}
			if x > w/2 {
				c = color.NRGBA{R: 0xe8, G: 0x6a, B: 0x10, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// A run In Agentry sends the picked design's id on its create, and nothing
// when the person left the default to the registry.
func TestAHostedRunSendsThePickedDesignsID(t *testing.T) {
	f := &hostedFake{enabled: true, outputs: map[string]string{"result.json": hostedEnvelope}}
	reg := f.start(t)
	signIn(t, reg)
	fastPoll(t)
	r, events := newRunAPI(t)
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, false, false, nil, "../not-an-id"); err == nil || !strings.Contains(err.Error(), "not one of your report designs") {
		t.Fatalf("a malformed id must be refused before the create: %v", err)
	}
	if n := len(f.creates); n != 0 {
		t.Fatalf("a malformed id reached the registry: %d creates", n)
	}
	for _, want := range []string{"0f1e2d3c-4b5a-4968-8776-655443322110", ""} {
		f.mu.Lock()
		f.run, f.created = nil, nil
		f.mu.Unlock()
		events.mu.Lock()
		events.runs = nil
		events.mu.Unlock()
		if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, false, false, nil, want); err != nil {
			t.Fatal(err)
		}
		f.mu.Lock()
		created, body := f.created, f.creates[len(f.creates)-1]
		f.mu.Unlock()
		if created == nil || created.ReportDesign != want || (want == "" && strings.Contains(body, "reportDesign")) || (want != "" && !strings.Contains(body, `"reportDesign":"`+want+`"`)) {
			t.Fatalf("picked %q sent %+v: %s", want, created, body)
		}
		f.finish(hostedrun.OutcomeSucceeded, "ok", 1)
		waitForFinal(t, events)
	}
}
