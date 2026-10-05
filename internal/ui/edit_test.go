package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// editing sets up the usual arrangement: a search on the left, a deck of
// yours on the right, the deck being the one edited.
func editing(t *testing.T) (Model, *cardList, *cardList) {
	t.Helper()

	m := sized(160, 30)
	m = withCards(m, "f", sample(), sortArrival)
	search := m.ws.current().cardsView()

	m = withCards(m, "d", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}},
	}, sortArrival)
	target := m.ws.current().cardsView()
	target.deck = &deck.Info{Name: "Elf Ball", Slug: "elf-ball", Format: "commander"}
	m.ws.editing = 1

	return m, search, target
}

func TestAddingACardFromASearch(t *testing.T) {
	m, search, target := editing(t)

	m = focusOn(m, 0) // the search
	search.selectByName("Llanowar Elves")
	m = drive(m, "a")

	if target.indexOfCard("Llanowar Elves") < 0 {
		t.Fatal("the card did not reach the deck")
	}
	if !strings.Contains(stripANSI(m.View()), "+1 Llanowar Elves") {
		t.Error("nothing said what happened")
	}
}

func TestAddingFromAnotherListAddsOnceAndInTheDeckAddsCopies(t *testing.T) {
	// From another list, a card the deck has is never doubled by accident;
	// in the deck itself, a a a on a basic land gives you three of it.
	m, search, target := editing(t)
	m = focusOn(m, 0)
	search.selectByName("Forest")

	m = drive(m, "a", "a", "a")
	i := target.indexOfCard("Forest")
	if i < 0 {
		t.Fatal("the land is not in the deck")
	}
	if target.all[i].Qty != 1 {
		t.Errorf("a from a search gave %d copies, want 1", target.all[i].Qty)
	}
	if !strings.Contains(m.notice, "already") {
		t.Errorf("notice %q should say the deck has it", m.notice)
	}

	m = focusOn(m, 1)
	target.selectByName("Forest")
	m = drive(m, "a", "a")
	if target.all[i].Qty != 3 {
		t.Errorf("a in the deck gave %d copies, want 3", target.all[i].Qty)
	}
}

func TestAddingFromAnotherDeckBringsItsTags(t *testing.T) {
	m, search, target := editing(t)
	search.all[2].Tags = []string{"ramp"} // Sol Ring, which the deck has
	search.refresh()
	m = focusOn(m, 0)
	search.selectByName("Sol Ring")
	m = drive(m, "a")
	i := target.indexOfCard("Sol Ring")
	if target.all[i].Qty != 1 || !hasTag(target.all[i].Tags, "ramp") {
		t.Errorf("Sol Ring should take ramp and no copy: %+v", target.all[i])
	}
	m.undo()
	if len(target.all[i].Tags) != 0 {
		t.Error("u didn't take the tag off again")
	}
}

func TestRemovingTakesOneCopyThenTheRow(t *testing.T) {
	m, search, target := editing(t)
	m = focusOn(m, 0)
	search.selectByName("Sol Ring")

	m = focusOn(m, 1)
	target.selectByName("Sol Ring")
	m = drive(m, "a") // now two
	m = drive(m, "x")
	if i := target.indexOfCard("Sol Ring"); i < 0 || target.all[i].Qty != 1 {
		t.Fatalf("after one x the deck holds %v", target.all)
	}
	m = drive(m, "x")
	if target.indexOfCard("Sol Ring") >= 0 {
		t.Error("the last copy did not take the row with it")
	}
}

func TestAddingWorksOnEverythingPickedOut(t *testing.T) {
	m, _, target := editing(t)
	m = focusOn(m, 0)
	m = drive(m, "v", "v") // two cards, and v steps down

	m = drive(m, "a")
	if len(target.all) != 3 {
		t.Errorf("the deck holds %d cards, want the original plus two", len(target.all))
	}
}

func TestEditingWithNoDeckOpenSaysSo(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "a")
	if !strings.Contains(stripANSI(m.View()), "no deck open") {
		t.Errorf("got:\n%s", stripANSI(m.View()))
	}
}

func TestSomebodyElsesDeckCannotBeEdited(t *testing.T) {
	m := withCards(sized(120, 30), "d", sample(), sortArrival)
	l := m.ws.current().cardsView()
	l.deck = &deck.Info{Name: "Borrowed", ID: "Y8dZ7"} // no slug: not yours
	m.ws.editing = 0

	m = drive(m, "a")
	if !strings.Contains(stripANSI(m.View()), "isn't yours") {
		t.Errorf("got:\n%s", stripANSI(m.View()))
	}
}

func TestUndoWalksBack(t *testing.T) {
	m, search, target := editing(t)
	m = focusOn(m, 0)
	search.selectByName("Llanowar Elves")

	m = drive(m, "a")
	m = drive(m, "u")
	if target.indexOfCard("Llanowar Elves") >= 0 {
		t.Error("undo left the card in the deck")
	}
	if !strings.Contains(stripANSI(m.View()), "undid") {
		t.Error("undo said nothing")
	}
}

