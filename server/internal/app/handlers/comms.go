package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// CommsHandlers handles HTTP requests for the communications domain.
type CommsHandlers struct {
	service              *comms.Service
	auditService         *audit.Service
	membersService       *members.Service
	groupsService        *groups.Service
	validate             *validator.Validate
	postmarkHeaderName   string
	postmarkHeaderValue  string
	postmarkBasicUser    string
	postmarkBasicPass    string
	allowInsecureWebhook bool
	store                storage.ObjectStorage
}

// SetStorage attaches object storage used for message attachment upload URLs.
func (h *CommsHandlers) SetStorage(store storage.ObjectStorage) {
	h.store = store
}

// NewCommsHandlers returns a new CommsHandlers.
func NewCommsHandlers(svc *comms.Service, membersSvc *members.Service, groupsSvc *groups.Service) *CommsHandlers {
	return &CommsHandlers{service: svc, membersService: membersSvc, groupsService: groupsSvc, validate: validator.New()}
}

// NewCommsHandlersWithAudit returns a CommsHandlers with audit logging enabled.
func NewCommsHandlersWithAudit(svc *comms.Service, auditSvc *audit.Service, membersSvc *members.Service, groupsSvc *groups.Service) *CommsHandlers {
	return &CommsHandlers{service: svc, auditService: auditSvc, membersService: membersSvc, groupsService: groupsSvc, validate: validator.New()}
}

// NewCommsHandlersWithWebhookAuth returns a CommsHandlers configured to verify
// Postmark webhook requests via custom header and/or HTTP basic auth.
func NewCommsHandlersWithWebhookAuth(svc *comms.Service, membersSvc *members.Service, groupsSvc *groups.Service, headerName, headerValue, basicUser, basicPass, appEnv string) *CommsHandlers {
	return &CommsHandlers{
		service:              svc,
		membersService:       membersSvc,
		groupsService:        groupsSvc,
		validate:             validator.New(),
		postmarkHeaderName:   strings.TrimSpace(headerName),
		postmarkHeaderValue:  strings.TrimSpace(headerValue),
		postmarkBasicUser:    strings.TrimSpace(basicUser),
		postmarkBasicPass:    strings.TrimSpace(basicPass),
		allowInsecureWebhook: strings.EqualFold(strings.TrimSpace(appEnv), "development") || strings.EqualFold(strings.TrimSpace(appEnv), "local") || strings.EqualFold(strings.TrimSpace(appEnv), "test"),
	}
}

// NewCommsHandlersWithAuditAndWebhookAuth returns a CommsHandlers configured
// for audit logging and Postmark webhook verification.
func NewCommsHandlersWithAuditAndWebhookAuth(svc *comms.Service, auditSvc *audit.Service, membersSvc *members.Service, groupsSvc *groups.Service, headerName, headerValue, basicUser, basicPass, appEnv string) *CommsHandlers {
	handler := NewCommsHandlersWithWebhookAuth(svc, membersSvc, groupsSvc, headerName, headerValue, basicUser, basicPass, appEnv)
	handler.auditService = auditSvc
	return handler
}

// RegisterPublicRoutes mounts unauthenticated provider webhook routes.
func (h *CommsHandlers) RegisterPublicRoutes(router chi.Router) {
	router.Post("/webhooks/postmark", h.handlePostmarkWebhook)
}

