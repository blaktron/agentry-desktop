//go:build windows

package app

import (
	"golang.org/x/sys/windows"
)

// openWithSystemViewer asks Windows to open a PDF copy OpenRunPDF wrote with
// the program the person has for PDFs: ShellExecute's "open" verb on a file
// whose name copyName made plain and which ends in .pdf.
func openWithSystemViewer(p string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
