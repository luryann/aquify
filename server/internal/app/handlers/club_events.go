package handlers

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/events"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/roster"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// CalendarFeedSource supplies the meet and practice items of a household's
// .ics feed (club events are added by the handler itself).
type CalendarFeedSource func(orgID string, householdSwimmers []roster.Swimmer, from, to time.Time) ([]events.CalendarItem, error)

// ClubEventsHandlers serves club events, RSVPs and calendar feeds.
type ClubEventsHandlers struct {
	service      *events.Service
	directory    roster.Directory
	auditService *audit.Service
	feedSource   CalendarFeedSource
	validate     *validator.Validate
}

// NewClubEventsHandlers wires the club events handlers. feedSource may be nil
// (feeds then contain only club events).
func NewClubEventsHandlers(svc *events.Service, dir roster.Directory, auditSvc *audit.Service, feedSource CalendarFeedSource) *ClubEventsHandlers {
	return &ClubEventsHandlers{service: svc, directory: dir, auditService: auditSvc, feedSource: feedSource, validate: httpx.NewValidator()}
}

// RegisterRoutes mounts the authenticated routes.
func (h *ClubEventsHandlers) RegisterRoutes(router chi.Router) {
	router.Get("/club-events", h.listEvents)
	router.Post("/club-events", h.createEvent)
	router.Get("/club-events/{eventID}", h.getEvent)
	router.Put("/club-events/{eventID}", h.updateEvent)
	router.Delete("/club-events/{eventID}", h.deleteEvent)
	router.Get("/club-events/{eventID}/rsvps", h.listRSVPs)
	router.Put("/club-events/{eventID}/rsvps", h.respondRSVP)
	router.Get("/club-events/{eventID}/rsvps/export", h.exportRSVPs)
	router.Get("/calendar/feed-token", h.getFeedToken)
	router.Post("/calendar/feed-token", h.rotateFeedToken)
	router.Delete("/calendar/feed-token", h.revokeFeedToken)
}

// RegisterPublicRoutes mounts the token-authenticated .ics feed.
func (h *ClubEventsHandlers) RegisterPublicRoutes(router chi.Router) {
	router.Get("/calendar/feed/{token}", h.serveFeed)
}

// ─── Scope helpers ────────────────────────────────────────────────────────────

// viewer captures who is asking and which swimmers/households they act for.
type viewer struct {
	principal *appmiddleware.Principal
	staff     bool
	// linked is the directory-resolved household set (multi-household parents).
	linked   []string
	swimmers []roster.Swimmer // swimmers the viewer may act/see for
}

// ownsHousehold applies the shared household-scope rule (household_scope.go)
// plus the viewer's linked households.
func (v *viewer) ownsHousehold(householdID string) bool {
	return principalMayActForHousehold(v.principal, householdID, v.linked)
}

func (h *ClubEventsHandlers) resolveViewer(principal *appmiddleware.Principal, orgID string) (*viewer, error) {
	v := &viewer{principal: principal, staff: roster.IsStaff(principal)}
	if v.staff {
		return v, nil
	}
	ids, err := h.directory.HouseholdIDsForPrincipal(principal, orgID)
	if err != nil {
		return nil, err
	}
	v.linked = ids
	all, err := h.directory.Swimmers(orgID)
	if err != nil {
		return nil, err
	}
	isParent := roster.HasAnyRole(principal, "parent")
	for _, sw := range all {
		if isParent && v.ownsHousehold(sw.HouseholdID) {
			v.swimmers = append(v.swimmers, sw)
		} else if !isParent && sw.ID == strings.TrimSpace(principal.SwimmerID) {
			v.swimmers = append(v.swimmers, sw)
		}
	}
	return v, nil
}

// eligibleSwimmers returns the viewer swimmers inside the event's audience.
func eligibleSwimmers(e *events.Event, swimmers []roster.Swimmer) []roster.Swimmer {
	out := []roster.Swimmer{}
	for _, sw := range swimmers {
		if e.Audience != "groups" || sw.InAnyGroup(e.GroupCodes) {
			out = append(out, sw)
		}
	}
	return out
}

func (v *viewer) canSee(e *events.Event) bool {
	if v.staff {
		return true
	}
	if e.Audience != "groups" {
		return true
	}
	return len(eligibleSwimmers(e, v.swimmers)) > 0
}

