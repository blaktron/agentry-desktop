//go:build windows

package meeting

import (
	"os/exec"
	"syscall"
)

// hideConsole keeps the sidecar from opening a console window of its own.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
