package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ttr/internal/paths"
	"ttr/internal/tagger"
)

func settingsPanel(t *testing.T, m Model) (Model, *settingsView) {
	t.Helper()
	m = drive(m, "space", "c")
	v, ok := m.ws.current().top().(*settingsView)
	if !ok {
		t.Fatalf("space c opened %T, not the settings", m.ws.current().top())
	}
	return m, v
}

// pointRow puts the cursor on the row with this label.
func pointRow(t *testing.T, v *settingsView, label string) {
	t.Helper()
	for i, r := range v.rows {
		if strings.HasPrefix(r.label, label) {
			v.cursor.at = i
			return
		}
	}
	t.Fatalf("no %q row", label)
}

func TestSpaceCOpensTheSettingsAndSpaceXClosesAPanel(t *testing.T) {
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, _ = settingsPanel(t, m)
	view := stripANSI(m.View())
	for _, want := range []string{"sync", "git remote", "downloads", "Scryfall Tagger's tags", "cache", "card pictures", "total"} {
		if !strings.Contains(view, want) {
			t.Errorf("the settings don't show %q", want)
		}
	}
	if m.ws.count() != 2 {
		t.Fatalf("%d panels, want the deck and the settings", m.ws.count())
	}
	m = drive(m, "space", "x")
	if m.ws.count() != 1 {
		t.Errorf("space x left %d panels", m.ws.count())
	}
}

func TestTheCursorSkipsTheHeadings(t *testing.T) {
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	for i := 0; i < len(v.rows)+2; i++ {
		if r, _ := v.current(); r.kind == srowHeading {
			t.Fatalf("the cursor sits on the %q heading", r.label)
		}
		m = drive(m, "j")
	}
}

func TestADownloadTurnsOffAndOnFromTheSettings(t *testing.T) {
	withTagger(t)
	t.Cleanup(func() { os.Remove(downloadsPath()) })
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "Scryfall Tagger")
	if hints := fmt.Sprint(v.keys()); !strings.Contains(hints, "turn on/off") || strings.Contains(hints, "clear it") {
		t.Errorf("hints on a download: %s", hints)
	}
	m = drive(m, "enter")
	if !downloadsOff()["tagger"] {
		t.Error("enter didn't turn the Tagger tags off")
	}
	if tagger.Current() != nil {
		t.Error("the Tagger tags are still in use after turning them off")
	}
	if r, _ := v.current(); r.value != "off" {
		t.Errorf("the row says %q", r.value)
	}
	m, cmd := press(m, "enter")
	if downloadsOff()["tagger"] || cmd == nil {
		t.Error("enter didn't turn them back on and load them")
	}
}

func TestDClearsACacheKind(t *testing.T) {
	pic := filepath.Join(paths.Cache(), "images", "x.crop.jpg")
	os.MkdirAll(filepath.Dir(pic), 0755)
	os.WriteFile(pic, make([]byte, 2048), 0644)

	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "card pictures")
	if r, _ := v.current(); r.value == "0 B" {
		t.Errorf("the pictures row says %q with a picture kept", r.value)
	}
	m, cmd := press(m, "d")
	m = settle(m, cmd)
	if _, err := os.Stat(pic); !os.IsNotExist(err) {
		t.Error("d didn't clear the pictures")
	}
	if !strings.Contains(m.notice, "cleared card pictures") {
		t.Errorf("notice %q", m.notice)
	}
	if r, _ := v.current(); r.value != "0 B" {
		t.Errorf("after clearing, the row says %q", r.value)
	}
}

func TestTheSettingsComeBackNextSession(t *testing.T) {
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, _ = settingsPanel(t, m)
	m.saveSession()
	t.Cleanup(func() { os.Remove(sessionPath()) })
	found := false
	for _, ps := range loadSession().Panels {
		if ps.Kind == "settings" {
			found = true
		}
	}
	if !found {
		t.Error("the settings panel isn't saved with the session")
	}
}
