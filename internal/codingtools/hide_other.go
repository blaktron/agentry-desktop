//go:build !windows

package codingtools

import "os/exec"

func hideWindow(*exec.Cmd) {}
