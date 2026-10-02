package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg" // Scryfall's pictures are JPEGs
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ttr/internal/browser"
	"ttr/internal/fetch"
	"ttr/internal/mtg"
	"ttr/internal/paths"
	"ttr/internal/prints"
	"ttr/internal/theme"
)

// gx: the card as printed, in the information panel.
//
// A terminal that speaks kitty's graphics protocol — kitty, ghostty, WezTerm
// — can draw a picture in among the text, and there gx shows the printing
// right where the card's text was. Anywhere else, gx opens the picture in the
// browser: a card you can't see is worse than a card in another window.
//
// The picture is drawn with kitty's Unicode placeholders rather than placed
// at a screen position. The image is sent once, off to the side, as a
// "virtual placement"; the panel then draws it with ordinary characters —
// one special character a cell, coloured with the image's number — which the
// terminal replaces with the picture. They are text as far as everything
// else is concerned, so the picture moves, scrolls and is overdrawn exactly
// the way the text around it is, and Bubbletea's redrawing never has to know
// there is a picture at all.

// kittyImageID is the one image ttr keeps in the terminal. It is carried in
// the placeholder's 256-colour foreground, so it has to be under 256; one
// picture is on screen at a time, so one number is all it needs.
const kittyImageID = 219

// imageDelay is how long the cursor rests on a card before its picture is
// asked for, so walking down a list with j isn't a download per card.
const imageDelay = 250 * time.Millisecond

// kittyGraphics reports whether the terminal can draw pictures. A variable,
// so the tests can decide.
var kittyGraphics = func() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty" {
		return true
	}
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "ghostty", "wezterm":
		return true
	}
	return false
}

// writeTerminal sends bytes to the terminal outside Bubbletea's drawing, for
// the image itself. One write, so it can't be split by a redraw going out at
// the same moment. A variable, so the tests can catch it.
var writeTerminal = func(b []byte) { os.Stdout.Write(b) }

type imgState int

const (
	imgFetching imgState = iota
	imgReady
	imgFailed
)

// cardPrintings is every printing of one card, newest first, and which of
// them is up: the most normal one to start with, then wherever H and L
// have taken it.
type cardPrintings struct {
	state imgState
	list  []mtg.Card
	at    int
	err   error
}

// current is the printing on show, if the list is in.
func (cp *cardPrintings) current() (mtg.Card, bool) {
	if cp == nil || cp.state != imgReady || cp.at < 0 || cp.at >= len(cp.list) {
		return mtg.Card{}, false
	}
	return cp.list[cp.at], true
}

// picture is one printing's picture, and how far along getting it is.
type picture struct {
	state imgState
	png   []byte
	w, h  int // in pixels
	err   error
}

// kittyShown is what the terminal is holding under kittyImageID: which
// printing, at what size in cells. The placeholders only draw once it's
// there.
type kittyShown struct {
	key        string
	cols, rows int
}

// imageMsg is a card's printings and the picture of the one chosen, fetched
// together since the one is no use without the other.
type imageMsg struct {
	key     string
	list    []mtg.Card
	at      int
	picture picture
	err     error
}

// pictureMsg is one printing's picture, for H and L.
type pictureMsg struct {
	id      string
	picture picture
}

type imageTickMsg struct {
	key string
	seq int
}

// imageKey is what a card's printings are filed under: the card, not the
// printing, since which printing is shown is the answer, not the question.
func imageKey(c mtg.Card) string {
	if c.OracleID != "" {
		return c.OracleID
	}
	return strings.ToLower(c.Name)
}

// gxCard is gx on a list of cards: its picture in the information panel, or
// in the browser where the terminal can't draw one.
func (m *Model) gxCard(c mtg.Card) tea.Cmd {
	if c.Name == "" {
		return nil
	}
	if !kittyGraphics() {
		m.notice = "this terminal can't draw pictures — opening it in the browser"
		return openPrintingInBrowser(c)
	}
	m.info.mode = infoImage
	m.info.offset = 0
	return m.fetchImage(c)
}

