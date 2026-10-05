package ui

import (
	"strings"
	"testing"

	"ttr/internal/config"
	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/prints"
	"ttr/internal/stats"
	"ttr/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// alt is a key with alt held, the way the terminal reports it.
func alt(m Model, r rune) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true})
	return next.(Model)
}

func cardNames(l *cardList) []string {
	var out []string
	for _, c := range l.rows {
		out = append(out, c.Card.Name)
	}
	return out
}

func TestTheCycleRunsInTheNewOrderWithNameLast(t *testing.T) {
	want := []cardSort{sortArrival, sortMana, sortColor, sortType, sortPower,
		sortToughness, sortEDHREC, sortUSD, sortRarity, sortInclusion, sortName}
	s := sortArrival
	for i, w := range want {
		if s != w {
			t.Fatalf("step %d is %v, want %v", i, s, w)
		}
		s = s.next(1)
	}
	if s != sortArrival {
		t.Errorf("the cycle didn't wrap: %v", s)
	}
}

func TestEachOrderStartsBestFirst(t *testing.T) {
	for s, want := range map[cardSort]bool{
		sortPower: true, sortToughness: true, sortUSD: true, sortRarity: true,
		sortArrival: false, sortEDHREC: false, sortMana: false, sortColor: false,
		sortType: false, sortInclusion: false, sortName: false,
	} {
		if got := s.descending(); got != want {
			t.Errorf("%v starts descending=%v, want %v", s, got, want)
		}
	}
}

func TestAltDotTurnsSort1Round(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, ".") // mana value, ascending
	l := m.ws.current().cardsView()
	if got := l.rows[0].Card.Name; got != "Forest" {
		t.Fatalf("mana value ascending starts with %s", got)
	}
	if !strings.Contains(stripANSI(m.View()), "mana value ↑") {
		t.Error("the header doesn't say which way the order runs")
	}
	m = alt(m, '.')
	if got := l.rows[0].Card.Name; got != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("alt+. left %s on top, want the dearest card", got)
	}
	if !strings.Contains(stripANSI(m.View()), "mana value ↓") {
		t.Error("the header didn't turn round with the order")
	}
	// Moving on to the next order starts it the way it reads best, not the
	// way the last one was left.
	m = drive(m, ".")
	if l.order != sortColor || l.desc1 {
		t.Errorf("stepped on to %v descending=%v, want colour ascending", l.order, l.desc1)
	}
}

func TestAltCommaTurnsSort2Round(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, ".", ".", ".", ",") // sort 1 type, sort 2 mana value
	l := m.ws.current().cardsView()
	if l.order != sortType || l.order2 != sortMana {
		t.Fatalf("got %v / %v", l.order, l.order2)
	}
	before := cardNames(l)
	m = alt(m, ',')
	if !l.desc2 {
		t.Fatal("alt+, didn't turn sort 2 round")
	}
	after := cardNames(l)
	// The two creatures swap within the creature group: Dwynen costs 4,
	// the elves 1.
	if strings.Join(before, "|") == strings.Join(after, "|") {
		t.Errorf("turning sort 2 round moved nothing: %v", after)
	}
}

func TestBlanksStayAtTheBottomWhicheverWayTheOrderRuns(t *testing.T) {
	cards := []deck.Card{
		{Card: mtg.Card{Name: "Unranked"}},
		{Card: mtg.Card{Name: "Best", EDHRECRank: 1}},
		{Card: mtg.Card{Name: "Worse", EDHRECRank: 50}},
	}
	up := sortCards(cards, sortSpec{first: sortEDHREC})
	down := sortCards(cards, sortSpec{first: sortEDHREC, desc1: true})
	if got := up[2].Card.Name; got != "Unranked" {
		t.Errorf("ascending put %s last", got)
	}
	if down[0].Card.Name != "Worse" || down[2].Card.Name != "Unranked" {
		t.Errorf("descending gave %v %v %v", down[0].Card.Name, down[1].Card.Name, down[2].Card.Name)
	}
}

func TestScryfallOrderTurnedRoundIsTheListUpsideDown(t *testing.T) {
	got := sortCards(sample(), sortSpec{first: sortArrival, desc1: true})
	if got[0].Card.Name != "Forest" || got[3].Card.Name != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("got %v first and %v last", got[0].Card.Name, got[3].Card.Name)
	}
}

func TestInclusionPutsTheEditingDecksCardsFirstThenOtherLists(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)      // the list being sorted
	m = withCards(m, "f", sample()[3:], sortArrival)  // another list: Forest
	m = withCards(m, "d", sample()[2:3], sortArrival) // the deck: Sol Ring
	m.ws.current().cardsView().deck = &deck.Info{Name: "Deck", Slug: "deck", Format: "commander"}
	m.ws.editing = 2
	m = focusOn(m, 0)

	l := m.ws.panels[0].cardsView()
	for l.order != sortInclusion {
		m = drive(m, ".")
	}
	got := cardNames(l)
	if got[0] != "Sol Ring" || got[1] != "Forest" {
		t.Errorf("inclusion order is %v, want Sol Ring, then Forest, then the rest", got)
	}
}

