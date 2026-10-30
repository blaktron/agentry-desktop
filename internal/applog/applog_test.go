package applog

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const canary = "agt_pat_CANARY-0123456789abcdef"

// logger writes through the redacting handler to a buffer, at debug level,
// the level at which the framework logs a call's arguments.
func logger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
}

// The secret-carrying calls, as the framework logs them when a call ends.
func TestACompletedCallOnASecretCarryingMethodLogsNoArgumentAndNoResult(t *testing.T) {
	cases := []struct{ name, method, args, result string }{
		{"sign-in", "SettingsAPI.SignIn", `["https://dev.secureagentry.net","` + canary + `"]`, `{"token":"` + canary + `"}`},
		{"SaveProfile", "RunAPI.SaveProfile", `["reg","acme/agent",{"GITHUB_TOKEN":"` + canary + `"},true]`, `null`},
		{"StoreModelKey, a secret with a quote in it", "RunAPI.StoreModelKey", `["anthropic","sk-\"` + canary + `"]`, `null`},
		{"StoreJevKey", "RunAPI.StoreJevKey", `["` + canary + `"]`, `null`},
		{"SetHostedSecret", "RunAPI.SetHostedSecret", `["https://dev.secureagentry.net","GITHUB_TOKEN","` + canary + `",false]`, `null`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger(&buf).Debug("Binding call complete:", "id", "abc", "method", c.method, "args", c.args, "result", c.result)
			got := buf.String()
			if strings.Contains(got, "CANARY") {
				t.Fatalf("a secret reached the log: %q", got)
			}
			if !strings.Contains(got, c.method) {
				t.Fatalf("the method is not in the line: %q", got)
			}
		})
	}
}

// A runtime call's arguments are withheld too (a clipboard text, a dialog's
// message).
func TestARuntimeCallLogsNoArguments(t *testing.T) {
	var buf bytes.Buffer
	logger(&buf).Debug("Runtime call:", "method", "Clipboard.SetText", "args", `{"text":"`+canary+`"}`)
	if got := buf.String(); strings.Contains(got, "CANARY") || !strings.Contains(got, "Clipboard.SetText") {
		t.Fatalf("got %q", got)
	}
}

// The redaction holds in a group and in the attributes a logger carries.
func TestTheRedactionReachesGroupsAndCarriedAttributes(t *testing.T) {
	var buf bytes.Buffer
	l := logger(&buf)
	l.Debug("x", slog.Group("call", slog.String("args", canary), slog.String("method", "m")))
	l.With("result", canary).Debug("y")
	if got := buf.String(); strings.Contains(got, "CANARY") {
		t.Fatalf("a secret reached the log: %q", got)
	}
}

// What is kept is what diagnoses a stale binding: the method and the error.
func TestTheMethodAndTheErrorSurvive(t *testing.T) {
	var buf bytes.Buffer
	logger(&buf).Error("Binding call failed:", "method", "SettingsAPI.SignIn", "error", "expected 2 arguments, got 3")
	got := buf.String()
	for _, want := range []string{"SettingsAPI.SignIn", "expected 2 arguments, got 3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q is missing from %q", want, got)
		}
	}
}

// Every other line passes through untouched, at its own level.
func TestOtherLinesPassThrough(t *testing.T) {
	var buf bytes.Buffer
	l := logger(&buf)
	l.Info("Starting up service:", "name", "App")
	l.Warn("w")
	if got := buf.String(); !strings.Contains(got, "name=App") || !strings.Contains(got, "level=WARN") {
		t.Fatalf("got %q", got)
	}
}

// The redaction keys on the attribute names the framework logs a call's
// arguments and result under. An upgrade that renamed either could not be
// seen from here, so the module's own source is read and the two log
// statements pinned.
func TestTheMessageProcessorStillLogsUnderTheseKeys(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v3").Output()
	if err != nil {
		t.Fatalf("locate the wails module: %v", err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "pkg", "application")
	for file, format := range map[string]string{
		"messageprocessor_call.go": `m.Debug("Binding call complete:", "id", *callID, "method", boundMethod, "args", string(jsonArgs), "result", string(jsonResult))`,
		"messageprocessor.go":      `m.Debug("Runtime call:", "method", objectName+"."+methodName, "args", req.Args.String())`,
	} {
		src, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if !strings.Contains(string(src), format) {
			t.Fatalf("wails %s no longer logs %s; revisit internal/applog before upgrading", file, format)
		}
	}
}
