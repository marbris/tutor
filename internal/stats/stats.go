// Package stats breaks a set of cards down by the properties worth counting:
// what the deck's author called them, what they are, and how they're printed.
//
// Every row carries the test it counted itself with, so filtering the cards
// to a category and the number printed beside that category can never
// disagree.
package stats

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/theme"
)

// The statistics panel is a list you can walk with J/K. Every row carries
// the test it counted itself with, so filtering the cards to a category and
// the number printed beside that category can never disagree.

type Row struct {
	Group string // "Color", "Rarity", …
	Label string // "Blue", "rare", "3", "Creature", "Ramp"
	Count int    // over the cards on screen — what the bar shows
	Base  int    // over the whole result set — whether the row exists at all
	Color lipgloss.Color
	Match func(deck.Card) bool

	// A row in a tree — the Scryfall Tagger group — has a Path naming it
	// from the top, how deep it sits, and whether it has rows under it.
	Path       string
	Depth      int
	Expandable bool
}

type Group struct {
	Title string
	Rows  []Row
}

// same reports whether two rows name the same category. The rows carry
// closures, so they can't be compared directly.
func (r Row) Same(other *Row) bool {
	return other != nil && r.Group == other.Group && r.Label == other.Label && r.Path == other.Path
}

// ── Building the rows ───────────────────────────────────────────

// colorRows is the colour spread of the spells. Lands are left out for the
// same reason they're left out of the curve: nearly all of them are
// colourless by the card's own reckoning, so counting them says how many
// lands the deck runs — under a "Colorless" heading, where it reads as if
// the deck were full of colourless spells.
func colorRows() []Row {
	spec := []struct {
		label string
		code  string
		color lipgloss.Color
	}{
		{"White", "W", theme.ManaW},
		{"Blue", "U", theme.ManaU},
		{"Black", "B", theme.ManaB},
		{"Red", "R", theme.ManaR},
		{"Green", "G", theme.ManaG},
	}

	rows := make([]Row, 0, len(spec)+2)
	for _, s := range spec {
		code := s.code
		rows = append(rows, Row{
			Group: "Color", Label: s.label, Color: s.color,
			Match: func(ci deck.Card) bool {
				if mtg.IsLand(ci.Card) {
					return false
				}
				for _, c := range ci.Card.DisplayColors() {
					if c == code {
						return true
					}
				}
				return false
			},
		})
	}
	rows = append(rows,
		Row{
			Group: "Color", Label: "Colorless", Color: theme.ManaC,
			Match: func(ci deck.Card) bool {
				return !mtg.IsLand(ci.Card) && len(ci.Card.DisplayColors()) == 0
			},
		},
		Row{
			Group: "Color", Label: "Multi", Color: theme.ManaMulti,
			Match: func(ci deck.Card) bool {
				return !mtg.IsLand(ci.Card) && len(ci.Card.DisplayColors()) > 1
			},
		},
	)
	return rows
}

// RarityColour is the colour a rarity is drawn in — its bar here, and a
// card list's names and column when the list is sorted by rarity. Read at
// call time rather than kept in a table, so a theme switch carries through.
func RarityColour(rarity string) lipgloss.Color {
	switch strings.ToLower(rarity) {
	case "common":
		return theme.RarityCommon
	case "uncommon":
		return theme.RarityUncommon
	case "rare":
		return theme.RarityRare
	case "mythic":
		return theme.RarityMythic
	case "special":
		return theme.RaritySpecial
	}
	return theme.TextMuted
}

// Ramp is the scale the numeric categories paint on, low to high — the
// curve's bars here, and a card list sorted by mana value, power, toughness,
// price or rank. Built from the theme's roles at call time, so a theme switch
// repaints it.
func Ramp() []lipgloss.Color {
	return []lipgloss.Color{
		theme.TextDim, theme.Info, theme.Member, theme.Success,
		theme.Highlight, theme.Accent, theme.Error, theme.Special,
	}
}

// XColour is a spell with X in its cost, off the ramp: it isn't the top of
// the curve, it's anywhere on it. The X row and a list sorted by mana value
// paint with it alike.
func XColour() lipgloss.Color { return theme.TextBright }

// RampColour is step n of the ramp, the top step standing for everything
// past it — a nine-drop is as hot as a seven.
func RampColour(n int) lipgloss.Color {
	r := Ramp()
	return r[max(0, min(n, len(r)-1))]
}

// TypeColour gives each card type its own colour, so a list ordered by type
// reads as bands rather than as a column of identical grey, and the type
// breakdown's bars match them.
//
// A mapping onto the existing roles rather than eight new ones: a theme
// that changes its greens changes creatures with them, which is the
// behaviour you would want anyway.
func TypeColour(typeLine string) lipgloss.Color {
	switch mtg.PrimaryType(typeLine) {
	case "Creature":
		return theme.Success
	case "Instant":
		return theme.ManaU
	case "Sorcery":
		return theme.ManaR
	case "Artifact":
		return theme.TextDim
	case "Enchantment":
		return theme.ManaW
	case "Planeswalker":
		return theme.ManaMulti
	case "Battle":
		return theme.Accent
	case "Land":
		return theme.Member
	}
	return theme.TextMuted
}

