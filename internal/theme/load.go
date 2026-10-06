package theme

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ttr/internal/config"
	"ttr/internal/jsonc"
	"ttr/internal/paths"
)

//go:embed themes/*.json
var builtin embed.FS

// DefaultName is the theme in force when nothing says otherwise, and the
// source of every fallback.
const DefaultName = "gruvbox"

// fallbackPalette is the default theme's colours, used for anything a theme
// leaves out. Filled in at startup from the embedded file, so there is one
// definition of gruvbox rather than two that can drift.
var fallbackPalette = map[string]string{}

func init() {
	if t, err := builtinTheme(DefaultName); err == nil {
		fallbackPalette = t.Palette
	}
	// A palette before any config is read, so a program that never calls
	// Load still draws in colour rather than in the zero value.
	Use(Theme{Palette: fallbackPalette})
}

// ── Configuration ───────────────────────────────────────────────

// Set writes the chosen theme to the config file, having checked it exists.
// It reads the whole file first and writes it back whole, so a sync remote or
// any other setting kept alongside the theme is left in place.
func Set(name string) error {
	if _, err := Find(name); err != nil {
		return err
	}
	c := config.Load()
	c.Theme = name
	return config.Save(c)
}

// Current is the name of the configured theme.
func Current() string {
	if name := config.Load().Theme; name != "" {
		return name
	}
	return DefaultName
}

// Load puts the configured theme in force. A theme that is missing or
// unreadable is reported, and the default stays up — a typo in a config file
// shouldn't leave someone staring at an unusable screen.
func Load() error {
	RemoveSeeded()

	name := Current()
	t, err := Find(name)
	if err != nil {
		return err
	}
	Use(t)
	return nil
}

// seeded is the sha256 of every built-in theme file as ttr once copied it
// into the config directory. It used to, so there would be a file to read;
// but a file there shadows the built-in of the same name, so those copies
// kept people on the colours of whichever version first ran, and a fixed
// built-in never reached them.
var seeded = map[string]bool{
	"98691de5b9495eb5ca7547fdb056c547c5d66edc2335920d0d516a9da848568a": true, // gruvbox
	"52dad0e5e25f2db976cc3bd3d1e61cc935cb7bb58bbca79565c9c28649fdd990": true, // nord, first
	"e33342d394f494b981d8d94a0ec5f56a6d831ca95c4ea5097401e610c5375e96": true, // nord, editing border
	"cc037380dab369bbbe3d789b96c626a249343c3ac29d5565aad363a645749f73": true, // terminal
	"aa5fd86725bf046d4d2b35f3dda0a1b9879df0ff5c728541c47e575aaf828adf": true, // terminal, transparent
}

// RemoveSeeded deletes the copies of built-ins that ttr itself wrote and
// nobody has changed since, so the built-ins show through again. A file that
// differs by a byte is somebody's edit and stays. Best-effort.
func RemoveSeeded() {
	entries, _ := os.ReadDir(Dir())
	for _, e := range entries {
		name, ok := themeName(e.Name())
		if !ok || !IsBuiltin(name) {
			continue
		}
		path := filepath.Join(Dir(), e.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if seeded[fmt.Sprintf("%x", sha256.Sum256(body))] {
			os.Remove(path)
		}
	}
}

// ── Finding themes ──────────────────────────────────────────────

// Dir is where your own themes go.
func Dir() string { return filepath.Join(paths.Config(), "themes") }

// Find looks for a theme by name: yours first, so a file in your config
// directory replaces a built-in of the same name.
func Find(name string) (Theme, error) {
	if t, err := readTheme(filepath.Join(Dir(), name+".json")); err == nil {
		return t, nil
	}
	t, err := builtinTheme(name)
	if err != nil {
		return Theme{}, fmt.Errorf("no theme called %q — try `ttr theme`", name)
	}
	return t, nil
}

// List names every theme available, yours and the built-ins, without
// duplicates.
func List() []string {
	seen := map[string]bool{}
	var out []string

	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}

	entries, _ := os.ReadDir(Dir())
	for _, e := range entries {
		if name, ok := themeName(e.Name()); ok {
			add(name)
		}
	}
	files, _ := builtin.ReadDir("themes")
	for _, f := range files {
		if name, ok := themeName(f.Name()); ok {
			add(name)
		}
	}

	sort.Strings(out)
	return out
}

