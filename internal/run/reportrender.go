package run

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/owner"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage/sockfilter"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-desktop/internal/runfile"
)

// A report design's render: the fixed sample report laid out in a design by
// the runner image's own export_pdf, on this computer, as a run's PDF is.
//
// The runner image's tools server runs alone in one container, configured by
// a tools spec that grants export_pdf and the scratch folder, with the
// design stamped into it as the engine stamps a run's. The container takes
// the flags the run's docker filter gives a run's tools server: no network
// at all, every capability dropped and only the tools server's own given
// back, no new privileges, a read-only root, /tmp and the scratch folder
// each a tmpfs of the run's bound, the policy's memory, CPU and process
// limits, and the policy's runtime, read back from the engine before it
// starts. One bound differs: the filter also holds a tools server's swap to
// its memory, which a container started here cannot be given, so swap is
// the engine's default. Typst then runs inside it under export_pdf's own
// confinement and its own time and address-space limits. The sample is a
// file the app writes; the container gets an empty, read-only workspace and
// an output folder of its own, made fresh for each render and removed after
// it, and a folder an earlier app left behind is swept at the next render.
//
// What comes back is held to the checks the run viewer applies to a run's
// export: the PDF the ledger names, under reports/, read with no link at any
// part of its path, starting %PDF- and hashing to the SHA-256 the ledger
// records. Nothing else the folder holds is read.

//go:embed reportsample.md
var reportSample string

// ReportSampleAuthor is the sample report's author, as its {author} token
// expands. ReportSampleTitle is its title.
const (
	ReportSampleAuthor = "The review team"
	ReportSampleTitle  = "Quarterly review"
)

// reportRenderTimeout bounds the render container from its start to its
// end, beyond export_pdf's own limit on Typst. The runner image is readied
// before it, under the caller's context alone: on first use it is built.
const reportRenderTimeout = 3 * time.Minute

// maxRenderAnswer bounds what the tools server may write to stdout: a few
// JSON-RPC answers.
const maxRenderAnswer = 1 << 20

// The sample's paths in the output folder, and the ledger's.
const (
	reportSampleSource = "reports/sample.md"
	reportSamplePDF    = "reports/sample.pdf"
	reportRenderLedger = "receipts/" + engine.PDFLedgerFile
)

// ReportRender is a render's PDF and what the ledger says of it.
type ReportRender struct {
	PDF    []byte
	SHA256 string
	Pages  int
	// Cover is the cover style drawn before the report, "" for none.
	Cover string
	// Logo says the design's logo was drawn.
	Logo bool
}

