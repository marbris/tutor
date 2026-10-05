package ui

import (
	"path"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/deck"
	"ttr/internal/keymap"
	"ttr/internal/moxfield"
	"ttr/internal/theme"
)

// The decks panel: everything you can open, in one list.
//
// Three kinds of thing live here and the design is deliberate about keeping
// them together rather than in three panes. They answer the same question —
// what can I look at? — and the differences between them are one letter
// wide:
//
//	L  a local deck, a file you can edit
//	R  a remote deck on Moxfield, which you can take a copy of
//	U  somebody you follow — a folder of their public decks
//
// The row is name, kind, legality, size and age. In a narrow panel the tail
// is given up from the right, because a name with no age beside it is still
// a name, and an age with no name is nothing.

type entryKind int

const (
	entryLocal entryKind = iota
	entryRemote
	entryUser
	entryFolder
	// entryUserDeck is one of a followed person's public decks, listed under
	// them from the cache. A remote you haven't followed, one by one.
	entryUserDeck
)

func (k entryKind) letter() string {
	switch k {
	case entryRemote, entryUserDeck:
		return "R"
	case entryUser:
		return "U"
	}
	return "L"
}

// moxFolder is the virtual folder the followed remotes and people live under.
// It isn't a directory on disk — it's built from the bookmarks each refresh —
// so the tree can show Moxfield as one branch beside your own folders. It
// sorts first, and in its own colour, so it never passes for one of yours.
const moxFolder = "moxfield"

// globalTagsFolder is the virtual folder the lists pinned to the global tags show in, as
// a second row each: the file stays in its own folder too. Its name has a
// space, which a folder on disk never does, so it can't be mistaken for one.
const globalTagsFolder = "global tags"

// virtualFolder reports whether a folder is built by the panel rather than
// found on disk.
func virtualFolder(f string) bool { return f == moxFolder || f == globalTagsFolder }

// userFolder is where a followed person's decks sit in the tree.
func userFolder(user string) string { return moxFolder + "/" + strings.ToLower(user) }

// deckEntry is one row.
type deckEntry struct {
	kind entryKind
	name string
	// slug identifies a local deck; id identifies a remote one; user is the
	// Moxfield name. Exactly one is set — except on a person's deck, which
	// has its id and the user it belongs to, and on a person in the tree,
	// whose slug is their folder.
	slug string
	id   string
	user string

	format string
	count  int // cards for a deck, decks for a user or a folder
	// colours is the deck's colour identity, in WUBRG order. Empty for a
	// remote, whose cards we haven't looked at, and for a person.
	colours  []string
	modified time.Time
	// legal is the verdict, which stays unknown until the background pass
	// gets to this deck.
	legal  deck.Legality
	broken bool

	// depth is how deep in the folder tree this row sits, for indentation. A
	// folder row also uses slug for its full path and name for its last
	// segment, and open for whether it is expanded.
	depth int
	open  bool
	// loading is a person whose decks are being fetched.
	loading bool

	// tagRef is the second row a pinned list gets, under the global tags folder.
	tagRef bool

	// folder is a local deck's location — the folder part of its slug, empty at
	// the top level. The grouped tree shows it as the branch a deck sits under;
	// a flattened filter view, where the branch is gone, shows it beside the
	// name so a match stays locatable.
	folder string
}

type deckListSort int

const (
	byModified deckListSort = iota
	byName
	bySize
	byKind
)

func (s deckListSort) String() string {
	switch s {
	case byName:
		return "name"
	case bySize:
		return "size"
	case byKind:
		return "kind"
	}
	return "last touched"
}

