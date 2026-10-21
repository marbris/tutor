package ui

import (
	"os"
	"testing"
	"time"

	"ttr/internal/mtg"
	"ttr/internal/prints"
)

// forgetChosen starts a test with no printings chosen, and reads the file
// again on next use.
func forgetChosen(t *testing.T) {
	t.Helper()
	reset := func() {
		os.Remove(chosenPath())
		chosenPrintings.Lock()
		chosenPrintings.loaded = false
		chosenPrintings.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func TestWherePrintingsAreLeftIsRemembered(t *testing.T) {
	forgetChosen(t)
	m := printingView(t, 160)
	drive(m, "H")
	if got := chosenFor("sol"); got != "old" {
		t.Fatalf("H left Sol Ring on %q, remembered %q", "old", got)
	}

	// Read back from the file, as the next run would.
	chosenPrintings.Lock()
	chosenPrintings.loaded = false
	chosenPrintings.Unlock()
	if got := chosenFor("sol"); got != "old" {
		t.Errorf("the choice didn't survive being read back: %q", got)
	}
}

func TestACardOpensOnThePrintingChosenForIt(t *testing.T) {
	forgetChosen(t)
	printings := []mtg.Card{
		{ID: "new", Set: "sld", SetType: "box", ReleasedAt: "2025-01-01", IllustrationID: "a", ImageURIs: mtg.ImageURIs{Normal: "n"}},
		{ID: "mid", Set: "cmm", SetType: "masters", ReleasedAt: "2023-08-04", IllustrationID: "b", ImageURIs: mtg.ImageURIs{Normal: "n"}},
		{ID: "old", Set: "lea", SetType: "core", ReleasedAt: "1993-08-05", IllustrationID: "c", ImageURIs: mtg.ImageURIs{Normal: "n"}},
	}
	own := mtg.Card{ID: "mid", Name: "Sol Ring", OracleID: "sol"}
	today := time.Now().Format("2006-01-02")

	choose("sol", "old")
	list, at, _ := prints.Start(printings, withChosen(own, printings), today)
	if list[at].ID != "old" {
		t.Errorf("opened on %q, want the chosen printing", list[at].ID)
	}

	// A choice no longer among the printings gives way to the card's own.
	choose("sol", "gone")
	list, at, _ = prints.Start(printings, withChosen(own, printings), today)
	if list[at].ID != "mid" {
		t.Errorf("with the choice gone, opened on %q, want the card's own", list[at].ID)
	}
}
