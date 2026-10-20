package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ttr/internal/cardstore"
)

// storeCards are a few cards as Scryfall's bulk file has them.
const storeCards = `{"id":"sol-cmm","oracle_id":"sol","layout":"normal","name":"Sol Ring","released_at":"2023-08-04","mana_cost":"{1}","cmc":1,"type_line":"Artifact","oracle_text":"{T}: Add {C}{C}.","legalities":{"commander":"legal"},"rarity":"uncommon","set":"cmm","set_name":"Commander Masters","set_type":"masters","collector_number":"410","border_color":"black","frame":"2015","edhrec_rank":1,"image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/s/o/sol.jpg?1"},"prices":{"usd":"1.50"}}
{"id":"elves-m19","oracle_id":"elves","layout":"normal","name":"Llanowar Elves","released_at":"2018-07-13","mana_cost":"{G}","cmc":1,"type_line":"Creature — Elf Druid","oracle_text":"{T}: Add {G}.","colors":["G"],"color_identity":["G"],"power":"1","toughness":"1","legalities":{"commander":"legal"},"rarity":"common","set":"m19","set_name":"Core Set 2019","set_type":"core","collector_number":"314","border_color":"black","frame":"2015","edhrec_rank":40,"image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/e/l/elves.jpg?1"},"prices":{"usd":"0.25"}}
{"id":"dwynen-ori","oracle_id":"dwynen","layout":"normal","name":"Dwynen, Gilt-Leaf Daen","released_at":"2015-07-17","mana_cost":"{2}{G}{G}","cmc":4,"type_line":"Legendary Creature — Elf Warrior","oracle_text":"Reach\nOther Elf creatures you control get +1/+1.","colors":["G"],"color_identity":["G"],"power":"3","toughness":"4","keywords":["Reach"],"legalities":{"commander":"legal"},"rarity":"rare","set":"ori","set_name":"Magic Origins","set_type":"core","collector_number":"172","border_color":"black","frame":"2015","edhrec_rank":3000,"image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/d/w/dwynen.jpg?1"},"prices":{"usd":"0.40"}}
`

// withStore puts a store of storeCards in use for the test.
func withStore(t *testing.T) *cardstore.Store {
	t.Helper()
	s, err := cardstore.Build(strings.NewReader(storeCards), filepath.Join(t.TempDir(), "store"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cardstore.SetCurrent(s)
	t.Cleanup(func() { cardstore.SetCurrent(nil) })
	return s
}

func TestAFirstCardDownloadIsPutInUseAndSaysSo(t *testing.T) {
	s, err := cardstore.Build(strings.NewReader(storeCards), filepath.Join(t.TempDir(), "store"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cardstore.SetCurrent(nil) })
	m := sized(160, 40)
	m.notice = "card data 40/76 MB"
	next, _ := m.handleCardStore(cardStoreMsg{store: s, first: true})
	m = next.(Model)
	if cardstore.Current() != s {
		t.Fatal("the store wasn't put in use")
	}
	if m.notice != "card data in: 3 cards" {
		t.Errorf("notice %q", m.notice)
	}
}

func TestACardStoreTurnedOffWhileOnItsWayIsNotUsed(t *testing.T) {
	s, err := cardstore.Build(strings.NewReader(storeCards), filepath.Join(t.TempDir(), "store"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cardstore.SetCurrent(nil) })
	saveDownloadsOff(map[string]bool{cardStoreDownload: true})
	t.Cleanup(func() { saveDownloadsOff(map[string]bool{}) })
	m := sized(160, 40)
	m.handleCardStore(cardStoreMsg{store: s})
	if cardstore.Current() != nil {
		t.Error("a store turned off was put in use")
	}
}
