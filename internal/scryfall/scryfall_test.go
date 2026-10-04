package scryfall

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ttr/internal/diskcache"
	"ttr/internal/mtg"
	"ttr/internal/paths"
)

// pagedServer stands in for Scryfall: a fixed number of cards handed out a
// page at a time, counting how many pages were actually asked for.
func pagedServer(t *testing.T, total, perPage int) (*httptest.Server, *int) {
	t.Helper()
	requests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := 1
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)

		start := (page - 1) * perPage
		if start >= total {
			w.WriteHeader(404)
			return
		}
		end := start + perPage
		if end > total {
			end = total
		}

		cards := make([]mtg.Card, 0, end-start)
		for i := start; i < end; i++ {
			cards = append(cards, mtg.Card{Name: fmt.Sprintf("Card %03d", i)})
		}
		json.NewEncoder(w).Encode(mtg.SearchResponse{
			Data: cards, TotalCards: total, HasMore: end < total,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func useSearch(t *testing.T, url string) {
	t.Helper()
	old := SearchURL
	SearchURL = url
	t.Cleanup(func() { SearchURL = old })
}

func TestSearchFollowsPagesUntilItHasEnough(t *testing.T) {
	srv, requests := pagedServer(t, 500, 175)
	useSearch(t, srv.URL)

	cards, total, err := Search("t:elf", "edhrec", "auto", 175)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 175 {
		t.Errorf("fetched %d cards, want the limit", len(cards))
	}
	if total != 500 {
		t.Errorf("total is %d, want what the query matched", total)
	}
	if *requests != 1 {
		t.Errorf("made %d requests for one page's worth", *requests)
	}
}

func TestSearchStopsWhenScryfallRunsOut(t *testing.T) {
	srv, requests := pagedServer(t, 30, 175)
	useSearch(t, srv.URL)

	cards, total, err := Search("t:elf", "edhrec", "auto", 175)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 30 {
		t.Errorf("fetched %d cards, want all 30", len(cards))
	}
	if total != 30 {
		t.Errorf("total is %d", total)
	}
	if *requests != 1 {
		t.Errorf("made %d requests", *requests)
	}
}

func TestSearchGathersSeveralPages(t *testing.T) {
	// A limit larger than a page means following HasMore.
	srv, requests := pagedServer(t, 200, 60)
	useSearch(t, srv.URL)

	cards, _, err := Search("t:elf", "edhrec", "auto", 175)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 175 {
		t.Errorf("fetched %d cards, want the limit", len(cards))
	}
	if *requests != 3 {
		t.Errorf("made %d requests, want three pages", *requests)
	}
	// And they arrive in order, which is the whole point of asking for one.
	if cards[0].Name != "Card 000" || cards[174].Name != "Card 174" {
		t.Errorf("came back as %s … %s", cards[0].Name, cards[174].Name)
	}
}

func TestSearchTruncatesToTheLimit(t *testing.T) {
	// A page can overshoot the limit; what comes back must not.
	srv, _ := pagedServer(t, 300, 60)
	useSearch(t, srv.URL)

	cards, _, err := Search("t:elf", "edhrec", "auto", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 100 {
		t.Errorf("fetched %d cards, want exactly the limit", len(cards))
	}
}

func TestNoResultsIsAnAnswerRatherThanAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	useSearch(t, srv.URL)

	cards, total, err := Search("t:elf t:island", "edhrec", "auto", 175)
	if err != nil {
		t.Fatalf("a query matching nothing was reported as a failure: %v", err)
	}
	if cards == nil {
		t.Error("came back nil rather than empty")
	}
	if len(cards) != 0 || total != 0 {
		t.Errorf("got %d cards, total %d", len(cards), total)
	}
}

func TestAFailureOnTheFirstPageIsAFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	useSearch(t, srv.URL)

	if _, _, err := Search("t:elf", "edhrec", "auto", 175); err == nil {
		t.Error("a server error was swallowed")
	}
}

func TestPagesAlreadyInHandBeatAFailureOnALaterOne(t *testing.T) {
	// Half an answer is worth more than none, and the total then describes
	// what we actually have rather than what the query claimed.
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page > 1 {
			w.WriteHeader(500)
			return
		}
		cards := make([]mtg.Card, 60)
		for i := range cards {
			cards[i] = mtg.Card{Name: fmt.Sprintf("Card %03d", i)}
		}
		json.NewEncoder(w).Encode(mtg.SearchResponse{
			Data: cards, TotalCards: 500, HasMore: true,
		})
	}))
	defer srv.Close()
	useSearch(t, srv.URL)

	cards, total, err := Search("t:elf", "edhrec", "auto", 175)
	if err != nil {
		t.Fatalf("the pages we had were thrown away: %v", err)
	}
	if len(cards) != 60 {
		t.Errorf("kept %d cards", len(cards))
	}
	if total != 60 {
		t.Errorf("total is %d, want what we actually have — 500 describes a "+
			"result set we failed to finish fetching", total)
	}
}

