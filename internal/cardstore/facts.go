package cardstore

import (
	"strings"

	"ttr/internal/mtg"
)

// Facts is what a search asks of a card: its words, its numbers, its
// colors and formats, and the sets and rarities it was printed at. The
// store holds one for every card, so a search never reads the disk; a card
// in a list gets one made from it (Of), so / and the search speak one
// language.
type Facts struct {
	OracleID string
	Name     string
	ManaCost string
	TypeLine string
	// Text is every face's rules text at once.
	Text string
	CMC  float64

	Power, Toughness, Loyalty string
	Colors, Identity          Colors
	// Legal and Banned are bit sets over Formats; restricted counts as
	// legal, as it does on Scryfall.
	Legal, Banned uint64
	Keywords      []string
	EDHREC        int

	// The printing a search shows, the most normal one: its price, release
	// date and rarity.
	USD      float64
	HasUSD   bool
	Released string
	Rarity   string

	// Sets and Rarities are every printing's, so set: and r: match a card
	// printed there or so, as on Scryfall. For a card in a list, its own.
	Sets     []string
	Rarities Rarities

	// Tags are your own, for a card in a list; the store has none.
	Tags []string
}

// Of is a card's facts, as it stands in a list.
func Of(c mtg.Card, tags []string) Facts {
	f := Facts{
		OracleID: c.OracleID, Name: c.Name, ManaCost: c.DisplayManaCost(), TypeLine: c.TypeLine,
		Text: c.CombinedOracle(), CMC: c.CMC,
		Power: c.Power, Toughness: c.Toughness, Loyalty: c.Loyalty,
		Colors: ColorsOf(c.DisplayColors()), Identity: ColorsOf(c.ColorIdentity),
		Keywords: c.Keywords, EDHREC: c.EDHRECRank,
		Released: c.ReleasedAt, Rarity: c.Rarity, Tags: tags,
	}
	// A double-faced card's power is its front face's.
	if f.Power == "" && f.Toughness == "" && f.Loyalty == "" && len(c.CardFaces) > 0 {
		front := c.CardFaces[0]
		f.Power, f.Toughness, f.Loyalty = front.Power, front.Toughness, front.Loyalty
	}
	f.Legal, f.Banned = legalityBits(c.Legalities)
	f.USD, f.HasUSD = c.USD()
	if c.Set != "" {
		f.Sets = []string{strings.ToLower(c.Set)}
	}
	f.Rarities = RarityOf(c.Rarity)
	return f
}

// ── Colors ──────────────────────────────────────────────────────

// Colors is a set of the five colors, W U B R G as bits 0–4.
type Colors uint8

const colorLetters = "WUBRG"

// colorLists is every set of colors as Scryfall writes it, made once, so
// List costs nothing.
var colorLists = func() (out [32][]string) {
	for set := range out {
		var l []string
		for i, r := range colorLetters {
			if set&(1<<i) != 0 {
				l = append(l, string(r))
			}
		}
		out[set] = l
	}
	return
}()

// ColorsOf is colors as Scryfall lists them ("W", "U", …).
func ColorsOf(list []string) Colors {
	var c Colors
	for _, s := range list {
		if i := strings.Index(colorLetters, strings.ToUpper(s)); i >= 0 && len(s) == 1 {
			c |= 1 << i
		}
	}
	return c
}

// List is the colors as Scryfall lists them.
func (c Colors) List() []string { return colorLists[c&31] }

// ── Rarities ────────────────────────────────────────────────────

// Rarities is a set of rarities, as bits in RarityOrder's order.
type Rarities uint8

// RarityOrder is the rarities from common up, as r>=rare reads them.
var RarityOrder = []string{"common", "uncommon", "rare", "mythic", "special", "bonus"}

// RarityOf is the set holding one rarity, or none for one not known.
func RarityOf(r string) Rarities {
	for i, name := range RarityOrder {
		if strings.EqualFold(r, name) {
			return 1 << i
		}
	}
	return 0
}

// Has reports whether rarity i of RarityOrder is in the set.
func (r Rarities) Has(i int) bool { return r&(1<<i) != 0 }

// ── Formats ─────────────────────────────────────────────────────

// Formats are the formats Legal and Banned count bits over: Scryfall's, as
// of 2026. One Scryfall adds later isn't matched locally until it's added
// here.
var Formats = []string{
	"standard", "future", "historic", "timeless", "gladiator", "pioneer", "modern", "legacy",
	"pauper", "vintage", "penny", "commander", "oathbreaker", "standardbrawl", "brawl",
	"alchemy", "paupercommander", "duel", "oldschool", "premodern", "predh", "competitivebrawl",
	"tlr",
}

// FormatBit is a format's bit in Legal and Banned, and whether it has one.
func FormatBit(format string) (uint64, bool) {
	for i, f := range Formats {
		if f == format {
			return 1 << i, true
		}
	}
	return 0, false
}

// legalityBits is Scryfall's legalities as bit sets.
func legalityBits(legalities map[string]string) (legal, banned uint64) {
	for format, l := range legalities {
		bit, ok := FormatBit(format)
		if !ok {
			continue
		}
		switch l {
		case "legal", "restricted":
			legal |= bit
		case "banned":
			banned |= bit
		}
	}
	return legal, banned
}
