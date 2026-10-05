package ui

// The information panel, pinned to the right.
//
// It is never focused. K and J scroll it half a screen from wherever you
// happen to be — which is what keeps it a panel you read rather than a place
// you have to go and come back from. The cost is two keys in the shift
// space; the saving is a whole mode.
//
// In statistics J and K walk the categories instead, and ctrl+j and ctrl+k
// turn the groups over.

type infoMode int

const (
	// infoCard follows the cursor: whatever is highlighted, described.
	infoCard infoMode = iota
	// infoStats is the histograms, which double as a filter for the list
	// they were counted from.
	infoStats
	// infoVersions is gv: a deck's git history, or a card's printed text
	// through the years.
	infoVersions
	// infoImage is gx: the card as printed, following the cursor.
	infoImage
)

func (i infoMode) String() string {
	switch i {
	case infoStats:
		return "statistics"
	case infoVersions:
		return "versions"
	case infoImage:
		return "printing"
	}
	return "card"
}

type infoPanel struct {
	mode infoMode
	// offset is how far the view is scrolled.
	offset int
	// oracle is the card infoVersions was opened on. The printed-text
	// history is about one card, so it lasts exactly as long as the cursor
	// stays on that card; see leaveVersions.
	oracle string
}

// leaveVersions puts the panel back to describing the card under the cursor
// once the cursor is no longer on the card the history was opened for.
//
// gv is the only key into that view and there was no key out, so a printed
// history stood over every card you moved to afterwards — each of them
// showing "gv for how its text has changed", which is the prompt to press
// the key you had just pressed. Checked from one place rather than at every
// key that can move the cursor, because that list — j, k, gg, G, a
// filter narrowing the list out from under it — is exactly the list somebody
// adds to and forgets.
func (p *infoPanel) leaveVersions(oracle string) {
	if p.mode == infoVersions && oracle != p.oracle {
		p.mode = infoCard
		p.offset = 0
	}
}

// toggle switches to a mode, or back to the card if already there. One key
// in and out beats two keys that both mean "show me statistics".
func (p *infoPanel) toggle(mode infoMode) {
	if p.mode == mode {
		p.mode = infoCard
	} else {
		p.mode = mode
	}
	p.offset = 0
}
