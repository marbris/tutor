package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/stats"
	"ttr/internal/theme"
)

// ── The second order ────────────────────────────────────────────

func TestTheSecondOrderBreaksTheFirstOnesTies(t *testing.T) {
	cards := []deck.Card{
		{Card: mtg.Card{Name: "A Green Elf", TypeLine: "Creature — Elf", Colors: []string{"G"}}},
		{Card: mtg.Card{Name: "B White Knight", TypeLine: "Creature — Knight", Colors: []string{"W"}}},
		{Card: mtg.Card{Name: "C Blue Bolt", TypeLine: "Instant", Colors: []string{"U"}}},
		{Card: mtg.Card{Name: "D Black Wurm", TypeLine: "Creature — Wurm", Colors: []string{"B"}}},
	}
	got := sortCards(cards, sortSpec{first: sortType, then: sortColor})
	var names []string
	for _, c := range got {
		names = append(names, c.Card.Name)
	}
	// Creatures first, and WUBRG among them; then the instant.
	want := []string{"B White Knight", "D Black Wurm", "A Green Elf", "C Blue Bolt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("sorted %v, want %v", names, want)
	}
}

func TestTheSecondOrderColoursTheNames(t *testing.T) {
	green := mtg.Card{Name: "Elf", TypeLine: "Creature — Elf", Colors: []string{"G"}, Rarity: "mythic"}
	if got := nameColour(green, sortType, sortColor); got != theme.ManaG {
		t.Errorf("type then colour: name is %v, want green", got)
	}
	if got := nameColour(green, sortMana, sortRarity); got != stats.RarityColour("mythic") {
		t.Errorf("mana then rarity: name is %v, want mythic", got)
	}
	// The numbers go on a ramp: a seven-drop is hotter than a two-drop, and
	// anything past the top step is as hot as it.
	two, seven, nine := green, green, green
	two.CMC, seven.CMC, nine.CMC = 2, 7, 9
	if nameColour(two, sortType, sortMana) == nameColour(seven, sortType, sortMana) {
		t.Error("mana value two and seven are the same colour")
	}
	if nameColour(seven, sortType, sortMana) != nameColour(nine, sortType, sortMana) {
		t.Error("seven and nine aren't both the top of the ramp")
	}
	// Price and rank by band: a dollar apart is the same band.
	cheap, cheaper := green, green
	cheap.Prices.USD, cheaper.Prices.USD = "0.40", "0.10"
	dear := green
	dear.Prices.USD = "80.00"
	if nameColour(cheap, sortType, sortUSD) != nameColour(cheaper, sortType, sortUSD) {
		t.Error("two cards under a dollar are different colours")
	}
	if nameColour(cheap, sortType, sortUSD) == nameColour(dear, sortType, sortUSD) {
		t.Error("forty cents and eighty dollars are the same colour")
	}
	top, deep := green, green
	top.EDHRECRank, deep.EDHRECRank = 5, 50000
	if nameColour(top, sortType, sortEDHREC) == nameColour(deep, sortType, sortEDHREC) {
		t.Error("rank 5 and rank 50000 are the same colour")
	}
}

func TestCommaCyclesTheSecondOrderAndKeepsTheCursor(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortType)
	l := m.ws.current().cardsView()
	l.selectByName("Sol Ring")
	m = drive(m, ",")
	if l.order2 == sortArrival {
		t.Fatal(", didn't set a second order")
	}
	if c, _ := l.current(); c.Card.Name != "Sol Ring" {
		t.Errorf("the cursor moved to %s", c.Card.Name)
	}
	m = drive(m, "<")
	if l.order2 != sortArrival {
		t.Errorf("< didn't step back: %v", l.order2)
	}
	_ = m
}

// ── NOT in the statistics ───────────────────────────────────────

func TestNInTheStatisticsNegates(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, "s")
	for i := 0; i < 40; i++ {
		if r, ok := m.statUnder(); ok && r.Group == "Type" && r.Label == "Land" {
			break
		}
		m = drive(m, "J")
	}
	m = drive(m, "alt+n")
	l := m.ws.current().cardsView()
	for _, c := range l.rows {
		if mtg.IsLand(c.Card) {
			t.Errorf("NOT Land kept %s", c.Card.Name)
		}
	}
	if !strings.Contains(l.statFilter.String(), "¬Land") {
		t.Errorf("filter reads %q", l.statFilter.String())
	}
}

