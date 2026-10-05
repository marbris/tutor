package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ttr/internal/config"
	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/moxfield"
	"ttr/internal/paths"
	"ttr/internal/rules"
	"ttr/internal/theme"
	"ttr/internal/ui"
)

// The entry point and the non-TUI paths out of it: the one-shot card lookup
// that prints to stdout, and the `ttr rules` / `ttr deck` subcommands.

// version is the release, set at build time with
// -ldflags "-X main.version=$(VERSION)". It stays "dev" for a plain go build.
var version = "dev"

// ── Stdout output ───────────────────────────────────────────────

// ── Entry point ─────────────────────────────────────────────────

func main() {
	// Files used to live in one directory; put any left there where they
	// now belong, before anything goes looking for them.
	paths.Migrate()
	// Deck files are .list files now, since they hold tag lists too.
	if _, err := deck.MigrateExt(); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: renaming .deck files to .list:", err)
	}

	// A broken theme file is worth saying so about, but not worth refusing
	// to start over: the default is already in force.
	if err := theme.Load(); err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	}
	// The same for keys.json: what can't be used is named, and the default
	// keys stand in for it.
	if err := keymap.Load(); err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	}

	// And for config.json itself, and the list orders in it.
	if err := config.Check(); err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	}
	if err := ui.SetSortConfig(config.Load().Sort); err != nil {
		fmt.Fprintln(os.Stderr, "Warning:", err)
	}
	ui.SetOtagPrefix(config.Load().Otag())

	args := os.Args[1:]
	if len(args) == 0 {
		// Back to the workspace you left, or the splash if there is none.
		run(ui.NewRestored())
		return
	}

	switch args[0] {
	case "-h", "--help", "help":
		printUsage()
		return

	case "-v", "--version", "version":
		fmt.Println("ttr " + version)
		return

	case "theme":
		runTheme(args[1:])
		return

	case "keys":
		runKeys(args[1:])
		return

	case "init":
		runInit(args[1:])
		return

	case "cache":
		runCache(args[1:])
		return

	case "rules":
		runRules(args[1:])
		return

	case "deck":
		runDeck(args[1:])
		return

	case "sync":
		runSync(args[1:])
		return
	}

	query := strings.Join(args, " ")

	// A Moxfield link passed straight in opens that deck.
	if id, ok := moxfield.Ref(query); ok {
		run(ui.NewWithRemote(id))
		return
	}

	// A query matching exactly one card almost never wanted an interface:
	// you asked what the card does.
	if ui.PrintCardIfSingle(query) {
		return
	}
	run(ui.NewWithQuery(query))
}

const deckUsage = `Usage:
  ttr deck <name>                 Open one of your decks
  ttr deck <id | url>             Browse a deck on Moxfield, without saving it
  ttr deck list                   List your decks
  ttr deck new <name> [format]    Start an empty deck
  ttr deck import <id|url> [as]   Copy a Moxfield deck in so you can edit it
  ttr deck rm <name>              Delete a deck
  ttr deck log <name>             What you've changed, and when
  ttr deck restore <name> <ref>   Bring back an earlier version

Decks are files in ` + "`" + `ttr deck dir` + "`" + `, one card per line — edit them here or in
your editor. That directory is a git repository, so every change is kept and
` + "`" + `git log` + "`" + ` works on your decks like anything else. Inside the app, w imports
the Moxfield deck you're looking at.`

// runDeck opens a deck, or handles one of the deck subcommands. References
// are checked here so a typo fails on the terminal; the deck itself loads
// once the TUI is up, which keeps the "loading deck…" line on screen.
func runDeck(args []string) {
	if len(args) == 0 {
		fmt.Println(deckUsage)
		os.Exit(1)
	}

	switch args[0] {
	case "list", "ls":
		runDeckList()
		return
	case "new", "create":
		runDeckNew(args[1:])
		return
	case "import", "save":
		runDeckImport(args[1:])
		return
	case "rm", "remove", "forget", "delete":
		runDeckRemove(args[1:])
		return
	case "log", "history":
		runDeckLog(args[1:])
		return
	case "restore":
		runDeckRestore(args[1:])
		return
	case "dir":
		fmt.Println(deck.Dir())
		return
	}

	arg := strings.Join(args, " ")

	// One of your own decks wins over reading the same text as a Moxfield
	// id, so a deck called "ghen" can't be shadowed by anything on Moxfield.
	if deck.Exists(arg) {
		run(ui.NewWithDeck(arg))
		return
	}

	id, ok := moxfield.Ref(arg)
	if !ok {
		fmt.Fprintf(os.Stderr, "No deck called %q, and that isn't a Moxfield id or URL.\n", arg)
		fmt.Println("Your decks:")
		runDeckList()
		os.Exit(1)
	}
	run(ui.NewWithRemote(id))
}

func runDeckList() {
	slugs, err := deck.List()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(slugs) == 0 {
		fmt.Println("No decks yet. Copy one in from Moxfield with:")
		fmt.Println("  ttr deck import <moxfield url>")
		return
	}

	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight)
	dimStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)

	width := 0
	for _, s := range slugs {
		if len(s) > width {
			width = len(s)
		}
	}

	for _, slug := range slugs {
		pad := strings.Repeat(" ", width-len(slug))
		d, err := deck.Read(slug)
		if err != nil {
			fmt.Printf("%s  %s\n", nameStyle.Render(slug+pad), dimStyle.Render(err.Error()))
			continue
		}
		total, unique := d.Counts()
		fmt.Printf("%s  %s\n", nameStyle.Render(slug+pad), d.Name)

		detail := fmt.Sprintf("%d cards, %d distinct", total, unique)
		if d.Format != "" {
			detail = d.Format + " · " + detail
		}
		fmt.Printf("%s  %s\n", strings.Repeat(" ", width), dimStyle.Render(detail))
	}

}

