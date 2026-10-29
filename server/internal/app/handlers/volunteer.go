package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/volunteer"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// VolunteerHandlers serves the volunteer jobs & service-hours API.
type VolunteerHandlers struct {
	service      *volunteer.Service
	auditService *audit.Service
	validate     *validator.Validate
}

// NewVolunteerHandlers returns VolunteerHandlers.
func NewVolunteerHandlers(svc *volunteer.Service, auditSvc *audit.Service) *VolunteerHandlers {
	return &VolunteerHandlers{service: svc, auditService: auditSvc, validate: validator.New()}
}

// RegisterRoutes mounts volunteer routes on the protected router.
func (h *VolunteerHandlers) RegisterRoutes(router chi.Router) {
	router.Route("/volunteer", func(r chi.Router) {
		r.Get("/settings", h.getSettings)
		r.Put("/settings", h.updateSettings)
		r.Get("/templates", h.listTemplates)
		r.Get("/jobs", h.listJobs)
		r.Post("/jobs", h.createJob)
		r.Post("/jobs/bulk", h.bulkCreateJobs)
		r.Get("/jobs/{jobID}", h.getJob)
		r.Patch("/jobs/{jobID}", h.updateJob)
		r.Delete("/jobs/{jobID}", h.deleteJob)
		r.Post("/jobs/{jobID}/signups", h.signUp)
		r.Get("/signups", h.listSignups)
		r.Post("/signups/{signupID}/cancel", h.cancelSignup)
		r.Post("/signups/{signupID}/check-in", h.checkIn)
		r.Get("/requirements", h.listRequirements)
		r.Put("/requirements", h.upsertRequirement)
		r.Delete("/requirements/{requirementID}", h.deleteRequirement)
		r.Get("/credits", h.listCredits)
		r.Post("/credits", h.addCredit)
		r.Get("/ledger", h.seasonLedger)
		r.Get("/households/{householdID}/ledger", h.householdLedger)
		r.Get("/buyouts", h.listBuyouts)
		r.Post("/buyouts", h.billBuyout)
	})
}

func (h *VolunteerHandlers) audit(p *appmiddleware.Principal, orgID, action, resourceType, resourceID string, before, after any, meta map[string]string) {
	recordAudit(h.auditService, p, orgID, action, resourceType, resourceID, before, after, meta)
}

// writeVolunteerError maps domain errors to HTTP responses.
func writeVolunteerError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *volunteer.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", ve.Error(), map[string]any{"field": ve.Field})
	case errors.Is(err, volunteer.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role for this volunteer action.", nil)
	case errors.Is(err, volunteer.ErrJobNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Volunteer job not found.", nil)
	case errors.Is(err, volunteer.ErrSignupNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Volunteer signup not found.", nil)
	case errors.Is(err, volunteer.ErrRequirementNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Requirement not found.", nil)
	case errors.Is(err, volunteer.ErrSeasonNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "season_not_found", "Season not found.", nil)
	case errors.Is(err, volunteer.ErrMeetNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet not found.", nil)
	case errors.Is(err, volunteer.ErrHouseholdNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "household_not_found", "Household not found.", nil)
	case errors.Is(err, volunteer.ErrJobFull):
		httpx.WriteError(w, r, http.StatusConflict, "job_full", "This job is already full.", nil)
	case errors.Is(err, volunteer.ErrDuplicateSignup):
		httpx.WriteError(w, r, http.StatusConflict, "duplicate_signup", err.Error(), nil)
	case errors.Is(err, volunteer.ErrJobStarted):
		httpx.WriteError(w, r, http.StatusConflict, "job_started", "Signups are closed because this job has started.", nil)
	case errors.Is(err, volunteer.ErrCancellationClosed):
		httpx.WriteError(w, r, http.StatusConflict, "cancellation_window_closed", "The cancellation window for this job has closed. Contact the club to cancel.", nil)
	case errors.Is(err, volunteer.ErrSignupNotActive):
		httpx.WriteError(w, r, http.StatusConflict, "signup_not_active", err.Error(), nil)
	case errors.Is(err, volunteer.ErrInvalidTransition):
		httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", err.Error(), nil)
	case errors.Is(err, volunteer.ErrJobHasWorkedSignups):
		httpx.WriteError(w, r, http.StatusConflict, "job_has_worked_signups", err.Error(), nil)
	case errors.Is(err, volunteer.ErrCapacityBelowFilled):
		httpx.WriteError(w, r, http.StatusConflict, "capacity_below_filled", err.Error(), nil)
	case errors.Is(err, volunteer.ErrCertificationRequired):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "certification_required", err.Error(), nil)
	case errors.Is(err, volunteer.ErrAgeRequirement):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "age_requirement", err.Error(), nil)
	case errors.Is(err, volunteer.ErrBuyoutExists):
		httpx.WriteError(w, r, http.StatusConflict, "buyout_exists", err.Error(), nil)
	case errors.Is(err, volunteer.ErrNothingOwed):
		httpx.WriteError(w, r, http.StatusConflict, "nothing_owed", err.Error(), nil)
	case errors.Is(err, volunteer.ErrNoBuyoutRate):
		httpx.WriteError(w, r, http.StatusConflict, "no_buyout_rate", err.Error(), nil)
	case errors.Is(err, volunteer.ErrBillingUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "billing_unavailable", err.Error(), nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Volunteer request failed.", err)
	}
}

func parseTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// ─── Settings & templates ─────────────────────────────────────────────────────

func (h *VolunteerHandlers) getSettings(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	st, err := h.service.GetSettings(r.Context(), p, orgID)
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"settings": st})
}

