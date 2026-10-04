package stats

import (
	"sort"
	"strings"

	"ttr/internal/catalog"
	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// The Type group, as a tree: each card type, and under it (enter) the
// subtypes the cards have — the creature types under Creature, Equipment
// under Artifact — each with its own bar, to filter by like any other.
//
// A subtype sits under the type Scryfall's catalogs say it belongs to, so
// on "Artifact Creature — Equipment Golem" Equipment counts under Artifact
// and Golem under Creature. Before the catalogs are in, or for a subtype
// they don't know, it sits under the card's primary type.

// TypeGroup is the group's title.
const TypeGroup = "Type"

// cardTypes are the types the group counts, in no order: the rows are
// ordered by count. One with no cards isn't shown.
var cardTypes = []string{"Creature", "Instant", "Sorcery", "Artifact", "Enchantment", "Planeswalker", "Battle", "Land"}

// typePathPrefix keeps the group's paths apart from Scryfall Tagger's in the
// one set of opened rows.
const typePathPrefix = "type:"

// typeRows is the tree of types and, under the opened ones, subtypes that
// the cards in rowSource have, each level commonest first.
func typeRows(rowSource []deck.Card, open map[string]bool) []Row {
	base := map[string]int{}
	subBase := map[string]map[string]int{}
	for _, e := range rowSource {
		n := copies(e)
		for _, t := range cardTypes {
			if hasType(e, t) {
				base[t] += n
			}
		}
		for t, subs := range subtypesByType(e) {
			if subBase[t] == nil {
				subBase[t] = map[string]int{}
			}
			for _, s := range subs {
				subBase[t][s] += n
			}
		}
	}

	types := append([]string(nil), cardTypes...)
	sort.SliceStable(types, func(i, j int) bool { return base[types[i]] > base[types[j]] })

	var rows []Row
	for _, t := range types {
		if base[t] == 0 {
			continue
		}
		r := TypeRow(typePathPrefix + t)
		r.Expandable = len(subBase[t]) > 0
		rows = append(rows, r)
		if !open[r.Path] {
			continue
		}
		subs := make([]string, 0, len(subBase[t]))
		for s := range subBase[t] {
			subs = append(subs, s)
		}
		sort.Slice(subs, func(i, j int) bool {
			if subBase[t][subs[i]] != subBase[t][subs[j]] {
				return subBase[t][subs[i]] > subBase[t][subs[j]]
			}
			return subs[i] < subs[j]
		})
		for _, s := range subs {
			c := TypeRow(r.Path + pathSep + s)
			c.Depth = 1
			rows = append(rows, c)
		}
	}
	return rows
}

// TypeRow is the row for a type ("type:Creature") or a subtype under it
// ("type:Creature/Elf"), whether or not it's on show: a statistics filter
// restored from the last session names its rows this way.
func TypeRow(path string) Row {
	rest := strings.TrimPrefix(path, typePathPrefix)
	t, sub, isSub := strings.Cut(rest, pathSep)
	if !isSub {
		return Row{
			Group: TypeGroup, Label: t, Path: path, Color: TypeColour(t),
			Match: func(ci deck.Card) bool { return hasType(ci, t) },
		}
	}
	return Row{
		Group: TypeGroup, Label: sub, Path: path, Color: TypeColour(t),
		Match: func(ci deck.Card) bool {
			for _, s := range subtypesByType(ci)[t] {
				if s == sub {
					return true
				}
			}
			return false
		},
	}
}

// IsTypePath reports whether a saved row's path is one of the Type group's.
func IsTypePath(path string) bool { return strings.HasPrefix(path, typePathPrefix) }

// hasType is whether a card is of a type, on any of its faces.
func hasType(e deck.Card, t string) bool {
	return strings.Contains(e.Card.TypeLine, t)
}

// subtypesByType is a card's subtypes, each under the card type it counts
// under. Each face of a double-faced card has its own type line.
func subtypesByType(e deck.Card) map[string][]string {
	var out map[string][]string
	cat := catalog.Current()
	for _, face := range strings.Split(e.Card.TypeLine, " // ") {
		types, subs, ok := strings.Cut(face, "—")
		if !ok {
			continue
		}
		var mine []string
		for _, t := range cardTypes {
			if strings.Contains(types, t) {
				mine = append(mine, t)
			}
		}
		if len(mine) == 0 {
			continue
		}
		for _, s := range strings.Fields(subs) {
			under := ""
			if belongs, known := cat.SubtypeOf(s); known {
				for _, t := range belongs {
					if contains(mine, t) {
						under = t
						break
					}
				}
			}
			if under == "" {
				under = primaryOf(mine)
			}
			if out == nil {
				out = map[string][]string{}
			}
			if !contains(out[under], s) {
				out[under] = append(out[under], s)
			}
		}
	}
	return out
}

// primaryOf is the type a card with several is filed under, by the same
// precedence the deck list uses.
func primaryOf(types []string) string {
	for _, t := range mtg.TypePrecedence {
		if contains(types, t) {
			return t
		}
	}
	return types[0]
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
