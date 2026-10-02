// Package scryfall is the card database: searching it, and looking cards up
// by identity.
//
// Everything here is a plain function that either answers or fails. The
// Bubbletea commands that call these on a background thread live with the UI,
// which is the only part of the program that has a main thread to stay off.
package scryfall

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"ttr/internal/diskcache"
	"ttr/internal/fetch"
	"ttr/internal/mtg"
)

// SortOptions is the order a query can ask Scryfall for. This decides which
// cards come back when a search is larger than one page, which is a
// different thing from the order they are then shown in.
var SortOptions = []string{
	"name",
	"released",
	"set",
	"rarity",
	"color",
	"usd",
	"cmc",
	"power",
	"toughness",
	"edhrec",
	"artist",
	"review",
}

// DirOptions is which way a query asks Scryfall to run the sort. auto leaves
// it to Scryfall, which picks the natural direction for each order — newest
// first for released, most played first for edhrec.
var DirOptions = []string{
	"auto",
	"asc",
	"desc",
}

// SearchURL is the endpoint, as a variable so a test can point it at a
// server it controls. Paging is the part of this worth testing and it can't
// be exercised against the real Scryfall without asking for a lot of cards.
var SearchURL = "https://api.scryfall.com/cards/search"

// Search runs a query, following pages until it has limit cards or Scryfall
// runs out. It returns the cards, and how many the query matched in total —
// which is usually more than were fetched.
//
// A query that matches nothing is not an error: no results is an answer.
func Search(query, sort, dir string, limit int) ([]mtg.Card, int, error) {
	var all []mtg.Card

	for page := 1; ; page++ {
		if page > 1 {
			time.Sleep(PageDelay)
		}
		u := fmt.Sprintf("%s?q=%s&order=%s&dir=%s&page=%d",
			SearchURL, url.QueryEscape(query), url.QueryEscape(sort), url.QueryEscape(dir), page,
		)
		body, err := fetch.Get(u)
		if err != nil {
			if _, ok := err.(fetch.NotFound); ok {
				return []mtg.Card{}, 0, nil
			}
			// Pages already in hand beat an error on a later one. The
			// total is what we actually have, not what the query claimed:
			// the number Scryfall reported belongs to a result set we
			// failed to finish fetching.
			if len(all) > 0 {
				all = truncate(all, limit)
				return all, len(all), nil
			}
			return nil, 0, err
		}

		var sr mtg.SearchResponse
		if err := json.Unmarshal(body, &sr); err != nil {
			return nil, 0, err
		}
		all = append(all, sr.Data...)

		if len(all) >= limit || !sr.HasMore {
			return truncate(all, limit), sr.TotalCards, nil
		}
	}
}

func truncate(cards []mtg.Card, limit int) []mtg.Card {
	if len(cards) > limit {
		return cards[:limit]
	}
	return cards
}

// rulingsMaxAge is how long a card's rulings are trusted. New rulings come
// with a set's release, a few times a year; a week is plenty fresh.
const rulingsMaxAge = 7 * 24 * time.Hour

// Rulings fetches a card's rulings, returning an empty (non-nil) slice when
// the card simply has none. They are kept on disk, so a card you have read
// before has them at once.
func Rulings(uri string) ([]mtg.Ruling, error) {
	if uri == "" {
		return []mtg.Ruling{}, nil
	}
	rel := filepath.Join("rulings", diskcache.Key(uri))
	var kept []mtg.Ruling
	fresh, have := diskcache.Load(rel, rulingsMaxAge, &kept)
	if fresh && kept != nil {
		return kept, nil
	}
	got, err := fetchRulings(uri)
	if err != nil {
		if have && kept != nil {
			return kept, nil
		}
		return nil, err
	}
	diskcache.Save(rel, got)
	return got, nil
}

func fetchRulings(uri string) ([]mtg.Ruling, error) {
	body, err := fetch.Get(uri)
	if err != nil {
		return nil, err
	}
	var rr mtg.RulingsResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, err
	}
	if rr.Data == nil {
		rr.Data = []mtg.Ruling{}
	}
	return rr.Data, nil
}

// CollectionURL is a variable so a test can point it somewhere unreachable.
// Simulating a failure by setting HTTP_PROXY doesn't work: Go resolves the
// proxy once per process, so it leaks into every request made afterwards.
var CollectionURL = "https://api.scryfall.com/cards/collection"

const (
	// Scryfall takes up to 75 identifiers per collection request.
	collectionChunk = 75

	// Scryfall asks for a pause between requests; a Commander deck is two.
	// Exported because every other paged fetch owes the same courtesy.
	PageDelay = 100 * time.Millisecond
)

// Identifiers resolves Scryfall collection identifiers — {"id": …},
// {"name": …} or {"set": …, "collector_number": …} — 75 at a time. The
// response is unordered, so both the cards and the identifiers Scryfall
// couldn't match come back for the caller to arrange.
func Identifiers(idents []map[string]string) ([]mtg.Card, []map[string]string, error) {
	var found []mtg.Card
	var missing []map[string]string

	for start := 0; start < len(idents); start += collectionChunk {
		end := start + collectionChunk
		if end > len(idents) {
			end = len(idents)
		}

		var req struct {
			Identifiers []map[string]string `json:"identifiers"`
		}
		req.Identifiers = idents[start:end]
		payload, err := json.Marshal(req)
		if err != nil {
			return nil, nil, err
		}

		if start > 0 {
			time.Sleep(PageDelay)
		}
		body, err := fetch.Post(CollectionURL, payload)
		if err != nil {
			return nil, nil, err
		}

		var cr struct {
			Data     []mtg.Card          `json:"data"`
			NotFound []map[string]string `json:"not_found"`
		}
		if err := json.Unmarshal(body, &cr); err != nil {
			return nil, nil, err
		}
		found = append(found, cr.Data...)
		missing = append(missing, cr.NotFound...)
	}
	return found, missing, nil
}

// Collection looks up cards by Scryfall id, keyed by id for the caller to
// arrange.
func Collection(ids []string) (map[string]mtg.Card, error) {
	idents := make([]map[string]string, len(ids))
	for i, id := range ids {
		idents[i] = map[string]string{"id": id}
	}
	cards, _, err := Identifiers(idents)
	if err != nil {
		return nil, err
	}
	out := make(map[string]mtg.Card, len(cards))
	for _, c := range cards {
		out[c.ID] = c
	}
	return out, nil
}
