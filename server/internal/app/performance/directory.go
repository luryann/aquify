package performance

import (
	"strings"
	"time"
)

// SwimmerProfile is the minimal swimmer view the performance domain needs to
// compute age groups, label record holders, and filter by training group.
type SwimmerProfile struct {
	ID            string
	HouseholdID   string
	FirstName     string
	LastName      string
	PreferredName string
	DateOfBirth   string // YYYY-MM-DD
	Gender        string // members vocabulary: female|male|non_binary|undisclosed
	GroupCodes    []string
}

// DisplayName returns "Preferred Last" when a preferred name exists.
func (p SwimmerProfile) DisplayName() string {
	first := strings.TrimSpace(p.PreferredName)
	if first == "" {
		first = strings.TrimSpace(p.FirstName)
	}
	return strings.TrimSpace(first + " " + strings.TrimSpace(p.LastName))
}

// MeetInfo is the minimal meet view used to date swims and label records.
type MeetInfo struct {
	ID        string
	Name      string
	StartDate string // YYYY-MM-DD
	Course    string
}

// Directory exposes cross-domain swimmer and meet facts. It is implemented by
// an adapter over the members and meets services in server wiring; when it is
// nil, records fall back to the Open age group and result timestamps.
type Directory interface {
	SwimmerProfiles(orgID string) []SwimmerProfile
	Meets(orgID string) []MeetInfo
}

// SetDirectory injects the cross-domain directory. Must be called during
// server wiring before requests are served.
func (s *Service) SetDirectory(d Directory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.directory = d
}

type directorySnapshot struct {
	swimmers map[string]SwimmerProfile
	meets    map[string]MeetInfo
}

// snapshotDirectory must be called WITHOUT holding s.mu so cross-service locks
// are never nested inside the performance lock.
func (s *Service) snapshotDirectory(orgID string) directorySnapshot {
	s.mu.RLock()
	d := s.directory
	s.mu.RUnlock()
	snap := directorySnapshot{swimmers: map[string]SwimmerProfile{}, meets: map[string]MeetInfo{}}
	if d == nil {
		return snap
	}
	for _, p := range d.SwimmerProfiles(orgID) {
		snap.swimmers[p.ID] = p
	}
	for _, m := range d.Meets(orgID) {
		snap.meets[m.ID] = m
	}
	return snap
}

// ─── Age groups ──────────────────────────────────────────────────────────────

// AgeGroupOpen is the any-age record/top-times bucket.
const AgeGroupOpen = "OPEN"

// AgeGroups lists the record age groups in display order.
var AgeGroups = []string{"8U", "9-10", "11-12", "13-14", "15-16", "17-18", AgeGroupOpen}

// ValidAgeGroup reports whether code is a known age-group code.
func ValidAgeGroup(code string) bool {
	for _, g := range AgeGroups {
		if g == code {
			return true
		}
	}
	return false
}

// AgeGroupForAge maps an age to its age-group code, or "" for 19 and over
// (those swims count only toward Open).
func AgeGroupForAge(age int) string {
	switch {
	case age < 0:
		return ""
	case age <= 8:
		return "8U"
	case age <= 10:
		return "9-10"
	case age <= 12:
		return "11-12"
	case age <= 14:
		return "13-14"
	case age <= 16:
		return "15-16"
	case age <= 18:
		return "17-18"
	default:
		return ""
	}
}

// ageGroupContains reports whether a swim at the given age counts for group.
func ageGroupContains(group string, age *int) bool {
	if group == AgeGroupOpen {
		return true
	}
	if age == nil {
		return false
	}
	return AgeGroupForAge(*age) == group
}

// AgeOn returns the swimmer's age on the given date, or nil when either date
// is missing or malformed.
func AgeOn(dateOfBirth, onDate string) *int {
	dob, err := parseDateOnly(dateOfBirth)
	if err != nil {
		return nil
	}
	on, err := parseDateOnly(onDate)
	if err != nil {
		return nil
	}
	age := on.Year() - dob.Year()
	if on.Month() < dob.Month() || (on.Month() == dob.Month() && on.Day() < dob.Day()) {
		age--
	}
	if age < 0 {
		return nil
	}
	return &age
}

func parseDateOnly(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if len(v) > 10 {
		v = v[:10]
	}
	return time.Parse("2006-01-02", v)
}

// normalizeGender maps result (M/F/X) or members (male/female) vocabulary to
// M or F; anything else returns "".
func normalizeGender(resultGender, swimmerGender string) string {
	switch strings.ToUpper(strings.TrimSpace(resultGender)) {
	case "M":
		return "M"
	case "F":
		return "F"
	}
	switch strings.ToLower(strings.TrimSpace(swimmerGender)) {
	case "male", "m":
		return "M"
	case "female", "f":
		return "F"
	}
	return ""
}

func isIndividualStroke(stroke string) bool {
	switch stroke {
	case "free", "back", "breast", "fly", "im":
		return true
	}
	return false
}

// swimFacts are the derived facts for one result row.
type swimFacts struct {
	result      *Result
	date        string
	age         *int
	gender      string
	swimmerName string
	meetName    string
}

// qualifyingSwim reports whether a result can count toward records/top lists.
func qualifyingSwim(r *Result) bool {
	return r.Status == "active" && r.IsOfficial && !r.DQ && r.FinTimeMs != nil && *r.FinTimeMs > 0 && isIndividualStroke(r.Stroke)
}

// factsFor derives swim date (meet start date, else result creation date),
// age at swim, normalized gender, and labels for a result.
func factsFor(r *Result, dir directorySnapshot) swimFacts {
	f := swimFacts{result: r, date: r.CreatedAt.UTC().Format("2006-01-02")}
	if m, ok := dir.meets[r.MeetID]; ok && r.MeetID != "" {
		f.meetName = m.Name
		if _, err := parseDateOnly(m.StartDate); err == nil {
			f.date = strings.TrimSpace(m.StartDate)[:10]
		}
	}
	p, ok := dir.swimmers[r.SwimmerID]
	if ok {
		f.age = AgeOn(p.DateOfBirth, f.date)
		f.swimmerName = p.DisplayName()
	}
	f.gender = normalizeGender(r.Gender, p.Gender)
	if f.swimmerName == "" {
		f.swimmerName = "Unknown swimmer"
	}
	return f
}

func inDateRange(date, from, to string) bool {
	if from != "" && date < from {
		return false
	}
	if to != "" && date > to {
		return false
	}
	return true
}
