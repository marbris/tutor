package ui

import (
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/deck"
)

// Decks changed outside this ttr.
//
// Two ttrs (or ttr and an editor) can have the same deck open. Each keeps
// the version of the file it last read or wrote, and looks at the file's
// stamp every second: a stamp it didn't write is somebody else's change,
// which it reads and merges into what it shows, card by card. Writing does
// the same the other way round: if the file changed since the version an
// edit was made on, the edit is merged onto the file rather than written over
// it. Undo snapshots are rebased the same way, so u only takes back this
// ttr's own edits.

// diskCopy is one version of a deck's file: its stamp, the cards it held,
// and when this ttr learned it (later versions have a higher ver).
type diskCopy struct {
	stamp string
	cards []deck.Card
	ver   uint64
}

// deckFiles is what this ttr knows of its decks' files. seen is the version
// each open deck's cards were last brought up to, and is only touched on the
// main loop. written and wrote belong to the writes, under writeMu: the
// sequence of the last write of each deck, and the stamp it left.
type deckFiles struct {
	seen    map[string]diskCopy
	written map[string]uint64
	wrote   map[string]string
}

func newDeckFiles() *deckFiles {
	return &deckFiles{seen: map[string]diskCopy{}, written: map[string]uint64{}, wrote: map[string]string{}}
}

var diskVer atomic.Uint64

// fileWrite is what a write left on disk, for the main loop to take in.
// theirs is set when another ttr had changed the file since base, the
// version the edit was made on: the open lists are rebased onto it.
type fileWrite struct {
	slug         string
	stamp        string
	cards        []deck.Card
	ver          uint64
	base, theirs []deck.Card
}

// mergeOutside is the cards a write should put in the file: ours, merged
// onto the file as it is now if it isn't base any more. Called under writeMu.
// With no base to merge from, or a file that won't read, ours are written as
// they are, as they always were.
func mergeOutside(slug string, base diskCopy, ours []deck.Card) ([]deck.Card, fileWrite) {
	w := fileWrite{slug: slug}
	if base.stamp == "" || fileStamp(deck.Path(slug)) == base.stamp {
		return ours, w
	}
	_, theirs, err := deck.Open(slug)
	if err != nil {
		return ours, w
	}
	w.base, w.theirs = base.cards, theirs
	return deck.MergeCards(base.cards, theirs, ours), w
}

// wrote finishes a fileWrite once the file is written. Called under writeMu.
func (f *deckFiles) wroteFile(w fileWrite, cards []deck.Card) fileWrite {
	w.stamp = fileStamp(deck.Path(w.slug))
	w.cards = cards
	w.ver = diskVer.Add(1)
	f.wrote[w.slug] = w.stamp
	return w
}

// tookWrite brings the open lists up to what a write left on disk.
func (m Model) tookWrite(w fileWrite) {
	if w.slug == "" {
		return
	}
	if w.theirs != nil {
		m.rebase(w.slug, w.base, w.theirs)
	}
	m.files.saw(w.slug, diskCopy{stamp: w.stamp, cards: w.cards, ver: w.ver})
}

// saw records a version of a deck's file, unless a later one is known.
func (f *deckFiles) saw(slug string, c diskCopy) {
	if s, ok := f.seen[slug]; ok && s.ver > c.ver {
		return
	}
	f.seen[slug] = c
}

// opened records the file a deck was just opened from. A deck already open
// elsewhere keeps the version its other lists are on, and the next look at
// the file brings them all up to date.
func (m Model) opened(slug string, c diskCopy, l *cardList) {
	for _, d := range m.openDecks() {
		if d.list != l && d.list.deck.Slug == slug {
			return
		}
	}
	m.files.seen[slug] = c
}

// rebase merges another ttr's change into every open list of a deck, and
// into their undo snapshots.
func (m Model) rebase(slug string, base, theirs []deck.Card) {
	for _, d := range m.openDecks() {
		l := d.list
		if l.deck.Slug != slug {
			continue
		}
		l.all = deck.MergeCards(base, theirs, l.all)
		for i := range l.undo {
			l.undo[i].cards = deck.MergeCards(base, theirs, l.undo[i].cards)
		}
		l.recheck()
		l.refresh()
	}
}

// ── Looking at the files ────────────────────────────────────────

const filesEvery = time.Second

type filesTickMsg struct{}

func filesTick() tea.Cmd {
	return tea.Tick(filesEvery, func(time.Time) tea.Msg { return filesTickMsg{} })
}

// outsideChange is a deck file another ttr or an editor has written.
type outsideChange struct {
	slug  string
	from  string // the stamp of the version it replaces
	copy  diskCopy
	dirty bool
}

type outsideChangesMsg struct{ changes []outsideChange }

func (m Model) handleFilesTick() (tea.Model, tea.Cmd) {
	return m, tea.Batch(m.checkFiles(), filesTick())
}

// checkFiles stats every open deck of yours, and reads the ones whose file
// someone else has written.
func (m Model) checkFiles() tea.Cmd {
	type look struct {
		slug string
		seen diskCopy
	}
	var looks []look
	asked := map[string]bool{}
	for _, d := range m.openDecks() {
		slug := d.list.deck.Slug
		s, ok := m.files.seen[slug]
		if !d.list.deck.Local() || !ok || asked[slug] {
			continue
		}
		asked[slug] = true
		looks = append(looks, look{slug, s})
	}
	if len(looks) == 0 {
		return nil
	}
	f := m.files
	return func() tea.Msg {
		var changed []outsideChange
		writeMu.Lock()
		for _, lk := range looks {
			stamp := fileStamp(deck.Path(lk.slug))
			if stamp == "" || stamp == lk.seen.stamp || stamp == f.wrote[lk.slug] {
				continue
			}
			changed = append(changed, outsideChange{slug: lk.slug, from: lk.seen.stamp,
				copy: diskCopy{stamp: stamp, ver: diskVer.Add(1)}})
		}
		writeMu.Unlock()

		var out []outsideChange
		for _, c := range changed {
			_, cards, err := deck.Open(c.slug)
			if err != nil {
				continue // half written by an editor, or gone: next look
			}
			c.copy.cards = cards
			c.dirty = deck.HasUncommittedEdits(c.slug)
			out = append(out, c)
		}
		if len(out) == 0 {
			return nil
		}
		return outsideChangesMsg{out}
	}
}

func (m Model) handleOutsideChanges(msg outsideChangesMsg) (tea.Model, tea.Cmd) {
	for _, c := range msg.changes {
		s, ok := m.files.seen[c.slug]
		if !ok || s.stamp != c.from || s.ver > c.copy.ver {
			continue // a write of ours got there first, and brought it in
		}
		m.rebase(c.slug, s.cards, c.copy.cards)
		m.files.seen[c.slug] = c.copy
		name := c.slug
		for _, d := range m.openDecks() {
			if d.list.deck.Slug == c.slug {
				d.list.dirty = d.list.dirty || c.dirty
				name = d.list.deck.Name
			}
		}
		m.notice = name + " changed in another ttr or editor"
	}
	return m, nil
}
