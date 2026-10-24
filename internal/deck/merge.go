package deck

import (
	"strconv"
	"strings"
)

// MergeCards is a three-way merge of a deck, card by card: base is what the
// file held when it was last read, theirs is what it holds now (another ttr,
// or an editor, wrote it since), and ours is the open deck. A card one side
// changed takes that side's version; a card both changed takes ours, since
// ours is the edit being made. Removing a card is a change like any other.
//
// Only the count, the commander flag and the tags are compared. The printing
// rides along with whichever side wins, and a card neither side changed keeps
// ours, so the screen doesn't swap printings under you.
//
// The result is in ours' order, with the cards only theirs added at the end.
func MergeCards(base, theirs, ours []Card) []Card {
	was, now := byKey(base), byKey(theirs)
	out := make([]Card, 0, len(ours)+len(theirs))
	seen := map[string]bool{}
	for _, k := range cardKeys(ours) {
		c := k.card
		seen[k.key] = true
		b, inBase := was[k.key]
		t, inTheirs := now[k.key]
		switch {
		case !inBase || !sameCard(b, c):
			out = append(out, c) // ours changed it: ours wins
		case !inTheirs:
			// theirs removed a card we left alone
		case !sameCard(b, t):
			out = append(out, t)
		default:
			out = append(out, c)
		}
	}
	for _, k := range cardKeys(theirs) {
		if seen[k.key] {
			continue
		}
		if b, inBase := was[k.key]; inBase && sameCard(b, k.card) {
			continue // we removed a card they left alone
		}
		// New in theirs, or we removed one they changed: theirs keeps it.
		out = append(out, k.card)
	}
	return out
}

type keyedCard struct {
	key  string
	card Card
}

// cardKeys names each card by its name, as the deck file does. A name that
// comes twice (two printings on their own lines) gets a count after it, so
// the second copy is matched with the second copy.
func cardKeys(cards []Card) []keyedCard {
	out := make([]keyedCard, len(cards))
	n := map[string]int{}
	for i, c := range cards {
		name := strings.ToLower(c.Card.Name)
		n[name]++
		key := name
		if n[name] > 1 {
			key += "\x00" + strconv.Itoa(n[name])
		}
		out[i] = keyedCard{key, c}
	}
	return out
}

func byKey(cards []Card) map[string]Card {
	out := map[string]Card{}
	for _, k := range cardKeys(cards) {
		out[k.key] = k.card
	}
	return out
}

func sameCard(a, b Card) bool {
	return a.Qty == b.Qty && a.Commander == b.Commander && sameTags(a.Tags, b.Tags)
}
