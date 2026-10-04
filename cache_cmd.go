package main

import (
	"fmt"
	"os"
	"strings"

	"ttr/internal/cache"
	"ttr/internal/paths"
)

// ttr cache: what's been downloaded and kept, and how much room it takes.
// Everything in the cache can be fetched again, so all of it is safe to
// clear — the only cost is the next fetch. The kinds are internal/cache's,
// which the settings panel shows too.

const cacheUsage = `Usage:
  ttr cache                 What's in the cache, and how much room it takes
  ttr cache clear [kind]    Empty the cache, or only one kind: %s

Everything in %s
can be downloaded again, so clearing it loses nothing but time: the next
time something is needed, it is fetched.`

func runCache(args []string) {
	names := make([]string, 0, len(cache.Kinds))
	for _, k := range cache.Kinds {
		names = append(names, k.Name)
	}
	usage := fmt.Sprintf(cacheUsage, strings.Join(names, ", "), paths.Cache())

	switch {
	case len(args) == 0:
		listCache()
	case args[0] == "clear" && len(args) <= 2:
		kind := ""
		if len(args) == 2 {
			kind = args[1]
			if !cache.IsKind(kind) {
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

func listCache() {
	use := cache.Usage()
	var total int64
	for _, u := range use {
		total += u.Size
	}

	fmt.Println(paths.Cache())
	fmt.Println()
	for _, k := range cache.Kinds {
		u := use[k.Name]
		files := ""
		if u.Files > 1 {
			files = fmt.Sprintf("%d files", u.Files)
		}
		line := fmt.Sprintf("  %-9s %9s  %-26s %s", k.Name, cache.Size(u.Size), k.What, files)
		fmt.Println(strings.TrimRight(line, " "))
	}
	if o := use[cache.Other]; o.Files > 0 {
		fmt.Printf("  %-9s %9s  %-26s %d files\n", cache.Other, cache.Size(o.Size), "anything else", o.Files)
	}
	fmt.Printf("\n  %-9s %9s\n", "total", cache.Size(total))
	fmt.Println("\nttr cache clear empties it, or ttr cache clear <kind> one kind of it.")
}

// clearCache deletes the cache's files, all of them or one kind's.
func clearCache(kind string) {
	n, freed, err := cache.Clear(kind)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	if kind != "" {
		for _, k := range cache.Kinds {
			if k.Name == kind {
				fmt.Printf("cleared %s: %d %s, %s — %s\n", k.What, n, plural("file", n), cache.Size(freed), k.Refetch)
				return
			}
		}
	}
	fmt.Printf("cleared the cache: %d %s, %s\n", n, plural("file", n), cache.Size(freed))
}

func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
