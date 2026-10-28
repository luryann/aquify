package performance

import (
	"fmt"
	"sort"
	"strings"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// TopTimesQuery selects the swims ranked on a top-N list. Age group is
// evaluated per swim (age on the swim date), so an 11-12 list holds swims done
// while the swimmer was 11 or 12. Each swimmer appears once, with their
// fastest qualifying swim in the window.
type TopTimesQuery struct {
	Course   string
	Gender   string // M | F; "" = one list per gender
	AgeGroup string // default OPEN
	Distance int    // 0 = every event with swims
	Stroke   string // "" = every stroke
	From     string // inclusive YYYY-MM-DD
	To       string // inclusive YYYY-MM-DD
	Limit    int    // default 10, max 100
}

// TopTimeEntry is one ranked swim.
type TopTimeEntry struct {
	Rank        int    `json:"rank"`
	SwimmerID   string `json:"swimmer_id"`
	SwimmerName string `json:"swimmer_name"`
	Age         *int   `json:"age,omitempty"`
	TimeMs      int    `json:"time_ms"`
	SwimDate    string `json:"swim_date"`
	MeetID      string `json:"meet_id,omitempty"`
	MeetName    string `json:"meet_name,omitempty"`
	ResultID    string `json:"result_id"`
}

// TopTimesList is the ranked list for one event.
type TopTimesList struct {
	RecordKey
	Label   string         `json:"label"`
	Entries []TopTimeEntry `json:"entries"`
}

func (q *TopTimesQuery) normalize() error {
	q.Course = strings.ToUpper(strings.TrimSpace(q.Course))
	if q.Course == "" {
		q.Course = "SCY"
	}
	if q.Course != "SCY" && q.Course != "SCM" && q.Course != "LCM" {
		return fmt.Errorf("%w: course must be SCY, SCM or LCM", ErrRecordInvalid)
	}
	q.Gender = strings.ToUpper(strings.TrimSpace(q.Gender))
	if q.Gender != "" && q.Gender != "M" && q.Gender != "F" {
		return fmt.Errorf("%w: gender must be M or F", ErrRecordInvalid)
	}
	q.AgeGroup = strings.ToUpper(strings.TrimSpace(q.AgeGroup))
	if q.AgeGroup == "" {
		q.AgeGroup = AgeGroupOpen
	}
	if !ValidAgeGroup(q.AgeGroup) {
		return fmt.Errorf("%w: unknown age group", ErrRecordInvalid)
	}
	q.Stroke = strings.ToLower(strings.TrimSpace(q.Stroke))
	if q.Stroke != "" && !isIndividualStroke(q.Stroke) {
		return fmt.Errorf("%w: stroke must be free, back, breast, fly or im", ErrRecordInvalid)
	}
	for _, d := range []string{q.From, q.To} {
		if d != "" {
			if _, err := parseDateOnly(d); err != nil {
				return fmt.Errorf("%w: from/to must be YYYY-MM-DD", ErrRecordInvalid)
			}
		}
	}
	if q.From != "" && q.To != "" && q.From > q.To {
		return fmt.Errorf("%w: from must be on or before to", ErrRecordInvalid)
	}
	if q.Limit <= 0 {
		q.Limit = 10
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	return nil
}

// TopTimes returns top-N lists. Readable by every club role (club top-10
// boards are shared with families, like the record board).
func (s *Service) TopTimes(orgID string, principal *appmiddleware.Principal, q TopTimesQuery) ([]TopTimesList, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	if err := q.normalize(); err != nil {
		return nil, err
	}
	dir := s.snapshotDirectory(orgID)

	s.mu.RLock()
	perKey := map[RecordKey]map[string]swimFacts{}
	for _, r := range s.results {
		if r.OrgID != orgID || r.Course != q.Course || !qualifyingSwim(r) {
			continue
		}
		if q.Distance > 0 && r.Distance != q.Distance || q.Stroke != "" && r.Stroke != q.Stroke {
			continue
		}
		f := factsFor(r, dir)
		if f.gender == "" || q.Gender != "" && f.gender != q.Gender {
			continue
		}
		if !inDateRange(f.date, q.From, q.To) || !ageGroupContains(q.AgeGroup, f.age) {
			continue
		}
		k := RecordKey{Course: q.Course, Gender: f.gender, AgeGroup: q.AgeGroup, Distance: r.Distance, Stroke: r.Stroke}
		m := perKey[k]
		if m == nil {
			m = map[string]swimFacts{}
			perKey[k] = m
		}
		prev, ok := m[r.SwimmerID]
		if !ok || fasterSwim(f, prev) {
			m[r.SwimmerID] = f
		}
	}
	s.mu.RUnlock()

	out := make([]TopTimesList, 0, len(perKey))
	for k, bySwimmer := range perKey {
		swims := make([]swimFacts, 0, len(bySwimmer))
		for _, f := range bySwimmer {
			swims = append(swims, f)
		}
		sort.Slice(swims, func(i, j int) bool { return fasterSwim(swims[i], swims[j]) })
		list := TopTimesList{RecordKey: k, Label: k.Label(), Entries: []TopTimeEntry{}}
		for i, f := range swims {
			if i >= q.Limit {
				break
			}
			rank := i + 1
			if i > 0 && *f.result.FinTimeMs == list.Entries[i-1].TimeMs {
				rank = list.Entries[i-1].Rank // ties share a rank
			}
			list.Entries = append(list.Entries, TopTimeEntry{
				Rank: rank, SwimmerID: f.result.SwimmerID, SwimmerName: f.swimmerName, Age: f.age,
				TimeMs: *f.result.FinTimeMs, SwimDate: f.date, MeetID: f.result.MeetID, MeetName: f.meetName, ResultID: f.result.ID,
			})
		}
		out = append(out, list)
	}
	sort.Slice(out, func(i, j int) bool { return lessRecordKey(out[i].RecordKey, out[j].RecordKey) })
	return out, nil
}

func fasterSwim(a, b swimFacts) bool {
	at, bt := *a.result.FinTimeMs, *b.result.FinTimeMs
	if at != bt {
		return at < bt
	}
	if a.date != b.date {
		return a.date < b.date
	}
	return a.result.ID < b.result.ID
}
