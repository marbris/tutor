package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"ttr/internal/deck"
)

func shareOn(t *testing.T) {
	t.Helper()
	if err := setSharingEditing(true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { setSharingEditing(false); os.Remove(sharedPath()) })
}

// openAlso opens one more deck from disk in a panel of its own.
func openAlso(t *testing.T, m Model, slug string) Model {
	t.Helper()
	p := m.ws.open(KindDecks)
	return settle(m, openLocalDeck(p.id, true, slug))
}

func TestTheShareSwitchIsInTheSettingsWithItsKey(t *testing.T) {
	t.Cleanup(func() { setSharingEditing(false) })
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "one editing deck")
	// With ? off, the info panel says the key; with it on, the hints do.
	if view := stripANSI(m.View()); !strings.Contains(view, "enter turns it on or off") {
		t.Errorf("the share row doesn't say its key:\n%s", view)
	}
	m = drive(m, "?")
	if view := stripANSI(m.View()); !strings.Contains(view, "turn on/off") {
		t.Errorf("the hints don't offer the share row's key:\n%s", view)
	}
	m = drive(m, "?")
	m = drive(m, "enter")
	if !sharingEditing() {
		t.Fatal("enter didn't turn sharing on")
	}
	if r, _ := v.current(); r.value != "on" {
		t.Errorf("the row says %q", r.value)
	}
	drive(m, "enter")
	if sharingEditing() {
		t.Error("enter didn't turn sharing off again")
	}
}

func TestEInOneTtrMovesTheEditingDeckInAnother(t *testing.T) {
	withStore(t)
	shareOn(t)
	seedDeck(t, "share-one", "name: One\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	seedDeck(t, "share-two", "name: Two\nformat: commander\n[mainboard]\n1 Llanowar Elves\n")
	t.Cleanup(func() { deck.Delete("share-one"); deck.Delete("share-two") })

	a, _ := openFromDisk(t, "share-one")
	a = openAlso(t, a, "share-two")
	b, _ := openFromDisk(t, "share-one")
	b = openAlso(t, b, "share-two")
	b.ws.editing = 0
	// B takes up whatever was shared before it looked.
	b = settle(b, b.checkShared())

	a.ws.editing = 0
	next, cmd := a.Update(keyMsg("e"))
	a = settle(next.(Model), cmd)
	if a.editingSlug() != "share-two" {
		t.Fatalf("e in A edits %q", a.editingSlug())
	}

	focused := b.ws.focused
	b = settle(b, b.checkShared())
	if b.editingSlug() != "share-two" {
		t.Errorf("B edits %q after A's e", b.editingSlug())
	}
	if b.ws.focused != focused {
		t.Error("following A moved B's focus")
	}
	// B's own choice isn't taken back from it by its own next look.
	if cmd := b.checkShared(); cmd != nil && cmd() != nil {
		t.Error("B took the same choice twice")
	}
}

func TestADeckSharedThatIsntOpenOpensInAPanelOfItsOwn(t *testing.T) {
	withStore(t)
	shareOn(t)
	seedDeck(t, "share-here", "name: Here\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	seedDeck(t, "share-there", "name: There\nformat: commander\n[mainboard]\n1 Llanowar Elves\n")
	t.Cleanup(func() { deck.Delete("share-here"); deck.Delete("share-there") })

	b, _ := openFromDisk(t, "share-here")
	b = settle(b, b.checkShared())
	panels, focused := b.ws.count(), b.ws.focused

	a, _ := openFromDisk(t, "share-there")
	if cmd := a.shareChoice(""); cmd != nil {
		cmd()
	}

	b = settle(b, b.checkShared())
	if b.ws.count() != panels+1 {
		t.Fatalf("%d panels, want one more for the shared deck", b.ws.count())
	}
	if b.editingSlug() != "share-there" {
		t.Errorf("B edits %q", b.editingSlug())
	}
	if b.ws.focused != focused {
		t.Error("the new panel took the focus")
	}
}

func TestWithSharingOffEachTtrKeepsItsOwn(t *testing.T) {
	withStore(t)
	seedDeck(t, "own-one", "name: One\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("own-one"); os.Remove(sharedPath()) })
	a, _ := openFromDisk(t, "own-one")
	if cmd := a.shareChoice(""); cmd != nil {
		t.Error("a choice was shared with the switch off")
	}
	if cmd := a.checkShared(); cmd != nil && cmd() != nil {
		t.Error("a choice was taken with the switch off")
	}
}

// Another ttr's choice wakes this one at once, not on the once-a-second
// look that idles.
func TestTheWatchWakesSoonAfterAChoiceIsShared(t *testing.T) {
	shareOn(t)
	m := sized(160, 30)
	done := make(chan time.Time)
	go func() { m.watchFiles()(); done <- time.Now() }()
	time.Sleep(2 * filesPoll)
	written := time.Now()
	if err := writeShared(sharedEditing{Slug: "elsewhere", By: 7, At: written.UnixNano()}); err != nil {
		t.Fatal(err)
	}
	if woke := (<-done).Sub(written); woke > 3*filesPoll {
		t.Errorf("woke %v after the choice, want within %v", woke, 3*filesPoll)
	}
}
