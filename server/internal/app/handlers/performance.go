package handlers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// PerformanceHandlers handles HTTP requests for the performance/best-times domain.
type PerformanceHandlers struct {
	service          *performance.Service
	membersService   *members.Service
	groupsService    *groups.Service
	meetsService     *meets.Service
	documentsService *documents.Service
	auditService     *audit.Service
	commsService     *comms.Service
	validate         *validator.Validate
}

// NewPerformanceHandlers returns a new PerformanceHandlers.
func NewPerformanceHandlers(svc *performance.Service) *PerformanceHandlers {
	return &PerformanceHandlers{
		service:  svc,
		validate: validator.New(),
	}
}

// NewPerformanceHandlersWithAudit returns a PerformanceHandlers with audit logging enabled.
func NewPerformanceHandlersWithAudit(svc *performance.Service, auditSvc *audit.Service) *PerformanceHandlers {
	return NewPerformanceHandlersWithAuditAndContext(svc, nil, nil, nil, nil, auditSvc)
}

// NewPerformanceHandlersWithAuditAndContext returns a PerformanceHandlers with
// the additional domain services needed to synthesize swimmer activity feeds.
func NewPerformanceHandlersWithAuditAndContext(
	svc *performance.Service,
	membersSvc *members.Service,
	groupsSvc *groups.Service,
	meetsSvc *meets.Service,
	documentsSvc *documents.Service,
	auditSvc *audit.Service,
) *PerformanceHandlers {
	return &PerformanceHandlers{
		service:          svc,
		membersService:   membersSvc,
		groupsService:    groupsSvc,
		meetsService:     meetsSvc,
		documentsService: documentsSvc,
		auditService:     auditSvc,
		validate:         validator.New(),
	}
}

// RegisterRoutes mounts all performance routes on the given router.
func (h *PerformanceHandlers) RegisterRoutes(router chi.Router) {
	// Results
	router.Post("/results", h.upsertResult)
	router.Post("/meets/{meetID}/results/import", h.importMeetResults)
	router.Get("/performance/best-times", h.listOrgBestTimes)
	router.Get("/performance/progression", h.listOrgProgression)

	// Per-swimmer performance
	router.Get("/swimmers/{swimmerID}/best-times", h.listBestTimes)
	router.Post("/swimmers/{swimmerID}/best-times/override", h.overrideBestTime)
	router.Get("/swimmers/{swimmerID}/progression", h.listProgression)
	router.Get("/swimmers/{swimmerID}/activity", h.listActivity)
	router.Get("/swimmers/{swimmerID}/cut-qualification", h.evaluateCutQualification)

	// Time standards management
	router.Get("/time-standards", h.listTimeStandards)
	router.Post("/time-standards", h.createTimeStandard)
	router.Get("/time-standards/{standardID}", h.getTimeStandard)
	router.Put("/time-standards/{standardID}", h.updateTimeStandard)
	router.Delete("/time-standards/{standardID}", h.deleteTimeStandard)

	// Team records, top times, goals, relays, progress reports
	h.registerExtensionRoutes(router)
}

// ─── Scope helper ─────────────────────────────────────────────────────────────

func perfScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}

func (h *PerformanceHandlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeAndValidateJSON(w, r, dst, h.validate, decodeAndValidateOptions{
		InvalidStatus:     http.StatusBadRequest,
		InvalidCode:       "invalid_request_body",
		ValidationStatus:  http.StatusUnprocessableEntity,
		ValidationCode:    "validation_error",
		ValidationMessage: "Request validation failed.",
	})
}

// ─── Request body types ───────────────────────────────────────────────────────

type splitInputBody struct {
	SplitNumber      int `json:"split_number"       validate:"required,gt=0"`
	SplitDistance    int `json:"split_distance"     validate:"required,gt=0"`
	SplitTimeMs      int `json:"split_time_ms"      validate:"gte=0"`
	CumulativeTimeMs int `json:"cumulative_time_ms" validate:"gte=0"`
}

