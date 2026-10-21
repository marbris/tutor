package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ttr/internal/deck"
	"ttr/internal/mtg"
)

// Holding a key down sends a press every 30 ms or so. A press that takes
// longer than that to answer piles the rest up, and the cursor runs on after
// the key is let go — so a press in a list of every card in three colours,
// with a deck open beside it, has to stay well under that.

func manyCards(n int) []deck.Card {
	types := []string{"Legendary Creature — Elf Faerie Noble", "Instant", "Sorcery",
		"Artifact Creature — Golem", "Enchantment — Aura"}
	colours := [][]string{{"R"}, {"G"}, {"B"}, {"R", "G"}, nil}
	out := make([]deck.Card, n)
	for i := range out {
		out[i] = deck.Card{Card: mtg.Card{
			Name:     fmt.Sprintf("Card Number %d, the Great", i),
			TypeLine: types[i%len(types)], CMC: float64(i % 8), Colors: colours[i%len(colours)],
			ManaCost: "{2}{R}{G}", OracleText: "Flying. When this enters, draw a card.",
		}}
	}
	return out
}

func BenchmarkScrollABigList(b *testing.B) {
	for _, order := range []cardSort{sortArrival, sortName, sortColor, sortType} {
		b.Run(order.String(), func(b *testing.B) {
			m := sized(220, 55)
			big := m.ws.open(KindFind)
			big.searchOpen = false
			big.show(newCardList(manyCards(20713), order, ""))
			beside := m.ws.open(KindFind)
			beside.searchOpen = false
			beside.show(newCardList(manyCards(100), order, ""))
			m.ws.focus(0)
			_ = m.View()

			down := tea.KeyMsg{Type: tea.KeyDown}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				next, _ := m.Update(down)
				m = next.(Model)
				_ = m.View()
			}
		})
	}
}

func TestTheFlagsFollowACardAddedBeside(t *testing.T) {
	// What a list holds is kept between frames; a card added to the list
	// beside it still has to light up here.
	m := sized(220, 55)
	big := newCardList(manyCards(50), sortName, "")
	m.ws.open(KindFind).show(big)
	beside := newCardList(manyCards(1), sortName, "")
	m.ws.open(KindFind).show(beside)

	added := markKey(big.all[7])
	if m.membersFor(big)[added] != 0 {
		t.Fatalf("flagged before it was added")
	}
	beside.all = append(beside.all, big.all[7])
	beside.refresh()
	if m.membersFor(big)[added] == 0 {
		t.Error("the card added beside isn't flagged in the big list")
	}
	if _, kept := m.membersFor(beside)[markKey(manyCards(100)[99])]; kept {
		t.Error("a list's flags hold a card it doesn't have")
	}
}
