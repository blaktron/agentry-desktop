package notify

import (
	"encoding/xml"
	"strings"
)

// toastXML is n as a Windows toast: two text lines in the generic template,
// raised in the foreground, with n's key as the click's argument. Every value
// is escaped, so nothing in a title, a body or a key can add an element or an
// attribute (an image fetched from an address, a protocol launch).
func toastXML(n Notice) string {
	esc := func(s string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	return `<toast activationType="foreground" launch="` + esc(n.Key) + `">` +
		`<visual><binding template="ToastGeneric">` +
		`<text>` + esc(n.Title) + `</text>` +
		`<text>` + esc(n.Body) + `</text>` +
		`</binding></visual></toast>`
}