type deckList struct {
	cursor
	all  []deckEntry
	rows []deckEntry

	// folders is every directory on disk, so a folder shows in the tree even
	// while it holds no decks yet — the panel mirrors the decks directory
	// rather than inferring folders only from the decks in them.
	folders []string

	order  deckListSort
	filter string

	// legality is what the background pass has worked out so far, kept
	// across reloads so the flags don't blink off every time a deck is
	// written.
	legality map[string]deck.Legality
	colours  map[string][]string

	// confirming is a deletion waiting for a yes, and holds the row it
	// would delete so that moving the cursor can't redirect it.
	confirming *deckEntry

	// expanded is the set of folder paths the tree is holding open. Folders are
	// collapsed by default — a folder absent here is shut — so opening the panel
	// shows folders folded until you open one, and the ones you open stay open
	// across reloads.
	expanded map[string]bool

	// moving is a deck staged by y (copy) or x (cut), waiting for a p to place
	// it in a folder. Held here rather than acted on at once so you can move the
	// cursor to the destination first.
	moving *deckMove

	// fetching is the people whose decks are being fetched, so their row can
	// say so rather than looking empty.
	fetching map[string]bool
}

// deckMove is a deck picked up for a move (cut) or a copy (yank).
type deckMove struct {
	slug string
	name string
	cut  bool
}

// newDeckList reads what's on disk and what's bookmarked.
func newDeckList() *deckList {
	l := &deckList{
		order:    byModified,
		legality: map[string]deck.Legality{},
		colours:  map[string][]string{},
		expanded: map[string]bool{},
		fetching: map[string]bool{},
	}
	l.reload()
	return l
}

// localSlugs is every deck of yours in the list, for the legality pass.
func (l *deckList) localSlugs() []string {
	var out []string
	for _, e := range l.all {
		if e.kind == entryLocal {
			out = append(out, e.slug)
		}
	}
	return out
}

// setLegality files what the background pass worked out and puts it on the
// row it belongs to.
func (l *deckList) setLegality(slug string, verdict deck.Legality, colours []string) {
	if l.legality == nil {
		l.legality = map[string]deck.Legality{}
	}
	if l.colours == nil {
		l.colours = map[string][]string{}
	}
	l.legality[slug] = verdict
	l.colours[slug] = colours
	for i := range l.all {
		if l.all[i].kind == entryLocal && l.all[i].slug == slug {
			l.all[i].legal = verdict
			l.all[i].colours = colours
		}
	}
	l.refresh()
}

// setRemoteMeta fills a followed deck's row with the summary a background
// fetch worked out: its colours, its size, and when it last changed.
func (l *deckList) setRemoteMeta(id string, meta moxfield.Meta) {
	for i := range l.all {
		if l.all[i].kind == entryRemote && l.all[i].id == id {
			l.all[i].colours = meta.Colors
			l.all[i].count = meta.Count
			l.all[i].modified = meta.Updated
		}
	}
	l.refresh()
}

// reload rebuilds from disk. Every change goes through it, so the list can
// never disagree with the files behind it.
func (l *deckList) reload() {
	var all []deckEntry

	l.folders, _ = deck.Folders()

	summaries, _ := deck.Summaries()
	for _, s := range summaries {
		all = append(all, deckEntry{
			kind: entryLocal, name: s.Name, slug: s.Slug, format: s.Format,
			count: s.Total, modified: s.Modified, broken: s.Broken,
			legal: l.legality[s.Slug], colours: l.colours[s.Slug],
			folder: folderOf(s.Slug),
		})
	}

	b := deck.LoadBookmarks()
	cached := deck.LoadUserDecks()

	// A person's decks come from the cache of their public list, and sit in
	// their folder; a followed deck that's in there too is listed once, there.
	under := map[string]bool{}
	for _, u := range b.Users {
		list, _ := deck.CachedUserDecks(cached, u)
		all = append(all, deckEntry{kind: entryUser, name: u, user: u, slug: userFolder(u), count: len(list.Decks)})
		for _, d := range list.Decks {
			under[d.ID] = true
			all = append(all, deckEntry{
				kind: entryUserDeck, name: d.Name, id: d.ID, user: u,
				format: d.Format, colours: d.Colors, count: d.Cards, modified: d.Updated,
				legal: deck.Legality{Known: d.Legal, Legal: d.Legal},
			})
		}
	}
	for _, r := range b.Remotes {
		if under[r.ID] {
			continue
		}
		all = append(all, deckEntry{
			kind: entryRemote, name: r.Name, id: r.ID,
			colours: r.Colors, count: r.Count, modified: r.Updated,
		})
	}

	l.all = all
	l.refresh()
}

