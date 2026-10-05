package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"

	tea "github.com/charmbracelet/bubbletea"
)

// drive runs a sequence of keys through the model, the way a person would.
func drive(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.Msg
		switch k {
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// openPanel opens a panel and fills it, which is the only way to get back
// out to the list: an empty panel has nothing to escape to, so esc closes
// it. Running a search is what turns the bar back into a header.
func openPanel(m Model, key, query string) Model {
	m = drive(m, "space", key)
	// A decks panel opens straight onto your decks, so there is no bar to
	// type into and nothing to ask for.
	if !m.ws.current().searchOpen {
		return m
	}
	for _, r := range query {
		m = drive(m, string(r))
	}
	return drive(m, "enter")
}

func sized(w, h int) Model {
	m := New()
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func TestStartsOnTheSplashWithNoPanels(t *testing.T) {
	m := sized(120, 40)
	if !m.ws.empty() {
		t.Error("something was open before anything was asked for")
	}
	if view := m.View(); view == "" {
		t.Error("the splash drew nothing")
	}
}

func TestTheLeaderOpensEachKindOfPanel(t *testing.T) {
	for key, want := range map[string]Kind{
		"f": KindFind,
		"d": KindDecks,
		"r": KindRules,
		"n": KindNew,
	} {
		m := drive(sized(120, 40), "space", key)
		if m.ws.count() != 1 {
			t.Fatalf("space %s opened %d panels", key, m.ws.count())
		}
		if got := m.ws.current().kind; got != want {
			t.Errorf("space %s opened a %v panel, want %v", key, got, want)
		}
	}
}

func TestANewPanelOpensBesideTheOneYouWereOn(t *testing.T) {
	// Not at the end: you opened it while working on this panel, so it
	// belongs next to this panel.
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")

	kinds := []Kind{}
	for _, p := range m.ws.panels {
		kinds = append(kinds, p.kind)
	}
	if len(kinds) != 3 || kinds[1] != KindDecks || kinds[2] != KindRules {
		t.Errorf("panels came out %v", kinds)
	}
}

func TestFocusStepsAlongTheRowAndStopsAtTheEnds(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")
	if m.ws.focused != 2 {
		t.Fatalf("focus is on %d, want the panel just opened", m.ws.focused)
	}

	m = drive(m, "h", "h")
	if m.ws.focused != 0 {
		t.Errorf("two lefts landed on %d, want 0", m.ws.focused)
	}
	m = drive(m, "h")
	if m.ws.focused != 0 {
		t.Errorf("left from the leftmost panel wrapped to %d", m.ws.focused)
	}
	m = drive(m, "l", "l", "l")
	if m.ws.focused != 2 {
		t.Errorf("right ran past the end to %d", m.ws.focused)
	}
}

func TestClosingAPanelFocusesItsLeftNeighbour(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")
	m = drive(m, "space", "x")
	if m.ws.count() != 2 {
		t.Fatalf("closed to %d panels, want 2", m.ws.count())
	}
	if m.ws.focused != 1 {
		t.Errorf("focus went to %d, want the panel to the left", m.ws.focused)
	}
}

func TestOnlyKeepsTheFocusedPanel(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = openPanel(m, "r", "flying")
	m = drive(m, "h") // onto the decks panel
	m = drive(m, "space", "o")

	if m.ws.count() != 1 {
		t.Fatalf("only left %d panels", m.ws.count())
	}
	if m.ws.current().kind != KindDecks {
		t.Errorf("only kept the %v panel, want the focused one", m.ws.current().kind)
	}
}

func TestAPanelCanBeMovedAlongTheRow(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	m = next.(Model)

	if m.ws.panels[0].kind != KindDecks {
		t.Errorf("panels are %v, %v; want decks moved to the left",
			m.ws.panels[0].kind, m.ws.panels[1].kind)
	}
	if m.ws.focused != 0 {
		t.Errorf("focus stayed at %d rather than travelling with the panel", m.ws.focused)
	}
}

func TestTabCyclesWhatANewPanelSearches(t *testing.T) {
	m := drive(sized(120, 40), "space", "n")
	if m.ws.current().kind != KindNew {
		t.Fatal("space n did not open an untyped panel")
	}
	m = drive(m, "tab")
	if got := m.ws.current().kind; got != KindFind {
		t.Errorf("first tab landed on %v, want find", got)
	}
	m = drive(m, "tab", "tab")
	if got := m.ws.current().kind; got != KindRules {
		t.Errorf("three tabs landed on %v, want rules", got)
	}
}

func TestTheSearchBarTakesTypingRatherThanCommands(t *testing.T) {
	// h, l and q are panel keys in a list and letters in a search bar.
	m := drive(sized(120, 40), "space", "f")
	m = drive(m, "h", "l", "q")
	if got := m.ws.current().search.Value(); got != "hlq" {
		t.Errorf("the bar holds %q, want the keys typed into it", got)
	}
	if m.ws.count() != 1 {
		t.Error("typing in the search bar changed the panels")
	}
}

func TestEscTakesTheFilterBeforeClosingThePanel(t *testing.T) {
	// esc is "back": a filtered panel loses its filter first, and only the
	// next esc closes it.
	m := withCards(sized(120, 40), "f", sample(), sortArrival)
	m = drive(m, "/", "e", "l", "f", "enter")

	m = drive(m, "esc")
	if m.ws.count() != 1 || m.ws.current().cardsView().filter != "" {
		t.Fatal("the first esc should clear the filter and keep the panel")
	}
	m = drive(m, "esc")
	if m.ws.count() != 0 {
		t.Error("the second esc did not close the panel")
	}
}

func TestSpaceUReopensAClosedPanelWhereItWas(t *testing.T) {
	m := sized(160, 30)
	m = withCards(m, "f", sample(), sortArrival)     // panel 0
	m = withCards(m, "d", sample()[:2], sortArrival) // panel 1
	m.ws.panels[0].cardsView().name = "keep-me"
	if m.ws.count() != 2 {
		t.Fatalf("setup left %d panels", m.ws.count())
	}

	m = focusOn(m, 0)
	m = drive(m, "space", "x") // close the first panel
	if m.ws.count() != 1 {
		t.Fatalf("close left %d panels", m.ws.count())
	}

	m = drive(m, "space", "u") // undo the close
	if m.ws.count() != 2 {
		t.Fatalf("space u left %d panels", m.ws.count())
	}
	if got := m.ws.panels[0].cardsView().name; got != "keep-me" {
		t.Errorf("the restored panel came back at the wrong spot; index 0 holds %q", got)
	}
	if m.ws.focused != 0 {
		t.Errorf("space u focused panel %d, want the one it restored", m.ws.focused)
	}
}

func TestSpaceUWithNothingClosedIsANoOp(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	before := m.ws.count()
	m = drive(m, "space", "u")
	if m.ws.count() != before {
		t.Errorf("space u changed the panels with nothing to restore: %d → %d", before, m.ws.count())
	}
}

func TestTheLeaderMenuAppearsAndCancels(t *testing.T) {
	m := openPanel(sized(120, 40), "f", "angel")
	m = drive(m, "space")
	if !m.leader {
		t.Fatal("space did not raise the menu")
	}
	if view := m.View(); view == "" {
		t.Error("the menu drew nothing")
	}
	m = drive(m, "z") // names nothing
	if m.leader {
		t.Error("an unknown key left the leader waiting")
	}
	if m.ws.count() != 1 {
		t.Error("an unknown leader key did something")
	}
}

func TestTheFrameFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 50}, {60, 20}} {
		m := sized(size[0], size[1])
		m = openPanel(m, "f", "angel")
		m = openPanel(m, "d", "marbri")
		m = openPanel(m, "r", "flying")

		view := m.View()
		lines := splitLines(view)
		if len(lines) > size[1] {
			t.Errorf("%dx%d: drew %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := visibleWidth(line); w > size[0] {
				t.Errorf("%dx%d: line %d is %d columns", size[0], size[1], i, w)
			}
		}
	}
}

