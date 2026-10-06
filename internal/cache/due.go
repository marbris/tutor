package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ttr/internal/paths"
)

// Refreshing the kinds that are fetched a card or a set at a time —
// pictures, printings, printed texts — doesn't download them all again at
// once: there are a thousand pictures. MarkDue notes when it was asked
// instead, and each file kept from before then is due: fetched again the
// next time it's wanted, the kept copy standing in if that fails.
//
// The marks are kept in the state dir, not the cache, so clearing the cache
// doesn't take them with it — though then there is nothing left to be due.

const dueFile = "refresh.json"

var (
	dueMu    sync.Mutex
	dueMarks map[string]int64 // read once: by kind, in Unix nanoseconds
)

func duePath() string { return filepath.Join(paths.State(), dueFile) }

func loadDue() map[string]int64 {
	if dueMarks != nil {
		return dueMarks
	}
	dueMarks = map[string]int64{}
	if body, err := os.ReadFile(duePath()); err == nil {
		json.Unmarshal(body, &dueMarks)
	}
	return dueMarks
}

// MarkDue makes every file of a kind kept until now due.
func MarkDue(kind string) error {
	dueMu.Lock()
	defer dueMu.Unlock()
	marks := loadDue()
	marks[kind] = time.Now().UnixNano()
	body, err := json.MarshalIndent(marks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(duePath()), 0755); err != nil {
		return err
	}
	return os.WriteFile(duePath(), body, 0644)
}

// coarse is how far a file's time can lag the clock. The kernel stamps files
// from a clock that ticks every few milliseconds, so a file written just
// after MarkDue can carry a time from just before it, and would be due again.
const coarse = 50 * time.Millisecond

// Due reports whether a file of a kind, written at modTime, was kept from
// before its kind was last refreshed.
func Due(kind string, modTime time.Time) bool {
	dueMu.Lock()
	defer dueMu.Unlock()
	at, ok := loadDue()[kind]
	return ok && modTime.UnixNano() < at-int64(coarse)
}

// DueFile is Due for a file on disk. A file that isn't there isn't due.
func DueFile(kind, path string) bool {
	info, err := os.Stat(path)
	return err == nil && Due(kind, info.ModTime())
}

// resetDue forgets the marks read, for the tests.
func resetDue() {
	dueMu.Lock()
	dueMarks = nil
	dueMu.Unlock()
}
