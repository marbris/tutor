package query

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ttr/internal/cardstore"
	"ttr/internal/deck"
	"ttr/internal/mtg"
)

func TestIsCommanderIsScryfalls(t *testing.T) {
	mk := func(tl, text, pow string, faces ...mtg.Face) deck.Card {
		return deck.Card{Card: mtg.Card{Name: tl, TypeLine: tl, OracleText: text, Power: pow, CardFaces: faces}}
	}
	for _, tc := range []struct {
		c    deck.Card
		want bool
	}{
		{mk("Legendary Creature — Ape", "", "7"), true},
		{mk("Legendary Enchantment — Background", "", ""), true},
		{mk("Legendary Artifact — Vehicle", "Crew 2", "4"), true},
		{mk("Legendary Artifact — Spacecraft", "Station", ""), false}, // no power: not a creature yet
		{mk("Legendary Planeswalker — Daretti", "Daretti, Scrap Savant can be your commander.", ""), true},
		{mk("Legendary Planeswalker — Jace", "", ""), false},
		{mk("Creature — Elf", "", "1"), false},
		{mk("Artifact — Vehicle", "", "3"), false},
		// A transforming card is its front.
		{mk("Creature — Human Monk // Legendary Creature — Human Monk", "", "", mtg.Face{Power: "1"}), false},
		{mk("Legendary Creature — Human // Legendary Creature — Avatar", "", "", mtg.Face{Power: "2"}), true},
	} {
		if got := Parse("is:commander").Match(tc.c); got != tc.want {
			t.Errorf("is:commander on %q (%q) = %v, want %v", tc.c.Card.TypeLine, tc.c.Card.OracleText, got, tc.want)
		}
	}
}

func TestUnknownKeywordsAreNamed(t *testing.T) {
	for q, want := range map[string][]string{
		`a:"rk post" year>2020 t:elf`: {"a:rk post", "year>2020"},
		"t:elf mv<=2 is:commander":    nil,
		"is:spell":                    {"is:spell"},
		"circle of protection: red":   nil,
	} {
		if got := Parse(q).Unknown(); !reflect.DeepEqual(got, want) {
			t.Errorf("Unknown(%q) = %q, want %q", q, got, want)
		}
	}
}

func TestASearchsPlainWordsAreNames(t *testing.T) {
	all := cards()
	if !Parse("add").Match(all["elves"]) {
		t.Error("/ stopped matching a word in the text")
	}
	if ParseSearch("add").Match(all["elves"]) {
		t.Error("a search's plain word matched the text, not the name")
	}
	if !ParseSearch("ape").Match(all["kog"]) {
		t.Error("a search's plain word didn't match the name")
	}
	if !ParseSearch(`!"sol ring"`).Match(all["ring"]) || ParseSearch(`!"sol"`).Match(all["ring"]) {
		t.Error("! isn't the exact name")
	}
}

const storeCards = `{"id":"a1","oracle_id":"a","layout":"normal","name":"Alpha Elf","released_at":"2020-01-01","cmc":3,"type_line":"Creature — Elf","colors":["G"],"power":"2","legalities":{"commander":"legal"},"rarity":"common","set":"new","set_type":"expansion","edhrec_rank":50,"image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/a/1/a1.jpg"},"prices":{"usd":"3.00"}}
{"id":"a2","oracle_id":"a","layout":"normal","name":"Alpha Elf","released_at":"1995-01-01","cmc":3,"type_line":"Creature — Elf","colors":["G"],"power":"2","legalities":{"commander":"legal"},"rarity":"rare","set":"old","set_type":"expansion","edhrec_rank":50,"image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/a/2/a2.jpg"},"prices":{"usd":"30.00"}}
{"id":"b1","oracle_id":"b","layout":"normal","name":"Beta Elf","released_at":"2024-01-01","cmc":1,"type_line":"Legendary Creature — Elf","colors":["G"],"power":"5","legalities":{"commander":"legal"},"rarity":"mythic","set":"new","set_type":"expansion","edhrec_rank":10,"image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/b/1/b1.jpg"},"prices":{"usd":"0.50"}}
{"id":"c1","oracle_id":"c","layout":"normal","name":"Gamma Bolt","released_at":"2010-01-01","cmc":1,"type_line":"Instant","colors":["R"],"legalities":{"commander":"banned"},"rarity":"common","set":"mid","set_type":"expansion","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/c/1/c1.jpg"}}
`

func store(t *testing.T) *cardstore.Store {
	t.Helper()
	s, err := cardstore.Build(strings.NewReader(storeCards), filepath.Join(t.TempDir(), "s"), time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func names(s *cardstore.Store, idx []int) string {
	var out []string
	for _, i := range idx {
		out = append(out, s.Facts(i).Name)
	}
	return strings.Join(out, ", ")
}

func TestSearchTheStore(t *testing.T) {
	s := store(t)
	for _, tc := range []struct{ q, order, dir, want string }{
		{"elf", "name", "auto", "Alpha Elf, Beta Elf"},
		{"t:creature", "edhrec", "auto", "Beta Elf, Alpha Elf"},
		{"t:creature", "cmc", "auto", "Beta Elf, Alpha Elf"},
		{"t:creature", "cmc", "desc", "Alpha Elf, Beta Elf"},
		{"", "usd", "auto", "Alpha Elf, Beta Elf, Gamma Bolt"}, // dearest first; no price last
		{"", "edhrec", "auto", "Beta Elf, Alpha Elf, Gamma Bolt"},
		{"", "released", "auto", "Beta Elf, Alpha Elf, Gamma Bolt"},
		{"", "color", "auto", "Gamma Bolt, Alpha Elf, Beta Elf"},
		// Any printing: Alpha Elf was rare once, in the old set.
		{"r:rare", "name", "auto", "Alpha Elf"},
		{"set:old", "name", "auto", "Alpha Elf"},
		{"is:commander", "name", "auto", "Beta Elf"},
		{"banned:commander", "name", "auto", "Gamma Bolt"},
		{"o:nothing", "name", "auto", ""},
	} {
		if got := names(s, Search(s, ParseSearch(tc.q), tc.order, tc.dir)); got != tc.want {
			t.Errorf("%q by %s %s = %q, want %q", tc.q, tc.order, tc.dir, got, tc.want)
		}
	}
}
