package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"ttr/internal/cache"
	"ttr/internal/deck"
	"ttr/internal/launcher"
	"ttr/internal/paths"
	"ttr/internal/uninstall"
)

// ttr uninstall: the program, its launcher, and what it downloaded, after
// one yes. Your settings and your decks only after a yes each, asked
// separately and defaulting to no.

const uninstallUsage = `Usage:
  ttr uninstall             Remove Tutor, asking first
  ttr uninstall -y          Don't ask about the program; settings and decks are still asked

Removes the program, its launcher, the cache (everything downloaded) and the
state (where you were). Then asks, separately, whether to remove your
settings and your decks too. Both default to no.`

func runUninstall(args []string) {
	yes := false
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		case "-h", "--help", "help":
			fmt.Println(uninstallUsage)
			return
		default:
			fmt.Println(uninstallUsage)
			os.Exit(1)
		}
	}
	interactive := term.IsTerminal(os.Stdin.Fd())
	if !interactive && !yes {
		fail(fmt.Errorf("not a terminal to ask in; `ttr uninstall -y` removes Tutor and keeps your settings and decks"))
	}
	in := bufio.NewReader(os.Stdin)

	var plan uninstall.Plan
	exe, ok := launcher.Executable()
	fmt.Println("This removes:")
	if ok {
		plan.Program = exe
		fmt.Printf("  %-18s %s\n", "the program", exe)
	} else {
		fmt.Printf("  %-18s %s\n", "the program", "(a temporary build; nothing to remove)")
	}
	if launcher.Exists() {
		fmt.Printf("  %-18s %s\n", "the launcher", launcher.Path())
	}
	fmt.Printf("  %-18s %s (%s)\n", "downloads", paths.Cache(), cache.Size(uninstall.Size(paths.Cache())))
	fmt.Printf("  %-18s %s\n", "where you were", paths.State())
	fmt.Println()

	if !yes && !ask(in, "Remove Tutor?") {
		fmt.Println("Nothing removed.")
		return
	}

	if settings := uninstall.Settings(); len(settings) > 0 && interactive {
		fmt.Println()
		fmt.Println("Your settings:")
		for _, p := range settings {
			fmt.Println("  " + p)
		}
		plan.Settings = ask(in, "Remove your settings too?")
	}

	if decks, n := uninstall.Decks(), countLists(); len(decks) > 0 && interactive && n > 0 {
		fmt.Println()
		fmt.Printf("Your decks: %d %s in %s\n", n, plural("list", n), deck.Dir())
		if url, _, ok := deck.SyncRemote(); ok {
			fmt.Println("They sync to " + url + ". Run ttr sync first if you've changed them since.")
		} else {
			fmt.Println("They aren't pushed anywhere, so removing them loses them.")
		}
		plan.Decks = ask(in, "Remove your decks too?")
	}

	errs := uninstall.Run(plan)
	fmt.Println()
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	var kept []string
	if !plan.Settings && len(uninstall.Settings()) > 0 {
		kept = append(kept, "settings")
	}
	if !plan.Decks && countLists() > 0 {
		kept = append(kept, "decks ("+deck.Dir()+")")
	}
	switch {
	case len(errs) > 0:
		fmt.Println("Tutor is partly removed; the errors above say what's left.")
		os.Exit(1)
	case len(kept) > 0:
		fmt.Println("Tutor is removed. Kept: " + strings.Join(kept, " and ") + ".")
	default:
		fmt.Println("Tutor is removed, and everything it kept.")
	}
}

// ask puts a yes/no question that only a typed yes answers yes.
func ask(in *bufio.Reader, question string) bool {
	fmt.Print(question + " [y/N] ")
	line, _ := in.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func countLists() int {
	slugs, _ := deck.List()
	return len(slugs)
}
