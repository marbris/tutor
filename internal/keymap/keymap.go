// Package keymap is every rebindable key in the program, and the file that
// rebinds them.
//
// The views don't switch on keys; they switch on actions, and ask this
// package which action a key names where they are. The hints ask it the
// other way round — which keys an action is on — so a key moved in
// keys.json moves in the hints with it. One table, read in both directions,
// is what keeps the hints from lying.
//
// Keys are grouped into scopes, one for each place a key means something:
// the same letter is rename in the decks panel and remove in a card list,
// and neither is a conflict. Two actions on one key within a scope are.
//
// A few keys are not here on purpose. ctrl+c always quits. The keys inside a
// text field — a prompt, the / filter, the quit question's y and w — are
// typing, not commands.
package keymap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ttr/internal/jsonc"
	"ttr/internal/paths"
)

// Scope is a place keys mean something.
type Scope string

// Action is something a key does, named within its scope. The name is what
// keys.json calls it.
type Action string

const (
	Global   Scope = "global"   // the workspace, whatever panel is focused
	List     Scope = "list"     // moving through any list
	Cards    Scope = "cards"    // a card list: a search or a deck
	Decks    Scope = "decks"    // the decks panel
	Rules    Scope = "rules"    // the rules panel
	Versions Scope = "versions" // a deck's git versions
	Settings Scope = "settings" // the settings panel
	Stats    Scope = "stats"    // the statistics, while they're up
	Leader   Scope = "leader"   // after the leader
	Goto     Scope = "goto"     // after g
	TagMove  Scope = "tagmove"  // after T: moving tags between lists
	Search   Scope = "search"   // a search bar, which is a text field first
)