type volunteerSettingsBody struct {
	DefaultRequiredHours      *float64 `json:"default_required_hours"`
	DefaultBuyoutCentsPerHour *int     `json:"default_buyout_cents_per_hour"`
	CancellationCutoffHours   *int     `json:"cancellation_cutoff_hours"`
	RemindersEnabled          *bool    `json:"reminders_enabled"`
	Timezone                  *string  `json:"timezone"`
}

func (h *VolunteerHandlers) updateSettings(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	var body volunteerSettingsBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	st, err := h.service.UpdateSettings(r.Context(), p, orgID, volunteer.SettingsUpdate{
		DefaultRequiredHours:      body.DefaultRequiredHours,
		DefaultBuyoutCentsPerHour: body.DefaultBuyoutCentsPerHour,
		CancellationCutoffHours:   body.CancellationCutoffHours,
		RemindersEnabled:          body.RemindersEnabled,
		Timezone:                  body.Timezone,
	})
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.settings.update", "volunteer_settings", orgID, nil, st, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"settings": st})
}

func (h *VolunteerHandlers) listTemplates(w http.ResponseWriter, r *http.Request) {
	p, _, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(p, "admin", "coach") {
		writeVolunteerError(w, r, volunteer.ErrForbidden)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"templates": volunteer.Templates(), "roles": volunteer.Roles})
}

// ─── Jobs ─────────────────────────────────────────────────────────────────────

func (h *VolunteerHandlers) listJobs(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := volunteer.JobFilter{
		MeetID:   strings.TrimSpace(q.Get("meet_id")),
		SeasonID: strings.TrimSpace(q.Get("season_id")),
		Upcoming: q.Get("upcoming") == "true",
	}
	for name, dst := range map[string]*time.Time{"from": &f.From, "to": &f.To} {
		raw := q.Get(name)
		if raw == "" {
			continue
		}
		t, ok := parseTimestamp(raw)
		if !ok {
			httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", name+" must be RFC3339 or YYYY-MM-DD.", nil)
			return
		}
		*dst = t
	}
	jobs, err := h.service.ListJobs(r.Context(), p, orgID, f)
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	capacity, filled := 0, 0
	for _, j := range jobs {
		capacity += j.Capacity
		filled += j.FilledCount
	}
	fillRate := 0.0
	if capacity > 0 {
		fillRate = float64(filled) / float64(capacity)
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	page, total := paginateSlice(jobs, pagination)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"jobs":   page,
		"count":  len(page),
		"total":  total,
		"limit":  pagination.Limit,
		"offset": pagination.Offset,
		"summary": map[string]any{
			"job_count":      len(jobs),
			"total_capacity": capacity,
			"filled_spots":   filled,
			"open_spots":     capacity - filled,
			"fill_rate":      fillRate,
		},
	})
}

type volunteerJobBody struct {
	MeetID                string   `json:"meet_id"`
	EventName             string   `json:"event_name"`
	SeasonID              string   `json:"season_id"`
	Role                  string   `json:"role" validate:"required"`
	Title                 string   `json:"title" validate:"required"`
	Description           string   `json:"description"`
	Location              string   `json:"location"`
	StartsAt              string   `json:"starts_at" validate:"required"`
	EndsAt                string   `json:"ends_at" validate:"required"`
	Capacity              int      `json:"capacity" validate:"required,gte=1"`
	HourCredit            *float64 `json:"hour_credit"`
	MinAge                int      `json:"min_age"`
	RequiresCertification bool     `json:"requires_certification"`
}

