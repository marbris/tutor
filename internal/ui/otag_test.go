package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/config"
	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/scryfall"
	"ttr/internal/tagger"
)

func TestOtagNamesAreWhatWasTyped(t *testing.T) {
	got := otagNames("Removal  otag:ramp otag-draw removal")
	want := []string{"removal", "ramp", "draw"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestOtagQueriesQuoteEachNameAndStayShort(t *testing.T) {
	one := otagQueries("removal", []string{"Sol Ring", "Fire // Ice"})
	if len(one) != 1 || one[0] != `otag:removal (!"Sol Ring" or !"Fire // Ice")` {
		t.Errorf("got %q", one)
	}

	var names []string
	for i := 0; i < 200; i++ {
		names = append(names, "Some Fairly Long Card Name "+itoa(i))
	}
	qs := otagQueries("ramp", names)
	if len(qs) < 2 {
		t.Fatalf("200 names went in %d query", len(qs))
	}
	n := 0
	for _, q := range qs {
		if len(q) > otagQueryLen || len(q) > 1024 {
			t.Errorf("query is %d long", len(q))
		}
		n += strings.Count(q, "!\"")
	}
	if n != 200 {
		t.Errorf("%d names asked about, want 200", n)
	}
}

func TestATwoFacedCardIsFoundByEitherName(t *testing.T) {
	if !sameCard("Fire // Ice", "Fire // Ice") || !sameCard("Delver of Secrets", "Delver of Secrets // Insectile Aberration") {
		t.Error("didn't match")
	}
	if sameCard("Fire", "Fireball") {
		t.Error("matched a different card")
	}
}

func TestTOAsksScryfallWhenTheTaggerTagsArentIn(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		asked = append(asked, q)
		var sr mtg.SearchResponse
		if strings.HasPrefix(q, "otag:ramp ") {
			sr.Data = []mtg.Card{{Name: "Sol Ring"}, {Name: "Llanowar Elves"}}
			sr.TotalCards = 2
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(sr)
	}))
	defer srv.Close()
	old := scryfall.SearchURL
	scryfall.SearchURL = srv.URL
	t.Cleanup(func() { scryfall.SearchURL = old })

	seedDeck(t, "ghen", "name: Ghen\nformat: commander\n[mainboard]\n1 Sol Ring\n")
	t.Cleanup(func() { deck.Delete("ghen") })
	m, l := openDeckPanel(t, sized(160, 30), "ghen", "Ghen", sample())

	m = drive(m, "T", "o")
	if m.ws.current().asking != askOtag {
		t.Fatal("T o didn't ask for oracle tags")
	}
	for _, r := range "ramp removal" {
		m = drive(m, string(r))
	}
	m, cmd := press(m, "enter")
	next, _ := m.Update(msgOf(cmd))
	m = next.(Model)

	if len(asked) != 2 {
		t.Errorf("asked %d questions, want one per tag: %q", len(asked), asked)
	}
	tagged := map[string]bool{}
	for _, c := range l.all {
		for _, tg := range c.Tags {
			if tg == "otag-ramp" {
				tagged[c.Card.Name] = true
			}
			if tg == "otag-removal" {
				t.Errorf("%s tagged removal", c.Card.Name)
			}
		}
	}
	if !tagged["Sol Ring"] || !tagged["Llanowar Elves"] || len(tagged) != 2 {
		t.Errorf("tagged %v", tagged)
	}
	if !strings.Contains(m.notice, "otag-ramp: 2 of 4") {
		t.Errorf("notice %q", m.notice)
	}
}

func TestOtagTaggingIsWorkedOutLocallyFromTheTaggerTags(t *testing.T) {
	// No request may go out: the answer is on disk.
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	old := scryfall.SearchURL
	scryfall.SearchURL = srv.URL
	t.Cleanup(func() { scryfall.SearchURL = old })

	withTagger(t)
	m, l := ownDeck(sized(160, 30))
	l.all = taggedCards()
	l.refresh()
	m = drive(m, "T", "o")
	for _, r := range "removal nosuchtag" {
		m = drive(m, string(r))
	}
	m, cmd := press(m, "enter")
	if asked != 0 {
		t.Errorf("asked Scryfall %d times; the tags are local", asked)
	}
	if cmd == nil {
		t.Error("the tagging wasn't saved")
	}

	// removal has no cards of its own: its children carry them, and it
	// counts them, as otag: does.
	tagged := map[string]bool{}
	for _, c := range l.all {
		for _, tg := range c.Tags {
			if tg == "otag-removal" {
				tagged[c.Card.Name] = true
			}
		}
	}
	if len(tagged) != 2 || !tagged["shatter"] || !tagged["disenchant"] {
		t.Errorf("otag-removal went on %v, want shatter and disenchant", tagged)
	}
	if !strings.Contains(m.notice, "otag-removal: 2 of 4") || !strings.Contains(m.notice, "no nosuchtag") {
		t.Errorf("notice %q", m.notice)
	}

	// One undo takes it all back (u undoes the editing deck, so make it that).
	m = drive(m, "e", "u")
	for _, c := range l.all {
		if len(c.Tags) != 0 {
			t.Errorf("%s kept %v after undo", c.Card.Name, c.Tags)
		}
	}
}

// otagAddOf runs what enter handed back and keeps T O's answer, leaving
// any other command (a hover's rulings) unrun.
func otagAddOf(t *testing.T, cmd tea.Cmd) otagAddMsg {
	t.Helper()
	for _, msg := range messages(cmd) {
		if a, ok := msg.(otagAddMsg); ok {
			return a
		}
	}
	t.Fatal("T O handed back no answer to wait for")
	return otagAddMsg{}
}

