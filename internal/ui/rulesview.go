package ui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/keymap"
	"ttr/internal/mtg"
	"ttr/internal/rules"
	"ttr/internal/theme"
)

// The comprehensive rules, in a panel.
//
// Two ways in. Search it, and get the paragraphs that mention what you
// typed. Or open it over a card, and get the rules that card invokes —
// which is the one people actually want, because the question is almost
// never "what does 702.9 say", it is "why doesn't this work".
//
// The list is numbers and first lines. The whole rule is in the information
// panel, because a rule is a paragraph and a panel column is not.

type ruleRowKind int

const (
	rowHeading ruleRowKind = iota
	rowRule
	rowGlossary
)

// ruleRow is a line in the list: a group heading, a rule, or a glossary
// entry. Headings are in the list rather than around it so that scrolling
// carries them along; the cursor steps over them.
type ruleRow struct {
	kind    ruleRowKind
	heading string

	number string
	text   string
	// score is how well this answers the query; lower is better.
	score int
	term  string // the card's word that led here, when it came from a card
	entry rules.GlossaryEntry
}

type ruleOrder int

const (
	byRelevance ruleOrder = iota
	byNumber
)

func (o ruleOrder) String() string {
	if o == byNumber {
		return "rule number"
	}
	return "relevance"
}

type rulesView struct {
	cursor
	data rules.Data
	name string

	all  []ruleRow
	rows []ruleRow

	filter string
	order  ruleOrder
	// grouped is set when the rows came from a card, where the headings
	// mean something and reordering would throw them away.
	grouped bool
}

// newRuleSearch is what typing in the search bar produces.
func newRuleSearch(data rules.Data, query string) *rulesView {
	// The same quote-aware splitting the card filter uses, so there is one
	// query syntax in the program rather than two.
	terms := filterTerms(query)
	hits, entries := data.Search(terms)

	var rows []ruleRow
	for _, r := range hits {
		rows = append(rows, ruleRow{
			kind: rowRule, number: r.Number, text: r.Text,
			score: relevance(r.Number+" "+r.Text, terms),
		})
	}
	for _, g := range entries {
		rows = append(rows, ruleRow{
			kind: rowGlossary, term: g.Term, entry: g,
			score: relevance(g.Term+" "+g.Definition, terms),
		})
	}

	v := &rulesView{data: data, name: "rules: " + query, all: rows}
	v.refresh()
	return v
}

// relevance scores a hit by how early and how tightly the terms appear. A
// rule that opens with what you asked about is about it; one that mentions
// it in passing three hundred words in is not.
func relevance(text string, terms []string) int {
	lower := strings.ToLower(text)
	first, last := 1<<30, 0
	for _, t := range terms {
		i := strings.Index(lower, t)
		if i < 0 {
			continue
		}
		if i < first {
			first = i
		}
		if end := i + len(t); end > last {
			last = end
		}
	}
	if first == 1<<30 {
		return 1 << 30
	}
	// Where it starts, plus how far apart the terms ended up.
	return first + (last-first)/4
}

