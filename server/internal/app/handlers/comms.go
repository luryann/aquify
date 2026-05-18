package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// CommsHandlers handles HTTP requests for the communications domain.
type CommsHandlers struct {
	service              *comms.Service
	validate             *validator.Validate
	postmarkHeaderName   string
	postmarkHeaderValue  string
	postmarkBasicUser    string
	postmarkBasicPass    string
	allowInsecureWebhook bool
}

// NewCommsHandlers returns a new CommsHandlers.
func NewCommsHandlers(svc *comms.Service) *CommsHandlers {
	return &CommsHandlers{service: svc, validate: validator.New()}
}

// NewCommsHandlersWithWebhookAuth returns a CommsHandlers configured to verify
// Postmark webhook requests via custom header and/or HTTP basic auth.
func NewCommsHandlersWithWebhookAuth(svc *comms.Service, headerName, headerValue, basicUser, basicPass, appEnv string) *CommsHandlers {
	return &CommsHandlers{
		service:              svc,
		validate:             validator.New(),
		postmarkHeaderName:   strings.TrimSpace(headerName),
		postmarkHeaderValue:  strings.TrimSpace(headerValue),
		postmarkBasicUser:    strings.TrimSpace(basicUser),
		postmarkBasicPass:    strings.TrimSpace(basicPass),
		allowInsecureWebhook: strings.EqualFold(strings.TrimSpace(appEnv), "development") || strings.EqualFold(strings.TrimSpace(appEnv), "local") || strings.EqualFold(strings.TrimSpace(appEnv), "test"),
	}
}

// RegisterPublicRoutes mounts unauthenticated provider webhook routes.
func (h *CommsHandlers) RegisterPublicRoutes(router chi.Router) {
	router.Post("/webhooks/postmark", h.handlePostmarkWebhook)
}

// RegisterRoutes mounts all comms routes on the given protected router.
func (h *CommsHandlers) RegisterRoutes(router chi.Router) {
	// Announcements — writable by owner/admin/coach; readable by all auth users.
	router.Post("/announcements", h.createAnnouncement)
	router.Get("/announcements", h.listAnnouncements)
	router.Get("/announcements/{announcementID}", h.getAnnouncement)
	router.Post("/announcements/{announcementID}/publish", h.publishAnnouncement)
	router.Post("/announcements/{announcementID}/archive", h.archiveAnnouncement)

	// Email campaigns — owner/admin/coach only.
	router.Post("/email-campaigns", h.createCampaign)
	router.Get("/email-campaigns", h.listCampaigns)
	router.Get("/email-campaigns/{campaignID}", h.getCampaign)
	router.Post("/email-campaigns/{campaignID}/send", h.sendCampaign)
	router.Get("/email-campaigns/{campaignID}/deliveries", h.listDeliveries)

	// Suppression list — owner/admin only.
	router.Post("/suppression", h.addSuppression)
	router.Get("/suppression", h.listSuppression)
	router.Delete("/suppression/{suppressionID}", h.removeSuppression)
}

// ─── Scope helper ─────────────────────────────────────────────────────────────

func commsScopeFromRequest(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	return scopedRequestFromContext(w, r)
}

// hasAnyRole returns true when the principal holds at least one of the allowed roles.
func hasAnyRole(principal *appmiddleware.Principal, roles ...string) bool {
	for _, pr := range principal.Roles {
		for _, allowed := range roles {
			if strings.EqualFold(pr, allowed) {
				return true
			}
		}
	}
	return false
}

func (h *CommsHandlers) verifyPostmarkWebhook(r *http.Request) bool {
	headerConfigured := h.postmarkHeaderName != "" && h.postmarkHeaderValue != ""
	basicConfigured := h.postmarkBasicUser != "" || h.postmarkBasicPass != ""
	if !headerConfigured && !basicConfigured {
		return h.allowInsecureWebhook
	}
	if headerConfigured && r.Header.Get(h.postmarkHeaderName) != h.postmarkHeaderValue {
		return false
	}
	if basicConfigured {
		user, pass, ok := r.BasicAuth()
		if !ok || user != h.postmarkBasicUser || pass != h.postmarkBasicPass {
			return false
		}
	}
	return true
}

func parseWebhookTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts.UTC()
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts.UTC()
	}
	return time.Time{}
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key].(string); ok {
		return value
	}
	return ""
}

