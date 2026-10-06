package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"ttr/internal/theme"
)

const themeUsage = `Usage:
  ttr theme                 List the themes available
  ttr theme <name>          Use a theme
  ttr theme edit <name>     Copy a theme into your config to edit

Themes are JSON files in %s.
A theme names its colors; every role — accent, borders, mana, rarity —
falls back to the default mapping, so fourteen colors is a whole theme.
"transparent": true leaves the background to the terminal.`

func runTheme(args []string) {
	switch {
	case len(args) == 0:
		listThemes()

	case args[0] == "edit" && len(args) > 1:
		editTheme(args[1])

	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Printf(themeUsage+"\n", theme.Dir())

	case len(args) == 1:
		if err := theme.Set(args[0]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		fmt.Printf("Theme set to %s.\n", args[0])

	default:
		fmt.Printf(themeUsage+"\n", theme.Dir())
		os.Exit(1)
	}
}

// listThemes shows each theme's palette as swatches, which says more about
// whether you want it than its name does.
func listThemes() {
	current := theme.Current()
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	// The names column is as wide as the longest name, so every row's
	// swatches start in the same column.
	names := theme.List()
	width := 0
	for _, name := range names {
		width = max(width, len(name))
	}

	// Over the swatches, what each run of them is.
	if stdoutIsTerminal() {
		var head []string
		for i, g := range theme.PaletteGroups() {
			label := []string{"bg", "text", "colors"}[i]
			head = append(head, fmt.Sprintf("%-*s", 2*len(g), label))
		}
		fmt.Printf("  %-*s %s\n", width, "", dim.Render(strings.TrimRight(strings.Join(head, " "), " ")))
	}

	for _, name := range names {
		t, err := theme.Find(name)
		if err != nil {
			continue
		}

		marker := "  "
		if name == current {
			marker = lipgloss.NewStyle().Foreground(theme.Accent).Render("▸ ")
		}

		swatches := paletteBar(t)

		where := "built-in"
		if !theme.IsBuiltin(name) {
			where = "yours"
		}
		fmt.Printf("%s%-*s %s  %s\n", marker, width, name, swatches, dim.Render(where))
	}

	fmt.Println()
	fmt.Println(dim.Render("ttr theme <name> to switch · ttr theme edit <name> to copy one and change it"))
}

// paletteBar draws a theme's colours as blocks. Piped to a file there is no
// colour to draw with, so it says the values instead — a listing whose whole
// content is colour is a blank page in a pager.
func paletteBar(t theme.Theme) string {
	if !stdoutIsTerminal() {
		var parts []string
		for _, name := range theme.PaletteNames() {
			if value, ok := t.Palette[name]; ok {
				parts = append(parts, value)
			}
		}
		return strings.Join(parts, " ")
	}

	var b strings.Builder
	for i, g := range theme.PaletteGroups() {
		if i > 0 {
			b.WriteString(" ")
		}
		for _, name := range g {
			value, ok := t.Palette[name]
			if !ok {
				b.WriteString("  ")
				continue
			}
			b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color(value)).Render("  "))
		}
	}
	return b.String()
}

func stdoutIsTerminal() bool { return term.IsTerminal(os.Stdout.Fd()) }

// editTheme copies a theme into the config directory with every role written
// out, so there's something to edit rather than a blank file.
func editTheme(name string) {
	body, err := theme.Export(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	os.MkdirAll(theme.Dir(), 0755)
	path := filepath.Join(theme.Dir(), name+".json")
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "Error: %s already exists — edit it, or delete it first.\n", path)
		os.Exit(1)
	}
	if err := os.WriteFile(path, body, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %s\n", path)
	fmt.Println("It shadows the built-in of the same name. Edit and run `ttr theme " + name + "`.")
}
