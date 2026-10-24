package ui

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/paths"
)

// Sharing the editing deck between ttrs.
//
// With link windows on (a switch in the settings panel), every ttr edits the
// same deck: e or E in one, or opening a deck that becomes the editing one,
// writes its slug to editing.json in the state directory, and the others
// take it up on their next look at the files (outside.go), opening it in a
// panel of its own if it isn't open there. Each ttr keeps its own cursor,
// lists, info panel and statistics; only the deck that a, x and t act on is
// shared. The switch is a file of its own, so it is on or off for every ttr
// at once.

const (
	shareFile   = "share-editing"
	editingFile = "editing.json"
)

func shareSwitchPath() string { return filepath.Join(paths.State(), shareFile) }
func sharedPath() string      { return filepath.Join(paths.State(), editingFile) }

// sharingEditing reports whether the switch is on.
func sharingEditing() bool {
	_, err := os.Stat(shareSwitchPath())
	return err == nil
}

// setSharingEditing turns the switch on or off.
func setSharingEditing(on bool) error {
	if !on {
		if err := os.Remove(shareSwitchPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(paths.State(), 0755); err != nil {
		return err
	}
	return os.WriteFile(shareSwitchPath(), nil, 0644)
}

// sharedEditing is what editing.json holds: the deck, which ttr chose it,
// and when.
type sharedEditing struct {
	Slug string `json:"slug"`
	By   uint64 `json:"by"`
	At   int64  `json:"at"`
}

// newInstanceID tells this ttr's choices from another's. Not the pid: tests
// run two Models in one process.
func newInstanceID() uint64 { return rand.Uint64() | 1 }

func readShared() (sharedEditing, bool) {
	var s sharedEditing
	body, err := os.ReadFile(sharedPath())
	if err != nil || json.Unmarshal(body, &s) != nil || s.Slug == "" {
		return s, false
	}
	return s, true
}

func writeShared(s sharedEditing) error {
	body, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.State(), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(paths.State(), editingFile+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), sharedPath())
}

// editingSlug is the slug of the deck being edited, or "".
func (m Model) editingSlug() string {
	if l := m.ws.editingList(); l != nil && l.deck != nil && l.deck.Local() {
		return l.deck.Slug
	}
	return ""
}

// shareChoice tells the other ttrs the editing deck, when the switch is on
// and you just changed it.
func (m Model) shareChoice(before string) tea.Cmd {
	slug := m.editingSlug()
	if slug == "" || slug == before || slug == m.files.shared || !sharingEditing() {
		return nil
	}
	s := sharedEditing{Slug: slug, By: m.files.id, At: time.Now().UnixNano()}
	m.files.shared, m.files.sharedAt = slug, s.At
	return func() tea.Msg {
		if err := writeShared(s); err != nil {
			return noticeMsg{err: err}
		}
		return nil
	}
}

// sharedChoiceMsg is another ttr's choice of editing deck.
type sharedChoiceMsg struct{ s sharedEditing }

// checkShared reads editing.json, when the switch is on.
func (m Model) checkShared() tea.Cmd {
	id, after := m.files.id, m.files.sharedAt
	return func() tea.Msg {
		if !sharingEditing() {
			return nil
		}
		s, ok := readShared()
		if !ok || s.By == id || s.At <= after {
			return nil
		}
		return sharedChoiceMsg{s}
	}
}

// handleSharedChoice makes another ttr's editing deck this one's: the panel
// that has it, or a new one at the end, opened without taking the focus.
func (m Model) handleSharedChoice(msg sharedChoiceMsg) (tea.Model, tea.Cmd) {
	s := msg.s
	if s.At <= m.files.sharedAt {
		return m, nil
	}
	m.files.shared, m.files.sharedAt = s.Slug, s.At
	if m.editingSlug() == s.Slug {
		return m, nil
	}
	for i, p := range m.ws.panels {
		if !m.ws.editable(i) {
			continue
		}
		if l := p.cardsView(); l.deck.Slug == s.Slug {
			m.ws.editing = i
			m.notice = "editing " + l.deck.Name + ", as in another ttr"
			return m, nil
		}
	}
	focused := m.ws.focused
	p := m.ws.insert(KindDecks, len(m.ws.panels))
	m.ws.focus(focused)
	p.searchOpen = false
	p.search.Blur()
	p.loading = true
	p.title = s.Slug
	m.ws.editing = m.ws.indexOf(p)
	m.notice = "editing " + s.Slug + ", as in another ttr"
	return m, openLocalDeck(p.id, true, s.Slug)
}

// inactive reports whether this ttr should look as if it hasn't the keys:
// its terminal window is in the background, and the ttrs share an editing
// deck, so the one you're typing in has to stand out. Unshared, each ttr is
// on its own, and keeps showing its focused panel and cursor.
func (m Model) inactive() bool { return m.blurred && sharingEditing() }
