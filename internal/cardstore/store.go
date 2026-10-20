// Package cardstore is every card Scryfall knows, in every English printing,
// kept on disk and searched in memory.
//
// Scryfall's default_cards bulk file is downloaded once a week: about 80 MB,
// some 120,000 printings, 650 MB of JSON once unpacked. It is read as it
// arrives and written back one card a line, the card's rules text once and
// each printing's own few fields after it — about 80 MB. Only the Facts
// a search needs are held in memory, one per card, with where its line is;
// a card is read from disk in full when a list needs it.
//
// Tokens, emblems and art cards are left out, and so are the printings that
// exist only on MTGO or Arena, unless they are all a card has.
package cardstore

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"ttr/internal/mtg"
	"ttr/internal/paths"
)

// maxAge is how long the store is trusted. New sets come out every few
// weeks, and the prices only need to be right to the month.
const maxAge = 7 * 24 * time.Hour

// Dir is where the store's files are, in the cache directory.
func Dir() string { return filepath.Join(paths.Cache(), "cardstore") }

func cardsFile() string { return filepath.Join(Dir(), "cards.jsonl") }
func indexFile() string { return filepath.Join(Dir(), "index.gob") }

// format is the version of the files: a store written in another is made
// again from a fresh download.
const format = 1

// Store is the cards on disk, and their Facts in memory.
type Store struct {
	path string
	// Built is when the cards were downloaded, which is how old the prices
	// are.
	Built time.Time
	facts []Facts
	at    []span
	// byName is every card by its lowercased name, and a double-faced or
	// split card by its front face's too.
	byName map[string]int
}

// span is where a card's line is in the cards file.
type span struct {
	Off int64
	N   int32
	// Default is which of its printings a search shows.
	Default int16
}

// Len is how many cards there are.
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	return len(s.facts)
}

// Facts is card i's facts.
func (s *Store) Facts(i int) *Facts { return &s.facts[i] }

// Find is the card with a name, matched as a deck line is: whole, any case,
// or by its front face.
func (s *Store) Find(name string) (int, bool) {
	if s == nil {
		return 0, false
	}
	i, ok := s.byName[strings.ToLower(strings.TrimSpace(name))]
	return i, ok
}

// FindOracle is the card with an oracle id.
func (s *Store) FindOracle(oracleID string) (int, bool) {
	if s == nil || oracleID == "" {
		return 0, false
	}
	if i, ok := s.byName[oracleKey(oracleID)]; ok {
		return i, true
	}
	return 0, false
}

// oracleKey files an oracle id in byName, where no card name can collide
// with it.
func oracleKey(id string) string { return "\x00" + id }

// Card is card i as its most normal printing, read from disk.
func (s *Store) Card(i int) (mtg.Card, error) {
	l, err := s.read(i)
	if err != nil {
		return mtg.Card{}, err
	}
	return l.card(int(s.at[i].Default), s.Built), nil
}

// Printings is every printing of card i, newest first, and which of them a
// search shows.
func (s *Store) Printings(i int) ([]mtg.Card, int, error) {
	l, err := s.read(i)
	if err != nil {
		return nil, 0, err
	}
	out := make([]mtg.Card, len(l.P))
	for p := range l.P {
		out[p] = l.card(p, s.Built)
	}
	return out, int(s.at[i].Default), nil
}

// Printing is card i in the printing with a set code and collector number.
func (s *Store) Printing(i int, set, number string) (mtg.Card, bool) {
	l, err := s.read(i)
	if err != nil {
		return mtg.Card{}, false
	}
	for p, pr := range l.P {
		if strings.EqualFold(pr.Set, set) && strings.EqualFold(pr.Number, number) {
			return l.card(p, s.Built), true
		}
	}
	return mtg.Card{}, false
}

func (s *Store) read(i int) (line, error) {
	var l line
	if s == nil || i < 0 || i >= len(s.at) {
		return l, fmt.Errorf("no such card")
	}
	f, err := os.Open(s.path)
	if err != nil {
		return l, err
	}
	defer f.Close()
	buf := make([]byte, s.at[i].N)
	if _, err := f.ReadAt(buf, s.at[i].Off); err != nil {
		return l, err
	}
	err = json.Unmarshal(buf, &l)
	return l, err
}

// ── In use ──────────────────────────────────────────────────────

var (
	mu      sync.RWMutex
	current *Store
)

// Current is the store in use: nil until one is loaded, or with the
// download turned off.
func Current() *Store {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// SetCurrent puts a store in use.
func SetCurrent(s *Store) {
	mu.Lock()
	current = s
	mu.Unlock()
}

// Load is the store on disk, of any age, so it's in use from the start;
// stale says it's over a week old and Refresh should download it again.
// With none on disk, or one in an older format, it is nil, and stale is
// true.
func Load() (s *Store, stale bool) {
	s, err := open()
	if err != nil {
		return nil, true
	}
	return s, time.Since(s.Built) > maxAge
}

// ── The index ───────────────────────────────────────────────────

// index is what index.gob holds: everything the store keeps in memory.
type index struct {
	Format int
	Built  time.Time
	// Size is the cards file's, which the index belongs to.
	Size  int64
	Facts []Facts
	At    []span
}

func open() (*Store, error) {
	body, err := os.ReadFile(indexFile())
	if err != nil {
		return nil, err
	}
	var ix index
	if err := gob.NewDecoder(bytes.NewReader(body)).Decode(&ix); err != nil {
		return nil, err
	}
	if ix.Format != format || len(ix.Facts) != len(ix.At) {
		return nil, fmt.Errorf("the card store is from another version of Tutor")
	}
	if info, err := os.Stat(cardsFile()); err != nil || info.Size() != ix.Size {
		return nil, fmt.Errorf("the card store's files don't belong together")
	}
	s := newStore(cardsFile(), ix)
	// Decoding the index made as much garbage again as it kept; give the
	// room back now rather than whenever the runtime gets round to it.
	body = nil
	debug.FreeOSMemory()
	return s, nil
}

func newStore(path string, ix index) *Store {
	s := &Store{path: path, Built: ix.Built, facts: ix.Facts, at: ix.At,
		byName: make(map[string]int, 2*len(ix.Facts))}
	for i := range s.facts {
		f := &s.facts[i]
		name := strings.ToLower(f.Name)
		s.byName[name] = i
		if front, _, ok := strings.Cut(name, " // "); ok {
			if _, taken := s.byName[front]; !taken {
				s.byName[front] = i
			}
		}
		s.byName[oracleKey(f.OracleID)] = i
	}
	return s
}