func TestClosingTheLastPanelLandsOnTheSplash(t *testing.T) {
	// Not straight out of the program: there is always one press between
	// you and the exit, and esc from the splash is it.
	m := drive(sized(120, 40), "space", "f")
	m = drive(m, "esc")

	if !m.ws.empty() {
		t.Fatal("the panel did not close")
	}
	if view := m.View(); view == "" {
		t.Error("nothing was drawn after the last panel closed")
	}
}

// ── The list, driven through the model ──────────────────────────

// withCards opens a panel and fills it, standing in for the search that
// phase 5 will run.
func withCards(m Model, key string, cards []deck.Card, order cardSort) Model {
	m = openPanel(m, key, "query")
	l := newCardList(cards, order, "")
	l.name = "query"
	m.ws.current().show(l)
	return m
}

func TestJAndKMoveTheCursor(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "j", "j")
	if c, _ := m.ws.current().cardsView().current(); c.Card.Name != "Sol Ring" {
		t.Errorf("two js landed on %s", c.Card.Name)
	}
	m = drive(m, "k")
	if c, _ := m.ws.current().cardsView().current(); c.Card.Name != "Llanowar Elves" {
		t.Errorf("k landed on %s", c.Card.Name)
	}
}

func TestGGAndShiftGGoToTheEnds(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "G")
	if c, _ := m.ws.current().cardsView().current(); c.Card.Name != "Forest" {
		t.Errorf("G landed on %s, want the last card", c.Card.Name)
	}
	// gg, not g: g is a prefix so that gd and gv can exist.
	m = drive(m, "g", "g")
	if m.ws.current().cardsView().cursor.at != 0 {
		t.Error("gg did not go back to the top")
	}
}

