package ui

import (
	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/stats"
	"ttr/internal/tagger"
)

// F: filtering by a tag picked off the card.
//
// The information panel is read, never focused, so its tags can't be
// clicked on. F puts a highlight on the first of them instead, and while it
// is up the keys are the picker's: the moving keys walk the card's tags —
// yours, the global tags, then Scryfall Tagger's — and a, o and n add the
// highlighted one to the list's filter with AND, OR or NOT. It is the same
// filter the statistics build, so their column marks it, and the same key
// again takes it out. esc, F or enter put the highlight away.

// tagPick is the picker on a list: the card it was opened on, and which of
// that card's tags is highlighted.
type tagPick struct {
	card string
	at   int
}

// pickRows are the tags F walks on a card, in the order the card view
// draws them: its own, the global tags', then Scryfall Tagger's. An otag
// shown with the tags is also among Tagger's, and is one stop, not two.
func pickRows(c deck.Card) []stats.Row {
	var out []stats.Row
	seen := map[string]bool{}
	add := func(r stats.Row) {
		if !seen[r.Path] {
			seen[r.Path] = true
			out = append(out, r)
		}
	}
	for _, tag := range c.Tags {
		add(stats.NamedTagRow(tag))
	}
	for _, tag := range c.Borrowed {
		add(stats.NamedTagRow(tag))
	}
	for _, label := range cardOtags(c) {
		add(stats.OtagRow(label))
	}
	for _, label := range tagger.Current().Of(c.Card.OracleID) {
		add(stats.OtagRow(label))
	}
	return out
}

// pickable is the list F would pick on, and the card under its cursor:
// only with the card itself on show (not a picture or a history) and a tag
// on it to pick.
func (m Model) pickable() (*cardList, deck.Card, bool) {
	p := m.ws.current()
	if p == nil || m.info.shows(p) != infoCard {
		return nil, deck.Card{}, false
	}
	l := p.cardsView()
	if l == nil {
		return nil, deck.Card{}, false
	}
	c, ok := l.current()
	if !ok {
		return nil, deck.Card{}, false
	}
	c = effective(c, l.lenderKey())
	if len(pickRows(c)) == 0 {
		return nil, deck.Card{}, false
	}
	return l, c, true
}

// startPick is F: the first of the card's tags highlighted.
func (m *Model) startPick() {
	l, c, ok := m.pickable()
	if !ok {
		return
	}
	l.pick = &tagPick{card: c.Card.Name}
}

// picking is the list with the picker up, if there is one.
func (m Model) picking() *cardList {
	p := m.ws.current()
	if p == nil {
		return nil
	}
	if l := p.cardsView(); l != nil && l.pick != nil {
		return l
	}
	return nil
}

// pickKey is the picker's keymap. Every key is its own while it is up:
// what it doesn't use does nothing, rather than moving the list out from
// under the highlight.
func (m *Model) pickKey(l *cardList, key string) {
	c, ok := l.current()
	rows := pickRows(effective(c, l.lenderKey()))
	// The card it was opened on is gone from under the cursor — a NOT that
	// filtered it out, cards arriving — so there is nothing to pick from.
	if !ok || c.Card.Name != l.pick.card || len(rows) == 0 {
		l.pick = nil
		return
	}
	l.pick.at = minInt(l.pick.at, len(rows)-1)
	op := stats.And
	switch keymap.Lookup(keymap.Pick, key) {
	case keymap.PickNext:
		l.pick.at = (l.pick.at + 1) % len(rows)
		return
	case keymap.PickPrev:
		l.pick.at = (l.pick.at - 1 + len(rows)) % len(rows)
		return
	case keymap.PickDone:
		l.pick = nil
		return
	case keymap.PickAnd:
	case keymap.PickOr:
		op = stats.Or
	case keymap.PickNot:
		op = stats.AndNot
	default:
		return
	}
	l.statFilter = l.statFilter.Toggle(op, rows[l.pick.at])
	l.refresh()
	// The cursor stays on the card while the rows change round it, so the
	// picker stays on the tags it was walking; only a filter that hides the
	// card itself takes it away.
	if !l.selectByName(l.pick.card) {
		l.pick = nil
		m.notice = c.Card.Name + " is filtered out — " + keymap.Hint(keymap.Global, keymap.GlobalPick) + " on another card picks again"
	}
}

// pickView is what the card view needs to draw the picker: which tag is
// highlighted, and the filter, to mark the ones in it. nil draws no picker.
type pickView struct {
	at   int
	rows []stats.Row
	expr stats.Expr
}

// view is the picker as the card c draws it, or nil when it isn't up on c.
func (p *tagPick) view(c deck.Card, expr stats.Expr) *pickView {
	if p == nil || c.Card.Name != p.card {
		return nil
	}
	rows := pickRows(c)
	if len(rows) == 0 {
		return nil
	}
	return &pickView{at: minInt(p.at, len(rows)-1), rows: rows, expr: expr}
}

// index is where a tag sits among the picker's rows, or -1.
func (v *pickView) index(r stats.Row) int {
	if v == nil {
		return -1
	}
	for i, row := range v.rows {
		if row.Path == r.Path {
			return i
		}
	}
	return -1
}
