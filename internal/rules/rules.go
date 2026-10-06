package rules

// Comprehensive Rules engine — merged in from the mtg-rules project.
// Parses the official rules text, indexes keyword abilities/actions and
// glossary terms, and matches them against a card's oracle text.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"ttr/internal/fetch"
	"ttr/internal/mtg"
	"ttr/internal/paths"
)

// ── Rules data ──────────────────────────────────────────────────

type Rule struct {
	Number   string // e.g. "100.1a"
	Text     string // full text of the rule
	Children []int  // indices of sub-rules
	Parent   int    // index of parent rule, -1 if none
	Depth    int    // 0 = section, 1 = category, 2 = rule, 3 = sub-rule
}

type GlossaryEntry struct {
	Term       string
	Definition string
}

type KeywordKind int

const (
	KeywordAbility KeywordKind = iota // 702.x — keyword abilities (flying, trample…)
	KeywordAction                     // 701.x — keyword actions (destroy, scry…)
	AbilityWord                       // 207.2c — ability words (landfall, metalcraft…)
)

type Keyword struct {
	Name string
	Rule string
	Kind KeywordKind
}

type Data struct {
	Rules    []Rule
	Glossary []GlossaryEntry
	Index    map[string]int // rule number -> index

	keywords   map[string]Keyword       // the rules' own keywords, by kwKey
	kw         *keywordIndex            // those plus Scryfall's catalogs; shared by every copy
	glossary   map[string]GlossaryEntry // lowercased term -> entry
	glossaryRe *regexp.Regexp           // alternation of matchable terms
	typeRules  map[string]string        // card type -> category rule number
}

func (d Data) Loaded() bool { return len(d.Rules) > 0 }

// ── Parsing ─────────────────────────────────────────────────────

var (
	sectionRe  = regexp.MustCompile(`^(\d)\. (.+)$`)
	categoryRe = regexp.MustCompile(`^(\d{3})\. (.+)$`)
	ruleRe     = regexp.MustCompile(`^(\d{3}\.\d+[a-z]*)\.?\s(.*)$`)
	subRuleRe  = regexp.MustCompile(`[a-z]$`)
	suffixRe   = regexp.MustCompile(`[a-z]+$`)
)

func Parse(text string) Data {
	lines := strings.Split(text, "\n")
	data := Data{
		Index: make(map[string]int),
	}

	rulesStart := -1
	glossaryStart := -1
	creditsStart := -1

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "1. Game Concepts" {
			for j := i + 1; j < len(lines) && j < i+20; j++ {
				if ruleRe.MatchString(strings.TrimSpace(lines[j])) {
					rulesStart = i
					break
				}
			}
		}
		if line == "Glossary" {
			glossaryStart = i
		}
		if line == "Credits" {
			creditsStart = i
		}
	}

	if rulesStart == -1 {
		return data
	}
	if creditsStart == -1 {
		creditsStart = len(lines)
	}

	endOfRules := creditsStart
	if glossaryStart > 0 {
		endOfRules = glossaryStart
	}

	for i := rulesStart; i < endOfRules; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Section: "1. Game Concepts"
		if m := sectionRe.FindStringSubmatch(line); m != nil && len(m[1]) == 1 {
			idx := len(data.Rules)
			data.Rules = append(data.Rules, Rule{
				Number: m[1],
				Text:   m[2],
				Parent: -1,
				Depth:  0,
			})
			data.Index[m[1]] = idx
			continue
		}

		// Category: "100. General"
		if m := categoryRe.FindStringSubmatch(line); m != nil {
			idx := len(data.Rules)
			parentIdx := findParentSection(m[1], &data)
			data.Rules = append(data.Rules, Rule{
				Number: m[1],
				Text:   m[2],
				Parent: parentIdx,
				Depth:  1,
			})
			if parentIdx >= 0 {
				data.Rules[parentIdx].Children = append(data.Rules[parentIdx].Children, idx)
			}
			data.Index[m[1]] = idx
			continue
		}

		// Rule: "100.1. text" or "100.1a text"
		if m := ruleRe.FindStringSubmatch(line); m != nil {
			ruleNum := m[1]
			ruleText := m[2]

			// Collect continuation lines
			for i+1 < endOfRules {
				next := strings.TrimSpace(lines[i+1])
				if next == "" {
					break
				}
				if ruleRe.MatchString(next) || sectionRe.MatchString(next) || categoryRe.MatchString(next) {
					break
				}
				i++
				ruleText += " " + next
			}

			depth := 2
			if subRuleRe.MatchString(ruleNum) {
				depth = 3
			}

			parentIdx := findParentRule(ruleNum, &data)
			idx := len(data.Rules)
			data.Rules = append(data.Rules, Rule{
				Number: ruleNum,
				Text:   ruleText,
				Parent: parentIdx,
				Depth:  depth,
			})
			if parentIdx >= 0 {
				data.Rules[parentIdx].Children = append(data.Rules[parentIdx].Children, idx)
			}
			data.Index[ruleNum] = idx
			continue
		}
	}

	// Glossary
	if glossaryStart > 0 {
		var currentTerm string
		var currentDef strings.Builder

		for i := glossaryStart + 1; i < creditsStart; i++ {
			parseGlossaryLine(strings.TrimSpace(lines[i]), &data, &currentTerm, &currentDef)
		}
		if currentTerm != "" {
			data.Glossary = append(data.Glossary, GlossaryEntry{
				Term:       currentTerm,
				Definition: strings.TrimSpace(currentDef.String()),
			})
		}
	}

	buildKeywordIndex(&data)
	buildGlossaryIndex(&data)
	buildTypeIndex(&data)

	return data
}

