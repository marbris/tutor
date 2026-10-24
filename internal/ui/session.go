package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/deck"
	"ttr/internal/paths"
	"ttr/internal/scryfall"
	"ttr/internal/stats"
)

// What `ttr` on its own comes back to.
//
// The workspace is the thing worth restoring: which panels were open, what
// each was showing, how it was sorted and filtered, which deck you were
// building, the card each list's cursor was on, and the statistics column
// as you left it. Not what was picked out with v: a selection you don't
// remember making is a trap for the next a or x.
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
	// Muted is a deck space t stopped lending its tags.
	Muted bool `json:"muted,omitempty"`
	// QuerySort and QueryDir are the order a find panel asks Scryfall for.
	QuerySort string `json:"querySort,omitempty"`
	QueryDir  string `json:"queryDir,omitempty"`
	// Card is the card under the cursor, by name: the cards may come back
	// in another order, or with one gone.
	Card string `json:"card,omitempty"`
}

// statsSession is the statistics column: whether it was up, for which list
// (S: the editing deck), the category highlighted, and counts or odds.
type statsSession struct {
	Open    bool   `json:"open,omitempty"`
	Editing bool   `json:"editing,omitempty"`
	Group   string `json:"group,omitempty"`
	Label   string `json:"label,omitempty"`
	Path    string `json:"path,omitempty"`
	Odds    int    `json:"odds,omitempty"`
}

// savedClause is one statistics category. A category is found again by its
// group and label: the test it filters with is a closure, and can't be kept.
type savedClause struct {
	Op    string `json:"op"`
	Group string `json:"group"`
	Label string `json:"label"`
	// Path is a Scryfall Tagger row's place in the tree.
	Path string `json:"path,omitempty"`
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
	// OpenTags is the Scryfall Tagger rows opened in the statistics.
	OpenTags []string `json:"openTags,omitempty"`
	// OpenGroups is the statistics groups opened from their heading.
	OpenGroups []string `json:"openGroups,omitempty"`
	// Info is the information panel's view, when it is one worth coming
	// back to: "printing" for gx's. The others are put up for a moment:
	// a printed history is about one card. (The statistics have their own.)
	Info string `json:"info,omitempty"`
	// Stats is the statistics column.
	Stats statsSession `json:"stats,omitempty"`
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
	s := session{Editing: -1, LastTag: m.lastTag, TagsByName: m.stats.tagsByName}
	st := m.stats
	s.Stats = statsSession{Open: st.open, Editing: st.editing, Group: st.group, Label: st.label, Path: st.path, Odds: st.odds}
	for path := range m.stats.openTags {
		s.OpenTags = append(s.OpenTags, path)
	}
	sort.Strings(s.OpenTags)
	for title := range m.stats.openGroups {
		s.OpenGroups = append(s.OpenGroups, title)
	}
	sort.Strings(s.OpenGroups)
	if m.info.mode == infoImage || (m.info.mode == infoVersions && m.info.prev == infoImage) {
		s.Info = infoImage.String()
	}

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
				ps.Muted = globalTags.muted[v.deck.Slug]
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
		case *settingsView:
			ps.Kind = "settings"
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

		// By where the panel lands among those saved: a panel left out
		// (somebody's deck, an empty one) mustn't shift the focus or the
		// editing deck onto its neighbour.
		if m.ws.editing == i {
			s.Editing = len(s.Panels)
		}
		if m.ws.focused >= i {
			s.Focused = len(s.Panels)
		}
		s.Panels = append(s.Panels, ps)
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
	if len(s.OpenTags) > 0 {
		m.stats.openTags = map[string]bool{}
		for _, path := range s.OpenTags {
			m.stats.openTags[path] = true
		}
		m.stats.openGen++
	}
	if len(s.OpenGroups) > 0 {
		m.stats.openGroups = map[string]bool{}
		for _, title := range s.OpenGroups {
			m.stats.openGroups[title] = true
		}
		m.stats.foldGen++
	}
	if s.Info == infoImage.String() && kittyGraphics() {
		m.info.mode = infoImage
	}

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
				cmds = append(cmds, m.search(p, false))
			}

		case "decks":
			l := newDeckList()
			m.ws.open(KindDecks).show(l)
			cmds = append(cmds, loadDecks(l))

		case "settings":
			m.openSettings()

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
			if ps.Muted {
				if globalTags.muted == nil {
					globalTags.muted = map[string]bool{}
				}
				globalTags.muted[ps.Deck] = true
			}
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
	// The statistics come back as they were, counting whatever list they
	// were counting once its cards arrive.
	if st := s.Stats; st.Open {
		m.stats.open, m.stats.editing = true, st.Editing
		m.stats.group, m.stats.label, m.stats.path, m.stats.odds = st.Group, st.Label, st.Path, st.Odds
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
	if c, ok := l.current(); ok {
		ps.Card = c.Card.Name
	}
	for _, cl := range l.statFilter {
		ps.Stats = append(ps.Stats, savedClause{Op: opName(cl.Op), Group: cl.Row.Group, Label: cl.Row.Label, Path: cl.Row.Path})
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
			// A Tagger row is found by its path, opened or not, and whether
			// or not the tags are in yet: it asks for them as it matches.
			if sc.Group == stats.TaggerGroup && sc.Path != "" {
				l.statFilter, _ = l.statFilter.Add(parseOp(sc.Op), stats.TaggerRow(sc.Path))
				continue
			}
			// So is a type or subtype, opened or not.
			if sc.Group == stats.TypeGroup && stats.IsTypePath(sc.Path) {
				l.statFilter, _ = l.statFilter.Add(parseOp(sc.Op), stats.TypeRow(sc.Path))
				continue
			}
			// And a tag, or one half of it.
			if sc.Group == "Tags" && stats.IsTagPath(sc.Path) {
				l.statFilter, _ = l.statFilter.Add(parseOp(sc.Op), stats.TagRow(sc.Path))
				continue
			}
			if r, ok := findRow(groups, sc.Group, sc.Label); ok {
				l.statFilter, _ = l.statFilter.Add(parseOp(sc.Op), r)
			}
		}
	}
	l.refresh()
	// A card no longer there, or filtered away, leaves the cursor at the top.
	if ps.Card != "" {
		l.selectByName(ps.Card)
	}
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
