package ui

import (
	"errors"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/prints"
)

func historyCard() mtg.Card {
	return mtg.Card{
		ID: "x", OracleID: "oid", PrintsSearchURI: "https://example.invalid/prints",
		Name: "Test Bird", TypeLine: "Creature — Bird", OracleText: "Flying",
	}
}

func TestGVOnACardOpensItsPrintedText(t *testing.T) {
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v")

	if m.info.mode != infoVersions {
		t.Fatalf("gv left the panel in mode %v", m.info.mode)
	}
	if _, ok := m.histories["oid"]; !ok {
		t.Error("nothing was started for this card")
	}
	if !strings.Contains(stripANSI(m.View()), "printed text") {
		t.Error("the panel does not say what it is showing")
	}
}

func TestACardWithNoPrintingsLinkSaysSo(t *testing.T) {
	card := historyCard()
	card.PrintsSearchURI = ""
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: card}}, sortArrival)
	m = drive(m, "g", "v")

	if !strings.Contains(stripANSI(m.View()), "no printing history") {
		t.Errorf("got:\n%s", stripANSI(m.View()))
	}
}

func TestMissingSetsAreFetchedAtOnceNewestFirstOneAtATime(t *testing.T) {
	// No go-ahead to give: the sets come straight away, the newest first
	// (the wording in force now matters most), one download at a time.
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v")

	next, cmd := m.Update(printingsMsg{card: "oid", printings: []prints.Printing{
		{Name: "Test Bird", Set: "AAA", SetName: "Alpha", Released: "1993-08-05"},
		{Name: "Test Bird", Set: "CCC", SetName: "Gamma", Released: "2020-01-01"},
		{Name: "Test Bird", Set: "BBB", SetName: "Beta", Released: "1994-04-11"},
	}})
	m = next.(Model)

	h := m.histories["oid"]
	if cmd == nil || h.fetching != "CCC" {
		t.Fatalf("downloading %q first, want the newest set, CCC", h.fetching)
	}
	if len(h.queue) != 2 || h.queue[0] != "BBB" {
		t.Errorf("queued %v, want BBB then AAA", h.queue)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "fetching 3 more sets") {
		t.Errorf("the panel doesn't say what's coming:\n%s", view)
	}

	// Each arrival starts the next, and the history fills in as they come.
	next, cmd = m.Update(setTextMsg{card: "oid", set: "CCC", cards: map[string]string{"test bird": "Flying"}})
	m = next.(Model)
	if cmd == nil || h.fetching != "BBB" {
		t.Errorf("after CCC, downloading %q, want BBB", h.fetching)
	}
	if len(h.revisions) == 0 {
		t.Error("nothing shown until every set is in")
	}
	// A set that won't download is left out, and the rest still come.
	next, _ = m.Update(setTextMsg{card: "oid", set: "BBB", err: errTest})
	m = next.(Model)
	next, _ = m.Update(setTextMsg{card: "oid", set: "AAA", cards: map[string]string{"test bird": "Does not tap when attacking."}})
	m = next.(Model)
	if h.state != histReady {
		t.Errorf("state %v after every set came or failed, want ready", h.state)
	}
}

func TestYYanksInThePrintedTextPanelNow(t *testing.T) {
	// y used to be the go-ahead to download; with nothing to ask, it is a
	// card list's yank again, as everywhere else.
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v", "y")
	if len(m.register) == 0 {
		t.Error("y didn't yank")
	}
}