// RegisterRoutes mounts all comms routes on the given protected router.
func (h *CommsHandlers) RegisterRoutes(router chi.Router) {
	// Announcements — writable by admin/coach; readable by all auth users.
	router.Post("/announcements", h.createAnnouncement)
	router.Get("/announcements", h.listAnnouncements)
	router.Get("/announcements/{announcementID}", h.getAnnouncement)
	router.Post("/announcements/{announcementID}/publish", h.publishAnnouncement)
	router.Post("/announcements/{announcementID}/archive", h.archiveAnnouncement)
	router.Patch("/announcements/{announcementID}", h.setAnnouncementPublic)

	// Email campaigns — admin/coach only.
	router.Post("/email-campaigns", h.createCampaign)
	router.Get("/email-campaigns", h.listCampaigns)
	router.Get("/email-campaigns/{campaignID}", h.getCampaign)
	router.Post("/email-campaigns/{campaignID}/send", h.sendCampaign)
	router.Get("/email-campaigns/{campaignID}/deliveries", h.listDeliveries)

	// Suppression list — admin only.
	router.Post("/suppression", h.addSuppression)
	router.Get("/suppression", h.listSuppression)
	router.Delete("/suppression/{suppressionID}", h.removeSuppression)
	router.Get("/communication-preferences", h.listNotificationPreferences)
	router.Put("/communication-preferences", h.updateNotificationPreferences)

	// Messages — a household+swimmer thread between a parent/swimmer and staff.
	router.Post("/messages", h.sendMessage)
	router.Get("/messages", h.listMessages)
	router.Get("/message-threads", h.listMessageThreads)
	router.Patch("/messages/{messageID}", h.editMessage)
	router.Post("/messages/{messageID}/reactions", h.addReaction)
	router.Delete("/messages/{messageID}/reactions/{emoji}", h.removeReaction)
	router.Post("/messages/read", h.markThreadRead)
	router.Post("/messages/upload-url", h.requestMessageUploadURL)
	router.Get("/messages/stream", h.streamMessages)
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

func (h *CommsHandlers) emitAudit(principal *appmiddleware.Principal, action, resourceType, resourceID string, metadata map[string]string, after any) {
	if h.auditService == nil || principal == nil {
		return
	}
	_, _ = h.auditService.Append(audit.AppendRequest{
		OrgID:        principal.OrgID,
		ActorUserID:  principal.UserID,
		ActorRoles:   append([]string{}, principal.Roles...),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		After:        after,
		Metadata:     metadata,
	})
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

func (h *CommsHandlers) resolveCampaignRecipients(orgID string, campaign *comms.EmailCampaign) ([]comms.Recipient, error) {
	if campaign == nil {
		return nil, comms.ErrCampaignNotFound
	}
	if h.membersService == nil {
		return nil, errors.New("member recipient resolution is unavailable")
	}

	allowedHouseholds := map[string]struct{}{}
	switch strings.TrimSpace(strings.ToLower(campaign.Audience)) {
	case "all":
		for _, household := range h.membersService.SnapshotHouseholds(orgID) {
			allowedHouseholds[household.ID] = struct{}{}
		}
	case "group":
		if h.groupsService == nil {
			return nil, errors.New("group recipient resolution is unavailable")
		}
		for _, registration := range h.groupsService.SnapshotRegistrations(orgID) {
			if registration.GroupID != campaign.GroupID {
				continue
			}
			if !slices.Contains([]string{"pending", "active"}, strings.TrimSpace(strings.ToLower(registration.Status))) {
				continue
			}
			allowedHouseholds[registration.HouseholdID] = struct{}{}
		}
	default:
		return nil, errors.New("campaign audience is unsupported for recipient resolution")
	}

	recipients := make([]comms.Recipient, 0, len(allowedHouseholds))
	seen := make(map[string]struct{})
	appendRecipient := func(email string) {
		normalized := strings.ToLower(strings.TrimSpace(email))
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		recipients = append(recipients, comms.Recipient{Email: normalized})
	}

	for _, household := range h.membersService.SnapshotHouseholds(orgID) {
		if _, ok := allowedHouseholds[household.ID]; !ok {
			continue
		}
		appendRecipient(household.PrimaryEmail)
	}
	for _, contact := range h.membersService.SnapshotContacts(orgID) {
		if _, ok := allowedHouseholds[contact.HouseholdID]; !ok {
			continue
		}
		appendRecipient(contact.Email)
	}

	return recipients, nil
}

func (h *CommsHandlers) handlePostmarkWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.verifyPostmarkWebhook(r) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "invalid_webhook_auth", "Postmark webhook authentication failed.", nil)
		return
	}

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
	if err := decodeJSONBody(w, r, &body); err != nil {
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

func (h *CommsHandlers) listNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	preferences := h.service.ListNotificationPreferences(orgID, principal.UserID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"preferences": preferences})
}

