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
	if got := p.askInput.Value(); got != "Solemn Simulacrum" && got != "Soldevi Golem" {
		t.Errorf("shift+tab made %q, want to step back round", got)
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
