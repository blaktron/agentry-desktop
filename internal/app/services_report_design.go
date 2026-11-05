package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/reportthumbs"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/reportdesign"
	"github.com/google/uuid"
)

// The report designs: how the PDFs a run exports with export_pdf look, the
// person's choice, never the model's. The account keeps a library of up to
// ten named designs with one default, each a theme document version 3 (a
// brand block, a second colour, header and footer slots built from a closed
// set of tokens, a cover page in one of six styles or the person's own
// cover, a logo and a cover image), at /user/report-designs on
// user:profile, which the app's sign-in carries. Settings → Report design is
// the library and its editor; the run setup picks one of the library for a
// run, with the default preselected; a local run carries the picked design
// whole, its images included, and a run In Agentry sends its id.
//
// The core's pkg/reportdesign is the one definition: the page's controls are
// its fields, its closed lists and families are the page's choices, and
// every check the page shows (the contrast guard for each colour, the field
// a save would be refused for) is that package's answer through
// CheckReportDesign, so the page cannot disagree with the core's save. The
// kits' thumbnails are the pinned CLI's (pkg/reportthumbs), rendered by the
// runner's own template.
//
// The images are the person's own, drawn by the pinned CLI where the design
// asks. The core keeps them beside the design and answers them only when
// asked (include=assets), which the editor's read does; each is uploaded and
// removed on its own, and a Save sends the design without them, so the core
// keeps the stored ones. The page never holds their bytes in the design it
// edits: the view gives each as a data: URL beside the design, so the
// editor's checks do not carry megabytes back and forth on every change.

// ReportDesignLibrary is the account's library as the page lists it.
// Available is false on a registry that has no report design library, where
// the page says so.
type ReportDesignLibrary struct {
	Available bool               `json:"available"`
	Designs   []ReportDesignCard `json:"designs"`
	// Default is the default design's id, "" for none.
	Default string `json:"default"`
	// Max is how many designs the library holds at most.
	Max int `json:"max"`
}

// ReportDesignCard is one design of the library as its card shows it. The
// kit it started from names the thumbnail the card draws.
type ReportDesignCard struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BasedOn        string `json:"basedOn"`
	Version        int    `json:"version"`
	Default        bool   `json:"default"`
	HasLogo        bool   `json:"hasLogo"`
	HasCoverImage  bool   `json:"hasCoverImage"`
	HasCustomCover bool   `json:"hasCustomCover"`
	UpdatedAt      string `json:"updatedAt"`
}

// ReportDesignView is one design of the library as the editor holds it.
// Design is the document without its images, which come beside it as data:
// URLs. Unreadable says the registry holds a design this build cannot read
// (a later core's), which Save replaces; it is the reason, never the
// document.
type ReportDesignView struct {
	ID         string               `json:"id"`
	Default    bool                 `json:"default"`
	Design     *reportdesign.Design `json:"design,omitempty"`
	Label      string               `json:"label,omitempty"`
	Unreadable string               `json:"unreadable,omitempty"`
	// Logo is the design's logo as a PNG data: URL, "" for none.
	Logo string `json:"logo,omitempty"`
	// CoverImage is the design's cover image as a PNG or JPEG data: URL,
	// "" for none.
	CoverImage string `json:"coverImage,omitempty"`
	// CustomCover is the design's own cover, nil for none.
	CustomCover *ReportCustomCoverView `json:"customCover,omitempty"`
}

// ReportCustomCoverView is a design's custom cover as the editor shows it:
// its last render as a PNG data: URL ("" when it has none), and how much
// code and how many images it holds. The code itself never reaches the
// page: the editor shows and removes the cover, it does not write it.
type ReportCustomCoverView struct {
	Thumbnail   string `json:"thumbnail,omitempty"`
	SourceBytes int    `json:"sourceBytes"`
	Images      int    `json:"images"`
}

