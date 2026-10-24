package deck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The decks directory is a git repository, so every change to a deck is
// recorded and nothing is ever really lost. We shell out to git rather than
// vendoring an implementation: it's a hundred lines instead of a dependency,
// and it leaves you a repository you can use from the terminal like any
// other — `git log`, `git diff`, `git revert` all work on your decks.
//
// Git is never load-bearing. The deck file is written first and committed
// afterwards, so a machine without git, or a repository in some state we
// didn't expect, costs you the history and not the deck.

const GitNotInstalled = "git isn't installed — decks are saved, but not versioned"

var (
	gitLookupOnce sync.Once
	gitPath       string
)

// GitAvailable reports whether we have a git to shell out to. Looked up
// once: it can't change while the program is running.
func GitAvailable() bool {
	gitLookupOnce.Do(func() {
		if p, err := exec.LookPath("git"); err == nil {
			gitPath = p
		}
	})
	return gitPath != ""
}

// git runs a git command in the decks directory.
func git(args ...string) (string, error) {
	if !GitAvailable() {
		return "", fmt.Errorf("git not found")
	}
	var out []byte
	var err error
	// Another ttr committing at the same moment holds git's lock for a few
	// milliseconds: wait for it rather than fail.
	for try := 0; ; try++ {
		cmd := exec.Command(gitPath, args...)
		cmd.Dir = Dir()
		out, err = cmd.CombinedOutput()
		if err == nil || try == 4 || !strings.Contains(string(out), "index.lock") {
			break
		}
		time.Sleep(time.Duration(50<<try) * time.Millisecond)
	}
	text := strings.TrimRight(string(out), "\n")
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("git %s: %s", args[0], firstLine(text))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return text, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ── The repository ──────────────────────────────────────────────

// ensureRepo makes the decks directory a git repository the first time
// it's needed. If it already sits inside one — someone pointed
// TTR_DECKS_DIR at a checkout they keep themselves — that repository is
// used as it is, rather than nesting a second one inside it.
func ensureRepo() error {
	if !GitAvailable() {
		return fmt.Errorf("git not found")
	}
	if _, err := git("rev-parse", "--git-dir"); err == nil {
		return nil
	}

	if _, err := git("init", "-q"); err != nil {
		return err
	}

	// Committing fails outright without an identity. Rather than refuse to
	// version anything, fall back to one scoped to this repository, leaving
	// a configured identity — global or otherwise — well alone.
	if out, err := git("config", "user.email"); err != nil || strings.TrimSpace(out) == "" {
		if _, err := git("config", "user.email", "ttr@localhost"); err != nil {
			return err
		}
		if _, err := git("config", "user.name", "ttr"); err != nil {
			return err
		}
	}
	return nil
}

// record records one deck file. Only that file is staged, so a
// repository holding other things — or another deck edited in your editor —
// is never swept into a commit it didn't ask to be in.
func record(slug, message string) error {
	if err := ensureRepo(); err != nil {
		return err
	}

	rel := slug + FileExt
	if _, err := git("add", "--", rel); err != nil {
		return err
	}

	// Nothing staged means nothing changed — saving a deck you didn't edit
	// is not an error, it just isn't a commit.
	if _, err := git("diff", "--cached", "--quiet", "--", rel); err == nil {
		return nil
	}

	_, err := git("commit", "-q", "-m", message, "--", rel)
	return err
}

// HasUncommittedEdits reports whether a tracked deck file differs from
// what was last committed. An untracked deck has no edits to lose — its
// first commit records the whole thing anyway.
func HasUncommittedEdits(slug string) bool {
	if !GitAvailable() || !Exists(slug) {
		return false
	}
	if err := ensureRepo(); err != nil {
		return false
	}

	rel := slug + FileExt
	if _, err := git("ls-files", "--error-unmatch", "--", rel); err != nil {
		return false // not tracked yet
	}
	_, err := git("diff", "--quiet", "--", rel)
	return err != nil
}

// DeleteCommitted removes a deck and records that too, so a deck you
// delete by mistake is still in the history.
func DeleteCommitted(slug string) error {
	if err := Delete(slug); err != nil {
		return err
	}
	err := record(slug, "Delete "+slug)
	pruneEmptyDirs(filepath.Dir(Path(slug)))
	return err
}

// Move relocates a deck from one slug to another — between folders, or to a new
// name — and records it. It prefers `git mv` so the history follows the file;
// an untracked or git-less deck falls back to a plain rename, still recorded
// where it can be. The empty folder a move leaves behind is swept up so the
// tree doesn't fill with them.
func Move(oldSlug, newSlug string) error {
	if oldSlug == newSlug {
		return nil
	}
	if Exists(newSlug) {
		return fmt.Errorf("a deck already exists at %q", newSlug)
	}
	oldPath, newPath := Path(oldSlug), Path(newSlug)
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return err
	}

	moved := false
	if GitAvailable() && ensureRepo() == nil {
		if _, err := git("mv", oldSlug+FileExt, newSlug+FileExt); err == nil {
			_, _ = git("commit", "-q", "-m", "Move "+oldSlug+" to "+newSlug,
				"--", oldSlug+FileExt, newSlug+FileExt)
			moved = true
		}
	}
	if !moved {
		if err := os.Rename(oldPath, newPath); err != nil {
			return err
		}
		_ = record(oldSlug, "Move "+oldSlug+" to "+newSlug)
		_ = record(newSlug, "Move "+oldSlug+" to "+newSlug)
	}

	pruneEmptyDirs(filepath.Dir(oldPath))
	return nil
}