func (l *deckList) refresh() {
	// A filter flattens the tree: fold the folders away and show every leaf
	// that matches, wherever it lives, so a match can't hide inside a folder
	// you happen to have shut.
	if terms := filterTerms(l.filter); len(terms) > 0 {
		var kept []deckEntry
		for _, e := range l.all {
			if matchesEntry(e, terms) {
				e.depth = 0
				kept = append(kept, e)
			}
		}
		sort.SliceStable(kept, func(i, j int) bool { return lessEntry(kept[i], kept[j], l.order) })
		l.rows = kept
		l.cursor.clamp(len(l.rows))
		return
	}

	l.rows = l.buildTree()
	l.cursor.clamp(len(l.rows))
}

// buildTree lays the entries out as a folder tree flattened for display: each
// folder row, then — only when it is expanded — its subfolders and the decks
// inside it, indented one level deeper. Local decks group by the folder part of
// their slug; the followed remotes and people all sit under one virtual
// moxfield folder.
func (l *deckList) buildTree() []deckEntry {
	type leaf struct {
		e   deckEntry
		dir string
	}
	var leaves []leaf
	hasMox, hasTags := false, false
	// A person is a folder in the tree, holding their decks.
	people := map[string]deckEntry{}
	for _, e := range l.all {
		switch e.kind {
		case entryLocal:
			leaves = append(leaves, leaf{e, folderOf(e.slug)})
			if globalTags.active(e.slug) {
				ref := e
				ref.tagRef = true
				leaves = append(leaves, leaf{ref, globalTagsFolder})
				hasTags = true
			}
		case entryRemote:
			hasMox = true
			leaves = append(leaves, leaf{e, moxFolder})
		case entryUser:
			hasMox = true
			people[e.slug] = e
		case entryUserDeck:
			leaves = append(leaves, leaf{e, userFolder(e.user)})
		}
	}

	// The leaves each folder holds, and how many there are anywhere beneath it.
	leavesByDir := map[string][]deckEntry{}
	count := map[string]int{}
	for _, lf := range leaves {
		leavesByDir[lf.dir] = append(leavesByDir[lf.dir], lf.e)
		for d := lf.dir; d != ""; d = folderOf(d) {
			count[d]++
		}
	}

	// The set of folders, and each one's direct subfolders.
	subfolders := map[string][]string{}
	seen := map[string]bool{}
	addFolder := func(f string) {
		for f != "" {
			parent := folderOf(f)
			if key := parent + "\x00" + f; !seen[key] {
				seen[key] = true
				subfolders[parent] = append(subfolders[parent], f)
			}
			f = parent
		}
	}
	for _, lf := range leaves {
		addFolder(lf.dir)
	}
	// Folders on disk that hold no decks yet still get a row, so the tree
	// mirrors the directory structure rather than only the decks in it.
	for _, f := range l.folders {
		addFolder(f)
	}
	if hasMox {
		addFolder(moxFolder)
	}
	if hasTags {
		addFolder(globalTagsFolder)
	}
	for f := range people {
		addFolder(f)
	}

	var out []deckEntry
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		subs := append([]string(nil), subfolders[dir]...)
		// The virtual folders come first: moxfield, then the global tags.
		rank := func(f string) int {
			switch f {
			case moxFolder:
				return 0
			case globalTagsFolder:
				return 1
			}
			return 2
		}
		sort.Slice(subs, func(i, j int) bool {
			if rank(subs[i]) != rank(subs[j]) {
				return rank(subs[i]) < rank(subs[j])
			}
			return subs[i] < subs[j]
		})
		for _, f := range subs {
			open := l.expanded[f]
			row := deckEntry{
				kind: entryFolder, name: pathBase(f), slug: f,
				depth: depth, count: count[f], open: open,
			}
			if person, ok := people[f]; ok {
				row = person
				row.depth, row.count, row.open = depth, count[f], open
				row.loading = l.fetching[strings.ToLower(person.user)]
			}
			out = append(out, row)
			if open {
				walk(f, depth+1)
			}
		}
		ls := append([]deckEntry(nil), leavesByDir[dir]...)
		sort.SliceStable(ls, func(i, j int) bool { return lessEntry(ls[i], ls[j], l.order) })
		for _, e := range ls {
			e.depth = depth
			out = append(out, e)
		}
	}
	walk("", 0)
	return out
}

