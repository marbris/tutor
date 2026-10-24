package ui

import (
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/launcher"
	"ttr/internal/upgrade"
)

// Checking for an update from the settings panel: ttr upgrade, in the TUI.
// enter on the version row asks GitHub for the newest release; when it is
// newer, enter again downloads it, checks it against the release's
// SHA256SUMS and puts it in place of this ttr. The one running carries on
// as it is, and the next start is the new one.

// Version is this ttr's version, set by main ("dev" for a build from source).
var Version = "dev"

// updateState is what the version row knows.
type updateState struct {
	checking, installing bool
	// checked is a check that came back; latest is the release it found,
	// when that is newer than this ttr.
	checked   bool
	latest    *upgrade.Release
	installed string // the version put in place, for the next start
	err       error
}

// value is what the version row says beside the version.
func (u updateState) value() string {
	switch {
	case u.installing:
		return Version + " · installing " + u.latest.Version() + "…"
	case u.checking:
		return Version + " · checking…"
	case u.installed != "":
		return Version + " · " + u.installed + " next start"
	case u.err != nil && u.latest != nil:
		return Version + " · couldn't install " + u.latest.Version()
	case u.err != nil:
		return Version + " · couldn't check"
	case u.latest != nil:
		return Version + " · " + u.latest.Version() + " available"
	case u.checked:
		return Version + " · up to date"
	}
	return Version
}

// canInstall is an update found and not yet put in place.
func (u updateState) canInstall() bool {
	return u.latest != nil && u.installed == "" && !u.installing
}

type updateCheckedMsg struct {
	panel int
	rel   upgrade.Release
	err   error
}

type updateInstalledMsg struct {
	panel   int
	version string
	err     error
}

func checkForUpdate(panelID int) tea.Cmd {
	return func() tea.Msg {
		rel, err := upgrade.Latest()
		return updateCheckedMsg{panel: panelID, rel: rel, err: err}
	}
}

func installUpdate(panelID int, rel upgrade.Release) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg { return updateInstalledMsg{panel: panelID, err: err} }
		exe, ok := launcher.Executable()
		if !ok {
			return fail(errTemporaryBuild)
		}
		bin, sums, err := rel.Find(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return fail(err)
		}
		body, err := upgrade.Download(bin, sums)
		if err != nil {
			return fail(err)
		}
		if err := upgrade.Replace(exe, body); err != nil {
			return fail(err)
		}
		return updateInstalledMsg{panel: panelID, version: rel.Version()}
	}
}

type updateError string

func (e updateError) Error() string { return string(e) }

const errTemporaryBuild = updateError("this ttr is a temporary build, with nothing to replace")

// updateKey is enter on the version row: check, or install what was found.
func (m *Model) updateKey(v *settingsView, p *panel) tea.Cmd {
	u := &v.update
	switch {
	case u.checking || u.installing:
		return nil
	case u.canInstall():
		u.installing, u.err = true, nil
		v.refresh()
		return installUpdate(p.id, *u.latest)
	case Version == "dev":
		m.notice = "built from source: git pull && make build updates it"
		return nil
	}
	*u = updateState{checking: true}
	v.refresh()
	return checkForUpdate(p.id)
}

func (m Model) handleUpdateChecked(msg updateCheckedMsg) (tea.Model, tea.Cmd) {
	v := m.settingsIn(msg.panel)
	if v == nil {
		return m, nil
	}
	v.update = updateState{checked: true, err: msg.err}
	switch {
	case msg.err != nil:
		m.notice = "couldn't check for an update: " + errorText(msg.err)
	case upgrade.Newer(msg.rel.Version(), Version):
		rel := msg.rel
		v.update.latest = &rel
		m.notice = "ttr " + rel.Version() + " is out — enter installs it"
	default:
		m.notice = "ttr " + Version + " is the newest"
	}
	v.refresh()
	return m, nil
}

func (m Model) handleUpdateInstalled(msg updateInstalledMsg) (tea.Model, tea.Cmd) {
	v := m.settingsIn(msg.panel)
	if v == nil {
		return m, nil
	}
	v.update.installing = false
	if msg.err != nil {
		v.update.err = msg.err
		m.notice = "couldn't install: " + errorText(msg.err)
	} else {
		v.update.installed = msg.version
		m.notice = "ttr " + msg.version + " installed — it starts next time"
		if v.update.latest != nil && v.update.latest.URL != "" {
			m.notice += " · what's new: " + v.update.latest.URL
		}
	}
	v.refresh()
	return m, nil
}

// settingsIn is the settings view of a panel still open, or nil.
func (m Model) settingsIn(panelID int) *settingsView {
	p := m.ws.byID(panelID)
	if p == nil {
		return nil
	}
	v, _ := p.top().(*settingsView)
	return v
}
