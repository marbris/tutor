package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ttr/internal/mtg"
	"ttr/internal/paths"
	"ttr/internal/tagger"
	"ttr/internal/theme"
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

func TestRMakesThePicturesDueAndTheNextLookFetchesAgain(t *testing.T) {
	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 49, 68)), nil)
	hits, broken := 0, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hits++
		w.Write(jpg.Bytes())
	}))
	defer srv.Close()
	p := mtg.Card{ID: "duetest", ImageURIs: mtg.ImageURIs{BorderCrop: srv.URL + "/crop.jpg"}}
	kept := filepath.Join(paths.Cache(), "images", "duetest.crop.jpg")
	t.Cleanup(func() { os.Remove(kept) })

	printingPNG(p, 0)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(kept, past, past)

	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "card pictures")
	m, cmd := press(m, "r")
	if cmd != nil {
		t.Error("marking the pictures due shouldn't fetch anything yet")
	}
	if !strings.Contains(m.notice, "next time") {
		t.Errorf("notice %q", m.notice)
	}

	// Scryfall down: the kept picture stands in, and stays due.
	broken = true
	if _, _, _, _, err := printingPNG(p, 0); err != nil {
		t.Errorf("a due picture with Scryfall down failed: %v", err)
	}
	broken = false
	printingPNG(p, 0)
	printingPNG(p, 0)
	if hits != 2 {
		t.Errorf("fetched %d times, want twice: once, and once refreshed", hits)
	}
}

func TestROnTaggerDownloadsItAgain(t *testing.T) {
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "Scryfall Tagger's tags")
	m, cmd := press(m, "r")
	if cmd == nil {
		t.Fatal("r didn't start a download")
	}
	if !strings.Contains(m.notice, "again") {
		t.Errorf("notice %q", m.notice)
	}
	if m = drive(m, "?"); !strings.Contains(stripANSI(m.View()), "r refresh") {
		t.Error("no r hint")
	}
}

func TestRSaysWhenADownloadIsOff(t *testing.T) {
	saveDownloadsOff(map[string]bool{"rulings": true})
	t.Cleanup(func() { saveDownloadsOff(map[string]bool{}) })
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "every card's rulings")
	m, cmd := press(m, "r")
	if cmd != nil || !strings.Contains(m.notice, "turned off") {
		t.Errorf("r on a download that's off: notice %q", m.notice)
	}
}

func TestTheThemeDropdownPutsEachThemeOnAndEscPutsTheOldOneBack(t *testing.T) {
	t.Cleanup(func() { theme.Set(theme.DefaultName); theme.Load() })
	theme.Set("gruvbox")
	theme.Load()

	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "theme")
	m = drive(m, "enter")
	if !v.picking {
		t.Fatal("enter on the theme row didn't open the dropdown")
	}
	if r, _ := v.current(); r.kind != srowThemeChoice || r.name != "gruvbox" {
		t.Fatalf("the dropdown opened on %q, not the theme in force", r.name)
	}
	view := stripANSI(m.View())
	for _, want := range []string{"nord", "gruvbox-light", "light", "dark"} {
		if !strings.Contains(view, want) {
			t.Errorf("the dropdown doesn't show %q", want)
		}
	}

	m = drive(m, "j")
	r, _ := v.current()
	if theme.Current() != r.name {
		t.Fatalf("j moved to %q but the saved theme is %q", r.name, theme.Current())
	}
	for _, row := range v.rows {
		if row.kind == srowTheme && row.value != r.name {
			t.Errorf("the theme row says %q while %q is on", row.value, r.name)
		}
	}
	want, _ := theme.Find(r.name)
	if string(theme.Bg) != want.Palette["bg"] {
		t.Errorf("the background is %s, not %s's %s", theme.Bg, r.name, want.Palette["bg"])
	}
	if !strings.Contains(fmt.Sprint(m.hintGroups()), "put gruvbox back") {
		t.Error("esc's hint doesn't say it puts gruvbox back")
	}

	m = drive(m, "esc")
	if v.picking {
		t.Error("esc left the dropdown open")
	}
	if theme.Current() != "gruvbox" {
		t.Errorf("esc left %q on, not gruvbox", theme.Current())
	}
	if m.ws.count() != 2 {
		t.Error("esc took the panel too")
	}
}

func TestEnterKeepsTheThemeUnderTheCursor(t *testing.T) {
	t.Cleanup(func() { theme.Set(theme.DefaultName); theme.Load() })
	theme.Set("gruvbox")
	theme.Load()

	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "theme")
	m = drive(m, "enter", "k", "enter")
	if v.picking {
		t.Fatal("enter didn't close the dropdown")
	}
	if theme.Current() == "gruvbox" {
		t.Fatal("enter after k kept gruvbox")
	}
	kept := theme.Current()
	if r, _ := v.current(); r.kind != srowTheme || r.value != kept {
		t.Errorf("the cursor is on %q (%q), not the theme row saying %q", r.label, r.value, kept)
	}
	// Up and down off the ends of the dropdown stay in it.
	m = drive(m, "enter")
	for i := 0; i < 40; i++ {
		m = drive(m, "k")
	}
	if r, _ := v.current(); r.kind != srowThemeChoice {
		t.Errorf("k walked out of the dropdown onto %q", r.label)
	}
}

func TestSwitchingThemeSetsTheTerminalsBackgroundAndTerminalPutsItBack(t *testing.T) {
	var sent []string
	old := writeTerminal
	writeTerminal = func(b []byte) { sent = append(sent, string(b)) }
	t.Cleanup(func() {
		writeTerminal = old
		theme.Set(theme.DefaultName)
		theme.Load()
		theme.RestoreTerminalBackground(io.Discard)
	})

	m := sized(160, 40)
	m.useTheme("gruvbox")
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "\x1b]11;#") {
		t.Fatalf("gruvbox sent %q, want the terminal's background set", sent)
	}
	m.useTheme("terminal")
	if len(sent) != 2 || sent[1] != "\x1b]111\x1b\\" {
		t.Errorf("terminal sent %q, want the background put back", sent)
	}
}