type upsertResultBody struct {
	SwimmerID   string           `json:"swimmer_id"   validate:"required"`
	MeetID      string           `json:"meet_id"`
	MeetEventID string           `json:"meet_event_id"`
	ImportID    string           `json:"import_id"`
	EventNumber string           `json:"event_number"`
	Distance    int              `json:"distance"     validate:"required,gt=0"`
	Stroke      string           `json:"stroke"       validate:"required,oneof=free back breast fly im free_relay medley_relay"`
	Course      string           `json:"course"       validate:"required,oneof=SCY SCM LCM"`
	Gender      string           `json:"gender"       validate:"required,oneof=M F X"`
	Heat        *int             `json:"heat"`
	Lane        *int             `json:"lane"`
	Place       *int             `json:"place"`
	SeedTimeMs  *int             `json:"seed_time_ms"`
	FinTimeMs   *int             `json:"fin_time_ms"`
	DQ          bool             `json:"dq"`
	DQCode      string           `json:"dq_code"`
	DQNote      string           `json:"dq_note"`
	IsOfficial  bool             `json:"is_official"`
	Splits      []splitInputBody `json:"splits"`
}

type overrideBestTimeBody struct {
	EventDistance int    `json:"event_distance" validate:"required,gt=0"`
	Stroke        string `json:"stroke"         validate:"required,oneof=free back breast fly im"`
	Course        string `json:"course"         validate:"required,oneof=SCY SCM LCM"`
	BestTimeMs    *int   `json:"best_time_ms"`
	Reason        string `json:"reason"         validate:"required"`
	RecordedAt    string `json:"recorded_at"`
}

type createTimeStandardBody struct {
	Name          string `json:"name"           validate:"required"`
	SeasonYear    int    `json:"season_year"    validate:"required,gt=2000"`
	StandardCode  string `json:"standard_code"  validate:"required"`
	Description   string `json:"description"`
	EventDistance int    `json:"event_distance" validate:"required,gt=0"`
	Stroke        string `json:"stroke"         validate:"required,oneof=free back breast fly im"`
	Course        string `json:"course"         validate:"required,oneof=SCY SCM LCM"`
	Gender        string `json:"gender"         validate:"required,oneof=M F X"`
	AgeMin        *int   `json:"age_min"`
	AgeMax        *int   `json:"age_max"`
	TimeMs        int    `json:"time_ms"        validate:"required,gt=0"`
	IsActive      bool   `json:"is_active"`
}

type updateTimeStandardBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	TimeMs      *int    `json:"time_ms"`
	IsActive    *bool   `json:"is_active"`
}

type swimmerActivityRecord struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail,omitempty"`
	Status     string    `json:"status,omitempty"`
	SourceID   string    `json:"source_id,omitempty"`
	MeetID     string    `json:"meet_id,omitempty"`
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// upsertResult POST /results
func (h *PerformanceHandlers) upsertResult(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body upsertResultBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}

	splits := make([]performance.SplitInput, 0, len(body.Splits))
	for _, sp := range body.Splits {
		splits = append(splits, performance.SplitInput{
			SplitNumber:      sp.SplitNumber,
			SplitDistance:    sp.SplitDistance,
			SplitTimeMs:      sp.SplitTimeMs,
			CumulativeTimeMs: sp.CumulativeTimeMs,
		})
	}

	result, breaks, err := h.service.UpsertResultWithRecords(orgID, principal.UserID, principal, performance.UpsertResultInput{
		SwimmerID:   body.SwimmerID,
		MeetID:      body.MeetID,
		MeetEventID: body.MeetEventID,
		ImportID:    body.ImportID,
		EventNumber: body.EventNumber,
		Distance:    body.Distance,
		Stroke:      body.Stroke,
		Course:      body.Course,
		Gender:      body.Gender,
		Heat:        body.Heat,
		Lane:        body.Lane,
		Place:       body.Place,
		SeedTimeMs:  body.SeedTimeMs,
		FinTimeMs:   body.FinTimeMs,
		DQ:          body.DQ,
		DQCode:      body.DQCode,
		DQNote:      body.DQNote,
		IsOfficial:  body.IsOfficial,
		Splits:      splits,
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to upsert results.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"result": result, "new_team_records": recordBreaksOrEmpty(breaks)})
}

