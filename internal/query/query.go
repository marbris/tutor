// Package query is Tutor's search language: a part of Scryfall's syntax,
// matched against the cards in a list (the / filter) and against every card
// there is (a search, over internal/cardstore).
//
//	t:creature  o:"draw a card"  mv<=3  c:rg  id:bant  pow>=4  is:commander
//	otag:ramp  tag:wincon  f:commander  r:mythic  usd<5  kw:flying  set:mh3
//	-t:land  (t:instant or t:sorcery)  !"sol ring"
//
// Terms side by side must all match; "or" between them is either; a leading
// - turns a term round; parentheses group. A plain word is matched as /
// always has, against the name, the text, the type line and the tags — or,
// in a search, against the name alone, as on Scryfall. A word that looks
// like a keyword but isn't one Tutor knows is plain text too, and Unknown
// names it, so a search can offer to ask Scryfall instead.
package query

import (
	"strings"
	"unicode"

	"ttr/internal/cardstore"
	"ttr/internal/deck"
)

// Query is a parsed filter. The zero Query matches everything.
type Query struct {
	root    node
	unknown []string
}

// Empty reports whether the query narrows nothing.
func (q Query) Empty() bool { return q.root == nil }

// Unknown is the words that looked like keywords Tutor doesn't know, such
// as a:"rk post" or year>2020, which were matched as plain text.
func (q Query) Unknown() []string { return q.unknown }

// Match reports whether a card in a list passes the filter.
func (q Query) Match(c deck.Card) bool {
	if q.root == nil {
		return true
	}
	f := cardstore.Of(c.Card, c.AllTags())
	return q.root.match(&f)
}

// MatchFacts reports whether a card passes, by its facts.
func (q Query) MatchFacts(f *cardstore.Facts) bool {
	return q.root == nil || q.root.match(f)
}

// Parse reads a filter. It never fails: what it can't read as a keyword is
// plain text, and an unbalanced parenthesis closes itself.
func Parse(s string) Query { return parse(s, false) }

// ParseSearch reads a search: as Parse, but a plain word is looked for in
// the name alone, as Scryfall does.
func ParseSearch(s string) Query { return parse(s, true) }

func parse(s string, namesOnly bool) Query {
	p := parser{toks: tokenize(s), namesOnly: namesOnly}
	var parts andNode
	for p.at < len(p.toks) {
		if n := p.or(); n != nil {
			parts = append(parts, n)
		}
		// A ")" with no "(" before it stops the parse there; step over it.
		if t, ok := p.peek(); ok && t.kind == tokClose {
			p.at++
		}
	}
	switch len(parts) {
	case 0:
		return Query{unknown: p.unknown}
	case 1:
		return Query{root: parts[0], unknown: p.unknown}
	}
	return Query{root: parts, unknown: p.unknown}
}

// ── The tree ────────────────────────────────────────────────────

type node interface{ match(*cardstore.Facts) bool }

type andNode []node
type orNode []node
type notNode struct{ n node }

func (a andNode) match(c *cardstore.Facts) bool {
	for _, n := range a {
		if !n.match(c) {
			return false
		}
	}
	return true
}

func (o orNode) match(c *cardstore.Facts) bool {
	for _, n := range o {
		if n.match(c) {
			return true
		}
	}
	return false
}

func (n notNode) match(c *cardstore.Facts) bool { return !n.n.match(c) }

// ── Tokens ──────────────────────────────────────────────────────

type tokKind int

const (
	tokWord tokKind = iota
	tokOpen
	tokClose
	tokOr
)

type token struct {
	kind tokKind
	text string // the word, its quotes removed, lowercased
	neg  bool   // a leading -
}

// tokenize splits a filter into words, parentheses and "or". Quotes keep a
// phrase together, inside a word too: o:"draw a card".
func tokenize(s string) []token {
	var toks []token
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
			continue
		case r == '(':
			toks = append(toks, token{kind: tokOpen})
			i++
			continue
		case r == ')':
			toks = append(toks, token{kind: tokClose})
			i++
			continue
		}
		neg := false
		if r == '-' && i+1 < len(rs) && !unicode.IsSpace(rs[i+1]) {
			neg = true
			i++
		}
		if i < len(rs) && rs[i] == '(' && neg {
			// -( … ): the minus applies to the group.
			toks = append(toks, token{kind: tokOpen, neg: true})
			i++
			continue
		}
		var b strings.Builder
		inQuote := false
		for i < len(rs) {
			r := rs[i]
			if r == '"' {
				inQuote = !inQuote
				i++
				continue
			}
			if !inQuote && (unicode.IsSpace(r) || r == '(' || r == ')') {
				break
			}
			b.WriteRune(r)
			i++
		}
		w := strings.ToLower(b.String())
		if w == "" {
			continue
		}
		if w == "or" && !neg {
			toks = append(toks, token{kind: tokOr})
			continue
		}
		toks = append(toks, token{kind: tokWord, text: w, neg: neg})
	}
	return toks
}

// ── Parsing ─────────────────────────────────────────────────────

type parser struct {
	toks []token
	at   int
	// namesOnly is a search's plain words: names, not text.
	namesOnly bool
	unknown   []string
}

func (p *parser) peek() (token, bool) {
	if p.at >= len(p.toks) {
		return token{}, false
	}
	return p.toks[p.at], true
}

// or is and-groups joined by "or".
func (p *parser) or() node {
	var alts orNode
	for {
		if n := p.and(); n != nil {
			alts = append(alts, n)
		}
		t, ok := p.peek()
		if !ok || t.kind != tokOr {
			break
		}
		p.at++
	}
	switch len(alts) {
	case 0:
		return nil
	case 1:
		return alts[0]
	}
	return alts
}

// and is terms side by side, up to an "or", a ")" or the end.
func (p *parser) and() node {
	var all andNode
	for {
		t, ok := p.peek()
		if !ok || t.kind == tokOr || t.kind == tokClose {
			break
		}
		p.at++
		var n node
		if t.kind == tokOpen {
			n = p.or()
			if c, ok := p.peek(); ok && c.kind == tokClose {
				p.at++
			}
			if n == nil {
				continue
			}
		} else if n = p.term(t.text); n == nil {
			continue
		}
		if t.neg {
			n = notNode{n}
		}
		all = append(all, n)
	}
	switch len(all) {
	case 0:
		return nil
	case 1:
		return all[0]
	}
	return all
}
