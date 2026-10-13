package ui

import (
	"strings"
	"testing"
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

	lines := splitLines(stripANSI(strings.Join(cardMeta(card, card.Card, 24, nil, nil), "\n")))
	// The tags are the first block, up to the blank line before the facts.
	end := 1
	for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
		end++
	}
	checkWrapped(t, "the tags", lines[:end], 24, append(card.Tags, card.Borrowed...)...)
}
