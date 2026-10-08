package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/paths"
	"ttr/internal/stats"
)

func tagged(l *cardList, name string) []string {
	for _, c := range l.all {
		if c.Card.Name == name {
			return c.Tags
		}
	}
	return nil
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// withPinned pins a list to the global tags for one test, and unpins it after.
func withPinned(t *testing.T, slug, body string) {
	t.Helper()
	seedDeck(t, slug, body)
	t.Cleanup(func() {
		deck.Delete(slug)
		os.Remove(pinnedPath())
		globalTags = &tagIndex{}
	})
}

func TestTInTheDecksPanelPinsAListWhoseTagsCountEverywhere(t *testing.T) {
	resetDecks(t)
	withPinned(t, "ramp-cards", "name: ramp-cards\nformat: tags\n[mainboard]\n1 Sol Ring [ramp]\n1 Llanowar Elves [ramp, elf]\n")

	m := openPanel(sized(160, 30), "d", "")
	l := m.ws.current().top().(*deckList)
	for i, r := range l.rows {
		if r.slug == "ramp-cards" {
			l.cursor.at = i
		}
	}
	m = drive(m, "t")
	if !globalTags.active("ramp-cards") {
		t.Fatal("t didn't pin the list")
	}
	found := false
	for _, r := range l.rows {
		if r.kind == entryFolder && r.slug == globalTagsFolder {
			found = true
		}
	}
	if !found {
		t.Error("no global tags folder in the tree")
	}
	if got := loadPinned(); len(got) != 1 || got[0] != "ramp-cards" {
		t.Errorf("remembered %v", got)
	}

	// A deck with nothing tagged now counts and filters by the list's tags.
	m = withCards(m, "f", sample(), sortArrival)
	dl := m.ws.current().cardsView()
	r, ok := findRow(stats.Groups(effectiveAll(dl.all, dl.lenderKey()), effectiveAll(dl.all, dl.lenderKey())), "Tags", "ramp")
	if !ok || r.Base != 2 {
		t.Fatalf("statistics see ramp %v times", r.Base)
	}
	dl.statFilter, _ = dl.statFilter.Add(stats.And, r)
	dl.refresh()
	if len(dl.rows) != 2 {
		t.Errorf("filtering by ramp left %d cards", len(dl.rows))
	}
	dl.statFilter = nil
	dl.setFilter("elf")
	if len(dl.rows) < 1 {
		t.Error("/ didn't find the pinned list's elf tag")
	}
	if len(tagged(dl, "Sol Ring")) != 0 {
		t.Error("the pinned list's tags were written into the list")
	}
}

func TestTTCopiesTagsIntoTheEditingDeckForCardsItHas(t *testing.T) {
	m, deckList := openDeckPanel(t, sized(160, 30), "ghen", "Ghen", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	})
	m = withCards(m, "f", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"ramp"}},
		{Qty: 1, Card: mtg.Card{Name: "Rancor"}, Tags: []string{"aura"}},
	}, sortArrival)

	m = drive(m, "T", "t", "enter") // empty: every tag
	if !hasTag(tagged(deckList, "Sol Ring"), "ramp") {
		t.Error("Sol Ring didn't take ramp")
	}
	if len(deckList.all) != 1 {
		t.Error("T t added a card")
	}

	m = drive(m, "T", "a", "enter")
	if !hasTag(tagged(deckList, "Rancor"), "aura") || len(deckList.all) != 2 {
		t.Errorf("T a didn't add Rancor with its tag: %v", deckList.all)
	}

	m.undo()
	m.undo()
	if len(deckList.all) != 1 || len(tagged(deckList, "Sol Ring")) != 0 {
		t.Error("two undos didn't put the deck back")
	}
}

func TestTGBakesTheGlobalTagsIn(t *testing.T) {
	withPinned(t, "ramp-cards", "name: ramp-cards\nformat: tags\n[mainboard]\n1 Sol Ring [ramp]\n")
	m, l := openDeckPanel(t, sized(160, 30), "ghen", "Ghen", sample())
	m.togglePin("ramp-cards", "ramp-cards")
	m = drive(m, "T", "g", "enter") // empty: every tag
	if !hasTag(tagged(l, "Sol Ring"), "ramp") {
		t.Errorf("Sol Ring has %v", tagged(l, "Sol Ring"))
	}
	if !strings.Contains(m.notice, "1 card took") {
		t.Errorf("notice %q", m.notice)
	}
}

