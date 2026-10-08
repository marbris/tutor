package theme

import "io"

// ── The terminal's own background ───────────────────────────────

// ttr paints every cell, but a terminal can have pixels outside its cells:
// kitty centres the grid and fills the leftover strip at the top and bottom,
// and any window padding, with the terminal's background, not ttr's. So
// while a theme that paints its background is in force, the terminal's own
// background is set to it (OSC 11), and put back on the way out (OSC 111).
// Terminals that don't know the sequences ignore them.

const resetBackground = "\x1b]111\x1b\\"

// backgroundSet is whether ttr has changed the terminal's background, and so
// owes it a reset.
var backgroundSet bool

// TerminalBackground is the escape that makes the terminal's background the
// theme's, or "" when the theme leaves the background to the terminal. A
// theme whose background isn't "#rrggbb" (an ANSI index) leaves it too.
func TerminalBackground() string {
	s := string(Surface)
	if transparent || len(s) != 7 || s[0] != '#' {
		return ""
	}
	return "\x1b]11;" + s + "\x1b\\"
}

// SyncTerminalBackground sets the terminal's background to the theme in
// force, or resets it if ttr set it before and this theme paints none.
func SyncTerminalBackground(w io.Writer) {
	if seq := TerminalBackground(); seq != "" {
		io.WriteString(w, seq)
		backgroundSet = true
		return
	}
	RestoreTerminalBackground(w)
}

// RestoreTerminalBackground puts the terminal's own background back, if ttr
// changed it.
func RestoreTerminalBackground(w io.Writer) {
	if backgroundSet {
		io.WriteString(w, resetBackground)
		backgroundSet = false
	}
}
