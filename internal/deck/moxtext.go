package deck

import "strings"

// MoxfieldText is the deck as text for Moxfield's import: the boards under
// the headings Arena exports use, which Moxfield reads, and each card line
// as the file has it, with the tags as #tag the way Moxfield's own "with
// tags" export writes them. A section of your own isn't a board Moxfield
// has, so it is left out.
//
//	Commander
//	1 Ghen, Arcanum Weaver #wincon
//
//	Deck
//	1 Sol Ring (C21) 263 #ramp #artifact
func MoxfieldText(d *File) string {
	boards := []struct{ section, heading string }{
		{"commander", "Commander"},
		{"mainboard", "Deck"},
		{"sideboard", "Sideboard"},
		{"maybeboard", "Maybeboard"},
	}
	var b strings.Builder
	for _, board := range boards {
		entries := d.Section(board.section)
		if len(entries) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(board.heading + "\n")
		for _, e := range entries {
			b.WriteString(e.moxfieldLine() + "\n")
		}
	}
	return b.String()
}

func (e Entry) moxfieldLine() string {
	var b strings.Builder
	b.WriteString(itoa(max(e.Qty, 1)) + " " + e.Name)
	if e.Set != "" && e.Collector != "" {
		b.WriteString(" (" + strings.ToUpper(e.Set) + ") " + e.Collector)
	}
	for _, t := range e.Tags {
		b.WriteString(" #" + t)
	}
	return b.String()
}