const (
	GlobalLeader    Action = "leader"
	GlobalGoto      Action = "goto"
	GlobalPanelPrev Action = "panel.prev"
	GlobalPanelNext Action = "panel.next"
	GlobalMoveLeft  Action = "panel.move-left"
	GlobalMoveRight Action = "panel.move-right"
	GlobalBar       Action = "bar"
	GlobalEditNext  Action = "editing.next"
	GlobalEditPrev  Action = "editing.prev"
	GlobalInfoUp    Action = "info.half-page-up"
	GlobalInfoDown  Action = "info.half-page-down"
	GlobalStats     Action = "stats"
	GlobalStatsEdit Action = "stats.editing"
	// H and L only do anything while the printing view is up.
	GlobalPrintingOlder Action = "printing.older"
	GlobalPrintingNewer Action = "printing.newer"
	GlobalPrintingFace  Action = "printing.face"
	GlobalHelp          Action = "help"
	GlobalBack          Action = "back"
	GlobalClearFilter   Action = "clear-filters"
	GlobalClearAll      Action = "clear-all-filters"
	GlobalQuit          Action = "quit"

	ListDown   Action = "down"
	ListUp     Action = "up"
	ListBottom Action = "bottom"

	CardsSort1Next Action = "sort1.next"
	CardsSort1Prev Action = "sort1.prev"
	CardsSort2Next Action = "sort2.next"
	CardsSort2Prev Action = "sort2.prev"
	CardsSort1Dir  Action = "sort1.direction"
	CardsSort2Dir  Action = "sort2.direction"
	CardsSelect    Action = "select"
	CardsSelectAll Action = "select-all"
	CardsFilter    Action = "filter"
	CardsAdd       Action = "add"
	CardsAddTagged Action = "add-tagged"
	CardsRemove    Action = "remove"
	CardsYank      Action = "yank"
	CardsPut       Action = "put"
	CardsTag       Action = "tag"
	CardsCommander Action = "commander"
	CardsUndo      Action = "undo"
	CardsWrite     Action = "write"
	CardsWriteNew  Action = "write-new"
	CardsTagMove   Action = "tag-move"

	DecksSortNext   Action = "sort.next"
	DecksSortPrev   Action = "sort.prev"
	DecksFilter     Action = "filter"
	DecksOpen       Action = "open"
	DecksOpenBeside Action = "open-beside"
	DecksNew        Action = "new"
	DecksRename     Action = "rename"
	DecksCopy       Action = "copy"
	DecksCopyBoth   Action = "copy-with-considering"
	DecksYank       Action = "yank"
	DecksCut        Action = "cut"
	DecksPut        Action = "put"
	DecksDelete     Action = "delete"
	DecksGlobalTags Action = "global-tags"

	RulesOrderNext Action = "order.next"
	RulesOrderPrev Action = "order.prev"
	RulesFilter    Action = "filter"
	RulesSync      Action = "sync"

	VersionsFilter Action = "filter"
	VersionsRevert Action = "revert"
	VersionsCopy   Action = "copy"

	StatsDown      Action = "down"
	StatsUp        Action = "up"
	StatsNextGroup Action = "group.next"
	StatsPrevGroup Action = "group.prev"
	StatsAnd       Action = "and"
	StatsOr        Action = "or"
	StatsNot       Action = "not"
	StatsDrop      Action = "drop"
	StatsClear     Action = "clear"
	StatsOddsNext  Action = "odds.next"
	StatsOddsPrev  Action = "odds.prev"
	StatsClose     Action = "close"
	StatsBack      Action = "back"
	SettingsChange Action = "change"
	SettingsClear  Action = "clear"

	StatsTagOrder Action = "tag.order"
	StatsExpand   Action = "expand"

	LeaderFind      Action = "find"
	LeaderDecks     Action = "decks"
	LeaderRules     Action = "rules"
	LeaderNew       Action = "new"
	LeaderSync      Action = "sync"
	LeaderCommitAll Action = "commit-all"
	LeaderClose     Action = "close"
	LeaderSettings  Action = "settings"
	LeaderUndoClose Action = "undo-close"
	LeaderOnly      Action = "only"
	LeaderLendTags  Action = "lend-tags"

	GotoTop      Action = "top"
	GotoEditing  Action = "editing"
	GotoVersions Action = "versions"
	GotoImage    Action = "image"
	GotoImageAll Action = "image.all"

	TagMoveJoin   Action = "join"
	TagMoveUpsert Action = "upsert"
	TagMoveGlobal Action = "global"
	TagMoveMerge  Action = "merge"

	SearchNextTarget  Action = "target.next"
	SearchPrevTarget  Action = "target.prev"
	SearchRun         Action = "run"
	SearchBack        Action = "back"
	SearchHistoryPrev Action = "history.prev"
	SearchHistoryNext Action = "history.next"
	SearchQuerySort   Action = "query-sort"
	SearchQueryDir    Action = "query-dir"
)

type binding struct {
	scope  Scope
	action Action
	keys   []string
}

