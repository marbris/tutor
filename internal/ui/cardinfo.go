package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/rules"
	"ttr/internal/stats"
	"ttr/internal/theme"
)

// A card, in the information panel.
//
// The oracle text is the part worth trouble. Rules text is dense and reads
// as a wall unless the things that mean something are picked out: mana
// symbols in their colours, keyword abilities the rules define, reminder
// text pushed into the background, the card's own name where it refers to
// itself. Ported from the old two-pane screen, which got this right.

// runeStyle is how one byte of text should look. Building a mask over the
// string, rather than styling as we go, is what lets the patterns below
// have a priority order: whoever claims a range first keeps it.
type runeStyle struct {
	col    lipgloss.Color
	bold   bool
	italic bool
	set    bool
}

var (
	reminderRe = regexp.MustCompile(`\([^)]*\)?`)
	symbolRe   = regexp.MustCompile(`\{[^}]{1,12}\}`)
	loyaltyRe  = regexp.MustCompile(`(?m)^[+\x{2212}-]?[0-9X]+:`)
	ptRe       = regexp.MustCompile(`[+\x{2212}-][0-9X]+/[+\x{2212}-][0-9X]+`)
)

func oracleStyles() (plain, reminder, ability, action, word, loyalty, buff, debuff runeStyle) {
	return runeStyle{col: theme.Text, set: true},
		runeStyle{col: theme.TextMuted, italic: true, set: true},
		runeStyle{col: theme.Highlight, bold: true, set: true},
		runeStyle{col: theme.Member, set: true},
		runeStyle{col: theme.Special, italic: true, set: true},
		runeStyle{col: theme.Accent, bold: true, set: true},
		runeStyle{col: theme.Success, set: true},
		runeStyle{col: theme.Error, set: true}
}

// symbolColour is what goes inside a pair of braces. Hybrid symbols get the
// hybrid colour rather than one of their halves, since picking a half would
// be picking wrong half the time.
func symbolColour(inner string) lipgloss.Color {
	upper := strings.ToUpper(inner)

	switch upper {
	case "T", "Q":
		return theme.Accent
	case "E":
		return theme.Member
	case "S":
		return theme.ManaU
	case "C", "X", "Y", "Z":
		return theme.ManaC
	}

	colours := map[rune]lipgloss.Color{
		'W': theme.ManaW, 'U': theme.ManaU, 'B': theme.ManaB,
		'R': theme.ManaR, 'G': theme.ManaG,
	}

	found, count := lipgloss.Color(""), 0
	for _, r := range upper {
		if col, ok := colours[r]; ok {
			count++
			found = col
		}
	}
	switch {
	case count == 1:
		return found
	case count > 1:
		return theme.ManaMulti
	}
	return theme.ManaC
}

// colourForCard is the colour a card's own name is written in.
func colourForCard(colors []string) lipgloss.Color {
	if len(colors) > 1 {
		return theme.ManaMulti
	}
	if len(colors) == 1 {
		switch colors[0] {
		case "W":
			return theme.ManaW
		case "U":
			return theme.ManaU
		case "B":
			return theme.ManaB
		case "R":
			return theme.ManaR
		case "G":
			return theme.ManaG
		}
	}
	return theme.ManaC
}

// selfNames are the forms of its own name a card may refer to itself by.
func selfNames(name string) []string {
	if name == "" {
		return nil
	}
	// Split cards: "Fire // Ice".
	parts := strings.Split(name, " // ")
	out := make([]string, 0, len(parts)*2)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
		// The part before the comma of "Ojutai, Soul of Winter" is the name
		// the rules text uses; what follows is a title.
		if i := strings.Index(p, ","); i > 0 {
			if short := strings.TrimSpace(p[:i]); short != "" && short != p {
				out = append(out, short)
			}
		}
	}
	return out
}

var literalReCache = map[string]*regexp.Regexp{}

func literalRe(s string) *regexp.Regexp {
	if re, ok := literalReCache[s]; ok {
		return re
	}
	re := regexp.MustCompile(regexp.QuoteMeta(s))
	literalReCache[s] = re
	return re
}