// ReportDesignOptions are the editor's choices, all of them the core's:
// the ten kits with their thumbnails, the families and which role each may
// set, the closed lists, the tokens a slot may name, and the texts' limits.
type ReportDesignOptions struct {
	Kits            []ReportDesignKit    `json:"kits"`
	Families        []ReportDesignFamily `json:"families"`
	HeadingFonts    []string             `json:"headingFonts"`
	BodyFonts       []string             `json:"bodyFonts"`
	CodeFonts       []string             `json:"codeFonts"`
	Papers          []string             `json:"papers"`
	TitleBlocks     []string             `json:"titleBlocks"`
	HeadingStyles   []string             `json:"headingStyles"`
	HeadingScales   []string             `json:"headingScales"`
	Margins         []string             `json:"margins"`
	QuoteStyles     []string             `json:"quoteStyles"`
	PageNumbers     []string             `json:"pageNumbers"`
	HeaderStyles    []string             `json:"headerStyles"`
	CoverStyles     []ReportCoverStyle   `json:"coverStyles"`
	CoverFacts      []string             `json:"coverFacts"`
	CoverWhens      []string             `json:"coverWhens"`
	Tokens          []string             `json:"tokens"`
	MaxName         int                  `json:"maxName"`
	MaxLine         int                  `json:"maxLine"`
	MaxContactLines int                  `json:"maxContactLines"`
	SlotCount       int                  `json:"slotCount"`
	MaxLibrary      int                  `json:"maxLibrary"`
	DefaultKitID    string               `json:"defaultKitId"`
}

// ReportDesignKit is one built-in theme as a kit: its document, exactly as
// the core ships it, and its two thumbnails as data: URLs (the page's CSP
// admits data: images), "" when the pinned CLI has none for it.
type ReportDesignKit struct {
	ID     string              `json:"id"`
	Design reportdesign.Design `json:"design"`
	// Cover is the sample's cover page in the kit's cover style; Page its
	// first body page.
	Cover string `json:"cover"`
	Page  string `json:"page"`
}

// ReportDesignFamily is a family the Typst image carries.
type ReportDesignFamily struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	HeadingsOnly bool   `json:"headingsOnly,omitempty"`
}

// ReportCoverStyle is a cover style and which colours carry text on it.
type ReportCoverStyle struct {
	Name            string `json:"name"`
	AccentOnText    bool   `json:"accentOnText"`
	SecondaryOnText bool   `json:"secondaryOnText"`
}

// ReportDesignCheck is the core's verdict on a design the editor holds.
// Problem is the first rule it breaks, in document order, as the core's
// save would refuse it; nil when it can be saved. Accent and Secondary are
// the contrast guard's readings, Tint and SecondaryTint each colour's
// derived tint ("" for a colour that is not #rrggbb, or no secondary),
// Unedited whether it is its kit's own document, Label its name as a status
// line says it, and Header and Footer the slots with their tokens expanded
// from the sample report's facts, as the preview draws them.
type ReportDesignCheck struct {
	Problem       *ReportDesignProblem `json:"problem,omitempty"`
	Accent        ReportColourCheck    `json:"accent"`
	Secondary     ReportColourCheck    `json:"secondary"`
	Tint          string               `json:"tint"`
	SecondaryTint string               `json:"secondaryTint"`
	Unedited      bool                 `json:"unedited"`
	Label         string               `json:"label"`
	Header        []string             `json:"header"`
	Footer        []string             `json:"footer"`
}

// ReportDesignProblem is a field the core refuses and its words.
type ReportDesignProblem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ReportColourCheck is the contrast rule for one of the design's colours:
// its ratio on white, the ratio it needs (4.5 where it colours text, 3
// where it colours only graphics), and the nearest shade that passes, which
// is the colour itself when it does. Valid is false for a colour that is
// not #rrggbb; Set is false for a secondary the design does not have.
type ReportColourCheck struct {
	Set     bool    `json:"set"`
	Valid   bool    `json:"valid"`
	OK      bool    `json:"ok"`
	Ratio   float64 `json:"ratio"`
	Need    float64 `json:"need"`
	OnText  bool    `json:"onText"`
	Nearest string  `json:"nearest"`
}

// ReportColourSuggestion is what Brand from logo offers: the logo's two
// dominant colours, each as the nearest shade that passes the contrast rule
// for text, and as read. Secondary is "" when the logo has one colour. The
// page shows them for the person to take; nothing applies them.
type ReportColourSuggestion struct {
	Accent        string `json:"accent"`
	Secondary     string `json:"secondary"`
	FromAccent    string `json:"fromAccent"`
	FromSecondary string `json:"fromSecondary"`
}

// reportDesignReadTimeout bounds the reads a run makes before it starts.
const reportDesignReadTimeout = 10 * time.Second

// sampleFacts are the sample report's facts the preview expands the slots
// with, beside what the design itself names. The pages are what the sample
// runs to.
var sampleFacts = map[string]string{
	"title":   "Quarterly review",
	"author":  "The review team",
	"date":    "4 November 2026",
	"page":    "2",
	"pages":   "4",
	"section": "Summary",
}

// ReportDesignOptions are the editor's choices. It reads nothing but this
// build, so it needs no sign-in.
func (s *SettingsAPI) ReportDesignOptions() ReportDesignOptions {
	return reportDesignOptions()
}

