package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// cell is one character as a terminal would draw it.
type cell struct {
	r rune
	s sgr
}

// cells plays a line through the way a terminal does, where a reset goes
// back to the terminal's own background.
func cells(t *testing.T, line string) []cell {
	t.Helper()
	var out []cell
	var s sgr
	for len(line) > 0 {
		if line[0] == '\x1b' {
			end := strings.IndexByte(line, 'm')
			if !s.apply(line[2:end], "") {
				t.Fatalf("unreadable escape in %q", line)
			}
			line = line[end+1:]
			continue
		}
		r := []rune(line)[0]
		out = append(out, cell{r, s})
		line = line[len(string(r)):]
	}
	return out
}

const testBg = "48;2;40;40;40"

func samePainting(t *testing.T, line string) string {
	t.Helper()
	got, ok := paintLine(line, testBg)
	if !ok {
		t.Fatalf("paintLine gave up on %q", line)
	}
	want := cells(t, paintAfterResets(line, testBg))
	have := cells(t, got)
	if len(want) != len(have) {
		t.Fatalf("%d characters, want %d", len(have), len(want))
	}
	for i := range want {
		if want[i] != have[i] {
			t.Fatalf("character %d (%q) drawn with %+v, want %+v\nline %q\ngot  %q",
				i, want[i].r, have[i].s, want[i].s, line, got)
		}
	}
	return got
}

func TestPaintingTheBackgroundOnceDrawsTheSameCells(t *testing.T) {
	for _, line := range []string{
		"",
		"plain",
		"\x1b[38;2;1;2;3mred\x1b[0m then \x1b[1;38;2;4;5;6mbold\x1b[0m plain",
		"\x1b[1mbold\x1b[0m\x1b[2mfaint\x1b[22m\x1b[1;2mboth\x1b[22mnone",
		"\x1b[3;4;7;9mall\x1b[23;24;27;29mnone\x1b[mreset",
		"\x1b[38;2;9;9;9;48;2;7;7;7mown bg\x1b[49mterminal's\x1b[0mtheme's",
		"\x1b[38;5;1m\U0010EEEE\U0010EEEE\x1b[39m after the picture",
		"\x1b[31;42mansi\x1b[91;102mbright\x1b[39;49mdefaults",
		"\x1b[1m\x1b[0m\x1b[38;2;1;1;1m", // escapes with nothing after them
	} {
		samePainting(t, line)
	}
}

func TestAnUnknownEscapeIsPaintedTheOldWay(t *testing.T) {
	line := "\x1b[58;5;3munderline colour\x1b[0m"
	if _, ok := paintLine(line, testBg); ok {
		t.Fatal("paintLine claimed an escape it doesn't follow")
	}
	if got := paintBackground(line, "#282828"); !strings.Contains(got, "58;5;3") {
		t.Errorf("the line lost its escape: %q", got)
	}
}

func TestAPaintedFrameIsTheSameAndSmaller(t *testing.T) {
	before := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(before)

	m := sized(160, 40)
	p := m.ws.open(KindFind)
	p.searchOpen = false
	p.show(newCardList(manyCards(60), sortType, ""))
	frame := m.viewWorkspace()

	old, now := 0, 0
	for _, line := range strings.Split(frame, "\n") {
		old += len(paintAfterResets(line, testBg))
		now += len(samePainting(t, line))
	}
	if now*10 > old*8 {
		t.Errorf("the frame is %d bytes painted once, %d the old way", now, old)
	}
	t.Logf("%d bytes, from %d", now, old)
}