func TestDotCyclesTheSortAndTheHeaderSaysSo(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, ".")
	if got := m.ws.current().cardsView().order; got != sortMana {
		t.Errorf(". moved to %v, want mana value", got)
	}
	if !strings.Contains(stripANSI(m.View()), "mana value") {
		t.Error("the panel does not say what it is sorted by")
	}
	m = drive(m, ">", ">")
	if got := m.ws.current().cardsView().order; got != sortName {
		t.Errorf("> wrapped to %v", got)
	}
}

func TestTheHeaderPutsSort2BeforeSort1(t *testing.T) {
	// Sort 2 colours the names on the left, sort 1 fills the column on the
	// right, and the header reads in the same order.
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, ".", ",", ",")
	l := m.ws.current().cardsView()
	one, two := l.orderName(), l.order2Name()
	if one == "" || two == "" || one == two {
		t.Fatalf("want two different orders, got %q and %q", one, two)
	}
	view := stripANSI(m.View())
	i, j := strings.Index(view, two+" · "+one), strings.Index(view, one+" · "+two)
	if i < 0 || j >= 0 {
		t.Errorf("the header should read %q · %q", two, one)
	}
}

func TestCommaIsNoLongerTheLeader(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, ",")
	if m.leader {
		t.Error(", raised the leader menu")
	}
	if m.ws.current().cardsView().order2 == sortArrival {
		t.Error(", didn't cycle sort 2")
	}
}

func TestSortingStillWorksWithTheStatisticsUp(t *testing.T) {
	// The statistics used to claim o and O, so the list couldn't be
	// re-sorted while they were up. The sort keys are clear of them now.
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, "s")
	if m.info.mode != infoStats {
		t.Fatal("s did not bring the statistics up")
	}
	m = drive(m, ".")
	if got := m.ws.current().cardsView().order; got != sortMana {
		t.Errorf(". with the statistics up moved to %v, want mana value", got)
	}
}

func TestSlashOpensTheFilterAndNarrowsAsYouType(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "/")
	if !m.ws.current().filtering {
		t.Fatal("/ did not open the filter")
	}
	m = drive(m, "e", "l", "f")
	if got := m.ws.current().cardsView().count(); got != 2 {
		t.Errorf("filtering to 'elf' left %d cards", got)
	}
	m = drive(m, "enter")
	if m.ws.current().filtering {
		t.Error("enter left the filter prompt open")
	}
}

func TestEscAbandonsTheFilterPrompt(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "/", "e", "l", "f", "esc")
	if m.ws.current().cardsView().count() != 4 {
		t.Error("esc left the half-typed narrowing in place")
	}
}

