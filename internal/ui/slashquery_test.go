package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestSlashTakesScryfallsSyntaxOverTheList(t *testing.T) {
	m := withCards(sized(200, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "/")
	if view := stripANSI(m.View()); !strings.Contains(view, "t:creature mv<=3") {
		t.Errorf("the / bar doesn't show its syntax:\n%s", firstLines(view, 3))
	}
	m = typeIn(m, "t:creature mv<=3")
	m = drive(m, "enter")
	if l.count() != 2 {
		t.Errorf("t:creature mv<=3 left %d cards, want the two elves", l.count())
	}
	// Plain words still match as before, tags among them.
	m = drive(m, "/")
	for range "t:creature mv<=3" {
		m = drive(m, "backspace")
	}
	m = typeIn(m, "wincon")
	m = drive(m, "enter")
	if l.count() != 1 {
		t.Errorf("wincon left %d cards, want the Craterhoof", l.count())
	}
}

func TestSlashOtagCountsTheTagsUnderIt(t *testing.T) {
	withTagger(t)
	m := withCards(sized(160, 30), "f", taggedCards(), sortArrival)
	l := m.ws.current().cardsView()
	m = typeIn(drive(m, "/"), "otag:removal")
	m = drive(m, "enter")
	if l.count() != 2 {
		t.Errorf("otag:removal left %d cards, want shatter and disenchant", l.count())
	}
	m = typeIn(drive(m, "/"), " -otag:removal-enchantment")
	m = drive(m, "enter")
	if l.count() != 1 || l.rows[0].Card.Name != "shatter" {
		t.Errorf("otag:removal -otag:removal-enchantment left %d cards", l.count())
	}
}

func TestCtrlSlashFiltersEveryList(t *testing.T) {
	m := withCards(sized(200, 30), "d", deckSample(), sortArrival)
	a := m.ws.current().cardsView()
	m = withCards(m, "d", deckSample(), sortArrival)
	b := m.ws.current().cardsView()
	m = drive(typeIn(drive(m, "/"), "wincon"), "enter") // b had its own filter
	all, wincon := a.count(), b.count()

	m = typeIn(drive(m, "ctrl+_"), " t:creature mv<=3")
	if a.filter != "wincon t:creature mv<=3" || b.filter != a.filter {
		t.Errorf("ctrl+/ left the filters %q and %q, want the same query on both", a.filter, b.filter)
	}
	m = drive(m, "esc")
	if a.count() != all || b.count() != wincon || b.filter != "wincon" {
		t.Errorf("esc left %d and %d cards (%q), want %d and %d", a.count(), b.count(), b.filter, all, wincon)
	}

	m = drive(typeIn(drive(m, "ctrl+_"), " t:creature"), "enter")
	if a.filter != "wincon t:creature" || b.filter != a.filter {
		t.Errorf("enter kept %q and %q, want the query on both", a.filter, b.filter)
	}
	if hints := fmt.Sprint(m.hintGroups()); !strings.Contains(hints, "/ ctrl+/") {
		t.Errorf("the hints don't name ctrl+/: %s", hints)
	}
}
