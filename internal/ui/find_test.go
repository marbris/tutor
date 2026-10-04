package ui

import (
	"errors"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/fetch"
	"ttr/internal/mtg"
	"ttr/internal/scryfall"
)

// answer files a result as though it had come back from Scryfall, in reply
// to whatever the panel is currently asking.
func answer(m Model, p *panel, cards []deck.Card, total int, err error) Model {
	return answerTo(m, p.id, p.title, cards, total, err)
}

// answerTo files a result against a particular query, which is how a late
// reply to a search you've already moved on from is simulated.
func answerTo(m Model, id int, query string, cards []deck.Card, total int, err error) Model {
	next, _ := m.handleSearchDone(searchDoneMsg{
		panel: id, query: query, cards: cards, total: total, err: err,
	})
	return next.(Model)
}

// typed opens a find panel with a query in the bar, ready for enter.
func typed(m Model, query string) (Model, *panel) {
	m = drive(m, "space", "f")
	for _, r := range query {
		m = drive(m, string(r))
	}
	return m, m.ws.current()
}

func TestEnterStartsASearchAndShowsItWaiting(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")

	if !p.loading {
		t.Error("the panel is not waiting for anything")
	}
	if p.title != "t:elf" {
		t.Errorf("the header says %q", p.title)
	}
	if p.searchOpen {
		t.Error("the bar stayed open over the results that are coming")
	}
	if !strings.Contains(stripANSI(m.View()), "searching") {
		t.Error("nothing on screen says a search is running")
	}
}

func TestResultsFillThePanel(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 213, nil)

	if p.loading {
		t.Error("still waiting after the answer arrived")
	}
	if p.cardsView() == nil || p.cardsView().count() != 4 {
		t.Fatalf("the panel holds %v", p.cardsView())
	}
	if !strings.Contains(stripANSI(m.View()), "Sol Ring") {
		t.Error("the results are not on screen")
	}
}

func TestScryfallsOrderIsKept(t *testing.T) {
	// The query asked for an order; re-sorting on arrival would throw away
	// the answer to the question just asked.
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)

	if p.cardsView().order != sortArrival {
		t.Errorf("results came in sorted by %v", p.cardsView().order)
	}
	if got := p.cardsView().rows[0].Card.Name; got != "Dwynen, Gilt-Leaf Daen" {
		t.Errorf("first row is %s, want the first card Scryfall sent", got)
	}
	if !strings.Contains(stripANSI(m.View()), "scryfall order") {
		t.Error("the panel does not say the order is Scryfall's")
	}
}

func TestTheCountSaysHowManyWereMatchedNotJustFetched(t *testing.T) {
	// 175 is a page, not an answer, and it shouldn't look like one.
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 213, nil)

	// The header counts copies, not rows: this sample is four rows but ten
	// cards. What matters here is the "/213" — a page is not the answer.
	if got := stripANSI(m.View()); !strings.Contains(got, "10/213") {
		t.Errorf("the count does not mention the 213 matched:\n%s", got)
	}
}

func TestNoResultsIsAnAnswerNotAnError(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf t:island")
	m = drive(m, "enter")
	m = answer(m, p, nil, 0, nil)

	view := stripANSI(m.View())
	if !strings.Contains(view, "no results") {
		t.Errorf("got:\n%s", view)
	}
}

func TestAFailedSearchSaysWhy(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, nil, 0, errors.New("dial tcp: no route to host"))

	view := stripANSI(m.View())
	if !strings.Contains(view, "no route to host") {
		t.Errorf("the error is not on screen:\n%s", view)
	}
}

func TestA404ReadsAsNoResults(t *testing.T) {
	if got := errorText(fetch.NotFound{}); got != "no results" {
		t.Errorf("got %q, want it phrased as an answer", got)
	}
}

func TestAnAnswerToAClosedPanelIsDropped(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	id := p.id
	m = drive(m, "space", "x")

	// Must not panic, and must not resurrect anything.
	next, _ := m.handleSearchDone(searchDoneMsg{panel: id, query: "t:elf", cards: sample()})
	if got := next.(Model).ws.count(); got != 0 {
		t.Errorf("a late answer left %d panels open", got)
	}
}

