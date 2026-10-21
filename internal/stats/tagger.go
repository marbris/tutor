package stats

import (
	"sort"
	"strings"

	"ttr/internal/deck"
	"ttr/internal/tagger"
	"ttr/internal/theme"
)

// The Scryfall Tagger group: what Tagger says the cards do, apart from the
// deck's own tags, which a few hundred of Tagger's would bury.
//
// Tagger's tags are a family tree, so the group is drawn as one: the tags
// at the top first, each counting every card that has it or anything under
// it — removal counts the artifact removal and the enchantment removal —
// and a tag opened (enter) shows its children beneath it, indented, each
// with its own bar. A tag can have more than one parent, so a row is named
// by its path from the top, not by its tag alone: removal-artifact under
// removal and under artifact-matters are two rows.

// TaggerGroup is the group's title.
const TaggerGroup = "Scryfall Tagger"

// pathSep joins the tags of a path. Tag names are slugs and have no slash.
const pathSep = "/"

// taggerRows is the tree of tags the cards in rowSource have, the roots and
// whatever open has opened, each level by how many of the counted cards it
// holds, and the untagged remainder last. A tag none of the counted cards
// has is left out — the tree is long, and the narrowing would leave it full
// of zeros — unless keep has it or a tag under it: a row in the filter has
// to stay, or the filter couldn't be taken off it.
func taggerRows(rowSource, counted []deck.Card, open, keep map[string]bool) []Row {
	tg := tagger.Current()
	if tg == nil {
		return nil
	}
	base := map[int]int{}
	tagged := false
	for _, e := range rowSource {
		for t := range tg.Closure(e.Card.OracleID) {
			base[t] += copies(e)
			tagged = true
		}
	}
	if !tagged {
		return nil
	}
	count := map[int]int{}
	untagged := 0
	for _, e := range counted {
		closure := tg.Closure(e.Card.OracleID)
		for t := range closure {
			count[t] += copies(e)
		}
		if len(closure) == 0 {
			untagged += copies(e)
		}
	}

	// kept reports whether path is in keep, or leads to a path that is.
	kept := func(path string) bool {
		for k := range keep {
			if k == path || strings.HasPrefix(k, path+pathSep) {
				return true
			}
		}
		return false
	}

	// present is the tags among is that the counted cards have, or that keep
	// holds on to, the most counted first.
	present := func(is []int, parent string) []int {
		var out []int
		for _, i := range is {
			if base[i] > 0 && (count[i] > 0 || kept(join(parent, tg.Tags[i].Label))) {
				out = append(out, i)
			}
		}
		sort.SliceStable(out, func(a, b int) bool {
			if count[out[a]] != count[out[b]] {
				return count[out[a]] > count[out[b]]
			}
			if base[out[a]] != base[out[b]] {
				return base[out[a]] > base[out[b]]
			}
			return tg.Tags[out[a]].Label < tg.Tags[out[b]].Label
		})
		return out
	}

	var rows []Row
	var walk func(tags []int, parent string, depth int)
	walk = func(tags []int, parent string, depth int) {
		for _, t := range present(tags, parent) {
			path := join(parent, tg.Tags[t].Label)
			children := present(tg.Tags[t].Children, path)
			r := taggerRow(path)
			r.Depth, r.Expandable = depth, len(children) > 0
			rows = append(rows, r)
			if open[path] {
				walk(children, path, depth+1)
			}
		}
	}
	walk(tg.Roots(), "", 0)

	if untagged > 0 || keep[untaggedLabel] {
		rows = append(rows, TaggerRow(untaggedLabel))
	}
	return rows
}

// join is a tag's path under its parent's.
func join(parent, label string) string {
	if parent == "" {
		return label
	}
	return parent + pathSep + label
}

// TaggerRow is the row for a path of tags, whether or not it's on show: a
// statistics filter restored from the last session names its rows this
// way, and the tags may not even be in yet. It asks for them as it matches.
func TaggerRow(path string) Row {
	if path == untaggedLabel {
		return Row{
			Group: TaggerGroup, Label: untaggedLabel, Path: untaggedLabel, Color: theme.TextDim,
			Match: func(ci deck.Card) bool { return !tagger.Current().Tagged(ci.Card.OracleID) },
		}
	}
	return taggerRow(path)
}

func taggerRow(path string) Row {
	label := path[strings.LastIndex(path, pathSep)+1:]
	return Row{
		Group: TaggerGroup, Label: label, Path: path, Color: theme.Info,
		Match: func(ci deck.Card) bool {
			tg := tagger.Current()
			t, ok := tg.Find(label)
			return ok && tg.Has(ci.Card.OracleID, t)
		},
	}
}
