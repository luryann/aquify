package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/go-chi/chi/v5"
)

// Team records, top-N lists, swimmer goals, relay builder and progress
// reports. Routes are mounted from PerformanceHandlers.RegisterRoutes.

// SetCommsService wires the communications service used to create
// "new team record" announcement drafts.
func (h *PerformanceHandlers) SetCommsService(svc *comms.Service) {
	h.commsService = svc
}

func (h *PerformanceHandlers) registerExtensionRoutes(router chi.Router) {
	// Team records
	router.Get("/performance/records", h.listRecords)
	router.Post("/performance/records", h.createManualRecord)
	router.Get("/performance/records/history", h.getRecordHistory)
	router.Get("/performance/records/pending", h.listPendingRecords)
	router.Post("/performance/records/rebuild", h.rebuildRecords)
	router.Patch("/performance/records/{entryID}", h.updateRecordEntry)
	router.Delete("/performance/records/{entryID}", h.deleteRecordEntry)
	router.Post("/performance/records/{entryID}/approve", h.approveRecordEntry)
	router.Post("/performance/records/{entryID}/reject", h.rejectRecordEntry)
	router.Post("/performance/records/{entryID}/announcement", h.draftRecordAnnouncement)

	// Top-N lists
	router.Get("/performance/top-times", h.listTopTimes)

	// Goals
	router.Get("/performance/goals", h.listOrgGoals)
	router.Get("/swimmers/{swimmerID}/goals", h.listGoals)
	router.Post("/swimmers/{swimmerID}/goals", h.createGoal)
	router.Patch("/swimmers/{swimmerID}/goals/{goalID}", h.updateGoal)
	router.Delete("/swimmers/{swimmerID}/goals/{goalID}", h.deleteGoal)

	// Relay builder
	router.Post("/performance/relays/optimize", h.optimizeRelay)
	router.Get("/performance/relay-lineups", h.listRelayLineups)
	router.Post("/performance/relay-lineups", h.createRelayLineup)
	router.Put("/performance/relay-lineups/{lineupID}", h.replaceRelayLineup)
	router.Delete("/performance/relay-lineups/{lineupID}", h.deleteRelayLineup)

	// Progress report
	router.Get("/performance/progress-report", h.progressReport)
}

// writePerfExtError maps performance-extension sentinel errors to HTTP.
func writePerfExtError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, performance.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role for this performance action.", nil)
	case errors.Is(err, performance.ErrRecordNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "record_not_found", "Record entry not found.", nil)
	case errors.Is(err, performance.ErrRecordNotPending):
		httpx.WriteError(w, r, http.StatusConflict, "record_not_pending", err.Error(), nil)
	case errors.Is(err, performance.ErrGoalNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "goal_not_found", err.Error(), nil)
	case errors.Is(err, performance.ErrRelayLineupNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "relay_lineup_not_found", "Relay lineup not found.", nil)
	case errors.Is(err, performance.ErrRecordInvalid), errors.Is(err, performance.ErrGoalInvalid), errors.Is(err, performance.ErrRelayInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
	}
}

func (h *PerformanceHandlers) audit(principal *appmiddleware.Principal, orgID, action, resourceType, resourceID string, before, after any, meta map[string]string) {
	if h.auditService == nil || principal == nil {
		return
	}
	_, _ = h.auditService.Append(audit.AppendRequest{
		OrgID:        orgID,
		ActorUserID:  principal.UserID,
		ActorRoles:   principal.Roles,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       before,
		After:        after,
		Metadata:     meta,
	})
}

func queryInt(r *http.Request, name string) int {
	v, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(name)))
	if err != nil {
		return 0
	}
	return v
}

// ─── Records ─────────────────────────────────────────────────────────────────

func (h *PerformanceHandlers) listRecords(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	records, err := h.service.ListRecords(orgID, principal, performance.ListRecordsFilters{
		Course:   strings.ToUpper(q.Get("course")),
		Gender:   strings.ToUpper(q.Get("gender")),
		AgeGroup: strings.ToUpper(q.Get("age_group")),
		Stroke:   strings.ToLower(q.Get("stroke")),
		Distance: queryInt(r, "distance"),
	})
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":      len(records),
		"records":    records,
		"age_groups": performance.AgeGroups,
	})
}

func (h *PerformanceHandlers) getRecordHistory(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	history, err := h.service.GetRecordHistory(orgID, principal, performance.RecordKey{
		Course:   strings.ToUpper(q.Get("course")),
		Gender:   strings.ToUpper(q.Get("gender")),
		AgeGroup: strings.ToUpper(q.Get("age_group")),
		Distance: queryInt(r, "distance"),
		Stroke:   strings.ToLower(q.Get("stroke")),
	})
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"history": history})
}

