package training

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ─── Quick-entry parser ───────────────────────────────────────────────────────
//
// ParseQuickEntry converts coach shorthand into structured sections:
//
//	Warmup:
//	400 choice
//	8x50 kick w/ fins @1:00
//	Main
//	3 x {4x100 free @1:30 EN2, 100 easy} rest 1:00
//	3 rounds:
//	  4x50 fly @:50 SP2
//	  200 free r:20 EN1
//	Cooldown: 200 easy
//
// Rules: "N x DIST", "NxDIST" or "DIST"; strokes free/back/breast/fly/IM/choice/
// kick/pull/drill (and long forms); send-off "@1:30", "on 1:30", "@90"; rest
// "r:15", "rest :20", ":20 rest"; zones REC/EN1-3/SP1-3 or easy/aerobic/
// threshold/vo2/race pace/lactate/sprint; equipment fins/paddles/snorkel/
// buoy/board/band/parachute/drag suit. Rounds use "N x {...}" / "N x (...)"
// (inline, comma or semicolon separated, or spanning lines until "}") or an
// "N rounds:" header followed by indented lines. Unrecognised words are kept
// as set notes. Lines that cannot be parsed are reported with their line
// number instead of being dropped.

// ParseError describes one quick-entry line that could not be parsed.
type ParseError struct {
	Line    int    `json:"line"`
	Text    string `json:"text"`
	Message string `json:"message"`
}

// ParseResult is the structured output of ParseQuickEntry.
type ParseResult struct {
	Sections []WorkoutSection `json:"sections"`
	Errors   []ParseError     `json:"errors"`
}

var (
	reSetHead     = regexp.MustCompile(`^(?:(\d+)\s*(?:x|×|\*)\s*)?(\d+)`)
	reUnit        = regexp.MustCompile(`^\s*(?:yards?|yds?|y|meters?|metres?|m)\b`)
	reRoundHead   = regexp.MustCompile(`^(\d+)\s*(?:x|×|\*|rounds?(?:\s+of)?)\s*([{(:])\s*(.*)$`)
	reRoundHeadNL = regexp.MustCompile(`^(\d+)\s*(?:x|×|\*|rounds?(?:\s+of)?)\s*$`)
	reInterval    = regexp.MustCompile(`(?:@|\bon(?:\s+the)?\b)\s*(\d{1,2}:\d{2}|:\d{2}|\d{1,4})\b`)
	reRestBefore  = regexp.MustCompile(`\b(?:rest|r)\s*:?\s*(\d{1,2}:\d{2}|:\d{2}|\d{1,4})\s*(?:s|sec|secs|seconds)?\b`)
	reRestAfter   = regexp.MustCompile(`(\d{1,2}:\d{2}|:\d{2}|\d{1,4})\s*(?:s|sec|secs|seconds)?\s*(?:rest|r)\b`)
	reBullet      = regexp.MustCompile(`^(?:[-*•]\s+|\d+[.)]\s+)`)
	reSectionHead = regexp.MustCompile(`^(warm[\s-]?up|wu|pre[\s-]?set|main(?:\s+set)?|kick(?:\s+set)?|pull(?:\s+set)?|drills?(?:\s+set)?|cool[\s-]?down|cd|warm[\s-]?down|swim[\s-]?down)\s*(?:[:\-–—]\s*(.*)|\s+(\d.*))?$`)
	// Every replacement keeps the byte length so original-case notes stay aligned.
	reMultiWord = strings.NewReplacer("pull buoy", "pull_buoy", "pull-buoy", "pull_buoy", "drag suit", "drag_suit", "kick board", "kick_board", "race pace", "race_pace", "tempo trainer", "tempo_trainer")
)

var strokeAliases = map[string]string{
	"free": "free", "freestyle": "free", "fr": "free", "fs": "free",
	"back": "back", "backstroke": "back", "bk": "back",
	"breast": "breast", "breaststroke": "breast", "br": "breast", "brst": "breast",
	"fly": "fly", "butterfly": "fly", "fl": "fly",
	"im": "im", "choice": "choice", "ch": "choice", "chc": "choice",
	"kick": "kick", "k": "kick",
	"pull": "pull", "pl": "pull",
	"drill": "drill", "drills": "drill", "dr": "drill",
}

