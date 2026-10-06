package launcher

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ttr-launcher-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("APPDATA", filepath.Join(root, "appdata"))
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	os.Setenv("DISPLAY", ":0")
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}

func linuxOnly(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("writes a .desktop file")
	}
}

func TestTheFirstRunMakesALauncherAndADeletedOneStaysDeleted(t *testing.T) {
	linuxOnly(t)
	t.Cleanup(func() { os.Remove(markerPath()); Remove() })

	if said := firstRun("/opt/tutor/ttr"); !strings.Contains(said, "Added Tutor") {
		t.Errorf("the first run said %q", said)
	}
	body, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal("no launcher after the first run:", err)
	}
	for _, want := range []string{"Exec=/opt/tutor/ttr\n", "Terminal=true\n", "Name=Tutor\n"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the launcher lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "Icon=") {
		t.Error("the launcher has an icon; the note wants none rather than a bad one")
	}

	if said := firstRun("/opt/tutor/ttr"); said != "" {
		t.Errorf("the second run said %q", said)
	}
	Remove()
	if said := firstRun("/opt/tutor/ttr"); said != "" || Exists() {
		t.Errorf("a deleted launcher came back (%q)", said)
	}
}

func TestALauncherForAMovedBinaryIsMadeAgain(t *testing.T) {
	linuxOnly(t)
	t.Cleanup(func() { os.Remove(markerPath()); Remove() })

	firstRun("/opt/tutor/ttr")
	if said := firstRun("/home/me/bin/ttr"); !strings.Contains(said, "/home/me/bin/ttr") {
		t.Errorf("the run after moving said %q", said)
	}
	body, _ := os.ReadFile(Path())
	if !strings.Contains(string(body), "Exec=/home/me/bin/ttr\n") {
		t.Errorf("the launcher still runs the old binary:\n%s", body)
	}
}

func TestNoLauncherWithoutADesktop(t *testing.T) {
	linuxOnly(t)
	t.Cleanup(func() { os.Remove(markerPath()); Remove() })
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")

	if said := firstRun("/opt/tutor/ttr"); said != "" || Exists() {
		t.Errorf("made a launcher over ssh (%q)", said)
	}
	if _, err := os.Stat(markerPath()); err == nil {
		t.Error("the marker was written, so the first graphical run won't make one")
	}
}

func TestExecQuoting(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/local/bin/ttr":           "/usr/local/bin/ttr",
		"/home/a b/ttr":                `"/home/a b/ttr"`,
		`/x/$HOME/"q"/ttr`:             `"/x/\$HOME/\"q\"/ttr"`,
		"/opt/100%/ttr":                "/opt/100%%/ttr",
		"/Users/o'brien/bin/ttr (old)": `"/Users/o'brien/bin/ttr (old)"`,
	} {
		if got := desktopQuote(in); got != want {
			t.Errorf("desktopQuote(%q) = %s, want %s", in, got, want)
		}
	}
	if got := shellQuote("/Users/o'brien/ttr"); got != `'/Users/o'\''brien/ttr'` {
		t.Errorf("shellQuote = %s", got)
	}
	if got := psQuote(`C:\Users\o'brien\ttr.exe`); got != `'C:\Users\o''brien\ttr.exe'` {
		t.Errorf("psQuote = %s", got)
	}
}

func TestTheMacScriptOpensTerminal(t *testing.T) {
	got := appScript("/usr/local/bin/ttr")
	if !strings.HasPrefix(got, "#!/bin/sh\n") || !strings.Contains(got, "exec open -a Terminal '/usr/local/bin/ttr'\n") {
		t.Errorf("appScript:\n%s", got)
	}
	if !strings.Contains(infoPlist, "<string>Tutor</string>") || strings.Contains(infoPlist, "CFBundleIconFile") {
		t.Error("the Info.plist doesn't name the script, or names an icon")
	}
}
