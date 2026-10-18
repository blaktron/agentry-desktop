package destinations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
)

// env points the CLI's configuration folder and the keychain at throwaway state.
func env(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(configdir.EnvVar, dir)
	t.Setenv(modelkey.EnvNoKeyring, "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	return dir
}

func TestASavedDestinationIsTheCLIsAndItsCredentialsNeverComeBack(t *testing.T) {
	dir := env(t)
	err := Save(Input{Name: "n8n", URL: "https://hooks.example.com/webhook/abc?token=URLTOKEN", Secret: "whsec_TYPED", Authorization: "Bearer TYPEDAUTH"})
	if err != nil {
		t.Fatal(err)
	}
	// The CLI's own readers see what the app saved.
	r, err := destination.Resolve([]string{"n8n"})
	if err != nil || len(r) != 1 || r[0].Secret != "whsec_TYPED" || r[0].Authorization != "Bearer TYPEDAUTH" || r[0].URL != "https://hooks.example.com/webhook/abc?token=URLTOKEN" {
		t.Fatalf("%+v %v", r, err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, destination.File)); strings.Contains(string(raw), "TYPED") {
		t.Errorf("the file holds a credential: %s", raw)
	}
	st, err := Load()
	if err != nil || len(st.Destinations) != 1 {
		t.Fatalf("%+v %v", st, err)
	}
	b, _ := json.Marshal(st)
	for _, bad := range []string{"TYPED", "URLTOKEN", "/webhook/", "token="} {
		if strings.Contains(string(b), bad) {
			t.Errorf("the view carries %q: %s", bad, b)
		}
	}
	v := st.Destinations[0]
	if !v.HasSecret || !v.HasAuthorization || v.Where != "https://hooks.example.com" || v.DefaultFor == nil {
		t.Errorf("view: %+v", v)
	}
}

func TestSavingAgainKeepsTheCredentialsUnlessTheHostChanges(t *testing.T) {
	env(t)
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/a", Secret: "s1", Authorization: "Bearer A"}); err != nil {
		t.Fatal(err)
	}
	// A change of path keeps both.
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/b"}); err != nil {
		t.Fatal(err)
	}
	if s, a, _ := destination.Secrets("n8n"); s != "s1" || a != "Bearer A" {
		t.Errorf("after a path change: %q %q", s, a)
	}
	// A new secret replaces the secret and keeps the value.
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/b", Secret: "s2"}); err != nil {
		t.Fatal(err)
	}
	if s, a, _ := destination.Secrets("n8n"); s != "s2" || a != "Bearer A" {
		t.Errorf("after a new secret: %q %q", s, a)
	}
	// Removing the value is asked for.
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/b", ClearAuthorization: true}); err != nil {
		t.Fatal(err)
	}
	if _, a, _ := destination.Secrets("n8n"); a != "" {
		t.Errorf("a value kept after it was cleared: %q", a)
	}
	if err := Save(Input{Name: "n8n", URL: "https://one.example.com/b", Authorization: "Bearer B"}); err != nil {
		t.Fatal(err)
	}
	// Another host: the value saved for the first is not sent to it.
	if err := Save(Input{Name: "n8n", URL: "https://two.example.com/b"}); err != nil {
		t.Fatal(err)
	}
	if s, a, _ := destination.Secrets("n8n"); s != "s2" || a != "" {
		t.Errorf("after a change of host: %q %q", s, a)
	}
}

func TestASaveTheCLIWouldRefuseIsRefused(t *testing.T) {
	env(t)
	for _, in := range []Input{
		{Name: "a", URL: "http://hooks.example.com/x", Secret: "s"},
		{Name: "a", URL: "https://10.0.0.5/x", Secret: "s"},
		{Name: "a", URL: "https://127.0.0.1:8443/x", OnMyNetwork: true, Secret: "s"},
		{Name: "a", URL: "https://user:pw@hooks.example.com/x", Secret: "s"},
		{Name: "Bad Name", URL: "https://hooks.example.com/x", Secret: "s"},
		{Name: "a", URL: "https://hooks.example.com/x"},
		{Name: "a", URL: "https://hooks.example.com/x", Secret: "s", Authorization: "Bearer x\r\nX: y"},
	} {
		if err := Save(in); err == nil {
			t.Errorf("%+v was saved", in)
		}
	}
	if st, _ := Load(); len(st.Destinations) != 0 {
		t.Errorf("a refused save kept %+v", st.Destinations)
	}
	if destination.HasSecret("a") {
		t.Error("a refused save kept a secret")
	}
	if got := Check("https://10.0.0.5/x", false); !strings.Contains(got, "On my network") {
		t.Errorf("Check: %q", got)
	}
	if got := Check("https://10.0.0.5/x", true); got != "" {
		t.Errorf("Check on my network: %q", got)
	}
}

func TestRemoveTakesTheCredentialsAndDefaultsWithIt(t *testing.T) {
	env(t)
	if err := Save(Input{Name: "n8n", URL: "https://hooks.example.com/x", Secret: "s", Authorization: "Bearer A"}); err != nil {
		t.Fatal(err)
	}
	if err := SetDefault("acme/weekly", []string{"n8n"}); err != nil {
		t.Fatal(err)
	}
	if got := DefaultFor("acme/weekly@1.0.0"); len(got) != 1 || got[0] != "n8n" {
		t.Errorf("default: %v", got)
	}
	if st, _ := Load(); len(st.Destinations[0].DefaultFor) != 1 {
		t.Errorf("view: %+v", st.Destinations)
	}
	if err := Remove("n8n"); err != nil {
		t.Fatal(err)
	}
	if destination.HasSecret("n8n") || destination.HasAuthorization("n8n") || len(DefaultFor("acme/weekly")) != 0 {
		t.Error("a removed destination left something behind")
	}
	if err := Remove("n8n"); err == nil {
		t.Error("removing a destination twice succeeded")
	}
	if got := DefaultFor("acme/other"); got == nil {
		t.Error("the default set is nil, which marshals as null")
	}
}

func TestTheKeychainBeingOffRefusesASave(t *testing.T) {
	env(t)
	t.Setenv(modelkey.EnvNoKeyring, "1")
	if err := Save(Input{Name: "a", URL: "https://hooks.example.com/x", Secret: "s"}); err == nil {
		t.Error("a destination was saved with the keychain off")
	}
	if st, _ := Load(); !st.KeychainOff {
		t.Error("the status does not say the keychain is off")
	}
}
