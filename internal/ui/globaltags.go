package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"ttr/internal/deck"
	"ttr/internal/paths"
)

// Tag lists: tags that hold across every list on screen.
//
// A tag list is an ordinary .list file — often format: tags, a card a line,
// each with its tags — that you turn on with t in the decks panel. While it
// is on, its tags count as every list's own: a deck with nothing tagged ramp
// still shows ramp in its statistics, and / and the statistics filter find
// it, when a tag list says the card is ramp. They are never written into the
// deck; the deck's own tags stay its own, and T g is how you copy them in.

const tagListsFile = "taglists.json"

// tagIndex is the tag lists that are on, and what they say.
type tagIndex struct {
	slugs []string
	// tags is each card's tags across the lists, by lowercased name — and
	// by its front face too, for a list that names a two-faced card by it.
	tags map[string][]string
	// gen counts rebuilds, for the statistics to know the tags have moved.
	gen int
}

// globalTags is the one index, shared by every list, because the lists
// narrow themselves without being told what the workspace holds.
var globalTags = &tagIndex{}

func tagListsPath() string { return filepath.Join(paths.State(), tagListsFile) }

// loadTagLists reads which tag lists were on, dropping any since deleted.
func loadTagLists() []string {
	body, err := os.ReadFile(tagListsPath())
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

func saveTagLists(slugs []string) {
	body, err := json.Marshal(slugs)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(tagListsPath()), 0755)
	os.WriteFile(tagListsPath(), body, 0644)
}

func (x *tagIndex) active(slug string) bool {
	for _, s := range x.slugs {
		if s == slug {
			return true
		}
	}
	return false
}

// toggle turns a tag list on or off, and says which.
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

// of is the tags the lists give a card.
func (x *tagIndex) of(name string) []string {
	if len(x.tags) == 0 {
		return nil
	}
	k := strings.ToLower(name)
	if t, ok := x.tags[k]; ok {
		return t
	}
	front, _, ok := strings.Cut(k, " // ")
	if ok {
		return x.tags[front]
	}
	return nil
}

// rebuild reads the lists again. A list open on screen is read from there,
// since its latest edit may not have reached the file yet.
func (x *tagIndex) rebuild(open map[string][]deck.Card) {
	x.gen++
	x.tags = map[string][]string{}
	add := func(name string, tags []string) {
		if len(tags) == 0 {
			return
		}
		k := strings.ToLower(name)
		x.tags[k] = deck.ApplyTagEdits(x.tags[k], tags, nil)
		if front, _, ok := strings.Cut(k, " // "); ok {
			x.tags[front] = deck.ApplyTagEdits(x.tags[front], tags, nil)
		}
	}
	for _, slug := range x.slugs {
		if cards, ok := open[slug]; ok {
			for _, c := range cards {
				add(c.Card.Name, c.Tags)
			}
			continue
		}
		f, err := deck.Read(slug)
		if err != nil {
			continue
		}
		for _, e := range f.Entries {
			add(e.Name, e.Tags)
		}
	}
}

// effective is a card with the tag lists' tags added to its own, for
// narrowing and counting. The card in the list is left as it is.
func effective(c deck.Card) deck.Card {
	extra := globalTags.of(c.Card.Name)
	if len(extra) == 0 {
		return c
	}
	c.Tags = deck.ApplyTagEdits(c.Tags, extra, nil)
	return c
}

func effectiveAll(cards []deck.Card) []deck.Card {
	if len(globalTags.tags) == 0 {
		return cards
	}
	out := make([]deck.Card, len(cards))
	for i, c := range cards {
		out[i] = effective(c)
	}
	return out
}

// onlyGlobal is the tags a card has from the tag lists and not of its own.
func onlyGlobal(c deck.Card) []string {
	var out []string
	for _, t := range globalTags.of(c.Card.Name) {
		mine := false
		for _, o := range c.Tags {
			if o == t {
				mine = true
				break
			}
		}
		if !mine {
			out = append(out, t)
		}
	}
	return out
}

// refreshGlobalTags reads the tag lists again and re-narrows every list on
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

// toggleTagList is t in the decks panel.
func (m *Model) toggleTagList(slug, name string) {
	if globalTags.toggle(slug) {
		m.notice = name + " is a tag list now — its tags count in every list"
	} else {
		m.notice = name + " is no longer a tag list"
	}
	saveTagLists(globalTags.slugs)
	m.refreshGlobalTags()
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			if l, ok := v.(*deckList); ok {
				l.refresh()
			}
		}
	}
}
