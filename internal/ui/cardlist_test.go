package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

func sample() []deck.Card {
	return []deck.Card{
		{Qty: 1, Commander: true, Card: mtg.Card{
			Name: "Dwynen, Gilt-Leaf Daen", TypeLine: "Legendary Creature — Elf Archer",
			ManaCost: "{2}{G}{G}", CMC: 4, Colors: []string{"G"}, Rarity: "rare",
			OracleText: "Other Elves you control get +1/+1.", EDHRECRank: 900,
		}},
		{Qty: 1, Card: mtg.Card{
			Name: "Llanowar Elves", TypeLine: "Creature — Elf Druid",
			ManaCost: "{G}", CMC: 1, Colors: []string{"G"}, Rarity: "common",
			OracleText: "{T}: Add {G}.", EDHRECRank: 200,
		}},
		{Qty: 1, Card: mtg.Card{
			Name: "Sol Ring", TypeLine: "Artifact", ManaCost: "{1}", CMC: 1,
			Rarity: "uncommon", OracleText: "{T}: Add {C}{C}.", EDHRECRank: 1,
		}},
		{Qty: 7, Card: mtg.Card{
			Name: "Forest", TypeLine: "Basic Land — Forest", CMC: 0, Rarity: "common",
		}},
	}
}

func names(l *cardList) []string {
	var out []string
	for _, c := range l.rows {
		out = append(out, c.Card.Name)
	}
	return out
}

func TestArrivalOrderIsLeftAlone(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	got := strings.Join(names(l), ",")
	want := "Dwynen, Gilt-Leaf Daen,Llanowar Elves,Sol Ring,Forest"
	if got != want {
		t.Errorf("got %s, want the order they came in", got)
	}
}

func TestSortingByManaValue(t *testing.T) {
	l := newCardList2(sample(), sortMana)
	if got := names(l)[0]; got != "Forest" {
		t.Errorf("cheapest is %s, want the zero-cost land", got)
	}
	if got := names(l)[3]; got != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("dearest is %s", got)
	}
}

func TestSortingByManaValueCountsXHigh(t *testing.T) {
	// Fireball's mana value is 1, but nobody casts it for one.
	cards := []deck.Card{
		{Card: mtg.Card{Name: "Fireball", ManaCost: "{X}{R}", CMC: 1}},
		{Card: mtg.Card{Name: "Hoof", ManaCost: "{5}{G}{G}{G}", CMC: 8}},
		{Card: mtg.Card{Name: "Bolt", ManaCost: "{R}", CMC: 1}},
	}
	l := newCardList2(cards, sortMana)
	if got := strings.Join(names(l), ","); got != "Bolt,Hoof,Fireball" {
		t.Errorf("by mana value: %s, want the X spell above the eight-drop", got)
	}
}

func TestSortingByUSDPutsTheDearestFirst(t *testing.T) {
	cards := []deck.Card{
		{Card: mtg.Card{Name: "Cheap", Prices: mtg.Prices{USD: "0.10"}}},
		{Card: mtg.Card{Name: "Priceless"}}, // no price at all
		{Card: mtg.Card{Name: "Dear", Prices: mtg.Prices{USD: "50.00"}}},
	}
	l := newCardList2(cards, sortUSD)
	got := names(l)
	if got[0] != "Dear" || got[1] != "Cheap" {
		t.Errorf("usd sort put them %v, want dearest first", got)
	}
	// A card with no known price sorts last, not as though it were free.
	if got[2] != "Priceless" {
		t.Errorf("the priceless card is at %v, want the bottom", got)
	}
}

func TestCommandersSortWithEveryoneElse(t *testing.T) {
	// They lead a decklist because that's how a decklist is built, not
	// because anything pins them there.
	l := newCardList2(sample(), sortMana)
	if names(l)[0] == "Dwynen, Gilt-Leaf Daen" {
		t.Error("the commander was pinned to the top of a mana-value sort")
	}
}

func TestSortingKeepsTheCursorOnTheSameCard(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.move(2) // Sol Ring
	if c, _ := l.current(); c.Card.Name != "Sol Ring" {
		t.Fatalf("cursor is on %s", c.Card.Name)
	}

	l.cycleSort(1)
	if c, _ := l.current(); c.Card.Name != "Sol Ring" {
		t.Errorf("after re-sorting the cursor is on %s, want Sol Ring", c.Card.Name)
	}
}

func TestFilteringIsLiteralAndSearchesTheRulesText(t *testing.T) {
	l := newCardList2(sample(), sortArrival)

	l.setFilter("elf")
	if got := len(l.rows); got != 2 {
		t.Errorf("'elf' matched %d cards (%v), want the two Elves", got, names(l))
	}

	l.setFilter("add")
	if got := len(l.rows); got != 2 {
		t.Errorf("'add' matched %d cards (%v), want the two manadorks", got, names(l))
	}
}

func TestTheHeaderCountsCopiesNotRows(t *testing.T) {
	// The sample is four rows but ten cards — the 7x Forest is seven of them.
	// "how many cards" is what a deck's size answers, so the header sums them.
	l := newCardList2(sample(), sortArrival)
	if got := l.cardCount(); got != 10 {
		t.Errorf("cardCount = %d, want 10 copies across the four rows", got)
	}

	// Filtered down, the numerator is the copies still shown and the
	// denominator the copies in the whole list: the Forests, then all ten.
	l.setFilter("forest")
	if got := stripANSI(l.subtitle()); !strings.HasPrefix(got, "7/10") {
		t.Errorf("subtitle is %q, want it to open 7/10", got)
	}
}