// defaults is the keymap as it ships, in the order `ttr keys` lists it.
var defaults = []binding{
	{Global, GlobalLeader, k(" ")},
	{Global, GlobalGoto, k("g")},
	{Global, GlobalPanelPrev, k("h", "left")},
	{Global, GlobalPanelNext, k("l", "right")},
	{Global, GlobalMoveLeft, k("ctrl+h", "ctrl+left")},
	{Global, GlobalMoveRight, k("ctrl+l", "ctrl+right")},
	{Global, GlobalBar, k("i")},
	{Global, GlobalEditNext, k("e")},
	{Global, GlobalEditPrev, k("E")},
	{Global, GlobalInfoUp, k("K", "shift+up")},
	{Global, GlobalInfoDown, k("J", "shift+down")},
	{Global, GlobalPrintingOlder, k("H")},
	{Global, GlobalPrintingNewer, k("L")},
	{Global, GlobalPrintingFace, k("f")},
	{Global, GlobalStats, k("s")},
	{Global, GlobalStatsEdit, k("S")},
	{Global, GlobalHelp, k("?")},
	{Global, GlobalBack, k("esc")},
	{Global, GlobalClearFilter, k("b")},
	{Global, GlobalClearAll, k("B")},
	{Global, GlobalQuit, k("q")},

	{List, ListDown, k("j", "down")},
	{List, ListUp, k("k", "up")},
	{List, ListBottom, k("G")},

	// The primary sort decides the right-hand column, so it is on the
	// right-hand key; the secondary colours the names on the left.
	{Cards, CardsSort1Next, k(".")},
	{Cards, CardsSort1Prev, k(">")},
	{Cards, CardsSort2Next, k(",")},
	{Cards, CardsSort2Prev, k("<")},
	// ctrl+. would be the natural key, but a terminal sends it as a plain
	// dot; alt reaches the program as itself.
	{Cards, CardsSort1Dir, k("alt+.")},
	{Cards, CardsSort2Dir, k("alt+,")},
	{Cards, CardsFilter, k("/")},
	{Cards, CardsSelect, k("v")},
	{Cards, CardsSelectAll, k("V")},
	{Cards, CardsAdd, k("a")},
	{Cards, CardsAddTagged, k("A")},
	{Cards, CardsRemove, k("x")},
	{Cards, CardsYank, k("y")},
	{Cards, CardsPut, k("p")},
	{Cards, CardsTag, k("t")},
	{Cards, CardsCommander, k("c")},
	{Cards, CardsUndo, k("u")},
	{Cards, CardsWrite, k("w")},
	{Cards, CardsWriteNew, k("W")},
	// T is a prefix: the tags of a whole list at once, moved between lists.
	{Cards, CardsTagMove, k("T")},

	{Decks, DecksSortNext, k(".")},
	{Decks, DecksSortPrev, k(">")},
	{Decks, DecksFilter, k("/")},
	{Decks, DecksOpen, k("enter")},
	{Decks, DecksOpenBeside, k("L")},
	{Decks, DecksNew, k("n")},
	{Decks, DecksRename, k("r")},
	{Decks, DecksCopy, k("c")},
	{Decks, DecksCopyBoth, k("C")},
	{Decks, DecksDelete, k("d")},
	{Decks, DecksCut, k("x")},
	{Decks, DecksYank, k("y")},
	{Decks, DecksPut, k("p")},
	{Decks, DecksGlobalTags, k("t")},

	{Rules, RulesOrderNext, k(".")},
	{Rules, RulesOrderPrev, k(">")},
	{Rules, RulesFilter, k("/")},
	{Rules, RulesSync, k("s")},

	{Versions, VersionsFilter, k("/")},
	{Versions, VersionsRevert, k("r")},
	{Versions, VersionsCopy, k("c")},

	{Settings, SettingsChange, k("enter")},
	{Settings, SettingsClear, k("d")},

	{Stats, StatsDown, k("j", "down")},
	{Stats, StatsUp, k("k", "up")},
	{Stats, StatsNextGroup, k("J", "shift+down")},
	{Stats, StatsPrevGroup, k("K", "shift+up")},
	{Stats, StatsAnd, k("a")},
	{Stats, StatsOr, k("o")},
	{Stats, StatsNot, k("n")},
	{Stats, StatsDrop, k("x")},
	{Stats, StatsClear, k("X")},
	{Stats, StatsOddsNext, k("p")},
	{Stats, StatsOddsPrev, k("P")},
	{Stats, StatsClose, k("s")},
	{Stats, StatsBack, k("esc")},
	{Stats, StatsTagOrder, k("tab")},
	{Stats, StatsExpand, k("enter")},

	{Leader, LeaderFind, k("f")},
	{Leader, LeaderDecks, k("d")},
	{Leader, LeaderRules, k("r")},
	{Leader, LeaderNew, k("n")},
	{Leader, LeaderSync, k("s")},
	{Leader, LeaderCommitAll, k("w")},
	{Leader, LeaderClose, k("x")},
	{Leader, LeaderSettings, k("c")},
	{Leader, LeaderUndoClose, k("u")},
	{Leader, LeaderOnly, k("o")},
	{Leader, LeaderLendTags, k("t")},

	{Goto, GotoTop, k("g")},
	{Goto, GotoEditing, k("d")},
	{Goto, GotoVersions, k("v")},
	{Goto, GotoImage, k("x")},
	{Goto, GotoImageAll, k("X")},

	// After T, the second key is the verb it echoes: t tags only what's
	// there, a adds what isn't.
	{TagMove, TagMoveJoin, k("t")},
	{TagMove, TagMoveUpsert, k("a")},
	{TagMove, TagMoveGlobal, k("g")},
	{TagMove, TagMoveMerge, k("m")},

	{Search, SearchNextTarget, k("tab")},
	{Search, SearchPrevTarget, k("shift+tab")},
	{Search, SearchHistoryPrev, k("up")},
	{Search, SearchHistoryNext, k("down")},
	{Search, SearchQuerySort, k("ctrl+o")},
	// Not ctrl+i, which a terminal sends as the same byte as tab.
	{Search, SearchQueryDir, k("ctrl+r")},
	{Search, SearchRun, k("enter")},
	{Search, SearchBack, k("esc")},
}

