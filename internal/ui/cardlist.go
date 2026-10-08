package ui

import (
	"strings"

	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/mtg"
	"ttr/internal/query"
	"ttr/internal/rules"
	"ttr/internal/stats"

	tea "github.com/charmbracelet/bubbletea"
)

// A list of cards in a panel: what's in it, how it's ordered, what's been
// narrowed away and what's been picked out.
//
// Search results and decks are the same thing here. A search result is a
// deck card with no quantity and no tags, which is what it is, and having
// one type means every key that works on a list works on both.

type cardList struct {
	// all is every card, in the order it arrived. Nothing reorders this;
	// the sort and the filter build rows from it, so both can be undone.
	all []deck.Card
	// gen counts refreshes. Everything that changes the cards or their
	// narrowing goes through refresh, so the statistics, kept between
	// frames, know by it when to count again.
	gen   int
	order cardSort
	// order2 breaks order's ties and colours the names — , and <. Arrival
	// means there is none.
	order2 cardSort
	// desc1 and desc2 are which way each order runs. Each starts the way
	// its order reads best, and alt+. and alt+, turn them round.
	desc1, desc2 bool
	// members is where else on screen each card is, for the inclusion
	// order. The workspace keeps it current; see Model.resortInclusion.
	members map[string]membership

	// rows is what's on screen: all, narrowed and sorted.
	rows []deck.Card

	cursor // where you are in rows, and how far it has scrolled

	// statFilter is the statistics categories the list is narrowed to,
	// added from the bars in the information panel. Separate from the text
	// filter because they compose: filter to "elf", then narrow to lands.
	statFilter stats.Expr

	// filter is a literal narrowing. Terms are substrings, all of them have
	// to appear, and they're matched against the name and the rules text —
	// which is the one thing a fuzzy match makes useless, since over a
	// hundred cards of oracle text almost anything matches something.
	filter string

	// marks are the cards picked out with v, by lowercased name so they
	// survive the list being filtered or re-sorted underneath them.
	marks map[string]bool

	// name is what the header calls this list, and matched is how many
	// cards the query behind it found — usually more than were fetched.
	name    string
	matched int

	// deck is set when this list is a deck rather than a search result. A
	// local one can be edited; a borrowed one can't.
	deck *deck.Info

	// rules and rulings are what the information panel needs to describe a
	// card properly. They arrive after the cards do, and the panel simply
	// shows less until they have.
	rules     rules.Data
	rulings   map[string][]mtg.Ruling
	rulingErr map[string]error

	// legality is the verdict on this deck, when it is one and someone has
	// worked it out. Recomputed as you edit, since an edit is exactly what
	// changes the answer.
	legality *deck.Legality

	// dirty means there are edits not yet committed. Every edit is written
	// to the file straight away, so nothing is lost by quitting; w is what
	// records the change in the deck's history, and quitting asks about it.
	dirty bool
	// unwritten is an edit the file hasn't had yet — picked up after the
	// key that made it, and written off the main thread.
	unwritten bool
	// wasClean is set by the first edit since the last commit, which is
	// when edits made in another editor get committed before ttr writes
	// over them.
	wasClean bool
	// tagEdits counts edits, so the global tags know a list they borrow
	// from has changed.
	tagEdits int
	// undo holds the deck as it stood before each edit.
	undo []undoStep

	// arrivalName is what to call the order the cards came in — "as found"
	// says nothing, where "scryfall order" and "decklist" say what you're
	// looking at.
	arrivalName string
}

func newCardList(cards []deck.Card, order cardSort, arrivalName string) *cardList {
	if arrivalName == "" {
		arrivalName = "as found"
	}
	l := &cardList{
		all: cards, order: order, desc1: order.descending(), marks: map[string]bool{}, arrivalName: arrivalName,
		rulings: map[string][]mtg.Ruling{}, rulingErr: map[string]error{},
	}
	l.refresh()
	return l
}

// recheck works out this deck's legality again, from the cards in hand.
// Called after every edit: adding a card is the thing that makes a deck
// illegal, so the answer has to keep up.
func (l *cardList) recheck() {
	if l.deck == nil {
		return
	}
	// The cards are already resolved — they're on screen — so this needs
	// nothing fetched and can happen on every keystroke.
	verdict := deck.Check(l.deck.Format, l.all, true)
	l.legality = &verdict
}

