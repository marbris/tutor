package ui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/theme"
)

// T: the tags of a whole list at once, moved between lists.
//
// t, a and A move cards one at a time. These move tags a list at a time, as
// joins on the card's name, and the second key echoes the one-card verb:
//
//	T t  this list's tags onto the editing deck, for the cards it already has
//	T a  the same, and the cards it lacks are added with their tags
//
// T t and T a ask which tags first, completing from the ones in this list
// with tab; left empty, every tag moves.
//	T g  the global tags written into this list's own
//	T m  every list on screen gets every other list's tags, for its cards
//
// "This list's cards" are the ones picked out with v, or failing that every
// card showing — so / and the statistics narrow what moves.

// tagMoveCmd is one entry in the menu T raises.
type tagMoveCmd struct {
	action keymap.Action
	what   string
}

var tagMoveMenu = []tagMoveCmd{
	{keymap.TagMoveJoin, "tags → editing deck"},
	{keymap.TagMoveUpsert, "tags + cards → editing deck"},
	{keymap.TagMoveGlobal, "global tags → this list"},
	{keymap.TagMoveMerge, "merge tags across lists"},
}

// tagMoveParts is the menu, an entry each, led by T so it reads as what was
// pressed and what comes next.
func tagMoveParts() []string {
	key := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	what := lipgloss.NewStyle().Foreground(theme.Text)
	var parts []string
	for _, c := range tagMoveMenu {
		if k := keymap.Hint(keymap.TagMove, c.action); k != "" {
			parts = append(parts, key.Render(k)+" "+what.Render(c.what))
		}
	}
	if lead := keymap.Hint(keymap.Cards, keymap.CardsTagMove); lead != "" && len(parts) > 0 {
		parts[0] = key.Render(lead+":") + " " + parts[0]
	}
	return parts
}

func (m Model) tagMoveBarLines() []string {
	return packStyled(tagMoveParts(), leaderSep(), maxInt(m.width-2, 1))
}

// shownOrPicked is what T moves the tags of: the cards picked with v, or
// every card showing.
func (l *cardList) shownOrPicked() []deck.Card {
	if len(l.marks) > 0 {
		return l.selection()
	}
	return l.rows
}

// handleTagMove runs the key after T. A key that names nothing cancels.
func (m Model) handleTagMove(key string) (tea.Model, tea.Cmd) {
	p := m.ws.current()
	if p == nil {
		return m, nil
	}
	l := p.cardsView()
	if l == nil {
		return m, nil
	}

	switch keymap.Lookup(keymap.TagMove, key) {
	case keymap.TagMoveJoin, keymap.TagMoveUpsert:
		if target, why := m.editTarget(); target == nil || target == l {
			if target == l {
				why = "this is the editing deck — T t and T a bring tags into it from another list"
			}
			m.notice = why
			return m, nil
		}
		p.ask(askTagMove, "bring tags", "")
		p.askInput.Placeholder = "ramp removal … · tab completes · empty: every tag"
		p.tagMoveAdd = keymap.Lookup(keymap.TagMove, key) == keymap.TagMoveUpsert
	case keymap.TagMoveGlobal:
		m.bakeGlobalTags(l)
	case keymap.TagMoveMerge:
		m.mergeShownTags()
	}
	return m, nil
}

// tagsInto is T t and T a: from the list in front of you into the editing
// deck. only is the tags to bring, every one when empty; with some named,
// only the cards carrying one of them take part, and only those tags go.
func (m *Model) tagsInto(from *cardList, addMissing bool, only []string) {
	target, why := m.editTarget()
	if target == nil {
		m.notice = why
		return
	}
	if target == from {
		m.notice = "this is the editing deck — T t and T a bring tags into it from another list"
		return
	}
	src := onlyTags(from.shownOrPicked(), only)
	if len(src) == 0 {
		m.notice = "no card here has " + strings.Join(only, " or ")
		return
	}
	_, r := m.bring(bringing{what: "tags from " + from.name, cards: src, carry: true, addMissing: addMissing})
	if !r.changed() {
		m.notice = "nothing to bring over — no card here has a tag the deck's copy lacks"
		return
	}
	m.notice = itoa(r.tagged) + " tagged"
	if addMissing {
		m.notice += ", " + itoa(r.added) + " added"
	}
	m.notice += " in " + target.name
}

// bakeGlobalTags is T g: the global tags made this list's own.
func (m *Model) bakeGlobalTags(l *cardList) {
	if l.deck == nil || !l.deck.Local() {
		m.notice = "that list isn't yours — " + keymap.Hint(keymap.Cards, keymap.CardsWrite) + " takes a copy you can tag"
		return
	}
	if len(globalTags.slugs) == 0 {
		m.notice = "no global tags — t in the decks panel pins a list to them"
		return
	}
	l.pushUndo("tags from the global tags")
	n := 0
	for i, c := range l.all {
		merged := deck.ApplyTagEdits(c.Tags, globalTags.of(c.Card.Name, l.lenderKey()), nil)
		if len(merged) != len(c.Tags) {
			l.all[i].Tags = merged
			n++
		}
	}
	if n == 0 {
		l.undoLast()
		m.notice = "the global tags have nothing this list hasn't"
		return
	}
	l.refresh()
	what := " cards took"
	if n == 1 {
		what = " card took"
	}
	m.notice = itoa(n) + what + " tags from the global tags"
}

// mergeShownTags is T m: every list on screen gives its tags to the others.
// Only your own lists change; the rest only give.
func (m *Model) mergeShownTags() {
	var lists []*cardList
	for _, p := range m.ws.panels {
		if l := p.cardsView(); l != nil {
			lists = append(lists, l)
		}
	}
	var cards [][]deck.Card
	for _, l := range lists {
		cards = append(cards, l.all)
	}
	out, tagged := deck.MergeTags(cards...)

	var changed []string
	total := 0
	for i, l := range lists {
		if tagged[i] == 0 || l.deck == nil || !l.deck.Local() {
			continue
		}
		l.pushUndo("merged tags")
		l.all = out[i]
		l.refresh()
		changed = append(changed, l.name)
		total += tagged[i]
	}
	if len(changed) == 0 {
		m.notice = "the lists on screen already share their tags"
		return
	}
	m.notice = "merged tags: " + itoa(total) + " cards in " + strings.Join(changed, ", ")
}

// onlyTags is the cards carrying any of the tags, each with only those, or
// the cards as they are when no tags are named.
func onlyTags(cards []deck.Card, tags []string) []deck.Card {
	if len(tags) == 0 {
		return cards
	}
	want := map[string]bool{}
	for _, t := range tags {
		want[strings.ToLower(t)] = true
	}
	var out []deck.Card
	for _, c := range cards {
		var kept []string
		for _, t := range c.Tags {
			if want[strings.ToLower(t)] {
				kept = append(kept, t)
			}
		}
		if len(kept) > 0 {
			c.Tags = kept
			out = append(out, c)
		}
	}
	return out
}

// sourceTags is the tags T t and T a would move from a list, for tab to
// complete from.
func sourceTags(l *cardList) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range l.shownOrPicked() {
		for _, t := range c.Tags {
			if k := strings.ToLower(t); !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}
