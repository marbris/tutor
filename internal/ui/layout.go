package ui

// How the row is divided.
//
// The workspace is a row of panels with the information panel pinned to the
// right. The information panel is just another column: it takes the same
// share as each panel, so a lone deck sits beside an information panel half
// the screen wide, three panels each take a quarter, and so on. Every panel
// opened makes every column — the information panel included — narrower.
//
// That holds until they reach a floor, past which narrowing them further
// would leave each one too thin to read a card name in; after that the row
// scrolls instead, and the panels you can't see are still there.

const (
	// minPanel is the narrowest a column is allowed to become, borders
	// included. Below about this, a card row is an abbreviation of an
	// abbreviation and the column stops earning its place. The information
	// panel is held to the same floor.
	minPanel = 16

	// topRows is the two lines above the panels: the keys that apply, and
	// the last thing you did. Always two, so the panels never move.
	topRows = 2
)

// layout is the arithmetic for one frame: how wide everything is, and which
// panels are on screen when they don't all fit.
type layout struct {
	// info is the information panel's width, or 0 when the terminal is too
	// narrow to afford one.
	info int
	// panels are the widths of the visible panels, left to right.
	panels []int
	// first is the index of the leftmost visible panel.
	first int
	// height is the room a panel has, the two lines above it excluded.
	height int
}

// visible is how many panels the layout is showing.
func (l layout) visible() int { return len(l.panels) }

// shows reports whether a panel index is on screen.
func (l layout) shows(i int) bool { return i >= l.first && i < l.first+l.visible() }

// computeLayout divides the terminal between the panels and the information
// panel.
//
// scroll is where the row was scrolled to last frame; it's honoured unless
// it would put the focused panel off screen, because the panel you're typing
// into has to be one you can see.
func computeLayout(width, height, count, focused, scroll int) layout {
	l := layout{height: maxInt(height-topRows, 1)}
	if count == 0 {
		return l
	}

	// Every column, the information panel included, is held to the same
	// floor, so the most columns that fit is the width divided by it.
	maxCols := width / minPanel
	if maxCols < 1 {
		maxCols = 1
	}

	// One column for the information panel, the rest for the panels. When
	// there is only room for a single column the panel wins it: what you're
	// working in beats what you're reading.
	totalCols := count + 1
	if totalCols > maxCols {
		totalCols = maxCols
	}
	visible := totalCols - 1
	if visible < 1 {
		visible = minInt(count, maxCols)
		l.first = clampFirst(scroll, focused, visible, count)
		l.panels = share(width, visible)
		return l
	}

	// Equal shares across the panels and the information panel together, so
	// the panel beside a lone deck and the information panel are the same
	// width. The remainder lands on the leftmost panels; the information
	// panel takes the last, plain share.
	cols := share(width, totalCols)
	l.info = cols[len(cols)-1]
	l.first = clampFirst(scroll, focused, visible, count)
	l.panels = cols[:visible]
	return l
}

// clampFirst keeps the focused panel on screen while otherwise leaving the
// scroll position alone — a row that recentred itself every time focus moved
// would shuffle panels under the cursor for no reason.
func clampFirst(scroll, focused, visible, count int) int {
	first := scroll
	if first > count-visible {
		first = count - visible
	}
	if first < 0 {
		first = 0
	}
	if focused < first {
		first = focused
	}
	if focused >= first+visible {
		first = focused - visible + 1
	}
	return first
}

// share divides a width into n columns, giving the leftmost the remainder so
// the total is exact — a column short of the terminal's width leaves a seam
// down the right-hand side.
func share(total, n int) []int {
	if n <= 0 {
		return nil
	}
	base, extra := total/n, total%n
	out := make([]int, n)
	for i := range out {
		out[i] = base
		if i < extra {
			out[i]++
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