func parseGlossaryLine(line string, data *Data, currentTerm *string, currentDef *strings.Builder) {
	if line == "" {
		if *currentTerm != "" {
			data.Glossary = append(data.Glossary, GlossaryEntry{
				Term:       *currentTerm,
				Definition: strings.TrimSpace(currentDef.String()),
			})
			*currentTerm = ""
			currentDef.Reset()
		}
		return
	}

	if *currentTerm == "" {
		*currentTerm = line
	} else {
		if currentDef.Len() > 0 {
			currentDef.WriteString(" ")
		}
		currentDef.WriteString(line)
	}
}

func findParentSection(catNum string, data *Data) int {
	if len(catNum) >= 1 {
		if idx, ok := data.Index[string(catNum[0])]; ok {
			return idx
		}
	}
	return -1
}

func findParentRule(ruleNum string, data *Data) int {
	// "702.9a" -> parent is "702.9"
	if suffixRe.MatchString(ruleNum) {
		parent := suffixRe.ReplaceAllString(ruleNum, "")
		if idx, ok := data.Index[parent]; ok {
			return idx
		}
	}

	// "702.9" -> parent is "702"
	parts := strings.Split(ruleNum, ".")
	if len(parts) >= 2 {
		if idx, ok := data.Index[parts[0]]; ok {
			return idx
		}
	}

	return -1
}

// ── Keyword / glossary indexes ──────────────────────────────────

var (
	// Keyword names: letters, digits, apostrophes, spaces, hyphens, "!"
	kwNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9’' !-]*$`)
	// Glossary terms worth matching against card text
	glossTermRe = regexp.MustCompile(`^[A-Za-z][A-Za-z’' -]*$`)
	// "See rule 702.9" / "See rule 207.2c"
	SeeRuleRe = regexp.MustCompile(`[Ss]ee rule (\d{1,3}(?:\.\d+[a-z]*)?)`)
)

func buildKeywordIndex(d *Data) {
	d.keywords = make(map[string]Keyword)

	add := func(name, rule string, kind KeywordKind) {
		name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "."))
		if len(name) < 4 || len(name) > 40 || !kwNameRe.MatchString(name) {
			return
		}
		key := kwKey(name)
		if _, dup := d.keywords[key]; dup {
			return
		}
		d.keywords[key] = Keyword{Name: name, Rule: rule, Kind: kind}
	}

	for _, r := range d.Rules {
		if r.Depth != 2 {
			continue
		}
		var kind KeywordKind
		switch {
		case strings.HasPrefix(r.Number, "702."):
			kind = KeywordAbility
		case strings.HasPrefix(r.Number, "701."):
			kind = KeywordAction
		default:
			continue
		}
		name := r.Text
		add(name, r.Number, kind)
		// "Tap and Untap" / "Daybound and Nightbound" — index both halves too
		if parts := strings.Split(name, " and "); len(parts) == 2 {
			add(parts[0], r.Number, kind)
			add(parts[1], r.Number, kind)
		}
	}

	// Ability words are only ever listed inside rule 207.2c
	if idx, ok := d.Index["207.2c"]; ok {
		for _, w := range abilityWords(d.Rules[idx].Text) {
			add(w, "207.2c", AbilityWord)
		}
	}

	d.kw = &keywordIndex{}
	d.kw.set(d.keywords)
}

