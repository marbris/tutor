package prints

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"ttr/internal/diskcache"
	"ttr/internal/fetch"
	"ttr/internal/mtg"
	"ttr/internal/scryfall"
)

// The printing gx shows.
//
// "The card" as a picture is a question with a hundred answers for a staple,
// and most of them are not what you picture: promos, borderless showcases,
// Secret Lair oddities, MTGO's own renderings. What you picture is the card
// as it was printed in an ordinary set — the most recent such printing,
// since its wording is the one in force, but a real and normal printing
// comes before a recent one. A card last printed in 1996 is shown as it was
// printed in 1996, not as a judge promo with its type line cut short.

// ordinarySets are the kinds of set whose printings look like the card.
var ordinarySets = map[string]bool{
	"core": true, "expansion": true, "commander": true, "masters": true,
	"draft_innovation": true, "starter": true,
}

// oddSets are sets whose type says ordinary but whose printings aren't: The
// List's reprints carry a mark in the corner, and the Mystery Boosters are
// playtest oddities and reprints of every kind.
var oddSets = map[string]bool{
	"plst": true, "mb1": true, "mb2": true, "fmb1": true, "pmei": true,
}

// specialFrames are the treatments that make a printing not the card as you
// picture it.
var specialFrames = map[string]bool{
	"showcase": true, "extendedart": true, "etched": true, "inverted": true,
	"textless": true, "fullart": true,
}

// Ordinary reports whether a printing looks like the card: in paper, in
// English, in an ordinary set, with an ordinary frame.
func Ordinary(c mtg.Card) bool {
	// A reprint in an old frame — the retro-frame versions of a remastered
	// set — is a throwback, not the card as it looks now.
	return (c.Frame == "" || c.Frame == "2015") && OrdinaryAnyFrame(c)
}

// OrdinaryAnyFrame is Ordinary in whichever frame the card was printed in:
// a card last printed in Mirage looks like Mirage printed it.
func OrdinaryAnyFrame(c mtg.Card) bool {
	if c.Digital || (c.Lang != "" && c.Lang != "en") {
		return false
	}
	if c.Promo || c.FullArt || c.Variation {
		return false
	}
	if c.BorderColor == "borderless" || c.BorderColor == "gold" || c.BorderColor == "silver" {
		return false
	}
	for _, f := range c.FrameEffects {
		if specialFrames[f] {
			return false
		}
	}
	return ordinarySets[c.SetType] && !oddSets[c.Set]
}

// Prefer picks from printings listed newest first, the most normal one
// there is: an ordinary printing in today's frame, then an ordinary one in
// an older frame, then any paper printing that isn't a promo, then
// anything. A printing from a set not out yet by today — "2006-01-02" — is
// a preview, not the card as printed, and is passed over while there is
// anything else.
func Prefer(printings []mtg.Card, today string) (mtg.Card, bool) {
	i, ok := PreferIndex(printings, today)
	if !ok {
		return mtg.Card{}, false
	}
	return printings[i], true
}

// PreferIndex is Prefer, as where in the list the choice sits.
func PreferIndex(printings []mtg.Card, today string) (int, bool) {
	out := func(c mtg.Card) bool { return c.ReleasedAt == "" || c.ReleasedAt <= today }
	paper := func(c mtg.Card) bool {
		return !c.Digital && !c.Promo && (c.Lang == "" || c.Lang == "en")
	}
	for _, want := range []func(mtg.Card) bool{
		func(c mtg.Card) bool { return out(c) && Ordinary(c) },
		func(c mtg.Card) bool { return out(c) && OrdinaryAnyFrame(c) },
		func(c mtg.Card) bool { return out(c) && paper(c) },
		out,
		func(mtg.Card) bool { return true },
	} {
		for i, c := range printings {
			if want(c) && c.Image("normal") != "" {
				return i, true
			}
		}
	}
	return 0, false
}

// printsQuery is the search for every printing of a card, newest first: by
// its oracle id where it has one, which is exact, otherwise by exact name.
func printsQuery(c mtg.Card) string {
	q := fmt.Sprintf("!%q", c.Name)
	if c.OracleID != "" {
		q = "oracleid:" + c.OracleID
	}
	v := url.Values{}
	v.Set("q", q+" unique:prints")
	v.Set("order", "released")
	v.Set("dir", "desc")
	return "https://api.scryfall.com/cards/search?" + v.Encode()
}

// printingsMaxAge is how long a card's list of printings is trusted: the
// prices in it change once a day.
const printingsMaxAge = 24 * time.Hour

// All is every paper printing of a card, newest first, and how many bytes
// it took to find out — none, when the list kept from last time is still
// good. MTGO's and Arena's own printings are left out — they aren't cards
// anyone holds — unless they are all the card has.
func All(c mtg.Card) ([]mtg.Card, int, error) {
	rel := filepath.Join("printings", diskcache.Key(printsQuery(c)))
	var kept []mtg.Card
	fresh, have := diskcache.Load(rel, printingsMaxAge, &kept)
	if fresh && len(kept) > 0 {
		return kept, 0, nil
	}
	list, size, err := fetchAll(c)
	if err != nil {
		// A day-old price beats no picture at all.
		if have && len(kept) > 0 {
			return kept, size, nil
		}
		return nil, size, err
	}
	diskcache.Save(rel, list)
	return list, size, nil
}

// fetchAll is All, asked of Scryfall.
func fetchAll(c mtg.Card) ([]mtg.Card, int, error) {
	var all []mtg.Card
	size := 0
	for page, next := 0, printsQuery(c); next != ""; page++ {
		if page > 0 {
			time.Sleep(scryfall.PageDelay)
		}
		body, err := fetch.Get(next)
		if err != nil {
			return nil, size, err
		}
		size += len(body)
		var sr mtg.SearchResponse
		if err := json.Unmarshal(body, &sr); err != nil {
			return nil, size, err
		}
		all = append(all, sr.Data...)
		if !sr.HasMore {
			break
		}
		next = sr.NextPage
	}
	paper := paperOnly(all)
	if len(paper) == 0 {
		return nil, size, fmt.Errorf("no printings of %s", c.Name)
	}
	return paper, size, nil
}

// paperOnly drops the printings that exist only on a screen, unless that is
// all there is: an Alchemy card is still a card to look at.
func paperOnly(all []mtg.Card) []mtg.Card {
	var paper []mtg.Card
	for _, p := range all {
		if !p.Digital {
			paper = append(paper, p)
		}
	}
	if len(paper) == 0 {
		return all
	}
	return paper
}

// Preferred finds the printing gx should show for a card.
func Preferred(c mtg.Card) (mtg.Card, error) {
	list, _, err := All(c)
	if err != nil {
		return mtg.Card{}, err
	}
	if best, ok := Prefer(list, time.Now().Format("2006-01-02")); ok {
		return best, nil
	}
	return mtg.Card{}, fmt.Errorf("no picture of %s", c.Name)
}
