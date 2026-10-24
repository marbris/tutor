package ui

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/stats"
	"ttr/internal/tagger"
)

// withTagger puts a small set of Tagger tags in force for one test:
// removal with two children, and ramp.
func withTagger(t *testing.T) {
	t.Helper()
	const lines = `{"id":"r","label":"removal","parent_ids":[],"child_ids":["ra","re"],"taggings":[]}
{"id":"ra","label":"removal-artifact","parent_ids":["r"],"child_ids":[],"taggings":[{"oracle_id":"shatter"},{"oracle_id":"disenchant"}]}
{"id":"re","label":"removal-enchantment","parent_ids":["r"],"child_ids":[],"taggings":[{"oracle_id":"disenchant"}]}
{"id":"rp","label":"ramp","parent_ids":[],"child_ids":[],"taggings":[{"oracle_id":"sol"}]}
`
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(lines))
	w.Close()
	d, err := tagger.Read(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	old := tagger.Current()
	tagger.SetCurrent(d)
	t.Cleanup(func() { tagger.SetCurrent(old) })
}

func TestTabCompletesAnOracleTagInTheSearchBar(t *testing.T) {
	withTagger(t)
	m, p := typed(sized(120, 30), "t:instant otag:re")
	if !m.canCompleteOtag(p) {
		t.Fatal("tab isn't offered for completing the tag")
	}
	m = drive(m, "tab")
	if got := p.search.Value(); got != "t:instant otag:removal" {
		t.Errorf("tab made %q, want the common part, removal", got)
	}
	if !strings.Contains(m.notice, "removal-artifact") {
		t.Errorf("the notice doesn't list the tags that fit: %q", m.notice)
	}
	m = drive(m, "tab")
	if got := p.search.Value(); got != "t:instant otag:removal" {
		t.Errorf("the first step of the walk is %q", got)
	}
	m = drive(m, "tab")
	if got := p.search.Value(); got != "t:instant otag:removal-artifact" {
		t.Errorf("the second step of the walk is %q", got)
	}
	if p.kind != KindFind {
		t.Error("tab changed the bar's target while completing")
	}
}

func TestTabCompletesANegatedOrBracketedTagToTheEnd(t *testing.T) {
	withTagger(t)
	m, p := typed(sized(120, 30), "(-function:ra")
	m = drive(m, "tab")
	if got := p.search.Value(); got != "(-function:ramp " {
		t.Errorf("tab made %q", got)
	}
}

func TestTabStillChangesTargetOnAnyOtherWord(t *testing.T) {
	withTagger(t)
	m, p := typed(sized(120, 30), "t:elf")
	if m.canCompleteOtag(p) {
		t.Error("completion offered on t:elf")
	}
	m = drive(m, "tab")
	if m.ws.current().kind == KindFind {
		t.Error("tab didn't move the bar on to the next target")
	}
}

func TestTheInfoPanelShowsTheCardsTaggerTags(t *testing.T) {
	withTagger(t)
	card := deck.Card{Card: mtg.Card{Name: "Disenchant", OracleID: "disenchant"}}
	body := stripANSI(strings.Join(cardMeta(card, card.Card, 40, nil, nil), "\n"))
	if !strings.Contains(body, "scryfall tagger") || !strings.Contains(body, "removal-artifact removal-enchantment") {
		t.Errorf("the tagger tags are missing:\n%s", body)
	}
	plain := deck.Card{Card: mtg.Card{Name: "Island", OracleID: "island"}}
	if body := stripANSI(strings.Join(cardMeta(plain, plain.Card, 40, nil, nil), "\n")); strings.Contains(body, "scryfall tagger") {
		t.Errorf("a card Tagger hasn't tagged has a tagger heading:\n%s", body)
	}
}

func taggedCards() []deck.Card {
	var out []deck.Card
	for _, id := range []string{"shatter", "disenchant", "sol", "island"} {
		out = append(out, deck.Card{Qty: 1, Card: mtg.Card{Name: id, OracleID: id, TypeLine: "Instant"}})
	}
	return out
}

func TestEnterOpensATaggerTagInTheStatistics(t *testing.T) {
	withTagger(t)
	m := withCards(sized(140, 50), "f", taggedCards(), sortArrival)
	m = drive(m, "s")
	if r, _ := m.statUnder(); r.Group == stats.TaggerGroup {
		t.Fatal("the statistics open on the Tagger group, want it last")
	}
	m = pointStat(t, m, stats.TaggerGroup, "removal")
	if hints := fmt.Sprint(m.statsHints()); !strings.Contains(hints, "show the tags under it") {
		t.Error("enter isn't offered on removal")
	}
	m = drive(m, "enter")
	body := stripANSI(strings.Join(m.renderStats(60), "\n"))
	if !strings.Contains(body, "▾ removal") || !strings.Contains(body, "    removal-artifact") {
		t.Errorf("removal didn't open:\n%s", body)
	}
	m = drive(m, "ctrl+j", "alt+a")
	l := m.ws.current().cardsView()
	if len(l.rows) != 2 {
		t.Errorf("filtering by removal-artifact left %d cards, want 2", len(l.rows))
	}
	m = drive(m, "ctrl+k", "enter")
	if body := stripANSI(strings.Join(m.renderStats(60), "\n")); strings.Contains(body, "removal-enchantment") {
		t.Errorf("a second enter didn't close removal:\n%s", body)
	}
}

