package config

import (
	"encoding/json"
	"testing"
)

func TestOtagPrefixDefaultsButCanBeEmpty(t *testing.T) {
	for body, want := range map[string]string{
		`{}`:                    "otag-",
		`{"otag_prefix": ""}`:   "",
		`{"otag_prefix": "o/"}`: "o/",
	} {
		var c Config
		if err := json.Unmarshal([]byte(body), &c); err != nil {
			t.Fatal(err)
		}
		if got := c.Otag(); got != want {
			t.Errorf("%s: got %q, want %q", body, got, want)
		}
	}
}
