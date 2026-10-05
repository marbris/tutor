// Package config is ttr's settings file: one small JSON object in the config
// directory, holding the handful of choices that outlive a session.
//
// It is deliberately one owner for one file. The theme lived here first and
// wrote the whole file itself; sync now shares it, so both read-modify-write
// through here rather than each marshalling its own struct and stepping on the
// other's keys.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"ttr/internal/jsonc"
	"ttr/internal/paths"
)

// Config is the whole settings file. Every field carries omitempty, so a file
// only ever holds what was actually set — an untouched install writes nothing
// it didn't have to.
type Config struct {
	Theme string `json:"theme,omitempty"`
	Sync  *Sync  `json:"sync,omitempty"`
	Sort  *Sort  `json:"sort,omitempty"`
	// OtagPrefix goes in front of a Scryfall Tagger tag to make it one of
	// yours: T o and T O tag by removal as otag-removal. A pointer, so ""
	// (no prefix at all) isn't mistaken for unset.
	OtagPrefix *string `json:"otag_prefix,omitempty"`
}

// DefaultOtagPrefix is the otag prefix when config.json doesn't set one.
const DefaultOtagPrefix = "otag-"

// Otag is the prefix in force: the one set, or the default.
func (c Config) Otag() string {
	if c.OtagPrefix == nil {
		return DefaultOtagPrefix
	}
	return *c.OtagPrefix
}

// Sort is how the list orders behave: which ones . and , step through, in
// what order, and which way each one starts out running. Either half can be
// left out and the shipped one stands.
type Sort struct {
	// Cycle is the orders in the order they come round. One left out isn't
	// offered at all.
	Cycle []string `json:"cycle,omitempty"`
	// Direction is "asc" or "desc" by order name, for those that should
	// start the other way from how they ship.
	Direction map[string]string `json:"direction,omitempty"`
}

// Sync is where your decks are mirrored: a git remote you own, and the branch
// they live on. Absent until `ttr sync` is set up.
type Sync struct {
	Remote string `json:"remote"`
	Branch string `json:"branch,omitempty"`
}

// Path is the settings file itself.
func Path() string { return filepath.Join(paths.Config(), "config.json") }

// Load reads the settings. A missing or unreadable file is not an error — it
// is simply the zero config, which every caller already treats as "nothing set
// yet".
func Load() Config {
	var c Config
	body, err := os.ReadFile(Path())
	if err != nil || jsonc.Empty(body) {
		return c
	}
	json.Unmarshal(jsonc.Strip(body), &c)
	return c
}

// Check reports a settings file that is there but can't be read as one — a
// missing comma after an edit, say. Load carries on regardless, with nothing
// set; this is so the reason gets said once, at startup.
func Check() error {
	body, err := os.ReadFile(Path())
	if err != nil || jsonc.Empty(body) {
		return nil
	}
	var c Config
	if err := json.Unmarshal(jsonc.Strip(body), &c); err != nil {
		return fmt.Errorf("%s: %w", Path(), err)
	}
	return nil
}

// Save writes the settings back, whole. Callers Load, change one field, and
// Save, so the keys they don't touch survive.
//
// Not over a file that can't be read, though: Load gave the caller nothing
// for it, and writing that back would lose every setting in it for the sake
// of a typo.
func Save(c Config) error {
	if err := Check(); err != nil {
		return fmt.Errorf("%w — fix it first, or it would be overwritten", err)
	}
	body, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(body, '\n'), 0644)
}
