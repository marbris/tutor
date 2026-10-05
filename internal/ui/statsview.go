package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ttr/internal/catalog"
	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/stats"
	"ttr/internal/tagger"
	"ttr/internal/theme"
)

// Statistics, in the information panel.
//
// s takes you there and s brings you back: while the statistics are up they
// have the keys, and j/k walk the categories without touching the list. A
// category narrows the list only when you add it — a to add it with AND, o
// with OR — so reading the bars and filtering by them are separate acts, and
// several categories can narrow at once. h and l still move between lists,
// and the bars follow.
//
// The bars all share one scale, so a glance compares them. Scaling each
// group to its own widest bar would make a deck with two of something look
// like a deck full of it.

// statsState is only the facts a frame can't work out for itself, and the
// bars as last counted. The bars are a function of the cards, the tags and
// how they're narrowed, so they're kept only under a key naming all of
// those (statsKey): j and k, which change none of it, draw from the kept
// bars instead of counting the whole list, Tagger tree and all, several
// times a key — which, with a key held down, queued keys faster than they
// were drawn, and the highlight ran on after the key came up.
type statsState struct {
	// group and label name the highlighted category. A name rather than an
	// index, so the highlight survives the rows being recounted, reordered
	// or swapped for another list's; when the name isn't there any more it
	// falls back to the first row.
	group, label string
	// path is the highlighted row's place in the Scryfall Tagger tree,
	// where the same tag can sit under two parents.
	path string
	// top is the group drawn first. J and K turn the order over, so the
	// group you want to read sits at the top rather than off the bottom.
	top string
	// editing is S: the deck you're editing, wherever you are.
	editing bool
	// odds is 0 for counts, or n for the chance of at least n in the
	// opening hand. p and P step it.
	odds int
	// tagsByName is tab on a tag: the tags alphabetical rather than
	// commonest first.
	tagsByName bool
	// openTags is the Scryfall Tagger rows opened with enter, by path.
	openTags map[string]bool
	// openGen counts enter on a Tagger row, which changes the rows in place.
	openGen int
	// memo is the bars as last counted. A pointer, so every copy of the
	// Model shares it; nil counts afresh every time.
	memo *statsMemo
}

// statsKey is everything the bars are counted from.
type statsKey struct {
	list       *cardList
	gen        int // the list's refreshes: its cards and filters
	tagGen     int // the global tags' rebuilds
	tagger     *tagger.Data
	catalog    *catalog.Data // where each subtype counts
	tagsByName bool
	open       int // openGen
	openLen    int
}

type statsMemo struct {
	key    statsKey
	groups []stats.Group
	ok     bool

	// The bars as last drawn, with no row highlighted: drawing them is most
	// of what a frame costs, and walking them changes only two rows.
	drawnKey drawKey
	drawn    []string
	bars     []statBar // each row's bar, to draw the highlighted one
	barLine  []int     // which line of drawn each row is on
	drawnOK  bool
}

// drawKey is everything the drawn bars depend on beyond the counting.
type drawKey struct {
	counted statsKey
	top     string
	width   int
	odds    int
	expr    string
	hints   bool
}

const maxOdds = 4

// statRows flattens the groups into the rows j and k step through.
func statRows(groups []stats.Group) []stats.Row {
	var out []stats.Row
	for _, g := range groups {
		out = append(out, g.Rows...)
	}
	return out
}

// statGroups counts the cards, in the order they're drawn. The source decides
// which categories exist — the whole list — while the counted set is what the
// numbers describe, so a category the narrowing has emptied stays put and
// reads zero rather than vanishing under the cursor.
func (m Model) statGroups() []stats.Group {
	return rotateGroups(m.countedGroups(), m.stats.top)
}

// countedGroups is the groups in their own order, before J or K turns them:
// the kept count when nothing it was counted from has changed.
func (m Model) countedGroups() []stats.Group {
	l := m.statList()
	key := statsKey{
		list: l, tagGen: globalTags.gen, tagger: tagger.Current(), catalog: catalog.Current(),
		tagsByName: m.stats.tagsByName, open: m.stats.openGen, openLen: len(m.stats.openTags),
	}
	if l != nil {
		key.gen = l.gen
	}
	memo := m.stats.memo
	if memo != nil && memo.ok && memo.key == key {
		return memo.groups
	}
	counted, source := m.statCards()
	groups := stats.GroupsBy(source, counted, m.stats.tagsByName, m.stats.openTags)
	if memo != nil && l != nil {
		memo.key, memo.groups, memo.ok = key, groups, true
	}
	return groups
}

