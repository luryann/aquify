package handlers

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/go-chi/chi/v5"
)

const maxResultsFileBytes = 25 << 20

type unmatchedResult struct {
	Name      string `json:"name"`
	USSNumber string `json:"uss_number,omitempty"`
	Event     string `json:"event"`
	Reason    string `json:"reason"`
}

// importMeetResults handles POST /meets/{meetID}/results/import: a multipart
// SD3/CL2/HY3/ZIP results file whose individual swims are matched to club
// swimmers (USA Swimming ID first, then name + DOB, gender as tiebreaker) and
// recorded as official results, which recomputes best times. Re-importing the
// same file is a no-op for swims already recorded.
func (h *PerformanceHandlers) importMeetResults(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin or coach can import results.", nil)
		return
	}
	if h.meetsService == nil || h.membersService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Results import is not configured.", nil)
		return
	}
	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))
	meet, err := h.meetsService.GetMeet(principal, orgID, meetID)
	if err != nil {
		if errors.Is(err, meets.ErrMeetNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
		} else {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to import results for this meet.", nil)
		}
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_file", "Multipart field 'file' is required.", nil)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxResultsFileBytes+1))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_file", "Failed to read uploaded file.", nil)
		return
	}
	if len(data) > maxResultsFileBytes {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "Results file exceeds the 25 MB upload cap.", nil)
		return
	}

	parsed, warnings, err := meets.ParseResultsFile(header.Filename, data)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_results_file", err.Error(), nil)
		return
	}

	matcher := newResultSwimmerMatcher(h.membersService.SnapshotSwimmers(orgID))
	var (
		imported, duplicates, skipped int
		unmatched                     []unmatchedResult
		newRecords                    []performance.RecordBreak
	)
	for _, res := range parsed.IndividualResults {
		eventLabel := fmt.Sprintf("%d %s", res.Event.Distance, res.Event.Stroke)
		stroke := string(res.Event.Stroke)
		if res.Event.Distance <= 0 || stroke == "" || stroke == string(meetfiles.StrokeFreestyleRelay) || stroke == string(meetfiles.StrokeMedleyRelay) {
			skipped++
			continue
		}
		dq := strings.EqualFold(strings.TrimSpace(res.FinalsTime.Raw), "DQ")
		if !res.FinalsTime.Valid && !dq {
			skipped++ // NT / NS / SCR / DNF: no swim to record
			continue
		}
		swimmer, reason := matcher.match(res.Swimmer)
		if swimmer == nil {
			unmatched = append(unmatched, unmatchedResult{
				Name:      strings.TrimSpace(res.Swimmer.FirstName + " " + res.Swimmer.LastName),
				USSNumber: res.Swimmer.USSNumber,
				Event:     eventLabel,
				Reason:    reason,
			})
			continue
		}
		course := resultCourse(res.Event.Course, parsed.Course, meet.Course)
		if course == "" {
			skipped++
			continue
		}
		if h.service.HasMeetResult(orgID, swimmer.ID, meet.ID, res.Event.Distance, stroke, course) {
			duplicates++
			continue
		}
		input := performance.UpsertResultInput{
			SwimmerID:   swimmer.ID,
			MeetID:      meet.ID,
			EventNumber: strings.TrimSpace(res.Event.EventNumber),
			Distance:    res.Event.Distance,
			Stroke:      stroke,
			Course:      course,
			Gender:      resultGender(res.Event.EventGender, swimmer.Gender),
			DQ:          dq,
			IsOfficial:  true,
		}
		if res.FinalsTime.Valid {
			ms := int(math.Round(res.FinalsTime.Seconds * 1000))
			input.FinTimeMs = &ms
		}
		if res.SeedTime.Valid {
			ms := int(math.Round(res.SeedTime.Seconds * 1000))
			input.SeedTimeMs = &ms
		}
		if res.Place > 0 {
			place := res.Place
			input.Place = &place
		}
		_, breaks, err := h.service.UpsertResultWithRecords(orgID, principal.UserID, principal, input)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "result_import_failed", fmt.Sprintf("Imported %d results before a failure: %v", imported, err), nil)
			return
		}
		newRecords = append(newRecords, breaks...)
		imported++
	}

	if h.auditService != nil {
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       "results.imported",
			ResourceType: "meet",
			ResourceID:   meet.ID,
			Metadata: map[string]string{
				"file":       header.Filename,
				"imported":   fmt.Sprintf("%d", imported),
				"duplicates": fmt.Sprintf("%d", duplicates),
				"unmatched":  fmt.Sprintf("%d", len(unmatched)),
				"skipped":    fmt.Sprintf("%d", skipped),
				"records":    fmt.Sprintf("%d", len(newRecords)),
			},
		})
	}

	if unmatched == nil {
		unmatched = []unmatchedResult{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "imported",
		"imported":   imported,
		"duplicates": duplicates,
		"skipped":    skipped,
		"unmatched":  unmatched,
		"warnings":   warnings,
		// Team records broken by this import (age group and Open). Entries
		// with status "pending" await admin approval because the standing
		// record is locked.
		"new_team_records": recordBreaksOrEmpty(newRecords),
	})
}

