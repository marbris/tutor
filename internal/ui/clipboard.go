package ui

import (
	"os"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"ttr/internal/deck"
	"ttr/internal/keymap"
)

// gm: a deck of yours, copied as text for Moxfield's import (see
// deck.MoxfieldText), tags and all, so a deck built here can go up there
// by pasting.

// copyText puts text on the system clipboard: through the system's own
// tool where there is one (pbcopy on macOS, wl-copy or xclip on Linux, the
// clipboard API on Windows), and otherwise by asking the terminal with
// OSC 52, which reaches the clipboard over ssh too. A variable, so tests
// don't touch the real clipboard.
var copyText = func(text string) error {
	if err := clipboard.WriteAll(text); err == nil {
		return nil
	}
	termenv.NewOutput(os.Stdout).Copy(text)
	return nil
}

// copyForMoxfield is gm: the deck under the cursor in the decks panel, or
// the deck in front of you, as the file holds it.
func (m *Model) copyForMoxfield(p *panel) tea.Cmd {
	switch v := p.top().(type) {
	case *deckList:
		e, ok := v.current()
		switch {
		case !ok || e.kind == entryFolder || e.kind == entryUser:
		case e.kind != entryLocal:
			m.notice = "that deck is on Moxfield already — " + gotoHint(keymap.GotoImage) + " opens it"
			return nil
		default:
			slug := e.slug
			return func() tea.Msg {
				d, err := deck.Read(slug)
				if err != nil {
					return noticeMsg{err: err}
				}
				return copiedForMoxfield(d)
			}
		}
	case *cardList:
		if v.deck != nil && v.deck.Local() {
			info, cards := *v.deck, append([]deck.Card(nil), v.all...)
			return func() tea.Msg { return copiedForMoxfield(deck.Updated(info, cards)) }
		}
	}
	m.notice = gotoHint(keymap.GotoMoxfield) + " copies a deck of yours for Moxfield — a deck in the decks panel, or one open"
	return nil
}

// copiedForMoxfield copies the deck and says so.
func copiedForMoxfield(d *deck.File) tea.Msg {
	if err := copyText(deck.MoxfieldText(d)); err != nil {
		return noticeMsg{err: err}
	}
	n := 0
	for _, e := range d.Entries {
		if e.Section == "commander" || e.Section == "mainboard" {
			n += max(e.Qty, 1)
		}
	}
	return noticeMsg{text: "copied " + d.Name + " for Moxfield, " + itoa(n) + " " + plural("card", n) +
		" — paste it into Moxfield's import"}
}
