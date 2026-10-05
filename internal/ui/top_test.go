package ui

import (
	"strings"
	"testing"
)

// The keys and the last thing you did sit in two lines above the panels,
// always two, so the panels never move when a notice comes or goes.

func TestThePanelsStayPutWhateverTheTopLinesSay(t *testing.T) {
	base := withCards(sized(120, 30), "f", sample(), sortArrival)

	noticed := base
	noticed.notice = "+1 Sol Ring"
	expanded := base
	expanded.hintsExpanded = true

	states := map[string]Model{
		"plain":      base,
		"a notice":   noticed,
		"? on":       expanded,
		"the leader": drive(base, "space"),
		"the i bar":  drive(base, "i"),
		"the T menu": drive(base, "T"),
		"the / bar":  drive(base, "/"),
	}
	for name, m := range states {
		if got := m.topHeight(); got != topRows {
			t.Errorf("%s: the top takes %d lines, want %d:\n%s", name, got, topRows, topBlock(m))
		}
		lines := strings.Split(stripANSI(m.View()), "\n")
		if len(lines) != 30 {
			t.Errorf("%s: the frame is %d lines, want 30", name, len(lines))
		}
		if !strings.HasPrefix(lines[topRows], "╭") {
			t.Errorf("%s: the panels don't start on line %d: %q", name, topRows, lines[topRows])
		}
	}
}

func TestTheKeysAreAboveTheNotice(t *testing.T) {
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	m.notice = "+1 Sol Ring"
	lines := strings.Split(stripANSI(m.View()), "\n")
	if !strings.Contains(lines[0], "quit") {
		t.Errorf("the keys aren't on the first line: %q", lines[0])
	}
	if !strings.Contains(lines[1], "+1 Sol Ring") {
		t.Errorf("the notice isn't on the second line, next to the panels: %q", lines[1])
	}
}

func TestTheCompletionListIsNextToTheBarYouTypeIn(t *testing.T) {
	withCardNames(t)
	m, _ := ownDeck(sized(160, 30))
	m = drive(typeIn(drive(m, "i"), "sol"), "tab")
	if m.notice == "" {
		t.Fatal("tab listed nothing")
	}
	lines := strings.Split(stripANSI(m.View()), "\n")
	want := stripANSI(m.notice)
	if len(want) > 20 {
		want = want[:20]
	}
	if !strings.Contains(lines[1], want) {
		t.Errorf("the completion list isn't on the second line:\n%s\n%s", lines[0], lines[1])
	}
}

func TestWithKeysOnTheLeaderMenuSharesTheKeysLine(t *testing.T) {
	m := withCards(sized(80, 30), "f", sample(), sortArrival)
	m.hintsExpanded = true

	lines := strings.Split(topBlock(m), "\n")
	if !strings.Contains(lines[0], "space:") {
		t.Errorf("the leader's menu doesn't lead the keys line: %q", lines[0])
	}
	if !strings.Contains(topBlock(m), "q quit") {
		t.Errorf("q quit gave way to the leader's menu:\n%s", topBlock(m))
	}

	// With a notice up, the keys line keeps to its line, and the leader's
	// last entries give way to ? and q.
	m.notice = "+1 Sol Ring"
	lines = strings.Split(topBlock(m), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "q quit") || !strings.Contains(lines[1], "Sol Ring") {
		t.Errorf("the keys and the notice don't share the two lines:\n%s", topBlock(m))
	}
}

func TestTheLeaderMenuFitsTheTwoLines(t *testing.T) {
	m := drive(withCards(sized(80, 30), "f", sample(), sortArrival), "space")
	if got := len(m.leaderBarLines()); got > topRows {
		t.Errorf("at 80 columns the leader's menu takes %d lines:\n%s", got, topBlock(m))
	}
	if !strings.Contains(topBlock(m), "only") {
		t.Errorf("the menu lost its last entry:\n%s", topBlock(m))
	}
}
