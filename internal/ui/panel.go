package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/theme"
)

// A panel is one vertical strip of the workspace: a search bar over whatever
// that search turned up.
//
// The bar and the header are the same line. You type into it to fill the
// panel, and once something is in there the line stops being an input and
// starts describing what you're looking at — pressing i turns it back. A
// panel that permanently spent a row on an empty search box would be paying
// for it in the one direction there is never enough of.

// Kind is what a panel searches. It decides the panel's keymap and what its
// search bar means; the design doc's ctrl+f / ctrl+l / ctrl+r are the space
// prefix's f, d and r.
type Kind int

const (
	// KindNew is a panel that hasn't been told what it is yet. Tab in its
	// search bar cycles through the others.
	KindNew Kind = iota
	KindFind
	KindDecks
	KindRules
	KindCards
	// KindSettings is the settings panel, space c. Not a search target.
	KindSettings
)

// kinds is the cycle tab walks, leaving out the two that aren't a search
// target: an untyped panel, and a card list, which is something a search
// produces rather than something you ask for.
var kinds = []Kind{KindFind, KindDecks, KindRules}

func (k Kind) String() string {
	switch k {
	case KindFind:
		return "find"
	case KindDecks:
		return "decks"
	case KindRules:
		return "rules"
	case KindCards:
		return "cards"
	case KindSettings:
		return "settings"
	}
	return "new"
}

// prompt is what the search bar offers to do, which is the only thing
// distinguishing an empty panel of one kind from another.
func (k Kind) prompt() string {
	switch k {
	case KindFind:
		return "scryfall"
	case KindDecks:
		return "moxfield user or deck url"
	case KindRules:
		return "rules"
	}
	return "tab to choose"
}

// barLabel is what opening the bar with i offers to do. prompt names the
// thing, which reads right after enter; this names the act, which reads
// right beside a key you press to get there.
func (k Kind) barLabel() string {
	switch k {
	case KindFind:
		return "search scryfall"
	case KindDecks:
		return "search moxfield"
	case KindRules:
		return "search the rules"
	}
	return "the search bar"
}

func (k Kind) placeholder() string {
	switch k {
	case KindFind:
		return "t:creature c:R cmc<=3 otag:removal"
	case KindDecks:
		return "a moxfield username, or a deck url"
	case KindRules:
		return "flying, 702.9, sacrifice"
	}
	return "tab: scryfall · decks · rules"
}

// next moves a panel to the following search target, wrapping. An untyped
// panel lands on the first rather than the second.
func (k Kind) next(delta int) Kind {
	at := 0
	for i, c := range kinds {
		if c == k {
			at = i + delta
			break
		}
	}
	n := len(kinds)
	return kinds[((at%n)+n)%n]
}

type panel struct {
	// id survives reordering and closing, which an index doesn't — a search
	// in flight has to find its way back to the panel that asked for it.
	id   int
	kind Kind

	// search is the bar, and searchOpen says whether it's showing. It starts
	// open on a new panel because an empty panel is a question.
	search     textinput.Model
	searchOpen bool

	// title is what the header says once the bar has closed.
	title string

	// stack is what the panel is showing, innermost last. A deck's versions
	// sit on top of the decks list; esc pops back rather than closing.
	stack []view

	// previewing means the stack holds a tab-preview rather than committed
	// content: tabbing to the decks target fills the body with the decks
	// list while the bar is still open over it, so the decks appear without
	// waiting for enter. tab may freely replace a preview; it must not touch
	// a real search or deck reached by reopening the bar with i.
	previewing bool

	// filtering is the / prompt, open only while you're typing in it.
	filtering bool
	// filterBefore is the filter the prompt opened on, which esc puts back.
	filterBefore string
	filterInput  textinput.Model

	// asking is the one-line prompt — a new deck's name, a rename, a URL
	// to follow — open only while you're answering it.
	asking   askKind
	askInput textinput.Model
	// tagComp is tab completion under way in the tag prompt.
	tagComp *tagCompletion
	// tagMoveAdd is whether the tags being asked for are T a's, which adds
	// the cards the editing deck lacks, rather than T t's.
	tagMoveAdd bool
	// otagComp is tab completion under way of an otag: in the search bar.
	otagComp *tagCompletion
	// writeToNewPane remembers whether it was w or W that raised the
	// save-as prompt, since the answer arrives long after the key.
	writeToNewPane bool

	// loading is a request in flight; err is the last one that failed.
	loading bool
	err     error
	// total is how many cards the query matched, which is usually more than
	// were fetched.
	total int

	// querySort is the order the *request* asks for — which cards come back
	// — as an index into scryfall.SortOptions.
	querySort int
	// queryDir is which way that order runs, as an index into
	// scryfall.DirOptions. Zero is auto: Scryfall's choice.
	queryDir int
	// lastSorts is the list a failed search threw away, kept only for its
	// orders, so the next search that works is laid out the same.
	lastSorts *cardList
	// pending is how the list this panel is waiting for was laid out when
	// the last session ended, applied once it arrives.
	pending *panelSession

	// historyAt is where up and down have walked to, and draft is what was
	// in the bar before the walk started. The queries themselves belong to
	// the Model — one list, shared by every find panel — and are passed in
	// when the walk asks for them. A panel holding its own copy is how a
	// panel came to be opened with an empty one.
	historyAt int
	draft     string
}

