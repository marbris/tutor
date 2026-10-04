package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/mtg"
	"ttr/internal/rulings"
	"ttr/internal/scryfall"
)

// Rulings, read from Scryfall's rulings file on disk (internal/rulings) the
// moment the cursor lands. Until that file is in, or for a card with no
// oracle id, they're fetched as the cursor settles:
//
// Every card has its own rulings endpoint, so showing them means a request
// per card — which would be a request per keypress if it happened the moment
// the cursor moved. It waits instead, and a sequence number throws away the
// ticks for cards you scrolled straight past.

// rulingsDelay is how long a card has to stay under the cursor before it is
// worth asking about.
const rulingsDelay = 120 * time.Millisecond

type rulingsTickMsg struct {
	panel int
	card  string
	uri   string
	seq   int
}

type rulingsMsg struct {
	panel   int
	card    string
	rulings []mtg.Ruling
	err     error
}

// hover is called after every move, and starts the clock on whatever is now
// under the cursor.
func (m *Model) hover() tea.Cmd {
	p := m.ws.current()
	if p == nil {
		return nil
	}
	l := p.cardsView()
	if l == nil {
		return nil
	}
	c, ok := l.current()
	if !ok || c.Card.ID == "" {
		return nil
	}
	if _, done := l.rulings[c.Card.ID]; done {
		return nil
	}
	if _, failed := l.rulingErr[c.Card.ID]; failed {
		return nil
	}
	// Kept on disk, they're read now: no request, so nothing to wait for.
	if rs, ok := rulings.Current().Of(c.Card.OracleID); ok {
		l.rulings[c.Card.ID] = rs
		return nil
	}

	m.hoverSeq++
	seq, id, uri, panelID := m.hoverSeq, c.Card.ID, c.Card.RulingsURI, p.id
	return tea.Tick(rulingsDelay, func(time.Time) tea.Msg {
		return rulingsTickMsg{panel: panelID, card: id, uri: uri, seq: seq}
	})
}

func (m Model) handleRulingsTick(msg rulingsTickMsg) (tea.Model, tea.Cmd) {
	// A tick from a card the cursor has since left is a question nobody is
	// asking any more.
	if msg.seq != m.hoverSeq {
		return m, nil
	}
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	l := p.cardsView()
	if l == nil {
		return m, nil
	}
	if c, ok := l.current(); !ok || c.Card.ID != msg.card {
		return m, nil
	}

	ask := func() tea.Msg {
		got, err := scryfall.Rulings(msg.uri)
		return rulingsMsg{panel: msg.panel, card: msg.card, rulings: got, err: err}
	}
	// Rulings kept on disk show at once, however old. Stale ones are asked
	// for again behind them (and stand in if that fails).
	kept, fresh, ok := scryfall.KeptRulings(msg.uri)
	if !ok {
		return m, ask
	}
	show := func() tea.Msg { return rulingsMsg{panel: msg.panel, card: msg.card, rulings: kept} }
	if fresh {
		return m, show
	}
	return m, tea.Sequence(show, ask)
}

func (m Model) handleRulings(msg rulingsMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	// Filed against every list holding this card, not just the one that
	// asked: the same card in two panels is the same card.
	for _, other := range m.ws.panels {
		l := other.cardsView()
		if l == nil {
			continue
		}
		if msg.err != nil {
			l.rulingErr[msg.card] = msg.err
			continue
		}
		l.rulings[msg.card] = msg.rulings
	}
	return m, nil
}

// rulingsLoadedMsg is the rulings file on disk, indexed, at the start.
type rulingsLoadedMsg struct {
	store *rulings.Store
	stale bool
}

// loadRulingsFile reads or downloads the rulings file, quietly: without it,
// rulings are fetched a card at a time as before.
func loadRulingsFile() tea.Msg {
	s, stale, _ := rulings.Load()
	return rulingsLoadedMsg{store: s, stale: stale}
}

func refreshRulingsFile() tea.Msg {
	s, _ := rulings.Refresh()
	return rulingsLoadedMsg{store: s}
}

func (m Model) handleRulingsLoaded(msg rulingsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.store == nil || downloadsOff()["rulings"] {
		return m, nil // none, or turned off while it was on its way
	}
	rulings.SetCurrent(msg.store)
	cmd := m.hover() // the card under the cursor needn't wait for a move
	if msg.stale {
		return m, tea.Batch(cmd, refreshRulingsFile)
	}
	return m, cmd
}
