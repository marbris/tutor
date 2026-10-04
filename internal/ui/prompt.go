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
	// askOtag is tab from askAddCard: oracle tags whose cards in the deck
	// in front of you get tagged otag-<tag>.
	askOtag
	// askRemote is enter on the git remote in the settings panel.
	askRemote
	// askTagMove is T t and T a: which of this list's tags to bring into
	// the editing deck, every one when left empty.
	askTagMove
)

// Labels of the two sides of the i bar, which tab swaps between.
const (
	addCardLabel = "add from scryfall"
	otagLabel    = "tag by otag"
)

// askAdd raises the i bar: a card to add, or with tab, oracle tags.
func (p *panel) askAdd(initial string) {
	p.ask(askAddCard, addCardLabel, initial)
	p.askInput.Placeholder = "a card name, tab completes it · tab here: tag this list by otag"
}

// swapAddOtag turns the i bar from adding a card to tagging by otag and
// back, keeping what was typed.
func (p *panel) swapAddOtag() {
	if p.asking == askAddCard {
		p.asking = askOtag
		p.askInput.Prompt = otagLabel + ": "
		p.askInput.Placeholder = "removal ramp …, tab completes · tab here: add a card"
		return
	}
	p.asking = askAddCard
	p.askInput.Prompt = addCardLabel + ": "
	p.askInput.Placeholder = "a card name, tab completes it · tab here: tag this list by otag"
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
	if (p.asking == askTag || p.asking == askTagMove) && (key == "tab" || key == "shift+tab") {
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
		if strings.TrimSpace(answer) == "" && kind != askTagMove {
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

		case askAddCard:
			return m, runAddCard(p.id, answer)

		case askRemote:
			return m, connectRemote(p.id, answer)

		case askTagMove:
			if l := p.cardsView(); l != nil {
				m.tagsInto(l, p.tagMoveAdd, strings.Fields(strings.ToLower(answer)))
			}

		case askOtag:
			l := p.cardsView()
			tags := otagNames(answer)
			if l == nil || len(tags) == 0 || len(l.all) == 0 {
				return m, nil
			}
			if msg, unknown, ok := localOtag(tags, l.all); ok {
				msg.panel = p.id
				next, cmd := m.handleOtag(msg)
				if len(unknown) > 0 {
					nm := next.(Model)
					no := "Scryfall Tagger has no " + strings.Join(unknown, ", ")
					if len(msg.tags) == 0 {
						nm.notice = no
					} else {
						nm.notice += " · " + no
					}
					next = nm
				}
				return next, cmd
			}
			m.notice = "asking scryfall about " + strings.Join(tags, ", ") + "…"
			return m, runOtag(p.id, tags, uniqueNames(l.all))

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
