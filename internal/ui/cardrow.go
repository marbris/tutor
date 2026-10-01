package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/stats"
	"ttr/internal/theme"
)

// One card, on one line, in a column that may be very narrow.
//
// A row is two columns: the card's name, and whatever property the list is
// sorted by. With several panels open there is never enough width, so rather
// than truncating — which loses the end of every name equally — the row gives
// things up in a fixed order:
//
//  1. everything fits
//  2. the type line becomes an initialism: Legendary Creature — Elf Faerie
//     Noble → LC-EFN
//  3. the name becomes one too, with a full stop to say so: Dwynen,
//     Gilt-Leaf Daen → D,GLD.
//  4. only then is anything cut off
//
// An initialism is still recognisable to someone who knows the card, which a
// name cut off after nine characters often isn't.
//
// The ladder is decided per row, not per list, so a column ends up mixing
// full names with shortened ones. That is what the full stop is for: it says
// which is which. Shortening every row to match the longest would cost the
// names that fit perfectly well, and gain only tidiness.

const (
	// gutter is the marker column: one character and a space.
	gutter = 2
	// gap is the space between the name and the column beside it.
	gap = 1
)

// rowState is what the list knows about a card that the card doesn't.
type rowState struct {
	selected bool       // picked out with v
	member   membership // where else on screen the card is — see membersFor
	cursor   bool       // under the cursor
	// then is the list's second order. The first decides the column; this
	// one decides the colour of the name.
	then cardSort
}

// renderRow draws one card to exactly width columns.
func renderRow(c deck.Card, order cardSort, st rowState, width int) string {
	return renderRowCol(c, order, st, width, 0)
}

// renderRowCol draws one card, sharing a name-column width with the rest of
// the list when one is given.
//
// nameCol of zero is the per-row ladder: each row decides for itself how much
// to give up, which mixes full and shortened columns down the list. A positive
// nameCol pins the boundary between the name and the column beside it, so the
// second column lines up and never reads as overlapping the names — which is
// what the type line, the only column long enough to collide, needs.
func renderRowCol(c deck.Card, order cardSort, st rowState, width, nameCol int) string {
	if width < 1 {
		return ""
	}

	mark, markCol := marker(c, st)
	body := maxInt(width-gutter, 1)

	name := cardName(c)
	col := order.column(c.Card)
	var text, colText string
	if nameCol > 0 {
		text, colText = layoutColumns(name, col, nameCol, body)
	} else {
		text, colText = layoutRow(name, col, order.abbreviates(), body)
	}

	// The sort decides what the row is about, so it decides what is worth
	// colouring. Sorting by colour and reading a column of grey names tells
	// you nothing the order didn't already.
	hue := nameColour(c.Card, order, st.then)

	if st.cursor {
		// The cursor is a background so it reads at a glance across four
		// panels, where a colour change alone gets lost. It takes the row's
		// own colours rather than a neutral grey, so the row you're looking
		// at says the most, not the least: the name on the name's colour,
		// the column on the column's, each written in whichever of dark or
		// light stands out from it. The marker does the same, its glyph
		// becoming the block's text; a row with no marker keeps the plain
		// highlight there.
		gutterCell := highlightLine(pad(mark, gutter), gutter, theme.SelectionBg)
		if markCol != "" {
			gutterCell = onColour(markCol).Bold(st.selected).Render(pad(mark, gutter))
		}
		line := gutterCell + onColour(hue).Bold(true).Render(text)
		if colText != "" {
			line += onColour(columnColour(c.Card, order)).Render(colText)
		}
		return line
	}

	nameStyle := lipgloss.NewStyle().Foreground(hue)
	markStyle := lipgloss.NewStyle().Foreground(markCol).Bold(st.selected)
	return markStyle.Render(pad(mark, gutter)) + nameStyle.Render(text) + paintColumn(colText, c.Card, order)
}

// onColour is a block of colour with legible text on it.
func onColour(bg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Background(bg).Foreground(theme.OnColour(bg))
}

// nameColour is what the card's name is written in.
//
// The second order decides it: sorted by type and then by colour, the type
// column carries the type and the name carries the card's colour, so one
// row says two things. An order with no colour of its own — mana value,
// price, a rank — leaves the name plain.
//
// With no second order, the name carries the card's type — the one thing a
// row of names and mana costs doesn't otherwise show. Two exceptions. Sorted
// by colour, the name carries the colour: the column beside it is a mana
// cost, which says the colour symbol by symbol but not at a glance. Sorted
// by type, the type column is already painted, and the name stays plain so
// the coloured bands are the types alone.
func nameColour(c mtg.Card, order, then cardSort) lipgloss.Color {
	if then == sortArrival {
		switch order {
		case sortColor:
			then = sortColor
		case sortType:
			return theme.Text
		default:
			then = sortType
		}
	}
	if col, ok := sortColour(c, then); ok {
		return col
	}
	return theme.Text
}

