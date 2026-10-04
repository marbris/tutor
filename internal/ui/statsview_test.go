package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/stats"
	"ttr/internal/theme"
)

func deckSample() []deck.Card {
	mk := func(name, tl, cost string, cmc float64, colors []string, rarity string, qty int, tags ...string) deck.Card {
		return deck.Card{Qty: qty, Tags: tags, Card: mtg.Card{
			Name: name, TypeLine: tl, ManaCost: cost, CMC: cmc,
			Colors: colors, Rarity: rarity,
		}}
	}
	return []deck.Card{
		mk("Llanowar Elves", "Creature — Elf Druid", "{G}", 1, []string{"G"}, "common", 1, "ramp"),
		mk("Elvish Mystic", "Creature — Elf Druid", "{G}", 1, []string{"G"}, "common", 1, "ramp"),
		mk("Sol Ring", "Artifact", "{1}", 1, nil, "uncommon", 1, "ramp"),
		mk("Beast Within", "Instant", "{2}{G}", 3, []string{"G"}, "uncommon", 1, "removal"),
		mk("Craterhoof Behemoth", "Creature — Beast", "{5}{G}{G}{G}", 8, []string{"G"}, "mythic", 1, "wincon"),
		mk("Forest", "Basic Land — Forest", "", 0, nil, "common", 12),
	}
}

func TestSShowsTheHistograms(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	view := stripANSI(m.View())
	for _, want := range []string{"Tags", "ramp", "Type", "Creature", "Mana Value"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q is missing from the statistics:\n%s", want, view)
		}
	}
}

// statGroupTitles is the groups in the order they're drawn.
func statGroupTitles(m Model) []string {
	var out []string
	for _, g := range m.statGroups() {
		out = append(out, g.Title)
	}
	return out
}

// pointStat puts the highlight on a category by name.
func pointStat(t *testing.T, m Model, group, label string) Model {
	t.Helper()
	for _, r := range statRows(m.statGroups()) {
		if r.Group == group && r.Label == label {
			m.pointAt(r)
			return m
		}
	}
	t.Fatalf("no %s %s row", group, label)
	return m
}

func TestWalkingTheBarsDoesntNarrow(t *testing.T) {
	// Reading the bars and filtering by them are separate acts: you can walk
	// every category without the list moving under you.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s", "j", "j", "j")

	if l.count() != 6 {
		t.Errorf("walking the bars narrowed the list to %d", l.count())
	}
	if c, _ := l.current(); c.Card.Name != "Llanowar Elves" {
		t.Errorf("j moved the list's cursor to %s, not the bars'", c.Card.Name)
	}
	if m.statCursor(m.statGroups()) != 3 {
		t.Errorf("the highlight is on row %d, want 3", m.statCursor(m.statGroups()))
	}
}

func TestSGoesBackToTheList(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s", "a") // narrow to the first category: tagged ramp
	narrowed := l.count()
	if narrowed != 3 {
		t.Fatalf("adding ramp left %d cards, want 3", narrowed)
	}

	m = drive(m, "s")
	if m.info.mode == infoStats {
		t.Fatal("s did not close the statistics")
	}
	if l.count() != narrowed {
		t.Errorf("closing the statistics undid the filter: %d cards", l.count())
	}
	// The keys are the list's again.
	m = drive(m, "j")
	if c, _ := l.current(); c.Card.Name == l.rows[0].Card.Name {
		t.Error("j didn't move the list after leaving the statistics")
	}
}

