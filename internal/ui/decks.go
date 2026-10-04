package ui

import (
	"fmt"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/deck"
	"ttr/internal/moxfield"
	"ttr/internal/mtg"
)

// What the decks panel actually does: opening things, and changing them.
//
// Everything that touches the network or the disk runs off the main thread
// and comes back as a message. The panel it came from is found by id, since
// the row can be reordered or closed while a deck is being resolved.

// ── Messages ────────────────────────────────────────────────────

type deckOpenedMsg struct {
	panel   int
	newPane bool
	info    deck.Info
	cards   []deck.Card
	err     error
	// uncommitted is a deck of yours whose file has changes since its last
	// commit — edits from last time, written but never committed.
	uncommitted bool
}

// userDecksFetchedMsg is a person's public decks, fetched and cached.
type userDecksFetchedMsg struct {
	user string
	err  error
}

// reloadDecksMsg tells every decks panel to read the directory again, after
// something changed it.
type reloadDecksMsg struct{}

// noticeMsg is a one-line result — "copied", "deleted" — shown until the
// next keypress.
type noticeMsg struct {
	text string
	err  error
	// moved is a deck or folder that now lives somewhere else, [from, to],
	// and renamed a deck's new name keyed by where it lives now — so a
	// deck open in a panel follows, rather than being written back to where
	// it was, under the name it had.
	moved   [2]string
	renamed [2]string
}

// ── Opening ─────────────────────────────────────────────────────

// openEntry acts on the highlighted row: a deck becomes a list of cards, a
// person becomes a list of their decks.
func (m *Model) openEntry(l *deckList, p *panel, newPane bool) tea.Cmd {
	e, ok := l.current()
	if !ok {
		return nil
	}

	target := p
	if newPane {
		// L opens to the right and leaves you where you were, so the list
		// you were browsing is still under the cursor.
		target = m.ws.open(p.kind)
		m.ws.focus(m.ws.indexOf(p))
	}
	target.loading = true
	target.err = nil

	switch e.kind {
	case entryLocal:
		target.title = e.name
		return openLocalDeck(target.id, newPane, e.slug)
	case entryRemote:
		target.title = e.name
		return openRemoteDeck(target.id, newPane, e.id, true)
	case entryUserDeck:
		// Already listed under its author, so looking doesn't follow it.
		target.title = e.name
		return openRemoteDeck(target.id, newPane, e.id, false)
	}
	target.loading = false
	return nil
}

func openLocalDeck(panelID int, newPane bool, slug string) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := deck.Open(slug)
		return deckOpenedMsg{panel: panelID, newPane: newPane, info: info, cards: cards, err: err,
			uncommitted: deck.HasUncommittedEdits(slug)}
	}
}

func openRemoteDeck(panelID int, newPane bool, id string, follow bool) tea.Cmd {
	return func() tea.Msg {
		info, cards, err := moxfield.Load(id)
		if err == nil && follow {
			// Looking at somebody's deck is how you decide to follow it, so
			// following happens by looking. It costs a line in a file, and
			// the alternative is finding your way back to a deck you saw
			// once and can't name.
			follow := deck.LoadBookmarks()
			follow.AddRemote(deck.Remote{Name: info.Name, ID: id, URL: info.URL})
			deck.SaveBookmarks(follow)
		}
		return deckOpenedMsg{panel: panelID, newPane: newPane, info: info, cards: cards, err: err}
	}
}

// fetchUserDecks fetches a person's public decks and caches them, which is
// what their folder in the decks list is drawn from.
func fetchUserDecks(user string) tea.Cmd {
	return func() tea.Msg {
		_, decks, err := moxfield.UserDecks(user)
		if err != nil {
			return userDecksFetchedMsg{user: user, err: err}
		}
		out := make([]deck.UserDeck, 0, len(decks))
		for _, d := range decks {
			out = append(out, deck.UserDeck{
				Name: deck.ImportName(d.Name), ID: d.PublicID, URL: d.PublicURL,
				Format: d.Format, Colors: d.Colors, Cards: d.Cards, Legal: d.Legal,
				Updated: d.UpdatedAt(),
			})
		}
		return userDecksFetchedMsg{user: user, err: deck.SaveUserDecks(user, out)}
	}
}

// fetchUserIfStale fetches a person's decks when their folder opens, unless
// the cached list is fresh enough to go on with.
func (l *deckList) fetchUserIfStale(user string) tea.Cmd {
	if list, ok := deck.CachedUserDecks(deck.LoadUserDecks(), user); ok && !list.Stale() {
		return nil
	}
	if l.fetching[strings.ToLower(user)] {
		return nil
	}
	l.fetching[strings.ToLower(user)] = true
	l.refresh()
	return fetchUserDecks(user)
}

