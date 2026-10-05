package keymap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Cleanup(Reset)
}

func write(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(Path(), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultsHaveNoConflicts(t *testing.T) {
	Reset()
	for s, actions := range bound {
		if clash := conflicts(actions); len(clash) > 0 {
			t.Errorf("%s: %v", s, clash)
		}
	}
}

func TestDefaultsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range defaults {
		id := string(b.scope) + "." + string(b.action)
		if seen[id] {
			t.Errorf("%s is listed twice", id)
		}
		seen[id] = true
	}
}

func TestMissingFileIsDefaults(t *testing.T) {
	isolate(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := Lookup(Cards, "."); got != CardsSort1Next {
		t.Fatalf(". in a card list = %q, want sort1.next", got)
	}
	if got := Lookup(Global, ","); got != "" {
		t.Fatalf(", should no longer be the leader, got %q", got)
	}
}

func TestOverrideReplacesKeys(t *testing.T) {
	isolate(t)
	write(t, `{"cards": {"sort1.next": ["z", "Z"], "sort1.prev": "ctrl+z"}}`)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := Lookup(Cards, "z"); got != CardsSort1Next {
		t.Fatalf("z = %q, want sort1.next", got)
	}
	if got := Lookup(Cards, "."); got != "" {
		t.Fatalf(". should be free once sort1.next moved, got %q", got)
	}
	if got := Lookup(Cards, "ctrl+z"); got != CardsSort1Prev {
		t.Fatalf("a single string should bind too, got %q", got)
	}
	// Untouched actions and scopes keep their defaults.
	if got := Lookup(Cards, ","); got != CardsSort2Next {
		t.Fatalf(", = %q, want sort2.next", got)
	}
	if got := Lookup(Decks, "."); got != DecksSortNext {
		t.Fatalf("decks . = %q, want sort.next", got)
	}
}

func TestSpaceByName(t *testing.T) {
	isolate(t)
	write(t, `{"global": {"leader": ["space", ";"]}}`)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Lookup(Global, " ") != GlobalLeader || Lookup(Global, ";") != GlobalLeader {
		t.Fatal("space and ; should both be the leader")
	}
}

func TestUnbinding(t *testing.T) {
	isolate(t)
	write(t, `{"cards": {"commander": []}}`)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := Lookup(Cards, "c"); got != "" {
		t.Fatalf("c = %q, want nothing", got)
	}
	if got := Hint(Cards, CardsCommander); got != "" {
		t.Fatalf("an unbound action has no hint, got %q", got)
	}
}

func TestUnknownNamesWarnAndAreIgnored(t *testing.T) {
	isolate(t)
	write(t, `{"cards": {"sort1.next": ["z"], "flip": ["f"]}, "nowhere": {"x": ["x"]}}`)
	err := Load()
	if err == nil {
		t.Fatal("unknown names should be reported")
	}
	for _, want := range []string{`no action "flip"`, `no scope "nowhere"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q doesn't mention %s", err, want)
		}
	}
	// What was good in the file still applies.
	if got := Lookup(Cards, "z"); got != CardsSort1Next {
		t.Fatalf("z = %q, want sort1.next", got)
	}
}

func TestConflictKeepsScopeDefaults(t *testing.T) {
	isolate(t)
	write(t, `{"cards": {"sort1.next": ["a"], "sort2.next": ["q"]}, "decks": {"sort.next": ["z"]}}`)
	err := Load()
	if err == nil || !strings.Contains(err.Error(), "a is on add and sort1.next") {
		t.Fatalf("want the clash named, got %v", err)
	}
	// The whole scope falls back, not just the clashing action...
	if Lookup(Cards, "a") != CardsAdd || Lookup(Cards, ".") != CardsSort1Next || Lookup(Cards, "q") != "" {
		t.Fatal("cards should be back on its defaults")
	}
	// ...and other scopes still take their changes.
	if Lookup(Decks, "z") != DecksSortNext {
		t.Fatal("the decks rebinding should still apply")
	}
}

func TestShadowingAcrossScopesIsFine(t *testing.T) {
	isolate(t)
	// o is OR in the statistics; putting a sort on it in the card list
	// is what layering is for, not a conflict.
	write(t, `{"cards": {"sort1.next": ["o"]}}`)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestBadJSON(t *testing.T) {
	isolate(t)
	write(t, `{"cards": `)
	if err := Load(); err == nil {
		t.Fatal("malformed keys.json should be reported")
	}
	if Lookup(Cards, ".") != CardsSort1Next {
		t.Fatal("defaults should stand after a malformed file")
	}
}

func TestHint(t *testing.T) {
	Reset()
	cases := []struct {
		got, want string
	}{
		{Hint(Cards, CardsSort1Next, CardsSort1Prev), ". >"},
		{Hint(Cards, CardsSort2Next, CardsSort2Prev), ", <"},
		{Hint(List, ListDown, ListUp), "j k"},
		{Hint(Global, GlobalMoveLeft, GlobalMoveRight), "ctrl+h/l"},
		{Hint(Global, GlobalLeader), "space"},
		{Hint(Search, SearchHistoryPrev, SearchHistoryNext), "↑ ↓"},
		{Hint(Search, SearchPrevTarget), "shift+tab"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("hint %q, want %q", c.got, c.want)
		}
	}
}

func TestDefaultsJSONRoundTrips(t *testing.T) {
	isolate(t)
	body := DefaultsJSON()
	var parsed map[string]map[string][]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("defaults aren't valid JSON: %v\n%s", err, body)
	}
	if parsed["global"]["leader"][0] != "space" {
		t.Fatalf("the leader should be written as space, got %q", parsed["global"]["leader"])
	}
	write(t, string(body))
	if err := Load(); err != nil {
		t.Fatalf("the defaults file should load cleanly: %v", err)
	}
	for _, b := range defaults {
		if got := strings.Join(Keys(b.scope, b.action), " "); got != strings.Join(b.keys, " ") {
			t.Errorf("%s.%s = %q after round trip, want %q", b.scope, b.action, got, b.keys)
		}
	}
}

func TestARenamedActionStillMoves(t *testing.T) {
	isolate(t)
	write(t, `{"decks": {"tag-list": "P"}}`)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := Lookup(Decks, "P"); got != DecksGlobalTags {
		t.Fatalf("tag-list, the old name, should move global-tags; P = %q", got)
	}
}
