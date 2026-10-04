package deck

import (
	"strings"
	"testing"

	"ttr/internal/mtg"
)

// commanderDeck builds a legal 100-card Commander deck to bend out of shape.
func commanderDeck() []Card {
	cards := []Card{{
		Qty: 1, Commander: true, Card: mtg.Card{
			Name: "Ghen, Arcanum Weaver", TypeLine: "Legendary Creature — Human Wizard",
			ColorIdentity: []string{"B", "R", "W"},
			Legalities:    map[string]string{"commander": "legal"},
		},
	}}
	cards = append(cards, Card{
		Qty: 99, Card: mtg.Card{
			Name: "Plains", TypeLine: "Basic Land — Plains",
			ColorIdentity: []string{"W"},
			Legalities:    map[string]string{"commander": "legal"},
		},
	})
	return cards
}

func problemText(l Legality) string {
	var out []string
	for _, p := range l.Problems {
		out = append(out, p.Text+" ["+strings.Join(p.Cards, ", ")+"]")
	}
	return strings.Join(out, "; ")
}

func TestALegalCommanderDeckIsLegal(t *testing.T) {
	l := Check("commander", commanderDeck(), true)
	if !l.Legal {
		t.Errorf("a legal deck was rejected: %s", problemText(l))
	}
	if l.Flag() != "*" {
		t.Errorf("flag is %q", l.Flag())
	}
}

func TestCommanderWantsExactlyAHundred(t *testing.T) {
	cards := commanderDeck()
	cards[1].Qty = 98 // one short

	l := Check("commander", cards, true)
	if l.Legal {
		t.Error("a 99-card Commander deck was accepted")
	}
	if !strings.Contains(problemText(l), "exactly 100") {
		t.Errorf("problems are %q", problemText(l))
	}
}

func TestSixtyCardFormatsWantAtLeastSixty(t *testing.T) {
	// Modern asks for a minimum, not an exact count — 63 cards is legal.
	cards := []Card{{Qty: 63, Card: mtg.Card{
		Name: "Mountain", TypeLine: "Basic Land — Mountain",
		Legalities: map[string]string{"modern": "legal"},
	}}}
	if l := Check("modern", cards, true); !l.Legal {
		t.Errorf("63 cards was rejected: %s", problemText(l))
	}

	cards[0].Qty = 59
	if l := Check("modern", cards, true); l.Legal {
		t.Error("59 cards was accepted")
	}
}

func TestCommanderIsSingleton(t *testing.T) {
	cards := commanderDeck()
	cards[1].Qty = 97
	cards = append(cards, Card{Qty: 2, Card: mtg.Card{
		Name: "Sol Ring", TypeLine: "Artifact",
		Legalities: map[string]string{"commander": "legal"},
	}})

	l := Check("commander", cards, true)
	if l.Legal {
		t.Error("two Sol Rings were accepted in Commander")
	}
	if !strings.Contains(problemText(l), "Sol Ring") {
		t.Errorf("problems are %q", problemText(l))
	}
}

func TestBasicLandsAreExemptFromTheSingletonRule(t *testing.T) {
	// 99 Plains is the whole point of the exemption.
	if l := Check("commander", commanderDeck(), true); !l.Legal {
		t.Errorf("99 basics were rejected: %s", problemText(l))
	}
}

func TestCardsThatSayTheyAreExemptAreExempt(t *testing.T) {
	// Relentless Rats and its imitators say so in their own text.
	cards := commanderDeck()
	cards[1].Qty = 60
	cards = append(cards, Card{Qty: 39, Card: mtg.Card{
		Name: "Relentless Rats", TypeLine: "Creature — Rat",
		OracleText:    "A deck can have any number of cards named Relentless Rats.",
		ColorIdentity: []string{"B"},
		Legalities:    map[string]string{"commander": "legal"},
	}})

	if l := Check("commander", cards, true); !l.Legal {
		t.Errorf("Relentless Rats was held to the singleton rule: %s", problemText(l))
	}
}

func TestADeckWithNoCommanderIsNotACommanderDeck(t *testing.T) {
	cards := commanderDeck()
	cards[0].Commander = false

	l := Check("commander", cards, true)
	if l.Legal {
		t.Error("a Commander deck with an empty command zone was accepted")
	}
	if !strings.Contains(problemText(l), "no commander") {
		t.Errorf("problems are %q", problemText(l))
	}
}

func TestOnlySomeCardsCanCommand(t *testing.T) {
	cards := commanderDeck()
	cards[0].Card = mtg.Card{
		Name: "Sol Ring", TypeLine: "Artifact",
		Legalities: map[string]string{"commander": "legal"},
	}

	l := Check("commander", cards, true)
	if !strings.Contains(problemText(l), "can't be a commander") {
		t.Errorf("problems are %q", problemText(l))
	}
}

func TestAPlaneswalkerThatSaysItCanIsAllowed(t *testing.T) {
	cards := commanderDeck()
	cards[0].Card = mtg.Card{
		Name: "Rowan, Scholar of Sparks", TypeLine: "Legendary Planeswalker — Rowan",
		OracleText:    "Rowan can be your commander.",
		ColorIdentity: []string{"R", "U", "W", "B"},
		Legalities:    map[string]string{"commander": "legal"},
	}
	if l := Check("commander", cards, true); !l.Legal {
		t.Errorf("a planeswalker commander was rejected: %s", problemText(l))
	}
}