func (m Model) handleUserDecksFetched(msg userDecksFetchedMsg) (tea.Model, tea.Cmd) {
	for _, p := range m.ws.panels {
		for _, v := range p.stack {
			if l, ok := v.(*deckList); ok {
				delete(l.fetching, strings.ToLower(msg.user))
			}
		}
	}
	if msg.err != nil {
		m.notice = "couldn't fetch " + msg.user + "'s decks: " + msg.err.Error()
	}
	return m, reloadDecks
}

// handleDeckOpened files a resolved deck.
func (m Model) handleDeckOpened(msg deckOpenedMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil // the panel closed while the cards were resolving
	}
	p.loading = false

	// A deck whose cards mostly resolved is still worth showing; the error
	// rides alongside as a notice rather than instead of the deck.
	if msg.err != nil && len(msg.cards) == 0 {
		p.err = msg.err
		return m, nil
	}
	if msg.err != nil {
		m.notice = msg.err.Error()
	}

	l := newCardList(msg.cards, firstSort(), "decklist")
	l.name = msg.info.Name
	l.deck = &msg.info
	l.dirty = msg.uncommitted
	l.recheck()
	if p.pending != nil {
		p.pending.applyLayout(l)
		p.pending = nil
	}

	// A deck opened in its own panel replaces what was there; one opened
	// from the decks list steps into it, so esc goes back to the list.
	if msg.newPane || p.top() == nil {
		p.show(l)
	} else {
		p.push(l)
		p.title = l.name
	}
	m.ws.deriveEditingIfUnpinned()
	// A remote just followed should appear in any decks list on screen.
	if !msg.info.Local() {
		return m, reloadDecks
	}
	return m, tea.Batch(reloadDecks, refreshDeckCards(p.id, msg.info.Slug))
}

// deckRefreshedMsg is the cards of a deck that were more than a day old in
// the cache, asked for again: the new card by the id of the one it replaces.
type deckRefreshedMsg struct {
	panel int
	cards map[string]mtg.Card
}

// refreshDeckCards asks again, behind the deck that has just opened, for the
// cards whose prices are over a day old. Quiet: offline, the old prices stay.
func refreshDeckCards(panelID int, slug string) tea.Cmd {
	return func() tea.Msg {
		cards, err := deck.RefreshStale(slug)
		if err != nil || len(cards) == 0 {
			return nil
		}
		return deckRefreshedMsg{panel: panelID, cards: cards}
	}
}

// handleDeckRefreshed swaps the refreshed cards in. Only the card data
// changes, not the deck: counts, tags and edits made meanwhile stay, so it is
// no edit, with nothing to undo or save.
func (m Model) handleDeckRefreshed(msg deckRefreshedMsg) (tea.Model, tea.Cmd) {
	p := m.ws.byID(msg.panel)
	if p == nil {
		return m, nil
	}
	l := p.cardsView()
	if l == nil {
		return m, nil
	}
	changed := false
	for i := range l.all {
		if c, ok := msg.cards[l.all[i].Card.ID]; ok {
			l.all[i].Card = c
			changed = true
		}
	}
	if changed {
		l.refresh()
	}
	return m, nil
}

// ── Changing things ─────────────────────────────────────────────

// deleteEntry removes whatever the row stands for: a deck file, a remote
// link, a person. Only the first of those loses anything that can't be got
// back, which is why only that one asks first.
func (m *Model) deleteEntry(l *deckList, e deckEntry) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			if err := deck.DeleteCommitted(e.slug); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "deleted " + e.name}
		case entryRemote:
			b := deck.LoadBookmarks()
			b.RemoveRemote(e.id)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "stopped following " + e.name}
		case entryUser:
			b := deck.LoadBookmarks()
			b.RemoveUser(e.user)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			deck.ForgetUserDecks(e.user)
			return noticeMsg{text: "stopped following " + e.user}
		}
		return nil
	}
}

