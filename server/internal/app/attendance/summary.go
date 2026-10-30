// Package attendance aggregates practice attendance per swimmer using the
// training service's exported read methods (it never writes attendance).
package attendance

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/roster"
)

// Practice is the practice projection this package needs.
type Practice struct {
	ID      string
	Date    string // YYYY-MM-DD
	GroupID string // group ID or code; empty = all groups
	Type    string
}

// Record is one attendance mark.
type Record struct {
	SwimmerID string
	Status    string
}

// PracticeSource reads practices and attendance. Production wiring adapts
// the training service's exported List/ListAttendance methods.
type PracticeSource interface {
	Practices(orgID string) ([]Practice, error)
	Attendance(orgID, practiceID string) ([]Record, error)
}

// Mark is one practice outcome for the swimmer.
type Mark struct {
	PracticeID string `json:"practice_id"`
	Date       string `json:"date"`
	Type       string `json:"type"`
	Status     string `json:"status"` // present | absent | excused | unmarked
}

// Summary is a swimmer's attendance over a date window.
type Summary struct {
	SwimmerID         string   `json:"swimmer_id"`
	From              string   `json:"from"`
	To                string   `json:"to"`
	EligiblePractices int      `json:"eligible_practices"`
	Present           int      `json:"present"`
	Absent            int      `json:"absent"`
	Excused           int      `json:"excused"`
	Unmarked          int      `json:"unmarked"`
	AttendancePct     *float64 `json:"attendance_pct"`
	Recent            []Mark   `json:"recent"`
}

// RecentLimit caps the recent-marks list.
const RecentLimit = 12

// Summarize computes attendance for one swimmer over [from, to] (inclusive
// YYYY-MM-DD). Eligible practices are non-rest practices for one of the
// swimmer's groups, or for all groups (no group set). The percentage is
// present / (present + absent); excused and unmarked practices are excluded
// from the denominator so missing roll calls do not penalise the swimmer.
// ErrInvalidRange is returned when from/to are not a valid YYYY-MM-DD window.
var ErrInvalidRange = errors.New("attendance: from and to must be YYYY-MM-DD dates with from <= to")

// Summarize aggregates attendance. from and to are inclusive YYYY-MM-DD dates.
func Summarize(src PracticeSource, dir roster.Directory, orgID string, swimmer roster.Swimmer, from, to string) (Summary, error) {
	out := Summary{SwimmerID: swimmer.ID, From: from, To: to, Recent: []Mark{}}
	f, ferr := time.Parse("2006-01-02", from)
	t, terr := time.Parse("2006-01-02", to)
	if ferr != nil || terr != nil || f.After(t) {
		return out, ErrInvalidRange
	}
	practices, err := src.Practices(orgID)
	if err != nil {
		return out, err
	}
	codeCache := map[string]string{}
	resolve := func(ref string) string {
		if code, ok := codeCache[ref]; ok {
			return code
		}
		code := dir.ResolveGroupCode(orgID, ref)
		codeCache[ref] = code
		return code
	}
	marks := []Mark{}
	for _, p := range practices {
		if p.Date < from || p.Date > to || strings.EqualFold(p.Type, "rest") {
			continue
		}
		if ref := strings.TrimSpace(p.GroupID); ref != "" && !swimmer.InGroup(resolve(ref)) {
			continue
		}
		records, err := src.Attendance(orgID, p.ID)
		if err != nil {
			return out, err
		}
		status := "unmarked"
		for _, rec := range records {
			if rec.SwimmerID == swimmer.ID {
				status = rec.Status
				break
			}
		}
		out.EligiblePractices++
		switch status {
		case "present":
			out.Present++
		case "absent":
			out.Absent++
		case "excused":
			out.Excused++
		default:
			out.Unmarked++
		}
		marks = append(marks, Mark{PracticeID: p.ID, Date: p.Date, Type: p.Type, Status: status})
	}
	if denom := out.Present + out.Absent; denom > 0 {
		pct := math.Round(float64(out.Present)/float64(denom)*1000) / 10
		out.AttendancePct = &pct
	}
	sort.SliceStable(marks, func(i, j int) bool { return marks[i].Date > marks[j].Date })
	if len(marks) > RecentLimit {
		marks = marks[:RecentLimit]
	}
	out.Recent = marks
	return out, nil
}
