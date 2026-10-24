package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/theme"
)

// A deck's versions.
//
// gv means "versions of the thing under the cursor" throughout: here, the
// commits that touched a deck; on a card, the wordings it has been printed
// with. Same question, two kinds of thing — which is why it is one key
// rather than two.

type versionList struct {
	cursor
	slug    string
	name    string
	all     []deck.Commit
	commits []deck.Commit
	filter  string
	// diffs are what each commit did, fetched as the cursor reaches it.
	// Local git, so this is fast enough not to need the debouncing a
	// network request does.
	diffs map[string]string
}

type versionsMsg struct {
	panel   int
	slug    string
	name    string
	all     []deck.Commit
	commits []deck.Commit
	filter  string
	err     error
}

// versionLimit is deep enough to find the change you half-remember making,
// and shallow enough that opening it is instant.
const versionLimit = 200

func loadVersions(panelID int, slug, name string) tea.Cmd {
	return func() tea.Msg {
		commits, err := deck.History(slug, versionLimit)
		return versionsMsg{panel: panelID, slug: slug, name: name, commits: commits, err: err}
	}
}

// revertVersion brings a deck back to an older version, recorded as a new
// commit — the version you reverted from stays in the history, and so does the
// one you reverted over. r.
func revertVersion(slug, name, hash string) tea.Cmd {
	return func() tea.Msg {
		if err := deck.Restore(slug, hash); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "reverted " + name + " to " + shortHash(hash)}
	}
}

// copyVersion makes a new deck out of a deck as it stood at an older version,
// leaving the deck you copied from untouched. c.
func copyVersion(slug, name, hash string) tea.Cmd {
	return func() tea.Msg {
		old, err := deck.At(slug, hash)
		if err != nil {
			return noticeMsg{err: err}
		}
		newSlug, copied, err := deck.New(uniqueName(old.Name+" copy"), old.Format)
		if err != nil {
			return noticeMsg{err: err}
		}
		copied.Entries = old.Entries
		copied.Source = old.Source
		if _, _, err := deck.SaveVersioned(newSlug, copied); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "copied to " + newSlug}
	}
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func (m Model) handleVersions(msg versionsMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	p.loading = false
	if msg.err != nil {
		p.err = msg.err
		return m, nil
	}

	v := &versionList{
		slug: msg.slug, name: msg.name, all: msg.commits, commits: msg.commits,
		diffs: map[string]string{},
	}
	p.push(v)
	p.title = v.title()
	return m, v.wantDiff()
}

func (l *versionList) title() string { return l.name + " · versions" }

func (l *versionList) subtitle() string {
	if len(l.commits) == 0 {
		return ""
	}
	return itoa(len(l.commits)) + " " + plural("version", len(l.commits))
}

func (l *versionList) lines(width, height int, focused bool, m *Model) []string {
	if len(l.commits) == 0 {
		return fillTo(mutedLines("no versions recorded", width), width, height)
	}

	l.cursor.scrollInto(height, len(l.commits))
	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.commits) && len(lines) < height; i++ {
		c := l.commits[i]
		under := focused && i == l.cursor.at

		when := c.When
		subject := fit(c.Subject, maxInt(width-textWidth(when)-1, 0))

		style := lipgloss.NewStyle().Foreground(theme.Text)
		if under {
			style = style.Foreground(theme.SelectionFg).Bold(true)
		}
		line := style.Render(subject) + " " +
			lipgloss.NewStyle().Foreground(theme.TextDim).Render(when)
		if under {
			line = highlightLine(line, width, theme.SelectionBg)
		}
		lines = append(lines, line)
	}
	return fillTo(lines, width, height)
}

func (l *versionList) filterText() string { return l.filter }

func (l *versionList) setFilter(s string) {
	l.filter = s
	l.commits = l.all
	if terms := filterTerms(s); len(terms) > 0 {
		kept := make([]deck.Commit, 0, len(l.all))
		for _, c := range l.all {
			hay := strings.ToLower(c.Subject + " " + c.When)
			keep := true
			for _, t := range terms {
				if !strings.Contains(hay, t) {
					keep = false
					break
				}
			}
			if keep {
				kept = append(kept, c)
			}
		}
		l.commits = kept
	}
	l.cursor.clamp(len(l.commits))
}

