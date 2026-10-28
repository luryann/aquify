package training

import (
	"errors"
	"sort"
	"time"
)

// Analytics conventions:
//   - Weeks are ISO weeks (Monday start), keyed by the Monday date.
//   - Volumes are yards; metre workouts are converted (×1.0936).
//   - A practice's planned yards come from its structured workout when it has
//     one, otherwise from yardage_target.
//   - swimmer_yards = planned yards × swimmers marked present.
//   - Attendance % = present / (present + absent). Excused and unmarked
//     swimmers are excluded from the denominator so practices where the coach
//     did not take attendance do not count against anyone.

// ErrInvalidRange is returned for a bad or oversized analytics date range.
var ErrInvalidRange = errors.New("from/to must be YYYY-MM-DD, from <= to, and span at most 366 days")

// MaxAnalyticsDays bounds an analytics query.
const MaxAnalyticsDays = 366

// ValidateRange checks an inclusive YYYY-MM-DD range.
func ValidateRange(from, to string) error {
	f, err1 := time.Parse("2006-01-02", from)
	t, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil || t.Before(f) || t.Sub(f).Hours()/24 > MaxAnalyticsDays {
		return ErrInvalidRange
	}
	return nil
}

// PlannedYards returns a practice's planned volume in yards.
func PlannedYards(p *Practice, w *Workout) int {
	if w != nil {
		return w.Totals.DistanceYards
	}
	return p.YardageTarget
}

func zoneYards(w *Workout) map[string]int {
	out := map[string]int{}
	if w == nil {
		return out
	}
	for zone, d := range w.Totals.ByZone {
		out[zone] = ToYards(d, w.Course)
	}
	return out
}

func strokeYards(w *Workout) map[string]int {
	out := map[string]int{}
	if w == nil {
		return out
	}
	for stroke, d := range w.Totals.ByStroke {
		out[stroke] = ToYards(d, w.Course)
	}
	return out
}

// practicesInRange returns the org's practices dated within [from, to] plus
// their linked workouts.
func (s *Service) practicesInRange(orgID, from, to string) ([]*Practice, map[string]*Workout, error) {
	all, err := s.List(orgID)
	if err != nil {
		return nil, nil, err
	}
	out := []*Practice{}
	for _, p := range all {
		if p.Date >= from && p.Date <= to {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].ID < out[j].ID
	})
	workouts, err := s.WorkoutsForPractices(orgID, out)
	if err != nil {
		return nil, nil, err
	}
	return out, workouts, nil
}

// AttendanceCounts tallies attendance statuses.
type AttendanceCounts struct {
	Present int `json:"present"`
	Absent  int `json:"absent"`
	Excused int `json:"excused"`
}

func (c *AttendanceCounts) add(status string) {
	switch status {
	case "present":
		c.Present++
	case "absent":
		c.Absent++
	case "excused":
		c.Excused++
	}
}

// Pct returns present / (present + absent) as 0–100, or nil when nothing
// countable was marked.
func (c AttendanceCounts) Pct() *float64 {
	den := c.Present + c.Absent
	if den == 0 {
		return nil
	}
	v := float64(c.Present) * 100 / float64(den)
	v = float64(int(v*10+0.5)) / 10
	return &v
}

// GroupWeek is one group's training load for one ISO week.
type GroupWeek struct {
	WeekStart     string   `json:"week_start"`
	GroupID       string   `json:"group_id"`
	Practices     int      `json:"practices"`
	PlannedYards  int      `json:"planned_yards"`
	SwimmerYards  int      `json:"swimmer_yards"`
	AttendancePct *float64 `json:"attendance_pct"`
	AttendanceCounts
}

// GroupSummary aggregates one group over the whole range.
type GroupSummary struct {
	GroupID       string   `json:"group_id"`
	Practices     int      `json:"practices"`
	PlannedYards  int      `json:"planned_yards"`
	SwimmerYards  int      `json:"swimmer_yards"`
	AttendancePct *float64 `json:"attendance_pct"`
	AttendanceCounts
}

// TrainingAnalytics is the coach dashboard payload.
type TrainingAnalytics struct {
	From                string         `json:"from"`
	To                  string         `json:"to"`
	Weeks               []GroupWeek    `json:"weeks"`
	Groups              []GroupSummary `json:"groups"`
	ZoneYards           map[string]int `json:"zone_yards"`
	StrokeYards         map[string]int `json:"stroke_yards"`
	TotalPractices      int            `json:"total_practices"`
	StructuredPractices int            `json:"structured_practices"`
}

