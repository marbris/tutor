package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"ttr/internal/paths"
)

// ttr cache: what's been downloaded and kept, and how much room it takes.
// Everything in the cache can be fetched again, so all of it is safe to
// clear — the only cost is the next fetch.

const cacheUsage = `Usage:
  ttr cache                 What's in the cache, and how much room it takes
  ttr cache clear [kind]    Empty the cache, or only one kind: %s

Everything in %s
can be downloaded again, so clearing it loses nothing but time: the next
time something is needed, it is fetched.`

// cacheKind is one kind of thing the cache holds, and how to recognise its
// files by their path inside the cache directory.
type cacheKind struct {
	name  string // what `ttr cache clear` calls it
	what  string // what the listing calls it
	match func(rel string) bool
	// refetch is what clearing it costs.
	refetch string
}

var cacheKinds = []cacheKind{
	{"cards", "card data for your decks", func(r string) bool { return r == "cards.json" || r == "cards.fetched.json" },
		"decks look their cards up again on opening"},
	{"pictures", "card pictures (gx)", func(r string) bool { return strings.HasPrefix(r, "images"+string(filepath.Separator)) },
		"gx downloads each picture again"},
	{"printings", "lists of each card's printings (gx)", func(r string) bool { return strings.HasPrefix(r, "printings"+string(filepath.Separator)) },
		"gx asks Scryfall for each card's printings again"},
	{"tagger", "Scryfall Tagger's tags", func(r string) bool { return strings.HasPrefix(r, "tagger"+string(filepath.Separator)) },
		"the tags download again, about 6 MB"},
	{"rulings", "card rulings", func(r string) bool { return strings.HasPrefix(r, "rulings"+string(filepath.Separator)) },
		"each card's rulings are fetched again"},
	{"texts", "printed card texts (gv)", func(r string) bool { return strings.HasPrefix(r, "originals"+string(filepath.Separator)) },
		"gv downloads each set's text again"},
	{"rules", "comprehensive rules", func(r string) bool { return strings.HasPrefix(r, "comprules") },
		"the rules download again, and gv in the rules has no older release to compare"},
	{"decks", "followed Moxfield decks", func(r string) bool { return r == "userdecks.json" },
		"the decks of people you follow are fetched again"},
}

// kindOf is the kind a cached file belongs to, or "other".
func kindOf(rel string) string {
	for _, k := range cacheKinds {
		if k.match(rel) {
			return k.name
		}
	}
	return "other"
}

func runCache(args []string) {
	names := make([]string, 0, len(cacheKinds))
	for _, k := range cacheKinds {
		names = append(names, k.name)
	}
	usage := fmt.Sprintf(cacheUsage, strings.Join(names, ", "), paths.Cache())

	switch {
	case len(args) == 0:
		listCache()
	case args[0] == "clear" && len(args) <= 2:
		kind := ""
		if len(args) == 2 {
			kind = args[1]
			if !isKind(kind) {
				fmt.Fprintf(os.Stderr, "no kind of cache called %q — %s\n", kind, strings.Join(names, ", "))
				os.Exit(1)
			}
		}
		clearCache(kind)
	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Println(usage)
	default:
		fmt.Println(usage)
		os.Exit(1)
	}
}

func isKind(name string) bool {
	for _, k := range cacheKinds {
		if k.name == name {
			return true
		}
	}
	return false
}

// cachedFile is one file in the cache.
type cachedFile struct {
	path, rel string
	size      int64
}

// cacheFiles is every file in the cache, with its size.
func cacheFiles() []cachedFile {
	root := paths.Cache()
	var out []cachedFile
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, cachedFile{path, rel, info.Size()})
		return nil
	})
	return out
}

func listCache() {
	size := map[string]int64{}
	count := map[string]int{}
	var total int64
	for _, f := range cacheFiles() {
		k := kindOf(f.rel)
		size[k] += f.size
		count[k]++
		total += f.size
	}

	fmt.Println(paths.Cache())
	fmt.Println()
	for _, k := range cacheKinds {
		files := ""
		if count[k.name] > 1 {
			files = fmt.Sprintf("%d files", count[k.name])
		}
		line := fmt.Sprintf("  %-9s %9s  %-26s %s", k.name, humanSize(size[k.name]), k.what, files)
		fmt.Println(strings.TrimRight(line, " "))
	}
	if count["other"] > 0 {
		fmt.Printf("  %-9s %9s  %-26s %d files\n", "other", humanSize(size["other"]), "anything else", count["other"])
	}
	fmt.Printf("\n  %-9s %9s\n", "total", humanSize(total))
	fmt.Println("\nttr cache clear empties it, or ttr cache clear <kind> one kind of it.")
}

// clearCache deletes the cache's files — all of them, or one kind's — and
// the directories they leave empty.
func clearCache(kind string) {
	var freed int64
	n := 0
	for _, f := range cacheFiles() {
		if kind != "" && kindOf(f.rel) != kind {
			continue
		}
		if err := os.Remove(f.path); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			continue
		}
		freed += f.size
		n++
	}
	removeEmptyDirs(paths.Cache())

	what := "the cache"
	if kind != "" {
		what = kind
		for _, k := range cacheKinds {
			if k.name == kind {
				fmt.Printf("cleared %s: %d %s, %s — %s\n", k.what, n, plural("file", n), humanSize(freed), k.refetch)
				return
			}
		}
	}
	fmt.Printf("cleared %s: %d %s, %s\n", what, n, plural("file", n), humanSize(freed))
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

func humanSize(n int64) string {
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

func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
