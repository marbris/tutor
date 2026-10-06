package ui

import (
	"testing"

	"ttr/internal/mtg"
)

// printingUp is a list with Sol Ring's picture already fetched and gx
// pressed on it.
func printingUp(t *testing.T) Model {
	t.Helper()
	withKitty(t, true)
	card := mtg.Card{Name: "Sol Ring", OracleID: "sol"}
	m := withCards(sized(160, 40), "f", sample()[2:3], sortArrival)
	m.ws.current().cardsView().all[0].Card = card
	m.ws.current().cardsView().refresh()
	m.images[imageKey(card)] = &cardPrintings{state: imgReady, list: []mtg.Card{
		{ID: "cmm1", Set: "cmm", SetName: "Commander Masters", ReleasedAt: "2023-08-04"},
	}}
	m.pictures["cmm1"] = &picture{state: imgReady, png: []byte("png"), w: 488, h: 680}
	m = drive(m, "g", "x")
	if m.info.mode != infoImage {
		t.Fatalf("gx left the panel in %v", m.info.mode)
	}
	return m
}

func TestClosingTheStatisticsGoesBackToThePrinting(t *testing.T) {
	for _, keys := range [][]string{{"s", "s"}, {"S", "s"}, {"s", "S", "S"}} {
		m := printingUp(t)
		m.ws.editing = m.ws.focused
		m = drive(m, keys...)
		if m.info.mode != infoImage {
			t.Errorf("%v from the printing view ended in %v", keys, m.info.mode)
		}
		if m.kitty.key != "cmm1" {
			t.Errorf("%v: the picture wasn't put back", keys)
		}
	}
}

func TestClosingTheStatisticsElsewhereGoesBackToTheCard(t *testing.T) {
	m := withCards(sized(160, 40), "f", sample(), sortArrival)
	m = drive(m, "s", "s")
	if m.info.mode != infoCard {
		t.Errorf("s s ended in %v", m.info.mode)
	}
	m = drive(m, "s", "g", "x")
	m = drive(m, "g", "x")
	m = drive(m, "s", "s")
	if m.info.mode != infoCard {
		t.Errorf("the printing closed under the statistics came back: %v", m.info.mode)
	}
}

func TestTheRulesOpenedFromThePrintingViewShowTheRule(t *testing.T) {
	m := printingUp(t)
	m.rules = fakeRules(t)
	m = drive(m, "space", "r")
	if m.ws.current().kind != KindRules {
		t.Fatalf("space r focused a %v panel", m.ws.current().kind)
	}
	if got := m.infoTitle(); got != "rule" {
		t.Errorf("the info panel is headed %q over the rules", got)
	}
	if m.kitty.key != "" {
		t.Error("the picture stayed in the terminal over the rules")
	}

	m = drive(m, "h")
	if got := m.infoTitle(); got != "printing" {
		t.Errorf("back on the list the info panel is headed %q", got)
	}
	if m.kitty.key != "cmm1" {
		t.Error("the picture didn't come back with the list")
	}
}