// GroupAnalytics computes weekly volume, zone/stroke distribution and
// attendance for every group (or only groupFilter) in [from, to].
func (s *Service) GroupAnalytics(orgID, from, to, groupFilter string, canon func(string) string) (*TrainingAnalytics, error) {
	if err := ValidateRange(from, to); err != nil {
		return nil, err
	}
	if canon == nil {
		canon = func(v string) string { return v }
	}
	all, workouts, err := s.practicesInRange(orgID, from, to)
	if err != nil {
		return nil, err
	}
	practices := []*Practice{}
	for _, p := range all {
		if groupFilter == "" || canon(p.GroupID) == groupFilter {
			practices = append(practices, p)
		}
	}
	ids := make([]string, 0, len(practices))
	for _, p := range practices {
		ids = append(ids, p.ID)
	}
	records, err := s.attendanceForPractices(orgID, ids)
	if err != nil {
		return nil, err
	}
	byPractice := map[string]*AttendanceCounts{}
	for _, rec := range records {
		c := byPractice[rec.PracticeID]
		if c == nil {
			c = &AttendanceCounts{}
			byPractice[rec.PracticeID] = c
		}
		c.add(rec.Status)
	}

	out := &TrainingAnalytics{From: from, To: to, Weeks: []GroupWeek{}, Groups: []GroupSummary{}, ZoneYards: map[string]int{}, StrokeYards: map[string]int{}}
	type wk struct{ week, group string }
	weekRows := map[wk]*GroupWeek{}
	groupRows := map[string]*GroupSummary{}
	if groupFilter != "" {
		groupRows[groupFilter] = &GroupSummary{GroupID: groupFilter}
	}
	for _, p := range practices {
		g := canon(p.GroupID)
		w := workouts[p.WorkoutID]
		yards := PlannedYards(p, w)
		counts := AttendanceCounts{}
		if c := byPractice[p.ID]; c != nil {
			counts = *c
		}
		k := wk{WeekStart(p.Date), g}
		row := weekRows[k]
		if row == nil {
			row = &GroupWeek{WeekStart: k.week, GroupID: g}
			weekRows[k] = row
		}
		sum := groupRows[g]
		if sum == nil {
			sum = &GroupSummary{GroupID: g}
			groupRows[g] = sum
		}
		for _, target := range []*AttendanceCounts{&row.AttendanceCounts, &sum.AttendanceCounts} {
			target.Present += counts.Present
			target.Absent += counts.Absent
			target.Excused += counts.Excused
		}
		row.Practices++
		row.PlannedYards += yards
		row.SwimmerYards += yards * counts.Present
		sum.Practices++
		sum.PlannedYards += yards
		sum.SwimmerYards += yards * counts.Present
		out.TotalPractices++
		if w != nil {
			out.StructuredPractices++
			for z, y := range zoneYards(w) {
				out.ZoneYards[z] += y
			}
			for st, y := range strokeYards(w) {
				out.StrokeYards[st] += y
			}
		}
	}

	// Dense weeks per group so charts show zero-volume weeks explicitly.
	weeks := weekStarts(from, to)
	groupIDs := make([]string, 0, len(groupRows))
	for g := range groupRows {
		groupIDs = append(groupIDs, g)
	}
	sort.Strings(groupIDs)
	for _, g := range groupIDs {
		sum := groupRows[g]
		sum.AttendancePct = sum.AttendanceCounts.Pct()
		out.Groups = append(out.Groups, *sum)
		for _, ws := range weeks {
			row := weekRows[wk{ws, g}]
			if row == nil {
				row = &GroupWeek{WeekStart: ws, GroupID: g}
			}
			row.AttendancePct = row.AttendanceCounts.Pct()
			out.Weeks = append(out.Weeks, *row)
		}
	}
	return out, nil
}

// SwimmerWeek is one swimmer's training load for one ISO week.
type SwimmerWeek struct {
	WeekStart     string   `json:"week_start"`
	Yards         int      `json:"yards"`
	Scheduled     int      `json:"scheduled"`
	AttendancePct *float64 `json:"attendance_pct"`
	AttendanceCounts
}

// SwimmerAnalytics is the per-swimmer volume and attendance payload.
type SwimmerAnalytics struct {
	SwimmerID     string         `json:"swimmer_id"`
	From          string         `json:"from"`
	To            string         `json:"to"`
	Yards         int            `json:"yards"`
	Scheduled     int            `json:"scheduled"`
	AttendancePct *float64       `json:"attendance_pct"`
	Weeks         []SwimmerWeek  `json:"weeks"`
	ZoneYards     map[string]int `json:"zone_yards"`
	AttendanceCounts
}

// SwimmerAnalytics computes a swimmer's attended volume and attendance in
// [from, to]. groups holds the swimmer's canonical group IDs; "scheduled"
// counts practices for those groups (plus team-wide practices) and any other
// practice where the swimmer was marked.
func (s *Service) SwimmerAnalytics(orgID, swimmerID string, groups map[string]bool, from, to string, canon func(string) string) (*SwimmerAnalytics, error) {
	if err := ValidateRange(from, to); err != nil {
		return nil, err
	}
	if canon == nil {
		canon = func(v string) string { return v }
	}
	practices, workouts, err := s.practicesInRange(orgID, from, to)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(practices))
	for _, p := range practices {
		ids = append(ids, p.ID)
	}
	records, err := s.attendanceForPractices(orgID, ids)
	if err != nil {
		return nil, err
	}
	status := map[string]string{}
	for _, rec := range records {
		if rec.SwimmerID == swimmerID {
			status[rec.PracticeID] = rec.Status
		}
	}

	out := &SwimmerAnalytics{SwimmerID: swimmerID, From: from, To: to, Weeks: []SwimmerWeek{}, ZoneYards: map[string]int{}}
	weekRows := map[string]*SwimmerWeek{}
	for _, p := range practices {
		st, marked := status[p.ID]
		g := canon(p.GroupID)
		inGroup := g == "" || groups[g]
		if !inGroup && !marked {
			continue
		}
		ws := WeekStart(p.Date)
		row := weekRows[ws]
		if row == nil {
			row = &SwimmerWeek{WeekStart: ws}
			weekRows[ws] = row
		}
		row.Scheduled++
		out.Scheduled++
		row.add(st)
		out.add(st)
		if st == "present" {
			w := workouts[p.WorkoutID]
			yards := PlannedYards(p, w)
			row.Yards += yards
			out.Yards += yards
			for z, y := range zoneYards(w) {
				out.ZoneYards[z] += y
			}
		}
	}
	out.AttendancePct = out.AttendanceCounts.Pct()
	for _, ws := range weekStarts(from, to) {
		row := weekRows[ws]
		if row == nil {
			row = &SwimmerWeek{WeekStart: ws}
		}
		row.AttendancePct = row.AttendanceCounts.Pct()
		out.Weeks = append(out.Weeks, *row)
	}
	return out, nil
}