// ── i on a deck ─────────────────────────────────────────────────

func ownDeck(m Model) (Model, *cardList) {
	m = withCards(m, "d", sample(), sortArrival)
	l := m.ws.current().cardsView()
	l.deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
	m.ws.editing = m.ws.focused // as opening it for real would make it
	return m, l
}

func TestIOnADeckAsksForACard(t *testing.T) {
	m, _ := ownDeck(sized(140, 30))
	m = drive(m, "i")
	p := m.ws.current()
	if p.asking != askAddCard {
		t.Fatalf("i on a deck asked %v", p.asking)
	}
	if p.searchOpen {
		t.Error("i opened the panel's search bar too")
	}
}

func TestIOnSomebodyElsesDeckRefuses(t *testing.T) {
	m := withCards(sized(140, 30), "d", sample(), sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Theirs"}
	m = drive(m, "i")
	if p := m.ws.current(); p.asking != askNone || p.searchOpen {
		t.Error("i on a borrowed deck raised a prompt")
	}
	if m.notice == "" {
		t.Error("nothing said why")
	}
}

func TestAddCardAddsOneAnswer(t *testing.T) {
	m, l := ownDeck(sized(140, 30))
	bolt := deck.Card{Card: mtg.Card{Name: "Lightning Bolt", TypeLine: "Instant"}}
	next, _ := m.Update(addCardMsg{panel: m.ws.current().id, query: "bolt",
		cards: []deck.Card{bolt}, total: 1})
	m = next.(Model)
	if i := l.indexOfCard("Lightning Bolt"); i < 0 || l.all[i].Qty != 1 {
		t.Fatal("the one answer wasn't added")
	}
	if !l.dirty {
		t.Error("the deck isn't marked as edited")
	}
}

func TestAddCardPrefersTheExactName(t *testing.T) {
	m, l := ownDeck(sized(140, 30))
	many := []deck.Card{
		{Card: mtg.Card{Name: "Forest Bear"}},
		{Card: mtg.Card{Name: "Forest"}},
		{Card: mtg.Card{Name: "Dryad of the Forest"}},
	}
	before := l.all[l.indexOfCard("Forest")].Qty
	next, _ := m.Update(addCardMsg{panel: m.ws.current().id, query: "forest", cards: many, total: 3})
	m = next.(Model)
	if got := l.all[l.indexOfCard("Forest")].Qty; got != before+1 {
		t.Errorf("Forest is %d, want %d", got, before+1)
	}
	if l.indexOfCard("Forest Bear") >= 0 {
		t.Error("a near miss was added too")
	}
}

func TestAddCardAsksAgainWhenAmbiguous(t *testing.T) {
	m, l := ownDeck(sized(140, 30))
	n := len(l.all)
	many := []deck.Card{{Card: mtg.Card{Name: "Elvish Mystic"}}, {Card: mtg.Card{Name: "Elvish Visionary"}}}
	next, _ := m.Update(addCardMsg{panel: m.ws.current().id, query: "elvish", cards: many, total: 2})
	m = next.(Model)
	if len(l.all) != n {
		t.Error("an ambiguous answer changed the deck")
	}
	if !strings.Contains(m.notice, "2 matches") {
		t.Errorf("notice %q", m.notice)
	}
	p := m.ws.current()
	if p.asking != askAddCard || p.askInput.Value() != "elvish" {
		t.Errorf("the prompt didn't come back with the query: %v %q", p.asking, p.askInput.Value())
	}
}

// ── The header ──────────────────────────────────────────────────

func TestTheHeaderWrapsRatherThanCutting(t *testing.T) {
	m, l := ownDeck(sized(60, 30))
	l.setFilter("elf")
	l.statFilter, _ = l.statFilter.Add(stats.And, stats.Row{Group: "Type", Label: "Creature",
		Match: func(c deck.Card) bool { return strings.Contains(c.Card.TypeLine, "Creature") }})
	l.refresh()
	p := m.ws.current()

	for _, width := range []int{14, 20, 30, 60} {
		lines := p.subLines(width)
		text := strings.Join(lines, " ")
		for _, want := range []string{"committed", "[1]2/10", "as found", "/elf", "Creature"} {
			if !strings.Contains(strings.ReplaceAll(text, " ", ""), strings.ReplaceAll(want, " ", "")) {
				t.Errorf("at %d the header lost %q:\n%s", width, want, strings.Join(lines, "\n"))
			}
		}
		for _, line := range lines {
			if textWidth(line) > width {
				t.Errorf("at %d a header line is %d wide: %q", width, textWidth(line), line)
			}
			if strings.Contains(line, "…") {
				t.Errorf("at %d the header was cut: %q", width, line)
			}
		}
	}
	if len(p.subLines(14)) <= len(p.subLines(60)) {
		t.Error("a narrower panel didn't take more lines")
	}
}

func TestPanelsShareOneHeaderHeight(t *testing.T) {
	// A search has one header row and a filtered deck three; the rules under
	// them still land on the same line, or the rows of cards don't line up.
	m := withCards(sized(160, 30), "f", sample(), sortArrival)
	m, l := ownDeck(m)
	l.setFilter("elf")
	for _, line := range splitLines(stripANSI(m.View())) {
		if strings.Count(line, "│─") >= 2 {
			return
		}
	}
	t.Errorf("the panels' rules aren't level:\n%s", stripANSI(m.View()))
}

// ── Hints in the panels ─────────────────────────────────────────

func TestQuestionMarkPutsTheKeysInThePanels(t *testing.T) {
	m, _ := ownDeck(sized(160, 40))
	m = drive(m, "?")
	view := stripANSI(m.View())
	// No headings in the panels: every key in the block is about the panel
	// it sits in.
	for _, heading := range []string{"info panel:", "navigation:", "select:"} {
		if strings.Contains(view, heading) {
			t.Errorf("the keys are headed %q", heading)
		}
	}
	for _, want := range []string{"select", "sort 1 & 2", "add from scryfall", "card history"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q isn't on screen:\n%s", want, view)
		}
	}
}

