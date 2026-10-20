package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/cardstore"
)

// Every card and printing, kept on disk (internal/cardstore). Loaded at
// the start, off the main loop, from disk whatever its age, and downloaded
// again behind it once it is a week old. With none kept, it is downloaded
// now, about 80 MB, with a notice counting it; nothing waits for it, and
// until it is in, everything asks Scryfall as it always did.

type cardStoreMsg struct {
	store *cardstore.Store
	stale bool
	// first is a download with no store kept before it, which says so
	// when it lands.
	first bool
	err   error
}

// cardStoreTickMsg counts the download along in the notice.
type cardStoreTickMsg struct{}

// loadCardStore reads the store, or downloads it when there's none.
func loadCardStore() tea.Msg {
	s, stale := cardstore.Load()
	if s != nil {
		return cardStoreMsg{store: s, stale: stale}
	}
	s, err := cardstore.Refresh()
	return cardStoreMsg{store: s, first: true, err: err}
}

// refreshCardStore downloads the store again, quietly: the kept one stays
// in use if it fails.
func refreshCardStore() tea.Msg {
	s, err := cardstore.Refresh()
	return cardStoreMsg{store: s, err: err}
}

// cardStoreTick looks at the download again in a moment.
func cardStoreTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return cardStoreTickMsg{} })
}

// cardStoreProgress is the notice while the store downloads.
func cardStoreProgress() (string, bool) {
	done, of, ok := cardstore.Downloading()
	if !ok {
		return "", false
	}
	return "card data " + itoa(int(done>>20)) + "/" + itoa(int(of>>20)) + " MB", true
}

// handleCardStoreTick puts the download's progress in the notice, unless
// something else is being said there.
func (m Model) handleCardStoreTick() (tea.Model, tea.Cmd) {
	text, ok := cardStoreProgress()
	if !ok {
		return m, nil
	}
	if m.notice == "" || isCardStoreNotice(m.notice) {
		m.notice = text
	}
	return m, cardStoreTick()
}

func isCardStoreNotice(s string) bool { return len(s) >= 9 && s[:9] == "card data" }

func (m Model) handleCardStore(msg cardStoreMsg) (tea.Model, tea.Cmd) {
	if isCardStoreNotice(m.notice) {
		m.notice = ""
	}
	if msg.store == nil || downloadsOff()[cardStoreDownload] {
		if msg.first && msg.err != nil {
			m.notice = "card data: " + errorText(msg.err) + " — searches ask Scryfall"
		}
		return m, nil // none, or turned off while it was on its way
	}
	cardstore.SetCurrent(msg.store)
	if msg.first {
		m.notice = "card data in: " + itoa(msg.store.Len()) + " cards"
	}
	if msg.stale {
		return m, tea.Batch(refreshCardStore, cardStoreTick())
	}
	return m, nil
}

// cardStoreDownload is the store's name as a download and a cache kind.
const cardStoreDownload = "allcards"