func (h *CommsHandlers) updateNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}

	var body struct {
		Preferences []comms.NotificationPreferenceSetting `json:"preferences" validate:"required,min=1,dive"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	preferences, err := h.service.ReplaceNotificationPreferences(orgID, principal.UserID, principal.HouseholdID, principal.UserID, body.Preferences)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "update_failed", err.Error(), nil)
		return
	}

	h.emitAudit(principal, audit.ActionCommsNotificationPreferencesUpdate, "communication_preferences", principal.UserID, map[string]string{
		"household_id": principal.HouseholdID,
	}, preferences)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"preferences": preferences})
}

// ─── Messages ─────────────────────────────────────────────────────────────────

// primaryRole returns the principal's first role, used to label who sent a message.
func primaryRole(principal *appmiddleware.Principal) string {
	if principal == nil || len(principal.Roles) == 0 {
		return ""
	}
	return principal.Roles[0]
}

// swimmerHouseholdID returns the household a swimmer belongs to, or "" if unknown.
func (h *CommsHandlers) swimmerHouseholdID(orgID, swimmerID string) string {
	if h.membersService == nil {
		return ""
	}
	for _, swimmer := range h.membersService.SnapshotSwimmers(orgID) {
		if swimmer.ID == swimmerID {
			return swimmer.HouseholdID
		}
	}
	return ""
}

// authorizeMessageThread checks that the principal may read/write the given
// household+swimmer thread. Staff (admin/coach) may access any thread in
// the org; parent/swimmer may only access their own household's threads.
func (h *CommsHandlers) authorizeMessageThread(principal *appmiddleware.Principal, orgID, householdID, swimmerID string) bool {
	if hasAnyRole(principal, "admin", "coach") {
		return true
	}
	if !hasAnyRole(principal, "parent", "swimmer") {
		return false
	}
	if strings.TrimSpace(principal.HouseholdID) == "" || strings.TrimSpace(principal.HouseholdID) != strings.TrimSpace(householdID) {
		return false
	}
	return h.swimmerHouseholdID(orgID, swimmerID) == householdID
}

// Chat attachments are images shared inline in a conversation, not legal
// documents — a smaller cap and an image-only allowlist versus the document
// upload path in handlers/documents.go.
const maxMessageAttachmentBytes int64 = 10 << 20 // 10 MiB

var allowedMessageAttachmentMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/heic": true,
	"image/webp": true,
}

func messageAttachmentObjectKey(orgID, householdID, swimmerID, fileName string) string {
	stamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	return fmt.Sprintf("messages/%s/%s/%s/%s-%s", orgID, householdID, swimmerID, stamp, fileName)
}

func validateMessageAttachmentObjectKey(orgID, householdID, swimmerID, objectKey string) error {
	prefix := fmt.Sprintf("messages/%s/%s/%s/", strings.TrimSpace(orgID), strings.TrimSpace(householdID), strings.TrimSpace(swimmerID))
	if !strings.HasPrefix(strings.TrimSpace(objectKey), prefix) {
		return fmt.Errorf("object key %q does not belong to this thread", objectKey)
	}
	return nil
}

type sendMessageAttachmentBody struct {
	ObjectKey string `json:"object_key" validate:"required"`
	FileName  string `json:"file_name"  validate:"required"`
	FileMIME  string `json:"file_mime"  validate:"required"`
}

func (h *CommsHandlers) sendMessage(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}

	var body struct {
		HouseholdID string                      `json:"household_id" validate:"required"`
		SwimmerID   string                      `json:"swimmer_id"   validate:"required"`
		Body        string                      `json:"body"         validate:"required,max=4000"`
		Attachments []sendMessageAttachmentBody `json:"attachments"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	if !h.authorizeMessageThread(principal, orgID, body.HouseholdID, body.SwimmerID) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to message this thread.", nil)
		return
	}

	attachments := make([]comms.AttachmentInput, 0, len(body.Attachments))
	for _, in := range body.Attachments {
		fileName, err := sanitizeDocumentFilename(in.FileName)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_filename", err.Error(), nil)
			return
		}
		if err := validateMessageAttachmentObjectKey(orgID, body.HouseholdID, body.SwimmerID, in.ObjectKey); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_object_key", "Attachment object key is not valid for this thread.", nil)
			return
		}
		declaredMIME := strings.ToLower(strings.TrimSpace(in.FileMIME))
		if !allowedMessageAttachmentMIMETypes[declaredMIME] {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_mime_type", "Attachment type is not permitted.", nil)
			return
		}
		if h.store == nil {
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "storage_unavailable", "Object storage is not configured.", nil)
			return
		}
		exists, err := h.store.Exists(context.Background(), in.ObjectKey)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to validate uploaded attachment.", err)
			return
		}
		if !exists {
			httpx.WriteError(w, r, http.StatusConflict, "missing_uploaded_file", "Uploaded attachment was not found in object storage.", nil)
			return
		}
		actualSize, err := h.store.ObjectSize(context.Background(), in.ObjectKey)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to validate uploaded attachment.", err)
			return
		}
		if actualSize > maxMessageAttachmentBytes {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "file_too_large", "Attachment exceeds the maximum allowed size.", nil)
			return
		}
		attachments = append(attachments, comms.AttachmentInput{
			ObjectKey: in.ObjectKey,
			Filename:  fileName,
			MimeType:  declaredMIME,
			SizeBytes: actualSize,
		})
	}

	message, err := h.service.SendMessage(comms.CreateMessageInput{
		OrgID:        orgID,
		HouseholdID:  body.HouseholdID,
		SwimmerID:    body.SwimmerID,
		Body:         body.Body,
		SenderUserID: principal.UserID,
		SenderRole:   primaryRole(principal),
		Attachments:  attachments,
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "send_failed", err.Error(), nil)
		return
	}

	h.emitAudit(principal, audit.ActionCommsMessageSend, "message", message.ID, map[string]string{
		"household_id": body.HouseholdID,
		"swimmer_id":   body.SwimmerID,
	}, message)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"message": message})
}

