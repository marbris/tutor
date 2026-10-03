package tagger

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// fixture is a small Tagger file: removal has no cards of its own, only
// through its children, and removal-artifact also sits under
// artifact-matters.
const fixture = `{"id":"r","label":"removal","parent_ids":[],"child_ids":["ra","re"],"taggings":[]}
{"id":"ra","label":"removal-artifact","parent_ids":["r","am"],"child_ids":[],"taggings":[{"oracle_id":"shatter"},{"oracle_id":"disenchant"}]}
{"id":"re","label":"removal-enchantment","parent_ids":["r"],"child_ids":[],"taggings":[{"oracle_id":"disenchant"}]}
{"id":"am","label":"artifact-matters","parent_ids":[],"child_ids":["ra"],"taggings":[]}
{"id":"sr","label":"spot removal","slug":"spot-removal","parent_ids":[],"child_ids":[],"taggings":[{"oracle_id":"shatter"}]}
{"id":"rp","label":"ramp","parent_ids":[],"child_ids":[],"taggings":[{"oracle_id":"sol"}]}
`

func gzipped(s string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

func read(t *testing.T) *Data {
	t.Helper()
	d, err := Read(gzipped(fixture))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestACardHasItsTagsAndEveryTagAboveThem(t *testing.T) {
	d := read(t)
	removal, _ := d.Find("removal")
	artifacts, _ := d.Find("artifact-matters")
	enchantment, _ := d.Find("removal-enchantment")
	if !d.Has("shatter", removal) || !d.Has("shatter", artifacts) {
		t.Error("Shatter isn't counted under both of removal-artifact's parents")
	}
	if d.Has("shatter", enchantment) {
		t.Error("Shatter counted as enchantment removal")
	}
	if d.Has("sol", removal) {
		t.Error("Sol Ring counted as removal")
	}
	if got := strings.Join(d.Of("disenchant"), " "); got != "removal-artifact removal-enchantment" {
		t.Errorf("Disenchant's own tags are %q", got)
	}
	if d.Tagged("unknown") || !d.Tagged("sol") {
		t.Error("Tagged is wrong")
	}
}

func TestRootsAndChildren(t *testing.T) {
	d := read(t)
	var roots []string
	for _, i := range d.Roots() {
		roots = append(roots, d.Tags[i].Label)
	}
	if got := strings.Join(roots, " "); got != "artifact-matters ramp removal spot-removal" {
		t.Errorf("roots are %q", got)
	}
	removal, _ := d.Find("removal")
	if n := len(d.Tags[removal].Children); n != 2 {
		t.Errorf("removal has %d children", n)
	}
	if got := strings.Join(d.Labels("removal-"), " "); got != "removal-artifact removal-enchantment" {
		t.Errorf("labels under removal- are %q", got)
	}
}

func TestLoadDownloadsOnceAndKeepsTheResult(t *testing.T) {
	hits := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/meta" {
			w.Write([]byte(`{"jsonl_download_uri":"` + srv.URL + `/tags.jsonl.gz"}`))
			return
		}
		w.Write(gzipped(fixture))
	}))
	defer srv.Close()
	old := BulkURL
	BulkURL = srv.URL + "/meta"
	defer func() { BulkURL = old }()

	for run := 0; run < 2; run++ {
		d, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		removal, _ := d.Find("removal")
		if !d.Has("disenchant", removal) {
			t.Errorf("run %d: the loaded data lost its tree", run)
		}
	}
	if hits != 2 || !Cached() {
		t.Errorf("%d requests over two loads, want the two of the first", hits)
	}
}
