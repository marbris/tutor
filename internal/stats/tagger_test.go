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
	g, ok := taggerGroup(GroupsBy(cards, cards, false, nil))
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
	g, _ := taggerGroup(GroupsBy(cards, cards, false, map[string]bool{"removal": true}))
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
	if _, ok := taggerGroup(GroupsBy(cards, cards, false, nil)); ok {
		t.Error("a Tagger group with no tags loaded")
	}
}