// requestMessageUploadURL handles POST /api/v1/messages/upload-url
func (h *CommsHandlers) requestMessageUploadURL(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if h.store == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "storage_unavailable", "Object storage is not configured.", nil)
		return
	}

	var body struct {
		HouseholdID   string `json:"household_id"     validate:"required"`
		SwimmerID     string `json:"swimmer_id"       validate:"required"`
		FileName      string `json:"file_name"        validate:"required"`
		FileMIME      string `json:"file_mime"        validate:"required"`
		FileSizeBytes int64  `json:"file_size_bytes"  validate:"required,gt=0"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Validation failed.", httpx.FormatValidationErrors(err))
		return
	}
	if !h.authorizeMessageThread(principal, orgID, body.HouseholdID, body.SwimmerID) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to message this thread.", nil)
		return
	}
	fileName, err := sanitizeDocumentFilename(body.FileName)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_filename", err.Error(), nil)
		return
	}
	declaredMIME := strings.ToLower(strings.TrimSpace(body.FileMIME))
	if !allowedMessageAttachmentMIMETypes[declaredMIME] {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_mime_type", "Attachment type is not permitted.", nil)
		return
	}
	if body.FileSizeBytes > maxMessageAttachmentBytes {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "file_too_large", "Declared file size exceeds the maximum allowed size.", nil)
		return
	}

	objectKey := messageAttachmentObjectKey(orgID, body.HouseholdID, body.SwimmerID, fileName)
	expiresIn := 15 * time.Minute
	uploadURL, err := h.store.PresignedPutURL(objectKey, expiresIn)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create upload URL.", err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"upload_url":         uploadURL,
		"object_key":         objectKey,
		"upload_expires_at":  time.Now().UTC().Add(expiresIn),
		"expected_file_name": fileName,
	})
}

func (h *CommsHandlers) listMessages(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	householdID := strings.TrimSpace(r.URL.Query().Get("household_id"))
	swimmerID := strings.TrimSpace(r.URL.Query().Get("swimmer_id"))
	if householdID == "" || swimmerID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "household_id and swimmer_id are required.", nil)
		return
	}
	if !h.authorizeMessageThread(principal, orgID, householdID, swimmerID) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to view this thread.", nil)
		return
	}

	messages := h.service.ListMessages(orgID, householdID, swimmerID, principal.UserID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages, "count": len(messages)})
}

func (h *CommsHandlers) listMessageThreads(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to view the message inbox.", nil)
		return
	}

	threads := h.service.ListMessageThreads(orgID, principal.UserID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"threads": threads, "count": len(threads)})
}

// editMessage handles PATCH /api/v1/messages/{messageID} — sender-only, unrestricted.
func (h *CommsHandlers) editMessage(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	messageID := chi.URLParam(r, "messageID")

	var body struct {
		Body string `json:"body" validate:"required,max=4000"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}

	message, err := h.service.EditMessage(orgID, messageID, principal.UserID, body.Body)
	if err != nil {
		switch {
		case errors.Is(err, comms.ErrMessageNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Message not found.", nil)
		case errors.Is(err, comms.ErrForbidden):
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You may only edit your own messages.", nil)
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, "edit_failed", err.Error(), nil)
		}
		return
	}

	h.emitAudit(principal, audit.ActionCommsMessageEdit, "message", message.ID, map[string]string{
		"household_id": message.HouseholdID,
		"swimmer_id":   message.SwimmerID,
	}, message)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"message": message})
}

