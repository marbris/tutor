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

func TestInitialismsFromTheDesign(t *testing.T) {
	// The three worked examples in the design doc, which between them cover
	// the awkward parts: an em dash between clauses, a hyphenated word, a
	// comma, and little words that must stay little.
	for in, want := range map[string]string{
		"Legendary Creature — Elf Faerie Noble": "LC-EFN",
	} {
		if got := initialism(in); got != want {
			t.Errorf("initialism(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Dwynen, Gilt-Leaf Daen":    "D,GLD",
		"Miara, Thorn of the Glade": "M,TotG",
	} {
		if got := initialism(in); got != want {
			t.Errorf("initialism(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInitialismKeepsCaseSoLittleWordsStayLittle(t *testing.T) {
	// "of the" becoming "OT" would read as two more nouns.
	if got := initialism("Sword of Feast and Famine"); got != "SoFaF" {
		t.Errorf("got %q, want SoFaF", got)
	}
}

func TestInitialismHandlesDoubleFacedNames(t *testing.T) {
	// The "//" between the halves of a split or double-faced name survives as
	// "//", so the two halves stay legibly two rather than joining across a
	// dash.
	if got := initialism("Brutal Cathar // Moonrage Brute"); got != "BC//MB" {
		t.Errorf("got %q, want BC//MB", got)
	}
	if got := initialism("Battle - Siege // Creature - Dragon"); got != "B-S//C-D" {
		t.Errorf("got %q, want B-S//C-D", got)
	}
}

func TestTypeColumnSharesANameBoundaryAcrossRows(t *testing.T) {
	// Sorted by type, the whole list abbreviates the type column on one shared
	// boundary rather than each row on its own fit — so a short-named row can't
	// show a fuller type than a long-named one, which is the overlap the column
	// layout removes.
	short := deck.Card{Card: mtg.Card{Name: "Ix", TypeLine: "Legendary Creature — Elf"}}
	long := deck.Card{Card: mtg.Card{
		Name: "A Reasonably Long Deck Name", TypeLine: "Legendary Creature — Elf",
	}}
	l := newCardList([]deck.Card{short, long}, sortType, "")

	lines := l.render(40, 5, nil, false, false)
	joined := stripANSI(lines[0]) + "\n" + stripANSI(lines[1])
	// The long name — which fits — sets the shared column wide, leaving the
	// type column too narrow for the full line, so both rows show the
	// initialism. The short-named row does not get to keep the full type.
	if !strings.Contains(joined, "A Reasonably Long Deck Name") || !strings.Contains(joined, "Ix") {
		t.Fatalf("both cards should be present in full:\n%s", joined)
	}
	if strings.Count(joined, "LC-E") != 2 {
		t.Errorf("both type columns should abbreviate on the shared boundary:\n%s", joined)
	}
}

func TestTheTypeColumnListNeverOverrunsItsWidth(t *testing.T) {
	// The shared-column path is not the per-row ladder, so it earns its own
	// width check across the range a panel can be.
	cards := []deck.Card{
		{Card: mtg.Card{Name: "Ix", TypeLine: "Creature — Elf"}},
		{Card: mtg.Card{Name: "A Reasonably Long Deck Name",
			TypeLine: "Legendary Enchantment Creature — God"}},
		{Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}},
	}
	l := newCardList(cards, sortType, "")
	for width := 6; width <= 60; width++ {
		for _, line := range l.render(width, len(cards), nil, false, false) {
			if got := textWidth(stripANSI(line)); got > width {
				t.Errorf("width %d rendered %d columns: %q", width, got, stripANSI(line))
			}
		}
	}
}

func TestSortByUSDColumnShowsThePrice(t *testing.T) {
	c := deck.Card{Card: mtg.Card{Name: "Sol Ring", Prices: mtg.Prices{USD: "3.99"}}}
	if got := row(c, sortUSD, 40); !strings.Contains(got, "$3.99") {
		t.Errorf("the usd column does not show the price: %q", got)
	}

	// A card with no price shows a dash rather than posing as free.
	none := deck.Card{Card: mtg.Card{Name: "Some Token"}}
	if got := row(none, sortUSD, 40); !strings.Contains(got, "—") {
		t.Errorf("a priceless card should show a dash: %q", got)
	}
}

func TestManaCostLosesItsBraces(t *testing.T) {
	for in, want := range map[string]string{
		"{3}{B}{G}": "3BG",
		"{W}":       "W",
		"":          "",
		"{2}{W/U}":  "2W/U",
		"{X}{R}{R}": "XRR",
	} {
		got := manaCost(mtg.Card{ManaCost: in})
		if got != want {
			t.Errorf("manaCost(%q) = %q, want %q", in, got, want)
		}
	}
}

// row is the visible text of a rendered row, which is what the ladder is
// really about.
func row(c deck.Card, order cardSort, width int) string {
	return stripANSI(renderRow(c, order, rowState{}, width))
}

func TestTheRowIsExactlyAsWideAsItIsToldToBe(t *testing.T) {
	c := deck.Card{Card: mtg.Card{
		Name: "Dwynen, Gilt-Leaf Daen", TypeLine: "Legendary Creature — Elf Faerie Noble",
		ManaCost: "{2}{G}{G}",
	}}
	for _, order := range allSorts {
		for width := 6; width <= 60; width++ {
			if got := textWidth(row(c, order, width)); got > width {
				t.Errorf("order %v at width %d rendered %d columns", order, width, got)
			}
		}
	}
}

func TestTheLadderGivesUpTheTypeLineBeforeTheName(t *testing.T) {
	c := deck.Card{Card: mtg.Card{
		Name:     "Dwynen, Gilt-Leaf Daen",
		TypeLine: "Legendary Creature — Elf Faerie Noble",
	}}

	// Wide: both in full.
	wide := row(c, sortType, 64)
	if !strings.Contains(wide, "Dwynen, Gilt-Leaf Daen") ||
		!strings.Contains(wide, "Legendary Creature — Elf Faerie Noble") {
		t.Errorf("at 64 columns the row should be unabbreviated:\n%q", wide)
	}

	// Narrower: the type line goes first, the name is still readable.
	mid := row(c, sortType, 34)
	if !strings.Contains(mid, "Dwynen, Gilt-Leaf Daen") {
		t.Errorf("the name was shortened before the type line:\n%q", mid)
	}
	if !strings.Contains(mid, "LC-EFN") {
		t.Errorf("the type line was not abbreviated:\n%q", mid)
	}

	// Narrower still: now the name goes too.
	narrow := row(c, sortType, 18)
	if !strings.Contains(narrow, "D,GLD") {
		t.Errorf("the name was not abbreviated:\n%q", narrow)
	}
}

func TestAManaCostIsNeverAbbreviated(t *testing.T) {
	// A cost is already as short as it goes, and half a cost is a lie.
	c := deck.Card{Card: mtg.Card{Name: "Dwynen, Gilt-Leaf Daen", ManaCost: "{2}{G}{G}"}}
	for width := 12; width <= 40; width++ {
		if got := row(c, sortMana, width); !strings.Contains(got, "2GG") {
			t.Errorf("width %d lost the mana cost: %q", width, got)
		}
	}
}

func TestARankIsNeverAbbreviated(t *testing.T) {
	c := deck.Card{Card: mtg.Card{Name: "Dwynen, Gilt-Leaf Daen", EDHRECRank: 1234}}
	if got := row(c, sortEDHREC, 16); !strings.Contains(got, "1234") {
		t.Errorf("the rank was mangled: %q", got)
	}
}

func TestQuantityRidesWithTheName(t *testing.T) {
	c := deck.Card{Qty: 7, Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains"}}
	if got := row(c, sortMana, 30); !strings.Contains(got, "7x Plains") {
		t.Errorf("got %q, want the count in front of the name", got)
	}

	// A single copy says nothing, because every card in a singleton deck
	// would say 1x and none of them would be telling you anything.
	one := deck.Card{Qty: 1, Card: mtg.Card{Name: "Sol Ring"}}
	if got := row(one, sortMana, 30); strings.Contains(got, "1x") {
		t.Errorf("got %q, want no count for a single copy", got)
	}
}

func TestTheMarkerSaysOneThingAtATime(t *testing.T) {
	c := deck.Card{Card: mtg.Card{Name: "Sol Ring"}, Commander: true}

	if got, _ := marker(c, rowState{selected: true}); got != "▸" {
		t.Errorf("a selected commander shows %q, want the selection", got)
	}
	if got, _ := marker(c, rowState{}); got != "★" {
		t.Errorf("a commander shows %q", got)
	}
	plain := deck.Card{Card: mtg.Card{Name: "Sol Ring"}}
	for member, want := range map[membership]string{inEditing: "●", inFocused: "◆", inOther: "▲"} {
		if got, _ := marker(plain, rowState{member: member}); got != want {
			t.Errorf("mark %v shows %q, want %q", member, got, want)
		}
	}
	if got, _ := marker(plain, rowState{}); got != " " {
		t.Errorf("an ordinary card shows %q, want nothing", got)
	}
}

func TestAVeryNarrowPanelStillShowsSomething(t *testing.T) {
	c := deck.Card{Card: mtg.Card{
		Name: "Miara, Thorn of the Glade", TypeLine: "Legendary Creature — Elf Scout",
	}}
	got := row(c, sortType, 8)
	if strings.TrimSpace(got) == "" {
		t.Error("an 8-column row came out blank")
	}
	if textWidth(got) > 8 {
		t.Errorf("%q is wider than 8", got)
	}
}

// deckCardNamed is a card with nothing but a name, for tests about layout
// rather than about cards.
func deckCardNamed(name string) deck.Card {
	return deck.Card{Card: mtg.Card{Name: name}}
}

func TestAnEmojiPresentationSequenceIsMeasuredAsTwoCells(t *testing.T) {
	// U+FE0F asks for the emoji form of the character before it, which
	// terminals draw double-width. runewidth measures the base alone and
	// says one, which overflows the row by a column.
	if got := textWidth("♟️"); got != 2 {
		t.Errorf("width of a chess pawn with the emoji selector is %d, want 2", got)
	}
	if got := textWidth("👑"); got != 2 {
		t.Errorf("width of a crown is %d, want 2", got)
	}
	if got := textWidth("abc"); got != 3 {
		t.Errorf("plain text measured %d", got)
	}
}

func TestTruncateAgreesWithTextWidth(t *testing.T) {
	// If the two use different measures, a row is cut to a width that then
	// measures as something else and the line wraps.
	for _, s := range []string{
		"👑-Marchesa d'Amati, First of her Name: Queens Gambit♟️",
		"Dwynen, Gilt-Leaf Daen",
		"日本語のカード名",
	} {
		for width := 1; width <= 40; width++ {
			if got := textWidth(truncate(s, width)); got > width {
				t.Errorf("truncate(%q, %d) measures %d", s, width, got)
			}
		}
	}
}

func TestSplitCardManaKeepsBothHalvesApart(t *testing.T) {
	// "2U // {2R" was the symptom: splitting on the closing brace left the
	// separator glued to the next symbol.
	for cost, want := range map[string]string{
		"{2}{U} // {2}{R}": "2U // 2R",
		"{3}{B}{G}":        "3BG",
		"{2}{W/U}":         "2W/U",
		"{X}{R}":           "XR",
		"":                 "",
	} {
		if got := manaCost(mtg.Card{ManaCost: cost}); got != want {
			t.Errorf("manaCost(%q) = %q, want %q", cost, got, want)
		}
	}
}

func TestTheNameTakesItsColourFromWhatYouSortedBy(t *testing.T) {
	// The order you chose is the question you are asking.
	green := mtg.Card{Name: "Llanowar Elves", TypeLine: "Creature — Elf", Colors: []string{"G"}}

	if got := nameColour(green, sortColor, sortArrival); got != theme.ManaG {
		t.Errorf("sorting by colour painted the name %v, want green", got)
	}
	// Sorting by type colours the type column, not the name: the name stays
	// plain so the coloured bands are the types alone.
	if got := nameColour(green, sortType, sortArrival); got != theme.Text {
		t.Errorf("sorting by type painted the name %v, want plain", got)
	}
	// Any other order, Scryfall's own included, leaves the name to say
	// what the rest of the row doesn't: the card's type.
	for _, order := range []cardSort{sortArrival, sortMana, sortUSD} {
		if got := nameColour(green, order, sortArrival); got != theme.Success {
			t.Errorf("sorting by %v painted the name %v, want the creature colour", order, got)
		}
	}
}

func TestEachCardTypeGetsItsOwnColour(t *testing.T) {
	// So a list ordered by type reads as bands rather than a grey column.
	seen := map[lipgloss.Color]string{}
	for _, line := range []string{
		"Creature — Elf", "Instant", "Sorcery", "Enchantment",
		"Legendary Planeswalker — Jace", "Basic Land — Forest",
	} {
		c := stats.TypeColour(line)
		if other, dup := seen[c]; dup {
			t.Errorf("%q and %q are the same colour", line, other)
		}
		seen[c] = line
	}
}

func TestManaIsPaintedSymbolBySymbol(t *testing.T) {
	// Which is how a curve is read at a glance; one colour for the whole
	// cost would say nothing a number doesn't.
	//
	// Asserted on the colours rather than the output, because lipgloss drops
	// styling entirely when nothing is attached to a terminal.
	if stripANSI(paintMana("2GG")) != "2GG" {
		t.Errorf("painting changed the text: %q", stripANSI(paintMana("2GG")))
	}

	generic, green := symbolColour("2"), symbolColour("G")
	if generic == green {
		t.Error("a generic cost and a green pip are the same colour")
	}
	for _, pair := range [][2]string{{"W", "U"}, {"U", "B"}, {"B", "R"}, {"R", "G"}} {
		if symbolColour(pair[0]) == symbolColour(pair[1]) {
			t.Errorf("%s and %s are the same colour", pair[0], pair[1])
		}
	}
	// A hybrid is neither of its halves, since picking one would be wrong
	// half the time.
	if h := symbolColour("W/U"); h == symbolColour("W") || h == symbolColour("U") {
		t.Error("a hybrid symbol took one of its halves' colours")
	}
}

func TestPaintingNeverChangesAColumnsWidth(t *testing.T) {
	// The ladder has already decided the widths; painting must not disturb
	// them or the row wraps.
	c := deck.Card{Card: mtg.Card{
		Name: "Dwynen, Gilt-Leaf Daen", TypeLine: "Legendary Creature — Elf Archer",
		ManaCost: "{2}{G}{G}", Colors: []string{"G"},
	}}
	for _, order := range allSorts {
		for width := 8; width <= 50; width++ {
			got := stripANSI(renderRow(c, order, rowState{}, width))
			if textWidth(got) > width {
				t.Errorf("order %v at width %d rendered %d columns", order, width, textWidth(got))
			}
		}
	}
}

func TestTheCursorRowWearsItsOwnColours(t *testing.T) {
	// Under the cursor the name and the column become blocks of their own
	// colours, rather than one neutral bar that hides what the sort says.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	c := deck.Card{Qty: 1, Card: mtg.Card{
		Name: "Merfolk Looter", TypeLine: "Creature — Merfolk Rogue",
		Colors: []string{"U"}, ManaCost: "{1}{U}",
	}}
	st := rowState{cursor: true, then: sortColor, member: inEditing}
	const width = 50
	got := renderRow(c, sortType, st, width)

	nameBg := nameColour(c.Card, sortType, sortColor)
	colBg := columnColour(c.Card, sortType)
	if nameBg == colBg {
		t.Fatalf("test needs two different colours, both are %s", nameBg)
	}
	for what, bg := range map[string]lipgloss.Color{"marker": theme.BorderEditing, "name": nameBg, "column": colBg} {
		// lipgloss folds the foreground into the same sequence, so look for
		// the background's parameter rather than a whole sequence.
		param := strings.TrimSuffix(strings.TrimPrefix(bgStart(bg), "\x1b["), "m")
		if param == "" || !strings.Contains(got, param) {
			t.Errorf("the %s isn't on its own colour (%s): %q", what, bg, got)
		}
	}
	if plain := stripANSI(got); textWidth(plain) != width {
		t.Errorf("cursor row is %d wide, want %d: %q", textWidth(plain), width, plain)
	}
	if stripANSI(got) != stripANSI(renderRow(c, sortType, rowState{then: sortColor, member: inEditing}, width)) {
		t.Error("the cursor changed the row's text, not just its colours")
	}
}

func TestTruncateCutsCleanWithoutDots(t *testing.T) {
	for _, c := range []struct {
		s     string
		width int
		want  string
	}{
		{"Craterhoof Behemoth", 10, "Craterhoof"},
		{"Craterhoof Behemoth", 1, "C"},
		{"Sol Ring", 8, "Sol Ring"},
		{"日本語の名前", 5, "日本"},
	} {
		if got := truncate(c.s, c.width); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.s, c.width, got, c.want)
		}
	}
}
