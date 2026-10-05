package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/keymap"
)

func TestTheLeaderMenuShowsEveryEntry(t *testing.T) {
	// The entries you can't see are exactly the ones you opened the menu to
	// read, so it wraps rather than being cut off.
	for _, width := range []int{50, 70, 100, 160} {
		m := withCards(sized(width, 24), "f", sample(), sortArrival)
		m = drive(m, "space")

		bar := stripANSI(strings.Join(m.leaderBarLines(), " "))
		for _, c := range leaderMenu {
			if !strings.Contains(bar, c.what) {
				t.Errorf("at %d columns %q is missing from the menu:\n%s",
					width, c.what, bar)
			}
		}
	}
}

func TestTheMenuNeverPushesThePanelsOffTheScreen(t *testing.T) {
	// It used to reserve one row however many it took, and the frame grew
	// past the terminal — the panels lost their top edge.
	for _, width := range []int{40, 50, 70, 100, 200} {
		for _, height := range []int{10, 14, 24, 40} {
			m := withCards(sized(width, height), "f", sample(), sortArrival)
			m = drive(m, "space")

			lines := splitLines(m.View())
			if len(lines) > height {
				t.Errorf("%dx%d: the menu grew the frame to %d lines",
					width, height, len(lines))
			}
			for i, line := range lines {
				if w := visibleWidth(line); w > width {
					t.Errorf("%dx%d: line %d is %d columns", width, height, i, w)
				}
			}
		}
	}
}

func TestEveryMenuKeyPutsTheMenuAway(t *testing.T) {
	// Including the ones that go on to do something else.
	for _, c := range leaderMenu {
		m := withCards(sized(140, 30), "f", sample(), sortArrival)
		m = drive(m, "space")
		if !m.leader {
			t.Fatal("space did not raise the menu")
		}
		key := keymap.Keys(keymap.Leader, c.action)[0]
		m = drive(m, key)
		if m.leader {
			t.Errorf("the menu stayed up after %q", key)
		}
	}
}

func TestTheReferenceDescribesThePanelYouAreIn(t *testing.T) {
	// A reference listing keys that do nothing here is worse than none.
	search := withCards(sized(140, 40), "f", sample(), sortArrival)
	searchKeys := keyText(search)

	own := withCards(sized(140, 40), "d", sample(), sortArrival)
	own.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
	// What opening a deck does, which this bypasses by building the list
	// directly. The editing keys are listed against the deck they change,
	// so there has to be one.
	own.ws.editing = own.ws.focused
	deckKeys := keyText(own)

	// A search with nothing being edited offers the way to choose a deck.
	if !strings.Contains(searchKeys, "choose a deck to edit") {
		t.Errorf("a search doesn't offer a deck to edit:\n%s", searchKeys)
	}

	// Editing keys are listed against the deck they change, and not offered
	// at all with no deck to change: on a search with nothing being edited,
	// every one of them would do nothing but explain itself.
	for _, only := range []string{"add + tag", "x remove"} {
		if !strings.Contains(deckKeys, only) {
			t.Errorf("%q is missing from a deck's keys:\n%s", only, deckKeys)
		}
		if strings.Contains(searchKeys, only) {
			t.Errorf("%q is offered on a search with no deck being edited", only)
		}
	}
}

func TestTheDecksPanelHasItsOwnKeys(t *testing.T) {
	m := drive(sized(140, 40), "space", "d")
	got := keyText(m)

	if !strings.Contains(got, "decks") {
		t.Errorf("headed:\n%s", got)
	}
	for _, want := range []string{"search moxfield", "new", "rename", "git push"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing:\n%s", want, got)
		}
	}
}

func TestTheBarsKeysAreOnTheHintLineNotInTheReference(t *testing.T) {
	// ? is a printable key, so while you're typing it types. The bar's own
	// keys belong where you can see them without stopping.
	m := drive(sized(140, 40), "space", "f")
	if !strings.Contains(stripANSI(m.View()), "another target") {
		t.Errorf("the hint line does not describe the bar:\n%s", stripANSI(m.View()))
	}

	m = drive(m, "?")
	if got := m.ws.current().search.Value(); got != "?" {
		t.Errorf("? did something other than type: bar holds %q", got)
	}
}

func TestTheExpandedBarFitsTheScreen(t *testing.T) {
	// Growing the bar with ? takes room from the panels rather than pushing
	// the frame past the terminal.
	for _, size := range [][2]int{{60, 20}, {80, 24}, {100, 30}, {200, 50}, {70, 16}} {
		m := withCards(sized(size[0], size[1]), "d", sample(), sortArrival)
		m.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
		m = drive(m, "?")

		lines := splitLines(m.View())
		if len(lines) > size[1] {
			t.Errorf("%dx%d: the frame is %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := visibleWidth(line); w > size[0] {
				t.Errorf("%dx%d: line %d is %d columns", size[0], size[1], i, w)
			}
		}
	}
}

// keyText is the screen with ? on, as plain text — the keys are drawn
// inside the panels they act on, so the whole frame is where to read them.
func keyText(m Model) string {
	m.hintsExpanded = true
	return stripANSI(m.View())
}

// footerOf is the same: what ? shows, wherever on the screen it shows it.
func footerOf(m Model) string { return keyText(m) }

// topBlock is the two lines above the panels alone.
func topBlock(m Model) string {
	return stripANSI(m.viewTop())
}

