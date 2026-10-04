package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/keymap"
	"ttr/internal/rules"
)

// Keys, in two spaces.
//
// Bare keys act on the contents of the focused panel. The leader acts on the
// panels themselves — making them, closing them, moving them about. That
// split is what lets r mean rename in a list and <space>r open a rules
// panel without either of them being ambiguous, the same way vim has f,
// ctrl+f, gf and zf all meaning different things.
//
// The leader is space. Comma used to be an alias for it; it sorts now, on
// the left-hand one of the two sort keys.
//
// Which key is which comes from internal/keymap, which reads keys.json:
// everything below switches on actions, never on the keys themselves.
//
// It follows from that choice that the leader does nothing inside a search
// bar, where space is a space — the same way vim's leader does nothing in
// insert mode. The bar is insert mode. Since a new panel opens with its bar
// focused, opening a second empty panel means finishing or abandoning the
// first, which is fair: an empty panel is a question you haven't answered.

// leaderCmd is one entry in the menu the leader raises.
type leaderCmd struct {
	action keymap.Action
	what   string
	run    func(*Model)
}

// leaderMenu is the menu, in the order it's shown: making panels, then
// getting rid of them, then moving among them.
var leaderMenu = []leaderCmd{
	{keymap.LeaderFind, "find", func(m *Model) { m.ws.open(KindFind) }},
	{keymap.LeaderDecks, "decks", nil}, // opens the list and checks it, so it needs a command
	{keymap.LeaderRules, "rules", nil}, // needs a command, so it is run below
	{keymap.LeaderNew, "new", func(m *Model) { m.ws.open(KindNew) }},
	{keymap.LeaderSync, "git push", nil},              // hands back a command, so it is run below
	{keymap.LeaderCommitAll, "commit all lists", nil}, // so does this
	{keymap.LeaderSettings, "settings", func(m *Model) { m.openSettings() }},
	{keymap.LeaderClose, "close", func(m *Model) { m.ws.close() }},
	{keymap.LeaderUndoClose, "undo close", func(m *Model) { m.ws.restoreClosed() }},
	{keymap.LeaderOnly, "only", func(m *Model) { m.ws.only() }},
}

// handleLeader runs the command a key names. A key that names nothing
// cancels, rather than doing something surprising with a near miss.
func (m *Model) handleLeader(key string) tea.Cmd {
	m.leader = false
	action := keymap.Lookup(keymap.Leader, key)

	// A few entries hand back a command rather than only changing the
	// workspace, so they can't sit in the table above.
	switch action {
	case keymap.LeaderRules:
		return m.openRules()
	case keymap.LeaderDecks:
		l := newDeckList()
		m.ws.open(KindDecks).show(l)
		return loadDecks(l)
	case keymap.LeaderSync:
		// Mirror to the git remote, from wherever you are: it is about all
		// your decks, not the one under the cursor. It touches the network,
		// so it runs off the main thread and reports back as a notice.
		return syncDecks
	case keymap.LeaderCommitAll:
		return m.commitAll()
	}

	for _, c := range leaderMenu {
		if action != "" && c.action == action && c.run != nil {
			c.run(m)
			return nil
		}
	}
	return nil
}