// rotateGroups turns the groups over so top comes first, the rest following
// in their usual order and wrapping round. A top that isn't there — a list
// with no tags has no Tags — starts at the next group that is.
func rotateGroups(groups []stats.Group, top string) []stats.Group {
	if len(groups) == 0 || top == "" {
		return groups
	}
	start := groupIndex(top)
	if start < 0 {
		return groups
	}
	var out []stats.Group
	for i := range stats.GroupOrder {
		title := stats.GroupOrder[(start+i)%len(stats.GroupOrder)]
		for _, g := range groups {
			if g.Title == title {
				out = append(out, g)
			}
		}
	}
	return out
}

func groupIndex(title string) int {
	for i, t := range stats.GroupOrder {
		if t == title {
			return i
		}
	}
	return -1
}

// statCards is what the statistics describe: the focused list, or the
// editing deck when S asked for it.
func (m Model) statCards() (counted, source []deck.Card) {
	if l := m.statList(); l != nil {
		return effectiveAll(l.narrowed(), l.lenderKey()), effectiveAll(l.all, l.lenderKey())
	}
	return nil, nil
}

// statList is the list the statistics describe and narrow.
func (m Model) statList() *cardList {
	if m.stats.editing {
		return m.ws.editingList()
	}
	p := m.ws.current()
	if p == nil {
		return nil
	}
	return p.cardsView()
}

// statCursor is the flattened index of the highlighted category in the rows
// as drawn — the first row when the highlight names one that isn't there.
func (m Model) statCursor(groups []stats.Group) int {
	for i, r := range statRows(groups) {
		if r.Group == m.stats.group && r.Label == m.stats.label && r.Path == m.stats.path {
			return i
		}
	}
	return 0
}

// statUnder is the highlighted category, if there's anything to highlight.
func (m Model) statUnder() (stats.Row, bool) {
	groups := m.statGroups()
	rows := statRows(groups)
	if len(rows) == 0 {
		return stats.Row{}, false
	}
	return rows[m.statCursor(groups)], true
}

// onTags reports whether the highlight is on a tag, where tab reorders them.
func (m Model) onTags() bool {
	r, ok := m.statUnder()
	return ok && r.Group == "Tags"
}

func (m *Model) pointAt(r stats.Row) {
	m.stats.group, m.stats.label, m.stats.path = r.Group, r.Label, r.Path
}

// onBranch reports whether the highlight is on a Scryfall Tagger row with
// rows under it, and whether they are showing.
func (m Model) onBranch() (branch, open bool) {
	r, ok := m.statUnder()
	if !ok || !r.Expandable {
		return false, false
	}
	return true, m.stats.openTags[r.Path]
}

// toggleBranch is enter on a Scryfall Tagger row: its children shown under
// it, or put away again.
func (m *Model) toggleBranch() bool {
	r, ok := m.statUnder()
	if !ok || !r.Expandable {
		return false
	}
	if m.stats.openTags == nil {
		m.stats.openTags = map[string]bool{}
	}
	if m.stats.openTags[r.Path] {
		delete(m.stats.openTags, r.Path)
	} else {
		m.stats.openTags[r.Path] = true
	}
	m.stats.openGen++
	return true
}

// moveStat walks the categories. Only moving; nothing narrows until you add.
//
// The groups are a ring, so the walk never stops. k on the first row turns
// the ring back a group, the way K does, and lands on the last row of the
// group that comes round to the top — the row that was out of sight above.
// j on the last row turns it forward, and lands on the first row of the
// group that has gone round to the bottom.
func (m *Model) moveStat(delta int) {
	groups := m.statGroups()
	rows := statRows(groups)
	if len(rows) == 0 {
		return
	}
	at := m.statCursor(groups) + delta
	switch {
	case at < 0:
		m.rotateStat(-1)
		top := m.statGroups()[0]
		m.pointAt(top.Rows[len(top.Rows)-1])
	case at >= len(rows):
		m.rotateStat(1)
		groups = m.statGroups()
		m.pointAt(groups[len(groups)-1].Rows[0])
	default:
		m.pointAt(rows[at])
	}
}

