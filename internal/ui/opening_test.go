package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// seedCache writes a card cache so a deck can be opened without the network.
func seedCache(t *testing.T, cards map[string]mtg.Card) {
	t.Helper()
	body, err := json.Marshal(cards)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deck.CachePath(), body, 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(deck.CachePath()) })
}

// decksPanel opens the decks panel over one seeded deck.
func decksPanel(t *testing.T) (Model, *deckList) {
	t.Helper()
	seedCache(t, map[string]mtg.Card{
		"sol ring":             {Name: "Sol Ring", TypeLine: "Artifact"},
		"ghen, arcanum weaver": {Name: "Ghen, Arcanum Weaver", TypeLine: "Legendary Creature — Human"},
	})
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[commander]\n1 Ghen, Arcanum Weaver\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m := drive(sized(160, 30), "space", "d")
	return m, m.ws.current().top().(*deckList)
}

func TestEnterOpensADeckInThePanelYouAreIn(t *testing.T) {
	// Stepping into it, so esc goes back to the list rather than closing.
	m, l := decksPanel(t)
	l.cursor.at = 0

	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter did nothing")
	}
	m = settle(m, cmd)

	p := m.ws.current()
	if m.ws.count() != 1 {
		t.Errorf("enter opened %d panels", m.ws.count())
	}
	if len(p.stack) != 2 {
		t.Fatalf("the deck did not go on top of the list: %d views", len(p.stack))
	}
	if p.cardsView() == nil {
		t.Fatal("no cards")
	}

	// And esc comes back out to the decks list.
	m = drive(m, "esc")
	if _, ok := m.ws.current().top().(*deckList); !ok {
		t.Errorf("esc left the panel showing %T", m.ws.current().top())
	}
}

func TestLOpensADeckBesideAndLeavesYouWhereYouWere(t *testing.T) {
	// So the list you were browsing is still under the cursor.
	m, l := decksPanel(t)
	l.cursor.at = 0

	m, cmd := press(m, "L")
	if cmd == nil {
		t.Fatal("L did nothing")
	}
	m = settle(m, cmd)

	if m.ws.count() != 2 {
		t.Fatalf("L opened %d panels", m.ws.count())
	}
	if m.ws.focused != 0 {
		t.Errorf("focus moved to panel %d", m.ws.focused)
	}
	if _, ok := m.ws.panels[0].top().(*deckList); !ok {
		t.Error("the decks list is no longer under the cursor")
	}
	if m.ws.panels[1].cardsView() == nil {
		t.Error("the new panel has no cards")
	}
}

func TestAnOpenedDeckBecomesTheOneBeingEdited(t *testing.T) {
	m, l := decksPanel(t)
	l.cursor.at = 0

	m, cmd := press(m, "enter")
	m = settle(m, cmd)

	if m.ws.editing != 0 {
		t.Errorf("the editing deck is panel %d", m.ws.editing)
	}
	if _, why := m.editTarget(); why != "" {
		t.Errorf("it can't be edited: %s", why)
	}
}

func TestOpeningADeckWhoseCardsAreUnknownStillOpensIt(t *testing.T) {
	// A deck that opens missing its two newest cards beats one that won't
	// open at all.
	seedCache(t, map[string]mtg.Card{"sol ring": {Name: "Sol Ring"}})
	seedDeck(t, "part", "name: Part\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("part") })

	m := sized(160, 30)
	p := m.ws.open(KindDecks)
	next, _ := m.Update(deckOpenedMsg{
		panel: p.id,
		info:  deck.Info{Name: "Part", Slug: "part"},
		cards: []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}},
		err:   deck.UnresolvedError{Names: []string{"Nonesuch"}},
	})
	m = next.(Model)

	if p.cardsView() == nil {
		t.Fatal("the deck did not open")
	}
	if !strings.Contains(stripANSI(m.View()), "Nonesuch") {
		t.Errorf("nothing said which card was missing:\n%s", stripANSI(m.View()))
	}
}

func TestDeletingADeckActuallyDeletesIt(t *testing.T) {
	m, l := decksPanel(t)
	l.cursor.at = 0
	if !deck.Exists("ghen") {
		t.Fatal("the deck wasn't there to begin with")
	}

	m = drive(m, "d")
	m, cmd := press(m, "y")
	if cmd == nil {
		t.Fatal("y did nothing")
	}
	m = settle(m, cmd)

	if deck.Exists("ghen") {
		t.Error("the deck is still there")
	}
	if !strings.Contains(stripANSI(m.View()), "deleted") {
		t.Errorf("nothing said so:\n%s", stripANSI(m.View()))
	}
}

