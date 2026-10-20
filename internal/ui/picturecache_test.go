package ui

import (
	"strconv"
	"strings"
	"testing"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

func TestThePictureCacheLetsTheLeastRecentlyUsedGoButNotTheOneOnScreen(t *testing.T) {
	pc := newPictureCache()
	pc.budget = 10
	pic := func() *picture { return &picture{state: imgReady, png: make([]byte, 4)} }
	pc.put("a", pic(), "")
	pc.put("b", pic(), "")
	pc.get("a")
	pc.put("c", pic(), "")
	if _, ok := pc.peek("b"); ok {
		t.Error("b, the least recently used, was kept")
	}
	for _, k := range []string{"a", "c"} {
		if _, ok := pc.peek(k); !ok {
			t.Errorf("%s was let go", k)
		}
	}
	// a is the oldest now, but it's on screen.
	pc.put("d", pic(), "a")
	if _, ok := pc.peek("a"); !ok {
		t.Error("the picture on screen was let go")
	}
	if _, ok := pc.peek("c"); ok {
		t.Error("c should have gone in a's place")
	}
	if pc.bytes != 8 {
		t.Errorf("counted %d bytes, want 8", pc.bytes)
	}
}

func TestAPictureLetGoIsLoadedFromDiskAgain(t *testing.T) {
	m := printingView(t, 160)
	delete(m.pictures.byKey, "mid") // the one on show, let go
	old := printingPNG
	printingPNG = func(p mtg.Card, face int) ([]byte, int, int, int, error) {
		return []byte("again"), 488, 680, 0, nil
	}
	t.Cleanup(func() { printingPNG = old })

	cmd := m.imageHover()
	if cmd == nil {
		t.Fatal("resting on the card didn't ask for its picture again")
	}
	m = settle(m, cmd)
	if _, _, pic := m.shown(); pic == nil || string(pic.png) != "again" {
		t.Errorf("the picture wasn't loaded again: %+v", pic)
	}
}

func TestGXKeepsOnlyTheNearestPicturesInMemory(t *testing.T) {
	withKitty(t, true)
	var converted, kept int
	oldLoad, oldKeep := loadPrintingsFor, keepPrintingsFor
	loadPrintingsFor = func(c mtg.Card) (imageMsg, int) {
		converted++
		return imageMsg{key: imageKey(c), list: []mtg.Card{{ID: c.Name, Name: c.Name}},
			picture: picture{state: imgReady, png: []byte("png"), w: 488, h: 680}}, 0
	}
	keepPrintingsFor = func(c mtg.Card) (imageMsg, int) {
		kept++
		return imageMsg{key: imageKey(c), list: []mtg.Card{{ID: c.Name, Name: c.Name}}, onDisk: true}, 0
	}
	t.Cleanup(func() { loadPrintingsFor, keepPrintingsFor = oldLoad, oldKeep })

	var cards []deck.Card
	for i := range prefetchInHand + 6 {
		name := "Card " + strconv.Itoa(i)
		cards = append(cards, deck.Card{Card: mtg.Card{Name: name, OracleID: name}})
	}
	m := withCards(sized(160, 40), "f", cards, sortArrival)
	m, cmd := press(m, "g")
	m, cmd = press(m, "X")
	m = settle(m, cmd)

	if converted != prefetchInHand || kept != 6 {
		t.Errorf("converted %d and only kept %d, want %d and 6", converted, kept, prefetchInHand)
	}
	if _, ok := m.pictures.peek("Card 29"); ok {
		t.Error("a picture past the first stretch was kept in memory")
	}
	if cp := m.images[imageKey(cards[29].Card)]; cp == nil || cp.state != imgReady {
		t.Error("the printings of a card past the first stretch weren't kept")
	}
	if !strings.Contains(m.notice, "30 pictures ready") {
		t.Errorf("notice %q", m.notice)
	}
}
