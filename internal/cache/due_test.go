package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ttr/internal/paths"
)

func TestAFileKeptBeforeARefreshIsDue(t *testing.T) {
	resetDue()
	old := filepath.Join(paths.Cache(), "images", "old.crop.jpg")
	os.MkdirAll(filepath.Dir(old), 0755)
	os.WriteFile(old, []byte("x"), 0644)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(old, past, past)

	if DueFile("pictures", old) {
		t.Fatal("due before anything was refreshed")
	}
	if err := MarkDue("pictures"); err != nil {
		t.Fatal(err)
	}
	if !DueFile("pictures", old) {
		t.Error("a picture kept from before the refresh isn't due")
	}
	if DueFile("texts", old) {
		t.Error("refreshing pictures made another kind due")
	}
	if Due("pictures", time.Now()) {
		t.Error("a picture fetched since is due")
	}

	// The mark outlives the run.
	resetDue()
	if !DueFile("pictures", old) {
		t.Error("the mark wasn't kept")
	}
}