func (h *VolunteerHandlers) createJob(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(p, "admin", "coach") {
		writeVolunteerError(w, r, volunteer.ErrForbidden)
		return
	}
	var body volunteerJobBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	start, ok1 := parseTimestamp(body.StartsAt)
	end, ok2 := parseTimestamp(body.EndsAt)
	if !ok1 || !ok2 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "starts_at and ends_at must be RFC3339 timestamps.", nil)
		return
	}
	job, err := h.service.CreateJob(r.Context(), p, orgID, volunteer.JobInput{
		MeetID: body.MeetID, EventName: body.EventName, SeasonID: body.SeasonID, Role: body.Role, Title: body.Title,
		Description: body.Description, Location: body.Location, StartsAt: start, EndsAt: end, Capacity: body.Capacity,
		HourCredit: body.HourCredit, MinAge: body.MinAge, RequiresCertification: body.RequiresCertification,
	})
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.job.create", "volunteer_job", job.ID, nil, job, nil)
	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{"job": job})
}

type volunteerShiftBody struct {
	StartsAt string `json:"starts_at" validate:"required"`
	EndsAt   string `json:"ends_at" validate:"required"`
	Label    string `json:"label"`
}

type volunteerBulkBody struct {
	MeetID     string                   `json:"meet_id"`
	EventName  string                   `json:"event_name"`
	SeasonID   string                   `json:"season_id"`
	Location   string                   `json:"location"`
	TemplateID string                   `json:"template_id"`
	Roles      []volunteer.TemplateRole `json:"roles"`
	Shifts     []volunteerShiftBody     `json:"shifts" validate:"required,min=1,max=40,dive"`
}

func (h *VolunteerHandlers) bulkCreateJobs(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(p, "admin", "coach") {
		writeVolunteerError(w, r, volunteer.ErrForbidden)
		return
	}
	var body volunteerBulkBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	shifts := make([]volunteer.Shift, 0, len(body.Shifts))
	for _, sh := range body.Shifts {
		start, ok1 := parseTimestamp(sh.StartsAt)
		end, ok2 := parseTimestamp(sh.EndsAt)
		if !ok1 || !ok2 {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Every shift needs RFC3339 starts_at and ends_at.", nil)
			return
		}
		shifts = append(shifts, volunteer.Shift{StartsAt: start, EndsAt: end, Label: sh.Label})
	}
	jobs, err := h.service.BulkCreateJobs(r.Context(), p, orgID, volunteer.BulkInput{
		MeetID: body.MeetID, EventName: body.EventName, SeasonID: body.SeasonID, Location: body.Location,
		TemplateID: body.TemplateID, Roles: body.Roles, Shifts: shifts,
	})
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	ids := make([]string, 0, len(jobs))
	for _, j := range jobs {
		ids = append(ids, j.ID)
	}
	h.audit(p, orgID, "volunteer.job.bulk_create", "volunteer_job", body.MeetID, nil, map[string]any{"job_ids": ids}, map[string]string{
		"template_id": body.TemplateID, "meet_id": body.MeetID, "event_name": body.EventName,
	})
	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{"jobs": jobs, "count": len(jobs)})
}

func (h *VolunteerHandlers) getJob(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	job, err := h.service.GetJob(r.Context(), p, orgID, chi.URLParam(r, "jobID"))
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"job": job})
}

type volunteerJobPatchBody struct {
	EventName             *string  `json:"event_name"`
	SeasonID              *string  `json:"season_id"`
	Role                  *string  `json:"role"`
	Title                 *string  `json:"title"`
	Description           *string  `json:"description"`
	Location              *string  `json:"location"`
	StartsAt              *string  `json:"starts_at"`
	EndsAt                *string  `json:"ends_at"`
	Capacity              *int     `json:"capacity"`
	HourCredit            *float64 `json:"hour_credit"`
	MinAge                *int     `json:"min_age"`
	RequiresCertification *bool    `json:"requires_certification"`
}

