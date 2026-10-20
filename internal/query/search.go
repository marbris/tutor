package query

import (
	"sort"
	"strings"

	"ttr/internal/cardstore"
)

// Search is every card in the store a query matches, in the order a search
// asks for — the orders Scryfall takes (scryfall.SortOptions), with "auto"
// running each the way Scryfall would: newest, dearest and biggest first,
// the rest from the bottom up. Orders the store can't follow (artist,
// review) go by name.
func Search(s *cardstore.Store, q Query, order, dir string) []int {
	var out []int
	for i := range s.Len() {
		if q.MatchFacts(s.Facts(i)) {
			out = append(out, i)
		}
	}
	less := orderBy(order)
	desc := dir == "desc" || (dir != "asc" && descByDefault[order])
	sort.SliceStable(out, func(a, b int) bool {
		fa, fb := s.Facts(out[a]), s.Facts(out[b])
		if c := less(fa, fb); c != 0 {
			return (c < 0) != desc
		}
		return fa.Name < fb.Name
	})
	return out
}

// descByDefault are the orders "auto" runs from the top.
var descByDefault = map[string]bool{"released": true, "usd": true, "power": true, "toughness": true}

// orderBy compares two cards by an order: below zero when a comes first
// going up.
func orderBy(order string) func(a, b *cardstore.Facts) int {
	switch order {
	case "released":
		return func(a, b *cardstore.Facts) int { return strings.Compare(a.Released, b.Released) }
	case "set":
		return func(a, b *cardstore.Facts) int { return strings.Compare(first(a.Sets), first(b.Sets)) }
	case "rarity":
		return func(a, b *cardstore.Facts) int {
			return rarities[strings.ToLower(a.Rarity)] - rarities[strings.ToLower(b.Rarity)]
		}
	case "color":
		return func(a, b *cardstore.Facts) int { return colorRank(a.Colors) - colorRank(b.Colors) }
	case "usd":
		return func(a, b *cardstore.Facts) int {
			// A card with no price sorts as the cheapest.
			return cmpFloat(price(a), price(b))
		}
	case "cmc":
		return func(a, b *cardstore.Facts) int { return cmpFloat(a.CMC, b.CMC) }
	case "power":
		return func(a, b *cardstore.Facts) int { return cmpStat(a.Power, b.Power) }
	case "toughness":
		return func(a, b *cardstore.Facts) int { return cmpStat(a.Toughness, b.Toughness) }
	case "edhrec":
		return func(a, b *cardstore.Facts) int { return rank(a.EDHREC) - rank(b.EDHREC) }
	}
	return func(a, b *cardstore.Facts) int { return strings.Compare(a.Name, b.Name) }
}

func first(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return l[0]
}

func price(f *cardstore.Facts) float64 {
	if !f.HasUSD {
		return -1
	}
	return f.USD
}

// rank is an EDHREC rank, a card with none coming last.
func rank(r int) int {
	if r <= 0 {
		return 1 << 30
	}
	return r
}

// colorRank orders colors as Scryfall does: white, blue, black, red, green,
// then the multicolored, then colorless.
func colorRank(c cardstore.Colors) int {
	l := c.List()
	switch len(l) {
	case 0:
		return 100
	case 1:
		return strings.Index("WUBRG", l[0])
	}
	return 10 + len(l)
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// cmpStat compares powers or toughnesses, a card with none coming first
// going up.
func cmpStat(a, b string) int {
	na, oka := stat(a)
	nb, okb := stat(b)
	if !oka {
		na = -1
	}
	if !okb {
		nb = -1
	}
	return cmpFloat(na, nb)
}