func reportDesignOptions() ReportDesignOptions {
	o := ReportDesignOptions{
		HeadingFonts:    reportdesign.FontsFor("headings"),
		BodyFonts:       reportdesign.FontsFor("body"),
		CodeFonts:       reportdesign.FontsFor("code"),
		Papers:          append([]string(nil), reportdesign.Papers...),
		TitleBlocks:     append([]string(nil), reportdesign.TitleBlocks...),
		HeadingStyles:   append([]string(nil), reportdesign.HeadingStyles...),
		HeadingScales:   append([]string(nil), reportdesign.HeadingScales...),
		Margins:         append([]string(nil), reportdesign.MarginPresets...),
		QuoteStyles:     append([]string(nil), reportdesign.QuoteStyles...),
		PageNumbers:     append([]string(nil), reportdesign.PageNumberStyles...),
		HeaderStyles:    append([]string(nil), reportdesign.HeaderStyles...),
		CoverFacts:      append([]string(nil), reportdesign.CoverFacts...),
		CoverWhens:      append([]string(nil), reportdesign.CoverWhens...),
		Tokens:          append([]string(nil), reportdesign.Tokens...),
		MaxName:         reportdesign.MaxNameRunes,
		MaxLine:         reportdesign.MaxLineRunes,
		MaxContactLines: reportdesign.MaxContactLines,
		SlotCount:       reportdesign.SlotCount,
		MaxLibrary:      reportdesign.MaxLibrary,
		DefaultKitID:    reportdesign.DefaultID,
	}
	for _, d := range reportdesign.Builtins() {
		k := ReportDesignKit{ID: d.BasedOn, Design: d}
		if png, ok := reportthumbs.CoverPNG(d.BasedOn); ok {
			k.Cover = pngURL(png)
		}
		if png, ok := reportthumbs.PNG(d.BasedOn); ok {
			k.Page = pngURL(png)
		}
		o.Kits = append(o.Kits, k)
	}
	for _, f := range reportdesign.Families() {
		o.Families = append(o.Families, ReportDesignFamily{Name: f.Name, Kind: string(f.Kind), HeadingsOnly: f.HeadingsOnly})
	}
	for _, c := range reportdesign.CoverStyles() {
		o.CoverStyles = append(o.CoverStyles, ReportCoverStyle{Name: c.Name, AccentOnText: c.AccentOnText, SecondaryOnText: c.SecondaryOnText})
	}
	return o
}

func pngURL(png []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

// imageURL is a design's image as a data: URL, "" for none.
func imageURL(i reportdesign.Image) string {
	switch i.Format() {
	case "png":
		return "data:image/png;base64," + i.PNG
	case "jpeg":
		return "data:image/jpeg;base64," + i.JPEG
	}
	return ""
}

// CheckReportDesign is the core's verdict on the design the editor holds.
// It reads nothing but the design.
func (s *SettingsAPI) CheckReportDesign(d reportdesign.Design) ReportDesignCheck {
	return checkReportDesign(d)
}

func checkReportDesign(d reportdesign.Design) ReportDesignCheck {
	c := ReportDesignCheck{Label: designLabel(d), Unedited: isUnedited(d)}
	if err := d.Validate(); err != nil {
		var fe *reportdesign.FieldError
		if errors.As(err, &fe) {
			c.Problem = &ReportDesignProblem{Field: fe.Field, Message: fe.Message}
		} else {
			c.Problem = &ReportDesignProblem{Message: err.Error()}
		}
	}
	c.Accent, c.Tint = colourCheck(d.Accent, d.MinContrast(), d.AccentOnText())
	if d.Secondary != "" {
		c.Secondary, c.SecondaryTint = colourCheck(d.Secondary, d.MinSecondaryContrast(), d.SecondaryOnText())
	}
	facts := make(map[string]string, len(sampleFacts)+2)
	for k, v := range sampleFacts {
		facts[k] = v
	}
	facts["brand"] = d.Brand.Name
	facts["confidentiality"] = d.Brand.Confidentiality
	c.Header = make([]string, reportdesign.SlotCount)
	c.Footer = make([]string, reportdesign.SlotCount)
	for i := range reportdesign.SlotCount {
		c.Header[i] = reportdesign.ExpandSlot(d.Header.Slots[i], facts)
		c.Footer[i] = reportdesign.ExpandSlot(d.Footer.Slots[i], facts)
	}
	return c
}

// colourCheck is the contrast rule's reading of one colour, and its tint.
func colourCheck(colour string, need float64, onText bool) (ReportColourCheck, string) {
	c := ReportColourCheck{Set: true, Need: need, OnText: onText, Nearest: "#000000", Ratio: 1}
	ratio, err := reportdesign.Contrast(colour, reportdesign.White)
	if err != nil {
		return c, ""
	}
	c.Valid = true
	c.Ratio = ratio
	c.OK = ratio >= need
	if near, err := reportdesign.NearestPassing(colour, need); err == nil {
		c.Nearest = near
	}
	tint, _ := reportdesign.Tint(colour)
	return c, tint
}

// isUnedited reports whether d is its kit's own document, compared by the
// core's canonical form without images.
func isUnedited(d reportdesign.Design) bool {
	b, ok := reportdesign.Builtin(d.BasedOn)
	return ok && bytes.Equal(d.WithoutAssets().Canonical(), b.Canonical())
}

// designLabel is a design's name as a status line says it: a kit's own
// name when it is that kit unedited, marked when it is a kit edited, and
// the person's name with its kit otherwise.
func designLabel(d reportdesign.Design) string {
	kit := d.BasedOn
	if b, ok := reportdesign.Builtin(d.BasedOn); ok {
		kit = b.Name
	}
	switch {
	case isUnedited(d):
		return d.Name
	case d.Name == kit:
		return d.Name + ", edited"
	default:
		return fmt.Sprintf("%s (from %s)", d.Name, kit)
	}
}

// ReportDesignLibrary lists the account's report designs at the registry.
func (s *SettingsAPI) ReportDesignLibrary(url string) (*ReportDesignLibrary, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return reportDesignLibrary(s.app.ctx, url, token)
}

// OpenReportDesign reads one design of the library with its images, as the
// editor holds it.
func (s *SettingsAPI) OpenReportDesign(url, id string) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return openReportDesign(s.app.ctx, url, token, id)
}