func TestATaggerFilterFromLastSessionWorksOnceTheTagsArrive(t *testing.T) {
	withTagger(t)
	d := tagger.Current()
	tagger.SetCurrent(nil)

	m := withCards(sized(140, 50), "f", taggedCards(), sortArrival)
	l := m.ws.current().cardsView()
	ps := panelSession{Stats: []savedClause{{Op: "and", Group: stats.TaggerGroup, Label: "removal", Path: "removal"}}}
	ps.applyLayout(l)
	if len(l.statFilter) != 1 {
		t.Fatal("the Tagger category wasn't restored")
	}
	next, _ := m.Update(taggerMsg{data: d})
	m = next.(Model)
	if n := len(m.ws.current().cardsView().rows); n != 2 {
		t.Errorf("%d cards after the tags arrived, want the 2 removal spells", n)
	}
}

func TestATaggerTagsMeaningShowsUnderItInTheStatistics(t *testing.T) {
	const lines = `{"id":"r","label":"removal","description":"Get things off the table. See also [spot removal](spot-removal).","parent_ids":[],"child_ids":["ra"],"taggings":[]}
{"id":"ra","label":"removal-artifact","parent_ids":["r"],"child_ids":[],"taggings":[{"oracle_id":"shatter"}]}
`
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(lines))
	w.Close()
	d, err := tagger.Read(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	old := tagger.Current()
	tagger.SetCurrent(d)
	t.Cleanup(func() { tagger.SetCurrent(old) })

	m := withCards(sized(140, 50), "f", taggedCards(), sortArrival)
	m = pointStat(t, drive(m, "s"), stats.TaggerGroup, "removal")
	body := stripANSI(strings.Join(m.renderStats(70), "\n"))
	if !strings.Contains(body, "Get things off the table. See also spot removal.") {
		t.Errorf("removal's meaning isn't under it, links made plain:\n%s", body)
	}
	// One without a description says so.
	m = drive(m, "enter", "ctrl+j")
	body = stripANSI(strings.Join(m.renderStats(70), "\n"))
	if !strings.Contains(body, "no description on Tagger") {
		t.Errorf("a tag with no description doesn't say so:\n%s", body)
	}
	if strings.Contains(body, "Get things off the table") {
		t.Error("the meaning stayed with the row the highlight left")
	}
}

func TestAltTShowsATaggerTagWithTheTagsAndKeepsIt(t *testing.T) {
	isolate(t)
	freshGlobalTags(t)
	withTagger(t)
	m := withCards(sized(140, 50), "f", taggedCards(), sortArrival)
	m = pointStat(t, drive(m, "s"), stats.TaggerGroup, "removal")
	if r, _ := m.statUnder(); r.Label != "removal" {
		t.Fatalf("on %s/%s, want the Tagger group's removal", r.Group, r.Label)
	}
	if hints := fmt.Sprint(m.statsHints()); !strings.Contains(hints, "show with the tags") {
		t.Error("alt+t isn't offered on removal")
	}
	m = drive(m, "alt+t")
	r, ok := findRow(m.statGroups(), "Tags", "removal")
	if !ok || r.Count != 2 || r.Path != "otag:removal" {
		t.Fatalf("removal isn't among the tags counting 2: %+v", r)
	}
	if got := loadOtags(); len(got) != 1 || got[0] != "removal" {
		t.Errorf("globalotags.json holds %v", got)
	}

	// Taken out again from its row among the tags.
	m.pointAt(r)
	if hints := fmt.Sprint(m.statsHints()); !strings.Contains(hints, "take out of the tags") {
		t.Error("alt+t isn't offered on the otag row")
	}
	m = drive(m, "alt+t")
	if _, ok := findRow(m.statGroups(), "Tags", "removal"); ok {
		t.Error("removal is still among the tags")
	}
	if got := loadOtags(); len(got) != 0 {
		t.Errorf("globalotags.json still holds %v", got)
	}
}

func TestAltNOnATaggerRowCanBeTakenBackThoughItCountsNothing(t *testing.T) {
	withTagger(t)
	m := withCards(sized(140, 50), "f", taggedCards(), sortArrival)
	l := m.ws.current().cardsView()
	m = drive(m, "s")
	m = pointStat(t, m, stats.TaggerGroup, "ramp")
	m = drive(m, "alt+n")
	if l.count() != 3 {
		t.Fatalf("not ramp left %d cards, want 3", l.count())
	}
	r, ok := findRow(m.statGroups(), stats.TaggerGroup, "ramp")
	if !ok || r.Count != 0 {
		t.Fatalf("ramp, in the filter, should still show at 0: %+v %v", r, ok)
	}
	if u, _ := m.statUnder(); u.Label != "ramp" {
		t.Errorf("the highlight moved to %s", u.Label)
	}
	m = drive(m, "alt+n")
	if len(l.statFilter) != 0 || l.count() != 4 {
		t.Errorf("alt+n again left the filter %v and %d cards", l.statFilter, l.count())
	}
}
