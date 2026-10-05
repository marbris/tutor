package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/config"
	"ttr/internal/deck"
	"ttr/internal/fetch"
	"ttr/internal/keymap"
	"ttr/internal/mtg"
	"ttr/internal/scryfall"
	"ttr/internal/tagger"
)

// T o and T O: tag the editing deck from Scryfall's oracle tags.
//
// T o: type "removal" and every card in the deck that Scryfall tags
// otag:removal gets the tag otag-removal. T O does that and adds every
// other card Scryfall tags so, which is how a list like otag-ball-lightning
// is made in one go.
//
// Which cards carry a tag is worked out here, from Scryfall Tagger's tags
// kept on disk and the oracle id every card carries — at once, with nothing
// asked. A tag counts the cards under it, as otag: does on Scryfall and as
// the statistics do: removal finds the artifact removal. T O still has to
// fetch the cards it adds that aren't cached, by oracle id. Only when the
// tags aren't in (or, for T o, a card has no oracle id) does it ask Scryfall
// which cards carry the tag instead.
//
// For T o that is one question per tag: otag:removal (!"Sol Ring" or
// !"Rancor" or …), over the deck's own names, tagging whatever comes back.
// Names go in batches, so no one query grows longer than a URL should. A
// batch that matches more than a page's worth is paged through, which only
// happens when the first page came back full.

// otagQueryLen is how long one query is allowed to grow. Scryfall reads
// only the first 1024 characters of a query and drops the rest, which cuts
// off the closing parenthesis: "Your search contains unclosed parentheses."
// Lengths here are bytes, which are never fewer than characters.
const otagQueryLen = 1000

// otagPrefix is put in front of an oracle tag to make it one of yours:
// config.json's otag_prefix, otag- unless it says otherwise.
var otagPrefix = config.DefaultOtagPrefix

// SetOtagPrefix puts config.json's otag prefix in force.
func SetOtagPrefix(prefix string) { otagPrefix = prefix }

// otagMsg carries the answer back: for each tag, the names in the list
// Scryfall gave it.
type otagMsg struct {
	panel   int
	tags    []string
	hits    map[string][]string
	unknown []string // tags Tagger hasn't got
	err     error
}

// otagNames is what was typed, as oracle tags: one per word, lowercased,
// with any otag: or otag- someone typed out of habit taken off.
func otagNames(input string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(input)) {
		w = strings.TrimPrefix(w, "otag:")
		w = strings.TrimPrefix(w, config.DefaultOtagPrefix)
		if otagPrefix != "" {
			w = strings.TrimPrefix(w, otagPrefix)
		}
		if w != "" && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// otagQueries is the queries that ask which of names Scryfall gives tag,
// each short enough to send.
func otagQueries(tag string, names []string) []string {
	var out []string
	head := "otag:" + tag + " ("
	var b strings.Builder
	for _, n := range names {
		term := fmt.Sprintf("!%q", n)
		if b.Len() > 0 && len(head)+b.Len()+len(" or ")+len(term)+1 > otagQueryLen {
			out = append(out, head+b.String()+")")
			b.Reset()
		}
		if b.Len() > 0 {
			b.WriteString(" or ")
		}
		b.WriteString(term)
	}
	if b.Len() > 0 {
		out = append(out, head+b.String()+")")
	}
	return out
}

// uniqueNames is each card in the list once, in the order they're kept.
func uniqueNames(cards []deck.Card) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range cards {
		k := strings.ToLower(c.Card.Name)
		if !seen[k] {
			seen[k] = true
			out = append(out, c.Card.Name)
		}
	}
	return out
}

// localOtag is the answer from the Tagger tags on disk: for each tag, the
// names in the list it covers, and the tags Tagger doesn't have. ok is
// false when it can't answer — no tags loaded, or a card with no oracle
// id to look up — and Scryfall has to be asked.
func localOtag(tags []string, cards []deck.Card) (msg otagMsg, unknown []string, ok bool) {
	tg := tagger.Current()
	if tg == nil {
		return otagMsg{}, nil, false
	}
	for _, c := range cards {
		if c.Card.OracleID == "" {
			return otagMsg{}, nil, false
		}
	}
	msg = otagMsg{hits: map[string][]string{}}
	for _, tag := range tags {
		t, found := tg.Find(tag)
		if !found {
			unknown = append(unknown, tag)
			continue
		}
		msg.tags = append(msg.tags, tag)
		for _, c := range cards {
			if tg.Has(c.Card.OracleID, t) {
				msg.hits[tag] = append(msg.hits[tag], c.Card.Name)
			}
		}
	}
	return msg, unknown, true
}

