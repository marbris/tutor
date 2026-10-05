package ui

import (
	"os"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
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

// withTagList turns a tag list on for one test and off again after.
func withTagList(t *testing.T, slug, body string) {
	t.Helper()
	seedDeck(t, slug, body)
	t.Cleanup(func() {
		deck.Delete(slug)
		os.Remove(tagListsPath())
		globalTags = &tagIndex{}
	})
}

func TestTInTheDecksPanelMakesATagListWhoseTagsCountEverywhere(t *testing.T) {
	resetDecks(t)
	withTagList(t, "ramp-cards", "name: ramp-cards\nformat: tags\n[mainboard]\n1 Sol Ring [ramp]\n1 Llanowar Elves [ramp, elf]\n")

	m := openPanel(sized(160, 30), "d", "")
	l := m.ws.current().top().(*deckList)
	for i, r := range l.rows {
		if r.slug == "ramp-cards" {
			l.cursor.at = i
		}
	}
	m = drive(m, "t")
	if !globalTags.active("ramp-cards") {
		t.Fatal("t didn't turn the tag list on")
	}
	found := false
	for _, r := range l.rows {
		if r.kind == entryFolder && r.slug == tagListsFolder {
			found = true
		}
	}
	if !found {
		t.Error("no tag lists folder in the tree")
	}
	if got := loadTagLists(); len(got) != 1 || got[0] != "ramp-cards" {
		t.Errorf("remembered %v", got)
	}

	// A deck with nothing tagged now counts and filters by the list's tags.
	m = withCards(m, "f", sample(), sortArrival)
	dl := m.ws.current().cardsView()
	r, ok := findRow(stats.Groups(effectiveAll(dl.all), effectiveAll(dl.all)), "Tags", "ramp")
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
		t.Error("/ didn't find the tag list's elf tag")
	}
	if len(tagged(dl, "Sol Ring")) != 0 {
		t.Error("the tag list's tags were written into the list")
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

func TestTGBakesTheTagListsIn(t *testing.T) {
	withTagList(t, "ramp-cards", "name: ramp-cards\nformat: tags\n[mainboard]\n1 Sol Ring [ramp]\n")
	m, l := openDeckPanel(t, sized(160, 30), "ghen", "Ghen", sample())
	m.toggleTagList("ramp-cards", "ramp-cards")
	m = drive(m, "T", "g")
	if !hasTag(tagged(l, "Sol Ring"), "ramp") {
		t.Errorf("Sol Ring has %v", tagged(l, "Sol Ring"))
	}
	if !strings.Contains(m.notice, "1 card took") {
		t.Errorf("notice %q", m.notice)
	}
}

func TestTMMergesTheTagsOfEveryListOnScreen(t *testing.T) {
	m, a := openDeckPanel(t, sized(200, 30), "ghen", "Ghen", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"ramp"}},
		{Qty: 1, Card: mtg.Card{Name: "Rancor"}},
	})
	m = withCards(m, "f", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}, Tags: []string{"rock"}},
		{Qty: 1, Card: mtg.Card{Name: "Rancor"}, Tags: []string{"aura"}},
	}, sortArrival)
	search := m.ws.current().cardsView()

	m = drive(m, "T", "m")
	if !hasTag(tagged(a, "Sol Ring"), "rock") || !hasTag(tagged(a, "Rancor"), "aura") {
		t.Errorf("the deck has %v", a.all)
	}
	if hasTag(tagged(search, "Sol Ring"), "ramp") {
		t.Error("a search that isn't yours was changed")
	}
}

func TestTShowsItsMenuAndAnythingElseCancels(t *testing.T) {
	m := withCards(sized(160, 30), "f", sample(), sortArrival)
	m = drive(m, "T")
	if !m.tagPrefix || !strings.Contains(stripANSI(m.View()), "merge tags across lists") {
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
