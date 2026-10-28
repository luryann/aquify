package training

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ─── Vocabulary ───────────────────────────────────────────────────────────────
//
// Intensity scheme: USA Swimming energy-system zones. The admin UI maps each
// zone to a fixed colour so coaches used to Commit-style colour zones get the
// same at-a-glance read:
//
//	REC  recovery / easy          (white)
//	EN1  aerobic base             (pink)
//	EN2  threshold                (red)
//	EN3  VO2 / overload           (blue)
//	SP1  lactate tolerance / race pace (purple)
//	SP2  lactate production       (green)
//	SP3  alactic sprint           (yellow)

// Course codes.
const (
	CourseSCY = "SCY"
	CourseSCM = "SCM"
	CourseLCM = "LCM"
)

// YardsPerMeter converts metre distances to yards for cross-course analytics.
const YardsPerMeter = 1.0936133

// Default per-100 pace (seconds) used to estimate swim time when a set has a
// rest interval instead of a send-off. Kick/drill are slower than swim.
var basePacePer100 = map[string]int{
	"free": 90, "back": 100, "breast": 110, "fly": 100, "im": 105,
	"choice": 95, "kick": 120, "pull": 95, "drill": 115,
}

// Limits keep a single workout document bounded.
const (
	MaxSections        = 20
	MaxSetsPerSection  = 60
	MaxNestingDepth    = 3
	MaxRepeats         = 99
	MaxSetDistance     = 10000
	MaxIntervalSeconds = 3600
	MaxWorkoutDistance = 40000
	MaxTitleLength     = 160
	MaxNotesLength     = 500
	MaxTags            = 12
	MaxTagLength       = 32
)

// SectionKinds lists the allowed section kinds in display order.
var SectionKinds = []string{"warmup", "pre_set", "main", "kick", "pull", "drill", "cooldown", "custom"}

// Strokes lists the allowed set strokes / activity categories.
var Strokes = []string{"free", "back", "breast", "fly", "im", "choice", "kick", "pull", "drill"}

// Zones lists the allowed energy-system zones, easiest first.
var Zones = []string{"REC", "EN1", "EN2", "EN3", "SP1", "SP2", "SP3"}

// Equipment lists the recognised equipment codes.
var Equipment = []string{"fins", "paddles", "snorkel", "pull_buoy", "kickboard", "band", "parachute", "drag_suit", "tempo_trainer"}

// ZoneUnassigned is the per-zone bucket for distance without an explicit zone.
const ZoneUnassigned = "unassigned"

func setOf(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

var (
	validSectionKinds = setOf(SectionKinds)
	validStrokes      = setOf(Strokes)
	validZones        = setOf(Zones)
	validEquipment    = setOf(Equipment)
	validCourses      = setOf([]string{CourseSCY, CourseSCM, CourseLCM})
)

// ─── Types ────────────────────────────────────────────────────────────────────

// WorkoutSet is either a leaf set (repeats × distance) or a round
// (repeats × {sets}). Stroke, zone and equipment set on a round are inherited
// by children that leave them empty.
type WorkoutSet struct {
	Repeats         int          `json:"repeats"`
	Distance        int          `json:"distance,omitempty"`
	Stroke          string       `json:"stroke,omitempty"`
	IntervalSeconds int          `json:"interval_seconds,omitempty"`
	RestSeconds     int          `json:"rest_seconds,omitempty"`
	Zone            string       `json:"zone,omitempty"`
	Equipment       []string     `json:"equipment,omitempty"`
	Notes           string       `json:"notes,omitempty"`
	Sets            []WorkoutSet `json:"sets,omitempty"`
}

// IsRound reports whether the set is a nested round.
func (ws WorkoutSet) IsRound() bool { return len(ws.Sets) > 0 }

// WorkoutSection is an ordered block within a workout.
type WorkoutSection struct {
	Kind  string       `json:"kind"`
	Title string       `json:"title,omitempty"`
	Notes string       `json:"notes,omitempty"`
	Sets  []WorkoutSet `json:"sets"`
}

// SectionTotal summarises one section.
type SectionTotal struct {
	Index            int    `json:"index"`
	Kind             string `json:"kind"`
	Title            string `json:"title,omitempty"`
	Distance         int    `json:"distance"`
	EstimatedSeconds int    `json:"estimated_seconds"`
}

// WorkoutTotals is computed server-side from the structured sections.
type WorkoutTotals struct {
	Distance         int            `json:"distance"`
	Unit             string         `json:"unit"` // yd | m
	DistanceYards    int            `json:"distance_yards"`
	Lengths          int            `json:"lengths"`
	ByStroke         map[string]int `json:"by_stroke"`
	ByZone           map[string]int `json:"by_zone"`
	BySection        []SectionTotal `json:"by_section"`
	EstimatedSeconds int            `json:"estimated_seconds"`
	Warnings         []string       `json:"warnings"`
}

// Workout is a structured practice plan. Templates live in the library;
// non-template workouts are instances owned by a practice.
type Workout struct {
	ID              string           `json:"id"`
	OrgID           string           `json:"org_id"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	Course          string           `json:"course"`
	IsTemplate      bool             `json:"is_template"`
	Tags            []string         `json:"tags"`
	Sections        []WorkoutSection `json:"sections"`
	SourceWorkoutID string           `json:"source_workout_id,omitempty"`
	Totals          WorkoutTotals    `json:"totals"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	CreatedBy       string           `json:"created_by"`
	UpdatedBy       string           `json:"updated_by"`
}

// WorkoutInput is the writable shape of a workout.
type WorkoutInput struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Course      string           `json:"course"`
	IsTemplate  bool             `json:"is_template"`
	Tags        []string         `json:"tags"`
	Sections    []WorkoutSection `json:"sections"`
}

