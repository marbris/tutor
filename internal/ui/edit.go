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

// add puts a copy of each card into the deck, or another copy of one already
// there. a a a on a basic land gives you three of it, which is why this
// counts up rather than refusing.
func (m *Model) add(cards []deck.Card) {
	l, why := m.editTarget()
	if l == nil {
		m.notice = why
		return
	}

	m.notice = addTo(l, cards)
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

// tag applies an edit to the tags of the selected cards, in the editing
// deck. Cards that aren't in the deck aren't tagged: a tag is something the
// deck's author said about a card in their deck.
func (m *Model) tag(cards []deck.Card, input string) {
	l, why := m.editTarget()
	if l == nil {
		m.notice = why
		return
	}

	add, remove := parseTagEdit(input)
	if len(add) == 0 && len(remove) == 0 {
		return
	}

	l.pushUndo("tag " + itoa(len(cards)) + " cards")
	changed, absent := 0, 0
	for _, c := range cards {
		i := l.indexOfCard(c.Card.Name)
		if i < 0 {
			absent++
			continue
		}
		l.all[i].Tags = deck.ApplyTagEdits(l.all[i].Tags, add, remove)
		changed++
	}
	l.refresh()

	if len(add) > 0 {
		m.lastTag = add[len(add)-1]
	}
	switch {
	case changed == 0:
		l.undoLast()
		m.notice = "none of those are in the deck"
	case absent > 0:
		m.notice = itoa(changed) + " tagged, " + itoa(absent) + " not in the deck"
	default:
		m.notice = itoa(changed) + " tagged"
	}
}

// tagWithLast is A: add the cards to the deck and tag them with the tag you
// last used, in one keystroke. Sorting a search into a deck is dozens of
// these, and having to retype the tag each time is what makes people stop.
func (m *Model) tagWithLast(cards []deck.Card) {
	if m.lastTag == "" {
		m.notice = "no tag used yet — " + keymap.Hint(keymap.Cards, keymap.CardsTag) + " first"
		return
	}
	m.add(cards)
	if l, _ := m.editTarget(); l != nil {
		m.tag(cards, m.lastTag)
		m.notice = itoa(len(cards)) + " added and tagged " + m.lastTag
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
