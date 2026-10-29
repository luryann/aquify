package handlers

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
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
	principal, orgID, ok := scopedRequestFromContext(w, r)
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

	filename, data, ok := readResultsUpload(w, r)
	if !ok {
		return
	}

	parsed, warnings, err := meets.ParseResultsFile(filename, data)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_results_file", err.Error(), nil)
		return
	}

	matcher := newResultSwimmerMatcher(h.membersService.SnapshotSwimmers(orgID))
	progress, importErr := importParsedResults(h.service, orgID, principal, meet.ID, meet.Course, parsed, matcher)
	imported, duplicates, skipped := progress.Imported, progress.Duplicates, progress.Skipped
	unmatched, newRecords := progress.Unmatched, progress.NewRecords

	if importErr != nil {
		// Idempotency is per swim: everything recorded so far stays recorded and
		// a re-import skips it. Report that progress instead of a bare error.
		h.auditResultsImport(principal, orgID, meet.ID, filename, "results.import_partial", progress)
		httpx.WriteError(w, r, http.StatusInternalServerError, "result_import_failed",
			fmt.Sprintf("The import stopped after recording %d result(s); re-importing the same file is safe and resumes where it stopped (already-recorded swims are skipped).", progress.Imported),
			importFailureDetails{
				Partial: true, Imported: progress.Imported, Duplicates: progress.Duplicates, Skipped: progress.Skipped,
				Unmatched: len(progress.Unmatched), RecordDetectionFailures: progress.RecordDetectionFailures,
				Processed: progress.Processed, Total: progress.Total, NewTeamRecords: recordBreaksOrEmpty(progress.NewRecords), RetrySafe: true,
			})
		slog.Error("results import stopped mid-file", "meet_id", meet.ID, "imported", progress.Imported, "processed", progress.Processed, "total", progress.Total, "error", importErr)
		return
	}

	h.auditResultsImport(principal, orgID, meet.ID, filename, "results.imported", progress)

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
		// Swims whose result was saved but whose team-record detection failed;
		// an admin can run a records rebuild to repair the affected slots.
		"record_detection_failures": progress.RecordDetectionFailures,
	})
}

// importFailureDetails is the body of a mid-file failure: how far the import
// got, so the caller knows what is already recorded.
type importFailureDetails struct {
	Partial                 bool                      `json:"partial"`
	Imported                int                       `json:"imported"`
	Duplicates              int                       `json:"duplicates"`
	Skipped                 int                       `json:"skipped"`
	Unmatched               int                       `json:"unmatched"`
	RecordDetectionFailures int                       `json:"record_detection_failures"`
	Processed               int                       `json:"rows_processed"`
	Total                   int                       `json:"rows_total"`
	NewTeamRecords          []performance.RecordBreak `json:"new_team_records"`
	RetrySafe               bool                      `json:"retry_safe"`
}

// resultsRecorder is the slice of the performance service the import needs.
type resultsRecorder interface {
	HasMeetResult(orgID, swimmerID, meetID string, distance int, stroke, course string) bool
	UpsertResultWithRecords(orgID, actorID string, principal *appmiddleware.Principal, input performance.UpsertResultInput) (*performance.Result, []performance.RecordBreak, error)
}

type resultsImportProgress struct {
	Imported, Duplicates, Skipped, RecordDetectionFailures int
	Processed, Total                                       int
	Unmatched                                              []unmatchedResult
	NewRecords                                             []performance.RecordBreak
}