func (v *viewer) ownsRSVP(r *events.RSVP) bool {
	if v.staff {
		return true
	}
	if roster.HasAnyRole(v.principal, "parent") {
		return v.ownsHousehold(r.HouseholdID)
	}
	// Swimmer: their own RSVP, or their household's household-mode RSVP.
	if r.SwimmerID != "" {
		return r.SwimmerID == strings.TrimSpace(v.principal.SwimmerID)
	}
	return v.ownsHousehold(r.HouseholdID)
}

// ─── Views ────────────────────────────────────────────────────────────────────

type clubEventView struct {
	*events.Event
	RSVPOpen           bool             `json:"rsvp_open"`
	RSVPSummary        events.Summary   `json:"rsvp_summary"`
	MyRSVPs            []*events.RSVP   `json:"my_rsvps,omitempty"`
	EligibleSwimmerIDs []string         `json:"eligible_swimmer_ids,omitempty"`
	EligibleSwimmers   []roster.Swimmer `json:"eligible_swimmers,omitempty"`
}

func (h *ClubEventsHandlers) buildView(v *viewer, e *events.Event, rsvps []*events.RSVP) clubEventView {
	view := clubEventView{Event: e, RSVPOpen: e.RSVPOpen(h.service.Now()), RSVPSummary: events.Summarize(e, rsvps)}
	if !v.staff {
		view.MyRSVPs = []*events.RSVP{}
		for _, r := range rsvps {
			if v.ownsRSVP(r) {
				view.MyRSVPs = append(view.MyRSVPs, r)
			}
		}
		view.EligibleSwimmerIDs = []string{}
		view.EligibleSwimmers = []roster.Swimmer{}
		for _, sw := range eligibleSwimmers(e, v.swimmers) {
			view.EligibleSwimmerIDs = append(view.EligibleSwimmerIDs, sw.ID)
			view.EligibleSwimmers = append(view.EligibleSwimmers, roster.Swimmer{ID: sw.ID, HouseholdID: sw.HouseholdID, FirstName: sw.FirstName, LastName: sw.LastName, DisplayName: sw.DisplayName, GroupCodes: []string{}})
		}
	}
	return view
}

// visibleEvent loads the {eventID} event for a viewer, answering 404 (not 403) when it is outside their audience.
func (h *ClubEventsHandlers) visibleEvent(w http.ResponseWriter, r *http.Request, principal *appmiddleware.Principal, orgID string) (*events.Event, *viewer, bool) {
	e, err := h.service.Get(orgID, chi.URLParam(r, "eventID"))
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load event.")
		return nil, nil, false
	}
	v, err := h.resolveViewer(principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve household scope.", err)
		return nil, nil, false
	}
	if !v.canSee(e) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Event not found.", nil)
		return nil, nil, false
	}
	return e, v, true
}

func parseTimeParam(raw string, endOfDay bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, err
	}
	if endOfDay {
		t = t.Add(24 * time.Hour)
	}
	return &t, nil
}

func (h *ClubEventsHandlers) writeServiceError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	var verr *events.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", verr.Error(), []httpx.ValidationIssue{{Field: verr.Field, Message: verr.Message}})
	case errors.Is(err, events.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Event not found.", nil)
	case errors.Is(err, events.ErrRSVPModeLocked):
		httpx.WriteError(w, r, http.StatusConflict, "rsvp_mode_locked", "RSVP mode cannot change after families have responded.", nil)
	case errors.Is(err, events.ErrEventFull):
		httpx.WriteError(w, r, http.StatusConflict, "event_full", "This event is at capacity.", nil)
	case errors.Is(err, events.ErrRSVPClosed):
		httpx.WriteError(w, r, http.StatusConflict, "rsvp_closed", "RSVPs are closed for this event.", nil)
	case errors.Is(err, events.ErrRSVPDisabled):
		httpx.WriteError(w, r, http.StatusConflict, "rsvp_disabled", "This event does not take RSVPs.", nil)
	case errors.Is(err, events.ErrEventCancelled):
		httpx.WriteError(w, r, http.StatusConflict, "event_cancelled", "This event has been cancelled.", nil)
	case errors.Is(err, events.ErrInvalidRSVP):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
	case errors.Is(err, events.ErrFeedTokenMissing):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "No active calendar feed.", nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", fallback, err)
	}
}

func (h *ClubEventsHandlers) appendAudit(principal *appmiddleware.Principal, orgID, action, resourceType, resourceID string, before, after any, meta map[string]string) {
	recordAudit(h.auditService, principal, orgID, action, resourceType, resourceID, before, after, meta)
}

// ─── Events CRUD ──────────────────────────────────────────────────────────────

