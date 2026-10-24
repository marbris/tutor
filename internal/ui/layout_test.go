package ui

import "testing"

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

func TestPanelsFillTheWidthExactly(t *testing.T) {
	// A column short of the terminal's width leaves a seam down the side.
	for _, width := range []int{80, 100, 120, 131, 200, 201} {
		for count := 1; count <= 6; count++ {
			l := computeLayout(width, 40, count, 0, 0, false)
			if got := sum(l.panels) + l.info; got != width {
				t.Errorf("width %d, %d panels: covered %d columns", width, count, got)
			}
		}
	}
}

func TestPanelWidthsDifferByAtMostOne(t *testing.T) {
	l := computeLayout(131, 40, 4, 0, 0, false)
	lo, hi := l.panels[0], l.panels[0]
	for _, w := range l.panels {
		if w < lo {
			lo = w
		}
		if w > hi {
			hi = w
		}
	}
	if hi-lo > 1 {
		t.Errorf("widths %v are uneven", l.panels)
	}
}

func TestTheRowScrollsRatherThanShrinkingPastTheFloor(t *testing.T) {
	// Eight panels on a narrow terminal can't each be readable, so some go
	// off screen instead of all of them becoming unusable.
	l := computeLayout(100, 40, 8, 0, 0, false)
	if l.visible() >= 8 {
		t.Fatalf("showed all 8 panels in 100 columns: %v", l.panels)
	}
	for _, w := range l.panels {
		if w < minPanel {
			t.Errorf("panel of %d columns is below the %d floor", w, minPanel)
		}
	}
}

func TestTheFocusedPanelIsAlwaysOnScreen(t *testing.T) {
	// Typing into a panel you can't see would be indefensible.
	for focused := 0; focused < 8; focused++ {
		l := computeLayout(100, 40, 8, focused, 0, false)
		if !l.shows(focused) {
			t.Errorf("focus on panel %d, showing %d..%d",
				focused, l.first, l.first+l.visible()-1)
		}
	}
}

func TestScrollPositionIsKeptWhenItCan(t *testing.T) {
	// Moving focus within the visible window shouldn't shuffle the row.
	l := computeLayout(100, 40, 8, 3, 2, false)
	if l.first != 2 {
		t.Errorf("first = %d, want the scroll position left alone at 2", l.first)
	}
}

func TestScrollFollowsFocusOffEitherEnd(t *testing.T) {
	l := computeLayout(100, 40, 8, 7, 0, false)
	if !l.shows(7) {
		t.Error("scrolling right did not follow focus")
	}
	l = computeLayout(100, 40, 8, 0, 5, false)
	if l.first != 0 {
		t.Errorf("first = %d, want 0 after focus moved to the far left", l.first)
	}
}

func TestTheInformationPanelGivesWayOnANarrowTerminal(t *testing.T) {
	// Below a point there is only room for one of the two, and the panel
	// you're working in wins.
	l := computeLayout(30, 40, 1, 0, 0, false)
	if l.info != 0 {
		t.Errorf("info = %d, want it dropped at 30 columns", l.info)
	}
	if sum(l.panels) != 30 {
		t.Errorf("panels = %v, want the full 30 columns", l.panels)
	}
}

func TestTheInformationPanelTakesAnEqualShare(t *testing.T) {
	// The information panel is just another column: it is the same width as
	// each panel, give or take the odd remainder column.
	for _, width := range []int{80, 120, 200, 400} {
		for count := 1; count <= 4; count++ {
			l := computeLayout(width, 40, count, 0, 0, false)
			if l.info == 0 {
				t.Errorf("width %d, %d panels: information panel dropped", width, count)
				continue
			}
			for _, w := range l.panels {
				if diff := w - l.info; diff < -1 || diff > 1 {
					t.Errorf("width %d, %d panels: panel %d vs info %d differ by more than one",
						width, count, w, l.info)
				}
			}
		}
	}
}

func TestNoPanelsMeansNoLayout(t *testing.T) {
	l := computeLayout(120, 40, 0, 0, 0, false)
	if l.visible() != 0 || l.info != 0 {
		t.Errorf("empty workspace produced %d panels and info %d", l.visible(), l.info)
	}
}

func TestTheStatisticsTakeAColumnOfTheirOwn(t *testing.T) {
	for _, width := range []int{80, 120, 200} {
		for count := 1; count <= 3; count++ {
			l := computeLayout(width, 40, count, 0, 0, true)
			if l.stats == 0 || l.info == 0 {
				t.Errorf("width %d, %d panels: stats %d, info %d", width, count, l.stats, l.info)
				continue
			}
			if got := sum(l.panels) + l.stats + l.info; got != width {
				t.Errorf("width %d, %d panels: columns add to %d", width, count, got)
			}
			for _, w := range append([]int{l.stats}, l.panels...) {
				if diff := w - l.info; diff < -1 || diff > 1 {
					t.Errorf("width %d, %d panels: %d vs info %d", width, count, w, l.info)
				}
			}
		}
	}
	// Room for a panel and one more column only: the statistics have it.
	l := computeLayout(2*minPanel+5, 40, 1, 0, 0, true)
	if l.visible() != 1 || l.stats == 0 || l.info != 0 {
		t.Errorf("narrow: %d panels, stats %d, info %d", l.visible(), l.stats, l.info)
	}
}