func (h *PerformanceHandlers) listPendingRecords(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	pending, err := h.service.ListPendingRecords(orgID, principal)
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(pending), "entries": pending})
}

type createManualRecordBody struct {
	Course      string `json:"course"       validate:"required,oneof=SCY SCM LCM"`
	Gender      string `json:"gender"       validate:"required,oneof=M F"`
	AgeGroup    string `json:"age_group"    validate:"required"`
	Distance    int    `json:"distance"     validate:"required,gt=0"`
	Stroke      string `json:"stroke"       validate:"required,oneof=free back breast fly im"`
	TimeMs      int    `json:"time_ms"      validate:"required,gt=0"`
	SwimmerID   string `json:"swimmer_id"`
	SwimmerName string `json:"swimmer_name" validate:"max=120"`
	SwimmerAge  *int   `json:"swimmer_age"`
	SwimDate    string `json:"swim_date"    validate:"required"`
	MeetName    string `json:"meet_name"    validate:"max=200"`
	Notes       string `json:"notes"        validate:"max=1000"`
	Locked      bool   `json:"locked"`
	Verified    bool   `json:"verified"`
}

func (h *PerformanceHandlers) createManualRecord(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body createManualRecordBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}
	entry, err := h.service.CreateManualRecord(orgID, principal.UserID, principal, performance.CreateManualRecordInput(body))
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	h.audit(principal, orgID, "performance.team_record.manual_created", "team_record_entry", entry.ID, nil, entry, nil)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"entry": entry})
}

type updateRecordEntryBody struct {
	Locked      *bool   `json:"locked"`
	Verified    *bool   `json:"verified"`
	Notes       *string `json:"notes"`
	SwimmerName *string `json:"swimmer_name"`
	SwimDate    *string `json:"swim_date"`
	MeetName    *string `json:"meet_name"`
	TimeMs      *int    `json:"time_ms"`
}

func (h *PerformanceHandlers) updateRecordEntry(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body updateRecordEntryBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}
	entryID := chi.URLParam(r, "entryID")
	before, after, err := h.service.UpdateRecordEntry(orgID, principal.UserID, entryID, principal, performance.UpdateRecordEntryInput(body))
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	h.audit(principal, orgID, "performance.team_record.updated", "team_record_entry", entryID, before, after, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entry": after})
}

type decideRecordBody struct {
	Notes string `json:"notes" validate:"max=1000"`
}

func (h *PerformanceHandlers) decideRecord(w http.ResponseWriter, r *http.Request, approve bool) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body decideRecordBody
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request_body", err.Error(), nil)
		return
	}
	entryID := chi.URLParam(r, "entryID")
	entry, err := h.service.DecideRecordEntry(orgID, principal.UserID, entryID, principal, approve, body.Notes)
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	action := "performance.team_record.rejected"
	if approve {
		action = "performance.team_record.approved"
	}
	h.audit(principal, orgID, action, "team_record_entry", entryID, nil, entry, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entry": entry})
}

func (h *PerformanceHandlers) approveRecordEntry(w http.ResponseWriter, r *http.Request) {
	h.decideRecord(w, r, true)
}

func (h *PerformanceHandlers) rejectRecordEntry(w http.ResponseWriter, r *http.Request) {
	h.decideRecord(w, r, false)
}

func (h *PerformanceHandlers) deleteRecordEntry(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	entryID := chi.URLParam(r, "entryID")
	entry, err := h.service.DeleteManualRecord(orgID, principal.UserID, entryID, principal)
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	h.audit(principal, orgID, "performance.team_record.deleted", "team_record_entry", entryID, entry, nil, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": entryID})
}

func (h *PerformanceHandlers) rebuildRecords(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	summary, err := h.service.RebuildRecords(orgID, principal.UserID, principal)
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	h.audit(principal, orgID, "performance.team_record.rebuilt", "team_record", orgID, nil, summary, map[string]string{
		"entries_created": strconv.Itoa(summary.EntriesCreated),
		"entries_removed": strconv.Itoa(summary.EntriesRemoved),
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"summary": summary})
}

func (h *PerformanceHandlers) draftRecordAnnouncement(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	if h.commsService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Announcements are not configured.", nil)
		return
	}
	entryID := chi.URLParam(r, "entryID")
	title, body, err := h.service.RecordAnnouncementDraft(orgID, principal, entryID)
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	if len(title) > 255 {
		title = title[:255]
	}
	announcement, err := h.commsService.CreateAnnouncement(comms.CreateAnnouncementInput{
		OrgID: orgID, Title: title, Body: body, Audience: "all", CreatedBy: principal.UserID,
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "announcement_failed", err.Error(), nil)
		return
	}
	h.audit(principal, orgID, "performance.team_record.announcement_drafted", "announcement", announcement.ID, nil, announcement, map[string]string{"record_entry_id": entryID})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"announcement": announcement})
}

