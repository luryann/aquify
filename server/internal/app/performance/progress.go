package performance

import (
	"fmt"
	"math"
	"sort"
	"strings"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// ProgressReportInput compares each swimmer's best times in a baseline period
// (A) against a comparison period (B) — two date ranges or two seasons.
type ProgressReportInput struct {
	FromA, ToA string
	FromB, ToB string
	GroupCode  string
	SwimmerID  string
	Course     string
	Limit      int // size of the most-improved list (default 10, max 50)
}

// DateRange is an inclusive YYYY-MM-DD range.
type DateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ProgressEvent is one event compared across both periods.
type ProgressEvent struct {
	Distance int     `json:"distance"`
	Stroke   string  `json:"stroke"`
	Course   string  `json:"course"`
	BeforeMs int     `json:"before_ms"`
	AfterMs  int     `json:"after_ms"`
	DropMs   int     `json:"drop_ms"` // positive = faster
	Percent  float64 `json:"percent"` // positive = faster
}

// SwimmerProgress summarizes one swimmer.
type SwimmerProgress struct {
	SwimmerID      string          `json:"swimmer_id"`
	SwimmerName    string          `json:"swimmer_name"`
	GroupCodes     []string        `json:"group_codes"`
	Events         []ProgressEvent `json:"events"`
	EventsCompared int             `json:"events_compared"`
	EventsImproved int             `json:"events_improved"`
	NewEvents      int             `json:"new_events"` // swum in B but not in A
	AvgPercent     float64         `json:"avg_percent"`
	TotalDropMs    int             `json:"total_drop_ms"`
}

// ProgressReport is the full report.
type ProgressReport struct {
	PeriodA      DateRange         `json:"period_a"`
	PeriodB      DateRange         `json:"period_b"`
	GroupCode    string            `json:"group_code,omitempty"`
	Course       string            `json:"course,omitempty"`
	Swimmers     []SwimmerProgress `json:"swimmers"`
	MostImproved []SwimmerProgress `json:"most_improved"`
}

func validRange(from, to, label string) (DateRange, error) {
	f, err1 := parseDateOnly(from)
	t, err2 := parseDateOnly(to)
	if err1 != nil || err2 != nil {
		return DateRange{}, fmt.Errorf("%w: %s range needs from and to as YYYY-MM-DD", ErrRecordInvalid, label)
	}
	if f.After(t) {
		return DateRange{}, fmt.Errorf("%w: %s range starts after it ends", ErrRecordInvalid, label)
	}
	return DateRange{From: f.Format("2006-01-02"), To: t.Format("2006-01-02")}, nil
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ProgressReport computes time drops between two periods. Staff see every
// swimmer; parent/swimmer callers see only their own household.
func (s *Service) ProgressReport(orgID string, principal *appmiddleware.Principal, in ProgressReportInput) (*ProgressReport, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	a, err := validRange(in.FromA, in.ToA, "baseline")
	if err != nil {
		return nil, err
	}
	b, err := validRange(in.FromB, in.ToB, "comparison")
	if err != nil {
		return nil, err
	}
	course := strings.ToUpper(strings.TrimSpace(in.Course))
	if course != "" && course != "SCY" && course != "SCM" && course != "LCM" {
		return nil, fmt.Errorf("%w: course must be SCY, SCM or LCM", ErrRecordInvalid)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	group := strings.ToUpper(strings.TrimSpace(in.GroupCode))
	scoped := s.requireAdminOrCoach(principal) != nil
	dir := s.snapshotDirectory(orgID)

	type evKey struct {
		swimmer  string
		distance int
		stroke   string
		course   string
	}
	bestA := map[evKey]int{}
	bestB := map[evKey]int{}
	s.mu.RLock()
	for _, r := range s.results {
		if r.OrgID != orgID || !qualifyingSwim(r) {
			continue
		}
		if course != "" && r.Course != course || in.SwimmerID != "" && r.SwimmerID != in.SwimmerID {
			continue
		}
		f := factsFor(r, dir)
		k := evKey{r.SwimmerID, r.Distance, r.Stroke, r.Course}
		t := *r.FinTimeMs
		if inDateRange(f.date, a.From, a.To) {
			if cur, ok := bestA[k]; !ok || t < cur {
				bestA[k] = t
			}
		}
		if inDateRange(f.date, b.From, b.To) {
			if cur, ok := bestB[k]; !ok || t < cur {
				bestB[k] = t
			}
		}
	}
	s.mu.RUnlock()

	bySwimmer := map[string]*SwimmerProgress{}
	get := func(id string) *SwimmerProgress {
		sp := bySwimmer[id]
		if sp == nil {
			p := dir.swimmers[id]
			name := p.DisplayName()
			if name == "" {
				name = "Unknown swimmer"
			}
			codes := append([]string{}, p.GroupCodes...)
			sp = &SwimmerProgress{SwimmerID: id, SwimmerName: name, GroupCodes: codes, Events: []ProgressEvent{}}
			bySwimmer[id] = sp
		}
		return sp
	}
	allowed := func(id string) bool {
		if group != "" && !containsFold(dir.swimmers[id].GroupCodes, group) {
			return false
		}
		if scoped && !s.householdScopedSwimmerAllowed(principal, orgID, id) {
			return false
		}
		return true
	}
	for k, after := range bestB {
		if !allowed(k.swimmer) {
			continue
		}
		sp := get(k.swimmer)
		before, ok := bestA[k]
		if !ok {
			sp.NewEvents++
			continue
		}
		drop := before - after
		ev := ProgressEvent{Distance: k.distance, Stroke: k.stroke, Course: k.course, BeforeMs: before, AfterMs: after, DropMs: drop, Percent: round2(float64(drop) / float64(before) * 100)}
		sp.Events = append(sp.Events, ev)
	}
	report := &ProgressReport{PeriodA: a, PeriodB: b, GroupCode: group, Course: course, Swimmers: []SwimmerProgress{}, MostImproved: []SwimmerProgress{}}
	for _, sp := range bySwimmer {
		sum := 0.0
		for _, ev := range sp.Events {
			sum += ev.Percent
			sp.TotalDropMs += ev.DropMs
			if ev.DropMs > 0 {
				sp.EventsImproved++
			}
		}
		sp.EventsCompared = len(sp.Events)
		if sp.EventsCompared > 0 {
			sp.AvgPercent = round2(sum / float64(sp.EventsCompared))
		}
		sort.Slice(sp.Events, func(i, j int) bool {
			x, y := sp.Events[i], sp.Events[j]
			if x.Course != y.Course {
				return courseIndex(x.Course) < courseIndex(y.Course)
			}
			if x.Stroke != y.Stroke {
				return strokeOrder[x.Stroke] < strokeOrder[y.Stroke]
			}
			return x.Distance < y.Distance
		})
		report.Swimmers = append(report.Swimmers, *sp)
	}
	sort.Slice(report.Swimmers, func(i, j int) bool {
		if report.Swimmers[i].SwimmerName != report.Swimmers[j].SwimmerName {
			return report.Swimmers[i].SwimmerName < report.Swimmers[j].SwimmerName
		}
		return report.Swimmers[i].SwimmerID < report.Swimmers[j].SwimmerID
	})
	ranked := make([]SwimmerProgress, 0, len(report.Swimmers))
	for _, sp := range report.Swimmers {
		if sp.EventsCompared > 0 && sp.AvgPercent > 0 {
			ranked = append(ranked, sp)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].AvgPercent != ranked[j].AvgPercent {
			return ranked[i].AvgPercent > ranked[j].AvgPercent
		}
		return ranked[i].EventsImproved > ranked[j].EventsImproved
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	report.MostImproved = ranked
	return report, nil
}
