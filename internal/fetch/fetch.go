// Package fetch is the one place ttr talks to the network.
//
// Four services are involved — Scryfall for cards, Moxfield for decks,
// MTGJSON for printed text, and Wizards for the rules — and they all want the
// same courtesy: a User-Agent that says who is calling. Keeping that in one
// place is the only way it stays true of all four.
package fetch

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UserAgent identifies ttr to every service it calls.
const UserAgent = "ttr/2.1"

// NotFound is a 404, which several callers treat as an answer rather than a
// failure: no such card, no such deck, no results.
type NotFound struct{}

func (e NotFound) Error() string { return "no results found" }

// Get fetches a URL as JSON.
func Get(u string) ([]byte, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json;q=0.9,*/*;q=0.8")
	return do(http.DefaultClient, req, u)
}

// Post sends a JSON body and reads a JSON reply.
func Post(u string, payload []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", u, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return do(http.DefaultClient, req, u)
}

// GetFile fetches something that isn't JSON — the comprehensive rules are a
// plain text file, served through a redirect that drops headers unless they
// are set again on the way.
func GetFile(u string) ([]byte, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Set("User-Agent", UserAgent)
			req.Header.Set("Accept", "*/*")
			return nil
		},
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "*/*")
	return do(client, req, u)
}

func do(client *http.Client, req *http.Request, u string) ([]byte, error) {
	lim := limiterFor(req.URL.Host)
	if lim != nil {
		if err := lim.check(); err != nil {
			return nil, err
		}
		// A search waits its turn among the searches, then among everything
		// — always in that order, so two requests can't each hold the turn
		// the other is waiting for.
		turns := []*limiter{lim}
		if path := pathLimiterFor(req.URL.Host, req.URL.Path); path != nil {
			turns = []*limiter{path, lim}
		}
		for _, l := range turns {
			l.take()
		}
		var once sync.Once
		sent := func() {
			once.Do(func() {
				for i := len(turns) - 1; i >= 0; i-- {
					turns[i].sent()
				}
			})
		}
		// The turn passes on once this request is on the wire, not when it
		// was let go: a new connection's handshake can hold one back while
		// the next goes straight out, and the two would land together.
		req = req.WithContext(httptrace.WithClientTrace(req.Context(),
			&httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { sent() }}))
		defer sent() // for a request that never got as far as writing
		if err := lim.check(); err != nil {
			return nil, err
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests && lim != nil {
		return nil, lim.backOff(resp.Header.Get("Retry-After"))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, NotFound{}
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s (%d): %s", Host(u), resp.StatusCode, string(body))
	}
	return body, nil
}

// Host labels an error with the service that produced it — cards and rulings
// come from Scryfall, decks from Moxfield.
func Host(u string) string {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return "request"
	}
	return strings.TrimPrefix(parsed.Host, "api.")
}

// ── Rate limits ─────────────────────────────────────────────────

// Scryfall's API has hard rate limits, and answers a client that breaks
// them with a 429, thirty seconds locked out, and then a block
// (https://scryfall.com/docs/api/rate-limits):
//
//	/cards/search, /cards/named, /cards/random, /cards/collection   2 a second
//	/cards/manifest                                                 10 a minute
//	everything else                                                 10 a second
//
// Every caller pausing between its own pages wasn't enough: the picture
// under the cursor, its rulings, a search and gX each paced themselves and
// none of them knew about the others — and a list of printings is a
// /cards/search, held to two a second, not ten. So the pacing is here, where
// every request passes, shared, and by endpoint. Each gap has a margin over
// the limit, so a request delayed on its way out can't bunch up with the
// next.
//
// The pictures come from cards.scryfall.io, which has no limit.

const (
	// scryfallGap paces every request to the API: 8 a second.
	scryfallGap = 125 * time.Millisecond
	// searchGap paces the card searches: 1.6 a second.
	searchGap = 625 * time.Millisecond
	// manifestGap paces /cards/manifest: 8 a minute.
	manifestGap = 7500 * time.Millisecond
)

// coolOff is how long to leave the API alone after a 429 that doesn't say.
// Scryfall locks a client out for 30 seconds; a little longer, to be sure.
const coolOff = 35 * time.Second

// limiters are the hosts that are paced, by host name. A variable, so the
// tests can pace a test server.
var limiters = map[string]*limiter{
	"api.scryfall.com": {gap: scryfallGap},
}

func limiterFor(host string) *limiter { return limiters[host] }

// pathLimiter is a tighter pace for some of a host's endpoints, on top of
// the host's own.
type pathLimiter struct {
	prefix string
	lim    *limiter
}

// pathLimiters are the endpoints with limits of their own, by host. The
// four searches share one limit between them.
var pathLimiters = func() map[string][]pathLimiter {
	search := &limiter{gap: searchGap}
	return map[string][]pathLimiter{
		"api.scryfall.com": {
			{"/cards/search", search},
			{"/cards/named", search},
			{"/cards/random", search},
			{"/cards/collection", search},
			{"/cards/manifest", &limiter{gap: manifestGap}},
		},
	}
}()

func pathLimiterFor(host, path string) *limiter {
	for _, pl := range pathLimiters[host] {
		if strings.HasPrefix(path, pl.prefix) {
			return pl.lim
		}
	}
	return nil
}

// limiter spaces the requests to one host, or one endpoint, and holds them
// all back for a while once the host has said to.
//
// One request has the turn at a time, from when it may go until it has been
// written to the connection; the next may go a gap after that.
type limiter struct {
	gap  time.Duration
	turn sync.Mutex
	last time.Time // when the last request went out; under turn

	mu    sync.Mutex
	until time.Time // after a 429: nothing goes before this
}

// RateLimited is a request not sent, or refused, because the service asked
// ttr to slow down.
type RateLimited struct {
	Until time.Time
}

func (e RateLimited) Error() string {
	wait := time.Until(e.Until).Round(time.Second)
	if wait < time.Second {
		wait = time.Second
	}
	return "Scryfall asked ttr to slow down — try again in " + wait.String()
}

// take waits for the turn, and then for a gap after the request before it
// went out.
func (l *limiter) take() {
	l.turn.Lock()
	time.Sleep(time.Until(l.last.Add(l.gap)))
}

// sent passes the turn on: the request holding it has gone.
func (l *limiter) sent() {
	l.last = time.Now()
	l.turn.Unlock()
}

// check refuses a request while the host is cooling off — a request sent
// then only lengthens the block.
func (l *limiter) check() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Now().Before(l.until) {
		return RateLimited{Until: l.until}
	}
	return nil
}

// backOff records a 429: nothing more goes to the host until it said, or
// for coolOff.
func (l *limiter) backOff(retryAfter string) error {
	d := coolOff
	if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs > 0 {
		d = time.Duration(secs) * time.Second
	}
	l.mu.Lock()
	l.until = time.Now().Add(d)
	until := l.until
	l.mu.Unlock()
	return RateLimited{Until: until}
}
