package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func configHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "cfg"))
	t.Cleanup(func() { Use(Theme{Palette: fallbackPalette}) })
	return root
}

func TestAPaletteIsAWholeTheme(t *testing.T) {
	// The point of the split: fourteen colours and no roles at all still
	// produces a complete, coherent interface, because the default mapping
	// resolves against whatever palette it's given.
	Use(Theme{
		Name: "inverted",
		Palette: map[string]string{
			"bg": "#ffffff", "bgAlt": "#eeeeee", "fg": "#000000",
			"fgDim": "#555555", "white": "#000000", "gray": "#888888",
			"red": "#aa0000", "green": "#00aa00", "yellow": "#aaaa00",
			"blue": "#0000aa", "purple": "#aa00aa", "aqua": "#00aaaa",
			"orange": "#aa5500",
		},
	})

	for name, got := range map[string]lipgloss.Color{
		"Surface": Surface, "Accent": Accent, "Error": Error,
		"ManaG": ManaG, "RarityMythic": RarityMythic, "BarFill": BarFill,
	} {
		if got == "" {
			t.Errorf("%s is empty; every role should have resolved", name)
		}
	}

	// Accent follows the mapping (orange), not gruvbox's orange.
	if Accent != lipgloss.Color("#aa5500") {
		t.Errorf("Accent = %s, want the theme's own orange", Accent)
	}
	if Surface != lipgloss.Color("#ffffff") {
		t.Errorf("Surface = %s, want the theme's own bg", Surface)
	}
}

func TestARoleCanBeOverridden(t *testing.T) {
	Use(Theme{
		Palette: map[string]string{"bg": "#000000", "aqua": "#00ffff", "orange": "#ff8800"},
		Roles:   map[string]string{"borderFocus": "aqua"},
	})
	if BorderFocus != lipgloss.Color("#00ffff") {
		t.Errorf("BorderFocus = %s, want the aqua it was pointed at", BorderFocus)
	}
	if Accent != lipgloss.Color("#ff8800") {
		t.Errorf("Accent = %s, want the default mapping's orange", Accent)
	}
}

func TestARoleCanNameAColourOutright(t *testing.T) {
	Use(Theme{
		Palette: map[string]string{"bg": "#000000"},
		Roles:   map[string]string{"accent": "#ff00ff"},
	})
	if Accent != lipgloss.Color("#ff00ff") {
		t.Errorf("Accent = %s, want the literal it was given", Accent)
	}
}

func TestMissingColoursFallBackToTheDefault(t *testing.T) {
	// A theme that names three colours is still usable; the rest come from
	// gruvbox rather than coming out blank.
	Use(Theme{Palette: map[string]string{"bg": "#101010", "fg": "#f0f0f0", "red": "#ff0000"}})

	if Bg != lipgloss.Color("#101010") {
		t.Errorf("Bg = %s, want the theme's own", Bg)
	}
	if Error != lipgloss.Color("#ff0000") {
		t.Errorf("Error = %s, want the theme's own red", Error)
	}
	if Purple != lipgloss.Color(fallbackPalette["purple"]) {
		t.Errorf("Purple = %s, want gruvbox's %s", Purple, fallbackPalette["purple"])
	}
}

func TestAnsiIndicesAreAccepted(t *testing.T) {
	// A theme can defer to the terminal's own scheme, which is the whole
	// idea behind the "terminal" built-in.
	Use(Theme{Palette: map[string]string{"bg": "0", "fg": "7", "orange": "11"}})
	if Accent != lipgloss.Color("11") {
		t.Errorf("Accent = %s, want ANSI 11", Accent)
	}
}

func TestNonsenseValuesDoNotProduceBlankColours(t *testing.T) {
	Use(Theme{Palette: map[string]string{"bg": "not a colour", "orange": ""}})
	if Bg != lipgloss.Color(fallbackPalette["bg"]) {
		t.Errorf("Bg = %s, want the fallback", Bg)
	}
	if Accent == "" {
		t.Error("Accent came out blank")
	}
}

