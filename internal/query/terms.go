package query

import (
	"strconv"
	"strings"

	"ttr/internal/cardstore"
	"ttr/internal/tagger"
)

// termFunc is a term that matches: a card in, yes or no out.
type termFunc func(*cardstore.Facts) bool

func (f termFunc) match(c *cardstore.Facts) bool { return f(c) }

// term turns one word into what it matches. A keyword it knows, with a value
// it can read, is that keyword; anything else is plain text, and a word that
// looked like a keyword is noted as unknown.
func (p *parser) term(w string) node {
	if name, ok := strings.CutPrefix(w, "!"); ok && name != "" {
		return termFunc(func(c *cardstore.Facts) bool { return strings.EqualFold(c.Name, name) })
	}
	key, op, val, ok := splitTerm(w)
	if ok {
		if n := keyword(key, op, val); n != nil {
			return n
		}
		if val != "" {
			p.unknown = append(p.unknown, key+op+val)
		}
	}
	if p.namesOnly {
		return contains(func(c *cardstore.Facts) string { return c.Name }, w)
	}
	return plain(w)
}

// plain is a word matched as / always has: anywhere in the name, the text,
// the type line or the tags.
func plain(w string) node {
	return termFunc(func(c *cardstore.Facts) bool {
		hay := strings.ToLower(c.Name + "\n" + c.Text + "\n" + c.TypeLine + "\n" + strings.Join(c.Tags, " "))
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
			return contains(func(c *cardstore.Facts) string { return c.TypeLine }, val)
		}
	case "o", "oracle":
		if text {
			return contains(func(c *cardstore.Facts) string { return c.Text }, val)
		}
	case "name":
		if text {
			return contains(func(c *cardstore.Facts) string { return c.Name }, val)
		}
	case "mv", "cmc", "manavalue":
		return number(op, val, func(c *cardstore.Facts) (float64, bool) { return c.CMC, true })
	case "pow", "power":
		return number(op, val, func(c *cardstore.Facts) (float64, bool) { return stat(c.Power) })
	case "tou", "toughness":
		return number(op, val, func(c *cardstore.Facts) (float64, bool) { return stat(c.Toughness) })
	case "loy", "loyalty":
		return number(op, val, func(c *cardstore.Facts) (float64, bool) { return stat(c.Loyalty) })
	case "usd":
		return number(op, val, func(c *cardstore.Facts) (float64, bool) { return c.USD, c.HasUSD })
	case "c", "color":
		return colors(op, opGe, val, func(c *cardstore.Facts) []string { return c.Colors.List() })
	case "id", "identity", "ci":
		return colors(op, opLe, val, func(c *cardstore.Facts) []string { return c.Identity.List() })
	case "r", "rarity":
		return rarity(op, val)
	case "f", "format", "legal":
		if text {
			bit, _ := cardstore.FormatBit(val)
			return termFunc(func(c *cardstore.Facts) bool { return c.Legal&bit != 0 })
		}
	case "banned":
		if text {
			bit, _ := cardstore.FormatBit(val)
			return termFunc(func(c *cardstore.Facts) bool { return c.Banned&bit != 0 })
		}
	case "s", "set", "e", "edition":
		if text {
			return termFunc(func(c *cardstore.Facts) bool {
				for _, s := range c.Sets {
					if strings.EqualFold(s, val) {
						return true
					}
				}
				return false
			})
		}
	case "kw", "keyword":
		if text {
			return termFunc(func(c *cardstore.Facts) bool {
				for _, k := range c.Keywords {
					if strings.EqualFold(k, val) {
						return true
					}
				}
				return false
			})
		}
	case "tag":
		if text {
			return termFunc(func(c *cardstore.Facts) bool {
				for _, t := range c.Tags {
					if strings.EqualFold(t, val) {
						return true
					}
				}
				return false
			})
		}
	case "is":
		if op == opColon && val == "commander" {
			return termFunc(commander)
		}
	case "otag", "oracletag", "function":
		if text {
			return termFunc(func(c *cardstore.Facts) bool {
				tg := tagger.Current()
				t, ok := tg.Find(val)
				return ok && tg.Has(c.OracleID, t)
			})
		}
	}
	return nil
}

func contains(field func(*cardstore.Facts) string, val string) node {
	return termFunc(func(c *cardstore.Facts) bool { return strings.Contains(strings.ToLower(field(c)), val) })
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

func number(op, val string, get func(*cardstore.Facts) (float64, bool)) node {
	want, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return nil
	}
	return termFunc(func(c *cardstore.Facts) bool {
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

// rarities in order, for r>=rare: cardstore.RarityOrder, and the letters.
var rarities = map[string]int{"common": 0, "c": 0, "uncommon": 1, "u": 1, "rare": 2, "r": 2,
	"mythic": 3, "m": 3, "special": 4, "s": 4, "bonus": 5, "b": 5}

// rarity matches a card printed at a rarity that compares: in the store,
// any of its printings, as on Scryfall.
func rarity(op, val string) node {
	want, ok := rarities[val]
	if !ok {
		return nil
	}
	return termFunc(func(c *cardstore.Facts) bool {
		for got := range cardstore.RarityOrder {
			if c.Rarities.Has(got) && compare(op, float64(got), float64(want)) {
				return true
			}
		}
		return false
	})
}

// commander is is:commander, as Scryfall has it: a card whose front is a
// legendary creature, a Background, or a legendary Vehicle or Spacecraft
// with power — or one that says it can be your commander.
func commander(c *cardstore.Facts) bool {
	front, _, _ := strings.Cut(c.TypeLine, " // ")
	if strings.Contains(front, "Legendary") {
		switch {
		case strings.Contains(front, "Creature"), strings.Contains(front, "Background"):
			return true
		case (strings.Contains(front, "Vehicle") || strings.Contains(front, "Spacecraft")) && c.Power != "":
			return true
		}
	}
	return strings.Contains(c.Text, "can be your commander")
}
