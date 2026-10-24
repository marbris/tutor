package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// openFromDisk opens a deck the way the decks panel does, file and all.
func openFromDisk(t *testing.T, slug string) (Model, *cardList) {
	t.Helper()
	m := sized(160, 30)
	p := m.ws.open(KindDecks)
	m = settle(m, openLocalDeck(p.id, true, slug))
	l := m.ws.byID(p.id).cardsView()
	if l == nil || l.deck == nil {
		t.Fatalf("%s didn't open: %v", slug, m.ws.byID(p.id).err)
	}
	m.ws.editing = m.ws.indexOf(m.ws.byID(p.id))
	return m, l
}

func joinedNames(cards []deck.Card) string {
	var out []string
	for _, c := range cards {
		out = append(out, c.Card.Name)
	}
	return strings.Join(out, ", ")
}

func TestTwoTtrsEditingOneDeckKeepBothEdits(t *testing.T) {
	withStore(t)
	seedDeck(t, "two-ttrs", "name: Two\nformat: commander\n[mainboard]\n1 Sol Ring\n1 Llanowar Elves\n")
	t.Cleanup(func() { deck.Delete("two-ttrs") })

	a, _ := openFromDisk(t, "two-ttrs")
	b, lb := openFromDisk(t, "two-ttrs")

	a.add([]deck.Card{{Card: mtg.Card{Name: "Dwynen, Gilt-Leaf Daen", TypeLine: "Legendary Creature"}}})
	a = settle(a, a.autosave())
	b.remove([]deck.Card{{Card: mtg.Card{Name: "Sol Ring"}}})
	b = settle(b, b.autosave())

	body := mustRead(t, "two-ttrs").String()
	if !strings.Contains(body, "Dwynen") || strings.Contains(body, "Sol Ring") {
		t.Fatalf("one ttr's edit was written over:\n%s", body)
	}
	// B wrote on top of A's add, so it shows it too.
	if got := joinedNames(lb.all); !strings.Contains(got, "Dwynen") {
		t.Errorf("B shows %s", got)
	}

	// B's u takes back B's removal, not A's add.
	b.undo()
	b = settle(b, b.autosave())
	body = mustRead(t, "two-ttrs").String()
	if !strings.Contains(body, "Dwynen") || !strings.Contains(body, "Sol Ring") {
		t.Errorf("undo in B:\n%s", body)
	}
}

func TestAnotherTtrsEditShowsWithoutReopening(t *testing.T) {
	withStore(t)
	seedDeck(t, "seen-elsewhere", "name: Seen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("seen-elsewhere") })

	a, _ := openFromDisk(t, "seen-elsewhere")
	b, lb := openFromDisk(t, "seen-elsewhere")

	// A nothing changed: a look at the files finds nothing.
	if cmd := b.checkFiles(); cmd != nil && cmd() != nil {
		t.Fatal("a file nobody touched came back changed")
	}

	a.add([]deck.Card{{Card: mtg.Card{Name: "Llanowar Elves", TypeLine: "Creature"}}})
	a = settle(a, a.autosave())
	// A's own write isn't news to A.
	if cmd := a.checkFiles(); cmd != nil && cmd() != nil {
		t.Error("A took its own write for someone else's")
	}

	b = settle(b, b.checkFiles())
	if got := joinedNames(lb.all); !strings.Contains(got, "Llanowar Elves") {
		t.Errorf("B shows %s after looking", got)
	}
	if !strings.Contains(b.notice, "changed in another ttr") {
		t.Errorf("notice %q", b.notice)
	}
	// Taking A's change in is no edit of B's: nothing to undo, nothing to write.
	if len(lb.undo) != 0 || lb.unwritten {
		t.Errorf("undo %d, unwritten %v", len(lb.undo), lb.unwritten)
	}
}
