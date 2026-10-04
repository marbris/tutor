package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/catalog"
)

// Scryfall's catalogs: every keyword and type it knows. Loaded at the start,
// off the main loop, like the Tagger tags: from disk whatever their age, and
// fetched again behind once a week old. Until they're in, highlighting and
// the statistics go by the rules text and the type line alone.

type catalogMsg struct {
	data  *catalog.Data
	stale bool
}

// loadCatalog reads or fetches the catalogs, quietly: they're a refinement,
// and offline is no reason to complain.
func loadCatalog() tea.Msg {
	d, stale, _ := catalog.Load()
	return catalogMsg{data: d, stale: stale}
}

func refreshCatalog() tea.Msg {
	d, _ := catalog.Refresh()
	return catalogMsg{data: d}
}

// handleCatalog puts the catalogs in use, and folds their keywords into the
// rules every list and panel already holds.
func (m Model) handleCatalog(msg catalogMsg) (tea.Model, tea.Cmd) {
	if msg.data == nil || downloadsOff()["catalogs"] {
		return m, nil // none, or turned off while they were on their way
	}
	catalog.SetCurrent(msg.data)
	m.rules.AddCatalog(msg.data)
	if msg.stale {
		return m, refreshCatalog
	}
	return m, nil
}
