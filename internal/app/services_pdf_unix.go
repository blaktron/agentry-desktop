//go:build !windows

package app

import (
	"os/exec"
	goruntime "runtime"
)

// openWithSystemViewer asks the OS to open a PDF copy OpenRunPDF wrote, with
// the program the person has for PDFs, and reaps it when it exits.
func openWithSystemViewer(p string) error {
	name := "xdg-open"
	if goruntime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, p)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
