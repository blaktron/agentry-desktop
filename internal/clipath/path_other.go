//go:build !windows

package clipath

// The installer's flags mean nothing off Windows: the Linux packages put the
// CLI in /usr/bin, and macOS offers the command from the app.
const cliName = "agentry"

func expandEnv(s string) string { return s }

func editUserPath(func(string) (string, bool)) (bool, error) { return false, nil }