// loadPrintings is the slow part of gx, off the main loop: every printing,
// the one to show, and its picture — and how many bytes that took, which
// gX adds up.
func loadPrintings(c mtg.Card) (imageMsg, int) {
	key := imageKey(c)
	list, size, err := prints.All(c)
	if err != nil {
		return imageMsg{key: key, err: err}, size
	}
	at, ok := prints.PreferIndex(list, time.Now().Format("2006-01-02"))
	if !ok {
		return imageMsg{key: key, err: fmt.Errorf("no picture of %s", c.Name)}, size
	}
	pic, got := loadPicture(list[at])
	return imageMsg{key: key, list: list, at: at, picture: pic}, size + got
}

// loadPicture is one printing's picture, and the bytes downloaded for it.
func loadPicture(p mtg.Card) (picture, int) {
	data, w, h, got, err := printingPNG(p)
	if err != nil {
		return picture{state: imgFailed, err: err}, got
	}
	return picture{state: imgReady, png: data, w: w, h: h}, got
}

// fetchImage asks for a card's printings and picture, unless it has them or
// is asking.
func (m *Model) fetchImage(c mtg.Card) tea.Cmd {
	key := imageKey(c)
	if cp, ok := m.images[key]; ok && cp.state != imgFailed {
		return m.fetchShown(cp)
	}
	m.images[key] = &cardPrintings{state: imgFetching}
	return func() tea.Msg {
		msg, _ := loadPrintingsFor(c)
		return msg
	}
}

// fetchShown asks for the picture of the printing on show, if it isn't in
// hand or on its way.
func (m *Model) fetchShown(cp *cardPrintings) tea.Cmd {
	p, ok := cp.current()
	if !ok {
		return nil
	}
	if pic, ok := m.pictures[p.ID]; ok && pic.state != imgFailed {
		return nil
	}
	m.pictures[p.ID] = &picture{state: imgFetching}
	return func() tea.Msg {
		pic, _ := loadPicture(p)
		return pictureMsg{id: p.ID, picture: pic}
	}
}

// imageHover follows the cursor while the picture is up: rest on a card and
// its picture is asked for.
func (m *Model) imageHover() tea.Cmd {
	if m.info.mode != infoImage {
		return nil
	}
	c := m.focusedCard()
	if c == nil {
		return nil
	}
	key := imageKey((*c))
	if _, ok := m.images[key]; ok {
		return nil
	}
	m.imageSeq++
	seq := m.imageSeq
	return tea.Tick(imageDelay, func(time.Time) tea.Msg { return imageTickMsg{key: key, seq: seq} })
}

func (m Model) handleImageTick(msg imageTickMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.imageSeq || m.info.mode != infoImage {
		return m, nil
	}
	c := m.focusedCard()
	if c == nil || imageKey((*c)) != msg.key {
		return m, nil
	}
	cmd := m.fetchImage((*c))
	return m, cmd
}

func (m Model) handleImage(msg imageMsg) (tea.Model, tea.Cmd) {
	m.storeImage(msg)
	return m, nil
}

// storeImage files a card's printings and its picture.
func (m *Model) storeImage(msg imageMsg) {
	if msg.err != nil {
		m.images[msg.key] = &cardPrintings{state: imgFailed, err: msg.err}
		return
	}
	m.images[msg.key] = &cardPrintings{state: imgReady, list: msg.list, at: msg.at}
	pic := msg.picture
	m.pictures[msg.list[msg.at].ID] = &pic
}

func (m Model) handlePicture(msg pictureMsg) (tea.Model, tea.Cmd) {
	pic := msg.picture
	m.pictures[msg.id] = &pic
	return m, nil
}

// shown is the focused card's printings, the printing on show and its
// picture, as far as each is known.
func (m Model) shown() (*cardPrintings, mtg.Card, *picture) {
	c := m.focusedCard()
	if c == nil {
		return nil, mtg.Card{}, nil
	}
	cp := m.images[imageKey(*c)]
	p, ok := cp.current()
	if !ok {
		return cp, mtg.Card{}, nil
	}
	return cp, p, m.pictures[p.ID]
}

