package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/keymap"
	"ttr/internal/rules"
	"ttr/internal/theme"
)

// The diff between two releases of the rules, as a rules panel.
//
// The list is the rules that changed — one row each, by number — and the
// information panel shows what changed in the highlighted one: the old wording
// and the new. Reached with gv from the rules panel; it opens on the rule you
// were reading, so "what changed around 702.9" is a scroll away from reading
// 702.9, and unchanged rules simply aren't in the way.

type rulesDiff struct {
	cursor
	name    string
	changes []rules.RuleChange
}

// rulesDiffMsg carries a built diff back to the main thread, with the rule to
// open on.
type rulesDiffMsg struct {
	panel   int
	changes []rules.RuleChange
	focus   string
	err     error
}

// loadRulesDiff builds the diff off the main thread — it parses two megabyte
// files — and reports it back to the panel that asked.
func loadRulesDiff(panelID int, focus string) tea.Cmd {
	return func() tea.Msg {
		changes, err := rules.DiffPrevious()
		return rulesDiffMsg{panel: panelID, changes: changes, focus: focus, err: err}
	}
}

func (m Model) handleRulesDiff(msg rulesDiffMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	p.loading = false
	if msg.err != nil {
		m.notice = "error: " + msg.err.Error()
		return m, nil
	}

	v := newRulesDiff(msg.changes, msg.focus)
	p.push(v)
	p.title = v.title()
	return m, nil
}

func newRulesDiff(changes []rules.RuleChange, focus string) *rulesDiff {
	v := &rulesDiff{name: "rules diff", changes: changes}
	v.cursor.at = v.focusRow(focus)
	return v
}

// focusRow is the row to open on: the first changed rule at or after the one
// that was highlighted, so gv lands where you were reading. No focus opens at
// the top.
func (v *rulesDiff) focusRow(focus string) int {
	if focus == "" {
		return 0
	}
	for i, c := range v.changes {
		if !lessRuleNumber(c.Number, focus) {
			return i
		}
	}
	return 0
}

// ── As a view ───────────────────────────────────────────────────

func (v *rulesDiff) title() string { return v.name }

func (v *rulesDiff) subtitle() string {
	var added, removed, changed int
	for _, c := range v.changes {
		switch c.Kind {
		case rules.Added:
			added++
		case rules.Removed:
			removed++
		default:
			changed++
		}
	}
	return itoa(changed) + " changed · " + itoa(added) + " added · " + itoa(removed) + " removed"
}

func (v *rulesDiff) lines(width, height int, focused bool, m *Model) []string {
	if len(v.changes) == 0 {
		return fillTo(mutedLines("no rules changed between the two releases", width), width, height)
	}

	v.cursor.scrollInto(height, len(v.changes))
	out := make([]string, 0, height)
	for i := v.cursor.offset; i < len(v.changes) && len(out) < height; i++ {
		out = append(out, renderChangeRow(v.changes[i], width, focused && i == v.cursor.at))
	}
	return fillTo(out, width, height)
}

// renderChangeRow is one changed rule: a marker for what happened to it, its
// number, and the first line of the wording that stands now (or stood, for a
// removed one).
//
// Every fit is on plain text before it is styled — fitting an already-styled
// string measures the escape codes as characters, which is what wraps a row
// and breaks the whole panel.
func renderChangeRow(c rules.RuleChange, width int, under bool) string {
	if width < 1 {
		return ""
	}
	mark, markStyle := changeMark(c.Kind)
	body := c.New
	if c.Kind == rules.Removed {
		body = c.Old
	}

	numStyle := lipgloss.NewStyle().Foreground(theme.Highlight)
	bodyStyle := lipgloss.NewStyle().Foreground(theme.TextDim)
	if under {
		numStyle = numStyle.Foreground(theme.SelectionFg).Bold(true)
		bodyStyle = bodyStyle.Foreground(theme.Text)
	}

	// The marker and the number are the label; the rest of the row is the first
	// line of the rule, if any width is left for it.
	labelPlain := mark + " " + c.Number
	labelW := textWidth(labelPlain)
	line := markStyle.Render(mark) + " " + numStyle.Render(c.Number)
	if rest := width - labelW - 1; rest > 3 {
		line += " " + bodyStyle.Render(fit(firstSentence(body), rest))
	} else {
		line = numStyle.Render(fit(labelPlain, width))
	}

	if under {
		return highlightLine(line, width, theme.SelectionBg)
	}
	return line
}

func changeMark(k rules.ChangeKind) (string, lipgloss.Style) {
	switch k {
	case rules.Added:
		return "+", lipgloss.NewStyle().Foreground(theme.DiffAdd)
	case rules.Removed:
		return "-", lipgloss.NewStyle().Foreground(theme.DiffRemove)
	default:
		return "~", lipgloss.NewStyle().Foreground(theme.Accent)
	}
}

// info is the diff for the highlighted rule: the old wording and the new, which
// is the whole point of the panel — the list can only say a rule changed.
func (v *rulesDiff) info(width int) []string {
	if v.cursor.at < 0 || v.cursor.at >= len(v.changes) {
		return nil
	}
	c := v.changes[v.cursor.at]

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	remove := lipgloss.NewStyle().Foreground(theme.DiffRemove)
	add := lipgloss.NewStyle().Foreground(theme.DiffAdd)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	var kind string
	switch c.Kind {
	case rules.Added:
		kind = "added"
	case rules.Removed:
		kind = "removed"
	default:
		kind = "changed"
	}

	out := append(wrapStyled(c.Number+" · "+kind, width, head), "")
	if c.Old != "" {
		out = append(out, wrapStyled("before", width, dim)...)
		out = append(out, wrapStyled(c.Old, width, remove)...)
	}
	if c.Old != "" && c.New != "" {
		out = append(out, "")
	}
	if c.New != "" {
		out = append(out, wrapStyled("after", width, dim)...)
		out = append(out, wrapStyled(c.New, width, add)...)
	}
	return out
}

func (v *rulesDiff) clear() bool { return false }

func (v *rulesDiff) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	if v.cursor.navKey(k, len(v.changes)) {
		return true, nil
	}
	return false, nil
}

func (v *rulesDiff) keys() []hintGroup {
	return []hintGroup{
		{"navigation", [][2]string{}},
		{"info panel", [][2]string{hint("read the change", keymap.Global, keymap.GlobalInfoUp, keymap.GlobalInfoDown)}},
	}
}