// RenderReportSample lays the sample report out in design, a theme
// document the pinned CLI's copy of the rules accepts, on this computer's
// container engine. logw takes the image build's progress.
func RenderReportSample(ctx context.Context, design []byte, logw io.Writer) (*ReportRender, error) {
	stamp, err := toolsrv.StampTheme(design)
	if err != nil {
		return nil, fmt.Errorf("the design cannot be drawn by this app (%w)", err)
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, _, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, err
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	docker := newDockerClient()
	if err := docker.Available(ctx); err != nil {
		return nil, fmt.Errorf("the container engine does not answer, and a render runs in a container: %w", err)
	}
	eng, err := dockerx.DescribeEngine(ctx, docker)
	if err != nil {
		return nil, fmt.Errorf("read the container engine: %w", err)
	}
	tag := runnerImageTag(pol)
	if err := gateway.EnsureRunnerImage(ctx, docker, pol, tag, filepath.Join(home, "image"), logw); err != nil {
		return nil, err
	}
	return renderReportSample(ctx, docker, reportRenderSetup{
		image: tag, root: filepath.Join(home, "report-render"), engine: eng,
		runtime: pol.Docker.Runtime, limits: pol.Docker.Limits.WithDefaults(),
	}, stamp)
}

// reportRenderSetup is where and how a render's container runs.
type reportRenderSetup struct {
	image, root, runtime string
	engine               dockerx.Engine
	limits               policy.Limits
}

func renderReportSample(ctx context.Context, d dockerx.Client, rs reportRenderSetup, stamp *toolsrv.ThemeStamp) (*ReportRender, error) {
	if err := os.MkdirAll(rs.root, 0o700); err != nil {
		return nil, fmt.Errorf("make the render's folder: %w", err)
	}
	sweepRenderFolders(rs.root, time.Now().Add(-staleRender))
	dir, err := os.MkdirTemp(rs.root, "render-")
	if err != nil {
		return nil, fmt.Errorf("make the render's folder: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	planDir, workDir, outDir := filepath.Join(dir, "plan"), filepath.Join(dir, "workspace"), filepath.Join(dir, "out")
	for _, p := range []string{planDir, workDir, filepath.Join(outDir, "reports"), filepath.Join(outDir, "receipts")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return nil, fmt.Errorf("make the render's folder: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, filepath.FromSlash(reportSampleSource)), []byte(reportSample), 0o644); err != nil { //nolint:gosec // read by the render container
		return nil, fmt.Errorf("write the sample report: %w", err)
	}
	ts := reportRenderSpec(stamp, rs.engine)
	b, err := json.MarshalIndent(&ts, "", "  ")
	if err != nil {
		return nil, err
	}
	specFile := filepath.Join(planDir, "toolspec.json")
	if err := os.WriteFile(specFile, append(b, '\n'), 0o644); err != nil { //nolint:gosec // read by the render container
		return nil, fmt.Errorf("write the render's tools spec: %w", err)
	}

	runID, err := engine.NewRunID()
	if err != nil {
		return nil, err
	}
	s := reportRenderContainer(rs, runID, specFile, workDir, outDir)
	ctx, cancel := context.WithTimeout(ctx, reportRenderTimeout)
	defer cancel()
	defer func() { _ = d.RemoveForce(context.WithoutCancel(ctx), s.Name) }()
	stdout := &cappedBuffer{max: maxRenderAnswer}
	stderr := &cappedBuffer{max: 64 << 10}
	code, err := dockerx.RunForegroundUnder(ctx, d, s, "report render", strings.NewReader(reportRenderCalls()), stdout, stderr)
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("the render took longer than %s and was stopped", reportRenderTimeout)
		}
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("start the render's container: %w", err)
	}
	answer, failed, found := exportAnswer(stdout.Bytes())
	switch {
	case found && failed:
		return nil, fmt.Errorf("the design could not be laid out: %s", answer)
	case !found:
		return nil, fmt.Errorf("the render's container ended (exit %d) without laying the sample out%s", code, lastLine(stderr.String()))
	}
	return readRender(outDir)
}

// reportRenderSpec is the tools spec of a render: export_pdf granted with
// the design stamped, the scratch folder and the run's tmpfs bound, every
// other tool absent.
func reportRenderSpec(stamp *toolsrv.ThemeStamp, eng dockerx.Engine) toolsrv.Spec {
	ts := toolsrv.Spec{
		SpecVersion:  toolsrv.SpecVersion,
		WorkspaceDir: "/workspace",
		OutDir:       spec.OutMount,
		ScratchDir:   spec.ScratchMount,
		TmpfsBytes:   spec.ToolsTmpfsBytes,
		PDF: toolsrv.PDF{
			Enabled: true,
			Theme:   stamp,
			Author:  ReportSampleAuthor,
			Ledger:  path.Join(spec.OutMount, reportRenderLedger),
		},
	}
	if uid, gid, ok := eng.PersonIDs(); ok {
		ts.Owner = &toolsrv.Owner{UID: uid, GID: gid}
	}
	return ts
}

// reportRenderContainer is the render's container, held as the run's docker
// filter holds a run's tools server.
func reportRenderContainer(rs reportRenderSetup, runID, specFile, workDir, outDir string) dockerx.RunSpec {
	tmpfs := fmt.Sprintf("size=%d,mode=1777", spec.ToolsTmpfsBytes)
	s := dockerx.RunSpec{
		Name:  "agentry-" + runID + "-report-render",
		Image: rs.image,
		Argv:  spec.ToolsCmd(),
		Mounts: []dockerx.Mount{
			{Source: specFile, Target: spec.ToolSpecMount, ReadOnly: true, Relabel: true},
			{Source: workDir, Target: "/workspace", ReadOnly: true, Relabel: true},
			{Source: outDir, Target: spec.OutMount, Relabel: true},
		},
		Network:     "none",
		Labels:      owner.With(map[string]string{"agentry.run": runID}),
		AutoRemove:  true,
		NoLog:       true,
		Interactive: true,
		Runtime:     rs.runtime,
	}
	s = s.WithPosture(rs.limits.Memory, rs.limits.CPUs, rs.limits.Pids)
	s.CapAdd = sockfilter.ToolsCapabilities()
	s.ReadOnly = true
	s.Tmpfs = []string{spec.ToolsTmpMount + ":" + tmpfs, spec.ScratchMount + ":" + tmpfs}
	s.DisableSELinuxLabel = rs.engine.SandboxUnlabelled(rs.runtime)
	return s
}

// reportRenderCalls are the lines the render sends the tools server: the
// handshake, then export_pdf on the sample in the report form. The server
// answers each line before it reads the next and ends at the end of its
// input.
func reportRenderCalls() string {
	lines := []any{
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
			"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "agentry-desktop-report-render", "version": "1"},
		}},
		map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{
			"name": toolsrv.ToolExportPDF, "arguments": map[string]any{"source": reportSampleSource, "path": path.Base(reportSamplePDF), "form": toolsrv.FormReport},
		}},
	}
	var b strings.Builder
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b.Write(j)
		b.WriteByte('\n')
	}
	return b.String()
}