// listBestTimes GET /swimmers/{swimmerID}/best-times
// Allowed roles: admin, coach, parent (parent household scoping deferred to DB layer).
func (h *PerformanceHandlers) listBestTimes(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")

	filters := performance.ListBestTimesFilters{
		Course: r.URL.Query().Get("course"),
		Stroke: r.URL.Query().Get("stroke"),
	}

	bestTimes, err := h.service.ListBestTimes(orgID, principal, swimmerID, filters)
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read best times.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"swimmer_id": swimmerID,
		"count":      len(bestTimes),
		"best_times": bestTimes,
	})
}

// listOrgBestTimes GET /performance/best-times
func (h *PerformanceHandlers) listOrgBestTimes(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}

	bestTimes, err := h.service.ListBestTimesForOrg(orgID, principal, performance.ListBestTimesFilters{
		SwimmerID: r.URL.Query().Get("swimmer_id"),
		Course:    r.URL.Query().Get("course"),
		Stroke:    r.URL.Query().Get("stroke"),
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read best times.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":      len(bestTimes),
		"best_times": bestTimes,
	})
}

// overrideBestTime POST /swimmers/{swimmerID}/best-times/override
func (h *PerformanceHandlers) overrideBestTime(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")

	var body overrideBestTimeBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}

	bt, internalAudit, err := h.service.OverrideBestTime(orgID, principal.UserID, principal, performance.OverrideBestTimeInput{
		SwimmerID:     swimmerID,
		EventDistance: body.EventDistance,
		Stroke:        body.Stroke,
		Course:        body.Course,
		BestTimeMs:    body.BestTimeMs,
		Reason:        body.Reason,
		RecordedAt:    body.RecordedAt,
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may override best times.", nil)
			return
		}
		if errors.Is(err, performance.ErrOverrideRequiresReason) {
			// 422 Unprocessable Entity: request is well-formed but fails semantic validation.
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "override_requires_reason", err.Error(), nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	if h.auditService != nil {
		meta := map[string]string{"swimmer_id": swimmerID}
		if body.Reason != "" {
			meta["reason"] = body.Reason
		}
		_, _ = h.auditService.Append(audit.AppendRequest{
			OrgID:        orgID,
			ActorUserID:  principal.UserID,
			ActorRoles:   principal.Roles,
			Action:       audit.ActionPerformanceBestTimeOverride,
			ResourceType: "best_time",
			ResourceID:   bt.ID,
			After:        bt,
			Metadata:     meta,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"best_time":   bt,
		"audit_event": internalAudit,
	})
}

// listProgression GET /swimmers/{swimmerID}/progression
// Allowed roles: admin, coach, parent (parent household scoping deferred to DB layer).
func (h *PerformanceHandlers) listProgression(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")

	q := r.URL.Query()
	distanceStr := q.Get("distance")
	var distance int
	if distanceStr != "" {
		d, err := strconv.Atoi(distanceStr)
		if err == nil {
			distance = d
		}
	}

	results, err := h.service.ListProgression(orgID, principal, swimmerID, performance.ListResultsFilters{
		EventDistance: distance,
		Stroke:        q.Get("stroke"),
		Course:        q.Get("course"),
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read progression data.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"swimmer_id": swimmerID,
		"count":      len(results),
		"results":    results,
	})
}

// listActivity GET /swimmers/{swimmerID}/activity
func (h *PerformanceHandlers) listActivity(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	if h.membersService == nil || h.groupsService == nil || h.meetsService == nil || h.documentsService == nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "activity_unavailable", "Swimmer activity service is not configured.", nil)
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")

	swimmers, err := h.membersService.ListSwimmers(principal, orgID, members.ListSwimmersInput{})
	if err != nil {
		if errors.Is(err, members.ErrForbidden) || errors.Is(err, members.ErrParentHouseholdScope) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read swimmer activity.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	visible := false
	for _, swimmer := range swimmers {
		if swimmer.ID == swimmerID {
			visible = true
			break
		}
	}
	if !visible {
		httpx.WriteError(w, r, http.StatusNotFound, "swimmer_not_found", "Swimmer was not found.", nil)
		return
	}

	activities, err := h.buildActivityFeed(orgID, principal, swimmerID)
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) || errors.Is(err, meets.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read swimmer activity.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"swimmer_id": swimmerID,
		"count":      len(activities),
		"activities": activities,
	})
}

