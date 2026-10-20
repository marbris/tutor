package cardstore

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ttr/internal/mtg"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ttr-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}

// fixture is a few cards as Scryfall's bulk file has them: Sol Ring in an
// ordinary set and as a newer promo, an MTGO-only printing of it, a token,
// an Alchemy card, a transforming card and a reversible one.
const fixture = `[
{"id":"sol-promo","oracle_id":"sol","layout":"normal","name":"Sol Ring","lang":"en","released_at":"2025-06-01","mana_cost":"{1}","cmc":1,"type_line":"Artifact","oracle_text":"{T}: Add {C}{C}.","colors":[],"color_identity":[],"keywords":[],"legalities":{"commander":"legal","vintage":"restricted","legacy":"banned","modern":"not_legal"},"digital":false,"rarity":"rare","promo":true,"set":"plst","set_name":"The List","set_type":"promo","collector_number":"1","border_color":"black","frame":"2015","edhrec_rank":1,"image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/s/o/sol-promo.jpg?1"},"prices":{"usd":"9.00"}},
{"id":"sol-cmm","oracle_id":"sol","layout":"normal","name":"Sol Ring","lang":"en","released_at":"2023-08-04","mana_cost":"{1}","cmc":1,"type_line":"Artifact","oracle_text":"{T}: Add {C}{C}.","colors":[],"color_identity":[],"keywords":[],"legalities":{"commander":"legal","vintage":"restricted","legacy":"banned","modern":"not_legal"},"digital":false,"rarity":"uncommon","set":"cmm","set_name":"Commander Masters","set_type":"masters","collector_number":"410","border_color":"black","frame":"2015","edhrec_rank":1,"illustration_id":"art-sol","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/s/o/sol-cmm.jpg?2"},"prices":{"usd":"1.50"}},
{"id":"sol-mtgo","oracle_id":"sol","layout":"normal","name":"Sol Ring","lang":"en","released_at":"2026-01-01","digital":true,"rarity":"mythic","set":"vma","set_type":"masters","collector_number":"7","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/s/o/sol-mtgo.jpg?3"}},
{"id":"tok","oracle_id":"goblin","layout":"token","name":"Goblin","released_at":"2020-01-01","type_line":"Token Creature — Goblin","set":"tm20"},
{"id":"alc","oracle_id":"alchemy","layout":"normal","name":"A-Alchemy Card","released_at":"2022-01-01","digital":true,"rarity":"rare","set":"ymid","set_type":"alchemy","collector_number":"1","type_line":"Creature — Elf","legalities":{"alchemy":"legal"}},
{"id":"delver","oracle_id":"delver","layout":"transform","name":"Delver of Secrets // Insectile Aberration","released_at":"2011-09-30","cmc":1,"type_line":"Creature — Human Wizard // Creature — Human Insect","colors":["U"],"color_identity":["U"],"keywords":["Transform"],"legalities":{"commander":"legal"},"rarity":"common","set":"isd","set_name":"Innistrad","set_type":"expansion","collector_number":"51","border_color":"black","frame":"2003","card_faces":[{"name":"Delver of Secrets","mana_cost":"{U}","type_line":"Creature — Human Wizard","oracle_text":"At the beginning of your upkeep, look at the top card of your library.","colors":["U"],"power":"1","toughness":"1","illustration_id":"art-d1","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/d/e/delver.jpg?4"}},{"name":"Insectile Aberration","type_line":"Creature — Human Insect","oracle_text":"Flying","colors":["U"],"power":"3","toughness":"2","illustration_id":"art-d2","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/back/d/e/delver.jpg?4"}}],"prices":{"usd":"0.30"}},
{"id":"rev","layout":"reversible_card","name":"Zndrsplt // Zndrsplt","released_at":"2022-01-01","rarity":"rare","set":"sld","set_type":"box","collector_number":"9","card_faces":[{"oracle_id":"zndr","name":"Zndrsplt","type_line":"Legendary Creature — Goblin","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/front/r/e/rev.jpg?5"}},{"oracle_id":"zndr","name":"Zndrsplt","type_line":"Legendary Creature — Goblin","image_uris":{"border_crop":"https://cards.scryfall.io/border_crop/back/r/e/rev.jpg?5"}}]}
]`