func TestRenamingADeckChangesItsTitleAndNotItsFile(t *testing.T) {
	// The slug names a file with a git history; renaming that would lose
	// the history rather than move it.
	m, l := decksPanel(t)
	l.cursor.at = 0

	msg := renameCmd(deckEntry{kind: entryLocal, slug: "ghen", name: "Ghen"}, "Ghen Reborn")().(noticeMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if !deck.Exists("ghen") {
		t.Error("the file moved")
	}
	d, err := deck.Read("ghen")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Ghen Reborn" {
		t.Errorf("the title is %q", d.Name)
	}
	_ = m
}

func TestCWithNoDeckOpenDoesNothing(t *testing.T) {
	// c is a role in a deck you're editing. With no such deck open it says
	// so rather than conjuring a deck up around the card.
	m := withCards(sized(140, 30), "f", []deck.Card{
		{Card: mtg.Card{Name: "Ghen, Arcanum Weaver", TypeLine: "Legendary Creature — Human"}},
	}, sortArrival)

	m, cmd := press(m, "c")
	if cmd != nil {
		t.Fatal("c ran a command with no deck open")
	}
	if deck.Exists("ghen-arcanum-weaver") {
		deck.Delete("ghen-arcanum-weaver")
		t.Fatal("c made a deck when it should have done nothing")
	}
	if !strings.Contains(stripANSI(m.View()), "no deck open to edit") {
		t.Errorf("c did not explain itself:\n%s", stripANSI(m.View()))
	}
}

func TestSaveEverythingWritesEveryDeckWithOutstandingEdits(t *testing.T) {
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	seedDeck(t, "one", "name: One\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	seedDeck(t, "two", "name: Two\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("one"); deck.Delete("two") })

	m := sized(200, 30)
	m, a := openDeckPanel(t, m, "one", "One", []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}})
	m, b := openDeckPanel(t, m, "two", "Two", []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}})

	a.all = append(a.all, deck.Card{Qty: 1, Card: mtg.Card{Name: "Llanowar Elves"}})
	a.dirty = true
	b.all = append(b.all, deck.Card{Qty: 1, Card: mtg.Card{Name: "Birds of Paradise"}})
	b.dirty = true

	if got := len(m.dirtyDecks()); got != 2 {
		t.Fatalf("%d decks are outstanding", got)
	}
	m = settle(m, m.saveEverything())

	for slug, card := range map[string]string{"one": "Llanowar Elves", "two": "Birds of Paradise"} {
		d, err := deck.Read(slug)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(d.String(), card) {
			t.Errorf("%s was not written:\n%s", slug, d.String())
		}
	}
}

func TestOpeningARemoteDeckFollowsIt(t *testing.T) {
	// Looking at somebody's deck is how you decide to follow it, and the
	// alternative is finding your way back to a deck you saw once and can't
	// name.
	t.Cleanup(func() { deck.SaveBookmarks(deck.Bookmarks{}) })

	var b deck.Bookmarks
	b.AddRemote(deck.Remote{Name: "Hinata", ID: "abc"})
	if err := deck.SaveBookmarks(b); err != nil {
		t.Fatal(err)
	}

	// A remote in the bookmarks appears in the decks list as R.
	l := newDeckList()
	found := false
	for _, e := range l.all {
		if e.kind == entryRemote && e.id == "abc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the followed deck is not listed: %v", l.all)
	}
	if got := stripANSI(renderEntry(deckEntry{kind: entryRemote, name: "Hinata", id: "abc"}, 30, false)); !strings.Contains(got, "R") {
		t.Errorf("it is not marked as remote: %q", got)
	}
}

// followed sets up someone followed, with their decks already cached.
func followed(t *testing.T, user string, decks ...deck.UserDeck) {
	t.Helper()
	t.Cleanup(func() {
		deck.SaveBookmarks(deck.Bookmarks{})
		deck.ForgetUserDecks(user)
	})
	var b deck.Bookmarks
	b.AddUser(user)
	if err := deck.SaveBookmarks(b); err != nil {
		t.Fatal(err)
	}
	if err := deck.SaveUserDecks(user, decks); err != nil {
		t.Fatal(err)
	}
}

