// Package theme is the colours the interface is drawn in.
//
// A theme is two things. The palette is the colours themselves, under the
// names a terminal colour scheme usually gives them — bg, fg, red, yellow
// and so on. The roles say what each colour is *for*: which one draws a
// focused border, which one marks a card you already own, which one is a
// mythic rare.
//
// Splitting them is what makes a theme cheap to write. Porting an existing
// scheme means listing fourteen colours and nothing else: every role falls
// back to the default *mapping* — accent is the orange one, error is the red
// one — resolved against whatever palette you supplied. Override a role only
// where your scheme disagrees.
package theme

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a palette, and optionally what to do with it.
type Theme struct {
	Name string `json:"name"`
	// Palette maps a colour name to a value: "#rrggbb", or an ANSI index
	// "0"–"15" to take the colour from the terminal's own scheme.
	Palette map[string]string `json:"palette"`
	// Roles maps a role to a palette name, or to a literal value for a
	// colour the palette doesn't carry. Anything omitted is inherited.
	Roles map[string]string `json:"roles,omitempty"`
	// Transparent leaves the background to the terminal: nothing is painted
	// behind the text but selections and bars. For a theme that takes its
	// colours from the terminal's own scheme, and for see-through terminals.
	Transparent bool `json:"transparent,omitempty"`
}

// transparent is whether the theme in force paints its background.
var transparent bool

// Transparent reports whether the theme in force leaves the background to
// the terminal.
func Transparent() bool { return transparent }

// ── The palette in use ──────────────────────────────────────────

var (
	Bg       lipgloss.Color
	BgAlt    lipgloss.Color
	BgSel    lipgloss.Color
	Fg       lipgloss.Color
	FgBright lipgloss.Color
	FgDim    lipgloss.Color
	FgMuted  lipgloss.Color
	Red      lipgloss.Color
	Green    lipgloss.Color
	Yellow   lipgloss.Color
	Blue     lipgloss.Color
	Purple   lipgloss.Color
	Aqua     lipgloss.Color
	Orange   lipgloss.Color
)

// paletteNames is every colour a theme may name, in the order `ttr theme`
// prints them and a theme file lists them: the backgrounds, the text from
// strongest to faintest, then the hues.
//
// The names say what a colour is for rather than what it looks like. "white"
// was the brightest text, which in a light theme is black.
var paletteNames = []string{
	"bg", "bgAlt", "bgSel",
	"fg", "fgBright", "fgDim", "fgMuted",
	"red", "green", "yellow", "blue", "purple", "aqua", "orange",
}

// paletteGroups is paletteNames in its three groups, with what each is for.
var paletteGroups = []group{
	{"Backgrounds: the screen, raised things (borders, empty bars), the selection.", paletteNames[0:3]},
	{"Text: ordinary, strongest, secondary, faintest.", paletteNames[3:7]},
	{"Colors.", paletteNames[7:]},
}

// group is a run of names a theme file lists together, under a comment.
type group struct {
	comment string
	names   []string
}

// paletteAliases are the names the palette had before, still read so a theme
// written with them means what it meant.
var paletteAliases = map[string]string{
	"white": "fgBright",
	"gray":  "fgMuted",
}

var paletteVars = map[string]*lipgloss.Color{
	"bg": &Bg, "bgAlt": &BgAlt, "bgSel": &BgSel,
	"fg": &Fg, "fgBright": &FgBright, "fgDim": &FgDim, "fgMuted": &FgMuted,
	"red": &Red, "green": &Green, "yellow": &Yellow,
	"blue": &Blue, "purple": &Purple, "aqua": &Aqua, "orange": &Orange,
}

// ── The roles in use ────────────────────────────────────────────

var (
	// Surfaces
	Surface       lipgloss.Color
	SurfaceAlt    lipgloss.Color
	Border        lipgloss.Color
	BorderFocus   lipgloss.Color
	BorderEditing lipgloss.Color

	// Text
	Text       lipgloss.Color
	TextBright lipgloss.Color
	TextDim    lipgloss.Color
	TextMuted  lipgloss.Color

	// Meaning
	Accent    lipgloss.Color
	Highlight lipgloss.Color
	Success   lipgloss.Color
	Error     lipgloss.Color
	Info      lipgloss.Color
	Special   lipgloss.Color

	// Picking things out
	SelectionBg lipgloss.Color
	SelectionFg lipgloss.Color
	Marked      lipgloss.Color
	Member      lipgloss.Color
	// MemberOther marks a card that is also in a list that is neither the
	// editing deck nor the focused one. Those two mark theirs in their own
	// border colours.
	MemberOther lipgloss.Color

	// Mana
	ManaW     lipgloss.Color
	ManaU     lipgloss.Color
	ManaB     lipgloss.Color
	ManaR     lipgloss.Color
	ManaG     lipgloss.Color
	ManaC     lipgloss.Color
	ManaMulti lipgloss.Color

	// Rarity
	RarityCommon   lipgloss.Color
	RarityUncommon lipgloss.Color
	RarityRare     lipgloss.Color
	RarityMythic   lipgloss.Color
	RaritySpecial  lipgloss.Color

	// Histograms
	BarFill  lipgloss.Color
	BarEmpty lipgloss.Color

	// Diffs
	DiffAdd    lipgloss.Color
	DiffRemove lipgloss.Color
)