func build(t *testing.T) *Store {
	t.Helper()
	s, err := Build(strings.NewReader(fixture), filepath.Join(t.TempDir(), "store"), time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTheStoreHoldsTheCardsAndLeavesOutTokens(t *testing.T) {
	s := build(t)
	if s.Len() != 4 {
		t.Errorf("%d cards, want Sol Ring, the Alchemy card, Delver and Zndrsplt", s.Len())
	}
	if _, ok := s.Find("Goblin"); ok {
		t.Error("the token was kept")
	}
	for _, name := range []string{"sol ring", "Delver of Secrets // Insectile Aberration", "Delver of Secrets", "A-Alchemy Card", "Zndrsplt"} {
		if _, ok := s.Find(name); !ok {
			t.Errorf("no %q", name)
		}
	}
	if i, ok := s.FindOracle("delver"); !ok || s.Facts(i).Name != "Delver of Secrets // Insectile Aberration" {
		t.Error("Delver isn't found by its oracle id")
	}
}

func TestACardIsItsMostNormalPrintingAndKeepsEveryPaperOne(t *testing.T) {
	s := build(t)
	i, _ := s.Find("Sol Ring")
	c, err := s.Card(i)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "sol-cmm" || c.Prices.USD != "1.50" || c.Set != "cmm" {
		t.Errorf("Sol Ring is %s (%s, $%s), want the Commander Masters printing", c.ID, c.Set, c.Prices.USD)
	}
	if c.Image("normal") != "https://cards.scryfall.io/normal/front/s/o/sol-cmm.jpg?2" ||
		c.ImageURIs.PNG != "https://cards.scryfall.io/png/front/s/o/sol-cmm.png?2" {
		t.Errorf("the picture links weren't made from the border crop: %+v", c.ImageURIs)
	}
	if c.Legalities["vintage"] != "restricted" || c.Legalities["modern"] != "not_legal" || c.OracleText != "{T}: Add {C}{C}." {
		t.Errorf("the card's own fields are wrong: %+v", c)
	}
	if !c.PricedAt.Equal(s.Built) {
		t.Error("the price doesn't say how old it is")
	}

	ps, def, _ := s.Printings(i)
	if len(ps) != 2 || ps[0].ID != "sol-promo" || ps[1].ID != "sol-cmm" || def != 1 {
		t.Errorf("printings %v (default %d), want the promo then Commander Masters, without MTGO's", ids(ps), def)
	}
	if p, ok := s.Printing(i, "PLST", "1"); !ok || p.ID != "sol-promo" {
		t.Error("the pinned printing (plst) 1 wasn't found")
	}

	if a, ok := s.Find("A-Alchemy Card"); !ok {
		t.Error("an Alchemy card is all screen, and should be kept all the same")
	} else if c, _ := s.Card(a); !c.Digital {
		t.Error("the Alchemy card lost its digital printing")
	}
}

func TestFactsCoverEveryPrinting(t *testing.T) {
	s := build(t)
	i, _ := s.Find("Sol Ring")
	f := s.Facts(i)
	if strings.Join(f.Sets, ",") != "plst,cmm" {
		t.Errorf("sets %v", f.Sets)
	}
	if !f.Rarities.Has(1) || !f.Rarities.Has(2) || f.Rarities.Has(3) {
		t.Errorf("rarities %b, want uncommon and rare, not MTGO's mythic", f.Rarities)
	}
	commander, _ := FormatBit("commander")
	vintage, _ := FormatBit("vintage")
	legacy, _ := FormatBit("legacy")
	if f.Legal&commander == 0 || f.Legal&vintage == 0 || f.Banned&legacy == 0 || f.Legal&legacy != 0 {
		t.Errorf("legal %b banned %b", f.Legal, f.Banned)
	}
	if f.USD != 1.5 || f.Rarity != "uncommon" || f.Released != "2023-08-04" {
		t.Errorf("the facts aren't the default printing's: %+v", f)
	}

	d, _ := s.Find("Delver of Secrets")
	df := s.Facts(d)
	if df.Power != "1" || df.Colors.List()[0] != "U" || !strings.Contains(df.Text, "Flying") {
		t.Errorf("Delver's facts: %+v", df)
	}
	c, _ := s.Card(d)
	if c.PictureFaces() != 2 || c.FaceImage(1, "border_crop") != "https://cards.scryfall.io/border_crop/back/d/e/delver.jpg?4" {
		t.Errorf("Delver lost its back face's picture: %+v", c.CardFaces)
	}
}

func TestAStoreIsLoadedFromDiskAndRefreshedFromScryfall(t *testing.T) {
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write([]byte(fixture))
	w.Close()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bulk" {
			rw.Write([]byte(`{"jsonl_download_uri":"` + srv.URL + `/cards.jsonl.gz"}`))
			return
		}
		rw.Write(gz.Bytes())
	}))
	defer srv.Close()
	old := BulkURL
	BulkURL = srv.URL + "/bulk"
	defer func() { BulkURL = old }()

	if s, stale := Load(); s != nil || !stale {
		t.Fatal("a store was loaded before one was made")
	}
	if _, err := Refresh(); err != nil {
		t.Fatal(err)
	}
	s, stale := Load()
	if s == nil || stale || s.Len() != 4 {
		t.Fatalf("the refreshed store didn't load: %v, stale %v", s, stale)
	}
	if _, _, ok := Downloading(); ok {
		t.Error("the download still says it's running")
	}

	// A cards file that doesn't match its index isn't used.
	os.WriteFile(cardsFile(), []byte("{}\n"), 0644)
	if s, _ := Load(); s != nil {
		t.Error("a store whose files don't belong together was loaded")
	}
}

func ids(ps []mtg.Card) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}
