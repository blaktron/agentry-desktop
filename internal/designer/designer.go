// Package designer runs the built-in Report Designer on this computer, as
// `agentry report-design design | cover` does, against the model the runner
// policy names: the CLI's own run (pkg/reportdesigner/build), with the
// person's images read from the files a native picker returned and the web
// pages they named read by AI Review with their own TypeSafe key first.
//
// What it makes is a new design, written to the run's output folder and
// held to the core's rules, with only the images the person gave or the
// design already had. Nothing reaches the account's library here: the
// person sees the result first, and saving it is a step of its own.
package designer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry/pkg/reportdesign"

	"github.com/blaktron/agentry-cli/pkg/reportdesigner/build"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
	"github.com/blaktron/agentry-desktop/internal/voice"
)

// The two jobs, in the words the page and the hosted run use.
const (
	JobDesign = string(build.Design)
	JobCover  = string(build.Cover)
)

// The limits of one run's inputs: the CLI's, and the image file's cap
// before the core's own after decoding.
const (
	MaxBriefBytes  = build.MaxBriefBytes
	MaxReferences  = build.MaxReferences
	MaxURLs        = build.MaxURLs
	MaxImageBytes  = 4 << 20
	MaxCoverImages = reportdesign.MaxCoverImages
)

// NoReviewKey is why web addresses are refused on this computer without the
// person's own TypeSafe key, in the words the dialog and the run use.
const NoReviewKey = "AI Review reads every page from a web address before the Report Designer does, and that takes your own TypeSafe key: " +
	"add one under Settings → Provider settings → AI Review key, or leave the addresses out"

// Builder is the Report Designer's run: agentry-cli's pkg/reportdesigner/build,
// or a test's.
type Builder func(ctx context.Context, d dockerx.Client, o build.Options) (*build.Result, error)

// Request is one Report Designer run on this computer.
type Request struct {
	// Job is JobDesign or JobCover.
	Job string
	// Brief is what the person wants, at most 4 KiB.
	Brief string
	// From is the design a Design starts from (optional) or a Cover is
	// written for (required), with its images.
	From *reportdesign.Design
	// Logo is a Design's logo file and Images the reference images (a
	// Design) or the images a cover may draw (a Cover), each a file a
	// native picker returned.
	Logo   string
	Images []string
	// URLs are a Design's web addresses, each page read by AI Review with
	// the person's own TypeSafe key first.
	URLs []string
	// Document is a Design's one document to build the design around, a
	// file a native picker returned; "" for none.
	Document string
	// Home is the runner home; OutDir the folder the run writes its
	// reports, receipts and the design under design/ into.
	Home    string
	OutDir  string
	RunID   string
	Build   Builder
	Log     io.Writer
	OnUsage func(receipt.Usage)
}

// Result is what a run made: the design, never anyone's default, and what
// the run said of it.
type Result struct {
	Design reportdesign.Design
	// Looked is the Designer's own word that it saw its renders.
	Looked bool
	// Thumbnail says a Cover's design carries the image of its last render.
	Thumbnail bool
	Previews  int
	// Dir is the folder the design was written to (design.json inside).
	Dir   string
	Pages []voice.Page
	// DocumentPages is how many images of the document the Designer looked
	// at; LogoFromDocument says the design's logo is a crop of it.
	DocumentPages    int
	LogoFromDocument bool
}

// Check holds a request to the rules the run holds it to, before anything
// is read: the brief, the job, how many images and addresses, each address
// as the CLI parses it, a Cover's design, and that only a Design takes a
// document.
func Check(job, brief string, from bool, logo string, images, urls []string, document string) error {
	b := strings.TrimSpace(brief)
	switch {
	case b == "":
		return errors.New("say what your reports are and how they should look: the Report Designer works from a brief")
	case len(brief) > MaxBriefBytes:
		return fmt.Errorf("a brief is at most %d KiB", MaxBriefBytes>>10)
	case !utf8.ValidString(brief):
		return errors.New("a brief is UTF-8 text")
	}
	addrs := knowledge.TrimmedLines(urls)
	switch job {
	case JobDesign:
		if len(images) > MaxReferences {
			return fmt.Errorf("a design takes at most %d reference images", MaxReferences)
		}
		if len(addrs) > MaxURLs {
			return fmt.Errorf("a design takes at most %d web addresses", MaxURLs)
		}
		if p := voice.CheckAddresses(addrs).First(); p != "" {
			return fmt.Errorf("the web addresses: %s", p)
		}
	case JobCover:
		if !from {
			return errors.New("a cover is written for one of your designs: choose it first")
		}
		if logo != "" || len(addrs) > 0 || document != "" {
			return errors.New("a cover takes the design's own logo, no web addresses and no document")
		}
		if len(images) > MaxCoverImages {
			return fmt.Errorf("a cover draws at most %d images", MaxCoverImages)
		}
	default:
		return fmt.Errorf("%q is not design or cover", job)
	}
	return nil
}