func runDeckNew(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ttr deck new <name> [format]")
		os.Exit(1)
	}

	format := ""
	name := strings.Join(args, " ")
	// A trailing format is a convenience, not a requirement: "ttr deck new
	// Ghen commander" and "ttr deck new Ghen" both work.
	if len(args) > 1 && knownFormat(args[len(args)-1]) {
		format = strings.ToLower(args[len(args)-1])
		name = strings.Join(args[:len(args)-1], " ")
	}

	slug, d, err := deck.New(name, format)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if _, warning, err := deck.SaveVersioned(slug, d); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	} else if warning != "" {
		fmt.Println(lipgloss.NewStyle().Foreground(theme.Accent).Render("  " + warning))
	}

	fmt.Printf("Created %q (%s)\n", d.Name, d.Format)
	fmt.Printf("  %s\n", deck.Path(slug))
	fmt.Printf("Open it with: ttr deck %s\n", slug)
	fmt.Println(lipgloss.NewStyle().Foreground(theme.TextMuted).
		Render("Search for a card and press a to add it, or c to make it a commander."))
}

// knownFormat is the set of formats worth recognising as a trailing word.
func knownFormat(s string) bool {
	switch strings.ToLower(s) {
	case "commander", "standard", "pioneer", "modern", "legacy",
		"vintage", "pauper", "brawl", "oathbreaker", "limited":
		return true
	}
	return false
}

func runDeckImport(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ttr deck import <deck-id | moxfield url> [name]")
		os.Exit(1)
	}

	id, ok := moxfield.Ref(args[0])
	if !ok {
		fmt.Println("Not a Moxfield deck id or URL:", args[0])
		os.Exit(1)
	}

	fmt.Println("Fetching deck…")
	d, err := moxfield.Import(id)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	slug := deck.Slugify(d.Name)
	if len(args) > 1 {
		slug = deck.Slugify(strings.Join(args[1:], " "))
	}
	if slug == "" {
		slug = id
	}

	verb := "Imported"
	if deck.Exists(slug) {
		verb = "Updated"
	}
	subject, warning, err := deck.SaveVersioned(slug, d)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	total, unique := d.Counts()
	fmt.Printf("%s %q — %d cards, %d distinct\n", verb, d.Name, total, unique)
	fmt.Printf("  %s\n", deck.Path(slug))
	if warning != "" {
		fmt.Printf("  %s\n", lipgloss.NewStyle().Foreground(theme.Accent).Render(warning))
	} else {
		fmt.Printf("  %s\n", lipgloss.NewStyle().Foreground(theme.TextMuted).Render("committed: "+subject))
	}
	fmt.Printf("Open it with: ttr deck %s\n", slug)
}

const deckLogLimit = 50

func runDeckLog(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: ttr deck log <name>")
		os.Exit(1)
	}
	slug := args[0]
	if !deck.Exists(slug) {
		fmt.Printf("No deck called %q.\n", slug)
		os.Exit(1)
	}

	commits, err := deck.History(slug, deckLogLimit)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if len(commits) == 0 {
		fmt.Printf("No history for %s yet.\n", slug)
		return
	}

	hashStyle := lipgloss.NewStyle().Foreground(theme.Highlight)
	dimStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)
	for _, c := range commits {
		fmt.Printf("%s  %s\n", hashStyle.Render(c.Short), c.Subject)
		fmt.Printf("%s  %s\n", strings.Repeat(" ", len(c.Short)), dimStyle.Render(c.When))
	}
	fmt.Printf("\n%s\n", dimStyle.Render("ttr deck restore "+slug+" <ref>  ·  git -C "+deck.RepoPath()+" show <ref>"))
}

func runDeckRestore(args []string) {
	if len(args) != 2 {
		fmt.Println("Usage: ttr deck restore <name> <ref>")
		os.Exit(1)
	}
	slug, ref := args[0], args[1]
	if !deck.Exists(slug) {
		fmt.Printf("No deck called %q.\n", slug)
		os.Exit(1)
	}

	// Show what's coming back before it lands, since this overwrites the
	// deck you have open.
	old, err := deck.At(slug, ref)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if err := deck.Restore(slug, ref); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	total, unique := old.Counts()
	fmt.Printf("Restored %s to %s — %d cards, %d distinct\n", slug, ref, total, unique)
	fmt.Println(lipgloss.NewStyle().Foreground(theme.TextMuted).
		Render("The version you restored over is still in the history."))
}

func runDeckRemove(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: ttr deck rm <name>")
		os.Exit(1)
	}
	name := args[0]

	removed := false
	if deck.Exists(name) {
		if err := deck.DeleteCommitted(name); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		removed = true
	}
	if !removed {
		fmt.Printf("No deck called %q.\n", name)
		os.Exit(1)
	}
	fmt.Printf("Deleted %s.\n", name)
}

// runRules opens the comprehensive-rules browser directly.
func runRules(args []string) {
	if len(args) > 0 && args[0] == "update" {
		fmt.Println("Downloading the comprehensive rules…")
		if err := rules.Download(); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		fmt.Println("Saved to", rules.FilePath())
		return
	}

	if !rules.Cached() {
		fmt.Println("Downloading the comprehensive rules…")
	}
	run(ui.NewWithRules(strings.Join(args, " ")))
}
