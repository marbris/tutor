package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/cache"
	"ttr/internal/catalog"
	"ttr/internal/deck"
	"ttr/internal/rulings"
	"ttr/internal/tagger"
)

// r in the settings panel: one kind of data, fetched again.
//
// The bulk files — Tagger's tags, the rulings, the catalogs, the rules, the
// decks of the people you follow — download again at once. The kinds
// fetched a card or a set at a time — pictures, printings, printed texts —
// are only marked due (cache.MarkDue): each is fetched again the next time
// it's shown, the kept copy standing in until then. There are a thousand
// pictures, and a refresh shouldn't queue them all. Card data is the same,
// a deck at a time: the open decks ask again now, the rest as they open.

// refreshedMsg is a refresh done: what to say, and the message the kind's
// own loader produced, to be handled as it would be at startup.
type refreshedMsg struct {
	panel  int
	notice string
	inner  tea.Msg
}

// refreshEffect says what r does to a kind, for the info panel.
func refreshEffect(kind string) string {
	switch kind {
	case "pictures", "printings", "texts":
		return "each is fetched again the next time it's shown."
	case "cards":
		return "the open decks ask Scryfall for their cards again now, the others as they open."
	case "decks":
		return "the decks of the people you follow are fetched again now."
	}
	return "downloads it again now."
}

// refreshKind is r on a row of the settings panel.
func (m *Model) refreshKind(panelID int, kind string) tea.Cmd {
	if downloadsOff()[kind] {
		m.notice = kind + " is turned off; enter turns it on"
		return nil
	}
	what := kind
	for _, k := range cache.Kinds {
		if k.Name == kind {
			what = k.What
		}
	}
	// done runs a download and says how it went; the loaded data goes on
	// to the kind's own handler, as at startup.
	done := func(load func() (tea.Msg, error)) tea.Cmd {
		return func() tea.Msg {
			inner, err := load()
			if err != nil {
				return refreshedMsg{panel: panelID, notice: "couldn't refresh " + what + ": " + err.Error()}
			}
			return refreshedMsg{panel: panelID, notice: what + " refreshed", inner: inner}
		}
	}

	switch kind {
	case "tagger":
		m.notice = "downloading " + what + " again…"
		return done(func() (tea.Msg, error) {
			d, err := tagger.Refresh()
			return taggerMsg{data: d}, err
		})
	case "rulings":
		m.notice = "downloading " + what + " again…"
		return done(func() (tea.Msg, error) {
			s, err := rulings.Refresh()
			return rulingsLoadedMsg{store: s}, err
		})
	case "catalogs":
		m.notice = "downloading " + what + " again…"
		return done(func() (tea.Msg, error) {
			d, err := catalog.Refresh()
			return catalogMsg{data: d}, err
		})
	case "rules":
		// The rules sync says for itself whether there was a new release.
		m.notice = "checking for new comprehensive rules…"
		return func() tea.Msg { return refreshedMsg{panel: panelID, inner: syncRules()} }
	case "decks":
		users := deck.LoadBookmarks().Users
		if len(users) == 0 {
			m.notice = "you follow nobody on Moxfield"
			return nil
		}
		m.notice = "fetching the decks of " + itoa(len(users)) + " " + plural("person", len(users)) + " again…"
		cmds := make([]tea.Cmd, 0, len(users))
		for _, u := range users {
			cmds = append(cmds, fetchUserDecks(u))
		}
		return tea.Batch(append(cmds, func() tea.Msg {
			return settingsDoneMsg{panel: panelID}
		})...)
	case "cards":
		if err := deck.MarkCardsStale(); err != nil {
			m.notice = "error: " + err.Error()
			return nil
		}
		var cmds []tea.Cmd
		for _, p := range m.ws.panels {
			if l := p.cardsView(); l != nil && l.deck != nil && l.deck.Local() {
				cmds = append(cmds, refreshDeckCards(p.id, l.deck.Slug))
			}
		}
		m.notice = what + ": the open decks ask again, the others as they open"
		return tea.Batch(cmds...)
	case "pictures", "printings", "texts":
		if err := cache.MarkDue(kind); err != nil {
			m.notice = "error: " + err.Error()
			return nil
		}
		m.notice = what + ": each is fetched again the next time it's shown"
		return nil
	}
	return nil
}

func (m Model) handleRefreshed(msg refreshedMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg.inner != nil {
		next, c := m.Update(msg.inner)
		m, cmd = next.(Model), c
	}
	if msg.notice != "" {
		m.notice = msg.notice
	}
	if p := m.ws.byID(msg.panel); p != nil {
		if v, ok := p.top().(*settingsView); ok {
			v.refresh()
		}
	}
	return m, cmd
}