// handleKey routes a keypress: the leader first, then whatever the focused
// panel's search bar wants, then the workspace's own keys.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.goPrefix {
		m.goPrefix = false
		return m.handleGoto(key)
	}
	if m.tagPrefix {
		m.tagPrefix = false
		return m.handleTagMove(key)
	}

	// The unsaved-changes question takes every key until it's answered.
	if m.quitting {
		m.quitting = false
		switch key {
		case "y":
			return m, m.quit()
		case "w":
			cmd := m.saveEverything()
			return m, cmd
		}
		return m, nil
	}

	if m.leader {
		// Sequenced rather than returned inline. This is the shape to watch
		// for throughout: the call mutates m, and `return m, m.f()` leaves
		// the compiler free to copy m for the return value before f runs.
		// Every call of that shape in this package is split in two.
		cmd := m.handleLeader(key)
		return m, cmd
	}

	// With nothing open, the leader is the only way forward, so the splash
	// takes a couple of shortcuts to it.
	if m.ws.empty() {
		if key == "ctrl+c" {
			return m.tryQuit()
		}
		switch keymap.Lookup(keymap.Global, key) {
		case keymap.GlobalLeader:
			m.leader = true
		case keymap.GlobalQuit, keymap.GlobalBack:
			return m.tryQuit()
		case keymap.GlobalHelp:
			m.hintsExpanded = !m.hintsExpanded
		}
		return m, nil
	}

	p := m.ws.current()

	// A prompt is a text field, and takes the keys while it's open.
	if p.asking != askNone {
		return m.handleAskKey(msg)
	}

	// The filter prompt is a text field too, and takes precedence over the
	// list's keys while it's open.
	if p.filtering {
		return m.handleFilterKey(msg)
	}

	// A focused search bar is a text field first: it gets the printable
	// keys, and the leader with them, or you could never type a space.
	if p.searchOpen && p.search.Focused() {
		return m.handleSearchKey(msg)
	}

	global := keymap.Lookup(keymap.Global, key)

	// y in the printed-text panel is the go-ahead to download the sets that
	// aren't cached. It has to be claimed before the view sees it, because
	// a card list takes y for yank.
	if global == keymap.GlobalFetchSets && m.info.mode == infoVersions {
		if c := m.focusedCard(); c != nil {
			if h, ok := m.histories[c.OracleID]; ok && h.state == histWaiting {
				cmd := m.fetchAllSets(h)
				return m, cmd
			}
		}
	}

	// g is a prefix everywhere and never a key on its own, so it has to be
	// claimed before any view sees it — a list would otherwise take it for
	// "go to the top" and gd and gv could never be typed. gg still means
	// the top: handleGoto passes the second g back down.
	if global == keymap.GlobalGoto {
		m.goPrefix = true
		return m, nil
	}

	// While the statistics are up they have first claim on the keys: j and
	// k walk the bars, not the cards. What they don't claim goes on to the
	// list behind them and then the workspace, as it would without them.
	// Moved onto something that isn't a list of cards, there's nothing to
	// count, and the view there has its keys back — all but s, which still
	// closes.
	statsUp := m.info.mode == infoStats &&
		(m.statList() != nil || keymap.Lookup(keymap.Stats, key) == keymap.StatsClose)
	if statsUp && m.statsKey(key) {
		return m, nil
	}
	// What the statistics leave alone still reaches the list behind them —
	// v, y, t and / work on the cards while the bars are up.
	if v := p.top(); v != nil {
		// The view has first refusal on anything that isn't the workspace's.
		if handled, cmd := v.key(key, &m, p); handled {
			// The cursor may have moved, so start the clock on whatever is
			// under it now.
			hover := m.hover()
			return m, tea.Batch(cmd, hover)
		}
	}

	if key == "ctrl+c" {
		return m.tryQuit()
	}

	switch global {
	case keymap.GlobalLeader:
		m.leader = true

	case keymap.GlobalPanelPrev:
		m.ws.step(-1)
		cmd := m.hover()
		return m, cmd
	case keymap.GlobalPanelNext:
		m.ws.step(1)
		cmd := m.hover()
		return m, cmd

	// ctrl with a direction carries the panel itself along the row, focus
	// going with it, on the keys your fingers are already on for moving
	// between panels — and, unlike a leader sequence, it repeats.
	case keymap.GlobalMoveLeft:
		m.ws.movePanel(-1)
	case keymap.GlobalMoveRight:
		m.ws.movePanel(1)

	case keymap.GlobalBar:
		// On a deck, i fetches a card into it: the panel's own bar would
		// follow a Moxfield user, which is the decks panel's business and
		// not something you reach for from inside a deck.
		if p.kind == KindSettings {
			return m, nil // nothing to search here
		}
		if l := p.cardsView(); l != nil && l.deck != nil {
			if !l.deck.Local() {
				m.notice = "that deck isn't yours — " + keymap.Hint(keymap.Cards, keymap.CardsWrite) + " takes a copy you can add to"
				return m, nil
			}
			p.askAdd("")
			return m, nil
		}
		// The bar keeps the query that produced what's on screen, so i is
		// "edit this search" rather than "start again" — with the cursor
		// where you'd carry on typing.
		p.searchOpen = true
		p.search.Focus()
		p.search.CursorEnd()

	case keymap.GlobalEditNext:
		m.ws.cycleEditing(1)
	case keymap.GlobalEditPrev:
		m.ws.cycleEditing(-1)

	case keymap.GlobalInfoUp:
		m.scrollInfoHalf(-1)
	case keymap.GlobalInfoDown:
		m.scrollInfoHalf(1)
	case keymap.GlobalPrintingOlder:
		return m, m.stepPrinting(true)
	case keymap.GlobalPrintingNewer:
		return m, m.stepPrinting(false)
	case keymap.GlobalPrintingFace:
		return m, m.flipPrinting()
	case keymap.GlobalStats:
		m.toggleStats(false)
	case keymap.GlobalStatsEdit:
		m.toggleStats(true)

	case keymap.GlobalHelp:
		// Grow the hint bar to the whole keymap, or shrink it back. It stays
		// where you put it rather than closing on the next key, so you can
		// read it and act at the same time.
		m.hintsExpanded = !m.hintsExpanded

	case keymap.GlobalBack:
		_, run := (&m).escStep(p)
		run()

	case keymap.GlobalClearFilter:
		// Clear the narrowings on the list in front of you — the text filter
		// and the statistics categories both — at once, where esc takes them
		// a step at a time.
		m.clearActiveFilters()
	case keymap.GlobalClearAll:
		m.clearAllFilters()

	case keymap.GlobalQuit:
		return m.tryQuit()
	}
	return m, nil
}

