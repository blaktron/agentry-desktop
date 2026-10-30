// Package applog is the log handler the desktop hands the Wails runtime. It
// exists because of what the framework logs at debug level: every bound call
// with its arguments and its result ("Binding call complete:"), and every
// runtime call with its arguments ("Runtime call:"). Five of the app's calls
// carry a secret as an argument: the personal access token on SignIn, MCP
// secret values on SaveProfile, the model key on StoreModelKey, the person's
// AI Review key on StoreJevKey and a run In Agentry's secret on
// SetHostedSecret. No secret reaches a log, so this handler withholds the
// arguments and the result of every call, whatever the log level, and keeps
// the method and the error, so a stale binding is still diagnosable from the
// log.
//
// Everything else passes through to the inner handler unchanged.
package applog

import (
	"context"
	"log/slog"
)

// withheld replaces the value of an attribute that can carry a secret.
const withheld = "(withheld)"

// Keys are the attributes the framework puts a call's arguments and result
// under; a test pins them against the module's source.
var Keys = []string{"args", "result"}

// New wraps inner so that no bound call's arguments or result reach it.
func New(inner slog.Handler) slog.Handler { return redacting{inner} }

type redacting struct{ inner slog.Handler }

func (r redacting) Enabled(ctx context.Context, l slog.Level) bool { return r.inner.Enabled(ctx, l) }

func (r redacting) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(redact(a))
		return true
	})
	return r.inner.Handle(ctx, clean)
}

func (r redacting) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = redact(a)
	}
	return redacting{r.inner.WithAttrs(clean)}
}

func (r redacting) WithGroup(name string) slog.Handler { return redacting{r.inner.WithGroup(name)} }

// redact returns a with its value withheld when its key is one of Keys, in a
// group at any depth.
func redact(a slog.Attr) slog.Attr {
	for _, k := range Keys {
		if a.Key == k {
			return slog.String(a.Key, withheld)
		}
	}
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		clean := make([]slog.Attr, len(group))
		for i, g := range group {
			clean[i] = redact(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(clean...)}
	}
	return a
}
