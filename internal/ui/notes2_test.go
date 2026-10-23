package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// The changes from notes/post-implementation-notes-2.md (outside the repo) that are worth a test
// of their own.

// creatures is a handful of cards with power and toughness set, for the
// power/toughness sort — sample() has none.
func creatures() []deck.Card {
	return []deck.Card{
		{Card: mtg.Card{Name: "Grizzly Bears", TypeLine: "Creature — Bear",
			Power: "2", Toughness: "2"}},
		{Card: mtg.Card{Name: "Tarmogoyf", TypeLine: "Creature — Lhurgoyf",
			Power: "4", Toughness: "5"}},
		{Card: mtg.Card{Name: "Ornithopter", TypeLine: "Artifact Creature — Thopter",
			Power: "0", Toughness: "2"}},
		{Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}},
	}
}

func TestSortingByPowerIsBiggestFirstAndLandsThoseWithout(t *testing.T) {
	sorted := sortCards(creatures(), sortSpec{first: sortPower, desc1: true})
	got := make([]string, len(sorted))
	for i, c := range sorted {
		got[i] = c.Card.Name
	}
	want := []string{"Tarmogoyf", "Grizzly Bears", "Ornithopter", "Sol Ring"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("power order is %v, want %v", got, want)
	}
}

func TestSortingByToughnessBreaksTiesByPower(t *testing.T) {
	sorted := sortCards(creatures(), sortSpec{first: sortToughness, desc1: true})
	// Tarmogoyf (5) then the two 2-toughness creatures, biggest power first,
	// then Sol Ring which has neither.
	got := make([]string, len(sorted))
	for i, c := range sorted {
		got[i] = c.Card.Name
	}
	want := []string{"Tarmogoyf", "Grizzly Bears", "Ornithopter", "Sol Ring"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("toughness order is %v, want %v", got, want)
	}
}

func TestPowerAndToughnessAreShownTogether(t *testing.T) {
	c := deck.Card{Card: mtg.Card{Name: "Tarmogoyf", Power: "4", Toughness: "5"}}
	for _, order := range []cardSort{sortPower, sortToughness} {
		if got := row(c, order, 30); !strings.Contains(got, "4/5") {
			t.Errorf("sorted by %v, the row is %q, want it to show 4/5", order, got)
		}
	}
}

func TestTabToTheDecksTargetShowsTheDecksAtOnce(t *testing.T) {
	// A blank panel, tabbed round to the decks target, lists the decks
	// straight away rather than waiting for enter — with the bar still open
	// over them.
	m := drive(sized(140, 40), "space", "f")
	m = drive(m, "tab") // find → decks
	p := m.ws.current()
	if p.kind != KindDecks {
		t.Fatalf("two tabs landed on %v, want decks", p.kind)
	}
	if _, ok := p.top().(*deckList); !ok {
		t.Errorf("the decks aren't shown until enter: top is %T", p.top())
	}
	if !p.searchOpen {
		t.Error("the bar closed; it should stay open over the preview")
	}

	// Tabbing on to rules clears the preview rather than leaving decks under
	// a rules panel.
	m = drive(m, "tab")
	if p.top() != nil {
		t.Errorf("the decks preview outlived the decks target: %T", p.top())
	}
}

func TestCtrlLCarriesThePanelRight(t *testing.T) {
	m := openPanel(sized(200, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	m = focusOn(m, 0)
	m = drive(m, "ctrl+l")

	if m.ws.panels[0].kind != KindDecks {
		t.Errorf("panels are %v, %v; want find carried to the right",
			m.ws.panels[0].kind, m.ws.panels[1].kind)
	}
	if m.ws.focused != 1 {
		t.Errorf("focus stayed at %d rather than travelling with the panel", m.ws.focused)
	}
}

func TestTheLeaderMenuHasNoBackgroundBand(t *testing.T) {
	// A band of colour across the bottom contrasts with a semi-transparent
	// terminal; the menu is plain text now, so it carries no background
	// escape (ESC[48;…).
	m := drive(sized(140, 40), "space")
	if !m.leader {
		t.Fatal("space did not raise the leader menu")
	}
	if raw := m.viewLeaderBar(); strings.Contains(raw, "\x1b[48") || strings.Contains(raw, "48;") {
		t.Errorf("the leader menu still paints a background:\n%q", raw)
	}
}

func TestThePanelIndexIsGone(t *testing.T) {
	// The hint bar no longer shows "which panel of how many".
	m := openPanel(sized(120, 40), "f", "angel")
	m = openPanel(m, "d", "marbri")
	view := stripANSI(m.View())
	for _, gone := range []string{"1/2", "2/2"} {
		if strings.Contains(view, gone) {
			t.Errorf("the panel index %q is still shown:\n%s", gone, view)
		}
	}
}

func TestEachHintGroupIsOneRow(t *testing.T) {
	// A group renders on exactly one line, however many keys it has — the
	// overflow is dropped, not wrapped.
	m := withCards(sized(160, 30), "f", sample(), sortArrival)
	groups := m.hintGroups()
	lines := m.hintGroupLines(groups, 160)
	if got, want := len(lines), len(groups); got != want {
		t.Errorf("%d hint rows for %d groups; each group should be one row", got, want)
	}
	for i, line := range lines {
		if w := visibleWidth(line); w > 160 {
			t.Errorf("row %d is %d columns, past the 160 it was given", i, w)
		}
	}
}

func TestTheHintBarStartsCollapsedAndQuestionMarkGrowsIt(t *testing.T) {
	// At rest the bar shows only ? and q; ? grows the keys into the panels
	// and the leader's menu along the bottom, and ? again shrinks it back.
	// There is no separate reference window — the panels stay visible
	// throughout.
	m := withCards(sized(140, 30), "f", sample(), sortArrival)

	rest := stripANSI(m.View())
	if !strings.Contains(rest, "? keys") || !strings.Contains(rest, "q quit") {
		t.Errorf("the resting bar doesn't show ? and q:\n%s", rest)
	}
	if strings.Contains(rest, "space") {
		t.Errorf("the resting bar still offers the menu:\n%s", rest)
	}
	if strings.Contains(rest, "sort 1 & 2") || strings.Contains(rest, "space:") {
		t.Errorf("the resting bar already shows the full keymap:\n%s", rest)
	}
	// The panels are still there — ? is not a window that replaces them.
	if !strings.Contains(rest, "Sol Ring") {
		t.Errorf("the panels aren't visible at rest:\n%s", rest)
	}

	m = drive(m, "?")
	grown := stripANSI(m.View())
	for _, want := range []string{"sort 1 & 2", "select", "space: f find", "Sol Ring"} {
		if !strings.Contains(grown, want) {
			t.Errorf("after ? the bar/panels don't show %q:\n%s", want, grown)
		}
	}

	m = drive(m, "?")
	if shrunk := stripANSI(m.View()); strings.Contains(shrunk, "navigation:") {
		t.Errorf("? again did not shrink the bar:\n%s", shrunk)
	}
}