// rotateStat turns the group order over by one: J brings the next group to
// the top, K the one before. The highlight goes with it, to the new top.
func (m *Model) rotateStat(delta int) {
	groups := m.countedGroups()
	if len(groups) == 0 {
		return
	}
	// Where the present top sits among the groups this list has.
	cur := 0
	if top := rotateGroups(groups, m.stats.top); len(top) > 0 {
		for i, g := range groups {
			if g.Title == top[0].Title {
				cur = i
			}
		}
	}
	next := groups[((cur+delta)%len(groups)+len(groups))%len(groups)]
	m.stats.top = next.Title
	m.pointAt(next.Rows[0])
}

// addStat narrows the list by the highlighted category, joined to whatever
// already narrows it with op.
func (m *Model) addStat(op stats.Op) {
	l := m.statList()
	r, ok := m.statUnder()
	if l == nil || !ok {
		return
	}
	expr, added := l.statFilter.Add(op, r)
	if !added {
		m.notice = r.Label + " is already in the filter"
		return
	}
	l.statFilter = expr
	l.refresh()
}

// dropStat takes the highlighted category out of the narrowing, leaving
// the others — x on mana value 1 keeps mana value 2. x.
func (m *Model) dropStat() {
	l := m.statList()
	r, ok := m.statUnder()
	if l == nil || !ok {
		return
	}
	if !l.statFilter.Has(r) {
		m.notice = r.Label + " isn't in the filter"
		return
	}
	l.statFilter = l.statFilter.Without(r)
	l.refresh()
}

// clearStatFilter drops the statistics narrowing from one list.
func clearStatFilter(l *cardList) bool {
	if l == nil || len(l.statFilter) == 0 {
		return false
	}
	l.statFilter = nil
	l.refresh()
	return true
}

// clearActiveFilters drops the narrowings on the focused list alone: its text
// filter and the statistics categories. b.
func (m *Model) clearActiveFilters() {
	p := m.ws.current()
	if p == nil {
		return
	}
	clearStatFilter(p.cardsView())
	if v, ok := p.top().(filterable); ok {
		v.setFilter("")
	}
}

// clearAllFilters drops every narrowing on every list — the statistics
// categories and the text filters alike. B.
func (m *Model) clearAllFilters() {
	for _, p := range m.ws.panels {
		clearStatFilter(p.cardsView())
		if v, ok := p.top().(filterable); ok {
			v.setFilter("")
		}
	}
}

// stepOdds moves between counts and the opening-hand odds: counts, then at
// least one, two, three, four, and round to counts again.
func (m *Model) stepOdds(delta int) {
	m.stats.odds = ((m.stats.odds+delta)%(maxOdds+1) + maxOdds + 1) % (maxOdds + 1)
}

// toggleStats opens the statistics or closes them. The narrowing lives on
// the list, so closing leaves it in place and the panel's subtitle still
// names it.
func (m *Model) toggleStats(editing bool) {
	if m.info.mode == infoStats && m.stats.editing == editing {
		m.info.mode = infoCard
		return
	}
	if editing && m.ws.editingList() == nil {
		m.notice = "no deck is being edited — " + keymap.Hint(keymap.Global, keymap.GlobalEditNext) + " chooses one"
		return
	}
	m.info.mode = infoStats
	m.stats.editing = editing
	m.info.offset = 0
}

