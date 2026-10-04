// Package mtg is the game's own vocabulary: a card as Scryfall sends it, and
// the handful of facts about card types that everything else agrees on.
//
// Nothing here knows about searching, decks, or the terminal. It is the
// bottom of the dependency graph and stays that way.
package mtg

import (
	"strconv"
	"strings"
)

// SearchResponse is a page of results from Scryfall's search endpoint.
type SearchResponse struct {
	Data       []Card `json:"data"`
	TotalCards int    `json:"total_cards"`
	HasMore    bool   `json:"has_more"`
	NextPage   string `json:"next_page"`
}

type Card struct {
	ID              string `json:"id"`
	OracleID        string `json:"oracle_id"`
	PrintsSearchURI string `json:"prints_search_uri"`
	Set             string `json:"set"`
	CollectorNumber string `json:"collector_number"`
	ReleasedAt      string `json:"released_at"`
	Lang            string `json:"lang"`
	Digital         bool   `json:"digital"`

	// What a printing looks like, and how ordinary a printing it is: gx
	// shows the card as most recently printed, but not as a promo, a
	// borderless showcase or some other special treatment.
	ScryfallURI  string    `json:"scryfall_uri"`
	ImageURIs    ImageURIs `json:"image_uris"`
	SetType      string    `json:"set_type"`
	BorderColor  string    `json:"border_color"`
	Frame        string    `json:"frame"`
	FrameEffects []string  `json:"frame_effects"`
	Promo        bool      `json:"promo"`
	FullArt      bool      `json:"full_art"`
	Variation    bool      `json:"variation"`
	Games        []string  `json:"games"`
	// IllustrationID names the artwork, which printings share when they
	// reprint the same picture. A double-faced card has one per face.
	IllustrationID string `json:"illustration_id"`

	Name          string            `json:"name"`
	ManaCost      string            `json:"mana_cost"`
	TypeLine      string            `json:"type_line"`
	OracleText    string            `json:"oracle_text"`
	Colors        []string          `json:"colors"`
	ColorIdentity []string          `json:"color_identity"`
	Power         string            `json:"power"`
	Toughness     string            `json:"toughness"`
	Loyalty       string            `json:"loyalty"`
	SetName       string            `json:"set_name"`
	Rarity        string            `json:"rarity"`
	RulingsURI    string            `json:"rulings_uri"`
	Legalities    map[string]string `json:"legalities"`
	CMC           float64           `json:"cmc"`
	EDHRECRank    int               `json:"edhrec_rank"`
	Keywords      []string          `json:"keywords"`
	Prices        Prices            `json:"prices"`

	// Transforming and modal double-faced cards carry no top-level oracle
	// text, mana cost or colors at all — it's per face.
	CardFaces []Face `json:"card_faces"`
}

type Face struct {
	Name       string   `json:"name"`
	ManaCost   string   `json:"mana_cost"`
	TypeLine   string   `json:"type_line"`
	OracleText string   `json:"oracle_text"`
	Colors     []string `json:"colors"`
	Power      string   `json:"power"`
	Toughness  string   `json:"toughness"`
	Loyalty    string   `json:"loyalty"`
	// A double-faced card has an image per face and none of its own.
	ImageURIs      ImageURIs `json:"image_uris"`
	IllustrationID string    `json:"illustration_id"`
}

// ImageURIs are the sizes Scryfall renders a printing at.
type ImageURIs struct {
	Normal string `json:"normal"`
	Large  string `json:"large"`
	PNG    string `json:"png"`
	// BorderCrop is the card with its rounded corners cut off: square, so
	// nothing shows behind the corners on a dark terminal.
	BorderCrop string `json:"border_crop"`
}