// IsBuiltin reports whether a name is one of ours, which is what tells a
// listing that a theme can be copied as a starting point.
func IsBuiltin(name string) bool {
	_, err := builtinTheme(name)
	return err == nil
}

func themeName(file string) (string, bool) {
	if !strings.HasSuffix(file, ".json") {
		return "", false
	}
	return strings.TrimSuffix(file, ".json"), true
}

func builtinTheme(name string) (Theme, error) {
	body, err := builtin.ReadFile("themes/" + name + ".json")
	if err != nil {
		return Theme{}, err
	}
	return parse(body)
}

func readTheme(path string) (Theme, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, err
	}
	return parse(body)
}

// parse reads a theme file, which may carry comments. One that is nothing
// but comments — the template `ttr init` writes — is a theme that changes
// nothing, so everything falls back to the default.
func parse(body []byte) (Theme, error) {
	var t Theme
	if jsonc.Empty(body) {
		return t, nil
	}
	if err := json.Unmarshal(jsonc.Strip(body), &t); err != nil {
		return Theme{}, err
	}
	return normalize(t), nil
}

// Export writes a theme out as a file to edit: the palette and then every
// role, in reading order and under a comment per group, so the file shows
// every knob there is rather than only the ones this theme happened to change.
func Export(name string) ([]byte, error) {
	t, err := Find(name)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"name\": %s,\n", strconv.Quote(t.Name))
	if t.Transparent {
		b.WriteString("  // The terminal's own background shows through.\n")
		b.WriteString("  \"transparent\": true,\n")
	}

	b.WriteString("  \"palette\": {\n")
	writeGroups(&b, paletteGroups, func(name string) (string, bool) {
		v, ok := t.Palette[name]
		return v, ok
	})
	b.WriteString("  },\n")

	b.WriteString("  // What each color is for: a palette name, or a color outright.\n")
	b.WriteString("  \"roles\": {\n")
	writeGroups(&b, roleGroups, func(role string) (string, bool) {
		ref, ok := t.Roles[role]
		if !ok || strings.TrimSpace(ref) == "" {
			ref = defaultRoles[role]
		}
		if name, old := paletteAliases[ref]; old {
			ref = name
		}
		return ref, true
	})
	b.WriteString("  }\n}\n")
	return []byte(b.String()), nil
}

// writeGroups writes the members of a JSON object a group at a time, each
// under its comment, with the commas JSON wants between them.
func writeGroups(b *strings.Builder, groups []group, value func(string) (string, bool)) {
	var lines []string
	var comments = map[int]string{}
	for _, g := range groups {
		first := true
		for _, name := range g.names {
			v, ok := value(name)
			if !ok {
				continue
			}
			if first {
				comments[len(lines)] = g.comment
				first = false
			}
			lines = append(lines, fmt.Sprintf("%-18s %s", strconv.Quote(name)+":", strconv.Quote(v)))
		}
	}
	for i, line := range lines {
		if c, ok := comments[i]; ok {
			b.WriteString("    // " + c + "\n")
		}
		comma := ","
		if i == len(lines)-1 {
			comma = ""
		}
		b.WriteString("    " + line + comma + "\n")
	}
}

// PaletteNames is the colours a theme may name, in a sensible reading order.
func PaletteNames() []string { return paletteNames }

// PaletteGroups is PaletteNames split into backgrounds, text and colors.
func PaletteGroups() [][]string {
	var out [][]string
	for _, g := range paletteGroups {
		out = append(out, g.names)
	}
	return out
}
