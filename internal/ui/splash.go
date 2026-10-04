package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ttr/internal/keymap"
	"ttr/internal/theme"
)

// The splash.
//
// The splash is what `ttr` opens to with nothing restored: no panels, and
// therefore nothing to look at but the way in. It says the three keys that
// open something and gets out of the way.

func (m Model) viewSplash() string {
	name := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("Tutor")
	tag := lipgloss.NewStyle().Foreground(theme.TextDim).
		Render("Magic: The Gathering — cards, decks and rules")

	key := lipgloss.NewStyle().Foreground(theme.Highlight).Bold(true)
	what := lipgloss.NewStyle().Foreground(theme.Text)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	rows := []struct{ k, v string }{
		{leaderHint(keymap.LeaderFind), "find cards on Scryfall"},
		{leaderHint(keymap.LeaderDecks), "your decks, and Moxfield"},
		{leaderHint(keymap.LeaderRules), "the comprehensive rules"},
		{leaderHint(keymap.LeaderSettings), "settings: sync, downloads, cache"},
	}

	var lines []string
	lines = append(lines, name, tag, "")
	for _, r := range rows {
		lines = append(lines, key.Render(pad(r.k, 9))+" "+what.Render(r.v))
	}
	var how []string
	for _, h := range []struct {
		action keymap.Action
		what   string
	}{
		{keymap.GlobalLeader, " for the menu"},
		{keymap.GlobalHelp, " for the keys"},
		{keymap.GlobalQuit, " to quit"},
	} {
		if k := keymap.Hint(keymap.Global, h.action); k != "" {
			how = append(how, k+h.what)
		}
	}
	lines = append(lines, "", dim.Render(strings.Join(how, " · ")))

	block := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}
