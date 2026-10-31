package app

import "testing"

// The Licences page reads the notices through a bound service: it is in the
// list the window binds, and it returns the text it was given, whole.
func TestTheNoticesAreBoundAndServedWhole(t *testing.T) {
	const text = "Third-party notices\n\n  héllo\n"
	var bound *NoticesAPI
	for _, b := range New(text).Bind() {
		if n, ok := b.Instance().(*NoticesAPI); ok {
			bound = n
		}
	}
	if bound == nil {
		t.Fatal("NoticesAPI is not among the bound services")
	}
	if got := bound.Text(); got != text {
		t.Fatalf("Text() = %q, want %q", got, text)
	}
}