// CheckCoverImages holds a Cover's images to the most a cover draws,
// counting those the design's own custom cover already draws.
func CheckCoverImages(job string, from reportdesign.Design, images int) error {
	if job != JobCover {
		return nil
	}
	if have := len(from.CustomCover.Images); have+images > MaxCoverImages {
		return fmt.Errorf("a cover draws at most %d images, and the design's cover already has %d", MaxCoverImages, have)
	}
	return nil
}

// ReadImage reads an image file the person picked, within the cap the CLI
// holds a file to before the core's own after decoding.
func ReadImage(what, file string) ([]byte, error) {
	f, err := os.Open(file) //nolint:gosec // a file a native picker returned
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, MaxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", what, filepath.Base(file), err)
	}
	if len(b) > MaxImageBytes {
		return nil, fmt.Errorf("%s %s is over %d MB", what, filepath.Base(file), MaxImageBytes>>20)
	}
	return b, nil
}

// Model names the model the runner policy gives the Report Designer.
func Model() string { return knowledge.PolicyModel() }

// Run runs the Report Designer on this computer against the runner
// policy's model. It costs no credits; the pages from web addresses are
// read by AI Review first, and nothing else it reads or writes is reviewed.
func Run(ctx context.Context, req Request) (*Result, error) {
	if err := Check(req.Job, req.Brief, req.From != nil, req.Logo, req.Images, req.URLs, req.Document); err != nil {
		return nil, err
	}
	if req.From != nil {
		if err := CheckCoverImages(req.Job, *req.From, len(req.Images)); err != nil {
			return nil, err
		}
	}
	var logo []byte
	if req.Logo != "" {
		var err error
		if logo, err = ReadImage("the logo", req.Logo); err != nil {
			return nil, err
		}
	}
	var doc *build.Document
	if req.Document != "" {
		var err error
		if doc, err = ReadDocument(req.Document); err != nil {
			return nil, err
		}
		if _, err := build.DocumentKind(doc.Name, doc.Data); err != nil {
			return nil, fmt.Errorf("the document %s: %w", doc.Name, err)
		}
	}
	var images [][]byte
	for _, p := range req.Images {
		b, err := ReadImage("the image", p)
		if err != nil {
			return nil, err
		}
		images = append(images, b)
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, polSHA, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	logw := req.Log
	if logw == nil {
		logw = io.Discard
	}
	urls, review, web, err := voice.PageReview(pol, req.URLs, NoReviewKey)
	if err != nil {
		return nil, err
	}
	reviewed := "nothing it reads or writes is reviewed"
	if len(urls) > 0 {
		reviewed = "nothing it reads from your files, or writes, is reviewed"
	}
	_, _ = fmt.Fprintf(logw, "The Report Designer runs on this computer against the model your runner policy names (%s). "+
		"It costs no credits, and %s. It looks at its renders and your references when the model reads images.\n", knowledge.CreatorModel(pol), reviewed)
	if doc != nil {
		_, _ = fmt.Fprintf(logw, "It reads your document %s in full, after the same checks as any input, and keeps it only for the run.\n", doc.Name)
	}
	if len(urls) > 0 {
		_, _ = fmt.Fprintln(logw, "Each page from a web address is fetched first and read by AI Review with your TypeSafe key; one it flags is withheld. "+
			"A page drawn by scripts is rendered in a headless browser and read by AI Review there.")
	}
	b := req.Build
	if b == nil {
		b = build.Run
	}
	budgets := settings.BuiltinBudgets(localrun.ReportDesigner(), pol.Budgets).Run
	res, err := b(ctx, dockerx.New(), build.Options{
		Job: build.Job(req.Job), Brief: req.Brief, From: req.From, Logo: logo, Images: images, URLs: urls, Document: doc, Review: review, Web: web,
		Home: req.Home, Policy: pol, PolicySHA: polSHA, PolicyFile: &polFile, Build: version.Build(), Credits: run.CreditsClient(pol),
		OutDir: req.OutDir, Budgets: &budgets, RunID: req.RunID, Stderr: logw, OnUsage: req.OnUsage,
	})
	var pages []voice.Page
	if res != nil {
		for _, p := range res.Pages {
			pages = append(pages, voice.Page{URL: p.URL, Sample: p.Sample, Rendered: p.Rendered, Withheld: p.Withheld})
			if p.Withheld != "" {
				_, _ = fmt.Fprintf(logw, "withheld: %s (%s)\n", p.URL, p.Withheld)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return &Result{Design: res.Design, Looked: res.Looked, Thumbnail: res.Thumbnail, Previews: res.Previews, Dir: res.Dir, Pages: pages,
		DocumentPages: res.DocumentPages, LogoFromDocument: res.LogoFromDocument}, nil
}
