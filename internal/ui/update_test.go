package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ttr/internal/upgrade"
)

// releaseServer answers "what is the newest release" with tag.
func releaseServer(t *testing.T, tag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://example.invalid/` + tag + `","assets":[]}`))
	}))
	t.Cleanup(srv.Close)
	was := upgrade.LatestURL
	upgrade.LatestURL = srv.URL
	t.Cleanup(func() { upgrade.LatestURL = was })
}

func runningVersion(t *testing.T, v string) {
	t.Helper()
	was := Version
	Version = v
	t.Cleanup(func() { Version = was })
}

func TestTheVersionRowFindsANewerReleaseAndOffersToInstallIt(t *testing.T) {
	runningVersion(t, "11.1.0")
	releaseServer(t, "v11.2.0")
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "version")

	// ? off: the info panel says the key. ? on: the hints do.
	if view := stripANSI(m.View()); !strings.Contains(view, "enter checks for a newer release") {
		t.Errorf("the version row doesn't say its key:\n%s", view)
	}
	m = drive(m, "?")
	if view := stripANSI(m.View()); !strings.Contains(view, "check for updates") {
		t.Errorf("the hints don't offer the check:\n%s", view)
	}
	m = drive(m, "?")

	next, cmd := m.Update(keyMsg("enter"))
	m = next.(Model)
	if r, _ := v.current(); !strings.Contains(r.value, "checking") {
		t.Errorf("while checking the row says %q", r.value)
	}
	m = settle(m, cmd)
	if r, _ := v.current(); r.value != "11.1.0 · 11.2.0 available" {
		t.Errorf("after the check the row says %q", r.value)
	}
	if !strings.Contains(m.notice, "11.2.0 is out") {
		t.Errorf("notice %q", m.notice)
	}
	if hints := stripANSI(m.View()); !strings.Contains(hints, "enter installs it") {
		t.Errorf("an update found isn't offered:\n%s", hints)
	}

	// A test binary has nothing to replace, so installing fails, and says so.
	next, cmd = m.Update(keyMsg("enter"))
	m = settle(next.(Model), cmd)
	if r, _ := v.current(); !strings.Contains(r.value, "couldn't install 11.2.0") {
		t.Errorf("after a failed install the row says %q", r.value)
	}
	if !strings.Contains(m.notice, "couldn't install") {
		t.Errorf("notice %q", m.notice)
	}
}

func TestTheVersionRowSaysWhenItIsTheNewest(t *testing.T) {
	runningVersion(t, "11.2.0")
	releaseServer(t, "v11.2.0")
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "version")
	next, cmd := m.Update(keyMsg("enter"))
	m = settle(next.(Model), cmd)
	if r, _ := v.current(); r.value != "11.2.0 · up to date" {
		t.Errorf("the row says %q", r.value)
	}
	if m.notice != "ttr 11.2.0 is the newest" {
		t.Errorf("notice %q", m.notice)
	}
}

func TestABuildFromSourceIsntOfferedAnUpdate(t *testing.T) {
	runningVersion(t, "dev")
	m := withCards(sized(160, 40), "d", deckSample(), sortArrival)
	m, v := settingsPanel(t, m)
	pointRow(t, v, "version")
	if hints := v.keys(); strings.Contains(stripANSI(fmtHints(hints)), "check for updates") {
		t.Error("a build from source offers a check")
	}
	m = drive(m, "enter")
	if !strings.Contains(m.notice, "make build") {
		t.Errorf("notice %q", m.notice)
	}
}

func fmtHints(gs []hintGroup) string {
	var b strings.Builder
	for _, g := range gs {
		for _, k := range g.keys {
			b.WriteString(k[0] + " " + k[1] + "\n")
		}
	}
	return b.String()
}