// ─── Validation ───────────────────────────────────────────────────────────────

// ErrInvalidWorkout wraps every workout validation failure.
var ErrInvalidWorkout = errors.New("invalid workout")

// WorkoutValidationError carries the offending field path.
type WorkoutValidationError struct {
	Field   string
	Message string
}

func (e *WorkoutValidationError) Error() string { return e.Field + ": " + e.Message }

// Unwrap lets errors.Is(err, ErrInvalidWorkout) match.
func (e *WorkoutValidationError) Unwrap() error { return ErrInvalidWorkout }

func invalid(field, format string, args ...any) error {
	return &WorkoutValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// NormalizeCourse upper-cases a course code, defaulting to SCY.
func NormalizeCourse(course string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(course))
	if c == "" {
		return CourseSCY, nil
	}
	if !validCourses[c] {
		return "", invalid("course", "must be SCY, SCM, or LCM")
	}
	return c, nil
}

// NormalizeWorkoutInput trims, canonicalises and validates a workout input.
func NormalizeWorkoutInput(in WorkoutInput) (WorkoutInput, error) {
	out := WorkoutInput{
		Title:       strings.TrimSpace(in.Title),
		Description: strings.TrimSpace(in.Description),
		IsTemplate:  in.IsTemplate,
	}
	if out.Title == "" {
		return out, invalid("title", "is required")
	}
	if len(out.Title) > MaxTitleLength {
		return out, invalid("title", "must be at most %d characters", MaxTitleLength)
	}
	if len(out.Description) > 4000 {
		return out, invalid("description", "must be at most 4000 characters")
	}
	course, err := NormalizeCourse(in.Course)
	if err != nil {
		return out, err
	}
	out.Course = course
	tags, err := normalizeTags(in.Tags)
	if err != nil {
		return out, err
	}
	out.Tags = tags
	sections, err := NormalizeSections(in.Sections)
	if err != nil {
		return out, err
	}
	out.Sections = sections
	if total := ComputeTotals(out.Sections, out.Course).Distance; total > MaxWorkoutDistance {
		return out, invalid("sections", "total distance %d exceeds the %d limit", total, MaxWorkoutDistance)
	}
	return out, nil
}

func normalizeTags(raw []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range raw {
		tag := strings.ToLower(strings.Join(strings.Fields(t), " "))
		if tag == "" || seen[tag] {
			continue
		}
		if len(tag) > MaxTagLength {
			return nil, invalid("tags", "each tag must be at most %d characters", MaxTagLength)
		}
		seen[tag] = true
		out = append(out, tag)
	}
	if len(out) > MaxTags {
		return nil, invalid("tags", "at most %d tags", MaxTags)
	}
	sort.Strings(out)
	return out, nil
}

