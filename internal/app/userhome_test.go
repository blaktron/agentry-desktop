package app

import "testing"

// setUserHome makes dir the user's home for the test, the way each platform
// reads it: os.UserHomeDir takes HOME on Unix and USERPROFILE on Windows, so
// a test that set only HOME would write into the real profile on Windows.
func setUserHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}
