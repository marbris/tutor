package mtg

import (
	"strings"
	"testing"
)

// dfc is a transforming card, which is where all the awkwardness lives: it
// carries nothing at the top level and everything per face.
func dfc() Card {
	return Card{
		Name:     "Brutal Cathar // Moonrage Brute",
		TypeLine: "Creature — Human Cleric // Creature — Werewolf",
		CardFaces: []Face{
			{
				Name: "Brutal Cathar", ManaCost: "{2}{W}", TypeLine: "Creature — Human Cleric",
				OracleText: "When this creature enters, exile target creature.",
				Colors:     []string{"W"}, Power: "2", Toughness: "2",
			},
			{
				Name: "Moonrage Brute", TypeLine: "Creature — Werewolf",
				OracleText: "Trample", Colors: []string{"R"}, Power: "3", Toughness: "3",
			},
		},
	}
}

func TestASingleFacedCardIsItsOwnFace(t *testing.T) {
	c := Card{Name: "Sol Ring", ManaCost: "{1}", TypeLine: "Artifact"}
	faces := c.Faces()
	if len(faces) != 1 || faces[0].Name != "Sol Ring" {
		t.Errorf("got %v", faces)
	}
}

func TestEachFaceComesBackAsACardInItsOwnRight(t *testing.T) {
	// Anything that renders a card can then work a face at a time, which is
	// the only way to see the back of a transforming card at all.
	faces := dfc().Faces()
	if len(faces) != 2 {
		t.Fatalf("got %d faces", len(faces))
	}
	if faces[0].Name != "Brutal Cathar" || faces[1].Name != "Moonrage Brute" {
		t.Errorf("faces are %s and %s", faces[0].Name, faces[1].Name)
	}
	if faces[0].TypeLine != "Creature — Human Cleric" {
		t.Errorf("the front face kept the combined type line: %q", faces[0].TypeLine)
	}
	if len(faces[0].CardFaces) != 0 {
		t.Error("a face still carries the faces, which would recurse")
	}
}

func TestAFaceWithNoColoursKeepsTheCardsRatherThanGoingColourless(t *testing.T) {
	c := dfc()
	c.CardFaces[1].Colors = nil
	c.Colors = []string{"W"}

	back := c.Faces()[1]
	if len(back.Colors) != 1 || back.Colors[0] != "W" {
		t.Errorf("the back face came out %v", back.Colors)
	}
}

func TestCombinedOracleIsEveryFaceAtOnce(t *testing.T) {
	// For matching against a card's wording rather than displaying it: a
	// filter for "trample" has to find the back of a werewolf.
	got := dfc().CombinedOracle()
	if !strings.Contains(got, "exile target creature") || !strings.Contains(got, "Trample") {
		t.Errorf("got %q", got)
	}
}

func TestCombinedOracleOfAPlainCardIsItsText(t *testing.T) {
	c := Card{OracleText: "{T}: Add {G}."}
	if got := c.CombinedOracle(); got != "{T}: Add {G}." {
		t.Errorf("got %q", got)
	}
}

func TestDisplayColoursFallBackToTheFrontFace(t *testing.T) {
	// A transforming card carries no colours at the top level.
	c := dfc()
	if got := c.DisplayColors(); len(got) != 1 || got[0] != "W" {
		t.Errorf("got %v, want the front face's", got)
	}

	plain := Card{Colors: []string{"U", "R"}}
	if got := plain.DisplayColors(); len(got) != 2 {
		t.Errorf("got %v", got)
	}
}

func TestDisplayManaCostFallsBackToTheFrontFace(t *testing.T) {
	if got := dfc().DisplayManaCost(); got != "{2}{W}" {
		t.Errorf("got %q, want the front face's cost", got)
	}
	plain := Card{ManaCost: "{G}{G}"}
	if got := plain.DisplayManaCost(); got != "{G}{G}" {
		t.Errorf("got %q", got)
	}
}

func TestPrimaryTypePicksOneSection(t *testing.T) {
	for typeLine, want := range map[string]string{
		"Creature — Elf Druid":                "Creature",
		"Artifact Creature — Golem":           "Creature", // creature wins
		"Legendary Artifact Land":             "Land",     // land beats artifact
		"Enchantment Artifact":                "Artifact",
		"Legendary Planeswalker — Jace":       "Planeswalker",
		"Basic Land — Forest":                 "Land",
		"Instant":                             "Instant",
		"Battle — Siege":                      "Battle",
		"Kindred Sorcery — Elf":               "Sorcery",
		"Creature — Human // Creature — Wolf": "Creature",
		"Plane — Dominaria":                   "Other",
	} {
		if got := PrimaryType(typeLine); got != want {
			t.Errorf("PrimaryType(%q) = %q, want %q", typeLine, got, want)
		}
	}
}

func TestPrimaryTypeReadsTheFrontFace(t *testing.T) {
	// A modal double-faced card is filed under what its front half is.
	if got := PrimaryType("Land // Creature — Elf"); got != "Land" {
		t.Errorf("got %q", got)
	}
}

func TestIsLand(t *testing.T) {
	if !IsLand(Card{TypeLine: "Basic Land — Island"}) {
		t.Error("a basic land is not a land")
	}
	if IsLand(Card{TypeLine: "Creature — Elf Druid"}) {
		t.Error("an elf is a land")
	}
	// An artifact land is a land, which is why Land beats Artifact.
	if !IsLand(Card{TypeLine: "Artifact Land"}) {
		t.Error("an artifact land is not a land")
	}
}

func TestSectionsAndPrecedenceAgree(t *testing.T) {
	// Every type the precedence list can return has to have somewhere to go
	// in a decklist, or cards would vanish from the deck view.
	in := map[string]bool{}
	for _, s := range Sections {
		in[s] = true
	}
	for _, t2 := range TypePrecedence {
		if !in[t2] {
			t.Errorf("%q can be a primary type but has no section", t2)
		}
	}
	if !in["Other"] {
		t.Error("nothing catches the types that match nothing")
	}
}

func TestFaceImageIsEachSideOfADoubleFacedCard(t *testing.T) {
	dfc := Card{CardFaces: []Face{
		{Name: "Front", ImageURIs: ImageURIs{Normal: "f.jpg", BorderCrop: "fc.jpg"}},
		{Name: "Back", ImageURIs: ImageURIs{Normal: "b.jpg", BorderCrop: "bc.jpg"}},
	}}
	if n := dfc.PictureFaces(); n != 2 {
		t.Errorf("a double-faced card has %d pictures", n)
	}
	if got := dfc.FaceImage(1, "border_crop"); got != "bc.jpg" {
		t.Errorf("back face is %q", got)
	}
	if got := dfc.FaceImage(0, "border_crop"); got != "fc.jpg" {
		t.Errorf("front face is %q", got)
	}
	adventure := Card{ImageURIs: ImageURIs{Normal: "a.jpg"}, CardFaces: []Face{{Name: "Giant"}, {Name: "Stomp"}}}
	if n := adventure.PictureFaces(); n != 1 {
		t.Errorf("an adventure card has %d pictures", n)
	}
}
