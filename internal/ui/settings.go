package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/cache"
	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/theme"
)

// The settings panel, space c: what used to be ttr sync remote and
// ttr cache, in the TUI. Three groups of rows:
//
//   - sync: the git remote your decks mirror to. enter sets it, d
//     disconnects.
//   - downloads: the bulk files Tutor keeps (downloads.go). enter turns one
//     on or off.
//   - cache: what's kept, by kind, and how much room it takes. d clears a
//     kind. Everything in the cache can be fetched again.
//
// The info panel says what the row under the cursor is, and what clearing
// it costs.

type settingsRowKind int

const (
	srowHeading settingsRowKind = iota
	srowRemote
	srowDownload
	srowCacheKind
	srowCacheTotal
)

type settingsRow struct {
	kind  settingsRowKind
	name  string // the heading, download or cache kind
	label string
	value string
}

type settingsView struct {
	cursor
	rows []settingsRow
}

func newSettings() *settingsView {
	v := &settingsView{}
	v.refresh()
	return v
}

// refresh reads everything the rows show afresh: the remote, the switches,
// and the cache's sizes, which takes a walk of the cache directory — so on
// opening and after each change, not on every frame.
func (v *settingsView) refresh() {
	at := v.cursor.at
	v.rows = nil
	add := func(r settingsRow) { v.rows = append(v.rows, r) }

	add(settingsRow{kind: srowHeading, label: "sync"})
	remote := "not connected"
	if url, branch, ok := deck.SyncRemote(); ok {
		remote = url + " (" + branch + ")"
	}
	add(settingsRow{kind: srowRemote, label: "git remote", value: remote})

	add(settingsRow{kind: srowHeading, label: "downloads"})
	off := downloadsOff()
	for _, d := range downloads {
		state := "on"
		if off[d.name] {
			state = "off"
		}
		add(settingsRow{kind: srowDownload, name: d.name, label: d.what, value: state})
	}

	add(settingsRow{kind: srowHeading, label: "cache"})
	use := cache.Usage()
	var total int64
	for _, u := range use {
		total += u.Size
	}
	for _, k := range cache.Kinds {
		u := use[k.Name]
		add(settingsRow{kind: srowCacheKind, name: k.Name, label: k.What, value: cache.Size(u.Size)})
	}
	add(settingsRow{kind: srowCacheTotal, label: "total", value: cache.Size(total)})

	v.cursor.at = at
	v.settle(1)
}

// settle keeps the cursor off the headings, moving on in direction dir.
func (v *settingsView) settle(dir int) {
	n := len(v.rows)
	v.cursor.clamp(n)
	for i := 0; i < n && v.rows[v.cursor.at].kind == srowHeading; i++ {
		next := v.cursor.at + dir
		if next < 0 || next >= n {
			dir = -dir
			next = v.cursor.at + dir
		}
		v.cursor.at = next
	}
}

func (v *settingsView) current() (settingsRow, bool) {
	if v.cursor.at < 0 || v.cursor.at >= len(v.rows) {
		return settingsRow{}, false
	}
	return v.rows[v.cursor.at], true
}

func (v *settingsView) title() string    { return "settings" }
func (v *settingsView) subtitle() string { return "sync · downloads · cache" }
func (v *settingsView) clear() bool      { return false }

func (v *settingsView) lines(width, height int, focused bool, m *Model) []string {
	v.cursor.scrollInto(height, len(v.rows))
	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	text := lipgloss.NewStyle().Foreground(theme.Text)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	var out []string
	for i := v.cursor.offset; i < len(v.rows) && len(out) < height; i++ {
		r := v.rows[i]
		if r.kind == srowHeading {
			out = append(out, head.Render(fit(r.label, width)))
			continue
		}
		valueW := textWidth(r.value)
		label := fit("  "+r.label, maxInt(width-valueW-1, 0))
		if focused && i == v.cursor.at {
			line := lipgloss.NewStyle().Foreground(theme.SelectionFg).Bold(true).Render(label) + " " +
				lipgloss.NewStyle().Foreground(theme.SelectionFg).Render(r.value)
			out = append(out, highlightLine(line, width, theme.SelectionBg))
			continue
		}
		out = append(out, text.Render(label)+" "+dim.Render(r.value))
	}
	return fillTo(out, width, height)
}

func (v *settingsView) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	switch keymap.Lookup(keymap.List, k) {
	case keymap.ListDown:
		v.cursor.move(1, len(v.rows))
		v.settle(1)
		return true, nil
	case keymap.ListUp:
		v.cursor.move(-1, len(v.rows))
		v.settle(-1)
		return true, nil
	}
	r, ok := v.current()
	if !ok {
		return false, nil
	}
	switch keymap.Lookup(keymap.Settings, k) {
	case keymap.SettingsChange:
		switch r.kind {
		case srowRemote:
			url, _, _ := deck.SyncRemote()
			p.ask(askRemote, "git remote", url)
			p.askInput.Placeholder = "git@github.com:you/mtg-decks.git — a private repository you own"
			return true, nil
		case srowDownload:
			cmd := m.toggleDownload(r.name)
			v.refresh()
			return true, cmd
		}
	case keymap.SettingsClear:
		switch r.kind {
		case srowRemote:
			if !deck.SyncConfigured() {
				m.notice = "no git remote to disconnect"
				return true, nil
			}
			return true, disconnectRemote(p.id)
		case srowCacheKind:
			return true, clearCacheKind(p.id, r.name)
		}
	}
	return false, nil
}

