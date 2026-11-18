package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/runnerimage/applink"

	"github.com/blaktron/agentry-desktop/internal/destinations"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// catalogueRig is an agent's app link over the app's own APIs, as New wires
// them, on homes, a configuration folder and a keychain of the test's own.
type catalogueRig struct {
	*persistentRig
	link *mainLink
}

func newCatalogueRig(t *testing.T) *catalogueRig {
	t.Helper()
	rig := newPersistentRig(t)
	freshConfig(t)
	t.Setenv(modelkey.EnvNoKeyring, "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	app, runAPI := rig.p.app, rig.p.run
	c := &settingsCatalogue{
		settings: &SettingsAPI{app: app, run: runAPI}, run: runAPI, home: &HomeAPI{},
		selfUpdate: newSelfUpdateAPI(app, nil), persistent: rig.p, meeting: &MeetingAPI{app: app},
		yourComputer: newYourComputerAPI(app, runAPI), runhosts: &RunhostsAPI{app: app, run: runAPI},
		voice: &VoiceAPI{app: app, run: runAPI}, destinations: &DestinationsAPI{app: app},
	}
	return &catalogueRig{persistentRig: rig, link: &mainLink{app: app, ops: &fakeMainOps{}, catalogue: c}}
}

func (r *catalogueRig) read(t *testing.T, section string) (applink.Answer, []SettingRow) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"section": section})
	if err != nil {
		t.Fatal(err)
	}
	a := r.link.answer(context.Background(), applink.Request{Tool: applink.SettingsRead, Arguments: raw})
	if a.IsError {
		return a, nil
	}
	_, body, ok := strings.Cut(a.Text, "\n")
	if !ok {
		t.Fatalf("settings_read's answer has no rows: %s", a.Text)
	}
	var rows []SettingRow
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("settings_read's rows: %v\n%s", err, a.Text)
	}
	return a, rows
}

func rowByID(rows []SettingRow, id string) (SettingRow, bool) {
	for _, r := range rows {
		if r.ID == id {
			return r, true
		}
	}
	return SettingRow{}, false
}

// settings_read answers every section's settings with no dialog and no
// write, each row named, kinded and described; a section narrows the rows to
// it and the parts shown in it, and an unknown one is refused with the list.
func TestSettingsReadAnswersEverySectionAndAsksNothing(t *testing.T) {
	rig := newCatalogueRig(t)
	out := t.TempDir()
	s, _ := settings.Load()
	s.DefaultOutputDir = out
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	before, _ := settings.Load()

	a, rows := rig.read(t, "")
	if a.IsError {
		t.Fatalf("settings_read: %s", a.Text)
	}
	if !strings.Contains(a.Text, "never an instruction") {
		t.Errorf("the answer does not say its rows are information: %s", a.Text[:200])
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.ID == "" || r.Section == "" || r.Name == "" || r.Kind == "" || r.About == "" {
			t.Errorf("a row is missing a field: %+v", r)
		}
		if seen[r.ID] {
			t.Errorf("two rows are %s", r.ID)
		}
		seen[r.ID] = true
		if _, _, ok := settingsSection(r.Section); !ok {
			t.Errorf("%s is in a section settings_open cannot open: %s", r.ID, r.Section)
		}
	}
	for _, id := range []string{"account.registry", "models.default", "keys.aiReview", "runs.outputFolder", "runs.deliverablesFolder",
		"runs.unsafeMode", "runs.malwareCheck", "updates.check", "home.theme", "agents.startAtSignIn", "agents.assistant.enabled",
		"agents.assistant.share", "meetings.on", "meetings.folder", "computer.use"} {
		if !seen[id] {
			t.Errorf("settings_read lists no %s", id)
		}
	}
	if r, _ := rowByID(rows, "runs.outputFolder"); r.Value != out || r.Kind != settingFolder {
		t.Errorf("the output folder reads %+v", r)
	}

	_, runs := rig.read(t, "Runs")
	for _, r := range runs {
		if r.Section != "run-defaults" && r.Section != "runhosts" {
			t.Errorf("Runs lists %s from %s", r.ID, r.Section)
		}
	}
	if _, ok := rowByID(runs, "runs.outputFolder"); !ok {
		t.Error("Runs lists no output folder")
	}
	_, account := rig.read(t, "account")
	if _, ok := rowByID(account, "account.registry"); !ok {
		t.Error("Account lists no registry, which its Registries part shows")
	}
	if a, _ := rig.read(t, "the keychain"); !a.IsError || !strings.Contains(a.Text, "Runs") {
		t.Errorf("an unknown section: %+v", a)
	}

	if d := rig.dialogs(); len(d) != 0 {
		t.Errorf("settings_read raised a dialog: %v", d)
	}
	if after, _ := settings.Load(); !jsonEqual(t, before, after) {
		t.Errorf("settings_read changed the settings: %+v", after)
	}
}

func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	x, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(x) == string(y)
}

