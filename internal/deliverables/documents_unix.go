//go:build !windows

package deliverables

func documentsDir(goos string, getenv func(string) string, home func() (string, error)) (string, error) {
	return unixDocuments(goos, getenv, home)
}