// sortColour is the colour an order paints a card. The categorical orders
// use the category's own colour — its colour, its type, its rarity. The
// numeric ones place the card on a ramp from cool to hot: mana value, power
// and toughness by the number itself, price and EDHREC rank by band, since a
// dollar or a rank more is no difference worth a colour.
func sortColour(c mtg.Card, s cardSort) (lipgloss.Color, bool) {
	switch s {
	case sortColor:
		return colourForCard(c.DisplayColors()), true
	case sortType:
		return stats.TypeColour(c.TypeLine), true
	case sortRarity:
		return stats.RarityColour(c.Rarity), true
	case sortMana:
		return stats.RampColour(int(manaSortValue(c))), true
	case sortPower, sortToughness:
		stat := c.Power
		if s == sortToughness {
			stat = c.Toughness
		}
		v, ok := statValue(stat)
		if !ok || v < 0 {
			return theme.TextMuted, true // no number, or a * with none honest
		}
		return stats.RampColour(v), true
	case sortUSD:
		v, ok := c.USD()
		if !ok {
			return theme.TextMuted, true
		}
		return stats.PriceColour(stats.PriceBand(v)), true
	case sortEDHREC:
		return stats.RampColour(edhrecBand(c.EDHRECRank)), true
	}
	return "", false
}

// edhrecBand puts a rank on the ramp: the most played cards hottest, the
// unranked coolest.
func edhrecBand(rank int) int {
	switch {
	case rank <= 0:
		return 0
	case rank <= 100:
		return 7
	case rank <= 500:
		return 6
	case rank <= 1000:
		return 5
	case rank <= 2500:
		return 4
	case rank <= 5000:
		return 3
	case rank <= 10000:
		return 2
	}
	return 1
}

// paintColumn renders the second column. A mana cost gets a colour per
// symbol, which is how you read a curve at a glance; anything else takes the
// colour the order gives the card — a type's, a rarity's, or a step on the
// ramp for a price, a rank, a power or a toughness.
func paintColumn(text string, c mtg.Card, order cardSort) string {
	if text == "" {
		return ""
	}
	if order.showsMana() {
		return paintMana(text)
	}
	// Everything else takes the colour its order gives the card: a type's,
	// a rarity's, or a place on the ramp for a number — the same colour the
	// name takes when the order is second, so the two sorts read alike.
	return lipgloss.NewStyle().Foreground(columnColour(c, order)).Render(text)
}

// columnColour is the second column's colour as one colour. A mana cost is
// painted symbol by symbol, which a background can't be, so as a block it
// takes the card's colour instead.
func columnColour(c mtg.Card, order cardSort) lipgloss.Color {
	if order.showsMana() {
		return colourForCard(c.DisplayColors())
	}
	if col, ok := sortColour(c, order); ok {
		return col
	}
	return theme.TextDim
}

// paintMana colours a rendered cost symbol by symbol. It works from the text
// rather than the symbol list because the ladder may have padded it, and the
// padding has to keep its place.
func paintMana(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch r {
		case ' ', '/':
			b.WriteString(lipgloss.NewStyle().Foreground(theme.TextMuted).Render(string(r)))
		default:
			b.WriteString(lipgloss.NewStyle().
				Foreground(symbolColour(string(r))).Render(string(r)))
		}
	}
	return b.String()
}

// layoutRow works the ladder, returning the name and the column already
// padded to fill the width between them.
func layoutRow(name, col string, colAbbreviates bool, width int) (string, string) {
	fits := func(n, c string) bool {
		need := textWidth(n) + textWidth(c)
		if c != "" {
			need += gap
		}
		return need <= width
	}

	// 1. As they are.
	if fits(name, col) {
		return padBetween(name, col, width)
	}

	// 2. Shorten the column, if it's the kind that can be.
	if colAbbreviates {
		if short := initialism(col, false); fits(name, short) {
			return padBetween(name, short, width)
		}
		col = initialism(col, false)
	}

	// 3. Shorten the name.
	short := initialism(name, true)
	if fits(short, col) {
		return padBetween(short, col, width)
	}

	// 4. Give up and cut. The column goes first: you can work out a mana
	// cost from the card, but not a name you can't read.
	if textWidth(short) <= width {
		return padBetween(short, "", width)
	}
	return truncate(short, width), ""
}