// statsKey is the keymap while the statistics are up. It returns false for
// anything it leaves to the workspace — h and l among them, which still move
// between lists.
func (m *Model) statsKey(key string) bool {
	switch keymap.Lookup(keymap.Stats, key) {
	case keymap.StatsDown:
		m.moveStat(1)
	case keymap.StatsUp:
		m.moveStat(-1)
	case keymap.StatsNextGroup:
		m.rotateStat(1)
	case keymap.StatsPrevGroup:
		m.rotateStat(-1)
	case keymap.StatsOr:
		m.addStat(stats.Or)
	case keymap.StatsAnd:
		m.addStat(stats.And)
	case keymap.StatsNot:
		m.addStat(stats.AndNot)
	case keymap.StatsDrop:
		m.dropStat()
	case keymap.StatsClear:
		clearStatFilter(m.statList())
	case keymap.StatsOddsNext:
		m.stepOdds(1)
	case keymap.StatsOddsPrev:
		m.stepOdds(-1)
	case keymap.StatsClose:
		m.info.mode = infoCard
	case keymap.StatsExpand:
		// Only on a tag with tags under it.
		if !m.toggleBranch() {
			return false
		}
	case keymap.StatsTagOrder:
		// Only on a tag: elsewhere tab would reorder rows you can't see.
		if !m.onTags() {
			return false
		}
		// The highlight stays on its tag as the tags move round it.
		if r, ok := m.statUnder(); ok {
			m.pointAt(r)
		}
		m.stats.tagsByName = !m.stats.tagsByName
	case keymap.StatsBack:
		// Back, a step at a time: the categories, then the text filter,
		// then out of the statistics.
		l := m.statList()
		switch {
		case clearStatFilter(l):
		case l != nil && l.filter != "":
			l.setFilter("")
		default:
			m.info.mode = infoCard
		}
	default:
		return false
	}
	return true
}

// ── Drawing ─────────────────────────────────────────────────────

// statLine is which rendered line a category sits on, counting the group
// headings and the blank lines between them — which is what the panel has to
// scroll by.
func statLine(groups []stats.Group, row int) int {
	if row < 0 {
		return 0
	}
	line, at := 0, 0
	for _, g := range groups {
		if len(g.Rows) == 0 {
			continue
		}
		if line > 0 {
			line++ // the blank line between groups
		}
		line++ // the heading
		for range g.Rows {
			if at == row {
				return line
			}
			at++
			line++
		}
	}
	return line
}

// statHeading is which rendered line the heading of a category's group sits
// on.
func statHeading(groups []stats.Group, row int) int {
	line, at := 0, 0
	for _, g := range groups {
		if len(g.Rows) == 0 {
			continue
		}
		if line > 0 {
			line++
		}
		heading := line
		line += 1 + len(g.Rows)
		if row < at+len(g.Rows) {
			return heading
		}
		at += len(g.Rows)
	}
	return 0
}

// statScroll is where the statistics are scrolled to, from offset: moved
// only as far as it takes to keep the highlighted category in view, so the
// view holds still while the highlight walks inside it and moves only when
// the highlight reaches an edge.
//
// At the top edge it goes far enough back to show the group's heading too,
// whenever the two fit together. J and K turn a group to the top and put
// the highlight on its first category; scrolling only as far as the
// category left the heading — the one line saying what the bars count —
// just above the top edge.
func (m Model) statScroll(offset, height, total int) int {
	groups := m.statGroups()
	row := m.statCursor(groups)
	line := statLine(groups, row)
	offset = scrollTo(line, offset, height, total)
	if head := statHeading(groups, row); line == offset && head < offset && line-head < height {
		offset = head
	}
	return offset
}

// followStats keeps the statistics' scroll position, after anything that
// may have moved the highlight. The position is kept rather than worked out
// afresh each frame: worked out afresh, it was always the least scroll that
// showed the highlight, so walking back up from the bottom dragged the view
// along a line at a time.
func (m *Model) followStats() {
	if m.info.mode != infoStats {
		return
	}
	inner, room, _, ok := m.infoSpan()
	if !ok {
		return
	}
	m.info.offset = m.statScroll(m.info.offset, room, len(m.renderStats(inner)))
}

// statOdds is the chance of at least n of a category in the opening hand,
// drawn from the whole list.
func statOdds(r stats.Row, pop, n int) float64 {
	return stats.AtLeast(pop, r.Base, n, stats.HandSize)
}

// statPop is how many cards the odds are drawn from: the whole list,
// unnarrowed.
func (m Model) statPop() int {
	_, source := m.statCards()
	n := 0
	for _, c := range source {
		n += max(c.Qty, 1)
	}
	return n
}

