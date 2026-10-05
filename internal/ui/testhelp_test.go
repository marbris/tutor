package ui

import (
	"strings"

	"ttr/internal/mtg"

	tea "github.com/charmbracelet/bubbletea"
)

// splitLines and visibleWidth measure a rendered frame the way a terminal
// would: by what it shows, not by the escape sequences that colour it.

func splitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func visibleWidth(s string) int {
	return textWidth(stripANSI(s))
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// msgOf runs a command and hands back what it produced, for tests that want
// the answer without a running program.
func msgOf(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func sizeOf(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

// focusOn moves focus to a panel by index, without going through the keys.
func focusOn(m Model, at int) Model {
	m.ws.focus(at)
	return m
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+j":
		return tea.KeyMsg{Type: tea.KeyCtrlJ}
	case "ctrl+k":
		return tea.KeyMsg{Type: tea.KeyCtrlK}
	}
	if r, ok := strings.CutPrefix(k, "alt+"); ok {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r), Alt: true}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// press sends one key and hands back the command it produced, for the paths
// that do their work off the main thread.
func press(m Model, k string) (Model, tea.Cmd) {
	next, cmd := m.Update(keyMsg(k))
	return next.(Model), cmd
}

// settle runs a command and feeds its message back, the way the program
// would — unwrapping batches, which hand back a list of commands rather than
// running them.
func settle(m Model, cmd tea.Cmd) Model {
	for _, msg := range messages(cmd) {
		next, more := m.Update(msg)
		m = next.(Model)
		if more != nil {
			m = settle(m, more)
		}
	}
	return m
}

// messages runs a command and flattens whatever it produced.
func messages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, messages(c)...)
	}
	return out
}

func mkReader(s string) *strings.Reader { return strings.NewReader(s) }

func mtgCard(cost string) mtg.Card { return mtg.Card{ManaCost: cost} }
