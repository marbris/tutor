package deck

import (
	"sort"
	"strings"

	"ttr/internal/mtg"
)

// Whether a deck is legal, and if not, what is wrong with it.
//
// Three different things go under that name and a deck has to pass all
// three: every card has to be allowed in the format, the deck has to be the
// right size and shape, and — in Commander — everything in it has to fit
// inside the commander's colors. Scryfall answers the first on its own;
// the other two are the deck's business and are worked out here.
//
// A deck whose cards aren't all known is reported as unknown rather than
// legal. Saying a deck is fine because we couldn't check it is the one
// answer that would actually mislead.

// Problem is one reason a deck isn't legal, in the words you would use.
type Problem struct {
	// What is wrong, as a sentence.
	Text string
	// Cards is which cards are at fault, when it's about particular ones.
	Cards []string
}

// Legality is the verdict.
type Legality struct {
	Format string
	// Known is false when the deck's cards couldn't all be looked up, in
	// which case Legal means nothing.
	Known    bool
	Legal    bool
	Problems []Problem
}

// Flag is the one character the decks list has room for.
func (l Legality) Flag() string {
	switch {
	case !l.Known:
		return " "
	case l.Legal:
		return "*"
	}
	return "!"
}

// Summary is a phrase for the information panel.
func (l Legality) Summary() string {
	switch {
	case !l.Known:
		return "legality unknown — some cards aren't cached"
	case l.Legal:
		return "legal in " + l.Format
	}
	return "not legal in " + l.Format
}

// formatRules is what a format asks of a deck. Commander is the one this
// program is really for; the rest are here so a deck marked "modern" isn't
// silently judged by Commander's rules.
type formatRules struct {
	size      int  // the exact or minimum number of cards
	exactSize bool // Commander is exactly 100; Modern is at least 60
	copies    int  // how many of one card, basics excepted
	commander bool // whether the format has a command zone
}

var formats = map[string]formatRules{
	"commander":   {size: 100, exactSize: true, copies: 1, commander: true},
	"brawl":       {size: 60, exactSize: true, copies: 1, commander: true},
	"oathbreaker": {size: 60, exactSize: true, copies: 1, commander: true},
	"standard":    {size: 60, copies: 4},
	"pioneer":     {size: 60, copies: 4},
	"modern":      {size: 60, copies: 4},
	"legacy":      {size: 60, copies: 4},
	"vintage":     {size: 60, copies: 4},
	"pauper":      {size: 60, copies: 4},
}

// Check works out whether a deck is legal.
//
// known says whether every card in it was resolved; a deck with unknown
// cards can still be checked for size and shape, but not for anything that
// needs to know what the cards are.
func Check(format string, cards []Card, known bool) Legality {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = DefaultFormat
	}

	rules, ok := formats[format]
	if !ok {
		// A format we have no rules for is not a deck we can call illegal.
		return Legality{Format: format, Known: false}
	}

	out := Legality{Format: format, Known: known}
	out.Problems = append(out.Problems, sizeProblems(rules, cards)...)
	out.Problems = append(out.Problems, copyProblems(rules, cards)...)

	if known {
		out.Problems = append(out.Problems, cardProblems(format, cards)...)
		if rules.commander {
			out.Problems = append(out.Problems, commanderProblems(cards)...)
		}
	}

	out.Legal = known && len(out.Problems) == 0
	return out
}

func total(cards []Card) int {
	n := 0
	for _, c := range cards {
		n += maxCount(c.Qty, 1)
	}
	return n
}

