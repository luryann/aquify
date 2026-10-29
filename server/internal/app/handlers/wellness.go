package handlers

import (
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/roster"
	"github.com/dareaquatics/aquify/server/internal/app/wellness"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// WellnessHandlers serves daily readiness check-ins and coach views.
//
// Privacy: check-ins (including notes) are readable only by admin/coach and
// by the swimmer's own household (parent or the swimmer); every other
// principal is denied, including parents of other households.
type WellnessHandlers struct {
	service   *wellness.Service
	directory roster.Directory
	validate  *validator.Validate
}

// NewWellnessHandlers wires the wellness handlers.
func NewWellnessHandlers(svc *wellness.Service, dir roster.Directory) *WellnessHandlers {
	return &WellnessHandlers{service: svc, directory: dir, validate: httpx.NewValidator()}
}

// RegisterRoutes mounts the wellness routes.
func (h *WellnessHandlers) RegisterRoutes(router chi.Router) {
	router.Put("/wellness/checkins", h.upsertCheckIn)
	router.Get("/wellness/swimmers/{swimmerID}", h.swimmerTrend)
	router.Get("/wellness/heatmap", h.heatmap)
}

const (
	wellnessDefaultTrendDays   = 30
	wellnessMaxTrendDays       = 90
	wellnessDefaultHeatmapDays = 14
	wellnessMaxHeatmapDays     = 28
)

func clampDays(raw string, def, max int) int {
	d := parsePositiveInt(raw, def)
	if d > max {
		return max
	}
	return d
}

// readableSwimmer resolves the {swimmerID} route swimmer for a read: staff may
// read anyone, family members their own household. It writes the 404/403/500
// itself; what is kept for call-site readability.
func readableSwimmer(w http.ResponseWriter, r *http.Request, dir roster.Directory, principal *appmiddleware.Principal, orgID, what string) (roster.Swimmer, bool) {
	sw, allowed, err := householdSwimmer(dir, principal, orgID, chi.URLParam(r, "swimmerID"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve swimmer.", err)
		return roster.Swimmer{}, false
	}
	staff := hasAnyRole(principal, "admin", "coach")
	// Unknown and foreign swimmers are indistinguishable to family members so
	// the route cannot be used to probe which swimmer IDs exist in other households.
	if sw.ID == "" || (!allowed && !staff) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Swimmer not found.", nil)
		return roster.Swimmer{}, false
	}
	return sw, true
}

// householdSwimmer resolves a swimmer the principal may read/write as a
// household member (parent of the household, or the swimmer themself).
func householdSwimmer(dir roster.Directory, principal *appmiddleware.Principal, orgID, swimmerID string) (roster.Swimmer, bool, error) {
	all, err := dir.Swimmers(orgID)
	if err != nil {
		return roster.Swimmer{}, false, err
	}
	sw, found := roster.SwimmerByID(all, swimmerID)
	if !found {
		return roster.Swimmer{}, false, nil
	}
	if roster.HasAnyRole(principal, "parent") {
		ids, err := dir.HouseholdIDsForPrincipal(principal, orgID)
		if err != nil {
			return roster.Swimmer{}, false, err
		}
		for _, id := range ids {
			if id == sw.HouseholdID {
				return sw, true, nil
			}
		}
	}
	if roster.HasAnyRole(principal, "swimmer") && strings.TrimSpace(principal.SwimmerID) == sw.ID {
		return sw, true, nil
	}
	return sw, false, nil
}

type wellnessCheckInBody struct {
	SwimmerID    string  `json:"swimmer_id"`
	Date         string  `json:"date"`
	SleepHours   float64 `json:"sleep_hours" validate:"gte=0,lte=24"`
	SleepQuality int     `json:"sleep_quality" validate:"required,gte=1,lte=5"`
	Soreness     int     `json:"soreness" validate:"required,gte=1,lte=5"`
	Energy       int     `json:"energy" validate:"required,gte=1,lte=5"`
	Stress       int     `json:"stress" validate:"required,gte=1,lte=5"`
	Mood         int     `json:"mood" validate:"required,gte=1,lte=5"`
	Note         string  `json:"note" validate:"max=2000"`
}

func (h *WellnessHandlers) upsertCheckIn(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Check-ins are submitted by the swimmer or their parent.", nil)
		return
	}
	var body wellnessCheckInBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	swimmerID := strings.TrimSpace(body.SwimmerID)
	if swimmerID == "" && hasAnyRole(principal, "swimmer") {
		swimmerID = strings.TrimSpace(principal.SwimmerID)
	}
	if swimmerID == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "swimmer_id is required.", nil)
		return
	}
	sw, allowed, err := householdSwimmer(h.directory, principal, orgID, swimmerID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve household scope.", err)
		return
	}
	if !allowed {
		httpx.WriteError(w, r, http.StatusForbidden, "household_scope_violation", "You can only check in for swimmers in your household.", nil)
		return
	}
	role := "parent"
	if hasAnyRole(principal, "swimmer") && strings.TrimSpace(principal.SwimmerID) == sw.ID {
		role = "swimmer"
	}
	checkin, created, err := h.service.Upsert(orgID, wellness.Input{
		SwimmerID:   sw.ID,
		HouseholdID: sw.HouseholdID,
		Date:        body.Date,
		Metrics: wellness.Metrics{
			SleepHours: body.SleepHours, SleepQuality: body.SleepQuality, Soreness: body.Soreness,
			Energy: body.Energy, Stress: body.Stress, Mood: body.Mood,
		},
		Note:          body.Note,
		SubmittedBy:   principal.UserID,
		SubmittedRole: role,
	})
	if err != nil {
		var verr *wellness.ValidationError
		if errors.As(err, &verr) {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", verr.Error(), []httpx.ValidationIssue{{Field: verr.Field, Message: verr.Message}})
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to save check-in.", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, map[string]any{"checkin": checkin, "created": created})
}

func (h *WellnessHandlers) swimmerTrend(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	sw, ok := readableSwimmer(w, r, h.directory, principal, orgID, "wellness")
	if !ok {
		return
	}
	days := clampDays(r.URL.Query().Get("days"), wellnessDefaultTrendDays, wellnessMaxTrendDays)
	today := h.service.Today()
	from, to, _ := wellness.DateRange(today, days)
	// Load one extra day ahead to include check-ins logged "tomorrow" in
	// timezones ahead of UTC.
	checkins, err := h.service.List(orgID, []string{sw.ID}, from, nextISODate(to))
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load check-ins.", err)
		return
	}
	var todays *wellness.CheckIn
	for _, c := range checkins {
		if c.Date == today {
			todays = c
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"swimmer":       map[string]any{"id": sw.ID, "display_name": sw.DisplayName, "household_id": sw.HouseholdID},
		"from":          from,
		"to":            to,
		"today":         today,
		"checkins":      checkins,
		"today_checkin": todays,
		"flags":         wellness.Flags(checkins, today),
		"averages":      wellness.Average(checkins),
	})
}