func TestAnEditThatChangedNothingIsNotUndoable(t *testing.T) {
	// Removing a card that isn't there shouldn't leave a step that undoes
	// the edit before it.
	m, search, target := editing(t)
	m = focusOn(m, 0)
	search.selectByName("Llanowar Elves")

	m = drive(m, "a")             // a real edit
	search.selectByName("Forest") // not in the deck
	m = drive(m, "x")             // does nothing
	m = drive(m, "u")             // should undo the add

	if target.indexOfCard("Llanowar Elves") >= 0 {
		t.Error("undo walked back the wrong step")
	}
}

func TestYankAndPutMoveCardsBetweenTwoDecks(t *testing.T) {
	// Neither of them has to be the deck you're building — that's the point
	// of having y and p as well as a.
	m, search, target := editing(t)
	m = focusOn(m, 0)
	search.selectByName("Dwynen, Gilt-Leaf Daen")

	m = drive(m, "y")
	if len(m.register) != 1 {
		t.Fatalf("the register holds %d cards", len(m.register))
	}

	m = focusOn(m, 1)
	m = drive(m, "p")
	if target.indexOfCard("Dwynen, Gilt-Leaf Daen") < 0 {
		t.Error("put did not land the card")
	}
}

func TestTheRegisterCarriesQuantityAndTags(t *testing.T) {
	// A card moved between decks brings what you knew about it, not just
	// its name.
	m, _, target := editing(t)
	m.register = []deck.Card{{
		Qty: 4, Tags: []string{"ramp"},
		Card: mtg.Card{Name: "Rampant Growth"},
	}}

	m = focusOn(m, 1)
	m = drive(m, "p")

	i := target.indexOfCard("Rampant Growth")
	if i < 0 {
		t.Fatal("nothing was put")
	}
	if target.all[i].Qty != 4 {
		t.Errorf("quantity is %d, want the 4 it was yanked with", target.all[i].Qty)
	}
	if len(target.all[i].Tags) != 1 || target.all[i].Tags[0] != "ramp" {
		t.Errorf("tags are %v", target.all[i].Tags)
	}
}

func TestPuttingIntoSomethingThatIsNotYours(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m.register = []deck.Card{{Card: mtg.Card{Name: "Sol Ring"}}}
	m = drive(m, "p")
	if !strings.Contains(stripANSI(m.View()), "needs a deck you can edit") {
		t.Errorf("got:\n%s", stripANSI(m.View()))
	}
}

func TestTaggingTheCardsInTheDeck(t *testing.T) {
	m, _, target := editing(t)
	m = focusOn(m, 1)
	m = drive(m, "t")
	if m.ws.current().asking != askTag {
		t.Fatal("t did not ask for a tag")
	}
	for _, r := range "ramp" {
		m = drive(m, string(r))
	}
	m = drive(m, "enter")

	i := target.indexOfCard("Sol Ring")
	if i < 0 || len(target.all[i].Tags) != 1 || target.all[i].Tags[0] != "ramp" {
		t.Errorf("tags are %v", target.all[i].Tags)
	}
}

func TestATagCanBeTakenOffAgain(t *testing.T) {
	m, _, target := editing(t)
	m.tag([]deck.Card{target.all[0]}, "ramp draw")
	m.tag([]deck.Card{target.all[0]}, "-draw")

	i := target.indexOfCard("Sol Ring")
	if got := strings.Join(target.all[i].Tags, ","); got != "ramp" {
		t.Errorf("tags are %q, want just ramp", got)
	}
}

func TestTaggingSkipsCardsThatArentInTheDeck(t *testing.T) {
	// A tag is something a deck's author said about a card in their deck.
	m, search, _ := editing(t)
	m.tag([]deck.Card{search.all[1]}, "ramp") // Llanowar Elves, not in the deck
	if !strings.Contains(m.notice, "none of those are in the deck") {
		t.Errorf("notice is %q", m.notice)
	}
}

func TestBigAAddsAndTagsInOneKey(t *testing.T) {
	// Sorting a search into a deck is dozens of these, and retyping the tag
	// each time is what makes people stop.
	m, search, target := editing(t)
	m.lastTag = "removal"

	m = focusOn(m, 0)
	search.selectByName("Llanowar Elves")
	m = drive(m, "A", "enter") // the prompt opens with the last tag

	i := target.indexOfCard("Llanowar Elves")
	if i < 0 {
		t.Fatal("A did not add the card")
	}
	if len(target.all[i].Tags) != 1 || target.all[i].Tags[0] != "removal" {
		t.Errorf("tags are %v, want the last one used", target.all[i].Tags)
	}
}