func newPanel(kind Kind) *panel {
	in := textinput.New()
	in.Prompt = "⌕ "
	in.Placeholder = kind.placeholder()
	in.Focus()

	f := textinput.New()
	f.Prompt = "/"

	p := &panel{
		kind: kind, search: in, searchOpen: true, filterInput: f,
		historyAt: historyIdle,
		// EDHREC rank is the useful default for a Commander player: the
		// cards other people actually play come back first.
		querySort: defaultQuerySort,
	}
	p.restyle()
	return p
}

// show replaces whatever the panel held with a view, which is what turns a
// search bar into a header.
func (p *panel) show(v view) {
	p.title = v.title()
	p.stack = []view{v}
	p.previewing = false
	p.searchOpen = false
	p.search.Blur()
	// Whatever was being waited for has arrived, by definition.
	p.loading = false
	p.err = nil
}

// push steps into something reached from the current view — a deck's
// versions, a user's decks — keeping the way back.
func (p *panel) push(v view) {
	p.stack = append(p.stack, v)
	p.previewing = false
}

// pop steps back out, reporting whether there was anywhere to go.
func (p *panel) pop() bool {
	if len(p.stack) < 2 {
		return false
	}
	p.stack = p.stack[:len(p.stack)-1]
	p.title = p.stack[len(p.stack)-1].title()
	return true
}

// top is what the panel is showing now, or nil before anything has filled it.
func (p *panel) top() view {
	if len(p.stack) == 0 {
		return nil
	}
	return p.stack[len(p.stack)-1]
}

// cardsView is the top view when it happens to be a list of cards, which is
// what the membership marks and the editing deck are about.
func (p *panel) cardsView() *cardList {
	l, _ := p.top().(*cardList)
	return l
}

// filterable is a view that / can narrow.
//
// An interface rather than a type switch, because the switch had no case for
// the rules panel and so / did nothing there — silently, since a filter that
// matches everything looks exactly like one that wasn't applied. A view that
// can be narrowed now says so in its own file.
type filterable interface {
	setFilter(string)
	filterText() string
}

// setFilter narrows whatever the panel is showing. A view that can't be
// narrowed simply ignores it.
func (p *panel) setFilter(s string) {
	if v, ok := p.top().(filterable); ok {
		v.setFilter(s)
	}
}

// clearFilter drops the text filter on whatever is showing, reporting
// whether there was one to drop.
func (p *panel) clearFilter() bool {
	v, ok := p.top().(filterable)
	if !ok || v.filterText() == "" {
		return false
	}
	v.setFilter("")
	return true
}

// openFilter raises the / prompt over whatever is showing.
func (p *panel) openFilter(current string) {
	p.filtering = true
	p.filterBefore = current
	p.filterInput.SetValue(current)
	p.filterInput.CursorEnd()
	p.filterInput.Focus()
	p.filterInput.Placeholder = ""
}

// restyle repaints the search bar. Colours are read at render time rather
// than at construction, so switching theme doesn't need the panels rebuilt.
func (p *panel) restyle() {
	p.search.PromptStyle = lipgloss.NewStyle().Foreground(theme.Accent)
	p.search.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	p.search.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.TextMuted)
	p.filterInput.PromptStyle = lipgloss.NewStyle().Foreground(theme.Highlight)
	p.filterInput.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	p.filterInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.TextMuted)
}

