package ui

import (
	"strings"
	"testing"

	"ttr/internal/rules"
)

// Things that ran off the edge, or were cut with "…", now wrap.

func checkWrapped(t *testing.T, what string, lines []string, width int, want ...string) {
	t.Helper()
	text := strings.Join(lines, " ")
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("%s lost %q:\n%s", what, w, strings.Join(lines, "\n"))
		}
	}
	for _, line := range lines {
		if textWidth(line) > width {
			t.Errorf("%s has a line %d wide at %d: %q", what, textWidth(line), width, line)
		}
		if strings.Contains(line, "…") {
			t.Errorf("%s was cut: %q", what, line)
		}
	}
}

func TestACardsTagsWrapInTheInfoPanel(t *testing.T) {
	card := sample()[1]
	card.Tags = []string{"ramp", "mana-dork", "elf", "tribal-payoff", "one-drop", "untapper", "early-game"}
	card.Borrowed = []string{"budget-staple"}

	lines := splitLines(stripANSI(strings.Join(cardMeta(card, card.Card, 24, nil, nil, nil), "\n")))
	// The tags are the first block, up to the blank line before the facts.
	end := 1
	for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
		end++
	}
	checkWrapped(t, "the tags", lines[:end], 24, append(card.Tags, card.Borrowed...)...)
}

func TestALongDeckNameWrapsInTheInfoPanel(t *testing.T) {
	name := "Isshin, Two Heavens as One — the attack trigger pile"
	l := &deckList{order: byName, all: []deckEntry{
		{kind: entryLocal, name: name, slug: "isshin", count: 100},
	}}
	l.refresh()

	lines := splitLines(stripANSI(strings.Join(l.info(20), "\n")))
	end := 0
	for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
		end++
	}
	if end < 2 {
		t.Fatalf("the name didn't wrap:\n%s", strings.Join(lines, "\n"))
	}
	checkWrapped(t, "the name", lines[:end], 20, strings.Fields(name)...)
}

func TestAListPanelsTitleWrapsAndTheHeadersStayLevel(t *testing.T) {
	// A narrow search panel beside a deck: the query wraps onto a second
	// line, and the deck's rule moves down with it.
	m := withCards(sized(90, 30), "f", sample(), sortArrival)
	query := "t:creature o:draw id<=sultai -t:legendary"
	m.ws.current().title = query
	m.ws.current().cardsView().name = query
	m, l := ownDeck(m)
	l.name = "Ghen, Arcanum Weaver and the enchantment pile"
	m.ws.current().title = l.name

	view := stripANSI(m.View())
	for _, want := range append(strings.Fields(query), strings.Fields(l.name)...) {
		if !strings.Contains(view, want) {
			t.Errorf("%q isn't in the headers:\n%s", want, view)
		}
	}
	level := false
	for _, line := range splitLines(view) {
		if strings.Count(line, "│─") >= 2 {
			level = true
		}
	}
	if !level {
		t.Errorf("the panels' rules aren't level:\n%s", view)
	}
}

func TestALongTypeLineWrapsInTheInfoPanel(t *testing.T) {
	card := sample()[0] // Legendary Creature — Elf Archer
	lines := splitLines(stripANSI(strings.Join(cardInfo(card, 20, rules.Data{}, nil, nil, nil), "\n")))
	// The type line follows the name, up to the blank line before the text.
	end := 1
	for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
		end++
	}
	if end < 3 {
		t.Fatalf("the type line didn't wrap:\n%s", strings.Join(lines, "\n"))
	}
	checkWrapped(t, "the type line", lines[1:end], 20, strings.Fields(card.Card.TypeLine)...)
}
