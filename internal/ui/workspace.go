package ui

// The workspace: a row of panels, one of which has focus, with the
// information panel pinned to the right.
//
// Panels are siblings, not a stack. Opening one doesn't hide another, and
// closing one doesn't reveal what was underneath — there is nothing
// underneath. That is the whole difference from the screen-and-back-stack
// arrangement this replaces.

type workspace struct {
	panels  []*panel
	focused int // index into panels; meaningless when there are none
	// editing is the panel whose deck is the target of a/x/t. Derived
	// rather than chosen: see editingPanel.
	editing int

	// scroll is the leftmost visible panel when they don't all fit.
	scroll int

	// nextID hands out panel identities, which outlive a panel's position.
	nextID int

	// closed is the panels shut with c or esc, newest last, so space u can
	// bring the last one back where it was. They are kept whole — the same
	// panel, lifted out of the row — so restoring one costs no re-fetch.
	closed []closedPanel

	width, height int
}

// closedPanel remembers a shut panel and where it sat, for undo-close.
type closedPanel struct {
	panel   *panel
	at      int
	editing bool
}

// closedLimit caps how far back undo-close reaches — deep enough for the
// mistakes you actually make, shallow enough not to pin every panel you have
// ever closed in memory.
const closedLimit = 20

func newWorkspace() workspace {
	return workspace{editing: -1}
}

func (w workspace) count() int { return len(w.panels) }

func (w workspace) empty() bool { return len(w.panels) == 0 }

// current is the focused panel, or nil when the splash is up.
func (w *workspace) current() *panel {
	if w.empty() {
		return nil
	}
	return w.panels[w.focused]
}

// ── Opening and closing ─────────────────────────────────────────

// open puts a new panel to the right of the focused one and moves focus
// into it. To the right rather than at the end, because a panel is opened
// in the middle of doing something with the one you're on, and it belongs
// beside it.
func (w *workspace) open(kind Kind) *panel {
	w.nextID++
	p := newPanel(kind)
	p.id = w.nextID
	at := w.focused + 1
	if w.empty() {
		at = 0
	}
	w.panels = append(w.panels, nil)
	copy(w.panels[at+1:], w.panels[at:])
	w.panels[at] = p
	w.focus(at)
	return p
}

// indexOf is where a panel sits in the row.
func (w *workspace) indexOf(p *panel) int {
	for i, other := range w.panels {
		if other == p {
			return i
		}
	}
	return w.focused
}

// deriveEditingIfUnpinned re-checks the target after something has changed
// what the panels hold.
func (w *workspace) deriveEditingIfUnpinned() { w.deriveEditing() }

// byID finds a panel that a request was started from, or nil if it has since
// been closed.
func (w *workspace) byID(id int) *panel {
	for _, p := range w.panels {
		if p.id == id {
			return p
		}
	}
	return nil
}

// close removes the focused panel. Focus goes to its left neighbour, which
// is where you came from if you opened this one.
func (w *workspace) close() {
	if w.empty() {
		return
	}
	at := w.focused

	// Remember it whole, so space u can bring it back where it was rather than
	// re-opening an empty panel and re-running the search.
	w.closed = append(w.closed, closedPanel{
		panel: w.panels[at], at: at, editing: w.editing == at,
	})
	if len(w.closed) > closedLimit {
		w.closed = w.closed[len(w.closed)-closedLimit:]
	}

	w.panels = append(w.panels[:at], w.panels[at+1:]...)

	if w.editing == at {
		w.editing = -1
	} else if w.editing > at {
		w.editing--
	}

	if at >= len(w.panels) {
		at = len(w.panels) - 1
	}
	w.focus(maxInt(at, 0))
}

// restoreClosed brings back the last panel closed, at the spot it held, with
// focus — the counterpart to shutting one by mistake. Nothing to bring back is
// a no-op, not an error.
func (w *workspace) restoreClosed() {
	n := len(w.closed)
	if n == 0 {
		return
	}
	last := w.closed[n-1]
	w.closed = w.closed[:n-1]

	at := last.at
	if at > len(w.panels) {
		at = len(w.panels)
	}
	if at < 0 {
		at = 0
	}

	// Inserting at `at` shifts everything from there up by one; the editing
	// index has to move with it before the panel takes its place.
	if w.editing >= at {
		w.editing++
	}
	w.panels = append(w.panels, nil)
	copy(w.panels[at+1:], w.panels[at:])
	w.panels[at] = last.panel

	if last.editing {
		w.editing = at
	}
	w.focus(at)
}

// only closes everything but the focused panel.
func (w *workspace) only() {
	if w.empty() {
		return
	}
	keep := w.panels[w.focused]
	wasEditing := w.editing == w.focused

	// Each panel it closes is remembered as if closed one at a time — the
	// ones to the right from the far end in, then the ones to the left — so
	// space u brings them back one by one, each where it was.
	var order []int
	for i := len(w.panels) - 1; i > w.focused; i-- {
		order = append(order, i)
	}
	for i := w.focused - 1; i >= 0; i-- {
		order = append(order, i)
	}
	for _, i := range order {
		w.closed = append(w.closed, closedPanel{panel: w.panels[i], at: i, editing: w.editing == i})
	}
	if len(w.closed) > closedLimit {
		w.closed = w.closed[len(w.closed)-closedLimit:]
	}

	w.panels = []*panel{keep}
	w.focused, w.scroll = 0, 0
	if wasEditing {
		w.editing = 0
	} else {
		w.editing = -1
	}
}