// copyEntry duplicates a local deck, or takes a local copy of a remote one.
// A remote whose copy already exists is synced instead — overwritten, and
// the change recorded in git, which is what makes overwriting safe.
func copyEntry(e deckEntry) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			d, err := deck.Read(e.slug)
			if err != nil {
				return noticeMsg{err: err}
			}
			slug, copied, err := deck.New(uniqueName(d.Name+" copy"), d.Format)
			if err != nil {
				return noticeMsg{err: err}
			}
			copied.Entries = d.Entries
			if _, _, err := deck.SaveVersioned(slug, copied); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "copied to " + slug}

		case entryRemote, entryUserDeck:
			d, err := moxfield.Import(e.id)
			if err != nil {
				return noticeMsg{err: err}
			}
			slug := deck.Slugify(d.Name)
			existed := deck.Exists(slug)
			subject, _, err := deck.SaveVersioned(slug, d)
			if err != nil {
				return noticeMsg{err: err}
			}
			if existed {
				if subject == "" {
					return noticeMsg{text: slug + " is already up to date"}
				}
				return noticeMsg{text: "synced " + slug + ": " + subject}
			}
			return noticeMsg{text: "copied to " + slug}
		}
		return noticeMsg{text: "nothing to copy"}
	}
}

// copyEntryBoth takes a remote deck's main list and its Considering list as
// two separate local decks — c makes one copy, C makes both. The Considering
// list is where an author keeps the cards they're weighing, and it's often
// the more interesting half to borrow.
func copyEntryBoth(e deckEntry) tea.Cmd {
	return func() tea.Msg {
		if e.kind != entryRemote && e.kind != entryUserDeck {
			return noticeMsg{text: "only a Moxfield deck has a considering list"}
		}

		main, err := moxfield.Import(e.id)
		if err != nil {
			return noticeMsg{err: err}
		}
		mainSlug := deck.Slugify(main.Name)
		if _, _, err := deck.SaveVersioned(mainSlug, main); err != nil {
			return noticeMsg{err: err}
		}

		cons, err := moxfield.ImportConsidering(e.id)
		if err != nil {
			return noticeMsg{err: err}
		}
		if cons == nil {
			return noticeMsg{text: "copied to " + mainSlug + " · nothing being considered"}
		}
		consSlug := deck.Slugify(cons.Name)
		if _, _, err := deck.SaveVersioned(consSlug, cons); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "copied to " + mainSlug + " and " + consSlug}
	}
}

// uniqueName finds a name whose slug isn't taken, so copying twice gives you
// two decks rather than an error.
func uniqueName(base string) string {
	if !deck.Exists(deck.Slugify(base)) {
		return base
	}
	for i := 2; i < 100; i++ {
		name := fmt.Sprintf("%s %d", base, i)
		if !deck.Exists(deck.Slugify(name)) {
			return name
		}
	}
	return base
}

// newDeck makes an empty one, in the folder the decks panel was pointing at. A
// deck you've just created is empty by definition, and you fill it by adding
// cards to it.
func newDeckCmd(dir, name string) tea.Cmd {
	return func() tea.Msg {
		slug, d, err := deck.New(inFolder(dir, name), deck.DefaultFormat)
		if err != nil {
			return noticeMsg{err: err}
		}
		if _, _, err := deck.SaveVersioned(slug, d); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "made " + slug}
	}
}

// newFolder makes an empty directory in the tree, under the folder the panel is
// pointing at. Typing "dirname/" at the new prompt lands here — a folder made
// before it has any decks, so you can file decks into it afterwards.
func newFolderCmd(dir, name string) tea.Cmd {
	return func() tea.Msg {
		slug, err := deck.NewFolder(inFolder(dir, name))
		if err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "made folder " + slug}
	}
}

// inFolder puts a name inside a folder, for slugifying into a path. A name that
// already carries its own slashes nests under the folder all the same.
func inFolder(dir, name string) string {
	if dir == "" || virtualFolder(dir) {
		return name
	}
	return dir + "/" + name
}

// putDeck places a staged deck into a folder: a cut moves it, a yank copies it.
// The moxfield folder is virtual, so nothing local can be put there.
func (m *Model) putDeck(mv deckMove, dir string) tea.Cmd {
	if dir == moxFolder {
		return func() tea.Msg {
			return noticeMsg{err: fmt.Errorf("the moxfield folder is for followed decks, not yours")}
		}
	}
	if dir == tagListsFolder {
		return func() tea.Msg {
			return noticeMsg{err: fmt.Errorf("the tag lists folder shows the tag lists that are on — t turns one on")}
		}
	}
	dest := uniqueSlug(inFolder(dir, path.Base(mv.slug)))
	// Only the file's location changes; the deck keeps its own name, which no
	// longer carries the folder. The destination folder is where it now lives.
	where := dir
	if where == "" {
		where = "the top level"
	}

	if mv.cut {
		return func() tea.Msg {
			if dest == mv.slug {
				return noticeMsg{text: mv.name + " is already there"}
			}
			if err := deck.Move(mv.slug, dest); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "moved " + mv.name + " to " + where, moved: [2]string{mv.slug, dest}}
		}
	}
	return func() tea.Msg {
		d, err := deck.Read(mv.slug)
		if err != nil {
			return noticeMsg{err: err}
		}
		if _, _, err := deck.SaveVersioned(dest, d); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "copied " + mv.name + " to " + where}
	}
}