// roleVars is every role, and where its colour lands.
var roleVars = map[string]*lipgloss.Color{
	"surface": &Surface, "surfaceAlt": &SurfaceAlt,
	"border": &Border, "borderFocus": &BorderFocus, "borderEditing": &BorderEditing,

	"text": &Text, "textBright": &TextBright, "textDim": &TextDim, "textMuted": &TextMuted,

	"accent": &Accent, "highlight": &Highlight, "success": &Success,
	"error": &Error, "info": &Info, "special": &Special,

	"selectionBg": &SelectionBg, "selectionFg": &SelectionFg,
	"marked": &Marked, "member": &Member, "memberOther": &MemberOther,

	"manaW": &ManaW, "manaU": &ManaU, "manaB": &ManaB, "manaR": &ManaR,
	"manaG": &ManaG, "manaC": &ManaC, "manaMulti": &ManaMulti,

	"rarityCommon": &RarityCommon, "rarityUncommon": &RarityUncommon,
	"rarityRare": &RarityRare, "rarityMythic": &RarityMythic,
	"raritySpecial": &RaritySpecial,

	"barFill": &BarFill, "barEmpty": &BarEmpty,
	"diffAdd": &DiffAdd, "diffRemove": &DiffRemove,
}

// roleGroups is every role in the order a theme file lists them.
var roleGroups = []group{
	{"Surfaces", []string{"surface", "surfaceAlt", "border", "borderFocus", "borderEditing"}},
	{"Text", []string{"text", "textBright", "textDim", "textMuted"}},
	{"Meaning", []string{"accent", "highlight", "success", "error", "info", "special"}},
	{"Picking things out", []string{"selectionBg", "selectionFg", "marked", "member", "memberOther"}},
	{"Mana", []string{"manaW", "manaU", "manaB", "manaR", "manaG", "manaC", "manaMulti"}},
	{"Rarity", []string{"rarityCommon", "rarityUncommon", "rarityRare", "rarityMythic", "raritySpecial"}},
	{"Histograms", []string{"barFill", "barEmpty"}},
	{"Diffs", []string{"diffAdd", "diffRemove"}},
}

// defaultRoles is what every role means unless a theme says otherwise. The
// values are palette names, not colours — which is the point. A theme that
// lists nothing but fourteen colours still gets a coherent interface,
// because these say accent is *the orange one*, whatever orange means to it.
var defaultRoles = map[string]string{
	"surface": "bg", "surfaceAlt": "bgAlt",
	"border": "bgAlt", "borderFocus": "orange", "borderEditing": "aqua",

	"text": "fg", "textBright": "fgBright", "textDim": "fgDim", "textMuted": "fgMuted",

	"accent": "orange", "highlight": "yellow", "success": "green",
	"error": "red", "info": "blue", "special": "purple",

	"selectionBg": "bgSel", "selectionFg": "fgBright",
	"marked": "yellow", "member": "aqua", "memberOther": "fgMuted",

	// Black mana is drawn purple: a glyph in the terminal's background colour
	// is a glyph you can't see.
	"manaW": "fgBright", "manaU": "blue", "manaB": "purple", "manaR": "red",
	"manaG": "green", "manaC": "fgDim", "manaMulti": "yellow",

	"rarityCommon": "fg", "rarityUncommon": "fgDim", "rarityRare": "yellow",
	"rarityMythic": "orange", "raritySpecial": "purple",

	"barFill": "aqua", "barEmpty": "bgAlt",
	"diffAdd": "green", "diffRemove": "red",
}

// Use makes a theme the one in force. Anything the theme leaves out falls
// back to the built-in default, so a partial file is a valid file.
func Use(t Theme) {
	t = normalize(t)
	transparent = t.Transparent
	for name, target := range paletteVars {
		*target = colorOf(t.Palette[name], fallbackPalette[name])
	}
	for role, target := range roleVars {
		*target = resolveRole(t, role)
	}
}

// resolveRole finds a role's colour: what the theme says, else what the
// default mapping says, resolved against the theme's own palette.
func resolveRole(t Theme, role string) lipgloss.Color {
	ref, ok := t.Roles[role]
	if !ok || strings.TrimSpace(ref) == "" {
		ref = defaultRoles[role]
	}
	if name, old := paletteAliases[ref]; old {
		ref = name
	}
	// A role may name a palette colour, or give a value outright.
	if value, isPaletteName := t.Palette[ref]; isPaletteName {
		return colorOf(value, fallbackPalette[ref])
	}
	if literal(ref) {
		return lipgloss.Color(ref)
	}
	// Names a palette colour this theme doesn't carry.
	return colorOf(fallbackPalette[ref], fallbackPalette["fg"])
}

// colorOf takes a value, or the fallback when it's missing or malformed.
func colorOf(value, fallback string) lipgloss.Color {
	if literal(value) {
		return lipgloss.Color(value)
	}
	return lipgloss.Color(fallback)
}

// literal reports whether a value is a colour rather than a name: a hex
// triplet, or an ANSI index 0–15 that takes its colour from the terminal.
func literal(s string) bool {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "#") {
		return len(s) == 4 || len(s) == 7
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= 0 && n <= 15
}

// normalize reads a theme the way it was meant: the palette's old names as
// the new ones, and a selection colour from the theme's own raised background
// when it names none — closer to what it meant than the default theme's.
func normalize(t Theme) Theme {
	palette := make(map[string]string, len(t.Palette))
	for name, value := range t.Palette {
		palette[name] = value
	}
	for old, name := range paletteAliases {
		if value, ok := palette[old]; ok {
			if _, set := palette[name]; !set {
				palette[name] = value
			}
			delete(palette, old)
		}
	}
	if _, ok := palette["bgSel"]; !ok {
		if alt, ok := palette["bgAlt"]; ok {
			palette["bgSel"] = alt
		}
	}
	t.Palette = palette
	return t
}
