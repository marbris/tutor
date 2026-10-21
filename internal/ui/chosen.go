package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"ttr/internal/mtg"
	"ttr/internal/paths"
)

// The printing you chose for a card.
//
// Wherever H and L leave the printing view is the printing that card is
// shown in from then on — in every list, deck and session — until they move
// it again. It is kept apart from the decks: the deck file says what you
// own, and this says what you like to look at, so choosing one writes no
// deck and makes no commit.
//
// Kept in the state directory, by the key gx files a card under (imageKey),
// as the chosen printing's Scryfall id. A choice whose printing has gone
// from the card's list is passed over, and the card's own printing shown.

const chosenFile = "printings.json"

// chosenPrintings is every choice made. Read on first use; the printings
// are worked out off the main loop, hence the lock.
var chosenPrintings struct {
	sync.Mutex
	loaded bool
	ids    map[string]string
}

func chosenPath() string { return filepath.Join(paths.State(), chosenFile) }

// chosenFor is the printing chosen for a card, or "".
func chosenFor(key string) string {
	chosenPrintings.Lock()
	defer chosenPrintings.Unlock()
	loadChosen()
	return chosenPrintings.ids[key]
}

// choose remembers a card's printing, and writes the choices down.
func choose(key, id string) {
	chosenPrintings.Lock()
	defer chosenPrintings.Unlock()
	loadChosen()
	if key == "" || id == "" || chosenPrintings.ids[key] == id {
		return
	}
	chosenPrintings.ids[key] = id
	body, err := json.Marshal(chosenPrintings.ids)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(chosenPath()), 0755)
	os.WriteFile(chosenPath(), body, 0644)
}

// loadChosen reads the choices, once. The caller holds the lock.
func loadChosen() {
	if chosenPrintings.loaded {
		return
	}
	chosenPrintings.loaded = true
	chosenPrintings.ids = map[string]string{}
	if body, err := os.ReadFile(chosenPath()); err == nil {
		json.Unmarshal(body, &chosenPrintings.ids)
	}
}

// withChosen is the card as gx should open it: on the printing chosen for
// it, when that is one of its printings, and otherwise on its own.
func withChosen(c mtg.Card, printings []mtg.Card) mtg.Card {
	id := chosenFor(imageKey(c))
	if id == "" || id == c.ID {
		return c
	}
	for _, p := range printings {
		if p.ID == id {
			c.ID = id
			return c
		}
	}
	return c
}