// clear has nothing transient to drop for esc; the filter is cleared with b.
func (l *versionList) clear() bool { return false }

func (l *versionList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	if l.cursor.navKey(k, len(l.commits)) {
		return true, l.wantDiff()
	}
	switch keymap.Lookup(keymap.Versions, k) {
	case keymap.VersionsFilter:
		p.openFilter(l.filter)
		return true, nil
	case keymap.VersionsRevert:
		if c, ok := l.current(); ok {
			return true, revertVersion(l.slug, l.name, c.Hash)
		}
	case keymap.VersionsCopy:
		if c, ok := l.current(); ok {
			return true, copyVersion(l.slug, l.name, c.Hash)
		}
	}
	return false, nil
}

// current is the commit under the cursor.
func (l *versionList) current() (deck.Commit, bool) {
	if l.cursor.at < 0 || l.cursor.at >= len(l.commits) {
		return deck.Commit{}, false
	}
	return l.commits[l.cursor.at], true
}

// wantDiff fetches the diff for whatever the cursor has landed on, if it
// isn't already in hand.
func (l *versionList) wantDiff() tea.Cmd {
	if l.cursor.at >= len(l.commits) {
		return nil
	}
	c := l.commits[l.cursor.at]
	if _, have := l.diffs[c.Hash]; have {
		return nil
	}
	slug, hash := l.slug, c.Hash
	return func() tea.Msg {
		text, err := deck.Diff(slug, hash)
		if err != nil {
			text = "couldn't read this version: " + err.Error()
		}
		return diffMsg{slug: slug, hash: hash, diff: text}
	}
}

type diffMsg struct {
	slug string
	hash string
	diff string
}

func (m Model) handleDiff(msg diffMsg) (tea.Model, tea.Cmd) {
	for _, p := range m.ws.panels {
		if v, ok := p.top().(*versionList); ok && v.slug == msg.slug {
			v.diffs[msg.hash] = msg.diff
		}
	}
	return m, nil
}

// info is what the version under the cursor did to the deck. A commit
// subject says "+3 cards"; the diff says which three.
func (l *versionList) info(width int) []string {
	if l.cursor.at >= len(l.commits) {
		return nil
	}
	c := l.commits[l.cursor.at]

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	// The subject is the commit's one sentence, and the end of it is the
	// point: it wraps.
	out := wrapStyled(c.Subject, width, head)
	out = append(out, wrapStyled(c.Short+" · "+c.When, width, dim)...)
	out = append(out, mutedLines("r revert to this · c copy from it", width)...)
	out = append(out, "")

	diff, have := l.diffs[c.Hash]
	if !have {
		return append(out, mutedLine("…", width))
	}
	return append(out, renderDiff(diff, width)...)
}

// renderDiff shows what changed and nothing else. Git's headers — the index
// line, the file names, the @@ markers — are noise here: there is one file,
// and you know which.
func renderDiff(diff string, width int) []string {
	add := lipgloss.NewStyle().Foreground(theme.DiffAdd)
	remove := lipgloss.NewStyle().Foreground(theme.DiffRemove)
	same := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var out []string
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"),
			strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "@@"):
			continue
		case strings.HasPrefix(line, "+"):
			out = append(out, wrapStyled(line, width, add)...)
		case strings.HasPrefix(line, "-"):
			out = append(out, wrapStyled(line, width, remove)...)
		case strings.TrimSpace(line) == "":
			continue
		default:
			out = append(out, wrapStyled(line, width, same)...)
		}
	}
	if len(out) == 0 {
		return mutedLines("nothing changed in the deck itself", width)
	}
	return out
}

func (l *versionList) keys() []hintGroup {
	return []hintGroup{
		{"navigation", [][2]string{
			hint("filter", keymap.Versions, keymap.VersionsFilter),
		}},
		{"versions", [][2]string{
			hint("revert to", keymap.Versions, keymap.VersionsRevert),
			hint("copy from", keymap.Versions, keymap.VersionsCopy),
		}},
	}
}
