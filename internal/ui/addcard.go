package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/cardstore"
	"ttr/internal/deck"
	"ttr/internal/fetch"
	"ttr/internal/scryfall"
)

// i on a deck: one card, fetched from Scryfall straight into the deck in
// front of you — the way you add the card you already know the name of,
// without opening a search panel to find it in.
//
// A query is a query, so it can match more than one card. One answer is
// added. Among several, a card whose name is exactly what you typed wins —
// "Forest" matches every card with a forest in its text, and you meant the
// land. Anything else is ambiguous, and the prompt comes back with your
// query in it to be made more specific.

// addCardMsg carries a finished lookup back to the panel that asked.
type addCardMsg struct {
	panel int
	query string
	cards []deck.Card
	total int
	err   error
}

// runAddCard looks the card up off the main thread: among the cards on
// disk when they're in and the query is one they can answer, and otherwise
// on Scryfall.
func runAddCard(panelID int, query string) tea.Cmd {
	return func() tea.Msg {
		if s := cardstore.Current(); s != nil {
			cards, total, unknown, err := searchStore(s, query, "edhrec", "auto", maxResults)
			if err == nil && len(unknown) == 0 {
				return addCardMsg{panel: panelID, query: query, cards: cards, total: total}
			}
		}
		cards, total, err := scryfall.Search(query, "edhrec", "auto", maxResults)
		msg := addCardMsg{panel: panelID, query: query, total: total, err: err}
		for _, c := range cards {
			msg.cards = append(msg.cards, deck.Card{Card: c})
		}
		return msg
	}
}

// pickAddCard is the one card a lookup settles on, or false when it didn't
// settle on one.
func pickAddCard(query string, cards []deck.Card, total int) (deck.Card, bool) {
	if len(cards) == 1 && total <= 1 {
		return cards[0], true
	}
	var exact []deck.Card
	for _, c := range cards {
		if strings.EqualFold(c.Card.Name, strings.TrimSpace(query)) {
			exact = append(exact, c)
		}
	}
	if len(exact) == 1 {
		return exact[0], true
	}
	return deck.Card{}, false
}

// handleAddCard files the answer into the deck that asked, if it is still
// there and still a deck of yours.
func (m Model) handleAddCard(msg addCardMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	l := p.cardsView()
	if l == nil || l.deck == nil || !l.deck.Local() {
		return m, nil // moved on to something that isn't a deck of yours
	}

	if msg.err != nil {
		if _, ok := msg.err.(fetch.NotFound); ok {
			m.notice = "no card matches " + msg.query
		} else {
			m.notice = "error: " + msg.err.Error()
		}
		return m, nil
	}

	c, ok := pickAddCard(msg.query, msg.cards, msg.total)
	if !ok {
		n := max(msg.total, len(msg.cards))
		if n == 0 {
			m.notice = "no card matches " + msg.query
			return m, nil
		}
		m.notice = itoa(n) + " matches — be more specific"
		// Asked again over the same panel, so fixing the query is typing
		// rather than starting over. Not over another prompt, though — the
		// answer came back after you'd moved on to asking something else.
		if p.asking == askNone && !p.filtering {
			p.askAdd(msg.query)
		}
		return m, nil
	}

	m.notice = addTo(l, []deck.Card{c})
	l.selectByName(c.Card.Name)
	return m, m.autosave()
}
