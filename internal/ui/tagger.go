package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/tagger"
)

// Scryfall Tagger's tags, for the info panel, the statistics and otag:
// completion. Loaded at the start, off the main loop — from disk, or once
// a week downloaded — into tagger.Current, and until then everything that
// uses them shows nothing, rather than waiting.

type taggerMsg struct {
	data *tagger.Data
}

// loadTagger reads or downloads the tags. A failure is quiet: Tagger's
// tags are extra, and offline is no reason to complain at startup.
func loadTagger() tea.Msg {
	d, _ := tagger.Load()
	return taggerMsg{data: d}
}

// handleTagger puts the tags in use. A list narrowed by a Tagger category
// before they arrived — a session restored — is narrowed again now that
// the category can match.
func (m Model) handleTagger(msg taggerMsg) (tea.Model, tea.Cmd) {
	if msg.data == nil {
		return m, nil
	}
	tagger.SetCurrent(msg.data)
	for _, p := range m.ws.panels {
		if l := p.cardsView(); l != nil && len(l.statFilter) > 0 {
			l.refresh()
		}
	}
	return m, nil
}
