package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/theme"
)

// A one-line question, asked in the panel's header where the search bar
// normally sits.
//
// Naming a new deck and renaming one are the same interaction with different
// consequences, so they are the same prompt with different labels. It lives
// in the header rather than a dialogue because a panel twenty columns wide
// has no room for a dialogue, and because the answer belongs to this panel
// rather than to the screen.

type askKind int

const (
	askNone askKind = iota
	askNewDeck
	askRename
	askFollow
	askTag
	askWrite
	// askAddCard is i on a deck: a Scryfall query whose one answer goes
	// into the deck in front of you.
	askAddCard
	// askOtag is T o and T O: oracle tags whose cards in the editing deck
	// in front of you get tagged otag-<tag>.
	askOtag
	// askRemote is enter on the git remote in the settings panel.
	askRemote
	// askTagMove is T t and T a: which of this list's tags to bring into
	// the editing deck, every one when left empty.
	askTagMove
	// askAddTag is A: the tags to add the cards with.
	askAddTag
	// askGlobalTags is T g: which of the global tags to make the editing
	// deck's own, every one when left empty.
	askGlobalTags
)

// addCardLabel is the i bar's label.
const addCardLabel = "add from scryfall"

// askAdd raises the i bar: a card to add.
func (p *panel) askAdd(initial string) {
	p.ask(askAddCard, addCardLabel, initial)
	p.askInput.Placeholder = "a card name, tab completes it"
}

// ask raises the prompt.
func (p *panel) ask(kind askKind, label, initial string) {
	p.asking = kind
	p.tagComp = nil
	p.askInput = textinput.New()
	p.askInput.Prompt = label + ": "
	p.askInput.PromptStyle = lipgloss.NewStyle().Foreground(theme.Accent)
	p.askInput.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	p.askInput.SetValue(initial)
	p.askInput.CursorEnd()
	p.askInput.Focus()
}

func (p *panel) stopAsking() {
	p.asking = askNone
	p.askInput.Blur()
}

// handleAskKey runs the prompt. Enter acts, esc abandons, tab completes a
// tag, and everything else is typing — there is nothing else a one-line
// question needs.
func (m Model) handleAskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.ws.current()

	key := msg.String()
	if (p.asking == askTag || p.asking == askTagMove || p.asking == askAddTag || p.asking == askGlobalTags) && (key == "tab" || key == "shift+tab") {
		delta := 1
		if key == "shift+tab" {
			delta = -1
		}
		m.completeTag(p, delta)
		return m, nil
	}
	if (p.asking == askAddCard || p.asking == askOtag) && (key == "tab" || key == "shift+tab") {
		delta := 1
		if key == "shift+tab" {
			delta = -1
		}
		m.tabInAddBar(p, delta)
		return m, nil
	}
	// Anything but tab ends a walk through the tags that fit.
	p.tagComp = nil

	switch key {
	case "esc":
		p.stopAsking()
		return m, nil

	case "enter":
		kind, answer := p.asking, p.askInput.Value()
		p.stopAsking()
		// Empty is no answer, except to which tags T moves: then it's all.
		// For A it's just an add.
		if strings.TrimSpace(answer) == "" && kind != askTagMove && kind != askAddTag && kind != askGlobalTags {
			return m, nil
		}

		switch kind {
		case askNewDeck:
			dir := ""
			if l, ok := p.top().(*deckList); ok {
				dir = l.currentFolder()
			}
			// A trailing slash means a folder, not a deck: "dirname/" makes the
			// directory so decks can be filed into it afterwards.
			if strings.HasSuffix(answer, "/") {
				return m, newFolderCmd(dir, strings.TrimSuffix(answer, "/"))
			}
			return m, newDeckCmd(dir, answer)
		case askFollow:
			return m, follow(answer)
		case askRename:
			if l, ok := p.top().(*deckList); ok {
				if e, ok := l.current(); ok {
					if e.kind == entryFolder {
						return m, renameFolderCmd(e, answer)
					}
					return m, renameCmd(e, answer)
				}
			}

		case askTag:
			if l := p.cardsView(); l != nil {
				m.tag(l.selection(), answer)
			}

		case askAddTag:
			if l := p.cardsView(); l != nil {
				m.addTagged(l.selection(), answer)
			}

		case askAddCard:
			return m, runAddCard(p.id, answer)

		case askRemote:
			return m, connectRemote(p.id, answer)

		case askTagMove:
			if l := p.cardsView(); l != nil {
				m.tagsInto(l, p.tagMoveAdd, strings.Fields(strings.ToLower(answer)))
			}

		case askOtag:
			return m, m.otagInto(answer, p.otagAdd)

		case askGlobalTags:
			m.bakeGlobalTags(strings.Fields(strings.ToLower(answer)))

		case askWrite:
			if l := p.cardsView(); l != nil {
				return m, saveAsNew(p.id, p.writeToNewPane, answer, l.all)
			}
		}
		return m, nil

	case "ctrl+c":
		return m, m.quit()
	}

	var cmd tea.Cmd
	p.askInput, cmd = p.askInput.Update(msg)
	return m, cmd
}
