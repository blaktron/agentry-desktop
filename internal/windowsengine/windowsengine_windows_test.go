//go:build windows

package windowsengine

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

// Every command the installer runs is a console program started by a
// windowed application: each is created with no console window.
func TestEveryCommandIsCreatedWithNoConsoleWindow(t *testing.T) {
	i, _ := rig(t, map[string]int{})
	for _, stand := range []bool{true, false} {
		if !stand {
			i.Command = nil
		}
		for _, name := range []string{"wsl.exe", "powershell.exe", "msiexec.exe", i.Rdctl()} {
			cmd := i.cmd(context.Background(), name)
			if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
				t.Errorf("%s (stand-in %v) is created with a console window: %+v", name, stand, cmd.SysProcAttr)
			}
			if cmd.WaitDelay == 0 {
				t.Errorf("%s (stand-in %v) has no bound on reading its output", name, stand)
			}
		}
	}
}
