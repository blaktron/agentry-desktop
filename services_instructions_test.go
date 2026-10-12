package main

// The Agent tab's instruction file (agentry-desktop#835), against the fake
// registry Edit's tests use: the path is the manifest's entrypoint, and the
// file comes back as text with the registry's truncation and binary flags.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestInstructionsReadsTheFileTheManifestNames(t *testing.T) {
	f := newEditFake(t)
	signIn(t, f.url)
	b := &BrowseAPI{app: &App{ctx: context.Background()}}
	v, err := b.Instructions(f.url, "chris", "meeting-actions", "1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.Path != "PROMPT.md" || v.Content != "# Meeting actions\n\nRead the notes.\n" || v.Truncated || v.Binary {
		t.Fatalf("view %+v", v)
	}
	f.mu.Lock()
	asked := strings.Join(f.asked, "\n")
	f.mu.Unlock()
	if !strings.Contains(asked, "/versions/1.2.0/manifest") || !strings.Contains(asked, "/versions/1.2.0/file?path=PROMPT.md") {
		t.Fatalf("asked:\n%s", asked)
	}
}

func TestInstructionsDefaultsToAgentMD(t *testing.T) {
	f := newEditFake(t)
	signIn(t, f.url)
	b := &BrowseAPI{app: &App{ctx: context.Background()}}
	f.mu.Lock()
	f.manifest = strings.Replace(editManifest, `"entrypoints": {"instruction": "PROMPT.md"},`, "", 1)
	f.mu.Unlock()
	v, err := b.Instructions(f.url, "chris", "meeting-actions", "1.2.0")
	if err != nil || v.Path != "AGENT.md" {
		t.Fatalf("view %+v %v", v, err)
	}
	f.mu.Lock()
	asked := strings.Join(f.asked, "\n")
	f.mu.Unlock()
	if !strings.Contains(asked, "/versions/1.2.0/file?path=AGENT.md") {
		t.Fatalf("asked:\n%s", asked)
	}
}

func TestInstructionsCarriesTheRegistrysFlags(t *testing.T) {
	f := newEditFake(t)
	signIn(t, f.url)
	b := &BrowseAPI{app: &App{ctx: context.Background()}}
	f.mu.Lock()
	f.file["truncated"] = true
	f.mu.Unlock()
	v, err := b.Instructions(f.url, "chris", "meeting-actions", "1.2.0")
	if err != nil || !v.Truncated || v.Content == "" {
		t.Fatalf("truncated: %+v %v", v, err)
	}
	f.mu.Lock()
	f.file["binary"] = true
	delete(f.file, "content")
	f.mu.Unlock()
	if v, err = b.Instructions(f.url, "chris", "meeting-actions", "1.2.0"); err != nil || !v.Binary || v.Content != "" {
		t.Fatalf("binary: %+v %v", v, err)
	}
}

func TestInstructionsSaysWhenTheRegistryRefuses(t *testing.T) {
	f := newEditFake(t)
	signIn(t, f.url)
	b := &BrowseAPI{app: &App{ctx: context.Background()}}
	// The fake serves no other QuickAgent: its manifest is a 404.
	if _, err := b.Instructions(f.url, "chris", "nothing-here", "1.0.0"); err == nil {
		t.Fatal("a missing release read without an error")
	}
	// The file endpoint's own gate (a quarantined version is 409): the
	// refusal reaches the card, and no text does.
	f.mu.Lock()
	f.fileStatus = http.StatusConflict
	f.mu.Unlock()
	if v, err := b.Instructions(f.url, "chris", "meeting-actions", "1.2.0"); err == nil || v != nil {
		t.Fatalf("a refused file read: %+v %v", v, err)
	}
}
