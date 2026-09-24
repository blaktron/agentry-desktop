// Package validate is the author drop-zone engine: offline parity with the
// server's submit validation (D-12) — the manifest rules via pkg/manifest
// (the rule ids it emits are V1–V5 and V7–V10: V6 was withdrawn with the
// mcp-server kind; agentry-desktop#51), the S1 inventory subset via the
// pkg/archive content sniffers, and the S3/S4 scan via pkg/rules over the
// EMBEDDED rule bundle (pkg/ruleset).
// Parity is by construction: these are the same packages the pipeline runs.
//
// One deliberate departure from the CLI's `agentry validate`: a folder with
// no agent.yaml still gets scanned (every regular file, with a warning and a
// failed manifest check) instead of erroring out — a drop-zone that refuses
// to look at a folder teaches authors to stop dropping folders in.
package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"
	"github.com/blaktron/agentry/pkg/rules"
	"github.com/blaktron/agentry/pkg/ruleset"
)

// Issue is one manifest validation result.
type Issue struct {
	Rule    string `json:"rule"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Warning bool   `json:"warning"`
}

// InventoryFinding is one S1-subset result.
type InventoryFinding struct {
	Scanner  string `json:"scanner"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	FilePath string `json:"filePath"`
	Detail   string `json:"detail"`
}

// ScanFinding is one S3/S4 finding, flattened for the frontend.
type ScanFinding struct {
	RuleID     string   `json:"ruleId"`
	Scanner    string   `json:"scanner"`
	Severity   string   `json:"severity"`
	Confidence string   `json:"confidence"`
	Title      string   `json:"title"`
	FilePath   string   `json:"filePath"`
	LineStart  int      `json:"lineStart"`
	LineEnd    int      `json:"lineEnd"`
	Excerpt    string   `json:"excerpt"`
	Taxonomy   []string `json:"taxonomy,omitempty"`
}

// Result is one drop-zone run.
type Result struct {
	Dir string `json:"dir"`

	// Manifest header (empty when agent.yaml was missing/unparsable).
	ManifestName    string   `json:"manifestName,omitempty"`
	ManifestModes   []string `json:"manifestModes,omitempty"`
	ManifestVersion string   `json:"manifestVersion,omitempty"`
	// ManifestError is set when agent.yaml is missing or does not parse; the
	// scan still runs over every file, but the run cannot pass.
	ManifestError string  `json:"manifestError,omitempty"`
	Issues        []Issue `json:"issues"`

	Inventory []InventoryFinding `json:"inventory"`
	Findings  []ScanFinding      `json:"findings"`

	BundleVersion string   `json:"bundleVersion"`
	BundleSource  string   `json:"bundleSource"`
	FileCount     int      `json:"fileCount"`
	ReadErrors    []string `json:"readErrors,omitempty"`

	ErrorCount       int  `json:"errorCount"`
	WarnCount        int  `json:"warnCount"`
	InventoryFailing int  `json:"inventoryFailing"`
	Passed           bool `json:"passed"`
}

// scanFile is one file of the resolved include set.
type scanFile struct {
	path    string
	content []byte
}

