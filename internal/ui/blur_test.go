package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"ttr/internal/theme"
)

// ansiOf is how a colour starts in the output, as foreground or background.
func ansiOf(c lipgloss.Color) (fg, bg string) {
	f := lipgloss.NewStyle().Foreground(c).Render("x")
	b := lipgloss.NewStyle().Background(c).Render("x")
	return f[:strings.Index(f, "x")], b[:strings.Index(b, "x")]
}

// With the editing deck shared, a ttr whose window is in the background
// draws no panel as having the keys: no focus border, no cursor row. Focus
// back, and it looks as before.
func TestABlurredTtrDrawsNoFocusAndAFocusedOneDoes(t *testing.T) {
	shareOn(t)
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	m := withCards(sized(160, 30), "d", deckSample(), sortArrival)
	focusFg, _ := ansiOf(theme.BorderFocus)
	_, cursorBg := ansiOf(theme.SelectionBg)
	panel := func(m Model) string { return m.viewPanel(m.ws.current(), m.ws.focused, 50, 20, 3) }

	before := panel(m)
	if !strings.Contains(before, focusFg) {
		t.Fatal("the focused panel has no focus border to begin with")
	}
	next, _ := m.Update(tea.BlurMsg{})
	m = next.(Model)
	blurred := panel(m)
	if strings.Contains(blurred, focusFg) {
		t.Error("the background window still draws its panel as focused")
	}
	// The card under the cursor keeps a grey bar, for the card column and
	// the statistics beside it: in the selection's colours, which always read.
	c, _ := m.ws.current().cardsView().current()
	row := func(frame string) string {
		for _, line := range splitLines(frame) {
			if strings.Contains(stripANSI(line), c.Card.Name) {
				return line
			}
		}
		return ""
	}
	selFg, _ := ansiOf(theme.SelectionFg)
	if idle := row(blurred); !strings.Contains(idle, cursorBg) || !strings.Contains(idle, selFg) || idle == row(before) {
		t.Errorf("the cursor row in the background isn't the grey bar:\n%q\nfocused:\n%q", idle, row(before))
	}
	next, _ = m.Update(tea.FocusMsg{})
	m = next.(Model)
	if panel(m) != before {
		t.Error("focus back didn't bring the focused look back")
	}

	// A key is the window having the focus, whatever the terminal said.
	next, _ = m.Update(tea.BlurMsg{})
	m = drive(next.(Model), "j")
	if m.blurred {
		t.Error("a key press left the window blurred")
	}
}

// Unshared, each ttr is on its own: in the background it still shows its
// focused panel and cursor.
func TestUnsharedABlurredTtrKeepsItsFocus(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	m := withCards(sized(160, 30), "d", deckSample(), sortArrival)
	panel := func(m Model) string { return m.viewPanel(m.ws.current(), m.ws.focused, 50, 20, 3) }
	before := panel(m)
	next, _ := m.Update(tea.BlurMsg{})
	if panel(next.(Model)) != before {
		t.Error("with sharing off, the background window lost its focused look")
	}
}