// MoveFolder renames a folder by relocating every deck under it to the new
// folder path — "aggro/mono-red" becomes "midrange/mono-red" when aggro is
// renamed to midrange. Each deck keeps its own name; only its file moves, via
// Move, so git history follows and the emptied folder is swept up.
func MoveFolder(oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	slugs, err := List()
	if err != nil {
		return err
	}
	for _, s := range slugs {
		if s == oldPath || strings.HasPrefix(s, oldPath+"/") {
			if err := Move(s, newPath+strings.TrimPrefix(s, oldPath)); err != nil {
				return err
			}
		}
	}
	return nil
}

// pruneEmptyDirs removes now-empty folders left by a move or delete, up to but
// not including the decks directory itself.
func pruneEmptyDirs(dir string) {
	root := Dir()
	for dir != root && strings.HasPrefix(dir, root) {
		if err := os.Remove(dir); err != nil {
			return // not empty, or gone already — nothing more to do
		}
		dir = filepath.Dir(dir)
	}
}

// ── History ─────────────────────────────────────────────────────

type Commit struct {
	Hash    string
	Short   string
	When    string // relative, as git prints it
	Subject string
}

// History lists the commits that touched one deck, newest first.
func History(slug string, limit int) ([]Commit, error) {
	if err := ensureRepo(); err != nil {
		return nil, err
	}

	// %x1f is a unit separator — safe in a way that any character a commit
	// subject might contain is not. --follow keeps a deck's history whole
	// across a move between folders, where the file's path changes but the
	// deck does not.
	out, err := git("log", fmt.Sprintf("-%d", limit), "--follow",
		"--format=%H%x1f%h%x1f%cr%x1f%s", "--", slug+FileExt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 4 {
			continue
		}
		commits = append(commits, Commit{Hash: f[0], Short: f[1], When: f[2], Subject: f[3]})
	}
	return commits, nil
}

// Diff is what one commit did to one deck.
func Diff(slug, hash string) (string, error) {
	return git("show", "--format=%b", "--patch", hash, "--", slug+FileExt)
}

// At reads a deck as it stood at a commit.
func At(slug, hash string) (*File, error) {
	out, err := git("show", hash+":"+slug+FileExt)
	if err != nil {
		return nil, err
	}
	return ParseFile(strings.NewReader(out))
}

// Restore brings an old version back as a new commit, rather than
// rewinding history — the version you restored from is still there, and so
// is the one you restored over.
func Restore(slug, hash string) error {
	old, err := At(slug, hash)
	if err != nil {
		return err
	}
	if err := Write(slug, old); err != nil {
		return err
	}

	short := hash
	if len(short) > 7 {
		short = short[:7]
	}
	return record(slug, "Restore "+slug+" to "+short)
}

// ── Saving ──────────────────────────────────────────────────────

// SaveVersioned writes a deck and commits it, describing the change in
// the commit message. The write comes first and its error is the only one
// that can fail the save: a deck that's on disk but unversioned is a much
// better outcome than one that is neither.
//
// It returns the commit subject, so the caller can say what it recorded, and
// a warning for anything that went wrong with git alone.
func SaveVersioned(slug string, d *File) (subject, warning string, err error) {
	before, _ := Read(slug) // nil when the deck is new, which is fine

	// A deck file is meant to be edited in your own editor too, and those
	// edits are only ever committed the next time ttr writes. Record them
	// first, or writing over them would lose them for good — the one thing
	// keeping the history is supposed to prevent.
	RecordOutsideEdits(slug)

	if err := Write(slug, d); err != nil {
		return "", "", err
	}

	subject = commitSubject(before, d)
	if !GitAvailable() {
		return subject, GitNotInstalled, nil
	}
	if err := record(slug, commitMessage(before, d)); err != nil {
		return subject, "saved, but not committed: " + err.Error(), nil
	}
	return subject, "", nil
}