func TestTheHintBarDoesNotOfferStatisticsWhereThereAreNone(t *testing.T) {
	// s counts a list of cards. A decks panel has none, and the bar used to
	// offer it there anyway, because it picked one of three fixed strings.
	for _, key := range []string{"d", "r"} {
		m := drive(sized(120, 30), "space", key)
		if strings.Contains(footerOf(m), "stats") {
			t.Errorf("<space>%s offers stats:\n%s", key, footerOf(m))
		}
	}

	cards := withCards(sized(120, 30), "f", sample(), sortArrival)
	if !strings.Contains(footerOf(cards), "stats") {
		t.Errorf("a list of cards doesn't offer stats:\n%s", footerOf(cards))
	}
}

func TestTheHintBarSaysWhichDeckTheEditingKeysChange(t *testing.T) {
	// a, x, t, T, c and u act on the editing deck, not on the list under
	// the cursor. Reading "add" over somebody else's deck and watching the
	// card land elsewhere is what this names away.
	m := withCards(sized(140, 30), "d", sample(), sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
	m.ws.editing = m.ws.focused

	// A second panel, so the editing deck is not the one in front of you.
	m = withCards(m, "f", sample(), sortArrival)
	// The keys are drawn under the deck they change, not under the list
	// in front of you: which deck a lands in is where its hint sits.
	m.hintsExpanded = true
	deckPanel := stripANSI(m.viewPanel(m.ws.panels[0], 0, 60, 30, 2))
	searchPanel := stripANSI(m.viewPanel(m.ws.panels[1], 1, 60, 30, 2))
	for _, want := range []string{"add", "add + tag latest", "remove", "undo"} {
		if !strings.Contains(deckPanel, want) {
			t.Errorf("%q isn't under the editing deck:\n%s", want, deckPanel)
		}
	}
	if strings.Contains(searchPanel, "add + tag latest") {
		t.Errorf("the editing keys are under the search, not the deck:\n%s", searchPanel)
	}
	if !strings.Contains(searchPanel, "sort 1 & 2") {
		t.Errorf("the focused panel lost its own keys:\n%s", searchPanel)
	}
}

func TestNoEditingDeckMeansNoEditingKeys(t *testing.T) {
	// With nothing being edited they do nothing but explain themselves, so
	// what is offered instead is the way to choose a deck.
	m := withCards(sized(120, 30), "f", sample(), sortArrival)
	got := footerOf(m)
	for _, gone := range []string{"add to deck", "add a copy", "remove from deck"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q offered with no deck being edited:\n%s", gone, got)
		}
	}
	if !strings.Contains(got, "choose a deck to edit") {
		t.Errorf("no way offered to choose one:\n%s", got)
	}
}

func TestTheHintBarAndTheReferenceAgree(t *testing.T) {
	// Two lists describing one keymap is how one of them comes to lie. Every
	// key the reference names has to be on the bar, in every context.
	cases := map[string]Model{
		"decks":   drive(sized(140, 40), "space", "d"),
		"rules":   drive(sized(140, 40), "space", "r"),
		"search":  withCards(sized(140, 40), "f", sample(), sortArrival),
		"a deck":  editingDeckModel(),
		"stats":   drive(editingDeckModel(), "s"),
		"the bar": drive(sized(140, 40), "space", "f"),
	}
	for name, m := range cases {
		bar := footerOf(m)
		for _, row := range m.contextKeys() {
			if !strings.Contains(bar, row[1]) {
				t.Errorf("%s: the reference offers %q (%s) and the bar doesn't:\n%s",
					name, row[1], row[0], bar)
			}
		}
	}
}

func editingDeckModel() Model {
	m := withCards(sized(140, 40), "d", sample(), sortArrival)
	m.ws.current().cardsView().deck = &deck.Info{Name: "Ghen", Slug: "ghen"}
	m.ws.editing = m.ws.focused
	return m
}

func TestNoKeyIsOfferedTwice(t *testing.T) {
	// The decks panel listed i twice: the view claimed it and the panel
	// added it again with a different label.
	cases := map[string]Model{
		"decks":  drive(sized(140, 40), "space", "d"),
		"rules":  drive(sized(140, 40), "space", "r"),
		"search": withCards(sized(140, 40), "f", sample(), sortArrival),
		"a deck": editingDeckModel(),
	}
	for name, m := range cases {
		seen := map[string]string{}
		for _, row := range m.contextKeys() {
			if was, dup := seen[row[0]]; dup {
				t.Errorf("%s: %q offered as %q and again as %q", name, row[0], was, row[1])
			}
			seen[row[0]] = row[1]
		}
	}
}

func TestTheNoticeSitsUnderTheKeysAndGoesAway(t *testing.T) {
	// The result of the last thing you did gets its own line under the keys,
	// next to the panels, rather than sharing a line with them — so it can be read at a glance
	// and the keys for what to do next aren't crowded. The next key clears
	// it, which is what it always claimed to and once didn't.
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m.notice = "+1 Sol Ring"

	got := topBlock(m)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], "+1 Sol Ring") {
		t.Fatalf("the notice isn't on the second line:\n%s", got)
	}
	if strings.Contains(lines[1], "quit") {
		t.Errorf("the keys are packed onto the notice line:\n%s", lines[1])
	}
	if !strings.Contains(lines[0], "q quit") {
		t.Errorf("the keys aren't shown above the notice:\n%s", got)
	}

	m = drive(m, "j")
	if m.notice != "" {
		t.Errorf("the next key left the notice up: %q", m.notice)
	}
}

func TestAKeyThatSetsANoticeKeepsIt(t *testing.T) {
	// Clearing happens before the key is dispatched, so a key that says
	// something still gets to say it.
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, "p") // nothing yanked, and nowhere to put it
	if m.notice == "" {
		t.Error("the key said nothing")
	}
}