var zoneAliases = map[string]string{
	"rec": "REC", "easy": "REC", "ez": "REC", "recovery": "REC", "loosen": "REC",
	"en1": "EN1", "aerobic": "EN1",
	"en2": "EN2", "threshold": "EN2",
	"en3": "EN3", "vo2": "EN3", "overload": "EN3",
	"sp1": "SP1", "race_pace": "SP1",
	"sp2": "SP2", "lactate": "SP2",
	"sp3": "SP3", "sprint": "SP3",
}

var equipmentAliases = map[string]string{
	"fins": "fins", "fin": "fins", "zoomers": "fins",
	"paddles": "paddles", "paddle": "paddles",
	"snorkel": "snorkel", "snorkels": "snorkel",
	"pull_buoy": "pull_buoy", "buoy": "pull_buoy",
	"kickboard": "kickboard", "kick_board": "kickboard", "board": "kickboard",
	"band": "band", "strap": "band",
	"parachute": "parachute", "chute": "parachute",
	"drag_suit":     "drag_suit",
	"tempo_trainer": "tempo_trainer",
}

var fillerWords = map[string]bool{"w/": true, "with": true, "on": true, "the": true, "of": true, "swim": true, "and": true, "&": true, "+": true}

func sectionKindForHeader(h string) string {
	h = strings.ToLower(h)
	switch {
	case strings.HasPrefix(h, "warm") && !strings.Contains(h, "down"), h == "wu":
		return "warmup"
	case strings.HasPrefix(h, "pre"):
		return "pre_set"
	case strings.HasPrefix(h, "main"):
		return "main"
	case strings.HasPrefix(h, "kick"):
		return "kick"
	case strings.HasPrefix(h, "pull"):
		return "pull"
	case strings.HasPrefix(h, "drill"):
		return "drill"
	default:
		return "cooldown"
	}
}

// parseClock parses "1:30", ":45" or "90" into seconds.
func parseClock(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty time")
	}
	if strings.Contains(raw, ":") {
		parts := strings.SplitN(raw, ":", 2)
		mins := 0
		if parts[0] != "" {
			m, err := strconv.Atoi(parts[0])
			if err != nil {
				return 0, err
			}
			mins = m
		}
		secs, err := strconv.Atoi(parts[1])
		if err != nil || secs >= 60 {
			return 0, fmt.Errorf("invalid time %q", raw)
		}
		return mins*60 + secs, nil
	}
	return strconv.Atoi(raw)
}

type roundFrame struct {
	set    *WorkoutSet
	indent int  // header indentation for ":"-style rounds
	brace  bool // closed by "}" / ")"
}

type quickParser struct {
	sections []WorkoutSection
	errors   []ParseError
	stack    []*roundFrame
	// lastClosed is the round most recently closed by "}" / ")".
	lastClosed *WorkoutSet
}

func (p *quickParser) currentSection() *WorkoutSection {
	if len(p.sections) == 0 {
		p.sections = append(p.sections, WorkoutSection{Kind: "main", Sets: []WorkoutSet{}})
	}
	return &p.sections[len(p.sections)-1]
}

// addSet appends to the innermost open round or the current section and
// returns a pointer to the stored set.
func (p *quickParser) addSet(set WorkoutSet) *WorkoutSet {
	if n := len(p.stack); n > 0 {
		parent := p.stack[n-1].set
		parent.Sets = append(parent.Sets, set)
		return &parent.Sets[len(parent.Sets)-1]
	}
	sec := p.currentSection()
	sec.Sets = append(sec.Sets, set)
	return &sec.Sets[len(sec.Sets)-1]
}

func (p *quickParser) fail(line int, text, msg string) {
	p.errors = append(p.errors, ParseError{Line: line, Text: text, Message: msg})
}

// ParseQuickEntry parses shorthand text into structured workout sections.
// The returned sections are normalised; parse errors are per line.
func ParseQuickEntry(text string) ParseResult {
	p := &quickParser{}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for idx, raw := range lines {
		lineNo := idx + 1
		expanded := strings.ReplaceAll(raw, "\t", "    ")
		trimmed := strings.TrimSpace(expanded)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		indent := len(expanded) - len(strings.TrimLeft(expanded, " "))
		if reBullet.MatchString(trimmed) {
			// A bullet nests one level deeper than an un-bulleted line.
			indent += 2
		}
		// Close indentation-scoped rounds that this line has dedented out of.
		for len(p.stack) > 0 {
			top := p.stack[len(p.stack)-1]
			if top.brace || indent > top.indent {
				break
			}
			p.stack = p.stack[:len(p.stack)-1]
		}
		trimmed = strings.TrimSpace(reBullet.ReplaceAllString(trimmed, ""))
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "total") {
			continue
		}
		p.parseLine(trimmed, indent, lineNo)
	}
	for _, frame := range p.stack {
		if frame.brace {
			p.fail(len(lines), "", "round opened with \"{\" or \"(\" was never closed")
		}
	}
	p.stack = nil

	out := make([]WorkoutSection, 0, len(p.sections))
	for _, sec := range p.sections {
		if len(sec.Sets) == 0 && sec.Notes == "" {
			continue
		}
		out = append(out, sec)
	}
	normalized, err := NormalizeSections(out)
	if err != nil {
		p.fail(0, "", err.Error())
		normalized = out
	}
	errs := p.errors
	if errs == nil {
		errs = []ParseError{}
	}
	return ParseResult{Sections: normalized, Errors: errs}
}