// escStep is what esc will do next, and the doing of it. One function for
// both, so the hint naming the next step and the key taking it can't come
// apart.
//
// The cascade, outward one step at a time — esc is "back": step off the
// information panel's modes, drop a transient selection, then the
// narrowings one at a time — the text filter, the statistics filter — then
// step back out of a sub-view, and close the panel. Closing the last one
// lands on the splash rather than quitting — esc *from* the splash is what
// leaves.
func (m *Model) escStep(p *panel) (string, func()) {
	switch {
	// The picture gx put up comes off the same way.
	case m.info.mode == infoImage:
		return "close printing", func() {
			m.info.mode = infoCard
			m.info.offset = 0
		}
	// A printed history was put on the information panel from here, so it
	// comes off from here too, before esc starts taking the panel itself
	// apart.
	case m.info.mode == infoVersions:
		return "close printed text", func() {
			m.info.mode = infoCard
			m.info.offset = 0
		}
	case p.cardsView() != nil && p.cardsView().markCount() > 0:
		return "drop picks", func() { p.top().clear() }
	case func() bool { v, ok := p.top().(filterable); return ok && v.filterText() != "" }():
		return "clear /filter", func() { p.clearFilter() }
	case p.cardsView() != nil && len(p.cardsView().statFilter) > 0:
		return "clear stats-filter", func() { clearStatFilter(p.cardsView()) }
	case len(p.stack) > 1:
		label := "back"
		if _, ok := p.stack[len(p.stack)-2].(*deckList); ok {
			label = "back to decks"
		}
		return label, func() { p.pop() }
	}
	return "close panel", func() { m.ws.close() }
}

