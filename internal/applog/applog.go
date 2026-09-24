// Package applog is the logger the desktop hands the Wails runtime. It
// exists because of one line in the framework: when the dispatcher cannot
// parse a bound call's arguments, or the bound method panics, Wails v2 logs
// "process message error: <message> -> <err>" at error level, and <message>
// is the raw JSON of the call, arguments included (agentry-desktop#102).
// Three of the app's calls carry a secret as an argument: the personal
// access token on SignIn, MCP secret values on SaveProfile, the model key
// on StoreModelKey. No secret reaches a log (ARCHITECTURE §9), so this
// logger rewrites that line to the kind of message, the method name and
// the error, and withholds the arguments. It also withholds the trace-level
// "json call result data" line, whose body is a call's result.
//
// Everything else passes through to the inner logger unchanged. The error
// text itself is kept: it is what diagnoses a stale binding, and it is the
// framework's or a method's own message, never the call's arguments (a
// method that panics with a secret in the panic value is that method's bug).
package applog

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/logger"
)

// The two formats inside Wails this logger keys on (internal/frontend/
// dispatcher, dispatcher.go and calls.go); a test pins them against the
// module's source so an upgrade that rewords them fails loudly.
const (
	dispatchPrefix = "process message error: "
	resultPrefix   = "json call result data: "
	separator      = " -> "
)

// New wraps inner so that no bound call's arguments or result reach it.
func New(inner logger.Logger) logger.Logger { return redacting{inner} }

type redacting struct{ inner logger.Logger }

func (r redacting) Print(m string)   { r.inner.Print(Redact(m)) }
func (r redacting) Trace(m string)   { r.inner.Trace(Redact(m)) }
func (r redacting) Debug(m string)   { r.inner.Debug(Redact(m)) }
func (r redacting) Info(m string)    { r.inner.Info(Redact(m)) }
func (r redacting) Warning(m string) { r.inner.Warning(Redact(m)) }
func (r redacting) Error(m string)   { r.inner.Error(Redact(m)) }
func (r redacting) Fatal(m string)   { r.inner.Fatal(Redact(m)) }

// Redact returns line with any IPC message body or call result withheld.
func Redact(line string) string {
	if strings.HasPrefix(line, resultPrefix) {
		return resultPrefix + "(withheld)"
	}
	rest, ok := strings.CutPrefix(line, dispatchPrefix)
	if !ok {
		return line
	}
	// The error is the last thing on the line, so the last separator ends
	// the message. A secret that contains " -> " sits before it and is
	// withheld with the rest; an error that contains one loses its head,
	// which leaks nothing.
	i := strings.LastIndex(rest, separator)
	if i < 0 {
		return dispatchPrefix + "(message withheld)"
	}
	return dispatchPrefix + describe(rest[:i]) + separator + rest[i+len(separator):]
}

// describe names a message without its body: a call by its method, every
// other kind by its type byte.
func describe(message string) string {
	if message == "" {
		return "(empty message)"
	}
	if message[0] == 'C' {
		var call struct {
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(message[1:]), &call) == nil && call.Name != "" && !strings.ContainsAny(call.Name, " \"\n") {
			return "call " + call.Name + " (arguments withheld)"
		}
		return "call (unreadable; arguments withheld)"
	}
	if message[0] == 'c' {
		var call struct {
			ID int `json:"id"`
		}
		if json.Unmarshal([]byte(message[1:]), &call) == nil {
			return "call by id " + strconv.Itoa(call.ID) + " (arguments withheld)"
		}
		return "call by id (unreadable; arguments withheld)"
	}
	return "message of kind " + string(message[0]) + " (body withheld)"
}
