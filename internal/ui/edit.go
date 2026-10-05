package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/deck"
	"ttr/internal/keymap"
)

// Editing the deck you're building.
//
// Every command here works on "the selected cards", which means the ones
// picked out with v, or — with nothing picked out — the one under the
// cursor. That rule lives in cardList.selection, so none of these has to
// decide what an empty selection means.
//
// Nothing here writes to disk itself. An edit marks the list, and the
// model writes the file after the key that made it (see autosave). w is
// the other half: one git commit per press, describing everything since
// the last one.

// undoStep is the deck as it stood before an edit, and what that edit was.
// Whole copies rather than a diff: a deck is a hundred rows, the edits are
// small and rare in machine terms, and a diff that gets it wrong corrupts
// the deck rather than merely failing.
type undoStep struct {
	what  string
	cards []deck.Card
}

const undoDepth = 50

// editTarget is the deck a/x/t/c will change, or nil and the reason why.
func (m *Model) editTarget() (*cardList, string) {
	l := m.ws.editingList()
	if l == nil {
		return nil, "no deck open to edit — open one of yours first"
	}
	if l.deck == nil || !l.deck.Local() {
		return nil, "that deck isn't yours — " + keymap.Hint(keymap.Cards, keymap.CardsWrite) + " takes a copy you can edit"
	}
	return l, ""
}

// pushUndo records the deck as it stands, before something changes it.
func (l *cardList) pushUndo(what string) {
	l.undo = append(l.undo, undoStep{what: what, cards: append([]deck.Card(nil), l.all...)})
	if len(l.undo) > undoDepth {
		l.undo = l.undo[len(l.undo)-undoDepth:]
	}
	l.edited()
}

// edited notes a change to the deck: uncommitted, and not yet written.
func (l *cardList) edited() {
	l.tagEdits++
	if !l.dirty {
		l.wasClean = true
	}
	l.dirty = true
	l.unwritten = true
}

// indexOfCard finds a card in the deck by name. Names rather than ids: a
// deck holds one Sol Ring however many printings Scryfall knows about.
func (l *cardList) indexOfCard(name string) int {
	want := strings.ToLower(name)
	for i, c := range l.all {
		if strings.ToLower(c.Card.Name) == want {
			return i
		}
	}
	return -1
}

// ── Adding and removing ─────────────────────────────────────────

// ── Bringing cards and tags into the editing deck ───────────────

// bringing is one tag or add edit: cards from the list in front of you, or
// from Scryfall Tagger or the global tags, into the editing deck. a, A, t,
// T t, T a, T o, T O and T g are all one of these, differing only in the
// fields below (log/design/keymap.md, "Tags").
type bringing struct {
	what  string // the undo step's name
	cards []deck.Card
	// carry brings the cards' own tags along.
	carry bool
	// add and remove are typed tags, put on every card and taken off.
	add, remove []string
	// addMissing adds a card the deck lacks; without it, such a card is
	// left out.
	addMissing bool
	// another gives a card the deck has another copy — a pressed in the
	// editing deck itself. Brought from anywhere else, a card the deck has
	// only takes the tags.
	another bool
}

// brought is what a bringing did.
type brought struct {
	added   int // cards new to the deck
	raised  int // cards given another copy
	tagged  int // cards whose tags changed
	present int // cards the deck had
	absent  int // cards it lacked and didn't take
}

func (r brought) changed() bool { return r.added+r.raised+r.tagged > 0 }

// bring is every tag and add edit, into the editing deck, as one undo step.
// It returns the deck, or nil (and the notice says why) when there is none.
func (m *Model) bring(b bringing) (*cardList, brought) {
	var r brought
	l, why := m.editTarget()
	if l == nil {
		m.notice = why
		return nil, r
	}
	l.pushUndo(b.what)
	for _, c := range b.cards {
		i := l.indexOfCard(c.Card.Name)
		if i < 0 {
			if !b.addMissing {
				r.absent++
				continue
			}
			nc := deck.Card{Card: c.Card, Qty: 1}
			if b.carry {
				nc.Tags = append([]string(nil), c.Tags...)
			}
			nc.Tags = deck.ApplyTagEdits(nc.Tags, b.add, b.remove)
			l.all = append(l.all, nc)
			r.added++
			continue
		}
		r.present++
		if b.another {
			l.all[i].Qty++
			r.raised++
		}
		next := l.all[i].Tags
		if b.carry {
			next = deck.ApplyTagEdits(next, c.Tags, nil)
		}
		next = deck.ApplyTagEdits(next, b.add, b.remove)
		if !sameTags(next, l.all[i].Tags) {
			l.all[i].Tags = next
			r.tagged++
		}
	}
	if !r.changed() {
		l.undoLast() // nothing changed, so there is nothing to undo
		return l, r
	}
	l.refresh()
	l.recheck()
	return l, r
}

