package app

// NoticesAPI serves the third-party notices the app ships with: the licence
// texts of the software and assets inside it, which the Licences page in
// Settings shows. The text is handed over at start-up (the entry point
// embeds THIRD_PARTY_NOTICES), so reading it touches no file and no network.
type NoticesAPI struct {
	text string
}

// Text is the whole notices file, as the build generated it.
func (n *NoticesAPI) Text() string { return n.text }
