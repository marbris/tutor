package deck

import (
	"fmt"
	"strings"
	"testing"

	"ttr/internal/mtg"
)

func mc(name string, qty int, tags ...string) Card {
	return Card{Card: mtg.Card{Name: name}, Qty: qty, Tags: tags}
}

func show(cards []Card) string {
	var parts []string
	for _, c := range cards {
		parts = append(parts, fmt.Sprintf("%d %s%v", c.Qty, c.Card.Name, c.Tags))
	}
	return strings.Join(parts, ", ")
}

func TestMergeCardsKeepsBothSidesEdits(t *testing.T) {
	base := []Card{mc("Sol Ring", 1), mc("Island", 10), mc("Opt", 1)}
	theirs := []Card{mc("Sol Ring", 1, "ramp"), mc("Island", 10), mc("Opt", 1), mc("Brainstorm", 1)}
	ours := []Card{mc("Sol Ring", 1), mc("Island", 12), mc("Ponder", 1)} // took Opt out
	got := show(MergeCards(base, theirs, ours))
	want := "1 Sol Ring[ramp], 12 Island[], 1 Ponder[], 1 Brainstorm[]"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestMergeCardsOursWinsACardBothChanged(t *testing.T) {
	base := []Card{mc("Island", 10)}
	theirs := []Card{mc("Island", 8)}
	ours := []Card{mc("Island", 12)}
	if got := show(MergeCards(base, theirs, ours)); got != "12 Island[]" {
		t.Errorf("got %s", got)
	}
}

func TestMergeCardsRemovals(t *testing.T) {
	base := []Card{mc("Opt", 1), mc("Ponder", 1), mc("Island", 10)}
	// They took Opt out and retagged Ponder; we took Ponder out.
	theirs := []Card{mc("Ponder", 1, "draw"), mc("Island", 10)}
	ours := []Card{mc("Opt", 1), mc("Island", 10)}
	// Opt: theirs removed, we left it → gone. Ponder: we removed, they
	// changed → theirs keeps it.
	if got := show(MergeCards(base, theirs, ours)); got != "10 Island[], 1 Ponder[draw]" {
		t.Errorf("got %s", got)
	}
}

func TestMergeCardsWithNothingChangedIsOurs(t *testing.T) {
	base := []Card{mc("Opt", 1), mc("Island", 10)}
	if got := show(MergeCards(base, base, base)); got != show(base) {
		t.Errorf("got %s", got)
	}
}
