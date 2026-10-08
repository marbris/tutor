package stats

import (
	"math"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

func find(t *testing.T, rows []Row, label string) Row {
	t.Helper()
	for _, r := range rows {
		if r.Label == label {
			return r
		}
	}
	t.Fatalf("no %q row", label)
	return Row{}
}

func TestExprFoldsFromTheLeftInTheOrderItWasBuilt(t *testing.T) {
	// The example from the notes: a creature, a black, an artifact (or), a
	// mana value of five — artifacts and black creatures, all costing five.
	creature := find(t, everyType(), "Creature")
	artifact := find(t, everyType(), "Artifact")
	black := find(t, colorRows(), "Black")
	five := find(t, cmcRows(), "5")

	var e Expr
	e, _ = e.Add(And, creature)
	e, _ = e.Add(And, black)
	e, _ = e.Add(Or, artifact)
	e, _ = e.Add(And, five)

	if got, want := e.String(), "((Creature ∧ Black) ∨ Artifact) ∧ 5"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}

	card := func(tl string, cmc float64, colors ...string) deck.Card {
		return deck.Card{Card: mtg.Card{Name: tl, TypeLine: tl, CMC: cmc, Colors: colors}, Qty: 1}
	}
	cases := []struct {
		c    deck.Card
		want bool
	}{
		{card("Creature — Horror", 5, "B"), true},
		{card("Creature — Horror", 4, "B"), false},
		{card("Creature — Angel", 5, "W"), false},
		{card("Artifact", 5), true},
		{card("Artifact", 3), false},
		{card("Enchantment", 5, "B"), false},
	}
	for _, c := range cases {
		if got := e.Match(c.c); got != c.want {
			t.Errorf("%s cmc %v %v: match = %v, want %v", c.c.Card.TypeLine, c.c.Card.CMC, c.c.Card.Colors, got, c.want)
		}
	}
}

func TestExprFirstOpDoesntMatterAndDuplicatesArentAdded(t *testing.T) {
	black := find(t, colorRows(), "Black")
	var a, o Expr
	a, _ = a.Add(And, black)
	o, _ = o.Add(Or, black)
	c := deck.Card{Card: mtg.Card{TypeLine: "Instant", Colors: []string{"U"}}}
	if a.Match(c) != o.Match(c) {
		t.Error("the first clause's op changed the result")
	}
	if _, added := a.Add(Or, black); added {
		t.Error("the same category was added twice")
	}
}

func TestWithoutDropsOnlyThatCategory(t *testing.T) {
	var e Expr
	e, _ = e.Add(And, find(t, cmcRows(), "1"))
	e, _ = e.Add(Or, find(t, cmcRows(), "2"))
	e, _ = e.Add(And, find(t, everyType(), "Creature"))
	e = e.Without(find(t, cmcRows(), "1"))
	if got := e.String(); got != "2 ∧ Creature" {
		t.Errorf("after dropping 1: %q", got)
	}
}

func TestEmptyExprMatchesEverything(t *testing.T) {
	if !(Expr{}).Match(deck.Card{}) {
		t.Error("an empty narrowing narrowed")
	}
}

func TestAtLeast(t *testing.T) {
	near := func(got, want float64) bool { return math.Abs(got-want) < 0.001 }

	// 36 lands in 99: almost always one in seven.
	if got := AtLeast(99, 36, 1, HandSize); !near(got, 0.9628) {
		t.Errorf("P(≥1 of 36/99) = %.4f", got)
	}
	// A single copy in 60: 7/60.
	if got := AtLeast(60, 1, 1, HandSize); !near(got, 7.0/60) {
		t.Errorf("P(≥1 of 1/60) = %.4f", got)
	}
	// Four copies in 60, at least two: 1 - P(0) - P(1) = 0.0632.
	if got := AtLeast(60, 4, 2, HandSize); !near(got, 0.0632) {
		t.Errorf("P(≥2 of 4/60) = %.4f", got)
	}
	if got := AtLeast(60, 1, 2, HandSize); got != 0 {
		t.Errorf("two of a single copy = %v", got)
	}
	if got := AtLeast(5, 5, 4, HandSize); !near(got, 1) {
		t.Errorf("whole deck = %v", got)
	}
}

func TestAndNotDropsTheCategory(t *testing.T) {
	creature := find(t, everyType(), "Creature")
	black := find(t, colorRows(), "Black")
	card := func(tl string, colors ...string) deck.Card {
		return deck.Card{Card: mtg.Card{Name: tl, TypeLine: tl, Colors: colors}, Qty: 1}
	}
	blackElf := card("Creature — Elf", "B")
	greenElf := card("Creature — Elf", "G")
	blackSpell := card("Instant", "B")

	// After another category, n is AND NOT: creatures that aren't black.
	var e Expr
	e, _ = e.Add(And, creature)
	e, _ = e.Add(AndNot, black)
	if got, want := e.String(), "Creature ∧¬ Black"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	if e.Match(blackElf) || !e.Match(greenElf) || e.Match(blackSpell) {
		t.Error("creature AND NOT black matched the wrong cards")
	}

	// First, it is plain NOT: everything that isn't black.
	var n Expr
	n, _ = n.Add(AndNot, black)
	if got, want := n.String(), "¬Black"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	if n.Match(blackElf) || !n.Match(greenElf) || n.Match(blackSpell) {
		t.Error("NOT black matched the wrong cards")
	}

	// And it folds like the others: (¬Black ∨ Creature) ∧¬ … reads left to right.
	n, _ = n.Add(Or, creature)
	if got, want := n.String(), "¬Black ∨ Creature"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	if !n.Match(blackElf) || n.Match(blackSpell) {
		t.Error("NOT black OR creature matched the wrong cards")
	}
}

// everyType is a row for each card type, shown or not.
func everyType() []Row {
	var out []Row
	for _, t := range cardTypes {
		out = append(out, TypeRow(typePathPrefix+t))
	}
	return out
}

func TestToggleTakesOutTheSameOpAndSwitchesAnother(t *testing.T) {
	creature := find(t, everyType(), "Creature")
	black := find(t, colorRows(), "Black")
	five := find(t, cmcRows(), "5")

	var e Expr
	e = e.Toggle(And, creature)
	e = e.Toggle(And, black)
	e = e.Toggle(And, five)
	if got := e.String(); got != "(Creature ∧ Black) ∧ 5" {
		t.Fatalf("built %q", got)
	}

	// Another op switches it in place.
	e = e.Toggle(Or, black)
	if got := e.String(); got != "(Creature ∨ Black) ∧ 5" {
		t.Errorf("alt+o on Black: %q", got)
	}
	e = e.Toggle(AndNot, black)
	if got := e.String(); got != "(Creature ∧¬ Black) ∧ 5" {
		t.Errorf("alt+n on Black: %q", got)
	}
	// The same op again takes it out.
	e = e.Toggle(AndNot, black)
	if got := e.String(); got != "Creature ∧ 5" {
		t.Errorf("alt+n twice on Black: %q", got)
	}

	// On the first category And and Or are one thing, so either takes it out.
	if got := e.Toggle(Or, creature).String(); got != "5" {
		t.Errorf("alt+o on the first (and) category: %q", got)
	}
	// But a negated first category is switched back, not taken out.
	e = e.Toggle(AndNot, creature)
	if got := e.String(); got != "¬Creature ∧ 5" {
		t.Errorf("alt+n on the first category: %q", got)
	}
	if got := e.Toggle(And, creature).String(); got != "Creature ∧ 5" {
		t.Errorf("alt+a on a negated first category: %q", got)
	}
}