// ─── Top times ───────────────────────────────────────────────────────────────

func (h *PerformanceHandlers) listTopTimes(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	lists, err := h.service.TopTimes(orgID, principal, performance.TopTimesQuery{
		Course:   q.Get("course"),
		Gender:   q.Get("gender"),
		AgeGroup: q.Get("age_group"),
		Distance: queryInt(r, "distance"),
		Stroke:   q.Get("stroke"),
		From:     strings.TrimSpace(q.Get("from")),
		To:       strings.TrimSpace(q.Get("to")),
		Limit:    queryInt(r, "limit"),
	})
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(lists), "lists": lists})
}

// ─── Goals ───────────────────────────────────────────────────────────────────

func (h *PerformanceHandlers) listOrgGoals(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	goals, err := h.service.ListOrgGoals(orgID, principal, strings.TrimSpace(r.URL.Query().Get("swimmer_id")))
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	page, total := paginateSlice(goals, parseListPagination(r, listPaginationOptions{}))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(page), "total": total, "goals": page})
}

func (h *PerformanceHandlers) listGoals(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	goals, err := h.service.ListGoals(orgID, principal, swimmerID, includeArchived)
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"swimmer_id": swimmerID, "count": len(goals), "goals": goals})
}

type createGoalBody struct {
	Distance     int    `json:"distance"       validate:"required,gt=0"`
	Stroke       string `json:"stroke"         validate:"required,oneof=free back breast fly im"`
	Course       string `json:"course"         validate:"required,oneof=SCY SCM LCM"`
	TargetTimeMs int    `json:"target_time_ms" validate:"required,gt=0"`
	Deadline     string `json:"deadline"`
	Notes        string `json:"notes"          validate:"max=500"`
}

func (h *PerformanceHandlers) createGoal(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body createGoalBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")
	goal, err := h.service.CreateGoal(orgID, principal.UserID, principal, swimmerID, performance.GoalInput(body))
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	h.audit(principal, orgID, "performance.goal.created", "swimmer_goal", goal.ID, nil, goal.Goal, map[string]string{"swimmer_id": swimmerID})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"goal": goal})
}

type updateGoalBody struct {
	TargetTimeMs *int    `json:"target_time_ms"`
	Deadline     *string `json:"deadline"`
	Notes        *string `json:"notes"`
	Status       *string `json:"status"`
}

func (h *PerformanceHandlers) updateGoal(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body updateGoalBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}
	swimmerID, goalID := chi.URLParam(r, "swimmerID"), chi.URLParam(r, "goalID")
	goal, err := h.service.UpdateGoal(orgID, principal.UserID, principal, swimmerID, goalID, performance.UpdateGoalInput(body))
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	h.audit(principal, orgID, "performance.goal.updated", "swimmer_goal", goalID, nil, goal.Goal, map[string]string{"swimmer_id": swimmerID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"goal": goal})
}

func (h *PerformanceHandlers) deleteGoal(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID, goalID := chi.URLParam(r, "swimmerID"), chi.URLParam(r, "goalID")
	if err := h.service.DeleteGoal(orgID, principal.UserID, principal, swimmerID, goalID); err != nil {
		writePerfExtError(w, r, err)
		return
	}
	h.audit(principal, orgID, "performance.goal.deleted", "swimmer_goal", goalID, nil, nil, map[string]string{"swimmer_id": swimmerID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": goalID})
}

// ─── Relays ──────────────────────────────────────────────────────────────────

type relayLockBody struct {
	Team      int    `json:"team"       validate:"gte=0"`
	Leg       int    `json:"leg"        validate:"gte=1,lte=4"`
	SwimmerID string `json:"swimmer_id" validate:"required"`
}

type optimizeRelayBody struct {
	MeetID             string          `json:"meet_id"`
	Course             string          `json:"course"`
	Gender             string          `json:"gender"      validate:"required,oneof=M F X"`
	AgeGroup           string          `json:"age_group"`
	RelayType          string          `json:"relay_type"  validate:"required,oneof=free medley"`
	Distance           int             `json:"distance"    validate:"required,oneof=200 400 800"`
	Teams              int             `json:"teams"       validate:"gte=0,lte=6"`
	AsOf               string          `json:"as_of"`
	GroupCode          string          `json:"group_code"`
	Locks              []relayLockBody `json:"locks"       validate:"max=24,dive"`
	ExcludedSwimmerIDs []string        `json:"excluded_swimmer_ids" validate:"max=500"`
	IncludeSwimmerIDs  []string        `json:"include_swimmer_ids"  validate:"max=500"`
}

func (h *PerformanceHandlers) optimizeRelay(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body optimizeRelayBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}
	locks := make([]performance.RelayLegLock, 0, len(body.Locks))
	for _, l := range body.Locks {
		locks = append(locks, performance.RelayLegLock(l))
	}
	result, err := h.service.OptimizeRelay(orgID, principal, performance.RelayOptimizeInput{
		MeetID: strings.TrimSpace(body.MeetID), Course: body.Course, Gender: body.Gender, AgeGroup: body.AgeGroup,
		RelayType: body.RelayType, Distance: body.Distance, Teams: body.Teams, AsOf: body.AsOf, GroupCode: body.GroupCode,
		Locks: locks, ExcludedSwimmerIDs: body.ExcludedSwimmerIDs, IncludeSwimmerIDs: body.IncludeSwimmerIDs,
	})
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"proposal": result})
}

