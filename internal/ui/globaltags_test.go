package ui

import (
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/stats"
)

// freshGlobalTags gives a test a global tags index of its own.
func freshGlobalTags(t *testing.T) {
	t.Helper()
	globalTags = &tagIndex{}
	t.Cleanup(func() { globalTags = &tagIndex{} })
}

func tagCount(l *cardList, tag string) int {
	all := effectiveAll(l.all, l.lenderKey())
	r, ok := findRow(stats.Groups(all, all), "Tags", tag)
	if !ok {
		return 0
	}
	return r.Base
}

// Scenario 1 of notes/2026-10-05-improvement-notes-11.md: a tag on a list
// that's open counts in the other lists open beside it.
func TestAnOpenListLendsItsTagsToTheOthers(t *testing.T) {
	freshGlobalTags(t)
	m, one := openDeckPanel(t, sized(200, 30), "list-1", "list 1", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"ramp"}},
	})
	m, two := openDeckPanel(t, m, "list-2", "list 2", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	})
	m = drive(m, "j") // any message syncs the global tags

	two.setFilter("tag:ramp")
	if len(two.rows) != 1 {
		t.Errorf("tag:ramp in list 2 found %d cards, want Sol Ring", len(two.rows))
	}
	two.setFilter("")
	if n := tagCount(two, "ramp"); n != 1 {
		t.Errorf("list 2's statistics count ramp %d times", n)
	}
	if len(tagged(two, "Sol Ring")) != 0 {
		t.Error("the borrowed tag was written into list 2")
	}
	if c := effective(one.all[0], one.lenderKey()); len(c.Borrowed) != 0 {
		t.Errorf("list 1 borrows its own tag back: %v", c.Borrowed)
	}
}

// Scenario 2: both lists lend until one is muted with space t, and a muted
// list still sees its own tags and everyone else's.
func TestSpaceTStopsAListLending(t *testing.T) {
	freshGlobalTags(t)
	m, one := openDeckPanel(t, sized(200, 30), "list-1", "list 1", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"ramp"}},
	})
	m, two := openDeckPanel(t, m, "list-2", "list 2", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"otag-ramp"}},
	})
	m = drive(m, "j")
	if tagCount(one, "otag-ramp") != 1 || tagCount(two, "ramp") != 1 {
		t.Fatal("both lists should see both tags while both lend")
	}

	m = drive(m, "space", "t") // list 2 is focused
	if tagCount(one, "otag-ramp") != 0 {
		t.Error("list 1 still sees list 2's tag after list 2 was muted")
	}
	if tagCount(two, "otag-ramp") != 1 || tagCount(two, "ramp") != 1 {
		t.Error("a muted list should still see its own tags and the others'")
	}

	m = drive(m, "space", "t")
	if tagCount(one, "otag-ramp") != 1 {
		t.Error("space t again didn't make list 2 lend again")
	}
}

// An edit to a list shows up in the others at once.
func TestAnEditToALendingListReachesTheOthers(t *testing.T) {
	freshGlobalTags(t)
	m, _ := openDeckPanel(t, sized(200, 30), "list-2", "list 2", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	})
	other := m.ws.current().cardsView()
	m, one := openDeckPanel(t, m, "list-1", "list 1", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	})
	m = drive(m, "t")
	for _, r := range "draw" {
		m = drive(m, string(r))
	}
	m = drive(m, "enter")
	if !hasTag(tagged(one, "Sol Ring"), "draw") {
		t.Fatalf("t didn't tag list 1: %v", tagged(one, "Sol Ring"))
	}
	if tagCount(other, "draw") != 1 {
		t.Error("list 2 doesn't see the tag just put on in list 1")
	}
}