// newCardRules is what opening the panel over a card produces: everything
// that card invokes, grouped by what kind of thing it is.
//
// The grouping is the point. A card with flying, a sacrifice cost and a
// graveyard trigger raises three different questions, and running them
// together as one list of numbers makes you read all of it to find the one
// you wanted.
func newCardRules(data rules.Data, c mtg.Card) *rulesView {
	groups := []struct {
		heading string
		keep    func(rules.RuleMatch) bool
	}{
		{"abilities", func(m rules.RuleMatch) bool {
			return m.Kind == rules.MatchKeyword && m.Kw.Kind == rules.KeywordAbility
		}},
		{"actions", func(m rules.RuleMatch) bool {
			return m.Kind == rules.MatchKeyword && m.Kw.Kind == rules.KeywordAction
		}},
		{"ability words", func(m rules.RuleMatch) bool {
			return m.Kind == rules.MatchKeyword && m.Kw.Kind == rules.AbilityWord
		}},
		{"zones", func(m rules.RuleMatch) bool {
			return m.Kind == rules.MatchGlossary && rules.IsZone(m.Term)
		}},
		{"card types", func(m rules.RuleMatch) bool { return m.Kind == rules.MatchType }},
		{"glossary", func(m rules.RuleMatch) bool {
			return m.Kind == rules.MatchGlossary && !rules.IsZone(m.Term)
		}},
	}

	matches := data.MatchCard(c)
	var rows []ruleRow
	for _, g := range groups {
		var kept []ruleRow
		for _, match := range matches {
			if !g.keep(match) {
				continue
			}
			row := ruleRow{kind: rowRule, number: match.Rule, term: match.Term}
			if r, ok := data.Rule(match.Rule); ok {
				row.text = r.Text
				// A keyword's own rule is often just the keyword: 702.17 is
				// the word "Reach" and nothing else, with everything you
				// wanted in 702.17a. Listing "Reach  Reach" says nothing.
				if repeatsTerm(r.Text, match.Term) {
					if subs := data.Subrules(match.Rule); len(subs) > 0 {
						row.text = subs[0].Text
					}
				}
			}
			if match.Rule == "" {
				row.kind = rowGlossary
				row.entry = match.Entry
			}
			kept = append(kept, row)
		}
		if len(kept) == 0 {
			continue
		}
		rows = append(rows, ruleRow{kind: rowHeading, heading: g.heading})
		rows = append(rows, kept...)
	}

	v := &rulesView{data: data, name: c.Name, all: rows, grouped: true, order: byNumber}
	v.refresh()
	v.cursor.at = v.firstSelectable()
	return v
}

func (v *rulesView) refresh() {
	rows := v.all
	if terms := filterTerms(v.filter); len(terms) > 0 {
		rows = keepMatching(rows, terms)
	}

	if !v.grouped {
		sorted := append([]ruleRow(nil), rows...)
		sort.SliceStable(sorted, func(i, j int) bool {
			if v.order == byNumber {
				return lessRuleNumber(sorted[i].number, sorted[j].number)
			}
			if sorted[i].score != sorted[j].score {
				return sorted[i].score < sorted[j].score
			}
			return lessRuleNumber(sorted[i].number, sorted[j].number)
		})
		rows = sorted
	}

	v.rows = rows
	v.cursor.clamp(len(v.rows))
	if !v.selectable(v.cursor.at) {
		v.cursor.at = v.firstSelectable()
	}
}

// keepMatching narrows the rows, dropping a heading whose group has been
// emptied — a heading with nothing under it is a lie.
func keepMatching(rows []ruleRow, terms []string) []ruleRow {
	var out []ruleRow
	for _, r := range rows {
		if r.kind == rowHeading {
			out = append(out, r)
			continue
		}
		hay := strings.ToLower(r.number + " " + r.text + " " + r.term + " " +
			r.entry.Term + " " + r.entry.Definition)
		keep := true
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, r)
		}
	}

	// Drop headings left with nothing under them.
	var kept []ruleRow
	for i, r := range out {
		if r.kind == rowHeading && (i+1 >= len(out) || out[i+1].kind == rowHeading) {
			continue
		}
		kept = append(kept, r)
	}
	return kept
}

// lessRuleNumber orders 100.1a before 100.2 before 101.1, which string
// comparison does not: "100.10" sorts before "100.2".
func lessRuleNumber(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		if nx, ny := leadingNumber(x), leadingNumber(y); nx != ny {
			return nx < ny
		}
		if x != y {
			return x < y
		}
	}
	return len(as) < len(bs)
}