func (h *ClubEventsHandlers) listEvents(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	from, err := parseTimeParam(r.URL.Query().Get("from"), false)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "from must be YYYY-MM-DD or RFC3339.", nil)
		return
	}
	to, err := parseTimeParam(r.URL.Query().Get("to"), true)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "to must be YYYY-MM-DD or RFC3339.", nil)
		return
	}
	v, err := h.resolveViewer(principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve household scope.", err)
		return
	}
	list, err := h.service.List(orgID, events.ListFilter{From: from, To: to})
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to list events.")
		return
	}
	visible := []*events.Event{}
	for _, e := range list {
		if v.canSee(e) {
			visible = append(visible, e)
		}
	}
	pagination := parseListPagination(r, listPaginationOptions{})
	page, total := paginateSlice(visible, pagination)
	ids := make([]string, 0, len(page))
	for _, e := range page {
		ids = append(ids, e.ID)
	}
	rsvps, err := h.service.ListRSVPsForEvents(orgID, ids)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load RSVPs.")
		return
	}
	views := make([]clubEventView, 0, len(page))
	for _, e := range page {
		views = append(views, h.buildView(v, e, rsvps[e.ID]))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"events": views,
		"count":  len(views),
		"total":  total,
		"limit":  pagination.Limit,
		"offset": pagination.Offset,
	})
}

func (h *ClubEventsHandlers) getEvent(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	e, v, ok := h.visibleEvent(w, r, principal, orgID)
	if !ok {
		return
	}
	rsvps, err := h.service.ListRSVPs(orgID, e.ID)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load RSVPs.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"event": h.buildView(v, e, rsvps)})
}

type clubEventBody struct {
	Title        string     `json:"title" validate:"required,max=200"`
	Category     string     `json:"category" validate:"required"`
	Description  string     `json:"description" validate:"max=5000"`
	Location     string     `json:"location" validate:"max=300"`
	StartsAt     time.Time  `json:"starts_at" validate:"required"`
	EndsAt       *time.Time `json:"ends_at"`
	AllDay       bool       `json:"all_day"`
	Audience     string     `json:"audience"`
	GroupCodes   []string   `json:"group_codes" validate:"max=50"`
	RSVPEnabled  bool       `json:"rsvp_enabled"`
	RSVPMode     string     `json:"rsvp_mode"`
	RSVPDeadline *time.Time `json:"rsvp_deadline"`
	Capacity     *int       `json:"capacity"`
	FeeCents     int        `json:"fee_cents"`
	Status       string     `json:"status"`
	Public       bool       `json:"public"`
}

func (b clubEventBody) input() events.EventInput {
	return events.EventInput{
		Title: b.Title, Category: b.Category, Description: b.Description, Location: b.Location,
		StartsAt: b.StartsAt, EndsAt: b.EndsAt, AllDay: b.AllDay, Audience: b.Audience, GroupCodes: b.GroupCodes,
		RSVPEnabled: b.RSVPEnabled, RSVPMode: b.RSVPMode, RSVPDeadline: b.RSVPDeadline, Capacity: b.Capacity,
		FeeCents: b.FeeCents, Status: b.Status, Public: b.Public,
	}
}

func (h *ClubEventsHandlers) createEvent(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may create events.", nil)
		return
	}
	var body clubEventBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	e, err := h.service.Create(orgID, principal.UserID, body.input())
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to create event.")
		return
	}
	h.appendAudit(principal, orgID, "club_event.create", "club_event", e.ID, nil, e, nil)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"event": e})
}

func (h *ClubEventsHandlers) updateEvent(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may update events.", nil)
		return
	}
	id := chi.URLParam(r, "eventID")
	before, err := h.service.Get(orgID, id)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load event.")
		return
	}
	var body clubEventBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	e, err := h.service.Update(orgID, id, principal.UserID, body.input())
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to update event.")
		return
	}
	action := "club_event.update"
	if before.Status != "cancelled" && e.Status == "cancelled" {
		action = "club_event.cancel"
	}
	h.appendAudit(principal, orgID, action, "club_event", e.ID, before, e, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"event": e})
}

