package cache

import (
	"os"
	"path/filepath"
	"testing"

	"ttr/internal/paths"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ttr-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}

func put(t *testing.T, rel string, size int) {
	t.Helper()
	p := filepath.Join(paths.Cache(), rel)
	os.MkdirAll(filepath.Dir(p), 0755)
	if err := os.WriteFile(p, make([]byte, size), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestUsageByKindAndClearingOneKind(t *testing.T) {
	put(t, "images/a.crop.jpg", 1000)
	put(t, "images/b.crop.jpg", 500)
	put(t, "tagger/oracle-tags.json", 300)
	put(t, "stray.bin", 7)

	use := Usage()
	if u := use["pictures"]; u.Files != 2 || u.Size != 1500 {
		t.Errorf("pictures: %+v", u)
	}
	if u := use["tagger"]; u.Files != 1 || u.Size != 300 {
		t.Errorf("tagger: %+v", u)
	}
	if u := use[Other]; u.Files != 1 {
		t.Errorf("other: %+v", u)
	}

	n, freed, err := Clear("pictures")
	if err != nil || n != 2 || freed != 1500 {
		t.Errorf("clearing pictures: %d files, %d bytes, %v", n, freed, err)
	}
	if _, err := os.Stat(filepath.Join(paths.Cache(), "images")); !os.IsNotExist(err) {
		t.Error("the emptied images directory is still there")
	}
	if Usage()["tagger"].Files != 1 {
		t.Error("clearing pictures took the Tagger tags too")
	}
}
