// Package cache is what's kept in the cache directory, by kind: how much
// room each kind takes, and clearing it. Everything in the cache can be
// fetched again, so all of it is safe to clear; the only cost is the next
// fetch. Both ttr cache and the settings panel use it.
package cache

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"ttr/internal/paths"
)

// Kind is one kind of thing the cache holds, and how to recognise its files
// by their path inside the cache directory.
type Kind struct {
	Name  string // what `ttr cache clear` calls it
	What  string // what the listing calls it
	match func(rel string) bool
	// Refetch is what clearing it costs.
	Refetch string
}

var Kinds = []Kind{
	{"cards", "card data for your decks", func(r string) bool { return r == "cards.json" || r == "cards.fetched.json" },
		"decks look their cards up again on opening"},
	{"pictures", "card pictures (gx)", func(r string) bool { return strings.HasPrefix(r, "images"+string(filepath.Separator)) },
		"gx downloads each picture again"},
	{"printings", "lists of each card's printings (gx)", func(r string) bool { return strings.HasPrefix(r, "printings"+string(filepath.Separator)) },
		"gx asks Scryfall for each card's printings again"},
	{"catalogs", "Scryfall's lists of keywords and types", func(r string) bool { return r == "catalogs.json" },
		"the lists are fetched again, a dozen small requests"},
	{"tagger", "Scryfall Tagger's tags", func(r string) bool { return strings.HasPrefix(r, "tagger"+string(filepath.Separator)) },
		"the tags download again, about 6 MB"},
	{"rulings", "card rulings", func(r string) bool { return strings.HasPrefix(r, "rulings"+string(filepath.Separator)) },
		"the rulings file downloads again, about 5 MB"},
	{"texts", "printed card texts (gv)", func(r string) bool { return strings.HasPrefix(r, "originals"+string(filepath.Separator)) },
		"gv downloads each set's text again"},
	{"rules", "comprehensive rules", func(r string) bool { return strings.HasPrefix(r, "comprules") },
		"the rules download again, and gv in the rules has no older release to compare"},
	{"decks", "followed Moxfield decks", func(r string) bool { return r == "userdecks.json" },
		"the decks of people you follow are fetched again"},
}

// Other is the name for files no kind claims.
const Other = "other"

// KindOf is the kind a cached file belongs to, or Other.
func KindOf(rel string) string {
	for _, k := range Kinds {
		if k.match(rel) {
			return k.Name
		}
	}
	return Other
}

// IsKind reports whether name is one of the kinds.
func IsKind(name string) bool {
	for _, k := range Kinds {
		if k.Name == name {
			return true
		}
	}
	return false
}

// Use is how much room a kind takes.
type Use struct {
	Size  int64
	Files int
}

// file is one file in the cache.
type file struct {
	path, rel string
	size      int64
}

// files is every file in the cache, with its size.
func files() []file {
	root := paths.Cache()
	var out []file
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, file{path, rel, info.Size()})
		return nil
	})
	return out
}

// Usage is the room each kind takes, Other among them, by name.
func Usage() map[string]Use {
	out := map[string]Use{}
	for _, f := range files() {
		k := KindOf(f.rel)
		u := out[k]
		u.Size += f.size
		u.Files++
		out[k] = u
	}
	return out
}

// Clear deletes the cache's files, all of them ("") or one kind's, and the
// directories that leaves empty. It reports how many files went and how
// much room that freed; a file that won't go is skipped, and the first such
// error is returned.
func Clear(kind string) (n int, freed int64, err error) {
	for _, f := range files() {
		if kind != "" && KindOf(f.rel) != kind {
			continue
		}
		if e := os.Remove(f.path); e != nil {
			if err == nil {
				err = e
			}
			continue
		}
		freed += f.size
		n++
	}
	removeEmptyDirs(paths.Cache())
	return n, freed, err
}

// removeEmptyDirs takes out the directories under root that clearing left
// empty, deepest first, keeping root itself.
func removeEmptyDirs(root string) {
	var dirs []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Remove(dirs[i]) // fails, harmlessly, on one that isn't empty
	}
}

// Size is a size the way you'd say it.
func Size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