func (h *VolunteerHandlers) updateJob(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(p, "admin", "coach") {
		writeVolunteerError(w, r, volunteer.ErrForbidden)
		return
	}
	var body volunteerJobPatchBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	up := volunteer.JobUpdate{
		EventName: body.EventName, SeasonID: body.SeasonID, Role: body.Role, Title: body.Title,
		Description: body.Description, Location: body.Location, Capacity: body.Capacity,
		HourCredit: body.HourCredit, MinAge: body.MinAge, RequiresCertification: body.RequiresCertification,
	}
	if body.StartsAt != nil {
		t, ok := parseTimestamp(*body.StartsAt)
		if !ok {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "starts_at must be an RFC3339 timestamp.", nil)
			return
		}
		up.StartsAt = &t
	}
	if body.EndsAt != nil {
		t, ok := parseTimestamp(*body.EndsAt)
		if !ok {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "ends_at must be an RFC3339 timestamp.", nil)
			return
		}
		up.EndsAt = &t
	}
	before, job, err := h.service.UpdateJob(r.Context(), p, orgID, chi.URLParam(r, "jobID"), up)
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.job.update", "volunteer_job", job.ID, before, job, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"job": job})
}

func (h *VolunteerHandlers) deleteJob(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	job, err := h.service.DeleteJob(r.Context(), p, orgID, chi.URLParam(r, "jobID"))
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.job.delete", "volunteer_job", job.ID, job, nil, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// ─── Signups ──────────────────────────────────────────────────────────────────

type volunteerSignupBody struct {
	HouseholdID   string `json:"household_id"`
	VolunteerName string `json:"volunteer_name" validate:"required"`
	SwimmerID     string `json:"swimmer_id"`
	Certified     bool   `json:"certified"`
	Notes         string `json:"notes"`
}

func (h *VolunteerHandlers) signUp(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	var body volunteerSignupBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	su, err := h.service.SignUp(r.Context(), p, orgID, chi.URLParam(r, "jobID"), volunteer.SignupInput{
		HouseholdID: body.HouseholdID, VolunteerName: body.VolunteerName, SwimmerID: body.SwimmerID,
		Certified: body.Certified, Notes: body.Notes,
	})
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.signup.create", "volunteer_signup", su.ID, nil, su.Signup, map[string]string{
		"job_id": su.JobID, "household_id": su.HouseholdID,
	})
	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{"signup": su})
}

func (h *VolunteerHandlers) listSignups(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	list, err := h.service.ListSignups(r.Context(), p, orgID, volunteer.SignupFilter{
		JobID:       strings.TrimSpace(q.Get("job_id")),
		HouseholdID: strings.TrimSpace(q.Get("household_id")),
		MeetID:      strings.TrimSpace(q.Get("meet_id")),
		Status:      strings.TrimSpace(q.Get("status")),
	})
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	page, total := paginateSlice(list, pagination)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"signups": page,
		"count":   len(page),
		"total":   total,
		"limit":   pagination.Limit,
		"offset":  pagination.Offset,
	})
}

type volunteerCancelBody struct {
	Reason string `json:"reason" validate:"max=500"`
}

func (h *VolunteerHandlers) cancelSignup(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	var body volunteerCancelBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{AllowEmpty: true}) {
		return
	}
	before, su, err := h.service.Cancel(r.Context(), p, orgID, chi.URLParam(r, "signupID"), body.Reason)
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.signup.cancel", "volunteer_signup", su.ID, before, su, map[string]string{"reason": body.Reason})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"signup": su})
}

type volunteerCheckInBody struct {
	Status string   `json:"status" validate:"required,oneof=worked no_show confirmed"`
	Hours  *float64 `json:"hours"`
}

func (h *VolunteerHandlers) checkIn(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(p, "admin", "coach") {
		writeVolunteerError(w, r, volunteer.ErrForbidden)
		return
	}
	var body volunteerCheckInBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	before, su, err := h.service.CheckIn(r.Context(), p, orgID, chi.URLParam(r, "signupID"), body.Status, body.Hours)
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.signup.check_in", "volunteer_signup", su.ID, before, su, map[string]string{"status": su.Status})
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"signup": su})
}

// ─── Requirements & credits ───────────────────────────────────────────────────

func (h *VolunteerHandlers) listRequirements(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	list, err := h.service.ListRequirements(r.Context(), p, orgID, strings.TrimSpace(r.URL.Query().Get("season_id")))
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"requirements": list})
}

