// Package catalog is Scryfall's catalogs: the lists of every keyword and
// every type Scryfall knows, kept on disk.
//
// The rules text names most keywords, but not their variants (Forestwalk is
// landwalk, Plainscycling is cycling) nor the newest and the digital ones,
// and it lists subtypes in long sentences rather than by type. Scryfall
// keeps each as a plain list, a few kilobytes apiece, updated with each set.
// They are asked for once a week, a dozen small requests, and kept as one
// file.
package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ttr/internal/diskcache"
	"ttr/internal/fetch"
)

// The catalogs Tutor uses, by Scryfall's names for them.
const (
	KeywordAbilities = "keyword-abilities"
	KeywordActions   = "keyword-actions"
	AbilityWords     = "ability-words"
	CardTypes        = "card-types"
	Supertypes       = "supertypes"
)

// SubtypeCatalogs is each card type's list of subtypes. The type decides
// where a subtype is counted: Equipment is an artifact type, Golem a
// creature type, though both can sit on one type line.
var SubtypeCatalogs = map[string]string{
	"Creature":     "creature-types",
	"Planeswalker": "planeswalker-types",
	"Land":         "land-types",
	"Artifact":     "artifact-types",
	"Enchantment":  "enchantment-types",
	"Battle":       "battle-types",
	// Instants and sorceries share their subtypes (Adventure, Lesson, …).
	"Instant": "spell-types",
	"Sorcery": "spell-types",
}

// names is every catalog fetched.
func names() []string {
	out := []string{KeywordAbilities, KeywordActions, AbilityWords, CardTypes, Supertypes}
	seen := map[string]bool{}
	for _, c := range SubtypeCatalogs {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// Data is the catalogs, by name.
type Data struct {
	Lists map[string][]string `json:"lists"`

	// subtypeOf is each subtype's card types, lowercased subtype first.
	subtypeOf map[string][]string
}

// List is one catalog: nil when it isn't in.
func (d *Data) List(name string) []string {
	if d == nil {
		return nil
	}
	return d.Lists[name]
}

// SubtypeOf is the card types a subtype belongs to, and whether the
// catalogs know it at all.
func (d *Data) SubtypeOf(subtype string) ([]string, bool) {
	if d == nil {
		return nil, false
	}
	if d.subtypeOf == nil {
		d.subtypeOf = map[string][]string{}
		for t, c := range SubtypeCatalogs {
			for _, s := range d.Lists[c] {
				k := strings.ToLower(s)
				d.subtypeOf[k] = append(d.subtypeOf[k], t)
			}
		}
	}
	ts, ok := d.subtypeOf[strings.ToLower(subtype)]
	return ts, ok
}

// current is the catalogs in use, once they are in.
var current *Data

// Current is the catalogs in use: nil until they are loaded. Everything that
// uses them asks at the moment it needs them, and does without until then.
func Current() *Data { return current }

// SetCurrent puts a set of catalogs in use.
func SetCurrent(d *Data) { current = d }

// BaseURL is where the catalogs are. A variable, for the tests.
var BaseURL = "https://api.scryfall.com/catalog/"

// maxAge is how long the catalogs are trusted. They change with each set.
const maxAge = 7 * 24 * time.Hour

const cacheFile = "catalogs.json"

// Load is the catalogs kept on disk, of any age, so they are in use from
// the start; stale says they are over a week old and Refresh should fetch
// them again. With nothing kept, they are fetched now.
func Load() (d *Data, stale bool, err error) {
	var kept Data
	if fresh, have := diskcache.Load(cacheFile, maxAge, &kept); have && len(kept.Lists) > 0 {
		return &kept, !fresh, nil
	}
	d, err = Refresh()
	return d, false, err
}

// Kept is the catalogs on disk, of any age, without fetching: nil when
// there are none. For a one-off command, which shouldn't make a dozen
// requests to print one card.
func Kept() *Data {
	var kept Data
	if _, have := diskcache.Load(cacheFile, maxAge, &kept); have && len(kept.Lists) > 0 {
		return &kept
	}
	return nil
}

// Refresh fetches every catalog and keeps them. All or nothing: a failure
// leaves the kept copy as it was.
func Refresh() (*Data, error) {
	d := &Data{Lists: map[string][]string{}}
	for _, name := range names() {
		body, err := fetch.Get(BaseURL + name)
		if err != nil {
			return nil, fmt.Errorf("catalog %s: %w", name, err)
		}
		var c struct {
			Data []string `json:"data"`
		}
		if err := json.Unmarshal(body, &c); err != nil {
			return nil, fmt.Errorf("catalog %s: %w", name, err)
		}
		d.Lists[name] = c.Data
	}
	diskcache.Save(cacheFile, d)
	return d, nil
}
