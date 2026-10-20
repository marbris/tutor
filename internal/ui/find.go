package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/cardstore"
	"ttr/internal/deck"
	"ttr/internal/fetch"
	"ttr/internal/keymap"
	"ttr/internal/query"
	"ttr/internal/scryfall"
)

// Searching for cards.
//
// With the cards on disk (internal/cardstore), a search runs over them, in
// Tutor's own part of Scryfall's syntax (internal/query), and asks nobody.
// A word it doesn't know is named in the notice, and alt+enter sends the
// query to Scryfall instead. Without them — turned off, or not downloaded
// yet — a search asks Scryfall, as it always did.
//
// Two orders are in play and they are not the same thing. The query sort —
// ctrl+o, with ctrl+r for its direction — is part of the request: it
// decides which cards come back when a query matches more than one page.
// Both only change what the next enter asks for; neither sends anything. The
// list sorts — . and , with alt for their directions — decide how the cards
// already in front of you are arranged, and carry over from one search to
// the next. Confusing them means
// re-fetching to reorder, or reordering and wondering why the cards changed.

// maxResults is one Scryfall page, which is the whole result set kept.
const maxResults = 175

// maxLocal is how many cards a local search keeps, in its order. More is
// cheap to find but no use to scroll; the header says how many matched.
const maxLocal = 1000

// searchDoneMsg carries a finished search back to the panel that asked for
// it. Panels are identified by id rather than by index because the row can
// be reordered or closed while a request is in flight.
type searchDoneMsg struct {
	panel int
	query string
	cards []deck.Card
	total int
	err   error
	// local is a search of the cards on disk; unknown is the words in it
	// that it couldn't read.
	local   bool
	unknown []string
}

// runLocalSearch searches the cards on disk, off the main thread.
func runLocalSearch(s *cardstore.Store, panelID int, q, order, dir string) tea.Cmd {
	return func() tea.Msg {
		parsed := query.ParseSearch(q)
		var found []int
		// Nothing left once the words it couldn't read are out is no
		// question at all, not one every card answers.
		if !parsed.Empty() {
			found = query.Search(s, parsed, order, dir)
		}
		msg := searchDoneMsg{panel: panelID, query: q, total: len(found), local: true, unknown: parsed.Unknown()}
		if len(found) == 0 {
			msg.err = fetch.NotFound{}
			return msg
		}
		found = found[:min(len(found), maxLocal)]
		msg.cards = make([]deck.Card, 0, len(found))
		for _, i := range found {
			c, err := s.Card(i)
			if err != nil {
				return searchDoneMsg{panel: panelID, query: q, err: err}
			}
			msg.cards = append(msg.cards, deck.Card{Card: c})
		}
		return msg
	}
}

// runSearch asks Scryfall, off the main thread.
func runSearch(panelID int, query, order, dir string) tea.Cmd {
	return func() tea.Msg {
		cards, total, err := scryfall.Search(query, order, dir, maxResults)
		if err != nil {
			return searchDoneMsg{panel: panelID, query: query, err: err}
		}

		// A search result is a deck card with no quantity and no tags,
		// which is exactly what it is.
		out := make([]deck.Card, len(cards))
		for i, c := range cards {
			out[i] = deck.Card{Card: c}
		}
		return searchDoneMsg{panel: panelID, query: query, cards: out, total: total}
	}
}

// search starts one for the focused panel and puts it into its waiting
// state: over the cards on disk if they're in, or asking Scryfall when
// askScryfall says to or they aren't.
func (m *Model) search(p *panel, askScryfall bool) tea.Cmd {
	query := p.search.Value()
	if query == "" {
		return nil
	}

	m.history = RememberQuery(m.history, query)
	SaveQueryHistory(m.history)
	p.leaveHistory()

	p.loading = true
	p.err = nil
	p.searchOpen = false
	p.search.Blur()
	p.title = query

	if !askScryfall && m.storeLoading {
		// A search at the start, from the session or the command line:
		// the cards on disk are a moment from loaded, and it waits for
		// them rather than asking Scryfall.
		p.awaitingStore = true
		return nil
	}
	return p.runSearch(askScryfall)
}

