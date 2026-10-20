package ui

import (
	"os"
	"path/filepath"
	"testing"

	"ttr/internal/cardstore"
)

// TestMain redirects every directory the program writes to, the same as
// every other package here.
//
// It matters more in this one than most: the decks panel reads the deck
// directory the moment it opens, and its keys create, rename and delete
// decks. A test run that found the real directory would be editing someone's
// collection.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ttr-ui-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	os.Setenv("TTR_DECKS_DIR", filepath.Join(root, "decks"))
	// Nothing goes to the real terminal: a theme switch would set its
	// background. Tests that want the bytes catch them themselves.
	writeTerminal = func([]byte) {}
	// Nor does a test download every card: the store comes from fixtures.
	cardstore.BulkURL = "http://127.0.0.1:1/no-downloads-in-tests"

	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
