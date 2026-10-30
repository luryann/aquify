package performance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

var (
	ErrGoalNotFound = errors.New("goal not found")
	ErrGoalInvalid  = errors.New("invalid goal")
)

const maxGoalsPerSwimmer = 40

// Goal is a swimmer's target time for one event/course.
type Goal struct {
	ID             string    `json:"id"`
	OrgID          string    `json:"org_id"`
	SwimmerID      string    `json:"swimmer_id"`
	Distance       int       `json:"distance"`
	Stroke         string    `json:"stroke"`
	Course         string    `json:"course"`
	TargetTimeMs   int       `json:"target_time_ms"`
	BaselineTimeMs *int      `json:"baseline_time_ms,omitempty"` // best time when the goal was set
	Deadline       string    `json:"deadline,omitempty"`         // YYYY-MM-DD
	Notes          string    `json:"notes,omitempty"`
	Status         string    `json:"status"` // active | archived
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedBy      string    `json:"created_by"`
	UpdatedBy      string    `json:"updated_by"`
}

// GoalProgress decorates a goal with the swimmer's current best time.
type GoalProgress struct {
	Goal
	BestTimeMs      *int       `json:"best_time_ms"`
	BestRecordedAt  *time.Time `json:"best_recorded_at,omitempty"`
	GapMs           *int       `json:"gap_ms,omitempty"`           // best − target; ≤0 means achieved
	ProgressPercent *float64   `json:"progress_percent,omitempty"` // 0–100 from baseline toward target
	Achieved        bool       `json:"achieved"`
	DaysRemaining   *int       `json:"days_remaining,omitempty"`
	Overdue         bool       `json:"overdue"`
}

// GoalInput creates a goal.
type GoalInput struct {
	Distance     int    `json:"distance"`
	Stroke       string `json:"stroke"`
	Course       string `json:"course"`
	TargetTimeMs int    `json:"target_time_ms"`
	Deadline     string `json:"deadline"`
	Notes        string `json:"notes"`
}

// UpdateGoalInput patches a goal.
type UpdateGoalInput struct {
	TargetTimeMs *int    `json:"target_time_ms"`
	Deadline     *string `json:"deadline"` // "" clears
	Notes        *string `json:"notes"`
	Status       *string `json:"status"`
}

// requireGoalAccess allows staff for any swimmer and parent/swimmer for their
// own household's swimmers only.
func (s *Service) requireGoalAccess(principal *appmiddleware.Principal, orgID, swimmerID string) error {
	if err := s.requireReadAccess(principal); err != nil {
		return err
	}
	return s.checkParentHouseholdForSwimmer(principal, orgID, swimmerID)
}

func normalizeDeadline(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	t, err := parseDateOnly(v)
	if err != nil {
		return "", fmt.Errorf("%w: deadline must be YYYY-MM-DD", ErrGoalInvalid)
	}
	return t.Format("2006-01-02"), nil
}

func (s *Service) goalProgressLocked(g *Goal, today time.Time) GoalProgress {
	gp := GoalProgress{Goal: *g}
	if bt := s.btByKey[btKey(g.OrgID, g.SwimmerID, g.Distance, g.Stroke, g.Course)]; bt != nil && bt.BestTimeMs != nil {
		best := *bt.BestTimeMs
		gp.BestTimeMs = &best
		gp.BestRecordedAt = bt.RecordedAt
		gap := best - g.TargetTimeMs
		gp.GapMs = &gap
		gp.Achieved = best <= g.TargetTimeMs
		pct := 0.0
		switch {
		case gp.Achieved:
			pct = 100
		case g.BaselineTimeMs != nil && *g.BaselineTimeMs > g.TargetTimeMs:
			pct = float64(*g.BaselineTimeMs-best) / float64(*g.BaselineTimeMs-g.TargetTimeMs) * 100
		}
		pct = math.Max(0, math.Min(100, math.Round(pct*10)/10))
		gp.ProgressPercent = &pct
	}
	if g.Deadline != "" {
		if d, err := parseDateOnly(g.Deadline); err == nil {
			days := int(math.Floor(d.Sub(today).Hours() / 24))
			gp.DaysRemaining = &days
			gp.Overdue = days < 0 && !gp.Achieved
		}
	}
	return gp
}

