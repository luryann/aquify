// Package wellness stores daily athlete readiness check-ins (sleep, soreness,
// energy, stress, mood) and derives readiness scores, trend summaries and
// coach-facing flags.
package wellness

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrInvalid wraps every validation failure.
	ErrInvalid = errors.New("invalid wellness check-in")
	// ErrNotFound is returned when a check-in does not exist.
	ErrNotFound = errors.New("wellness check-in not found")
)

const (
	// MaxNoteLen bounds the free-text note.
	MaxNoteLen = 500
	// BackfillDays is how far back a check-in may be submitted.
	BackfillDays = 7
	dateLayout   = "2006-01-02"
)

// Metrics are the self-reported values. Scales are 1–5 where 5 means "more"
// (more sore, more stressed, more energy, better mood, better sleep).
type Metrics struct {
	SleepHours   float64 `json:"sleep_hours"`
	SleepQuality int     `json:"sleep_quality"`
	Soreness     int     `json:"soreness"`
	Energy       int     `json:"energy"`
	Stress       int     `json:"stress"`
	Mood         int     `json:"mood"`
}

// CheckIn is one swimmer's check-in for one calendar day.
type CheckIn struct {
	ID            string    `json:"id"`
	OrgID         string    `json:"org_id"`
	SwimmerID     string    `json:"swimmer_id"`
	HouseholdID   string    `json:"household_id"`
	Date          string    `json:"date"`
	Metrics                 // embedded; serialised inline
	Note          string    `json:"note,omitempty"`
	Readiness     float64   `json:"readiness"`
	SubmittedBy   string    `json:"submitted_by"`
	SubmittedRole string    `json:"submitted_role"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Readiness converts metrics into a 1–5 composite where higher is better:
// the mean of sleep quality, energy, mood, and the inverted soreness and
// stress scales, rounded to one decimal.
func Readiness(m Metrics) float64 {
	sum := float64(m.SleepQuality + m.Energy + m.Mood + (6 - m.Soreness) + (6 - m.Stress))
	return math.Round(sum/5*10) / 10
}

// Input is a check-in submission.
type Input struct {
	SwimmerID     string
	HouseholdID   string
	Date          string // YYYY-MM-DD; defaults to today
	Metrics       Metrics
	Note          string
	SubmittedBy   string
	SubmittedRole string
}

// ValidationError names the failing field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Unwrap lets errors.Is match ErrInvalid.
func (e *ValidationError) Unwrap() error { return ErrInvalid }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

func validScale(v int) bool { return v >= 1 && v <= 5 }

// Service stores check-ins in memory or Postgres (canonical when set).
type Service struct {
	mu       sync.Mutex
	repo     *Repository
	now      func() time.Time
	checkins map[string]*CheckIn // key: swimmerID|date
}

// NewService returns an in-memory Service.
func NewService() *Service {
	return &Service{now: func() time.Time { return time.Now().UTC() }, checkins: map[string]*CheckIn{}}
}

// NewServiceWithPool returns a DB-backed Service (in-memory when nil).
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool != nil {
		s.repo = newRepository(pool)
	}
	return s
}

// SetClock overrides the time source (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Today returns the service's current date (UTC) as YYYY-MM-DD.
func (s *Service) Today() string { return s.now().Format(dateLayout) }

func key(swimmerID, date string) string { return swimmerID + "|" + date }

// Upsert stores the day's check-in, replacing an earlier submission for the
// same swimmer and date. created reports whether a new row was inserted.
func (s *Service) Upsert(orgID string, in Input) (*CheckIn, bool, error) {
	in.SwimmerID = strings.TrimSpace(in.SwimmerID)
	in.Note = strings.TrimSpace(in.Note)
	in.Date = strings.TrimSpace(in.Date)
	today := s.now().Truncate(24 * time.Hour)
	if in.Date == "" {
		in.Date = today.Format(dateLayout)
	}
	day, err := time.Parse(dateLayout, in.Date)
	switch {
	case in.SwimmerID == "":
		return nil, false, invalid("swimmer_id", "is required")
	case err != nil:
		return nil, false, invalid("date", "must be YYYY-MM-DD")
	case day.After(today.AddDate(0, 0, 1)):
		return nil, false, invalid("date", "cannot be in the future")
	case day.Before(today.AddDate(0, 0, -BackfillDays)):
		return nil, false, invalid("date", fmt.Sprintf("cannot be more than %d days ago", BackfillDays))
	case math.IsNaN(in.Metrics.SleepHours) || in.Metrics.SleepHours < 0 || in.Metrics.SleepHours > 24:
		return nil, false, invalid("sleep_hours", "must be between 0 and 24")
	case !validScale(in.Metrics.SleepQuality):
		return nil, false, invalid("sleep_quality", "must be 1–5")
	case !validScale(in.Metrics.Soreness):
		return nil, false, invalid("soreness", "must be 1–5")
	case !validScale(in.Metrics.Energy):
		return nil, false, invalid("energy", "must be 1–5")
	case !validScale(in.Metrics.Stress):
		return nil, false, invalid("stress", "must be 1–5")
	case !validScale(in.Metrics.Mood):
		return nil, false, invalid("mood", "must be 1–5")
	case len([]rune(in.Note)) > MaxNoteLen:
		return nil, false, invalid("note", fmt.Sprintf("must be at most %d characters", MaxNoteLen))
	}
	in.Metrics.SleepHours = math.Round(in.Metrics.SleepHours*10) / 10

	now := s.now()
	c := &CheckIn{
		ID:            uuid.New().String(),
		OrgID:         orgID,
		SwimmerID:     in.SwimmerID,
		HouseholdID:   strings.TrimSpace(in.HouseholdID),
		Date:          in.Date,
		Metrics:       in.Metrics,
		Note:          in.Note,
		Readiness:     Readiness(in.Metrics),
		SubmittedBy:   in.SubmittedBy,
		SubmittedRole: in.SubmittedRole,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if s.repo != nil {
		saved, created, err := s.repo.Upsert(context.Background(), c)
		if err != nil {
			return nil, false, err
		}
		saved.Readiness = Readiness(saved.Metrics)
		return saved, created, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(c.SwimmerID, c.Date)
	created := true
	if prev, ok := s.checkins[k]; ok && prev.OrgID == orgID {
		c.ID = prev.ID
		c.CreatedAt = prev.CreatedAt
		created = false
	}
	cp := *c
	s.checkins[k] = &cp
	return c, created, nil
}

// List returns check-ins in [from, to] (inclusive dates) for the given
// swimmers (all swimmers when swimmerIDs is nil), ordered by swimmer, date.
func (s *Service) List(orgID string, swimmerIDs []string, from, to string) ([]*CheckIn, error) {
	var out []*CheckIn
	if s.repo != nil {
		list, err := s.repo.List(context.Background(), orgID, swimmerIDs, from, to)
		if err != nil {
			return nil, err
		}
		out = list
	} else {
		var want map[string]bool
		if swimmerIDs != nil {
			want = map[string]bool{}
			for _, id := range swimmerIDs {
				want[id] = true
			}
		}
		s.mu.Lock()
		for _, c := range s.checkins {
			if c.OrgID != orgID || c.Date < from || c.Date > to {
				continue
			}
			if want != nil && !want[c.SwimmerID] {
				continue
			}
			cp := *c
			out = append(out, &cp)
		}
		s.mu.Unlock()
	}
	for _, c := range out {
		c.Readiness = Readiness(c.Metrics)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SwimmerID != out[j].SwimmerID {
			return out[i].SwimmerID < out[j].SwimmerID
		}
		return out[i].Date < out[j].Date
	})
	if out == nil {
		out = []*CheckIn{}
	}
	return out, nil
}

// DateRange returns the inclusive [from, to] window of `days` days ending
// on `end` (YYYY-MM-DD), plus every date in it.
func DateRange(end string, days int) (string, string, []string) {
	t, err := time.Parse(dateLayout, end)
	if err != nil {
		t = time.Now().UTC()
	}
	if days < 1 {
		days = 1
	}
	dates := make([]string, 0, days)
	for i := days - 1; i >= 0; i-- {
		dates = append(dates, t.AddDate(0, 0, -i).Format(dateLayout))
	}
	return dates[0], dates[len(dates)-1], dates
}

// ─── Flags ────────────────────────────────────────────────────────────────────

// Flag is a coach-facing alert derived from a swimmer's recent check-ins.
type Flag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // warning | alert
	Message  string `json:"message"`
	Since    string `json:"since"`
}

const (
	// LowReadinessThreshold marks a readiness score as low.
	LowReadinessThreshold = 2.5
	// ShortSleepHours marks a night as short.
	ShortSleepHours = 6.0
	// StreakDays is how many consecutive days trigger a streak flag.
	StreakDays = 3
)

// trailingStreak returns the length and start date of the run of
// consecutive-day check-ins (ending at the latest one) matching pred.
func trailingStreak(sorted []*CheckIn, pred func(*CheckIn) bool) (int, string) {
	n := 0
	since := ""
	var prev time.Time
	for i := len(sorted) - 1; i >= 0; i-- {
		c := sorted[i]
		d, err := time.Parse(dateLayout, c.Date)
		if err != nil || !pred(c) {
			break
		}
		if n > 0 && prev.Sub(d) != 24*time.Hour {
			break
		}
		n++
		since = c.Date
		prev = d
	}
	return n, since
}

// Flags evaluates one swimmer's check-ins (any order) as of `asOf`.
// Streak flags only fire while the streak is current (latest check-in is
// today or yesterday), so stale history does not keep a swimmer flagged.
func Flags(checkins []*CheckIn, asOf string) []Flag {
	flags := []Flag{}
	if len(checkins) == 0 {
		return flags
	}
	sorted := append([]*CheckIn{}, checkins...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })
	latest := sorted[len(sorted)-1]
	asOfT, err := time.Parse(dateLayout, asOf)
	latestT, err2 := time.Parse(dateLayout, latest.Date)
	if err != nil || err2 != nil || asOfT.Sub(latestT) > 24*time.Hour {
		return flags
	}
	readiness := func(c *CheckIn) float64 { return Readiness(c.Metrics) }

	if n, since := trailingStreak(sorted, func(c *CheckIn) bool { return readiness(c) <= LowReadinessThreshold }); n >= StreakDays {
		flags = append(flags, Flag{Code: "low_readiness_streak", Severity: "alert", Since: since,
			Message: fmt.Sprintf("Readiness at or below %.1f for %d days running", LowReadinessThreshold, n)})
	}
	if n, since := trailingStreak(sorted, func(c *CheckIn) bool { return c.SleepHours < ShortSleepHours }); n >= StreakDays {
		flags = append(flags, Flag{Code: "short_sleep_streak", Severity: "warning", Since: since,
			Message: fmt.Sprintf("Under %.0f hours of sleep for %d nights running", ShortSleepHours, n)})
	}
	if n, since := trailingStreak(sorted, func(c *CheckIn) bool { return c.Soreness >= 4 }); n >= StreakDays {
		flags = append(flags, Flag{Code: "soreness_streak", Severity: "warning", Since: since,
			Message: fmt.Sprintf("High soreness reported %d days running", n)})
	}
	if len(sorted) >= 5 {
		var sum float64
		prior := sorted[:len(sorted)-1]
		for _, c := range prior {
			sum += readiness(c)
		}
		baseline := sum / float64(len(prior))
		if drop := baseline - readiness(latest); drop >= 1.0 {
			flags = append(flags, Flag{Code: "sharp_drop", Severity: "warning", Since: latest.Date,
				Message: fmt.Sprintf("Readiness %.1f is %.1f below their recent average", readiness(latest), drop)})
		}
	}
	if latest.Mood <= 1 || latest.Stress >= 5 {
		flags = append(flags, Flag{Code: "check_in_with_athlete", Severity: "alert", Since: latest.Date,
			Message: "Very low mood or very high stress reported — consider a check-in conversation"})
	}
	return flags
}

// Averages summarises a set of check-ins.
type Averages struct {
	Count        int     `json:"count"`
	Readiness    float64 `json:"readiness"`
	SleepHours   float64 `json:"sleep_hours"`
	SleepQuality float64 `json:"sleep_quality"`
	Soreness     float64 `json:"soreness"`
	Energy       float64 `json:"energy"`
	Stress       float64 `json:"stress"`
	Mood         float64 `json:"mood"`
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// Average computes mean metrics (zero-valued when empty).
func Average(checkins []*CheckIn) Averages {
	a := Averages{Count: len(checkins)}
	if a.Count == 0 {
		return a
	}
	for _, c := range checkins {
		a.Readiness += Readiness(c.Metrics)
		a.SleepHours += c.SleepHours
		a.SleepQuality += float64(c.SleepQuality)
		a.Soreness += float64(c.Soreness)
		a.Energy += float64(c.Energy)
		a.Stress += float64(c.Stress)
		a.Mood += float64(c.Mood)
	}
	n := float64(a.Count)
	a.Readiness = round1(a.Readiness / n)
	a.SleepHours = round1(a.SleepHours / n)
	a.SleepQuality = round1(a.SleepQuality / n)
	a.Soreness = round1(a.Soreness / n)
	a.Energy = round1(a.Energy / n)
	a.Stress = round1(a.Stress / n)
	a.Mood = round1(a.Mood / n)
	return a
}
