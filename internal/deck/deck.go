package deck

import (
	"fmt"
	"os"
	"strings"

	"ttr/internal/mtg"
)

// A deck as the program holds it while you work on it: what it is, and the
// cards in it with their quantities and tags. The file format is in file.go;
// this is what that turns into once the card names have been resolved.

type Info struct {
	Name   string
	Author string
	Format string
	ID     string
	URL    string
	Total  int // cards counting quantities
	Unique int // distinct cards

	// Slug names the file this deck was read from, and is empty for a deck
	// being browsed straight off Moxfield. Editing needs somewhere to write
	// to, so it's what tells an editable deck from a borrowed one.
	Slug string
}

func (d Info) Local() bool { return d.Slug != "" }

// Card is one card in a deck: the card itself, plus what the deck says about
// it.
type Card struct {
	Card      mtg.Card
	Qty       int
	Commander bool
	Tags      []string
	// Borrowed is the tags other lists give the card through the global
	// tags, worked out as it is counted or narrowed. Never written.
	Borrowed []string
}

// AllTags is the card's own tags, then the borrowed ones.
func (d Card) AllTags() []string {
	if len(d.Borrowed) == 0 {
		return d.Tags
	}
	return append(append([]string(nil), d.Tags...), d.Borrowed...)
}

// Section is the heading a card is listed under.
func (d Card) Section() string {
	if d.Commander {
		return "Commander"
	}
	return mtg.PrimaryType(d.Card.TypeLine)
}

// FileFrom builds a deck file out of a deck that's already open, so saving
// what's on screen never needs to fetch it again — and works with no network
// at all.
func FileFrom(info Info, cards []Card) *File {
	d := &File{Name: info.Name, Format: info.Format, Source: info.URL}
	for _, dc := range cards {
		section := "mainboard"
		if dc.Commander {
			section = "commander"
		}
		d.Entries = append(d.Entries, Entry{
			Qty:     dc.Qty,
			Name:    dc.Card.Name,
			Tags:    dc.Tags,
			Section: section,
		})
	}
	return d
}

// Merge builds a deck file out of a deck that's already open, on top of the
// file it was opened from. What the screen doesn't show is kept as the file
// had it: the comments and header keys, the maybeboard and any section of
// your own, which board each card was on, and the printing pinned with
// (set) collector. Only the cards and their counts and tags come from the
// screen. With no file to build on, it is FileFrom.
func Merge(old *File, info Info, cards []Card) *File {
	d := FileFrom(info, cards)
	if old == nil {
		return d
	}
	d.Notes, d.Extra = old.Notes, old.Extra
	if d.Source == "" {
		d.Source = old.Source
	}

	was := map[string]Entry{}
	for _, e := range old.MainEntries() {
		k := strings.ToLower(e.Name)
		if _, seen := was[k]; !seen {
			was[k] = e
		}
	}
	for i, e := range d.Entries {
		o, ok := was[strings.ToLower(e.Name)]
		if !ok {
			front, _, _ := strings.Cut(e.Name, " // ")
			o, ok = was[strings.ToLower(front)]
		}
		if !ok {
			continue
		}
		d.Entries[i].Set, d.Entries[i].Collector = o.Set, o.Collector
		if e.Section == "mainboard" && o.Section != "commander" {
			d.Entries[i].Section = o.Section
		}
	}
	for _, e := range old.Entries {
		if e.Section == "maybeboard" {
			d.Entries = append(d.Entries, e)
		}
	}
	return d
}

// Updated is the file a deck's slug should now hold: the open deck, merged
// onto what is on disk.
func Updated(info Info, cards []Card) *File {
	old, _ := Read(info.Slug)
	return Merge(old, info, cards)
}

// Open reads a deck file and resolves its card names. Cards that won't
// resolve don't stop the deck opening — the error rides alongside the cards
// that did, for the caller to show as a notice.
func Open(slug string) (Info, []Card, error) {
	d, err := Read(slug)
	if err != nil {
		if os.IsNotExist(err) {
			return Info{}, nil, fmt.Errorf("no saved deck %q", slug)
		}
		return Info{}, nil, err
	}

	// An empty deck is a deck — `ttr deck new` makes one, and you fill it
	// by adding cards to it. Only a deck whose cards all failed to resolve
	// is a problem worth refusing to open.
	cards, resolveErr := Resolve(d.MainEntries())
	if len(cards) == 0 && len(d.MainEntries()) > 0 {
		return Info{}, nil, resolveErr
	}

	total, unique := d.Counts()
	info := Info{
		Name:   d.Name,
		Format: d.Format,
		URL:    d.Source,
		Slug:   slug,
		Total:  total,
		Unique: unique,
	}
	return info, cards, resolveErr
}

// MainEntries is the deck proper — everything a maybeboard shortlist isn't.
func (d *File) MainEntries() []Entry {
	out := make([]Entry, 0, len(d.Entries))
	for _, e := range d.Entries {
		if e.Section != "maybeboard" {
			out = append(out, e)
		}
	}
	return out
}
