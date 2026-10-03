package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/tagger"
)

// Scryfall Tagger's tags, for the info panel, the statistics and otag:
// completion. Loaded at the start, off the main loop — from disk, or once
// a week downloaded — and until then everything that uses them shows
// nothing, rather than waiting.

// taggerData is the tags once they are in; nil until then, or when there
// were none to be had. Shared, like globalTags, by everything that shows
// them.
var taggerData *tagger.Data

type taggerMsg struct {
	data *tagger.Data
}

// loadTagger reads or downloads the tags. A failure is quiet: Tagger's
// tags are extra, and offline is no reason to complain at startup.
func loadTagger() tea.Msg {
	d, _ := tagger.Load()
	return taggerMsg{data: d}
}