func (h *ClubEventsHandlers) deleteEvent(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may delete events.", nil)
		return
	}
	id := chi.URLParam(r, "eventID")
	before, err := h.service.Get(orgID, id)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load event.")
		return
	}
	if err := h.service.Delete(orgID, id, principal.UserID); err != nil {
		h.writeServiceError(w, r, err, "Failed to delete event.")
		return
	}
	h.appendAudit(principal, orgID, "club_event.delete", "club_event", id, before, nil, nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// ─── RSVPs ────────────────────────────────────────────────────────────────────

type rsvpRosterRow struct {
	*events.RSVP
	HouseholdName string `json:"household_name,omitempty"`
	SwimmerName   string `json:"swimmer_name,omitempty"`
}

func (h *ClubEventsHandlers) rosterRows(orgID string, rsvps []*events.RSVP) ([]rsvpRosterRow, error) {
	names, err := h.directory.HouseholdNames(orgID)
	if err != nil {
		return nil, err
	}
	swimmers, err := h.directory.Swimmers(orgID)
	if err != nil {
		return nil, err
	}
	swimmerNames := map[string]string{}
	for _, sw := range swimmers {
		swimmerNames[sw.ID] = sw.DisplayName
	}
	rows := make([]rsvpRosterRow, 0, len(rsvps))
	for _, rv := range rsvps {
		rows = append(rows, rsvpRosterRow{RSVP: rv, HouseholdName: names[rv.HouseholdID], SwimmerName: swimmerNames[rv.SwimmerID]})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Response != rows[j].Response {
			return responseRank(rows[i].Response) < responseRank(rows[j].Response)
		}
		if rows[i].HouseholdName != rows[j].HouseholdName {
			return rows[i].HouseholdName < rows[j].HouseholdName
		}
		return rows[i].SwimmerName < rows[j].SwimmerName
	})
	return rows, nil
}

func responseRank(r string) int {
	switch r {
	case "yes":
		return 0
	case "maybe":
		return 1
	default:
		return 2
	}
}

func (h *ClubEventsHandlers) listRSVPs(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	e, v, ok := h.visibleEvent(w, r, principal, orgID)
	if !ok {
		return
	}
	all, err := h.service.ListRSVPs(orgID, e.ID)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load RSVPs.")
		return
	}
	scoped := []*events.RSVP{}
	for _, rv := range all {
		if v.ownsRSVP(rv) {
			scoped = append(scoped, rv)
		}
	}
	rows, err := h.rosterRows(orgID, scoped)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load roster names.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"rsvps":   rows,
		"summary": events.Summarize(e, all),
	})
}

type rsvpBody struct {
	HouseholdID string `json:"household_id"`
	SwimmerID   string `json:"swimmer_id"`
	Response    string `json:"response" validate:"required,oneof=yes no maybe"`
	GuestCount  int    `json:"guest_count" validate:"gte=0,lte=50"`
	Note        string `json:"note" validate:"max=500"`
}

func (h *ClubEventsHandlers) respondRSVP(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	var body rsvpBody
	if !decodeAndValidateJSON(w, r, &body, h.validate, decodeAndValidateOptions{}) {
		return
	}
	e, v, ok := h.visibleEvent(w, r, principal, orgID)
	if !ok {
		return
	}

	in := events.RSVPInput{
		HouseholdID: strings.TrimSpace(body.HouseholdID),
		SwimmerID:   strings.TrimSpace(body.SwimmerID),
		Response:    body.Response,
		GuestCount:  body.GuestCount,
		Note:        body.Note,
	}
	enforceWindow := true
	if v.staff {
		enforceWindow = false
		allSwimmers, err := h.directory.Swimmers(orgID)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load roster.", err)
			return
		}
		if in.SwimmerID != "" {
			sw, found := roster.SwimmerByID(allSwimmers, in.SwimmerID)
			if !found {
				httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Unknown swimmer.", nil)
				return
			}
			if in.HouseholdID != "" && in.HouseholdID != sw.HouseholdID {
				httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Swimmer does not belong to that household.", nil)
				return
			}
			in.HouseholdID = sw.HouseholdID
		}
	} else {
		isParent := roster.HasAnyRole(principal, "parent")
		if e.RSVPMode == "swimmer" {
			if !isParent {
				in.SwimmerID = strings.TrimSpace(principal.SwimmerID)
			}
			sw, found := roster.SwimmerByID(eligibleSwimmers(e, v.swimmers), in.SwimmerID)
			if !found {
				httpx.WriteError(w, r, http.StatusForbidden, "household_scope_violation", "You can only RSVP for eligible swimmers in your household.", nil)
				return
			}
			in.HouseholdID = sw.HouseholdID
		} else {
			if !isParent {
				httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "A parent must RSVP for the household.", nil)
				return
			}
			if in.HouseholdID == "" && len(v.linked) == 1 {
				in.HouseholdID = v.linked[0]
			}
			if !v.ownsHousehold(in.HouseholdID) {
				httpx.WriteError(w, r, http.StatusForbidden, "household_scope_violation", "You can only RSVP for your own household.", nil)
				return
			}
			in.SwimmerID = ""
		}
	}

	result, err := h.service.Respond(orgID, e.ID, principal.UserID, in, enforceWindow)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to save RSVP.")
		return
	}
	if v.staff {
		h.appendAudit(principal, orgID, "club_event.rsvp.staff_update", "club_event_rsvp", result.RSVP.ID, nil, result.RSVP,
			map[string]string{"event_id": e.ID, "household_id": result.RSVP.HouseholdID})
	}
	if result.Billing == events.BillingCharged || result.Billing == events.BillingReleased {
		h.appendAudit(principal, orgID, "club_event.rsvp.fee_"+string(result.Billing), "club_event_rsvp", result.RSVP.ID, nil, result.RSVP,
			map[string]string{"event_id": e.ID, "invoice_id": result.RSVP.InvoiceID})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"rsvp": result.RSVP, "billing": result.Billing})
}

