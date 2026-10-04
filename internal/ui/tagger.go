package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/tagger"
)

// Scryfall Tagger's tags, for the info panel, the statistics and otag:
// completion. Loaded at the start, off the main loop, into tagger.Current:
// from disk whatever its age, and downloaded again behind it once it is a
// week old; downloaded at once only when nothing is kept. Until they are in,
// everything that uses them shows nothing, rather than waiting.

type taggerMsg struct {
	data *tagger.Data
	// stale is a copy over a week old, in use while it's downloaded again.
	stale bool
}

// loadTagger reads or downloads the tags. A failure is quiet: Tagger's
// tags are extra, and offline is no reason to complain at startup.
func loadTagger() tea.Msg {
	d, stale, _ := tagger.Load()
	return taggerMsg{data: d, stale: stale}
}

// refreshTagger downloads the tags again, quietly: the stale copy stays in
// use if it fails.
func refreshTagger() tea.Msg {
	d, _ := tagger.Refresh()
	return taggerMsg{data: d}
}

// handleTagger puts the tags in use. A list narrowed by a Tagger category
// before they arrived — a session restored — is narrowed again now that
// the category can match.
func (m Model) handleTagger(msg taggerMsg) (tea.Model, tea.Cmd) {
	if msg.data == nil || downloadsOff()["tagger"] {
		return m, nil // none, or turned off while it was on its way
	}
	tagger.SetCurrent(msg.data)
	for _, p := range m.ws.panels {
		if l := p.cardsView(); l != nil && len(l.statFilter) > 0 {
			l.refresh()
		}
	}
	if msg.stale {
		return m, refreshTagger
	}
	return m, nil
}
