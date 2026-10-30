package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/seasons"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/go-chi/chi/v5"
)

// Structured workouts, season periodization and training analytics.
// Writes: admin/coach. Reads of practice workouts / schedule / swimmer
// analytics: parent and swimmer too, scoped to their household's groups.

// SetSeasonsService wires the seasons domain so periodization phases can be
// validated against (and linked to) existing seasons.
func (h *TrainingHandlers) SetSeasonsService(svc *seasons.Service) {
	h.seasonsService = svc
}

func (h *TrainingHandlers) registerWorkoutRoutes(router chi.Router) {
	router.Get("/workouts", h.listWorkouts)
	router.Post("/workouts", h.createWorkout)
	router.Post("/workouts/parse", h.parseWorkoutText)
	router.Post("/workouts/preview", h.previewWorkout)
	router.Get("/workouts/{id}", h.getWorkout)
	router.Put("/workouts/{id}", h.updateWorkout)
	router.Delete("/workouts/{id}", h.deleteWorkout)
	router.Post("/workouts/{id}/duplicate", h.duplicateWorkout)
	router.Post("/workouts/{id}/assign", h.assignWorkout)

	router.Get("/practices/{id}/workout", h.getPracticeWorkout)
	router.Put("/practices/{id}/workout", h.setPracticeWorkout)
	router.Delete("/practices/{id}/workout", h.clearPracticeWorkout)

	router.Get("/training/schedule", h.trainingSchedule)
	router.Get("/training/analytics", h.trainingAnalytics)
	router.Get("/training/analytics/swimmers/{swimmerID}", h.swimmerTrainingAnalytics)

	router.Get("/seasons/{id}/training-phases", h.listTrainingPhases)
	router.Post("/seasons/{id}/training-phases", h.createTrainingPhase)
	router.Get("/seasons/{id}/training-plan", h.seasonTrainingPlan)
	router.Put("/training-phases/{phaseID}", h.updateTrainingPhase)
	router.Delete("/training-phases/{phaseID}", h.deleteTrainingPhase)
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

func requireStaff(w http.ResponseWriter, r *http.Request, principal *appmiddleware.Principal, action string) bool {
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may "+action+".", nil)
		return false
	}
	return true
}

func (h *TrainingHandlers) audit(principal *appmiddleware.Principal, orgID, action, resourceType, resourceID string, before, after any, metadata map[string]string) {
	recordAudit(h.auditService, principal, orgID, action, resourceType, resourceID, before, after, metadata)
}

