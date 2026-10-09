//go:build windows

package schedules

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: the console program runs with no
// console window flashing up over the app.
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
