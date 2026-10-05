package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/theme"
)

// Drawing the workspace: the two lines along the top that say what the keys
// do here and what just happened, then the row of panels, and the
// information panel beside them.

// viewWorkspace lays the whole frame out.
func (m Model) viewWorkspace() string {
	ws := m.ws // a copy: layout records the scroll position, and View is a
	l := ws.layoutWithTop(m.topHeight())

	// Every visible panel's header is as tall as the tallest, so the rules
	// under them — and the first rows of cards — line up across the row.
	subHeight := 0
	for i, width := range l.panels {
		subHeight = maxInt(subHeight, len(ws.panels[l.first+i].subLines(maxInt(width-2, 1))))
	}

	var columns []string
	for i, width := range l.panels {
		at := l.first + i
		columns = append(columns, m.viewPanel(ws.panels[at], at, width, l.height, subHeight))
	}
	if l.info > 0 {
		columns = append(columns, m.viewInfo(l.info, l.height))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	return lipgloss.JoinVertical(lipgloss.Left, m.viewTop(), row)
}

// viewPanel draws one panel: a border, its header, and its contents.
// subHeight is how many lines the header under the title takes; the panel
// pads its own to that, so neighbours stay level.
func (m Model) viewPanel(p *panel, index, width, height, subHeight int) string {
	focused := index == m.ws.focused
	editing := index == m.ws.editing

	// The border says which panel has the keys, and which one a/x/t are
	// going to write to — the two things you need to know without looking.
	colour := theme.Border
	switch {
	case focused:
		colour = theme.BorderFocus
	case editing:
		colour = theme.BorderEditing
	}

	inner := maxInt(width-2, 1)
	p.restyle()

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	if !focused {
		head = lipgloss.NewStyle().Foreground(theme.TextDim)
	}

	text, styled := p.header(inner)
	headLine := text
	if !styled {
		headLine = head.Render(fit(text, inner))
	}
	lines := []string{headLine}
	muted := lipgloss.NewStyle().Foreground(theme.TextMuted)
	sub := p.subLines(inner)
	for _, s := range sub {
		lines = append(lines, muted.Render(pad(s, inner)))
	}
	for i := len(sub); i < subHeight; i++ {
		lines = append(lines, strings.Repeat(" ", inner))
	}
	lines = append(lines, lipgloss.NewStyle().
		Foreground(theme.Border).
		Render(strings.Repeat("─", inner)))

	// With ? on, the focused panel's keys sit along its bottom — under the
	// cards they act on, rather than in a bar the width of the screen — and
	// the editing deck's panel carries the keys that change it. One panel
	// can be both, and then has both, each under its own rule.
	var hints []string
	if m.hintsExpanded {
		var blocks [][]hintGroup
		if focused {
			blocks = append(blocks, m.panelHintGroups(p))
		}
		if editing {
			if keys := m.editHints(); len(keys) > 0 {
				blocks = append(blocks, []hintGroup{{"", keys}})
			}
		}
		hints = hintFooter(blocks, inner, height-2-len(lines))
	}
	lines = append(lines, m.viewPanelBody(p, inner, maxInt(height-2-len(lines)-len(hints), 1))...)
	lines = append(lines, hints...)

	body := lipgloss.NewStyle().
		Width(inner).
		Height(maxInt(height-2, 1)).
		MaxWidth(inner).
		Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colour).
		Render(body)
}

// viewPanelBody is what a panel holds: its cards, or — before anything has
// filled it — what it's waiting for.
func (m Model) viewPanelBody(p *panel, width, height int) []string {
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	if p.loading {
		return fillTo([]string{dim.Render(fit("searching…", width))}, width, height)
	}
	if p.err != nil {
		return fillTo([]string{
			lipgloss.NewStyle().Foreground(theme.Error).Render(fit(errorText(p.err), width)),
		}, width, height)
	}

	if v := p.top(); v != nil {
		focused := m.ws.panels[m.ws.focused] == p
		return v.lines(width, height, focused, &m)
	}

	// Out of the bar (esc leaves it, and no longer closes the panel), say
	// how to get back into it.
	how := "type a " + p.kind.prompt()
	if !p.searchOpen {
		how = keymap.Hint(keymap.Global, keymap.GlobalBar) + " to search · " +
			leaderHint(keymap.LeaderClose) + " to close"
	}
	return fillTo([]string{
		dim.Render(fit("nothing here yet", width)),
		"",
		dim.Render(fit(how, width)),
	}, width, height)
}