// runSearch sends the search in the panel's bar.
func (p *panel) runSearch(askScryfall bool) tea.Cmd {
	if s := cardstore.Current(); s != nil && !askScryfall {
		return runLocalSearch(s, p.id, p.title, p.queryOrder(), p.queryDirection())
	}
	return runSearch(p.id, p.title, p.queryOrder(), p.queryDirection())
}

// searchesWaiting sends the searches that waited for the cards on disk, to
// them if they loaded and to Scryfall if they didn't.
func (m *Model) searchesWaiting() tea.Cmd {
	m.storeLoading = false
	var cmds []tea.Cmd
	for _, p := range m.ws.panels {
		if p.awaitingStore {
			p.awaitingStore = false
			cmds = append(cmds, p.runSearch(false))
		}
	}
	return tea.Batch(cmds...)
}

// unknownNotice is what a local search says about the words it couldn't
// read.
func unknownNotice(words []string) string {
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = "“" + w + "”"
	}
	verb := " isn't"
	if len(words) > 1 {
		verb = " aren't"
	}
	return strings.Join(quoted, ", ") + verb + " searched locally · " +
		keymap.Hint(keymap.Search, keymap.SearchScryfall) + " asks Scryfall"
}

// handleSearchDone files a finished search, if the panel that asked for it
// is still there and still waiting for this query.
func (m Model) handleSearchDone(msg searchDoneMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil || p.title != msg.query {
		// The panel closed, or moved on to another search. Either way this
		// answer is to a question nobody is asking any more.
		return m, nil
	}

	p.loading = false
	if len(msg.unknown) > 0 {
		m.notice = unknownNotice(msg.unknown)
	}
	if msg.err != nil {
		// The orders outlive a failed search too, so the next one that
		// works still comes back laid out the way you had it.
		if l := p.cardsView(); l != nil {
			p.lastSorts = l
		}
		p.err = msg.err
		p.stack = nil
		return m, nil
	}

	// Laid out the way the last search in this panel was: the orders are
	// how you read a list, not part of the question. A panel's first search
	// starts at the head of the cycle — Scryfall's own order, as it ships,
	// which is the answer to the order the query asked for.
	arrival := "scryfall order"
	if msg.local {
		arrival = "search order"
	}
	l := newCardList(msg.cards, firstSort(), arrival)
	if prev := p.cardsView(); prev != nil {
		l.keepSorts(prev)
	} else if p.lastSorts != nil {
		l.keepSorts(p.lastSorts)
	}
	p.lastSorts = nil
	if p.pending != nil {
		p.pending.applyLayout(l)
		p.pending = nil
	}
	l.name = msg.query
	l.matched = msg.total
	p.show(l)
	return m, nil
}

// errorText is what a failed search says. "No results" is an answer rather
// than a failure, and reads better as one.
func errorText(err error) string {
	if _, ok := err.(fetch.NotFound); ok {
		return "no results"
	}
	return err.Error()
}

// cycleQuerySort changes which cards a large query will come back with. It
// sends nothing: the bar may hold a query you haven't finished, and the
// panel may be showing results for a different one. Enter sends.
func (p *panel) cycleQuerySort(delta int) {
	n := len(scryfall.SortOptions)
	p.querySort = ((p.querySort+delta)%n + n) % n
}

// cycleQueryDir changes which way the next search asks for its order to
// run: auto, ascending, descending. Like the order, it waits for enter.
func (p *panel) cycleQueryDir(delta int) {
	n := len(scryfall.DirOptions)
	p.queryDir = ((p.queryDir+delta)%n + n) % n
}

// queryOrder is the name of the order the request will ask for.
func (p *panel) queryOrder() string { return scryfall.SortOptions[p.querySort] }

// queryDirection is the name of the direction the request will ask for.
func (p *panel) queryDirection() string { return scryfall.DirOptions[p.queryDir] }

// queryOrderLabel is the order as the header shows it: the name, with an
// arrow when the direction is chosen rather than left to Scryfall.
func (p *panel) queryOrderLabel() string {
	switch p.queryDirection() {
	case "asc":
		return p.queryOrder() + " ↑"
	case "desc":
		return p.queryOrder() + " ↓"
	}
	return p.queryOrder()
}