func TestShiftJAndKTurnTheGroupsOver(t *testing.T) {
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	full := []string{"Tags", "Type", "Color (excl. lands)", "Mana Value (excl. lands)", "Rarity", "Price (USD)"}
	have := statGroupTitles(m)
	// The sample has no prices, so that group is absent; the order holds.
	want := []string{}
	for _, t := range full {
		for _, h := range have {
			if h == t {
				want = append(want, t)
			}
		}
	}
	if strings.Join(have, ",") != strings.Join(want, ",") {
		t.Fatalf("opened in the order %v", have)
	}

	m = drive(m, "J")
	if got := statGroupTitles(m); strings.Join(got, ",") != strings.Join(append(want[1:], want[0]), ",") {
		t.Errorf("J: %v", got)
	}
	m = drive(m, "J")
	if got := statGroupTitles(m); strings.Join(got, ",") != strings.Join(append(want[2:], want[:2]...), ",") {
		t.Errorf("J J: %v", got)
	}
	m = drive(m, "K", "K", "K")
	last := len(want) - 1
	if got := statGroupTitles(m); strings.Join(got, ",") != strings.Join(append([]string{want[last]}, want[:last]...), ",") {
		t.Errorf("K from the start: %v", got)
	}
	// The highlight goes to the new top.
	if row, _ := m.statUnder(); row.Group != statRows(m.statGroups())[0].Group {
		t.Errorf("highlight on %s %s, not the top group", row.Group, row.Label)
	}
}

func TestAddingCategoriesFoldsAndAndOr(t *testing.T) {
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s")

	m = pointStat(t, m, "Type", "Creature")
	m = drive(m, "a")
	if l.count() != 3 {
		t.Fatalf("creatures: %d", l.count())
	}
	m = pointStat(t, m, "Mana Value", "1")
	m = drive(m, "a")
	if l.count() != 2 {
		t.Errorf("creatures ∧ 1: %d, want the two elves", l.count())
	}
	m = pointStat(t, m, "Type", "Artifact")
	m = drive(m, "o")
	if l.count() != 3 {
		t.Errorf("(creatures ∧ 1) ∨ artifact: %d, want the elves and the Sol Ring", l.count())
	}
	if got := l.statFilter.String(); got != "(Creature ∧ 1) ∨ Artifact" {
		t.Errorf("filter reads %q", got)
	}
	if !strings.Contains(stripANSI(m.View()), "(Creature ∧ 1) ∨ Artifact") {
		t.Error("the filter isn't shown")
	}

	// x takes out the highlighted category alone: Artifact goes, Creature
	// from the same group stays.
	m = drive(m, "x")
	if got := l.statFilter.String(); got != "Creature ∧ 1" {
		t.Errorf("after x on Artifact: %q", got)
	}
	if l.count() != 2 {
		t.Errorf("creatures ∧ 1: %d cards", l.count())
	}
	// X clears the rest: x, only more.
	m = drive(m, "X")
	if len(l.statFilter) != 0 || l.count() != 6 {
		t.Errorf("X left %q, %d cards", l.statFilter.String(), l.count())
	}
}

func TestBInTheStatisticsClearsBothFilters(t *testing.T) {
	// The statistics leave b to the list: it clears the text filter and the
	// categories together, as it does with the statistics closed.
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "/", "e", "l", "enter", "s", "a")
	if l.filter == "" || len(l.statFilter) == 0 {
		t.Fatal("not narrowed both ways")
	}
	m = drive(m, "b")
	if l.filter != "" || len(l.statFilter) != 0 {
		t.Errorf("b left the filter %q and the categories %q", l.filter, l.statFilter.String())
	}
	if m.info.mode != infoStats {
		t.Error("b closed the statistics")
	}
}

func TestEscStepsBackAFilterAtATime(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "/", "e", "l", "enter", "s", "a")
	if l.filter == "" || len(l.statFilter) == 0 {
		t.Fatal("not narrowed both ways")
	}

	m = drive(m, "esc")
	if len(l.statFilter) != 0 || l.filter == "" {
		t.Error("the first esc should take the categories and leave the text filter")
	}
	m = drive(m, "esc")
	if l.filter != "" || m.info.mode != infoStats {
		t.Error("the second esc should take the text filter and stay in the statistics")
	}
	m = drive(m, "esc")
	if m.info.mode == infoStats {
		t.Error("the third esc should leave the statistics")
	}
	if m.ws.count() != 1 {
		t.Error("esc closed the panel")
	}
}

func TestPStepsThroughTheOdds(t *testing.T) {
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s", "p")
	if m.stats.odds != 1 {
		t.Fatalf("odds = %d", m.stats.odds)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "P(≥1 in opening 7)") || !strings.Contains(view, "%") {
		t.Errorf("no odds on show:\n%s", view)
	}
	m = drive(m, "p", "p", "p", "p")
	if m.stats.odds != 0 {
		t.Errorf("five presses left odds at %d, want back to counts", m.stats.odds)
	}
	m = drive(m, "P")
	if m.stats.odds != 4 {
		t.Errorf("P from counts: %d, want 4", m.stats.odds)
	}
}

