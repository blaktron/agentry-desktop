//go:build windows

package linkservice

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"syscall"
)

// ownProcessGroup starts the server in a process group of its own and
// with no console window of its own.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
}

// terminate ends the server and the children it started.
func terminate(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run() //nolint:gosec // the server's own pid
	}
}

// kill ends the server's own process.
func kill(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// listenOwnerOnly listens on a Unix socket in the runner home, whose
// folder is the person's own profile's.
func listenOwnerOnly(path string) (net.Listener, error) {
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on the link socket: %w", err)
	}
	return ln, nil
}
