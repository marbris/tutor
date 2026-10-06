// Package launcher puts Tutor in the desktop's application launcher, so it
// can be started without opening a terminal first. A launcher there is a
// small file that opens a terminal running ttr:
//
//   - Linux and the BSDs: a .desktop file in the data directory's
//     applications folder, with Terminal=true.
//   - macOS: a minimal Tutor.app in ~/Applications whose program is a shell
//     script asking Terminal to run ttr.
//   - Windows: a Tutor shortcut in the Start menu, made by PowerShell.
//
// None has an icon. A bad logo would be worse than none, so the desktop
// shows its generic one.
//
// It is made once, on the first run (FirstRun). A marker in the state
// directory remembers that, so a launcher you delete stays deleted; it holds
// the path the launcher runs, so one left pointing at a binary that has since
// moved is made again.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"ttr/internal/paths"
)

// markerFile, in the state directory, says a launcher was made, and for
// which binary.
const markerFile = "launcher"

// Path is where this system's launcher goes, whether or not it is there.
// Empty on a system with no launcher to put Tutor in.
func Path() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home(), "Applications", "Tutor.app")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return ""
		}
		return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Tutor.lnk")
	case "plan9", "js", "wasip1", "ios", "android":
		return ""
	}
	return filepath.Join(dataHome(), "applications", "ttr.desktop")
}

// Exists reports whether the launcher is there.
func Exists() bool {
	p := Path()
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// Install writes the launcher, running exe, over any that's there.
func Install(exe string) error {
	p := Path()
	if p == "" {
		return fmt.Errorf("no application launcher on %s", runtime.GOOS)
	}
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = installApp(p, exe)
	case "windows":
		err = installShortcut(p, exe)
	default:
		err = writeFile(p, desktopEntry(exe), 0644)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath(), []byte(exe+"\n"), 0644)
}

// Remove takes the launcher away. Its marker stays, so it isn't made again
// on the next run.
func Remove() error {
	p := Path()
	if p == "" || !Exists() {
		return nil
	}
	return os.RemoveAll(p)
}

// FirstRun makes the launcher if it has never been made, or makes it again
// if the binary it runs has moved. It says what it did, for printing, or ""
// if it did nothing. Errors are reported the same way, once: the marker is
// written regardless, so a system that can't take a launcher isn't asked
// again on every run.
func FirstRun() string {
	exe, ok := Executable()
	if !ok {
		return ""
	}
	return firstRun(exe)
}

func firstRun(exe string) string {
	if Path() == "" || !graphical() {
		return ""
	}
	body, err := os.ReadFile(markerPath())
	made := err == nil
	if made {
		was := strings.TrimSpace(string(body))
		if was == exe || !Exists() {
			return ""
		}
	}
	if err := Install(exe); err != nil {
		os.WriteFile(markerPath(), []byte(exe+"\n"), 0644)
		return "Couldn't add Tutor to the application launcher: " + err.Error()
	}
	if made {
		return "The application launcher now starts ttr from " + exe + "."
	}
	return "Added Tutor to the application launcher: " + Path() + "\n" +
		"`ttr launcher remove` takes it away again."
}

// Executable is the binary that is running, links resolved, and whether it
// is one worth pointing a launcher at: one `go run` or `go test` built in a
// temporary directory is gone as soon as it exits.
func Executable() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if strings.Contains(exe, "go-build") || strings.HasPrefix(exe, os.TempDir()) ||
		strings.HasSuffix(exe, ".test") || strings.HasSuffix(exe, ".test.exe") {
		return exe, false
	}
	return exe, true
}

// graphical reports whether there's a desktop to launch from. On Linux a
// session over ssh has none; one is made the first time there is.
func graphical() bool {
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func markerPath() string { return filepath.Join(paths.State(), markerFile) }

// ── Linux ───────────────────────────────────────────────────────

// desktopEntry is the .desktop file: the freedesktop.org desktop entry spec,
// with Terminal=true so the launcher opens the desktop's terminal around ttr.
func desktopEntry(exe string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Tutor\n" +
		"GenericName=Magic: The Gathering cards\n" +
		"Comment=Magic: The Gathering cards, rules and decks in the terminal\n" +
		"Exec=" + desktopQuote(exe) + "\n" +
		"Terminal=true\n" +
		"Categories=Game;CardGame;\n" +
		"Keywords=mtg;magic;scryfall;deck;cards;\n" +
		"StartupNotify=false\n"
}

// desktopQuote writes a path as the Exec key wants it: in double quotes if
// it has anything the spec reserves, with ", `, $ and \ escaped, and % doubled
// in any case, since it introduces field codes.
func desktopQuote(path string) string {
	path = strings.ReplaceAll(path, "%", "%%")
	if !strings.ContainsAny(path, " \t\n\"'\\><~|&;$*?#()`") {
		return path
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range path {
		if strings.ContainsRune("\"`$\\", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func dataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home(), ".local", "share")
}

// ── macOS ───────────────────────────────────────────────────────

// installApp writes Tutor.app: an Info.plist and a script. `open -a Terminal`
// on an executable runs it in a new Terminal window.
func installApp(app, exe string) error {
	if err := writeFile(filepath.Join(app, "Contents", "Info.plist"), infoPlist, 0644); err != nil {
		return err
	}
	return writeFile(filepath.Join(app, "Contents", "MacOS", "Tutor"), appScript(exe), 0755)
}

func appScript(exe string) string {
	return "#!/bin/sh\n" +
		"# Opens ttr in Terminal. Written by ttr; `ttr launcher remove` takes it away.\n" +
		"exec open -a Terminal " + shellQuote(exe) + "\n"
}

// infoPlist is the least a bundle needs. LSUIElement keeps the script, which
// exits at once, from bouncing in the Dock.
const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Tutor</string>
	<key>CFBundleDisplayName</key>
	<string>Tutor</string>
	<key>CFBundleIdentifier</key>
	<string>io.github.marbris.tutor</string>
	<key>CFBundleExecutable</key>
	<string>Tutor</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>LSUIElement</key>
	<true/>
</dict>
</plist>
`

// shellQuote puts a path in single quotes for sh, closing and reopening them
// around any quote inside.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ── Windows ─────────────────────────────────────────────────────

// installShortcut has PowerShell make the .lnk, through the WScript.Shell
// COM object every Windows has; Go can't write one without cgo or a library.
func installShortcut(lnk, exe string) error {
	if err := os.MkdirAll(filepath.Dir(lnk), 0755); err != nil {
		return err
	}
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", shortcutScript(lnk, exe)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func shortcutScript(lnk, exe string) string {
	return "$s = (New-Object -ComObject WScript.Shell).CreateShortcut(" + psQuote(lnk) + "); " +
		"$s.TargetPath = " + psQuote(exe) + "; " +
		"$s.WorkingDirectory = " + psQuote(home()) + "; " +
		"$s.Description = 'Magic: The Gathering cards, rules and decks in the terminal'; " +
		"$s.Save()"
}

// psQuote is a PowerShell single-quoted string: a quote inside is doubled.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// ── Files ───────────────────────────────────────────────────────

func writeFile(path, body string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func home() string {
	if dir, err := os.UserHomeDir(); err == nil {
		return dir
	}
	return os.Getenv("HOME")
}
