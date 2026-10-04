package rules

import (
	"testing"

	"ttr/internal/catalog"
	"ttr/internal/mtg"
)

// miniRules is just enough of a rulebook: a few keywords and the ability
// words, with the rules' curly apostrophe.
const miniRules = `Contents
1. Game Concepts
100. General
100.1. These Magic rules apply to any Magic game.

2. Parts of a Card
207. Text Box
207.2c An ability word appears in italics. The ability words are council’s dilemma, landfall, and threshold.

7. Additional Rules
701. Keyword Actions
701.1. Most actions are described in plain English.
701.10. Scry
701.10a To scry N, look at the top N cards.
702. Keyword Abilities
702.1. Most abilities describe what they do.
702.9. Flying
702.9a Flying is an evasion ability.
702.11. Hexproof
702.11a Hexproof is a static ability.
702.14. Landwalk
702.14a Landwalk is a generic term.
702.29. Cycling
702.29a Cycling is an activated ability.

Glossary

Credits
`

func TestScryfallsKeywordsAreHighlightedWithTheRuleTheyAreAFormOf(t *testing.T) {
	d := Parse(miniRules)
	if !d.Loaded() {
		t.Fatal("the mini rulebook didn't parse")
	}
	copyHeld := d // a list or panel holding the rules from before the catalogs
	c := &catalog.Data{Lists: map[string][]string{
		catalog.KeywordAbilities: {"Flying", "Forestwalk", "Plainscycling", "Hexproof from", "Specialize"},
		catalog.KeywordActions:   {"Scry", "Seek"},
		catalog.AbilityWords:     {"Council's dilemma", "Landfall"},
	}}
	d.AddCatalog(c)

	for text, rule := range map[string]string{
		"Forestwalk": "702.14", "Plainscycling": "702.29", "Hexproof from": "702.11",
		"Flying": "702.9", "Specialize": "", "Seek": "",
	} {
		kw, ok := copyHeld.kw.lookup(text)
		if !ok {
			t.Errorf("%s isn't a keyword after the catalogs, in a copy held from before", text)
			continue
		}
		if kw.Rule != rule {
			t.Errorf("%s -> rule %q, want %q", text, kw.Rule, rule)
		}
	}

	// Highlighted in card text, with either apostrophe.
	text := "Forestwalk\nCouncil's dilemma — Each player votes.\nCouncil’s dilemma again. Seek a card."
	got := map[string]bool{}
	for _, sp := range d.KeywordSpans(text) {
		got[sp.Text] = true
	}
	for _, want := range []string{"Forestwalk", "Council's dilemma", "Council’s dilemma", "Seek"} {
		if !got[want] {
			t.Errorf("%q isn't highlighted; found %v", want, got)
		}
	}

	// The rules panel lists the ones with a rule, not Seek.
	var listed []string
	for _, m := range d.MatchCard(mtg.Card{OracleText: "Forestwalk. Seek a nonland card."}) {
		if m.Kind == MatchKeyword {
			listed = append(listed, m.Term)
		}
	}
	if len(listed) != 1 || listed[0] != "Forestwalk" {
		t.Errorf("the rules panel lists %v, want only Forestwalk", listed)
	}
}
