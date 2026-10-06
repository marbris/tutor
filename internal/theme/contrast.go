package theme

import (
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// OnColour is what to write on a background of bg: the theme's near-black or
// its near-white, whichever stands out from it more. It is what lets a row
// take a card's own colour as its background — a yellow name wants dark text,
// a blue one light — without a theme having to say so for every colour.
func OnColour(bg lipgloss.Color) lipgloss.Color {
	l, ok := luminance(bg)
	if !ok {
		return FgBright
	}
	dark, okDark := luminance(Bg)
	light, okLight := luminance(FgBright)
	if !okDark {
		dark = 0
	}
	if !okLight {
		light = 1
	}
	if contrast(l, dark) >= contrast(l, light) {
		return Bg
	}
	return FgBright
}

// contrast is the WCAG contrast ratio between two relative luminances.
func contrast(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

// ansiRGB is xterm's default for the sixteen terminal colours. The real ones
// are whatever the terminal's scheme says, which a program can't ask; the
// defaults are a fair guess at which are light and which dark.
var ansiRGB = [16][3]uint8{
	{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0},
	{0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
	{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
	{92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

// luminance is a colour's WCAG relative luminance, 0 for black to 1 for white.
func luminance(c lipgloss.Color) (float64, bool) {
	rgb, ok := rgbOf(string(c))
	if !ok {
		return 0, false
	}
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(rgb[0]) + 0.7152*lin(rgb[1]) + 0.0722*lin(rgb[2]), true
}

// rgbOf reads a colour as the theme files write them: "#rgb", "#rrggbb", or
// an ANSI index 0–15.
func rgbOf(s string) ([3]uint8, bool) {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		if n >= 0 && n < len(ansiRGB) {
			return ansiRGB[n], true
		}
		return [3]uint8{}, false
	}
	hex := strings.TrimPrefix(s, "#")
	if hex == s {
		return [3]uint8{}, false
	}
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return [3]uint8{}, false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return [3]uint8{}, false
	}
	return [3]uint8{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
}
