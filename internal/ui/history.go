package ui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/mtg"
	"ttr/internal/prints"
	"ttr/internal/theme"
)

// A card's printed text, through the years.
//
// gv on a card, the same key that shows a deck's versions — same question,
// two kinds of thing. Scryfall only serves a card's *current* wording, so
// this comes from MTGJSON, which publishes whole sets: a heavily reprinted
// card means one file per set it appeared in.
//
// That is the whole difficulty: a set's file is about 1.5 MB, and a card
// reprinted in forty sets is forty of them. So the sets already on disk are
// shown at once, and the rest are fetched straight away, newest first and
// one at a time, the history filling in as each lands. Each set is kept, and
// shared by every card printed in it, so the cost falls fast.

type histState int

const (
	histPrintings histState = iota // finding out where it was printed
	histFetching                   // sets still coming
	histReady
	histFailed
)

// cardHistory is the printed-text history of one card, and how far along it
// is.
type cardHistory struct {
	state     histState
	card      mtg.Card
	printings []prints.Printing
	// queue is the sets still to download, newest first, one at a time;
	// fetching is the one on its way, and reading the ones being read from
	// disk.
	queue     []string
	fetching  string
	reading   map[string]bool
	originals map[string]map[string]string
	revisions []prints.TextRevision
	err       error
}

type printingsMsg struct {
	card      string
	printings []prints.Printing
	err       error
}

type setTextMsg struct {
	card  string
	set   string
	cards map[string]string
	err   error
}

// openHistory is gv on a card.
func (m *Model) openHistory(c mtg.Card) tea.Cmd {
	if c.OracleID == "" || c.PrintsSearchURI == "" {
		m.notice = "no printing history for this card"
		return nil
	}

	m.info.open(infoVersions)
	m.info.oracle = c.OracleID
	if h, ok := m.histories[c.OracleID]; ok && h.state != histFailed {
		return nil // already have it, or already asking
	}

	m.histories[c.OracleID] = &cardHistory{
		state: histPrintings, card: c,
		originals: map[string]map[string]string{},
	}
	oracle, uri := c.OracleID, c.PrintsSearchURI
	return func() tea.Msg {
		list, err := prints.Printings(uri)
		return printingsMsg{card: oracle, printings: list, err: err}
	}
}

func (m Model) handlePrintings(msg printingsMsg) (tea.Model, tea.Cmd) {
	h, ok := m.histories[msg.card]
	if !ok {
		return m, nil
	}
	if msg.err != nil {
		h.state, h.err = histFailed, msg.err
		return m, nil
	}

	h.printings = msg.printings
	h.reading = map[string]bool{}
	// Each set once, newest first: the wording in force now matters most.
	byNew := append([]prints.Printing(nil), msg.printings...)
	sort.SliceStable(byNew, func(a, b int) bool { return byNew[a].Released > byNew[b].Released })
	seen := map[string]bool{}
	var cmds []tea.Cmd
	for _, p := range byNew {
		if seen[p.Set] {
			continue
		}
		seen[p.Set] = true
		// What's on disk costs nothing: read it all now.
		if prints.IsCached(p.Set) {
			h.reading[p.Set] = true
			cmds = append(cmds, fetchSetText(msg.card, p.Set))
			continue
		}
		h.queue = append(h.queue, p.Set)
	}
	cmds = append(cmds, h.fetchNext())
	h.state = histFetching
	if h.done() {
		m.finishHistory(h)
	}
	return m, tea.Batch(cmds...)
}

// fetchNext starts the next set in the queue downloading, if none is.
func (h *cardHistory) fetchNext() tea.Cmd {
	if h.fetching != "" || len(h.queue) == 0 {
		return nil
	}
	h.fetching, h.queue = h.queue[0], h.queue[1:]
	return fetchSetText(h.card.OracleID, h.fetching)
}

// done reports whether every set has arrived or failed.
func (h *cardHistory) done() bool {
	return h.fetching == "" && len(h.queue) == 0 && len(h.reading) == 0
}

// remaining is how many sets are still to come.
func (h *cardHistory) remaining() int {
	n := len(h.queue) + len(h.reading)
	if h.fetching != "" {
		n++
	}
	return n
}

func fetchSetText(oracle, set string) tea.Cmd {
	return func() tea.Msg {
		cards, err := prints.SetOriginals(set)
		return setTextMsg{card: oracle, set: set, cards: cards, err: err}
	}
}

// handleSetText takes one set's text, shows the history as far as it goes,
// and starts the next download. A set that won't download is left out.
func (m Model) handleSetText(msg setTextMsg) (tea.Model, tea.Cmd) {
	h, ok := m.histories[msg.card]
	if !ok {
		return m, nil
	}
	if msg.err == nil {
		h.originals[msg.set] = msg.cards
	}
	delete(h.reading, msg.set)
	var next tea.Cmd
	if h.fetching == msg.set {
		h.fetching = ""
		next = h.fetchNext()
	}
	m.finishHistory(h)
	return m, next
}

// finishHistory works the wordings out from what has arrived so far.
func (m *Model) finishHistory(h *cardHistory) {
	h.revisions = prints.BuildRevisions(h.card, h.printings, h.originals)
	if h.done() {
		h.state = histReady
	} else {
		h.state = histFetching
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// renderHistory draws what is known so far, and says what isn't.
func (m Model) renderHistory(c mtg.Card, width int) []string {
	h, ok := m.histories[c.OracleID]
	if !ok {
		return []string{mutedLine("gv for how its text has changed", width)}
	}

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)
	muted := lipgloss.NewStyle().Foreground(theme.TextMuted)

	switch h.state {
	case histPrintings:
		return []string{muted.Render(fit("finding the printings…", width))}
	case histFailed:
		return wrapStyled(errorText(h.err), width, lipgloss.NewStyle().Foreground(theme.Error))
	}

	// Newest first: the wording in force today on top, and the older ones
	// under it, the way you read back through a history.
	var out []string
	for i := range h.revisions {
		rev := h.revisions[len(h.revisions)-1-i]
		if i > 0 {
			out = append(out, "")
		}
		label := rev.SetName
		if label == "" {
			label = "current oracle text"
		}
		if year := year(rev.Released); year != "" {
			label += " · " + year
		}
		if rev.Printings > 1 {
			label += " · " + itoa(rev.Printings) + " printings"
		}
		if rev.Current {
			label += " · current"
		}
		out = append(out, head.Render(fit(label, width)))
		out = append(out, highlightOracle(rev.Text, c, width, m.rules)...)
	}

	if len(h.revisions) == 0 && h.state == histReady {
		out = append(out, muted.Render(fit("no printed text on record", width)))
	}

	if h.state == histFetching {
		n := h.remaining()
		out = append(out, "", dim.Render(fit("fetching "+itoa(n)+" more "+plural("set", n)+
			" from MTGJSON, newest first…", width)))
	}
	return out
}

func year(released string) string {
	if i := strings.Index(released, "-"); i > 0 {
		return released[:i]
	}
	return released
}
