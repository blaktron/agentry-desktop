package meeting

import (
	"errors"
	"io/fs"
	"os"
	"runtime"
	"time"
)

// replaceFile renames tmp over dst. Windows refuses to rename over a file
// another handle has open (an antivirus scan, the search indexer), so there
// a refusal is tried again for about half a second before it is an error.
func replaceFile(tmp, dst string) error {
	err := os.Rename(tmp, dst)
	if runtime.GOOS != "windows" {
		return err
	}
	for wait := 10 * time.Millisecond; err != nil && errors.Is(err, fs.ErrPermission) && wait <= 320*time.Millisecond; wait *= 2 {
		time.Sleep(wait)
		err = os.Rename(tmp, dst)
	}
	return err
}