// uniqueSlug finds a free slug at or beside the one asked for, so putting a
// deck where one already lives makes a second rather than an error.
func uniqueSlug(slug string) string {
	if !deck.Exists(slug) {
		return slug
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", slug, i)
		if !deck.Exists(candidate) {
			return candidate
		}
	}
	return slug
}

// renameDeck changes a local deck's title, or what a remote is called in your
// list. A name with a slash in it moves the deck too: "dirname/deckname" puts
// it in dirname — beside it, in the folder it's in now, made if it isn't
// there — as deckname.deck, called deckname. A leading slash means the top
// level. The move is a git mv, so the history follows the file.
func renameCmd(e deckEntry, name string) tea.Cmd {
	return func() tea.Msg {
		switch e.kind {
		case entryLocal:
			d, err := deck.Read(e.slug)
			if err != nil {
				return noticeMsg{err: err}
			}
			name = strings.TrimSpace(name)
			slug := e.slug
			var moved [2]string
			if strings.Contains(name, "/") {
				dest, base, err := renameDest(e.slug, name)
				if err != nil {
					return noticeMsg{err: err}
				}
				if dest != e.slug {
					if err := deck.Move(e.slug, dest); err != nil {
						return noticeMsg{err: err}
					}
					moved = [2]string{e.slug, dest}
				}
				slug, name = dest, base
			}
			d.Name = name
			if _, _, err := deck.SaveVersioned(slug, d); err != nil {
				return noticeMsg{err: err}
			}
			text := "renamed to " + name
			if moved[1] != "" {
				text = "moved to " + slug
			}
			return noticeMsg{text: text, moved: moved, renamed: [2]string{slug, name}}
		case entryRemote:
			b := deck.LoadBookmarks()
			b.RenameRemote(e.id, name)
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "renamed to " + name}
		}
		return noticeMsg{text: "a person can't be renamed"}
	}
}

// renameDest is where "dirname/deckname" puts a deck that lives at slug, and
// the name it ends up with.
func renameDest(slug, name string) (dest, base string, err error) {
	top := strings.HasPrefix(name, "/")
	name = strings.Trim(name, "/ ")
	base = strings.TrimSpace(path.Base(name))
	if name == "" || base == "" || base == "." {
		return "", "", fmt.Errorf("a deck needs a name")
	}
	dir := folderOf(slug)
	if top {
		dir = ""
	}
	if sub := path.Dir(name); sub != "." {
		dir = inFolder(dir, sub)
	}
	dest = deck.Slugify(inFolder(dir, base))
	if dest == slug {
		return dest, base, nil
	}
	return uniqueSlug(dest), base, nil
}

// renameFolderCmd renames a folder — the folder is a location on disk, so this
// moves every deck under it to the renamed path, each keeping its own name. The
// moxfield folder is virtual, built from the bookmarks, so there's nothing to
// rename there.
func renameFolderCmd(e deckEntry, name string) tea.Cmd {
	return func() tea.Msg {
		if virtualFolder(e.slug) {
			return noticeMsg{err: fmt.Errorf("the %s folder isn't yours to rename", e.slug)}
		}
		newPath := inFolder(folderOf(e.slug), deck.Slugify(strings.TrimSpace(name)))
		if newPath == "" {
			return noticeMsg{err: fmt.Errorf("a folder needs a name")}
		}
		if newPath == e.slug {
			return noticeMsg{text: e.slug + " is already called that"}
		}
		if err := deck.MoveFolder(e.slug, newPath); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "renamed folder to " + newPath, moved: [2]string{e.slug, newPath}}
	}
}

// follow records whatever was pasted into the decks panel's search bar: a
// Moxfield deck URL or id becomes a remote, anything else is taken for a
// username.
func follow(input string) tea.Cmd {
	return func() tea.Msg {
		b := deck.LoadBookmarks()

		if id, ok := moxfield.Ref(input); ok {
			name := id
			if d, err := moxfield.Fetch(id); err == nil && d.Name != "" {
				name = d.Name
			}
			b.AddRemote(deck.Remote{Name: name, ID: id, URL: input})
			if err := deck.SaveBookmarks(b); err != nil {
				return noticeMsg{err: err}
			}
			return noticeMsg{text: "following " + name}
		}

		user, ok := moxfield.User(input)
		if !ok {
			return noticeMsg{err: fmt.Errorf("not a Moxfield deck or username: %q", input)}
		}
		b.AddUser(user)
		if err := deck.SaveBookmarks(b); err != nil {
			return noticeMsg{err: err}
		}
		// Their decks are fetched now, so the folder has something in it and
		// the / filter can find them before it's ever been opened.
		if msg := fetchUserDecks(user)().(userDecksFetchedMsg); msg.err != nil {
			return noticeMsg{text: "following " + user + " — couldn't fetch their decks: " + msg.err.Error()}
		}
		return noticeMsg{text: "following " + user}
	}
}

