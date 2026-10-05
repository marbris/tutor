package ui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"ttr/internal/deck"
	"ttr/internal/paths"
)

// Global tags: tags that hold across every list on screen.
//
// A list pinned to the global tags is an ordinary .list file — often format:
// tags, a card a line, each with its tags — pinned with t in the decks panel.
// While it is pinned, its tags count as every list's own: a deck with nothing
// tagged ramp still shows ramp in its statistics, and / and the statistics
// filter find it, when a pinned list says the card is ramp. They are never
// written into the deck; the deck's own tags stay its own, and T g is how you
// copy them in.
//
// Before 8.0.0 these were "tag lists", kept in taglists.json; that file is
// still read when globaltags.json isn't there yet.

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
	// gen counts rebuilds, for the statistics to know the tags have moved.
	gen int
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

// rebuild reads the lists again. A list open on screen is read from there,
// since its latest edit may not have reached the file yet.
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
	for _, slug := range x.slugs {
		if cards, ok := open[slug]; ok {
			for _, c := range cards {
				add(slug, c.Card.Name, c.Tags)
			}
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

// lenderKey is what a list lends the global tags under: a local list's slug.
// A list with none — a search, a deck borrowed from Moxfield — lends nothing
// yet, and sees every list's tags as borrowed.
func (l *cardList) lenderKey() string {
	if l == nil || l.deck == nil {
		return ""
	}
	return l.deck.Slug
}

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

// refreshGlobalTags reads the lending lists again and re-narrows every list on
// screen, whose filters may now match differently.
func (m Model) refreshGlobalTags() {
	open := map[string][]deck.Card{}
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			if l, ok := v.(*cardList); ok && l.deck != nil && l.deck.Local() && globalTags.active(l.deck.Slug) {
				open[l.deck.Slug] = l.all
			}
		}
	}
	globalTags.rebuild(open)
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			if l, ok := v.(*cardList); ok {
				l.refresh()
			}
		}
	}
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
