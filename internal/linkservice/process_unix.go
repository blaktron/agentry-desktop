//go:build !windows

package linkservice

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
)

// ownProcessGroup puts the server in a process group of its own, so its
// end reaches the children a launcher such as uvx starts.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminate asks the server's process group to end.
func terminate(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}

// kill ends the server's process group.
func kill(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// listenOwnerOnly listens on a Unix socket that only its owner can use:
// made under a umask that leaves no group or other bits, then set 0600.
func listenOwnerOnly(path string) (net.Listener, error) {
	old := syscall.Umask(0o077)
	ln, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		return nil, fmt.Errorf("listen on the link socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("make the link socket owner-only: %w", err)
	}
	return ln, nil
}