// folderOf is the folder a slug or folder path sits in — "aggro" for
// "aggro/mono-red", "" for a root deck. Slugs use forward slashes, so this is
// path.Dir with the "." it returns for a bare name folded to "".
func folderOf(slug string) string {
	if dir := path.Dir(slug); dir != "." {
		return dir
	}
	return ""
}

// pathBase is the last segment of a folder path — "mono-red" for
// "aggro/mono-red".
func pathBase(p string) string { return path.Base(p) }

func matchesEntry(e deckEntry, terms []string) bool {
	hay := strings.ToLower(e.name + " " + e.slug + " " + e.format + " " + e.user + " " + e.kind.letter())
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

func lessEntry(a, b deckEntry, s deckListSort) bool {
	switch s {
	case byName:
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	case bySize:
		return a.count > b.count
	case byKind:
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	}
	// Last touched, newest first. Things with no date — remotes and users —
	// sort to the bottom rather than claiming 1970.
	if a.modified.IsZero() != b.modified.IsZero() {
		return !a.modified.IsZero()
	}
	if !a.modified.Equal(b.modified) {
		return a.modified.After(b.modified)
	}
	return strings.ToLower(a.name) < strings.ToLower(b.name)
}

func (l *deckList) current() (deckEntry, bool) {
	if l.cursor.at < 0 || l.cursor.at >= len(l.rows) {
		return deckEntry{}, false
	}
	return l.rows[l.cursor.at], true
}

func (l *deckList) filterText() string { return l.filter }

func (l *deckList) setFilter(s string) {
	l.filter = s
	l.refresh()
}

func (l *deckList) cycleSort(delta int) {
	const n = 4
	l.order = deckListSort(((int(l.order)+delta)%n + n) % n)
	l.refresh()
}

// ── As a view ───────────────────────────────────────────────────

func (l *deckList) title() string { return "decks" }

func (l *deckList) subtitle() string {
	leaves := 0
	for _, r := range l.rows {
		if r.kind != entryFolder && r.kind != entryUser && !r.tagRef {
			leaves++
		}
	}
	out := itoa(leaves)
	if leaves != len(l.all) {
		out += "/" + itoa(len(l.all))
	}
	out += " · " + l.order.String()
	if l.filter != "" {
		out += " · /" + l.filter
	}
	// A staged deck is invisible otherwise: nothing on the row says it's about
	// to move, so the subtitle carries it until a p places it.
	if l.moving != nil {
		verb := "copy"
		if l.moving.cut {
			verb = "move"
		}
		out += " · " + verb + " " + l.moving.name
	}
	return out
}

func (l *deckList) lines(width, height int, focused bool, m *Model) []string {
	if l.confirming != nil {
		return fillTo([]string{
			lipgloss.NewStyle().Foreground(theme.Error).Bold(true).
				Render(fit("delete "+l.confirming.name+"?", width)),
			mutedLine("y to confirm · any other key cancels", width),
		}, width, height)
	}
	if len(l.rows) == 0 {
		what := "no decks yet — " + keymap.Hint(keymap.Decks, keymap.DecksNew) + " to make one"
		if l.filter != "" {
			what = "nothing matches"
		}
		return fillTo([]string{mutedLine(what, width)}, width, height)
	}

	l.cursor.scrollInto(height, len(l.rows))
	// One set of column widths for the whole list, so the kind letter and each
	// value line up down the panel however little any single row carries — a
	// user, who has only their letter, still sits it under the L and R above.
	cols := measureDeckCols(l.rows, width)
	lines := make([]string, 0, height)
	for i := l.cursor.offset; i < len(l.rows) && len(lines) < height; i++ {
		lines = append(lines, renderEntryCols(l.rows[i], cols, width, focused && i == l.cursor.at))
	}
	return fillTo(lines, width, height)
}

// clear has nothing transient to drop for esc; the filter is cleared with b.
func (l *deckList) clear() bool { return false }

// ── Drawing a row ───────────────────────────────────────────────

// deckCols are the widths the decks list gives its right-hand columns. Shared
// across the list and held fixed per row — a row without colours or a count
// still reserves the space — so every column lines up vertically. A width of
// zero means no row has that column, or the panel is too narrow to keep it.
type deckCols struct {
	pips  int
	count int
	age   int
}

// measureDeckCols works out those widths from the rows, then gives columns up
// from the right until the tail leaves room for a name — the same order the
// old per-row tail yielded in, decided once for the list so the drop is
// uniform and the columns stay aligned.
func measureDeckCols(rows []deckEntry, width int) deckCols {
	var c deckCols
	for _, e := range rows {
		if e.kind == entryFolder || e.kind == entryUser {
			continue // folders draw their own row, without these columns
		}
		c.pips = maxInt(c.pips, textWidth(manaPips(e.colours)))
		c.count = maxInt(c.count, textWidth(entryCount(e)))
		c.age = maxInt(c.age, textWidth(shortAge(e.modified)))
	}

	const minName = 12
	for deckTailWidth(c)+minName+1 > width {
		switch {
		case c.age > 0:
			c.age = 0
		case c.count > 0:
			c.count = 0
		case c.pips > 0:
			c.pips = 0
		default:
			return c
		}
	}
	return c
}

// deckTailWidth is how wide the right-hand block is: the two-character kind and
// legality flag, then a space and its width for each column still standing.
func deckTailWidth(c deckCols) int {
	w := 2 // the kind letter and the legality mark
	for _, col := range []int{c.pips, c.count, c.age} {
		if col > 0 {
			w += 1 + col
		}
	}
	return w
}

// entryCount is the size column's text: a local deck says its size even when
// that's zero — blank reads as "we haven't looked", and an empty deck is a
// fact — while a remote says nothing until we've looked, and a person never.
func entryCount(e deckEntry) string {
	if e.kind == entryLocal || e.count > 0 {
		return itoa(e.count)
	}
	return ""
}

// renderEntry draws one row with columns sized to itself, which is what the
// tests want to measure. The list draws with renderEntryCols instead, so its
// columns are shared and aligned.
func renderEntry(e deckEntry, width int, under bool) string {
	return renderEntryCols(e, measureDeckCols([]deckEntry{e}, width), width, under)
}

// renderEntryCols draws one row against a shared set of column widths: the
// name, then a right-aligned tail of kind, legality, colours, size and age.
// Each column keeps its slot whether or not this row fills it, so the columns
// line up down the list.
func renderEntryCols(e deckEntry, cols deckCols, width int, under bool) string {
	if width < 1 {
		return ""
	}

	indent := strings.Repeat("  ", e.depth)

	if e.kind == entryFolder || e.kind == entryUser {
		return renderFolderRow(e, indent, width, under)
	}

	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	// The kind letter and its legality mark travel together: two characters
	// saying what this is and whether it's playable.
	slots := []string{dim.Render(e.kind.letter()) + legalMark(e, e.legal.Flag())}
	if cols.pips > 0 {
		slots = append(slots, paintMana(padLeft(manaPips(e.colours), cols.pips)))
	}
	if cols.count > 0 {
		slots = append(slots, dim.Render(padLeft(entryCount(e), cols.count)))
	}
	if cols.age > 0 {
		slots = append(slots, dim.Render(padLeft(shortAge(e.modified), cols.age)))
	}
	right := strings.Join(slots, " ")
	tailWidth := textWidth(stripStyles(right))

	name := indent + e.name
	if e.broken {
		name += " (unreadable)"
	}

	// A flattened filter view has dropped the folder branches, so a nested
	// deck (now at depth 0) shows its folder beside the name to stay locatable.
	// A grouped deck sits under its branch (depth >= 1) and a root deck has no
	// folder, so neither gets the suffix.
	suffix := ""
	if e.depth == 0 && e.folder != "" {
		suffix = "  " + e.folder
	}
	if e.depth == 0 && e.kind == entryUserDeck {
		suffix = "  " + e.user
	}
	avail := maxInt(width-tailWidth-1, 0)
	left := fit(name, maxInt(avail-textWidth(suffix), 0))

	nameStyle := lipgloss.NewStyle().Foreground(theme.Text)
	switch {
	case e.broken:
		nameStyle = nameStyle.Foreground(theme.Error)
	case e.kind == entryRemote || e.kind == entryUserDeck:
		nameStyle = nameStyle.Foreground(theme.TextDim)
	}
	if under {
		nameStyle = nameStyle.Foreground(theme.SelectionFg).Bold(true)
	}

	// The folder hint rides in the name's style when the row is selected, so it
	// reads against the highlight; otherwise it's dim, a step back from the name.
	folderStyle := lipgloss.NewStyle().Foreground(theme.TextDim)
	if under {
		folderStyle = nameStyle
	}

	line := nameStyle.Render(left) + folderStyle.Render(suffix) + " " + right
	if under {
		return highlightLine(line, width, theme.SelectionBg)
	}
	return line
}

// renderFolderRow draws a folder: a disclosure marker, its name, and how many
// decks are under it — no legality or colour columns, since a folder has
// neither. The count sits on the right the way a deck's size does. A person
// you follow is drawn the same way, since that's what they are here: a folder
// of their decks.
func renderFolderRow(e deckEntry, indent string, width int, under bool) string {
	marker := "▸"
	if e.open {
		marker = "▾"
	}

	dim := lipgloss.NewStyle().Foreground(theme.TextDim)
	tail := itoa(e.count)
	if e.loading {
		tail = "…"
	}
	count := dim.Render(tail)
	tailWidth := textWidth(tail)

	colour := theme.Accent
	switch {
	case e.kind == entryUser:
		colour = theme.Info
	case e.slug == moxFolder:
		colour = theme.Special
	case e.slug == globalTagsFolder && e.kind == entryFolder, e.tagRef:
		colour = theme.Highlight
	}
	nameStyle := lipgloss.NewStyle().Foreground(colour).Bold(true)
	if under {
		nameStyle = nameStyle.Foreground(theme.SelectionFg)
	}
	left := fit(indent+marker+" "+e.name, maxInt(width-tailWidth-1, 0))

	line := nameStyle.Render(left) + " " + count
	if under {
		return highlightLine(line, width, theme.SelectionBg)
	}
	return line
}

// shortAge is how long ago, in one or two characters plus a unit. Empty for
// things that have no date, rather than a dash that reads as a value.
func shortAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return itoa(maxInt(int(d.Minutes()), 1)) + "m"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h"
	case d < 365*24*time.Hour:
		return itoa(int(d.Hours()/24)) + "d"
	}
	return itoa(int(d.Hours()/24/365)) + "y"
}

