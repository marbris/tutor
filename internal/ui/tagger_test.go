package ui

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

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
	old := taggerData
	taggerData = d
	t.Cleanup(func() { taggerData = old })
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
