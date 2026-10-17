package windowsengine

import (
	"time"

	"golang.org/x/sys/windows"
)

// bootTime is when Windows last started: now less the time since boot.
func bootTime() (time.Time, error) {
	return time.Now().Add(-windows.DurationSinceBoot()), nil
}
