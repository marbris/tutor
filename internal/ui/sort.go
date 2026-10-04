package ui

import (
	"sort"
	"strconv"
	"strings"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// How a list is ordered on screen, which is a different thing from the order
// a Scryfall query asks for. That one is part of the search — it decides
// which 175 cards come back — and only applies to search results. This one
// rearranges what is already in front of you, without asking anyone.
//
// The order also decides the list's second column: you sorted by a property,
// so that property is the one worth showing.

type cardSort int

const (
	// sortArrival is the order the cards came in: Scryfall's for results,
	// decklist order for a deck.
	sortArrival cardSort = iota
	sortMana
	sortName
	sortType
	sortColor
	sortRarity
	sortEDHREC
	sortPower
	sortToughness
	sortUSD
	// sortInclusion groups a list by where else its cards are: in the deck
	// being edited first, then in the focused list, then in some other list
	// on screen, then only here.
	sortInclusion
)

// allSorts is every order there is, whether or not the cycle offers it.
var allSorts = []cardSort{
	sortArrival, sortMana, sortName, sortType, sortColor, sortRarity, sortEDHREC,
	sortPower, sortToughness, sortUSD, sortInclusion,
}

// defaultCycle is the order . and , step through as it ships. Name is left
// out: a list in alphabetical order is one you could only want for finding
// a card, and / does that better.
var defaultCycle = []cardSort{
	sortArrival, sortMana, sortColor, sortType, sortPower, sortToughness,
	sortEDHREC, sortUSD, sortRarity, sortInclusion,
}

// sortCycle is the cycle in force, which config.json can reorder and trim.
var sortCycle = defaultCycle

// defaultDescending is the orders that read best-first from the top down:
// the biggest creatures, the dearest cards, the rarest. The rest start
// ascending — a rank is best at 1, and a colour or a type has no better end.
var defaultDescending = map[cardSort]bool{
	sortPower: true, sortToughness: true, sortUSD: true, sortRarity: true,
}

// sortDescending is the directions in force, which config.json can flip.
var sortDescending = defaultDescending

func (s cardSort) String() string {
	switch s {
	case sortMana:
		return "mana value"
	case sortName:
		return "name"
	case sortType:
		return "type"
	case sortColor:
		return "color"
	case sortRarity:
		return "rarity"
	case sortEDHREC:
		return "edhrec"
	case sortPower:
		return "power"
	case sortToughness:
		return "toughness"
	case sortUSD:
		return "usd"
	case sortInclusion:
		return "inclusion"
	}
	return "as found"
}

// parseCardSort is the order a name from String stands for.
func parseCardSort(name string) (cardSort, bool) {
	if name == "" {
		return 0, false
	}
	if name == "colour" { // the name before 5.3, in saved sessions
		return sortColor, true
	}
	for _, s := range allSorts {
		if s.String() == name {
			return s, true
		}
	}
	return 0, false
}

// next cycles through the orders in force, wrapping. An order the cycle
// doesn't offer — one config.json has since left out — steps to its start.
func (s cardSort) next(delta int) cardSort {
	n := len(sortCycle)
	for i, c := range sortCycle {
		if c == s {
			return sortCycle[((i+delta)%n+n)%n]
		}
	}
	return sortCycle[0]
}

// firstSort is where a new list starts: the head of the cycle, which is the
// order the cards came in unless config.json says otherwise.
func firstSort() cardSort { return sortCycle[0] }

// descending is the direction an order starts in.
func (s cardSort) descending() bool { return sortDescending[s] }

// showsMana reports whether the second column is a mana cost, which decides
// whether it gets painted symbol by symbol.
func (s cardSort) showsMana() bool {
	switch s {
	case sortType, sortRarity, sortEDHREC, sortPower, sortToughness, sortUSD:
		return false
	}
	return true
}

// column is what the second column shows under this order. Sorting by name
// or by arrival leaves nothing worth repeating, so both fall back to the
// mana cost — the one property you scan a list for regardless.
func (s cardSort) column(c mtg.Card) string {
	switch s {
	case sortType:
		return c.TypeLine
	case sortRarity:
		return c.Rarity
	case sortEDHREC:
		if c.EDHRECRank == 0 {
			return "—"
		}
		return itoa(c.EDHRECRank)
	case sortPower, sortToughness:
		// Power and toughness travel together: sorting by one still shows
		// both, since a 2/5 and a 5/2 are a different card and the number
		// you didn't sort by is how you tell them apart.
		return powerToughness(c)
	case sortUSD:
		return usdText(c)
	}
	return manaCost(c)
}

// usdText is a card's dollar price for the column: "$3.99", or a dash for a
// card Scryfall has no price for — the same dash the other columns use for a
// value a card simply doesn't have.
func usdText(c mtg.Card) string {
	v, ok := c.USD()
	if !ok {
		return "—"
	}
	return "$" + strconv.FormatFloat(v, 'f', 2, 64)
}

// powerToughness is a creature's "P/T", or a planeswalker's loyalty, or a
// dash for anything that has neither — so a list sorted by power still lines
// up, lands and all.
func powerToughness(c mtg.Card) string {
	switch {
	case c.Power != "" || c.Toughness != "":
		return c.Power + "/" + c.Toughness
	case c.Loyalty != "":
		return c.Loyalty
	}
	return "—"
}

// abbreviates reports whether this column can be shortened when the panel
// runs out of room. A type line can; a mana cost is already as short as it
// goes, and a rank abbreviated is a wrong number.
func (s cardSort) abbreviates() bool { return s == sortType }

// manaSymbols pulls a cost apart into the symbols it is made of, with "//"
// standing between the halves of a split card.
//
// Kept as a list rather than flattened to a string, because the two things
// that want it want different shapes: the ladder needs its width, and the
// row needs to paint each symbol its own colour. Flattening first loses
// where one symbol ends and the next begins, which for a hybrid like {W/U}
// is the whole question.
func manaSymbols(cost string) []string {
	if cost == "" {
		return nil
	}
	var out []string
	for _, face := range strings.Split(cost, "//") {
		if len(out) > 0 {
			out = append(out, "//")
		}
		for _, loc := range symbolRe.FindAllStringIndex(face, -1) {
			out = append(out, face[loc[0]+1:loc[1]-1])
		}
	}
	return out
}

// manaText is the cost as the design asks for it: 3BG, no braces, and
// "2U // 2R" for a split card. Hybrid and phyrexian symbols keep their
// innards, since {W/U} shortened to anything is a different card.
func manaText(symbols []string) string {
	var b strings.Builder
	for _, s := range symbols {
		if s == "//" {
			b.WriteString(" // ")
			continue
		}
		b.WriteString(s)
	}
	return b.String()
}

// manaCost is the whole job for callers that only want the text.
func manaCost(c mtg.Card) string { return manaText(manaSymbols(c.DisplayManaCost())) }

// sortSpec is everything a list's order depends on: the two orders, which
// way each runs, and — for the inclusion order — where else each card is.
type sortSpec struct {
	first, then cardSort
	desc1       bool
	desc2       bool
	members     map[string]membership
}

// sortCards returns the cards in the given order, with a second order
// breaking the first one's ties. Arrival order is the last tiebreak
// throughout — a stable sort over the list as it came in — so cards that
// compare equal stay where you last saw them.
//
// The second order sits between the first order's own key and whatever the
// first order does inside its groups: sorted by type, then by colour, the
// creatures are laid out WUBRG before the colour sort's dearest-first gets a
// say, and only then does the name decide.
//
// Commanders sort with everything else. They come first in decklist order
// because that is how a decklist is built, not because anything pins them
// there: sorting by mana value means by mana value.
func sortCards(cards []deck.Card, spec sortSpec) []deck.Card {
	if len(cards) < 2 {
		return cards
	}
	if spec.first == sortArrival {
		// Arrival has no key to compare, and the second order is only a
		// tiebreak — so what it does there is colour the names, not move
		// them. Turned round, it is the list upside down.
		if !spec.desc1 {
			return cards
		}
		out := make([]deck.Card, len(cards))
		for i, c := range cards {
			out[len(cards)-1-i] = c
		}
		return out
	}
	out := append([]deck.Card(nil), cards...)
	less := lessFor(spec)
	sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

// lessFor composes the comparison: the first order's key, the second
// order's key, the first order's inner tiebreak, and the name.
func lessFor(spec sortSpec) func(a, b deck.Card) bool {
	return func(a, b deck.Card) bool {
		if c := keyCmp(spec.first, spec.desc1, spec.members, a, b); c != 0 {
			return c < 0
		}
		if spec.then != sortArrival && spec.then != spec.first {
			if c := keyCmp(spec.then, spec.desc2, spec.members, a, b); c != 0 {
				return c < 0
			}
		}
		if c := innerCmp(spec.first, a.Card, b.Card); c != 0 {
			return c < 0
		}
		return a.Card.Name < b.Card.Name
	}
}

// keyCmp compares two cards on the one property an order is about, in the
// direction asked for, and reports them equal when that property is — the
// name is the caller's.
//
// A card with no value at all — unranked, unpriced, no power — goes to the
// bottom whichever way the order runs: turning a list round is for reading
// it from the other end, not for bringing the blanks to the top.
func keyCmp(s cardSort, desc bool, members map[string]membership, a, b deck.Card) int {
	va, oka := sortKey(s, members, a)
	vb, okb := sortKey(s, members, b)
	if oka != okb {
		if oka {
			return -1
		}
		return 1
	}
	if s == sortName {
		c := strings.Compare(a.Card.Name, b.Card.Name)
		if desc {
			return -c
		}
		return c
	}
	c := cmpFloat(va, vb)
	if desc {
		return -c
	}
	return c
}

// sortKey is the number an order ranks a card by, low to high, and whether
// the card has one at all. Each order's key runs its natural way up — mana
// value from 0, rarity from common, power from the smallest — and the
// direction is laid over it by keyCmp.
func sortKey(s cardSort, members map[string]membership, dc deck.Card) (float64, bool) {
	c := dc.Card
	switch s {
	case sortMana:
		return manaSortValue(c), true
	case sortType:
		return float64(typeRank(c.TypeLine)), true
	case sortColor:
		return float64(colorRank(c)), true
	case sortRarity:
		r := rarityRank(c.Rarity)
		return float64(r), r > 0
	case sortEDHREC:
		return float64(c.EDHRECRank), c.EDHRECRank > 0
	case sortPower:
		v, ok := statValue(c.Power)
		return float64(v), ok
	case sortToughness:
		v, ok := statValue(c.Toughness)
		return float64(v), ok
	case sortUSD:
		return c.USD()
	case sortInclusion:
		// The order the markers rank in: in the deck being edited, then in
		// the list you're in, then in some other list, then only here.
		return float64(inEditing - members[markKey(dc)]), true
	}
	return 0, true
}

// xHigh lifts a spell with X in its cost above every fixed cost. Its mana
// value counts X as zero, which is what it is in the library and never what
// it is when you cast it: Fireball is not a one-drop.
const xHigh = 100

// manaSortValue is the mana value an order by mana value ranks a card by,
// with X spells above the rest.
func manaSortValue(c mtg.Card) float64 {
	for _, f := range c.Faces() {
		if strings.Contains(strings.ToUpper(f.ManaCost), "{X}") {
			return c.CMC + xHigh
		}
	}
	return c.CMC
}

// innerCmp is what an order does within one of its own groups, before the
// name has the last word.
func innerCmp(s cardSort, a, b mtg.Card) int {
	switch s {
	case sortColor:
		// Dearest first within a colour, so the lands — which cost nothing
		// and are colourless — end up at the very bottom instead of heading
		// the last group.
		return -cmpFloat(a.CMC, b.CMC)
	case sortPower:
		// Toughness breaks a tie in power, and power one in toughness: a 2/5
		// and a 2/2 are not the same card.
		return cmpStat(a.Toughness, b.Toughness)
	case sortToughness:
		return cmpStat(a.Power, b.Power)
	}
	return 0
}

// cmpStat orders a power or toughness biggest first, with the cards that
// have one at all ahead of those that don't.
func cmpStat(a, b string) int {
	va, oka := statValue(a)
	vb, okb := statValue(b)
	if oka != okb {
		if oka {
			return -1
		}
		return 1
	}
	return -cmpInt(va, vb)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
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

// statValue reads a power or toughness for sorting. Empty means the card has
// none — a land, an instant — and sorts apart from the ones that do. A value
// that isn't a plain number, "*" or "1+*", counts as having one but sorts
// below the plain numbers, since there is no honest number to place it at.
func statValue(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	return -1, true
}

// typeRank orders by the precedence a decklist groups by, so sorting by type
// reads the way a decklist does.
func typeRank(typeLine string) int {
	primary := mtg.PrimaryType(typeLine)
	for i, t := range mtg.TypePrecedence {
		if t == primary {
			return i
		}
	}
	return len(mtg.TypePrecedence) // "Other"
}

// colorRank puts the mono-coloured cards in WUBRG order, then everything
// multicoloured, then the colourless — which is how a deck is laid out when
// it's laid out by colour at all.
func colorRank(c mtg.Card) int {
	colors := c.DisplayColors()
	switch len(colors) {
	case 0:
		return 6
	case 1:
		if i := strings.Index("WUBRG", colors[0]); i >= 0 {
			return i
		}
		return 5
	}
	return 5
}

func rarityRank(r string) int {
	switch strings.ToLower(r) {
	case "mythic":
		return 4
	case "rare":
		return 3
	case "uncommon":
		return 2
	case "common":
		return 1
	}
	return 0
}