// key is the decks panel's own keymap. The mutations arrive in the next
// commit; this is navigation, ordering and opening.
func (l *deckList) key(k string, m *Model, p *panel) (bool, tea.Cmd) {
	// A pending deletion swallows the next key, whatever it is: a
	// confirmation you can answer by accident isn't one.
	if l.confirming != nil {
		target := *l.confirming
		l.confirming = nil
		if k == "y" {
			return true, m.deleteEntry(l, target)
		}
		return true, nil
	}

	if l.cursor.navKey(k, len(l.rows)) {
		return true, nil
	}

	switch action := keymap.Lookup(keymap.Decks, k); action {
	case keymap.DecksSortNext:
		l.cycleSort(1)
	case keymap.DecksSortPrev:
		l.cycleSort(-1)
	case keymap.DecksFilter:
		p.openFilter(l.filter)
	case keymap.DecksOpen, keymap.DecksOpenBeside:
		if e, ok := l.current(); ok && (e.kind == entryFolder || e.kind == entryUser) {
			l.toggleFolder(e.slug)
			if e.kind == entryUser && l.expanded[e.slug] {
				return true, l.fetchUserAgain(e.user)
			}
			return true, nil
		}
		return true, m.openEntry(l, p, action == keymap.DecksOpenBeside)

	case keymap.DecksNew:
		p.ask(askNewDeck, "name", "")

	case keymap.DecksRename:
		if e, ok := l.current(); ok {
			switch {
			case e.kind == entryLocal || e.kind == entryRemote:
				p.ask(askRename, "rename", e.name)
			case e.kind == entryFolder && !virtualFolder(e.slug):
				p.ask(askRename, "rename folder", e.name)
			}
		}

	case keymap.DecksCopy:
		if e, ok := l.current(); ok && e.kind != entryFolder && e.kind != entryUser {
			return true, copyEntry(e)
		}

	case keymap.DecksCopyBoth:
		// A remote's Considering list, alongside the main copy c would take.
		if e, ok := l.current(); ok && (e.kind == entryRemote || e.kind == entryUserDeck) {
			return true, copyEntryBoth(e)
		}

	// ── Moving decks between folders ─────────────────────────────
	// y picks a deck up to copy, x to move; p drops it into the folder the
	// cursor is in. Staged rather than immediate so you can navigate first.
	case keymap.DecksYank:
		if e, ok := l.current(); ok && e.kind == entryLocal {
			l.moving = &deckMove{slug: e.slug, name: e.name, cut: false}
		}
	case keymap.DecksPut:
		if l.moving != nil {
			mv := *l.moving
			l.moving = nil
			return true, m.putDeck(mv, l.currentFolder())
		}

	case keymap.DecksCut:
		if e, ok := l.current(); ok && e.kind == entryLocal {
			l.moving = &deckMove{slug: e.slug, name: e.name, cut: true}
		}

	case keymap.DecksGlobalTags:
		if e, ok := l.current(); ok && e.kind == entryLocal {
			m.togglePin(e.slug, e.name)
			l.refresh()
		}

	case keymap.DecksDelete:
		e, ok := l.current()
		if !ok || e.kind == entryFolder || e.kind == entryUserDeck {
			// A folder goes when its last deck does; a person's deck goes
			// when you stop following them.
			return true, nil
		}
		if e.kind == entryLocal {
			// A deck file is the only one of the three that loses work.
			// Following and unfollowing are free, so they just happen.
			l.confirming = &e
			return true, nil
		}
		return true, m.deleteEntry(l, e)

	default:
		return false, nil
	}
	return true, nil
}

