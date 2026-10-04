package query

import (
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

func cards() map[string]deck.Card {
	mk := func(name, tl, text string, cmc float64, colors, id []string, pow, tou, rarity, usd string, tags ...string) deck.Card {
		return deck.Card{Qty: 1, Tags: tags, Card: mtg.Card{
			Name: name, TypeLine: tl, OracleText: text, CMC: cmc, Colors: colors, ColorIdentity: id,
			Power: pow, Toughness: tou, Rarity: rarity, Prices: mtg.Prices{USD: usd}, Set: "m21",
			Legalities: map[string]string{"commander": "legal", "standard": "not_legal"},
		}}
	}
	out := map[string]deck.Card{
		"elves": mk("Llanowar Elves", "Creature — Elf Druid", "{T}: Add {G}.", 1, []string{"G"}, []string{"G"}, "1", "1", "common", "0.25", "ramp"),
		"bolt":  mk("Lightning Bolt", "Instant", "Lightning Bolt deals 3 damage to any target.", 1, []string{"R"}, []string{"R"}, "", "", "uncommon", "1.10", "removal"),
		"hoof":  mk("Craterhoof Behemoth", "Creature — Beast", "Haste. When this enters, creatures you control get +X/+X.", 8, []string{"G"}, []string{"G"}, "5", "5", "mythic", "40.00", "wincon"),
		"ring":  mk("Sol Ring", "Artifact", "{T}: Add {C}{C}.", 1, nil, nil, "", "", "uncommon", "1.50", "ramp"),
		"kog":   mk("Kogla, the Titan Ape", "Legendary Creature — Ape", "Draw a card.", 6, []string{"R", "G"}, []string{"R", "G"}, "7", "6", "rare", "0.40"),
		"tarmo": mk("Tarmogoyf", "Creature — Lhurgoyf", "", 2, []string{"G"}, []string{"G"}, "*", "1+*", "mythic", "9.00"),
	}
	hoof := out["hoof"]
	hoof.Card.Keywords = []string{"Haste"}
	out["hoof"] = hoof
	return out
}

func TestQueries(t *testing.T) {
	all := cards()
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"", []string{"elves", "bolt", "hoof", "ring", "kog", "tarmo"}},
		{"t:creature", []string{"elves", "hoof", "kog", "tarmo"}},
		{"t:creature mv<=2", []string{"elves", "tarmo"}},
		{"-t:creature", []string{"bolt", "ring"}},
		{"t:instant or t:artifact", []string{"bolt", "ring"}},
		{"(t:instant or t:artifact) mv=1 -tag:ramp", []string{"bolt"}},
		{`o:"any target"`, []string{"bolt"}},
		{"o:add", []string{"elves", "ring"}},
		{"c:rg", []string{"kog"}},
		{"c:g", []string{"elves", "hoof", "kog", "tarmo"}},
		{"c=g", []string{"elves", "hoof", "tarmo"}},
		{"c:c", []string{"ring"}},
		{"c:m", []string{"kog"}},
		{"id:gruul", []string{"elves", "bolt", "hoof", "ring", "kog", "tarmo"}},
		{"id:g", []string{"elves", "hoof", "ring", "tarmo"}},
		{"pow>=5", []string{"hoof", "kog"}},
		{"pow=0", []string{"tarmo"}},
		{"tou>1", []string{"hoof", "kog"}},
		{"r>=rare", []string{"hoof", "kog", "tarmo"}},
		{"r:mythic", []string{"hoof", "tarmo"}},
		{"usd<1", []string{"elves", "kog"}},
		{"tag:ramp", []string{"elves", "ring"}},
		{"f:commander", []string{"elves", "bolt", "hoof", "ring", "kog", "tarmo"}},
		{"f:standard", nil},
		{"kw:haste", []string{"hoof"}},
		{"set:M21 t:artifact", []string{"ring"}},
		// Plain words match as / always has: name, text, type and tags.
		{"elf", []string{"elves"}},
		{"wincon", []string{"hoof"}},
		{`"titan ape"`, []string{"kog"}},
		// Not a keyword, or not a value it can read: plain text.
		{"zz:top", nil},
		{"mv<x", nil},
		{"Ape", []string{"kog"}},
		// A stray parenthesis doesn't lose the rest.
		{"t:creature ) c=g mv>5", []string{"hoof"}},
		{"(t:creature c:r", []string{"kog"}},
	} {
		q := Parse(tc.q)
		got := map[string]bool{}
		for k, c := range all {
			if q.Match(c) {
				got[k] = true
			}
		}
		if len(got) != len(tc.want) {
			t.Errorf("%q matched %v, want %v", tc.q, keys(got), tc.want)
			continue
		}
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%q matched %v, want %v", tc.q, keys(got), tc.want)
				break
			}
		}
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
