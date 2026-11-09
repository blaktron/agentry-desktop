package linkservice

import "context"

// RunningWindow is a window open on this display, with the identifiers
// computer use can match its application by.
type RunningWindow struct {
	Title       string   `json:"title"`
	Identifiers []string `json:"identifiers"`
}

// RunningWindows are the windows open on this display with their
// identifiers, where the window system lets Agentry Desktop read them; an
// error says where it does not, and the person types the identifier.
func RunningWindows(ctx context.Context) ([]RunningWindow, error) {
	return runningWindows(ctx)
}