func TestTheEscCascadeInAList(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "/", "e", "l", "f", "enter")
	// And a statistics filter on top: Creature, the second type down —
	// the first is Land, the commonest, which would leave no elves.
	m = drive(m, "s", "j", "a", "s")
	m = drive(m, "v") // pick one out

	m = drive(m, "esc") // the selection is transient, so it goes first
	if l.markCount() != 0 {
		t.Error("the first esc did not clear the selection")
	}
	if l.filter == "" || len(l.statFilter) == 0 {
		t.Fatal("the first esc took a filter too")
	}
	m = drive(m, "esc")
	if l.filter != "" || len(l.statFilter) == 0 {
		t.Error("the second esc should take the text filter, and only that")
	}
	m = drive(m, "esc")
	if len(l.statFilter) != 0 || m.ws.count() != 1 {
		t.Error("the third esc should take the statistics filter and keep the panel")
	}
	m = drive(m, "esc")
	if m.ws.count() != 0 {
		t.Error("esc did not close the panel once the filters were gone")
	}
}

func TestEscInThePromptPutsBackTheFilterItOpenedOn(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "/", "e", "l", "enter")
	m = drive(m, "/", "f", "esc")
	if l.filter != "el" {
		t.Errorf("abandoning an edit left the filter at %q, want the %q it had", l.filter, "el")
	}
}

func TestBClearsTheFilterWithoutClosingThePanel(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m = drive(m, "/", "e", "l", "f", "enter")
	if m.ws.current().cardsView().count() != 2 {
		t.Fatalf("the filter did not narrow the list: %d rows", m.ws.current().cardsView().count())
	}

	m = drive(m, "b")
	if got := m.ws.current().cardsView().filter; got != "" {
		t.Errorf("b left the filter %q in place", got)
	}
	if m.ws.count() != 1 {
		t.Error("b closed the panel instead of clearing the filter")
	}
	if m.ws.current().cardsView().count() != 4 {
		t.Error("clearing the filter did not put the whole list back")
	}
}

func TestEveryListFlagsWhatTheEditingDeckHolds(t *testing.T) {
	m := sized(160, 30)
	m = withCards(m, "f", sample(), sortArrival)
	m = withCards(m, "d", sample()[:2], sortArrival)

	// Pretend the second panel is the deck being built.
	m.ws.editing = 1

	members := m.membersFor(m.ws.panels[0].cardsView())
	if members["sol ring"] != notElsewhere {
		t.Error("Sol Ring is not in the deck but was flagged")
	}
	if members["llanowar elves"] != inEditing {
		t.Error("Llanowar Elves is in the deck and should be flagged in the search")
	}
}

func TestOtherListsGetTheWeakerMark(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)      // a search
	m = withCards(m, "f", sample()[2:], sortArrival)  // another: Sol Ring, Forest
	m = withCards(m, "d", sample()[1:2], sortArrival) // the deck: the elves
	m.ws.editing = 2

	members := m.membersFor(m.ws.panels[0].cardsView())
	if members["sol ring"] != inOther {
		t.Errorf("Sol Ring is in the other search: %v, want the weak mark", members["sol ring"])
	}
	if members["llanowar elves"] != inEditing {
		t.Error("the deck's card should keep the strong mark")
	}
	if members["dwynen, gilt-leaf daen"] != notElsewhere {
		t.Error("a card nowhere else was flagged")
	}
}

func TestTheEditingDeckMarksTheFocusedListInOrange(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample()[2:3], sortArrival) // Sol Ring
	m = withCards(m, "f", sample()[3:], sortArrival)  // Forest
	m = withCards(m, "d", sample(), sortArrival)
	m = focusOn(m, 1)
	m.ws.editing = 2

	members := m.membersFor(m.ws.panels[2].cardsView())
	if members["forest"] != inFocused {
		t.Error("the focused list's card should carry the focused mark in the deck")
	}
	if members["sol ring"] != inOther {
		t.Error("another list's card should be marked weakly in the deck")
	}
}

func TestTheEditingDeckFlagsWhatTheListsHaveTurnedUp(t *testing.T) {
	m := sized(160, 30)
	m = withCards(m, "f", sample()[2:], sortArrival) // Sol Ring, Forest
	m = withCards(m, "d", sample(), sortArrival)
	m.ws.editing = 1

	members := m.membersFor(m.ws.panels[1].cardsView())
	if members["sol ring"] == notElsewhere {
		t.Error("the deck should flag the card the search turned up")
	}
	if members["dwynen, gilt-leaf daen"] != notElsewhere {
		t.Error("a card no list is showing was flagged in the deck")
	}
}