func TestBigAWithNothingTypedJustAdds(t *testing.T) {
	m, search, target := editing(t)
	m = focusOn(m, 0)
	search.selectByName("Llanowar Elves")
	m = drive(m, "A")
	if m.ws.current().asking != askAddTag || m.ws.current().askInput.Value() != "" {
		t.Fatal("A with no tag used yet should ask, with nothing filled in")
	}
	m = drive(m, "r", "a", "m", "p", "enter")
	i := target.indexOfCard("Llanowar Elves")
	if i < 0 || !hasTag(target.all[i].Tags, "ramp") {
		t.Fatalf("A ramp didn't add and tag: %v", target.all)
	}
	if m.lastTag != "ramp" {
		t.Errorf("last tag is %q", m.lastTag)
	}

	search.selectByName("Forest")
	m = drive(m, "A")
	m.ws.current().askInput.SetValue("")
	m = drive(m, "enter")
	if j := target.indexOfCard("Forest"); j < 0 || len(target.all[j].Tags) != 0 {
		t.Error("A with the prompt emptied should just add")
	}
}

func TestSettingACommander(t *testing.T) {
	m, _, target := editing(t)
	m = focusOn(m, 1)
	m = drive(m, "c")

	i := target.indexOfCard("Sol Ring")
	if !target.all[i].Commander {
		t.Error("c did not put the card in the command zone")
	}
	m = drive(m, "c")
	if target.all[i].Commander {
		t.Error("c did not toggle back off")
	}
}

func TestEditsMarkTheDeckUnsaved(t *testing.T) {
	m, search, target := editing(t)
	if target.dirty {
		t.Fatal("the deck started out dirty")
	}
	m = focusOn(m, 0)
	search.selectByName("Llanowar Elves")
	m = drive(m, "a")

	if !target.dirty {
		t.Error("the edit did not mark the deck unsaved")
	}
	if !strings.Contains(stripANSI(m.View()), "uncommitted") {
		t.Error("the panel does not say it needs committing")
	}
}

func TestQuittingAsksAboutUnsavedEdits(t *testing.T) {
	// Saving is explicit, so this is the price of that.
	m, search, _ := editing(t)
	m = focusOn(m, 0)
	search.selectByName("Llanowar Elves")
	m = drive(m, "a")

	m = focusOn(m, 1)
	next, cmd := m.Update(keyMsg("q"))
	m = next.(Model)
	if cmd != nil {
		t.Error("q quit with edits outstanding")
	}
	if !m.quitting {
		t.Fatal("q did not raise the question")
	}
	if !strings.Contains(stripANSI(m.View()), "uncommitted edits") {
		t.Error("the question is not on screen")
	}
}

func TestQuittingWithNothingOutstandingJustQuits(t *testing.T) {
	m, _, _ := editing(t)
	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("q asked a question it had no reason to ask")
	}
}

func TestTheQuitQuestionCannotBeAnsweredByAccident(t *testing.T) {
	m, search, _ := editing(t)
	m = focusOn(m, 0)
	search.selectByName("Llanowar Elves")
	m = drive(m, "a")

	next, _ := m.Update(keyMsg("q"))
	m = next.(Model)
	next, cmd := m.Update(keyMsg("j")) // anything that isn't y or w
	m = next.(Model)

	if cmd != nil {
		t.Error("a stray key quit the program")
	}
	if m.quitting {
		t.Error("the question is still standing")
	}
}

func TestEditingRechecksLegality(t *testing.T) {
	// Adding a card is exactly the thing that makes a deck illegal, so the
	// answer has to keep up with the edits rather than waiting for a save.
	m, search, target := editing(t)
	target.deck.Format = "commander"
	target.recheck()

	if target.legality == nil || target.legality.Legal {
		t.Fatal("a one-card deck should not be a legal Commander deck")
	}
	before := len(target.legality.Problems)

	m = focusOn(m, 0)
	search.selectByName("Llanowar Elves")
	m = drive(m, "a")

	if target.legality == nil {
		t.Fatal("the verdict went missing after an edit")
	}
	if len(target.legality.Problems) == 0 && before > 0 {
		t.Error("the verdict was not recomputed")
	}
}

func TestAnIllegalDeckSaysSoWhereYouAreWorkingOnIt(t *testing.T) {
	m, _, target := editing(t)
	target.deck.Format = "commander"
	target.recheck()

	if !strings.Contains(stripANSI(m.View()), "illegal") {
		t.Errorf("the panel does not flag it:\n%s", stripANSI(m.View()))
	}
}

func TestATagWithASpaceCanBeTypedIfQuoted(t *testing.T) {
	// A deck imported from Moxfield can arrive carrying "fast mana"; without
	// quotes it could be read but never removed.
	add, remove := parseTagEdit(`"fast mana" ramp -"card draw"`)
	if len(add) != 2 || add[0] != "fast mana" || add[1] != "ramp" {
		t.Errorf("adding %v", add)
	}
	if len(remove) != 1 || remove[0] != "card draw" {
		t.Errorf("removing %v", remove)
	}
}

func TestTagEditsAreLowercased(t *testing.T) {
	// So "Ramp" and "ramp" are one category in the statistics rather than
	// two bars saying the same thing.
	add, _ := parseTagEdit("Ramp RAMP")
	for _, tag := range add {
		if tag != "ramp" {
			t.Errorf("got %q", tag)
		}
	}
}