func (h *ClubEventsHandlers) exportRSVPs(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin/coach may export RSVP rosters.", nil)
		return
	}
	e, err := h.service.Get(orgID, chi.URLParam(r, "eventID"))
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load event.")
		return
	}
	rsvps, err := h.service.ListRSVPs(orgID, e.ID)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load RSVPs.")
		return
	}
	rows, err := h.rosterRows(orgID, rsvps)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load roster names.", err)
		return
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"Event", "Household", "Swimmer", "Response", "Guests", "Headcount", "Note", "Invoice ID", "Responded At"})
	for _, row := range rows {
		headcount := ""
		if row.Response == "yes" {
			headcount = strconv.Itoa(row.Headcount())
		}
		_ = cw.Write([]string{
			csvSafe(e.Title), csvSafe(row.HouseholdName), csvSafe(row.SwimmerName), row.Response,
			strconv.Itoa(row.GuestCount), headcount, csvSafe(row.Note), row.InvoiceID,
			row.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	cw.Flush()
	h.appendAudit(principal, orgID, "club_event.rsvp.export", "club_event", e.ID, nil, nil, map[string]string{"rows": strconv.Itoa(len(rows))})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-rsvps.csv"`, slugify(e.Title)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// csvSafe neutralises spreadsheet formula injection in user-supplied cells.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 60 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "event"
	}
	return out
}

// ─── Calendar feed tokens ─────────────────────────────────────────────────────

// feedHousehold resolves which household a feed-token request targets.
func (h *ClubEventsHandlers) feedHousehold(w http.ResponseWriter, r *http.Request, principal *appmiddleware.Principal, orgID string) (string, bool) {
	requested := strings.TrimSpace(r.URL.Query().Get("household_id"))
	if hasAnyRole(principal, "admin") {
		if requested == "" && !hasAnyRole(principal, "parent") {
			httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "household_id is required.", nil)
			return "", false
		}
		if requested != "" {
			names, err := h.directory.HouseholdNames(orgID)
			if err != nil {
				httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load households.", err)
				return "", false
			}
			if _, exists := names[requested]; !exists {
				httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Household not found.", nil)
				return "", false
			}
			return requested, true
		}
	}
	if !hasAnyRole(principal, "parent") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only a parent (or admin) may manage a household calendar feed.", nil)
		return "", false
	}
	ids, err := h.directory.HouseholdIDsForPrincipal(principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve household scope.", err)
		return "", false
	}
	if requested == "" {
		if len(ids) == 0 {
			httpx.WriteError(w, r, http.StatusForbidden, "household_scope_violation", "No household is linked to this account.", nil)
			return "", false
		}
		sort.Strings(ids)
		if own := strings.TrimSpace(principal.HouseholdID); principalMayActForHousehold(principal, own, ids) {
			return own, true
		}
		return ids[0], true
	}
	if principalMayActForHousehold(principal, requested, ids) {
		return requested, true
	}
	httpx.WriteError(w, r, http.StatusForbidden, "household_scope_violation", "You can only manage your own household's feed.", nil)
	return "", false
}

const feedPathPrefix = "/api/v1/calendar/feed/"

func (h *ClubEventsHandlers) getFeedToken(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	householdID, ok := h.feedHousehold(w, r, principal, orgID)
	if !ok {
		return
	}
	tok, err := h.service.ActiveFeedToken(orgID, householdID)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to load calendar feed.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"household_id": householdID, "active": tok != nil, "feed": tok})
}

