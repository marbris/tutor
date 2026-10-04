// Package rulings is every card's rulings, from Scryfall's bulk file, kept
// on disk and read a card at a time.
//
// Scryfall gives each card its own rulings endpoint, so showing them used to
// mean a request per card, after the cursor had settled. The bulk file has
// all of them, about 80,000, 5 MB to download and 26 MB unpacked. It is
// downloaded once a week and written back grouped by card, one line each,
// and only where each card's line starts is kept in memory: a card's
// rulings are a read of one line, done as the cursor lands.
package rulings

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"ttr/internal/fetch"
	"ttr/internal/mtg"
	"ttr/internal/paths"
)

// BulkURL is where Scryfall says the current rulings file is. A variable,
// for the tests.
var BulkURL = "https://api.scryfall.com/bulk-data/rulings"

// maxAge is how long the file is trusted. Rulings change a few times a year.
const maxAge = 7 * 24 * time.Hour

// file is the rulings grouped by card: an oracle id, a tab, and the card's
// rulings as JSON, one card a line.
func file() string { return filepath.Join(paths.Cache(), "rulings", "bulk.tsv") }

// Store is the rulings on disk, and where each card's line is.
type Store struct {
	path string
	at   map[string]span
}

type span struct{ off, n int64 }

// Of is a card's rulings. ok is false only when the store can't say; a card
// the file doesn't list has none, since the file lists every card that has.
func (s *Store) Of(oracleID string) (rs []mtg.Ruling, ok bool) {
	if s == nil || oracleID == "" {
		return nil, false
	}
	sp, has := s.at[oracleID]
	if !has {
		return []mtg.Ruling{}, true
	}
	f, err := os.Open(s.path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	buf := make([]byte, sp.n)
	if _, err := f.ReadAt(buf, sp.off); err != nil {
		return nil, false
	}
	if i := bytes.IndexByte(buf, '\t'); i >= 0 {
		buf = buf[i+1:]
	}
	if json.Unmarshal(buf, &rs) != nil {
		return nil, false
	}
	return rs, true
}

var current *Store

// Current is the rulings in use: nil until they're loaded.
func Current() *Store { return current }

// SetCurrent puts a store in use.
func SetCurrent(s *Store) { current = s }

// Load is the rulings on disk, of any age, so they're in use from the
// start; stale says they're over a week old and Refresh should download
// them again. With none on disk, they are downloaded now.
func Load() (s *Store, stale bool, err error) {
	if info, err := os.Stat(file()); err == nil {
		if s, err := index(file()); err == nil {
			return s, time.Since(info.ModTime()) > maxAge, nil
		}
	}
	s, err = Refresh()
	return s, false, err
}

// Kept is the rulings on disk without downloading: nil when there are none.
// For a one-off command.
func Kept() *Store {
	s, err := index(file())
	if err != nil {
		return nil
	}
	return s
}

// Refresh downloads the rulings and writes them grouped by card. A failure
// leaves the file on disk as it was.
func Refresh() (*Store, error) {
	body, err := fetch.Get(BulkURL)
	if err != nil {
		return nil, err
	}
	var meta struct {
		JSONL string `json:"jsonl_download_uri"`
	}
	if err := json.Unmarshal(body, &meta); err != nil || meta.JSONL == "" {
		return nil, fmt.Errorf("the rulings bulk entry has no download link")
	}
	gz, err := fetch.GetFile(meta.JSONL)
	if err != nil {
		return nil, err
	}
	byCard, err := read(gz)
	if err != nil {
		return nil, err
	}
	if err := write(file(), byCard); err != nil {
		return nil, err
	}
	return index(file())
}

// Make writes rulings, by oracle id, to a file at path and indexes it: a
// store without the download, for tests elsewhere.
func Make(path string, byCard map[string][]mtg.Ruling) (*Store, error) {
	if err := write(path, byCard); err != nil {
		return nil, err
	}
	return index(path)
}

// read turns the bulk file round: each card's rulings, by oracle id, in the
// order Scryfall published them.
func read(gz []byte) (map[string][]mtg.Ruling, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	type raw struct {
		OracleID string `json:"oracle_id"`
		Source   string `json:"source"`
		Comment  string `json:"comment"`
	}
	out := map[string][]mtg.Ruling{}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r raw
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		if r.OracleID != "" {
			out[r.OracleID] = append(out[r.OracleID], mtg.Ruling{Source: r.Source, Comment: r.Comment})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the rulings file has no rulings")
	}
	return out, nil
}

// write keeps the grouped rulings, aside and then moved into place, so a
// run that dies halfway leaves the old file.
func write(path string, byCard map[string][]mtg.Ruling) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	ids := make([]string, 0, len(byCard))
	for id := range byCard {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b bytes.Buffer
	for _, id := range ids {
		js, err := json.Marshal(byCard[id])
		if err != nil {
			return err
		}
		b.WriteString(id)
		b.WriteByte('\t')
		b.Write(js)
		b.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// index reads where each card's line starts: the ids only, not the rulings.
// One buffer, reused line after line, so reading 20 MB leaves nothing
// behind but the index.
func index(path string) (*Store, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := &Store{path: path, at: map[string]span{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<16), 16<<20)
	var off int64
	for sc.Scan() {
		line := sc.Bytes()
		n := int64(len(line)) + 1 // and its newline
		if i := bytes.IndexByte(line, '\t'); i > 0 {
			s.at[string(line[:i])] = span{off: off, n: n}
		}
		off += n
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(s.at) == 0 {
		return nil, fmt.Errorf("no rulings in %s", path)
	}
	return s, nil
}