// importParsedResults records each matched swim in file order. It is safe to
// re-run after a failure: swims already recorded for the swimmer/event/meet are
// skipped, so a retry resumes where the previous run stopped. On error the
// progress made so far is returned alongside it.
func importParsedResults(rec resultsRecorder, orgID string, principal *appmiddleware.Principal, meetID, meetCourse string, parsed *meetfiles.ParseResult, matcher *resultSwimmerMatcher) (resultsImportProgress, error) {
	prog := resultsImportProgress{Total: len(parsed.IndividualResults)}
	for _, res := range parsed.IndividualResults {
		eventLabel := fmt.Sprintf("%d %s", res.Event.Distance, res.Event.Stroke)
		stroke := string(res.Event.Stroke)
		if res.Event.Distance <= 0 || stroke == "" || stroke == string(meetfiles.StrokeFreestyleRelay) || stroke == string(meetfiles.StrokeMedleyRelay) {
			prog.Skipped++
			prog.Processed++
			continue
		}
		dq := strings.EqualFold(strings.TrimSpace(res.FinalsTime.Raw), "DQ")
		if !res.FinalsTime.Valid && !dq {
			prog.Skipped++ // NT / NS / SCR / DNF: no swim to record
			prog.Processed++
			continue
		}
		swimmer, reason := matcher.match(res.Swimmer)
		if swimmer == nil {
			prog.Unmatched = append(prog.Unmatched, unmatchedResult{
				Name:      strings.TrimSpace(res.Swimmer.FirstName + " " + res.Swimmer.LastName),
				USSNumber: res.Swimmer.USSNumber,
				Event:     eventLabel,
				Reason:    reason,
			})
			prog.Processed++
			continue
		}
		course := resultCourse(res.Event.Course, parsed.Course, meetCourse)
		if course == "" {
			prog.Skipped++
			prog.Processed++
			continue
		}
		if rec.HasMeetResult(orgID, swimmer.ID, meetID, res.Event.Distance, stroke, course) {
			prog.Duplicates++
			prog.Processed++
			continue
		}
		input := resultUpsertInput(res, swimmer, meetID, course, stroke, dq)
		_, breaks, err := rec.UpsertResultWithRecords(orgID, principal.UserID, principal, input)
		if err != nil {
			if errors.Is(err, performance.ErrRecordDetection) {
				// The result itself is saved (a retry would skip it), so count it and
				// keep going; the failure is reported for a records rebuild.
				prog.NewRecords = append(prog.NewRecords, breaks...)
				prog.Imported++
				prog.RecordDetectionFailures++
				prog.Processed++
				continue
			}
			return prog, err
		}
		prog.NewRecords = append(prog.NewRecords, breaks...)
		prog.Imported++
		prog.Processed++
	}
	return prog, nil
}

func (h *PerformanceHandlers) auditResultsImport(principal *appmiddleware.Principal, orgID, meetID, filename, action string, p resultsImportProgress) {
	if h.auditService == nil {
		return
	}
	_ = appendAudit(h.auditService, audit.AppendRequest{
		OrgID:        orgID,
		ActorUserID:  principal.UserID,
		ActorRoles:   principal.Roles,
		Action:       action,
		ResourceType: "meet",
		ResourceID:   meetID,
		Metadata: map[string]string{
			"file":                      filename,
			"imported":                  fmt.Sprintf("%d", p.Imported),
			"duplicates":                fmt.Sprintf("%d", p.Duplicates),
			"unmatched":                 fmt.Sprintf("%d", len(p.Unmatched)),
			"skipped":                   fmt.Sprintf("%d", p.Skipped),
			"records":                   fmt.Sprintf("%d", len(p.NewRecords)),
			"record_detection_failures": fmt.Sprintf("%d", p.RecordDetectionFailures),
			"rows_processed":            fmt.Sprintf("%d", p.Processed),
			"rows_total":                fmt.Sprintf("%d", p.Total),
		},
	})
}

// readResultsUpload reads the multipart "file" field (25 MB cap), writing the
// error response itself when it fails.
func readResultsUpload(w http.ResponseWriter, r *http.Request) (string, []byte, bool) {
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_file", "Multipart field 'file' is required.", nil)
		return "", nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxResultsFileBytes+1))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_file", "Failed to read uploaded file.", nil)
		return "", nil, false
	}
	if len(data) > maxResultsFileBytes {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "Results file exceeds the 25 MB upload cap.", nil)
		return "", nil, false
	}
	return header.Filename, data, true
}

func secondsToMs(seconds float64) *int {
	ms := int(math.Round(seconds * 1000))
	return &ms
}

func resultUpsertInput(res meetfiles.ParsedIndividualResult, swimmer *members.Swimmer, meetID, course, stroke string, dq bool) performance.UpsertResultInput {
	input := performance.UpsertResultInput{
		SwimmerID:   swimmer.ID,
		MeetID:      meetID,
		EventNumber: strings.TrimSpace(res.Event.EventNumber),
		Distance:    res.Event.Distance,
		Stroke:      stroke,
		Course:      course,
		Gender:      resultGender(res.Event.EventGender, swimmer.Gender),
		DQ:          dq,
		IsOfficial:  true,
	}
	if res.FinalsTime.Valid {
		input.FinTimeMs = secondsToMs(res.FinalsTime.Seconds)
	}
	if res.SeedTime.Valid {
		input.SeedTimeMs = secondsToMs(res.SeedTime.Seconds)
	}
	if res.Place > 0 {
		place := res.Place
		input.Place = &place
	}
	return input
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
