package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/go-chi/chi/v5"
)

// AuditHandlers handles HTTP requests for the audit-log domain.
type AuditHandlers struct {
	service *audit.Service
}

// NewAuditHandlers returns a new AuditHandlers.
func NewAuditHandlers(svc *audit.Service) *AuditHandlers {
	return &AuditHandlers{service: svc}
}

// RegisterRoutes mounts all audit routes on the given protected router.
func (h *AuditHandlers) RegisterRoutes(router chi.Router) {
	// Audit event query — admin only.
	router.Get("/audit-events", h.listAuditEvents)
}

// listAuditEvents handles GET /api/v1/audit-events.
//
// Query parameters:
//
//	actor_id      — filter by acting user ID
//	action        — filter by action string (exact, case-insensitive)
//	resource_type — filter by resource type
//	resource_id   — filter by resource ID
//	from          — ISO-8601 lower bound for occurred_at
//	to            — ISO-8601 upper bound for occurred_at
//	limit         — max results (default 50, max 200)
//	cursor        — opaque cursor for pagination (event ID)
func (h *AuditHandlers) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Audit log access requires admin role.", nil)
		return
	}

	q := r.URL.Query()

	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 200 {
		limit = 200
	}

	filter := audit.Filter{
		OrgID:        orgID,
		ActorUserID:  strings.TrimSpace(q.Get("actor_id")),
		Action:       strings.TrimSpace(q.Get("action")),
		ResourceType: strings.TrimSpace(q.Get("resource_type")),
		ResourceID:   strings.TrimSpace(q.Get("resource_id")),
		Limit:        limit,
		Cursor:       strings.TrimSpace(q.Get("cursor")),
	}

	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_param", "Invalid 'from' timestamp: use RFC3339 format (e.g. 2026-01-01T00:00:00Z).", nil)
			return
		}
		filter.From = &t
	}
	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_param", "Invalid 'to' timestamp: use RFC3339 format (e.g. 2026-01-01T00:00:00Z).", nil)
			return
		}
		filter.To = &t
	}

	events, err := h.service.Query(filter)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to query audit events.", err)
		return
	}

	var nextCursor string
	if len(events) == limit {
		nextCursor = events[len(events)-1].ID
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"events":      events,
		"next_cursor": nextCursor,
	})
}