// CreateReportDesign holds a design to the core's rules and adds it to the
// library, as the default when asked.
func (s *SettingsAPI) CreateReportDesign(url string, d reportdesign.Design, makeDefault bool) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return createReportDesign(s.app.ctx, url, token, d, makeDefault)
}

// SaveReportDesign holds a design to the core's rules and saves it over
// the design id of the library, whole; the stored images stay.
func (s *SettingsAPI) SaveReportDesign(url, id string, d reportdesign.Design) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return saveReportDesign(s.app.ctx, url, token, id, d)
}

// DeleteReportDesign removes a design, its images included, and lists the
// library as it is then.
func (s *SettingsAPI) DeleteReportDesign(url, id string) (*ReportDesignLibrary, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return deleteReportDesign(s.app.ctx, url, token, id)
}

// SetDefaultReportDesign makes a design the default a run takes when the
// person picks none, or with "" makes none the default.
func (s *SettingsAPI) SetDefaultReportDesign(url, id string) (*ReportDesignLibrary, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return setDefaultReportDesign(s.app.ctx, url, token, id)
}

// DuplicateReportDesign adds a copy of a design, its images included,
// named after it, and opens the copy.
func (s *SettingsAPI) DuplicateReportDesign(url, id string) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return duplicateReportDesign(s.app.ctx, url, token, id)
}

// SuggestReportColours reads a design's logo for the two colours Brand
// from logo offers. A design without a logo, or a logo with no colour to
// read, is an error in the core's words.
func (s *SettingsAPI) SuggestReportColours(url, id string) (*ReportColourSuggestion, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return suggestReportColours(s.app.ctx, url, token, id)
}

// ChooseReportLogo asks for a PNG or JPEG with the system's file picker and
// uploads it as the logo of the design. A cancelled pick changes nothing
// and answers no view. The path never comes from the page: the picker is
// opened here.
func (s *SettingsAPI) ChooseReportLogo(url, id string) (*ReportDesignView, error) {
	return s.chooseReportImage(url, id, logoUpload)
}

// ChooseReportCoverImage asks for a PNG or JPEG with the system's file
// picker and uploads it as the design's cover image, as ChooseReportLogo
// does for the logo.
func (s *SettingsAPI) ChooseReportCoverImage(url, id string) (*ReportDesignView, error) {
	return s.chooseReportImage(url, id, coverImageUpload)
}

// RemoveReportLogo takes the logo off a design and keeps the design.
func (s *SettingsAPI) RemoveReportLogo(url, id string) (*ReportDesignView, error) {
	return s.removeReportAsset(url, id, logoUpload)
}

