package main

import (
	"fmt"
	"os"

	"ttr/internal/launcher"
)

// ttr launcher: the entry in the desktop's application launcher. The first
// run makes it (launcher.FirstRun); this makes it again, or takes it away.

const launcherUsage = `Usage:
  ttr launcher              Put Tutor in the application launcher (again)
  ttr launcher remove       Take it out

The launcher opens a terminal running ttr. The first run puts it there; one
you remove stays removed.
  Linux:   a .desktop file in ~/.local/share/applications
  macOS:   Tutor.app in ~/Applications
  Windows: a Tutor shortcut in the Start menu`

func runLauncher(args []string) {
	switch {
	case len(args) == 0:
		exe, ok := launcher.Executable()
		if !ok {
			fail(fmt.Errorf("%s is a temporary build; run an installed ttr", exe))
		}
		if err := launcher.Install(exe); err != nil {
			fail(err)
		}
		fmt.Println("Tutor is in the application launcher:", launcher.Path())

	case args[0] == "remove":
		if !launcher.Exists() {
			fmt.Println("Tutor isn't in the application launcher.")
			return
		}
		if err := launcher.Remove(); err != nil {
			fail(err)
		}
		fmt.Println("Removed", launcher.Path())

	case args[0] == "-h" || args[0] == "--help" || args[0] == "help":
		fmt.Println(launcherUsage)

	default:
		fmt.Println(launcherUsage)
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
