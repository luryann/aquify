package training

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrPhaseNotFound = errors.New("training phase not found")
	ErrPhaseOverlap  = errors.New("training phase overlaps another phase for this group")
)

// PhaseKinds lists periodization phases in typical season order.
var PhaseKinds = []string{"base", "build", "peak", "taper", "recovery"}

var validPhaseKinds = setOf(PhaseKinds)

// MaxWeeklyYardageTarget bounds a phase's weekly target.
const MaxWeeklyYardageTarget = 200000

// PlanPhase is one block of a season's periodization plan for a group.
// Seasons live in the seasons domain; phases reference them by ID.
type PlanPhase struct {
	ID                  string    `json:"id"`
	OrgID               string    `json:"org_id"`
	SeasonID            string    `json:"season_id"`
	GroupID             string    `json:"group_id"`
	Phase               string    `json:"phase"`
	Name                string    `json:"name"`
	StartDate           string    `json:"start_date"`
	EndDate             string    `json:"end_date"`
	WeeklyYardageTarget int       `json:"weekly_yardage_target"`
	Focus               string    `json:"focus"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	CreatedBy           string    `json:"created_by"`
	UpdatedBy           string    `json:"updated_by"`
}

// PhaseInput is the writable shape of a phase.
type PhaseInput struct {
	GroupID             string
	Phase               string
	Name                string
	StartDate           string
	EndDate             string
	WeeklyYardageTarget int
	Focus               string
}

// SeasonWindow carries the season bounds the phase must sit inside.
type SeasonWindow struct {
	ID        string
	StartDate string
	EndDate   string
}

func parseDate(field, v string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	if err != nil {
		return time.Time{}, &WorkoutValidationError{Field: field, Message: "must be YYYY-MM-DD"}
	}
	return t, nil
}

func normalizePhase(in PhaseInput, season SeasonWindow) (PhaseInput, error) {
	out := PhaseInput{
		GroupID:             strings.TrimSpace(in.GroupID),
		Phase:               strings.ToLower(strings.TrimSpace(in.Phase)),
		Name:                strings.TrimSpace(in.Name),
		StartDate:           strings.TrimSpace(in.StartDate),
		EndDate:             strings.TrimSpace(in.EndDate),
		WeeklyYardageTarget: in.WeeklyYardageTarget,
		Focus:               strings.TrimSpace(in.Focus),
	}
	if out.GroupID == "" {
		return out, invalid("group_id", "is required")
	}
	if !validPhaseKinds[out.Phase] {
		return out, invalid("phase", "must be one of %s", strings.Join(PhaseKinds, ", "))
	}
	if len(out.Name) > 80 {
		return out, invalid("name", "must be at most 80 characters")
	}
	if len(out.Focus) > MaxNotesLength {
		return out, invalid("focus", "must be at most %d characters", MaxNotesLength)
	}
	if out.WeeklyYardageTarget < 0 || out.WeeklyYardageTarget > MaxWeeklyYardageTarget {
		return out, invalid("weekly_yardage_target", "must be between 0 and %d", MaxWeeklyYardageTarget)
	}
	start, err := parseDate("start_date", out.StartDate)
	if err != nil {
		return out, err
	}
	end, err := parseDate("end_date", out.EndDate)
	if err != nil {
		return out, err
	}
	if end.Before(start) {
		return out, invalid("end_date", "must be on or after start_date")
	}
	if season.StartDate != "" && out.StartDate < season.StartDate {
		return out, invalid("start_date", "must be within the season (starts %s)", season.StartDate)
	}
	if season.EndDate != "" && out.EndDate > season.EndDate {
		return out, invalid("end_date", "must be within the season (ends %s)", season.EndDate)
	}
	return out, nil
}

// checkPhaseOverlap rejects a phase whose dates intersect another phase of the
// same season and group.
func checkPhaseOverlap(existing []*PlanPhase, p *PlanPhase) error {
	for _, other := range existing {
		if other.ID == p.ID || other.SeasonID != p.SeasonID || other.GroupID != p.GroupID {
			continue
		}
		if p.StartDate <= other.EndDate && other.StartDate <= p.EndDate {
			return fmt.Errorf("%w (%s %s–%s)", ErrPhaseOverlap, other.Phase, other.StartDate, other.EndDate)
		}
	}
	return nil
}

func (s *Service) savePhase(p *PlanPhase) error {
	if s.repo != nil {
		return s.repo.SavePhase(context.Background(), p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := []*PlanPhase{}
	for _, other := range s.phases {
		if other.OrgID == p.OrgID && other.SeasonID == p.SeasonID {
			existing = append(existing, other)
		}
	}
	if err := checkPhaseOverlap(existing, p); err != nil {
		return err
	}
	cp := *p
	s.phases[p.ID] = &cp
	return nil
}

// CreatePhase adds a periodization phase to a season.
func (s *Service) CreatePhase(orgID string, season SeasonWindow, in PhaseInput, actor string) (*PlanPhase, error) {
	norm, err := normalizePhase(in, season)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	p := &PlanPhase{
		ID: newID(), OrgID: orgID, SeasonID: season.ID, GroupID: norm.GroupID, Phase: norm.Phase,
		Name: norm.Name, StartDate: norm.StartDate, EndDate: norm.EndDate,
		WeeklyYardageTarget: norm.WeeklyYardageTarget, Focus: norm.Focus,
		CreatedAt: now, UpdatedAt: now, CreatedBy: actor, UpdatedBy: actor,
	}
	if err := s.savePhase(p); err != nil {
		return nil, err
	}
	return p, nil
}

// GetPhase returns one phase (org-scoped).
func (s *Service) GetPhase(orgID, id string) (*PlanPhase, error) {
	if s.repo != nil {
		return getPhase(context.Background(), s.repo.pool, orgID, id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.phases[id]
	if !ok || p.OrgID != orgID {
		return nil, ErrPhaseNotFound
	}
	cp := *p
	return &cp, nil
}

// UpdatePhase replaces a phase's writable fields.
func (s *Service) UpdatePhase(orgID, id string, season SeasonWindow, in PhaseInput, actor string) (*PlanPhase, error) {
	existing, err := s.GetPhase(orgID, id)
	if err != nil {
		return nil, err
	}
	norm, err := normalizePhase(in, season)
	if err != nil {
		return nil, err
	}
	p := *existing
	p.GroupID, p.Phase, p.Name = norm.GroupID, norm.Phase, norm.Name
	p.StartDate, p.EndDate = norm.StartDate, norm.EndDate
	p.WeeklyYardageTarget, p.Focus = norm.WeeklyYardageTarget, norm.Focus
	p.UpdatedAt, p.UpdatedBy = time.Now().UTC(), actor
	if err := s.savePhase(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// DeletePhase removes a phase.
func (s *Service) DeletePhase(orgID, id string) error {
	if s.repo != nil {
		return s.repo.DeletePhase(context.Background(), orgID, id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.phases[id]
	if !ok || p.OrgID != orgID {
		return ErrPhaseNotFound
	}
	delete(s.phases, id)
	return nil
}

// ListPhases returns a season's phases ordered by group then start date.
func (s *Service) ListPhases(orgID, seasonID string) ([]*PlanPhase, error) {
	if s.repo != nil {
		return s.repo.ListPhases(context.Background(), orgID, seasonID)
	}
	s.mu.RLock()
	out := []*PlanPhase{}
	for _, p := range s.phases {
		if p.OrgID == orgID && p.SeasonID == seasonID {
			cp := *p
			out = append(out, &cp)
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].GroupID != out[j].GroupID {
			return out[i].GroupID < out[j].GroupID
		}
		if out[i].StartDate != out[j].StartDate {
			return out[i].StartDate < out[j].StartDate
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// PlanWeek compares a phase's weekly target with the yardage actually
// planned (sum of practice planned yards) for one group and ISO week.
type PlanWeek struct {
	WeekStart    string `json:"week_start"`
	WeekEnd      string `json:"week_end"`
	GroupID      string `json:"group_id"`
	PhaseID      string `json:"phase_id,omitempty"`
	Phase        string `json:"phase,omitempty"`
	TargetYards  int    `json:"target_yards"`
	PlannedYards int    `json:"planned_yards"`
	Practices    int    `json:"practices"`
}

// SeasonPlan is the periodization view for a season.
type SeasonPlan struct {
	SeasonID  string       `json:"season_id"`
	StartDate string       `json:"start_date"`
	EndDate   string       `json:"end_date"`
	Phases    []*PlanPhase `json:"phases"`
	Weeks     []PlanWeek   `json:"weeks"`
}

// MaxPlanWeeks bounds the weekly breakdown.
const MaxPlanWeeks = 104

// SeasonPlan builds the target-vs-planned weekly breakdown for a season.
// groupFilter limits output to one canonical group; canon maps a practice's
// stored group reference to the canonical group ID used by phases.
func (s *Service) SeasonPlan(orgID string, season SeasonWindow, groupFilter string, canon func(string) string) (*SeasonPlan, error) {
	if canon == nil {
		canon = func(v string) string { return v }
	}
	phases, err := s.ListPhases(orgID, season.ID)
	if err != nil {
		return nil, err
	}
	if groupFilter != "" {
		filtered := []*PlanPhase{}
		for _, p := range phases {
			if p.GroupID == groupFilter {
				filtered = append(filtered, p)
			}
		}
		phases = filtered
	}
	start, end := season.StartDate, season.EndDate
	for _, p := range phases {
		if start == "" || p.StartDate < start {
			start = p.StartDate
		}
		if end == "" || p.EndDate > end {
			end = p.EndDate
		}
	}
	plan := &SeasonPlan{SeasonID: season.ID, StartDate: start, EndDate: end, Phases: phases, Weeks: []PlanWeek{}}
	if start == "" || end == "" || end < start {
		return plan, nil
	}

	groups := []string{}
	seen := map[string]bool{}
	if groupFilter != "" {
		groups = append(groups, groupFilter)
		seen[groupFilter] = true
	}
	for _, p := range phases {
		if !seen[p.GroupID] {
			seen[p.GroupID] = true
			groups = append(groups, p.GroupID)
		}
	}
	sort.Strings(groups)

	practices, workouts, err := s.practicesInRange(orgID, start, end)
	if err != nil {
		return nil, err
	}
	type key struct{ week, group string }
	planned := map[key]int{}
	counts := map[key]int{}
	for _, p := range practices {
		k := key{WeekStart(p.Date), canon(p.GroupID)}
		planned[k] += PlannedYards(p, workouts[p.WorkoutID])
		counts[k]++
	}

	weeks := weekStarts(start, end)
	for _, g := range groups {
		for _, ws := range weeks {
			we := addDays(ws, 6)
			row := PlanWeek{WeekStart: ws, WeekEnd: we, GroupID: g}
			best := 0
			for _, p := range phases {
				if p.GroupID != g {
					continue
				}
				if days := overlapDays(ws, we, p.StartDate, p.EndDate); days > best {
					best = days
					row.PhaseID, row.Phase, row.TargetYards = p.ID, p.Phase, p.WeeklyYardageTarget
				}
			}
			row.PlannedYards = planned[key{ws, g}]
			row.Practices = counts[key{ws, g}]
			plan.Weeks = append(plan.Weeks, row)
		}
	}
	return plan, nil
}

// ─── Date helpers ─────────────────────────────────────────────────────────────

// WeekStart returns the Monday (YYYY-MM-DD) of the ISO week containing date.
func WeekStart(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	offset := (int(t.Weekday()) + 6) % 7
	return t.AddDate(0, 0, -offset).Format("2006-01-02")
}

func addDays(date string, n int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func weekStarts(from, to string) []string {
	out := []string{}
	for ws := WeekStart(from); ws <= to && len(out) < MaxPlanWeeks; ws = addDays(ws, 7) {
		out = append(out, ws)
	}
	return out
}

func overlapDays(aStart, aEnd, bStart, bEnd string) int {
	start, end := aStart, aEnd
	if bStart > start {
		start = bStart
	}
	if bEnd < end {
		end = bEnd
	}
	if end < start {
		return 0
	}
	s, _ := time.Parse("2006-01-02", start)
	e, _ := time.Parse("2006-01-02", end)
	return int(e.Sub(s).Hours()/24) + 1
}