// setKind retargets a panel, which only means anything while it's empty.
func (p *panel) setKind(k Kind) {
	p.kind = k
	p.search.Placeholder = k.placeholder()
}

// header is the line at the top of a panel: whichever text field is open,
// otherwise a description of what's below.
//
// It returns whether the line is already styled, because an input's View
// carries its own colour — and measuring that with runeLen counts the escape
// sequences as characters, which pads it to the wrong width and wraps the
// line. A panel one row taller than its neighbours is the visible symptom.
func (p *panel) header(width int) (string, bool) {
	switch {
	case p.searchOpen:
		fitInput(&p.search, width)
		return p.search.View(), true
	case p.asking != askNone:
		fitInput(&p.askInput, width)
		return p.askInput.View(), true
	case p.filtering:
		fitInput(&p.filterInput, width)
		return p.filterInput.View(), true
	}

	name := p.title
	if name == "" {
		name = p.kind.String()
	}
	return name, false
}

// fitInput sizes a text field to the panel. An empty one draws its
// placeholder, and the text field takes its prompt off the width for that
// itself: given the room left after the prompt, as typed text needs, it
// took the prompt off twice and cut the placeholder short.
func fitInput(in *textinput.Model, width int) {
	if in.Value() == "" {
		in.Width = width
		return
	}
	in.Width = inputWidth(width, in.Prompt)
}

// inputWidth is how much room a text field's text has: the panel, less its
// prompt, less one for the cursor sitting past the end of what you typed.
func inputWidth(width int, prompt string) int {
	return maxInt(width-textWidth(prompt)-1, 4)
}

// empty reports whether a panel has nothing in it yet, which is what makes
// esc close it rather than clear something.
func (p *panel) empty() bool { return len(p.stack) == 0 && p.title == "" }

// subtitle is the count line under the header: how many cards, and what has
// been done to narrow or reorder them.
func (p *panel) subtitle() string {
	// While the bar is open a find panel says what order it will ask for,
	// since that's the one thing about the request you can't see in the bar.
	if p.searchOpen {
		if p.kind == KindFind {
			return "order: " + p.queryOrderLabel() + " · ctrl+o/r"
		}
		return ""
	}
	if v := p.top(); v != nil {
		return v.subtitle()
	}
	return ""
}

// headerRows is everything under the panel's title, a row of segments per
// kind of fact. A list of cards has three; other views have their subtitle
// as a single row.
func (p *panel) headerRows() [][]string {
	if p.searchOpen {
		if p.kind == KindFind {
			return [][]string{{"order: " + p.queryOrderLabel(), "ctrl+o/r"}}
		}
		return nil
	}
	if l := p.cardsView(); l != nil {
		order := ""
		if l.deck == nil && p.kind == KindFind {
			order = p.queryOrderLabel()
		}
		return l.headerRows(order)
	}
	if v := p.top(); v != nil {
		if sub := v.subtitle(); sub != "" {
			return [][]string{{sub}}
		}
	}
	return nil
}

// subLines lays the header rows out at a width. Each row wraps onto as many
// lines as it needs, breaking between segments — and inside one only when it
// is wider than the panel on its own, a long filter say. Nothing is cut off:
// a header that ends in "…" is hiding the one fact you were looking for.
func (p *panel) subLines(width int) []string {
	var out []string
	for _, row := range p.headerRows() {
		var parts []string
		for _, seg := range row {
			if seg == "" {
				continue
			}
			if textWidth(seg) <= width {
				parts = append(parts, seg)
				continue
			}
			parts = append(parts, hardWrap(seg, width)...)
		}
		out = append(out, packStyled(parts, " · ", width)...)
	}
	return out
}

// hardWrap wraps on spaces, and breaks a word that is wider than the width on
// its own rather than letting it overrun.
func hardWrap(s string, width int) []string {
	var out []string
	for _, line := range wrap(s, width) {
		r := []rune(line)
		for textWidth(string(r)) > width {
			n := 0
			for n < len(r) && textWidth(string(r[:n+1])) <= width {
				n++
			}
			n = maxInt(n, 1)
			out = append(out, string(r[:n]))
			r = r[n:]
		}
		out = append(out, string(r))
	}
	return out
}