func k(keys ...string) []string { return keys }

// renamed is actions known by an older name, so a keys.json written before
// the rename still moves them.
var renamed = map[Scope]map[string]Action{
	Decks: {"tag-list": DecksGlobalTags},
}

// The keymap in force: each scope's actions and their keys, and the same
// read backwards.
var (
	bound map[Scope]map[Action][]string
	byKey map[Scope]map[string]Action
)

func init() { Reset() }

// Reset puts the shipped keymap back in force.
func Reset() {
	bound = map[Scope]map[Action][]string{}
	for _, b := range defaults {
		if bound[b.scope] == nil {
			bound[b.scope] = map[Action][]string{}
		}
		bound[b.scope][b.action] = b.keys
	}
	index()
}

func index() {
	byKey = map[Scope]map[string]Action{}
	for s, actions := range bound {
		byKey[s] = map[string]Action{}
		for a, keys := range actions {
			for _, key := range keys {
				byKey[s][key] = a
			}
		}
	}
}

// Lookup is the action a key names in a scope, or "" where it names none.
func Lookup(s Scope, key string) Action { return byKey[s][key] }

// Keys is every key an action is on, the first being the one hints show.
// None at all means it has been unbound.
func Keys(s Scope, a Action) []string { return bound[s][a] }

// Hint shows the keys for a few actions the way the hint bar writes them:
// each action's first key, "j k". Keys sharing a modifier share it once,
// "ctrl+k/j". An action with no key is left out, and "" means none had one.
func Hint(s Scope, actions ...Action) string {
	var shown []string
	for _, a := range actions {
		if keys := Keys(s, a); len(keys) > 0 {
			shown = append(shown, Display(keys[0]))
		}
	}
	if len(shown) > 1 {
		if i := strings.LastIndex(shown[0], "+"); i > 0 {
			prefix := shown[0][:i+1]
			rest := []string{shown[0][i+1:]}
			for _, k := range shown[1:] {
				if !strings.HasPrefix(k, prefix) || strings.Contains(k[len(prefix):], "+") {
					rest = nil
					break
				}
				rest = append(rest, k[len(prefix):])
			}
			if rest != nil {
				return prefix + strings.Join(rest, "/")
			}
		}
	}
	return strings.Join(shown, " ")
}

// Display is how a key is written for a person to read.
func Display(key string) string {
	switch key {
	case " ":
		return "space"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	}
	return key
}

// Path is where your rebindings live.
func Path() string { return filepath.Join(paths.Config(), "keys.json") }

// Load puts keys.json in force over the defaults. Like a broken theme, a
// broken keymap is worth a warning but not worth refusing to start over:
// whatever can't be used is reported, and the defaults stand in for it.
func Load() error {
	Reset()
	body, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// A file of nothing but comments is the template `ttr init` writes,
	// with nothing uncommented yet: the defaults, as they are.
	if jsonc.Empty(body) {
		return nil
	}
	return apply(jsonc.Strip(body))
}

