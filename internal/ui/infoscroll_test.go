package ui

import (
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
	"ttr/internal/rules"
)

// longInfo is a search whose highlighted card has far more to read than the
// information panel has room for: a dozen rulings, each its own paragraph.
func longInfo(t *testing.T) Model {
	t.Helper()
	m := withCards(sized(120, 20), "f", sample(), sortArrival)
	l := m.ws.current().cardsView()
	c, ok := l.current()
	if !ok {
		t.Fatal("no card under the cursor")
	}
	if l.rulings == nil {
		l.rulings = map[string][]mtg.Ruling{}
	}
	var rs []mtg.Ruling
	for i := 0; i < 12; i++ {
		rs = append(rs, mtg.Ruling{Source: "wotc", Comment: "Ruling number " + itoa(i) +
			" says something long enough to wrap onto a second line of the panel."})
	}
	l.rulings[c.Card.ID] = rs
	if _, _, most, _ := m.infoSpan(); most == 0 {
		t.Fatal("the fixture fits the panel; nothing to scroll")
	}
	return m
}

func TestShiftJScrollsHalfTheInfoPanel(t *testing.T) {
	m := longInfo(t)
	_, room, most, _ := m.infoSpan()

	m = drive(m, "J")
	if got, want := m.info.offset, minInt(room/2, most); got != want {
		t.Errorf("J scrolled to %d, want half of %d rows = %d", got, room, want)
	}
	m = drive(m, "K")
	if m.info.offset != 0 {
		t.Errorf("K came back to %d, want 0", m.info.offset)
	}
}

func TestShiftJStopsAtTheBottom(t *testing.T) {
	m := longInfo(t)
	_, _, most, _ := m.infoSpan()
	for i := 0; i < 40; i++ {
		m = drive(m, "J")
	}
	if m.info.offset != most {
		t.Fatalf("offset %d, want the bottom %d", m.info.offset, most)
	}
	// One K is one move up — nothing to unwind first.
	m = drive(m, "K")
	if m.info.offset >= most {
		t.Errorf("K at the bottom didn't move: %d", m.info.offset)
	}
}

func TestTheInfoPanelHintsHalfAPage(t *testing.T) {
	m := drive(longInfo(t), "?")
	if !strings.Contains(stripANSI(m.View()), "half page") {
		t.Error("K J aren't offered as half a page")
	}
}

func TestPowerToughnessSitsUnderTheTextOnTheRight(t *testing.T) {
	// Where the card prints it: after the rules text, in the bottom-right.
	c := deck.Card{Qty: 1, Card: mtg.Card{
		Name: "Grizzly Bears", TypeLine: "Creature — Bear",
		OracleText: "Trample", Power: "2", Toughness: "2",
	}}
	const width = 30
	lines := cardInfo(c, width, rules.Data{}, []mtg.Ruling{}, nil, nil)

	text, pt := -1, -1
	for i, l := range lines {
		plain := stripANSI(l)
		switch {
		case strings.Contains(plain, "Trample"):
			text = i
		case strings.TrimSpace(plain) == "2/2":
			pt = i
			if !strings.HasSuffix(plain, "2/2") || textWidth(plain) != width {
				t.Errorf("2/2 isn't flush right: %q", plain)
			}
		}
	}
	if text < 0 || pt < 0 {
		t.Fatalf("missing the text (%d) or the 2/2 (%d)", text, pt)
	}
	if pt != text+1 {
		t.Errorf("2/2 on line %d, want straight after the text on %d", pt, text)
	}
}