// stepPrinting is H and L in the printing view: an older printing of the
// card, or a newer one. The list is newest first, so older is further on.
func (m *Model) stepPrinting(older bool) tea.Cmd {
	if m.info.mode != infoImage {
		return nil
	}
	cp, _, _ := m.shown()
	if cp == nil || cp.state != imgReady {
		return nil
	}
	switch {
	case older && cp.at >= len(cp.list)-1:
		m.notice = "that's the oldest printing"
		return nil
	case !older && cp.at <= 0:
		m.notice = "that's the newest printing"
		return nil
	case older:
		cp.at++
	default:
		cp.at--
	}
	return m.fetchShown(cp)
}

// printingPNG is a printing's picture as a PNG — the one format kitty takes
// without being told the pixel size — and how many bytes were downloaded
// for it (none, from the cache). A variable, so the tests needn't go to
// Scryfall.
//
// The cache keeps Scryfall's JPEG, not the PNG: a tenth of the size, and
// turning one into the other takes about 50ms, off the main loop and once a
// run per picture. Pictures cached as PNGs by older versions are still used
// as they are.
var printingPNG = func(p mtg.Card) ([]byte, int, int, int, error) {
	dir := filepath.Join(paths.Cache(), "images")
	if data, err := os.ReadFile(filepath.Join(dir, p.ID+".png")); err == nil {
		if cfg, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
			return data, cfg.Width, cfg.Height, 0, nil
		}
	}

	jpg := filepath.Join(dir, p.ID+".jpg")
	raw, err := os.ReadFile(jpg)
	got := 0
	if err != nil {
		raw, err = fetch.GetFile(p.Image("normal"))
		if err != nil {
			return nil, 0, 0, 0, err
		}
		got = len(raw)
		// The cache is a convenience: failing to keep a copy loses nothing.
		if os.MkdirAll(dir, 0755) == nil {
			os.WriteFile(jpg, raw, 0644)
		}
	}
	data, w, h, err := toPNG(raw)
	if err != nil {
		os.Remove(jpg) // a broken copy would only fail again next time
		return nil, 0, 0, got, err
	}
	return data, w, h, got, nil
}

// toPNG turns a picture in any format the image package reads into a PNG,
// with its size in pixels.
func toPNG(raw []byte) ([]byte, int, int, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	return buf.Bytes(), b.Dx(), b.Dy(), nil
}

// openPrintingInBrowser finds the printing gx would show, and opens its
// picture in the browser.
func openPrintingInBrowser(c mtg.Card) tea.Cmd {
	return func() tea.Msg {
		p, err := prints.Preferred(c)
		if err != nil {
			return noticeMsg{err: err}
		}
		if err := browser.Open(p.Image("large")); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "opened " + p.Name + " (" + strings.ToUpper(p.Set) + ") in the browser"}
	}
}

// openInBrowser opens a link, reporting back only if it couldn't.
func openInBrowser(url, what string) tea.Cmd {
	return func() tea.Msg {
		if err := browser.Open(url); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "opened " + what + " in the browser"}
	}
}

// ── Drawing ─────────────────────────────────────────────────────

// imageFit is the size in cells a card's picture takes in a space: the
// full width if the height allows, otherwise the full height. aspect is how
// many times taller than wide a cell is.
func imageFit(w, h, cols, rows int, aspect float64) (int, int) {
	if w <= 0 || h <= 0 || cols <= 0 || rows <= 0 || aspect <= 0 {
		return 0, 0
	}
	fitRows := int(float64(cols)*float64(h)/float64(w)/aspect + 0.5)
	if fitRows <= rows {
		return cols, maxInt(fitRows, 1)
	}
	fitCols := int(float64(rows)*float64(w)*aspect/float64(h) + 0.5)
	return maxInt(minInt(fitCols, cols), 1), rows
}

// syncImage keeps the terminal holding the picture the panel wants to draw,
// at the size it wants to draw it: sent when it changes, and taken back when
// the panel stops showing pictures.
func (m Model) syncImage() (Model, tea.Cmd) {
	want := kittyShown{}
	var img *picture
	if m.info.mode == infoImage {
		if _, p, pic := m.shown(); pic != nil && pic.state == imgReady {
			if inner, room, _, ok := m.infoSpan(); ok {
				cols, rows := imageFit(pic.w, pic.h, inner, room-imageCaptionRows, cellAspect())
				if cols > 0 && rows > 0 && rows <= len(placeholderDiacritics) {
					want = kittyShown{p.ID, cols, rows}
					img = pic
				}
			}
		}
	}
	if want == m.kitty {
		return m, nil
	}
	m.kitty = want
	if img == nil {
		if !kittyGraphics() {
			return m, nil
		}
		return m, func() tea.Msg { writeTerminal([]byte(kittyDelete())); return nil }
	}
	payload := kittyDelete() + kittyTransmit(img.png, want.cols, want.rows)
	return m, func() tea.Msg { writeTerminal([]byte(payload)); return nil }
}

