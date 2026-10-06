// Package uninstall is what ttr uninstall removes, and the removing.
//
// Tutor's files are in four groups, as internal/paths keeps them, plus the
// program and its launcher. The cache and the state go with the program,
// since nothing in them is anyone's work. Settings and decks are asked about
// one at a time.
//
// On Linux each group has its own directory. On macOS and Windows the
// settings, the decks and the state share one, so nothing here removes a
// directory by name alone: Sweep removes everything in it but what is kept,
// and the directory itself only once that leaves it empty.
package uninstall

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"ttr/internal/config"
	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/launcher"
	"ttr/internal/paths"
	"ttr/internal/theme"
)

// Settings is the files you wrote, or ttr wrote for you: config.json,
// keys.json, the themes, and the templates ttr init put beside them.
func Settings() []string {
	files := []string{config.Path(), keymap.Path(), theme.Dir()}
	for _, f := range []string{config.Path(), keymap.Path()} {
		files = append(files, f+".default")
	}
	return existing(files)
}

// Decks is your lists and the decks you follow.
func Decks() []string { return existing([]string{deck.Dir(), deck.BookmarksPath()}) }

// Plan is what a run removes. The program, launcher, cache and state always
// go; settings and decks only when asked for.
type Plan struct {
	Program  string // empty for a temporary build, which removes itself
	Settings bool
	Decks    bool
}

// Run removes what the plan says, and reports each thing that couldn't be,
// carrying on past it.
func Run(p Plan) []error {
	var errs []error
	note := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	var keep []string
	if !p.Settings {
		keep = append(keep, Settings()...)
	}
	if !p.Decks {
		keep = append(keep, Decks()...)
	}

	note(launcher.Remove())
	note(Sweep(paths.Cache(), keep))
	note(Sweep(paths.State(), keep))
	if p.Settings {
		note(Sweep(paths.Config(), keep))
	}
	if p.Decks {
		// TTR_DECKS_DIR may be anywhere; it goes by name, having been named
		// in the question.
		note(os.RemoveAll(deck.Dir()))
		note(Sweep(paths.Data(), keep))
	}
	if p.Program != "" {
		note(RemoveProgram(p.Program))
	}
	return errs
}

// Sweep removes everything in root except the paths in keep, and root
// itself if that leaves it empty. A directory holding something kept is
// swept in turn rather than removed.
func Sweep(root string, keep []string) error {
	if !ours(root) {
		return fmt.Errorf("not removing %s: it isn't ttr's", root)
	}
	return sweep(root, keep)
}

func sweep(dir string, keep []string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var first error
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		switch {
		case kept(path, keep):
		case holds(path, keep):
			if err := sweep(path, keep); err != nil && first == nil {
				first = err
			}
		default:
			if err := os.RemoveAll(path); err != nil && first == nil {
				first = err
			}
		}
	}
	if left, _ := os.ReadDir(dir); len(left) == 0 {
		os.Remove(dir)
	}
	return first
}

// kept reports whether path is one of keep.
func kept(path string, keep []string) bool {
	for _, k := range keep {
		if samePath(path, k) {
			return true
		}
	}
	return false
}

// holds reports whether something kept is inside path.
func holds(path string, keep []string) bool {
	for _, k := range keep {
		rel, err := filepath.Rel(path, k)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }

// ours is the guard against a bad environment variable pointing a sweep at a
// home directory: every directory paths hands out is named ttr.
func ours(dir string) bool { return filepath.Base(filepath.Clean(dir)) == "ttr" }

// RemoveProgram deletes the binary. Windows won't delete a program that is
// running, but will rename it: it is moved aside, and a cmd started now
// deletes it once ttr has exited.
func RemoveProgram(exe string) error {
	if runtime.GOOS != "windows" {
		if err := os.Remove(exe); err != nil && !os.IsNotExist(err) {
			if os.IsPermission(err) {
				return fmt.Errorf("couldn't remove %s: you don't have permission. If a package manager "+
					"installed it, remove it with that; otherwise `sudo rm %s`", exe, exe)
			}
			return err
		}
		return nil
	}
	aside := exe + ".old"
	os.Remove(aside)
	if err := os.Rename(exe, aside); err != nil {
		return err
	}
	return exec.Command("cmd", "/c", "ping -n 3 127.0.0.1 >nul & del /f /q \""+aside+"\"").Start()
}

// Size is how much room the paths take, walking directories.
func Size(paths ...string) int64 {
	var total int64
	for _, p := range paths {
		filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if info, err := d.Info(); err == nil {
					total += info.Size()
				}
			}
			return nil
		})
	}
	return total
}

func existing(paths []string) []string {
	var out []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}
