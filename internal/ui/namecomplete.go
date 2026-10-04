package ui

import (
	"sort"
	"strings"

	"ttr/internal/catalog"
	"ttr/internal/tagger"
)

// Tab completion of card names in the i bar, from Scryfall's list of every
// card's name — the way a card you already know is added without spelling
// it out, or looking it up first.
//
// tab completes what is being typed: a card's name, or on the otag side the
// tag at the end. With nothing being typed, tab swaps the sides, as it
// always has. The completion is the tag prompt's, shell-style: one fit is
// filled in; several grow to what they share and are listed; tab again
// walks them, shift+tab back.

// nameIndex is the card names, sorted, with their lowercased forms for
// matching, built once for each set of catalogs.
var nameIndex struct {
	from  *catalog.Data
	names []string
	lower []string
}

func knownCardNames() (names, lower []string) {
	c := catalog.Current()
	if c == nil {
		return nil, nil
	}
	if nameIndex.from != c {
		names := append([]string(nil), c.List(catalog.CardNames)...)
		sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
		lower := make([]string, len(names))
		for i, n := range names {
			lower[i] = strings.ToLower(n)
		}
		nameIndex.from, nameIndex.names, nameIndex.lower = c, names, lower
	}
	return nameIndex.names, nameIndex.lower
}

// namesStarting is every card name that starts with prefix, ignoring case.
func namesStarting(prefix string) []string {
	names, lower := knownCardNames()
	p := strings.ToLower(prefix)
	i := sort.SearchStrings(lower, p)
	var out []string
	for ; i < len(lower) && strings.HasPrefix(lower[i], p); i++ {
		out = append(out, names[i])
	}
	return out
}

// addBarTyping reports whether tab in the i bar completes rather than swaps
// sides: something is being typed — a card name, or a tag at the end.
func addBarTyping(p *panel) bool {
	v := p.askInput.Value()
	switch p.asking {
	case askAddCard:
		return strings.TrimSpace(v) != ""
	case askOtag:
		return v != "" && !strings.HasSuffix(v, " ")
	}
	return false
}

// tabInAddBar is tab (delta 1) or shift+tab (-1) in the i bar.
func (m *Model) tabInAddBar(p *panel, delta int) {
	if !addBarTyping(p) {
		if delta > 0 {
			p.swapAddOtag()
		}
		return
	}
	if p.asking == askOtag {
		m.completeOtagInBar(p, delta)
		return
	}
	m.completeName(p, delta)
}

// completeOtagInBar finishes the tag at the end of the otag side.
func (m *Model) completeOtagInBar(p *panel, delta int) {
	labels := tagger.Current().Labels("")
	if len(labels) == 0 {
		m.notice = "Scryfall Tagger's tags aren't in yet"
		return
	}
	value := p.askInput.Value()
	start := strings.LastIndex(value, " ") + 1
	set := func(s string) string { p.setAsk(s); return p.askInput.Value() }
	m.complete(&p.tagComp, value, set, delta, value[:start], strings.ToLower(value[start:]), labels)
}

// completeName finishes the card name in the i bar.
func (m *Model) completeName(p *panel, delta int) {
	value := p.askInput.Value()
	set := func(s string) string { p.setAsk(s); return p.askInput.Value() }

	// Still walking: the input is what the last tab left, so step on.
	if c := p.tagComp; c != nil && c.shown == value && len(c.fits) > 1 {
		c.at = stepWalk(c.at, delta, len(c.fits))
		c.shown = set(c.fits[c.at])
		m.notice = nameList(c.fits, c.at)
		return
	}
	p.tagComp = nil

	if names, _ := knownCardNames(); len(names) == 0 {
		m.notice = "card names aren't downloaded yet"
		return
	}
	typed := strings.TrimLeft(value, " ")
	fits := namesStarting(typed)
	switch len(fits) {
	case 0:
		m.notice = `no card starts with "` + typed + `"`
		return
	case 1:
		set(fits[0])
		m.notice = ""
		return
	}

	c := &tagCompletion{fits: fits, at: -1}
	if common := commonPrefixFold(fits); len(common) > len(typed) {
		c.shown = set(common)
	} else {
		c.at = stepWalk(-1, delta, len(fits))
		c.shown = set(fits[c.at])
	}
	p.tagComp = c
	m.notice = nameList(fits, c.at)
}

// stepWalk is the next place in a walk of n, from at (-1 before it starts).
func stepWalk(at, delta, n int) int {
	switch {
	case at < 0 && delta > 0:
		return 0
	case at < 0:
		return n - 1
	}
	return (at + delta + n) % n
}

// commonPrefixFold is what every name starts with, ignoring case, in the
// first name's own letters.
func commonPrefixFold(names []string) string {
	lower := make([]string, len(names))
	for i, n := range names {
		lower[i] = strings.ToLower(n)
	}
	return names[0][:len(commonPrefix(lower))]
}

// nameList is the names that fit, for the notice line: how many, and a
// few around the one showing, in brackets.
func nameList(names []string, at int) string {
	const shown = 6
	from := 0
	if at >= shown {
		from = at - shown + 1
	}
	to := min(from+shown, len(names))
	head := itoa(len(names)) + " cards: "
	if from > 0 {
		head += "… "
	}
	tail := ""
	if to < len(names) {
		tail = " · …"
	}
	return head + tagList(names[from:to], at-from) + tail
}
