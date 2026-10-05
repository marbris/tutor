package ui

import (
	"strings"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/deck"
)

// Writing a list to disk.
//
// w means write *this list*, and what that produces depends on what the list
// is — the same way vim's :w saves the file and :w <name> makes a new one:
//
//	a local deck             committed — the file already has every edit
//	                         (see autosave), so w records it in the history
//	a search or a remote     a name is asked for, and it becomes yours
//
// W does the same but leaves the list where it was and opens the new deck in
// a panel of its own. That is the general rule: a capital puts the result in
// a new panel.

type deckSavedMsg struct {
	panel   int
	slug    string
	subject string
	warning string
	err     error
}

// write is w and W.
func (m *Model) write(l *cardList, p *panel, newPane bool) tea.Cmd {
	if l.deck != nil && l.deck.Local() {
		if !l.dirty {
			m.notice = "nothing to commit in " + l.deck.Name
			return nil
		}
		return saveDeck(p.id, *l.deck, l.all)
	}

	// Not yours yet, so it needs a name before it can be.
	name := l.name
	if l.deck != nil && l.deck.Name != "" {
		// Someone else's title; a slash in it would file the copy in folders.
		name = deck.ImportName(l.deck.Name)
	}
	p.ask(askWrite, "save as", name)
	p.writeToNewPane = newPane
	return nil
}

// saveDeck writes the deck and commits the change, off the main thread.
// It queues behind any autosave still in flight, so the commit can't be
// overtaken by an older write.
func saveDeck(panelID int, info deck.Info, cards []deck.Card) tea.Cmd {
	seq := writeSeq.Add(1)
	cards = append([]deck.Card(nil), cards...)
	return func() tea.Msg {
		writeMu.Lock()
		defer writeMu.Unlock()
		file := deck.Updated(info, cards)
		subject, warning, err := deck.CommitDeck(info.Slug, file)
		if err == nil {
			written[info.Slug] = seq
		}
		return deckSavedMsg{panel: panelID, slug: info.Slug, subject: subject, warning: warning, err: err}
	}
}

// ── Autosave ────────────────────────────────────────────────────

// Every edit is written to the file as soon as it's made, so an edit is
// never lost; committing is what w is for. The writes happen off the main
// thread, so they can finish out of order: each carries a number, and one
// older than the last write of that deck is dropped rather than put back
// over it.
var (
	writeMu  sync.Mutex
	writeSeq atomic.Uint64
	written  = map[string]uint64{}
)

type deckAutosavedMsg struct {
	slug string
	err  error
}

// autosave writes every deck with an edit the file hasn't had.
func (m Model) autosave() tea.Cmd {
	var cmds []tea.Cmd
	lenderEdited := false
	for _, p := range m.ws.panels {
		l := p.cardsView()
		if l == nil || !l.unwritten {
			continue
		}
		if l.deck != nil && globalTags.active(l.deck.Slug) {
			lenderEdited = true
		}
		l.unwritten = false
		if l.deck == nil || !l.deck.Local() {
			continue
		}
		cmds = append(cmds, autosaveDeck(*l.deck, l.all, l.wasClean))
		l.wasClean = false
	}
	// An edit to a list lending global tags changes what every other list counts.
	if lenderEdited {
		m.refreshGlobalTags()
	}
	return tea.Batch(cmds...)
}

func autosaveDeck(info deck.Info, cards []deck.Card, wasClean bool) tea.Cmd {
	seq := writeSeq.Add(1)
	cards = append([]deck.Card(nil), cards...)
	return func() tea.Msg {
		writeMu.Lock()
		defer writeMu.Unlock()
		if written[info.Slug] > seq {
			return nil
		}
		// The first edit since the last commit is the last chance to
		// record whatever was done to the file in another editor.
		if wasClean {
			deck.RecordOutsideEdits(info.Slug)
		}
		err := deck.Write(info.Slug, deck.Updated(info, cards))
		if err == nil {
			written[info.Slug] = seq
		}
		return deckAutosavedMsg{slug: info.Slug, err: err}
	}
}

// saveAsNew turns a search result or somebody else's deck into one of yours.
func saveAsNew(panelID int, newPane bool, name string, cards []deck.Card) tea.Cmd {
	return func() tea.Msg {
		slug, file, err := deck.New(uniqueName(name), deck.DefaultFormat)
		if err != nil {
			return deckSavedMsg{panel: panelID, err: err}
		}
		for _, c := range cards {
			section := "mainboard"
			if c.Commander {
				section = "commander"
			}
			file.Entries = append(file.Entries, deck.Entry{
				Qty: maxInt(c.Qty, 1), Name: c.Card.Name, Tags: c.Tags, Section: section,
			})
		}
		subject, warning, err := deck.SaveVersioned(slug, file)
		if err != nil {
			return deckSavedMsg{panel: panelID, err: err}
		}
		// Opened straight away, because the point of saving a search is to
		// start working on it.
		return deckWrittenMsg{
			panel: panelID, newPane: newPane, slug: slug,
			subject: subject, warning: warning,
		}
	}
}

// deckWrittenMsg is a new deck that now needs opening.
type deckWrittenMsg struct {
	panel   int
	newPane bool
	slug    string
	subject string
	warning string
}

func (m Model) handleDeckSaved(msg deckSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = "error: " + msg.err.Error()
		return m, nil
	}

	// By the deck rather than the panel: the list committed may not be the
	// one on top of it any more.
	for _, d := range m.openDecks() {
		if d.list.deck.Slug == msg.slug {
			d.list.dirty = false
		}
	}

	switch {
	case msg.warning != "":
		m.notice = "committed — " + msg.warning
	case msg.subject == "":
		m.notice = "nothing to commit"
	default:
		m.notice = "committed: " + msg.subject
	}
	return m, reloadDecks
}

func (m Model) handleDeckWritten(msg deckWrittenMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}

	m.notice = "saved as " + msg.slug
	if msg.warning != "" {
		m.notice += " — " + msg.warning
	}

	target := p
	if msg.newPane {
		target = m.ws.open(p.kind)
		m.ws.focus(m.ws.indexOf(p))
	}
	target.loading = true
	target.title = msg.slug
	return m, tea.Batch(openLocalDeck(target.id, msg.newPane, msg.slug), reloadDecks)
}

// dirtyDecks is every deck with edits not yet committed, which is what
// quitting asks about.
func (m Model) dirtyDecks() []string {
	var out []string
	for _, d := range m.openDecks() {
		if d.list.dirty {
			out = append(out, d.list.deck.Name)
		}
	}
	return out
}

// followMove points decks open in panels at where they live now, and gives
// them their new name, so the next write lands in the right file under the
// right name. A folder move carries every deck under it.
func (m Model) followMove(moved, renamed [2]string) {
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			l, ok := v.(*cardList)
			if !ok || l.deck == nil || !l.deck.Local() {
				continue
			}
			if from, to := moved[0], moved[1]; from != "" {
				switch {
				case l.deck.Slug == from:
					l.deck.Slug = to
				case strings.HasPrefix(l.deck.Slug, from+"/"):
					l.deck.Slug = to + strings.TrimPrefix(l.deck.Slug, from)
				}
			}
			if renamed[0] != "" && l.deck.Slug == renamed[0] {
				l.deck.Name = renamed[1]
				l.name = renamed[1]
				if p.top() == v {
					p.title = renamed[1]
				}
			}
		}
	}
}