// messageThreadForID looks up which household+swimmer thread a message belongs
// to, so reaction/read endpoints (addressed by message ID, not thread) can run
// the same thread-scoped authorization as the rest of the messages API.
func (h *CommsHandlers) messageThreadForID(orgID, messageID string) (householdID, swimmerID string, ok bool) {
	return h.service.MessageThread(orgID, messageID)
}

func (h *CommsHandlers) addReaction(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	messageID := chi.URLParam(r, "messageID")
	householdID, swimmerID, found := h.messageThreadForID(orgID, messageID)
	if !found || !h.authorizeMessageThread(principal, orgID, householdID, swimmerID) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Message not found.", nil)
		return
	}

	var body struct {
		Emoji string `json:"emoji" validate:"required"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.service.AddReaction(orgID, messageID, principal.UserID, body.Emoji); err != nil {
		if errors.Is(err, comms.ErrInvalidReaction) {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_reaction", "That emoji is not supported.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "reaction_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (h *CommsHandlers) removeReaction(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")
	householdID, swimmerID, found := h.messageThreadForID(orgID, messageID)
	if !found || !h.authorizeMessageThread(principal, orgID, householdID, swimmerID) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Message not found.", nil)
		return
	}
	if err := h.service.RemoveReaction(orgID, messageID, principal.UserID, emoji); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "reaction_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// markThreadRead handles POST /api/v1/messages/read?household_id=&swimmer_id=
func (h *CommsHandlers) markThreadRead(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	householdID := strings.TrimSpace(r.URL.Query().Get("household_id"))
	swimmerID := strings.TrimSpace(r.URL.Query().Get("swimmer_id"))
	if householdID == "" || swimmerID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "household_id and swimmer_id are required.", nil)
		return
	}
	if !h.authorizeMessageThread(principal, orgID, householdID, swimmerID) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to view this thread.", nil)
		return
	}
	if err := h.service.MarkThreadRead(orgID, householdID, swimmerID, principal.UserID); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "mark_read_failed", err.Error(), nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// streamMessages handles GET /api/v1/messages/stream — a server-sent-events feed
// of message create/edit/reaction events for the caller's org, filtered to
// threads the caller is authorized to see. Returns 503 if no realtime
// broadcaster is configured (e.g. REDIS_URL unset), so environments without
// Redis keep working exactly as before, just without live push.
func (h *CommsHandlers) streamMessages(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	broadcaster := h.service.Broadcaster()
	if broadcaster == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "realtime_unavailable", "Realtime updates are not available.", nil)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, r, http.StatusInternalServerError, "streaming_unsupported", "Streaming is not supported.", nil)
		return
	}

	isStaff := hasAnyRole(principal, "admin", "coach")

	ctx := r.Context()
	sub := broadcaster.Subscribe(ctx, orgID)
	defer sub.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case msg, chOk := <-ch:
			if !chOk {
				return
			}
			var event comms.MessageEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				continue
			}
			if !isStaff && !h.authorizeMessageThread(principal, orgID, event.HouseholdID, event.SwimmerID) {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg.Payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *CommsHandlers) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to create announcements.", nil)
		return
	}

	var body struct {
		Title    string `json:"title"    validate:"required,max=255"`
		Body     string `json:"body"     validate:"required"`
		Audience string `json:"audience" validate:"required,oneof=all group"`
		GroupID  string `json:"group_id"`
		Public   bool   `json:"public"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
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
		Public:    body.Public,
		CreatedBy: principal.UserID,
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "create_failed", err.Error(), nil)
		return
	}
	h.emitAudit(principal, audit.ActionCommsAnnouncementCreate, "announcement", ann.ID, map[string]string{
		"audience": body.Audience,
	}, ann)
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

