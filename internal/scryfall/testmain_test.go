package scryfall

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects every directory the program writes to, so a test can
// never touch the decks, settings or cache of whoever is running it.
//
// This lives here rather than in each helper because an individual test
// can't be trusted to remember. Setting HOME used to be enough; it stopped
// being enough when the paths started honouring XDG_*, and the first thing
// that happened was a test fixture appearing in a real deck directory.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ttr-test-*")
	if err != nil {
		panic(err)
	}

	os.Setenv("HOME", root)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	// Belt and braces: decks have their own override, and they are the one
	// thing here that would be someone's own work.
	os.Setenv("TTR_DECKS_DIR", filepath.Join(root, "decks"))

	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
