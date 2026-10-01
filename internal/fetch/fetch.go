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
		if err := lim.wait(); err != nil {
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

// Scryfall's API asks for fewer than ten requests a second, and answers a
// client that ignores it with a 429 and then a block. Every caller pausing
// between its own pages wasn't enough: the picture under the cursor, its
// rulings, a search and gX each paced themselves and none of them knew about
// the others. So the pacing is here, where every request passes, and shared.
//
// The pictures come from cards.scryfall.io, which has no such limit.

// scryfallGap is the least time between two requests to the API: a little
// over a tenth of a second, to stay under ten a second rather than on it.
const scryfallGap = 110 * time.Millisecond

// coolOff is how long to leave the API alone after a 429 that doesn't say.
// Scryfall's own message asks for 60 seconds.
const coolOff = 60 * time.Second

// limiters are the hosts that are paced, by host name. A variable, so the
// tests can pace a test server.
var limiters = map[string]*limiter{
	"api.scryfall.com": {gap: scryfallGap},
}

func limiterFor(host string) *limiter { return limiters[host] }

// limiter spaces the requests to one host, and holds them all back for a
// while once the host has said to.
type limiter struct {
	mu    sync.Mutex
	gap   time.Duration
	next  time.Time // the earliest the next request may go
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

// wait holds a request until its turn, or refuses it while the host is
// cooling off — a request sent then only lengthens the block.
func (l *limiter) wait() error {
	l.mu.Lock()
	now := time.Now()
	if now.Before(l.until) {
		until := l.until
		l.mu.Unlock()
		return RateLimited{Until: until}
	}
	at := l.next
	if at.Before(now) {
		at = now
	}
	l.next = at.Add(l.gap)
	l.mu.Unlock()
	time.Sleep(time.Until(at))
	return nil
}

// backOff records a 429: nothing more goes to the host until it said, or
// for a minute.
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