// Run validates dir. The error return is reserved for the engine itself
// failing (unwalkable dir, unloadable embedded bundle) — findings, manifest
// problems, and read errors are data in the Result.
func Run(dir string) (*Result, error) {
	res := &Result{Dir: dir, Issues: []Issue{}, Inventory: []InventoryFinding{}, Findings: []ScanFinding{}}

	files, err := archive.WalkWorkspace(dir)
	if err != nil {
		return nil, fmt.Errorf("walk workspace %s: %w", dir, err)
	}

	var m *manifest.Manifest
	raw, rerr := os.ReadFile(filepath.Join(dir, "agent.yaml")) //nolint:gosec // user-picked workspace
	switch {
	case rerr != nil && os.IsNotExist(rerr):
		res.ManifestError = "no agent.yaml in this folder — manifest rules skipped, every file scanned"
	case rerr != nil:
		res.ManifestError = fmt.Sprintf("read agent.yaml: %v", rerr)
	default:
		m, err = manifest.Parse(raw)
		if err != nil {
			m = nil
			res.ManifestError = fmt.Sprintf("agent.yaml does not parse: %v", err)
		}
	}

	if m != nil {
		res.ManifestName, res.ManifestVersion = m.Name, m.Version
		for _, mode := range m.SupportedModes() {
			res.ManifestModes = append(res.ManifestModes, string(mode))
		}
		// Server parity context: namespace/name from the manifest itself,
		// Public so V7 runs, no tag, no existing versions — the same context
		// the CLI validates in.
		issues := manifest.ValidateInContext(m, manifest.Context{
			Namespace: m.Namespace,
			Name:      m.Name,
			Public:    true,
			Files:     files,
		})
		for _, ierr := range issues {
			warning := manifest.IsWarning(ierr)
			if warning {
				res.WarnCount++
			} else {
				res.ErrorCount++
			}
			if issue, ok := manifest.AsIssue(ierr); ok {
				res.Issues = append(res.Issues, Issue{Rule: issue.Rule, Path: issue.Path, Message: issue.Message, Warning: warning})
				continue
			}
			res.Issues = append(res.Issues, Issue{Message: ierr.Error(), Warning: warning})
		}
		sort.SliceStable(res.Issues, func(i, j int) bool {
			return !res.Issues[i].Warning && res.Issues[j].Warning
		})
	}

	// Scan exactly what publish would ship when there is a manifest (the
	// resolved include set); every regular file otherwise.
	scanSet := files
	if m != nil {
		scanSet = manifest.ResolveInclude(m.Publish.Include, files)
	}
	loaded := make([]scanFile, 0, len(scanSet))
	for _, fi := range scanSet {
		if fi.Symlink {
			continue // V4 already rejects symlinks
		}
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(fi.Path))) //nolint:gosec // paths come from WalkWorkspace
		if err != nil {
			res.ReadErrors = append(res.ReadErrors, err.Error())
			res.ErrorCount++
			continue
		}
		loaded = append(loaded, scanFile{path: fi.Path, content: content})
	}
	res.FileCount = len(loaded)

	// S1 subset (D-12): magic bytes, never extensions — the inventory checks
	// that used to reject a workspace at submit after it passed locally.
	res.Inventory = scanS1(loaded)
	for _, f := range res.Inventory {
		if f.Severity != "info" {
			res.InventoryFailing++
		}
	}

	// S3/S4 over the embedded bundle. A bundle that will not load is a hard
	// error, never a skipped scan: reporting PASSED over content the engine
	// never examined is the failure mode §7 forbids.
	bundle, err := ruleset.Load()
	if err != nil {
		return nil, fmt.Errorf("load embedded rule bundle: %w", err)
	}
	res.BundleVersion, res.BundleSource = bundle.Version(), "embedded"

	rf := make([]rules.File, len(loaded))
	for i, f := range loaded {
		rf[i] = rules.File{Path: f.path, Content: f.content}
	}
	findings := rules.Scan(bundle, rf)
	sortFindings(findings)
	for _, f := range findings {
		res.Findings = append(res.Findings, ScanFinding{
			RuleID: f.RuleID, Scanner: f.Scanner,
			Severity: string(f.Severity), Confidence: string(f.Confidence),
			Title: f.Title, FilePath: f.FilePath,
			LineStart: f.LineStart, LineEnd: f.LineEnd,
			Excerpt: f.Excerpt, Taxonomy: f.Taxonomy,
		})
	}

	res.Passed = res.ManifestError == "" && res.ErrorCount == 0 &&
		res.InventoryFailing == 0 && len(res.Findings) == 0
	return res, nil
}

// scanS1 runs the S1 subset over already-read file contents — the same
// classification, severities, and wording the pipeline's S1 stage and the
// CLI's local scan produce, built on the same pkg/archive predicates.
func scanS1(files []scanFile) []InventoryFinding {
	var out []InventoryFinding
	for _, f := range files {
		switch {
		case archive.IsExecutable(f.content):
			out = append(out, InventoryFinding{
				Scanner: "S1/binary-executable", Severity: "critical",
				Title:    "Executable binary in package",
				FilePath: f.path,
				Detail:   "executables and shared libraries are blocked for all kinds in v1",
			})
		case archive.IsArchiveData(f.content):
			out = append(out, InventoryFinding{
				Scanner: "S1/nested-archive", Severity: "critical",
				Title:    "Nested archive in package",
				FilePath: f.path,
				Detail:   "nested archives are not allowed (unpack depth > 1)",
			})
		case archive.IsImagePath(f.path):
			// An image: not text, not a finding.
		case archive.LooksText(archive.Head(f.content)):
			if !utf8.Valid(f.content) {
				out = append(out, InventoryFinding{
					Scanner: "S1/non-utf8", Severity: "low",
					Title:    "Non-UTF-8 text file",
					FilePath: f.path,
					Detail:   "the registry inventories package text as UTF-8",
				})
			}
		default:
			out = append(out, InventoryFinding{
				Scanner: "S1/opaque-binary", Severity: "info",
				Title:    "Opaque binary file",
				FilePath: f.path,
				Detail:   "not text, not an image — a reviewer will be asked to look",
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FilePath != out[j].FilePath {
			return out[i].FilePath < out[j].FilePath
		}
		return out[i].Scanner < out[j].Scanner
	})
	return out
}

// severityRank orders finding severities most-severe-first for display.
var severityRank = map[rules.Severity]int{
	rules.SeverityCritical: 0,
	rules.SeverityHigh:     1,
	rules.SeverityMedium:   2,
	rules.SeverityLow:      3,
	rules.SeverityInfo:     4,
}

// sortFindings sorts findings by severity, then path, then line, then rule.
func sortFindings(findings []rules.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		if a.FilePath != b.FilePath {
			return a.FilePath < b.FilePath
		}
		if a.LineStart != b.LineStart {
			return a.LineStart < b.LineStart
		}
		return a.RuleID < b.RuleID
	})
}
