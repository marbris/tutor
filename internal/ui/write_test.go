package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// openDeckPanel puts a real deck of yours in a panel and marks it the one
// being edited.
func openDeckPanel(t *testing.T, m Model, slug, name string, cards []deck.Card) (Model, *cardList) {
	t.Helper()
	m = withCards(m, "d", cards, sortArrival)
	l := m.ws.current().cardsView()
	l.deck = &deck.Info{Name: name, Slug: slug, Format: "commander"}
	m.ws.editing = m.ws.focused
	return m, l
}

func TestWritingADeckReachesDiskAndGit(t *testing.T) {
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	seedDeck(t, "elf-ball", "name: Elf Ball\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("elf-ball") })

	m, l := openDeckPanel(t, sized(160, 24), "elf-ball", "Elf Ball", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}},
	})

	m.add([]deck.Card{{Card: mtg.Card{Name: "Llanowar Elves"}}})
	m.tag([]deck.Card{{Card: mtg.Card{Name: "Llanowar Elves"}}}, "ramp")

	msg := m.saveDeck(m.ws.current().id, *l.deck, l.all)().(deckSavedMsg)
	if msg.err != nil {
		t.Fatalf("saving failed: %v", msg.err)
	}

	body, err := deck.Read("elf-ball")
	if err != nil {
		t.Fatal(err)
	}
	// The tag has to survive the round trip, or tagging is decoration.
	if !strings.Contains(body.String(), "Llanowar Elves [ramp]") {
		t.Errorf("the file says:\n%s", body.String())
	}

	commits, err := deck.History("elf-ball", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) == 0 {
		t.Fatal("nothing was committed")
	}
	// One commit per w, saying what moved — not "update deck".
	if !strings.Contains(commits[0].Subject, "Llanowar Elves") {
		t.Errorf("the commit says %q", commits[0].Subject)
	}
}

func TestSavingClearsTheUnsavedMark(t *testing.T) {
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m, l := openDeckPanel(t, sized(160, 24), "ghen", "Ghen", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	})
	m.add([]deck.Card{{Card: mtg.Card{Name: "Llanowar Elves"}}})
	if !l.dirty {
		t.Fatal("the edit did not mark it unsaved")
	}

	msg := m.saveDeck(m.ws.current().id, *l.deck, l.all)().(deckSavedMsg)
	next, _ := m.handleDeckSaved(msg)
	m = next.(Model)

	if l.dirty {
		t.Error("saving did not clear the unsaved mark")
	}
	if len(m.dirtyDecks()) != 0 {
		t.Error("quitting would still ask")
	}
}

func TestWritingASearchAsksForAName(t *testing.T) {
	// It isn't yours yet, so it needs a name before it can be — :w versus
	// :w <name>.
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "w")

	p := m.ws.current()
	if p.asking != askWrite {
		t.Fatalf("w did not ask for a name (asking = %v)", p.asking)
	}
}

func TestSavingASearchMakesADeckOfYours(t *testing.T) {
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	msg := saveAsNew(1, false, "Elf Search", sample())().(deckWrittenMsg)
	t.Cleanup(func() { deck.Delete(msg.slug) })

	d, err := deck.Read(msg.slug)
	if err != nil {
		t.Fatalf("nothing was written: %v", err)
	}
	body := d.String()

	// The commander goes to the command zone, and quantities survive.
	if !strings.Contains(body, "[commander]") || !strings.Contains(body, "Dwynen") {
		t.Errorf("the commander did not reach the command zone:\n%s", body)
	}
	if !strings.Contains(body, "7 Forest") {
		t.Errorf("quantities were lost:\n%s", body)
	}
}

func TestSavingTwiceDoesNotOverwriteTheFirst(t *testing.T) {
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	first := saveAsNew(1, false, "Elves", sample())().(deckWrittenMsg)
	second := saveAsNew(1, false, "Elves", sample())().(deckWrittenMsg)
	t.Cleanup(func() { deck.Delete(first.slug); deck.Delete(second.slug) })

	if first.slug == second.slug {
		t.Errorf("both saves used %q", first.slug)
	}
}

