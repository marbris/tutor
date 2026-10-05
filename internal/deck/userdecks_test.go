package deck

import (
	"testing"
)

func TestUserDecksRoundTrip(t *testing.T) {
	isolate(t)

	if _, ok := CachedUserDecks(LoadUserDecks(), "MarBri"); ok {
		t.Fatal("a cache out of nowhere")
	}
	if err := SaveUserDecks("MarBri", []UserDeck{{Name: "Elf Ball", ID: "a", Cards: 100}}); err != nil {
		t.Fatal(err)
	}
	// Names are matched however they're cased.
	got, ok := CachedUserDecks(LoadUserDecks(), "marbri")
	if !ok || len(got.Decks) != 1 || got.Decks[0].Name != "Elf Ball" {
		t.Fatalf("got %+v", got)
	}

	if err := ForgetUserDecks("MARBRI"); err != nil {
		t.Fatal(err)
	}
	if _, ok := CachedUserDecks(LoadUserDecks(), "MarBri"); ok {
		t.Error("forgetting kept the list")
	}
}
