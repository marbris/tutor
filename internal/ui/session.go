package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/deck"
	"ttr/internal/paths"
	"ttr/internal/scryfall"
	"ttr/internal/stats"
)

// What `ttr` on its own comes back to.
//
// The workspace is the thing worth restoring: which panels were open, what
// each was showing, how it was sorted and filtered, and which deck you were
// building. Not the cursor, and not what was highlighted — coming back to a
// card you don't remember selecting is disorienting in a way that coming
// back to your panels isn't.
//
// Only things that will still be there tomorrow are recorded. A deck browsed
// off Moxfield isn't yours and might be gone; a search can always be run
// again, which is why it is the query that's kept and not its results.

const sessionFile = "session.json"

type panelSession struct {
	Kind string `json:"kind"`
	// Query is what a find or rules panel was showing.
	Query string `json:"query,omitempty"`
	// Deck is the slug of a local deck the panel had open.
	Deck string `json:"deck,omitempty"`

	// How the list was laid out and narrowed. Orders go by name, not by
	// number, so reordering the enum or the cycle can't turn one order into
	// another overnight.
	Sort1 string `json:"sort1,omitempty"`
	Sort2 string `json:"sort2,omitempty"`
	Desc1 bool   `json:"desc1,omitempty"`
	Desc2 bool   `json:"desc2,omitempty"`
	// Filter is the / filter.
	Filter string `json:"filter,omitempty"`
	// Stats is the statistics filter, a category at a time.
	Stats []savedClause `json:"stats,omitempty"`
	// QuerySort and QueryDir are the order a find panel asks Scryfall for.
	QuerySort string `json:"querySort,omitempty"`
	QueryDir  string `json:"queryDir,omitempty"`
}

// savedClause is one statistics category. A category is found again by its
// group and label: the test it filters with is a closure, and can't be kept.
type savedClause struct {
	Op    string `json:"op"`
	Group string `json:"group"`
	Label string `json:"label"`
}

type session struct {
	Panels  []panelSession `json:"panels,omitempty"`
	Focused int            `json:"focused,omitempty"`
	// Editing is the index of the panel that was the editing deck. Chosen
	// with e, so it is worth coming back to.
	Editing int `json:"editing,omitempty"`
	// LastTag is what A tags with.
	LastTag string `json:"lastTag,omitempty"`
	// TagsByName is the statistics' tags in alphabetical order (tab).
	TagsByName bool `json:"tagsByName,omitempty"`
}

func sessionPath() string { return filepath.Join(paths.State(), sessionFile) }

func loadSession() session {
	var s session
	body, err := os.ReadFile(sessionPath())
	if err != nil {
		return session{Editing: -1}
	}
	if json.Unmarshal(body, &s) != nil {
		return session{Editing: -1}
	}
	return s
}

// save records the workspace. Failures are silent: losing the session costs
// a few keystrokes tomorrow, and a dialogue about it on the way out would
// cost more.
func (m Model) saveSession() {
	s := session{Focused: m.ws.focused, Editing: -1, LastTag: m.lastTag, TagsByName: m.stats.tagsByName}

	for i, p := range m.ws.panels {
		ps := panelSession{Kind: p.kind.String()}
		switch v := p.top().(type) {
		case *cardList:
			// A local deck comes back; somebody else's doesn't, and a
			// search comes back as the query that produced it.
			switch {
			case v.deck != nil && v.deck.Local():
				ps.Kind = "deck"
				ps.Deck = v.deck.Slug
			case v.deck != nil:
				continue // borrowed, and might not be there tomorrow
			default:
				ps.Kind = "find"
				ps.Query = v.name
			}
			ps.keepLayout(v)
		case *rulesView:
			ps.Kind = "rules"
			ps.Query = v.name
		case *deckList:
			ps.Kind = "decks"
		case nil:
			// A search still in flight has no view yet but does have a
			// query, and that is the part worth keeping. A panel with
			// nothing in it at all is a question you hadn't answered.
			if p.kind == KindFind && p.title != "" {
				ps.Query = p.title
				break
			}
			continue
		default:
			// A sub-view — versions, somebody's decks — comes back as the
			// panel it was reached from.
			ps.Kind = p.kind.String()
		}

		if ps.Kind == "find" {
			ps.QuerySort, ps.QueryDir = p.queryOrder(), p.queryDirection()
		}

		if m.ws.editing == i {
			s.Editing = len(s.Panels)
		}
		s.Panels = append(s.Panels, ps)
	}

	if s.Focused >= len(s.Panels) {
		s.Focused = maxInt(len(s.Panels)-1, 0)
	}

	body, err := json.Marshal(s)
	if err != nil {
		return
	}
	os.WriteFile(sessionPath(), body, 0644)
}