func TestInclusionReSortsAsTheDeckChanges(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)
	m = withCards(m, "d", sample()[2:3], sortArrival) // Sol Ring
	m.ws.current().cardsView().deck = &deck.Info{Name: "Deck", Slug: "deck", Format: "commander"}
	m.ws.editing = 1
	m = focusOn(m, 0)

	l := m.ws.panels[0].cardsView()
	for l.order != sortInclusion {
		m = drive(m, ".")
	}
	// On the second row, the first card not in the deck.
	m = drive(m, "j")
	added, _ := l.current()
	next := l.rows[l.cursor.at+1]
	m = drive(m, "a")
	if got := cardNames(l)[:2]; got[0] != added.Card.Name && got[1] != added.Card.Name {
		t.Errorf("the added card didn't join the deck's group: %v", got)
	}
	if c, _ := l.current(); c.Card.Name != next.Card.Name {
		t.Errorf("the cursor is on %s, want the card that was below the one added (%s)", c.Card.Name, next.Card.Name)
	}
}

func TestANewSearchKeepsTheSorts(t *testing.T) {
	m, p := typed(sized(140, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)
	m = drive(m, ".", ",", ",")
	m = alt(m, '.')
	l := p.cardsView()
	order, order2, desc1, desc2 := l.order, l.order2, l.desc1, l.desc2

	m, _ = typed(drive(m, "i"), "t:goblin")
	m = drive(m, "enter")
	m = answer(m, p, sample()[1:], 3, nil)
	n := p.cardsView()
	if n == l {
		t.Fatal("the new answer didn't replace the list")
	}
	if n.order != order || n.order2 != order2 || n.desc1 != desc1 || n.desc2 != desc2 {
		t.Errorf("new search is %v/%v %v/%v, want %v/%v %v/%v",
			n.order, n.order2, n.desc1, n.desc2, order, order2, desc1, desc2)
	}
}

func TestTheSortConfigReordersAndTrimsTheCycle(t *testing.T) {
	defer SetSortConfig(nil)
	err := SetSortConfig(&config.Sort{
		Cycle:     []string{"scryfall", "name", "usd"},
		Direction: map[string]string{"usd": "asc", "name": "desc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sortArrival.next(1) != sortName || sortName.next(1) != sortUSD || sortUSD.next(1) != sortArrival {
		t.Error("the cycle isn't the one configured")
	}
	if sortMana.next(1) != sortArrival {
		t.Error("an order left out of the cycle should step to its start")
	}
	if sortUSD.descending() || !sortName.descending() {
		t.Error("the directions weren't applied")
	}
	if !sortPower.descending() {
		t.Error("an order the config doesn't mention lost its default direction")
	}

	if err := SetSortConfig(&config.Sort{Cycle: []string{"mana value", "bogus"}}); err == nil {
		t.Error("an unknown order name went unreported")
	}
	if firstSort() != sortMana {
		t.Error("the usable part of a half-broken cycle wasn't kept")
	}
}

func TestScryfallOrderColoursTheNamesByType(t *testing.T) {
	sol := mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}
	if got := nameColour(sol, sortArrival, sortArrival); got != stats.TypeColour(sol.TypeLine) {
		t.Errorf("Scryfall order painted an artifact %v", got)
	}
}

func TestThePrintedHistoryReadsNewestFirst(t *testing.T) {
	m := withCards(sized(140, 40), "f", []deck.Card{{Card: historyCard()}}, sortArrival)
	m = drive(m, "g", "v")
	h := m.histories["oid"]
	h.state = histReady
	h.revisions = []prints.TextRevision{
		{Text: "Flying (old)", SetName: "Alpha", Released: "1993-08-05"},
		{Text: "Flying", SetName: "Foundations", Released: "2024-11-15", Current: true},
	}
	body := stripANSI(strings.Join(m.renderHistory(historyCard(), 60), "\n"))
	if strings.Index(body, "Foundations") > strings.Index(body, "Alpha") {
		t.Errorf("the oldest wording is on top:\n%s", body)
	}
}

func TestEscBackIsOfferedOverThePrintedTextAndThePicture(t *testing.T) {
	withKitty(t, true)
	for _, keys := range [][]string{{"g", "v"}, {"g", "x"}} {
		m := withCards(sized(140, 40), "f", twoCards(), sortArrival)
		m = drive(m, keys...)
		if got := offered(m, "back"); got != "esc" {
			t.Errorf("%s: esc back is offered as %q", strings.Join(keys, ""), got)
		}
	}
	m := withCards(sized(140, 40), "f", twoCards(), sortArrival)
	if got := offered(m, "back"); got != "" {
		t.Error("esc back is offered over a plain card")
	}
}

func TestTheLeaderMenuIsOnlyTheLeadersKeys(t *testing.T) {
	got := stripANSI(strings.Join(sized(140, 30).leaderReference(200), " "))
	if strings.Contains(got, "keys") {
		t.Errorf("the leader menu offers ? keys: %s", got)
	}
}

func TestEChoosesTheNextDeckFromUnderTheEditingDeck(t *testing.T) {
	m, _, _ := twoDecks(t)
	m.ws.editing = 1
	m.hintsExpanded = true
	m = focusOn(m, 0) // the search
	search := stripANSI(m.viewPanel(m.ws.panels[0], 0, 60, 30, 2))
	editing := stripANSI(m.viewPanel(m.ws.panels[1], 1, 60, 30, 2))
	if strings.Contains(search, "next/prev deck") {
		t.Errorf("e is offered under the search:\n%s", search)
	}
	if !strings.Contains(editing, "e E next/prev deck") {
		t.Errorf("e isn't offered under the editing deck:\n%s", editing)
	}
}

func TestFirstAndLastAreOffered(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	if got := offered(m, "first/last"); got != "gg G" {
		t.Errorf("gg G offered as %q", got)
	}
}

func TestTheMarkersRankEditingThenFocusedThenOther(t *testing.T) {
	m := sized(240, 30)
	m = withCards(m, "f", sample(), sortArrival)     // 0: the list being marked
	m = withCards(m, "f", sample()[1:], sortArrival) // 1: focused — elves, Sol Ring, Forest
	m = withCards(m, "f", sample()[2:], sortArrival) // 2: another — Sol Ring, Forest
	m = withCards(m, "d", sample()[3:], sortArrival) // 3: the editing deck — Forest
	m.ws.panels[3].cardsView().deck = &deck.Info{Name: "Deck", Slug: "deck", Format: "commander"}
	m.ws.editing = 3
	m = focusOn(m, 1)

	members := m.membersFor(m.ws.panels[0].cardsView())
	for name, want := range map[string]membership{
		"forest":                 inEditing, // in all three: the editing deck wins
		"sol ring":               inFocused, // focused and another: focused wins
		"llanowar elves":         inFocused,
		"dwynen, gilt-leaf daen": notElsewhere,
	} {
		if members[name] != want {
			t.Errorf("%s is marked %v, want %v", name, members[name], want)
		}
	}

	// Each mark wears its panel's border colour.
	plain := deck.Card{Card: mtg.Card{Name: "Sol Ring"}}
	for member, want := range map[membership]string{
		inEditing: string(theme.BorderEditing), inFocused: string(theme.BorderFocus), inOther: string(theme.MemberOther),
	} {
		if _, col := marker(plain, rowState{member: member}); string(col) != want {
			t.Errorf("mark %v is %s, want %s", member, col, want)
		}
	}
	if theme.BorderEditing == theme.BorderFocus || theme.BorderEditing == theme.MemberOther {
		t.Error("the three marks aren't three colours")
	}
}

func TestTheLeaderMenuFillsTheWidthBeforeWrapping(t *testing.T) {
	// A coloured separator used to be measured by its escape codes, and the
	// menu wrapped with most of the line still empty.
	if lines := sized(200, 30).leaderReference(198); len(lines) != 1 {
		t.Errorf("the menu takes %d lines at 198 wide", len(lines))
	}
}

func TestGGAsksForTheRulingsOfTheCardItLandsOn(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	for i := range m.ws.current().cardsView().all {
		m.ws.current().cardsView().all[i].Card.ID = "id" + itoa(i)
	}
	m.ws.current().cardsView().refresh()
	m = drive(m, "j", "j")
	seq := m.hoverSeq
	m = drive(m, "g", "g")
	if m.hoverSeq == seq {
		t.Error("gg landed on a card and didn't ask for its rulings")
	}
}

func TestUndoCloseBringsBackWhatOnlyClosed(t *testing.T) {
	m := sized(240, 30)
	m = withCards(m, "f", sample()[:1], sortArrival)
	m = withCards(m, "f", sample()[1:2], sortArrival)
	m = withCards(m, "f", sample()[2:3], sortArrival)
	m = withCards(m, "f", sample()[3:], sortArrival)
	want := append([]*panel(nil), m.ws.panels...)
	m = focusOn(m, 1)
	m = drive(m, "space", "o")
	if m.ws.count() != 1 {
		t.Fatalf("only left %d panels", m.ws.count())
	}
	for i := 0; i < 3; i++ {
		m = drive(m, "space", "u")
	}
	if m.ws.count() != 4 {
		t.Fatalf("undo close brought back %d of 4", m.ws.count())
	}
	for i, p := range want {
		if m.ws.panels[i] != p {
			t.Errorf("panel %d isn't back where it was", i)
		}
	}
}

func TestEveryStatMarkFitsItsOneCellGutter(t *testing.T) {
	a, b := stats.Row{Label: "removal"}, stats.Row{Label: "Creature"}
	for _, op := range []stats.Op{stats.And, stats.Or, stats.AndNot} {
		expr := stats.Expr{{Op: stats.And, Row: a}, {Op: op, Row: b}}
		for _, r := range []stats.Row{a, b} {
			if w := lipgloss.Width(statMark(expr, r)); w != 1 {
				t.Errorf("%v: the mark %q is %d cells", op, statMark(expr, r), w)
			}
		}
	}
}
