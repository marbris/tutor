package ui

import (
	"sort"
	"strings"
	"unicode/utf8"

	"ttr/internal/tagger"
)

// Tab completion in the tag prompt, and of otag: in a Scryfall search.
//
// Tagging is the one prompt whose answers repeat: the same dozen tags go on
// card after card, and a tag typed slightly differently — "remova", "Ramp " —
// is a new tag rather than a typo, silently splitting a category in two. So
// tab finishes the word being typed from the tags that already exist, the
// way a shell finishes a filename:
//
//   - one tag fits: it is filled in, with a space after it for the next;
//   - several fit: the word grows to what they have in common, if anything,
//     and they are listed on the notice line;
//   - tab again walks through them, shift+tab back, and the walk passes
//     through what the first tab left on its way round — so tab when you
//     don't know whether there's anything to finish never costs you what
//     you typed.
//
// A leading - (take the tag off) is kept, and the completion is of the tag
// after it.

// tagCompletion is a walk through the tags that fit, while tab is being
// pressed. Any other key ends it.
type tagCompletion struct {
	before string   // the input up to the word being completed, and its "-"
	fits   []string // the tags that fit, in order
	at     int      // which one is showing; -1 is origin
	origin string   // what the first tab left, the walk's stop between last and first
	shown  string   // what the input held after the last tab
}

// step moves the walk one along (delta 1) or back (-1), origin included,
// and says what the input should hold.
func (c *tagCompletion) step(delta int) string {
	n := len(c.fits) + 1
	c.at = (c.at+1+delta+n)%n - 1
	if c.at < 0 {
		return c.origin
	}
	return c.before + c.fits[c.at]
}

// startWalk begins a walk on the first tab: the input grows to filled, what
// every fit shares, where that's longer than what was typed, and otherwise
// stays as it is. Nothing is walked until the next tab.
func startWalk(before string, fits []string, value, filled string, typed int, set func(string) string) *tagCompletion {
	c := &tagCompletion{before: before, fits: fits, at: -1, origin: value}
	if len(filled) > typed {
		c.origin = set(filled)
	}
	c.shown = c.origin
	return c
}

// knownTags is every tag in the decks on screen, the editing deck's among
// them, in order.
func (m Model) knownTags() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range m.ws.panels {
		l := p.cardsView()
		if l == nil || l.deck == nil {
			continue
		}
		for _, c := range l.all {
			for _, t := range c.Tags {
				if !seen[t] {
					seen[t] = true
					out = append(out, t)
				}
			}
		}
	}
	for _, t := range globalTags.all() {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// completeTag is tab (delta 1) or shift+tab (delta -1) in the tag prompt.
func (m *Model) completeTag(p *panel, delta int) {
	value := p.askInput.Value()
	// The word being completed is the last one; tags are single words.
	start := strings.LastIndex(value, " ") + 1
	before, word := value[:start], value[start:]
	if strings.HasPrefix(word, "-") {
		before, word = before+"-", word[1:]
	}
	set := func(s string) string { p.setAsk(s); return p.askInput.Value() }
	known := m.knownTags()
	// Which tags to move: the ones this list has to give.
	if l := p.cardsView(); p.asking == askTagMove && l != nil {
		known = sourceTags(l)
	}
	m.complete(&p.tagComp, value, set, delta, before, strings.ToLower(word), known)
}

// complete is the shell-style tab over the word at the end of an input:
// value is what the input holds, before and word that split at the word
// being completed, and known what it can become. set puts new text in the
// input and says what it then holds. comp is the walk in progress, if any.
func (m *Model) complete(comp **tagCompletion, value string, set func(string) string,
	delta int, before, word string, known []string) {
	// Still walking: the input is what the last tab left, so step on.
	if c := *comp; c != nil && c.shown == value && len(c.fits) > 1 {
		c.shown = set(c.step(delta))
		m.notice = tagList(c.fits, c.at)
		return
	}
	*comp = nil

	if len(known) == 0 {
		m.notice = "no tags yet to complete from"
		return
	}
	var fits []string
	for _, t := range known {
		if strings.HasPrefix(t, word) {
			fits = append(fits, t)
		}
	}
	switch len(fits) {
	case 0:
		m.notice = `no tag starts with "` + word + `"`
		return
	case 1:
		set(before + fits[0] + " ")
		return
	}

	*comp = startWalk(before, fits, value, before+commonPrefix(fits), len(before)+len(word), set)
	m.notice = tagList(fits, -1)
}

// otagFields are the search keywords that take an oracle tag.
var otagFields = []string{"otag:", "oracletag:", "function:"}

// otagWord splits a Scryfall query whose last word is an oracle tag being
// typed — otag:remo, -otag:ra, (function:card- — into what comes before
// the tag's name and the name so far.
func otagWord(value string) (before, word string, ok bool) {
	start := strings.LastIndex(value, " ") + 1
	token := value[start:]
	lead := len(token) - len(strings.TrimLeft(token, "-("))
	for _, f := range otagFields {
		if rest := token[lead:]; len(rest) >= len(f) && strings.EqualFold(rest[:len(f)], f) {
			cut := start + lead + len(f)
			return value[:cut], strings.ToLower(value[cut:]), true
		}
	}
	return "", "", false
}

// canCompleteOtag reports whether tab in the search bar would complete an
// oracle tag rather than change the bar's target.
func (m Model) canCompleteOtag(p *panel) bool {
	if p.kind != KindFind || tagger.Current() == nil {
		return false
	}
	_, _, ok := otagWord(p.search.Value())
	return ok
}

// completeOtag is tab in the search bar, on an otag: word: the tag's name
// finished from Scryfall Tagger's tags.
func (m *Model) completeOtag(p *panel, delta int) {
	value := p.search.Value()
	before, word, _ := otagWord(value)
	set := func(s string) string {
		p.search.SetValue(s)
		p.search.CursorEnd()
		return p.search.Value()
	}
	m.complete(&p.otagComp, value, set, delta, before, word, tagger.Current().Labels(""))
}

// setAsk replaces what the prompt holds, the cursor at the end of it.
func (p *panel) setAsk(s string) {
	p.askInput.SetValue(s)
	p.askInput.CursorEnd()
}

// tagList is the tags that fit, for the notice line, the one showing in
// brackets.
func tagList(tags []string, at int) string {
	shown := make([]string, len(tags))
	for i, t := range tags {
		if i == at {
			t = "[" + t + "]"
		}
		shown[i] = t
	}
	return strings.Join(shown, " · ")
}

// commonPrefix is what every string starts with.
func commonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	prefix := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, prefix) {
			_, size := utf8.DecodeLastRuneInString(prefix)
			prefix = prefix[:len(prefix)-size]
		}
	}
	return prefix
}