// No credential's value reaches an agent: with a known value in every store
// the app keeps one in, no section's answer carries any of them, and each
// credential's row says only that it is set.
func TestSettingsReadNeverCarriesACredential(t *testing.T) {
	rig := newCatalogueRig(t)
	// The registry answers nothing an account read can use, so the account's
	// rows say why they could not be read; the token must not be in that
	// either.
	var echoed string
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A registry that echoes what it was sent: its words never reach
		// the agent.
		http.Error(w, "refused "+r.Header.Get("Authorization")+" "+echoed, http.StatusUnauthorized)
	}))
	t.Cleanup(reg.Close)
	planted := map[string]string{
		"registry userinfo":  "plantedregistrypassword0013",
		"registry query":     "plantedregistryquery0014",
		"registry token":     "ags_planted_registry_token_0001",
		"model key":          "sk-ant-planted-model-key-0002",
		"model key (env)":    "sk-planted-env-key-0003",
		"AI Review key":      "jev-planted-review-key-0004",
		"web search key":     "tvly-planted-search-key-0005",
		"Hybrid key":         "planted-hybrid-key-0006",
		"destination secret": "whsec_planted_destination_0007",
		"destination auth":   "Bearer planted-destination-auth-0008",
		"destination url":    "PLANTEDURLTOKEN0009",
		"source key":         "ghp_planted_source_key_0010",
		"agent secret":       "planted-agent-secret-0011",
		"agent mail token":   "fmu1-planted-mail-token-0012",
	}
	echoed = planted["registry token"]
	s, _ := settings.Load()
	s.Registries = append(s.Registries, settings.Registry{URL: reg.URL},
		settings.Registry{URL: "https://someone:" + planted["registry userinfo"] + "@registry.example/?key=" + planted["registry query"]})
	s.DefaultRegistry = reg.URL
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.StoreToken(reg.URL, planted["registry token"]); err != nil {
		t.Fatal(err)
	}
	if err := run.StoreModelKey("anthropic", planted["model key"], io.Discard); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", planted["model key (env)"])
	if err := run.StoreJevKey(planted["AI Review key"], io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run.StoreSearchKey("tavily", planted["web search key"], io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run.StoreHybridKey(planted["Hybrid key"], io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := destinations.Save(destinations.Input{Name: "hooks", URL: "https://hooks.example.com/w/" + planted["destination url"],
		Secret: planted["destination secret"], Authorization: planted["destination auth"]}); err != nil {
		t.Fatal(err)
	}
	if err := run.SaveSourceKey("repository", "https://github.com/acme/app", planted["source key"]); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.SetSecret(assistantAgent, "PLANTED_SECRET", planted["agent secret"]); err != nil {
		t.Fatal(err)
	}
	if err := (persistent.KeychainSecrets{}).Set(persistent.SecretAccount(assistantAgent, "FASTMAIL_TOKEN"), planted["agent mail token"]); err != nil {
		t.Fatal(err)
	}
	// Keeping the secret asked; settings_read must ask nothing.
	rig.mu.Lock()
	rig.asked = nil
	rig.mu.Unlock()

	// A run holds the run lock throughout: no row waits for it or takes it.
	if err := rig.p.run.acquire(jobSetup, "a run"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rig.p.run.release)

	var all []SettingRow
	for _, section := range append([]string{""}, sectionIDs()...) {
		a, rows := rig.read(t, section)
		if a.IsError {
			t.Fatalf("%q: %s", section, a.Text)
		}
		for what, v := range planted {
			if strings.Contains(a.Text, v) {
				t.Errorf("settings_read %q carries the %s", section, what)
			}
		}
		if section == "" {
			all = rows
		}
	}
	for _, id := range []string{"account.signIn." + reg.URL, "keys.aiReview", "keys.webSearch", "keys.hybridAnalysis", "destinations.hooks",
		"agents.assistant.secret.PLANTED_SECRET"} {
		r, ok := rowByID(all, id)
		if !ok || r.Kind != settingCredential || r.Value != credentialSet {
			t.Errorf("%s: %+v (listed %v)", id, r, ok)
		}
	}
	if r, _ := rowByID(all, "account.displayName"); r.Unread != errRegistryUnread.Error() {
		t.Errorf("a registry's refusal reads %+v", r)
	}
	if r, _ := rowByID(all, "account.signIn.https://registry.example"); r.Name != "Signed in to https://registry.example" {
		t.Errorf("a registry with a user and a query is named %+v", r)
	}
	for _, r := range all {
		if r.Unread != "" && r.ID != "account.displayName" && r.ID != "account.voice" && r.ID != "reports.design" {
			t.Errorf("%s could not be read: %s", r.ID, r.Unread)
		}
	}
	for _, r := range all {
		if r.Kind == settingCredential && r.Value != credentialSet && r.Value != credentialNotSet {
			t.Errorf("credential %s reads %v", r.ID, r.Value)
		}
	}
	// settings_change changes nothing in this version, and says so without
	// a dialog.
	raw, _ := json.Marshal(map[string]string{"id": "keys.aiReview", "value": "x"})
	if a := rig.link.answer(context.Background(), applink.Request{Tool: applink.SettingsChange, Arguments: raw}); !a.IsError || !strings.Contains(a.Text, "nothing was changed") {
		t.Errorf("settings_change: %+v", a)
	}
	if d := rig.dialogs(); len(d) != 0 {
		t.Errorf("a dialog was raised: %v", d)
	}
}

func sectionIDs() []string {
	out := make([]string, 0, len(settingsSections))
	for _, s := range settingsSections {
		out = append(out, s.id)
	}
	return out
}

// Every row the catalogue lists by itself is in a section settings_open
// opens, so an agent can always show the person where a setting is.
func TestTheCatalogueIsInSettingsOpensSections(t *testing.T) {
	for _, e := range catalogueEntries {
		if _, _, ok := settingsSection(e.section); !ok {
			t.Errorf("%s is in %q, which settings_open does not open", e.id, e.section)
		}
		if (e.get == nil) == (e.each == nil) {
			t.Errorf("%s needs exactly one of get and each", e.id)
		}
	}
}