// renderStats draws the groups as horizontal bars.
func (m Model) renderStats(width int) []string {
	groups := m.statGroups()
	if len(groups) == 0 {
		return []string{mutedLine("nothing to count", width)}
	}

	var expr stats.Expr
	if l := m.statList(); l != nil {
		expr = l.statFilter
	}

	// One scale across every group, so a glance compares them.
	widest := 0
	for _, g := range groups {
		for _, r := range g.Rows {
			if r.Base > widest {
				widest = r.Base
			}
		}
	}
	if widest == 0 {
		widest = 1
	}

	labelWidth := 0
	for _, g := range groups {
		for _, r := range g.Rows {
			if w := textWidth(m.statLabel(r)); w > labelWidth {
				labelWidth = w
			}
		}
	}
	labelWidth = minInt(labelWidth, maxInt(width/2, 6))

	countWidth := textWidth(itoa(widest))
	if m.stats.odds > 0 {
		countWidth = len("100%")
	}
	// Two columns for the and/or mark, beside the label.
	barWidth := maxInt(width-labelWidth-countWidth-4, 1)

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	cursor := m.statCursor(groups)
	memo := m.stats.memo
	key := drawKey{counted: memo.keyOrZero(), top: m.stats.top, width: width, odds: m.stats.odds,
		expr: expr.String(), hints: m.hintsExpanded}
	if memo != nil && memo.ok && memo.drawnOK && memo.drawnKey == key {
		return memo.highlight(cursor, m.tagMeaning(width))
	}

	pop := m.statPop()
	var out []string
	var bars []statBar
	var barLine []int
	at := 0
	for _, g := range groups {
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, head.Render(fit(g.Title, width)))
		for _, r := range g.Rows {
			bar := statBar{row: r, label: m.statLabel(r), mark: statMark(expr, r),
				labelWidth: labelWidth, barWidth: barWidth, countWidth: countWidth}
			if m.stats.odds > 0 {
				bar.fraction = statOdds(r, pop, m.stats.odds)
				bar.value = fmt.Sprintf("%.0f%%", bar.fraction*100)
			} else {
				bar.fraction = float64(r.Count) / float64(widest)
				bar.value = itoa(r.Count)
			}
			bars = append(bars, bar)
			barLine = append(barLine, len(out))
			out = append(out, bar.render())
			at++
		}
	}

	out = append(out, "")
	if len(expr) > 0 {
		out = append(out, lipgloss.NewStyle().Foreground(theme.Marked).
			Render(fit("filter: "+expr.String(), width)))
	}
	// The short reminder, for when ? isn't drawing the whole keymap below.
	if !m.hintsExpanded {
		out = append(out, dim.Render(fit("a/o/n and/or/not · x remove · p odds · s back", width)))
	}
	if memo == nil || !memo.ok {
		d := statsMemo{drawn: out, bars: bars, barLine: barLine}
		return d.highlight(cursor, m.tagMeaning(width))
	}
	memo.drawnKey, memo.drawn, memo.bars, memo.barLine, memo.drawnOK = key, out, bars, barLine, true
	return memo.highlight(cursor, m.tagMeaning(width))
}

// keyOrZero is the key the groups were counted under, for drawKey.
func (m *statsMemo) keyOrZero() statsKey {
	if m == nil {
		return statsKey{}
	}
	return m.key
}

// highlight is the drawn bars with row at drawn under the cursor, and
// under it whatever there is to say about it: a copy, so the kept lines
// stay as they were.
func (m *statsMemo) highlight(row int, under []string) []string {
	out := append([]string(nil), m.drawn...)
	if row < 0 || row >= len(m.bars) {
		return out
	}
	b := m.bars[row]
	b.under = true
	at := m.barLine[row]
	out[at] = b.render()
	if len(under) == 0 {
		return out
	}
	return append(out[:at+1], append(under, out[at+1:]...)...)
}