func (p *quickParser) parseLine(text string, indent, lineNo int) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// Closing a brace round, optionally followed by round modifiers.
	if text[0] == '}' || text[0] == ')' {
		if !p.popBrace() {
			p.fail(lineNo, text, "closing bracket without an open round")
			return
		}
		if rest := strings.TrimSpace(text[1:]); rest != "" {
			p.applyRoundModifiers(p.lastClosed, rest)
		}
		return
	}
	lower := strings.ToLower(text)

	// Section header (never starts with a digit).
	if m := reSectionHead.FindStringSubmatch(lower); m != nil {
		kind := sectionKindForHeader(m[1])
		p.stack = nil
		p.sections = append(p.sections, WorkoutSection{Kind: kind, Sets: []WorkoutSet{}})
		content := m[2]
		if content == "" {
			content = m[3]
		}
		if content = strings.TrimSpace(content); content != "" {
			p.parseLine(originalTail(text, content), indent+1, lineNo)
		}
		return
	}
	if text[0] < '0' || text[0] > '9' {
		if strings.HasSuffix(text, ":") {
			title := strings.TrimSpace(strings.TrimSuffix(text, ":"))
			p.stack = nil
			p.sections = append(p.sections, WorkoutSection{Kind: "custom", Title: title, Sets: []WorkoutSet{}})
			return
		}
		p.fail(lineNo, text, "expected a distance such as \"400\" or \"8x100\"")
		return
	}

	// Round header: "3 x {", "3 rounds:", "3x (4x100, 200)".
	if m := reRoundHead.FindStringSubmatch(lower); m != nil {
		reps, _ := strconv.Atoi(m[1])
		opener := m[2]
		restText := originalTail(text, m[3])
		round := p.addSet(WorkoutSet{Repeats: reps, Sets: []WorkoutSet{}})
		if opener == ":" {
			p.stack = append(p.stack, &roundFrame{set: round, indent: indent})
			if strings.TrimSpace(restText) != "" {
				p.parseLine(restText, indent+1, lineNo)
			}
			return
		}
		inner, after, closed := splitBalanced(restText)
		p.stack = append(p.stack, &roundFrame{set: round, brace: true})
		for _, piece := range splitTopLevel(inner) {
			p.parseLine(piece, indent+1, lineNo)
		}
		if closed {
			p.popBrace()
			if strings.TrimSpace(after) != "" {
				p.applyRoundModifiers(round, after)
			}
		}
		return
	}
	if m := reRoundHeadNL.FindStringSubmatch(lower); m != nil {
		reps, _ := strconv.Atoi(m[1])
		round := p.addSet(WorkoutSet{Repeats: reps, Sets: []WorkoutSet{}})
		p.stack = append(p.stack, &roundFrame{set: round, indent: indent})
		return
	}

	// Inline lists inside a brace round on one physical line are split by
	// the caller; a top-level line is a single set.
	set, err := ParseSetLine(text)
	if err != nil {
		p.fail(lineNo, text, err.Error())
		return
	}
	p.addSet(set)
}

// originalTail returns the part of text that corresponds to lowerTail, a
// suffix of strings.ToLower(text). Lowercasing can change byte lengths for
// some non-ASCII runes, but it maps rune for rune, so the suffix is located by
// rune count rather than by byte offset.
func originalTail(text, lowerTail string) string {
	n := utf8.RuneCountInString(lowerTail)
	i := len(text)
	for ; n > 0 && i > 0; n-- {
		_, size := utf8.DecodeLastRuneInString(text[:i])
		i -= size
	}
	return text[i:]
}

func (p *quickParser) popBrace() bool {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].brace {
			p.lastClosed = p.stack[i].set
			p.stack = p.stack[:i]
			return true
		}
	}
	return false
}

