package catalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ttr/internal/paths"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ttr-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}

// serve stands in for Scryfall's catalogs, counting requests.
func serve(t *testing.T) *int {
	t.Helper()
	lists := map[string][]string{
		"creature-types":    {"Elf", "Golem"},
		"artifact-types":    {"Equipment"},
		"land-types":        {"Forest"},
		"keyword-abilities": {"Forestwalk", "Flying"},
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		name := strings.TrimPrefix(r.URL.Path, "/")
		json.NewEncoder(w).Encode(map[string]any{"object": "catalog", "data": lists[name]})
	}))
	t.Cleanup(srv.Close)
	old := BaseURL
	BaseURL = srv.URL + "/"
	t.Cleanup(func() { BaseURL = old })
	return &hits
}

func TestCatalogsAreFetchedOnceAndKept(t *testing.T) {
	hits := serve(t)
	d, stale, err := Load()
	if err != nil || stale {
		t.Fatalf("first load: stale %v, err %v", stale, err)
	}
	first := *hits
	if first != len(names()) {
		t.Errorf("%d requests, want one a catalog (%d)", first, len(names()))
	}
	if got := d.List(KeywordAbilities); len(got) != 2 {
		t.Errorf("keyword abilities %v", got)
	}

	// Kept: the second load asks nothing.
	if _, stale, _ := Load(); stale || *hits != first {
		t.Errorf("second load made %d requests, stale %v", *hits-first, stale)
	}

	// A week old: used at once, said to be stale, and Refresh asks again.
	old := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(filepath.Join(paths.Cache(), cacheFile), old, old)
	if d, stale, _ := Load(); d == nil || !stale || *hits != first {
		t.Errorf("stale load: data %v, stale %v, %d requests", d != nil, stale, *hits-first)
	}
	if _, err := Refresh(); err != nil || *hits != 2*first {
		t.Errorf("refresh: err %v, %d requests", err, *hits-first)
	}
}

func TestASubtypeBelongsToItsType(t *testing.T) {
	d := &Data{Lists: map[string][]string{
		"creature-types": {"Elf", "Golem"},
		"artifact-types": {"Equipment"},
		"spell-types":    {"Adventure"},
	}}
	for sub, want := range map[string]string{"Golem": "Creature", "equipment": "Artifact"} {
		ts, ok := d.SubtypeOf(sub)
		if !ok || len(ts) != 1 || ts[0] != want {
			t.Errorf("%s belongs to %v, want %s", sub, ts, want)
		}
	}
	// Spell types belong to both instants and sorceries.
	if ts, _ := d.SubtypeOf("Adventure"); len(ts) != 2 {
		t.Errorf("Adventure belongs to %v, want Instant and Sorcery", ts)
	}
	if _, ok := d.SubtypeOf("Dinosaur"); ok {
		t.Error("a subtype the catalogs don't have was known")
	}
	var none *Data
	if _, ok := none.SubtypeOf("Elf"); ok {
		t.Error("no catalogs, but Elf was known")
	}
}
