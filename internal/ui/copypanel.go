package ui

import (
	"maps"
	"slices"
)

// space n: a copy of the panel you're in, beside it, so the same cards can
// be looked at two ways at once — sorted differently, filtered differently,
// one showing the lands and the other the rest.
//
// The copy is taken from what's in hand rather than fetched again, so it
// comes up as it was: cards, sorts, filters, cursor. What was half done —
// a prompt being answered, cards picked with v — stays with the original.

// copyPanel puts a copy of the focused panel to its right and focuses it.
func (m *Model) copyPanel() {
	p := m.ws.current()
	if p == nil {
		return
	}
	var stack []view
	for _, v := range p.stack {
		c := copyView(v)
		if c == nil {
			// A view reached from another — a deck's versions — isn't
			// copied; the copy stops at the one it was reached from.
			break
		}
		stack = append(stack, c)
	}
	if len(p.stack) > 0 && len(stack) == 0 {
		m.notice = "there's one " + p.kind.String() + " panel — it isn't copied"
		return
	}

	c := m.ws.open(p.kind)
	id := c.id
	*c = *p
	c.id = id
	c.stack = stack
	if len(stack) > 0 {
		c.title = stack[len(stack)-1].title()
	}
	c.asking, c.tagComp, c.otagComp = askNone, nil, nil
	c.filtering, c.filterAll, c.filterAllBefore = false, false, nil
	c.search.Blur()
	c.askInput.Blur()
	c.filterInput.Blur()
	// An answer on its way finds its panel by id, and that is the original.
	c.loading, c.awaitingStore, c.pending = false, false, nil
	if len(stack) == 0 {
		c.searchOpen = true
		c.search.Focus()
	}
}

// copyView is a copy of a view that can stand on its own, or nil.
func copyView(v view) view {
	switch v := v.(type) {
	case *cardList:
		return v.copy()
	case *deckList:
		c := *v
		c.all, c.rows, c.folders = slices.Clone(v.all), slices.Clone(v.rows), slices.Clone(v.folders)
		c.legality, c.colours = maps.Clone(v.legality), maps.Clone(v.colours)
		c.expanded, c.fetching = maps.Clone(v.expanded), map[string]bool{}
		c.confirming, c.moving = nil, nil
		return &c
	case *rulesView:
		c := *v
		c.all, c.rows = slices.Clone(v.all), slices.Clone(v.rows)
		return &c
	}
	return nil
}

// copy is the list as it stands, sorted and narrowed the same, with nothing
// picked. The rulings are shared: they say the same of a card in either.
func (l *cardList) copy() *cardList {
	c := *l
	c.all, c.undo = slices.Clone(l.all), slices.Clone(l.undo)
	c.marks = map[string]bool{}
	c.members, c.nameSet, c.shownSet = nil, nil, nil
	if l.deck != nil {
		info := *l.deck
		c.deck = &info
	}
	c.refresh()
	return &c
}

// mirrorDeck brings every other open copy of l's deck up to date with it,
// so two panels on one deck can't disagree, and the one behind can't write
// its older cards over the newer. The undo steps come too: u in either
// takes back the same edit.
func (m Model) mirrorDeck(l *cardList) {
	if l.deck == nil || !l.deck.Local() {
		return
	}
	for _, d := range m.openDecks() {
		o := d.list
		if o == l || o.deck.Slug != l.deck.Slug {
			continue
		}
		o.all, o.undo = slices.Clone(l.all), slices.Clone(l.undo)
		o.dirty, o.tagEdits, o.legality = l.dirty, l.tagEdits, l.legality
		o.refresh()
	}
}