// fillTo pads a block out to the height it has to occupy, so the panel below
// it doesn't collapse around short content.
func fillTo(lines []string, width, height int) []string {
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// membership is how a card in one list relates to the others on screen:
// which other list it is also in, the strongest of them if several. Each is
// marked in the colour of that list's border, so a dot says which panel.
type membership int

const (
	notElsewhere membership = iota
	// inOther is in some list that is neither of the two below: grey, like
	// the border of a panel that is neither.
	inOther
	// inFocused is in the list you're in: orange, like its border.
	inFocused
	// inEditing is in the deck being edited: aqua, like its border. It
	// outranks the others — whether a card is already in the deck is the
	// question every list is being read to answer.
	inEditing
)

// membersFor is what a list should flag as living somewhere else too, and
// where. A list never marks itself: the editing deck marks what the focused
// list has, and the focused list marks what the editing deck has.
func (m Model) membersFor(l *cardList) map[string]membership {
	editing := m.ws.editingList()
	var focused *cardList
	if p := m.ws.current(); p != nil {
		focused = p.cardsView()
	}

	out := map[string]membership{}
	for _, other := range m.ws.panels {
		o := other.cardsView()
		if o == nil || o == l {
			continue
		}
		level := inOther
		switch o {
		case editing:
			level = inEditing
		case focused:
			level = inFocused
		}
		for name := range o.names() {
			if level > out[name] {
				out[name] = level
			}
		}
	}
	return out
}

// resortInclusion keeps every list's idea of where else its cards are
// current, and re-sorts the lists ordered by it whose cards have moved
// between groups.
//
// Every list keeps it, not only the ones sorted by it, so that stepping onto
// the inclusion order with . sorts by what is true now.
//
// The cursor stays on its card, unless that card is the one that moved: add
// a card to the deck and it goes up to join the others, and the cursor goes
// on to the card that was below it — so a run of adds walks down the list.
func (m Model) resortInclusion() {
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			l, ok := v.(*cardList)
			if !ok {
				continue
			}
			members := m.membersFor(l)
			if sameMembers(members, l.members) {
				continue
			}
			before := l.members
			l.members = members
			if !l.sortsBy(sortInclusion) {
				continue
			}

			on, had := l.current()
			var below deck.Card
			hasBelow := l.cursor.at+1 < len(l.rows)
			if hasBelow {
				below = l.rows[l.cursor.at+1]
			}
			l.refresh()
			switch {
			case !had:
			case before[markKey(on)] != members[markKey(on)] && hasBelow:
				l.selectByName(below.Card.Name)
			default:
				l.selectByName(on.Card.Name)
			}
		}
	}
}

func sameMembers(a, b map[string]membership) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// viewInfo draws the information panel.
func (m Model) viewInfo(width, height int) string {
	inner, hints, room := m.infoFrame(width, height)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)

	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(fit(m.infoTitle(), inner))

	lines := []string{
		title,
		lipgloss.NewStyle().Foreground(theme.Border).Render(strings.Repeat("─", inner)),
	}

	height -= len(hints) // the body scrolls in what the keys leave it

	body := m.infoContent(inner)
	offset := m.info.offset
	if m.info.mode == infoStats {
		// Kept by followStats as the highlight moves; checked again here,
		// so a frame drawn before that — a resize, cards arriving — still
		// shows the highlighted category. J past the bottom used to move a
		// cursor you could no longer see.
		offset = m.statScroll(offset, room, len(body))
	}
	// Scrolled with K and J from wherever you are — the panel is read, never
	// focused.
	if offset > maxInt(len(body)-room, 0) {
		offset = maxInt(len(body)-room, 0)
	}
	if offset < len(body) {
		body = body[offset:]
	} else {
		body = nil
	}
	lines = append(lines, body...)

	if len(body) == 0 {
		lines = append(lines, dim.Render(fit("nothing highlighted", inner)))
	}
	for len(lines) < height-2 {
		lines = append(lines, strings.Repeat(" ", inner))
	}
	if len(lines) > height-2 {
		lines = lines[:maxInt(height-2, 1)]
	}
	lines = append(lines, hints...)
	height += len(hints)

	block := lipgloss.NewStyle().Width(inner).Height(maxInt(height-2, 1)).
		MaxWidth(inner).Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Render(block)
}

// infoTitle names what the information panel is describing. The mode when
// there is one, and otherwise whatever is under the cursor — a panel headed
// "card" while showing a git diff is a small lie told constantly.
func (m Model) infoTitle() string {
	switch m.info.mode {
	case infoStats:
		return m.statTitle()
	case infoVersions:
		return "printed text"
	case infoImage:
		return "printing"
	}

	p := m.ws.current()
	if p == nil {
		return "card"
	}
	switch p.top().(type) {
	case *deckList:
		return "deck"
	case *rulesView:
		return "rule"
	case *versionList:
		return "version"
	}
	return "card"
}

