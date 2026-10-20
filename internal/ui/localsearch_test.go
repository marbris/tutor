package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ttr/internal/mtg"
	"ttr/internal/scryfall"
)

// scryfallAsked stands in for Scryfall's search and counts what it's asked.
func scryfallAsked(t *testing.T) *[]string {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode(mtg.SearchResponse{Data: []mtg.Card{{Name: "From Scryfall"}}, TotalCards: 1})
	}))
	t.Cleanup(srv.Close)
	old := scryfall.SearchURL
	scryfall.SearchURL = srv.URL
	t.Cleanup(func() { scryfall.SearchURL = old })
	return &asked
}

func TestASearchRunsOverTheCardsOnDisk(t *testing.T) {
	withStore(t)
	asked := scryfallAsked(t)
	m, p := typed(sized(160, 30), "t:creature")
	m, cmd := press(m, "enter")
	m = settle(m, cmd)

	if len(*asked) > 0 {
		t.Errorf("a search with the cards on disk asked Scryfall %v", *asked)
	}
	l := p.cardsView()
	if l == nil || len(l.rows) != 2 {
		t.Fatalf("the panel holds %v", l)
	}
	// edhrec, the default query sort: Llanowar Elves (40) before Dwynen.
	if l.rows[0].Card.Name != "Llanowar Elves" || l.rows[1].Card.Name != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("rows %s, %s", l.rows[0].Card.Name, l.rows[1].Card.Name)
	}
	if l.rows[0].Card.Prices.USD != "0.25" || l.rows[0].Card.Image("normal") == "" {
		t.Error("the cards came back without their printing")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "search order") {
		t.Error("the panel doesn't say the order is the search's")
	}
}

func TestAnUnknownKeywordIsNamedAndAltEnterAsksScryfall(t *testing.T) {
	withStore(t)
	asked := scryfallAsked(t)
	m, p := typed(sized(160, 30), "year>2020 elves")
	m, cmd := press(m, "enter")
	m = settle(m, cmd)
	if !strings.Contains(m.notice, "“year>2020” isn't searched locally") || !strings.Contains(m.notice, "alt+enter asks Scryfall") {
		t.Errorf("notice %q", m.notice)
	}
	if l := p.cardsView(); l == nil || len(l.rows) != 1 || l.rows[0].Card.Name != "Llanowar Elves" {
		t.Error("the rest of the query wasn't searched")
	}

	m = drive(m, "i")
	m, cmd = press(m, "alt+enter")
	m = settle(m, cmd)
	if len(*asked) != 1 || (*asked)[0] != "year>2020 elves" {
		t.Errorf("alt+enter asked Scryfall %v", *asked)
	}
	if l := p.cardsView(); l == nil || l.rows[0].Card.Name != "From Scryfall" {
		t.Error("Scryfall's answer isn't in the panel")
	}
}

func TestWithoutTheCardsASearchAsksScryfall(t *testing.T) {
	asked := scryfallAsked(t)
	m, _ := typed(sized(160, 30), "t:creature")
	m, cmd := press(m, "enter")
	settle(m, cmd)
	if len(*asked) != 1 {
		t.Errorf("Scryfall was asked %v", *asked)
	}
	if strings.Contains(stripANSI(m.View()), "ask scryfall instead") {
		t.Error("alt+enter is offered with nothing to choose between")
	}
}

func TestASearchAtTheStartWaitsForTheCardsOnDisk(t *testing.T) {
	asked := scryfallAsked(t)
	m := sized(160, 30)
	m.storeLoading = true
	p := m.ws.open(KindFind)
	p.search.SetValue("elves")
	if cmd := m.search(p, false); cmd != nil || !p.loading {
		t.Fatal("the search didn't wait")
	}
	s := withStore(t)
	cardstore := s // put in use by withStore; handed over as the load would
	next, cmd := m.handleCardStore(cardStoreMsg{store: cardstore})
	m = settle(next.(Model), cmd)
	if len(*asked) > 0 {
		t.Errorf("the waiting search asked Scryfall %v", *asked)
	}
	if l := p.cardsView(); l == nil || len(l.rows) != 1 {
		t.Errorf("the waiting search didn't run: %v", l)
	}
}

func TestASearchOfOnlyUnknownWordsFindsNothingRatherThanEverything(t *testing.T) {
	withStore(t)
	m, p := typed(sized(160, 30), `a:"rk post"`)
	m, cmd := press(m, "enter")
	m = settle(m, cmd)
	if p.err == nil || p.cardsView() != nil && len(p.cardsView().rows) > 0 {
		t.Error("a query of nothing but unknown words found cards")
	}
	if !strings.Contains(m.notice, "asks Scryfall") {
		t.Errorf("notice %q", m.notice)
	}
}
