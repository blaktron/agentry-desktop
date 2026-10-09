//go:build !windows

package schedules

import "os/exec"

// hideWindow does nothing off Windows, where no console window opens.
func hideWindow(*exec.Cmd) {}