// infoContent is the whole information-panel body for the current mode,
// before it is scrolled — the lines scrolling counts and the view draws
// from the same place, so they can't disagree about where the bottom is.
func (m Model) infoContent(inner int) []string {
	switch m.info.mode {
	case infoStats:
		return m.renderStats(inner)
	case infoVersions:
		return m.infoVersions(inner)
	case infoImage:
		return m.infoImageLines(inner)
	default:
		return m.infoBody(inner)
	}
}

// infoFrame is the information panel's geometry at a size: the width its
// text is drawn at, the key hints along its bottom, and how many lines of
// body it shows between them and its title. One function for drawing and
// for scrolling, so the two agree on where the bottom is.
func (m Model) infoFrame(width, height int) (inner int, hints []string, room int) {
	inner = maxInt(width-2, 1)
	const title = 2 // the title and the rule under it
	if m.hintsExpanded {
		hints = hintFooter([][]hintGroup{m.infoHintGroups()}, inner, height-2-title)
	}
	return inner, hints, maxInt(height-len(hints)-2-title, 1)
}

// infoSpan is what scrolling the information panel works within: the width
// its body is drawn at, how many lines it shows, and the furthest it can
// scroll. False when the terminal is too narrow to have one.
func (m Model) infoSpan() (inner, room, most int, ok bool) {
	ws := m.ws // a copy: layout records the scroll position
	l := ws.layoutWithTop(m.topHeight())
	if l.info == 0 {
		return 0, 0, 0, false
	}
	inner, _, room = m.infoFrame(l.info, l.height)
	return inner, room, maxInt(len(m.infoContent(inner))-room, 0), true
}

// scrollInfoHalf moves the information panel half its height, K up and J
// down. A card's text and rulings are read, not walked a line at a time.
func (m *Model) scrollInfoHalf(dir int) {
	_, room, most, ok := m.infoSpan()
	if !ok {
		return
	}
	at := minInt(m.info.offset, most) + dir*maxInt(room/2, 1)
	m.info.offset = maxInt(minInt(at, most), 0)
}