func (p *quickParser) applyRoundModifiers(round *WorkoutSet, text string) {
	if round == nil {
		return
	}
	mods := parseModifiers(text)
	if mods.interval > 0 {
		// A send-off on a whole round is really rest between rounds.
		mods.rest = mods.interval
	}
	round.RestSeconds = mods.rest
	if mods.stroke != "" {
		round.Stroke = mods.stroke
	}
	if mods.zone != "" {
		round.Zone = mods.zone
	}
	round.Equipment = mergeEquipment(round.Equipment, mods.equipment)
	if mods.notes != "" {
		round.Notes = strings.TrimSpace(round.Notes + " " + mods.notes)
	}
}

// splitBalanced returns the text inside the first balanced open/close pair
// (s starts just after the opener), the remainder after the closer, and
// whether a closer was found.
func splitBalanced(s string) (string, string, bool) {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '(':
			depth++
		case '}', ')':
			depth--
			if depth == 0 {
				return s[:i], s[i+1:], true
			}
		}
	}
	return s, "", false
}

// splitTopLevel splits on "," or ";" outside nested brackets.
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '(':
			depth++
		case '}', ')':
			depth--
		case ',', ';':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	pieces := make([]string, 0, len(out))
	for _, piece := range out {
		if strings.TrimSpace(piece) != "" {
			pieces = append(pieces, strings.TrimSpace(piece))
		}
	}
	return pieces
}

type modifiers struct {
	stroke    string
	zone      string
	interval  int
	rest      int
	equipment []string
	notes     string
}

func parseModifiers(text string) modifiers {
	m := modifiers{}
	lowered := strings.ToLower(text)
	work := reMultiWord.Replace(lowered)
	// Keep original casing for notes: mirror the underscore joins onto the
	// original text (all replacements are length-preserving).
	original := work
	if len(text) == len(lowered) && len(work) == len(lowered) {
		ob := []byte(text)
		for i := range ob {
			if work[i] != lowered[i] {
				ob[i] = work[i]
			}
		}
		original = string(ob)
	}
	mask := func(re *regexp.Regexp, set func(string)) {
		for {
			loc := re.FindStringSubmatchIndex(work)
			if loc == nil {
				return
			}
			set(work[loc[2]:loc[3]])
			blank := strings.Repeat(" ", loc[1]-loc[0])
			work = work[:loc[0]] + blank + work[loc[1]:]
			original = original[:loc[0]] + blank + original[loc[1]:]
		}
	}
	mask(reInterval, func(v string) {
		if secs, err := parseClock(v); err == nil && m.interval == 0 {
			m.interval = secs
		}
	})
	for _, re := range []*regexp.Regexp{reRestBefore, reRestAfter} {
		mask(re, func(v string) {
			if secs, err := parseClock(v); err == nil && m.rest == 0 {
				m.rest = secs
			}
		})
	}

	lowerWords := strings.Fields(work)
	origWords := strings.Fields(original)
	var notes []string
	var activity, swimStroke string
	for i, w := range lowerWords {
		orig := w
		if i < len(origWords) {
			orig = origWords[i]
		}
		token := strings.Trim(w, ",;.!")
		if token == "" {
			continue
		}
		if fillerWords[token] {
			continue
		}
		if s, ok := strokeAliases[token]; ok {
			switch s {
			case "kick", "pull", "drill":
				if activity == "" {
					activity = s
					continue
				}
			default:
				if swimStroke == "" {
					swimStroke = s
					continue
				}
			}
		}
		if z, ok := zoneAliases[token]; ok && z != "" && m.zone == "" {
			m.zone = z
			continue
		}
		if e, ok := equipmentAliases[token]; ok {
			m.equipment = mergeEquipment(m.equipment, []string{e})
			continue
		}
		notes = append(notes, strings.ReplaceAll(strings.Trim(orig, ",;"), "_", " "))
	}
	switch {
	case activity != "" && swimStroke != "":
		// "50 fly kick" → kick, keep the stroke as a note so nothing is lost.
		m.stroke = activity
		notes = append([]string{swimStroke}, notes...)
	case activity != "":
		m.stroke = activity
	default:
		m.stroke = swimStroke
	}
	m.notes = strings.TrimSpace(strings.Join(notes, " "))
	return m
}