func TestAStaleAnswerIsDropped(t *testing.T) {
	// Search, search again, and let the first one arrive last.
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)

	m = drive(m, "i", "esc") // esc clears the query it kept for editing
	for _, r := range "t:goblin" {
		m = drive(m, string(r))
	}
	m = drive(m, "enter")

	m = answerTo(m, p.id, "t:elf", sample()[:1], 1, nil) // the first query, arriving late
	if p.title != "t:goblin" {
		t.Fatalf("the panel is showing %q", p.title)
	}
	if !p.loading {
		t.Error("a stale answer was taken for the one being waited on")
	}
	if p.cardsView() != nil && p.cardsView().count() == 1 {
		t.Error("the stale answer's cards were installed")
	}
}

func TestQueryHistoryWalksBackAndForward(t *testing.T) {
	m := sized(120, 30)
	m.history = []string{"t:elf", "t:goblin", "c:r cmc<=2"}

	m = drive(m, "space", "f")
	p := m.ws.current()

	m = drive(m, "up")
	if got := p.search.Value(); got != "c:r cmc<=2" {
		t.Errorf("up gave %q, want the most recent", got)
	}
	m = drive(m, "up", "up")
	if got := p.search.Value(); got != "t:elf" {
		t.Errorf("three ups gave %q, want the oldest", got)
	}
	m = drive(m, "up")
	if got := p.search.Value(); got != "t:elf" {
		t.Errorf("walking past the oldest gave %q", got)
	}
	m = drive(m, "down", "down")
	if got := p.search.Value(); got != "c:r cmc<=2" {
		t.Errorf("two downs gave %q", got)
	}
}

func TestWalkingBackKeepsWhatYouWereTyping(t *testing.T) {
	// Pressing up out of curiosity shouldn't cost you the query you were
	// halfway through writing.
	m := sized(120, 30)
	m.history = []string{"t:elf"}
	m = drive(m, "space", "f")
	p := m.ws.current()

	for _, r := range "half a qu" {
		m = drive(m, string(r))
	}
	m = drive(m, "up")
	if got := p.search.Value(); got != "t:elf" {
		t.Fatalf("up gave %q", got)
	}
	m = drive(m, "down")
	if got := p.search.Value(); got != "half a qu" {
		t.Errorf("coming back gave %q, want the draft", got)
	}
}

func TestRunningASearchRemembersIt(t *testing.T) {
	m, _ := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	if len(m.history) == 0 || m.history[len(m.history)-1] != "t:elf" {
		t.Errorf("history is %v", m.history)
	}
}

func TestTheSameQueryTwiceIsRememberedOnce(t *testing.T) {
	got := RememberQuery(RememberQuery([]string{"a"}, "t:elf"), "t:elf")
	if len(got) != 2 || got[1] != "t:elf" {
		t.Errorf("got %v, want the earlier copy moved rather than duplicated", got)
	}
}

func TestCtrlOChangesTheOrderTheQueryAsksFor(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf")
	before := p.queryOrder()

	m = drive(m, "ctrl+o")
	if p.queryOrder() == before {
		t.Error("ctrl+o did not change the order")
	}
	if !strings.Contains(stripANSI(m.View()), p.queryOrder()) {
		t.Error("the panel does not say what order it will ask for")
	}
}

func TestTheQuerySortCanReachPowerAndToughness(t *testing.T) {
	// ctrl+o walks every order Scryfall offers, power and toughness among
	// them, so a search can come back biggest-first.
	m, p := typed(sized(120, 30), "t:creature")
	want := map[string]bool{"power": true, "toughness": true}
	seen := map[string]bool{p.queryOrder(): true}
	for i := 0; i < len(scryfall.SortOptions); i++ {
		m = drive(m, "ctrl+o")
		seen[p.queryOrder()] = true
	}
	for order := range want {
		if !seen[order] {
			t.Errorf("ctrl+o never reached %q; it cycled %v", order, seen)
		}
	}
}

func TestCtrlOOnlyCyclesTheOrderAndEnterSends(t *testing.T) {
	// With results already on screen, ctrl+o used to re-run the old query
	// straight away — and the answer arriving closed the bar you were
	// typing in. It should change what the next enter asks for, and no
	// more.
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)

	m = drive(m, "i")
	m = drive(m, "ctrl+o", "ctrl+o")
	if p.loading {
		t.Error("ctrl+o sent a query")
	}
	if !p.searchOpen {
		t.Error("ctrl+o closed the bar")
	}

	m = drive(m, "enter")
	if !p.loading {
		t.Error("enter did not send the query")
	}
}

