package query

import (
	"strconv"
	"strings"

	"ttr/internal/deck"
	"ttr/internal/tagger"
)

// termFunc is a term that matches: a card in, yes or no out.
type termFunc func(deck.Card) bool

func (f termFunc) match(c deck.Card) bool { return f(c) }

// term turns one word into what it matches. A keyword it knows, with a value
// it can read, is that keyword; anything else is plain text.
func term(w string) node {
	key, op, val, ok := splitTerm(w)
	if ok {
		if n := keyword(key, op, val); n != nil {
			return n
		}
	}
	return plain(w)
}

// plain is a word matched as / always has: anywhere in the name, the text,
// the type line or the tags.
func plain(w string) node {
	return termFunc(func(c deck.Card) bool {
		hay := strings.ToLower(c.Card.Name + "\n" + c.Card.CombinedOracle() + "\n" +
			c.Card.TypeLine + "\n" + strings.Join(c.AllTags(), " "))
		return strings.Contains(hay, w)
	})
}

// The comparisons a term can make. ":" is the keyword's own sense: equal
// for numbers, at least for colors, at most for identity.
const (
	opColon = ":"
	opEq    = "="
	opNe    = "!="
	opLt    = "<"
	opLe    = "<="
	opGt    = ">"
	opGe    = ">="
)

// splitTerm cuts a word at its comparison: mv<=3 is mv, <=, 3. The key is
// letters only, so a plain word with a colon in it (a card named
// "Circle of Protection: Red") isn't taken for a keyword.
func splitTerm(w string) (key, op, val string, ok bool) {
	i := 0
	for i < len(w) && w[i] >= 'a' && w[i] <= 'z' {
		i++
	}
	if i == 0 || i == len(w) {
		return "", "", "", false
	}
	for _, o := range []string{opLe, opGe, opNe, opColon, opEq, opLt, opGt} {
		if strings.HasPrefix(w[i:], o) {
			return w[:i], o, w[i+len(o):], true
		}
	}
	return "", "", "", false
}

// keyword is the term for a known key, or nil when the key isn't one, or
// the value can't be read for it.
func keyword(key, op, val string) node {
	if val == "" {
		return nil
	}
	text := op == opColon || op == opEq
	switch key {
	case "t", "type":
		if text {
			return contains(func(c deck.Card) string { return c.Card.TypeLine }, val)
		}
	case "o", "oracle":
		if text {
			return contains(func(c deck.Card) string { return c.Card.CombinedOracle() }, val)
		}
	case "name":
		if text {
			return contains(func(c deck.Card) string { return c.Card.Name }, val)
		}
	case "mv", "cmc", "manavalue":
		return number(op, val, func(c deck.Card) (float64, bool) { return c.Card.CMC, true })
	case "pow", "power":
		return number(op, val, func(c deck.Card) (float64, bool) { return stat(c.Card.Power) })
	case "tou", "toughness":
		return number(op, val, func(c deck.Card) (float64, bool) { return stat(c.Card.Toughness) })
	case "loy", "loyalty":
		return number(op, val, func(c deck.Card) (float64, bool) { return stat(c.Card.Loyalty) })
	case "usd":
		return number(op, val, func(c deck.Card) (float64, bool) { return c.Card.USD() })
	case "c", "color":
		return colors(op, opGe, val, func(c deck.Card) []string { return c.Card.DisplayColors() })
	case "id", "identity", "ci":
		return colors(op, opLe, val, func(c deck.Card) []string { return c.Card.ColorIdentity })
	case "r", "rarity":
		return rarity(op, val)
	case "f", "format", "legal":
		if text {
			return termFunc(func(c deck.Card) bool {
				l := c.Card.Legalities[val]
				return l == "legal" || l == "restricted"
			})
		}
	case "banned":
		if text {
			return termFunc(func(c deck.Card) bool { return c.Card.Legalities[val] == "banned" })
		}
	case "s", "set", "e", "edition":
		if text {
			return termFunc(func(c deck.Card) bool { return strings.EqualFold(c.Card.Set, val) })
		}
	case "kw", "keyword":
		if text {
			return termFunc(func(c deck.Card) bool {
				for _, k := range c.Card.Keywords {
					if strings.EqualFold(k, val) {
						return true
					}
				}
				return false
			})
		}
	case "tag":
		if text {
			return termFunc(func(c deck.Card) bool {
				for _, t := range c.AllTags() {
					if strings.EqualFold(t, val) {
						return true
					}
				}
				return false
			})
		}
	case "otag", "oracletag", "function":
		if text {
			return termFunc(func(c deck.Card) bool {
				tg := tagger.Current()
				t, ok := tg.Find(val)
				return ok && tg.Has(c.Card.OracleID, t)
			})
		}
	}
	return nil
}

func contains(field func(deck.Card) string, val string) node {
	return termFunc(func(c deck.Card) bool { return strings.Contains(strings.ToLower(field(c)), val) })
}

// stat is a power, toughness or loyalty as a number. A * counts as zero, as
// it does on Scryfall; anything else unreadable, or none at all, matches no
// comparison.
func stat(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	if n, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(s, "+"), "*"), 64); err == nil {
		return n, true
	}
	if strings.Contains(s, "*") {
		return 0, true
	}
	return 0, false
}

func number(op, val string, get func(deck.Card) (float64, bool)) node {
	want, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return nil
	}
	return termFunc(func(c deck.Card) bool {
		got, ok := get(c)
		return ok && compare(op, got, want)
	})
}

func compare(op string, got, want float64) bool {
	switch op {
	case opColon, opEq:
		return got == want
	case opNe:
		return got != want
	case opLt:
		return got < want
	case opLe:
		return got <= want
	case opGt:
		return got > want
	case opGe:
		return got >= want
	}
	return false
}

// rarities in order, for r>=rare.
var rarities = map[string]int{"common": 0, "c": 0, "uncommon": 1, "u": 1, "rare": 2, "r": 2,
	"mythic": 3, "m": 3, "special": 4, "s": 4, "bonus": 5, "b": 5}

func rarity(op, val string) node {
	want, ok := rarities[val]
	if !ok {
		return nil
	}
	return termFunc(func(c deck.Card) bool {
		got, ok := rarities[strings.ToLower(c.Card.Rarity)]
		return ok && compare(op, float64(got), float64(want))
	})
}
