package deck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ttr/internal/paths"
)

// The public decks of the people you follow, as last fetched.
//
// A person you follow shows as a folder of their decks, so the list has to be
// to hand without asking Moxfield every time the folder opens — and it has to
// be to hand for the / filter, which searches every deck in the panel whether
// its folder is open or not. It is cache rather than data: it can always be
// fetched again, and it is, once it's a day old.

const userDecksFile = "userdecks.json"

// UserDeck is one of someone's public decks — enough to list and choose by.
type UserDeck struct {
	Name    string    `json:"name"`
	ID      string    `json:"id"`
	URL     string    `json:"url,omitempty"`
	Format  string    `json:"format,omitempty"`
	Colors  []string  `json:"colors,omitempty"`
	Cards   int       `json:"cards,omitempty"`
	Legal   bool      `json:"legal,omitempty"`
	Updated time.Time `json:"updated,omitempty"`
}

// UserDeckList is one person's decks and when they were fetched.
type UserDeckList struct {
	Fetched time.Time  `json:"fetched"`
	Decks   []UserDeck `json:"decks"`
}

func userDecksPath() string { return filepath.Join(paths.Cache(), userDecksFile) }

func userKey(user string) string { return strings.ToLower(user) }

// LoadUserDecks reads every cached list, keyed by lower-cased user name. A
// missing or unreadable cache is an empty one.
func LoadUserDecks() map[string]UserDeckList {
	out := map[string]UserDeckList{}
	body, err := os.ReadFile(userDecksPath())
	if err != nil {
		return out
	}
	json.Unmarshal(body, &out)
	return out
}

// CachedUserDecks is one person's cached list, if there is one.
func CachedUserDecks(all map[string]UserDeckList, user string) (UserDeckList, bool) {
	u, ok := all[userKey(user)]
	return u, ok
}

// SaveUserDecks files a freshly fetched list.
func SaveUserDecks(user string, decks []UserDeck) error {
	all := LoadUserDecks()
	all[userKey(user)] = UserDeckList{Fetched: time.Now(), Decks: decks}
	return writeUserDecks(all)
}

// ForgetUserDecks drops someone's list, when you stop following them.
func ForgetUserDecks(user string) error {
	all := LoadUserDecks()
	if _, ok := all[userKey(user)]; !ok {
		return nil
	}
	delete(all, userKey(user))
	return writeUserDecks(all)
}

func writeUserDecks(all map[string]UserDeckList) error {
	if err := os.MkdirAll(filepath.Dir(userDecksPath()), 0755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(userDecksPath(), append(body, '\n'), 0644)
}