func TestEveryBuiltinThemeResolves(t *testing.T) {
	for _, name := range List() {
		th, err := Find(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		Use(th)
		for role := range roleVars {
			if *roleVars[role] == "" {
				t.Errorf("%s left role %q blank", name, role)
			}
		}
	}
}

func TestYourThemeShadowsTheBuiltin(t *testing.T) {
	configHome(t)
	if err := os.MkdirAll(Dir(), 0755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"name":"gruvbox","palette":{"orange":"#123456"}}`)
	if err := os.WriteFile(filepath.Join(Dir(), "gruvbox.json"), body, 0644); err != nil {
		t.Fatal(err)
	}

	th, err := Find("gruvbox")
	if err != nil {
		t.Fatal(err)
	}
	if th.Palette["orange"] != "#123456" {
		t.Errorf("orange = %s, want the copy in the config directory to win", th.Palette["orange"])
	}
}

func TestLoadRemovesTheCopiesTtrSeededButKeepsEdits(t *testing.T) {
	configHome(t)
	if err := os.MkdirAll(Dir(), 0755); err != nil {
		t.Fatal(err)
	}
	// nord.json exactly as the first version of ttr copied it, and a
	// gruvbox.json somebody changed by one colour.
	seededNord := `{
  "name": "nord",
  "palette": {
    "bg":     "#2e3440",
    "bgAlt":  "#3b4252",
    "fg":     "#d8dee9",
    "fgDim":  "#7b88a1",
    "white":  "#eceff4",
    "gray":   "#616e88",
    "red":    "#bf616a",
    "green":  "#a3be8c",
    "yellow": "#ebcb8b",
    "blue":   "#81a1c1",
    "purple": "#b48ead",
    "aqua":   "#88c0d0",
    "orange": "#d08770"
  },
  "roles": {
    "borderFocus": "aqua"
  }
}
`
	edited := `{"name":"gruvbox","palette":{"orange":"#fe8019"}}` + "\n"
	for name, body := range map[string]string{"nord": seededNord, "gruvbox": edited, "mine": seededNord} {
		if err := os.WriteFile(filepath.Join(Dir(), name+".json"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(Dir(), "nord.json")); !os.IsNotExist(err) {
		t.Error("the seeded nord.json is still there, shadowing the built-in")
	}
	for _, name := range []string{"gruvbox", "mine"} {
		if _, err := os.Stat(filepath.Join(Dir(), name+".json")); err != nil {
			t.Errorf("%s.json was removed: %v", name, err)
		}
	}
}

func TestThePalettesOldNamesStillWork(t *testing.T) {
	Use(Theme{
		Palette: map[string]string{"bg": "#000000", "bgAlt": "#111111", "white": "#fafafa", "gray": "#777777"},
		Roles:   map[string]string{"accent": "white"},
	})
	if FgBright != "#fafafa" || TextBright != "#fafafa" || Accent != "#fafafa" {
		t.Errorf("white didn't stand for fgBright: %s %s %s", FgBright, TextBright, Accent)
	}
	if FgMuted != "#777777" || TextMuted != "#777777" {
		t.Errorf("gray didn't stand for fgMuted: %s %s", FgMuted, TextMuted)
	}
	// No bgSel: the selection takes the theme's own raised background.
	if SelectionBg != "#111111" {
		t.Errorf("SelectionBg = %s, want the theme's bgAlt", SelectionBg)
	}
}

func TestSetRefusesAThemeThatIsntThere(t *testing.T) {
	configHome(t)
	if err := Set("nosuch"); err == nil {
		t.Error("Set accepted a theme that doesn't exist")
	}
	if Current() != DefaultName {
		t.Errorf("Current = %s, want the default to be untouched", Current())
	}
}

func TestSetAndCurrentRoundTrip(t *testing.T) {
	configHome(t)
	if err := Set("nord"); err != nil {
		t.Fatal(err)
	}
	if Current() != "nord" {
		t.Errorf("Current = %s, want nord", Current())
	}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestExportWritesEveryRole(t *testing.T) {
	body, err := Export("gruvbox")
	if err != nil {
		t.Fatal(err)
	}
	th, err := parse(body)
	if err != nil {
		t.Fatalf("%v in:\n%s", err, body)
	}
	// In reading order, under comments, not alphabetically.
	text := string(body)
	if !(strings.Index(text, `"bg"`) < strings.Index(text, `"fg"`) &&
		strings.Index(text, `"fg"`) < strings.Index(text, `"aqua"`) &&
		strings.Index(text, `"surface"`) < strings.Index(text, `"accent"`)) {
		t.Errorf("not in reading order:\n%s", text)
	}
	if !strings.Contains(text, "// Backgrounds") || !strings.Contains(text, "// Mana") {
		t.Errorf("no comments naming the groups:\n%s", text)
	}
	for _, name := range paletteNames {
		if _, ok := th.Palette[name]; !ok {
			t.Errorf("exported theme is missing colour %q", name)
		}
	}
	for role := range roleVars {
		if _, ok := th.Roles[role]; !ok {
			t.Errorf("exported theme is missing role %q", role)
		}
	}
}

func TestOnColourPicksTheLegibleSide(t *testing.T) {
	Use(Theme{Palette: fallbackPalette})
	for bg, want := range map[lipgloss.Color]lipgloss.Color{
		"#fabd2f": Bg,       // yellow wants dark text
		"#ebdbb2": Bg,       // the light foreground, as a bar
		"#282828": FgBright, // near-black wants light text
		"#076678": FgBright, // dark blue
		"#fff":    Bg,       // short hex
		"11":      Bg,       // bright yellow, from the terminal's scheme
		"4":       FgBright, // blue, from the terminal's scheme
	} {
		if got := OnColour(bg); got != want {
			t.Errorf("OnColour(%s) = %s, want %s", bg, got, want)
		}
	}
	// Something unreadable still gets an answer rather than a panic.
	if OnColour("nonsense") == "" {
		t.Error("an unreadable colour got no text colour")
	}
}

func TestEveryRoleIsInAGroup(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range roleGroups {
		for _, r := range g.names {
			if _, ok := roleVars[r]; !ok {
				t.Errorf("group %q lists %q, which is no role", g.comment, r)
			}
			seen[r] = true
		}
	}
	for r := range roleVars {
		if !seen[r] {
			t.Errorf("role %q is in no group, so a theme file wouldn't list it", r)
		}
	}
}

// TestBuiltinThemesAreReadable keeps the built-ins honest, the light ones
// especially: text has to stand out from the background and from the
// selection, and the three backgrounds have to be told apart.
func TestBuiltinThemesAreReadable(t *testing.T) {
	files, _ := builtin.ReadDir("themes")
	for _, f := range files {
		name, _ := themeName(f.Name())
		th, err := builtinTheme(name)
		if err != nil {
			t.Fatal(err)
		}
		if th.Transparent {
			continue // its colours are the terminal's, which nobody here can see
		}
		on := func(fg, bg string) float64 {
			a, okA := luminance(lipgloss.Color(th.Palette[fg]))
			b, okB := luminance(lipgloss.Color(th.Palette[bg]))
			if !okA || !okB {
				t.Fatalf("%s: %s or %s is not a colour", name, fg, bg)
			}
			return contrast(a, b)
		}
		// Colored text on a light background is harder to read than on a
		// dark one at the same contrast, and the published light palettes
		// are pale; so a light theme's hues are held to more. Yellow sets
		// the bar: darker than this and it reads as brown, too close to
		// orange to tell apart.
		hue := 2.0
		if l, _ := luminance(lipgloss.Color(th.Palette["bg"])); l > 0.5 {
			hue = 3.5
		}
		for _, c := range []struct {
			fg, bg string
			min    float64
		}{
			{"fg", "bg", 4.5},
			{"fg", "bgSel", 4.5},
			{"fgBright", "bgSel", 4.5},
			{"fgDim", "bg", 3},
			{"red", "bg", hue}, {"green", "bg", hue}, {"yellow", "bg", hue}, {"blue", "bg", hue},
			{"purple", "bg", hue}, {"aqua", "bg", hue}, {"orange", "bg", hue},
		} {
			if got := on(c.fg, c.bg); got < c.min {
				t.Errorf("%s: %s on %s has contrast %.1f, want at least %.1f", name, c.fg, c.bg, got, c.min)
			}
		}
		for _, pair := range [][2]string{{"bg", "bgAlt"}, {"bg", "bgSel"}, {"bgAlt", "bgSel"}} {
			if th.Palette[pair[0]] == th.Palette[pair[1]] {
				t.Errorf("%s: %s and %s are the same colour", name, pair[0], pair[1])
			}
		}
		for _, p := range paletteNames {
			if _, ok := th.Palette[p]; !ok {
				t.Errorf("%s has no %s", name, p)
			}
		}
	}
}

func TestWithNothingChosenTheTerminalsOwnColorsAreUsed(t *testing.T) {
	configHome(t)
	if Current() != "terminal" {
		t.Errorf("Current = %s, want terminal", Current())
	}
}
