package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// Text too long for a narrow column wraps, or is cut clean; nothing that
// was cut ends in "…" (the progress "searching…" and the like are not cuts).
func TestANarrowScreenCutsNothingWithDots(t *testing.T) {
	long := deck.Card{Qty: 1, Tags: []string{"an-uncommonly-long-tag-name"}, Card: mtg.Card{
		Name: "Asmoranomardicadaistinaculdacar, the Heir", TypeLine: "Legendary Creature — Human Wizard Noble",
		ManaCost: "{B}{R}", CMC: 2, OracleText: "When this enters, search your library for a card named The Underworld Cookbook.",
	}}
	cards := append(deckSample(), long)
	m := withCards(sized(70, 30), "d", cards, sortArrival)
	m = drive(m, "G")
	check := func(state string) {
		t.Helper()
		view := strings.ReplaceAll(stripANSI(m.View()), "fetching…", "") // rulings on their way
		if strings.Contains(view, "…") {
			t.Errorf("%s: a cut ends in dots:\n%s", state, view)
		}
	}
	check("card")
	m = drive(m, "s", "ctrl+p", "ctrl+p")
	check("statistics with odds")
	m = drive(m, "enter", "ctrl+j", "ctrl+a")
	check("statistics filtered")
	m = drive(m, "?")
	check("keys shown")
}

func TestANarrowStatisticsHeadingDropsWhatReadsLeast(t *testing.T) {
	b := statBar{row: headingRow("Mana Value (excl. lands)"), heading: true, value: "2 in filter", labelWidth: 18}
	if got := strings.TrimSpace(stripANSI(b.render())); got != "▸ Mana Value · 2" {
		t.Errorf("got %q", got)
	}
	b.labelWidth = 40
	if got := strings.TrimSpace(stripANSI(b.render())); got != "▸ Mana Value (excl. lands) · 2 in filter" {
		t.Errorf("wide: got %q", got)
	}
}
