package deck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ttr/internal/paths"
)

// Decks and people you keep an eye on without keeping a copy.
//
// A remote is a Moxfield deck you want listed but haven't taken a copy of —
// someone else's brew you're watching, or your own that lives there. A user
// is somebody whose decks you look through. Neither is a file on disk, so
// neither can be edited; both are one keystroke from becoming a local deck
// that can be.
//
// This is data rather than cache: nothing can reconstruct which decks you
// chose to follow.

const bookmarksFile = "bookmarks.json"

type Remote struct {
	// Name is what to call it in the list, and is the deck's own title
	// until you rename it.
	Name string `json:"name"`
	// ID is the Moxfield public id, which is the only part that must be
	// right — a URL can be pasted in any of several shapes.
	ID  string `json:"id"`
	URL string `json:"url,omitempty"`

	// The summary the decks list shows beside a remote without opening it —
	// its colours, its size and when it last changed on Moxfield. Filled in by
	// a background pass the first time the list is shown; Fetched records that
	// the pass has run, so an empty deck isn't fetched again on every open.
	Colors  []string  `json:"colors,omitempty"`
	Count   int       `json:"count,omitempty"`
	Updated time.Time `json:"updated,omitempty"`
	Fetched bool      `json:"fetched,omitempty"`
}

type Bookmarks struct {
	Remotes []Remote `json:"remotes,omitempty"`
	Users   []string `json:"users,omitempty"`
}

func bookmarksPath() string { return filepath.Join(paths.Data(), bookmarksFile) }

// BookmarksPath is the file of decks you follow, for ttr uninstall.
func BookmarksPath() string { return bookmarksPath() }

// LoadBookmarks reads them. A missing or unreadable file means none, which
// is what a first run looks like anyway.
func LoadBookmarks() Bookmarks {
	var b Bookmarks
	body, err := os.ReadFile(bookmarksPath())
	if err != nil {
		return b
	}
	json.Unmarshal(body, &b)
	return b
}

func SaveBookmarks(b Bookmarks) error {
	body, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bookmarksPath(), append(body, '\n'), 0644)
}

// AddRemote records a Moxfield deck, or updates the name of one already
// there. Following the same deck twice is a no-op rather than a duplicate.
func (b *Bookmarks) AddRemote(r Remote) {
	for i, existing := range b.Remotes {
		if existing.ID == r.ID {
			if r.Name != "" {
				b.Remotes[i].Name = r.Name
			}
			return
		}
	}
	b.Remotes = append(b.Remotes, r)
}

func (b *Bookmarks) RemoveRemote(id string) {
	out := b.Remotes[:0]
	for _, r := range b.Remotes {
		if r.ID != id {
			out = append(out, r)
		}
	}
	b.Remotes = out
}

// SetRemoteMeta records the summary a background fetch worked out for a
// remote, and marks it fetched so the pass leaves it alone next time. A remote
// no longer in the list is ignored, since it was unfollowed while the fetch
// was in flight.
func (b *Bookmarks) SetRemoteMeta(id string, colors []string, count int, updated time.Time) {
	for i := range b.Remotes {
		if b.Remotes[i].ID == id {
			b.Remotes[i].Colors = colors
			b.Remotes[i].Count = count
			b.Remotes[i].Updated = updated
			b.Remotes[i].Fetched = true
			return
		}
	}
}

func (b *Bookmarks) RenameRemote(id, name string) {
	for i, r := range b.Remotes {
		if r.ID == id {
			b.Remotes[i].Name = name
			return
		}
	}
}

// AddUser records somebody whose decks you look through. Names are compared
// without case, the way Moxfield treats them.
func (b *Bookmarks) AddUser(name string) {
	for _, u := range b.Users {
		if strings.EqualFold(u, name) {
			return
		}
	}
	b.Users = append(b.Users, name)
}

func (b *Bookmarks) RemoveUser(name string) {
	out := b.Users[:0]
	for _, u := range b.Users {
		if !strings.EqualFold(u, name) {
			out = append(out, u)
		}
	}
	b.Users = out
}
