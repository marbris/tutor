package ui

import (
	"fmt"
	"strings"
	"testing"

	"ttr/internal/catalog"
)

// withCardNames puts a few card names in use, as Scryfall's catalog would.
func withCardNames(t *testing.T) {
	t.Helper()
	old := catalog.Current()
	catalog.SetCurrent(&catalog.Data{Lists: map[string][]string{catalog.CardNames: {
		"Sol Ring", "Solemn Simulacrum", "Soldevi Golem", "Llanowar Elves", "Lightning Bolt",
	}}})
	t.Cleanup(func() { catalog.SetCurrent(old) })
}

func typeIn(m Model, s string) Model {
	for _, r := range s {
		m = drive(m, string(r))
	}
	return m
}

func TestTabCompletesACardNameInTheIBar(t *testing.T) {
	withCardNames(t)
	m, _ := ownDeck(sized(140, 30))
	m = typeIn(drive(m, "i"), "llan")
	p := m.ws.current()
	if hints := fmt.Sprint(m.hintGroups()); !strings.Contains(hints, "complete the name") {
		t.Errorf("tab isn't offered to complete the name: %s", hints)
	}
	m = drive(m, "tab")
	if got := p.askInput.Value(); got != "Llanowar Elves" {
		t.Errorf("tab made %q, want Llanowar Elves in its own letters", got)
	}
	if p.asking != askAddCard {
		t.Error("tab on a name swapped to otag")
	}
}

func TestSeveralNamesGrowToWhatTheyShareThenWalk(t *testing.T) {
	withCardNames(t)
	m, _ := ownDeck(sized(140, 30))
	m = typeIn(drive(m, "i"), "so")
	p := m.ws.current()
	m = drive(m, "tab")
	if got := p.askInput.Value(); got != "Sol" {
		t.Errorf("first tab made %q, want the shared Sol", got)
	}
	if !strings.Contains(m.notice, "3 cards") || !strings.Contains(m.notice, "Soldevi Golem") {
		t.Errorf("the notice doesn't list the fits: %q", m.notice)
	}
	m = drive(m, "tab")
	if got := p.askInput.Value(); got != "Sol Ring" {
		t.Errorf("second tab made %q, want the first fit", got)
	}
	m = drive(m, "shift+tab")
	if got := p.askInput.Value(); got != "Sol" {
		t.Errorf("shift+tab made %q, want back to Sol", got)
	}
	m = drive(m, "shift+tab")
	if got := p.askInput.Value(); got != "Solemn Simulacrum" {
		t.Errorf("shift+tab again made %q, want the last fit", got)
	}
}

func TestTabWithNothingToFinishKeepsWhatYouTyped(t *testing.T) {
	// "ins": every fit shares only what's typed, so the first tab lists
	// them and leaves the input; the walk starts on the second, and comes
	// back round to "ins" rather than losing it.
	old := catalog.Current()
	catalog.SetCurrent(&catalog.Data{Lists: map[string][]string{catalog.CardNames: {
		"Inspiring Unicorn", "Insatiable Avarice", "Insight",
	}}})
	t.Cleanup(func() { catalog.SetCurrent(old) })
	m, _ := ownDeck(sized(140, 30))
	m = typeIn(drive(m, "i"), "ins")
	p := m.ws.current()

	m = drive(m, "tab")
	if got := p.askInput.Value(); got != "ins" {
		t.Fatalf("first tab made %q, want ins left alone", got)
	}
	if !strings.Contains(m.notice, "3 cards") {
		t.Errorf("the fits aren't listed: %q", m.notice)
	}
	for _, want := range []string{"Insatiable Avarice", "Insight", "Inspiring Unicorn", "ins"} {
		m = drive(m, "tab")
		if got := p.askInput.Value(); got != want {
			t.Errorf("tab made %q, want %q", got, want)
		}
	}
	// Narrowing by typing, then tab again, finishes the one that's left.
	m = typeIn(m, "p")
	m = drive(m, "tab")
	if got := p.askInput.Value(); got != "Inspiring Unicorn" {
		t.Errorf("insp + tab made %q", got)
	}
}

func TestTabOnAnEmptyIBarStillSwapsToOtag(t *testing.T) {
	withCardNames(t)
	m, _ := ownDeck(sized(140, 30))
	m = drive(m, "i")
	p := m.ws.current()
	if hints := fmt.Sprint(m.hintGroups()); !strings.Contains(hints, "tag by otag instead") {
		t.Errorf("tab on an empty bar isn't offered as the swap: %s", hints)
	}
	m = drive(m, "tab")
	if p.asking != askOtag {
		t.Fatal("tab on an empty bar didn't swap to otag")
	}
	// On the otag side, tab completes the tag being typed.
	withTagger(t)
	m = typeIn(m, "remova")
	m = drive(m, "tab")
	if got := p.askInput.Value(); !strings.HasPrefix(got, "removal") {
		t.Errorf("tab on the otag side made %q, want removal…", got)
	}
	if p.asking != askOtag {
		t.Error("completing a tag swapped back")
	}
}

func TestNoCardStartsWithItSaysSo(t *testing.T) {
	withCardNames(t)
	m, _ := ownDeck(sized(140, 30))
	m = typeIn(drive(m, "i"), "zzz")
	m = drive(m, "tab")
	if !strings.Contains(m.notice, `no card starts with "zzz"`) {
		t.Errorf("notice %q", m.notice)
	}
}

func TestTheIBarsKeysAreOnTheBottomLineAndFollowTab(t *testing.T) {
	withCardNames(t)
	m, _ := ownDeck(sized(160, 30))
	m = drive(m, "i")
	lines := strings.Split(stripANSI(m.View()), "\n")
	bottom := lines[len(lines)-1]
	if !strings.Contains(bottom, "tag by otag instead") {
		t.Errorf("the bottom line doesn't offer tab's swap: %q", bottom)
	}
	m = typeIn(m, "llan")
	lines = strings.Split(stripANSI(m.View()), "\n")
	if bottom := lines[len(lines)-1]; !strings.Contains(bottom, "complete the name") {
		t.Errorf("with a name typed, the bottom line doesn't offer completing it: %q", bottom)
	}
}

func TestThePlaceholderIsShownInFullWhereThereIsRoom(t *testing.T) {
	withCardNames(t)
	m, _ := ownDeck(sized(200, 30))
	m = drive(m, "i")
	view := stripANSI(m.View())
	if !strings.Contains(view, "tab here: tag this list by otag") {
		t.Errorf("the placeholder is cut short in a wide panel:\n%s", firstLines(view, 3))
	}
	// And it doesn't push the line past the panel: every line is as wide
	// as the screen at most.
	for _, line := range strings.Split(view, "\n") {
		if w := textWidth(line); w > 200 {
			t.Errorf("a line is %d wide on a 200-wide screen: %q", w, line)
		}
	}
	m = drive(m, "tab")
	if view := stripANSI(m.View()); !strings.Contains(view, "tab here: add a card") {
		t.Errorf("the otag placeholder is cut short:\n%s", firstLines(view, 3))
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines[:min(n, len(lines))], "\n")
}