func mergeEquipment(existing, extra []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range append(append([]string{}, existing...), extra...) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

// ParseSetLine parses a single leaf set such as "8x100 free @1:30 EN2".
func ParseSetLine(text string) (WorkoutSet, error) {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	loc := reSetHead.FindStringSubmatchIndex(lower)
	if loc == nil {
		return WorkoutSet{}, fmt.Errorf("expected a distance such as \"400\" or \"8x100\"")
	}
	reps := 1
	if loc[2] >= 0 {
		reps, _ = strconv.Atoi(lower[loc[2]:loc[3]])
	}
	dist, _ := strconv.Atoi(lower[loc[4]:loc[5]])
	if reps < 1 || reps > MaxRepeats {
		return WorkoutSet{}, fmt.Errorf("repeats must be between 1 and %d", MaxRepeats)
	}
	if dist < 1 || dist > MaxSetDistance {
		return WorkoutSet{}, fmt.Errorf("distance must be between 1 and %d", MaxSetDistance)
	}
	restStart := loc[1]
	if u := reUnit.FindStringIndex(lower[restStart:]); u != nil {
		restStart += u[1]
	}
	rest := text[restStart:]
	if trimmedRest := strings.TrimSpace(rest); trimmedRest != "" && strings.ContainsRune("0123456789", rune(trimmedRest[0])) && !strings.HasPrefix(trimmedRest, ":") {
		// e.g. "100 200" — two distances on one line is ambiguous.
		if _, err := strconv.Atoi(strings.Fields(trimmedRest)[0]); err == nil {
			return WorkoutSet{}, fmt.Errorf("unexpected number %q after the distance; put each set on its own line", strings.Fields(trimmedRest)[0])
		}
	}
	mods := parseModifiers(rest)
	if mods.interval > MaxIntervalSeconds || mods.rest > MaxIntervalSeconds {
		return WorkoutSet{}, fmt.Errorf("interval/rest must be at most %s", FormatClock(MaxIntervalSeconds))
	}
	return WorkoutSet{
		Repeats:         reps,
		Distance:        dist,
		Stroke:          mods.stroke,
		IntervalSeconds: mods.interval,
		RestSeconds:     mods.rest,
		Zone:            mods.zone,
		Equipment:       mods.equipment,
		Notes:           mods.notes,
	}, nil
}

// ─── Text rendering ───────────────────────────────────────────────────────────

var sectionHeaderText = map[string]string{
	"warmup": "Warmup", "pre_set": "Pre-set", "main": "Main set", "kick": "Kick",
	"pull": "Pull", "drill": "Drill", "cooldown": "Cooldown",
}

var equipmentText = map[string]string{
	"pull_buoy": "buoy", "drag_suit": "drag suit", "tempo_trainer": "tempo trainer",
}

// FormatSetText renders a set back to quick-entry shorthand.
func FormatSetText(set WorkoutSet) string {
	var b strings.Builder
	if set.IsRound() {
		children := make([]string, 0, len(set.Sets))
		for _, c := range set.Sets {
			children = append(children, FormatSetText(c))
		}
		fmt.Fprintf(&b, "%dx {%s}", set.Repeats, strings.Join(children, "; "))
	} else if set.Repeats > 1 {
		fmt.Fprintf(&b, "%dx%d", set.Repeats, set.Distance)
	} else {
		fmt.Fprintf(&b, "%d", set.Distance)
	}
	if set.Stroke != "" {
		b.WriteString(" " + set.Stroke)
	}
	if set.IntervalSeconds > 0 {
		b.WriteString(" @" + FormatClock(set.IntervalSeconds))
	}
	if set.RestSeconds > 0 {
		b.WriteString(" rest " + FormatClock(set.RestSeconds))
	}
	if set.Zone != "" {
		b.WriteString(" " + set.Zone)
	}
	for _, e := range set.Equipment {
		label := e
		if t, ok := equipmentText[e]; ok {
			label = t
		}
		b.WriteString(" " + label)
	}
	if set.Notes != "" {
		b.WriteString(" " + set.Notes)
	}
	return b.String()
}

// FormatWorkoutText renders sections as quick-entry text that ParseQuickEntry
// reads back into the same structure.
func FormatWorkoutText(sections []WorkoutSection) string {
	var lines []string
	for _, sec := range sections {
		header := sectionHeaderText[sec.Kind]
		if sec.Kind == "custom" || header == "" {
			header = sec.Title
			if header == "" {
				header = "Set"
			}
		}
		lines = append(lines, header+":")
		for _, set := range sec.Sets {
			lines = append(lines, FormatSetText(set))
		}
	}
	return strings.Join(lines, "\n")
}
