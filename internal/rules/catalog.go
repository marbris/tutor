package rules

import (
	"regexp"
	"strings"

	"ttr/internal/catalog"
)

// Scryfall's keyword catalogs, folded into the keywords the rules name.
//
// The rules text has a rule for each keyword, but not every keyword has a
// rule under its own name: Forestwalk is landwalk, Plainscycling is cycling,
// Hexproof from is hexproof. Scryfall's catalogs list every keyword as it is
// printed, the newest and the digital ones too. So the catalogs decide what
// is a keyword, for highlighting, and the rules decide which rule it is,
// where they have one.

// keywordIndex is every keyword Tutor highlights, and the pattern that
// finds them. Every copy of a Data shares one, so the catalogs arriving
// after the rules reach every list and panel already holding the rules.
type keywordIndex struct {
	all  map[string]Keyword // by kwKey
	re   *regexp.Regexp
	from *catalog.Data // the catalogs folded in, if any
}

func (x *keywordIndex) set(all map[string]Keyword) {
	x.all = all
	var names []string
	for _, k := range all {
		names = append(names, k.Name)
		// Printed with either apostrophe: the rules use ’, Scryfall '.
		if alt := strings.ReplaceAll(k.Name, "’", "'"); alt != k.Name {
			names = append(names, alt)
		} else if alt := strings.ReplaceAll(k.Name, "'", "’"); alt != k.Name {
			names = append(names, alt)
		}
	}
	x.re = alternation(names)
}

func (x *keywordIndex) regexp() *regexp.Regexp {
	if x == nil {
		return nil
	}
	return x.re
}

func (x *keywordIndex) lookup(text string) (Keyword, bool) {
	if x == nil {
		return Keyword{}, false
	}
	k, ok := x.all[kwKey(text)]
	return k, ok
}

// kwKey is a keyword's name as the index files it: lowercased, with one
// kind of apostrophe.
func kwKey(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "’", "'"))
}

// AddCatalog folds Scryfall's keyword catalogs into the keywords, for this
// Data and every copy of it. Folding in the same catalogs again does
// nothing; newer ones replace the old.
func (d Data) AddCatalog(c *catalog.Data) {
	if d.kw == nil || c == nil || d.kw.from == c {
		return
	}
	all := make(map[string]Keyword, len(d.keywords)+300)
	for k, v := range d.keywords {
		all[k] = v
	}
	for _, list := range []struct {
		name string
		kind KeywordKind
	}{
		{catalog.KeywordAbilities, KeywordAbility},
		{catalog.KeywordActions, KeywordAction},
		{catalog.AbilityWords, AbilityWord},
	} {
		for _, name := range c.List(list.name) {
			key := kwKey(name)
			if _, have := all[key]; have || len(name) < 3 {
				continue
			}
			all[key] = Keyword{Name: name, Rule: d.ruleFor(key, list.kind), Kind: list.kind}
		}
	}
	d.kw.set(all)
	d.kw.from = c
}

// ruleFor is the rule a keyword the rules don't name is a form of: the
// rules' keyword it ends with (Typecycling, Megamorph, Commander ninjutsu)
// or starts with (Hexproof from, Partner with), the longest if several;
// and any landwalk is landwalk. "" when it is none of them.
func (d Data) ruleFor(key string, kind KeywordKind) string {
	best := ""
	rule := ""
	for k, v := range d.keywords {
		if v.Kind != kind || len(k) < 4 || len(k) <= len(best) {
			continue
		}
		if strings.HasSuffix(key, k) || strings.HasPrefix(key, k+" ") {
			best, rule = k, v.Rule
		}
	}
	if rule == "" && kind == KeywordAbility && strings.HasSuffix(key, "walk") {
		if v, ok := d.keywords["landwalk"]; ok {
			rule = v.Rule
		}
	}
	return rule
}
