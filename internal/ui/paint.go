package ui

import "strings"

// Painting a line's background without a reset after every word.
//
// lipgloss ends every styled word with a full reset, and a full reset takes
// the background with it, so it would have to be sent again before the next
// word. paintLine reads the line's colour changes instead and writes only the
// differences between one stretch of text and the next: a word that turns
// bold off says so (22) rather than resetting everything, and the background
// goes out once at the start of the line, and again only where something
// else changed it.

// sgr is what a stretch of text is drawn with: its colours, as the
// parameters that set them ("" for the terminal's own), and its attributes.
type sgr struct {
	fg, bg string
	attrs  uint16 // bit n is SGR attribute n, 1 (bold) to 9 (strike)
}

// attrOff is the code that turns each attribute off. 22 turns off both
// bold and faint.
var attrOff = map[int]string{1: "22", 2: "22", 3: "23", 4: "24", 5: "25", 7: "27", 8: "28", 9: "29"}

// paintLine draws line on the background bg ("48;2;r;g;b"), ending in a
// reset. False when the line holds an escape it doesn't follow, which is
// then painted the old way.
func paintLine(line, bg string) (string, bool) {
	var b strings.Builder
	b.Grow(len(line) + 32)
	var cur sgr // the line starts where the last one's reset left it
	want := sgr{bg: bg}
	for len(line) > 0 {
		esc := strings.IndexByte(line, '\x1b')
		text := line
		if esc >= 0 {
			text = line[:esc]
		}
		if text != "" {
			writeChange(&b, cur, want)
			cur = want
			b.WriteString(text)
		}
		if esc < 0 {
			break
		}
		line = line[esc:]
		end := strings.IndexByte(line, 'm')
		if len(line) < 2 || line[1] != '[' || end < 0 {
			return "", false
		}
		if !want.apply(line[2:end], bg) {
			return "", false
		}
		line = line[end+1:]
	}
	b.WriteString("\x1b[0m")
	return b.String(), true
}

// apply follows one escape's parameters. A reset goes back to the painted
// background, not the terminal's. False for anything it doesn't know.
func (s *sgr) apply(params, bg string) bool {
	if params == "" {
		*s = sgr{bg: bg}
		return true
	}
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		n, ok := sgrNumber(ps[i])
		if !ok {
			return false
		}
		switch {
		case n == 0:
			*s = sgr{bg: bg}
		case attrOff[n] != "":
			s.attrs |= 1 << n
		case n == 22:
			s.attrs &^= 1<<1 | 1<<2
		case n >= 23 && n <= 29 && n != 26:
			s.attrs &^= 1 << (n - 20)
		case n >= 30 && n <= 37, n >= 90 && n <= 97:
			s.fg = ps[i]
		case n == 39:
			s.fg = ""
		case n >= 40 && n <= 47, n >= 100 && n <= 107:
			s.bg = ps[i]
		case n == 49:
			s.bg = ""
		case n == 38 || n == 48:
			// 38;5;n or 38;2;r;g;b, and the same for 48.
			if i+1 >= len(ps) {
				return false
			}
			take := map[string]int{"5": 1, "2": 3}[ps[i+1]]
			if take == 0 || i+2+take > len(ps) {
				return false
			}
			for _, p := range ps[i+2 : i+2+take] {
				if _, ok := sgrNumber(p); !ok {
					return false
				}
			}
			colour := strings.Join(ps[i:i+2+take], ";")
			if n == 38 {
				s.fg = colour
			} else {
				s.bg = colour
			}
			i += 1 + take
		default:
			return false
		}
	}
	return true
}

func sgrNumber(p string) (int, bool) {
	if p == "" || len(p) > 3 {
		return 0, false
	}
	n := 0
	for _, r := range p {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// writeChange writes the one escape that takes text drawn with from to
// being drawn with to, or nothing when they are the same.
func writeChange(b *strings.Builder, from, to sgr) {
	if from == to {
		return
	}
	var ps []string
	if off := from.attrs &^ to.attrs; off != 0 {
		if off&(1<<1|1<<2) != 0 {
			ps = append(ps, "22")
			from.attrs &^= 1<<1 | 1<<2 // 22 took both; the one still wanted goes back on below
		}
		for n := 3; n <= 9; n++ {
			if off&(1<<n) != 0 && attrOff[n] != "" {
				ps = append(ps, attrOff[n])
			}
		}
	}
	for n := 1; n <= 9; n++ {
		if on := to.attrs &^ from.attrs; on&(1<<n) != 0 {
			ps = append(ps, itoa(n))
		}
	}
	if from.fg != to.fg {
		ps = append(ps, orDefault(to.fg, "39"))
	}
	if from.bg != to.bg {
		ps = append(ps, orDefault(to.bg, "49"))
	}
	b.WriteString("\x1b[")
	b.WriteString(strings.Join(ps, ";"))
	b.WriteByte('m')
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