// toggleFolder opens or shuts a folder, keeping the cursor on it so the row you
// pressed doesn't slide out from under you.
func (l *deckList) toggleFolder(slug string) {
	l.expanded[slug] = !l.expanded[slug]
	l.refresh()
	for i, r := range l.rows {
		if (r.kind == entryFolder || r.kind == entryUser) && r.slug == slug {
			l.cursor.at = i
			break
		}
	}
}

// currentFolder is where a put would land: the folder under the cursor, or the
// folder the highlighted deck sits in, or the root.
func (l *deckList) currentFolder() string {
	e, ok := l.current()
	if !ok {
		return ""
	}
	switch e.kind {
	case entryFolder:
		return e.slug
	case entryLocal:
		return folderOf(e.slug)
	case entryRemote, entryUser, entryUserDeck:
		return moxFolder
	}
	return ""
}

// info describes the highlighted row: what it is, how big, how old.
func (l *deckList) info(width int) []string {
	e, ok := l.current()
	if !ok {
		return nil
	}

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)

	out := []string{head.Render(fit(e.name, width)), ""}
	switch e.kind {
	case entryFolder:
		out = append(out, dim.Render(fit("folder", width)))
		out = append(out, dim.Render(fit(itoa(e.count)+" decks inside", width)))
		state := "enter: collapse"
		if !e.open {
			state = "enter: expand"
		}
		out = append(out, "", mutedLine(state, width))
		if !virtualFolder(e.slug) {
			out = append(out, mutedLine("p put", width))
		}
	case entryLocal:
		out = append(out, dim.Render(fit("local deck", width)))
		if globalTags.active(e.slug) {
			out = append(out, lipgloss.NewStyle().Foreground(theme.Highlight).
				Render(fit("pinned to the global tags", width)))
		}
		location := "top level"
		if e.folder != "" {
			location = "in " + e.folder
		}
		out = append(out, dim.Render(fit(location, width)))
		out = append(out, dim.Render(fit(itoa(e.count)+" cards", width)))
		if age := shortAge(e.modified); age != "" {
			out = append(out, dim.Render(fit("touched "+age+" ago", width)))
		}
		out = append(out, "")
		out = append(out, legalityLines(e.legal, width)...)
		out = append(out, "", mutedLine("enter: open", width))
		toggle := "t: pin to the global tags"
		if globalTags.active(e.slug) {
			toggle = "t: unpin from the global tags"
		}
		out = append(out, mutedLine(toggle, width))
		out = append(out, "", mutedLine("gv: versions", width))
	case entryRemote, entryUserDeck:
		out = append(out, dim.Render(fit("Moxfield", width)))
		if e.kind == entryUserDeck {
			out = append(out, dim.Render(fit("by "+e.user, width)))
		}
		if e.format != "" {
			out = append(out, dim.Render(fit(e.format, width)))
		}
		if e.count > 0 {
			out = append(out, dim.Render(fit(itoa(e.count)+" cards", width)))
		}
		if age := shortAge(e.modified); age != "" {
			out = append(out, dim.Render(fit("updated "+age+" ago", width)))
		}
		out = append(out, "",
			mutedLine("enter: look", width),
			mutedLine("c: copy deck", width),
			mutedLine("C: copy deck & considering", width))
	case entryUser:
		out = append(out, dim.Render(fit("Moxfield user", width)))
		out = append(out, dim.Render(fit(itoa(e.count)+" public decks", width)))
		state := "enter: collapse"
		if !e.open {
			state = "enter: expand"
		}
		out = append(out, "", mutedLine(state, width), mutedLine("d: unfollow", width))
	}
	return out
}