func TestTShowsItsMenuAndAnythingElseCancels(t *testing.T) {
	m := withCards(sized(160, 30), "f", sample(), sortArrival)
	m = drive(m, "T")
	if !m.tagPrefix || !strings.Contains(stripANSI(m.View()), "otag + cards") {
		t.Error("T didn't raise its menu")
	}
	m = drive(m, "z")
	if m.tagPrefix {
		t.Error("the menu stayed up")
	}
}

func TestTTMovesOnlyTheTagsNamed(t *testing.T) {
	m, deckList := openDeckPanel(t, sized(160, 30), "ghen", "Ghen", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	})
	m = withCards(m, "f", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"ramp", "artifact"}},
		{Qty: 1, Card: mtg.Card{Name: "Rancor"}, Tags: []string{"aura"}},
		{Qty: 1, Card: mtg.Card{Name: "Cultivate"}, Tags: []string{"ramp"}},
	}, sortArrival)

	m = drive(m, "T", "t")
	if p := m.ws.current(); p.asking != askTagMove {
		t.Fatal("T t didn't ask which tags")
	}
	lines := strings.Split(stripANSI(m.View()), "\n")
	if !strings.Contains(lines[0], "empty: every tag") {
		t.Errorf("the top line doesn't say what enter does: %q", lines[0])
	}
	// tab completes from this list's tags.
	m = drive(m, "r", "a", "tab")
	if got := m.ws.current().askInput.Value(); got != "ramp " {
		t.Errorf("tab made %q, want ramp from this list", got)
	}
	m = drive(m, "enter")
	tags := tagged(deckList, "Sol Ring")
	if !hasTag(tags, "ramp") || hasTag(tags, "artifact") {
		t.Errorf("Sol Ring took %v, want ramp and not artifact", tags)
	}

	// T a with a tag adds only the cards carrying it.
	m = drive(m, "T", "a")
	for _, r := range "ramp" {
		m = drive(m, string(r))
	}
	m = drive(m, "enter")
	if len(deckList.all) != 2 || !hasTag(tagged(deckList, "Cultivate"), "ramp") {
		t.Errorf("T a ramp: %d cards; want Cultivate added, not Rancor", len(deckList.all))
	}
}

func TestPinnedListsFromBeforeTheRenameAreStillPinned(t *testing.T) {
	resetDecks(t)
	withPinned(t, "ramp-cards", "name: ramp-cards\n[mainboard]\n1 Sol Ring [ramp]\n")
	old := filepath.Join(paths.State(), oldPinnedFile)
	os.MkdirAll(filepath.Dir(old), 0755)
	if err := os.WriteFile(old, []byte(`["ramp-cards"]`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(old) })

	if got := loadPinned(); len(got) != 1 || got[0] != "ramp-cards" {
		t.Fatalf("taglists.json should still be read, got %v", got)
	}
	savePinned([]string{"ramp-cards"})
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("taglists.json should go once globaltags.json is written")
	}
}

func TestTXClearsTheEditingDecksTagsAndUBringsThemBack(t *testing.T) {
	m, l := openDeckPanel(t, sized(160, 30), "ghen", "Ghen", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"ramp", "artifact"}},
		{Qty: 1, Card: mtg.Card{Name: "Rancor"}, Tags: []string{"aura"}},
	})
	m = drive(m, "T", "X")
	if len(tagged(l, "Sol Ring"))+len(tagged(l, "Rancor")) != 0 {
		t.Errorf("tags left: %v", l.all)
	}
	if !strings.Contains(m.notice, "tags cleared from 2 cards") {
		t.Errorf("notice %q", m.notice)
	}
	m = drive(m, "T", "X")
	if !strings.Contains(m.notice, "no card here has tags") {
		t.Errorf("second T X: notice %q", m.notice)
	}
	m.undo()
	if !hasTag(tagged(l, "Sol Ring"), "ramp") || !hasTag(tagged(l, "Rancor"), "aura") {
		t.Errorf("u didn't bring the tags back: %v", l.all)
	}
}

func TestTXFromAnotherListClearsOnlyThePickedCardsInTheEditingDeck(t *testing.T) {
	m, l := openDeckPanel(t, sized(160, 30), "ghen", "Ghen", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"ramp"}},
		{Qty: 1, Card: mtg.Card{Name: "Rancor"}, Tags: []string{"aura"}},
	})
	m = withCards(m, "f", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
		{Qty: 1, Card: mtg.Card{Name: "Rancor"}},
	}, sortArrival)
	m = drive(m, "v", "T", "X") // v picks the first: Sol Ring
	if len(tagged(l, "Sol Ring")) != 0 {
		t.Errorf("Sol Ring kept %v", tagged(l, "Sol Ring"))
	}
	if !hasTag(tagged(l, "Rancor"), "aura") {
		t.Error("Rancor lost its tag, though it wasn't picked")
	}
}