// apply lays a keys.json over the defaults, one scope at a time.
func apply(body []byte) error {
	var file map[string]map[string]json.RawMessage
	if err := json.Unmarshal(body, &file); err != nil {
		return fmt.Errorf("%s: %w", Path(), err)
	}

	var problems []string
	for _, scope := range sortedKeys(file) {
		s := Scope(scope)
		actions, ok := bound[s]
		if !ok {
			problems = append(problems, fmt.Sprintf("no scope %q", scope))
			continue
		}

		next := map[Action][]string{}
		for a, keys := range actions {
			next[a] = keys
		}
		for _, name := range sortedKeys(file[scope]) {
			a := Action(name)
			if now, ok := renamed[s][name]; ok {
				a = now
			}
			if _, ok := next[a]; !ok {
				problems = append(problems, fmt.Sprintf("%s: no action %q", scope, name))
				continue
			}
			keys, err := parseKeys(file[scope][name])
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s.%s: %v", scope, name, err))
				continue
			}
			next[a] = keys
		}

		if clash := conflicts(next); len(clash) > 0 {
			problems = append(problems, fmt.Sprintf("%s: %s — keeping this scope's defaults",
				scope, strings.Join(clash, "; ")))
			continue
		}
		bound[s] = next
	}
	index()

	if len(problems) > 0 {
		return fmt.Errorf("%s: %s", Path(), strings.Join(problems, ", "))
	}
	return nil
}

// parseKeys reads one action's keys: a list, or a single key on its own.
// "space" is accepted for the space bar, which is easy to misread as " ".
func parseKeys(raw json.RawMessage) ([]string, error) {
	var keys []string
	if err := json.Unmarshal(raw, &keys); err != nil {
		var one string
		if json.Unmarshal(raw, &one) != nil {
			return nil, errors.New("want a key or a list of keys")
		}
		keys = []string{one}
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "space" {
			key = " "
		}
		if key == "" {
			return nil, errors.New("empty key")
		}
		out = append(out, key)
	}
	return out, nil
}

// conflicts lists every key that more than one action in a scope is on.
func conflicts(actions map[Action][]string) []string {
	owners := map[string][]string{}
	for a, keys := range actions {
		for _, key := range keys {
			owners[key] = append(owners[key], string(a))
		}
	}
	var out []string
	for _, key := range sortedKeys(owners) {
		if names := owners[key]; len(names) > 1 {
			sort.Strings(names)
			out = append(out, fmt.Sprintf("%s is on %s", Display(key), strings.Join(names, " and ")))
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Entry is one row of the keymap in force, for listing it.
type Entry struct {
	Scope  Scope
	Action Action
	Keys   []string
}

// Current is the keymap in force, in the order it ships in.
func Current() []Entry {
	out := make([]Entry, 0, len(defaults))
	for _, b := range defaults {
		out = append(out, Entry{b.scope, b.action, Keys(b.scope, b.action)})
	}
	return out
}

// DefaultsJSON is the whole shipped keymap as a keys.json, to start editing
// from. The space bar is written "space".
func DefaultsJSON() []byte {
	var b strings.Builder
	b.WriteString("{\n")
	var scope Scope
	for i, d := range defaults {
		if d.scope != scope {
			if scope != "" {
				b.WriteString("\n  },\n")
			}
			scope = d.scope
			fmt.Fprintf(&b, "  %q: {\n", scope)
		} else {
			b.WriteString(",\n")
		}
		keys := make([]string, len(d.keys))
		for j, key := range d.keys {
			if key == " " {
				key = "space"
			}
			q, _ := json.Marshal(key)
			keys[j] = string(q)
		}
		fmt.Fprintf(&b, "    %q: [%s]", d.action, strings.Join(keys, ", "))
		if i == len(defaults)-1 {
			b.WriteString("\n  }\n")
		}
	}
	b.WriteString("}\n")
	return []byte(b.String())
}
