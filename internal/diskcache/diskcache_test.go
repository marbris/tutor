package diskcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ttr/internal/paths"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ttr-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}

func TestWhatIsSavedComesBackFresh(t *testing.T) {
	Save("things/a.json", []string{"x", "y"})
	var got []string
	fresh, ok := Load("things/a.json", time.Hour, &got)
	if !ok || !fresh || len(got) != 2 {
		t.Fatalf("fresh=%v ok=%v got=%v", fresh, ok, got)
	}
}

func TestAnOldCopyIsStaleButStillThere(t *testing.T) {
	Save("things/b.json", 42)
	old := time.Now().Add(-48 * time.Hour)
	os.Chtimes(filepath.Join(paths.Cache(), "things/b.json"), old, old)
	var got int
	fresh, ok := Load("things/b.json", 24*time.Hour, &got)
	if !ok || fresh || got != 42 {
		t.Errorf("fresh=%v ok=%v got=%v", fresh, ok, got)
	}
}

func TestNothingSavedIsNotOK(t *testing.T) {
	var got int
	if _, ok := Load("things/none.json", time.Hour, &got); ok {
		t.Error("a missing file loaded")
	}
}

func TestKeysAreStableAndDistinct(t *testing.T) {
	if Key("a") != Key("a") || Key("a") == Key("b") {
		t.Error("keys aren't a function of their input")
	}
}
