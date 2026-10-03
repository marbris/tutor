package ui

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ttr/internal/fetch"
	"ttr/internal/mtg"
	"ttr/internal/paths"
)

// withKitty pretends the terminal can draw pictures, and catches what would
// be sent to it.
func withKitty(t *testing.T, can bool) *[]string {
	t.Helper()
	sent := &[]string{}
	oldK, oldW := kittyGraphics, writeTerminal
	kittyGraphics = func() bool { return can }
	writeTerminal = func(b []byte) { *sent = append(*sent, string(b)) }
	t.Cleanup(func() { kittyGraphics, writeTerminal = oldK, oldW })
	return sent
}

func TestImageFitKeepsTheCardsShape(t *testing.T) {
	// 488×680, the size Scryfall's "normal" pictures are.
	cols, rows := imageFit(488, 680, 40, 100, 2)
	if cols != 40 || rows != 28 {
		t.Errorf("40 wide fits %d×%d, want 40×28", cols, rows)
	}
	cols, rows = imageFit(488, 680, 40, 14, 2)
	if rows != 14 || cols != 20 {
		t.Errorf("14 high fits %d×%d, want 20×14", cols, rows)
	}
}

func TestPlaceholdersNameEachRowOnce(t *testing.T) {
	rows := placeholderRows(3, 2)
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	for r, line := range rows {
		if strings.Count(line, string(rune(0x10EEEE))) != 3 {
			t.Errorf("row %d has %d cells", r, strings.Count(line, string(rune(0x10EEEE))))
		}
		if !strings.Contains(line, string(placeholderDiacritics[r])) {
			t.Errorf("row %d doesn't carry its row number", r)
		}
		if !strings.HasPrefix(line, "\x1b[38;5;219m") {
			t.Errorf("row %d isn't coloured with the image's number", r)
		}
	}
	if w := lipgloss.Width(rows[0]); w != 3 {
		t.Errorf("a 3-cell row measures %d wide", w)
	}
}

func TestTransmitIsChunked(t *testing.T) {
	data := make([]byte, 10000)
	got := kittyTransmit(data, 10, 7)
	if !strings.HasPrefix(got, "\x1b_Ga=T,U=1,f=100,q=2,i=219,c=10,r=7,m=1;") {
		t.Errorf("first chunk: %.60q", got)
	}
	if !strings.HasSuffix(got, "\x1b\\") || !strings.Contains(got, "\x1b_Gm=0;") {
		t.Error("the last chunk doesn't close the transmission")
	}
}

func TestGxShowsThePrintingAndSendsItOnce(t *testing.T) {
	sent := withKitty(t, true)
	card := mtg.Card{Name: "Sol Ring", OracleID: "sol"}
	m := withCards(sized(160, 40), "f", sample()[2:3], sortArrival)
	m.ws.current().cardsView().all[0].Card = card
	m.ws.current().cardsView().refresh()
	// As if the picture had already been fetched.
	m.images[imageKey(card)] = &cardPrintings{state: imgReady, list: []mtg.Card{
		{ID: "cmm1", Set: "cmm", SetName: "Commander Masters", ReleasedAt: "2023-08-04"},
	}}
	m.pictures["cmm1"] = &picture{state: imgReady, png: []byte("png"), w: 488, h: 680}

	m = drive(m, "g", "x")
	if m.info.mode != infoImage {
		t.Fatalf("gx left the panel in %v", m.info.mode)
	}
	if m.kitty.key != "cmm1" {
		t.Fatal("the terminal wasn't given the picture")
	}
	view := m.View()
	if !strings.Contains(view, string(rune(0x10EEEE))) {
		t.Error("no placeholders drawn")
	}
	if !strings.Contains(stripANSI(view), "Commander Masters (CMM) · 2023") {
		t.Error("the caption doesn't say which printing")
	}

	// Moving about doesn't send it again; esc takes it down.
	before := m.kitty
	m = drive(m, "?")
	m = drive(m, "?")
	if m.kitty != before {
		t.Error("the picture was resent at the same size")
	}
	m = drive(m, "esc")
	if m.info.mode != infoCard || m.kitty.key != "" {
		t.Error("esc didn't take the picture down")
	}
	_ = sent
}

func TestGxWithoutPicturesOpensTheBrowser(t *testing.T) {
	withKitty(t, false)
	m := withCards(sized(160, 40), "f", sample(), sortArrival)
	next, cmd := m.Update(keyMsg("g"))
	m = next.(Model)
	next, cmd = m.Update(keyMsg("x"))
	m = next.(Model)
	if m.info.mode == infoImage {
		t.Error("a terminal without pictures was put in picture mode")
	}
	if cmd == nil || !strings.Contains(m.notice, "browser") {
		t.Errorf("nothing said it's going to the browser: %q", m.notice)
	}
}

