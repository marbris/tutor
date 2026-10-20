package cardstore

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"ttr/internal/fetch"
	"ttr/internal/mtg"
	"ttr/internal/prints"
)

// BulkURL is where Scryfall says the current default_cards file is. A
// variable, for the tests.
var BulkURL = "https://api.scryfall.com/bulk-data/default-cards"

// left out are the layouts that aren't cards anyone puts in a deck.
var leftOut = map[string]bool{"token": true, "double_faced_token": true, "emblem": true, "art_series": true}

var got, total atomic.Int64

// Downloading is how far a download has got, in bytes, while one runs.
func Downloading() (done, of int64, ok bool) {
	of = total.Load()
	return got.Load(), of, of > 0
}

// Refresh downloads the cards and writes the store again. A failure leaves
// the store on disk as it was.
func Refresh() (*Store, error) {
	body, err := fetch.Get(BulkURL)
	if err != nil {
		return nil, err
	}
	var meta struct {
		JSONL string `json:"jsonl_download_uri"`
		Size  int64  `json:"compressed_size"`
	}
	if err := json.Unmarshal(body, &meta); err != nil || meta.JSONL == "" {
		return nil, fmt.Errorf("the default_cards bulk entry has no download link")
	}
	rc, n, err := fetch.Open(meta.JSONL)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	if n <= 0 {
		n = meta.Size
	}
	got.Store(0)
	total.Store(max(n, 1))
	defer total.Store(0)
	gz, err := gzip.NewReader(counting{rc})
	if err != nil {
		return nil, err
	}
	return Build(gz, Dir(), time.Now())
}

// counting counts what's read into got.
type counting struct{ r io.Reader }

func (c counting) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	got.Add(int64(n))
	return n, err
}

// raw is the part of a Scryfall card a build reads.
type raw struct {
	ID              string            `json:"id"`
	OracleID        string            `json:"oracle_id"`
	Layout          string            `json:"layout"`
	Name            string            `json:"name"`
	Lang            string            `json:"lang"`
	ReleasedAt      string            `json:"released_at"`
	ManaCost        string            `json:"mana_cost"`
	CMC             float64           `json:"cmc"`
	TypeLine        string            `json:"type_line"`
	OracleText      string            `json:"oracle_text"`
	Power           string            `json:"power"`
	Toughness       string            `json:"toughness"`
	Loyalty         string            `json:"loyalty"`
	Colors          []string          `json:"colors"`
	ColorIdentity   []string          `json:"color_identity"`
	Keywords        []string          `json:"keywords"`
	Legalities      map[string]string `json:"legalities"`
	Digital         bool              `json:"digital"`
	Rarity          string            `json:"rarity"`
	IllustrationID  string            `json:"illustration_id"`
	BorderColor     string            `json:"border_color"`
	Frame           string            `json:"frame"`
	FrameEffects    []string          `json:"frame_effects"`
	FullArt         bool              `json:"full_art"`
	Promo           bool              `json:"promo"`
	Variation       bool              `json:"variation"`
	Set             string            `json:"set"`
	SetName         string            `json:"set_name"`
	SetType         string            `json:"set_type"`
	CollectorNumber string            `json:"collector_number"`
	EDHRECRank      int               `json:"edhrec_rank"`
	ImageURIs       struct {
		BorderCrop string `json:"border_crop"`
	} `json:"image_uris"`
	Prices struct {
		USD     string `json:"usd"`
		USDFoil string `json:"usd_foil"`
	} `json:"prices"`
	CardFaces []struct {
		OracleID       string   `json:"oracle_id"`
		Name           string   `json:"name"`
		ManaCost       string   `json:"mana_cost"`
		TypeLine       string   `json:"type_line"`
		OracleText     string   `json:"oracle_text"`
		Colors         []string `json:"colors"`
		Power          string   `json:"power"`
		Toughness      string   `json:"toughness"`
		Loyalty        string   `json:"loyalty"`
		IllustrationID string   `json:"illustration_id"`
		ImageURIs      struct {
			BorderCrop string `json:"border_crop"`
		} `json:"image_uris"`
	} `json:"card_faces"`
}

