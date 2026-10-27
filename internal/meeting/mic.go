package meeting

// pickMic is the microphone to read, by its id: the chosen one when listed,
// otherwise the default (missing says the chosen one was not listed), and
// none when the default is not a listed microphone either. name is how the
// person knows it.
func pickMic(mics []Microphone, chosen, def string) (id, name string, missing bool) {
	find := func(id string) (Microphone, bool) {
		for _, m := range mics {
			if m.ID == id {
				return m, true
			}
		}
		return Microphone{}, false
	}
	if chosen != "" {
		if m, ok := find(chosen); ok {
			return m.ID, m.Name, false
		}
		missing = true
	}
	if def == "" {
		return "", "", missing
	}
	// The default is read only when it is a microphone: a default source
	// set to an output's monitor would record what the computer plays.
	if m, ok := find(def); ok {
		return m.ID, m.Name, missing
	}
	return "", "", missing
}