// listOrgProgression GET /performance/progression
func (h *PerformanceHandlers) listOrgProgression(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	distanceStr := q.Get("distance")
	var distance int
	if distanceStr != "" {
		d, err := strconv.Atoi(distanceStr)
		if err == nil {
			distance = d
		}
	}

	results, err := h.service.ListProgressionForOrg(orgID, principal, performance.ListResultsFilters{
		SwimmerID:     q.Get("swimmer_id"),
		EventDistance: distance,
		Stroke:        q.Get("stroke"),
		Course:        q.Get("course"),
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read progression data.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":   len(results),
		"results": results,
	})
}

func (h *PerformanceHandlers) buildActivityFeed(orgID string, principal *appmiddleware.Principal, swimmerID string) ([]swimmerActivityRecord, error) {
	activities := make([]swimmerActivityRecord, 0)

	results, err := h.service.ListProgression(orgID, principal, swimmerID, performance.ListResultsFilters{})
	if err != nil {
		return nil, err
	}
	for _, result := range results {
		detail := "Recorded progression result."
		if result.FinTimeMs != nil {
			detail = "Finished in " + formatActivityTime(*result.FinTimeMs) + "."
		} else if result.DQ {
			detail = "Marked as disqualified."
		} else if strings.TrimSpace(result.OverrideReason) != "" {
			detail = strings.TrimSpace(result.OverrideReason)
		}
		activities = append(activities, swimmerActivityRecord{
			ID:         "result:" + result.ID,
			Type:       "result",
			OccurredAt: result.UpdatedAt,
			Title:      strconv.Itoa(result.Distance) + " " + strings.ToUpper(result.Course) + " " + strokeActivityLabel(result.Stroke),
			Detail:     detail,
			Status:     result.Status,
			SourceID:   result.ID,
			MeetID:     result.MeetID,
		})
	}

	registrations := h.groupsService.SnapshotRegistrations(orgID)
	registrationByID := make(map[string]groups.Registration, len(registrations))
	for _, registration := range registrations {
		if registration.SwimmerID != swimmerID {
			continue
		}
		registrationByID[registration.ID] = registration
		activities = append(activities, swimmerActivityRecord{
			ID:         "registration:" + registration.ID,
			Type:       "registration",
			OccurredAt: registration.UpdatedAt,
			Title:      "Registration " + strings.ReplaceAll(registration.Status, "_", " "),
			Detail:     "Season " + registration.Season + " registration recorded.",
			Status:     registration.Status,
			SourceID:   registration.ID,
		})
	}

	for _, waiver := range h.groupsService.SnapshotWaivers(orgID) {
		registration, ok := registrationByID[waiver.RegistrationID]
		if !ok {
			continue
		}
		activities = append(activities, swimmerActivityRecord{
			ID:         "waiver:" + waiver.ID,
			Type:       "waiver",
			OccurredAt: waiver.AcceptedAt,
			Title:      "Waiver accepted",
			Detail:     strings.TrimSpace(waiver.WaiverType) + " waiver accepted for " + registration.Season + ".",
			Status:     "accepted",
			SourceID:   waiver.ID,
		})
	}

	householdID, ok := h.membersService.SwimmerHouseholdID(orgID, swimmerID)
	if !ok {
		return activities, nil
	}
	submissions, err := h.documentsService.ListSubmissions(orgID, householdID)
	if err != nil {
		return nil, err
	}
	for _, submission := range submissions {
		if strings.TrimSpace(submission.SwimmerID) != swimmerID {
			continue
		}
		occurredAt := submission.UpdatedAt
		if submission.SubmittedAt != nil {
			occurredAt = *submission.SubmittedAt
		}
		if submission.VerifiedAt != nil {
			occurredAt = *submission.VerifiedAt
		}
		detail := "Document record updated."
		if strings.TrimSpace(submission.DocumentTypeName) != "" {
			detail = submission.DocumentTypeName + " document " + strings.ReplaceAll(submission.Status, "_", " ") + "."
		}
		activities = append(activities, swimmerActivityRecord{
			ID:         "document:" + submission.ID,
			Type:       "document",
			OccurredAt: occurredAt,
			Title:      "Document " + strings.ReplaceAll(submission.Status, "_", " "),
			Detail:     detail,
			Status:     submission.Status,
			SourceID:   submission.ID,
		})
	}

	for _, meet := range h.meetsService.SnapshotMeets(orgID) {
		commitments, err := h.meetsService.ListCommitments(principal, orgID, meet.ID)
		if err != nil {
			if errors.Is(err, meets.ErrMeetNotFound) {
				continue
			}
			return nil, err
		}
		for _, commitment := range commitments {
			if commitment.SwimmerID != swimmerID {
				continue
			}
			meetName := meet.Name
			if strings.TrimSpace(meetName) == "" {
				meetName = "meet"
			}
			activities = append(activities, swimmerActivityRecord{
				ID:         "commitment:" + commitment.ID,
				Type:       "commitment",
				OccurredAt: commitment.UpdatedAt,
				Title:      "Meet commitment " + strings.ReplaceAll(commitment.Status, "_", " "),
				Detail:     "Response recorded for " + meetName + ".",
				Status:     commitment.Status,
				SourceID:   commitment.ID,
				MeetID:     commitment.MeetID,
			})
		}
	}

	sort.SliceStable(activities, func(i, j int) bool {
		if activities[i].OccurredAt.Equal(activities[j].OccurredAt) {
			return activities[i].ID > activities[j].ID
		}
		return activities[i].OccurredAt.After(activities[j].OccurredAt)
	})
	return activities, nil
}