// syncDecks mirrors the decks directory to its git remote — the s key in the
// decks panel. Setting the remote up is a terminal job (`ttr sync init`); this
// is only the recurring push-and-pull, so the whole of it is one background
// call whose result becomes a notice. The reload that follows any notice picks
// up whatever a pull brought in.
func syncDecks() tea.Msg {
	if !deck.SyncConfigured() {
		return noticeMsg{err: fmt.Errorf("syncing isn't set up — run `ttr sync remote <url>` in your shell")}
	}
	res, err := deck.Sync()
	if err != nil {
		return noticeMsg{err: err}
	}
	return noticeMsg{text: "sync: " + res.Summary()}
}

// reloadDecks tells every decks panel to read the directory again.
func reloadDecks() tea.Msg { return reloadDecksMsg{} }

// loadDecks is the background work a freshly opened decks list wants doing:
// the legality of the local decks, from the cache, and the summary of the
// followed ones, from Moxfield. Both fill in beside the rows as they arrive.
func loadDecks(l *deckList) tea.Cmd {
	return tea.Batch(checkLegality(l.localSlugs()), refreshRemotes())
}

// ── Legality ────────────────────────────────────────────────────

// legalityMsg carries the verdict on one deck back to the lists showing it,
// and its colours, which come out of the same resolution.
type legalityMsg struct {
	slug     string
	legality deck.Legality
	colours  []string
}

// checkLegality works out whether each local deck is legal, from the card
// cache alone. One command per deck rather than one for all of them, so the
// first answers appear while the rest are still being worked out.
func checkLegality(slugs []string) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(slugs))
	for _, slug := range slugs {
		s := slug
		cmds = append(cmds, func() tea.Msg {
			verdict, colours := deck.CheckCached(s)
			return legalityMsg{slug: s, legality: verdict, colours: colours}
		})
	}
	return tea.Batch(cmds...)
}

// ── Remote summaries ────────────────────────────────────────────

// remoteMetaMsg carries a followed deck's summary back to the lists showing
// it — its colours, size and age, worked out without opening it.
type remoteMetaMsg struct {
	id   string
	meta moxfield.Meta
}

// refreshRemotes fetches the summary of every followed deck that hasn't been
// looked at yet, so the decks list can show a remote's colours, size and age
// beside its name. Only the un-fetched ones, and one request each, so a list
// of remotes fills in over a moment rather than blocking on all of them —
// and a second visit to the panel costs nothing.
func refreshRemotes() tea.Cmd {
	b := deck.LoadBookmarks()
	var cmds []tea.Cmd
	for _, r := range b.Remotes {
		if r.Fetched {
			continue
		}
		id := r.ID
		cmds = append(cmds, func() tea.Msg {
			meta, err := moxfield.FetchMeta(id)
			if err != nil {
				return nil // a remote we can't reach keeps just its name
			}
			return remoteMetaMsg{id: id, meta: meta}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m Model) handleRemoteMeta(msg remoteMetaMsg) (tea.Model, tea.Cmd) {
	// Keep it for next time, so the fetch happens once rather than on every
	// visit to the decks panel.
	b := deck.LoadBookmarks()
	b.SetRemoteMeta(msg.id, msg.meta.Colors, msg.meta.Count, msg.meta.Updated)
	deck.SaveBookmarks(b)

	for _, p := range m.ws.panels {
		if l, ok := p.top().(*deckList); ok {
			l.setRemoteMeta(msg.id, msg.meta)
		}
	}
	return m, nil
}

func (m Model) handleLegality(msg legalityMsg) (tea.Model, tea.Cmd) {
	for _, p := range m.ws.panels {
		if l, ok := p.top().(*deckList); ok {
			l.setLegality(msg.slug, msg.legality, msg.colours)
		}
		if l := p.cardsView(); l != nil && l.deck != nil && l.deck.Slug == msg.slug {
			legality := msg.legality
			l.legality = &legality
		}
	}
	return m, nil
}
