// Package notify raises the app's desktop notifications: a title and a body
// the platform draws as plain text, and a click that brings the app forward
// on the page the notification is about. Each platform's own facility
// carries it (notify_*.go): org.freedesktop.Notifications over the session
// bus on Linux, UserNotifications in the app's bundle on macOS, and a toast
// under the app's own AppUserModelID on Windows.
//
// What a notification says comes from an agent's answer, so it is text and
// nothing else on every platform: one line each, control characters
// replaced, cut to a length, escaped wherever the facility would read
// markup (the Linux body, the Windows toast's XML).
package notify

import (
	"errors"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Notice is one notification.
type Notice struct {
	Title string
	Body  string
	// Key is what a click hands to the click handler: the page to open.
	Key string
}

// The longest title and body a notification carries, in characters; a
// longer one is cut and ends in an ellipsis.
const (
	maxTitle = 120
	maxBody  = 300
)

// ErrUnsupported is a platform with no notification facility here.
var ErrUnsupported = errors.New("this computer has no desktop notifications the app can raise")

var (
	mu      sync.Mutex
	onClick func(key string)
)

// SetOnClick sets what a click on a notification does, with its Notice's
// Key. It runs on a goroutine of its own: the platforms call back on their
// UI thread, and a call into the window's runtime from there re-enters that
// loop.
func SetOnClick(f func(key string)) {
	mu.Lock()
	defer mu.Unlock()
	onClick = f
}

// clicked is a platform's report of a click on the notification with key.
func clicked(key string) {
	mu.Lock()
	f := onClick
	mu.Unlock()
	if f != nil && key != "" {
		go f(key)
	}
}

// Show raises n, cleaned to plain text. An error says the platform refused
// it or has no facility for it.
func Show(n Notice) error {
	return show(Clean(n))
}

// Clean is n as every platform draws it: the title and the body each one
// line of text, control and format characters replaced by spaces, runs of
// space collapsed, and cut to maxTitle and maxBody characters.
func Clean(n Notice) Notice {
	return Notice{Title: plain(n.Title, maxTitle), Body: plain(n.Body, maxBody), Key: n.Key}
}

func plain(s string, limit int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:limit-1])) + "…"
}
