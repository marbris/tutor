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

// T: tags a whole list at a time, into the editing deck.
//
// t, a and A work on the cursor or the v picks. T works on a whole list,
// and the second key says where the tags come from (log/design/keymap.md,
// "Tags"):
//
//	T t  this list's tags onto the editing deck, for the cards it has
//	T a  the same, and the cards it lacks are added with their tags
//	T o  Scryfall Tagger's tag, on the editing deck's cards that carry it
//	T O  the same, and every other card that carries it added
//	T g  the global tags made the editing deck's own
//
// All but T o and T O ask which tags first, completing with tab; left empty,
// every tag moves. "This list's cards" are the ones picked out with v, or
// failing that every card showing — so / and the statistics narrow what
// moves. Each is one undo step on the editing deck.

// tagMoveCmd is one entry in the menu T raises.
type tagMoveCmd struct {
	action keymap.Action
	what   string
}

var tagMoveMenu = []tagMoveCmd{
	{keymap.TagMoveJoin, "tags → editing deck"},
	{keymap.TagMoveUpsert, "tags + cards → editing deck"},
	{keymap.TagMoveOtag, "otag → editing deck"},
	{keymap.TagMoveOtagAdd, "otag + cards → editing deck"},
	{keymap.TagMoveGlobal, "global tags → editing deck"},
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
	target, why := m.editTarget()
	if target == nil {
		m.notice = why
		return m, nil
	}

	switch action := keymap.Lookup(keymap.TagMove, key); action {
	case keymap.TagMoveJoin, keymap.TagMoveUpsert:
		if target == l {
			m.notice = "this is the editing deck — T t and T a bring tags into it from another list"
			return m, nil
		}
		p.ask(askTagMove, "bring tags", "")
		p.askInput.Placeholder = "ramp removal … · tab completes · empty: every tag"
		p.tagMoveAdd = action == keymap.TagMoveUpsert
	case keymap.TagMoveOtag, keymap.TagMoveOtagAdd:
		add := action == keymap.TagMoveOtagAdd
		what := "tag by otag"
		if add {
			what = "add by otag"
		}
		p.ask(askOtag, what+" → "+target.name, "")
		p.askInput.Placeholder = "ball-lightning removal … · tab completes"
		p.otagAdd = add
	case keymap.TagMoveGlobal:
		p.ask(askGlobalTags, "global tags → "+target.name, "")
		p.askInput.Placeholder = "ramp removal … · tab completes · empty: every tag"
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

// bakeGlobalTags is T g: the global tags made the editing deck's own — the
// ones named, or every one when only is empty.
func (m *Model) bakeGlobalTags(only []string) {
	target, why := m.editTarget()
	if target == nil {
		m.notice = why
		return
	}
	if globalTags.empty() {
		m.notice = "no other list lends tags — open one, or pin one with t in the decks panel"
		return
	}
	var src []deck.Card
	for _, c := range target.all {
		if tags := globalTags.of(c.Card.Name, target.lenderKey()); len(tags) > 0 {
			src = append(src, deck.Card{Card: c.Card, Tags: tags})
		}
	}
	src = onlyTags(src, only)
	r := bringInto(target, bringing{what: "tags from the global tags", cards: src, carry: true})
	if !r.changed() {
		m.notice = "the global tags have nothing " + target.name + " hasn't"
		return
	}
	what := " cards took"
	if r.tagged == 1 {
		what = " card took"
	}
	m.notice = itoa(r.tagged) + what + " tags from the global tags"
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