// buildMask decides how every byte of a card's text should look. The order
// is the priority order: reminder text swallows what's inside its brackets,
// symbols beat words, and the rules' keywords come last so they can't
// repaint something more specific.
func buildMask(text string, c mtg.Card, rd rules.Data) []runeStyle {
	plain, reminder, ability, action, word, loyalty, buff, debuff := oracleStyles()
	_ = plain

	mask := make([]runeStyle, len(text))
	fill := func(start, end int, st runeStyle) {
		if start < 0 || end > len(text) || start >= end {
			return
		}
		for i := start; i < end; i++ {
			if mask[i].set {
				return
			}
		}
		for i := start; i < end; i++ {
			mask[i] = st
		}
	}

	for _, loc := range reminderRe.FindAllStringIndex(text, -1) {
		fill(loc[0], loc[1], reminder)
	}
	for _, loc := range symbolRe.FindAllStringIndex(text, -1) {
		inner := text[loc[0]+1 : loc[1]-1]
		fill(loc[0], loc[1], runeStyle{col: symbolColour(inner), bold: true, set: true})
	}
	for _, loc := range loyaltyRe.FindAllStringIndex(text, -1) {
		fill(loc[0], loc[1], loyalty)
	}

	nameStyle := runeStyle{col: colourForCard(c.Colors), bold: true, set: true}
	for _, name := range selfNames(c.Name) {
		for _, sp := range rules.Scan(literalRe(name), text) {
			fill(sp.Start, sp.End, nameStyle)
		}
	}

	for _, loc := range ptRe.FindAllStringIndex(text, -1) {
		st := debuff
		if text[loc[0]] == '+' {
			st = buff
		}
		fill(loc[0], loc[1], st)
	}

	for _, sp := range rd.KeywordSpans(text) {
		switch sp.Kind {
		case rules.KeywordAbility:
			fill(sp.Start, sp.End, ability)
		case rules.KeywordAction:
			fill(sp.Start, sp.End, action)
		case rules.AbilityWord:
			fill(sp.Start, sp.End, word)
		}
	}
	return mask
}

// highlightOracle styles a card's text and wraps it. The result is fully
// styled: no outer Foreground should be applied to it, or the nested resets
// will strip the colours partway through.
func highlightOracle(text string, c mtg.Card, width int, rd rules.Data) []string {
	if text == "" {
		return nil
	}
	mask := buildMask(text, c, rd)

	var out []string
	for _, span := range wrapSpans(text, width) {
		out = append(out, emit(text, mask, span[0], span[1]))
	}
	return out
}

// wrapSpans breaks text into byte ranges that each fit the width, keeping
// the blank lines that separate a card's abilities.
func wrapSpans(s string, width int) [][2]int {
	var out [][2]int
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			out = append(out, splitLine(s, start, i, width)...)
			start = i + 1
		}
	}
	return out
}

// splitLine breaks one line at spaces, and mid-word only when a word is
// longer than the whole column.
func splitLine(s string, from, to, width int) [][2]int {
	if from >= to {
		return [][2]int{{from, to}}
	}

	var out [][2]int
	lineStart, lastSpace, w := from, -1, 0
	for i := from; i < to; i++ {
		if s[i] == ' ' {
			lastSpace = i
		}
		w++
		if w <= width {
			continue
		}
		switch {
		case lastSpace > lineStart:
			out = append(out, [2]int{lineStart, lastSpace})
			lineStart = lastSpace + 1
		default:
			out = append(out, [2]int{lineStart, i})
			lineStart = i
		}
		w = i - lineStart + 1
		lastSpace = -1
	}
	return append(out, [2]int{lineStart, to})
}

// emit renders a byte range, one run of identical styling at a time.
func emit(text string, mask []runeStyle, start, end int) string {
	if start >= end {
		return ""
	}
	var b strings.Builder
	runStart := start
	cur := styleAt(mask, start)
	for i := start + 1; i <= end; i++ {
		st := styleAt(mask, i)
		if i == end || st != cur {
			b.WriteString(renderRun(text[runStart:i], cur))
			runStart, cur = i, st
		}
	}
	return b.String()
}

func styleAt(mask []runeStyle, i int) runeStyle {
	if i < 0 || i >= len(mask) {
		return runeStyle{}
	}
	return mask[i]
}

func renderRun(s string, st runeStyle) string {
	style := lipgloss.NewStyle()
	if st.set {
		style = style.Foreground(st.col)
	} else {
		style = style.Foreground(theme.Text)
	}
	if st.bold {
		style = style.Bold(true)
	}
	if st.italic {
		style = style.Italic(true)
	}
	return style.Render(s)
}