func leadingNumber(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func (v *rulesView) selectable(i int) bool {
	return i >= 0 && i < len(v.rows) && v.rows[i].kind != rowHeading
}

func (v *rulesView) firstSelectable() int {
	for i := range v.rows {
		if v.rows[i].kind != rowHeading {
			return i
		}
	}
	return 0
}

// step moves the cursor, passing over headings — they are labels, not rows
// you can be on.
func (v *rulesView) step(delta int) {
	if delta == 0 || len(v.rows) == 0 {
		return
	}
	at := v.cursor.at
	for {
		at += delta
		if at < 0 || at >= len(v.rows) {
			return // nothing selectable that way; stay put
		}
		if v.rows[at].kind != rowHeading {
			v.cursor.at = at
			return
		}
	}
}

func (v *rulesView) current() (ruleRow, bool) {
	if !v.selectable(v.cursor.at) {
		return ruleRow{}, false
	}
	return v.rows[v.cursor.at], true
}

// ── As a view ───────────────────────────────────────────────────

func (v *rulesView) title() string { return v.name }

func (v *rulesView) subtitle() string {
	n := 0
	for _, r := range v.rows {
		if r.kind != rowHeading {
			n++
		}
	}
	out := itoa(n) + " " + plural("rule", n)
	if !v.grouped {
		out += " · " + v.order.String()
	}
	return out
}

func (v *rulesView) lines(width, height int, focused bool, m *Model) []string {
	if len(v.rows) == 0 {
		what := "nothing in the rules says that"
		if v.filter != "" {
			what = "nothing matches"
		}
		return fillTo([]string{mutedLine(what, width)}, width, height)
	}

	v.cursor.scrollInto(height, len(v.rows))
	lines := make([]string, 0, height)
	for i := v.cursor.offset; i < len(v.rows) && len(lines) < height; i++ {
		lines = append(lines, renderRuleRow(v.rows[i], width, focused && i == v.cursor.at))
	}
	return fillTo(lines, width, height)
}

// clear has nothing transient to drop for esc; the filter is cleared with b.
func (v *rulesView) clear() bool { return false }

func (v *rulesView) filterText() string { return v.filter }

func (v *rulesView) setFilter(s string) {
	v.filter = s
	v.refresh()
}

func (v *rulesView) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	// The shared list keys, handled here rather than by navKey because
	// headings aren't rows the cursor can rest on.
	if k == toTop {
		v.cursor.at = v.firstSelectable()
		return true, nil
	}
	switch keymap.Lookup(keymap.List, k) {
	case keymap.ListDown:
		v.step(1)
		return true, nil
	case keymap.ListUp:
		v.step(-1)
		return true, nil
	case keymap.ListBottom:
		v.cursor.bottom(len(v.rows))
		if !v.selectable(v.cursor.at) {
			v.step(-1)
		}
		return true, nil
	}

	switch keymap.Lookup(keymap.Rules, k) {
	case keymap.RulesOrderNext, keymap.RulesOrderPrev:
		// Two orders, so forward and back are the same step.
		if !v.grouped {
			v.order = 1 - v.order
			v.refresh()
		}
	case keymap.RulesFilter:
		p.openFilter(v.filter)
	case keymap.RulesSync:
		// Check for a new release and pull it in. Off the main thread, so it
		// reports back as a notice.
		return true, syncRules
	default:
		return false, nil
	}
	return true, nil
}