// exportAnswer is export_pdf's answer among the server's lines: its text,
// whether it is an error, and whether there was one.
func exportAnswer(out []byte) (text string, isErr, found bool) {
	for _, line := range bytes.Split(out, []byte("\n")) {
		var r struct {
			ID     json.RawMessage `json:"id"`
			Result *struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(line, &r) != nil || string(r.ID) != "2" {
			continue
		}
		if r.Error != nil {
			return r.Error.Message, true, true
		}
		if r.Result == nil {
			return "", true, true
		}
		parts := make([]string, 0, len(r.Result.Content))
		for _, c := range r.Result.Content {
			parts = append(parts, c.Text)
		}
		return strings.Join(parts, "\n"), r.Result.IsError, true
	}
	return "", false, false
}

// staleRender is how old a render folder must be for the sweep to take it:
// older than any render runs, so another app's render in progress is left.
const staleRender = time.Hour

// sweepRenderFolders removes render folders under root last changed before
// cutoff: what an app that was stopped mid-render left.
func sweepRenderFolders(root string, cutoff time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "render-") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

// maxRenderLedger bounds the ledger's read: one line is all a render writes.
const maxRenderLedger = 64 << 10

// readRender reads the render back from its output folder: the one PDF the
// ledger names at the sample's path, under reports/, held to the run
// viewer's checks. The container could write either file, so both are read
// with no link at any part of their paths.
func readRender(outDir string) (*ReportRender, error) {
	raw, ok := runfile.Read(outDir, reportRenderLedger, maxRenderLedger)
	if !ok {
		return nil, errors.New("the render's record could not be read: it is missing, not a regular file, reached through a link, or too large")
	}
	var entries []toolsrv.PDFEntry
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e toolsrv.PDFEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("the render's record could not be read: %w", err)
		}
		entries = append(entries, e)
	}
	if len(entries) != 1 || entries[0].Path != reportSamplePDF {
		return nil, errors.New("the render's record does not name the sample's PDF alone, so nothing is shown")
	}
	e := entries[0]
	b, ok := runfile.Read(outDir, e.Path, toolsrv.MaxPDFBytes)
	if !ok {
		return nil, errors.New("the render's PDF could not be read: it is gone, not a regular file, reached through a link, or too large")
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return nil, errors.New("the render's file is not a PDF, so it is not shown")
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != strings.ToLower(e.SHA256) {
		return nil, errors.New("the render's PDF is not the one its record names, so it is not shown")
	}
	return &ReportRender{PDF: b, SHA256: strings.ToLower(e.SHA256), Pages: max(e.Pages, 0), Cover: e.Cover, Logo: e.Logo}, nil
}

// cappedBuffer keeps the first max bytes written to it and drops the rest.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte  { return c.buf.Bytes() }
func (c *cappedBuffer) String() string { return c.buf.String() }

// lastLine is the last non-empty line of a container's stderr, as a
// sentence's tail: ": <line>", or "" for none.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	l := strings.TrimSpace(lines[len(lines)-1])
	if l == "" {
		return ""
	}
	if r := []rune(l); len(r) > 300 {
		l = string(r[:300]) + "…"
	}
	return ": " + l
}