// orderName is what the panel calls its current order, with the way it
// runs.
func (l *cardList) orderName() string {
	name := l.order.String()
	if l.order == sortArrival {
		name = l.arrivalName
	}
	return name + dirArrow(l.desc1)
}

// dirArrow is which way an order runs, as the header shows it: ↑ for
// ascending, ↓ for descending — the same arrows the query's order wears.
func dirArrow(desc bool) string {
	if desc {
		return " ↓"
	}
	return " ↑"
}

// spec is how this list is ordered, for sortCards.
func (l *cardList) spec() sortSpec {
	return sortSpec{first: l.order, then: l.order2, desc1: l.desc1, desc2: l.desc2, members: l.members}
}

// sortsBy reports whether either of the list's orders is s.
func (l *cardList) sortsBy(s cardSort) bool {
	return l.order == s || l.order2 == s
}

// refresh rebuilds what's on screen. Everything that changes the list goes
// through here, so the filter and the sort can't be applied in the wrong
// order or one of them forgotten.
func (l *cardList) refresh() {
	l.gen++
	rows := l.narrowed()
	l.rows = sortCards(rows, l.spec())
	l.cursor.clamp(len(l.rows))
}

// narrowed is the cards after both narrowings, before sorting. The
// statistics count this, so what the bars say and what the list shows can
// never disagree.
func (l *cardList) narrowed() []deck.Card {
	rows := l.all
	if len(l.statFilter) > 0 {
		kept := make([]deck.Card, 0, len(rows))
		for _, c := range rows {
			if l.statFilter.Match(effective(c, l.lenderKey())) {
				kept = append(kept, c)
			}
		}
		rows = kept
	}
	if q := query.Parse(l.filter); !q.Empty() {
		kept := make([]deck.Card, 0, len(rows))
		for _, c := range rows {
			if q.Match(effective(c, l.lenderKey())) {
				kept = append(kept, c)
			}
		}
		rows = kept
	}
	return rows
}

func (l *cardList) count() int  { return len(l.rows) }
func (l *cardList) total() int  { return len(l.all) }
func (l *cardList) empty() bool { return len(l.all) == 0 }

// cardCount and cardTotal sum quantities rather than rows: a row standing for
// 20 lands is 20 cards, not one. This is what the header reports, since "how
// many cards" is the question a deck's size answers. A search result carries
// no quantity but is still one card, so both floor a row at one — which leaves
// search totals unchanged, where every row is a single card anyway.
func (l *cardList) cardCount() int { return sumCopies(l.rows) }
func (l *cardList) cardTotal() int { return sumCopies(l.all) }

func sumCopies(cards []deck.Card) int {
	n := 0
	for _, c := range cards {
		if c.Qty < 1 {
			n++
		} else {
			n += c.Qty
		}
	}
	return n
}

// current is the card under the cursor.
func (l *cardList) current() (deck.Card, bool) {
	if l.cursor.at < 0 || l.cursor.at >= len(l.rows) {
		return deck.Card{}, false
	}
	return l.rows[l.cursor.at], true
}

// ── Moving ──────────────────────────────────────────────────────

func (l *cardList) move(delta int) { l.cursor.move(delta, len(l.rows)) }
func (l *cardList) top()           { l.cursor.top() }
func (l *cardList) bottom()        { l.cursor.bottom(len(l.rows)) }
func (l *cardList) clampCursor()   { l.cursor.clamp(len(l.rows)) }

// ── Ordering and narrowing ──────────────────────────────────────

// cycleSort reorders the list, keeping the cursor on the card it was on.
// Sorting moves the rows, not what you were looking at.
func (l *cardList) cycleSort(delta int) {
	on, had := l.current()
	l.order = l.order.next(delta)
	l.desc1 = l.order.descending()
	l.refresh()
	if had {
		l.selectByName(on.Card.Name)
	}
}

// cycleSort2 changes the second order the same way, which moves cards only
// within the first order's groups — and repaints every name.
func (l *cardList) cycleSort2(delta int) {
	on, had := l.current()
	l.order2 = l.order2.next(delta)
	l.desc2 = l.order2.descending()
	l.refresh()
	if had {
		l.selectByName(on.Card.Name)
	}
}

