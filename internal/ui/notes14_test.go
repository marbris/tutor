package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"ttr/internal/mtg"
	"ttr/internal/rules"
	"ttr/internal/theme"
)

// printingUp is a list with Sol Ring's picture already fetched and gx
// pressed on it.
func printingUp(t *testing.T) Model {
	t.Helper()
	withKitty(t, true)
	card := mtg.Card{Name: "Sol Ring", OracleID: "sol"}
	m := withCards(sized(160, 40), "f", sample()[2:3], sortArrival)
	m.ws.current().cardsView().all[0].Card = card
	m.ws.current().cardsView().refresh()
	m.images[imageKey(card)] = &cardPrintings{state: imgReady, list: []mtg.Card{
		{ID: "cmm1", Set: "cmm", SetName: "Commander Masters", ReleasedAt: "2023-08-04"},
	}}
	m.pictures["cmm1"] = &picture{state: imgReady, png: []byte("png"), w: 488, h: 680}
	m = drive(m, "g", "x")
	if m.info.mode != infoImage {
		t.Fatalf("gx left the panel in %v", m.info.mode)
	}
	return m
}

func TestClosingTheStatisticsGoesBackToThePrinting(t *testing.T) {
	for _, keys := range [][]string{{"s", "s"}, {"S", "s"}, {"s", "S", "S"}} {
		m := printingUp(t)
		m.ws.editing = m.ws.focused
		m = drive(m, keys...)
		if m.info.mode != infoImage {
			t.Errorf("%v from the printing view ended in %v", keys, m.info.mode)
		}
		if m.kitty.key != "cmm1" {
			t.Errorf("%v: the picture wasn't put back", keys)
		}
	}
}

func TestClosingTheStatisticsElsewhereGoesBackToTheCard(t *testing.T) {
	m := withCards(sized(160, 40), "f", sample(), sortArrival)
	m = drive(m, "s", "s")
	if m.info.mode != infoCard {
		t.Errorf("s s ended in %v", m.info.mode)
	}
	m = drive(m, "s", "g", "x")
	m = drive(m, "g", "x")
	m = drive(m, "s", "s")
	if m.info.mode != infoCard {
		t.Errorf("the printing closed under the statistics came back: %v", m.info.mode)
	}
}

func TestTheRulesOpenedFromThePrintingViewShowTheRule(t *testing.T) {
	m := printingUp(t)
	m.rules = fakeRules(t)
	m = drive(m, "space", "r")
	if m.ws.current().kind != KindRules {
		t.Fatalf("space r focused a %v panel", m.ws.current().kind)
	}
	if got := m.infoTitle(); got != "rule" {
		t.Errorf("the info panel is headed %q over the rules", got)
	}
	if m.kitty.key != "" {
		t.Error("the picture stayed in the terminal over the rules")
	}

	m = drive(m, "h")
	if got := m.infoTitle(); got != "printing" {
		t.Errorf("back on the list the info panel is headed %q", got)
	}
	if m.kitty.key != "cmm1" {
		t.Error("the picture didn't come back with the list")
	}
}

func TestTheEditingDecksCardKeysHideWhileTheRulesAreFocused(t *testing.T) {
	m := editingDeckModel()
	m.rules = fakeRules(t)
	has := func(m Model, label string) bool {
		for _, k := range m.editHints() {
			if k[1] == label {
				return true
			}
		}
		return false
	}
	if !has(m, "add") || !has(m, "undo") {
		t.Fatalf("on the deck itself the card keys are missing: %v", m.editHints())
	}
	m = drive(m, "space", "r")
	if m.ws.current().kind != KindRules {
		t.Fatalf("space r focused a %v panel", m.ws.current().kind)
	}
	for _, label := range []string{"add", "add + tag", "tag", "remove", "commander", "undo"} {
		if has(m, label) {
			t.Errorf("%q is offered under the deck while the rules are focused", label)
		}
	}
}

func TestRuleExamplesAreSetApartInTheInfoPanel(t *testing.T) {
	data := rules.Parse(strings.Join([]string{
		"1. Game Concepts",
		"107. Numbers and Symbols",
		"107.1b The game uses only positive numbers and zero.",
		"Example: If a 3/4 creature gets -5/-0, it’s a -2/4 creature.",
		"Example: Viridian Joiner is a 1/2 creature.",
		"",
	}, "\n"))
	v := newRuleSearch(data, "positive")
	lines := strings.Split(stripANSI(strings.Join(v.info(60), "\n")), "\n")
	want := []string{
		"The game uses only positive numbers and zero.",
		"",
		"  Example",
		"  If a 3/4 creature gets -5/-0, it’s a -2/4 creature.",
		"",
		"  Example",
		"  Viridian Joiner is a 1/2 creature.",
	}
	got := lines[2:]
	for i := range got {
		got[i] = strings.TrimRight(got[i], " ")
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the rule reads:\n%s", strings.Join(lines, "\n"))
	}
}

func TestTheThemesBackgroundIsSetAgainAfterEveryReset(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	m := withCards(sized(100, 20), "f", sample(), sortArrival)
	set := "\x1b[48;2;" // what the theme's background starts with
	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		if !strings.HasPrefix(line, set) {
			t.Fatalf("line %d doesn't start on the background: %q", i, line)
		}
		rest := line
		for {
			at := strings.Index(rest, "\x1b[0m")
			if at < 0 {
				break
			}
			rest = rest[at+len("\x1b[0m"):]
			if rest != "" && !strings.HasPrefix(rest, set) {
				t.Fatalf("line %d drops to the terminal's background after a reset: %q", i, line)
			}
		}
	}
}

func TestATransparentThemePaintsNoBackground(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)
	terminal, err := theme.Find("terminal")
	if err != nil {
		t.Fatal(err)
	}
	theme.Use(terminal)
	defer theme.Use(theme.Theme{})

	m := withCards(sized(100, 20), "f", sample(), sortArrival)
	for i, line := range strings.Split(m.View(), "\n") {
		if strings.HasPrefix(line, "\x1b[4") {
			t.Fatalf("line %d starts on a background: %q", i, line)
		}
	}
}
