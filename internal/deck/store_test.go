package deck

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ttr/internal/cardstore"
	"ttr/internal/mtg"
	"ttr/internal/scryfall"
)

const storeCards = `{"id":"sol-cmm","oracle_id":"sol","layout":"normal","name":"Sol Ring","released_at":"2023-08-04","type_line":"Artifact","legalities":{"commander":"legal"},"rarity":"uncommon","set":"cmm","set_type":"masters","collector_number":"410","border_color":"black","frame":"2015","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/s/o/sol-cmm.jpg"},"prices":{"usd":"1.50"}}
{"id":"sol-c21","oracle_id":"sol","layout":"normal","name":"Sol Ring","released_at":"2021-04-23","type_line":"Artifact","legalities":{"commander":"legal"},"rarity":"uncommon","set":"c21","set_type":"commander","collector_number":"263","border_color":"black","frame":"2015","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/s/o/sol-c21.jpg"},"prices":{"usd":"2.00"}}
{"id":"fire","oracle_id":"fire","layout":"split","name":"Fire // Ice","released_at":"2001-01-01","type_line":"Instant // Instant","legalities":{"commander":"legal"},"rarity":"uncommon","set":"apc","set_type":"expansion","collector_number":"128","border_color":"black","frame":"1997","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/f/i/fire.jpg"},"card_faces":[{"name":"Fire","oracle_text":"Fire deals 2 damage divided as you choose."},{"name":"Ice","oracle_text":"Tap target permanent."}]}
`

func withStore(t *testing.T) {
	t.Helper()
	s, err := cardstore.Build(strings.NewReader(storeCards), filepath.Join(t.TempDir(), "store"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cardstore.SetCurrent(s)
	t.Cleanup(func() { cardstore.SetCurrent(nil) })
}

func TestADeckResolvesFromTheCardsOnDisk(t *testing.T) {
	withStore(t)
	// Nothing to ask: if it reaches out, it fails.
	defer func(u string) { scryfall.CollectionURL = u }(scryfall.CollectionURL)
	scryfall.CollectionURL = "http://127.0.0.1:1/nothing-listening"

	cards, err := Resolve([]Entry{
		{Qty: 1, Name: "sol ring", Section: "mainboard", Tags: []string{"ramp"}},
		{Qty: 1, Name: "Sol Ring", Set: "c21", Collector: "263", Section: "mainboard"},
		{Qty: 1, Name: "Fire", Section: "mainboard"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 {
		t.Fatalf("%d cards", len(cards))
	}
	if cards[0].Card.ID != "sol-cmm" || cards[0].Tags[0] != "ramp" {
		t.Errorf("an unpinned Sol Ring is %s, want its most normal printing", cards[0].Card.ID)
	}
	if cards[1].Card.ID != "sol-c21" || cards[1].Card.Prices.USD != "2.00" {
		t.Errorf("the pinned printing is %s", cards[1].Card.ID)
	}
	if cards[2].Card.Name != "Fire // Ice" {
		t.Errorf("Fire found %q", cards[2].Card.Name)
	}
	if got, complete := ResolveCached([]Entry{{Qty: 1, Name: "Fire"}}); !complete || got[0].Card.Name != "Fire // Ice" {
		t.Error("ResolveCached doesn't use the cards on disk")
	}
	if got := CachedByOracle([]string{"sol"}); got["sol"].Name != "Sol Ring" {
		t.Error("CachedByOracle doesn't use the cards on disk")
	}
}

func TestACardTheStoreLacksStillComesFromTheKeptCards(t *testing.T) {
	withStore(t)
	seedCache(t, map[string]mtg.Card{"a-new card": {Name: "A-New Card", TypeLine: "Creature"}})
	cards, err := Resolve([]Entry{{Qty: 1, Name: "Sol Ring"}, {Qty: 1, Name: "A-New Card"}})
	if err != nil || len(cards) != 2 || cards[1].Card.Name != "A-New Card" {
		t.Errorf("got %v, %v", cards, err)
	}
}

func TestALegalityCheckNeedsNoReadOfTheDisk(t *testing.T) {
	withStore(t)
	entries := []Entry{{Qty: 1, Name: "Sol Ring"}, {Qty: 1, Name: "Fire", Section: "commander"}}
	full, _ := ResolveCached(entries)
	light, complete := resolveForCheck(entries, "commander")
	if !complete || len(light) != 2 {
		t.Fatalf("%v, %v", light, complete)
	}
	if a, b := Check("commander", full, true), Check("commander", light, true); a.Summary() != b.Summary() {
		t.Errorf("the light cards check as %q, the full ones as %q", b.Summary(), a.Summary())
	}
	if light[0].Card.Legalities["commander"] != "legal" || light[1].Card.Name != "Fire // Ice" {
		t.Errorf("light cards %+v", light)
	}
}
