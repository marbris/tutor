package ui

import (
	"fmt"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/stats"
)

func pickSample() []deck.Card {
	mk := func(name string, tags ...string) deck.Card {
		return deck.Card{Qty: 1, Tags: tags, Card: mtg.Card{Name: name, TypeLine: "Sorcery"}}
	}
	return []deck.Card{
		mk("Cultivate", "ramp", "land"),
		mk("Harmonize", "draw"),
		mk("Kodama's Reach", "ramp"),
		mk("Mulch", "land"),
		mk("Plain Card"),
	}
}

func TestFPicksATagOffTheCardAndFiltersByIt(t *testing.T) {
	m := withCards(sized(150, 40), "d", pickSample(), sortArrival)
	l := m.ws.current().cardsView()
	if !strings.Contains(fmt.Sprint(m.hintGroups()), "filter by a tag of this card") {
		t.Error("F isn't offered on a card with tags")
	}
	m = drive(m, "F")
	if l.pick == nil || l.pick.at != 0 {
		t.Fatalf("F didn't highlight the first tag: %+v", l.pick)
	}
	if !strings.Contains(fmt.Sprint(m.hintGroups()), "next/prev tag") {
		t.Error("the picker's keys aren't the hints")
	}
	// h and l are the picker's: they walk the tags, not the panels.
	m = drive(m, "l")
	if l.pick.at != 1 {
		t.Errorf("l left the highlight on %d", l.pick.at)
	}
	m = drive(m, "l")
	if l.pick.at != 0 {
		t.Errorf("l off the last tag went to %d, want round to 0", l.pick.at)
	}
	m = drive(m, "l", "a") // land
	if l.count() != 2 || !l.statFilter.Has(stats.NamedTagRow("land")) {
		t.Fatalf("a on land left %d cards, filter %s", l.count(), l.statFilter)
	}
	// The statistics see it: the land row is marked.
	m = drive(m, "esc", "s")
	if r, ok := findRow(m.statGroups(), "Tags", "land"); !ok || statMark(l.statFilter, r) == " " {
		t.Error("the statistics don't mark the picked tag")
	}
	m = drive(m, "s", "F", "l", "a")
	if len(l.statFilter) != 0 || l.count() != 5 {
		t.Errorf("a again didn't take land out: %s, %d cards", l.statFilter, l.count())
	}
	if l.pick == nil {
		t.Fatal("the picker went away after a filter that kept its card")
	}
	m = drive(m, "esc")
	if l.pick != nil {
		t.Error("esc didn't put the picker away")
	}
}

func TestNOnTheCardsOwnTagPutsThePickerAway(t *testing.T) {
	m := withCards(sized(150, 40), "d", pickSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "F", "n") // NOT ramp: Cultivate is gone
	if l.count() != 3 {
		t.Errorf("NOT ramp left %d cards", l.count())
	}
	if l.pick != nil {
		t.Error("the picker stayed up over a card it wasn't opened on")
	}
	if !strings.Contains(m.notice, "Cultivate is filtered out") {
		t.Errorf("notice %q", m.notice)
	}
}

func TestFDoesNothingOnACardWithoutTags(t *testing.T) {
	m := withCards(sized(150, 40), "d", pickSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "G", "F")
	if l.pick != nil {
		t.Error("F picked on a card with no tags")
	}
	if strings.Contains(fmt.Sprint(m.hintGroups()), "filter by a tag of this card") {
		t.Error("F is offered on a card with no tags")
	}
	// j is the list's again.
	m = drive(m, "k")
	if c, _ := l.current(); c.Card.Name != "Mulch" {
		t.Errorf("k went to %s", c.Card.Name)
	}
}

func TestFPicksAScryfallTaggerTag(t *testing.T) {
	isolate(t)
	freshGlobalTags(t)
	withTagger(t)
	m := withCards(sized(150, 40), "f", taggedCards(), sortArrival) // shatter first
	l := m.ws.current().cardsView()
	m = drive(m, "F")
	view := stripANSI(m.View())
	if !strings.Contains(view, "removal-artifact") || !strings.Contains(view, "F picking") {
		t.Fatalf("the picker isn't drawn:\n%s", view)
	}
	m = drive(m, "o")
	if l.count() != 2 {
		t.Errorf("removal-artifact left %d cards, want 2", l.count())
	}
	// Not one of the otags shown with the tags, but in the filter: the Tags
	// group shows it, so it can be taken off there.
	if _, ok := findRow(m.statGroups(), "Tags", "removal-artifact"); !ok {
		t.Error("the picked otag has no row among the tags")
	}
}
