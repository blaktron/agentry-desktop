package settings

import (
	"strings"
	"testing"
)

// A registry address refused for its shape is named without the userinfo
// typed into it, so neither a password nor a token before the "@" reaches
// a message on screen.
func TestValidateURLNeverShowsUserinfo(t *testing.T) {
	for _, raw := range []string{
		"http://user:secret@example.com",        // http off this machine
		"ftp://user:secret@example.com",         // not http(s)
		"https://user:secret@/path",             // no host
		"https://secret@example.com:99999x",     // does not parse
		"https://us er:secret@example.com/path", // does not parse
	} {
		err := ValidateURL(raw)
		if err == nil {
			t.Fatalf("%s: accepted", raw)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: the refusal shows the userinfo: %v", raw, err)
		}
	}
}

func TestShownURLDropsTheUserinfo(t *testing.T) {
	for raw, want := range map[string]string{
		"https://user:secret@registry.example.com/x": "https://registry.example.com/x",
		"https://ghp_token@registry.example.com":     "https://registry.example.com",
		"https://registry.example.com":               "https://registry.example.com",
		"https://us er:secret@registry.example.com":  "the address entered",
	} {
		if got := ShownURL(raw); got != want {
			t.Errorf("ShownURL(%q) = %q, want %q", raw, got, want)
		}
	}
}
