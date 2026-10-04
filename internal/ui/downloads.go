package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/catalog"
	"ttr/internal/paths"
	"ttr/internal/rulings"
	"ttr/internal/tagger"
)

// The bulk downloads Tutor keeps, each one a switch in the settings panel.
//
// All are on unless turned off. Off, a download isn't loaded or refreshed,
// and what uses it does without: the Tagger group and otag completion go and
// otag tagging asks Scryfall; rulings are fetched a card at a time;
// highlighting goes by the rules text alone. Turning one off keeps its
// files; clearing them is the cache's business, a row further down.
//
// The switches are kept in the state directory rather than config.json,
// which is written by hand and may carry comments that rewriting it would
// lose.

// download is one switch.
type download struct {
	name  string // as downloads.json and the cache kinds call it
	what  string
	size  string // what it costs, and how often
	load  tea.Cmd
	unuse func(m *Model)
}

var downloads = []download{
	{
		name: "tagger", what: "Scryfall Tagger's tags", size: "about 6 MB a week",
		load:  loadTagger,
		unuse: func(*Model) { tagger.SetCurrent(nil) },
	},
	{
		name: "rulings", what: "every card's rulings", size: "about 5 MB a week, 19 MB kept",
		load:  loadRulingsFile,
		unuse: func(*Model) { rulings.SetCurrent(nil) },
	},
	{
		name: "catalogs", what: "Scryfall's keywords, types and card names", size: "under 1 MB a week",
		load: loadCatalog,
		unuse: func(m *Model) {
			catalog.SetCurrent(nil)
			m.rules.AddCatalog(nil) // back to the rules text's own keywords
		},
	},
}

const downloadsFile = "downloads.json"

func downloadsPath() string { return filepath.Join(paths.State(), downloadsFile) }

// downloadsOff is the downloads turned off, by name.
func downloadsOff() map[string]bool {
	off := map[string]bool{}
	body, err := os.ReadFile(downloadsPath())
	if err != nil {
		return off
	}
	var saved struct {
		Off []string `json:"off"`
	}
	if json.Unmarshal(body, &saved) != nil {
		return off
	}
	for _, n := range saved.Off {
		off[n] = true
	}
	return off
}

func saveDownloadsOff(off map[string]bool) {
	var saved struct {
		Off []string `json:"off"`
	}
	for _, d := range downloads {
		if off[d.name] {
			saved.Off = append(saved.Off, d.name)
		}
	}
	body, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(downloadsPath()), 0755)
	os.WriteFile(downloadsPath(), body, 0644)
}

// downloadLoads is the loading of every download that's on, for the start.
func downloadLoads() []tea.Cmd {
	off := downloadsOff()
	var out []tea.Cmd
	for _, d := range downloads {
		if !off[d.name] {
			out = append(out, d.load)
		}
	}
	return out
}

// toggleDownload turns a download on, loading it, or off, out of use.
func (m *Model) toggleDownload(name string) tea.Cmd {
	off := downloadsOff()
	for _, d := range downloads {
		if d.name != name {
			continue
		}
		if off[name] {
			delete(off, name)
			saveDownloadsOff(off)
			m.notice = d.what + ": on, loading"
			return d.load
		}
		off[name] = true
		saveDownloadsOff(off)
		d.unuse(m)
		m.notice = d.what + ": off — the files stay until the cache is cleared"
	}
	return nil
}