// flipSort turns one of the two orders round — the first with first set,
// otherwise the second — keeping the cursor on its card.
func (l *cardList) flipSort(first bool) {
	on, had := l.current()
	if first {
		l.desc1 = !l.desc1
	} else {
		l.desc2 = !l.desc2
	}
	l.refresh()
	if had {
		l.selectByName(on.Card.Name)
	}
}

// keepSorts takes another list's orders and their directions, so a new
// search comes back the way the last one was laid out.
func (l *cardList) keepSorts(from *cardList) {
	l.order, l.order2 = from.order, from.order2
	l.desc1, l.desc2 = from.desc1, from.desc2
	l.refresh()
}

// order2Name is what the header calls the second order, or nothing when
// there isn't one.
func (l *cardList) order2Name() string {
	if l.order2 == sortArrival {
		return ""
	}
	return l.order2.String() + dirArrow(l.desc2)
}

func (l *cardList) filterText() string { return l.filter }

func (l *cardList) setFilter(s string) {
	on, had := l.current()
	l.filter = s
	l.refresh()
	// Stay on the same card if it survived the narrowing; otherwise start
	// at the top of what's left.
	if had && !l.selectByName(on.Card.Name) {
		l.cursor.at, l.cursor.offset = 0, 0
	}
}

func (l *cardList) selectByName(name string) bool {
	for i, c := range l.rows {
		if c.Card.Name == name {
			l.cursor.at = i
			return true
		}
	}
	return false
}

// ── Picking cards out ───────────────────────────────────────────

func markKey(c deck.Card) string { return strings.ToLower(c.Card.Name) }

func (l *cardList) marked(c deck.Card) bool { return l.marks[markKey(c)] }

func (l *cardList) markCount() int { return len(l.marks) }

// toggleMark picks the card under the cursor out, or puts it back, and
// steps down — so v v v takes three in a row.
func (l *cardList) toggleMark() {
	c, ok := l.current()
	if !ok {
		return
	}
	key := markKey(c)
	if l.marks[key] {
		delete(l.marks, key)
	} else {
		l.marks[key] = true
	}
	l.move(1)
}

// markAll takes everything the filter has left on screen, or lets it all go
// if it already has. Narrowing to a word and pressing V is the fast way to
// pick out a theme.
func (l *cardList) markAll() {
	all := true
	for _, c := range l.rows {
		if !l.marks[markKey(c)] {
			all = false
			break
		}
	}
	for _, c := range l.rows {
		if all {
			delete(l.marks, markKey(c))
		} else {
			l.marks[markKey(c)] = true
		}
	}
}

func (l *cardList) clearMarks() { l.marks = map[string]bool{} }

// selection is the cards picked out, or — with nothing picked out — the one
// under the cursor. Every command that acts on "the selected cards" goes
// through here, so none of them has to decide what to do with an empty
// selection.
func (l *cardList) selection() []deck.Card {
	if len(l.marks) == 0 {
		if c, ok := l.current(); ok {
			return []deck.Card{c}
		}
		return nil
	}
	var out []deck.Card
	for _, c := range l.all {
		if l.marks[markKey(c)] {
			out = append(out, c)
		}
	}
	return out
}

// ── Filtering ───────────────────────────────────────────────────

// filterTerms splits a filter into the substrings that all have to match.
// Quoting keeps a phrase together: `"first strike"` is one term, where
// first strike would be two.
func filterTerms(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}

	var terms []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				terms = append(terms, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		terms = append(terms, cur.String())
	}
	return terms
}

// ── Drawing ─────────────────────────────────────────────────────