// writeTrainingError maps training-domain errors onto HTTP responses.
func writeTrainingError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	var verr *training.WorkoutValidationError
	switch {
	case errors.As(err, &verr):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Validation failed.",
			[]httpx.ValidationIssue{{Field: verr.Field, Message: verr.Message}})
	case errors.Is(err, training.ErrInvalidAssignment):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Provide practice_id, or a date (and optional group_id) for a new practice.", nil)
	case errors.Is(err, training.ErrInvalidDate):
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Practice date must be YYYY-MM-DD.", nil)
	case errors.Is(err, training.ErrInvalidType):
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid practice type.", nil)
	case errors.Is(err, training.ErrInvalidRange):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_range", err.Error(), nil)
	case errors.Is(err, training.ErrWorkoutNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Workout not found.", nil)
	case errors.Is(err, training.ErrPracticeNoWorkout):
		httpx.WriteError(w, r, http.StatusNotFound, "no_workout", "This practice has no structured workout.", nil)
	case errors.Is(err, training.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Practice not found.", nil)
	case errors.Is(err, training.ErrPhaseNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Training phase not found.", nil)
	case errors.Is(err, training.ErrPhaseOverlap):
		httpx.WriteError(w, r, http.StatusConflict, "phase_overlap", err.Error(), nil)
	case errors.Is(err, seasons.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Season not found.", nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", fallback, err)
	}
}

// groupIndex canonicalises practice/phase group references (group ID or
// group code, any case) to the group ID, and reports whether a reference is
// a known group of the org.
func (h *TrainingHandlers) groupIndex(orgID string) (func(string) string, func(string) bool) {
	byKey := map[string]string{}
	if h.groupsService != nil {
		for _, g := range h.groupsService.SnapshotGroups(orgID) {
			byKey[g.ID] = g.ID
			byKey[strings.ToUpper(strings.TrimSpace(g.Code))] = g.ID
		}
	}
	lookup := func(v string) (string, bool) {
		v = strings.TrimSpace(v)
		if id, ok := byKey[v]; ok {
			return id, true
		}
		id, ok := byKey[strings.ToUpper(v)]
		return id, ok
	}
	canon := func(v string) string {
		if id, ok := lookup(v); ok {
			return id
		}
		return strings.TrimSpace(v)
	}
	known := func(v string) bool {
		if h.groupsService == nil {
			return true
		}
		_, ok := lookup(v)
		return ok
	}
	return canon, known
}

// practiceScope reports which practices a principal may read. Staff see all;
// parents/swimmers see team-wide practices (no group) plus practices of the
// groups their household's swimmers belong to.
type practiceScope struct {
	all    bool
	groups map[string]bool
	canon  func(string) string
}

func (s practiceScope) allows(p *training.Practice) bool {
	if s.all {
		return true
	}
	g := s.canon(p.GroupID)
	return g == "" || s.groups[g]
}

func (h *TrainingHandlers) practiceScopeFor(principal *appmiddleware.Principal, orgID string) (practiceScope, error) {
	canon, _ := h.groupIndex(orgID)
	scope := practiceScope{groups: map[string]bool{}, canon: canon}
	if hasAnyRole(principal, "admin", "coach") {
		scope.all = true
		return scope, nil
	}
	if h.membersService == nil {
		return scope, nil
	}
	links, err := h.membersService.ListSwimmerGroupLinks(principal, orgID)
	if err != nil {
		if errors.Is(err, members.ErrForbidden) {
			return scope, nil
		}
		return scope, err
	}
	for _, link := range links {
		scope.groups[canon(link.GroupCode)] = true
	}
	return scope, nil
}

// visiblePractice loads a practice and hides it (404) outside the scope.
func (h *TrainingHandlers) visiblePractice(w http.ResponseWriter, r *http.Request, principal *appmiddleware.Principal, orgID, id string) (*training.Practice, bool) {
	p, err := h.service.Get(orgID, id)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to get practice.")
		return nil, false
	}
	scope, err := h.practiceScopeFor(principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve practice scope.", err)
		return nil, false
	}
	if !scope.allows(p) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Practice not found.", nil)
		return nil, false
	}
	return p, true
}

func redactPracticeForViewer(principal *appmiddleware.Principal, p *training.Practice) {
	if !hasAnyRole(principal, "admin", "coach") {
		p.Notes = ""
	}
}

// ─── Workout library ──────────────────────────────────────────────────────────

type workoutBody struct {
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	Course      string                    `json:"course"`
	IsTemplate  *bool                     `json:"is_template"`
	Tags        []string                  `json:"tags"`
	Sections    []training.WorkoutSection `json:"sections"`
}

func (b workoutBody) input(defaultTemplate bool) training.WorkoutInput {
	isTemplate := defaultTemplate
	if b.IsTemplate != nil {
		isTemplate = *b.IsTemplate
	}
	return training.WorkoutInput{
		Title: b.Title, Description: b.Description, Course: b.Course,
		IsTemplate: isTemplate, Tags: b.Tags, Sections: b.Sections,
	}
}

// listWorkouts handles GET /api/v1/workouts
func (h *TrainingHandlers) listWorkouts(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "browse the workout library") {
		return
	}
	q := r.URL.Query()
	scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
	if scope != "" && scope != "templates" && scope != "all" {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "scope must be templates or all.", nil)
		return
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	items, err := h.service.ListWorkouts(orgID, training.WorkoutFilter{
		TemplatesOnly: scope != "all",
		Query:         q.Get("q"),
		Tag:           q.Get("tag"),
	})
	if err != nil {
		writeTrainingError(w, r, err, "Failed to list workouts.")
		return
	}
	page, total := paginateSlice(items, pagination)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"workouts": page, "count": len(page), "total": total,
		"limit": pagination.Limit, "offset": pagination.Offset,
	})
}