// imageCaptionRows is the room under the picture for which printing it is.
const imageCaptionRows = 2

func kittyDelete() string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", kittyImageID)
}

// kittyTransmit sends a PNG as a virtual placement of cols×rows cells, in
// the chunks the protocol asks for. q=2 keeps the terminal from answering,
// which would otherwise arrive as keypresses.
func kittyTransmit(data []byte, cols, rows int) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	const chunk = 4096
	for i := 0; i < len(enc); i += chunk {
		end := minInt(i+chunk, len(enc))
		more := 1
		if end == len(enc) {
			more = 0
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=T,U=1,f=100,q=2,i=%d,c=%d,r=%d,m=%d;%s\x1b\\",
				kittyImageID, cols, rows, more, enc[i:end])
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, enc[i:end])
		}
	}
	return b.String()
}

// placeholderRows are the lines that draw the picture: each row's first cell
// names its row and column with diacritics, and the rest inherit from the
// cell to their left, a column further along.
func placeholderRows(cols, rows int) []string {
	out := make([]string, rows)
	cell := string(rune(0x10EEEE))
	for r := 0; r < rows; r++ {
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b[38;5;%dm", kittyImageID)
		b.WriteString(cell)
		b.WriteRune(placeholderDiacritics[r])
		b.WriteRune(placeholderDiacritics[0])
		b.WriteString(strings.Repeat(cell, cols-1))
		b.WriteString("\x1b[39m")
		out[r] = b.String()
	}
	return out
}

// infoImageLines is the information panel with the picture up: the
// picture, which printing it is, and then what is worth knowing about the
// card — its price in this printing among it — for J to scroll down to.
func (m Model) infoImageLines(width int) []string {
	muted := lipgloss.NewStyle().Foreground(theme.TextMuted)
	c := m.focusedCard()
	if c == nil {
		return wrapStyled("no card here", width, muted)
	}
	cp, p, pic := m.shown()
	switch {
	case cp == nil || cp.state == imgFetching:
		return wrapStyled("fetching "+c.Name+"…", width, muted)
	case cp.state == imgFailed:
		return wrapStyled("no picture: "+errorText(cp.err), width, muted)
	}

	var lines []string
	switch {
	case pic == nil || pic.state == imgFetching:
		lines = wrapStyled("fetching the "+p.SetName+" printing…", width, muted)
	case pic.state == imgFailed:
		lines = wrapStyled("no picture: "+errorText(pic.err), width, muted)
	case m.kitty.key != p.ID:
		lines = wrapStyled("drawing…", width, muted)
	default:
		lines = placeholderRows(m.kitty.cols, m.kitty.rows)
		for i := range lines {
			lines[i] += strings.Repeat(" ", maxInt(width-m.kitty.cols, 0))
		}
	}

	caption := p.SetName + " (" + strings.ToUpper(p.Set) + ")"
	if len(p.ReleasedAt) >= 4 {
		caption += " · " + p.ReleasedAt[:4]
	}
	if p.CollectorNumber != "" {
		caption += " · #" + p.CollectorNumber
	}
	lines = append(lines, "")
	lines = append(lines, wrapStyled(caption, width, lipgloss.NewStyle().Foreground(theme.TextDim))...)
	if len(cp.list) > 1 {
		lines = append(lines, wrapStyled("printing "+itoa(cp.at+1)+" of "+itoa(len(cp.list))+", newest first", width, muted)...)
	}

	if l := m.ws.current().cardsView(); l != nil {
		if dc, ok := l.current(); ok {
			lines = append(lines, cardMeta(dc, p, width, l.rulings[dc.Card.ID], l.rulingErr[dc.Card.ID])...)
		}
	}
	return lines
}