func TestTheOddsAreForTheWholeList(t *testing.T) {
	// Twelve Forests in eighteen cards, seven drawn: all but certain.
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s", "p")
	for _, line := range m.renderStats(60) {
		plain := stripANSI(line)
		if strings.Contains(plain, "Land ") && !strings.Contains(plain, "100%") {
			t.Errorf("land odds: %q", plain)
		}
	}
}

func TestStatisticsFollowTheFocusedList(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)
	m = withCards(m, "d", deckSample(), sortArrival)
	m = drive(m, "s")

	counted, _ := m.statCards()
	if len(counted) != len(deckSample()) {
		t.Fatalf("counting %d", len(counted))
	}
	m = drive(m, "h")
	if m.info.mode != infoStats {
		t.Fatal("h left the statistics")
	}
	counted, _ = m.statCards()
	if len(counted) != len(sample()) {
		t.Errorf("after h the statistics count %d, want the search's %d", len(counted), len(sample()))
	}
}

func TestShiftSCountsTheEditingDeck(t *testing.T) {
	m, search, target := editing(t)
	m = focusOn(m, 0)
	m = drive(m, "S")
	if !m.stats.editing {
		t.Fatal("S did not go to the editing deck")
	}
	counted, _ := m.statCards()
	if len(counted) != len(target.all) {
		t.Errorf("counted %d, want the editing deck's %d", len(counted), len(target.all))
	}
	m = drive(m, "a")
	if len(target.statFilter) == 0 || len(search.statFilter) != 0 {
		t.Error("the filter should land on the editing deck alone")
	}
}

func TestShiftBClearsEveryList(t *testing.T) {
	m := sized(200, 30)
	m = withCards(m, "f", sample(), sortArrival)
	search := m.ws.panels[0].cardsView()
	m = drive(m, "s", "a", "s")
	m = withCards(m, "d", deckSample(), sortArrival)
	target := m.ws.panels[1].cardsView()
	m = drive(m, "s", "a", "s")
	if len(search.statFilter) == 0 || len(target.statFilter) == 0 {
		t.Fatal("not both narrowed")
	}
	m = drive(m, "B")
	if len(search.statFilter) != 0 || len(target.statFilter) != 0 {
		t.Error("B left a list narrowed")
	}
}

func TestTheBarsShareOneScale(t *testing.T) {
	// Scaling each group to its own widest bar would make a deck with two
	// of something look like a deck full of it.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	lines := m.renderStats(40)
	var lands, wincon string
	for _, line := range lines {
		plain := stripANSI(line)
		if strings.HasPrefix(plain, "Land") {
			lands = plain
		}
		if strings.HasPrefix(plain, "wincon") {
			wincon = plain
		}
	}
	if lands == "" || wincon == "" {
		t.Skip("those categories didn't appear")
	}
	if strings.Count(lands, "█") <= strings.Count(wincon, "█") {
		t.Errorf("twelve lands drew no bigger a bar than one wincon:\n%s\n%s", lands, wincon)
	}
}

func TestOneCardNeverReadsAsNone(t *testing.T) {
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	for _, line := range m.renderStats(40) {
		plain := stripANSI(line)
		if strings.HasPrefix(plain, "wincon") && !strings.Contains(plain, "█") {
			t.Errorf("a category with a card in it drew an empty bar: %q", plain)
		}
	}
}