// render draws the visible rows. members says which cards to flag as living
// in the editing deck as well as here.
func (l *cardList) render(width, height int, members map[string]membership, focused bool) []string {
	l.cursor.scrollInto(height, len(l.rows))

	// When the column beside the name is a type line it can be long enough to
	// crowd the names, so the whole list shares one name-column width and
	// abbreviates on that boundary — no ragged overlap between the two.
	nameCol := 0
	if l.order.abbreviates() {
		nameCol = nameColumnFor(l.rows, l.order, width)
	}

	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.rows) && len(lines) < height; i++ {
		c := l.rows[i]
		lines = append(lines, renderRowCol(c, l.order, rowState{
			selected: l.marked(c),
			member:   members[markKey(c)],
			cursor:   focused && i == l.cursor.at,
			then:     l.order2,
		}, width, nameCol))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// names is every card in the list by lowercased name, for the panel beside
// it to flag what they have in common.
func (l *cardList) names() map[string]bool {
	out := make(map[string]bool, len(l.all))
	for _, c := range l.all {
		out[markKey(c)] = true
	}
	return out
}

// ── As a view ───────────────────────────────────────────────────

func (l *cardList) title() string { return l.name }

func (l *cardList) subtitle() string {
	out := l.countText() + " · " + l.orderName()
	if o := l.order2Name(); o != "" {
		out += " · " + o
	}
	for _, seg := range l.narrowingSegments() {
		out += " · " + seg
	}
	return out
}

// countText is how many cards are showing, over how many there are.
func (l *cardList) countText() string {
	out := itoa(l.cardCount())
	if l.count() != l.total() {
		out += "/" + itoa(l.cardTotal())
	} else if l.matched > l.total() {
		// Scryfall matched more than one page; say so, or 175 looks like
		// the whole answer.
		out += "/" + itoa(l.matched)
	}
	return out
}

// narrowingSegments is what has been picked out and narrowed away: the two
// narrowings the rows can't show for themselves. A panel filtered down to a
// few cards otherwise looks like a short search, and a stat filter set from
// the panel beside it leaves no mark here at all.
func (l *cardList) narrowingSegments() []string {
	var out []string
	if n := l.markCount(); n > 0 {
		out = append(out, itoa(n)+" picked")
	}
	if l.filter != "" {
		out = append(out, "/"+l.filter)
	}
	if len(l.statFilter) > 0 {
		out = append(out, "["+l.statFilter.String()+"]")
	}
	return out
}

// headerRows is the header under the list's name, a row per kind of fact:
// the deck's state, then where you are and how it is ordered, then how it
// has been narrowed. Rows are wrapped, never cut — see panel.subLines.
//
// queryOrder is the order the Scryfall request asked for, for a list that
// is a search result; a deck came from no request and leaves it empty.
func (l *cardList) headerRows(queryOrder string) [][]string {
	var state []string
	if l.deck != nil && l.deck.Local() {
		if l.dirty {
			state = append(state, "uncommitted")
		} else {
			state = append(state, "committed")
		}
	}
	if key := l.lenderKey(); key != "" && !globalTags.lends(key) {
		state = append(state, "not lending tags")
	}
	if l.legality != nil && l.legality.Known {
		if l.legality.Legal {
			state = append(state, "legal")
		} else {
			state = append(state, "illegal")
		}
	}

	// [2]20/150: the second row from the top, twenty showing of 150.
	count := l.countText()
	if len(l.rows) > 0 {
		count = "[" + itoa(l.cursor.at+1) + "]" + count
	}
	where := []string{count}
	if queryOrder != "" {
		where = append(where, queryOrder)
	}
	// Sort 2 before sort 1, the way the columns they colour sit: sort 2
	// colours the names on the left, sort 1 fills the column on the right.
	if o := l.order2Name(); o != "" {
		where = append(where, o)
	}
	where = append(where, l.orderName())

	return [][]string{state, where, l.narrowingSegments()}
}

func (l *cardList) lines(width, height int, focused bool, m *Model) []string {
	if l.count() == 0 {
		what := "nothing matches"
		if l.total() == 0 {
			what = "no results"
		}
		return fillTo([]string{mutedLine(what, width)}, width, height)
	}
	return l.render(width, height, m.membersFor(l), focused)
}

func (l *cardList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	if l.cursor.navKey(k, len(l.rows)) {
		return true, nil
	}
	switch keymap.Lookup(keymap.Cards, k) {
	case keymap.CardsSort1Next:
		l.cycleSort(1)
	case keymap.CardsSort1Prev:
		l.cycleSort(-1)
	case keymap.CardsSort2Next:
		l.cycleSort2(1)
	case keymap.CardsSort2Prev:
		l.cycleSort2(-1)
	case keymap.CardsSort1Dir:
		l.flipSort(true)
	case keymap.CardsSort2Dir:
		l.flipSort(false)
	case keymap.CardsSelect:
		l.toggleMark()
	case keymap.CardsSelectAll:
		l.markAll()
	case keymap.CardsFilter:
		p.openFilter(l.filter)
		// A list of cards takes some of Scryfall's syntax (internal/query).
		p.filterInput.Placeholder = "words, or t:creature mv<=3 c:rg otag:ramp tag:wincon -t:land …"
	case keymap.CardsFilterAll:
		m.openFilterAll(p, l)

	// ── Editing ─────────────────────────────────────────────────

	case keymap.CardsAdd:
		m.add(l.selection())
	case keymap.CardsRemove:
		m.remove(l.selection())
	case keymap.CardsYank:
		m.yank(l.selection())
		l.clearMarks()
	case keymap.CardsPut:
		m.put(l)
	case keymap.CardsTag:
		p.ask(askTag, "tag", "")
	case keymap.CardsAddTagged:
		// Add and tag, the last tag filled in. It lives on A, beside a for
		// add, because it is an add that also tags — not a second kind of tag.
		if _, why := m.editTarget(); why != "" {
			m.notice = why
			break
		}
		p.ask(askAddTag, "add + tag", m.lastTag)
		p.askInput.Placeholder = "ramp -draw … · tab completes · empty: just add"
	case keymap.CardsCommander:
		return true, m.commander(currentOr(l))
	case keymap.CardsUndo:
		m.undo()

	case keymap.CardsWrite:
		return true, m.write(l, p, false)
	case keymap.CardsWriteNew:
		return true, m.write(l, p, true)

	case keymap.CardsTagMove:
		m.tagPrefix = true

	default:
		return false, nil
	}
	return true, nil
}

// currentOr is the card c acts on: the one under the cursor. Unlike the
// others, setting a commander is about one card — a deck with four of them
// is a mistake you'd have to mean.
func currentOr(l *cardList) deck.Card {
	c, _ := l.current()
	return c
}

// clear drops the transient selection for esc — the cards picked out with v.
// A filter is not transient and outlives the step back: b clears that, so esc
// can stay the way out.
func (l *cardList) clear() bool {
	if l.markCount() > 0 {
		l.clearMarks()
		return true
	}
	return false
}

// info is the card under the cursor, in full.
func (l *cardList) info(width int) []string {
	c, ok := l.current()
	if !ok {
		return nil
	}
	return cardInfo(effective(c, l.lenderKey()), width, l.rules, l.rulings[c.Card.ID], l.rulingErr[c.Card.ID])
}

// keys is what this list offers, in two groups: ordering and narrowing it,
// and picking cards out of it. Moving about is the workspace's to offer, and
// the editing keys are not here either: a, A, x, t, c and u act on the
// editing deck rather than on the list under the cursor, so they are drawn
// under that deck.
func (l *cardList) keys() []hintGroup {
	nav := [][2]string{
		hint("sort 1 & 2", keymap.Cards, keymap.CardsSort1Next, keymap.CardsSort1Prev,
			keymap.CardsSort2Next, keymap.CardsSort2Prev),
		hint("flip sort 1/2", keymap.Cards, keymap.CardsSort1Dir, keymap.CardsSort2Dir),
		hint("filter/all lists", keymap.Cards, keymap.CardsFilter, keymap.CardsFilterAll),
	}

	sel := [][2]string{
		hint("select", keymap.Cards, keymap.CardsSelect, keymap.CardsSelectAll),
		hint("yank", keymap.Cards, keymap.CardsYank),
	}
	// p puts into the list in front of you, so it only earns a hint when
	// that list is one of yours to write to.
	local := l.deck != nil && l.deck.Local()
	if local {
		sel = append(sel, hint("put", keymap.Cards, keymap.CardsPut))
	}
	if local {
		sel = append(sel, hint("commit", keymap.Cards, keymap.CardsWrite))
	} else {
		// Not yours, so writing it asks for a name and makes it yours.
		sel = append(sel, hint("save as new deck", keymap.Cards, keymap.CardsWrite, keymap.CardsWriteNew))
	}

	return []hintGroup{{"navigation", nav}, {"select", sel}}
}
