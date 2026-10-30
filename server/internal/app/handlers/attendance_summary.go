package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

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
	sw, ok := readableSwimmer(w, r, h.directory, principal, orgID, "attendance")
	if !ok {
		return
	}
	days := clampDays(r.URL.Query().Get("days"), attendanceDefaultDays, attendanceMaxDays)
	from, to, _ := wellness.DateRange(h.today(), days)
	// Optional explicit window; each bound defaults to the days-based range.
	q := r.URL.Query()
	if v := strings.TrimSpace(q.Get("from")); v != "" {
		from = v
	}
	if v := strings.TrimSpace(q.Get("to")); v != "" {
		to = v
	}
	if ft, e1 := time.Parse("2006-01-02", from); e1 == nil {
		if tt, e2 := time.Parse("2006-01-02", to); e2 == nil && tt.Sub(ft) > attendanceMaxDays*24*time.Hour {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_range", "The window may span at most 365 days.", nil)
			return
		}
	}
	summary, err := attendance.Summarize(h.source, h.directory, orgID, sw, from, to)
	if errors.Is(err, attendance.ErrInvalidRange) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_range", "from and to must be YYYY-MM-DD dates with from on or before to.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to aggregate attendance.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"summary": summary})
}