func TestRevisionsCollapseIdenticalWordings(t *testing.T) {
	m := withCards(sized(140, 30), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v")

	next, _ := m.Update(printingsMsg{card: "oid", printings: []prints.Printing{
		{Name: "Test Bird", Set: "AAA", SetName: "Alpha", Released: "1993-08-05"},
		{Name: "Test Bird", Set: "BBB", SetName: "Beta", Released: "1994-04-11"},
	}})
	m = next.(Model)

	h := m.histories["oid"]
	for _, s := range []struct{ set, text string }{
		{"AAA", "Does not tap when attacking."},
		{"BBB", "Flying"},
	} {
		next, _ = m.Update(setTextMsg{card: "oid", set: s.set,
			cards: map[string]string{"test bird": s.text}})
		m = next.(Model)
	}

	if len(h.revisions) < 2 {
		t.Fatalf("got %d revisions, want the wording to have changed", len(h.revisions))
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Alpha") {
		t.Errorf("the first printing is not named:\n%s", view)
	}
}

func TestADeckVersionShowsWhatItChanged(t *testing.T) {
	// A commit subject says "+3 cards"; the diff says which three.
	if !deck.GitAvailable() {
		t.Skip("git not installed")
	}
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	base, _ := deck.Read("ghen")
	deck.SaveVersioned("ghen", base)
	changed, _ := deck.ParseFile(strings.NewReader(
		"name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n1 Llanowar Elves\n"))
	deck.SaveVersioned("ghen", changed)

	m := withCards(sized(140, 30), "d", []deck.Card{{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}}, sortArrival)
	l := m.ws.current().cardsView()
	l.deck = &deck.Info{Name: "Ghen", Slug: "ghen"}

	next, cmd := m.Update(loadVersions(m.ws.current().id, "ghen", "Ghen")())
	m = next.(Model)
	if cmd == nil {
		t.Fatal("the version list did not ask for a diff")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	view := stripANSI(m.View())
	if !strings.Contains(view, "+1 Llanowar Elves") {
		t.Errorf("the diff does not say what changed:\n%s", view)
	}
}

func TestTheDiffLeavesOutGitsOwnBookkeeping(t *testing.T) {
	// There is one file and you know which; the headers are noise.
	got := strings.Join(renderDiff(strings.Join([]string{
		"diff --git a/ghen.list b/ghen.list",
		"index 1234567..89abcde 100644",
		"--- a/ghen.list",
		"+++ b/ghen.list",
		"@@ -1,4 +1,5 @@",
		" [mainboard]",
		"+1 Llanowar Elves",
		"-1 Sol Ring",
	}, "\n"), 40), "\n")
	got = stripANSI(got)

	for _, unwanted := range []string{"diff --git", "index ", "@@", "+++", "---"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%q survived:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "+1 Llanowar Elves") || !strings.Contains(got, "-1 Sol Ring") {
		t.Errorf("the changes were lost:\n%s", got)
	}
}

// twoCards is a list you can move the cursor within, both with a printings
// link so gv means something on either.
func twoCards() []deck.Card {
	first := historyCard()
	second := historyCard()
	second.ID, second.OracleID, second.Name = "y", "oid2", "Test Beast"
	return []deck.Card{{Card: first}, {Card: second}}
}

func TestEscLeavesThePrintedText(t *testing.T) {
	// gv was the only key in and there was no key out.
	m := withCards(sized(140, 30), "f", twoCards(), sortArrival)
	m = drive(m, "g", "v")
	if m.info.mode != infoVersions {
		t.Fatal("gv did not open the printed text")
	}

	m = drive(m, "esc")
	if m.info.mode != infoCard {
		t.Errorf("esc left the panel in mode %v", m.info.mode)
	}
	// And it took nothing else with it on the way out.
	if m.ws.count() != 1 {
		t.Errorf("esc closed the panel as well; %d left", m.ws.count())
	}
}

func TestMovingToAnotherCardLeavesThePrintedText(t *testing.T) {
	// A history stood over every card you moved to afterwards, each of them
	// showing "gv for how its text has changed" — the prompt to press the
	// key you had just pressed.
	m := withCards(sized(140, 30), "f", twoCards(), sortArrival)
	m = drive(m, "g", "v", "j")

	if m.info.mode != infoCard {
		t.Fatalf("moving to another card left the panel in mode %v", m.info.mode)
	}
	body := stripANSI(m.View())
	if strings.Contains(body, "gv for how its text has changed") {
		t.Error("still offering the key that was just pressed")
	}
	if !strings.Contains(body, "Test Beast") {
		t.Error("the panel does not describe the card under the cursor")
	}
}

func TestComingBackToTheCardKeepsItsHistory(t *testing.T) {
	// Leaving the view is not forgetting what was fetched for it.
	m := withCards(sized(140, 30), "f", twoCards(), sortArrival)
	m = drive(m, "g", "v", "j", "k")
	if m.info.mode != infoCard {
		t.Error("coming back re-opened the history by itself")
	}
	if _, ok := m.histories["oid"]; !ok {
		t.Error("the fetched history was thrown away")
	}
	m = drive(m, "g", "v")
	if m.info.mode != infoVersions {
		t.Error("gv on the same card again did not re-open it")
	}
}

func TestThePrintedTextStaysWhileTheCursorDoes(t *testing.T) {
	// Only *leaving* the card closes it — a keypress that doesn't move must
	// not.
	m := withCards(sized(140, 30), "f", twoCards(), sortArrival)
	m = drive(m, "g", "v", "k") // k at the top of the list moves nowhere
	if m.info.mode != infoVersions {
		t.Errorf("a key that moved nothing closed the view; mode %v", m.info.mode)
	}
}

var errTest = errors.New("no such set")