// CommitDeck writes a deck and commits it, describing the change against the
// last commit rather than against the file — which ttr has been writing on
// every edit, so the file already holds the change and says nothing about it.
// Like SaveVersioned, the write is the only thing that can fail it.
func CommitDeck(slug string, d *File) (subject, warning string, err error) {
	before, _ := At(slug, "HEAD") // nil when the deck was never committed

	if err := Write(slug, d); err != nil {
		return "", "", err
	}

	subject = commitSubject(before, d)
	if !GitAvailable() {
		return subject, GitNotInstalled, nil
	}
	if err := record(slug, commitMessage(before, d)); err != nil {
		return subject, "written, but not committed: " + err.Error(), nil
	}
	return subject, "", nil
}

// RecordOutsideEdits commits whatever was done to a deck in another editor
// since it was last committed, so the next write over it can't lose it.
func RecordOutsideEdits(slug string) {
	if HasUncommittedEdits(slug) {
		_ = record(slug, "Edit "+slug+" outside ttr")
	}
}

// ── Describing a change ─────────────────────────────────────────

type Change struct {
	Added    []string
	Removed  []string
	Requant  []string // quantity changed
	Retagged []string
}

func (c Change) Empty() bool {
	return len(c.Added)+len(c.Removed)+len(c.Requant)+len(c.Retagged) == 0
}

func (c Change) Count() int {
	return len(c.Added) + len(c.Removed) + len(c.Requant) + len(c.Retagged)
}

// DiffDecks works out what happened between two versions of a deck, by card
// name — the identity a person thinks in.
func DiffDecks(before, after *File) Change {
	var c Change
	if after == nil {
		return c
	}

	index := func(d *File) map[string]Entry {
		out := map[string]Entry{}
		if d == nil {
			return out
		}
		for _, e := range d.Entries {
			out[strings.ToLower(e.Name)] = e
		}
		return out
	}
	old, now := index(before), index(after)

	for key, e := range now {
		prev, existed := old[key]
		switch {
		case !existed:
			c.Added = append(c.Added, e.Name)
		case prev.Qty != e.Qty:
			c.Requant = append(c.Requant, e.Name)
		case !sameTags(prev.Tags, e.Tags):
			c.Retagged = append(c.Retagged, e.Name)
		}
	}
	for key, e := range old {
		if _, still := now[key]; !still {
			c.Removed = append(c.Removed, e.Name)
		}
	}

	// Map iteration is random; a commit message must not be.
	for _, s := range [][]string{c.Added, c.Removed, c.Requant, c.Retagged} {
		sort.Strings(s)
	}
	return c
}

func sameTags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// commitSubject is the one-line summary of a change. Small changes name the
// cards, because "+1 Sol Ring" is the whole story; larger ones count them,
// because forty card names is not a subject line.
func commitSubject(before, after *File) string {
	if before == nil {
		name := after.Name
		if name == "" {
			name = "deck"
		}
		return "Add " + name
	}

	c := DiffDecks(before, after)
	if c.Empty() {
		return "Update " + after.Name
	}

	if c.Count() <= 3 {
		var parts []string
		for _, n := range c.Added {
			parts = append(parts, "+"+n)
		}
		for _, n := range c.Removed {
			parts = append(parts, "-"+n)
		}
		for _, n := range c.Requant {
			parts = append(parts, "requantify "+n)
		}
		for _, n := range c.Retagged {
			parts = append(parts, "retag "+n)
		}
		return strings.Join(parts, ", ")
	}

	var parts []string
	if n := len(c.Added); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d", n))
	}
	if n := len(c.Removed); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d", n))
	}
	if n := len(c.Requant); n > 0 {
		parts = append(parts, fmt.Sprintf("%d requantified", n))
	}
	if n := len(c.Retagged); n > 0 {
		parts = append(parts, fmt.Sprintf("%d retagged", n))
	}
	return strings.Join(parts, ", ") + plural(" card", c.Count())
}

func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// commitMessage is the subject plus, when the subject had to summarise, the
// cards it summarised — so `git log` stays skimmable and `git log --format=%b`
// still tells you exactly what moved.
func commitMessage(before, after *File) string {
	subject := commitSubject(before, after)
	if before == nil {
		return subject
	}

	c := DiffDecks(before, after)
	if c.Count() <= 3 {
		return subject
	}

	var b strings.Builder
	b.WriteString(subject + "\n")
	section := func(label string, names []string) {
		if len(names) == 0 {
			return
		}
		b.WriteString("\n" + label + "\n")
		for _, n := range names {
			b.WriteString("  " + n + "\n")
		}
	}
	section("Added:", c.Added)
	section("Removed:", c.Removed)
	section("Quantity changed:", c.Requant)
	section("Retagged:", c.Retagged)
	return strings.TrimRight(b.String(), "\n")
}

// RepoPath is where the deck repository lives, for the CLI to print.
func RepoPath() string {
	if out, err := git("rev-parse", "--show-toplevel"); err == nil && out != "" {
		return out
	}
	return filepath.Clean(Dir())
}
