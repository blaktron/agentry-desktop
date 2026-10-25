//go:build !windows

package meeting

import "os/exec"

func hideConsole(*exec.Cmd) {}
