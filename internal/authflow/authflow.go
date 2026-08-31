// Package authflow is the desktop half of browser sign-in (core DEV-058):
// a one-shot loopback listener that receives the PAT the registry's
// /settings/tokens/desktop page mints after the user signs in (through
// the directory service, DEV-057) and clicks Authorize.
//
// Trust model: the listener binds 127.0.0.1 only, accepts exactly one
// callback, and requires the state nonce this process generated — a
// browser tab cannot plant a token the app did not ask for, and a token
// meant for this app cannot be delivered anywhere else (the registry pins
// its redirect to 127.0.0.1:<port>). The token is still verified against
// the registry (whoami) before it is stored, exactly like a pasted PAT.
package authflow

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Timeout bounds how long a sign-in may sit waiting on the browser.
const Timeout = 5 * time.Minute

// donePage is what the browser shows once the token has been handed over.
const donePage = `<!doctype html><meta charset="utf-8"><title>Signed in</title>
<body style="font-family:system-ui;background:#141210;color:#e7e5e4;display:grid;place-items:center;height:100vh;margin:0">
<div style="text-align:center"><h1 style="font-weight:600">Signed in</h1>
<p>You can close this tab and return to SecureAgentry Desktop.</p></div>`

// Flow is one in-flight browser sign-in.
type Flow struct {
	// Port is the loopback port the registry must redirect to.
	Port int
	// State is the nonce the callback must echo.
	State string

	srv  *http.Server
	ch   chan outcome
	once sync.Once
}

type outcome struct {
	token string
	err   error
}

// Start binds 127.0.0.1:0 and begins serving the one-shot callback.
func Start() (*Flow, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("bind loopback listener: %w", err)
	}
	stateBytes := make([]byte, 24)
	if _, err := rand.Read(stateBytes); err != nil {
		_ = ln.Close()
		return nil, err
	}
	f := &Flow{
		Port:  ln.Addr().(*net.TCPAddr).Port,
		State: base64.RawURLEncoding.EncodeToString(stateBytes),
		ch:    make(chan outcome, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", f.handleCallback)
	f.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = f.srv.Serve(ln) }()
	return f, nil
}

// AuthorizeURL is the page to open in the system browser.
func (f *Flow) AuthorizeURL(registry string) string {
	return fmt.Sprintf("%s/settings/tokens/desktop?port=%d&state=%s",
		registry, f.Port, url.QueryEscape(f.State))
}

func (f *Flow) handleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	token := r.URL.Query().Get("token")
	if subtle.ConstantTimeCompare([]byte(state), []byte(f.State)) != 1 {
		http.Error(w, "state mismatch — start the sign-in again from the app", http.StatusForbidden)
		f.deliver(outcome{err: errors.New("callback state mismatch")})
		return
	}
	if token == "" {
		http.Error(w, "no token in callback", http.StatusBadRequest)
		f.deliver(outcome{err: errors.New("callback carried no token")})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The token must never be cached or logged by anything in between —
	// there is nothing in between (loopback), but belt and braces.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(donePage))
	f.deliver(outcome{token: token})
}

func (f *Flow) deliver(o outcome) {
	f.once.Do(func() { f.ch <- o })
}

// Wait blocks until the callback arrives, the context ends, or Timeout.
func (f *Flow) Wait(ctx context.Context) (string, error) {
	t := time.NewTimer(Timeout)
	defer t.Stop()
	select {
	case o := <-f.ch:
		return o.token, o.err
	case <-t.C:
		return "", errors.New("browser sign-in timed out — start it again from Settings")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Close tears the listener down. Safe after Wait, and on every exit path.
func (f *Flow) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = f.srv.Shutdown(ctx)
}
