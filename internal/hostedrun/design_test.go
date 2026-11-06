package hostedrun

import (
	"encoding/json"
	"strings"
	"testing"
)

// The Report Designer In Agentry is offered by one rule: absent signed out
// or where the registry keeps no report designs, and otherwise offered,
// chosen only when nothing stands in the way, each reason in a sentence.
func TestTheReportDesignerInAgentryIsOfferedByOneRule(t *testing.T) {
	scopes := []string{ScopeRunRead, ScopeRunWrite, ScopeUserProfile}
	ready := func() DesignFacts {
		return DesignFacts{SignedIn: true, Library: true, Runs: true, Scopes: scopes, ScopesKnown: true,
			Terms: &DesignTerms{Enabled: true, Credits: 20, MaxImages: 4, Ops: []string{DesignOp, CoverOp}}, StatusKnown: true}
	}
	cases := []struct {
		name      string
		facts     func() DesignFacts
		op        string
		offered   bool
		available bool
		reason    string
	}{
		{"signed out", func() DesignFacts { f := ready(); f.SignedIn = false; return f }, DesignOp, false, false, ""},
		{"no library here", func() DesignFacts { f := ready(); f.Library = false; return f }, DesignOp, false, false, ""},
		{"no runs extension", func() DesignFacts { f := ready(); f.Runs = false; return f }, DesignOp, true, false, "does not run anything In Agentry"},
		{"scopes unknown", func() DesignFacts { f := ready(); f.ScopesKnown = false; return f }, DesignOp, true, false, "did not say what the stored token may do"},
		{"token without user:profile", func() DesignFacts { f := ready(); f.Scopes = scopes[:2]; return f }, DesignOp, true, false, "user:profile"},
		{"status unanswered", func() DesignFacts { f := ready(); f.StatusKnown = false; return f }, DesignOp, true, false, "did not answer"},
		{"no Report Designer here", func() DesignFacts { f := ready(); f.Terms = nil; return f }, DesignOp, true, false, "not running the Report Designer"},
		{"switched off", func() DesignFacts { f := ready(); f.Terms.Enabled = false; return f }, CoverOp, true, false, "not running the Report Designer"},
		{"no covers here", func() DesignFacts { f := ready(); f.Terms.Ops = []string{DesignOp}; return f }, CoverOp, true, false, "custom covers"},
		{"library full", func() DesignFacts { f := ready(); f.Full = true; return f }, DesignOp, true, false, "nowhere to save"},
		{"a run in flight", func() DesignFacts { f := ready(); f.Active = &Run{Kind: KindVoiceBuild}; return f }, DesignOp, true, false, "already has a Voice Writer run"},
		{"ready to design", ready, DesignOp, true, true, ""},
		{"ready for a cover", ready, CoverOp, true, true, ""},
		{"a registry listing no jobs does both", func() DesignFacts { f := ready(); f.Terms.Ops = nil; return f }, CoverOp, true, true, ""},
	}
	for _, c := range cases {
		o := DecideDesign(c.facts(), c.op)
		if o.Offered != c.offered || o.Available != c.available || !strings.Contains(o.Reason, c.reason) {
			t.Errorf("%s: got %+v", c.name, o)
		}
		if o.Offered && !o.Available && o.Reason == "" {
			t.Errorf("%s: unavailable with no reason", c.name)
		}
	}
}

// The registry's words reach the app as they are: the terms, the caller's
// hold and the run's record, as x-runs.yaml writes them.
func TestAReportDesignerRunReadsAsOne(t *testing.T) {
	var st Status
	if err := json.Unmarshal([]byte(`{"enabled":true,"designBuild":{"enabled":true,"credits":20,"caps":{"timeoutSeconds":1800,"maxTurns":0},
		"maxImages":4,"maxBriefBytes":4096,"ops":["design","cover"],"urls":{"enabled":true,"max":2,"maxPages":60}},
		"caller":{"tier":"standard","designBuild":{"holdCredits":20}}}`), &st); err != nil {
		t.Fatal(err)
	}
	u, ok := st.DesignBuild.TakesURLs()
	if !ok || u.Max != 2 || st.DesignBuild.Credits != 20 || st.DesignBuild.MaxImages != 4 || st.Caller.DesignBuild.HoldCredits != 20 {
		t.Fatalf("%+v %+v", st.DesignBuild, st.Caller)
	}
	var r Run
	if err := json.Unmarshal([]byte(`{"id":"01a1ad22-83e0-7f23-ae55-edb2187c4a3b","kind":"design_build","status":"finished",
		"designBuild":{"op":"design","applied":true,"designId":"7b0e5f2e-6a8c-4a77-9d0f-2a7f3c2b1e10","name":"Northwind Harbour Trust reports"}}`), &r); err != nil {
		t.Fatal(err)
	}
	if !r.IsDesignBuild() || r.IsVoiceBuild() || ActiveWhat(&r) != "a Report Designer run" || !r.DesignBuild.Applied || r.DesignBuild.Name == "" {
		t.Fatalf("%+v", r)
	}
	for _, note := range []string{"nothing", "invalid", "gone", "full", "unavailable"} {
		if s := DesignNoteSentence(note); !strings.Contains(s, "refunded") {
			t.Errorf("%s: %q", note, s)
		}
	}
	if DesignNoteSentence("unknown") != "" {
		t.Fatal("an unknown note is not guessed")
	}
	var none *DesignTerms
	if _, ok := none.TakesURLs(); ok || none.Runs(DesignOp) {
		t.Fatal("no terms take nothing")
	}
}

// A create carries the Designer's own fields and nothing of another kind's.
func TestADesignCreateCarriesFromAndLogo(t *testing.T) {
	b, err := json.Marshal(CreateRequest{Kind: KindDesignBuild, Op: DesignOp, Brief: "b", From: "f", Logo: "l", Inputs: []string{"l", "i"}})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["from"] != "f" || m["logo"] != "l" || m["kind"] != "design_build" || m["op"] != "design" {
		t.Fatalf("%s", b)
	}
	for _, k := range []string{"ref", "task", "note", "name", "pack"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s carries %s", b, k)
		}
	}
}