// renderRuleRow draws one line: a heading, or a number and the start of what
// it says.
func renderRuleRow(r ruleRow, width int, under bool) string {
	if r.kind == rowHeading {
		return lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
			Render(fit(r.heading, width))
	}

	label, body := r.number, r.text
	if r.kind == rowGlossary {
		label, body = r.entry.Term, r.entry.Definition
	}
	// A rule reached from a card is better labelled by the card's word than
	// by its number: you looked it up because the card said "flying".
	if r.term != "" && r.kind == rowRule {
		label = r.term
	}

	labelStyle := lipgloss.NewStyle().Foreground(theme.Highlight)
	bodyStyle := lipgloss.NewStyle().Foreground(theme.TextDim)
	if under {
		labelStyle = labelStyle.Foreground(theme.SelectionFg).Bold(true)
		bodyStyle = bodyStyle.Foreground(theme.Text)
	}

	// The label gets what it needs; the text gets the rest, if any is left.
	labelWidth := minInt(textWidth(label), maxInt(width/2, 8))
	line := labelStyle.Render(fit(label, labelWidth))
	if rest := width - labelWidth - 1; rest > 3 {
		line += " " + bodyStyle.Render(fit(firstSentence(body), rest))
	} else {
		line = labelStyle.Render(fit(label, width))
	}

	if under {
		return lipgloss.NewStyle().Background(theme.SelectionBg).Width(width).Render(line)
	}
	return line
}

// firstSentence is as much of a rule as fits on a line without pretending to
// be the whole of it.
func firstSentence(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// info is the whole rule, which is what the list can only gesture at.
//
// A rule alone is often not the answer: 702.9 says flying is an evasion
// ability and nothing else, and everything you wanted is in 702.9b. So the
// sub-rules come with it.
func (v *rulesView) info(width int) []string {
	r, ok := v.current()
	if !ok {
		return nil
	}

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	body := lipgloss.NewStyle().Foreground(theme.Text)
	sub := lipgloss.NewStyle().Foreground(theme.TextDim)

	if r.kind == rowGlossary {
		out := []string{head.Render(fit(r.entry.Term, width)), ""}
		return append(out, wrapStyled(r.entry.Definition, width, body)...)
	}

	var out []string
	title := r.number
	if r.term != "" {
		title += "  " + r.term
	}
	out = append(out, head.Render(fit(title, width)), "")

	// The rule as the book has it, not the stand-in the list borrowed from
	// a sub-rule to have something to show — that sub-rule is listed below
	// in its own right, and printing it twice reads as a mistake.
	text := r.text
	if actual, ok := v.data.Rule(r.number); ok {
		text = actual.Text
	}
	out = append(out, ruleText(text, width, body)...)

	for _, s := range v.data.Subrules(r.number) {
		out = append(out, "")
		out = append(out, sub.Render(fit(s.Number, width)))
		out = append(out, ruleText(s.Text, width, body)...)
	}
	return out
}

// ruleText is a rule's text with its examples set apart under it: each one
// headed "Example" and indented, rather than run on as if the rule said it.
func ruleText(text string, width int, style lipgloss.Style) []string {
	body, examples := rules.SplitExamples(text)
	var out []string
	if body != "" {
		out = wrapStyled(body, width, style)
	}
	label := lipgloss.NewStyle().Foreground(theme.Accent)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)
	const indent = "  "
	for _, ex := range examples {
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, indent+label.Render(fit("Example", maxInt(width-len(indent), 1))))
		for _, line := range wrap(ex, maxInt(width-len(indent), 1)) {
			out = append(out, indent+dim.Render(line))
		}
	}
	return out
}

// repeatsTerm reports whether a rule's text is just the word that led to it.
func repeatsTerm(text, term string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	term = strings.TrimSpace(strings.ToLower(term))
	return text == term || text == term+"s" || strings.TrimSuffix(text, "s") == term
}

func (v *rulesView) keys() []hintGroup {
	nav := [][2]string{
		hint("filter", keymap.Rules, keymap.RulesFilter),
	}
	if !v.grouped {
		nav = append(nav, hint("order", keymap.Rules, keymap.RulesOrderNext, keymap.RulesOrderPrev))
	}
	return []hintGroup{
		{"navigation", nav},
		{"rules", [][2]string{
			hint("sync", keymap.Rules, keymap.RulesSync),
			{gotoHint(keymap.GotoVersions), "diff vs previous"},
		}},
		{"info panel", [][2]string{hint("read rule", keymap.Global, keymap.GlobalInfoUp, keymap.GlobalInfoDown)}},
	}
}