func (h *CommsHandlers) setAnnouncementPublic(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to update announcements.", nil)
		return
	}
	var body struct {
		Public *bool `json:"public" validate:"required"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}
	if err := h.validate.Struct(body); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_error", "Request validation failed.", httpx.FormatValidationErrors(err))
		return
	}
	ann, err := h.service.SetAnnouncementPublic(orgID, chi.URLParam(r, "announcementID"), principal.UserID, *body.Public)
	if err != nil {
		if errors.Is(err, comms.ErrAnnouncementNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Announcement not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusBadRequest, "update_failed", err.Error(), nil)
		return
	}
	h.emitAudit(principal, audit.ActionCommsAnnouncementVisibility, "announcement", ann.ID, map[string]string{
		"public": strconv.FormatBool(*body.Public),
	}, ann)
	httpx.WriteJSON(w, http.StatusOK, ann)
}

func (h *CommsHandlers) publishAnnouncement(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
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
	h.emitAudit(principal, audit.ActionCommsAnnouncementPublish, "announcement", ann.ID, nil, ann)
	httpx.WriteJSON(w, http.StatusOK, ann)
}

func (h *CommsHandlers) archiveAnnouncement(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
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
	h.emitAudit(principal, audit.ActionCommsAnnouncementArchive, "announcement", ann.ID, nil, ann)
	httpx.WriteJSON(w, http.StatusOK, ann)
}

// ─── Campaign handlers ────────────────────────────────────────────────────────

func (h *CommsHandlers) createCampaign(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
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
	if err := decodeJSONBody(w, r, &body); err != nil {
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
	h.emitAudit(principal, audit.ActionCommsCampaignCreate, "campaign", campaign.ID, map[string]string{
		"audience": body.Audience,
	}, campaign)
	httpx.WriteJSON(w, http.StatusCreated, campaign)
}

func (h *CommsHandlers) listCampaigns(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
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
	if !hasAnyRole(principal, "admin", "coach") {
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
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to send campaigns.", nil)
		return
	}

	id := chi.URLParam(r, "campaignID")

	var body struct{}
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "Could not parse request body.", nil)
		return
	}

	campaignRecord, err := h.service.GetCampaign(orgID, id)
	if err != nil {
		if errors.Is(err, comms.ErrCampaignNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Campaign not found.", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "send_failed", err.Error(), nil)
		return
	}

	recipients, err := h.resolveCampaignRecipients(orgID, campaignRecord)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "recipient_resolution_failed", err.Error(), nil)
		return
	}
	if len(recipients) == 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "no_eligible_recipients", "No eligible org-member recipients were resolved for this campaign.", nil)
		return
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
	h.emitAudit(principal, audit.ActionCommsCampaignSend, "campaign", campaign.ID, map[string]string{
		"recipient_count": strconv.Itoa(len(recipients)),
	}, campaign)
	httpx.WriteJSON(w, http.StatusOK, campaign)
}

func (h *CommsHandlers) listDeliveries(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
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
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role to manage suppression list.", nil)
		return
	}

	var body struct {
		Email  string `json:"email"  validate:"required,email"`
		Reason string `json:"reason" validate:"required,oneof=bounce complaint manual"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
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
	h.emitAudit(principal, audit.ActionCommsSuppressionAdd, "suppression", entry.ID, map[string]string{
		"reason": body.Reason,
	}, entry)
	httpx.WriteJSON(w, http.StatusCreated, entry)
}

func (h *CommsHandlers) listSuppression(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := commsScopeFromRequest(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
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
	if !hasAnyRole(principal, "admin") {
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
	h.emitAudit(principal, audit.ActionCommsSuppressionRemove, "suppression", id, nil, nil)
	w.WriteHeader(http.StatusNoContent)
}
