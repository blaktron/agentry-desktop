// Package notify raises the app's desktop notifications: a title and a body
// the platform draws as plain text, a click that brings the app forward on
// the page the notification is about, and on Linux buttons, a notification
// replaced in place by its tag and one taken away. Each platform's own facility
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
	// Tag names a notification that a later one with the same tag replaces
	// in place, and that Withdraw takes away; "" for one that stands alone.
	Tag string
	// Actions are buttons, drawn where the platform's facility draws them
	// (Linux; elsewhere the notification has its click alone). A press
	// hands the notification's Key and the action's ID to the action
	// handler.
	Actions []Action
	// Resident keeps the notification on screen until it is replaced or
	// withdrawn, where the facility can.
	Resident bool
}

// Action is one button on a notification.
type Action struct {
	ID    string
	Label string
}

// The longest title, body and button label a notification carries, in
// characters; a longer one is cut and ends in an ellipsis.
const (
	maxTitle = 120
	maxBody  = 300
	maxLabel = 30
)

// ErrUnsupported is a platform with no notification facility here.
var ErrUnsupported = errors.New("this computer has no desktop notifications the app can raise")

var (
	mu       sync.Mutex
	onClick  func(key string)
	onAction func(key, action string)
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

// SetOnAction sets what a press on one of a notification's buttons does,
// with its Notice's Key and the Action's ID. It runs on a goroutine of its
// own, as a click does.
func SetOnAction(f func(key, action string)) {
	mu.Lock()
	defer mu.Unlock()
	onAction = f
}

// acted is a platform's report of a press on a button the notification
// with key offered.
func acted(key, action string) {
	mu.Lock()
	f := onAction
	mu.Unlock()
	if f != nil && action != "" {
		go f(key, action)
	}
}

// Withdraw takes away the notification raised with tag, if it is still
// shown. A platform that cannot take one away leaves it.
func Withdraw(tag string) error {
	if tag == "" {
		return nil
	}
	return withdraw(tag)
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

// Show raises n, cleaned to plain text, in place of the one with its tag if
// it has one. An error says the platform refused it or has no facility for
// it.
func Show(n Notice) error {
	return show(Clean(n))
}

// Clean is n as every platform draws it: the title and the body each one
// line of text, control and format characters replaced by spaces, runs of
// space collapsed, and cut to maxTitle and maxBody characters.
func Clean(n Notice) Notice {
	out := Notice{Title: plain(n.Title, maxTitle), Body: plain(n.Body, maxBody), Key: n.Key, Tag: n.Tag, Resident: n.Resident}
	for _, a := range n.Actions {
		out.Actions = append(out.Actions, Action{ID: a.ID, Label: plain(a.Label, maxLabel)})
	}
	return out
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
