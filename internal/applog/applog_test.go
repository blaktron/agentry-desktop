package applog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// recorder is the inner logger: every line the Wails runtime would print.
type recorder struct{ lines []string }

func (r *recorder) Print(m string)   { r.lines = append(r.lines, "PRINT "+m) }
func (r *recorder) Trace(m string)   { r.lines = append(r.lines, "TRACE "+m) }
func (r *recorder) Debug(m string)   { r.lines = append(r.lines, "DEBUG "+m) }
func (r *recorder) Info(m string)    { r.lines = append(r.lines, "INFO "+m) }
func (r *recorder) Warning(m string) { r.lines = append(r.lines, "WARNING "+m) }
func (r *recorder) Error(m string)   { r.lines = append(r.lines, "ERROR "+m) }
func (r *recorder) Fatal(m string)   { r.lines = append(r.lines, "FATAL "+m) }

const canary = "agt_pat_CANARY-0123456789abcdef"

// The dispatcher's own format (wails v2 internal/frontend/dispatcher,
// dispatcher.go); TestTheDispatcherStillSpeaksTheseFormats pins it.
func dispatchError(message, err string) string {
	return fmt.Sprintf("process message error: %s -> %s", message, err)
}

// The secret-carrying calls, each forced into the dispatcher's error path:
// a stale binding's argument mismatch, and a panic inside the method.
func TestADispatchErrorOnASecretCarryingCallLogsNoArgument(t *testing.T) {
	cases := []struct{ name, message, err string }{
		{"sign-in, stale binding",
			`C{"name":"main.SettingsAPI.SignIn","args":["https://dev.secureagentry.net","` + canary + `",7],"callbackID":"main.SettingsAPI.SignIn-1"}`,
			"error parsing arguments: expected 2 arguments, got 3"},
		{"SaveProfile, panic",
			`C{"name":"main.RunAPI.SaveProfile","args":["reg","acme/agent",{"GITHUB_TOKEN":"` + canary + `"},true],"callbackID":"x-2"}`,
			"runtime error: invalid memory address or nil pointer dereference"},
		{"StoreModelKey, a secret with the separator in it",
			`C{"name":"main.RunAPI.StoreModelKey","args":["anthropic","sk -> ` + canary + `"],"callbackID":"x-3"}`,
			"boom"},
		{"StoreJevKey, stale binding (the person's AI Review key)",
			`C{"name":"main.RunAPI.StoreJevKey","args":["` + canary + `","extra"],"callbackID":"x-5"}`,
			"error parsing arguments: expected 1 arguments, got 2"},
		{"SetHostedSecret, stale binding (a run In Agentry's secret)",
			`C{"name":"main.RunAPI.SetHostedSecret","args":["https://dev.secureagentry.net","GITHUB_TOKEN","` + canary + `",false,"extra"],"callbackID":"x-6"}`,
			"error parsing arguments: expected 4 arguments, got 5"},
		{"call by id (obfuscated build)",
			`c{"id":17,"args":["https://r","` + canary + `"],"callbackID":"x-4"}`,
			"error parsing arguments: bad"},
		{"an event carrying data",
			`EE{"name":"anything","data":["` + canary + `"]}`,
			"bad event"},
		{"no separator at all",
			`C{"name":"main.RunAPI.StoreModelKey","args":["anthropic","` + canary + `"]`,
			""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := &recorder{}
			log := New(rec)
			line := dispatchError(c.message, c.err)
			if c.err == "" {
				line = "process message error: " + c.message
			}
			log.Error(line)
			if len(rec.lines) != 1 {
				t.Fatalf("want one line, got %q", rec.lines)
			}
			got := rec.lines[0]
			if strings.Contains(got, "CANARY") {
				t.Fatalf("the argument reached the log: %q", got)
			}
			if !strings.HasPrefix(got, "ERROR process message error: ") {
				t.Fatalf("the line lost its shape: %q", got)
			}
		})
	}
}

// What is kept is what diagnoses a stale binding: the method and the error.
func TestTheMethodAndTheErrorSurvive(t *testing.T) {
	rec := &recorder{}
	New(rec).Error(dispatchError(
		`C{"name":"main.SettingsAPI.SignIn","args":["u","`+canary+`"],"callbackID":"c"}`,
		"error parsing arguments: expected 2 arguments, got 3"))
	want := "ERROR process message error: call main.SettingsAPI.SignIn (arguments withheld) -> error parsing arguments: expected 2 arguments, got 3"
	if rec.lines[0] != want {
		t.Fatalf("got  %q\nwant %q", rec.lines[0], want)
	}
}

// The trace-level line carries a call's result; a result can carry a secret
// too, and the frontend can lower the level at run time.
func TestACallResultIsNeverLogged(t *testing.T) {
	rec := &recorder{}
	New(rec).Trace(`json call result data: {"result":{"token":"` + canary + `"},"error":null,"callbackid":"c"}` + "\n")
	if strings.Contains(rec.lines[0], "CANARY") {
		t.Fatalf("a call result reached the log: %q", rec.lines[0])
	}
}

// Every other line passes through untouched, at its own level.
func TestOtherLinesPassThrough(t *testing.T) {
	rec := &recorder{}
	l := New(rec)
	l.Info("Blocked request from unauthorized origin: x")
	l.Warning("w")
	l.Debug("d")
	l.Print("p")
	l.Fatal("f")
	want := []string{"INFO Blocked request from unauthorized origin: x", "WARNING w", "DEBUG d", "PRINT p", "FATAL f"}
	if strings.Join(rec.lines, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", rec.lines)
	}
}

// The redaction keys on two format strings inside Wails. An upgrade that
// rewords either would silently turn this logger into a pass-through, so
// the module's own source is read and the formats pinned.
func TestTheDispatcherStillSpeaksTheseFormats(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v2").Output()
	if err != nil {
		t.Fatalf("locate the wails module: %v", err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "internal", "frontend", "dispatcher")
	for file, format := range map[string]string{
		"dispatcher.go": `d.log.Error("` + dispatchPrefix + `%s -> %s", message, err)`,
		"calls.go":      `d.log.Trace("` + resultPrefix + `%+v\n", string(messageData))`,
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