func TestTOBuildsAnOtagListFromTheTaggerTagsOnDisk(t *testing.T) {
	// Which cards carry the tag is on disk, so Scryfall is never searched;
	// it's only asked for the card it hasn't got, by oracle id.
	searched := 0
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searched++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer search.Close()
	var askedFor []string
	collection := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Identifiers []map[string]string `json:"identifiers"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var out struct {
			Data []mtg.Card `json:"data"`
		}
		for _, id := range req.Identifiers {
			askedFor = append(askedFor, id["oracle_id"])
			out.Data = append(out.Data, mtg.Card{Name: id["oracle_id"], OracleID: id["oracle_id"], TypeLine: "Instant"})
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer collection.Close()
	oldS, oldC := scryfall.SearchURL, scryfall.CollectionURL
	scryfall.SearchURL, scryfall.CollectionURL = search.URL, collection.URL
	t.Cleanup(func() { scryfall.SearchURL, scryfall.CollectionURL = oldS, oldC })

	withTagger(t)
	m, l := ownDeck(sized(160, 30))
	l.all = taggedCards()[:1] // shatter
	l.refresh()
	m = drive(m, "T", "O")
	for _, r := range "removal" {
		m = drive(m, string(r))
	}
	m, cmd := press(m, "enter")
	next, _ := m.Update(otagAddOf(t, cmd))
	m = next.(Model)

	if searched != 0 {
		t.Errorf("searched Scryfall %d times; the tags are on disk", searched)
	}
	if len(askedFor) != 1 || askedFor[0] != "disenchant" {
		t.Errorf("asked for %v, want only disenchant, the card the deck lacks", askedFor)
	}
	for _, name := range []string{"shatter", "disenchant"} {
		if !hasTag(tagged(l, name), "otag-removal") {
			t.Errorf("%s has %v, want otag-removal", name, tagged(l, name))
		}
	}
	if len(l.all) != 2 {
		t.Errorf("the deck holds %d cards, want 2", len(l.all))
	}
	if !strings.Contains(m.notice, "1 added, 1 tagged") {
		t.Errorf("notice %q", m.notice)
	}
	m.undo()
	if len(l.all) != 1 || len(tagged(l, "shatter")) != 0 {
		t.Error("one undo didn't take T O back")
	}
}

func TestTOSearchesScryfallOnlyWithoutTheTaggerTags(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		asked = append(asked, q)
		sr := mtg.SearchResponse{TotalCards: 2, Data: []mtg.Card{
			{Name: "Ball Lightning", OracleID: "bl"}, {Name: "Fireball Lightning", OracleID: "fbl"}}}
		json.NewEncoder(w).Encode(sr)
	}))
	defer srv.Close()
	old := scryfall.SearchURL
	scryfall.SearchURL = srv.URL
	t.Cleanup(func() { scryfall.SearchURL = old })
	oldTg := tagger.Current()
	tagger.SetCurrent(nil)
	t.Cleanup(func() { tagger.SetCurrent(oldTg) })

	m, l := ownDeck(sized(160, 30))
	l.all = nil
	l.refresh()
	m = drive(m, "T", "O")
	for _, r := range "ball-lightning" {
		m = drive(m, string(r))
	}
	m, cmd := press(m, "enter")
	next, _ := m.Update(otagAddOf(t, cmd))
	m = next.(Model)
	if len(asked) != 1 || asked[0] != "otag:ball-lightning" {
		t.Errorf("asked %q", asked)
	}
	if len(l.all) != 2 || !hasTag(tagged(l, "Ball Lightning"), "otag-ball-lightning") {
		t.Errorf("the list holds %v", l.all)
	}
	if !strings.Contains(m.notice, "aren't downloaded") {
		t.Errorf("notice %q should say why Scryfall was asked", m.notice)
	}
}

func TestTOFromAnotherListTagsTheEditingDeck(t *testing.T) {
	withTagger(t)
	m, l := ownDeck(sized(200, 30))
	l.all = taggedCards()
	l.refresh()
	m = withCards(m, "f", nil, sortArrival) // an empty search, focused
	m = drive(m, "T", "o", "r", "a", "m", "p", "enter")
	if !hasTag(tagged(l, "sol"), "otag-ramp") {
		t.Errorf("sol has %v, want otag-ramp in the editing deck", tagged(l, "sol"))
	}
}

func TestTheOtagPrefixIsConfigJSONs(t *testing.T) {
	for _, c := range []struct{ prefix, want string }{
		{"o/", "o/removal"},
		{"", "removal"},
	} {
		SetOtagPrefix(c.prefix)
		withTagger(t)
		m, l := ownDeck(sized(160, 30))
		l.all = taggedCards()
		l.refresh()
		m = drive(m, "T", "o")
		for _, r := range "removal" {
			m = drive(m, string(r))
		}
		press(m, "enter")
		n := 0
		for _, card := range l.all {
			for _, tg := range card.Tags {
				if tg == c.want {
					n++
				}
			}
		}
		if n != 2 {
			t.Errorf("prefix %q: %q went on %d cards, want 2", c.prefix, c.want, n)
		}
	}
	SetOtagPrefix(config.DefaultOtagPrefix)
}

func TestATypedOtagPrefixIsTakenOff(t *testing.T) {
	SetOtagPrefix("o/")
	defer SetOtagPrefix(config.DefaultOtagPrefix)
	got := otagNames("o/ramp otag-removal otag:draw")
	if strings.Join(got, " ") != "ramp removal draw" {
		t.Errorf("got %v", got)
	}
}