func resultCourse(candidates ...any) string {
	for _, c := range candidates {
		var v string
		switch t := c.(type) {
		case meetfiles.Course:
			v = string(t)
		case string:
			v = t
		}
		switch v = strings.ToUpper(strings.TrimSpace(v)); v {
		case "SCY", "SCM", "LCM":
			return v
		}
	}
	return ""
}

func resultGender(eventGender, swimmerGender string) string {
	switch g := strings.ToUpper(strings.TrimSpace(eventGender)); g {
	case "M", "F", "X":
		return g
	}
	switch strings.ToLower(swimmerGender) {
	case "female":
		return "F"
	case "male":
		return "M"
	}
	return "X"
}

type resultSwimmerMatcher struct {
	byUSS  map[string][]*members.Swimmer
	byName map[string][]*members.Swimmer
}

func normalizeUSS(id string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(id) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func nameKey(first, last string) string {
	return strings.ToLower(strings.TrimSpace(first)) + "|" + strings.ToLower(strings.TrimSpace(last))
}

func newResultSwimmerMatcher(swimmers []members.Swimmer) *resultSwimmerMatcher {
	m := &resultSwimmerMatcher{byUSS: map[string][]*members.Swimmer{}, byName: map[string][]*members.Swimmer{}}
	for i := range swimmers {
		sw := &swimmers[i]
		if uss := normalizeUSS(sw.USASwimmingID); uss != "" {
			m.byUSS[uss] = append(m.byUSS[uss], sw)
		}
		m.byName[nameKey(sw.FirstName, sw.LastName)] = append(m.byName[nameKey(sw.FirstName, sw.LastName)], sw)
		if pref := strings.TrimSpace(sw.PreferredName); pref != "" && !strings.EqualFold(pref, sw.FirstName) {
			m.byName[nameKey(pref, sw.LastName)] = append(m.byName[nameKey(pref, sw.LastName)], sw)
		}
	}
	return m
}

// match returns the club swimmer for a parsed result row, or nil with a reason.
func (m *resultSwimmerMatcher) match(p meetfiles.ParsedSwimmer) (*members.Swimmer, string) {
	if uss := normalizeUSS(p.USSNumber); uss != "" {
		if hits := m.byUSS[uss]; len(hits) == 1 {
			return hits[0], ""
		}
	}
	candidates := m.byName[nameKey(p.FirstName, p.LastName)]
	if len(candidates) == 0 {
		return nil, "no club swimmer with this name or USA Swimming ID"
	}
	if p.DOB != nil {
		dob := p.DOB.Format("2006-01-02")
		var byDOB []*members.Swimmer
		for _, c := range candidates {
			if strings.HasPrefix(c.DateOfBirth, dob) {
				byDOB = append(byDOB, c)
			}
		}
		if len(byDOB) == 0 {
			return nil, "name matches but date of birth does not"
		}
		candidates = byDOB
	}
	if len(candidates) > 1 && p.Gender != "" {
		var byGender []*members.Swimmer
		for _, c := range candidates {
			if resultGender("", c.Gender) == strings.ToUpper(p.Gender) {
				byGender = append(byGender, c)
			}
		}
		candidates = byGender
	}
	if len(candidates) != 1 {
		return nil, "multiple club swimmers match; add their USA Swimming ID"
	}
	return candidates[0], ""
}
