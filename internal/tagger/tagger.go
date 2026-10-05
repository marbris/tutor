// Package tagger is Scryfall Tagger's oracle tags, kept on disk the other
// way round from how Scryfall publishes them.
//
// Tagger is Scryfall's community project of tagging cards by what they do:
// removal, ramp, card-advantage, and some four and a half thousand more. A
// search can ask for them (otag:removal), but a search answers "which cards
// have this tag", and what the info panel and the statistics want is "which
// tags does this card have". Scryfall publishes every tag with its cards as
// one bulk file, about 6 MB gzipped, so it is downloaded once a week and
// turned round: card → tags, for every card Tagger knows, about 36,000.
// Kept as numbers rather than names, the result is a few megabytes and
// loads in a moment.
//
// The tags form a family tree: removal-artifact and removal-enchantment are
// children of removal, and a tag can have several parents. A parent often
// has no cards of its own — removal's cards are all in its children — so a
// card counts as having a tag when it has the tag or any of its
// descendants.
package tagger

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"ttr/internal/diskcache"
	"ttr/internal/fetch"
)

// Tag is one Tagger tag. Its Label is the tag's slug — spot-removal, not
// "spot removal" — since that is the name otag: takes. Parents and Children
// are indexes into Data.Tags.
type Tag struct {
	Label       string `json:"l"`
	Description string `json:"d,omitempty"`
	Parents     []int  `json:"p,omitempty"`
	Children    []int  `json:"c,omitempty"`
}

// Data is every tag, and every card's own tags by oracle id.
type Data struct {
	Tags  []Tag            `json:"tags"`
	Cards map[string][]int `json:"cards"`

	byLabel map[string]int
	// above is each tag with all its ancestors, worked out when first asked.
	above map[int][]int
	// has is each card's tags with all their ancestors, likewise.
	has map[string]map[int]bool
}

// current is the tags in use, once they are in.
var current *Data

// Current is the tags in use: nil until they have been loaded, or when there
// were none to be had. Everything that shows them asks here, at the moment
// it shows them, so whatever was drawn before they arrived fills in after.
func Current() *Data { return current }

// SetCurrent puts a set of tags in use.
func SetCurrent(d *Data) { current = d }

// BulkURL is where Scryfall says the current bulk file is. A variable, for
// the tests.
var BulkURL = "https://api.scryfall.com/bulk-data/oracle-tags"

// maxAge is how long a download is trusted. Tagger changes daily, but a tag
// more or less on a card is not worth 6 MB a day.
const maxAge = 7 * 24 * time.Hour

// cacheFile is where the turned-round data is kept, in the cache directory.
const cacheFile = "tagger/oracle-tags.json"

// Load is the tags kept on disk, of any age, so they are in use from the
// start; stale says they are over a week old and Refresh should download
// them again. With nothing kept, they are downloaded now.
func Load() (d *Data, stale bool, err error) {
	var kept Data
	if fresh, have := diskcache.Load(cacheFile, maxAge, &kept); have && len(kept.Tags) > 0 {
		kept.index()
		return &kept, !fresh, nil
	}
	d, err = Refresh()
	return d, false, err
}

// Refresh downloads the tags, turns them round and keeps them. A failure
// leaves the kept copy as it was.
func Refresh() (*Data, error) {
	d, err := download()
	if err != nil {
		return nil, err
	}
	diskcache.Save(cacheFile, d)
	return d, nil
}

