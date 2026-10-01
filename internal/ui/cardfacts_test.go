package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"ttr/internal/mtg"
	"ttr/internal/stats"
)

func TestPrintingFactsSplitPrintingFromWorth(t *testing.T) {
	c := mtg.Card{SetName: "Universes Within", Rarity: "rare", EDHRECRank: 10619,
		Prices: mtg.Prices{USD: "2.53"}}
	want := []string{"Universes Within · rare", "edhrec #10619 · $2.53"}
	if got := factLines(c); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintingFactsLeaveOutWhatACardLacks(t *testing.T) {
	// No price at all is left out, not shown as a dash; a foil-only price
	// still counts.
	cases := []struct {
		card mtg.Card
		want []string
	}{
		{mtg.Card{SetName: "Alpha", EDHRECRank: 5}, []string{"Alpha", "edhrec #5"}},
		{mtg.Card{Rarity: "rare"}, []string{"rare"}},
		{mtg.Card{Prices: mtg.Prices{USDFoil: "10"}}, []string{"$10.00"}},
		{mtg.Card{}, nil},
	}
	for _, tc := range cases {
		if got := factLines(tc.card); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%+v: got %q, want %q", tc.card, got, tc.want)
		}
	}
}

func TestStatsRowSitsFlushRight(t *testing.T) {
	row := stripANSI(statsRow(mtg.Card{Power: "7", Toughness: "4"}, 20))
	if row != strings.Repeat(" ", 17)+"7/4" {
		t.Errorf("got %q", row)
	}
	if statsRow(mtg.Card{TypeLine: "Instant"}, 20) != "" {
		t.Error("an instant has no stats row")
	}
}

func TestPrintingFactsColourTheRarity(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)
	c := mtg.Card{SetName: "Universes Within", Rarity: "mythic", EDHRECRank: 10}
	got := printingFacts(c, 40)
	if len(got) != 2 || strings.TrimSpace(stripANSI(got[0])) != "Universes Within · mythic" {
		t.Fatalf("got %q", got)
	}
	want := lipgloss.NewStyle().Foreground(stats.RarityColour("mythic")).Render("mythic")
	if !strings.HasSuffix(strings.TrimRight(got[0], " "), want) {
		t.Errorf("the rarity isn't in its colour: %q", got[0])
	}
	// Narrow, the set name wraps and the rarity still ends it, coloured.
	narrow := printingFacts(c, 12)
	if last := narrow[len(narrow)-2]; !strings.HasSuffix(strings.TrimRight(last, " "), want) {
		t.Errorf("wrapped, the rarity lost its colour: %q", narrow)
	}
}