func (h *CommsHandlers) handlePostmarkWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.verifyPostmarkWebhook(r) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "invalid_webhook_auth", "Postmark webhook authentication failed.", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB hard cap to prevent body-based DoS

	var body struct {
		RecordType        string         `json:"RecordType"`
		MessageID         string         `json:"MessageID"`
		Recipient         string         `json:"Recipient"`
		Email             string         `json:"Email"`
		DeliveredAt       string         `json:"DeliveredAt"`
		BouncedAt         string         `json:"BouncedAt"`
		ChangedAt         string         `json:"ChangedAt"`
		Details           string         `json:"Details"`
		Metadata          map[string]any `json:"Metadata"`
		SuppressSending   bool           `json:"SuppressSending"`
		SuppressionReason string         `json:"SuppressionReason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}

	recipient := strings.TrimSpace(body.Recipient)
	if recipient == "" {
		recipient = strings.TrimSpace(body.Email)
	}
	occurredAt := parseWebhookTime(body.DeliveredAt)
	if occurredAt.IsZero() {
		occurredAt = parseWebhookTime(body.BouncedAt)
	}
	if occurredAt.IsZero() {
		occurredAt = parseWebhookTime(body.ChangedAt)
	}

	err := h.service.ApplyPostmarkEvent(comms.PostmarkEventInput{
		OrgID:             metadataString(body.Metadata, "org_id"),
		MessageID:         body.MessageID,
		RecipientEmail:    recipient,
		RecordType:        body.RecordType,
		OccurredAt:        occurredAt,
		Details:           body.Details,
		SuppressionReason: body.SuppressionReason,
		SuppressSending:   body.SuppressSending,
	})
	if err != nil && !errors.Is(err, comms.ErrDeliveryNotFound) {
		httpx.WriteError(w, r, http.StatusInternalServerError, "webhook_apply_failed", "Failed to apply Postmark webhook.", nil)
		return
	}

	status := "processed"
	if errors.Is(err, comms.ErrDeliveryNotFound) {
		status = "ignored_missing_delivery"
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": status})
}

// ─── Announcement handlers ────────────────────────────────────────────────────

func (h *CommsHandlers) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to create announcements.", nil)
		return
	}

	var body struct {
		Title    string `json:"title"    validate:"required,max=255"`
		Body     string `json:"body"     validate:"required"`
		Audience string `json:"audience" validate:"required,oneof=all group"`
		GroupID  string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	ann, err := h.service.CreateAnnouncement(comms.CreateAnnouncementInput{
		OrgID:     orgID,
		Title:     body.Title,
		Body:      body.Body,
		Audience:  body.Audience,
		GroupID:   body.GroupID,
		CreatedBy: principal.UserID,
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "create_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, ann)
}

func (h *CommsHandlers) listAnnouncements(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	statusFilter := r.URL.Query().Get("status")
	announcements, err := h.service.ListAnnouncements(orgID, statusFilter)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "list_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"announcements": announcements})
}

func (h *CommsHandlers) getAnnouncement(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "announcementID")
	ann, err := h.service.GetAnnouncement(orgID, id)
	if err != nil {
		if errors.Is(err, comms.ErrAnnouncementNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Announcement not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "get_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ann)
}

func (h *CommsHandlers) publishAnnouncement(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to publish announcements.", nil)
		return
	}
	id := chi.URLParam(r, "announcementID")
	ann, err := h.service.PublishAnnouncement(orgID, id, principal.UserID)
	if err != nil {
		if errors.Is(err, comms.ErrAnnouncementNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Announcement not found.", nil)
			return
		}
		if errors.Is(err, comms.ErrInvalidTransition) {
			httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", "Announcement cannot be published from its current state.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "publish_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ann)
}

func (h *CommsHandlers) archiveAnnouncement(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to archive announcements.", nil)
		return
	}
	id := chi.URLParam(r, "announcementID")
	ann, err := h.service.ArchiveAnnouncement(orgID, id, principal.UserID)
	if err != nil {
		if errors.Is(err, comms.ErrAnnouncementNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Announcement not found.", nil)
			return
		}
		if errors.Is(err, comms.ErrInvalidTransition) {
			httpx.WriteError(w, r, http.StatusConflict, "invalid_transition", "Announcement is already archived.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "archive_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ann)
}

// ─── Campaign handlers ────────────────────────────────────────────────────────

func (h *CommsHandlers) createCampaign(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to create campaigns.", nil)
		return
	}

	var body struct {
		Name      string `json:"name"       validate:"required,max=255"`
		Subject   string `json:"subject"    validate:"required,max=255"`
		BodyHTML  string `json:"body_html"  validate:"required"`
		BodyText  string `json:"body_text"`
		FromName  string `json:"from_name"  validate:"required"`
		FromEmail string `json:"from_email" validate:"required,email"`
		Audience  string `json:"audience"   validate:"required,oneof=all group"`
		GroupID   string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	campaign, err := h.service.CreateCampaign(comms.CreateCampaignInput{
		OrgID:     orgID,
		Name:      body.Name,
		Subject:   body.Subject,
		BodyHTML:  body.BodyHTML,
		BodyText:  body.BodyText,
		FromName:  body.FromName,
		FromEmail: body.FromEmail,
		Audience:  body.Audience,
		GroupID:   body.GroupID,
		CreatedBy: principal.UserID,
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "create_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, campaign)
}

func (h *CommsHandlers) listCampaigns(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	campaigns, err := h.service.ListCampaigns(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "list_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"campaigns": campaigns})
}

func (h *CommsHandlers) getCampaign(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	id := chi.URLParam(r, "campaignID")
	campaign, err := h.service.GetCampaign(orgID, id)
	if err != nil {
		if errors.Is(err, comms.ErrCampaignNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Campaign not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "get_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, campaign)
}

func (h *CommsHandlers) sendCampaign(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to send campaigns.", nil)
		return
	}

	id := chi.URLParam(r, "campaignID")

	// Feature flag guard: arbitrary (caller-supplied) recipient lists are disabled
	// by default. Set COMMS_ALLOW_ARBITRARY_RECIPIENTS=true only in development or
	// when a full server-side org-member validation layer has been implemented.
	// In production, recipients MUST be resolved server-side from org membership
	// to prevent org-branded email from being sent to arbitrary external addresses.
	if os.Getenv("COMMS_ALLOW_ARBITRARY_RECIPIENTS") != "true" {
		httpx.WriteError(w, r, http.StatusForbidden, "arbitrary_recipients_disabled",
			"Sending to a caller-supplied recipient list is disabled. Set COMMS_ALLOW_ARBITRARY_RECIPIENTS=true only after implementing server-side org-member recipient resolution.", nil)
		return
	}

	var body struct {
		Recipients []struct {
			Email  string `json:"email"  validate:"required,email"`
			UserID string `json:"user_id"`
		} `json:"recipients" validate:"required,min=1"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	recipients := make([]comms.Recipient, len(body.Recipients))
	for i, rec := range body.Recipients {
		recipients[i] = comms.Recipient{Email: rec.Email, UserID: rec.UserID}
	}

	campaign, err := h.service.SendCampaign(r.Context(), orgID, id, comms.SendCampaignInput{
		Recipients: recipients,
		ActorID:    principal.UserID,
	})
	if err != nil {
		if errors.Is(err, comms.ErrCampaignNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Campaign not found.", nil)
			return
		}
		if errors.Is(err, comms.ErrCampaignNotDraft) {
			httpx.WriteError(w, r, http.StatusConflict, "campaign_not_draft", "Campaign must be in draft status to send.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "send_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, campaign)
}

func (h *CommsHandlers) listDeliveries(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	id := chi.URLParam(r, "campaignID")
	deliveries, err := h.service.ListDeliveries(orgID, id)
	if err != nil {
		if errors.Is(err, comms.ErrCampaignNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Campaign not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "list_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"deliveries": deliveries})
}

// ─── Suppression handlers ─────────────────────────────────────────────────────

func (h *CommsHandlers) addSuppression(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to manage suppression list.", nil)
		return
	}

	var body struct {
		Email  string `json:"email"  validate:"required,email"`
		Reason string `json:"reason" validate:"required,oneof=bounce complaint manual"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	entry, err := h.service.AddSuppression(comms.AddSuppressionInput{
		OrgID:     orgID,
		Email:     body.Email,
		Reason:    body.Reason,
		CreatedBy: principal.UserID,
	})
	if err != nil {
		if errors.Is(err, comms.ErrDuplicateSuppression) {
			httpx.WriteError(w, r, http.StatusConflict, "duplicate_suppression", "Email is already suppressed for this organisation.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusBadRequest, "add_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, entry)
}

func (h *CommsHandlers) listSuppression(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	entries, err := h.service.ListSuppression(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "list_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"suppression": entries})
}

func (h *CommsHandlers) removeSuppression(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "owner", "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role.", nil)
		return
	}
	id := chi.URLParam(r, "suppressionID")
	if err := h.service.RemoveSuppression(orgID, id); err != nil {
		if errors.Is(err, comms.ErrSuppressionNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Suppression entry not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "remove_failed", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
