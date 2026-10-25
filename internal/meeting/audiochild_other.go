//go:build !windows

package meeting

import (
	"fmt"
	"io"
)

// CaptureMain is the audio child, which runs on Windows alone.
func CaptureMain(_ io.Reader, out io.Writer) int {
	_, _ = fmt.Fprintln(out, "the audio child runs on Windows only")
	return 2
}
