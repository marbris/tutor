package ui

import (
	"strings"

	"ttr/internal/keymap"
	"ttr/internal/stats"
)

// What works right now.
//
// One source for every hint on screen. There used to be two: the reference asked each view what its keys
// were, and the bar had three hardcoded strings picked by a three-way
// switch. So the bar offered `s statistics` in a decks panel, where there is
// nothing to count, and offered `a/x/t` on a deck borrowed from Moxfield,
// where all three refuse — while saying nothing at all about `a`, `y`, `w`
// or `gv` on a search result, which fell through to the default branch.
//
// A hint that lies is worse than no hint, and two lists describing one
// keymap is how one of them comes to lie. There is one now: hintGroups.
//
// The keys are grouped — navigation, select, edit, info panel — so each
// block can lead each line with what it is rather than running the whole
// keymap together. Each view declares its own groups; the workspace folds in the
// keys it owns everywhere (the bar, the edit target, the information panel,
// and the way between and out of panels), so no view repeats them and none
// can offer esc under two different names.

// With ? on, the keys are drawn where they act: at the bottom of the focused
// panel, the keys for that panel; at the bottom of the editing deck, the keys
// that change it; at the bottom of the information panel, the keys for what
// it shows; and along the bottom of the screen, the leader's menu. Without
// it, the footer keeps only ? and q — and, while a search bar has the
// cursor, the bar's own keys.

// contextKeys is every key that does something where you are, flattened out
// of the groups.
func (m Model) contextKeys() [][2]string {
	var out [][2]string
	for _, g := range m.hintGroups() {
		out = append(out, g.keys...)
	}
	return out
}

// hint is one entry for the hint bar: the keys some actions are on, and
// what they do. The keys come from the keymap, so they are the ones in force.
func hint(what string, s keymap.Scope, actions ...keymap.Action) [2]string {
	return [2]string{keymap.Hint(s, actions...), what}
}

// listHint is a hint for the movement every list shares.
func listHint(what string, actions ...keymap.Action) [2]string {
	return hint(what, keymap.List, actions...)
}

// gotoHint is a g-prefixed key as it is typed: "gv".
func gotoHint(a keymap.Action) string {
	return seqHint(keymap.Hint(keymap.Global, keymap.GlobalGoto), "", keymap.Hint(keymap.Goto, a))
}

// leaderHint is a leader key as the hints write it: "space f".
func leaderHint(a keymap.Action) string {
	return seqHint(keymap.Hint(keymap.Global, keymap.GlobalLeader), " ", keymap.Hint(keymap.Leader, a))
}

// topBottomHint is the pair for the ends of a list: "gg G".
func topBottomHint() string {
	top := gotoHint(keymap.GotoTop)
	bottom := keymap.Hint(keymap.List, keymap.ListBottom)
	if top == "" || bottom == "" {
		return top + bottom
	}
	return top + " " + bottom
}

// seqHint is a prefix and the key after it, or nothing if either is unbound:
// half a sequence can't be typed.
func seqHint(prefix, sep, key string) string {
	if prefix == "" || key == "" {
		return ""
	}
	return prefix + sep + key
}