// handleSearchKey is the search bar's own keymap. Everything it doesn't
// claim goes into the input as text.
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.ws.current()

	key := msg.String()
	if key == "ctrl+c" {
		return m, m.quit()
	}

	switch keymap.Lookup(keymap.Search, key) {
	// tab finishes an oracle tag being typed — otag:remo — and otherwise
	// changes what the bar searches.
	case keymap.SearchNextTarget:
		if m.canCompleteOtag(p) {
			m.completeOtag(p, 1)
			return m, nil
		}
		p.setKind(p.kind.next(1))
		return m, m.previewKind(p)
	case keymap.SearchPrevTarget:
		if m.canCompleteOtag(p) {
			m.completeOtag(p, -1)
			return m, nil
		}
		p.setKind(p.kind.next(-1))
		return m, m.previewKind(p)

	case keymap.SearchBack:
		// The same cascade as everywhere else: clear what's clearable, then
		// leave, then close. Typing half a query and pressing esc should
		// lose the half-query, not the panel.
		if p.search.Value() != "" {
			p.search.SetValue("")
			return m, nil
		}
		if p.empty() {
			// Closing the last panel lands on the splash rather than
			// quitting. Leaving the program is what esc does *from* the
			// splash, so there is always one press between you and the exit.
			m.ws.close()
			return m, nil
		}
		// Leaving the bar over a tab-preview commits it: the decks it was
		// showing become the panel's content rather than vanishing.
		if p.previewing {
			p.previewing = false
			p.title = p.top().title()
		}
		p.searchOpen = false
		p.search.Blur()
		return m, nil

	case keymap.SearchRun:
		if p.kind == KindFind {
			cmd := m.search(p)
			return m, cmd
		}
		if p.kind == KindRules {
			q := p.search.Value()
			if q == "" {
				return m, nil
			}
			cmd := m.searchRules(p, q)
			return m, cmd
		}
		if p.kind == KindDecks {
			// The decks panel's bar follows a Moxfield deck or a person
			// rather than searching: what you can already see is filtered
			// with /, and what you can't is somewhere else entirely.
			q := p.search.Value()
			p.search.SetValue("")
			p.searchOpen = false
			p.search.Blur()
			var cmd tea.Cmd
			if p.top() == nil {
				l := newDeckList()
				p.show(l)
				cmd = loadDecks(l)
			} else {
				// A tab-preview committed by pressing enter: keep the decks
				// already on screen rather than fetching them again.
				p.previewing = false
				p.title = p.top().title()
			}
			// An empty bar was only a way of choosing the target, so enter
			// commits the decks list without trying to follow anything.
			if q == "" {
				return m, cmd
			}
			return m, follow(q)
		}
		// The other kinds get their own bar in the phases that build them.
		if q := p.search.Value(); q != "" {
			p.title = p.kind.String() + ": " + q
			p.searchOpen = false
			p.search.Blur()
		}
		return m, nil

	case keymap.SearchHistoryPrev:
		p.recall(-1, m.queryHistory(p))
		return m, nil
	case keymap.SearchHistoryNext:
		p.recall(1, m.queryHistory(p))
		return m, nil

	case keymap.SearchQuerySort:
		p.cycleQuerySort(1)
		return m, nil
	case keymap.SearchQueryDir:
		p.cycleQueryDir(1)
		return m, nil

	}

	// Anything else is typing, which ends a walk through the history: what
	// is in the bar is yours again rather than something recalled.
	before := p.search.Value()
	var cmd tea.Cmd
	p.search, cmd = p.search.Update(msg)
	if p.search.Value() != before {
		p.leaveHistory()
	}
	return m, cmd
}

// previewKind fills the body with what the newly chosen target will show,
// so tabbing to the decks target lists your decks straight away rather than
// leaving a blank panel until enter. The bar stays open over the preview.
//
// Only a preview is ours to replace. A panel already holding a real search
// or deck — reached by reopening its bar with i — keeps it: tab there is
// only retargeting the bar, not throwing away what you found.
func (m *Model) previewKind(p *panel) tea.Cmd {
	if p.previewing {
		p.stack = nil
		p.previewing = false
	}
	if p.top() != nil {
		return nil
	}
	if p.kind == KindDecks {
		l := newDeckList()
		p.stack = []view{l}
		p.previewing = true
		return loadDecks(l)
	}
	return nil
}

// handleFilterKey is the / prompt. It narrows as you type, so you can see
// what you're doing rather than committing blind.
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.ws.current()

	switch msg.String() {
	case "enter":
		p.filtering = false
		p.filterInput.Blur()
		return m, nil

	case "esc":
		// Abandoning the prompt puts back whatever was filtered before it
		// opened, rather than leaving a half-typed narrowing in place.
		p.filtering = false
		p.filterInput.Blur()
		p.setFilter(p.filterBefore)
		return m, nil

	case "ctrl+c":
		return m, m.quit()
	}

	var cmd tea.Cmd
	p.filterInput, cmd = p.filterInput.Update(msg)
	p.setFilter(p.filterInput.Value())
	return m, cmd
}