// createWorkout handles POST /api/v1/workouts
func (h *TrainingHandlers) createWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "create workouts") {
		return
	}
	var body workoutBody
	if !decodeAndValidateJSON(w, r, &body, nil, decodeAndValidateOptions{}) {
		return
	}
	wk, err := h.service.CreateWorkout(orgID, body.input(true), principal.UserID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to create workout.")
		return
	}
	h.audit(principal, orgID, "workout.create", "workout", wk.ID, nil, wk, nil)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"workout": wk})
}

type parseWorkoutBody struct {
	Text   string `json:"text" validate:"max=20000"`
	Course string `json:"course"`
}

// parseWorkoutText handles POST /api/v1/workouts/parse (no persistence).
func (h *TrainingHandlers) parseWorkoutText(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "parse workouts") {
		return
	}
	var body parseWorkoutBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	course, err := training.NormalizeCourse(body.Course)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to parse workout.")
		return
	}
	res := training.ParseQuickEntry(body.Text)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"course":   course,
		"sections": res.Sections,
		"errors":   res.Errors,
		"totals":   training.ComputeTotals(res.Sections, course),
	})
}

type previewWorkoutBody struct {
	Course   string                    `json:"course"`
	Sections []training.WorkoutSection `json:"sections"`
}

// previewWorkout handles POST /api/v1/workouts/preview (no persistence).
func (h *TrainingHandlers) previewWorkout(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "preview workouts") {
		return
	}
	var body previewWorkoutBody
	if !decodeAndValidateJSON(w, r, &body, nil, decodeAndValidateOptions{}) {
		return
	}
	course, err := training.NormalizeCourse(body.Course)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to preview workout.")
		return
	}
	sections, err := training.NormalizeSections(body.Sections)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to preview workout.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"course":   course,
		"sections": sections,
		"totals":   training.ComputeTotals(sections, course),
		"text":     training.FormatWorkoutText(sections),
	})
}

// getWorkout handles GET /api/v1/workouts/{id}
func (h *TrainingHandlers) getWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "view library workouts") {
		return
	}
	wk, err := h.service.GetWorkout(orgID, chi.URLParam(r, "id"))
	if err != nil {
		writeTrainingError(w, r, err, "Failed to get workout.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"workout": wk, "text": training.FormatWorkoutText(wk.Sections)})
}

// updateWorkout handles PUT /api/v1/workouts/{id}
func (h *TrainingHandlers) updateWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "update workouts") {
		return
	}
	id := chi.URLParam(r, "id")
	var body workoutBody
	if !decodeAndValidateJSON(w, r, &body, nil, decodeAndValidateOptions{}) {
		return
	}
	before, err := h.service.GetWorkout(orgID, id)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to update workout.")
		return
	}
	wk, err := h.service.UpdateWorkout(orgID, id, body.input(before.IsTemplate), principal.UserID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to update workout.")
		return
	}
	h.audit(principal, orgID, "workout.update", "workout", wk.ID, before, wk, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"workout": wk})
}

// deleteWorkout handles DELETE /api/v1/workouts/{id}
func (h *TrainingHandlers) deleteWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "delete workouts") {
		return
	}
	id := chi.URLParam(r, "id")
	before, err := h.service.GetWorkout(orgID, id)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to delete workout.")
		return
	}
	if err := h.service.DeleteWorkout(orgID, id); err != nil {
		writeTrainingError(w, r, err, "Failed to delete workout.")
		return
	}
	h.audit(principal, orgID, "workout.delete", "workout", id, before, nil, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

type duplicateWorkoutBody struct {
	Title      *string `json:"title" validate:"omitempty,max=160"`
	IsTemplate *bool   `json:"is_template"`
}

// duplicateWorkout handles POST /api/v1/workouts/{id}/duplicate
func (h *TrainingHandlers) duplicateWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "duplicate workouts") {
		return
	}
	var body duplicateWorkoutBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{AllowEmpty: true}) {
		return
	}
	id := chi.URLParam(r, "id")
	wk, err := h.service.DuplicateWorkout(orgID, id, training.DuplicateInput{Title: body.Title, IsTemplate: body.IsTemplate}, principal.UserID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to duplicate workout.")
		return
	}
	h.audit(principal, orgID, "workout.duplicate", "workout", wk.ID, nil, wk, map[string]string{"source_workout_id": id})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"workout": wk})
}