// legalityLines is the verdict and, when it is bad news, what is wrong with
// it. A deck that is merely "illegal" tells you nothing you can act on.
func legalityLines(verdict deck.Legality, width int) []string {
	style := lipgloss.NewStyle().Foreground(theme.TextMuted)
	switch {
	case verdict.Legal:
		style = lipgloss.NewStyle().Foreground(theme.Success)
	case verdict.Known:
		style = lipgloss.NewStyle().Foreground(theme.Error)
	}

	out := wrapStyled(verdict.Summary(), width, style)
	dim := lipgloss.NewStyle().Foreground(theme.TextDim)
	for _, p := range verdict.Problems {
		out = append(out, wrapStyled("· "+p.Text, width, dim)...)
		if len(p.Cards) > 0 {
			out = append(out, wrapStyled("  "+strings.Join(p.Cards, ", "), width,
				lipgloss.NewStyle().Foreground(theme.TextMuted))...)
		}
	}
	return out
}

// manaPips is a deck's colours as the letters people say them in. Five
// characters at most, which is worth the room: "is this the Mardu deck or
// the Simic one" is the question a list of deck names can't answer.
func manaPips(colours []string) string {
	if len(colours) == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range []string{"W", "U", "B", "R", "G"} {
		for _, have := range colours {
			if have == c {
				b.WriteString(c)
				break
			}
		}
	}
	return b.String()
}