type volunteerRequirementBody struct {
	SeasonID           string  `json:"season_id" validate:"required"`
	HouseholdID        string  `json:"household_id"`
	RequiredHours      float64 `json:"required_hours" validate:"gte=0"`
	BuyoutCentsPerHour int     `json:"buyout_cents_per_hour" validate:"gte=0"`
	Notes              string  `json:"notes"`
}

func (h *VolunteerHandlers) upsertRequirement(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	var body volunteerRequirementBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	req, err := h.service.UpsertRequirement(r.Context(), p, orgID, volunteer.RequirementInput{
		SeasonID: body.SeasonID, HouseholdID: body.HouseholdID, RequiredHours: body.RequiredHours,
		BuyoutCentsPerHour: body.BuyoutCentsPerHour, Notes: body.Notes,
	})
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.requirement.upsert", "volunteer_requirement", req.ID, nil, req, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"requirement": req})
}

func (h *VolunteerHandlers) deleteRequirement(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "requirementID")
	if err := h.service.DeleteRequirement(r.Context(), p, orgID, id); err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.requirement.delete", "volunteer_requirement", id, nil, nil, nil)
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *VolunteerHandlers) listCredits(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	list, err := h.service.ListCredits(r.Context(), p, orgID, strings.TrimSpace(q.Get("season_id")), strings.TrimSpace(q.Get("household_id")))
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"credits": list})
}

type volunteerCreditBody struct {
	SeasonID    string  `json:"season_id" validate:"required"`
	HouseholdID string  `json:"household_id" validate:"required"`
	Hours       float64 `json:"hours" validate:"required"`
	Reason      string  `json:"reason" validate:"required"`
}

func (h *VolunteerHandlers) addCredit(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	var body volunteerCreditBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	c, err := h.service.AddCredit(r.Context(), p, orgID, volunteer.CreditInput{
		SeasonID: body.SeasonID, HouseholdID: body.HouseholdID, Hours: body.Hours, Reason: body.Reason,
	})
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.credit.create", "volunteer_credit", c.ID, nil, c, map[string]string{"household_id": c.HouseholdID})
	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{"credit": c})
}

// ─── Ledger & buyouts ─────────────────────────────────────────────────────────

func (h *VolunteerHandlers) seasonLedger(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	seasonID := strings.TrimSpace(r.URL.Query().Get("season_id"))
	if seasonID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "season_id is required.", nil)
		return
	}
	rows, err := h.service.SeasonLedger(r.Context(), p, orgID, seasonID)
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	counts := map[string]int{}
	due := 0
	for _, row := range rows {
		counts[row.Status]++
		due += row.BuyoutDueCents
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ledger":  rows,
		"summary": map[string]any{"households": len(rows), "by_status": counts, "buyout_due_cents": due},
	})
}

func (h *VolunteerHandlers) householdLedger(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	rows, err := h.service.HouseholdLedger(r.Context(), p, orgID, chi.URLParam(r, "householdID"))
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"ledger": rows})
}

func (h *VolunteerHandlers) listBuyouts(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	list, err := h.service.ListBuyouts(r.Context(), p, orgID, strings.TrimSpace(r.URL.Query().Get("season_id")))
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"buyouts": list})
}

type volunteerBuyoutBody struct {
	SeasonID    string `json:"season_id" validate:"required"`
	HouseholdID string `json:"household_id" validate:"required"`
	DueDate     string `json:"due_date"`
	Issue       bool   `json:"issue"`
}

func (h *VolunteerHandlers) billBuyout(w http.ResponseWriter, r *http.Request) {
	p, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(p, "admin") {
		writeVolunteerError(w, r, volunteer.ErrForbidden)
		return
	}
	var body volunteerBuyoutBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	b, inv, err := h.service.BillBuyout(r.Context(), p, orgID, volunteer.BuyoutInput{
		SeasonID: body.SeasonID, HouseholdID: body.HouseholdID, DueDate: body.DueDate, Issue: body.Issue,
	})
	if err != nil {
		writeVolunteerError(w, r, err)
		return
	}
	h.audit(p, orgID, "volunteer.buyout.bill", "volunteer_buyout", b.ID, nil, b, map[string]string{
		"household_id": b.HouseholdID, "season_id": b.SeasonID, "invoice_id": b.InvoiceID,
	})
	_ = httpx.WriteJSON(w, http.StatusCreated, map[string]any{"buyout": b, "invoice": inv})
}