func sameTags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// fromEditing reports whether the list in front of you is the editing deck,
// where a gives another copy rather than bringing the card in.
func (m *Model) fromEditing() bool {
	p := m.ws.current()
	return p != nil && p.cardsView() != nil && p.cardsView() == m.ws.editingList()
}

// add is a: the cards into the editing deck, with their tags. In the deck
// itself it gives another copy — a a a on a basic land gives you three.
// From anywhere else a card the deck already has takes the tags and no
// copy, so copying from another deck never doubles a card by accident.
func (m *Model) add(cards []deck.Card) {
	l, r := m.bring(bringing{what: label("+", cards), cards: cards, carry: true, addMissing: true, another: m.fromEditing()})
	if l == nil {
		return
	}
	m.notice = bringNotice(cards, r, l.name)
}

// bringNotice says what an add did.
func bringNotice(cards []deck.Card, r brought, deckName string) string {
	if len(cards) == 1 {
		name := cards[0].Card.Name
		switch {
		case r.raised == 1:
			return "another " + name
		case r.added == 1:
			return "+1 " + name
		case r.tagged == 1:
			return name + " is in " + deckName + " already — it took the tags"
		}
		return name + " is in " + deckName + " already"
	}
	var parts []string
	if r.added > 0 {
		parts = append(parts, "+"+itoa(r.added)+" cards")
	}
	if r.raised > 0 {
		parts = append(parts, itoa(r.raised)+" more copies")
	}
	if n := r.present - r.raised; n > 0 {
		said := itoa(n) + " already there"
		if r.tagged > 0 {
			said += " (" + itoa(r.tagged) + " took tags)"
		}
		parts = append(parts, said)
	}
	if len(parts) == 0 {
		return "nothing to add"
	}
	return strings.Join(parts, ", ")
}

// addTo is the body of an add, against whichever list it is given: a for the
// editing deck, i for a card fetched into the deck in front of you. It
// returns the notice to show.
func addTo(l *cardList, cards []deck.Card) string {
	l.pushUndo(label("+", cards))
	added, raised := 0, 0
	for _, c := range cards {
		if i := l.indexOfCard(c.Card.Name); i >= 0 {
			l.all[i].Qty++
			raised++
			continue
		}
		// Tags travel with a card moved between decks, but a card added
		// from a search has none to bring.
		l.all = append(l.all, deck.Card{Card: c.Card, Qty: 1, Tags: c.Tags})
		added++
	}
	l.refresh()
	l.recheck()
	return addNotice(cards, added, raised)
}

// remove takes a copy away, and the row with it when the last one goes.
func (m *Model) remove(cards []deck.Card) {
	l, why := m.editTarget()
	if l == nil {
		m.notice = why
		return
	}

	l.pushUndo(label("-", cards))
	removed, missing := 0, 0
	for _, c := range cards {
		i := l.indexOfCard(c.Card.Name)
		if i < 0 {
			missing++
			continue
		}
		if l.all[i].Qty > 1 {
			l.all[i].Qty--
		} else {
			l.all = append(l.all[:i:i], l.all[i+1:]...)
		}
		removed++
	}
	l.refresh()
	l.recheck()

	switch {
	case removed == 0:
		m.notice = "not in the deck"
		l.undoLast() // nothing changed, so there is nothing to undo
	case missing > 0:
		m.notice = itoa(removed) + " removed, " + itoa(missing) + " weren't in the deck"
	default:
		m.notice = label("-", cards)
	}
}

func addNotice(cards []deck.Card, added, raised int) string {
	if len(cards) == 1 {
		if raised == 1 {
			return "another " + cards[0].Card.Name
		}
		return "+1 " + cards[0].Card.Name
	}
	out := "+" + itoa(added+raised) + " cards"
	if raised > 0 {
		out += " (" + itoa(raised) + " already there)"
	}
	return out
}

// label names an edit for the undo list and the notice line.
func label(sign string, cards []deck.Card) string {
	if len(cards) == 1 {
		return sign + "1 " + cards[0].Card.Name
	}
	return sign + itoa(len(cards)) + " cards"
}

// ── Yank and put ────────────────────────────────────────────────

// yank picks cards up. The register holds whole deck cards — quantity and
// tags included — so moving a card between decks moves what you knew about
// it, not just its name.
func (m *Model) yank(cards []deck.Card) {
	if len(cards) == 0 {
		return
	}
	m.register = append([]deck.Card(nil), cards...)
	m.notice = itoa(len(cards)) + " " + plural("card", len(cards)) + " yanked"
}