func TestColourIdentityIsEnforced(t *testing.T) {
	// The rule people actually trip over: a card's identity includes mana
	// symbols in its rules text, not just its cost.
	cards := commanderDeck()
	cards[1].Qty = 98
	cards = append(cards, Card{Qty: 1, Card: mtg.Card{
		Name: "Llanowar Elves", TypeLine: "Creature — Elf Druid",
		ColorIdentity: []string{"G"},
		Legalities:    map[string]string{"commander": "legal"},
	}})

	l := Check("commander", cards, true)
	if l.Legal {
		t.Error("a green card was accepted under a Mardu commander")
	}
	if !strings.Contains(problemText(l), "Llanowar Elves") {
		t.Errorf("problems are %q", problemText(l))
	}
	if !strings.Contains(problemText(l), "colors") {
		t.Errorf("the reason isn't stated: %q", problemText(l))
	}
}

func TestTheCommandersOwnColoursAreAllowed(t *testing.T) {
	cards := commanderDeck()
	cards[1].Qty = 98
	cards = append(cards, Card{Qty: 1, Card: mtg.Card{
		Name: "Lightning Bolt", TypeLine: "Instant",
		ColorIdentity: []string{"R"}, // Ghen is Mardu, so red is fine
		Legalities:    map[string]string{"commander": "legal"},
	}})
	if l := Check("commander", cards, true); !l.Legal {
		t.Errorf("a red card was rejected under a Mardu commander: %s", problemText(l))
	}
}

func TestTwoCommandersNeedAPartnerAbility(t *testing.T) {
	cards := commanderDeck()
	cards[1].Qty = 98
	cards = append(cards, Card{Qty: 1, Commander: true, Card: mtg.Card{
		Name: "Random Legend", TypeLine: "Legendary Creature — Human",
		ColorIdentity: []string{"W"},
		Legalities:    map[string]string{"commander": "legal"},
	}})

	l := Check("commander", cards, true)
	if l.Legal {
		t.Error("two unrelated commanders were accepted")
	}
	if !strings.Contains(problemText(l), "partner") {
		t.Errorf("problems are %q", problemText(l))
	}
}

func TestPartnersAreAllowedTogether(t *testing.T) {
	partner := func(name string, id []string) Card {
		return Card{Qty: 1, Commander: true, Card: mtg.Card{
			Name: name, TypeLine: "Legendary Creature — Human Soldier",
			OracleText:    "Partner (You can have two commanders if both have partner.)",
			ColorIdentity: id, Legalities: map[string]string{"commander": "legal"},
		}}
	}
	cards := []Card{
		partner("Ishai, Ojutai Dragonspeaker", []string{"U", "W"}),
		partner("Bruse Tarl, Boorish Herder", []string{"R", "W"}),
		{Qty: 98, Card: mtg.Card{
			Name: "Plains", TypeLine: "Basic Land — Plains",
			ColorIdentity: []string{"W"},
			Legalities:    map[string]string{"commander": "legal"},
		}},
	}
	if l := Check("commander", cards, true); !l.Legal {
		t.Errorf("partners were rejected: %s", problemText(l))
	}
}

func TestBannedCardsAreCalledOut(t *testing.T) {
	cards := commanderDeck()
	cards[1].Qty = 98
	cards = append(cards, Card{Qty: 1, Card: mtg.Card{
		Name: "Black Lotus", TypeLine: "Artifact",
		Legalities: map[string]string{"commander": "banned"},
	}})

	l := Check("commander", cards, true)
	if l.Legal {
		t.Error("a banned card was accepted")
	}
	if !strings.Contains(problemText(l), "banned") {
		t.Errorf("problems are %q", problemText(l))
	}
}

func TestAnUnknownDeckIsNotReportedAsLegal(t *testing.T) {
	// Saying a deck is fine because we couldn't check it is the one answer
	// that would actually mislead.
	l := Check("commander", commanderDeck(), false)
	if l.Legal {
		t.Error("an unchecked deck was called legal")
	}
	if l.Known {
		t.Error("it claims to know")
	}
	if l.Flag() != " " {
		t.Errorf("flag is %q, want nothing", l.Flag())
	}
	if !strings.Contains(l.Summary(), "unknown") {
		t.Errorf("summary is %q", l.Summary())
	}
}

func TestSizeIsStillCheckedWithoutCardData(t *testing.T) {
	// Counting doesn't need to know what the cards are.
	cards := commanderDeck()
	cards[1].Qty = 50
	l := Check("commander", cards, false)
	if !strings.Contains(problemText(l), "exactly 100") {
		t.Errorf("problems are %q", problemText(l))
	}
}

func TestAFormatWeHaveNoRulesForIsNotJudged(t *testing.T) {
	l := Check("canadian highlander", commanderDeck(), true)
	if l.Legal || l.Known {
		t.Error("a format with no rules was judged anyway")
	}
}

func TestAnEmptyFormatMeansCommander(t *testing.T) {
	if l := Check("", commanderDeck(), true); l.Format != "commander" {
		t.Errorf("format defaulted to %q", l.Format)
	}
}
