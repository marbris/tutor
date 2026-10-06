package ui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/fetch"
	"ttr/internal/mtg"
)

// gX: every card's picture in the list, fetched ahead.
//
// gx fetches a picture when the cursor rests on a card, which is a pause on
// every card when you walk a list in the printing view. gX takes the pauses
// up front: the picture gx would show for each card, one card at a time from
// the one under the cursor to the end and round to the top — so the cards
// nearest you arrive first — with the notice counting what has come down.
// A picture already on disk is read and converted to the PNG the terminal
// takes, so walking the list afterwards is instant either way.

// prefetchState is the gX in progress. seq rises with every gX, so a
// second one supersedes the first rather than racing it.
type prefetchState struct {
	seq   int
	panel int
	cards []mtg.Card
	next  int
	bytes int
	// failed is how many cards had no picture to be had.
	failed int
}

type prefetchMsg struct {
	panel, seq int
	img        imageMsg
	bytes      int
}

// loadPrintingsFor is loadPrintings, as a variable so the tests needn't go
// to Scryfall.
var loadPrintingsFor = loadPrintings

// gxAll is gX on a card list.
func (m *Model) gxAll(p *panel) tea.Cmd {
	l := p.cardsView()
	if l == nil || len(l.rows) == 0 {
		m.notice = "gX fetches the pictures of a list of cards"
		return nil
	}
	if !kittyGraphics() {
		m.notice = "this terminal can't draw pictures"
		return nil
	}
	m.info.mode = infoImage
	m.info.prev = infoCard
	m.info.offset = 0

	// From the cursor down, then round from the top; each card once.
	var cards []mtg.Card
	seen := map[string]bool{}
	for i := range l.rows {
		c := l.rows[(l.cursor.at+i)%len(l.rows)].Card
		if c.Name == "" || seen[imageKey(c)] {
			continue
		}
		seen[imageKey(c)] = true
		cards = append(cards, c)
	}
	m.prefetch = prefetchState{seq: m.prefetch.seq + 1, panel: p.id, cards: cards}
	return m.prefetchNext()
}

// prefetchNext starts on the next card that isn't already in hand or on its
// way, or reports that gX is done.
func (m *Model) prefetchNext() tea.Cmd {
	pf := &m.prefetch
	for pf.next < len(pf.cards) {
		c := pf.cards[pf.next]
		pf.next++
		key := imageKey(c)
		if cp, ok := m.images[key]; ok && cp.state != imgFailed {
			continue
		}
		m.images[key] = &cardPrintings{state: imgFetching, local: keptOnDisk(c)}
		m.notice = m.prefetchProgress()
		panel, seq := pf.panel, pf.seq
		// No pause here: fetch keeps Scryfall's pace, and a card already on
		// disk needs none — gX over a cached deck just reads and converts.
		return func() tea.Msg {
			img, n := loadPrintingsFor(c)
			return prefetchMsg{panel: panel, seq: seq, img: img, bytes: n}
		}
	}
	if len(pf.cards) > 0 {
		m.notice = itoa(len(pf.cards)) + " " + plural("picture", len(pf.cards)) +
			" in hand · " + byteText(pf.bytes) + " downloaded"
		if pf.failed > 0 {
			m.notice += " · " + itoa(pf.failed) + " with no picture"
		}
	}
	pf.cards = nil
	return nil
}

// prefetchProgress is the notice while gX runs: how far, and how much.
func (m Model) prefetchProgress() string {
	pf := m.prefetch
	return "pictures " + itoa(pf.next) + "/" + itoa(len(pf.cards)) + " · " + byteText(pf.bytes)
}

func (m Model) handlePrefetch(msg prefetchMsg) (tea.Model, tea.Cmd) {
	// What arrived is good whichever gX asked for it.
	m.storeImage(msg.img)
	if msg.seq != m.prefetch.seq || m.ws.byID(msg.panel) == nil {
		return m, nil
	}
	m.prefetch.bytes += msg.bytes
	if msg.img.err != nil {
		m.prefetch.failed++
	}
	// Told to slow down, gX stops rather than run down the rest of the list
	// asking again: every further request only lengthens the wait.
	var limited fetch.RateLimited
	if errors.As(msg.img.err, &limited) {
		// The card it was on is free to be asked for again later.
		delete(m.images, msg.img.key)
		m.notice = "gX stopped at " + itoa(m.prefetch.next) + "/" + itoa(len(m.prefetch.cards)) +
			": " + limited.Error()
		m.prefetch.cards = nil
		return m, nil
	}
	cmd := m.prefetchNext()
	return m, cmd
}

// byteText is a download's size the way you'd say it.
func byteText(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%d KB", n/1_000)
	}
	return itoa(n) + " B"
}