type heatmapCell struct {
	Date      string            `json:"date"`
	CheckIn   *wellness.CheckIn `json:"checkin"`
	Readiness *float64          `json:"readiness"`
}

type heatmapRow struct {
	SwimmerID   string            `json:"swimmer_id"`
	DisplayName string            `json:"display_name"`
	Cells       []heatmapCell     `json:"cells"`
	Flags       []wellness.Flag   `json:"flags"`
	Averages    wellness.Averages `json:"averages"`
	Latest      *wellness.CheckIn `json:"latest,omitempty"`
}

type heatmapDay struct {
	Date      string   `json:"date"`
	Count     int      `json:"count"`
	Readiness *float64 `json:"readiness"`
}

func (h *WellnessHandlers) heatmap(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may view the team wellness heat-map.", nil)
		return
	}
	group := roster.CanonicalGroupCode(r.URL.Query().Get("group"))
	days := clampDays(r.URL.Query().Get("days"), wellnessDefaultHeatmapDays, wellnessMaxHeatmapDays)
	today := h.service.Today()
	from, to, dates := wellness.DateRange(today, days)

	all, err := h.directory.Swimmers(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load roster.", err)
		return
	}
	swimmers := []roster.Swimmer{}
	ids := []string{}
	for _, sw := range all {
		if group != "" && !sw.InGroup(group) {
			continue
		}
		swimmers = append(swimmers, sw)
		ids = append(ids, sw.ID)
	}
	checkins, err := h.service.List(orgID, ids, from, to)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load check-ins.", err)
		return
	}
	bySwimmer := map[string][]*wellness.CheckIn{}
	for _, c := range checkins {
		bySwimmer[c.SwimmerID] = append(bySwimmer[c.SwimmerID], c)
	}
	dayTotals := map[string]float64{}
	dayCounts := map[string]int{}
	rows := make([]heatmapRow, 0, len(swimmers))
	flagged := 0
	for _, sw := range swimmers {
		list := bySwimmer[sw.ID]
		byDate := map[string]*wellness.CheckIn{}
		for _, c := range list {
			byDate[c.Date] = c
			dayTotals[c.Date] += c.Readiness
			dayCounts[c.Date]++
		}
		cells := make([]heatmapCell, 0, len(dates))
		for _, d := range dates {
			cell := heatmapCell{Date: d}
			if c, ok := byDate[d]; ok {
				score := c.Readiness
				cell.CheckIn = c
				cell.Readiness = &score
			}
			cells = append(cells, cell)
		}
		row := heatmapRow{SwimmerID: sw.ID, DisplayName: sw.DisplayName, Cells: cells, Flags: wellness.Flags(list, today), Averages: wellness.Average(list)}
		if len(list) > 0 {
			row.Latest = list[len(list)-1]
		}
		if len(row.Flags) > 0 {
			flagged++
		}
		rows = append(rows, row)
	}
	// Flagged swimmers first, then by name, so the coach sees who needs
	// attention without scrolling.
	sort.SliceStable(rows, func(i, j int) bool {
		fi, fj := len(rows[i].Flags) > 0, len(rows[j].Flags) > 0
		if fi != fj {
			return fi
		}
		return strings.ToLower(rows[i].DisplayName) < strings.ToLower(rows[j].DisplayName)
	})
	daily := make([]heatmapDay, 0, len(dates))
	for _, d := range dates {
		day := heatmapDay{Date: d, Count: dayCounts[d]}
		if day.Count > 0 {
			avg := math.Round(dayTotals[d]/float64(day.Count)*10) / 10
			day.Readiness = &avg
		}
		daily = append(daily, day)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"group":            group,
		"from":             from,
		"to":               to,
		"days":             dates,
		"swimmers":         rows,
		"daily":            daily,
		"flagged_count":    flagged,
		"roster_size":      len(rows),
		"checked_in_today": dayCounts[today],
	})
}

func nextISODate(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}
