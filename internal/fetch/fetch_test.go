package fetch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGetReturnsTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	body, err := Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("got %q", body)
	}
}

func TestEveryRequestSaysWhoIsCalling(t *testing.T) {
	// Four services are involved and all of them are entitled to know.
	var agent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	for _, call := range []func() ([]byte, error){
		func() ([]byte, error) { return Get(srv.URL) },
		func() ([]byte, error) { return Post(srv.URL, []byte("{}")) },
		func() ([]byte, error) { return GetFile(srv.URL) },
	} {
		agent = ""
		call()
		if agent != UserAgent {
			t.Errorf("sent User-Agent %q, want %q", agent, UserAgent)
		}
	}
}

func TestA404IsItsOwnKindOfAnswer(t *testing.T) {
	// Several callers treat it as "no such thing" rather than a failure, so
	// it has to be distinguishable from one.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	_, err := Get(srv.URL)
	if err == nil {
		t.Fatal("a 404 came back as success")
	}
	if _, ok := err.(NotFound); !ok {
		t.Errorf("got %T, want NotFound", err)
	}
}

func TestAnErrorNamesTheServiceAndTheStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		fmt.Fprint(w, "down for maintenance")
	}))
	defer srv.Close()

	_, err := Get(srv.URL)
	if err == nil {
		t.Fatal("a 503 came back as success")
	}
	msg := err.Error()
	if !strings.Contains(msg, "503") || !strings.Contains(msg, "down for maintenance") {
		t.Errorf("the error says %q", msg)
	}
}

func TestPostSendsTheBody(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		got = string(buf)
		fmt.Fprint(w, "{}")
	}))
	defer srv.Close()

	if _, err := Post(srv.URL, []byte(`{"identifiers":[]}`)); err != nil {
		t.Fatal(err)
	}
	if got != `{"identifiers":[]}` {
		t.Errorf("the server received %q", got)
	}
}

func TestGetFileKeepsItsHeadersThroughARedirect(t *testing.T) {
	// Wizards serves the rulebook through one, and Go drops headers across
	// hosts unless they are set again on the way.
	var agentAtTarget string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentAtTarget = r.Header.Get("User-Agent")
		fmt.Fprint(w, "the rules")
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	body, err := GetFile(redirect.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "the rules" {
		t.Errorf("got %q", body)
	}
	if agentAtTarget != UserAgent {
		t.Errorf("the User-Agent was lost across the redirect: %q", agentAtTarget)
	}
}

func TestHostNamesTheServiceRatherThanTheEndpoint(t *testing.T) {
	for u, want := range map[string]string{
		"https://api.scryfall.com/cards/search?q=x": "scryfall.com",
		"https://api2.moxfield.com/v3/decks/all/x":  "api2.moxfield.com",
		"https://mtgjson.com/api/v5/LEA.json.gz":    "mtgjson.com",
		"nonsense":                                  "request",
	} {
		if got := Host(u); got != want {
			t.Errorf("Host(%q) = %q, want %q", u, got, want)
		}
	}
}

// paced puts a test server under a limiter, as if it were Scryfall's API.
func paced(t *testing.T, srv *httptest.Server, gap time.Duration) *limiter {
	t.Helper()
	host := strings.TrimPrefix(srv.URL, "http://")
	lim := &limiter{gap: gap}
	limiters[host] = lim
	t.Cleanup(func() { delete(limiters, host) })
	return lim
}

func TestRequestsToAPacedHostAreSpacedOut(t *testing.T) {
	var mu sync.Mutex
	var at []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		at = append(at, time.Now())
		mu.Unlock()
	}))
	defer srv.Close()
	paced(t, srv, 30*time.Millisecond)

	// From several goroutines at once, the way the panels ask.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); Get(srv.URL) }()
	}
	wg.Wait()
	if len(at) != 4 {
		t.Fatalf("%d requests arrived", len(at))
	}
	sort.Slice(at, func(i, j int) bool { return at[i].Before(at[j]) })
	if span := at[3].Sub(at[0]); span < 85*time.Millisecond {
		t.Errorf("four requests arrived within %v, want at least three gaps apart", span)
	}
}

func TestA429HoldsBackEveryRequestUntilTheHostSaysSo(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"details": "You are being rate-limited"}`)
	}))
	defer srv.Close()
	paced(t, srv, time.Millisecond)

	_, err := Get(srv.URL)
	rl, ok := err.(RateLimited)
	if !ok {
		t.Fatalf("got %v, want RateLimited", err)
	}
	if strings.Contains(err.Error(), "details") || !strings.Contains(err.Error(), "slow down") {
		t.Errorf("error reads %q", err)
	}
	if d := time.Until(rl.Until); d < 25*time.Second || d > 31*time.Second {
		t.Errorf("cooling off for %v, want the 30s the host asked for", d)
	}
	if _, err := Get(srv.URL); err == nil || hits != 1 {
		t.Errorf("a request went out during the cool-off (%d hits)", hits)
	}
}

func TestSearchesAreHeldToTheirOwnTighterPace(t *testing.T) {
	var mu sync.Mutex
	var searches, others []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/cards/search") {
			searches = append(searches, time.Now())
		} else {
			others = append(others, time.Now())
		}
	}))
	defer srv.Close()
	paced(t, srv, 5*time.Millisecond)
	host := strings.TrimPrefix(srv.URL, "http://")
	pathLimiters[host] = []pathLimiter{{"/cards/search", &limiter{gap: 60 * time.Millisecond}}}
	t.Cleanup(func() { delete(pathLimiters, host) })

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); Get(srv.URL + "/cards/search?q=x") }()
		go func() { defer wg.Done(); Get(srv.URL + "/cards/abc/rulings") }()
	}
	wg.Wait()
	sort.Slice(searches, func(i, j int) bool { return searches[i].Before(searches[j]) })
	if len(searches) != 3 || searches[2].Sub(searches[0]) < 115*time.Millisecond {
		t.Errorf("three searches came within %v, want two search gaps", searches[2].Sub(searches[0]))
	}
	sort.Slice(others, func(i, j int) bool { return others[i].Before(others[j]) })
	if len(others) != 3 || others[2].Sub(others[0]) > 100*time.Millisecond {
		t.Errorf("the rulings waited on the searches: %v", others[2].Sub(others[0]))
	}
}

func TestScryfallsLimitsAreKeptWithAMargin(t *testing.T) {
	// The documented limits: 10/s overall, 2/s for the searches.
	if scryfallGap <= 100*time.Millisecond || searchGap <= 500*time.Millisecond || manifestGap <= 6*time.Second {
		t.Error("a gap is at or under Scryfall's limit")
	}
	for _, p := range []string{"/cards/search", "/cards/named", "/cards/random", "/cards/collection"} {
		if l := pathLimiterFor("api.scryfall.com", p); l == nil || l.gap != searchGap {
			t.Errorf("%s isn't paced as a search", p)
		}
	}
	if pathLimiterFor("api.scryfall.com", "/cards/abc/rulings") != nil {
		t.Error("rulings are paced as a search")
	}
}
