package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
)

// twoOfOne is deck one in panels 0 and 1, and deck two in panel 2, with
// panel 0 the editing deck.
func twoOfOne(t *testing.T) Model {
	t.Helper()
	withStore(t)
	seedDeck(t, "ed-one", "name: One\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	seedDeck(t, "ed-two", "name: Two\nformat: commander\n[mainboard]\n1 Llanowar Elves\n")
	t.Cleanup(func() { deck.Delete("ed-one"); deck.Delete("ed-two") })
	m, _ := openFromDisk(t, "ed-one")
	m = openAlso(t, m, "ed-one")
	m = openAlso(t, m, "ed-two")
	m.ws.editing = 0
	return m
}

func TestEveryPanelOfTheEditingDeckIsDrawnAsIt(t *testing.T) {
	m := twoOfOne(t)
	if !m.ws.showsEditing(0) || !m.ws.showsEditing(1) || m.ws.showsEditing(2) {
		t.Errorf("drawn as editing: %v %v %v", m.ws.showsEditing(0), m.ws.showsEditing(1), m.ws.showsEditing(2))
	}
	// Its keys sit under both, as they work in both.
	m = drive(m, "?")
	if n := strings.Count(stripANSI(m.View()), "next/prev deck"); n != 2 {
		t.Errorf("e E offered under %d panels, want both of the editing deck's", n)
	}
}

func TestECountsADeckInTwoPanelsOnce(t *testing.T) {
	m := twoOfOne(t)
	m = drive(m, "e")
	if m.editingSlug() != "ed-two" {
		t.Fatalf("e went to %q, not the other deck", m.editingSlug())
	}
	m = drive(m, "e")
	if m.editingSlug() != "ed-one" {
		t.Errorf("e from the last deck went to %q", m.editingSlug())
	}
	m = drive(m, "e")
	if m.editingSlug() != "ed-two" {
		t.Errorf("e took two steps through one deck's panels: %q", m.editingSlug())
	}
	m = drive(m, "E", "E")
	if m.editingSlug() != "ed-two" {
		t.Errorf("E E from two came to %q", m.editingSlug())
	}
}

func TestOneDeckInTwoPanelsDoesntOfferE(t *testing.T) {
	withStore(t)
	seedDeck(t, "ed-only", "name: Only\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ed-only") })
	m, _ := openFromDisk(t, "ed-only")
	m = openAlso(t, m, "ed-only")
	m = drive(m, "?")
	if strings.Contains(stripANSI(m.View()), "next/prev deck") {
		t.Error("e E offered with only one deck to go to")
	}
}

// One deck in two panels, one filtered: the filtered one marks what it picks
// out in the full one, and the full one marks nothing in the filtered one.
func TestAFilteredCopyOfADeckMarksItsCardsInTheFullOne(t *testing.T) {
	withStore(t)
	seedDeck(t, "ed-split", "name: Split\nformat: commander\n[mainboard]\n1 Sol Ring [ramp]\n1 Llanowar Elves\n")
	t.Cleanup(func() { deck.Delete("ed-split") })
	m, full := openFromDisk(t, "ed-split")
	m = openAlso(t, m, "ed-split")
	part := m.ws.current().cardsView()
	m = drive(m, "/", "e", "l", "v", "e", "s", "enter")
	if len(part.rows) != 1 {
		t.Fatalf("the filter shows %d cards", len(part.rows))
	}
	if full.members["llanowar elves"] == notElsewhere || full.members["sol ring"] != notElsewhere {
		t.Errorf("in the full list: elves %v, sol ring %v", full.members["llanowar elves"], full.members["sol ring"])
	}
	if part.members["llanowar elves"] != notElsewhere {
		t.Errorf("the unfiltered copy marked %v in the filtered one", part.members["llanowar elves"])
	}
}