// info says what the row is, and what changing or clearing it costs.
func (v *settingsView) info(width int) []string {
	r, ok := v.current()
	if !ok {
		return nil
	}
	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	var body []string
	switch r.kind {
	case srowRemote:
		body = []string{
			"Your decks are a git repository. Connect a private repository you own, on any git host, " +
				"and space s pushes your decks to it and pulls in what you changed elsewhere.",
			"enter sets the remote · d disconnects (your decks stay as they are)",
		}
	case srowDownload:
		for _, d := range downloads {
			if d.name == r.name {
				body = []string{d.what + ", " + d.size + ".", downloadEffect(d.name), "enter turns it on or off"}
			}
		}
	case srowCacheKind:
		for _, k := range cache.Kinds {
			if k.Name == r.name {
				body = []string{k.What + ": " + r.value + ".", "Clearing it: " + k.Refetch + ".", "d clears it"}
			}
		}
	case srowCacheTotal:
		body = []string{"Everything in the cache can be downloaded again, so clearing any of it loses nothing but time."}
	}
	out := []string{head.Render(fit(r.label, width)), ""}
	for _, para := range body {
		out = append(out, wrapStyled(para, width, lipgloss.NewStyle().Foreground(theme.Text))...)
		out = append(out, "")
	}
	return out
}

// downloadEffect is what a download being off does without.
func downloadEffect(name string) string {
	switch name {
	case "tagger":
		return "Off, the statistics have no Scryfall Tagger group, otag: isn't completed, and tagging by otag asks Scryfall."
	case "rulings":
		return "Off, each card's rulings are fetched from Scryfall as the cursor settles on it."
	case "catalogs":
		return "Off, keywords are highlighted from the rules text alone, subtypes sit under the card's main type, and card names aren't completed."
	}
	return ""
}

func (v *settingsView) keys() []hintGroup {
	nav := [][2]string{}
	var acts [][2]string
	if r, ok := v.current(); ok {
		switch r.kind {
		case srowRemote:
			acts = append(acts, hint("set the remote", keymap.Settings, keymap.SettingsChange))
			if deck.SyncConfigured() {
				acts = append(acts, hint("disconnect", keymap.Settings, keymap.SettingsClear))
			}
		case srowDownload:
			acts = append(acts, hint("turn on/off", keymap.Settings, keymap.SettingsChange))
		case srowCacheKind:
			acts = append(acts, hint("clear it", keymap.Settings, keymap.SettingsClear))
		}
	}
	return []hintGroup{{"navigation", nav}, {"settings", acts}}
}

// ── Doing it, off the main thread ───────────────────────────────

// settingsDoneMsg is a change made from the settings panel: what to say,
// and the panel to read afresh.
type settingsDoneMsg struct {
	panel  int
	notice string
}

func connectRemote(panelID int, url string) tea.Cmd {
	return func() tea.Msg {
		if err := deck.Connect(strings.TrimSpace(url), ""); err != nil {
			return settingsDoneMsg{panel: panelID, notice: "couldn't connect: " + err.Error()}
		}
		return settingsDoneMsg{panel: panelID, notice: "connected — space s pushes your decks"}
	}
}

func disconnectRemote(panelID int) tea.Cmd {
	return func() tea.Msg {
		if err := deck.Disconnect(); err != nil {
			return settingsDoneMsg{panel: panelID, notice: "couldn't disconnect: " + err.Error()}
		}
		return settingsDoneMsg{panel: panelID, notice: "disconnected; your decks are untouched"}
	}
}

func clearCacheKind(panelID int, kind string) tea.Cmd {
	return func() tea.Msg {
		n, freed, err := cache.Clear(kind)
		if err != nil {
			return settingsDoneMsg{panel: panelID, notice: "error: " + err.Error()}
		}
		for _, k := range cache.Kinds {
			if k.Name == kind {
				return settingsDoneMsg{panel: panelID, notice: "cleared " + k.What + ": " + itoa(n) + " " +
					plural("file", n) + ", " + cache.Size(freed) + " — " + k.Refetch}
			}
		}
		return settingsDoneMsg{panel: panelID}
	}
}

func (m Model) handleSettingsDone(msg settingsDoneMsg) (tea.Model, tea.Cmd) {
	if msg.notice != "" {
		m.notice = msg.notice
	}
	if p := m.ws.byID(msg.panel); p != nil {
		if v, ok := p.top().(*settingsView); ok {
			v.refresh()
		}
	}
	return m, nil
}

// openSettings is space c: the settings panel, beside the one you're in.
func (m *Model) openSettings() {
	m.ws.open(KindSettings).show(newSettings())
}