// handleGoto runs the g-prefixed keys.
func (m Model) handleGoto(key string) (tea.Model, tea.Cmd) {
	p := m.ws.current()
	if p == nil {
		return m, nil
	}

	switch keymap.Lookup(keymap.Goto, key) {
	case keymap.GotoTop:
		if v := p.top(); v != nil {
			v.key(toTop, &m, p) // the views' own "to the top"
		}

	case keymap.GotoEditing:
		// Straight to the deck being edited, from wherever you are.
		if m.ws.editing >= 0 {
			m.ws.focus(m.ws.editing)
		}

	case keymap.GotoVersions:
		cmd := m.versions(p)
		return m, cmd

	case keymap.GotoImage:
		cmd := m.gx(p)
		return m, cmd

	case keymap.GotoImageAll:
		cmd := m.gxAll(p)
		return m, cmd
	}
	return m, nil
}

// gx shows whatever is under the cursor as it looks: a card's printing in
// the information panel, a deck of somebody else's on Moxfield.
func (m *Model) gx(p *panel) tea.Cmd {
	switch v := p.top().(type) {
	case *cardList:
		if c, ok := v.current(); ok {
			return m.gxCard(c.Card)
		}
	case *deckList:
		e, ok := v.current()
		if !ok || e.id == "" || (e.kind != entryRemote && e.kind != entryUserDeck) {
			m.notice = "gx opens a Moxfield deck — this one is only here"
			return nil
		}
		return openInBrowser("https://moxfield.com/decks/"+e.id, e.name)
	}
	return nil
}

// versions opens the history of whatever is under the cursor: a deck's
// commits here, a card's printed wordings when the information panel learns
// to show them.
func (m *Model) versions(p *panel) tea.Cmd {
	switch v := p.top().(type) {
	case *deckList:
		e, ok := v.current()
		if !ok || e.kind != entryLocal {
			return nil
		}
		p.loading = true
		return loadVersions(p.id, e.slug, e.name)

	case *cardList:
		// A card row is a card, whatever list it is in. The cursor is on a
		// card, so gv is about that card — a deck's own versions belong to
		// the deck's row in the decks panel, where the cursor is on a deck.
		if c, ok := v.current(); ok {
			return m.openHistory(c.Card)
		}

	case *rulesView:
		// The rules have no per-item history; gv here is the diff between the
		// cached release and the one before it, opened on the rule you were on.
		if !rules.HasPrevious() {
			return func() tea.Msg {
				return noticeMsg{text: "no previous rules to compare — sync first"}
			}
		}
		focus := ""
		if r, ok := v.current(); ok {
			focus = r.number
		}
		p.loading = true
		return loadRulesDiff(p.id, focus)
	}
	return nil
}

// tryQuit leaves, unless a deck has edits that were never committed. They're
// written, so nothing is lost either way; but committing is explicit, and a
// history with a gap in it is worth one question on the way out.
func (m Model) tryQuit() (tea.Model, tea.Cmd) {
	if len(m.dirtyDecks()) == 0 {
		return m, m.quit()
	}
	m.quitting = true
	return m, nil
}

// saveEverything commits every deck with uncommitted edits, for the w in
// the quit question and for <space>w.
func (m Model) saveEverything() tea.Cmd {
	var cmds []tea.Cmd
	for _, d := range m.openDecks() {
		if d.list.dirty && d.list.deck.Local() {
			cmds = append(cmds, saveDeck(d.panel.id, *d.list.deck, d.list.all))
		}
	}
	return tea.Batch(cmds...)
}

// commitAll is <space>w: every open list with uncommitted edits, committed —
// the edits a/x/t made to one deck and the cards you put into another alike.
func (m *Model) commitAll() tea.Cmd {
	cmd := m.saveEverything()
	if cmd == nil {
		m.notice = "nothing to commit"
	}
	return cmd
}

// openDeck is a deck open somewhere in the workspace, and the panel it's in.
type openDeck struct {
	panel *panel
	list  *cardList
}

// openDecks is every deck open in any panel — not only the ones on top: a
// deck opened from the decks list and stepped back out of is still open,
// and can still have edits nobody has committed.
func (m Model) openDecks() []openDeck {
	var out []openDeck
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			if l, ok := v.(*cardList); ok && l.deck != nil {
				out = append(out, openDeck{p, l})
			}
		}
	}
	return out
}