// renderMana draws a mana cost with each symbol in its own colour.
func renderMana(cost string) string {
	if cost == "" {
		return ""
	}
	var b strings.Builder
	for _, loc := range symbolRe.FindAllStringIndex(cost, -1) {
		inner := cost[loc[0]+1 : loc[1]-1]
		b.WriteString(lipgloss.NewStyle().
			Foreground(symbolColour(inner)).Bold(true).Render(inner))
	}
	return b.String()
}

// ── The panel ───────────────────────────────────────────────────

// cardInfo is everything worth saying about a card, in the order you want to
// read it: what it is, what it does, and only then the bookkeeping.
func cardInfo(c deck.Card, width int, rd rules.Data, rulings []mtg.Ruling, rulingsErr error) []string {
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	var out []string

	// Each face gets its own heading and text. A transforming card carries
	// nothing at the top level, so this is the only way to see the back.
	faces := c.Card.Faces()
	for i, f := range faces {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, faceHeading(f, width))
		if line := typeLine(f, width); line != "" {
			out = append(out, dim.Render(line))
		}
		if text := f.OracleText; text != "" {
			out = append(out, "")
			out = append(out, highlightOracle(text, f, width, rd)...)
		}
		// Power/toughness below the text and flush right, where the card
		// prints it — on its own row rather than trailing the type line,
		// which is the thing that runs long and used to cut the "2/3" off.
		if pt := statsRow(f, width); pt != "" {
			out = append(out, pt)
		}
	}

	return append(out, cardMeta(c, c.Card, width, rulings, rulingsErr)...)
}

// cardMeta is the bookkeeping under a card — its tags, which printing and
// what it's worth, where it's legal, its rulings — for the printing p. The
// card view passes the card's own printing; the printing view whichever
// printing is up, since a price belongs to a printing.
func cardMeta(c deck.Card, p mtg.Card, width int, rulings []mtg.Ruling, rulingsErr error) []string {
	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)
	muted := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var out []string

	// The card's own tags, then in a dimmer hand the ones a tag list gives
	// it — they count the same, but they aren't in this list's file.
	if global := onlyGlobal(c); len(c.Tags) > 0 || len(global) > 0 {
		line := lipgloss.NewStyle().Foreground(theme.Highlight).Render(strings.Join(c.Tags, " "))
		if len(global) > 0 {
			if len(c.Tags) > 0 {
				line += " "
			}
			line += lipgloss.NewStyle().Foreground(theme.TextDim).Render(strings.Join(global, " "))
		}
		out = append(out, "", lipgloss.NewStyle().MaxWidth(width).Render(line))
	}

	// EDHREC ranks the card, not the printing; a printing that doesn't carry
	// the rank still has it.
	if p.EDHRECRank == 0 {
		p.EDHRECRank = c.Card.EDHRECRank
	}
	if facts := printingFacts(p, width); len(facts) > 0 {
		out = append(out, "")
		out = append(out, facts...)
	}

	if legal := legalities(c.Card, width); len(legal) > 0 {
		out = append(out, "")
		out = append(out, head.Render(fit("legal in", width)))
		out = append(out, legal...)
	}

	out = append(out, "")
	out = append(out, head.Render(fit("rulings", width)))
	switch {
	case rulingsErr != nil:
		out = append(out, muted.Render(fit("couldn't fetch them", width)))
	case rulings == nil:
		out = append(out, muted.Render(fit("…", width)))
	case len(rulings) == 0:
		out = append(out, muted.Render(fit("none", width)))
	default:
		for i, r := range rulings {
			if i > 0 {
				out = append(out, "")
			}
			out = append(out, wrapStyled(r.Comment, width, dim)...)
		}
	}
	return out
}

// faceHeading is a face's name and its cost on one line, the cost pushed to
// the right where the eye can find it down a column of them.
func faceHeading(f mtg.Card, width int) string {
	name := lipgloss.NewStyle().Foreground(theme.TextBright).Bold(true)
	cost := renderMana(f.ManaCost)
	costWidth := textWidth(f.DisplayManaCost()) - 2*strings.Count(f.DisplayManaCost(), "{")

	room := width - costWidth
	if costWidth == 0 || room < 6 {
		return name.Render(fit(f.Name, width))
	}
	return name.Render(fit(f.Name, room-1)) + " " + cost
}

// typeLine is the card's type line on its own, so a narrow panel gives up
// the end of the type before it gives up the power/toughness beside it.
func typeLine(f mtg.Card, width int) string {
	if f.TypeLine == "" {
		return ""
	}
	return fit(f.TypeLine, width)
}