func legalMark(e deckEntry, mark string) string {
	switch {
	case !e.legal.Known:
		return mark
	case e.legal.Legal:
		return lipgloss.NewStyle().Foreground(theme.Success).Render(mark)
	}
	return lipgloss.NewStyle().Foreground(theme.Error).Render(mark)
}

func (l *deckList) keys() []hintGroup {
	return []hintGroup{
		{"navigation", [][2]string{
			hint("sort", keymap.Decks, keymap.DecksSortNext, keymap.DecksSortPrev),
			hint("filter", keymap.Decks, keymap.DecksFilter),
		}},
		{"decks", [][2]string{
			hint("open/fold", keymap.Decks, keymap.DecksOpen),
			hint("open beside", keymap.Decks, keymap.DecksOpenBeside),
			hint("new", keymap.Decks, keymap.DecksNew),
			hint("rename", keymap.Decks, keymap.DecksRename),
			hint("copy deck/&considering", keymap.Decks, keymap.DecksCopy, keymap.DecksCopyBoth),
			hint("delete/cut/yank/put", keymap.Decks, keymap.DecksDelete, keymap.DecksCut, keymap.DecksYank, keymap.DecksPut),
			hint("pin to global tags", keymap.Decks, keymap.DecksGlobalTags),
			{gotoHint(keymap.GotoVersions), "versions"},
			{gotoHint(keymap.GotoImage), "open on moxfield"},
		}},
	}
}
