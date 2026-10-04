package query

import (
	"strings"

	"ttr/internal/deck"
)

// Colors and color identity, as Scryfall reads them: letters (c:rg), color
// names, guilds, shards and wedges (id:bant), c for colorless and m for
// multicolored. ":" means at least the colors for c:, and at most them for
// id:, which is the question a commander deck asks.

var colorNames = map[string]string{
	"white": "w", "blue": "u", "black": "b", "red": "r", "green": "g",
	"azorius": "wu", "dimir": "ub", "rakdos": "br", "gruul": "rg", "selesnya": "gw",
	"orzhov": "wb", "izzet": "ur", "golgari": "bg", "boros": "rw", "simic": "gu",
	"bant": "gwu", "esper": "wub", "grixis": "ubr", "jund": "brg", "naya": "rgw",
	"abzan": "wbg", "jeskai": "urw", "sultai": "bgu", "mardu": "rwb", "temur": "gur",
}

// colors is the term for c: and id:. colon is what ":" means for it.
func colors(op, colon, val string, get func(deck.Card) []string) node {
	if op == opColon {
		op = colon
	}
	switch val {
	case "c", "colorless":
		return termFunc(func(c deck.Card) bool {
			if op == opNe {
				return len(get(c)) > 0
			}
			return len(get(c)) == 0
		})
	case "m", "multicolor", "multicolored":
		return termFunc(func(c deck.Card) bool {
			if op == opNe {
				return len(get(c)) < 2
			}
			return len(get(c)) >= 2
		})
	}
	letters := val
	if named, ok := colorNames[val]; ok {
		letters = named
	}
	want := map[string]bool{}
	for _, r := range letters {
		l := string(r)
		if !strings.Contains("wubrg", l) {
			return nil // not a color: plain text after all
		}
		want[l] = true
	}
	return termFunc(func(c deck.Card) bool {
		got := map[string]bool{}
		for _, col := range get(c) {
			got[strings.ToLower(col)] = true
		}
		sub, super := within(got, want), within(want, got)
		switch op {
		case opEq:
			return sub && super
		case opNe:
			return !(sub && super)
		case opGe:
			return super
		case opGt:
			return super && !sub
		case opLe:
			return sub
		case opLt:
			return sub && !super
		}
		return false
	})
}

// within reports whether every color in a is also in b.
func within(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