// abilityWords pulls the comma-separated list out of rule 207.2c.
func abilityWords(text string) []string {
	const marker = "ability words are "
	i := strings.Index(text, marker)
	if i < 0 {
		return nil
	}
	list := text[i+len(marker):]
	if j := strings.Index(list, "."); j >= 0 {
		list = list[:j]
	}

	var out []string
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimSpace(strings.TrimPrefix(part, "and "))
		if part == "" {
			continue
		}
		out = append(out, titleCase(part))
	}
	return out
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		if len(r) == 0 {
			continue
		}
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func buildGlossaryIndex(d *Data) {
	d.glossary = make(map[string]GlossaryEntry)

	var names []string
	for _, g := range d.Glossary {
		term := strings.TrimSpace(g.Term)
		if !glossTermRe.MatchString(term) {
			continue
		}
		// Single short words are too noisy to match against card text
		if !strings.Contains(term, " ") && len(term) < 9 {
			continue
		}
		lower := strings.ToLower(term)
		if _, isKeyword := d.keywords[kwKey(term)]; isKeyword {
			continue
		}
		if _, dup := d.glossary[lower]; dup {
			continue
		}
		d.glossary[lower] = g
		names = append(names, term)
	}

	d.glossaryRe = alternation(names)
}

// Card types map onto the category rules in section 3.
var cardTypeRules = map[string]string{
	"Artifact":     "301",
	"Creature":     "302",
	"Enchantment":  "303",
	"Instant":      "304",
	"Land":         "305",
	"Planeswalker": "306",
	"Sorcery":      "307",
	"Kindred":      "308",
	"Dungeon":      "309",
	"Battle":       "310",
	"Plane":        "311",
	"Phenomenon":   "312",
	"Vanguard":     "313",
	"Scheme":       "314",
	"Conspiracy":   "315",
}

// buildTypeIndex keeps only the type->rule pairs that the loaded rules
// actually confirm, so a renumbered future release degrades quietly.
func buildTypeIndex(d *Data) {
	d.typeRules = make(map[string]string)
	for cardType, num := range cardTypeRules {
		idx, ok := d.Index[num]
		if !ok {
			continue
		}
		// The category is named in the plural, and not always regularly
		// ("Sorceries", "Phenomena"), so compare on a shared prefix.
		need := len(cardType)
		if need > 5 {
			need = 5
		}
		if commonPrefixLen(strings.ToLower(d.Rules[idx].Text), strings.ToLower(cardType)) >= need {
			d.typeRules[cardType] = num
		}
	}
}

