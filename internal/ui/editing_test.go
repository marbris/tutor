package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// twoDecks opens two decks of yours and a search, with nothing chosen.
func twoDecks(t *testing.T) (Model, *cardList, *cardList) {
	t.Helper()
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)

	m = withCards(m, "d", []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}}, sortArrival)
	a := m.ws.current().cardsView()
	a.deck = &deck.Info{Name: "One", Slug: "one", Format: "commander"}

	m = withCards(m, "d", []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}}, sortArrival)
	b := m.ws.current().cardsView()
	b.deck = &deck.Info{Name: "Two", Slug: "two", Format: "commander"}

	return m, a, b
}

func TestMovingBetweenDecksDoesNotRedirectTheEdits(t *testing.T) {
	// The whole reason for going back to e/E: reading a deck must not
	// silently point a/x/t at it. The target has to be predictable.
	m, _, _ := twoDecks(t)
	m.ws.editing = 1 // "One", chosen

	m = drive(m, "h") // onto "One"
	m = drive(m, "l") // and back onto "Two"

	if m.ws.editing != 1 {
		t.Errorf("looking at another deck moved the target to panel %d", m.ws.editing)
	}
}

func TestECyclesThroughTheDecksYouHaveOpen(t *testing.T) {
	m, _, _ := twoDecks(t)
	m.ws.editing = -1

	m = drive(m, "e")
	first := m.ws.editing
	if first < 0 {
		t.Fatal("e chose nothing")
	}

	m = drive(m, "e")
	if m.ws.editing == first {
		t.Error("a second e did not move on")
	}

	m = drive(m, "e")
	if m.ws.editing != first {
		t.Errorf("e did not wrap back round: %d", m.ws.editing)
	}
}

func TestBigECyclesTheOtherWay(t *testing.T) {
	m, _, _ := twoDecks(t)
	m.ws.editing = -1

	m = drive(m, "E")
	last := m.ws.editing
	m = drive(m, "e")
	if m.ws.editing == last {
		t.Error("e after E did not move")
	}
	m = drive(m, "E")
	if m.ws.editing != last {
		t.Errorf("E did not come back: %d", m.ws.editing)
	}
}

func TestEIsOnlyAboutDecksOfYours(t *testing.T) {
	// A search can't be edited, so e must never land on one.
	m, _, _ := twoDecks(t)
	for i := 0; i < 6; i++ {
		m = drive(m, "e")
		if m.ws.editing == 0 {
			t.Fatal("e landed on the search panel")
		}
	}
}

func TestWithOneDeckOpenTheTargetNeedsNoKeystroke(t *testing.T) {
	// Deriving still fills a hole; it just never chooses between candidates.
	m := withCards(sized(160, 30), "d", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	}, sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Only", Slug: "only", Format: "commander"}
	m.ws.deriveEditing()

	if m.ws.editing != 0 {
		t.Errorf("the only deck open is not the target: %d", m.ws.editing)
	}
}

func TestWithTwoDecksOpenNothingIsChosenForYou(t *testing.T) {
	m, _, _ := twoDecks(t)
	m.ws.editing = -1
	m.ws.deriveEditing()

	if m.ws.editing != -1 {
		t.Errorf("panel %d was chosen without being asked", m.ws.editing)
	}
	if _, why := m.editTarget(); why == "" {
		t.Error("editing was allowed with no target chosen")
	}
}

func TestClosingTheEditingDeckLetsGoOfIt(t *testing.T) {
	m, _, _ := twoDecks(t)
	m.ws.editing = 2
	m = drive(m, "space", "x")

	if m.ws.editing == 2 {
		t.Error("the target still points at the panel that closed")
	}
}

func TestGVOnACardIsAlwaysAboutTheCard(t *testing.T) {
	// Even in a deck of yours. The cursor is on a card, so the question is
	// about that card; a deck's own versions belong to its row in the decks
	// panel, where the cursor is on a deck.
	m := withCards(sized(160, 30), "d", []deck.Card{
		{Qty: 1, Card: mtg.Card{
			Name: "Sol Ring", OracleID: "oid",
			PrintsSearchURI: "https://example.invalid/p",
		}},
	}, sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}

	m = drive(m, "g", "v")
	if m.info.mode != infoVersions {
		t.Fatalf("gv left the panel in mode %v", m.info.mode)
	}
	if _, ok := m.histories["oid"]; !ok {
		t.Error("it went after the deck's versions rather than the card's")
	}
}

func TestSortingByColourSinksTheLands(t *testing.T) {
	l := newCardList([]deck.Card{
		{Qty: 12, Card: mtg.Card{Name: "Forest", TypeLine: "Basic Land — Forest", CMC: 0}},
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact", CMC: 1}},
		{Qty: 1, Card: mtg.Card{Name: "Llanowar Elves", TypeLine: "Creature — Elf",
			CMC: 1, Colors: []string{"G"}}},
	}, sortColor, "")

	last := l.rows[len(l.rows)-1]
	if last.Card.Name != "Forest" {
		t.Errorf("the last row is %s, want the land at the bottom", last.Card.Name)
	}
}

func TestStatisticsKeepUpWithTheList(t *testing.T) {
	// They used to be built once, when s was pressed, and never again — so
	// pressing s while a search was still running left "nothing to count"
	// standing over a panel full of cards.
	m, p := typed(sized(140, 30), "t:elf")
	m = drive(m, "enter")
	m = drive(m, "s")

	if len(m.statGroups()) != 0 {
		t.Fatalf("there was something to count before the results arrived")
	}

	m = answer(m, p, sample(), 4, nil)
	if len(m.statGroups()) == 0 {
		t.Error("the results arrived and the bars stayed empty")
	}
	if strings.Contains(stripANSI(m.View()), "nothing to count") {
		t.Errorf("still says there is nothing to count:\n%s", stripANSI(m.View()))
	}
}