type assignWorkoutBody struct {
	PracticeID string `json:"practice_id"`
	Date       string `json:"date"`
	GroupID    string `json:"group_id"`
	Type       string `json:"type"`
}

// assignWorkout handles POST /api/v1/workouts/{id}/assign
func (h *TrainingHandlers) assignWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "assign workouts") {
		return
	}
	var body assignWorkoutBody
	if !decodeAndValidateJSON(w, r, &body, nil, decodeAndValidateOptions{}) {
		return
	}
	if g := strings.TrimSpace(body.GroupID); g != "" && body.PracticeID == "" {
		canon, known := h.groupIndex(orgID)
		if !known(g) {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Validation failed.",
				[]httpx.ValidationIssue{{Field: "group_id", Message: "unknown group"}})
			return
		}
		body.GroupID = canon(g)
	}
	id := chi.URLParam(r, "id")
	practice, wk, err := h.service.AssignWorkout(orgID, id, training.AssignInput{
		PracticeID: body.PracticeID, Date: body.Date, GroupID: body.GroupID, Type: body.Type,
	}, principal.UserID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to assign workout.")
		return
	}
	h.audit(principal, orgID, "workout.assign", "practice", practice.ID, nil, practice,
		map[string]string{"source_workout_id": id, "workout_id": wk.ID})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"practice": practice, "workout": wk})
}

// ─── Practice ⇄ workout ───────────────────────────────────────────────────────

// getPracticeWorkout handles GET /api/v1/practices/{id}/workout
func (h *TrainingHandlers) getPracticeWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	p, ok := h.visiblePractice(w, r, principal, orgID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	_, wk, err := h.service.GetPracticeWorkout(orgID, p.ID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to load practice workout.")
		return
	}
	redactPracticeForViewer(principal, p)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"practice": p, "workout": wk})
}

// setPracticeWorkout handles PUT /api/v1/practices/{id}/workout
func (h *TrainingHandlers) setPracticeWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "edit practice workouts") {
		return
	}
	var body workoutBody
	if !decodeAndValidateJSON(w, r, &body, nil, decodeAndValidateOptions{}) {
		return
	}
	if body.IsTemplate != nil && *body.IsTemplate {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "A practice workout cannot be a template; duplicate it with is_template=true instead.", nil)
		return
	}
	id := chi.URLParam(r, "id")
	practice, wk, err := h.service.SetPracticeWorkout(orgID, id, body.input(false), principal.UserID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to save practice workout.")
		return
	}
	h.audit(principal, orgID, "practice.workout.set", "practice", practice.ID, nil, wk, map[string]string{"workout_id": wk.ID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"practice": practice, "workout": wk})
}

// clearPracticeWorkout handles DELETE /api/v1/practices/{id}/workout
func (h *TrainingHandlers) clearPracticeWorkout(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "remove practice workouts") {
		return
	}
	id := chi.URLParam(r, "id")
	practice, err := h.service.ClearPracticeWorkout(orgID, id, principal.UserID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to remove practice workout.")
		return
	}
	h.audit(principal, orgID, "practice.workout.clear", "practice", practice.ID, nil, practice, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"practice": practice})
}

// ─── Schedule & analytics ─────────────────────────────────────────────────────

func dateRangeFromQuery(r *http.Request, defaultBack, defaultForward int) (string, string, error) {
	q := r.URL.Query()
	today := time.Now().UTC()
	from := strings.TrimSpace(q.Get("from"))
	to := strings.TrimSpace(q.Get("to"))
	if from == "" {
		from = today.AddDate(0, 0, -defaultBack).Format("2006-01-02")
	}
	if to == "" {
		to = today.AddDate(0, 0, defaultForward).Format("2006-01-02")
	}
	if err := training.ValidateRange(from, to); err != nil {
		return "", "", err
	}
	return from, to, nil
}

