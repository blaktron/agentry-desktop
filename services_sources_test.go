package main

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"
)

// A saved or forgotten source key reaches the keys sealed beside the
// schedules that run while the person is signed out (agentry-cli#854,
// agentry-desktop#818): both ask for a re-seal, as the AI Review key's
// changes do; a refused save asks for none.
func TestASourceKeyChangeResealsTheSchedules(t *testing.T) {
	keyring.MockInit()
	t.Setenv("AGENTRY_NO_KEYRING", "")
	saved := resealSchedules
	t.Cleanup(func() { resealSchedules = saved })
	asked := 0
	resealSchedules = func(*App) { asked++ }
	r := &RunAPI{app: &App{ctx: context.Background()}}

	if _, err := r.SaveSourceKey("repository", "https://github.com/o/private", "  "); err == nil {
		t.Fatal("an empty token was saved")
	}
	if asked != 0 {
		t.Fatalf("a refused save asked for %d re-seal(s)", asked)
	}
	v, err := r.SaveSourceKey("repository", "https://github.com/o/private", "tok-818")
	if err != nil {
		t.Fatal(err)
	}
	if !v.Saved || v.Host != "github.com" || asked != 1 {
		t.Fatalf("after a save: %+v, re-seals %d", v, asked)
	}
	v, err = r.ForgetSourceKey("repository", "https://github.com/o/private")
	if err != nil {
		t.Fatal(err)
	}
	if v.Saved || asked != 2 {
		t.Fatalf("after a forget: %+v, re-seals %d", v, asked)
	}
}
