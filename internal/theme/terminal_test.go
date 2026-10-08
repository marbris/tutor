package theme

import (
	"strings"
	"testing"
)

func TestTheTerminalsBackgroundFollowsAPaintedThemeAndIsPutBack(t *testing.T) {
	t.Cleanup(func() { Use(Theme{Palette: fallbackPalette}); backgroundSet = false })

	gruvbox, err := builtinTheme("gruvbox")
	if err != nil {
		t.Fatal(err)
	}
	Use(gruvbox)
	var b strings.Builder
	SyncTerminalBackground(&b)
	if want := "\x1b]11;" + string(Surface) + "\x1b\\"; b.String() != want {
		t.Errorf("gruvbox wrote %q, want %q", b.String(), want)
	}

	// The terminal theme paints nothing, so the terminal gets its own back.
	term, err := builtinTheme("terminal")
	if err != nil {
		t.Fatal(err)
	}
	Use(term)
	if TerminalBackground() != "" {
		t.Error("the terminal theme sets a background")
	}
	b.Reset()
	SyncTerminalBackground(&b)
	if b.String() != resetBackground {
		t.Errorf("switching to terminal wrote %q, want the reset", b.String())
	}

	// Nothing set, nothing to put back.
	b.Reset()
	SyncTerminalBackground(&b)
	RestoreTerminalBackground(&b)
	if b.String() != "" {
		t.Errorf("wrote %q though nothing was set", b.String())
	}
}

func TestAnANSIBackgroundIsLeftToTheTerminal(t *testing.T) {
	t.Cleanup(func() { Use(Theme{Palette: fallbackPalette}) })
	Use(Theme{Palette: map[string]string{"bg": "0"}})
	if got := TerminalBackground(); got != "" {
		t.Errorf("an ANSI background gave %q", got)
	}
}