// printingView is a list on Sol Ring in the printing view, with three
// printings known and every picture in hand.
func printingView(t *testing.T, width int) Model {
	t.Helper()
	withKitty(t, true)
	card := mtg.Card{ID: "sol", Name: "Sol Ring", OracleID: "sol", Rarity: "uncommon",
		Legalities: map[string]string{"commander": "legal"}}
	m := withCards(sized(width, 40), "f", sample()[2:3], sortArrival)
	l := m.ws.current().cardsView()
	l.all[0].Card = card
	l.all[0].Tags = []string{"ramp"}
	l.refresh()
	l.rulings["sol"] = []mtg.Ruling{{Comment: "Sol Ring makes two colorless mana."}}
	m.images[imageKey(card)] = &cardPrintings{state: imgReady, at: 1, list: []mtg.Card{
		{ID: "new", Set: "sld", SetName: "Secret Lair Drop", ReleasedAt: "2025-01-01", Rarity: "rare", Prices: mtg.Prices{USD: "40"}},
		{ID: "mid", Set: "cmm", SetName: "Commander Masters", ReleasedAt: "2023-08-04", Rarity: "uncommon", Prices: mtg.Prices{USD: "1.50"}},
		{ID: "old", Set: "lea", SetName: "Limited Edition Alpha", ReleasedAt: "1993-08-05", Rarity: "uncommon", Prices: mtg.Prices{USD: "4000"}},
	}}
	for _, id := range []string{"new", "mid", "old"} {
		m.pictures[id] = &picture{state: imgReady, png: []byte("png"), w: 488, h: 680}
	}
	return drive(m, "g", "x")
}

func TestThePrintingViewShowsTheCardsDetailsUnderThePicture(t *testing.T) {
	m := printingView(t, 160)
	body := stripANSI(strings.Join(m.infoImageLines(40), "\n"))
	for _, want := range []string{"Commander Masters (CMM) · 2023", "artwork 2 of 3",
		"ramp", "$1.50", "uncommon", "legal in", "commander", "rulings", "two colorless mana"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q is missing from the printing view:\n%s", want, body)
		}
	}
}

func TestHAndLStepThroughThePrintings(t *testing.T) {
	m := printingView(t, 160)
	m = drive(m, "H")
	if _, p, _ := m.shown(); p.ID != "old" {
		t.Fatalf("H showed %q, want the older printing", p.ID)
	}
	if m.kitty.key != "old" {
		t.Error("the terminal wasn't given the older picture")
	}
	if body := stripANSI(strings.Join(m.infoImageLines(40), "\n")); !strings.Contains(body, "$4000") {
		t.Errorf("the price didn't follow the printing:\n%s", body)
	}
	m = drive(m, "H")
	if _, p, _ := m.shown(); p.ID != "old" || !strings.Contains(m.notice, "oldest") {
		t.Errorf("H past the oldest went to %q, notice %q", p.ID, m.notice)
	}
	m = drive(m, "L", "L")
	if _, p, _ := m.shown(); p.ID != "new" {
		t.Errorf("L L showed %q, want the newest", p.ID)
	}
}

func TestThePrintingCaptionWrapsRatherThanBeingCut(t *testing.T) {
	m := printingView(t, 160)
	body := stripANSI(strings.Join(m.infoImageLines(16), "\n"))
	if strings.Contains(body, "…") {
		t.Errorf("the caption was cut:\n%s", body)
	}
	if !strings.Contains(body, "Masters (CMM)") {
		t.Errorf("the caption lost its set code:\n%s", body)
	}
}

func TestJScrollsThePrintingView(t *testing.T) {
	m := printingView(t, 160)
	m = drive(m, "J")
	if m.info.offset == 0 {
		t.Error("J didn't scroll the printing view")
	}
}

func TestGXFetchesEveryPictureFromTheCursorRoundAndCountsTheBytes(t *testing.T) {
	withKitty(t, true)
	var asked []string
	old := loadPrintingsFor
	loadPrintingsFor = func(c mtg.Card) (imageMsg, int) {
		asked = append(asked, c.Name)
		p := mtg.Card{ID: c.Name, Name: c.Name}
		return imageMsg{key: imageKey(c), list: []mtg.Card{p},
			picture: picture{state: imgReady, png: []byte("png"), w: 488, h: 680}}, 400_000
	}
	t.Cleanup(func() { loadPrintingsFor = old })

	m := withCards(sized(160, 40), "f", sample(), sortArrival)
	m = drive(m, "j", "j") // on Sol Ring, the third of four
	m, cmd := press(m, "g")
	m, cmd = press(m, "X")
	if m.info.mode != infoImage {
		t.Fatal("gX didn't open the printing view")
	}
	m = settle(m, cmd)

	want := "Sol Ring,Forest,Dwynen, Gilt-Leaf Daen,Llanowar Elves"
	if got := strings.Join(asked, ","); got != want {
		t.Errorf("fetched %s, want from the cursor round: %s", got, want)
	}
	if !strings.Contains(m.notice, "4 pictures") || !strings.Contains(m.notice, "1.6 MB") {
		t.Errorf("notice %q", m.notice)
	}

	// Again, and nothing is fetched twice.
	asked = nil
	m, cmd = press(m, "g")
	m, cmd = press(m, "X")
	m = settle(m, cmd)
	if len(asked) != 0 {
		t.Errorf("a second gX fetched %v again", asked)
	}
}

