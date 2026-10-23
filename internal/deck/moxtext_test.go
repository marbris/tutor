package deck

import (
	"strings"
	"testing"
)

func TestMoxfieldText(t *testing.T) {
	d, err := ParseFile(strings.NewReader(`name: Ghen
format: commander

[commander]
1 Ghen, Arcanum Weaver [wincon]

[mainboard]
1 Sol Ring (c21) 263 [ramp, fast mana]
7 Plains

[sideboard]
1 Swords to Plowshares [removal]

[maybeboard]
1 Mana Crypt

[ideas]
1 Lightning Bolt
`))
	if err != nil {
		t.Fatal(err)
	}
	want := `Commander
1 Ghen, Arcanum Weaver #wincon

Deck
7 Plains
1 Sol Ring (C21) 263 #fast mana #ramp

Sideboard
1 Swords to Plowshares #removal

Maybeboard
1 Mana Crypt
`
	if got := MoxfieldText(d); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