func commonPrefixLen(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// alternation builds a case-insensitive regexp matching any of the names,
// longest first so "Double Strike" wins over "Double".
func alternation(names []string) *regexp.Regexp {
	if len(names) == 0 {
		return nil
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	re, err := regexp.Compile(`(?i)` + strings.Join(quoted, "|"))
	if err != nil {
		return nil
	}
	return re
}

// ── Scanning ────────────────────────────────────────────────────

// Span is where something was found in a piece of text.
type Span struct {
	Start, End int
	Text       string
}

// scan finds every whole-word occurrence of the alternation in s.
// Word boundaries are checked here rather than with \b so that names
// ending in punctuation ("For Mirrodin!") still match.
func Scan(re *regexp.Regexp, s string) []Span {
	if re == nil || s == "" {
		return nil
	}

	var out []Span
	pos := 0
	for pos < len(s) {
		loc := re.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		if boundedAt(s, start, end) {
			out = append(out, Span{Start: start, End: end, Text: s[start:end]})
			pos = end
		} else {
			pos = start + 1
		}
	}
	return out
}

func boundedAt(s string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(s[:start])
		first, _ := utf8.DecodeRuneInString(s[start:])
		if isWordRune(before) && isWordRune(first) {
			return false
		}
	}
	if end < len(s) {
		last, _ := utf8.DecodeLastRuneInString(s[:end])
		after, _ := utf8.DecodeRuneInString(s[end:])
		if isWordRune(last) && isWordRune(after) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// ── Matching a card against the rules ───────────────────────────

type MatchKind int

const (
	MatchKeyword MatchKind = iota
	MatchGlossary
	MatchType
)

type RuleMatch struct {
	Kind  MatchKind
	Term  string // display name, e.g. "Flying"
	Rule  string // rule number, may be "" for glossary-only hits
	Kw    Keyword
	Entry GlossaryEntry
}

const maxGlossaryMatches = 8

// MatchCard returns the rules relevant to a card, most specific first:
// keywords in the order they appear in the oracle text, then glossary
// concepts, then the card's types.
func (d Data) MatchCard(c mtg.Card) []RuleMatch {
	if !d.Loaded() {
		return nil
	}

	var out []RuleMatch
	seen := make(map[string]bool)

	// Both halves of a double-faced card, so the back face's keywords
	// still turn up in the rules panel.
	oracle := c.CombinedOracle()

	for _, sp := range Scan(d.kw.regexp(), oracle) {
		kw, ok := d.kw.lookup(sp.Text)
		// A keyword only Scryfall's catalogs know has no rule to show.
		if !ok || kw.Rule == "" {
			continue
		}
		key := kw.Rule
		if kw.Kind == AbilityWord {
			key = kw.Rule + "/" + kw.Name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, RuleMatch{Kind: MatchKeyword, Term: kw.Name, Rule: kw.Rule, Kw: kw})
	}

	glossaryHits := 0
	for _, sp := range Scan(d.glossaryRe, oracle) {
		if glossaryHits >= maxGlossaryMatches {
			break
		}
		entry, ok := d.glossary[strings.ToLower(sp.Text)]
		if !ok {
			continue
		}
		key := "g:" + strings.ToLower(entry.Term)
		if seen[key] {
			continue
		}
		seen[key] = true
		glossaryHits++

		rule := ""
		if m := SeeRuleRe.FindStringSubmatch(entry.Definition); m != nil {
			rule = m[1]
		}
		out = append(out, RuleMatch{Kind: MatchGlossary, Term: entry.Term, Rule: rule, Entry: entry})
	}

	for _, t := range cardTypes(c.TypeLine) {
		num, ok := d.typeRules[t]
		if !ok || seen["t:"+num] {
			continue
		}
		seen["t:"+num] = true
		out = append(out, RuleMatch{Kind: MatchType, Term: t, Rule: num})
	}

	return out
}

// cardTypes returns the card types on a type line, ignoring subtypes.
func cardTypes(typeLine string) []string {
	if typeLine == "" {
		return nil
	}
	// Only look at the front half of "Legendary Creature — Dragon"
	if i := strings.Index(typeLine, "—"); i >= 0 {
		typeLine = typeLine[:i]
	}
	// Double-faced type lines are joined with "//"
	typeLine = strings.ReplaceAll(typeLine, "//", " ")

	var out []string
	for _, word := range strings.Fields(typeLine) {
		word = strings.TrimSpace(word)
		if _, ok := cardTypeRules[word]; ok {
			out = append(out, word)
		}
	}
	return out
}

// ── File management ─────────────────────────────────────────────

// rulesURL is the last link known at build time. It is only a fallback: the
// live link is discovered from the rules page (sync.go), because the rules
// update a few times a year and the filename is date-stamped.
const rulesURL = "https://media.wizards.com/2026/downloads/MagicCompRules%2020260417.txt"

func FilePath() string {
	return filepath.Join(paths.Cache(), "comprules.txt")
}

// Download fetches the current rules the first time they are needed. It tries
// the live link and falls back to the built-in one, and records the version so
// a later Sync knows what is already on disk.
func Download() error {
	url, version, err := LatestURL()
	if err != nil {
		url, version = rulesURL, versionFromURL(rulesURL)
	}
	body, err := fetch.GetFile(url)
	if err != nil {
		return err
	}
	if err := os.WriteFile(FilePath(), body, 0644); err != nil {
		return err
	}
	m := readMeta()
	m.Current = version
	writeMeta(m)
	return nil
}

func Load() (Data, error) {
	path := FilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := Download(); err != nil {
			return Data{}, err
		}
	}

	text, err := readText(path)
	if err != nil {
		return Data{}, err
	}
	return Parse(text), nil
}

// readText reads a rules file and normalises it: the byte-order mark some
// downloads carry, and Windows line endings, both gone so the parser and the
// diff see the same text however it arrived.
func readText(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.TrimPrefix(string(content), "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text, nil
}

// KeywordSpan is a keyword the rules know about, found in a piece of text.
type KeywordSpan struct {
	Span
	Kind KeywordKind
}

// KeywordSpans finds every keyword ability, keyword action and ability word
// in text, so a caller can style them without knowing how the index is
// built. Returns nothing when the rules haven't been loaded.
func (d Data) KeywordSpans(text string) []KeywordSpan {
	if !d.Loaded() {
		return nil
	}
	var out []KeywordSpan
	for _, sp := range Scan(d.kw.regexp(), text) {
		kw, ok := d.kw.lookup(sp.Text)
		if !ok {
			continue
		}
		out = append(out, KeywordSpan{Span: sp, Kind: kw.Kind})
	}
	return out
}

// ── Zones ───────────────────────────────────────────────────────

// zones are the seven places a card can be (rule 400.1). They come through
// MatchCard as glossary terms like anything else, but they are worth
// separating when a card's rules are listed: "this card cares about the
// graveyard" is a different kind of fact from "this card has flying".
var zones = map[string]bool{
	"hand": true, "library": true, "battlefield": true, "graveyard": true,
	"stack": true, "exile": true, "command zone": true,
}

// IsZone reports whether a glossary term names one of the game's zones.
func IsZone(term string) bool { return zones[strings.ToLower(term)] }

// Search finds the rules whose number or text contains every term, and the
// glossary entries that do. Substring rather than fuzzy: a rulebook is a
// million characters, and a fuzzy match over it returns everything.
//
// The caller splits the query, because how a query is written is the user
// interface's business and this package has no opinion about quotes.
func (d Data) Search(terms []string) ([]Rule, []GlossaryEntry) {
	if len(terms) == 0 {
		return nil, nil
	}

	var hits []Rule
	for _, r := range d.Rules {
		// A section or category heading has no text worth listing on its
		// own; the rules under it are the answer.
		if r.Depth < 2 {
			continue
		}
		if containsAll(strings.ToLower(r.Number+" "+r.Text), terms) {
			hits = append(hits, r)
		}
	}

	var entries []GlossaryEntry
	for _, g := range d.Glossary {
		if containsAll(strings.ToLower(g.Term+" "+g.Definition), terms) {
			entries = append(entries, g)
		}
	}
	return hits, entries
}

func containsAll(hay string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

// Rule looks a rule up by its number.
func (d Data) Rule(number string) (Rule, bool) {
	i, ok := d.Index[number]
	if !ok || i < 0 || i >= len(d.Rules) {
		return Rule{}, false
	}
	return d.Rules[i], true
}

// Subrules are the lettered parts under a rule — 702.9a and friends — which
// is where the actual behaviour usually lives.
func (d Data) Subrules(number string) []Rule {
	r, ok := d.Rule(number)
	if !ok {
		return nil
	}
	out := make([]Rule, 0, len(r.Children))
	for _, i := range r.Children {
		if i >= 0 && i < len(d.Rules) {
			out = append(out, d.Rules[i])
		}
	}
	return out
}

// Cached reports whether the rulebook is already on disk. Parsing it to
// highlight a card's text is worth doing; downloading a megabyte to do so,
// before anyone has asked about a rule, is not.
func Cached() bool {
	_, err := os.Stat(FilePath())
	return err == nil
}

// exampleMark is how the rulebook starts an example. Each one is a line of
// its own in the file, which the parser joins onto the rule with a space.
const exampleMark = "Example: "

// SplitExamples separates a rule's text from the examples that follow it, so
// they can be set apart. Text keeps them, so a search still finds a word that
// only an example uses.
func SplitExamples(text string) (body string, examples []string) {
	parts := strings.Split(text, " "+exampleMark)
	body = parts[0]
	if strings.HasPrefix(body, exampleMark) {
		return "", append([]string{strings.TrimPrefix(body, exampleMark)}, parts[1:]...)
	}
	return body, parts[1:]
}