// runOtag asks Scryfall, off the main thread, pausing between requests.
func runOtag(panelID int, tags, names []string) tea.Cmd {
	return func() tea.Msg {
		msg := otagMsg{panel: panelID, tags: tags, hits: map[string][]string{}}
		first := true
		for _, tag := range tags {
			for _, q := range otagQueries(tag, names) {
				if !first {
					time.Sleep(scryfall.PageDelay)
				}
				first = false
				cards, _, err := scryfall.Search(q, "name", "auto", 1<<20)
				if err != nil {
					msg.err = err
					return msg
				}
				for _, c := range cards {
					msg.hits[tag] = append(msg.hits[tag], c.Name)
				}
			}
		}
		return msg
	}
}

// sameCard reports whether a name Scryfall gave back is this card. A card
// with two faces may be kept by its front face alone.
func sameCard(kept, found string) bool {
	if strings.EqualFold(kept, found) {
		return true
	}
	front, _, ok := strings.Cut(found, " // ")
	return ok && strings.EqualFold(kept, front)
}

// otagInto is T o (addMissing false) and T O (true): the editing deck
// tagged by the oracle tags typed, and with T O the cards it lacks added.
func (m *Model) otagInto(input string, addMissing bool) tea.Cmd {
	tags := otagNames(input)
	target, why := m.editTarget()
	if target == nil {
		m.notice = why
		return nil
	}
	p := m.ws.editingPanel()
	if len(tags) == 0 || p == nil {
		return nil
	}
	if addMissing {
		return m.otagAdd(p.id, target, tags)
	}
	if len(target.all) == 0 {
		m.notice = target.name + " is empty — " + keymap.Hint(keymap.TagMove, keymap.TagMoveOtagAdd) + " after " +
			keymap.Hint(keymap.Cards, keymap.CardsTagMove) + " adds the cards too"
		return nil
	}
	if msg, unknown, ok := localOtag(tags, target.all); ok {
		msg.panel, msg.unknown = p.id, unknown
		next, cmd := m.handleOtag(msg)
		*m = next.(Model)
		return cmd
	}
	m.notice = "asking scryfall about " + strings.Join(tags, ", ") + "…"
	return runOtag(p.id, tags, uniqueNames(target.all))
}

// handleOtag tags the deck that asked, if it is still there and still yours.
func (m Model) handleOtag(msg otagMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	l := p.cardsView()
	if l == nil || l.deck == nil || !l.deck.Local() {
		return m, nil
	}
	if msg.err != nil {
		if _, ok := msg.err.(fetch.NotFound); !ok {
			m.notice = "error: " + msg.err.Error()
			return m, nil
		}
	}

	// Each card the deck has, with the otag tags it is due.
	due := map[string][]string{}
	var said []string
	for _, tag := range msg.tags {
		mine := otagPrefix + tag
		n := 0
		for _, c := range l.all {
			for _, found := range msg.hits[tag] {
				if sameCard(c.Card.Name, found) {
					due[c.Card.Name] = append(due[c.Card.Name], mine)
					n++
					break
				}
			}
		}
		said = append(said, mine+": "+itoa(n)+" of "+itoa(len(l.all)))
	}
	var src []deck.Card
	for _, c := range l.all {
		if tags := due[c.Card.Name]; len(tags) > 0 {
			src = append(src, deck.Card{Card: c.Card, Tags: tags})
		}
	}
	bringInto(l, bringing{what: "tag by otag", cards: src, carry: true})
	if len(msg.unknown) > 0 {
		said = append(said, "Scryfall Tagger has no "+strings.Join(msg.unknown, ", "))
	}
	m.notice = strings.Join(said, " · ")
	return m, m.autosave()
}

// ── T O: every card with the tag ────────────────────────────────

// otagAddMsg carries T O's cards back: for each tag, the oracle ids of
// every card it covers, and the cards the deck hasn't got, by oracle id.
type otagAddMsg struct {
	panel   int
	tags    []string
	ids     map[string][]string
	cards   map[string]mtg.Card
	unknown []string
	// asked is true when Scryfall was asked which cards carry the tags,
	// the Tagger tags not being in.
	asked bool
	err   error
}

