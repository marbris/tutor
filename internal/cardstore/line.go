package cardstore

import (
	"strings"
	"time"

	"ttr/internal/mtg"
)

// A card in the cards file: a line with what is the same in every
// printing, then a line with each printing's own fields, newest first. The
// keys are short and empty fields left out: there are a hundred thousand
// printings.
type line struct {
	C oracle
	P []printing
}

type oracle struct {
	ID         string            `json:"o"`
	Name       string            `json:"n"`
	ManaCost   string            `json:"m,omitempty"`
	TypeLine   string            `json:"t,omitempty"`
	Text       string            `json:"x,omitempty"`
	Colors     []string          `json:"c,omitempty"`
	Identity   []string          `json:"ci,omitempty"`
	Power      string            `json:"pw,omitempty"`
	Toughness  string            `json:"th,omitempty"`
	Loyalty    string            `json:"ly,omitempty"`
	CMC        float64           `json:"v,omitempty"`
	EDHREC     int               `json:"e,omitempty"`
	Keywords   []string          `json:"k,omitempty"`
	Legalities map[string]string `json:"l,omitempty"` // not_legal left out
	Faces      []oracleFace      `json:"f,omitempty"`
}

type oracleFace struct {
	Name      string   `json:"n"`
	ManaCost  string   `json:"m,omitempty"`
	TypeLine  string   `json:"t,omitempty"`
	Text      string   `json:"x,omitempty"`
	Colors    []string `json:"c,omitempty"`
	Power     string   `json:"pw,omitempty"`
	Toughness string   `json:"th,omitempty"`
	Loyalty   string   `json:"ly,omitempty"`
}

type printing struct {
	ID           string   `json:"i"`
	Set          string   `json:"s"`
	SetName      string   `json:"sn,omitempty"`
	SetType      string   `json:"st,omitempty"`
	Number       string   `json:"n,omitempty"`
	Released     string   `json:"r,omitempty"`
	Lang         string   `json:"lg,omitempty"` // English left out
	Rarity       string   `json:"y,omitempty"`
	Digital      bool     `json:"d,omitempty"`
	Border       string   `json:"b,omitempty"`
	Frame        string   `json:"fr,omitempty"`
	FrameEffects []string `json:"fe,omitempty"`
	Promo        bool     `json:"p,omitempty"`
	FullArt      bool     `json:"fa,omitempty"`
	Variation    bool     `json:"va,omitempty"`
	Art          string   `json:"a,omitempty"`
	// Crop is the border-crop picture's link; the other sizes are the same
	// link with the size swapped (pictureLinks).
	Crop    string         `json:"img,omitempty"`
	USD     string         `json:"u,omitempty"`
	USDFoil string         `json:"uf,omitempty"`
	Faces   []printingFace `json:"f,omitempty"`
}

type printingFace struct {
	Crop string `json:"img,omitempty"`
	Art  string `json:"a,omitempty"`
}

// pictureLinks is every size of a picture from its border crop's link.
// Scryfall's links differ only in the size's name (checked against every
// printing in the bulk file, 2026-10-19).
func pictureLinks(crop string) mtg.ImageURIs {
	if crop == "" {
		return mtg.ImageURIs{}
	}
	size := func(s string) string { return strings.Replace(crop, "/border_crop/", "/"+s+"/", 1) }
	return mtg.ImageURIs{
		BorderCrop: crop,
		Normal:     size("normal"),
		Large:      size("large"),
		PNG:        strings.Replace(size("png"), ".jpg", ".png", 1),
	}
}

// card is the line as one printing of the card, as Scryfall would send it.
func (l line) card(p int, built time.Time) mtg.Card {
	o := l.C
	c := mtg.Card{
		OracleID: o.ID, Name: o.Name, ManaCost: o.ManaCost, TypeLine: o.TypeLine,
		OracleText: o.Text, Colors: o.Colors, ColorIdentity: o.Identity,
		Power: o.Power, Toughness: o.Toughness, Loyalty: o.Loyalty,
		CMC: o.CMC, EDHRECRank: o.EDHREC, Keywords: o.Keywords, Legalities: o.Legalities,
		PricedAt: built,
		PrintsSearchURI: "https://api.scryfall.com/cards/search?order=released&q=oracleid%3A" +
			o.ID + "&unique=prints",
	}
	// Kept without the formats a card isn't legal in; Scryfall lists them
	// all, and a deck's legality reads "not_legal" as an answer.
	if c.Legalities == nil {
		c.Legalities = make(map[string]string, len(Formats))
	}
	for _, f := range Formats {
		if _, ok := c.Legalities[f]; !ok {
			c.Legalities[f] = "not_legal"
		}
	}
	for _, f := range o.Faces {
		c.CardFaces = append(c.CardFaces, mtg.Face{Name: f.Name, ManaCost: f.ManaCost,
			TypeLine: f.TypeLine, OracleText: f.Text, Colors: f.Colors,
			Power: f.Power, Toughness: f.Toughness, Loyalty: f.Loyalty})
	}
	if p < 0 || p >= len(l.P) {
		return c
	}
	pr := l.P[p]
	c.ID, c.Set, c.SetName, c.SetType = pr.ID, pr.Set, pr.SetName, pr.SetType
	c.CollectorNumber, c.ReleasedAt, c.Rarity, c.Digital = pr.Number, pr.Released, pr.Rarity, pr.Digital
	c.Lang = pr.Lang
	if c.Lang == "" {
		c.Lang = "en"
	}
	c.BorderColor, c.Frame, c.FrameEffects = pr.Border, pr.Frame, pr.FrameEffects
	c.Promo, c.FullArt, c.Variation = pr.Promo, pr.FullArt, pr.Variation
	c.IllustrationID = pr.Art
	c.ImageURIs = pictureLinks(pr.Crop)
	c.Prices = mtg.Prices{USD: pr.USD, USDFoil: pr.USDFoil}
	c.Games = []string{"paper"}
	if pr.Digital {
		c.Games = nil
	}
	c.RulingsURI = "https://api.scryfall.com/cards/" + pr.ID + "/rulings"
	for i := range c.CardFaces {
		if i < len(pr.Faces) {
			c.CardFaces[i].ImageURIs = pictureLinks(pr.Faces[i].Crop)
			c.CardFaces[i].IllustrationID = pr.Faces[i].Art
		}
	}
	return c
}