func (h *ClubEventsHandlers) rotateFeedToken(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	householdID, ok := h.feedHousehold(w, r, principal, orgID)
	if !ok {
		return
	}
	secret, tok, err := h.service.RotateFeedToken(orgID, householdID, principal.UserID)
	if err != nil {
		h.writeServiceError(w, r, err, "Failed to create calendar feed.")
		return
	}
	h.appendAudit(principal, orgID, "calendar_feed.rotate", "calendar_feed_token", tok.ID, nil, tok, map[string]string{"household_id": householdID})
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"household_id": householdID,
		"active":       true,
		"feed":         tok,
		"token":        secret,
		"feed_path":    feedPathPrefix + secret + ".ics",
	})
}

func (h *ClubEventsHandlers) revokeFeedToken(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	householdID, ok := h.feedHousehold(w, r, principal, orgID)
	if !ok {
		return
	}
	if err := h.service.RevokeFeedToken(orgID, householdID); err != nil {
		h.writeServiceError(w, r, err, "Failed to revoke calendar feed.")
		return
	}
	h.appendAudit(principal, orgID, "calendar_feed.revoke", "calendar_feed_token", householdID, nil, nil, map[string]string{"household_id": householdID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"household_id": householdID, "active": false})
}

// serveFeed handles GET /api/v1/calendar/feed/{token}(.ics) — public, the
// unguessable token is the credential. Unknown/revoked tokens get a bare 404.
func (h *ClubEventsHandlers) serveFeed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(chi.URLParam(r, "token"), ".ics")
	tok, err := h.service.ResolveFeedToken(token)
	if err != nil {
		if errors.Is(err, events.ErrFeedTokenMissing) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Calendar feed not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Calendar feed unavailable.", err)
		return
	}
	orgID := tok.OrgID
	allSwimmers, err := h.directory.Swimmers(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Calendar feed unavailable.", err)
		return
	}
	household := roster.SwimmersInHousehold(allSwimmers, tok.HouseholdID)
	now := h.service.Now()
	from := now.AddDate(0, 0, -60)
	to := now.AddDate(1, 0, 0)

	items := []events.CalendarItem{}
	if h.feedSource != nil {
		extra, err := h.feedSource(orgID, household, from, to)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Calendar feed unavailable.", err)
			return
		}
		items = append(items, extra...)
	}
	list, err := h.service.List(orgID, events.ListFilter{From: &from, To: &to})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Calendar feed unavailable.", err)
		return
	}
	for _, e := range list {
		if e.Audience == "groups" && len(eligibleSwimmers(e, household)) == 0 {
			continue
		}
		items = append(items, ClubEventCalendarItem(e))
	}
	sort.SliceStable(items, func(i, j int) bool { return calendarSortKey(items[i]) < calendarSortKey(items[j]) })

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="club-calendar.ics"`)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(events.RenderICS("Swim club calendar", items, now)))
}

func calendarSortKey(it events.CalendarItem) string {
	if it.AllDay {
		return it.StartDate + "T00:00:00Z"
	}
	return it.Start.UTC().Format(time.RFC3339)
}

// ClubEventCalendarItem converts a club event to a feed item. All-day events
// carry UTC-midnight timestamps whose date part is the calendar date.
func ClubEventCalendarItem(e *events.Event) events.CalendarItem {
	desc := e.Description
	if e.RSVPEnabled {
		line := "RSVP in the parent portal."
		if e.FeeCents > 0 {
			line = fmt.Sprintf("RSVP in the parent portal. Fee: $%.2f per person.", float64(e.FeeCents)/100)
		}
		desc = strings.TrimSpace(desc + "\n\n" + line)
	}
	item := events.CalendarItem{
		UID:         "club-event-" + e.ID + "@dare",
		Title:       e.Title,
		Description: desc,
		Location:    e.Location,
		Categories:  strings.ReplaceAll(e.Category, "_", " "),
		Cancelled:   e.Status == "cancelled",
		Updated:     e.UpdatedAt,
	}
	if e.Status == "cancelled" {
		item.Title = "Cancelled: " + e.Title
	}
	if e.AllDay {
		item.AllDay = true
		item.StartDate = e.StartsAt.UTC().Format("2006-01-02")
		item.EndDate = item.StartDate
		if e.EndsAt != nil {
			item.EndDate = e.EndsAt.UTC().Format("2006-01-02")
		}
	} else {
		item.Start = e.StartsAt
		item.End = e.EndsAt
	}
	return item
}
