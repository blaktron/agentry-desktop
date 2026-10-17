//go:build !windows

package windowsengine

import (
	"errors"
	"time"
)

// bootTime is Windows' alone; elsewhere there is no restart to wait for.
func bootTime() (time.Time, error) {
	return time.Time{}, errors.New("not Windows")
}