func TestThePanelScrollsToKeepTheCategoryInView(t *testing.T) {
	// j past the bottom used to move a cursor you could no longer see.
	m := withCards(sized(90, 16), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	const room = 8
	if m.statOffset(room) != 0 {
		t.Fatalf("started scrolled to %d", m.statOffset(room))
	}
	for i := 0; i < 12; i++ {
		m = drive(m, "j")
	}
	if m.statOffset(room) == 0 {
		t.Error("walking down twelve categories never scrolled")
	}

	// And the highlighted category is inside the window.
	line := statLine(m.statGroups(), m.statCursor(m.statGroups()))
	at := m.statOffset(room)
	if line < at || line >= at+room {
		t.Errorf("category on line %d, showing %d..%d", line, at, at+room)
	}
}

func TestScrollingComesBackUpAgain(t *testing.T) {
	m := withCards(sized(90, 16), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	for i := 0; i < 12; i++ {
		m = drive(m, "j")
	}
	for i := 0; i < 12; i++ {
		m = drive(m, "k")
	}
	if got := m.statOffset(8); got != 0 {
		t.Errorf("came back to the top still scrolled to %d", got)
	}
}

func TestStatLineCountsHeadingsAndGaps(t *testing.T) {
	// The panel scrolls by rendered lines, not by category, so this has to
	// agree with what renderStats draws.
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	groups := m.statGroups()
	lines := m.renderStats(30)
	rows := statRows(groups)

	for row := 0; row < len(rows); row++ {
		at := statLine(groups, row)
		if at >= len(lines) {
			t.Fatalf("category %d maps to line %d of %d", row, at, len(lines))
		}
		if !strings.Contains(stripANSI(lines[at]), rows[row].Label) {
			t.Errorf("category %d (%s) maps to line %d, which says %q",
				row, rows[row].Label, at, stripANSI(lines[at]))
		}
	}
}

// searchSample is what a Scryfall search puts in a list: cards with no
// quantity, because nobody has chosen how many.
func searchSample() []deck.Card {
	var out []deck.Card
	for _, c := range deckSample() {
		c.Qty, c.Tags = 0, nil
		out = append(out, c)
	}
	return out
}

func TestSearchResultsHaveStatisticsToo(t *testing.T) {
	// They counted to nothing, because a search result has no quantity and
	// the bars summed quantities — so every row's base was zero, every row
	// was dropped, and the panel said "nothing to count" over a full screen.
	m := withCards(sized(120, 30), "f", searchSample(), sortArrival)
	m = drive(m, "s")

	groups := m.statGroups()
	if len(groups) == 0 {
		t.Fatal("a search of six cards produced no statistics at all")
	}
	body := strings.Join(m.renderStats(60), "\n")
	if strings.Contains(stripANSI(body), "nothing to count") {
		t.Error(`the panel still says "nothing to count"`)
	}

	// Three creatures among the six, counted one apiece.
	var creatures *stats.Row
	for i, r := range statRows(groups) {
		if r.Group == "Type" && r.Label == "Creature" {
			creatures = &statRows(groups)[i]
		}
	}
	if creatures == nil {
		t.Fatal("no Creature row")
	}
	if creatures.Base != 3 || creatures.Count != 3 {
		t.Errorf("Creature counted %d of %d, want 3 of 3", creatures.Count, creatures.Base)
	}
}

func TestADeckStillCountsItsCopies(t *testing.T) {
	// The floor at one must not flatten a real deck's twelve Forests.
	m := withCards(sized(120, 30), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	for _, r := range statRows(m.statGroups()) {
		if r.Group == "Type" && r.Label == "Land" {
			if r.Base != 12 {
				t.Errorf("twelve Forests counted as %d", r.Base)
			}
			return
		}
	}
	t.Fatal("no Land row")
}

func TestTheStatisticsHintsDontOfferTheKeysTheyTake(t *testing.T) {
	// j, k, a, x, o, O, p and P mean something else while the statistics
	// have the keys, so the list's hints for them would be offering keys that
	// don't do that. The rest — c, u, e, v, y — still reach the list, and
	// stay on offer.
	m, _, _ := editing(t)
	m = drive(m, "s")
	offered := map[string]bool{}
	for _, g := range m.hintGroups() {
		for _, k := range g.keys {
			offered[k[0]] = true
			if g.title == "statistics" {
				continue
			}
			for _, f := range strings.Fields(k[0]) {
				if statsTaken()[f] {
					t.Errorf("%q %q is offered in %q while the statistics have it", k[0], k[1], g.title)
				}
			}
		}
	}
	for _, want := range []string{"c", "u", "v V", "y"} {
		if !offered[want] {
			t.Errorf("%q isn't offered while the statistics are up", want)
		}
	}
}

func TestTurningAGroupOverShowsItsHeading(t *testing.T) {
	// After ctrl+j had scrolled the statistics, J put the next group on top
	// but scrolled only as far as its first category, so the heading saying
	// what the bars count sat one line above the top edge.
	m := withCards(sized(110, 22), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = next.(Model)
	for _, key := range []string{"J", "J", "K"} {
		m = drive(m, key)
		want := m.statGroups()[0].Title
		lines := strings.Split(stripANSI(m.View()), "\n")
		if !strings.Contains(lines[3], want) {
			t.Errorf("after %s the panel starts %q, want the %q heading", key, lines[3], want)
		}
	}
}

func TestStatHeadingIsTheGroupsFirstLine(t *testing.T) {
	m := withCards(sized(110, 22), "d", deckSample(), sortArrival)
	groups := m.statGroups()
	row := 0
	for _, g := range groups {
		for range g.Rows {
			if got, line := statHeading(groups, row), statLine(groups, row); got >= line {
				t.Errorf("row %d on line %d has its heading on %d", row, line, got)
			}
			row++
		}
	}
	// The first row of the second group: its heading is the line above it.
	first := len(groups[0].Rows)
	if statHeading(groups, first) != statLine(groups, first)-1 {
		t.Error("a group's first category isn't directly under its heading")
	}
}

func TestTheHighlightedStatWearsItsBarsColour(t *testing.T) {
	// Under the cursor the row takes its bar's colour as the background, so
	// the highlight says which category as well as which row.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	b := statBar{
		row:  stats.Row{Label: "Blue", Color: theme.ManaU},
		mark: " ", fraction: 0.5, value: "12",
		labelWidth: 8, barWidth: 10, countWidth: 3,
	}
	plain := b.render()
	b.under = true
	got := b.render()

	param := strings.TrimSuffix(strings.TrimPrefix(bgStart(theme.ManaU), "\x1b["), "m")
	if param == "" || !strings.Contains(got, param) {
		t.Errorf("the highlighted row isn't on the bar's colour: %q", got)
	}
	if stripANSI(got) != stripANSI(plain) {
		t.Errorf("the highlight changed the row's text:\n%q\n%q", stripANSI(got), stripANSI(plain))
	}
}

func TestJAndKWalkRoundTheGroups(t *testing.T) {
	// The groups are a ring: k off the top brings the group before round to
	// the top and lands on its last row; j off the bottom sends the top group
	// round to the bottom and lands on its first. Either way the highlight
	// moves one row, never jumps.
	m := withCards(sized(90, 40), "d", deckSample(), sortArrival)
	m = drive(m, "s")

	groups := m.statGroups()
	if len(groups) < 2 {
		t.Fatalf("need two groups to wrap between, have %d", len(groups))
	}
	first, last := groups[0], groups[len(groups)-1]

	m = drive(m, "k")
	got := m.statGroups()
	if got[0].Title != last.Title {
		t.Errorf("k off the top brought %q to the top, want %q", got[0].Title, last.Title)
	}
	if r, _ := m.statUnder(); r.Group != last.Rows[0].Group || r.Label != last.Rows[len(last.Rows)-1].Label {
		t.Errorf("k off the top landed on %s/%s, want the last row of %s", r.Group, r.Label, last.Title)
	}
	if got[1].Title != first.Title {
		t.Errorf("the old top group isn't just below: %q", got[1].Title)
	}

	// And back down: j off the last row sends the top group to the bottom.
	m = drive(m, "j") // onto what was the first row
	rows := statRows(m.statGroups())
	for m.statCursor(m.statGroups()) < len(rows)-1 {
		m = drive(m, "j")
	}
	top := m.statGroups()[0]
	m = drive(m, "j")
	got = m.statGroups()
	if got[len(got)-1].Title != top.Title {
		t.Errorf("j off the bottom sent %q to the bottom, want %q", got[len(got)-1].Title, top.Title)
	}
	if r, _ := m.statUnder(); r.Label != top.Rows[0].Label || r.Group != top.Rows[0].Group {
		t.Errorf("j off the bottom landed on %s/%s, want the first row of %s", r.Group, r.Label, top.Title)
	}
}

func TestTabOnATagSortsTheTagsByName(t *testing.T) {
	cards := deckSample()
	cards[3].Tags = []string{"anthem"} // Beast Within: alone, but first by name
	m := withCards(sized(120, 30), "d", cards, sortArrival)
	m = drive(m, "s")
	tags := func() string {
		for _, g := range m.statGroups() {
			if g.Title == "Tags" {
				var out []string
				for _, r := range g.Rows {
					out = append(out, r.Label)
				}
				return strings.Join(out, " ")
			}
		}
		return ""
	}
	offers := func(what string) bool {
		for _, g := range m.hintGroups() {
			for _, k := range g.keys {
				if k[1] == what {
					return true
				}
			}
		}
		return false
	}
	if got := tags(); got != "ramp anthem wincon untagged" {
		t.Fatalf("tags start as %q, want commonest first", got)
	}
	if !offers("tags by name") {
		t.Error("no hint for tab on a tag")
	}
	m = drive(m, "tab")
	if got := tags(); got != "anthem ramp wincon untagged" {
		t.Errorf("tags by name = %q", got)
	}
	if !offers("tags by count") {
		t.Error("the hint didn't follow the order")
	}
	if r, _ := m.statUnder(); r.Label != "ramp" {
		t.Errorf("the highlight moved to %q", r.Label)
	}

	// Off the tags, tab is not the statistics' to take.
	m = drive(m, "J")
	if m.onTags() {
		t.Fatal("J stayed on the tags")
	}
	if offers("tags by count") {
		t.Error("the tab hint shows off the tags")
	}
}

func TestWalkingBackUpHoldsTheViewUntilTheTopEdge(t *testing.T) {
	m := withCards(sized(90, 16), "d", deckSample(), sortArrival)
	m = drive(m, "s")
	for i := 0; i < 12; i++ {
		m = drive(m, "j")
	}
	bottom := m.info.offset
	if bottom == 0 {
		t.Fatal("walking down twelve categories never scrolled")
	}
	_, room, _, _ := m.infoSpan()
	groups := m.statGroups()
	line := statLine(groups, m.statCursor(groups))
	// Up through the rows on screen: the view stays where it is.
	for line-1 > bottom {
		m = drive(m, "k")
		groups = m.statGroups()
		line = statLine(groups, m.statCursor(groups))
		if line <= bottom {
			break // a heading or gap passed; the next k reaches the edge
		}
		if m.info.offset != bottom {
			t.Fatalf("k on line %d moved the view from %d to %d (room %d)", line, bottom, m.info.offset, room)
		}
	}
	// Past the top edge it follows.
	for i := 0; i < 12; i++ {
		m = drive(m, "k")
	}
	if m.info.offset >= bottom {
		t.Errorf("going on up never scrolled back (%d)", m.info.offset)
	}
}

func TestTheBarsAreCountedOnceUntilSomethingTheyCountChanges(t *testing.T) {
	// j and k only move the highlight: they draw from the bars as counted,
	// rather than counting the list again several times a key.
	m := withCards(sized(120, 40), "d", deckSample(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s")
	m.View()
	counted := m.stats.memo.groups
	m = drive(m, "j", "j", "k", "J", "K")
	m.View()
	if &m.stats.memo.groups[0] != &counted[0] {
		t.Error("walking the bars counted them again")
	}

	// Filtering by one changes what they count: counted again.
	m = pointStat(t, m, "Type", "Creature")
	m = drive(m, "a")
	m.View()
	if &m.stats.memo.groups[0] == &counted[0] {
		t.Fatal("filtering didn't count the bars again")
	}
	if !strings.Contains(stripANSI(m.View()), "Creature") || l.count() != 3 {
		t.Errorf("after filtering: %d cards", l.count())
	}

	// So does an edit to the list.
	before := m.stats.memo.groups
	m = drive(m, "X")
	l.all = l.all[:len(l.all)-1]
	l.refresh()
	m.View()
	if &m.stats.memo.groups[0] == &before[0] {
		t.Error("an edit didn't count the bars again")
	}
}