// download fetches the bulk file Scryfall points at and reads it.
func download() (*Data, error) {
	body, err := fetch.Get(BulkURL)
	if err != nil {
		return nil, err
	}
	var meta struct {
		JSONL string `json:"jsonl_download_uri"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, err
	}
	if meta.JSONL == "" {
		return nil, fmt.Errorf("Scryfall gave no address for the Tagger tags")
	}
	gz, err := fetch.GetFile(meta.JSONL)
	if err != nil {
		return nil, err
	}
	return Read(gz)
}

// Read turns Scryfall's bulk file — gzipped JSON lines, one tag each, with
// the cards that have it — into Data.
func Read(gz []byte) (*Data, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	type raw struct {
		ID       string   `json:"id"`
		Label    string   `json:"label"`
		Slug     string   `json:"slug"`
		Desc     string   `json:"description"`
		Parents  []string `json:"parent_ids"`
		Children []string `json:"child_ids"`
		Taggings []struct {
			OracleID string `json:"oracle_id"`
		} `json:"taggings"`
	}
	var tags []raw
	sc := bufio.NewScanner(zr)
	// A tag with thousands of cards is one long line.
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var t raw
		if err := json.Unmarshal(line, &t); err != nil {
			return nil, err
		}
		if t.Slug != "" {
			t.Label = t.Slug
		}
		tags = append(tags, t)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("the Tagger file has no tags")
	}

	sort.Slice(tags, func(i, j int) bool { return tags[i].Label < tags[j].Label })
	at := map[string]int{}
	for i, t := range tags {
		at[t.ID] = i
	}
	indexes := func(ids []string) []int {
		var out []int
		for _, id := range ids {
			if i, ok := at[id]; ok {
				out = append(out, i)
			}
		}
		return out
	}
	d := &Data{Cards: map[string][]int{}}
	for i, t := range tags {
		d.Tags = append(d.Tags, Tag{
			Label: t.Label, Description: t.Desc,
			Parents: indexes(t.Parents), Children: indexes(t.Children),
		})
		for _, tg := range t.Taggings {
			d.Cards[tg.OracleID] = append(d.Cards[tg.OracleID], i)
		}
	}
	d.index()
	return d, nil
}

func (d *Data) index() {
	d.byLabel = make(map[string]int, len(d.Tags))
	for i, t := range d.Tags {
		d.byLabel[t.Label] = i
	}
	d.above = map[int][]int{}
	d.has = map[string]map[int]bool{}
}

// Of is a card's own tags, by name, in alphabetical order — the ones Tagger
// gave it, not the parents they imply.
func (d *Data) Of(oracleID string) []string {
	if d == nil {
		return nil
	}
	var out []string
	for _, i := range d.Cards[oracleID] {
		out = append(out, d.Tags[i].Label)
	}
	sort.Strings(out)
	return out
}

// Tagged reports whether Tagger has given a card any tag at all.
func (d *Data) Tagged(oracleID string) bool {
	return d != nil && len(d.Cards[oracleID]) > 0
}

// Find is a tag's index by its name.
func (d *Data) Find(label string) (int, bool) {
	if d == nil {
		return 0, false
	}
	i, ok := d.byLabel[label]
	return i, ok
}

// Has reports whether a card has a tag: the tag itself, or any tag under it.
func (d *Data) Has(oracleID string, tag int) bool {
	return d.Closure(oracleID)[tag]
}

// Closure is every tag a card has: its own, and every tag above them.
func (d *Data) Closure(oracleID string) map[int]bool {
	if d == nil {
		return nil
	}
	set, ok := d.has[oracleID]
	if !ok {
		set = map[int]bool{}
		for _, t := range d.Cards[oracleID] {
			for _, a := range d.ancestry(t) {
				set[a] = true
			}
		}
		d.has[oracleID] = set
	}
	return set
}

// ancestry is a tag and every tag above it. The tree is a tree by intent;
// a loop in it is skipped rather than followed.
func (d *Data) ancestry(tag int) []int {
	if got, ok := d.above[tag]; ok {
		return got
	}
	seen := map[int]bool{tag: true}
	out := []int{tag}
	for i := 0; i < len(out); i++ {
		for _, p := range d.Tags[out[i]].Parents {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	d.above[tag] = out
	return out
}

// Roots is the tags with no parent.
func (d *Data) Roots() []int {
	if d == nil {
		return nil
	}
	var out []int
	for i, t := range d.Tags {
		if len(t.Parents) == 0 {
			out = append(out, i)
		}
	}
	return out
}

// Labels is every tag's name that starts with prefix, in order.
func (d *Data) Labels(prefix string) []string {
	if d == nil {
		return nil
	}
	var out []string
	for _, t := range d.Tags {
		if strings.HasPrefix(t.Label, prefix) {
			out = append(out, t.Label)
		}
	}
	return out
}

// With is the oracle id of every card with a tag — the tag itself, or any
// under it, as Has and otag: count — sorted. It reads without filling the
// per-card cache, so it costs one pass over the cards and can be asked once
// for a whole list.
func (d *Data) With(tag int) []string {
	if d == nil || tag < 0 || tag >= len(d.Tags) {
		return nil
	}
	under := map[int]bool{}
	var walk func(int)
	walk = func(t int) {
		if under[t] {
			return
		}
		under[t] = true
		for _, c := range d.Tags[t].Children {
			walk(c)
		}
	}
	walk(tag)
	var out []string
	for id, tags := range d.Cards {
		for _, t := range tags {
			if under[t] {
				out = append(out, id)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}