// NormalizeSections validates and canonicalises workout sections.
func NormalizeSections(sections []WorkoutSection) ([]WorkoutSection, error) {
	if len(sections) > MaxSections {
		return nil, invalid("sections", "at most %d sections", MaxSections)
	}
	out := make([]WorkoutSection, 0, len(sections))
	for i, sec := range sections {
		path := fmt.Sprintf("sections[%d]", i)
		kind := strings.ToLower(strings.TrimSpace(sec.Kind))
		if kind == "" {
			kind = "custom"
		}
		if !validSectionKinds[kind] {
			return nil, invalid(path+".kind", "must be one of %s", strings.Join(SectionKinds, ", "))
		}
		title := strings.TrimSpace(sec.Title)
		if len(title) > MaxTitleLength {
			return nil, invalid(path+".title", "must be at most %d characters", MaxTitleLength)
		}
		notes := strings.TrimSpace(sec.Notes)
		if len(notes) > MaxNotesLength {
			return nil, invalid(path+".notes", "must be at most %d characters", MaxNotesLength)
		}
		if len(sec.Sets) > MaxSetsPerSection {
			return nil, invalid(path+".sets", "at most %d sets per section", MaxSetsPerSection)
		}
		sets, err := normalizeSets(sec.Sets, path+".sets", 1)
		if err != nil {
			return nil, err
		}
		out = append(out, WorkoutSection{Kind: kind, Title: title, Notes: notes, Sets: sets})
	}
	return out, nil
}

