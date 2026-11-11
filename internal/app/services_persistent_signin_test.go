package app

import (
	"strings"
	"testing"
)

// A server that waits for a sign-in says so on its card with the card's own
// way out, never a terminal command, and keeps the runner's reason.
func TestASignInReasonNamesTheCardNotATerminal(t *testing.T) {
	cases := map[string]string{
		"it has no connection yet: connect it on a terminal: agentry agent connect assistant":                                    "it has no connection yet: sign in to it from this card",
		"its vendor no longer honours the connection (invalid_grant): connect it on a terminal: agentry agent connect assistant": "its vendor no longer honours the connection (invalid_grant): sign in to it from this card",
		"its connection could not be refreshed: it is tried again when the agent next starts":                                    "its connection could not be refreshed: it is tried again when the agent next starts",
	}
	for in, want := range cases {
		if got := cardSignInReason(in, "assistant"); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
		if strings.Contains(cardSignInReason(in, "assistant"), "agentry agent connect") {
			t.Errorf("%q still names the terminal", in)
		}
	}
}