// restore reopens what was there. Each panel that needs fetching hands back
// a command; the workspace is on screen before any of them answer, so it
// comes up at once and fills in.
func (m *Model) restore() tea.Cmd {
	s := loadSession()
	if len(s.Panels) == 0 {
		return nil
	}

	m.lastTag = s.LastTag
	m.stats.tagsByName = s.TagsByName

	var cmds []tea.Cmd
	for _, ps := range s.Panels {
		ps := ps
		switch ps.Kind {
		case "find":
			p := m.ws.open(KindFind)
			if i := indexOf(scryfall.SortOptions, ps.QuerySort); i >= 0 {
				p.querySort = i
			}
			if i := indexOf(scryfall.DirOptions, ps.QueryDir); i >= 0 {
				p.queryDir = i
			}
			if ps.Query != "" {
				p.pending = &ps
				p.search.SetValue(ps.Query)
				cmds = append(cmds, m.search(p))
			}

		case "decks":
			l := newDeckList()
			m.ws.open(KindDecks).show(l)
			cmds = append(cmds, loadDecks(l))

		case "rules":
			p := m.ws.open(KindRules)
			if q := ruleQuery(ps.Query); q != "" {
				cmds = append(cmds, m.searchRules(p, q))
			}

		case "deck":
			// A deck that has since been deleted simply doesn't come back.
			if ps.Deck == "" || !deck.Exists(ps.Deck) {
				continue
			}
			p := m.ws.open(KindDecks)
			p.searchOpen = false
			p.search.Blur()
			p.loading = true
			p.title = ps.Deck
			p.pending = &ps
			cmds = append(cmds, openLocalDeck(p.id, true, ps.Deck))
		}
	}

	if m.ws.count() == 0 {
		return nil
	}
	// Focus first: it re-checks the target, and the target is the thing we
	// are about to restore.
	m.ws.focus(minInt(s.Focused, m.ws.count()-1))
	if s.Editing >= 0 && s.Editing < m.ws.count() {
		m.ws.editing = s.Editing
	}
	return tea.Batch(cmds...)
}

// ruleQuery undoes the "rules: " a rules panel puts on its own title, so a
// restored search is the search rather than the label.
func ruleQuery(title string) string {
	const prefix = "rules: "
	if len(title) > len(prefix) && title[:len(prefix)] == prefix {
		return title[len(prefix):]
	}
	return ""
}

// keepLayout records how a list was ordered and narrowed.
func (ps *panelSession) keepLayout(l *cardList) {
	ps.Sort1, ps.Desc1 = l.order.String(), l.desc1
	if l.order2 != sortArrival {
		ps.Sort2, ps.Desc2 = l.order2.String(), l.desc2
	}
	ps.Filter = l.filter
	for _, cl := range l.statFilter {
		ps.Stats = append(ps.Stats, savedClause{Op: opName(cl.Op), Group: cl.Row.Group, Label: cl.Row.Label})
	}
}

// applyLayout lays a list out the way it was left. It runs when the cards
// arrive, since the categories of a statistics filter can only be found
// again among the cards they describe. An order no longer known, or a
// category the list no longer has, is left out rather than guessed at.
func (ps *panelSession) applyLayout(l *cardList) {
	if s, ok := parseCardSort(ps.Sort1); ok {
		l.order, l.desc1 = s, ps.Desc1
	}
	if s, ok := parseCardSort(ps.Sort2); ok {
		l.order2, l.desc2 = s, ps.Desc2
	}
	l.filter = ps.Filter
	if len(ps.Stats) > 0 {
		groups := stats.Groups(l.all, l.all)
		for _, sc := range ps.Stats {
			if r, ok := findRow(groups, sc.Group, sc.Label); ok {
				l.statFilter, _ = l.statFilter.Add(parseOp(sc.Op), r)
			}
		}
	}
	l.refresh()
}

func findRow(groups []stats.Group, group, label string) (stats.Row, bool) {
	for _, g := range groups {
		for _, r := range g.Rows {
			if r.Group == group && r.Label == label {
				return r, true
			}
		}
	}
	return stats.Row{}, false
}

func opName(o stats.Op) string {
	switch o {
	case stats.Or:
		return "or"
	case stats.AndNot:
		return "not"
	}
	return "and"
}

func parseOp(s string) stats.Op {
	switch s {
	case "or":
		return stats.Or
	case "not":
		return stats.AndNot
	}
	return stats.And
}

func indexOf(options []string, s string) int {
	for i, o := range options {
		if o == s {
			return i
		}
	}
	return -1
}
