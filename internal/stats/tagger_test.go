package stats

import (
	"bytes"
	"compress/gzip"
	"strconv"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/tagger"
	"ttr/internal/theme"
)

// withTags puts a small Tagger tree in use: removal over removal-artifact
// and removal-enchantment, which removal has no cards of its own beside;
// and ramp.
func withTags(t *testing.T) {
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

func oracle(id string) deck.Card {
	return deck.Card{Qty: 1, Card: mtg.Card{Name: id, OracleID: id, TypeLine: "Instant"}}
}

func taggerGroup(groups []Group) (Group, bool) {
	for _, g := range groups {
		if g.Title == TaggerGroup {
			return g, true
		}
	}
	return Group{}, false
}

func rowsText(g Group) string {
	var out []string
	for _, r := range g.Rows {
		out = append(out, strings.Repeat(">", r.Depth)+r.Label+"="+strconv.Itoa(r.Count))
	}
	return strings.Join(out, " ")
}

func TestTaggerCountsAParentByItsChildren(t *testing.T) {
	withTags(t)
	cards := []deck.Card{oracle("shatter"), oracle("disenchant"), oracle("sol"), oracle("island")}
	g, ok := taggerGroup(GroupsBy(cards, cards, Options{}))
	if !ok {
		t.Fatal("no Scryfall Tagger group")
	}
	if got := rowsText(g); got != "removal=2 ramp=1 untagged=1" {
		t.Errorf("rows %s", got)
	}
	if !g.Rows[0].Expandable || g.Rows[1].Expandable {
		t.Error("removal should open and ramp shouldn't")
	}
}

func TestAnOpenTagShowsItsChildrenUnderIt(t *testing.T) {
	withTags(t)
	cards := []deck.Card{oracle("shatter"), oracle("disenchant"), oracle("sol")}
	g, _ := taggerGroup(GroupsBy(cards, cards, Options{Open: map[string]bool{"removal": true}}))
	if got := rowsText(g); got != "removal=2 >removal-artifact=2 >removal-enchantment=1 ramp=1" {
		t.Errorf("rows %s", got)
	}
	if p := g.Rows[1].Path; p != "removal/removal-artifact" {
		t.Errorf("child's path is %q", p)
	}
}

func TestATaggerRowNarrowsByItsTagAndEverythingUnder(t *testing.T) {
	withTags(t)
	r := TaggerRow("removal")
	if !r.Match(oracle("disenchant")) || r.Match(oracle("sol")) {
		t.Error("removal doesn't match by its children")
	}
	if !TaggerRow("untagged").Match(oracle("island")) {
		t.Error("untagged doesn't match an untagged card")
	}
}

func TestNoTaggerGroupWithoutTheTags(t *testing.T) {
	old := tagger.Current()
	tagger.SetCurrent(nil)
	defer tagger.SetCurrent(old)
	cards := []deck.Card{oracle("shatter")}
	if _, ok := taggerGroup(GroupsBy(cards, cards, Options{})); ok {
		t.Error("a Tagger group with no tags loaded")
	}
}

func TestTheTaggerGroupIsLast(t *testing.T) {
	withTags(t)
	cards := []deck.Card{oracle("shatter"), oracle("sol")}
	groups := Groups(cards, cards)
	if len(groups) == 0 || groups[len(groups)-1].Title != TaggerGroup {
		t.Errorf("the groups end with %q, want the Tagger group", groups[len(groups)-1].Title)
	}
}

func TestAnOtagSitsAmongTheTagsInTaggerColour(t *testing.T) {
	withTags(t)
	mine := oracle("sol")
	mine.Tags = []string{"ramp"}
	cards := []deck.Card{oracle("shatter"), oracle("disenchant"), mine, oracle("island")}
	var tags []Row
	for _, g := range GroupsBy(cards, cards, Options{Otags: []string{"removal", "ramp"}}) {
		if g.Title == "Tags" {
			tags = g.Rows
		}
	}
	var got []string
	for _, r := range tags {
		got = append(got, r.Path+"="+strconv.Itoa(r.Count))
	}
	// removal counts its children's cards; ramp the otag and ramp the tag
	// are two rows.
	if s := strings.Join(got, " "); s != "otag:removal=2 tag:ramp=1 otag:ramp=1 tag:untagged=3" {
		t.Errorf("tags %s", s)
	}
	if tags[0].Color != theme.Info || tags[1].Color == theme.Info {
		t.Error("the otag isn't in Tagger's colour, or the tag is")
	}
	if r := TagRow("otag:removal"); r.FilterName() != "otag:removal" || !r.Match(oracle("shatter")) {
		t.Errorf("a restored otag row doesn't match by its tag: %+v", r)
	}
}

func TestTheTaggerGroupSortsByWhatItCountsNowAndHidesTheEmpty(t *testing.T) {
	withTags(t)
	all := []deck.Card{oracle("shatter"), oracle("disenchant"), oracle("sol"), oracle("island")}
	sol := []deck.Card{oracle("sol")}
	g, _ := taggerGroup(GroupsBy(all, sol, Options{}))
	if got := rowsText(g); got != "ramp=1" {
		t.Errorf("narrowed to Sol Ring, rows %s", got)
	}
	// A row in the filter stays, at zero, and the tree leads to it.
	g, _ = taggerGroup(GroupsBy(all, sol, Options{
		Open: map[string]bool{"removal": true},
		Keep: map[string]bool{"removal/removal-enchantment": true},
	}))
	if got := rowsText(g); got != "ramp=1 removal=0 >removal-enchantment=0" {
		t.Errorf("with a kept row, rows %s", got)
	}
}