func TestCtrlRCyclesTheDirectionTheQueryAsksFor(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf")
	if got := p.queryDirection(); got != "auto" {
		t.Errorf("default direction is %q, want auto", got)
	}

	for _, want := range []struct{ dir, glyph string }{
		{"asc", "↑"}, {"desc", "↓"},
	} {
		m = drive(m, "ctrl+r")
		if got := p.queryDirection(); got != want.dir {
			t.Errorf("got %q, want %q", got, want.dir)
		}
		if !strings.Contains(stripANSI(m.View()), p.queryOrder()+" "+want.glyph) {
			t.Errorf("the panel does not show %s for %s", want.glyph, want.dir)
		}
		if p.loading {
			t.Error("ctrl+r sent a query")
		}
	}

	m = drive(m, "ctrl+r")
	if got := p.queryDirection(); got != "auto" {
		t.Errorf("got %q, want it to come back round to auto", got)
	}
}

func TestTheResultsHeaderShowsTheDirection(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "ctrl+r", "enter")
	m = answer(m, p, sample(), 4, nil)
	if !strings.Contains(stripANSI(m.View()), p.queryOrder()+" ↑") {
		t.Error("the results header does not say the order ran ascending")
	}
}

func TestTheQuerySortAndTheListSortAreDifferentThings(t *testing.T) {
	// One decides which cards come back; the other decides how the ones in
	// front of you are arranged. Confusing them means re-fetching to
	// reorder.
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)

	queryBefore := p.queryOrder()
	m = drive(m, ".")
	if p.queryOrder() != queryBefore {
		t.Error(". changed the order the request asks for")
	}
	if p.cardsView().order == sortArrival {
		t.Error(". did not change the order on screen")
	}
}

func TestTheDefaultQueryOrderIsEDHRECRank(t *testing.T) {
	// For a Commander player the cards other people actually play are the
	// ones worth seeing first.
	m, _ := typed(sized(120, 30), "t:elf")
	if got := m.ws.current().queryOrder(); got != "edhrec" {
		t.Errorf("default order is %q", got)
	}
	if scryfall.SortOptions[defaultQuerySort] != "edhrec" {
		t.Error("defaultQuerySort no longer points at edhrec")
	}
}

func TestSearchResultsCarryNoQuantity(t *testing.T) {
	// A search result is a deck card with no quantity and no tags, which is
	// what it is — and what keeps "3x" out of a list of search results.
	cards := []deck.Card{{Card: mtg.Card{Name: "Sol Ring"}}}
	if cards[0].Qty != 0 {
		t.Error("a search result should not claim a count")
	}
}

func TestReopeningTheBarKeepsTheQueryForEditing(t *testing.T) {
	m, p := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter")
	m = answer(m, p, sample(), 4, nil)

	m = drive(m, "i")
	if got := p.search.Value(); got != "t:elf" {
		t.Errorf("the bar came back holding %q, want the query to edit", got)
	}
	for _, r := range " c:g" {
		m = drive(m, string(r))
	}
	if got := p.search.Value(); got != "t:elf c:g" {
		t.Errorf("typing appended to %q", got)
	}
}

func TestASecondPanelRecallsTheFirstPanelsQueries(t *testing.T) {
	// The list is the Model's, not the panel's. Each panel used to be handed
	// a copy, and only two of the ways a panel can open ever handed one
	// over — so a find panel opened with <space>f had no history at all,
	// which is every find panel opened during a session.
	m, _ := typed(sized(120, 30), "t:elf")
	m = drive(m, "enter") // running it is what remembers it, and blurs the bar
	m = drive(m, "space", "f")

	if got := m.ws.count(); got != 2 {
		t.Fatalf("%d panels, want 2", got)
	}
	m = drive(m, "up")
	if got := m.ws.current().search.Value(); got != "t:elf" {
		t.Errorf("up in the new panel gave %q, want the query run in the first", got)
	}
}

func TestOtherBarsDoNotRecallCardQueries(t *testing.T) {
	// The rules bar asks a different question, so the card queries are not
	// answers it should be offering.
	m := sized(120, 30)
	m.history = []string{"t:elf"}
	m = drive(m, "space", "r")

	m = drive(m, "up")
	if got := m.ws.current().search.Value(); got != "" {
		t.Errorf("the rules bar recalled %q", got)
	}
}