func TestByteText(t *testing.T) {
	for n, want := range map[int]string{512: "512 B", 2_500: "2 KB", 3_400_000: "3.4 MB"} {
		if got := byteText(n); got != want {
			t.Errorf("byteText(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestGXStopsWhenScryfallSaysSlowDown(t *testing.T) {
	withKitty(t, true)
	var asked []string
	old := loadPrintingsFor
	loadPrintingsFor = func(c mtg.Card) (imageMsg, int) {
		asked = append(asked, c.Name)
		return imageMsg{key: imageKey(c), err: fetch.RateLimited{Until: time.Now().Add(time.Minute)}}, 0
	}
	t.Cleanup(func() { loadPrintingsFor = old })

	m := withCards(sized(160, 40), "f", sample(), sortArrival)
	m, cmd := press(m, "g")
	m, cmd = press(m, "X")
	m = settle(m, cmd)
	if len(asked) != 1 {
		t.Errorf("gX kept asking after a 429: %v", asked)
	}
	if !strings.Contains(m.notice, "stopped") || !strings.Contains(m.notice, "slow down") {
		t.Errorf("notice %q", m.notice)
	}
	if _, kept := m.images[imageKey(sample()[0].Card)]; kept {
		t.Error("the refused card is marked as failed, so it won't be asked for again")
	}
}

func TestPicturesAreTheBorderCropCachedAsTheJPEGScryfallSent(t *testing.T) {
	// A small JPEG, served as if from cards.scryfall.io.
	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 49, 68)), nil)
	hits, asked := 0, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		asked = r.URL.Path
		w.Write(jpg.Bytes())
	}))
	defer srv.Close()
	p := mtg.Card{ID: "jpegtest", ImageURIs: mtg.ImageURIs{
		Normal: srv.URL + "/normal.jpg", BorderCrop: srv.URL + "/crop.jpg"}}

	for i, wantGot := range []int{jpg.Len(), 0} {
		data, w, h, got, err := printingPNG(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil || w != 49 || h != 68 {
			t.Errorf("run %d: not a 49×68 PNG (%d×%d, %v)", i, w, h, err)
		}
		if got != wantGot {
			t.Errorf("run %d: downloaded %d bytes, want %d", i, got, wantGot)
		}
	}
	if hits != 1 {
		t.Errorf("fetched %d times, want once", hits)
	}
	if asked != "/crop.jpg" {
		t.Errorf("asked for %s, want the border crop", asked)
	}
	kept, err := os.ReadFile(filepath.Join(paths.Cache(), "images", "jpegtest.crop.jpg"))
	if err != nil || !bytes.Equal(kept, jpg.Bytes()) {
		t.Errorf("the cache doesn't hold the JPEG as sent (%v)", err)
	}
}

func TestACardKeptOnDiskLoadsAtOnceAndSaysLoading(t *testing.T) {
	withKitty(t, true)
	oldKept, oldLoad := keptOnDisk, loadPrintingsFor
	keptOnDisk = func(mtg.Card) bool { return true }
	loaded := 0
	loadPrintingsFor = func(c mtg.Card) (imageMsg, int) {
		loaded++
		return imageMsg{key: imageKey(c), list: []mtg.Card{{ID: c.Name}},
			picture: picture{state: imgReady, png: []byte("png"), w: 488, h: 680}}, 0
	}
	t.Cleanup(func() { keptOnDisk, loadPrintingsFor = oldKept, oldLoad })

	m := withCards(sized(160, 40), "f", sample(), sortArrival)
	m.info.mode = infoImage
	m, cmd := press(m, "j")
	if c := m.focusedCard(); c == nil || m.images[imageKey(*c)] == nil {
		t.Fatal("the card on disk wasn't asked for straight away")
	}
	if body := stripANSI(strings.Join(m.infoImageLines(40), "\n")); !strings.Contains(body, "loading") {
		t.Errorf("a load from disk says:\n%s", body)
	}
	for _, msg := range messages(cmd) {
		if _, tick := msg.(imageTickMsg); tick {
			t.Error("it still waited for the cursor to rest")
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	if _, p, pic := m.shown(); pic == nil || p.ID != "Llanowar Elves" {
		t.Error("the loaded picture isn't on show")
	}
	if loaded != 1 {
		t.Errorf("loaded %d times", loaded)
	}
}