type scheduleItem struct {
	Practice *training.Practice `json:"practice"`
	Workout  *training.Workout  `json:"workout"`
}

// trainingSchedule handles GET /api/v1/training/schedule
func (h *TrainingHandlers) trainingSchedule(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	from, to, err := dateRangeFromQuery(r, 0, 27)
	if err != nil {
		writeTrainingError(w, r, err, "")
		return
	}
	scope, err := h.practiceScopeFor(principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve practice scope.", err)
		return
	}
	groupFilter := strings.TrimSpace(r.URL.Query().Get("group_id"))
	all, err := h.service.List(orgID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to load schedule.")
		return
	}
	practices := []*training.Practice{}
	for i := len(all) - 1; i >= 0; i-- { // List is newest-first; schedule reads oldest-first
		p := all[i]
		if p.Date < from || p.Date > to || !scope.allows(p) {
			continue
		}
		if groupFilter != "" && scope.canon(p.GroupID) != scope.canon(groupFilter) {
			continue
		}
		redactPracticeForViewer(principal, p)
		practices = append(practices, p)
	}
	workouts, err := h.service.WorkoutsForPractices(orgID, practices)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to load workouts.")
		return
	}
	items := make([]scheduleItem, 0, len(practices))
	for _, p := range practices {
		items = append(items, scheduleItem{Practice: p, Workout: workouts[p.WorkoutID]})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "items": items})
}

// trainingAnalytics handles GET /api/v1/training/analytics
func (h *TrainingHandlers) trainingAnalytics(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "view training analytics") {
		return
	}
	from, to, err := dateRangeFromQuery(r, 55, 0)
	if err != nil {
		writeTrainingError(w, r, err, "")
		return
	}
	canon, _ := h.groupIndex(orgID)
	groupFilter := canon(r.URL.Query().Get("group_id")) // canon trims; empty stays empty
	out, err := h.service.GroupAnalytics(orgID, from, to, groupFilter, canon)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to compute training analytics.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// swimmerTrainingAnalytics handles GET /api/v1/training/analytics/swimmers/{swimmerID}
func (h *TrainingHandlers) swimmerTrainingAnalytics(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	if h.membersService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "unavailable", "Member data is unavailable.", nil)
		return
	}
	from, to, err := dateRangeFromQuery(r, 55, 0)
	if err != nil {
		writeTrainingError(w, r, err, "")
		return
	}
	swimmerID := strings.TrimSpace(chi.URLParam(r, "swimmerID"))
	// ListSwimmers is household-scoped for parent/swimmer principals, so a
	// swimmer outside the caller's household is indistinguishable from missing.
	swimmers, err := h.membersService.ListSwimmers(principal, orgID, members.ListSwimmersInput{})
	if err != nil {
		if errors.Is(err, members.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Swimmer not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load swimmers.", err)
		return
	}
	found := false
	for _, sw := range swimmers {
		if sw.ID == swimmerID {
			found = true
			break
		}
	}
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Swimmer not found.", nil)
		return
	}
	links, err := h.membersService.ListSwimmerGroupLinks(principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load swimmer groups.", err)
		return
	}
	canon, _ := h.groupIndex(orgID)
	groups := map[string]bool{}
	for _, link := range links {
		if link.SwimmerID == swimmerID {
			groups[canon(link.GroupCode)] = true
		}
	}
	out, err := h.service.SwimmerAnalytics(orgID, swimmerID, groups, from, to, canon)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to compute swimmer analytics.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ─── Season periodization ─────────────────────────────────────────────────────

func (h *TrainingHandlers) seasonWindow(w http.ResponseWriter, r *http.Request, orgID, seasonID string) (training.SeasonWindow, bool) {
	if h.seasonsService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "unavailable", "Seasons are unavailable.", nil)
		return training.SeasonWindow{}, false
	}
	season, err := h.seasonsService.Get(orgID, seasonID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to load season.")
		return training.SeasonWindow{}, false
	}
	return training.SeasonWindow{ID: season.ID, StartDate: season.StartDate, EndDate: season.EndDate}, true
}

