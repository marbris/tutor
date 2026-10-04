package stats

import (
	"testing"

	"ttr/internal/catalog"
	"ttr/internal/deck"
	"ttr/internal/mtg"
)

func typed(name, typeLine string, qty int) deck.Card {
	return deck.Card{Qty: qty, Card: mtg.Card{Name: name, TypeLine: typeLine}}
}

func withCatalog(t *testing.T) {
	t.Helper()
	old := catalog.Current()
	catalog.SetCurrent(&catalog.Data{Lists: map[string][]string{
		"creature-types": {"Elf", "Druid", "Golem", "Dryad"},
		"artifact-types": {"Equipment"},
		"land-types":     {"Forest"},
	}})
	t.Cleanup(func() { catalog.SetCurrent(old) })
}

func labels(rows []Row) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		n := 0
		for _, e := range deckOfTypes() {
			if r.Match(e) {
				n += e.Qty
			}
		}
		out[r.Path] = n
	}
	return out
}

func deckOfTypes() []deck.Card {
	return []deck.Card{
		typed("Llanowar Elves", "Creature — Elf Druid", 1),
		typed("Elvish Mystic", "Creature — Elf Druid", 1),
		typed("Bone Saw", "Artifact — Equipment", 1),
		typed("Golem Foundry Blade", "Artifact Creature — Equipment Golem", 1),
		typed("Dryad Arbor", "Land Creature — Forest Dryad", 1),
		typed("Forest", "Basic Land — Forest", 3),
	}
}

func TestSubtypesSitUnderTheTypeTheyBelongTo(t *testing.T) {
	withCatalog(t)
	cards := deckOfTypes()
	open := map[string]bool{"type:Creature": true, "type:Artifact": true, "type:Land": true}
	got := labels(typeRows(cards, open))
	want := map[string]int{
		"type:Creature": 4, "type:Creature/Elf": 2, "type:Creature/Druid": 2,
		"type:Creature/Golem": 1, "type:Creature/Dryad": 1,
		"type:Artifact": 2, "type:Artifact/Equipment": 2,
		"type:Land": 4, "type:Land/Forest": 4,
	}
	for path, n := range want {
		if got[path] != n {
			t.Errorf("%s counts %d, want %d (all: %v)", path, got[path], n, got)
		}
	}
	// Equipment is an artifact type: not under Creature, though the Blade is
	// a creature too.
	if _, ok := got["type:Creature/Equipment"]; ok {
		t.Error("Equipment counted under Creature")
	}
}

func TestClosedTypesShowNoSubtypesButSayTheyHaveThem(t *testing.T) {
	withCatalog(t)
	rows := typeRows(deckOfTypes(), nil)
	for _, r := range rows {
		if r.Depth != 0 {
			t.Errorf("a closed tree shows %s", r.Path)
		}
	}
	if rows[0].Label != "Creature" && rows[0].Label != "Land" {
		t.Errorf("the commonest type isn't first: %s", rows[0].Label)
	}
	for _, r := range rows {
		if !r.Expandable {
			t.Errorf("%s has subtypes but can't be opened", r.Label)
		}
	}
}

func TestWithoutTheCatalogsASubtypeSitsUnderThePrimaryType(t *testing.T) {
	old := catalog.Current()
	catalog.SetCurrent(nil)
	t.Cleanup(func() { catalog.SetCurrent(old) })
	got := labels(typeRows(deckOfTypes(), map[string]bool{"type:Creature": true, "type:Artifact": true}))
	// The Blade is filed as a creature, so its Equipment goes there.
	if got["type:Creature/Equipment"] != 1 || got["type:Artifact/Equipment"] != 1 {
		t.Errorf("without catalogs: %v", got)
	}
}