func (h *PerformanceHandlers) listRelayLineups(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	lineups, err := h.service.ListRelayLineups(orgID, principal, strings.TrimSpace(r.URL.Query().Get("meet_id")))
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	page, total := paginateSlice(lineups, parseListPagination(r, listPaginationOptions{}))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"count": len(page), "total": total, "lineups": page})
}

type relayLineupBody struct {
	MeetID    string                  `json:"meet_id"    validate:"required"`
	Name      string                  `json:"name"       validate:"max=120"`
	Course    string                  `json:"course"     validate:"required,oneof=SCY SCM LCM"`
	Gender    string                  `json:"gender"     validate:"required,oneof=M F X"`
	AgeGroup  string                  `json:"age_group"  validate:"required"`
	RelayType string                  `json:"relay_type" validate:"required,oneof=free medley"`
	Distance  int                     `json:"distance"   validate:"required,oneof=200 400 800"`
	Teams     []performance.RelayTeam `json:"teams"      validate:"required,min=1,max=6"`
	Notes     string                  `json:"notes"      validate:"max=1000"`
}

func (h *PerformanceHandlers) saveRelayLineup(w http.ResponseWriter, r *http.Request, lineupID string) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body relayLineupBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}
	lineup, err := h.service.SaveRelayLineup(orgID, principal.UserID, principal, lineupID, performance.RelayLineupInput(body))
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	status := http.StatusCreated
	if lineupID != "" {
		status = http.StatusOK
	}
	h.audit(principal, orgID, "performance.relay_lineup.saved", "relay_lineup", lineup.ID, nil, lineup, map[string]string{"meet_id": lineup.MeetID})
	httpx.WriteJSON(w, status, map[string]any{"lineup": lineup})
}

func (h *PerformanceHandlers) createRelayLineup(w http.ResponseWriter, r *http.Request) {
	h.saveRelayLineup(w, r, "")
}

func (h *PerformanceHandlers) replaceRelayLineup(w http.ResponseWriter, r *http.Request) {
	h.saveRelayLineup(w, r, chi.URLParam(r, "lineupID"))
}

func (h *PerformanceHandlers) deleteRelayLineup(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	lineupID := chi.URLParam(r, "lineupID")
	if err := h.service.DeleteRelayLineup(orgID, principal.UserID, principal, lineupID); err != nil {
		writePerfExtError(w, r, err)
		return
	}
	h.audit(principal, orgID, "performance.relay_lineup.deleted", "relay_lineup", lineupID, nil, nil, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": lineupID})
}

// ─── Progress report ─────────────────────────────────────────────────────────

func (h *PerformanceHandlers) progressReport(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	report, err := h.service.ProgressReport(orgID, principal, performance.ProgressReportInput{
		FromA: q.Get("from_a"), ToA: q.Get("to_a"), FromB: q.Get("from_b"), ToB: q.Get("to_b"),
		GroupCode: q.Get("group_code"), SwimmerID: strings.TrimSpace(q.Get("swimmer_id")), Course: q.Get("course"),
		Limit: queryInt(r, "limit"),
	})
	if err != nil {
		writePerfExtError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": report})
}

// recordBreaksOrEmpty normalizes nil slices for JSON.
func recordBreaksOrEmpty(in []performance.RecordBreak) []performance.RecordBreak {
	if in == nil {
		return []performance.RecordBreak{}
	}
	return in
}