func strokeActivityLabel(stroke string) string {
	switch strings.ToLower(strings.TrimSpace(stroke)) {
	case "free", "freestyle":
		return "Freestyle"
	case "back", "backstroke":
		return "Backstroke"
	case "breast", "breaststroke":
		return "Breaststroke"
	case "fly", "butterfly":
		return "Butterfly"
	case "im", "individual_medley":
		return "IM"
	default:
		return stroke
	}
}

func formatActivityTime(ms int) string {
	totalHundredths := ms / 10
	minutes := totalHundredths / 6000
	seconds := (totalHundredths % 6000) / 100
	hundredths := totalHundredths % 100
	if minutes > 0 {
		return strconv.Itoa(minutes) + ":" + strconv.Itoa(seconds/10) + strconv.Itoa(seconds%10) + "." + strconv.Itoa(hundredths/10) + strconv.Itoa(hundredths%10)
	}
	return strconv.Itoa(seconds/10) + strconv.Itoa(seconds%10) + "." + strconv.Itoa(hundredths/10) + strconv.Itoa(hundredths%10)
}

// evaluateCutQualification GET /swimmers/{swimmerID}/cut-qualification?standard_id=...
// Allowed roles: admin, coach, parent (parent household scoping deferred to DB layer).
func (h *PerformanceHandlers) evaluateCutQualification(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	swimmerID := chi.URLParam(r, "swimmerID")
	standardID := r.URL.Query().Get("standard_id")
	if strings.TrimSpace(standardID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_standard_id", "Query parameter 'standard_id' is required.", nil)
		return
	}

	qual, err := h.service.EvaluateCutQualification(orgID, principal, swimmerID, standardID)
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to read cut qualification.", nil)
			return
		}
		if errors.Is(err, performance.ErrStandardNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "standard_not_found", "Time standard not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"qualification": qual})
}

