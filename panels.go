package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/theme"
	"ttr/internal/ui"
)

// Running the workspace. Every way into the program ends here with a model
// and, when it was asked to open something, the command that fetches it.
func run(m ui.Model, cmd tea.Cmd) {
	p := tea.NewProgram(m, tea.WithAltScreen())
	if cmd != nil {
		// Handed to the program rather than run here, so the workspace is
		// on screen while the fetching happens.
		go func() { p.Send(cmd()) }()
	}
	// The terminal's background takes the theme's, so the strips outside
	// the cells match too; it's put back before anything else is printed.
	theme.SyncTerminalBackground(os.Stdout)
	_, err := p.Run()
	theme.RestoreTerminalBackground(os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