// ── Moving around ───────────────────────────────────────────────

// focus moves to a panel by index, ignoring one that isn't there. The
// search bar's cursor follows, so it only ever blinks in one place.
func (w *workspace) focus(at int) {
	if at < 0 || at >= len(w.panels) {
		return
	}
	for i, p := range w.panels {
		if i == at && p.searchOpen {
			p.search.Focus()
		} else {
			p.search.Blur()
		}
	}
	w.focused = at
	// Focus no longer chooses the editing deck; it only fills a hole.
	w.deriveEditing()
}

// step moves focus along the row. It stops at the ends rather than wrapping:
// the panels are laid out in space, and a left that lands you on the far
// right is a surprise every time.
func (w *workspace) step(delta int) {
	if w.empty() {
		return
	}
	at := w.focused + delta
	if at < 0 || at >= len(w.panels) {
		return
	}
	w.focus(at)
}

// movePanel carries the focused panel along the row, focus going with it.
func (w *workspace) movePanel(delta int) {
	if w.empty() {
		return
	}
	at := w.focused
	to := at + delta
	if to < 0 || to >= len(w.panels) {
		return
	}
	w.panels[at], w.panels[to] = w.panels[to], w.panels[at]

	switch w.editing {
	case at:
		w.editing = to
	case to:
		w.editing = at
	}
	w.focused = to
}

// ── The editing deck ────────────────────────────────────────────

// deriveEditing keeps the target valid without choosing it for you.
//
// It was originally the other way round: the editing deck followed whichever
// local deck you last looked at. That turns out to be wrong, and obviously so
// once you use it — moving the cursor to read a deck silently redirects
// a/x/t at it, and the one thing this target must be is predictable. So it
// only ever fills a hole: nothing is editing, and exactly one deck is open.
// Anything else is e's business.
func (w *workspace) deriveEditing() {
	if w.editable(w.editing) || w.awaitingDeck(w.editing) {
		// Already have one — or one is on its way, which is what a restored
		// session looks like for the moment before the cards arrive.
		return
	}

	w.editing = -1
	found := -1
	for i := range w.panels {
		if !w.editable(i) {
			continue
		}
		if found >= 0 {
			return // several to choose from; e chooses
		}
		found = i
	}
	w.editing = found
}

// cycleEditing moves the target to the next open deck of yours. e and E walk
// it in either direction, and the row it lands on is the one a/x/t write to.
func (w *workspace) cycleEditing(delta int) {
	var decks []int
	for i := range w.panels {
		if w.editable(i) {
			decks = append(decks, i)
		}
	}
	if len(decks) == 0 {
		w.editing = -1
		return
	}

	at := -1
	for i, p := range decks {
		if p == w.editing {
			at = i
			break
		}
	}
	// Not currently on one: e goes to the first, E to the last, so a single
	// press always lands somewhere rather than needing two.
	if at < 0 {
		if delta > 0 {
			w.editing = decks[0]
		} else {
			w.editing = decks[len(decks)-1]
		}
		return
	}

	n := len(decks)
	w.editing = decks[((at+delta)%n+n)%n]
}

// awaitingDeck reports whether a panel is in the middle of opening a deck,
// so a target chosen before it arrives isn't thrown away.
func (w *workspace) awaitingDeck(i int) bool {
	if i < 0 || i >= len(w.panels) {
		return false
	}
	return w.panels[i].loading && w.panels[i].kind == KindDecks
}

// editable reports whether a panel holds a local deck, the only thing that
// can be edited.
func (w *workspace) editable(i int) bool {
	if i < 0 || i >= len(w.panels) {
		return false
	}
	l := w.panels[i].cardsView()
	return l != nil && l.deck != nil && l.deck.Local()
}

// editingList is the cards of the editing panel, or nil when there is no
// deck being edited.
func (w *workspace) editingList() *cardList {
	p := w.editingPanel()
	if p == nil {
		return nil
	}
	return p.cardsView()
}

// editingPanel is the panel that list is in, which is what saving needs: the
// result comes back addressed to a panel.
func (w *workspace) editingPanel() *panel {
	if w.editing < 0 || w.editing >= len(w.panels) {
		return nil
	}
	return w.panels[w.editing]
}

// ── Layout ──────────────────────────────────────────────────────

// layout works out this frame's widths and remembers where the row was
// scrolled to, so the next frame starts from the same place.
func (w *workspace) layout() layout { return w.layoutWithTop(topRows) }

// layoutWithTop is the same, told how many rows the top of the screen is
// taking. That is two, except for a leader menu too wide for two lines on a
// narrow terminal: then the panels give up the room rather than being pushed
// off the bottom.
func (w *workspace) layoutWithTop(top int) layout {
	l := computeLayout(w.width, w.height-(top-topRows), w.count(), w.focused, w.scroll)
	w.scroll = l.first
	return l
}