func rowNames(l *deckList) []string {
	var out []string
	for _, r := range l.rows {
		out = append(out, r.name)
	}
	return out
}

func TestAPersonIsAFolderOfTheirDecks(t *testing.T) {
	followed(t, "MarBri",
		deck.UserDeck{Name: "Hinata", ID: "abc", Cards: 100},
		deck.UserDeck{Name: "Elf Ball", ID: "def", Cards: 100},
	)
	l := newDeckList()
	l.expanded[moxFolder] = true
	l.refresh()

	var person *deckEntry
	for i, r := range l.rows {
		if r.kind == entryUser {
			person = &l.rows[i]
		}
		if r.kind == entryUserDeck {
			t.Errorf("%s shows before the person's folder is opened", r.name)
		}
	}
	if person == nil {
		t.Fatalf("no row for the person: %v", rowNames(l))
	}
	if person.count != 2 {
		t.Errorf("the person's folder counts %d decks", person.count)
	}

	// Opening them drops their decks down beneath, from the cache at once,
	// and asks Moxfield again behind it.
	for i, r := range l.rows {
		if r.kind == entryUser {
			l.cursor.at = i
		}
	}
	m := sized(160, 30)
	p := m.ws.open(KindDecks)
	p.show(l)
	handled, cmd := l.key("enter", &m, p)
	if !handled || cmd == nil {
		t.Errorf("opening a freshly cached person didn't fetch again (cmd %v)", cmd != nil)
	}
	got := strings.Join(rowNames(l), ",")
	if !strings.Contains(got, "Hinata") || !strings.Contains(got, "Elf Ball") {
		t.Errorf("their decks didn't drop down: %s", got)
	}
	if len(p.stack) != 1 {
		t.Error("the person's decks opened somewhere else rather than in the tree")
	}
}

func TestAPersonIsFetchedEachTimeTheFolderOpens(t *testing.T) {
	// Even with a list fetched just now: a deck made since should show.
	followed(t, "MarBri", deck.UserDeck{Name: "Hinata", ID: "abc"})
	l := newDeckList()
	if cmd := l.fetchUserAgain("MarBri"); cmd == nil {
		t.Error("a freshly cached person wasn't fetched again")
	}
	if cmd := l.fetchUserAgain("nobody"); cmd == nil {
		t.Error("a person with no cached list wasn't fetched")
	}
	if !l.fetching["nobody"] {
		t.Error("the fetch isn't marked as under way")
	}
	if cmd := l.fetchUserAgain("nobody"); cmd != nil {
		t.Error("a second open fetched again while the first was under way")
	}
}

func TestAPersonsDecksAreFoundByTheFilter(t *testing.T) {
	// Without opening their folder, and by their name too.
	followed(t, "MarBri",
		deck.UserDeck{Name: "Elf Ball", ID: "a"},
		deck.UserDeck{Name: "Goblin Rush", ID: "b"},
	)
	l := newDeckList()
	l.setFilter("elf")
	if got := rowNames(l); len(got) != 1 || got[0] != "Elf Ball" {
		t.Errorf("filtering to elf: %v", got)
	}
	l.setFilter("marbri goblin")
	if got := rowNames(l); len(got) != 1 || got[0] != "Goblin Rush" {
		t.Errorf("filtering by the person: %v", got)
	}
}

func TestAFollowedDeckInAPersonsListIsListedOnce(t *testing.T) {
	followed(t, "MarBri", deck.UserDeck{Name: "Elf Ball", ID: "a"})
	b := deck.LoadBookmarks()
	b.AddRemote(deck.Remote{Name: "Elf Ball", ID: "a"})
	b.AddRemote(deck.Remote{Name: "Other", ID: "z"})
	if err := deck.SaveBookmarks(b); err != nil {
		t.Fatal(err)
	}
	l := newDeckList()
	n := 0
	for _, e := range l.all {
		if e.id == "a" {
			n++
			if e.kind != entryUserDeck {
				t.Errorf("listed as %v, want under the person", e.kind)
			}
		}
	}
	if n != 1 {
		t.Errorf("listed %d times", n)
	}
}

func TestTheMoxfieldFolderComesFirst(t *testing.T) {
	seedDeck(t, "aaa/first", "name: First\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { resetDecks(t) })
	followed(t, "MarBri")

	l := newDeckList()
	if len(l.rows) == 0 || l.rows[0].slug != moxFolder {
		t.Errorf("rows start %v", rowNames(l))
	}
}