// otagAdd works out T O's cards from the Tagger tags on disk, and fetches
// the ones the deck hasn't got and the card cache doesn't hold.
func (m *Model) otagAdd(panelID int, target *cardList, tags []string) tea.Cmd {
	tg := tagger.Current()
	if tg == nil {
		m.notice = "Scryfall Tagger's tags aren't downloaded (space c) — asking scryfall about " + strings.Join(tags, ", ") + "…"
		return runOtagSearch(panelID, tags)
	}
	msg := otagAddMsg{panel: panelID, ids: map[string][]string{}}
	have := map[string]bool{}
	for _, c := range target.all {
		if c.Card.OracleID != "" {
			have[c.Card.OracleID] = true
		}
	}
	need := map[string]bool{}
	for _, tag := range tags {
		t, ok := tg.Find(tag)
		if !ok {
			msg.unknown = append(msg.unknown, tag)
			continue
		}
		msg.tags = append(msg.tags, tag)
		msg.ids[tag] = tg.With(t)
		for _, id := range msg.ids[tag] {
			if !have[id] {
				need[id] = true
			}
		}
	}
	if len(msg.tags) == 0 {
		m.notice = "Scryfall Tagger has no " + strings.Join(msg.unknown, ", ")
		return nil
	}
	if len(need) == 0 {
		next, cmd := m.handleOtagAdd(msg)
		*m = next.(Model)
		return cmd
	}
	m.notice = "fetching " + itoa(len(need)) + " " + plural("card", len(need)) + " for " +
		otagPrefix + strings.Join(msg.tags, ", "+otagPrefix) + "…"
	ids := make([]string, 0, len(need))
	for id := range need {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return func() tea.Msg {
		msg.cards = deck.CachedByOracle(ids)
		var missing []string
		for _, id := range ids {
			if _, ok := msg.cards[id]; !ok {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			got, err := scryfall.ByOracle(missing)
			if err != nil {
				msg.err = err
				return msg
			}
			for id, c := range got {
				msg.cards[id] = c
			}
		}
		return msg
	}
}

// runOtagSearch is T O without the Tagger tags: Scryfall asked which cards
// carry each tag, one search a tag, paged through.
func runOtagSearch(panelID int, tags []string) tea.Cmd {
	return func() tea.Msg {
		msg := otagAddMsg{panel: panelID, tags: tags, asked: true,
			ids: map[string][]string{}, cards: map[string]mtg.Card{}}
		for i, tag := range tags {
			if i > 0 {
				time.Sleep(scryfall.PageDelay)
			}
			cards, _, err := scryfall.Search("otag:"+tag, "name", "auto", 1<<20)
			if err != nil {
				if _, ok := err.(fetch.NotFound); ok {
					msg.unknown = append(msg.unknown, tag)
					continue
				}
				msg.err = err
				return msg
			}
			for _, c := range cards {
				msg.ids[tag] = append(msg.ids[tag], c.OracleID)
				msg.cards[c.OracleID] = c
			}
		}
		return msg
	}
}

// handleOtagAdd puts T O's cards into the deck that asked, tagged, as one
// undo step.
func (m Model) handleOtagAdd(msg otagAddMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	l := p.cardsView()
	if l == nil || l.deck == nil || !l.deck.Local() {
		return m, nil
	}
	if msg.err != nil {
		m.notice = "error: " + msg.err.Error()
		if _, ok := msg.err.(fetch.RateLimited); ok {
			m.notice = "scryfall asked us to slow down — try again in a minute"
		}
		return m, nil
	}

	byOracle := map[string]mtg.Card{}
	for _, c := range l.all {
		if c.Card.OracleID != "" {
			byOracle[c.Card.OracleID] = c.Card
		}
	}
	due := map[string][]string{}
	var order []string
	for _, tag := range msg.tags {
		for _, id := range msg.ids[tag] {
			if _, ok := byOracle[id]; !ok {
				c, ok := msg.cards[id]
				if !ok {
					continue // Scryfall had no card for it
				}
				byOracle[id] = c
			}
			if len(due[id]) == 0 {
				order = append(order, id)
			}
			due[id] = append(due[id], otagPrefix+tag)
		}
	}
	src := make([]deck.Card, 0, len(order))
	for _, id := range order {
		src = append(src, deck.Card{Card: byOracle[id], Tags: due[id]})
	}
	r := bringInto(l, bringing{what: "add by otag", cards: src, carry: true, addMissing: true})

	m.notice = itoa(r.added) + " added, " + itoa(r.tagged) + " tagged in " + l.name
	if len(msg.unknown) > 0 {
		m.notice += " · Scryfall Tagger has no " + strings.Join(msg.unknown, ", ")
	}
	if msg.asked {
		m.notice += " · asked Scryfall: the Tagger tags aren't downloaded"
	}
	return m, m.autosave()
}