// layoutColumns places the name and its column against a shared name-column
// width, so the whole list abbreviates on the same boundary rather than each
// row on its own. The column is shortened when it would cross into the name's
// half, the name only when it overruns its own — the same order the ladder
// gives things up in, decided once for the list instead of per row.
func layoutColumns(name, col string, nameCol, body int) (string, string) {
	typeCol := maxInt(body-nameCol-gap, 1)

	// The column keeps its full form while it fits its half, then becomes an
	// initialism, then is cut — but the threshold is the shared column, not
	// this row's name, so a short name can't buy a longer column than its
	// neighbours show.
	if textWidth(col) > typeCol {
		col = initialism(col, false)
	}
	if textWidth(col) > typeCol {
		col = truncate(col, typeCol)
	}

	// The name keeps its column; only a name that overruns even that is given
	// up, and to an initialism before a cut, the way the ladder does it.
	if textWidth(name) > nameCol {
		if short := initialism(name, true); textWidth(short) <= nameCol {
			name = short
		} else {
			name = truncate(name, nameCol)
		}
	}

	return padBetween(name, col, body)
}

// nameColumnFor is the width the name column takes across a set of rows: the
// longest name, held back far enough that the widest abbreviated column still
// fits beside it. It is what turns the per-row ladder into aligned columns,
// and is only worth computing for a column long enough to collide with a name
// — the type line.
func nameColumnFor(cards []deck.Card, order cardSort, width int) int {
	body := maxInt(width-gutter, 1)

	fullName, abbrCol := 0, 0
	for _, c := range cards {
		if w := textWidth(cardName(c)); w > fullName {
			fullName = w
		}
		if w := textWidth(initialism(order.column(c.Card), false)); w > abbrCol {
			abbrCol = w
		}
	}

	nameCol := fullName
	// Leave room for the widest abbreviated column plus the gap; a name past
	// that point is shortened rather than allowed to push the column off.
	if room := body - gap - abbrCol; nameCol > room {
		nameCol = room
	}
	if nameCol < 1 {
		nameCol = 1
	}
	return nameCol
}

// padBetween puts the two columns at either end of the width.
func padBetween(left, right string, width int) (string, string) {
	space := width - textWidth(left) - textWidth(right)
	if space < 0 {
		space = 0
	}
	return left + strings.Repeat(" ", space), right
}

// initialism reduces a phrase to its first letters, keeping enough
// punctuation to stay recognisable.
//
//	Legendary Creature — Elf Faerie Noble  →  LC-EFN
//	Dwynen, Gilt-Leaf Daen                 →  D,GLD.
//	Miara, Thorn of the Glade              →  M,TotG.
//
// Case is preserved, which is what keeps the little words little: "of the"
// contributes "ot", not "OT". A dash between clauses survives as a dash,
// because a type line without it reads as one long word. The full stop marks
// a shortened *name* — a type line is obviously not a name either way.
func initialism(s string, trailingDot bool) string {
	var b strings.Builder

	for _, word := range strings.Fields(s) {
		// The slash between the halves of a split card — a name or a type
		// line — survives whole: "Invasion of Tarkir // Defiant Thundermaw"
		// shortens to "IoT//DT.", not "IoT-DT.", so the two halves stay
		// legibly two.
		if word == "//" {
			b.WriteString("//")
			continue
		}
		// A dash standing on its own is a separator, not a word.
		if isDash(word) {
			b.WriteString("-")
			continue
		}
		// Hyphenated words are several words wearing one coat: Gilt-Leaf
		// gives up G and L.
		for _, part := range splitHyphens(word) {
			trailing := ""
			part = strings.TrimRightFunc(part, func(r rune) bool {
				if r == ',' || r == ':' {
					trailing = string(r) + trailing
					return true
				}
				return false
			})
			for _, r := range part {
				b.WriteRune(r)
				break
			}
			b.WriteString(trailing)
		}
	}

	out := b.String()
	if out == "" {
		return out
	}
	if trailingDot {
		out += "."
	}
	return out
}

func isDash(w string) bool {
	switch w {
	case "-", "–", "—":
		return true
	}
	return false
}

func splitHyphens(word string) []string {
	parts := strings.FieldsFunc(word, func(r rune) bool { return r == '-' || r == '–' })
	if len(parts) == 0 {
		return []string{word}
	}
	return parts
}

// cardName is the name as the row shows it, with the quantity in front when
// a deck runs more than one.
func cardName(c deck.Card) string {
	if c.Qty > 1 {
		return itoa(c.Qty) + "x " + c.Card.Name
	}
	return c.Card.Name
}

// marker is the one character in front of a row.
//
// One character, so there is a precedence: what you just picked out matters
// more than what the card is, which matters more than where else it lives.
func marker(c deck.Card, st rowState) (string, lipgloss.Color) {
	switch {
	case st.selected:
		return "▸", theme.Marked
	case c.Commander:
		return "★", theme.Accent
	// A shape each as well as a colour, so the three read apart at a glance
	// — and for anyone who can't tell aqua from grey.
	case st.member == inEditing:
		return "●", theme.BorderEditing
	case st.member == inFocused:
		return "◆", theme.BorderFocus
	case st.member == inOther:
		return "▲", theme.MemberOther
	}
	return " ", ""
}
