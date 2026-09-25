package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// benignWorkspace mirrors agentry/examples/injection-defense — the fixture
// the product treats as the benign lookalike that MUST pass.
func benignWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"agent.yaml": `schemaVersion: "2"
name: injection-defense
namespace: agentry
modes: [autonomous]
version: 1.0.0
summary: "Review a document for prompt-injection risk before an agent reads it"
license: Apache-2.0
authors:
  - name: Agentry maintainers
tags: [security, prompt-injection, review]

entrypoints:
  instruction: SKILL.md

runtime:
  interfaces: [claude-skill]

capabilities: {}

publish:
  include: [agent.yaml, README.md, SKILL.md, LICENSE]
`,
		"SKILL.md":  "# Injection defense\n\nReview the document and report risk.\n",
		"README.md": "# injection-defense\n\nA benign skill.\n",
		"LICENSE":   "Apache License 2.0\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestValidateBenignWorkspacePasses(t *testing.T) {
	res, err := Run(benignWorkspace(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.ManifestError != "" {
		t.Fatalf("manifest error: %s", res.ManifestError)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("benign workspace produced findings: %+v", res.Findings)
	}
	if res.InventoryFailing != 0 {
		t.Fatalf("benign workspace failed inventory: %+v", res.Inventory)
	}
	if !res.Passed {
		t.Fatalf("benign workspace did not pass: errors=%d issues=%+v", res.ErrorCount, res.Issues)
	}
	if res.BundleVersion == "" || res.BundleSource != "embedded" {
		t.Errorf("bundle provenance missing: %q %q", res.BundleVersion, res.BundleSource)
	}
}

// TestValidateFlagsZeroWidthSpace is the §10.5 acceptance check: a hidden
// zero-width character in shipped prose must produce an S3 finding.
func TestValidateFlagsZeroWidthSpace(t *testing.T) {
	dir := benignWorkspace(t)
	poisoned := "# Injection defense\n\nReview the doc\u200bument and report risk.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(poisoned), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range res.Findings {
		if f.Scanner == "S3" && strings.Contains(strings.ToLower(f.RuleID+f.Title), "zero") {
			found = true
		}
	}
	if !found {
		t.Fatalf("zero-width space not flagged; findings: %+v", res.Findings)
	}
	if res.Passed {
		t.Fatal("a workspace with an S3 finding passed")
	}
}

// TestValidateWithoutManifestStillScans: the drop-zone scans everything and
// reports the missing manifest, instead of refusing to look.
func TestValidateWithoutManifestStillScans(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("hi\u200bdden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.ManifestError == "" {
		t.Error("missing agent.yaml not reported")
	}
	if res.Passed {
		t.Error("a manifest-less folder must not pass")
	}
	if len(res.Findings) == 0 {
		t.Errorf("payload file not scanned without a manifest: %+v", res)
	}
}

// TestValidateFlagsExecutable: S1 parity — a Mach-O/ELF magic under a prose
// extension must be flagged critical (detection by content, not name).
func TestValidateFlagsExecutable(t *testing.T) {
	dir := benignWorkspace(t)
	elf := append([]byte{0x7f, 'E', 'L', 'F'}, make([]byte, 64)...)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), elf, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range res.Inventory {
		if f.Scanner == "S1/binary-executable" && f.FilePath == "README.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("renamed ELF not flagged: %+v", res.Inventory)
	}
	if res.Passed {
		t.Fatal("workspace with an executable passed")
	}
}

// TestValidateAGuidedManifestPasses: the pinned core carries manifest v2's
// optional guide block (DEV-141, agentry-desktop#228), so a workspace whose
// agent.yaml has one validates here instead of meeting the strict
// decoder's unknown-field refusal; a connect key that names no declared
// secret is still refused (V10).
func TestValidateAGuidedManifestPasses(t *testing.T) {
	dir := benignWorkspace(t)
	path := filepath.Join(dir, "agent.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	guided := string(raw) + `
guide:
  overview: |
    Reads the document and returns a **risk report**.
  prepare:
    - The document, as text or PDF
  files: "The document to review."
  task: "e.g. the vendor's onboarding guide"
`
	if err := os.WriteFile(path, []byte(guided), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.ManifestError != "" || !res.Passed {
		t.Fatalf("a guided manifest must validate: manifest error %q, errors=%d issues=%+v", res.ManifestError, res.ErrorCount, res.Issues)
	}

	undeclared := guided + "  connect:\n    GITHUB_TOKEN: \"A token with read access.\"\n"
	if err := os.WriteFile(path, []byte(undeclared), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("connect help for a secret the release does not declare must be refused")
	}
}
