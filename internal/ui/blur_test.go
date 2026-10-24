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

// A ttr whose window is in the background draws no panel as having the
// keys: no focus border, no cursor row. Focus back, and it looks as before.
func TestABlurredTtrDrawsNoFocusAndAFocusedOneDoes(t *testing.T) {
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
	if blurred := panel(m); strings.Contains(blurred, focusFg) || strings.Contains(blurred, cursorBg) {
		t.Error("the background window still draws its panel as focused")
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
