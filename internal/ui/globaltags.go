package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ttr/internal/deck"
	"ttr/internal/paths"
)

// Global tags: tags that hold across every list on screen.
//
// A list's tags describe that list, and they also set the dictionary while
// you work. Every list open on screen lends its tags to the others, and so
// does every list pinned with t in the decks panel, open or not — often a
// list of format: tags, a card a line, each with its tags. space t on a list
// stops it lending (mutes it), and again starts it.
//
// While a list lends, its tags count as every other list's own: a deck with
// nothing tagged ramp still shows ramp in its statistics, and / and the
// statistics filter find it, when another list says the card is ramp. They
// are never written into the deck; the deck's own tags stay its own, and
// T g is how you copy them in.
//
// Before 8.0.0 these were "tag lists", kept in taglists.json, and only the
// pinned ones lent; that file is still read when globaltags.json isn't there.

const (
	pinnedFile    = "globaltags.json"
	oldPinnedFile = "taglists.json"
)

// tagIndex is the lists lending the global tags, and what they say.
type tagIndex struct {
	// slugs is the pinned lists, which lend whether open or not.
	slugs []string
	// from is what each lending list says: its key (a local list's slug),
	// then each card's tags by lowercased name — and by its front face too,
	// for a list that names a two-faced card by it. A list can then be left
	// out of what it lends itself.
	from map[string]map[string][]string
	// muted is the lists space t has stopped lending, by key.
	muted map[string]bool
	// gen counts rebuilds, for the statistics to know the tags have moved.
	gen int
	// sig is what the lists on screen were at the last rebuild, to know
	// when to rebuild again.
	sig string
}

// globalTags is the one index, shared by every list, because the lists
// narrow themselves without being told what the workspace holds.
var globalTags = &tagIndex{}

func pinnedPath() string { return filepath.Join(paths.State(), pinnedFile) }

// loadPinned reads which lists were pinned, dropping any since deleted.
func loadPinned() []string {
	body, err := os.ReadFile(pinnedPath())
	if errors.Is(err, os.ErrNotExist) {
		body, err = os.ReadFile(filepath.Join(paths.State(), oldPinnedFile))
	}
	if err != nil {
		return nil
	}
	var slugs []string
	if json.Unmarshal(body, &slugs) != nil {
		return nil
	}
	var out []string
	for _, s := range slugs {
		if deck.Exists(s) {
			out = append(out, s)
		}
	}
	return out
}

func savePinned(slugs []string) {
	body, err := json.Marshal(slugs)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(pinnedPath()), 0755)
	if os.WriteFile(pinnedPath(), body, 0644) == nil {
		os.Remove(filepath.Join(paths.State(), oldPinnedFile))
	}
}

func (x *tagIndex) active(slug string) bool {
	for _, s := range x.slugs {
		if s == slug {
			return true
		}
	}
	return false
}

// toggle pins a list or unpins it, and says which.
func (x *tagIndex) toggle(slug string) bool {
	for i, s := range x.slugs {
		if s == slug {
			x.slugs = append(x.slugs[:i:i], x.slugs[i+1:]...)
			return false
		}
	}
	x.slugs = append(x.slugs, slug)
	return true
}

// empty reports whether no list lends a tag.
func (x *tagIndex) empty() bool { return len(x.from) == 0 }

// of is the tags the global tags give a card, but for what the list named
// except lends: a list sees its own tags as its own, not as borrowed ones.
func (x *tagIndex) of(name, except string) []string {
	if len(x.from) == 0 {
		return nil
	}
	k := strings.ToLower(name)
	front, _, two := strings.Cut(k, " // ")
	var out []string
	for key, tags := range x.from {
		if key == except {
			continue
		}
		t, ok := tags[k]
		if !ok && two {
			t = tags[front]
		}
		out = deck.ApplyTagEdits(out, t, nil)
	}
	return out
}

// all is every tag the global tags hold, for completion.
func (x *tagIndex) all() []string {
	seen := map[string]bool{}
	var out []string
	for _, tags := range x.from {
		for _, ts := range tags {
			for _, t := range ts {
				if !seen[t] {
					seen[t] = true
					out = append(out, t)
				}
			}
		}
	}
	return out
}