// Build reads Scryfall cards, one JSON object a line, and writes the store
// into dir.
func Build(r io.Reader, dir string, built time.Time) (*Store, error) {
	// Each card decoded is garbage at once; collecting it sooner keeps the
	// build's peak down, for the few seconds it runs.
	defer debug.SetGCPercent(debug.SetGCPercent(20))
	groups := map[string]*line{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		b := bytes.TrimSpace(sc.Bytes())
		b = bytes.TrimSuffix(b, []byte(","))
		if len(b) == 0 || b[0] != '{' {
			continue // the brackets of a plain JSON array
		}
		var c raw
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("reading the cards: %w", err)
		}
		if leftOut[c.Layout] {
			continue
		}
		id := c.OracleID
		if id == "" && len(c.CardFaces) > 0 {
			id = c.CardFaces[0].OracleID
		}
		if id == "" {
			continue
		}
		g := groups[id]
		if g == nil {
			g = &line{C: oracleOf(id, c)}
			groups[id] = g
		}
		g.P = append(g.P, printingOf(c))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading the cards: %w", err)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("the card download held no cards")
	}

	lines := make([]*line, 0, len(groups))
	for _, g := range groups {
		g.P = paperOnly(g.P)
		sort.SliceStable(g.P, func(a, b int) bool { return g.P[a].Released > g.P[b].Released })
		lines = append(lines, g)
	}
	groups = nil
	sort.Slice(lines, func(a, b int) bool {
		if lines[a].C.Name != lines[b].C.Name {
			return lines[a].C.Name < lines[b].C.Name
		}
		return lines[a].C.ID < lines[b].C.ID
	})

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	cardsPath := filepath.Join(dir, "cards.jsonl")
	tmp, err := os.CreateTemp(dir, "cards-*.tmp")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	w := bufio.NewWriterSize(tmp, 1<<20)
	ix := index{Format: format, Built: built,
		Facts: make([]Facts, 0, len(lines)), At: make([]span, 0, len(lines))}
	today := built.Format("2006-01-02")
	var off int64
	for _, l := range lines {
		body, err := json.Marshal(l)
		if err != nil {
			tmp.Close()
			return nil, err
		}
		def := defaultPrinting(*l, today)
		ix.At = append(ix.At, span{Off: off, N: int32(len(body)), Default: int16(def)})
		ix.Facts = append(ix.Facts, factsOf(*l, def, built))
		w.Write(body)
		w.WriteByte('\n')
		off += int64(len(body)) + 1
	}
	lines = nil
	if err := w.Flush(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	os.Chmod(tmp.Name(), 0644)
	ix.Size = off

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(ix); err != nil {
		return nil, err
	}
	indexPath := filepath.Join(dir, "index.gob")
	if err := os.Rename(tmp.Name(), cardsPath); err != nil {
		return nil, err
	}
	if err := writeAtomic(indexPath, buf.Bytes()); err != nil {
		return nil, err
	}
	s := newStore(cardsPath, ix)
	// Building held every printing at once; give the room back now rather
	// than whenever the runtime gets round to it.
	debug.FreeOSMemory()
	return s, nil
}

func writeAtomic(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	os.Chmod(tmp.Name(), 0644)
	return os.Rename(tmp.Name(), path)
}

func oracleOf(id string, c raw) oracle {
	o := oracle{ID: id, Name: c.Name, ManaCost: c.ManaCost, TypeLine: c.TypeLine, Text: c.OracleText,
		Colors: c.Colors, Identity: c.ColorIdentity, Power: c.Power, Toughness: c.Toughness,
		Loyalty: c.Loyalty, CMC: c.CMC, EDHREC: c.EDHRECRank, Keywords: c.Keywords}
	for format, l := range c.Legalities {
		if l != "not_legal" {
			if o.Legalities == nil {
				o.Legalities = map[string]string{}
			}
			o.Legalities[format] = l
		}
	}
	for _, f := range c.CardFaces {
		o.Faces = append(o.Faces, oracleFace{Name: f.Name, ManaCost: f.ManaCost, TypeLine: f.TypeLine,
			Text: f.OracleText, Colors: f.Colors, Power: f.Power, Toughness: f.Toughness, Loyalty: f.Loyalty})
	}
	return o
}

func printingOf(c raw) printing {
	p := printing{ID: c.ID, Set: c.Set, SetName: c.SetName, SetType: c.SetType, Number: c.CollectorNumber,
		Released: c.ReleasedAt, Rarity: c.Rarity, Digital: c.Digital, Border: c.BorderColor,
		Frame: c.Frame, FrameEffects: c.FrameEffects, Promo: c.Promo, FullArt: c.FullArt,
		Variation: c.Variation, Art: c.IllustrationID, Crop: c.ImageURIs.BorderCrop,
		USD: c.Prices.USD, USDFoil: c.Prices.USDFoil}
	if c.Lang != "en" {
		p.Lang = c.Lang
	}
	// A card printed on both sides has a picture per face, and none of its
	// own.
	if p.Crop == "" {
		for _, f := range c.CardFaces {
			p.Faces = append(p.Faces, printingFace{Crop: f.ImageURIs.BorderCrop, Art: f.IllustrationID})
		}
	}
	return p
}

// paperOnly drops the printings that exist only on a screen, unless that
// is all there is: an Alchemy card is still a card.
func paperOnly(all []printing) []printing {
	var paper []printing
	for _, p := range all {
		if !p.Digital {
			paper = append(paper, p)
		}
	}
	if len(paper) == 0 {
		return all
	}
	return paper
}

// defaultPrinting is the printing a search shows: the most normal one, as
// gx picks it.
func defaultPrinting(l line, today string) int {
	cards := make([]mtg.Card, len(l.P))
	for i := range l.P {
		cards[i] = l.card(i, time.Time{})
	}
	if i, ok := prints.PreferIndex(cards, today); ok {
		return i
	}
	return 0
}

func factsOf(l line, def int, built time.Time) Facts {
	f := Of(l.card(def, built), nil)
	seen := map[string]bool{}
	f.Sets = f.Sets[:0]
	for _, p := range l.P {
		set := strings.ToLower(p.Set)
		if !seen[set] {
			seen[set] = true
			f.Sets = append(f.Sets, set)
		}
		f.Rarities |= RarityOf(p.Rarity)
	}
	return f
}