func normalizeSets(sets []WorkoutSet, path string, depth int) ([]WorkoutSet, error) {
	out := make([]WorkoutSet, 0, len(sets))
	for i, set := range sets {
		p := fmt.Sprintf("%s[%d]", path, i)
		n, err := normalizeSet(set, p, depth)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func normalizeSet(set WorkoutSet, path string, depth int) (WorkoutSet, error) {
	out := set
	if out.Repeats == 0 {
		out.Repeats = 1
	}
	if out.Repeats < 1 || out.Repeats > MaxRepeats {
		return out, invalid(path+".repeats", "must be between 1 and %d", MaxRepeats)
	}
	if out.IntervalSeconds < 0 || out.IntervalSeconds > MaxIntervalSeconds {
		return out, invalid(path+".interval_seconds", "must be between 0 and %d", MaxIntervalSeconds)
	}
	if out.RestSeconds < 0 || out.RestSeconds > MaxIntervalSeconds {
		return out, invalid(path+".rest_seconds", "must be between 0 and %d", MaxIntervalSeconds)
	}
	out.Stroke = strings.ToLower(strings.TrimSpace(out.Stroke))
	if out.Stroke != "" && !validStrokes[out.Stroke] {
		return out, invalid(path+".stroke", "must be one of %s", strings.Join(Strokes, ", "))
	}
	out.Zone = strings.ToUpper(strings.TrimSpace(out.Zone))
	if out.Zone != "" && !validZones[out.Zone] {
		return out, invalid(path+".zone", "must be one of %s", strings.Join(Zones, ", "))
	}
	equipment := []string{}
	seen := map[string]bool{}
	for _, e := range out.Equipment {
		code := strings.ToLower(strings.TrimSpace(e))
		if code == "" || seen[code] {
			continue
		}
		if !validEquipment[code] {
			return out, invalid(path+".equipment", "must be one of %s", strings.Join(Equipment, ", "))
		}
		seen[code] = true
		equipment = append(equipment, code)
	}
	if len(equipment) == 0 {
		equipment = nil
	}
	out.Equipment = equipment
	out.Notes = strings.TrimSpace(out.Notes)
	if len(out.Notes) > MaxNotesLength {
		return out, invalid(path+".notes", "must be at most %d characters", MaxNotesLength)
	}
	if len(set.Sets) > 0 {
		if depth >= MaxNestingDepth {
			return out, invalid(path+".sets", "rounds may be nested at most %d levels", MaxNestingDepth-1)
		}
		if out.Distance != 0 {
			return out, invalid(path+".distance", "must be empty on a round (distance comes from its sets)")
		}
		if out.IntervalSeconds != 0 {
			return out, invalid(path+".interval_seconds", "use rest_seconds between rounds instead of a send-off")
		}
		if len(set.Sets) > MaxSetsPerSection {
			return out, invalid(path+".sets", "at most %d sets per round", MaxSetsPerSection)
		}
		children, err := normalizeSets(set.Sets, path+".sets", depth+1)
		if err != nil {
			return out, err
		}
		out.Sets = children
		return out, nil
	}
	out.Sets = nil
	if out.Distance < 1 || out.Distance > MaxSetDistance {
		return out, invalid(path+".distance", "must be between 1 and %d", MaxSetDistance)
	}
	return out, nil
}

// ─── Totals ───────────────────────────────────────────────────────────────────

// CourseUnit returns "yd" for SCY and "m" for metric courses.
func CourseUnit(course string) string {
	if course == CourseSCM || course == CourseLCM {
		return "m"
	}
	return "yd"
}

// PoolLength returns the length of one lap for a course.
func PoolLength(course string) int {
	if course == CourseLCM {
		return 50
	}
	return 25
}

// ToYards converts a course distance to yards (rounded).
func ToYards(distance int, course string) int {
	if CourseUnit(course) == "m" {
		return int(math.Round(float64(distance) * YardsPerMeter))
	}
	return distance
}

type inherited struct {
	stroke string
	zone   string
}

type accum struct {
	byStroke map[string]int
	byZone   map[string]int
	warnings []string
	pool     int
}

// ComputeTotals derives distance, per-stroke/per-zone totals and an estimated
// duration from structured sections. Inputs are assumed normalised; sets
// without a stroke fall back to the section's kick/pull/drill kind, then free.
func ComputeTotals(sections []WorkoutSection, course string) WorkoutTotals {
	if course == "" {
		course = CourseSCY
	}
	a := &accum{byStroke: map[string]int{}, byZone: map[string]int{}, pool: PoolLength(course)}
	totals := WorkoutTotals{Unit: CourseUnit(course), BySection: []SectionTotal{}}
	for i, sec := range sections {
		base := inherited{}
		switch sec.Kind {
		case "kick", "pull", "drill":
			base.stroke = sec.Kind
		}
		dist, secs := 0, 0
		for j, set := range sec.Sets {
			d, t := a.walk(set, base, 1, fmt.Sprintf("Section %d set %d", i+1, j+1))
			dist += d
			secs += t
		}
		totals.BySection = append(totals.BySection, SectionTotal{Index: i, Kind: sec.Kind, Title: sec.Title, Distance: dist, EstimatedSeconds: secs})
		totals.Distance += dist
		totals.EstimatedSeconds += secs
	}
	totals.ByStroke = a.byStroke
	totals.ByZone = a.byZone
	totals.DistanceYards = ToYards(totals.Distance, course)
	totals.Lengths = totals.Distance / a.pool
	totals.Warnings = a.warnings
	if totals.Warnings == nil {
		totals.Warnings = []string{}
	}
	return totals
}

// walk returns (distance, seconds) for one execution of set (including its
// own repeats). mult is the product of all enclosing round counts and is used
// to scale the per-stroke / per-zone buckets.
func (a *accum) walk(set WorkoutSet, parent inherited, mult int, label string) (int, int) {
	ctx := parent
	if set.Stroke != "" {
		ctx.stroke = set.Stroke
	}
	if set.Zone != "" {
		ctx.zone = set.Zone
	}
	reps := set.Repeats
	if reps < 1 {
		reps = 1
	}
	if len(set.Sets) > 0 {
		oneDist, oneSecs := 0, 0
		for k, child := range set.Sets {
			d, t := a.walk(child, ctx, mult*reps, fmt.Sprintf("%s.%d", label, k+1))
			oneDist += d
			oneSecs += t
		}
		// Stroke/zone buckets were already scaled by mult*reps inside the
		// children; the return value is this round's own per-execution total.
		return reps * oneDist, reps * (oneSecs + set.RestSeconds)
	}
	stroke := ctx.stroke
	if stroke == "" {
		stroke = "free"
	}
	zone := ctx.zone
	if zone == "" {
		zone = ZoneUnassigned
	}
	dist := reps * set.Distance
	a.byStroke[stroke] += dist * mult
	a.byZone[zone] += dist * mult
	if set.Distance%a.pool != 0 {
		a.warnings = append(a.warnings, fmt.Sprintf("%s: %d is not a whole number of %d-length laps", label, set.Distance, a.pool))
	}
	var secs int
	if set.IntervalSeconds > 0 {
		secs = reps * set.IntervalSeconds
		swim := estimateSwimSeconds(set.Distance, stroke)
		if set.IntervalSeconds < swim/2 {
			a.warnings = append(a.warnings, fmt.Sprintf("%s: send-off %s looks too fast for %d %s", label, FormatClock(set.IntervalSeconds), set.Distance, stroke))
		}
	} else {
		secs = reps * (estimateSwimSeconds(set.Distance, stroke) + set.RestSeconds)
	}
	return dist, secs
}

func estimateSwimSeconds(distance int, stroke string) int {
	pace, ok := basePacePer100[stroke]
	if !ok {
		pace = 95
	}
	return int(math.Round(float64(distance) * float64(pace) / 100))
}

// FormatClock renders seconds as m:ss (or :ss under a minute).
func FormatClock(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf(":%02d", seconds)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
