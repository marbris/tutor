package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ttr/internal/config"
	"ttr/internal/keymap"
	"ttr/internal/paths"
	"ttr/internal/theme"
	"ttr/internal/ui"
)

// ttr init: the settings files, written out with every default in them and
// every line commented out. A file like that changes nothing, and says
// everything: what can be set, what it is now, and how it's written.
// Uncommenting a line is how a setting is changed.

const initUsage = `Usage:
  ttr init                  Write commented-out templates of every settings file

Writes config.json, keys.json and themes/custom.json into %s,
each holding every default with every line commented out — so nothing
changes until you uncomment something. A file that is already there is left
alone, and the template is written beside it as <file>.default instead.`

// template is one settings file to write: where, and what.
type template struct {
	path string
	body string
}

func runInit(args []string) {
	if len(args) > 0 {
		fmt.Printf(initUsage+"\n", paths.Config())
		if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
			return
		}
		os.Exit(1)
	}

	templates, err := initTemplates()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	for _, t := range templates {
		path := t.path
		if _, err := os.Stat(path); err == nil {
			path += ".default"
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(path, []byte(t.body), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		if path != t.path {
			fmt.Printf("%s is already there — wrote the template beside it: %s\n", t.path, path)
		} else {
			fmt.Println("wrote", path)
		}
	}
}

// initTemplates is every template, with its defaults as they ship.
func initTemplates() ([]template, error) {
	sortDefaults := ui.SortDefaults()
	otag := config.DefaultOtagPrefix
	cfg := config.Config{
		OtagPrefix: &otag,
		Theme:      theme.DefaultName,
		Sync:       &config.Sync{Remote: "git@github.com:you/mtg-decks.git", Branch: "main"},
		Sort:       &sortDefaults,
	}
	cfgBody, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	themeBody, err := theme.Export(theme.DefaultName)
	if err != nil {
		return nil, err
	}

	return []template{
		{config.Path(), commentOut(configHeader, cfgBody)},
		{keymap.Path(), commentOut(keysHeader, keymap.DefaultsJSON())},
		{filepath.Join(theme.Dir(), "custom.json"), commentOut(themeHeader, themeBody)},
	}, nil
}

const configHeader = `ttr's settings, every one at its default. Everything is commented out, so
this file changes nothing: uncomment the lines you want — the braces too —
and edit them. // to the end of a line is ignored.

"sort" is the list orders: "cycle" is the orders . and , step through, in
that order — leave one out and it isn't offered; "name" is left out as it
ships. "direction" is which way each starts, "asc" or "desc".

"otag_prefix" goes in front of a Scryfall Tagger tag when T o or T O tags
your cards by it: removal becomes otag-removal. "" for none.

"sync" is where ttr sync pushes your decks; set it with ttr sync remote.
Note that ttr theme and ttr sync rewrite this file, and drop its comments.`

const keysHeader = `Every key ttr has, at its default. Everything is commented out, so this
file changes nothing: uncomment a scope's braces and the actions you want to
move, and change their keys. An action you leave out keeps its default; an
empty list unbinds it. See ttr keys -h.`

const themeHeader = `A theme to start from: the default one, every colour and role written
out, all commented out — so as it stands it is the default theme. Uncomment
it, change what you like, and choose it with ttr theme custom.`

// commentOut is a file of commented lines: the header, then the body.
func commentOut(header string, body []byte) string {
	var b strings.Builder
	for _, line := range strings.Split(header, "\n") {
		b.WriteString(strings.TrimRight("// "+line, " ") + "\n")
	}
	b.WriteString("//\n")
	for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
		b.WriteString("// " + line + "\n")
	}
	return b.String()
}
