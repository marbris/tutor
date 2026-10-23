package ui

import (
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/stats"
)

// Note 21: the markers beside a list count only what the other list shows.
func TestMarkersFollowTheOtherListsFilters(t *testing.T) {
	m := sized(240, 30)
	m = withCards(m, "f", sample(), sortArrival) // 0: the list being marked
	m = withCards(m, "f", sample(), sortArrival) // 1: the other, focused
	a, b := m.ws.panels[0].cardsView(), m.ws.panels[1].cardsView()

	if got := m.membersFor(a)["sol ring"]; got != inFocused {
		t.Fatalf("Sol Ring is marked %v before any filter", got)
	}

	b.setFilter("elves")
	members := m.membersFor(a)
	if members["sol ring"] != notElsewhere {
		t.Errorf("Sol Ring is still marked with the other list filtered to elves")
	}
	if members["llanowar elves"] != inFocused {
		t.Errorf("Llanowar Elves lost its mark, though the other list shows it")
	}

	b.setFilter("")
	if got := m.membersFor(a)["sol ring"]; got != inFocused {
		t.Errorf("clearing the filter left Sol Ring marked %v", got)
	}

	b.statFilter, _ = b.statFilter.Add(stats.And, stats.Row{Group: "Type", Label: "Land",
		Match: func(c deck.Card) bool { return mtg.IsLand(c.Card) }})
	b.refresh()
	members = m.membersFor(a)
	if members["sol ring"] != notElsewhere || members["forest"] != inFocused {
		t.Errorf("with the statistics on lands, Sol Ring is %v and Forest %v", members["sol ring"], members["forest"])
	}
}

// The list's inclusion order follows the other list's filter as it is typed.
func TestInclusionOrderFollowsAFilterElsewhere(t *testing.T) {
	m := sized(240, 30)
	m = withCards(m, "f", sample(), sortInclusion)
	m = withCards(m, "f", sample()[2:3], sortArrival) // Sol Ring alone
	a := m.ws.panels[0].cardsView()
	m = drive(m, "j") // any key lets the workspace catch up
	if a.members["sol ring"] != inFocused {
		t.Fatalf("Sol Ring is %v before the filter", a.members["sol ring"])
	}
	m = drive(m, "/", "e", "l", "f")
	if a.members["sol ring"] != notElsewhere {
		t.Errorf("typing a filter that hides Sol Ring left it %v beside", a.members["sol ring"])
	}
}

// t takes several tags at once, a space between them.
func TestTTakesSeveralTags(t *testing.T) {
	m := editingDeckModel()
	m = drive(m, "t", "r", "a", "m", "p", "space", "d", "r", "a", "w", "enter")
	l := m.ws.current().cardsView()
	c, _ := l.current()
	got := l.all[l.indexOfCard(c.Card.Name)].Tags
	if !sameTags(got, []string{"draw", "ramp"}) && !sameTags(got, []string{"ramp", "draw"}) {
		t.Errorf("t ramp draw left the tags %v", got)
	}
	if m.notice != "1 tagged ramp draw" {
		t.Errorf("the notice reads %q", m.notice)
	}
}