type phaseBody struct {
	GroupID             string `json:"group_id" validate:"required"`
	Phase               string `json:"phase" validate:"required"`
	Name                string `json:"name"`
	StartDate           string `json:"start_date" validate:"required"`
	EndDate             string `json:"end_date" validate:"required"`
	WeeklyYardageTarget int    `json:"weekly_yardage_target"`
	Focus               string `json:"focus"`
}

func (h *TrainingHandlers) phaseInput(w http.ResponseWriter, r *http.Request, orgID string, body phaseBody) (training.PhaseInput, bool) {
	canon, known := h.groupIndex(orgID)
	if !known(body.GroupID) {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Validation failed.",
			[]httpx.ValidationIssue{{Field: "group_id", Message: "unknown group"}})
		return training.PhaseInput{}, false
	}
	return training.PhaseInput{
		GroupID: canon(body.GroupID), Phase: body.Phase, Name: body.Name,
		StartDate: body.StartDate, EndDate: body.EndDate,
		WeeklyYardageTarget: body.WeeklyYardageTarget, Focus: body.Focus,
	}, true
}

// listTrainingPhases handles GET /api/v1/seasons/{id}/training-phases
func (h *TrainingHandlers) listTrainingPhases(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "view season plans") {
		return
	}
	season, ok := h.seasonWindow(w, r, orgID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	phases, err := h.service.ListPhases(orgID, season.ID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to list training phases.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"phases": phases})
}

// createTrainingPhase handles POST /api/v1/seasons/{id}/training-phases
func (h *TrainingHandlers) createTrainingPhase(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "edit season plans") {
		return
	}
	var body phaseBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	season, ok := h.seasonWindow(w, r, orgID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	in, ok := h.phaseInput(w, r, orgID, body)
	if !ok {
		return
	}
	phase, err := h.service.CreatePhase(orgID, season, in, principal.UserID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to create training phase.")
		return
	}
	h.audit(principal, orgID, "training_phase.create", "training_phase", phase.ID, nil, phase, map[string]string{"season_id": season.ID})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"phase": phase})
}

// updateTrainingPhase handles PUT /api/v1/training-phases/{phaseID}
func (h *TrainingHandlers) updateTrainingPhase(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "edit season plans") {
		return
	}
	var body phaseBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	before, err := h.service.GetPhase(orgID, chi.URLParam(r, "phaseID"))
	if err != nil {
		writeTrainingError(w, r, err, "Failed to update training phase.")
		return
	}
	season, ok := h.seasonWindow(w, r, orgID, before.SeasonID)
	if !ok {
		return
	}
	in, ok := h.phaseInput(w, r, orgID, body)
	if !ok {
		return
	}
	phase, err := h.service.UpdatePhase(orgID, before.ID, season, in, principal.UserID)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to update training phase.")
		return
	}
	h.audit(principal, orgID, "training_phase.update", "training_phase", phase.ID, before, phase, map[string]string{"season_id": season.ID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"phase": phase})
}

// deleteTrainingPhase handles DELETE /api/v1/training-phases/{phaseID}
func (h *TrainingHandlers) deleteTrainingPhase(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "edit season plans") {
		return
	}
	id := chi.URLParam(r, "phaseID")
	before, err := h.service.GetPhase(orgID, id)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to delete training phase.")
		return
	}
	if err := h.service.DeletePhase(orgID, id); err != nil {
		writeTrainingError(w, r, err, "Failed to delete training phase.")
		return
	}
	h.audit(principal, orgID, "training_phase.delete", "training_phase", id, before, nil, map[string]string{"season_id": before.SeasonID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// seasonTrainingPlan handles GET /api/v1/seasons/{id}/training-plan
func (h *TrainingHandlers) seasonTrainingPlan(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok || !requireStaff(w, r, principal, "view season plans") {
		return
	}
	season, ok := h.seasonWindow(w, r, orgID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	canon, _ := h.groupIndex(orgID)
	groupFilter := canon(r.URL.Query().Get("group_id")) // canon trims; empty stays empty
	plan, err := h.service.SeasonPlan(orgID, season, groupFilter, canon)
	if err != nil {
		writeTrainingError(w, r, err, "Failed to build season plan.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": plan})
}