func rarityRows(entries []deck.Card) []Row {
	// The usual rarities in their usual order, then anything unexpected.
	order := []string{"common", "uncommon", "rare", "mythic", "special", "bonus"}
	known := map[string]bool{}
	for _, r := range order {
		known[r] = true
	}
	var extra []string
	for _, e := range entries {
		r := e.Card.Rarity
		if r == "" {
			r = "unknown"
		}
		if !known[r] {
			known[r] = true
			extra = append(extra, r)
		}
	}
	sort.Strings(extra)

	rows := make([]Row, 0, len(order)+len(extra))
	for _, r := range append(order, extra...) {
		rarity := r
		rows = append(rows, Row{
			Group: "Rarity", Label: rarity, Color: RarityColour(rarity),
			Match: func(ci deck.Card) bool {
				got := ci.Card.Rarity
				if got == "" {
					got = "unknown"
				}
				return got == rarity
			},
		})
	}
	return rows
}

// cmcRows is the mana curve. Lands are left out of it: they nearly all cost
// nothing, so counting them buries the curve under a column at zero that
// says only how many lands the deck runs — which the type breakdown below
// already says, and better.
//
// A spell with X in its cost has a row of its own after 7+, and only that
// one: its mana value counts X as zero, so it would otherwise sit among the
// cheap spells it is nothing like. The mana value sort puts it last too.
func cmcRows() []Row {
	rows := make([]Row, 0, 9)
	for i := 0; i <= 7; i++ {
		n := i
		label := strconv.Itoa(n)
		if n == 7 {
			label = "7+"
		}
		rows = append(rows, Row{
			Group: "Mana Value", Label: label, Color: RampColour(n),
			Match: func(ci deck.Card) bool {
				if mtg.IsLand(ci.Card) || mtg.HasX(ci.Card) {
					return false
				}
				cmc := int(ci.Card.CMC)
				if n == 7 {
					return cmc >= 7
				}
				return cmc == n
			},
		})
	}
	rows = append(rows, Row{
		Group: "Mana Value", Label: "X", Color: XColour(),
		Match: func(ci deck.Card) bool {
			return !mtg.IsLand(ci.Card) && mtg.HasX(ci.Card)
		},
	})
	return rows
}

// isLand reports whether a card's front face is a land, which is what
// decides where it's counted.

func typeRows() []Row {
	types := []string{"Creature", "Instant", "Sorcery", "Artifact", "Enchantment", "Planeswalker", "Land"}
	rows := make([]Row, 0, len(types))
	for _, t := range types {
		cardType := t
		rows = append(rows, Row{
			Group: "Type", Label: cardType, Color: TypeColour(cardType),
			Match: func(ci deck.Card) bool {
				return strings.Contains(ci.Card.TypeLine, cardType)
			},
		})
	}
	return rows
}

// priceRows is the spread of what the cards cost, in the dollar bands a
// collection tends to clump into: a wall of commons under a dollar, a handful
// of staples in the tens, the odd reserved-list card off on its own. A card
// Scryfall has no price for is counted in none of them, the way a land is left
// out of the curve — a bar it can't be placed in is worse than no bar.
// PriceBand is which of the price bands a dollar value falls in, cheapest
// first — the same bands the statistics count, so a card list coloured by
// price and the bars agree about where the lines are.
func PriceBand(v float64) int {
	for i, b := range priceBands {
		if v >= b.lo && (b.hi == 0 || v < b.hi) {
			return i
		}
	}
	return 0
}

// PriceBands is how many bands there are.
func PriceBands() int { return len(priceBands) }

// PriceColour is where a price band sits on the ramp, the bands spread
// across it end to end.
func PriceColour(band int) lipgloss.Color {
	return RampColour(band * (len(Ramp()) - 1) / max(PriceBands()-1, 1))
}

var priceBands = []struct {
	label  string
	lo, hi float64 // [lo, hi); hi of 0 means no upper bound
}{
	{"<$0.5", 0, 0.5},
	{"$0.5–1", 0.5, 1},
	{"$1–5", 1, 5},
	{"$5–10", 5, 10},
	{"$10–20", 10, 20},
	{"$20–50", 20, 50},
	{"$50–100", 50, 100},
	{">$100", 100, 0},
}

func priceRows() []Row {
	rows := make([]Row, 0, len(priceBands))
	for i, band := range priceBands {
		lo, hi := band.lo, band.hi
		rows = append(rows, Row{
			Group: "Price (USD)", Label: band.label, Color: PriceColour(i),
			Match: func(ci deck.Card) bool {
				v, ok := ci.Card.USD()
				if !ok {
					return false
				}
				if hi == 0 {
					return v >= lo
				}
				return v >= lo && v < hi
			},
		})
	}
	return rows
}