func TestAnEditIsWrittenStraightAwayButNotCommitted(t *testing.T) {
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	if _, _, err := deck.SaveVersioned("ghen", mustRead(t, "ghen")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deck.Delete("ghen") })

	m, l := openDeckPanel(t, sized(160, 24), "ghen", "Ghen", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	})
	m.add([]deck.Card{{Card: mtg.Card{Name: "Llanowar Elves"}}})
	runCmd(m.autosave())

	d := mustRead(t, "ghen")
	if !strings.Contains(d.String(), "Llanowar Elves") {
		t.Errorf("the edit wasn't written:\n%s", d.String())
	}
	if !deck.HasUncommittedEdits("ghen") {
		t.Error("the edit was committed without w")
	}
	if !l.dirty {
		t.Error("the deck isn't marked uncommitted")
	}

	// w commits it, describing the change against the last commit.
	msg := m.saveDeck(m.ws.current().id, *l.deck, l.all)().(deckSavedMsg)
	if msg.subject != "+Llanowar Elves" {
		t.Errorf("commit subject %q", msg.subject)
	}
	if deck.HasUncommittedEdits("ghen") {
		t.Error("w left the deck uncommitted")
	}
}

func TestAnOlderAutosaveNeverLandsOverANewerOne(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })
	info := deck.Info{Name: "Ghen", Slug: "ghen", Format: "commander"}
	m := New()

	older := m.autosaveDeck(info, []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}}, false)
	newer := m.autosaveDeck(info, []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Mana Crypt"}}}, false)
	newer()
	older()
	if d := mustRead(t, "ghen"); !strings.Contains(d.String(), "Mana Crypt") {
		t.Errorf("the older write landed last:\n%s", d.String())
	}
}

func mustRead(t *testing.T, slug string) *deck.File {
	t.Helper()
	d, err := deck.Read(slug)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// runCmd runs a command and whatever batch it hands back, synchronously.
func runCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			runCmd(c)
		}
	}
}

func TestWritingAnUnchangedDeckSaysSoRatherThanCommitting(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m, _ := openDeckPanel(t, sized(160, 24), "ghen", "Ghen", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}},
	})
	m = drive(m, "w")
	if !strings.Contains(stripANSI(m.View()), "nothing to commit") {
		t.Errorf("got:\n%s", stripANSI(m.View()))
	}
}

func TestSpaceWSavesTheEditingDeckFromAnotherPanel(t *testing.T) {
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	seedDeck(t, "elf-ball", "name: Elf Ball\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("elf-ball") })

	m, l := openDeckPanel(t, sized(160, 24), "elf-ball", "Elf Ball", []deck.Card{
		{Qty: 1, Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}},
	})
	m.add([]deck.Card{{Card: mtg.Card{Name: "Llanowar Elves"}}})

	// Go somewhere else. Bare w would save whatever is here; <space>w has to
	// reach past it to the deck a/x/t have been writing to — which is the
	// whole point, since that deck is routinely not the one on screen.
	m = drive(m, "space", "f")
	if m.ws.focused == m.ws.editing {
		t.Fatal("still in the deck panel")
	}

	cmd := m.commitAll()
	if cmd == nil {
		t.Fatal("space w produced nothing to do")
	}
	msg, ok := cmd().(deckSavedMsg)
	if !ok {
		t.Fatalf("space w produced %T", cmd())
	}
	if msg.err != nil {
		t.Fatalf("saving failed: %v", msg.err)
	}
	if msg.panel != m.ws.editingPanel().id {
		t.Errorf("the result is addressed to panel %d, not the deck's", msg.panel)
	}

	body, err := deck.Read("elf-ball")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), "Llanowar Elves") {
		t.Errorf("the file says:\n%s", body.String())
	}

	// And the deck stops being marked unsaved once the result lands, even
	// though the cursor is in another panel.
	next, _ := m.Update(msg)
	m = next.(Model)
	if l.dirty {
		t.Error("the deck is still marked unsaved")
	}
	if !strings.Contains(m.notice, "Elf Ball") && !strings.Contains(m.notice, "Llanowar") {
		t.Errorf("nothing said it was saved; the notice is %q", m.notice)
	}
}

func TestSpaceWWithNothingToCommitSaysSo(t *testing.T) {
	m := drive(sized(120, 30), "space", "f")
	if cmd := m.commitAll(); cmd != nil {
		t.Error("something was saved with no deck open")
	}
	if !strings.Contains(m.notice, "nothing to commit") {
		t.Errorf("the notice says %q", m.notice)
	}
}