func TestFilterTermsAllHaveToMatch(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.setFilter("elf druid")
	if got := names(l); len(got) != 1 || got[0] != "Llanowar Elves" {
		t.Errorf("got %v, want only the Elf Druid", got)
	}
}

func TestAQuotedPhraseStaysTogether(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.setFilter(`"elf archer"`)
	if got := names(l); len(got) != 1 || got[0] != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("got %v, want the one Elf Archer", got)
	}
}

func TestClearingAFilterBringsEverythingBack(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.setFilter("elf")
	l.setFilter("")
	if got := len(l.rows); got != 4 {
		t.Errorf("%d cards came back, want all 4", got)
	}
}

func TestMarksSurviveFilteringAndSorting(t *testing.T) {
	// Marks are held by name for exactly this reason: the rows they were
	// made on get rebuilt underneath them constantly.
	l := newCardList2(sample(), sortArrival)
	l.toggleMark() // Dwynen

	l.setFilter("forest")
	l.cycleSort(1)
	l.setFilter("")

	if !l.marked(sample()[0]) {
		t.Error("the mark did not survive being filtered and re-sorted")
	}
	if l.markCount() != 1 {
		t.Errorf("%d marks, want 1", l.markCount())
	}
}

func TestMarkingStepsDownSoVVVTakesThree(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.toggleMark()
	l.toggleMark()
	l.toggleMark()
	if l.markCount() != 3 {
		t.Errorf("%d marks after three presses, want 3", l.markCount())
	}
}

func TestMarkAllTakesWhatTheFilterLeft(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.setFilter("elf")
	l.markAll()
	if l.markCount() != 2 {
		t.Errorf("%d marks, want just the two the filter left", l.markCount())
	}

	// And lets them all go again.
	l.markAll()
	if l.markCount() != 0 {
		t.Errorf("%d marks after a second V", l.markCount())
	}
}

func TestSelectionFallsBackToTheCursor(t *testing.T) {
	// Every command that acts on "the selected cards" needs an answer when
	// nothing is selected, and the answer is the row you're looking at.
	l := newCardList2(sample(), sortArrival)
	sel := l.selection()
	if len(sel) != 1 || sel[0].Card.Name != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("got %v, want the card under the cursor", sel)
	}

	l.toggleMark()
	l.toggleMark()
	if got := len(l.selection()); got != 2 {
		t.Errorf("selection is %d cards, want the 2 marked", got)
	}
}

func TestSelectionIsInArrivalOrderNotCursorOrder(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.move(2)
	l.toggleMark() // Sol Ring, third in the list
	l.top()
	l.toggleMark() // Dwynen, first

	sel := l.selection()
	if len(sel) != 2 {
		t.Fatalf("selection is %d cards, want 2", len(sel))
	}
	if sel[0].Card.Name != "Dwynen, Gilt-Leaf Daen" || sel[1].Card.Name != "Sol Ring" {
		t.Errorf("got %s then %s, want them in the order the deck lists them",
			sel[0].Card.Name, sel[1].Card.Name)
	}
}

func TestTheCursorStaysInsideTheList(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.move(-5)
	if l.cursor.at != 0 {
		t.Errorf("cursor ran off the top to %d", l.cursor.at)
	}
	l.move(99)
	if l.cursor.at != 3 {
		t.Errorf("cursor ran off the bottom to %d", l.cursor.at)
	}
}

func TestScrollingFollowsTheCursorMinimally(t *testing.T) {
	many := make([]deck.Card, 50)
	for i := range many {
		many[i] = deck.Card{Card: mtg.Card{Name: "Card " + itoa(i)}}
	}
	l := newCardList(many, sortArrival, "")

	l.cursor.scrollInto(10, len(l.rows))
	if l.cursor.offset != 0 {
		t.Errorf("offset %d at the top of the list", l.cursor.offset)
	}

	l.move(9)
	l.cursor.scrollInto(10, len(l.rows))
	if l.cursor.offset != 0 {
		t.Errorf("offset %d, want the view held still while the cursor is in it", l.cursor.offset)
	}

	l.move(1)
	l.cursor.scrollInto(10, len(l.rows))
	if l.cursor.offset != 1 {
		t.Errorf("offset %d, want one line of scroll and no more", l.cursor.offset)
	}
}

func TestAnEmptyFilterResultDoesNotBreakTheCursor(t *testing.T) {
	l := newCardList2(sample(), sortArrival)
	l.setFilter("nonesuch")
	if l.count() != 0 {
		t.Fatalf("%d rows matched nonsense", l.count())
	}
	if _, ok := l.current(); ok {
		t.Error("there is a current card in an empty list")
	}
	l.move(1)
	l.render(20, 5, nil, true) // must not panic
}

// newCardList2 is newCardList with the arrival name left to its default, so
// the tests above stay about behaviour rather than labels.
func newCardList2(cards []deck.Card, order cardSort) *cardList {
	return newCardList(cards, order, "")
}
