package stats

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

func TestLandsAreOutOfTheCurveAndTheColours(t *testing.T) {
	// A deck is mostly lands, and nearly all of them cost nothing and count
	// as colourless. Left in, they put a column at zero taller than the rest
	// of the curve put together and a "Colorless" bar that reads as if the
	// deck were full of colourless spells. The type breakdown already says
	// how many lands there are, and says it better.
	entries := []deck.Card{
		deck.Card{Card: mtg.Card{Name: "Command Tower", TypeLine: "Land", CMC: 0}, Qty: 1},
		deck.Card{Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains", CMC: 0}, Qty: 30},
		deck.Card{Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact", CMC: 1}, Qty: 1},
		deck.Card{Card: mtg.Card{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5, Colors: []string{"W"},
		}, Qty: 1},
	}

	count := func(rows []Row, label string) int {
		for _, r := range rows {
			if r.Label != label {
				continue
			}
			var n int
			for _, ci := range entries {
				if r.Match(ci) {
					n += ci.Qty
				}
			}
			return n
		}
		t.Fatalf("no %q row", label)
		return 0
	}

	if got := count(cmcRows(), "0"); got != 0 {
		t.Errorf("%d cards at mana value 0, want none — lands should be out of it", got)
	}
	if got := count(cmcRows(), "1"); got != 1 {
		t.Errorf("mana value 1 = %d, want the Sol Ring", got)
	}
	if got := count(colorRows(), "Colorless"); got != 1 {
		t.Errorf("Colorless = %d, want just the Sol Ring", got)
	}
	if got := count(colorRows(), "White"); got != 1 {
		t.Errorf("White = %d", got)
	}
}

func TestUntaggedIsTheLeftoverAndOnlyWhenSomethingIsTagged(t *testing.T) {
	// A histogram whose one bar is "untagged" says nothing, so a deck with no
	// tags has no tag group; once anything is tagged, the untagged remainder
	// earns a bar — and it sits below every real tag, however big it is.
	tagsGroup := func(entries []deck.Card) *Group {
		for _, g := range Groups(entries, entries) {
			if g.Title == "Tags" {
				return &g
			}
		}
		return nil
	}

	// Nothing tagged: no tag group at all, not a lone untagged bar.
	none := []deck.Card{
		{Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}, Qty: 1},
		{Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains"}, Qty: 5},
	}
	if g := tagsGroup(none); g != nil {
		t.Errorf("a deck with no tags still has a tag group: %+v", g.Rows)
	}

	// Some tagged, some not: a bar for the tag and one for the remainder, with
	// untagged last even though it holds more cards.
	some := []deck.Card{
		{Card: mtg.Card{Name: "Sol Ring", TypeLine: "Artifact"}, Qty: 1, Tags: []string{"ramp"}},
		{Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains"}, Qty: 20},
	}
	g := tagsGroup(some)
	if g == nil {
		t.Fatal("no tag group though a card is tagged")
	}
	if len(g.Rows) != 2 {
		t.Fatalf("want a tag row and an untagged row, got %+v", g.Rows)
	}
	if g.Rows[len(g.Rows)-1].Label != untaggedLabel {
		t.Errorf("untagged is not last: %+v", g.Rows)
	}
}

func TestStatGroupsSayTheyExcludeLands(t *testing.T) {
	// The reader has to be told, or the numbers look wrong.
	entries := []deck.Card{
		{Card: mtg.Card{Name: "Plains", TypeLine: "Basic Land — Plains"}, Qty: 30},
		{Card: mtg.Card{
			Name: "Serra Angel", TypeLine: "Creature — Angel", CMC: 5, Colors: []string{"W"},
		}, Qty: 1},
	}

	var titles []string
	for _, g := range Groups(entries, entries) {
		titles = append(titles, g.Title)
	}
	joined := strings.Join(titles, " | ")
	for _, want := range []string{"Mana Value (excl. lands)", "Color (excl. lands)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no group titled %q; titles are %s", want, joined)
		}
	}
}

func TestNumericAndTypeBarsTakeTheCardListsColours(t *testing.T) {
	// A bar should be the colour of the cards it counts when the list is
	// sorted by that category: the curve and the prices on the ramp, each
	// type in its own colour — not one colour for the whole group.
	for i, r := range cmcRows() {
		if r.Color != RampColour(i) {
			t.Errorf("mana value %s: bar %q, want ramp step %d %q", r.Label, r.Color, i, RampColour(i))
		}
	}
	for i, r := range priceRows() {
		if r.Color != PriceColour(i) {
			t.Errorf("price %s: bar %q, want %q", r.Label, r.Color, PriceColour(i))
		}
	}
	for _, g := range [][]Row{cmcRows(), priceRows(), typeRows()} {
		seen := map[string]string{}
		for _, r := range g {
			if prev, ok := seen[string(r.Color)]; ok {
				t.Errorf("%s: %s and %s share colour %q", r.Group, prev, r.Label, r.Color)
			}
			seen[string(r.Color)] = r.Label
		}
	}
}

func TestPriceBandEdges(t *testing.T) {
	// Each band is [lo, hi): a price on a boundary belongs to the band above,
	// and the last band has no ceiling.
	for v, want := range map[float64]string{
		0: "<$0.5", 0.49: "<$0.5", 0.5: "$0.5–1", 0.99: "$0.5–1", 1: "$1–5",
		99.99: "$50–100", 100: ">$100", 25000: ">$100",
	} {
		if got := priceBands[PriceBand(v)].label; got != want {
			t.Errorf("$%v: band %q, want %q", v, got, want)
		}
	}
	for i := 1; i < len(priceBands); i++ {
		if priceBands[i].lo != priceBands[i-1].hi {
			t.Errorf("gap between %q and %q", priceBands[i-1].label, priceBands[i].label)
		}
	}
}

func TestNamedGroupsLeadWithTheCommonestAndOrderedOnesKeepTheirOrder(t *testing.T) {
	mk := func(name, tl string, cmc float64, rarity string, qty int, tags ...string) deck.Card {
		return deck.Card{Qty: qty, Tags: tags, Card: mtg.Card{Name: name, TypeLine: tl, CMC: cmc, Rarity: rarity}}
	}
	entries := []deck.Card{
		mk("Sol Ring", "Artifact", 1, "uncommon", 1, "otag-ramp"),
		mk("Elf", "Creature — Elf", 1, "common", 3, "otag-ramp", "b-tribe"),
		mk("Hoof", "Creature — Beast", 8, "mythic", 1, "a-wincon"),
		mk("Forest", "Basic Land — Forest", 0, "common", 12),
	}
	labels := func(groups []Group, title string) []string {
		for _, g := range groups {
			if g.Title == title {
				var out []string
				for _, r := range g.Rows {
					out = append(out, r.Label)
				}
				return out
			}
		}
		return nil
	}
	groups := Groups(entries, entries)
	for title, want := range map[string]string{
		"Type":                     "Land Creature Artifact",
		"Tags":                     "otag-ramp b-tribe a-wincon untagged",
		"Mana Value (excl. lands)": "1 7+",
		"Rarity":                   "common uncommon mythic",
	} {
		if got := strings.Join(labels(groups, title), " "); got != want {
			t.Errorf("%s = %q, want %q", title, got, want)
		}
	}
	byName := GroupsBy(entries, entries, true, nil)
	if got := strings.Join(labels(byName, "Tags"), " "); got != "a-wincon b-tribe otag-ramp untagged" {
		t.Errorf("tags by name = %q", got)
	}
	if got := strings.Join(labels(byName, "Type"), " "); got != "Land Creature Artifact" {
		t.Errorf("by name reordered the types too: %q", got)
	}
}