// tagMeaning is what Scryfall Tagger says the highlighted tag means, to
// show under it: a few lines, indented with the row. Most tags have no
// description, and then it says so.
func (m Model) tagMeaning(width int) []string {
	r, ok := m.statUnder()
	if !ok || r.Group != stats.TaggerGroup || r.Label == "untagged" {
		return nil
	}
	text := "no description on Tagger"
	tg := tagger.Current()
	if t, found := tg.Find(r.Label); found && tg.Tags[t].Description != "" {
		text = plainMarkdown(tg.Tags[t].Description)
	}
	indent := strings.Repeat("  ", r.Depth+2)
	lines := wrapStyled(text, maxInt(width-len(indent), 10), lipgloss.NewStyle().Foreground(theme.TextMuted).Italic(true))
	if len(lines) > 3 {
		lines = append(lines[:2], mutedLine("…", width))
	}
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return lines
}

// mdLink is a Markdown link, which Tagger's descriptions use for related
// tags: [spot removal](spot-removal).
var mdLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// plainMarkdown is a description with its links reduced to their text.
func plainMarkdown(s string) string {
	return strings.TrimSpace(mdLink.ReplaceAllString(s, "$1"))
}

// statLabel is a row's name as drawn. A row in the Tagger tree is indented
// by its depth, with ▸ on a tag that has tags under it and ▾ once they show.
func (m Model) statLabel(r stats.Row) string {
	if (r.Group != stats.TaggerGroup && r.Group != stats.TypeGroup) || r.Label == "untagged" {
		return r.Label
	}
	mark := "  "
	if r.Expandable {
		mark = "▸ "
		if m.stats.openTags[r.Path] {
			mark = "▾ "
		}
	}
	return strings.Repeat("  ", r.Depth) + mark + r.Label
}

// statMark is the and/or beside a category that's part of the narrowing.
// The gutter is one cell, so and-not is its ¬ alone — the and goes without
// saying there — where the header, with room, writes ∧¬ in full. Two cells
// in a one-cell gutter wrapped the row and pushed the panel off the screen.
func statMark(expr stats.Expr, r stats.Row) string {
	i := expr.Index(r)
	switch {
	case i < 0:
		return " "
	case expr[i].Op == stats.AndNot:
		return "¬"
	case i == 0:
		return "•"
	default:
		return expr[i].Op.Symbol()
	}
}

type statBar struct {
	row                              stats.Row
	label                            string
	under                            bool
	mark                             string
	fraction                         float64
	value                            string
	labelWidth, barWidth, countWidth int
}

func (b statBar) render() string {
	filled := int(b.fraction * float64(b.barWidth))
	if b.fraction > 0 && filled == 0 {
		filled = 1 // one card should never read as none
	}
	filled = min(filled, b.barWidth)

	full, empty := strings.Repeat("█", filled), strings.Repeat("─", maxInt(b.barWidth-filled, 0))
	label, value := fit(b.label, b.labelWidth), pad(b.value, b.countWidth)

	if b.under {
		// The highlight is the bar's own colour, run under the whole row,
		// with the text and the bar in whichever of dark or light stands out
		// from it — the way the card lists draw their cursor. A neutral grey
		// said which row but not which category.
		return onColour(b.row.Color).Bold(true).
			Render(b.mark + " " + label + " " + full + empty + " " + value)
	}

	bar := lipgloss.NewStyle().Foreground(b.row.Color).Render(full) +
		lipgloss.NewStyle().Foreground(theme.BarEmpty).Render(empty)
	return lipgloss.NewStyle().Foreground(theme.Marked).Bold(true).Render(b.mark) + " " +
		lipgloss.NewStyle().Foreground(theme.Text).Render(label) + " " + bar + " " +
		lipgloss.NewStyle().Foreground(theme.TextDim).Render(value)
}

// statOffset is how far the panel is scrolled for the highlighted category,
// which is the same arithmetic the render does. Exposed so a test can ask
// without drawing.
func (m Model) statOffset(height int) int {
	return m.statScroll(m.info.offset, maxInt(height, 1), len(m.renderStats(30)))
}

// statTitle heads the panel: what's being counted, and how.
func (m Model) statTitle() string {
	t := "statistics"
	if m.stats.editing {
		t += " · editing deck"
	}
	if m.stats.odds > 0 {
		t += fmt.Sprintf(" · P(≥%d in opening %d)", m.stats.odds, stats.HandSize)
	}
	return t
}
