package uninstall

import (
	"os"
	"path/filepath"
	"testing"

	"ttr/internal/config"
	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/paths"
	"ttr/internal/theme"
)

// sandbox points every directory into a fresh temporary one. shared puts
// config, data and state in one directory, the way macOS and Windows have it.
func sandbox(t *testing.T, shared bool) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("TTR_DECKS_DIR", "")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	if shared {
		for _, v := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
			t.Setenv(v, filepath.Join(root, "support"))
		}
	} else {
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
		t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
		t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	}

	write := func(path string) {
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(config.Path())
	write(keymap.Path())
	write(filepath.Join(theme.Dir(), "mine.json"))
	write(filepath.Join(deck.Dir(), "aggro", "mono-red.list"))
	write(deck.BookmarksPath())
	write(filepath.Join(paths.State(), "session.json"))
	write(filepath.Join(paths.Cache(), "images", "a.jpg"))
	return root
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s is still there", path)
	}
}

func there(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s was removed", path)
	}
}

func TestKeepingSettingsAndDecksKeepsThemWhereTheyShareADirectory(t *testing.T) {
	for _, shared := range []bool{false, true} {
		sandbox(t, shared)
		if errs := Run(Plan{}); len(errs) > 0 {
			t.Fatalf("shared=%v: %v", shared, errs)
		}
		gone(t, filepath.Join(paths.State(), "session.json"))
		gone(t, filepath.Join(paths.Cache(), "images"))
		there(t, config.Path())
		there(t, keymap.Path())
		there(t, filepath.Join(theme.Dir(), "mine.json"))
		there(t, filepath.Join(deck.Dir(), "aggro", "mono-red.list"))
		there(t, deck.BookmarksPath())
	}
}

func TestRemovingEverythingLeavesNoDirectories(t *testing.T) {
	for _, shared := range []bool{false, true} {
		root := sandbox(t, shared)
		if errs := Run(Plan{Settings: true, Decks: true}); len(errs) > 0 {
			t.Fatalf("shared=%v: %v", shared, errs)
		}
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			left, _ := os.ReadDir(filepath.Join(root, e.Name()))
			if len(left) > 0 {
				t.Errorf("shared=%v: %s still holds %d entries", shared, e.Name(), len(left))
			}
		}
	}
}

func TestDecksGoWhenAskedAndSettingsStay(t *testing.T) {
	sandbox(t, true)
	Run(Plan{Decks: true})
	gone(t, filepath.Join(deck.Dir(), "aggro"))
	gone(t, deck.BookmarksPath())
	there(t, config.Path())
	there(t, filepath.Join(theme.Dir(), "mine.json"))
}

func TestASweepRefusesADirectoryThatIsntTtrs(t *testing.T) {
	home := t.TempDir()
	os.WriteFile(filepath.Join(home, "thesis.tex"), []byte("x"), 0644)
	if err := Sweep(home, nil); err == nil {
		t.Error("swept a directory not named ttr")
	}
	there(t, filepath.Join(home, "thesis.tex"))
}

func TestTheProgramIsRemoved(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "ttr")
	os.WriteFile(exe, []byte("x"), 0755)
	if err := RemoveProgram(exe); err != nil {
		t.Fatal(err)
	}
	gone(t, exe)
	if err := RemoveProgram(exe); err != nil {
		t.Errorf("a program already gone is an error: %v", err)
	}
}