func todayUTC() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// ListGoals returns a swimmer's goals with progress.
func (s *Service) ListGoals(orgID string, principal *appmiddleware.Principal, swimmerID string, includeArchived bool) ([]GoalProgress, error) {
	if err := s.requireGoalAccess(principal, orgID, swimmerID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	today := todayUTC()
	out := []GoalProgress{}
	for _, g := range s.goals {
		if g.OrgID != orgID || g.SwimmerID != swimmerID || (!includeArchived && g.Status != "active") {
			continue
		}
		out = append(out, s.goalProgressLocked(g, today))
	}
	sortGoals(out)
	return out, nil
}

// ListOrgGoals returns goals across swimmers (staff: all; families: own household).
func (s *Service) ListOrgGoals(orgID string, principal *appmiddleware.Principal, swimmerID string) ([]GoalProgress, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	scoped := principalRequiresHouseholdScope(principal) && s.requireAdminOrCoach(principal) != nil
	s.mu.RLock()
	defer s.mu.RUnlock()
	today := todayUTC()
	out := []GoalProgress{}
	for _, g := range s.goals {
		if g.OrgID != orgID || g.Status != "active" || (swimmerID != "" && g.SwimmerID != swimmerID) {
			continue
		}
		if scoped && !s.householdScopedSwimmerAllowed(principal, orgID, g.SwimmerID) {
			continue
		}
		out = append(out, s.goalProgressLocked(g, today))
	}
	sortGoals(out)
	return out, nil
}

func sortGoals(out []GoalProgress) {
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.SwimmerID != b.SwimmerID {
			return a.SwimmerID < b.SwimmerID
		}
		if a.Course != b.Course {
			return courseIndex(a.Course) < courseIndex(b.Course)
		}
		if a.Stroke != b.Stroke {
			return strokeOrder[a.Stroke] < strokeOrder[b.Stroke]
		}
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
}

func (s *Service) activeGoalCountLocked(orgID, swimmerID string) int {
	count := 0
	for _, g := range s.goals {
		if g.OrgID == orgID && g.SwimmerID == swimmerID && g.Status == "active" {
			count++
		}
	}
	return count
}

// CreateGoal adds a goal for a swimmer.
func (s *Service) CreateGoal(orgID, actorID string, principal *appmiddleware.Principal, swimmerID string, input GoalInput) (*GoalProgress, error) {
	if err := s.requireGoalAccess(principal, orgID, swimmerID); err != nil {
		return nil, err
	}
	if s.householdChecker != nil {
		if _, ok := s.householdChecker.SwimmerHouseholdID(orgID, swimmerID); !ok {
			return nil, fmt.Errorf("%w: swimmer not found", ErrGoalNotFound)
		}
	}
	stroke := strings.ToLower(strings.TrimSpace(input.Stroke))
	course := strings.ToUpper(strings.TrimSpace(input.Course))
	switch {
	case !isIndividualStroke(stroke):
		return nil, fmt.Errorf("%w: stroke must be free, back, breast, fly or im", ErrGoalInvalid)
	case course != "SCY" && course != "SCM" && course != "LCM":
		return nil, fmt.Errorf("%w: course must be SCY, SCM or LCM", ErrGoalInvalid)
	case input.Distance <= 0 || input.Distance > 1650:
		return nil, fmt.Errorf("%w: distance out of range", ErrGoalInvalid)
	case input.TargetTimeMs <= 0 || input.TargetTimeMs > 3_600_000:
		return nil, fmt.Errorf("%w: target_time_ms out of range", ErrGoalInvalid)
	case utf8.RuneCountInString(strings.TrimSpace(input.Notes)) > 500:
		return nil, fmt.Errorf("%w: notes max 500 characters", ErrGoalInvalid)
	}
	deadline, err := normalizeDeadline(input.Deadline)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeGoalCountLocked(orgID, swimmerID) >= maxGoalsPerSwimmer {
		return nil, fmt.Errorf("%w: a swimmer can have at most %d active goals", ErrGoalInvalid, maxGoalsPerSwimmer)
	}
	now := time.Now().UTC()
	g := &Goal{
		ID: s.newID(), OrgID: orgID, SwimmerID: swimmerID, Distance: input.Distance, Stroke: stroke, Course: course,
		TargetTimeMs: input.TargetTimeMs, Deadline: deadline, Notes: strings.TrimSpace(input.Notes), Status: "active",
		CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
	}
	if bt := s.btByKey[btKey(orgID, swimmerID, input.Distance, stroke, course)]; bt != nil && bt.BestTimeMs != nil {
		b := *bt.BestTimeMs
		g.BaselineTimeMs = &b
	}
	if s.repo != nil {
		if err := s.repo.UpsertGoal(context.Background(), g); err != nil {
			return nil, fmt.Errorf("persist goal: %w", err)
		}
	}
	s.goals[g.ID] = g
	s.appendAuditLocked(orgID, actorID, "goal.created", "swimmer_goal", g.ID, map[string]any{"swimmer_id": swimmerID})
	gp := s.goalProgressLocked(g, todayUTC())
	return &gp, nil
}

// UpdateGoal patches a goal.
func (s *Service) UpdateGoal(orgID, actorID string, principal *appmiddleware.Principal, swimmerID, goalID string, input UpdateGoalInput) (*GoalProgress, error) {
	if err := s.requireGoalAccess(principal, orgID, swimmerID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.goals[goalID]
	if !ok || g.OrgID != orgID || g.SwimmerID != swimmerID {
		return nil, ErrGoalNotFound
	}
	next := *g
	if input.TargetTimeMs != nil {
		if *input.TargetTimeMs <= 0 || *input.TargetTimeMs > 3_600_000 {
			return nil, fmt.Errorf("%w: target_time_ms out of range", ErrGoalInvalid)
		}
		next.TargetTimeMs = *input.TargetTimeMs
	}
	if input.Deadline != nil {
		d, err := normalizeDeadline(*input.Deadline)
		if err != nil {
			return nil, err
		}
		next.Deadline = d
	}
	if input.Notes != nil {
		if utf8.RuneCountInString(strings.TrimSpace(*input.Notes)) > 500 {
			return nil, fmt.Errorf("%w: notes max 500 characters", ErrGoalInvalid)
		}
		next.Notes = strings.TrimSpace(*input.Notes)
	}
	if input.Status != nil {
		if *input.Status != "active" && *input.Status != "archived" {
			return nil, fmt.Errorf("%w: status must be active or archived", ErrGoalInvalid)
		}
		// Un-archiving is subject to the same cap as creating.
		if *input.Status == "active" && g.Status != "active" && s.activeGoalCountLocked(orgID, swimmerID) >= maxGoalsPerSwimmer {
			return nil, fmt.Errorf("%w: a swimmer can have at most %d active goals", ErrGoalInvalid, maxGoalsPerSwimmer)
		}
		next.Status = *input.Status
	}
	next.UpdatedAt = time.Now().UTC()
	next.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.UpsertGoal(context.Background(), &next); err != nil {
			return nil, fmt.Errorf("persist goal: %w", err)
		}
	}
	s.goals[goalID] = &next
	s.appendAuditLocked(orgID, actorID, "goal.updated", "swimmer_goal", goalID, map[string]any{"swimmer_id": swimmerID})
	gp := s.goalProgressLocked(&next, todayUTC())
	return &gp, nil
}

// DeleteGoal removes a goal.
func (s *Service) DeleteGoal(orgID, actorID string, principal *appmiddleware.Principal, swimmerID, goalID string) error {
	if err := s.requireGoalAccess(principal, orgID, swimmerID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.goals[goalID]
	if !ok || g.OrgID != orgID || g.SwimmerID != swimmerID {
		return ErrGoalNotFound
	}
	if s.repo != nil {
		if err := s.repo.DeleteGoal(context.Background(), orgID, goalID); err != nil {
			return fmt.Errorf("delete goal: %w", err)
		}
	}
	delete(s.goals, goalID)
	s.appendAuditLocked(orgID, actorID, "goal.deleted", "swimmer_goal", goalID, map[string]any{"swimmer_id": swimmerID})
	return nil
}