// rebuild reads the lists again: every list in open, which are the ones on
// screen, and the pinned ones that aren't — but none that is muted. A list
// on screen is read from there, since its latest edit may not have reached
// the file yet.
func (x *tagIndex) rebuild(open map[string][]deck.Card) {
	x.gen++
	x.from = map[string]map[string][]string{}
	add := func(key, name string, tags []string) {
		if len(tags) == 0 {
			return
		}
		into := x.from[key]
		if into == nil {
			into = map[string][]string{}
			x.from[key] = into
		}
		k := strings.ToLower(name)
		into[k] = deck.ApplyTagEdits(into[k], tags, nil)
		if front, _, ok := strings.Cut(k, " // "); ok {
			into[front] = deck.ApplyTagEdits(into[front], tags, nil)
		}
	}
	for key, cards := range open {
		if x.muted[key] {
			continue
		}
		for _, c := range cards {
			add(key, c.Card.Name, c.Tags)
		}
	}
	for _, slug := range x.slugs {
		if _, ok := open[slug]; ok || x.muted[slug] {
			continue
		}
		f, err := deck.Read(slug)
		if err != nil {
			continue
		}
		for _, e := range f.Entries {
			add(slug, e.Name, e.Tags)
		}
	}
}

// lenderKey is what a list lends the global tags under: a local list's
// slug, or a Moxfield deck's id. A search has none; it has no tags to lend.
func (l *cardList) lenderKey() string {
	if l == nil || l.deck == nil {
		return ""
	}
	if l.deck.Local() {
		return l.deck.Slug
	}
	if id := firstOf(l.deck.ID, l.deck.URL, l.deck.Name); id != "" {
		return "moxfield:" + id
	}
	return ""
}

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// lends reports whether the list lends its tags: muted, it doesn't.
func (x *tagIndex) lends(key string) bool { return key != "" && !x.muted[key] }

// effective is a card with Borrowed filled in: the tags the global tags give
// it that it hasn't of its own, from every list but self. It counts and
// narrows by both; the card in the list is left as it is.
func effective(c deck.Card, self string) deck.Card {
	c.Borrowed = nil
	for _, t := range globalTags.of(c.Card.Name, self) {
		mine := false
		for _, o := range c.Tags {
			if strings.EqualFold(o, t) {
				mine = true
				break
			}
		}
		if !mine {
			c.Borrowed = append(c.Borrowed, t)
		}
	}
	return c
}

func effectiveAll(cards []deck.Card, self string) []deck.Card {
	if globalTags.empty() {
		return cards
	}
	out := make([]deck.Card, len(cards))
	for i, c := range cards {
		out[i] = effective(c, self)
	}
	return out
}

// openLenders is the lists on screen that could lend, by key, and a
// signature of them that changes whenever what they say might have: a list
// opened, closed, loaded, edited or muted.
func (m Model) openLenders() (map[string][]deck.Card, string) {
	open := map[string][]deck.Card{}
	var sig []string
	for _, p := range m.ws.panels {
		l := p.cardsView()
		key := l.lenderKey()
		if key == "" {
			continue
		}
		if _, seen := open[key]; seen {
			continue
		}
		open[key] = l.all
		sig = append(sig, fmt.Sprintf("%s:%d:%d:%p:%t", key, l.tagEdits, len(l.all), l.all, globalTags.muted[key]))
	}
	sort.Strings(sig)
	return open, strings.Join(sig, "|")
}

// refreshGlobalTags reads the lending lists again and re-narrows every list
// on screen, whose filters may now match differently.
func (m Model) refreshGlobalTags() {
	open, sig := m.openLenders()
	globalTags.sig = sig
	globalTags.rebuild(open)
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			if l, ok := v.(*cardList); ok {
				l.refresh()
			}
		}
	}
}

// syncGlobalTags rebuilds the global tags when the lists on screen have
// changed since the last time — one check after every message, rather than
// one in every handler that opens, closes, loads or edits a list.
func (m Model) syncGlobalTags() {
	if _, sig := m.openLenders(); sig != globalTags.sig {
		m.refreshGlobalTags()
	}
}

// toggleLending is space t: the list in front of you stops lending its tags
// to the global tags, or starts again.
func (m *Model) toggleLending() {
	var l *cardList
	if p := m.ws.current(); p != nil {
		l = p.cardsView()
	}
	key := l.lenderKey()
	if key == "" {
		m.notice = "only a list lends tags — this panel has none"
		return
	}
	if globalTags.muted == nil {
		globalTags.muted = map[string]bool{}
	}
	if globalTags.muted[key] {
		delete(globalTags.muted, key)
		m.notice = l.name + " lends its tags to the other lists again"
	} else {
		globalTags.muted[key] = true
		m.notice = l.name + " no longer lends its tags to the other lists"
		if globalTags.active(key) {
			m.notice += " (still pinned — t in the decks panel unpins)"
		}
	}
	m.refreshGlobalTags()
}

// togglePin is t in the decks panel.
func (m *Model) togglePin(slug, name string) {
	if globalTags.toggle(slug) {
		m.notice = name + " is pinned to the global tags — its tags count in every list"
	} else {
		m.notice = name + " is unpinned from the global tags"
	}
	savePinned(globalTags.slugs)
	m.refreshGlobalTags()
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			if l, ok := v.(*deckList); ok {
				l.refresh()
			}
		}
	}
}