func TestTheEscHintSaysWhatEscWillDo(t *testing.T) {
	m, l := ownDeck(sized(160, 40))
	p := m.ws.current()
	steps := []struct {
		setup func()
		want  string
	}{
		{func() { l.marks["sol ring"] = true }, "drop picks"},
		{func() { l.setFilter("elf") }, "clear /filter"},
		{func() {
			l.statFilter, _ = l.statFilter.Add(stats.And, stats.Row{Group: "Type", Label: "Land",
				Match: func(c deck.Card) bool { return mtg.IsLand(c.Card) }})
			l.refresh()
		}, "clear stats-filter"},
	}
	for _, s := range steps {
		s.setup()
	}
	// And then nothing: esc doesn't close the panel, so it has no hint.
	for _, want := range []string{"drop picks", "clear /filter", "clear stats-filter", ""} {
		label, _ := (&m).escStep(p)
		if label != want {
			t.Fatalf("esc hint says %q, want %q", label, want)
		}
		m = drive(m, "esc")
	}
	if m.ws.count() != 1 {
		t.Error("esc closed the panel")
	}
}

func TestTheStatisticsLeaveTheListItsOtherKeys(t *testing.T) {
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, "s", "v")
	if l := m.ws.current().cardsView(); l.markCount() != 1 {
		t.Errorf("v under the statistics picked %d cards", l.markCount())
	}
	m = drive(m, "/")
	if !m.ws.current().filtering {
		t.Error("/ under the statistics didn't open the filter")
	}
}

func TestTheFirstOrderColoursANumericColumn(t *testing.T) {
	// The column takes the same colour the name would under the same order
	// as the second one, so price, rank and power read alike either way.
	// Without a terminal lipgloss draws no colour at all, and every render
	// would compare equal.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	dim := lipgloss.NewStyle().Foreground(theme.TextDim).Render("x")
	dear := mtg.Card{Name: "Mox", Prices: mtg.Prices{USD: "900.00"}, EDHRECRank: 3, Power: "6", Toughness: "1"}
	for _, order := range []cardSort{sortUSD, sortEDHREC, sortPower, sortToughness} {
		want, _ := sortColour(dear, order)
		got := paintColumn("x", dear, order)
		if got != lipgloss.NewStyle().Foreground(want).Render("x") || got == dim {
			t.Errorf("%v: the column isn't painted with the order's colour: %q", order, got)
		}
	}
}
