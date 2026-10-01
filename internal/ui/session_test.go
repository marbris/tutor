package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/scryfall"
	"ttr/internal/stats"
)

func TestASavedWorkspaceComesBack(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m := openPanel(sized(160, 30), "f", "t:elf")
	m = drive(m, "space", "d")
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 2 {
		t.Fatalf("came back to %d panels, want 2", back.ws.count())
	}
	if back.ws.panels[0].kind != KindFind || back.ws.panels[1].kind != KindDecks {
		t.Errorf("came back as %v, %v", back.ws.panels[0].kind, back.ws.panels[1].kind)
	}
}

func TestASearchComesBackAsItsQueryNotItsResults(t *testing.T) {
	// Results can always be fetched again; what was worth keeping is what
	// you asked for.
	m, p := typed(sized(140, 30), "t:elf c:g")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 1 {
		t.Fatalf("came back to %d panels", back.ws.count())
	}
	if got := back.ws.panels[0].title; got != "t:elf c:g" {
		t.Errorf("came back showing %q", got)
	}
}

func TestADeckOfYoursComesBack(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m, _ := openDeckPanel(t, sized(140, 30), "ghen", "Ghen", sample())
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 1 {
		t.Fatalf("came back to %d panels", back.ws.count())
	}
	if got := back.ws.panels[0].title; got != "ghen" {
		t.Errorf("came back showing %q", got)
	}
}

func TestADeckDeletedSinceDoesNotComeBack(t *testing.T) {
	seedDeck(t, "gone", "name: Gone\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	m, _ := openDeckPanel(t, sized(140, 30), "gone", "Gone", sample())
	m.saveSession()
	deck.Delete("gone")

	back, _ := NewRestored()
	if back.ws.count() != 0 {
		t.Errorf("came back to %d panels, want none", back.ws.count())
	}
}

func TestSomebodyElsesDeckDoesNotComeBack(t *testing.T) {
	// It isn't yours and might not be there tomorrow.
	m := withCards(sized(140, 30), "d", sample(), sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Borrowed", ID: "Y8dZ7"}
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 0 {
		t.Errorf("came back to %d panels, want none", back.ws.count())
	}
}

func TestAnEmptyPanelDoesNotComeBack(t *testing.T) {
	// An empty panel is a question you hadn't answered.
	m := drive(sized(140, 30), "space", "f")
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.count() != 0 {
		t.Errorf("came back to %d panels, want none", back.ws.count())
	}
}

func TestWithNothingSavedTheSplashIsWhatYouGet(t *testing.T) {
	if err := writeString(sessionPath(), "{}"); err != nil {
		t.Fatal(err)
	}
	m, _ := NewRestored()
	next, _ := m.Update(sizeOf(120, 30))
	m = next.(Model)

	if !m.ws.empty() {
		t.Fatal("something was restored from an empty session")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "space f") {
		t.Errorf("the splash is not what came up:\n%s", view)
	}
}

func TestACorruptSessionIsNotFatal(t *testing.T) {
	if err := writeString(sessionPath(), "{not json"); err != nil {
		t.Fatal(err)
	}
	m, _ := NewRestored()
	if !m.ws.empty() {
		t.Error("something was restored from a broken file")
	}
}

func TestTheEditingDeckComesBack(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m := openPanel(sized(180, 30), "f", "t:elf")
	m, _ = openDeckPanel(t, m, "ghen", "Ghen", sample())
	m.saveSession()

	back, _ := NewRestored()
	if back.ws.editing != 1 {
		t.Errorf("the editing deck came back as panel %d", back.ws.editing)
	}
}

// laidOut gives a list two orders, a / filter and a statistics filter, the
// way someone would have left it.
func laidOut(t *testing.T, l *cardList) {
	t.Helper()
	l.order, l.desc1 = sortMana, true
	l.order2, l.desc2 = sortColor, false
	l.filter = "elf"
	r, ok := findRow(stats.Groups(l.all, l.all), "Type", "Creature")
	if !ok {
		t.Fatal("no Creature row to filter by")
	}
	l.statFilter, _ = l.statFilter.Add(stats.AndNot, r)
	l.refresh()
}

func checkLayout(t *testing.T, l *cardList) {
	t.Helper()
	if l.order != sortMana || !l.desc1 || l.order2 != sortColor || l.desc2 {
		t.Errorf("came back ordered %v %v / %v %v", l.order, l.desc1, l.order2, l.desc2)
	}
	if l.filter != "elf" {
		t.Errorf("came back filtered %q", l.filter)
	}
	if len(l.statFilter) != 1 || l.statFilter[0].Op != stats.AndNot || l.statFilter[0].Row.Label != "Creature" {
		t.Errorf("came back with statistics filter %v", l.statFilter.String())
	}
	if l.statFilter[0].Row.Match == nil {
		t.Error("the category came back without its test")
	}
}

func TestASearchComesBackLaidOutTheWayItWasLeft(t *testing.T) {
	m, p := typed(sized(140, 30), "t:elf")
	p.querySort = indexOf(scryfall.SortOptions, "edhrec")
	p.queryDir = indexOf(scryfall.DirOptions, "desc")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)
	laidOut(t, m.ws.current().cardsView())
	m.lastTag = "ramp"
	m.saveSession()

	back, _ := NewRestored()
	bp := back.ws.panels[0]
	if bp.queryOrder() != "edhrec" || bp.queryDirection() != "desc" {
		t.Errorf("came back asking for %s %s", bp.queryOrder(), bp.queryDirection())
	}
	if back.lastTag != "ramp" {
		t.Errorf("last tag came back as %q", back.lastTag)
	}
	back = answer(back, bp, sample(), 4, nil)
	checkLayout(t, back.ws.current().cardsView())
}

func TestADeckComesBackLaidOutTheWayItWasLeft(t *testing.T) {
	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })

	m, l := openDeckPanel(t, sized(140, 30), "ghen", "Ghen", sample())
	laidOut(t, l)
	m.saveSession()

	back, _ := NewRestored()
	bp := back.ws.panels[0]
	next, _ := back.handleDeckOpened(deckOpenedMsg{panel: bp.id, newPane: true, cards: sample(),
		info: deck.Info{Name: "Ghen", Slug: "ghen", Format: "commander"}})
	nm := next.(Model)
	checkLayout(t, nm.ws.current().cardsView())
}

func TestALayoutNoLongerKnownIsLeftOut(t *testing.T) {
	ps := panelSession{Sort1: "no such order", Stats: []savedClause{{Op: "and", Group: "Type", Label: "Planeswalker"}}}
	l := newCardList(sample(), sortArrival, "")
	ps.applyLayout(l)
	if l.order != sortArrival || len(l.statFilter) != 0 {
		t.Errorf("guessed: order %v, filter %v", l.order, l.statFilter.String())
	}
}

func TestThePrintingViewComesBack(t *testing.T) {
	withKitty(t, true)
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m.ws.current().title = "t:elf"
	m.info.mode = infoImage
	m.saveSession()

	back, _ := NewRestored()
	if back.info.mode != infoImage {
		t.Errorf("came back in %v, want the printing view", back.info.mode)
	}

	// And not in a terminal that can't draw it.
	withKitty(t, false)
	if back, _ := NewRestored(); back.info.mode == infoImage {
		t.Error("came back to pictures in a terminal without them")
	}

	// Leaving it, it stays left.
	withKitty(t, true)
	m.info.mode = infoCard
	m.saveSession()
	if back, _ := NewRestored(); back.info.mode != infoCard {
		t.Errorf("came back in %v after leaving the printing view", back.info.mode)
	}
}