// Image is the link to a printing's picture at the size asked for —
// "normal", "large" or "border_crop" — or its front face's, for a card that
// has one per face.
func (c Card) Image(size string) string {
	pick := func(u ImageURIs) string {
		if size == "large" && u.Large != "" {
			return u.Large
		}
		if size == "border_crop" && u.BorderCrop != "" {
			return u.BorderCrop
		}
		return u.Normal
	}
	if u := pick(c.ImageURIs); u != "" {
		return u
	}
	for _, f := range c.CardFaces {
		if u := pick(f.ImageURIs); u != "" {
			return u
		}
	}
	return ""
}

// PictureFaces is how many pictures a printing has: one per face for a
// card printed on both sides, one for everything else — split, flip and
// adventure cards share one picture between their faces.
func (c Card) PictureFaces() int {
	if c.ImageURIs.Normal != "" || len(c.CardFaces) < 2 || c.CardFaces[1].ImageURIs.Normal == "" {
		return 1
	}
	return len(c.CardFaces)
}

// FaceImage is Image for one face's picture, face 0 being the front.
func (c Card) FaceImage(face int, size string) string {
	if face <= 0 || face >= c.PictureFaces() {
		return c.Image(size)
	}
	return Card{ImageURIs: c.CardFaces[face].ImageURIs}.Image(size)
}

// Artwork is the printing's artwork, or its front face's.
func (c Card) Artwork() string {
	if c.IllustrationID != "" || len(c.CardFaces) == 0 {
		return c.IllustrationID
	}
	return c.CardFaces[0].IllustrationID
}

// Faces returns a card's printed faces as cards in their own right, so
// anything that renders a card can work a face at a time. A single-faced
// card comes back as itself.
func (c Card) Faces() []Card {
	if len(c.CardFaces) < 2 {
		return []Card{c}
	}
	out := make([]Card, 0, len(c.CardFaces))
	for _, f := range c.CardFaces {
		fc := c
		fc.CardFaces = nil
		fc.Name = f.Name
		fc.ManaCost = f.ManaCost
		fc.TypeLine = f.TypeLine
		fc.OracleText = f.OracleText
		fc.Power = f.Power
		fc.Toughness = f.Toughness
		fc.Loyalty = f.Loyalty
		if len(f.Colors) > 0 {
			fc.Colors = f.Colors
		}
		out = append(out, fc)
	}
	return out
}

// CombinedOracle is every face's text at once, for the places that match
// against a card's wording rather than display it.
func (c Card) CombinedOracle() string {
	if len(c.CardFaces) < 2 {
		return c.OracleText
	}
	var parts []string
	for _, f := range c.CardFaces {
		if f.OracleText != "" {
			parts = append(parts, f.OracleText)
		}
	}
	return strings.Join(parts, "\n")
}

// DisplayColors and DisplayManaCost fall back to the front face, which is
// where a transforming card keeps them.
func (c Card) DisplayColors() []string {
	if len(c.Colors) > 0 || len(c.CardFaces) == 0 {
		return c.Colors
	}
	return c.CardFaces[0].Colors
}

func (c Card) DisplayManaCost() string {
	if c.ManaCost != "" || len(c.CardFaces) == 0 {
		return c.ManaCost
	}
	return c.CardFaces[0].ManaCost
}

// Prices is what Scryfall last saw the card sell for, in a few currencies. We
// only read the non-foil US-dollar figure — it's the one most decklists are
// costed in — and it arrives as a string so that a card with no known price is
// an empty field rather than a misleading zero.
type Prices struct {
	USD     string `json:"usd"`
	USDFoil string `json:"usd_foil"`
}

// USD is the card's dollar price as a number, and whether it had one at all. A
// card Scryfall has no price for returns (0, false), so it can be told apart
// from a card that genuinely costs nothing.
func (c Card) USD() (float64, bool) {
	s := c.Prices.USD
	if s == "" {
		s = c.Prices.USDFoil // a foil-only card still has a price worth showing
	}
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

type Ruling struct {
	Source  string `json:"source"`
	Comment string `json:"comment"`
}

type RulingsResponse struct {
	Data []Ruling `json:"data"`
}