// scrollTo brings a line into view with as little movement as possible.
func scrollTo(line, offset, height, total int) int {
	if line < offset {
		offset = line
	}
	if line >= offset+height {
		offset = line - height + 1
	}
	if max := total - height; offset > max {
		offset = maxInt(max, 0)
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

// infoBody is what the focused panel has to say about its highlighted row.
func (m Model) infoBody(width int) []string {
	p := m.ws.current()
	if p == nil {
		return nil
	}
	v := p.top()
	if v == nil {
		return nil
	}
	return v.info(width)
}

// infoVersions is gv: a card's printed wordings, or — when what is
// highlighted isn't a card — whatever the view has to say.
func (m Model) infoVersions(width int) []string {
	if c := m.focusedCardValue(); c != nil {
		return m.renderHistory(*c, width)
	}
	return m.infoBody(width)
}

// ── The top lines ───────────────────────────────────────────────

// viewTop is the two lines above the panels: the leader menu while the
// leader is waiting, and otherwise the keys that apply where you are, with
// the last thing you did under them.
//
// They used to run along the bottom, but your eyes are at the top: the i bar
// and the search bars are in the panels' headers, and the tab-completion list
// is read while typing in them. The block used to grow and shrink too, with
// the notice coming and going, and every panel jumped with it.
func (m Model) viewTop() string {
	return strings.Join(m.topLines(), "\n")
}

// topLines is the top of the screen, a line each, always two: the keys, then
// the notice — the line nearest the panels, which is where you are typing.
// Only a leader menu too long for two lines, on a narrow terminal, takes a
// third rather than being cut.
func (m Model) topLines() []string {
	var lines []string
	switch {
	case m.quitting:
		lines = []string{m.viewQuitQuestion()}
	case m.leader:
		lines = strings.Split(m.viewLeaderBar(), "\n")
	case m.tagPrefix:
		lines = indent(m.tagMoveBarLines())
	default:
		notice := m.viewNotice()
		lines = indent(m.keyLines(notice == ""))
		if notice != "" {
			lines = append(lines, " "+notice)
		}
	}
	for len(lines) < topRows {
		lines = append(lines, "")
	}
	return lines
}

// indent sets lines in by a space, the margin the top lines keep.
func indent(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = " " + line
	}
	return out
}

// viewQuitQuestion asks about decks with edits that were written but never
// committed. Nothing is lost by quitting; the question is whether the
// history should have them.
func (m Model) viewQuitQuestion() string {
	decks := m.dirtyDecks()
	what := decks[0]
	if len(decks) > 1 {
		what = itoa(len(decks)) + " decks have"
	} else {
		what += " has"
	}

	key := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	text := lipgloss.NewStyle().Foreground(theme.Text)
	return lipgloss.NewStyle().
		Background(theme.SurfaceAlt).Width(m.width).MaxWidth(m.width).
		Render(" " + text.Render(what+" uncommitted edits — ") +
			key.Render("w") + text.Render(" commit and quit · ") +
			key.Render("y") + text.Render(" quit, leaving them uncommitted · any other key stays"))
}

// viewLeaderBar is the menu the leader raises, so it never has to be
// memorised. Being able to see the menu is what makes a two-key binding
// cheaper in practice than a one-key chord you can't remember.
func (m Model) viewLeaderBar() string {
	// No background fill: a band of colour across the top contrasts with an
	// otherwise semi-transparent terminal, where nothing else here paints
	// one. The menu is just text, indented a space like the keys line.
	return strings.Join(indent(m.leaderBarLines()), "\n")
}

// leaderBarLines is the menu, wrapped onto as many lines as it needs.
//
// It used to be cut off at the width, which is the wrong thing to do to a
// menu: the entries you can't see are exactly the ones you opened it to
// read, and the cut lands mid-entry where it looks like a rendering fault.
// The layout asks how tall this is, so past the two lines the top always
// has, growing it takes room from the panels rather than pushing them off
// the screen.
func (m Model) leaderBarLines() []string {
	return packStyled(leaderParts(), leaderSep(), maxInt(m.width-2, 1))
}

// leaderParts is the leader's menu, an entry each: the key, and what it does.
func leaderParts() []string {
	key := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	what := lipgloss.NewStyle().Foreground(theme.Text)

	var parts []string
	for _, c := range leaderMenu {
		if k := keymap.Hint(keymap.Leader, c.action); k != "" {
			parts = append(parts, key.Render(k)+" "+what.Render(c.what))
		}
	}
	return parts
}

func leaderSep() string {
	return lipgloss.NewStyle().Foreground(theme.TextMuted).Render(" · ")
}

// leaderReference is the leader's menu as ? shows it along the top: led by
// the leader itself, so it reads as what to press first — "space: f find
// · d decks · …" — without having to press it to find out.
func (m Model) leaderReference(width int) []string {
	return packStyled(m.leaderReferenceParts(), leaderSep(), width)
}

// leaderReferenceParts is the same, an entry each.
func (m Model) leaderReferenceParts() []string {
	lead := keymap.Hint(keymap.Global, keymap.GlobalLeader)
	parts := leaderParts()
	if lead == "" || len(parts) == 0 {
		return nil
	}
	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render(lead + ":")
	parts[0] = head + " " + parts[0]
	return parts
}

// topHeight is how many rows the top of the screen takes: two, unless a
// leader menu needs more on a narrow terminal.
func (m Model) topHeight() int {
	return len(m.topLines())
}

// keyLines is the keys line: the two keys that reach everything else, or a
// focused bar's own keys. It is one line, or two when spare is set — when no
// notice wants the second.
//
// With ? on, the leader's menu leads it: the panels carry their own keys,
// and the leader's are the ones that belong to none of them. ? and q always
// show, at the end; the leader's last entries give way to them when the
// line runs out, since pressing the leader shows its whole menu anyway.
func (m Model) keyLines(spare bool) []string {
	width := maxInt(m.width-2, 1)
	// One group, one row: no state offers more than that here. Cut to one
	// regardless, so the line can't grow the block.
	keys := m.hintGroupLines(m.footerGroups(), width)[:1]
	if !m.hintsExpanded || m.barFocused() {
		return keys
	}
	most := 1
	if spare {
		most = 2
	}
	parts := m.leaderReferenceParts()
	for n := len(parts); n >= 0; n-- {
		lines := packStyled(append(parts[:n:n], keys[0]), leaderSep(), width)
		if len(lines) <= most {
			return lines
		}
	}
	return keys
}

// barFocused reports whether a search bar, or the i bar, has the cursor,
// where every key is typing and the footer shows the bar's own keys.
func (m Model) barFocused() bool {
	p := m.ws.current()
	if p == nil {
		return false
	}
	switch p.asking {
	case askAddCard, askOtag, askTag, askTagMove, askAddTag, askGlobalTags:
		return true
	}
	return p.searchOpen && p.search.Focused()
}

// footerGroups is what the keys line shows: the two keys that reach
// everything else. ? draws the rest inside the panels they belong to.
//
// A focused search bar is the exception — it shows its own small keymap,
// and ? is a character there, so there is nothing to grow.
func (m Model) footerGroups() []hintGroup {
	if m.barFocused() {
		return m.hintGroups()
	}
	return dropUnbound([]hintGroup{{"", restingKeys()}})
}

// hintFooter is the keys for the bottom of a panel: for each block, a rule,
// then its groups, each wrapping onto as many lines as it needs. room is how
// many lines the panel has for body and keys together; the keys leave the
// body a few of them, and give up their last lines rather than the cards.
func hintFooter(blocks [][]hintGroup, width, room int) []string {
	rule := lipgloss.NewStyle().Foreground(theme.Border).Render(strings.Repeat("─", width))
	var out []string
	for _, groups := range blocks {
		if len(groups) == 0 {
			continue
		}
		out = append(out, rule)
		for _, line := range hintBlock(groups, width) {
			out = append(out, pad(line, width))
		}
	}
	max := room - minBodyUnderHints
	if max < 2 {
		return nil
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}

// minBodyUnderHints is how many rows a panel keeps for what it shows when
// the keys are drawn under it.
const minBodyUnderHints = 3

// hintBlock renders grouped keys for a panel: each group leads with its
// title, and its keys wrap rather than being dropped — the panel is where
// the whole keymap lives now, and a key cut off it is a key nobody finds.
func hintBlock(groups []hintGroup, width int) []string {
	// The group titles are a colour of their own, so they read as headings
	// rather than as one more key.
	head := lipgloss.NewStyle().Foreground(theme.Info).Bold(true)
	key := lipgloss.NewStyle().Foreground(theme.Accent)
	what := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var out []string
	for _, g := range groups {
		var parts []string
		if g.title != "" {
			parts = append(parts, head.Render(truncate(g.title+":", width)))
		}
		for _, r := range g.keys {
			k, w := r[0], r[1]
			// A part wider than the whole panel is cut rather than left to
			// overrun it; at any usable width a key and its label fit.
			if textWidth(k)+1+textWidth(w) > width {
				w = truncate(w, maxInt(width-textWidth(k)-1, 1))
			}
			parts = append(parts, key.Render(k)+" "+what.Render(w))
		}
		out = append(out, packStyled(parts, "  ", width)...)
	}
	return out
}

// hintGroupLines renders grouped keys, one row per group, each led by its
// title: "navigation: …", "select: …". A group is kept to a single row — keys
// that would overrun the width are dropped rather than wrapped, since the full
// set is one ? away. The labels are terse for the same reason: the bar is a
// reminder, not the manual.
func (m Model) hintGroupLines(groups []hintGroup, width int) []string {
	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	key := lipgloss.NewStyle().Foreground(theme.Accent)
	what := lipgloss.NewStyle().Foreground(theme.TextMuted)
	sep := what.Render("  ")
	sepW := textWidth("  ")

	var lines []string
	for _, g := range groups {
		var b strings.Builder
		used := 0
		if g.title != "" {
			b.WriteString(head.Render(g.title + ":"))
			used = textWidth(g.title) + 1 // the colon
		}

		for _, r := range g.keys {
			part := key.Render(r[0]) + " " + what.Render(r[1])
			w := textWidth(r[0]) + 1 + textWidth(r[1])
			if used+sepW+w > width {
				break // one row only; the rest is in the reference
			}
			if used > 0 {
				b.WriteString(sep)
				used += sepW
			}
			b.WriteString(part)
			used += w
		}
		lines = append(lines, b.String())
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// viewNotice is the last thing you did, on its own line under the keys.
func (m Model) viewNotice() string {
	if m.notice == "" {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(theme.Success)
	if strings.HasPrefix(m.notice, "error:") {
		style = lipgloss.NewStyle().Foreground(theme.Error)
	}
	// A line of its own, so it can afford to be readable — a notice cut
	// short is one you have to guess at, which is what "that deck isn't
	// yours — p nee…" reads like.
	return style.Render(truncate(m.notice, maxInt(m.width-2, 24)))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// mutedLine is a full-width line of dim text, which every view uses to say
// it has nothing to show.
func mutedLine(s string, width int) string {
	return lipgloss.NewStyle().Foreground(theme.TextMuted).Render(fit(s, width))
}

// wrapStyled wraps text and paints each line, for the information panel.
func wrapStyled(s string, width int, style lipgloss.Style) []string {
	lines := wrap(s, width)
	for i, line := range lines {
		lines[i] = style.Render(fit(line, width))
	}
	return lines
}
