// Package loginitem is Start Agentry when you sign in: an entry the system
// reads at sign-in that launches the app with HiddenFlag, so it starts in
// the background with its window hidden. Each platform keeps it in its own
// place (loginitem_*.go): an autostart desktop entry on Linux, a LaunchAgent
// on macOS, a value of the user's Run key on Windows. Go writes and removes
// it, and reads it back each time Settings opens, so an entry the person
// removed with the system's own tools reads as off.
package loginitem

import (
	"fmt"
	"strings"
)

// HiddenFlag is the argument the entry launches the app with.
const HiddenFlag = "--hidden"

// Name is what the entry is called where the system lists it.
const Name = "Agentry Desktop"

// desktopEntry is the Linux autostart entry for exe.
func desktopEntry(exe string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + Name + "\n" +
		"Comment=Starts Agentry in the background, where its persistent agents run\n" +
		"Exec=" + execQuote(exe) + " " + HiddenFlag + "\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
}

// execQuote quotes a path for a desktop entry's Exec key: in double quotes,
// with the characters the spec reserves escaped.
func execQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`, "%", "%%")
	return `"` + r.Replace(s) + `"`
}

// launchAgent is the macOS LaunchAgent for exe, labelled label.
func launchAgent(label, exe string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + xmlEscape(label) + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + xmlEscape(exe) + `</string>
		<string>` + HiddenFlag + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

// runValue is the Windows Run key's value for exe.
func runValue(exe string) string { return fmt.Sprintf("%q %s", exe, HiddenFlag) }
