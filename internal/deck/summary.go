package deck

import (
	"os"
	"time"
)

// Summary is a deck as the picker lists it: enough to draw a row without
// resolving a single card name against Scryfall.
//
// Deliberately cheap. Opening the decks panel reads every deck file, and a
// panel that had to reach the network to tell you what you own would be
// unusable on a train.
type Summary struct {
	Slug     string
	Name     string
	Format   string
	Total    int // cards, counting quantities
	Unique   int // distinct cards
	Modified time.Time
	// Source is the Moxfield deck this was imported from, if any. A local
	// deck with a source can be synced against it again.
	Source string
	// Broken is set when the file wouldn't parse. The row still appears —
	// silently omitting a deck you know you have is worse than showing it
	// with a warning.
	Broken bool
}

// Summaries lists every deck on disk, in no particular order.
func Summaries() ([]Summary, error) {
	slugs, err := List()
	if err != nil {
		return nil, err
	}

	out := make([]Summary, 0, len(slugs))
	for _, slug := range slugs {
		s := Summary{Slug: slug, Name: slugBase(slug)}
		if info, err := os.Stat(Path(slug)); err == nil {
			s.Modified = info.ModTime()
		}

		d, err := Read(slug)
		if err != nil {
			s.Broken = true
			out = append(out, s)
			continue
		}
		if d.Name != "" {
			s.Name = d.Name
		}
		s.Format = d.Format
		s.Source = d.Source
		s.Total, s.Unique = d.Counts()
		out = append(out, s)
	}
	return out, nil
}

// CheckCached works out a deck's legality from what is already in the card
// cache, and its colours while it is there — both want the same resolution
// and doing it twice would double the work for nothing.
//
// A deck whose cards aren't all known comes back unknown.
func CheckCached(slug string) (Legality, []string) {
	d, err := Read(slug)
	if err != nil {
		return Legality{}, nil
	}
	cards, complete := resolveForCheck(d.MainEntries(), d.Format)
	return Check(d.Format, cards, complete), Colours(cards)
}

// Colours is a deck's colour identity: every colour any card in it brings.
func Colours(cards []Card) []string {
	seen := map[string]bool{}
	for _, c := range cards {
		for _, colour := range c.Card.ColorIdentity {
			seen[colour] = true
		}
	}
	var out []string
	for _, c := range []string{"W", "U", "B", "R", "G"} {
		if seen[c] {
			out = append(out, c)
		}
	}
	return out
}