func TestTheQueryAndOrderReachTheRequest(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		json.NewEncoder(w).Encode(mtg.SearchResponse{})
	}))
	defer srv.Close()
	useSearch(t, srv.URL)

	Search(`o:"draw a card" c:u`, "cmc", "desc", 175)
	if !strings.Contains(got, "order=cmc") {
		t.Errorf("the order did not reach the request: %s", got)
	}
	if !strings.Contains(got, "dir=desc") {
		t.Errorf("the direction did not reach the request: %s", got)
	}
	if !strings.Contains(got, "draw+a+card") && !strings.Contains(got, "draw%20a%20card") {
		t.Errorf("the query did not reach the request: %s", got)
	}
}

func TestRulingsWithoutAURIIsEmptyRatherThanAFetch(t *testing.T) {
	got, err := Rulings("")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("came back nil; an empty slice says 'none', nil says 'not yet'")
	}
	if len(got) != 0 {
		t.Errorf("got %d rulings from nowhere", len(got))
	}
}

func TestACardWithNoRulingsComesBackEmptyNotNil(t *testing.T) {
	// The difference drives what the panel says: nil is "still loading",
	// empty is "there aren't any".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":null}`)
	}))
	defer srv.Close()

	got, err := Rulings(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("came back nil")
	}
}

func TestCollectionAsksInChunksAndKeysByID(t *testing.T) {
	var batches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		batches++
		var req struct {
			Identifiers []map[string]string `json:"identifiers"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Identifiers) > 75 {
			t.Errorf("asked for %d identifiers in one request", len(req.Identifiers))
		}

		cards := make([]mtg.Card, 0, len(req.Identifiers))
		for _, id := range req.Identifiers {
			cards = append(cards, mtg.Card{ID: id["id"], Name: "Card " + id["id"]})
		}
		json.NewEncoder(w).Encode(struct {
			Data []mtg.Card `json:"data"`
		}{cards})
	}))
	defer srv.Close()

	old := CollectionURL
	CollectionURL = srv.URL
	defer func() { CollectionURL = old }()

	ids := make([]string, 80)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%02d", i)
	}
	got, err := Collection(ids)
	if err != nil {
		t.Fatal(err)
	}
	if batches != 2 {
		t.Errorf("made %d requests for 80 identifiers", batches)
	}
	if len(got) != 80 {
		t.Errorf("got %d cards back", len(got))
	}
	if got["id-42"].Name != "Card id-42" {
		t.Errorf("keyed wrongly: %+v", got["id-42"])
	}
}

func TestRulingsAreKeptAndNotAskedForTwice(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `{"object":"list","data":[{"comment":"It does what it says."}]}`)
	}))
	defer srv.Close()

	for i := 0; i < 2; i++ {
		got, err := Rulings(srv.URL + "/cards/kept/rulings")
		if err != nil || len(got) != 1 {
			t.Fatalf("got %v, %v", got, err)
		}
	}
	if hits != 1 {
		t.Errorf("asked %d times, want once", hits)
	}
}

func TestStaleRulingsStandInWhenAskingFails(t *testing.T) {
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, `{"object":"list","data":[{"comment":"Old but true."}]}`)
	}))
	defer srv.Close()
	uri := srv.URL + "/cards/stale/rulings"
	Rulings(uri)
	path := filepath.Join(paths.Cache(), "rulings", diskcache.Key(uri))
	old := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(path, old, old)

	up = false
	got, err := Rulings(uri)
	if err != nil || len(got) != 1 {
		t.Errorf("with Scryfall down, got %v, %v; want the kept copy", got, err)
	}
}

func TestKeptRulingsAreAnsweredFromDiskWhateverTheirAge(t *testing.T) {
	uri := "https://api.scryfall.com/cards/kept-test/rulings"
	if _, _, ok := KeptRulings(uri); ok {
		t.Fatal("rulings never kept came back")
	}
	rel := filepath.Join("rulings", diskcache.Key(uri))
	diskcache.Save(rel, []mtg.Ruling{{Comment: "kept"}})
	got, fresh, ok := KeptRulings(uri)
	if !ok || !fresh || len(got) != 1 {
		t.Fatalf("just kept: ok %v fresh %v %v", ok, fresh, got)
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(filepath.Join(paths.Cache(), rel), old, old)
	got, fresh, ok = KeptRulings(uri)
	if !ok || fresh || len(got) != 1 {
		t.Errorf("a week old: ok %v fresh %v %v, want them back, stale", ok, fresh, got)
	}
}
