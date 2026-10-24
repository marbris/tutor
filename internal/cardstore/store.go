// Package cardstore is every card Scryfall knows, in every English printing,
// kept on disk and searched in memory.
//
// Scryfall's default_cards bulk file is downloaded once a week: about 80 MB,
// some 120,000 printings, 650 MB of JSON once unpacked. It is read as it
// arrives and written back a card at a time: a line with the card's rules
// text, then a line with each printing's own few fields — about 55 MB. Only the Facts
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
const format = 2

// Store is the cards on disk, and their Facts in memory.
type Store struct {
	path string
	// stamp is the cards file's size and modification time when the index
	// was read with it. Another ttr's weekly download puts a new file in
	// its place, and this index would read the wrong lines out of it.
	stamp string
	// Built is when the cards were downloaded, which is how old the prices
	// are.
	Built time.Time
	facts []Facts
	at    []span
	// byName is every card by its lowercased name, and a double-faced or
	// split card by its front face's too.
	byName map[string]int
}

// span is where a card's lines are in the cards file: the card's own,
// then a line for each printing.
type span struct {
	Off int64
	N   int32
	// Oracle is the length of the card's own line.
	Oracle int32
	// Default is which of its printings a search shows, and DefOff and DefN
	// where its line is, from Off: a card as its default printing is a read
	// of two short lines, not all of them.
	Default      int16
	DefOff, DefN int32
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
	if s == nil || i < 0 || i >= len(s.at) {
		return mtg.Card{}, fmt.Errorf("no such card")
	}
	sp := s.at[i]
	f, err := s.open()
	if err != nil {
		return mtg.Card{}, err
	}
	defer f.Close()
	own := make([]byte, sp.Oracle)
	def := make([]byte, sp.DefN)
	if _, err := f.ReadAt(own, sp.Off); err != nil {
		return mtg.Card{}, err
	}
	if _, err := f.ReadAt(def, sp.Off+int64(sp.DefOff)); err != nil {
		return mtg.Card{}, err
	}
	l := line{P: make([]printing, 1)}
	if err := json.Unmarshal(own, &l.C); err != nil {
		return mtg.Card{}, err
	}
	if err := json.Unmarshal(def, &l.P[0]); err != nil {
		return mtg.Card{}, err
	}
	return l.card(0, s.Built), nil
}

// Light is card i from its facts alone, without reading the disk: its
// name, type, text and color identity, and whether it's legal in one
// format. Enough to check a deck, for many decks at once.
func (s *Store) Light(i int, format string) mtg.Card {
	f := &s.facts[i]
	c := mtg.Card{OracleID: f.OracleID, Name: f.Name, ManaCost: f.ManaCost, TypeLine: f.TypeLine,
		OracleText: f.Text, CMC: f.CMC, ColorIdentity: f.Identity.List(), Colors: f.Colors.List(),
		Keywords: f.Keywords}
	if bit, ok := FormatBit(format); ok {
		l := "not_legal"
		switch {
		case f.Legal&bit != 0:
			l = "legal"
		case f.Banned&bit != 0:
			l = "banned"
		}
		c.Legalities = legality(format, l)
	}
	return c
}

// legalities are the one-format maps Light hands out, shared: a light card
// is read, never changed, and there are thousands of them at once.
var legalities sync.Map

func legality(format, l string) map[string]string {
	key := format + "\x00" + l
	if m, ok := legalities.Load(key); ok {
		return m.(map[string]string)
	}
	m, _ := legalities.LoadOrStore(key, map[string]string{format: l})
	return m.(map[string]string)
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
	f, err := s.open()
	if err != nil {
		return l, err
	}
	defer f.Close()
	buf := make([]byte, s.at[i].N)
	if _, err := f.ReadAt(buf, s.at[i].Off); err != nil {
		return l, err
	}
	rows := bytes.Split(bytes.TrimSuffix(buf, []byte("\n")), []byte("\n"))
	if err := json.Unmarshal(rows[0], &l.C); err != nil {
		return l, err
	}
	l.P = make([]printing, len(rows)-1)
	for p, row := range rows[1:] {
		if err := json.Unmarshal(row, &l.P[p]); err != nil {
			return l, err
		}
	}
	return l, nil
}

// ── In use ──────────────────────────────────────────────────────

var (
	mu      sync.RWMutex
	current *Store
	// pending is closed when a load from disk said to be on its way
	// (Expect) has put a store in use, or failed to (Settle).
	pending chan struct{}
)

// Current is the store in use: nil until one is loaded, or with the
// download turned off.
func Current() *Store {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// SetCurrent puts a store in use, and settles a load that was expected.
func SetCurrent(s *Store) {
	mu.Lock()
	current = s
	mu.Unlock()
	Settle()
}

// Expect says a store is being loaded from disk, so Ready waits for it:
// at the start, a deck opened from the session shouldn't miss the store
// by the 60 ms it takes to load.
func Expect() {
	mu.Lock()
	if pending == nil {
		pending = make(chan struct{})
	}
	mu.Unlock()
}

// Settle says the expected load is over, however it went.
func Settle() {
	mu.Lock()
	if pending != nil {
		close(pending)
		pending = nil
	}
	mu.Unlock()
}

// Ready is Current, once a load from disk that was expected is over. Off
// the main loop only: it waits.
func Ready() *Store {
	mu.RLock()
	p := pending
	mu.RUnlock()
	if p != nil {
		select {
		case <-p:
		case <-time.After(5 * time.Second):
		}
	}
	return Current()
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

// OnDisk reports whether there is a store to load, without loading it.
func OnDisk() bool {
	_, err := os.Stat(indexFile())
	return err == nil
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

// ErrReplaced is a read from a cards file that is no longer the one the
// store's index belongs to: another ttr downloaded the cards again. Load
// reads the new one.
var ErrReplaced = fmt.Errorf("the card data was downloaded again; reading it in")

// open is the cards file, if it is still the one the index belongs to.
func (s *Store) open() (*os.File, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err != nil || stampOf(info) != s.stamp {
		f.Close()
		return nil, ErrReplaced
	}
	return f, nil
}

// Replaced reports whether the cards file on disk is no longer the one this
// store was read with.
func (s *Store) Replaced() bool {
	if s == nil {
		return false
	}
	info, err := os.Stat(s.path)
	return err != nil || stampOf(info) != s.stamp
}

func stampOf(info os.FileInfo) string {
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
}

func newStore(path string, ix index) *Store {
	s := &Store{path: path, Built: ix.Built, facts: ix.Facts, at: ix.At,
		byName: make(map[string]int, 2*len(ix.Facts))}
	if info, err := os.Stat(path); err == nil && info.Size() == ix.Size {
		s.stamp = stampOf(info)
	}
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
