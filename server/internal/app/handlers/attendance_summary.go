package handlers

import (
	"net/http"

	"github.com/dareaquatics/aquify/server/internal/app/attendance"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/roster"
	"github.com/dareaquatics/aquify/server/internal/app/wellness"
	"github.com/go-chi/chi/v5"
)

// AttendanceSummaryHandlers serves per-swimmer practice attendance rates.
// It only reads practices/attendance through attendance.PracticeSource.
type AttendanceSummaryHandlers struct {
	source    attendance.PracticeSource
	directory roster.Directory
	today     func() string
}

// NewAttendanceSummaryHandlers wires the attendance summary read endpoint.
// today returns the current date as YYYY-MM-DD.
func NewAttendanceSummaryHandlers(source attendance.PracticeSource, dir roster.Directory, today func() string) *AttendanceSummaryHandlers {
	return &AttendanceSummaryHandlers{source: source, directory: dir, today: today}
}

// RegisterRoutes mounts the route.
func (h *AttendanceSummaryHandlers) RegisterRoutes(router chi.Router) {
	router.Get("/swimmers/{swimmerID}/attendance-summary", h.summary)
}

const (
	attendanceDefaultDays = 90
	attendanceMaxDays     = 365
)

func (h *AttendanceSummaryHandlers) summary(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	sw, allowed, err := householdSwimmer(h.directory, principal, orgID, chi.URLParam(r, "swimmerID"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve swimmer.", nil)
		return
	}
	if sw.ID == "" {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Swimmer not found.", nil)
		return
	}
	if !allowed && !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "household_scope_violation", "You can only view attendance for your own household.", nil)
		return
	}
	days := clampDays(r.URL.Query().Get("days"), attendanceDefaultDays, attendanceMaxDays)
	from, to, _ := wellness.DateRange(h.today(), days)
	summary, err := attendance.Summarize(h.source, h.directory, orgID, sw, from, to)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to aggregate attendance.", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"summary": summary})
}