// dropUnbound takes out the hints for keys that have been unbound in
// keys.json, and any group left empty by it.
func dropUnbound(groups []hintGroup) []hintGroup {
	var out []hintGroup
	for _, g := range groups {
		var keys [][2]string
		for _, k := range g.keys {
			if k[0] != "" {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			out = append(out, hintGroup{g.title, keys})
		}
	}
	return out
}

// restingKeys are the keys the footer always shows: the way to everything
// else, and the way out.
func restingKeys() [][2]string {
	return [][2]string{
		hint("keys", keymap.Global, keymap.GlobalHelp),
		hint("quit", keymap.Global, keymap.GlobalQuit),
	}
}

// hintGroups is the whole keymap for where you are: the focused panel's
// keys, the information panel's, and the footer's. The panels draw their
// shares of it; this is the union, for the tests that check them against
// each other.
func (m Model) hintGroups() []hintGroup {
	p := m.ws.current()
	if p == nil {
		return dropUnbound([]hintGroup{{"", restingKeys()}})
	}

	// While the bar has the cursor it has every key, so nothing else is
	// worth offering: a printable key types rather than acting.
	if p.searchOpen && p.search.Focused() {
		return dropUnbound([]hintGroup{{"search", m.barKeys(p)}})
	}
	// The i bar likewise, whose tab means what's being typed allows.
	if p.asking == askAddCard || p.asking == askOtag {
		return []hintGroup{{"", addBarKeys(p)}}
	}

	groups := m.panelHintGroups(p)
	if keys := m.editHints(); len(keys) > 0 {
		groups = append(groups, hintGroup{"edit", keys})
	}
	groups = append(groups, m.infoHintGroups()...)
	return dropUnbound(append(groups, hintGroup{"", restingKeys()}))
}

// statsTaken is every key the statistics claim while they're up, as the
// hints write them. The list behind them still has its other keys, so its
// hints stay — less these.
func statsTaken() map[string]bool {
	taken := map[string]bool{}
	for _, e := range keymap.Current() {
		if e.Scope == keymap.Stats {
			for _, k := range e.Keys {
				taken[keymap.Display(k)] = true
			}
		}
	}
	return taken
}

// panelHintGroups is what the focused panel shows at its bottom: the way
// about, the view's own keys, and the way out — compact, and without
// headings, since every key in the block is about the panel it sits in.
//
// The view's first group is its moving-about keys, and goes in among the
// workspace's: the order reads as you'd reach for them, from moving, through
// narrowing, to leaving. The rest follow, and e — which chooses the deck the
// edit keys go to — comes last, next to where those keys are.
func (m Model) panelHintGroups(p *panel) []hintGroup {
	statsUp := m.info.mode == infoStats && m.statList() != nil

	var viewNav [][2]string
	var rest []hintGroup
	if v := p.top(); v != nil {
		for i, g := range v.keys() {
			// The information panel's keys are drawn in that panel.
			if g.title == "info panel" {
				continue
			}
			if i == 0 {
				viewNav = g.keys
				continue
			}
			rest = append(rest, hintGroup{"", append([][2]string(nil), g.keys...)})
		}
	}

	var nav [][2]string
	if m.ws.count() > 1 {
		nav = append(nav,
			[2]string{moveKeys(), "down/up/left/right"},
			hint("move panel", keymap.Global, keymap.GlobalMoveLeft, keymap.GlobalMoveRight),
		)
	} else {
		nav = append(nav, listHint("down/up", keymap.ListDown, keymap.ListUp))
	}
	nav = append(nav, [2]string{topBottomHint(), "first/last"})

	// i reaches the panel's search bar — except on a deck, where it fetches
	// a card into it, and on somebody else's deck, where it does neither.
	if i, ok := m.iHint(p); ok {
		nav = append(nav, hint(i, keymap.Global, keymap.GlobalBar))
	}
	nav = append(nav, viewNav...)

	// b clears the narrowings, but only earns a hint while there is one to
	// clear — otherwise it is a key that does nothing, offered next to esc.
	if m.focusNarrowed() {
		nav = append(nav, hint("clear filters/all", keymap.Global, keymap.GlobalClearFilter, keymap.GlobalClearAll))
	}
	esc, _ := (&m).escStep(p)
	nav = append(nav, hint(esc, keymap.Global, keymap.GlobalBack))

	groups := append([]hintGroup{{"", nav}}, rest...)

	// With no deck being edited, e is how to choose one — offered where the
	// focused panel is a list of cards, since a, x and t do nothing from a
	// decks panel or the rules. Once there is one, e sits under it instead:
	// what it moves is which panel that is.
	if p.cardsView() != nil {
		if target := m.ws.editingList(); target == nil || target.deck == nil {
			groups = append(groups, hintGroup{"", [][2]string{
				hint("choose a deck to edit", keymap.Global, keymap.GlobalEditNext, keymap.GlobalEditPrev),
			}})
		}
	}

	if statsUp {
		groups = withoutTaken(groups)
	}
	return dropUnbound(groups)
}

// moveKeys is the four ways to move written as one: "j k h l" — down and up
// the list, left and right along the panels.
func moveKeys() string {
	vertical := keymap.Hint(keymap.List, keymap.ListDown, keymap.ListUp)
	horizontal := keymap.Hint(keymap.Global, keymap.GlobalPanelPrev, keymap.GlobalPanelNext)
	switch {
	case vertical == "":
		return horizontal
	case horizontal == "":
		return vertical
	}
	return vertical + " " + horizontal
}

// withoutTaken drops the keys the statistics have claimed, and any group
// left with nothing in it.
func withoutTaken(groups []hintGroup) []hintGroup {
	statsTaken := statsTaken()
	var out []hintGroup
	for _, g := range groups {
		var keys [][2]string
		for _, k := range g.keys {
			taken := false
			for _, f := range strings.Fields(k[0]) {
				if statsTaken[f] {
					taken = true
				}
			}
			if !taken {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			out = append(out, hintGroup{g.title, keys})
		}
	}
	return out
}

// iHint is what i does in this panel, or false where it does nothing.
func (m Model) iHint(p *panel) (string, bool) {
	if l := p.cardsView(); l != nil && l.deck != nil {
		if !l.deck.Local() {
			return "", false
		}
		return addCardLabel, true
	}
	return p.kind.barLabel(), true
}

// infoHintGroups is what the information panel shows at its bottom: the
// statistics' keys while they're up, otherwise the keys that move within
// what it shows.
func (m Model) infoHintGroups() []hintGroup {
	p := m.ws.current()
	if p == nil || (p.searchOpen && p.search.Focused()) {
		return nil
	}
	if m.info.mode == infoStats && m.statList() != nil {
		return dropUnbound([]hintGroup{{"statistics", m.statsHints()}})
	}
	// Untitled: the keys sit in the panel they move, so a heading naming
	// that panel says nothing.
	if keys := m.infoKeys(p); len(keys) > 0 {
		return dropUnbound([]hintGroup{{"", keys}})
	}
	// A view that reads its own rows into the panel — a rule, a change —
	// declares how to move through them.
	if v := p.top(); v != nil {
		for _, g := range v.keys() {
			if g.title == "info panel" {
				return dropUnbound([]hintGroup{{"", g.keys}})
			}
		}
	}
	return nil
}

// focusNarrowed reports whether the focused list has a narrowing b would
// clear: a text filter, or — for a card list — the statistics category too.
func (m Model) focusNarrowed() bool {
	p := m.ws.current()
	if p == nil {
		return false
	}
	if l := p.cardsView(); l != nil {
		return l.filter != "" || len(l.statFilter) > 0
	}
	switch v := p.top().(type) {
	case *deckList:
		return v.filter != ""
	case *versionList:
		return v.filter != ""
	case *rulesView:
		return v.filter != ""
	}
	return false
}

// addHints appends keys to the group with the given title, making it at the
// end if there isn't one yet.
func addHints(groups []hintGroup, title string, keys ...[2]string) []hintGroup {
	for i := range groups {
		if groups[i].title == title {
			groups[i].keys = append(groups[i].keys, keys...)
			return groups
		}
	}
	return append(groups, hintGroup{title, keys})
}

// barKeys is the search bar's own keymap. tab only means something where
// there is another kind to become, and the history and the ordering belong
// to a card query — the bar that follows a Moxfield user has neither.
func (m Model) barKeys(p *panel) [][2]string {
	out := [][2]string{hint("another target: scryfall, decks, rules", keymap.Search, keymap.SearchNextTarget)}
	if m.canCompleteOtag(p) {
		out[0] = hint("complete the tag", keymap.Search, keymap.SearchNextTarget, keymap.SearchPrevTarget)
	}
	if p.kind == KindFind {
		out = append(out,
			hint("queries you've run", keymap.Search, keymap.SearchHistoryPrev, keymap.SearchHistoryNext),
			hint("result order ↑↓", keymap.Search, keymap.SearchQuerySort, keymap.SearchQueryDir),
		)
	}
	return append(out,
		hint(p.kind.prompt(), keymap.Search, keymap.SearchRun),
		hint("leave the bar", keymap.Search, keymap.SearchBack),
	)
}

// addBarKeys is the i bar's keys. Its keys are the prompt's own, not the
// keymap's, like every one-line prompt's: tab, enter and esc.
func addBarKeys(p *panel) [][2]string {
	tab, enter := "tag by otag instead", "add the card"
	if p.asking == askOtag {
		tab, enter = "add a card instead", "tag the list"
	}
	if addBarTyping(p) {
		tab = "complete the name"
		if p.asking == askOtag {
			tab = "complete the tag"
		}
		return [][2]string{{"tab shift+tab", tab}, {"enter", enter}, {"esc", "leave"}}
	}
	return [][2]string{{"tab", tab}, {"enter", enter}, {"esc", "leave"}}
}

// editHints is the keys that change the editing deck, drawn at the bottom
// of that deck's panel whatever panel you are in. They act on the *editing*
// deck from anywhere — so the deck they change is routinely not the list in
// front of you, which is exactly why they sit under the deck rather than
// under the cursor.
//
// Nothing while no deck is being edited: e, in the focused panel, is how to
// choose one.
func (m Model) editHints() [][2]string {
	target := m.ws.editingList()
	if target == nil || target.deck == nil {
		return nil
	}
	keys := [][2]string{
		hint("add", keymap.Cards, keymap.CardsAdd),
		hint("add + tag latest", keymap.Cards, keymap.CardsAddTagged),
		hint("remove", keymap.Cards, keymap.CardsRemove),
		hint("commander", keymap.Cards, keymap.CardsCommander),
		hint("undo", keymap.Cards, keymap.CardsUndo),
	}
	// e and E move the editing deck on, so they sit under the deck they
	// move away from — but only when there is another deck to move to.
	if m.editableCount() > 1 {
		keys = append(keys, hint("next/prev deck", keymap.Global, keymap.GlobalEditNext, keymap.GlobalEditPrev))
	}
	var out [][2]string
	for _, k := range keys {
		if k[0] != "" {
			out = append(out, k)
		}
	}
	if m.info.mode == infoStats && m.statList() != nil {
		if g := withoutTaken([]hintGroup{{"", out}}); len(g) > 0 {
			return g[0].keys
		}
		return nil
	}
	return out
}

// infoKeys are the information-panel keys for a card list — statistics, and
// the two axes of moving through what the panel shows. The rules panel
// declares its own, since what K and J read there is a rule, not a card.
func (m Model) infoKeys(p *panel) [][2]string {
	if p.cardsView() == nil {
		return nil
	}
	var keys [][2]string
	// The printed history and the picture are put up over the card, and
	// esc takes them down again — back to the card.
	if m.info.mode == infoVersions || m.info.mode == infoImage {
		keys = append(keys, hint("back", keymap.Global, keymap.GlobalBack))
	}
	if m.info.mode == infoImage {
		keys = append(keys, hint("older/newer artwork", keymap.Global,
			keymap.GlobalPrintingOlder, keymap.GlobalPrintingNewer))
	}
	if m.canFlip() {
		keys = append(keys, hint("other face", keymap.Global, keymap.GlobalPrintingFace))
	}
	return append(keys,
		hint("stats", keymap.Global, keymap.GlobalStats),
		hint("editing deck stats", keymap.Global, keymap.GlobalStatsEdit),
		hint("half page", keymap.Global, keymap.GlobalInfoUp, keymap.GlobalInfoDown),
		[2]string{gotoHint(keymap.GotoVersions), "card history"},
		[2]string{gotoHint(keymap.GotoImage), "printing"},
		[2]string{gotoHint(keymap.GotoImageAll), "every card's picture"},
	)
}

// statsHints are the keys while the statistics have them. tab reorders the
// tags, so it is offered only on one, naming the order it would switch to.
func (m Model) statsHints() [][2]string {
	keys := [][2]string{
		hint("category", keymap.Stats, keymap.StatsDown, keymap.StatsUp),
		hint("turn groups", keymap.Stats, keymap.StatsNextGroup, keymap.StatsPrevGroup),
		hint("filter and/or/not", keymap.Stats, keymap.StatsAnd, keymap.StatsOr, keymap.StatsNot),
		hint("remove from filter", keymap.Stats, keymap.StatsDrop),
		hint("clear stats-filter", keymap.Stats, keymap.StatsClear),
		hint("odds", keymap.Stats, keymap.StatsOddsNext, keymap.StatsOddsPrev),
		hint("close", keymap.Stats, keymap.StatsClose),
		hint("back", keymap.Stats, keymap.StatsBack),
	}
	if m.onTags() {
		order := "tags by name"
		if m.stats.tagsByName {
			order = "tags by count"
		}
		keys = append(keys, hint(order, keymap.Stats, keymap.StatsTagOrder))
	}
	if branch, open := m.onBranch(); branch {
		kind := "tags"
		if r, _ := m.statUnder(); r.Group == stats.TypeGroup {
			kind = "types"
		}
		what := "show the " + kind + " under it"
		if open {
			what = "hide the " + kind + " under it"
		}
		keys = append(keys, hint(what, keymap.Stats, keymap.StatsExpand))
	}
	return keys
}

// editableCount is how many decks of yours are open — how many places e and
// E have to go.
func (m Model) editableCount() int {
	n := 0
	for i := range m.ws.panels {
		if m.ws.editable(i) {
			n++
		}
	}
	return n
}
