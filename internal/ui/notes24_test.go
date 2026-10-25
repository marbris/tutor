package ui

import (
	"testing"

	"ttr/internal/deck"
)

// sharedDecks opens deck one and deck two, which has most of one's cards,
// with one being edited and sorted by inclusion.
func sharedDecks(t *testing.T) (Model, *cardList) {
	t.Helper()
	withStore(t)
	seedDeck(t, "n24-one", "name: One\nformat: commander\n[mainboard]\n1 Sol Ring\n1 Llanowar Elves\n1 Forest\n1 Island\n1 Swamp\n1 Mountain\n1 Plains\n")
	seedDeck(t, "n24-two", "name: Two\nformat: commander\n[mainboard]\n1 Llanowar Elves\n1 Forest\n1 Island\n1 Swamp\n")
	t.Cleanup(func() { deck.Delete("n24-one"); deck.Delete("n24-two") })
	m, l := openFromDisk(t, "n24-one")
	m = openAlso(t, m, "n24-two")
	m.ws.editing = 0
	l.order = sortInclusion
	return m, l
}

// pointAt settles the lists with a key that changes nothing, then puts the
// cursor on a card.
func pointAt(t *testing.T, m Model, l *cardList, name string) Model {
	t.Helper()
	m = drive(m, "?", "?")
	if !l.selectByName(name) {
		t.Fatalf("no %s in %v", name, cardNames(l))
	}
	return m
}

func backAndForth(t *testing.T, m Model, l *cardList, name string) {
	t.Helper()
	for range 3 {
		m = drive(m, "l", "h")
		if c, _ := l.current(); c.Card.Name != name {
			t.Fatalf("after l h the cursor is on %s, want %s", c.Card.Name, name)
		}
	}
}

func TestGoingBackAndForthKeepsTheEditingDecksCard(t *testing.T) {
	m, l := sharedDecks(t)
	m = focusOn(m, 0)
	m = pointAt(t, m, l, "Island")
	backAndForth(t, m, l, "Island")
}

// The note's case: the editing deck in two panels, and a list that isn't.
func TestGoingBackAndForthFromACopyOfTheEditingDeck(t *testing.T) {
	m, _ := sharedDecks(t)
	m = focusOn(m, 0)
	m = openAlso(t, m, "n24-one")
	if m.ws.panels[1].cardsView().deck.Slug != "n24-one" || m.ws.panels[2].cardsView().deck.Slug != "n24-two" {
		t.Fatal("the copy didn't open between the two decks")
	}
	m = focusOn(m, 1)
	copy := m.ws.panels[1].cardsView()
	copy.order = sortInclusion
	m = pointAt(t, m, copy, "Island")
	backAndForth(t, m, copy, "Island")
}

func TestEKeepsTheCursorCard(t *testing.T) {
	m, l := sharedDecks(t)
	m = focusOn(m, 1)
	m = pointAt(t, m, l, "Island")
	m = drive(m, "e")
	if m.editingSlug() != "n24-two" {
		t.Fatalf("e went to %q", m.editingSlug())
	}
	if c, _ := l.current(); c.Card.Name != "Island" {
		t.Fatalf("after e the cursor is on %s, want Island", c.Card.Name)
	}
	m = drive(m, "E")
	if c, _ := l.current(); c.Card.Name != "Island" {
		t.Errorf("after E the cursor is on %s, want Island", c.Card.Name)
	}
}