func maxCount(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sizeProblems(r formatRules, cards []Card) []Problem {
	n := total(cards)
	switch {
	case r.exactSize && n != r.size:
		return []Problem{{Text: plural2(n, "card") + ", needs exactly " + itoa(r.size)}}
	case !r.exactSize && n < r.size:
		return []Problem{{Text: plural2(n, "card") + ", needs at least " + itoa(r.size)}}
	}
	return nil
}

// copyProblems finds cards run more times than the format allows. Basic
// lands are exempt, and so are the handful of cards that say in their own
// text that they are — Relentless Rats and its imitators.
func copyProblems(r formatRules, cards []Card) []Problem {
	var over []string
	for _, c := range cards {
		if c.Qty <= r.copies || unlimited(c.Card) {
			continue
		}
		over = append(over, c.Card.Name+" ×"+itoa(c.Qty))
	}
	if len(over) == 0 {
		return nil
	}
	sort.Strings(over)

	what := "only one of each card is allowed"
	if r.copies > 1 {
		what = "at most " + itoa(r.copies) + " of each card are allowed"
	}
	return []Problem{{Text: what, Cards: over}}
}

// unlimited reports whether a card may be run in any number: a basic land,
// or one that says so itself.
func unlimited(c mtg.Card) bool {
	if strings.Contains(c.TypeLine, "Basic") {
		return true
	}
	// "A deck can have any number of cards named Relentless Rats."
	return strings.Contains(c.OracleText, "any number of cards named")
}

// cardProblems finds cards the format doesn't allow at all.
func cardProblems(format string, cards []Card) []Problem {
	var banned, notLegal []string
	for _, c := range cards {
		switch c.Card.Legalities[format] {
		case "banned":
			banned = append(banned, c.Card.Name)
		case "legal", "restricted":
			// Restricted is a Vintage thing and means "at most one", which
			// the copy rules above already cover.
		case "":
			// Nothing known about this card in this format; the deck's
			// Known flag already says the answer is incomplete.
		default:
			notLegal = append(notLegal, c.Card.Name)
		}
	}

	var out []Problem
	if len(banned) > 0 {
		sort.Strings(banned)
		out = append(out, Problem{Text: "banned in " + format, Cards: banned})
	}
	if len(notLegal) > 0 {
		sort.Strings(notLegal)
		out = append(out, Problem{Text: "not legal in " + format, Cards: notLegal})
	}
	return out
}

// commanderProblems checks the command zone and the colours it dictates.
func commanderProblems(cards []Card) []Problem {
	var commanders []Card
	for _, c := range cards {
		if c.Commander {
			commanders = append(commanders, c)
		}
	}

	var out []Problem
	switch {
	case len(commanders) == 0:
		return []Problem{{Text: "no commander"}}
	case len(commanders) > 2:
		return []Problem{{Text: itoa(len(commanders)) + " commanders, at most two"}}
	case len(commanders) == 2:
		// Two are only allowed when both say so — partners, backgrounds,
		// "friends forever" and the like.
		for _, c := range commanders {
			if !canPartner(c.Card) {
				out = append(out, Problem{
					Text:  "two commanders, but this one has no partner ability",
					Cards: []string{c.Card.Name},
				})
			}
		}
	}

	for _, c := range commanders {
		if !canCommand(c.Card) {
			out = append(out, Problem{
				Text:  "can't be a commander",
				Cards: []string{c.Card.Name},
			})
		}
	}

	// Colour identity. Everything in the deck has to fit inside the
	// commander's colors — the rule people actually trip over, because a
	// card's identity includes mana symbols in its rules text.
	allowed := map[string]bool{}
	for _, c := range commanders {
		for _, colour := range c.Card.ColorIdentity {
			allowed[colour] = true
		}
	}

	var outside []string
	for _, c := range cards {
		if c.Commander {
			continue
		}
		for _, colour := range c.Card.ColorIdentity {
			if !allowed[colour] {
				outside = append(outside, c.Card.Name)
				break
			}
		}
	}
	if len(outside) > 0 {
		sort.Strings(outside)
		out = append(out, Problem{
			Text:  "outside the commander's colors",
			Cards: outside,
		})
	}
	return out
}

// canCommand reports whether a card is allowed in the command zone: a
// legendary creature, or anything that says it can be.
func canCommand(c mtg.Card) bool {
	if strings.Contains(c.OracleText, "can be your commander") {
		return true
	}
	front := c.TypeLine
	if i := strings.Index(front, "//"); i >= 0 {
		front = front[:i]
	}
	return strings.Contains(front, "Legendary") && strings.Contains(front, "Creature")
}

// canPartner reports whether a card may share the command zone.
func canPartner(c mtg.Card) bool {
	text := c.OracleText
	switch {
	case strings.Contains(text, "Partner"), strings.Contains(text, "partner"):
		return true
	case strings.Contains(text, "Friends forever"):
		return true
	case strings.Contains(text, "Choose a Background"):
		return true
	case strings.Contains(text, "Doctor's companion"):
		return true
	case strings.Contains(c.TypeLine, "Background"):
		return true
	}
	return false
}

func plural2(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return itoa(n) + " " + word + "s"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
