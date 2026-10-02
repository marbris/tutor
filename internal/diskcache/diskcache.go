// Package diskcache keeps what Scryfall has told us on disk, so it needn't
// be asked again on the next run.
//
// Scryfall asks clients to cache what they download for at least a day, and
// its prices only change once a day anyway. Asking again is also what costs
// the time: a list of printings is a search, and searches are held to two a
// second. So an answer is kept for as long as it can be trusted, and an older
// one is still better than nothing when asking again fails.
//
// A file's age is its modification time; there is nothing else to keep.
package diskcache

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"ttr/internal/paths"
)

// Key is a file name for anything — a URL, a query — that is safe on every
// system and the same every run.
func Key(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:8]) + ".json"
}

// Load reads the cached copy at rel, inside the cache directory, into v. ok
// is whether there was one; fresh, whether it is younger than maxAge.
func Load(rel string, maxAge time.Duration, v any) (fresh, ok bool) {
	path := filepath.Join(paths.Cache(), rel)
	info, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	body, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(body, v) != nil {
		return false, false
	}
	return time.Since(info.ModTime()) < maxAge, true
}

// Fresh reports whether there is a copy at rel younger than maxAge, without
// reading it.
func Fresh(rel string, maxAge time.Duration) bool {
	info, err := os.Stat(filepath.Join(paths.Cache(), rel))
	return err == nil && time.Since(info.ModTime()) < maxAge
}

// Save keeps v at rel. Failing to is silent: the cache is a convenience, and
// losing a copy costs only a request.
func Save(rel string, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		return
	}
	path := filepath.Join(paths.Cache(), rel)
	if os.MkdirAll(filepath.Dir(path), 0755) != nil {
		return
	}
	// Written aside and moved into place, so a run that dies halfway leaves
	// the old copy rather than half a new one.
	tmp := path + ".tmp"
	if os.WriteFile(tmp, body, 0644) != nil {
		return
	}
	os.Rename(tmp, path)
}