// put drops the register into whichever local deck is focused — which need
// not be the editing deck, and that is the point: y and p move cards between
// two lists without either of them being the one you're building.
func (m *Model) put(l *cardList) {
	if len(m.register) == 0 {
		m.notice = "nothing yanked"
		return
	}
	if l.deck == nil || !l.deck.Local() {
		m.notice = "that deck isn't yours — " + keymap.Hint(keymap.Cards, keymap.CardsPut) + " needs a deck you can edit"
		return
	}

	l.pushUndo("put " + itoa(len(m.register)) + " cards")
	for _, c := range m.register {
		if i := l.indexOfCard(c.Card.Name); i >= 0 {
			l.all[i].Qty += maxInt(c.Qty, 1)
			l.all[i].Tags = deck.ApplyTagEdits(l.all[i].Tags, c.Tags, nil)
			continue
		}
		put := c
		put.Qty = maxInt(c.Qty, 1)
		put.Commander = false // a commander is a role in one deck, not a property
		l.all = append(l.all, put)
	}
	l.refresh()
	l.recheck()
	m.notice = "put " + itoa(len(m.register)) + " " + plural("card", len(m.register))
}

// ── Tagging ─────────────────────────────────────────────────────

// tag is t: an edit to the tags of the selected cards, in the editing deck.
// Cards that aren't in the deck aren't tagged: a tag is something the deck's
// author said about a card in their deck.
func (m *Model) tag(cards []deck.Card, input string) {
	add, remove := parseTagEdit(input)
	if len(add) == 0 && len(remove) == 0 {
		return
	}
	l, r := m.bring(bringing{what: "tag " + itoa(len(cards)) + " cards", cards: cards, add: add, remove: remove})
	if l == nil {
		return
	}
	if len(add) > 0 {
		m.lastTag = add[len(add)-1]
	}
	switch {
	case r.present == 0:
		m.notice = "none of those are in the deck"
	case r.absent > 0:
		m.notice = itoa(r.present) + " tagged, " + itoa(r.absent) + " not in the deck"
	default:
		m.notice = itoa(r.present) + " tagged"
	}
}

// addTagged is A: a, and the typed tags put on as well. The prompt opens
// with the tag you last used, so sorting a search into a deck is A enter,
// A enter, … — having to retype the tag each time is what makes people stop.
func (m *Model) addTagged(cards []deck.Card, input string) {
	add, remove := parseTagEdit(input)
	l, r := m.bring(bringing{what: label("+", cards), cards: cards, carry: true, add: add, remove: remove, addMissing: true, another: m.fromEditing()})
	if l == nil {
		return
	}
	if len(add) > 0 {
		m.lastTag = add[len(add)-1]
	}
	m.notice = bringNotice(cards, r, l.name)
	if len(add) > 0 && r.changed() {
		m.notice += " · tagged " + strings.Join(add, " ")
	}
}

// parseTagEdit splits an answer into tags to add and tags to take away. A
// leading minus removes, so "ramp -draw" does both at once.
//
// Quoting works the same way it does in a filter, which matters because a
// deck imported from Moxfield can arrive carrying tags like "fast mana":
// without quotes those could be read but never typed, and so never removed.
// One query syntax across the program, again.
func parseTagEdit(input string) (add, remove []string) {
	for _, term := range filterTerms(input) {
		if strings.HasPrefix(term, "-") {
			if t := strings.TrimPrefix(term, "-"); t != "" {
				remove = append(remove, t)
			}
			continue
		}
		add = append(add, term)
	}
	return add, remove
}

// ── Commanders ──────────────────────────────────────────────────

// commander marks a card as the deck's commander, or unmarks it. Toggling
// rather than replacing, because a pair of partners is two of them.
func (m *Model) commander(c deck.Card) tea.Cmd {
	l, why := m.editTarget()
	if l == nil {
		// A commander is a role in a deck you're editing. With no such deck
		// open, c has nothing to act on — it says so rather than conjuring a
		// deck up around the card.
		m.notice = why
		return nil
	}

	i := l.indexOfCard(c.Card.Name)
	if i < 0 {
		l.pushUndo("commander " + c.Card.Name)
		l.all = append(l.all, deck.Card{Card: c.Card, Qty: 1, Commander: true})
		l.refresh()
		l.recheck()
		m.notice = c.Card.Name + " is the commander"
		return nil
	}

	l.pushUndo("commander " + c.Card.Name)
	l.all[i].Commander = !l.all[i].Commander
	l.refresh()
	l.recheck()
	if l.all[i].Commander {
		m.notice = c.Card.Name + " is the commander"
	} else {
		m.notice = c.Card.Name + " is no longer the commander"
	}
	return nil
}

// ── Undo ────────────────────────────────────────────────────────

// undoLast drops the most recent undo step without applying it, for edits
// that turned out to change nothing.
func (l *cardList) undoLast() {
	if n := len(l.undo); n > 0 {
		l.undo = l.undo[:n-1]
	}
}

func (m *Model) undo() {
	l, why := m.editTarget()
	if l == nil {
		m.notice = why
		return
	}
	n := len(l.undo)
	if n == 0 {
		m.notice = "nothing to undo"
		return
	}

	step := l.undo[n-1]
	l.undo = l.undo[:n-1]
	l.all = step.cards
	l.edited()
	l.refresh()
	l.recheck()
	m.notice = "undid " + step.what
}
