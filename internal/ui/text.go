package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Measuring and cutting text by the room it takes on screen.
//
// Not by bytes, and not by runes either. A card called "👑-Marchesa d'Amati"
// is nineteen runes and twenty-one columns, because an emoji occupies two
// cells; a row measured in runes overflows its panel, wraps, and leaves that
// panel a line taller than the one beside it. Every width in this package is
// a display width.

func textWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// runeWidth is how many cells a rune takes.
//
// runewidth alone isn't quite enough. A variation selector U+FE0F asks for
// the *emoji* presentation of the character before it, which terminals draw
// double-width — but runewidth measures the base character on its own and
// calls something like ♟ (U+265F) one cell. Counting the selector as one
// cell rather than zero gets the pair to the two the terminal will use. A
// deck of mine called "…Queens Gambit♟️" is what found this, twice.
//
// Emoji width in terminals is genuinely not fully determinable — different
// terminals disagree — so this is the closest honest approximation rather
// than a guarantee.
func runeWidth(r rune) int {
	switch r {
	case 0xFE0F: // emoji presentation
		return 1
	case 0xFE0E: // text presentation: leave the base narrow
		return 0
	}
	return runewidth.RuneWidth(r)
}

// truncate cuts to a display width, clean: no "…" after the cut. Text that
// can run long wraps instead (wrapStyled); what is cut here is a one-line row
// whose whole text the info panel shows. Walks runes with the same measure as
// textWidth, rather than runewidth's own, so the two can't disagree about
// where the cut falls.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if textWidth(s) <= max {
		return s
	}
	w, cut := 0, 0
	for i, r := range s {
		rw := runeWidth(r)
		if w+rw > max {
			cut = i
			break
		}
		w += rw
		cut = i + len(string(r))
	}
	return s[:cut]
}

// pad extends a string to a display width, for lining columns up by hand
// where a lipgloss Width() would also repaint the background.
func pad(s string, width int) string {
	if n := textWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// padLeft extends a string to a display width from the left, for a
// right-aligned column — the mirror of pad.
func padLeft(s string, width int) string {
	if n := textWidth(s); n < width {
		return strings.Repeat(" ", width-n) + s
	}
	return s
}

// fit does both: exactly this wide, cut or padded.
func fit(s string, width int) string { return pad(truncate(s, width), width) }

// wrap breaks text to a width, on spaces where it can. Blank lines in the
// source survive as blank lines, because a rule's paragraphs are how it is
// meant to be read.
func wrap(s string, width int) []string {
	if width < 1 {
		return nil
	}

	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case textWidth(line)+1+textWidth(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ansiReset is the sequence lipgloss ends every styled span with. It clears
// the background as well as the foreground, which is the whole reason a row
// needs highlightLine.
const ansiReset = "\x1b[0m"

// highlightLine paints a whole assembled row under bg and pads it to width, so
// a highlighted row is coloured edge to edge.
//
// A row is built from styled spans, each ending in lipgloss's reset — and a
// reset clears the background too. So the obvious
// lipgloss.NewStyle().Background(bg).Width(w).Render(line) colours only as far
// as the first reset: the marker, and nothing past it, which is the bug of a
// selection that covers just the leftmost column. This sets the background at
// the front and re-sets it after every reset, so the fill runs the whole row.
func highlightLine(line string, width int, bg lipgloss.Color) string {
	seq := bgStart(bg)
	body := seq + strings.ReplaceAll(line, ansiReset, ansiReset+seq)
	if pad := width - textWidth(stripStyles(line)); pad > 0 {
		body += strings.Repeat(" ", pad)
	}
	return body + ansiReset
}

// bgStart is the escape sequence that switches the background to bg, taken
// from lipgloss so it matches the colour profile in force. Empty when colour
// is off, which is what keeps highlightLine a no-op under a plain terminal and
// in tests.
func bgStart(bg lipgloss.Color) string {
	s := lipgloss.NewStyle().Background(bg).Render("\x00")
	if i := strings.IndexByte(s, 0); i >= 0 {
		return s[:i]
	}
	return ""
}

// stripStyles removes colour, for measuring text that has already been
// painted. Needed where a line is assembled from styled parts and then has
// to be padded to a width — measuring the escape sequences as characters is
// what wraps a row and grows a panel.
func stripStyles(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEscape = true
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// plural is the same six lines as in deck and moxfield; see the note there.
func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