// tagRows come from the deck itself — only a Moxfield deck whose author
// tagged their cards has any.
func tagRows(entries []deck.Card) []Row {
	seen := map[string]bool{}
	var labels []string
	for _, e := range entries {
		for _, t := range e.Tags {
			if !seen[t] {
				seen[t] = true
				labels = append(labels, t)
			}
		}
	}
	sort.Strings(labels)

	rows := make([]Row, 0, len(labels)+1)
	for _, l := range labels {
		tag := l
		rows = append(rows, Row{
			Group: "Tags", Label: tag, Color: theme.Highlight,
			Match: func(ci deck.Card) bool {
				for _, t := range ci.Tags {
					if t == tag {
						return true
					}
				}
				return false
			},
		})
	}
	// An untagged line, but only once something is tagged: a histogram whose
	// one bar is "untagged" says nothing, so a deck with no tags has no tag
	// group at all. When some cards are tagged, the untagged remainder is worth
	// seeing — and it sits at the bottom, as the leftover rather than a tag.
	if len(labels) > 0 {
		rows = append(rows, Row{
			Group: "Tags", Label: untaggedLabel, Color: theme.TextDim,
			Match: func(ci deck.Card) bool { return len(ci.Tags) == 0 },
		})
	}
	return rows
}

// untaggedLabel names the remainder row in the tag group. It is matched and
// ordered by this label, so it stays distinct from any real tag.
const untaggedLabel = "untagged"

// copies is how many cards an entry stands for.
//
// A search result carries no quantity, because it isn't in a deck and nobody
// has chosen how many — but it is still one card. Counting it as none is why
// a search had no statistics at all: every row's base came to zero, and a row
// nothing has ever matched is dropped, so every group was dropped and the
// panel said "nothing to count" over a screen full of cards. Both ways into a
// deck floor the quantity at one, so a quantity below one can only mean an
// entry that was never in a deck.
func copies(e deck.Card) int {
	if e.Qty < 1 {
		return 1
	}
	return e.Qty
}

// Groups builds the category list from rowSource and counts it over
// counted, the tags commonest first and Tagger's closed. See GroupsBy.
func Groups(rowSource, counted []deck.Card) []Group {
	return GroupsBy(rowSource, counted, false, nil)
}

// ordered are the groups whose rows have an order of their own — the curve
// from 0 up, the price bands from cheapest, the rarities from common. The
// rest are just names, and the biggest leads.
var ordered = map[string]bool{
	"Mana Value (excl. lands)": true, "Price (USD)": true, "Rarity": true,
	// A tree, ordered level by level as it's built.
	TaggerGroup: true,
}

// GroupsBy builds the category list from rowSource and counts it over
// counted. The two differ once you're filtering by a category: which rows
// exist, their order and their positions all come from the whole result set
// and so hold still, while the numbers beside them describe just the cards
// on screen — a category with nothing left in it stays put and reads zero.
//
// tagsByName puts the tags in alphabetical order rather than commonest
// first: tags tend to come in families — otag-removal, otag-ramp — which
// read better side by side.
//
// openTags is the Scryfall Tagger rows opened to show their children, by
// path.
func GroupsBy(rowSource, counted []deck.Card, tagsByName bool, openTags map[string]bool) []Group {
	// Most telling first: what the deck's author called their cards, then
	// what those cards are, and the printing details last.
	groups := []Group{
		{Title: "Tags", Rows: tagRows(rowSource)},
		{Title: TaggerGroup, Rows: taggerRows(rowSource, openTags)},
		{Title: "Type", Rows: typeRows()},
		{Title: "Color (excl. lands)", Rows: colorRows()},
		{Title: "Mana Value (excl. lands)", Rows: cmcRows()},
		{Title: "Rarity", Rows: rarityRows(rowSource)},
		{Title: "Price (USD)", Rows: priceRows()},
	}

	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		kept := make([]Row, 0, len(g.Rows))
		for _, r := range g.Rows {
			for _, e := range rowSource {
				if r.Match(e) {
					r.Base += copies(e)
				}
			}
			// A category nothing in the deck has ever matched is left out
			// entirely; one the current filter has emptied is not.
			if r.Base == 0 {
				continue
			}
			for _, e := range counted {
				if r.Match(e) {
					r.Count += copies(e)
				}
			}
			kept = append(kept, r)
		}
		if len(kept) == 0 {
			continue
		}
		// A group without an order of its own puts its commonest first — by
		// standing in the whole set, so walking the list can't reorder it.
		if !ordered[g.Title] {
			byName := tagsByName && g.Title == "Tags"
			sort.SliceStable(kept, func(i, j int) bool {
				// Untagged is the leftover, not a tag, so it sinks below every
				// real tag however many cards it holds.
				if iu, ju := kept[i].Label == untaggedLabel, kept[j].Label == untaggedLabel; iu != ju {
					return ju
				}
				if byName {
					return strings.ToLower(kept[i].Label) < strings.ToLower(kept[j].Label)
				}
				return kept[i].Base > kept[j].Base
			})
		}
		out = append(out, Group{Title: g.Title, Rows: kept})
	}
	return out
}
