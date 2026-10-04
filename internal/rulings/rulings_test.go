package rulings

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

const fixture = `{"object":"ruling","oracle_id":"bolt","source":"wotc","published_at":"2004-10-04","comment":"It deals 3 damage."}
{"object":"ruling","oracle_id":"hoof","source":"wotc","published_at":"2012-01-01","comment":"Trample matters."}
{"object":"ruling","oracle_id":"bolt","source":"scryfall","published_at":"2020-01-01","comment":"Any target means any target."}
`

func serve(t *testing.T) *int {
	t.Helper()
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write([]byte(fixture))
	w.Close()
	hits := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/meta" {
			rw.Write([]byte(`{"jsonl_download_uri":"` + srv.URL + `/rulings.jsonl.gz"}`))
			return
		}
		rw.Write(gz.Bytes())
	}))
	t.Cleanup(srv.Close)
	old := BulkURL
	BulkURL = srv.URL + "/meta"
	t.Cleanup(func() { BulkURL = old })
	return &hits
}

func TestRulingsAreDownloadedGroupedAndReadACardAtATime(t *testing.T) {
	hits := serve(t)
	s, stale, err := Load()
	if err != nil || stale {
		t.Fatalf("first load: stale %v, err %v", stale, err)
	}
	if *hits != 2 {
		t.Errorf("%d requests, want the bulk entry and the file", *hits)
	}
	rs, ok := s.Of("bolt")
	if !ok || len(rs) != 2 || !strings.Contains(rs[1].Comment, "any target") {
		t.Errorf("bolt's rulings: %v %v", ok, rs)
	}
	// A card the file doesn't list has none, and the store can say so.
	if rs, ok := s.Of("sol"); !ok || len(rs) != 0 {
		t.Errorf("a card without rulings: %v %v", ok, rs)
	}

	// Kept: read again without downloading.
	if s, stale, _ := Load(); s == nil || stale || *hits != 2 {
		t.Errorf("second load: %d requests, stale %v", *hits-2, stale)
	}
	if rs, _ := Kept().Of("hoof"); len(rs) != 1 {
		t.Errorf("Kept lost hoof's ruling: %v", rs)
	}

	// A week old: used, said to be stale.
	old := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(file(), old, old)
	if s, stale, _ := Load(); s == nil || !stale {
		t.Error("a week-old file isn't stale")
	}
}

func TestNoStoreCantSay(t *testing.T) {
	var s *Store
	if _, ok := s.Of("bolt"); ok {
		t.Error("no store answered")
	}
}