// listTimeStandards GET /time-standards
func (h *PerformanceHandlers) listTimeStandards(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	var yearFilter int
	if y := q.Get("season_year"); y != "" {
		if n, err := strconv.Atoi(y); err == nil {
			yearFilter = n
		}
	}
	var distFilter int
	if d := q.Get("event_distance"); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			distFilter = n
		}
	}
	activeOnly := q.Get("active_only") == "true" || q.Get("active_only") == "1"

	standards, err := h.service.ListTimeStandards(orgID, performance.ListTimeStandardsFilters{
		SeasonYear:    yearFilter,
		StandardCode:  q.Get("standard_code"),
		EventDistance: distFilter,
		Stroke:        q.Get("stroke"),
		Course:        q.Get("course"),
		Gender:        q.Get("gender"),
		ActiveOnly:    activeOnly,
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"count":          len(standards),
		"time_standards": standards,
	})
}

// createTimeStandard POST /time-standards
func (h *PerformanceHandlers) createTimeStandard(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	var body createTimeStandardBody
	if !h.decodeAndValidate(w, r, &body) {
		return
	}

	ts, err := h.service.CreateTimeStandard(orgID, principal.UserID, principal, performance.CreateTimeStandardInput{
		Name:          body.Name,
		SeasonYear:    body.SeasonYear,
		StandardCode:  body.StandardCode,
		Description:   body.Description,
		EventDistance: body.EventDistance,
		Stroke:        body.Stroke,
		Course:        body.Course,
		Gender:        body.Gender,
		AgeMin:        body.AgeMin,
		AgeMax:        body.AgeMax,
		TimeMs:        body.TimeMs,
		IsActive:      body.IsActive,
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage time standards.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"time_standard": ts})
}

// getTimeStandard GET /time-standards/{standardID}
func (h *PerformanceHandlers) getTimeStandard(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	standardID := chi.URLParam(r, "standardID")

	ts, err := h.service.GetTimeStandard(orgID, standardID)
	if err != nil {
		if errors.Is(err, performance.ErrStandardNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "standard_not_found", "Time standard not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"time_standard": ts})
}

// updateTimeStandard PUT /time-standards/{standardID}
func (h *PerformanceHandlers) updateTimeStandard(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	standardID := chi.URLParam(r, "standardID")

	var body updateTimeStandardBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request_body", err.Error(), nil)
		return
	}

	ts, err := h.service.UpdateTimeStandard(orgID, principal.UserID, standardID, principal, performance.UpdateTimeStandardInput{
		Name:        body.Name,
		Description: body.Description,
		TimeMs:      body.TimeMs,
		IsActive:    body.IsActive,
	})
	if err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage time standards.", nil)
			return
		}
		if errors.Is(err, performance.ErrStandardNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "standard_not_found", "Time standard not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"time_standard": ts})
}

// deleteTimeStandard DELETE /time-standards/{standardID}
func (h *PerformanceHandlers) deleteTimeStandard(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := perfScopeFromRequest(w, r)
	if !ok {
		return
	}
	standardID := chi.URLParam(r, "standardID")

	if err := h.service.DeleteTimeStandard(orgID, principal.UserID, standardID, principal); err != nil {
		if errors.Is(err, performance.ErrForbidden) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage time standards.", nil)
			return
		}
		if errors.Is(err, performance.ErrStandardNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "standard_not_found", "Time standard not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": standardID})
}