// RemoveReportCoverImage takes the cover image off a design and keeps the
// design.
func (s *SettingsAPI) RemoveReportCoverImage(url, id string) (*ReportDesignView, error) {
	return s.removeReportAsset(url, id, coverImageUpload)
}

// RemoveReportCustomCover takes the person's own cover off a design: its
// code, its images and its thumbnail. The design's cover style then draws.
func (s *SettingsAPI) RemoveReportCustomCover(url, id string) (*ReportDesignView, error) {
	return s.removeReportAsset(url, id, customCoverAsset)
}

func (s *SettingsAPI) chooseReportImage(url, id string, kind reportAsset) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	did, err := designID(id)
	if err != nil {
		return nil, err
	}
	path, err := pickReportImage(s.app.ctx, kind)
	if err != nil || path == "" {
		return nil, err
	}
	return uploadReportImage(s.app.ctx, url, token, did, kind, path)
}

func (s *SettingsAPI) removeReportAsset(url, id string, kind reportAsset) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	did, err := designID(id)
	if err != nil {
		return nil, err
	}
	return removeReportAsset(s.app.ctx, url, token, did, kind)
}

// reportAsset is one of a design's images as the core keeps them.
type reportAsset struct {
	// what names the image in the picker's title and in refusals.
	what string
	// maxUpload is the core's bound on the file before it is sent.
	maxUpload int
}

var (
	logoUpload       = reportAsset{what: "logo", maxUpload: reportdesign.MaxLogoUploadBytes}
	coverImageUpload = reportAsset{what: "cover image", maxUpload: reportdesign.MaxCoverImageUploadBytes}
	customCoverAsset = reportAsset{what: "custom cover"}
)

// pickReportImage is the system's file picker for an image; a test
// replaces it.
var pickReportImage = func(ctx context.Context, kind reportAsset) (string, error) {
	return nativeOpenFile(ctx, pickOptions{
		Title:   "Choose your " + kind.what,
		Filters: []fileFilter{{Name: "PNG or JPEG images", Pattern: "*.png;*.jpg;*.jpeg"}},
	})
}

// designID is a design's id as the page gave it, held to the shape of one
// before it reaches an address.
func designID(id string) (uuid.UUID, error) {
	u, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil || u == uuid.Nil {
		return uuid.Nil, errors.New("that is not one of your report designs")
	}
	return u, nil
}

func withAssets() *apiclient.ReportDesignInclude {
	include := apiclient.ReportDesignIncludeAssets
	return &include
}

func reportClient(url, token string) (*apiclient.ClientWithResponses, error) {
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	return client, nil
}

