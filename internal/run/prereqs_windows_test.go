//go:build windows

package run

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

// Restart now runs shutdown.exe, a console program: it is created with no
// console window.
func TestTheRestartIsCreatedWithNoConsoleWindow(t *testing.T) {
	cmd := restartCommand(context.Background(), false)
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("shutdown.exe is created with a console window: %+v", cmd.SysProcAttr)
	}
}