// statsLine is power/toughness, or a planeswalker's loyalty — the numbers a
// card prints in its bottom-right corner.
func statsLine(f mtg.Card, width int) string {
	switch {
	case f.Power != "" || f.Toughness != "":
		return truncate(f.Power+"/"+f.Toughness, width)
	case f.Loyalty != "":
		return truncate("loyalty "+f.Loyalty, width)
	}
	return ""
}

// statsRow is statsLine rendered flush right, where the card prints it, or
// "" for a face that has no numbers.
func statsRow(f mtg.Card, width int) string {
	pt := statsLine(f, width)
	if pt == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(strings.Repeat(" ", maxInt(width-textWidth(pt), 0)) + pt)
}

// printingFacts is the bookkeeping under a card's text, a line per kind:
// which printing it is (set · rarity), then what it's worth to you
// (edhrec rank · price). Whatever a card lacks is left out, and so is a
// line left with nothing on it. Wrapped to width and styled: muted, with
// the rarity in its own colour, the one the statistics draw it in.
func printingFacts(c mtg.Card, width int) []string {
	muted := lipgloss.NewStyle().Foreground(theme.TextMuted)
	var out []string
	for _, line := range factLines(c) {
		wrapped := wrap(line, width)
		for i, w := range wrapped {
			w = fit(w, width)
			text := strings.TrimRight(w, " ")
			pad := w[len(text):]
			// The rarity ends the printing line, and is one word, so it
			// ends the last line that line wraps to.
			if r := c.Rarity; r != "" && i == len(wrapped)-1 && strings.HasSuffix(line, r) && strings.HasSuffix(text, r) {
				out = append(out, muted.Render(text[:len(text)-len(r)])+
					lipgloss.NewStyle().Foreground(stats.RarityColour(r)).Render(r)+pad)
				continue
			}
			out = append(out, muted.Render(w))
		}
	}
	return out
}

// factLines are printingFacts' lines before they are wrapped and styled.
func factLines(c mtg.Card) []string {
	var printing, worth []string
	if c.SetName != "" {
		printing = append(printing, c.SetName)
	}
	if c.Rarity != "" {
		printing = append(printing, c.Rarity)
	}
	if c.EDHRECRank > 0 {
		worth = append(worth, "edhrec #"+itoa(c.EDHRECRank))
	}
	if _, ok := c.USD(); ok {
		worth = append(worth, usdText(c))
	}
	var out []string
	for _, facts := range [][]string{printing, worth} {
		if len(facts) > 0 {
			out = append(out, strings.Join(facts, " · "))
		}
	}
	return out
}

// formats are the ones worth reporting. Scryfall knows twenty; a Commander
// player wants to know about four of them.
var formats = []string{"commander", "standard", "modern", "legacy", "vintage", "pauper"}

func legalities(c mtg.Card, width int) []string {
	if len(c.Legalities) == 0 {
		return nil
	}
	yes := lipgloss.NewStyle().Foreground(theme.Success)
	banned := lipgloss.NewStyle().Foreground(theme.Error)

	var parts []string
	for _, f := range formats {
		switch c.Legalities[f] {
		case "legal":
			parts = append(parts, yes.Render(f))
		case "banned":
			// Worth saying out loud: a card banned in Commander is a
			// different problem from one that was never in the format.
			parts = append(parts, banned.Render(f+" (banned)"))
		case "restricted":
			parts = append(parts, banned.Render(f+" (restricted)"))
		}
		// Formats a card was simply never printed into say nothing. Under a
		// heading reading "legal in", listing them says the opposite.
	}
	if len(parts) == 0 {
		return nil
	}
	// Joined by hand rather than wrapped, because each part is already
	// styled and measuring styled text by eye is what wrapping does badly.
	return packStyled(parts, " ", width)
}

// packStyled fits already-coloured words onto as few lines as possible.
func packStyled(parts []string, sep string, width int) []string {
	var out []string
	line, lineWidth := "", 0
	// The separator may be styled too, so it is measured as it will show.
	sepW := lipgloss.Width(sep)
	for _, p := range parts {
		w := lipgloss.Width(p)
		switch {
		case line == "":
			line, lineWidth = p, w
		case lineWidth+sepW+w <= width:
			line += sep + p
			lineWidth += sepW + w
		default:
			out = append(out, line)
			line, lineWidth = p, w
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}