// reportDesignLibrary is GET /user/report-designs as the page lists it.
// A registry that has no library answers 404, which is not an error.
func reportDesignLibrary(ctx context.Context, url, token string) (*ReportDesignLibrary, error) {
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	resp, err := client.ListUserReportDesignsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return &ReportDesignLibrary{Max: reportdesign.MaxLibrary}, nil
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return nil, registry.Problem("read your report designs", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return libraryView(resp.JSON200), nil
}

func libraryView(l *apiclient.ReportDesignList) *ReportDesignLibrary {
	v := &ReportDesignLibrary{Available: true, Designs: []ReportDesignCard{}, Max: reportdesign.MaxLibrary}
	if l.Default != nil {
		v.Default = l.Default.String()
	}
	for _, d := range l.Designs {
		v.Designs = append(v.Designs, ReportDesignCard{
			ID: d.Id.String(), Name: d.Name, BasedOn: d.BasedOn, Version: d.Version, Default: d.Default,
			HasLogo: d.HasLogo, HasCoverImage: d.HasCoverImage, HasCustomCover: d.HasCustomCover,
			UpdatedAt: d.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return v
}

// designRecord is a design's record as the registry answers it, the
// document kept as the core's own bytes rather than read through the
// generated type, so a field a later core adds is kept for the parse to
// see rather than dropped.
type designRecord struct {
	ID      uuid.UUID       `json:"id"`
	Default bool            `json:"default"`
	Design  json.RawMessage `json:"design"`
}

// designDocument is the record's document, or an error when the body is
// not a record.
func designDocument(body []byte) (designRecord, error) {
	var rec designRecord
	if err := json.Unmarshal(body, &rec); err != nil || len(rec.Design) == 0 {
		return rec, errors.New("the registry's answer is not a report design")
	}
	return rec, nil
}

// reportDesignView is the record as the editor holds it: the document
// without its images, each image as a data: URL beside it.
func reportDesignView(rec designRecord) *ReportDesignView {
	v := &ReportDesignView{ID: rec.ID.String(), Default: rec.Default}
	d, err := reportdesign.Parse(rec.Design)
	if err != nil {
		v.Unreadable = err.Error()
		return v
	}
	// Parse held each image to its bounds; the page draws them only as
	// these data: URLs.
	if d.Logo != (reportdesign.Logo{}) {
		v.Logo = "data:image/png;base64," + d.Logo.PNG
	}
	v.CoverImage = imageURL(d.CoverImage)
	if !d.CustomCover.IsZero() {
		v.CustomCover = &ReportCustomCoverView{
			Thumbnail:   imageURL(d.CustomCover.Thumbnail),
			SourceBytes: len(d.CustomCover.Source),
			Images:      len(d.CustomCover.Images),
		}
	}
	d = d.WithoutAssets()
	v.Design, v.Label = &d, designLabel(d)
	return v
}

// readDesign is GET /user/report-designs/{id}?include=assets as the
// registry answers it.
func readDesign(ctx context.Context, client *apiclient.ClientWithResponses, url string, id uuid.UUID) (designRecord, error) {
	resp, err := client.GetUserReportDesignWithResponse(ctx, id, &apiclient.GetUserReportDesignParams{Include: withAssets()})
	if err != nil {
		return designRecord{}, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() != http.StatusOK {
		return designRecord{}, registry.Problem("read your report design", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return designDocument(resp.Body)
}

func openReportDesign(ctx context.Context, url, token, id string) (*ReportDesignView, error) {
	did, err := designID(id)
	if err != nil {
		return nil, err
	}
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	rec, err := readDesign(ctx, client, url, did)
	if err != nil {
		return nil, err
	}
	return reportDesignView(rec), nil
}

// sendable is a design as the page's Save or Create sends it: without its
// images, whatever the page holds, so the core keeps the stored ones, and
// held to the core's rules first, in the core's words, so nothing is sent
// that would be refused.
func sendable(d reportdesign.Design) ([]byte, error) {
	sent := d.WithoutAssets()
	if err := sent.Validate(); err != nil {
		return nil, err
	}
	return sent.Canonical(), nil
}

func createReportDesign(ctx context.Context, url, token string, d reportdesign.Design, makeDefault bool) (*ReportDesignView, error) {
	doc, err := sendable(d)
	if err != nil {
		return nil, err
	}
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	return createDesignDocument(ctx, client, url, doc, makeDefault)
}

// createDesignDocument is POST /user/report-designs with a document the
// caller holds to the core's rules (a duplicate carries its images).
func createDesignDocument(ctx context.Context, client *apiclient.ClientWithResponses, url string, doc []byte, makeDefault bool) (*ReportDesignView, error) {
	body := append([]byte(`{"default":`), []byte(fmt.Sprint(makeDefault))...)
	body = append(append(append(body, `,"design":`...), bytes.TrimSpace(doc)...), '}')
	resp, err := client.CreateUserReportDesignWithBodyWithResponse(ctx, &apiclient.CreateUserReportDesignParams{Include: withAssets()}, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, registry.Problem("add the report design", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	rec, err := designDocument(resp.Body)
	if err != nil {
		return nil, err
	}
	return reportDesignView(rec), nil
}

func saveReportDesign(ctx context.Context, url, token, id string, d reportdesign.Design) (*ReportDesignView, error) {
	did, err := designID(id)
	if err != nil {
		return nil, err
	}
	doc, err := sendable(d)
	if err != nil {
		return nil, err
	}
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	body := append(append([]byte(`{"design":`), bytes.TrimSpace(doc)...), '}')
	resp, err := client.UpdateUserReportDesignWithBodyWithResponse(ctx, did, &apiclient.UpdateUserReportDesignParams{Include: withAssets()}, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, registry.Problem("save your report design", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	rec, err := designDocument(resp.Body)
	if err != nil {
		return nil, err
	}
	return reportDesignView(rec), nil
}

func deleteReportDesign(ctx context.Context, url, token, id string) (*ReportDesignLibrary, error) {
	did, err := designID(id)
	if err != nil {
		return nil, err
	}
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	resp, err := client.DeleteUserReportDesignWithResponse(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return nil, registry.Problem("delete the report design", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return reportDesignLibrary(ctx, url, token)
}

func setDefaultReportDesign(ctx context.Context, url, token, id string) (*ReportDesignLibrary, error) {
	body := []byte(`{}`)
	if strings.TrimSpace(id) != "" {
		did, err := designID(id)
		if err != nil {
			return nil, err
		}
		body = []byte(`{"id":"` + did.String() + `"}`)
	}
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	resp, err := client.SetUserReportDesignDefaultWithBodyWithResponse(ctx, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return nil, registry.Problem("set your default report design", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return libraryView(resp.JSON200), nil
}

// duplicateReportDesign reads the design with its images and adds it again
// under a name that says so, every image included, so the copy draws as
// the original does. The copy is never the default.
func duplicateReportDesign(ctx context.Context, url, token, id string) (*ReportDesignView, error) {
	did, err := designID(id)
	if err != nil {
		return nil, err
	}
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	rec, err := readDesign(ctx, client, url, did)
	if err != nil {
		return nil, err
	}
	d, err := reportdesign.Parse(rec.Design)
	if err != nil {
		return nil, fmt.Errorf("this design is not one this app can read (%v); update the app", err)
	}
	d.Name = designCopyName(d.Name)
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return createDesignDocument(ctx, client, url, d.Canonical(), false)
}

// designCopyName is a copy's name: the original's with " copy", cut to the core's
// bound on a name at a rune boundary.
func designCopyName(name string) string {
	const suffix = " copy"
	runes := []rune(strings.TrimSpace(name))
	if keep := reportdesign.MaxNameRunes - len([]rune(suffix)); len(runes) > keep {
		runes = runes[:keep]
	}
	return strings.TrimSpace(string(runes)) + suffix
}

func suggestReportColours(ctx context.Context, url, token, id string) (*ReportColourSuggestion, error) {
	did, err := designID(id)
	if err != nil {
		return nil, err
	}
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	rec, err := readDesign(ctx, client, url, did)
	if err != nil {
		return nil, err
	}
	d, err := reportdesign.Parse(rec.Design)
	if err != nil {
		return nil, fmt.Errorf("this design is not one this app can read (%v); update the app", err)
	}
	if d.Logo == (reportdesign.Logo{}) {
		return nil, errors.New("this design has no logo to read colours from: choose a logo first")
	}
	png, err := d.Logo.Decode()
	if err != nil {
		return nil, err
	}
	s, err := reportdesign.SuggestColours(png)
	if err != nil {
		return nil, err
	}
	return &ReportColourSuggestion{Accent: s.Accent, Secondary: s.Secondary, FromAccent: s.FromAccent, FromSecondary: s.FromSecondary}, nil
}

// uploadReportImage sends the file at path as one of the design's images:
// one multipart part named file, its bytes alone (not its name), at most
// the core's bound for that image. The core decides what the bytes are,
// encodes the image again from its pixels, and refuses with its own words
// (415 for anything but a PNG or a JPEG, 413 over a bound).
func uploadReportImage(ctx context.Context, url, token string, id uuid.UUID, kind reportAsset, path string) (*ReportDesignView, error) {
	f, err := os.Open(path) //nolint:gosec // the file the person picked
	if err != nil {
		return nil, fmt.Errorf("open the %s: %w", kind.what, err)
	}
	defer func() { _ = f.Close() }()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("the %s must be a file: choose a PNG or a JPEG", kind.what)
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(kind.maxUpload)+1))
	if err != nil {
		return nil, fmt.Errorf("read the %s: %w", kind.what, err)
	}
	if len(data) > kind.maxUpload {
		return nil, fmt.Errorf("a %s is at most %d KB; nothing changed", kind.what, kind.maxUpload>>10)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "image")
	if err == nil {
		_, err = part.Write(data)
	}
	if err == nil {
		err = mw.Close()
	}
	if err != nil {
		return nil, err
	}
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	var status int
	var problem *apiclient.Problem
	var raw []byte
	switch kind {
	case logoUpload:
		resp, err := client.PutUserReportDesignLogoWithBodyWithResponse(ctx, id, mw.FormDataContentType(), &body)
		if err != nil {
			return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
		}
		status, problem, raw = resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body
	default:
		resp, err := client.PutUserReportDesignCoverImageWithBodyWithResponse(ctx, id, mw.FormDataContentType(), &body)
		if err != nil {
			return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
		}
		status, problem, raw = resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body
	}
	if status != http.StatusOK {
		return nil, registry.Problem("set your "+kind.what, status, problem, raw)
	}
	rec, err := designDocument(raw)
	if err != nil {
		return nil, err
	}
	return reportDesignView(rec), nil
}

// removeReportAsset takes one image off, then answers the design as it is.
func removeReportAsset(ctx context.Context, url, token string, id uuid.UUID, kind reportAsset) (*ReportDesignView, error) {
	client, err := reportClient(url, token)
	if err != nil {
		return nil, err
	}
	var status int
	var problem *apiclient.Problem
	var raw []byte
	switch kind {
	case logoUpload:
		resp, err := client.DeleteUserReportDesignLogoWithResponse(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
		}
		status, problem, raw = resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body
	case coverImageUpload:
		resp, err := client.DeleteUserReportDesignCoverImageWithResponse(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
		}
		status, problem, raw = resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body
	default:
		resp, err := client.DeleteUserReportDesignCustomCoverWithResponse(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
		}
		status, problem, raw = resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body
	}
	if status != http.StatusOK {
		return nil, registry.Problem("remove your "+kind.what, status, problem, raw)
	}
	rec, err := designDocument(raw)
	if err != nil {
		return nil, err
	}
	return reportDesignView(rec), nil
}

// reportThemeFor is the design a local run or chat at reg takes: the design
// the person picked in the run setup, by its id, or with no pick the
// account's default; read with the app's stored credential for that
// registry. A pick that cannot be read refuses the run.
func (r *RunAPI) reportThemeFor(ctx context.Context, reg, picked string, logw io.Writer) (engine.ReportTheme, error) {
	url := settings.Normalize(reg)
	return accountReportTheme(ctx, url, settings.TokenFor(url), picked, logw)
}

// accountReportTheme is a run's report design, whole with its images, as
// the CLI's chooser takes a design of the library: the zero value (Plain,
// chosen by the default) when there is no credential, the account has no
// default and nothing was picked, or the registry has no library. A
// registry address Settings would not accept is never sent the token. A
// design that was picked and cannot be read, or that this build cannot
// draw, is an error that refuses the run, since the person chose it, as
// the CLI refuses a flag that names no design; a default that cannot be
// read says so in the run's log and is Plain, since how a PDF looks is no
// reason to stop a run the person did not choose a look for.
func accountReportTheme(ctx context.Context, url, token, picked string, logw io.Writer) (engine.ReportTheme, error) {
	theme, err := pickedReportTheme(ctx, url, token, picked)
	if err != nil {
		if strings.TrimSpace(picked) != "" {
			return engine.ReportTheme{}, fmt.Errorf("the report design picked for this run cannot be used: %w", err)
		}
		_, _ = fmt.Fprintf(logw, "agentry: warning: %v; PDFs use Plain\n", err)
		return engine.ReportTheme{}, nil
	}
	return theme, nil
}

// pickedReportTheme is accountReportTheme's reading, with its reasons.
func pickedReportTheme(ctx context.Context, url, token, picked string) (engine.ReportTheme, error) {
	picked = strings.TrimSpace(picked)
	if token == "" {
		if picked != "" {
			return engine.ReportTheme{}, errors.New("the picked report design needs the sign-in it was picked under, and this app is signed out")
		}
		return engine.ReportTheme{}, nil
	}
	if settings.ValidateURL(url) != nil {
		return engine.ReportTheme{}, errors.New("the run's registry address is not one Settings accepts, so your report design was not read")
	}
	ctx, cancel := context.WithTimeout(ctx, reportDesignReadTimeout)
	defer cancel()
	client, err := reportClient(url, token)
	if err != nil {
		return engine.ReportTheme{}, err
	}
	var id uuid.UUID
	from := engine.ThemeFromLibraryFlag
	if picked != "" {
		if id, err = designID(picked); err != nil {
			return engine.ReportTheme{}, err
		}
	} else {
		lib, err := reportDesignLibrary(ctx, url, token)
		if err != nil {
			return engine.ReportTheme{}, fmt.Errorf("your account's report designs could not be read (%w)", err)
		}
		if !lib.Available || lib.Default == "" {
			return engine.ReportTheme{}, nil
		}
		if id, err = designID(lib.Default); err != nil {
			return engine.ReportTheme{}, err
		}
		from = engine.ThemeFromAccount
	}
	rec, err := readDesign(ctx, client, url, id)
	if err != nil {
		return engine.ReportTheme{}, fmt.Errorf("your report design could not be read (%w)", err)
	}
	d, err := reportdesign.Parse(rec.Design)
	if err == nil {
		// The engine stamps the design with the pinned CLI's copy of the
		// core's rules, which a core bump could move apart from this
		// build's: a design that copy refuses would fail the run in
		// grantPDF, so it is held to that copy here too.
		_, err = toolsrv.StampTheme(d.Canonical())
	}
	if err != nil {
		return engine.ReportTheme{}, fmt.Errorf("your report design is not one this app can draw (%w; update the app)", err)
	}
	return engine.ReportTheme{Design: d.Canonical(), From: from}, nil
}
