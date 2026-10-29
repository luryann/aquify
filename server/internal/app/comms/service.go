//  PROPRIETARY AND CONFIDENTIAL
//  COPYRIGHT (C) 2026 RYAN LU. ALL RIGHTS RESERVED.
//
//  THIS SOURCE FILE IS PART OF A PROPRIETARY, CLOSED-SOURCE SOFTWARE
//  SYSTEM OWNED EXCLUSIVELY BY RYAN LU. THIS FILE AND ALL ASSOCIATED
//  SOURCE CODE, OBJECT CODE, MODULES, COMPONENTS, DOCUMENTATION, AND
//  RELATED MATERIALS ARE CONFIDENTIAL AND PROPRIETARY INTELLECTUAL
//  PROPERTY OF RYAN LU.
//
//  USE OF THIS SOFTWARE IS PERMITTED ONLY FOR PERSONS OR ENTITIES
//  EXPRESSLY AUTHORIZED BY RYAN LU AND ONLY AFTER ACCEPTANCE OF THE
//  APPLICABLE LICENSE TERMS. DARE AQUATICS AND OTHER TEAMS OR
//  ORGANIZATIONS MAY USE THIS SOFTWARE ONLY IF SEPARATELY APPROVED BY
//  RYAN LU.
//
//  NO COPYING, MODIFICATION, DISTRIBUTION, HOSTING, DISCLOSURE,
//  REVERSE ENGINEERING, DECOMPILATION, OR OTHER USE IS PERMITTED
//  EXCEPT AS EXPRESSLY AUTHORIZED IN WRITING BY RYAN LU.
//
//  ALL RIGHTS NOT EXPRESSLY GRANTED ARE RESERVED BY RYAN LU.

// Package comms provides the in-memory service for announcements, email
// campaigns, email delivery tracking, and the suppression list.
//
// Data is held in in-memory maps protected by a RWMutex; with a database pool
// every write persists through the repository first.
package comms

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/dareaquatics/aquify/server/internal/postmark"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrAnnouncementNotFound = errors.New("announcement not found")
	ErrCampaignNotFound     = errors.New("email campaign not found")
	ErrDeliveryNotFound     = errors.New("email delivery not found")
	ErrSuppressionNotFound  = errors.New("suppression entry not found")
	ErrForbidden            = errors.New("forbidden")
	ErrInvalidTransition    = errors.New("invalid state transition")
	ErrDuplicateSuppression = errors.New("email already suppressed for this organisation")
	ErrCampaignNotDraft     = errors.New("campaign must be in draft status to send")
	ErrMessageNotFound      = errors.New("message not found")
	ErrInvalidReaction      = errors.New("emoji is not in the allowed reaction set")
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// Announcement is an org-wide or group-scoped notice visible in the portal.
type Announcement struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Status      string     `json:"status"`   // draft | published | archived
	Audience    string     `json:"audience"` // all | group
	GroupID     string     `json:"group_id,omitempty"`
	Public      bool       `json:"public"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CreatedBy   string     `json:"created_by"`
	UpdatedBy   string     `json:"updated_by"`
}

// Message is a direct note in a household+swimmer thread between a parent/swimmer
// and staff (coach/admin). Threads are scoped per swimmer so a household with
// multiple kids gets a separate conversation for each one.
type Message struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	HouseholdID  string     `json:"household_id"`
	SwimmerID    string     `json:"swimmer_id"`
	SenderUserID string     `json:"sender_user_id"`
	SenderRole   string     `json:"sender_role"`
	Body         string     `json:"body"`
	EditedAt     *time.Time `json:"edited_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// MessageEdit records the prior body of a message each time its sender edits it.
type MessageEdit struct {
	ID           string    `json:"id"`
	MessageID    string    `json:"message_id"`
	PreviousBody string    `json:"previous_body"`
	EditedAt     time.Time `json:"edited_at"`
	EditedBy     string    `json:"edited_by"`
}

// AllowedMessageReactions is the fixed set of emoji a message may be reacted with.
var AllowedMessageReactions = map[string]bool{
	"👍": true, "❤️": true, "😂": true, "🎉": true, "😮": true, "😢": true,
}

// MessageReaction is one user's emoji reaction on a message.
type MessageReaction struct {
	ID        string    `json:"id"`
	MessageID string    `json:"message_id"`
	UserID    string    `json:"user_id"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageAttachment is a file (image) sent along with a message.
type MessageAttachment struct {
	ID        string    `json:"id"`
	MessageID string    `json:"message_id"`
	ObjectKey string    `json:"object_key"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageRead is a per-user read watermark for a household+swimmer thread.
type MessageRead struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	HouseholdID string    `json:"household_id"`
	SwimmerID   string    `json:"swimmer_id"`
	UserID      string    `json:"user_id"`
	LastReadAt  time.Time `json:"last_read_at"`
}

// ReactionSummary aggregates one emoji's reaction count on a message for display.
type ReactionSummary struct {
	Emoji       string `json:"emoji"`
	Count       int    `json:"count"`
	ReactedByMe bool   `json:"reacted_by_me"`
}

// AttachmentView is the read-facing shape of an attachment, with a freshly
// signed download URL rather than the raw object key.
type AttachmentView struct {
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	SizeBytes   int64  `json:"size_bytes"`
	DownloadURL string `json:"download_url,omitempty"`
}

// MessageView is a Message enriched with reactions and attachments for API responses.
type MessageView struct {
	Message
	Reactions   []ReactionSummary `json:"reactions,omitempty"`
	Attachments []AttachmentView  `json:"attachments,omitempty"`
}

// MessageThreadSummary describes one household+swimmer conversation for an inbox view.
type MessageThreadSummary struct {
	HouseholdID    string    `json:"household_id"`
	SwimmerID      string    `json:"swimmer_id"`
	MessageCount   int       `json:"message_count"`
	LastBody       string    `json:"last_body"`
	LastSenderRole string    `json:"last_sender_role"`
	LastMessageAt  time.Time `json:"last_message_at"`
	UnreadCount    int       `json:"unread_count"`
}

// EmailCampaign is a bulk-email job targeting all org members or a group.
type EmailCampaign struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"org_id"`
	Name            string     `json:"name"`
	Subject         string     `json:"subject"`
	BodyHTML        string     `json:"body_html"`
	BodyText        string     `json:"body_text,omitempty"`
	FromName        string     `json:"from_name"`
	FromEmail       string     `json:"from_email"`
	Audience        string     `json:"audience"` // all | group
	GroupID         string     `json:"group_id,omitempty"`
	Status          string     `json:"status"` // draft | sending | sent | failed | cancelled
	SentAt          *time.Time `json:"sent_at,omitempty"`
	TotalRecipients int        `json:"total_recipients"`
	SentCount       int        `json:"sent_count"`
	FailedCount     int        `json:"failed_count"`
	SuppressedCount int        `json:"suppressed_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CreatedBy       string     `json:"created_by"`
	UpdatedBy       string     `json:"updated_by"`
}

// EmailDelivery records a single email send attempt for a recipient.
type EmailDelivery struct {
	ID                string     `json:"id"`
	OrgID             string     `json:"org_id"`
	CampaignID        string     `json:"campaign_id,omitempty"`
	RecipientEmail    string     `json:"recipient_email"`
	RecipientUserID   string     `json:"recipient_user_id,omitempty"`
	Status            string     `json:"status"` // pending | sent | delivered | bounced | failed | suppressed
	ProviderMessageID string     `json:"provider_message_id,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	SentAt            *time.Time `json:"sent_at,omitempty"`
	DeliveredAt       *time.Time `json:"delivered_at,omitempty"`
	BouncedAt         *time.Time `json:"bounced_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// SuppressionEntry records an email address that should never receive sends.
type SuppressionEntry struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Email     string    `json:"email"`
	Reason    string    `json:"reason"` // bounce | complaint | manual
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

// NotificationPreference stores a user's portal communication settings.
type NotificationPreference struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	UserID       string    `json:"user_id"`
	HouseholdID  string    `json:"household_id,omitempty"`
	EventID      string    `json:"event_id"`
	EmailEnabled bool      `json:"email_enabled"`
	PushEnabled  bool      `json:"push_enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	UpdatedBy    string    `json:"updated_by,omitempty"`
}

type NotificationPreferenceSetting struct {
	EventID      string `json:"event_id"`
	EmailEnabled bool   `json:"email_enabled"`
	PushEnabled  bool   `json:"push_enabled"`
}

type NotificationPreferenceView struct {
	EventID      string `json:"event_id"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	EmailEnabled bool   `json:"email_enabled"`
	PushEnabled  bool   `json:"push_enabled"`
}

// ─── Input types ──────────────────────────────────────────────────────────────

// AttachmentInput describes one already-uploaded object to link to a new message.
// The caller (handler layer) must have already validated the object's real MIME
// type/size/existence in storage before constructing this.
type AttachmentInput struct {
	ObjectKey string
	Filename  string
	MimeType  string
	SizeBytes int64
}

// CreateMessageInput holds fields for sending a message into a household+swimmer thread.
type CreateMessageInput struct {
	OrgID        string            `json:"org_id"       validate:"required"`
	HouseholdID  string            `json:"household_id" validate:"required"`
	SwimmerID    string            `json:"swimmer_id"   validate:"required"`
	Body         string            `json:"body"         validate:"required,max=4000"`
	SenderUserID string            `json:"-"`
	SenderRole   string            `json:"-"`
	Attachments  []AttachmentInput `json:"-"`
}

type CreateAnnouncementInput struct {
	OrgID     string `json:"org_id"    validate:"required"`
	Title     string `json:"title"     validate:"required,max=255"`
	Body      string `json:"body"      validate:"required"`
	Audience  string `json:"audience"  validate:"required,oneof=all group"`
	GroupID   string `json:"group_id"`
	Public    bool   `json:"public"`
	CreatedBy string `json:"-"`
}

type CreateCampaignInput struct {
	OrgID     string `json:"org_id"      validate:"required"`
	Name      string `json:"name"        validate:"required,max=255"`
	Subject   string `json:"subject"     validate:"required,max=255"`
	BodyHTML  string `json:"body_html"   validate:"required"`
	BodyText  string `json:"body_text"`
	FromName  string `json:"from_name"   validate:"required"`
	FromEmail string `json:"from_email"  validate:"required,email"`
	Audience  string `json:"audience"    validate:"required,oneof=all group"`
	GroupID   string `json:"group_id"`
	CreatedBy string `json:"-"`
}

// SendCampaignInput contains the recipient list and the actor sending the campaign.
// In a production system the recipients would be resolved from DB queries;
// here they are passed in directly by the handler (using mocked members).
type SendCampaignInput struct {
	// Recipients is the full candidate list before suppression filtering.
	Recipients []Recipient
	ActorID    string
}

// Recipient is a single addressee for a campaign send.
type Recipient struct {
	Email  string
	UserID string // optional
}

type AddSuppressionInput struct {
	OrgID     string `json:"org_id"   validate:"required"`
	Email     string `json:"email"    validate:"required,email"`
	Reason    string `json:"reason"   validate:"required,oneof=bounce complaint manual"`
	CreatedBy string `json:"-"`
}

// PostmarkEventInput represents an inbound Postmark delivery-state webhook.
type PostmarkEventInput struct {
	OrgID             string
	MessageID         string
	RecipientEmail    string
	RecordType        string
	OccurredAt        time.Time
	Details           string
	SuppressionReason string
	SuppressSending   bool
}

type notificationPreferenceDefinition struct {
	EventID      string
	Label        string
	Description  string
	DefaultEmail bool
	DefaultPush  bool
}

var notificationPreferenceDefinitions = []notificationPreferenceDefinition{
	{EventID: "meet_commitment", Label: "Meet Commitment Requests", Description: "When a coach sends a new meet invitation", DefaultEmail: true, DefaultPush: true},
	{EventID: "invoice_due", Label: "Invoice Due Reminders", Description: "When an invoice is due or overdue", DefaultEmail: true, DefaultPush: true},
	{EventID: "announcement", Label: "Club Announcements", Description: "General announcements from club staff", DefaultEmail: true, DefaultPush: false},
	{EventID: "doc_reminder", Label: "Document Reminders", Description: "When a required document is expiring", DefaultEmail: true, DefaultPush: false},
	{EventID: "entry_update", Label: "Entry Updates", Description: "When your swimmer's meet entries change", DefaultEmail: false, DefaultPush: true},
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service is the in-memory communications service.
type Service struct {
	mu                 sync.RWMutex
	repo               *Repository
	announcements      map[string]*Announcement
	campaigns          map[string]*EmailCampaign
	deliveries         map[string]*EmailDelivery
	suppression        map[string]*SuppressionEntry // key: orgID|email (lowercase)
	preferences        map[string]*NotificationPreference
	messages           map[string]*Message
	messageEdits       map[string]*MessageEdit
	messageReactions   map[string]*MessageReaction
	messageAttachments map[string]*MessageAttachment
	messageReads       map[string]*MessageRead // key: orgID|householdID|swimmerID|userID

	postmarkClient postmark.Client
	store          storage.ObjectStorage
	broadcaster    *MessageBroadcaster
}

// NewService returns a new in-memory Service with a stub Postmark client.
func NewService() *Service {
	return &Service{
		announcements:      make(map[string]*Announcement),
		campaigns:          make(map[string]*EmailCampaign),
		deliveries:         make(map[string]*EmailDelivery),
		suppression:        make(map[string]*SuppressionEntry),
		preferences:        make(map[string]*NotificationPreference),
		messages:           make(map[string]*Message),
		messageEdits:       make(map[string]*MessageEdit),
		messageReactions:   make(map[string]*MessageReaction),
		messageAttachments: make(map[string]*MessageAttachment),
		messageReads:       make(map[string]*MessageRead),
		postmarkClient:     postmark.NewStubClient(),
	}
}

// SetStorage attaches object storage used to sign message-attachment download URLs.
func (s *Service) SetStorage(store storage.ObjectStorage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

// SetBroadcaster attaches a realtime broadcaster used to publish message events.
// A nil broadcaster (the default) simply means no realtime push is available —
// sends/edits/reactions still persist normally.
func (s *Service) SetBroadcaster(b *MessageBroadcaster) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcaster = b
}

// Broadcaster returns the currently configured realtime broadcaster, or nil.
func (s *Service) Broadcaster() *MessageBroadcaster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.broadcaster
}

func messageReadKey(orgID, householdID, swimmerID, userID string) string {
	return orgID + "|" + householdID + "|" + swimmerID + "|" + userID
}

// NewServiceWithClient returns a Service using the provided Postmark client.
func NewServiceWithClient(client postmark.Client) *Service {
	svc := NewService()
	svc.postmarkClient = client
	return svc
}

// SetPostmarkClient replaces the Postmark client on an existing Service.
func (s *Service) SetPostmarkClient(client postmark.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.postmarkClient = client
}

// PostmarkClient returns the currently configured Postmark client.
func (s *Service) PostmarkClient() postmark.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.postmarkClient
}

// NewServiceWithPool creates a DB-backed comms Service and loads existing data
// from Postgres into memory on startup.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("comms: failed to load from DB", "err", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	announcements, campaigns, deliveries, suppressions, preferences, messages,
		messageEdits, messageReactions, messageAttachments, messageReads, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range announcements {
		s.announcements[a.ID] = a
	}
	for _, c := range campaigns {
		s.campaigns[c.ID] = c
	}
	for _, d := range deliveries {
		s.deliveries[d.ID] = d
	}
	for _, e := range suppressions {
		s.suppression[suppressionKey(e.OrgID, e.Email)] = e
	}
	for _, pref := range preferences {
		s.preferences[notificationPreferenceKey(pref.OrgID, pref.UserID, pref.EventID)] = pref
	}
	for _, m := range messages {
		s.messages[m.ID] = m
	}
	for _, e := range messageEdits {
		s.messageEdits[e.ID] = e
	}
	for _, react := range messageReactions {
		s.messageReactions[react.ID] = react
	}
	for _, a := range messageAttachments {
		s.messageAttachments[a.ID] = a
	}
	for _, read := range messageReads {
		s.messageReads[messageReadKey(read.OrgID, read.HouseholdID, read.SwimmerID, read.UserID)] = read
	}
	return nil
}

func (s *Service) newID() string {
	if s.repo != nil {
		return uuid.New().String()
	}
	return dbutil.NewHexID()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func suppressionKey(orgID, email string) string {
	return orgID + "|" + strings.ToLower(strings.TrimSpace(email))
}

func notificationPreferenceKey(orgID, userID, eventID string) string {
	return strings.TrimSpace(orgID) + "|" + strings.TrimSpace(userID) + "|" + strings.TrimSpace(eventID)
}

func notificationPreferenceDefinitionByID(eventID string) (notificationPreferenceDefinition, bool) {
	needle := strings.TrimSpace(eventID)
	for _, definition := range notificationPreferenceDefinitions {
		if definition.EventID == needle {
			return definition, true
		}
	}
	return notificationPreferenceDefinition{}, false
}

// isSuppressed returns true when the given email is in the suppression list.
// Caller must hold s.mu (at minimum RLock) before calling.
func (s *Service) isSuppressed(orgID, email string) bool {
	_, ok := s.suppression[suppressionKey(orgID, email)]
	return ok
}

func (s *Service) listNotificationPreferencesLocked(orgID, userID string) []NotificationPreferenceView {
	out := make([]NotificationPreferenceView, 0, len(notificationPreferenceDefinitions))
	for _, definition := range notificationPreferenceDefinitions {
		view := NotificationPreferenceView{
			EventID:      definition.EventID,
			Label:        definition.Label,
			Description:  definition.Description,
			EmailEnabled: definition.DefaultEmail,
			PushEnabled:  definition.DefaultPush,
		}
		if pref, ok := s.preferences[notificationPreferenceKey(orgID, userID, definition.EventID)]; ok && pref != nil {
			view.EmailEnabled = pref.EmailEnabled
			view.PushEnabled = pref.PushEnabled
		}
		out = append(out, view)
	}
	return out
}

// ListNotificationPreferences returns the caller's effective notification settings.
func (s *Service) ListNotificationPreferences(orgID, userID string) []NotificationPreferenceView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listNotificationPreferencesLocked(orgID, userID)
}

// ReplaceNotificationPreferences upserts the caller's notification settings.
func (s *Service) ReplaceNotificationPreferences(orgID, userID, householdID, actorID string, settings []NotificationPreferenceSetting) ([]NotificationPreferenceView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate every setting before writing any, so a bad request changes nothing.
	seen := make(map[string]struct{}, len(settings))
	definitions := make([]notificationPreferenceDefinition, len(settings))
	for i, setting := range settings {
		definition, ok := notificationPreferenceDefinitionByID(setting.EventID)
		if !ok {
			return nil, fmt.Errorf("unsupported notification event %q", setting.EventID)
		}
		if _, exists := seen[definition.EventID]; exists {
			return nil, fmt.Errorf("duplicate notification event %q", definition.EventID)
		}
		seen[definition.EventID] = struct{}{}
		definitions[i] = definition
	}

	now := time.Now().UTC()
	for i, setting := range settings {
		definition := definitions[i]
		key := notificationPreferenceKey(orgID, userID, definition.EventID)
		// Edit a copy; the cached preference is replaced only after it is saved.
		pref := &NotificationPreference{ID: s.newID(), OrgID: orgID, UserID: userID, CreatedAt: now}
		if existing := s.preferences[key]; existing != nil {
			cp := *existing
			pref = &cp
		}
		pref.HouseholdID = strings.TrimSpace(householdID)
		pref.EventID = definition.EventID
		pref.EmailEnabled = setting.EmailEnabled
		pref.PushEnabled = setting.PushEnabled
		pref.UpdatedAt = now
		pref.UpdatedBy = actorID
		if pref.CreatedAt.IsZero() {
			pref.CreatedAt = now
		}
		if s.repo != nil {
			if err := s.repo.UpsertNotificationPreference(context.Background(), pref); err != nil {
				return nil, fmt.Errorf("persist notification preference: %w", err)
			}
		}
		s.preferences[key] = pref
	}

	return s.listNotificationPreferencesLocked(orgID, userID), nil
}

// ─── Announcement operations ─────────────────────────────────────────────────

// CreateAnnouncement creates a new announcement in draft state.
func (s *Service) CreateAnnouncement(inp CreateAnnouncementInput) (*Announcement, error) {
	if inp.Audience == "group" && strings.TrimSpace(inp.GroupID) == "" {
		return nil, errors.New("group_id is required when audience is 'group'")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	a := &Announcement{
		ID:        s.newID(),
		OrgID:     inp.OrgID,
		Title:     strings.TrimSpace(inp.Title),
		Body:      strings.TrimSpace(inp.Body),
		Status:    "draft",
		Audience:  inp.Audience,
		GroupID:   inp.GroupID,
		Public:    inp.Public && inp.Audience == "all",
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: inp.CreatedBy,
		UpdatedBy: inp.CreatedBy,
	}
	if s.repo != nil {
		if err := s.repo.UpsertAnnouncement(context.Background(), a); err != nil {
			return nil, fmt.Errorf("persist announcement: %w", err)
		}
	}
	s.announcements[a.ID] = a
	return a, nil
}

// SetAnnouncementPublic toggles whether a published announcement appears on
// the public team website. Group-scoped announcements can never be public.
func (s *Service) SetAnnouncementPublic(orgID, id, actorID string, public bool) (*Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.announcements[id]
	if !ok || a.OrgID != orgID {
		return nil, ErrAnnouncementNotFound
	}
	if public && a.Audience != "all" {
		return nil, errors.New("only all-club announcements can be public")
	}
	next := *a
	next.Public = public
	next.UpdatedAt = time.Now().UTC()
	next.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.UpsertAnnouncement(context.Background(), &next); err != nil {
			return nil, fmt.Errorf("persist announcement: %w", err)
		}
	}
	*a = next
	cp := next
	return &cp, nil
}

// ListPublicAnnouncements returns published, explicitly public, all-club
// announcements for an org, newest first.
func (s *Service) ListPublicAnnouncements(orgID string) []Announcement {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Announcement{}
	for _, a := range s.announcements {
		if a.OrgID == orgID && a.Public && a.Status == "published" && a.Audience == "all" {
			out = append(out, *a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		pi, pj := out[i].CreatedAt, out[j].CreatedAt
		if out[i].PublishedAt != nil {
			pi = *out[i].PublishedAt
		}
		if out[j].PublishedAt != nil {
			pj = *out[j].PublishedAt
		}
		return pi.After(pj)
	})
	return out
}

// PublishAnnouncement transitions an announcement from draft → published.
func (s *Service) PublishAnnouncement(orgID, id, actorID string) (*Announcement, error) {
	return s.transitionAnnouncement(orgID, id, actorID, "published",
		func(status string) bool { return status == "draft" },
		func(a *Announcement, now time.Time) {
			a.Status = "published"
			a.PublishedAt = &now
		})
}

// ArchiveAnnouncement transitions an announcement to archived.
func (s *Service) ArchiveAnnouncement(orgID, id, actorID string) (*Announcement, error) {
	return s.transitionAnnouncement(orgID, id, actorID, "archived",
		func(status string) bool { return status != "archived" },
		func(a *Announcement, now time.Time) {
			a.Status = "archived"
			a.ArchivedAt = &now
		})
}

// transitionAnnouncement applies a status change to a copy of the announcement
// and persists it before the cached record is replaced, so a failed write
// leaves the in-memory state unchanged. label names the new state in errors.
func (s *Service) transitionAnnouncement(
	orgID, id, actorID, label string,
	allowed func(status string) bool,
	apply func(a *Announcement, now time.Time),
) (*Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	a, ok := s.announcements[id]
	if !ok || a.OrgID != orgID {
		return nil, ErrAnnouncementNotFound
	}
	if !allowed(a.Status) {
		return nil, ErrInvalidTransition
	}

	now := time.Now().UTC()
	cp := *a
	apply(&cp, now)
	cp.UpdatedAt = now
	cp.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.UpsertAnnouncement(context.Background(), &cp); err != nil {
			return nil, fmt.Errorf("persist %s announcement: %w", label, err)
		}
	}
	s.announcements[id] = &cp
	return &cp, nil
}

// GetAnnouncement returns a single announcement.
func (s *Service) GetAnnouncement(orgID, id string) (*Announcement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.announcements[id]
	if !ok || a.OrgID != orgID {
		return nil, ErrAnnouncementNotFound
	}
	cp := *a // callers must not share the cached record that later writes replace
	return &cp, nil
}

// ListAnnouncements returns all announcements for an org, newest first.
// status filter is optional (empty = all).
func (s *Service) ListAnnouncements(orgID, statusFilter string) ([]*Announcement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Announcement, 0)
	for _, a := range s.announcements {
		if a.OrgID != orgID {
			continue
		}
		if statusFilter != "" && a.Status != statusFilter {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// ─── Message operations ───────────────────────────────────────────────────────

// MessageThread returns which household+swimmer thread a message belongs to, so
// message-ID-addressed endpoints (reactions) can run the same thread-scoped
// authorization used everywhere else in the messages API.
func (s *Service) MessageThread(orgID, messageID string) (householdID, swimmerID string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, found := s.messages[messageID]
	if !found || m.OrgID != orgID {
		return "", "", false
	}
	return m.HouseholdID, m.SwimmerID, true
}

// SendMessage appends a message to a household+swimmer thread, optionally with
// attachments that have already been uploaded and validated by the caller.
func (s *Service) SendMessage(inp CreateMessageInput) (*Message, error) {
	s.mu.Lock()

	now := time.Now().UTC()
	m := &Message{
		ID:           s.newID(),
		OrgID:        inp.OrgID,
		HouseholdID:  inp.HouseholdID,
		SwimmerID:    inp.SwimmerID,
		SenderUserID: inp.SenderUserID,
		SenderRole:   inp.SenderRole,
		Body:         strings.TrimSpace(inp.Body),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if s.repo != nil {
		if err := s.repo.UpsertMessage(context.Background(), m); err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("persist message: %w", err)
		}
	}
	s.messages[m.ID] = m

	for _, in := range inp.Attachments {
		a := &MessageAttachment{
			ID:        s.newID(),
			MessageID: m.ID,
			ObjectKey: in.ObjectKey,
			Filename:  in.Filename,
			MimeType:  in.MimeType,
			SizeBytes: in.SizeBytes,
			CreatedAt: now,
		}
		if s.repo != nil {
			if err := s.repo.InsertMessageAttachment(context.Background(), a); err != nil {
				s.mu.Unlock()
				return nil, fmt.Errorf("persist message attachment: %w", err)
			}
		}
		s.messageAttachments[a.ID] = a
	}
	broadcaster := s.broadcaster
	view := s.buildMessageView(m, inp.SenderUserID)
	s.mu.Unlock()

	publishMessageEvent(broadcaster, inp.OrgID, "message.created", m, view)
	return m, nil
}

// publishMessageEvent pushes a realtime event for m's thread (Publish ignores a nil broadcaster).
func publishMessageEvent(b *MessageBroadcaster, orgID, eventType string, m *Message, view MessageView) {
	b.Publish(context.Background(), orgID, MessageEvent{
		Type:        eventType,
		HouseholdID: m.HouseholdID,
		SwimmerID:   m.SwimmerID,
		Message:     view,
	})
}

// buildMessageView enriches a Message with its reactions and attachments for API
// responses. Caller must hold at least a read lock, or call before/after locking.
func (s *Service) buildMessageView(m *Message, viewerUserID string) MessageView {
	counts := make(map[string]int)
	reactedByMe := make(map[string]bool)
	for _, react := range s.messageReactions {
		if react.MessageID != m.ID {
			continue
		}
		counts[react.Emoji]++
		if react.UserID == viewerUserID {
			reactedByMe[react.Emoji] = true
		}
	}
	reactions := make([]ReactionSummary, 0, len(counts))
	for emoji, count := range counts {
		reactions = append(reactions, ReactionSummary{Emoji: emoji, Count: count, ReactedByMe: reactedByMe[emoji]})
	}
	sort.Slice(reactions, func(i, j int) bool { return reactions[i].Emoji < reactions[j].Emoji })

	attachments := make([]AttachmentView, 0)
	for _, a := range s.messageAttachments {
		if a.MessageID != m.ID {
			continue
		}
		view := AttachmentView{Filename: a.Filename, MimeType: a.MimeType, SizeBytes: a.SizeBytes}
		if s.store != nil {
			if url, err := s.store.PresignedGetURL(a.ObjectKey, 15*time.Minute); err == nil {
				view.DownloadURL = url
			}
		}
		attachments = append(attachments, view)
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].Filename < attachments[j].Filename })

	return MessageView{Message: *m, Reactions: reactions, Attachments: attachments}
}

// ListMessages returns every message in a household+swimmer thread, oldest first,
// enriched with reactions/attachments and reacted-by-me flags for viewerUserID.
func (s *Service) ListMessages(orgID, householdID, swimmerID, viewerUserID string) []MessageView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := make([]*Message, 0)
	for _, m := range s.messages {
		if m.OrgID != orgID || m.HouseholdID != householdID || m.SwimmerID != swimmerID {
			continue
		}
		msgs = append(msgs, m)
	}
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].CreatedAt.Before(msgs[j].CreatedAt)
	})

	out := make([]MessageView, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, s.buildMessageView(m, viewerUserID))
	}
	return out
}

// ListMessageThreads returns one summary row per household+swimmer thread for an
// org, most recently active first — used for a coach/admin inbox view. UnreadCount
// is computed relative to viewerUserID's read watermark for each thread.
func (s *Service) ListMessageThreads(orgID, viewerUserID string) []MessageThreadSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type key struct{ householdID, swimmerID string }
	threads := make(map[key]*MessageThreadSummary)
	for _, m := range s.messages {
		if m.OrgID != orgID {
			continue
		}
		k := key{m.HouseholdID, m.SwimmerID}
		summary, ok := threads[k]
		if !ok {
			summary = &MessageThreadSummary{HouseholdID: m.HouseholdID, SwimmerID: m.SwimmerID}
			threads[k] = summary
		}
		summary.MessageCount++
		if m.CreatedAt.After(summary.LastMessageAt) {
			summary.LastMessageAt = m.CreatedAt
			summary.LastBody = m.Body
			summary.LastSenderRole = m.SenderRole
		}
		watermark := s.messageReads[messageReadKey(orgID, m.HouseholdID, m.SwimmerID, viewerUserID)]
		if m.SenderUserID != viewerUserID && (watermark == nil || m.CreatedAt.After(watermark.LastReadAt)) {
			summary.UnreadCount++
		}
	}

	out := make([]MessageThreadSummary, 0, len(threads))
	for _, summary := range threads {
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastMessageAt.After(out[j].LastMessageAt)
	})
	return out
}

// EditMessage updates a message's body, sender-only, keeping a history entry of
// the prior body. Edits are unrestricted (no time window).
func (s *Service) EditMessage(orgID, messageID, userID, newBody string) (*Message, error) {
	s.mu.Lock()

	current, ok := s.messages[messageID]
	if !ok || current.OrgID != orgID {
		s.mu.Unlock()
		return nil, ErrMessageNotFound
	}
	if current.SenderUserID != userID {
		s.mu.Unlock()
		return nil, ErrForbidden
	}

	now := time.Now().UTC()
	edit := &MessageEdit{
		ID:           s.newID(),
		MessageID:    current.ID,
		PreviousBody: current.Body,
		EditedAt:     now,
		EditedBy:     userID,
	}
	// Edit a copy so a failed write leaves the cached message untouched.
	m := new(Message)
	*m = *current
	m.Body = strings.TrimSpace(newBody)
	m.EditedAt = &now
	m.UpdatedAt = now

	if s.repo != nil {
		if err := s.repo.InsertMessageEdit(context.Background(), edit); err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("persist message edit: %w", err)
		}
		if err := s.repo.UpdateMessageBody(context.Background(), m.ID, m.Body, now); err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("persist message body: %w", err)
		}
	}
	s.messages[messageID] = m
	s.messageEdits[edit.ID] = edit
	broadcaster := s.broadcaster
	view := s.buildMessageView(m, userID)
	s.mu.Unlock()

	publishMessageEvent(broadcaster, orgID, "message.updated", m, view)
	return m, nil
}

// AddReaction records userID's emoji reaction on a message. Adding the same
// (message, user, emoji) twice is a no-op.
func (s *Service) AddReaction(orgID, messageID, userID, emoji string) error {
	if !AllowedMessageReactions[emoji] {
		return ErrInvalidReaction
	}
	s.mu.Lock()

	m, ok := s.messages[messageID]
	if !ok || m.OrgID != orgID {
		s.mu.Unlock()
		return ErrMessageNotFound
	}
	for _, react := range s.messageReactions {
		if react.MessageID == messageID && react.UserID == userID && react.Emoji == emoji {
			s.mu.Unlock()
			return nil
		}
	}
	react := &MessageReaction{ID: s.newID(), MessageID: messageID, UserID: userID, Emoji: emoji, CreatedAt: time.Now().UTC()}
	if s.repo != nil {
		if err := s.repo.UpsertMessageReaction(context.Background(), react); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("persist reaction: %w", err)
		}
	}
	s.messageReactions[react.ID] = react
	broadcaster := s.broadcaster
	view := s.buildMessageView(m, userID)
	s.mu.Unlock()

	publishMessageEvent(broadcaster, orgID, "message.reaction_added", m, view)
	return nil
}

// RemoveReaction removes userID's own emoji reaction from a message, if present.
func (s *Service) RemoveReaction(orgID, messageID, userID, emoji string) error {
	s.mu.Lock()

	m, ok := s.messages[messageID]
	if !ok || m.OrgID != orgID {
		s.mu.Unlock()
		return ErrMessageNotFound
	}
	if s.repo != nil {
		if err := s.repo.DeleteMessageReaction(context.Background(), messageID, userID, emoji); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("delete reaction: %w", err)
		}
	}
	for id, react := range s.messageReactions {
		if react.MessageID == messageID && react.UserID == userID && react.Emoji == emoji {
			delete(s.messageReactions, id)
		}
	}
	broadcaster := s.broadcaster
	view := s.buildMessageView(m, userID)
	s.mu.Unlock()

	publishMessageEvent(broadcaster, orgID, "message.reaction_removed", m, view)
	return nil
}

// MarkThreadRead records that userID has read a household+swimmer thread as of now.
func (s *Service) MarkThreadRead(orgID, householdID, swimmerID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := messageReadKey(orgID, householdID, swimmerID, userID)
	existing := s.messageReads[key]
	now := time.Now().UTC()
	read := &MessageRead{
		ID:          s.newID(),
		OrgID:       orgID,
		HouseholdID: householdID,
		SwimmerID:   swimmerID,
		UserID:      userID,
		LastReadAt:  now,
	}
	if existing != nil {
		read.ID = existing.ID
	}
	if s.repo != nil {
		if err := s.repo.UpsertMessageRead(context.Background(), read); err != nil {
			return fmt.Errorf("persist message read: %w", err)
		}
	}
	s.messageReads[key] = read
	return nil
}

// ─── Campaign operations ──────────────────────────────────────────────────────

// CreateCampaign creates a new email campaign in draft state.
func (s *Service) CreateCampaign(inp CreateCampaignInput) (*EmailCampaign, error) {
	if inp.Audience == "group" && strings.TrimSpace(inp.GroupID) == "" {
		return nil, errors.New("group_id is required when audience is 'group'")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	c := &EmailCampaign{
		ID:        s.newID(),
		OrgID:     inp.OrgID,
		Name:      strings.TrimSpace(inp.Name),
		Subject:   strings.TrimSpace(inp.Subject),
		BodyHTML:  inp.BodyHTML,
		BodyText:  inp.BodyText,
		FromName:  strings.TrimSpace(inp.FromName),
		FromEmail: strings.ToLower(strings.TrimSpace(inp.FromEmail)),
		Audience:  inp.Audience,
		GroupID:   inp.GroupID,
		Status:    "draft",
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: inp.CreatedBy,
		UpdatedBy: inp.CreatedBy,
	}
	if s.repo != nil {
		if err := s.repo.UpsertCampaign(context.Background(), c); err != nil {
			return nil, fmt.Errorf("persist campaign: %w", err)
		}
	}
	s.campaigns[c.ID] = c
	return c, nil
}

// GetCampaign returns a single email campaign.
func (s *Service) GetCampaign(orgID, id string) (*EmailCampaign, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.campaigns[id]
	if !ok || c.OrgID != orgID {
		return nil, ErrCampaignNotFound
	}
	cp := *c // the send path mutates the cached campaign in place
	return &cp, nil
}

// ListCampaigns returns all campaigns for an org, newest first.
func (s *Service) ListCampaigns(orgID string) ([]*EmailCampaign, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*EmailCampaign, 0)
	for _, c := range s.campaigns {
		if c.OrgID == orgID {
			cp := *c
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// revertCampaignSendFailure moves a campaign that was flipped to "sending"
// back to "draft" (so it can be retried) when a mid-send persistence error
// prevents the send loop from reaching its normal terminal-status update.
// Persistence of the reverted status is best-effort: if it also fails, the
// original error is still returned to the caller rather than being masked.
func (s *Service) revertCampaignSendFailure(c *EmailCampaign, actorID string, sendErr error) error {
	s.mu.Lock()
	c.Status = "draft"
	c.UpdatedAt = time.Now().UTC()
	c.UpdatedBy = actorID
	s.mu.Unlock()

	if s.repo != nil {
		_ = s.repo.UpsertCampaign(context.Background(), c)
	}
	return sendErr
}

// SendCampaign sends a campaign to the provided recipient list, enforcing
// the suppression list. The campaign must be in draft status.
func (s *Service) SendCampaign(ctx context.Context, orgID, campaignID string, inp SendCampaignInput) (*EmailCampaign, error) {
	s.mu.Lock()
	c, ok := s.campaigns[campaignID]
	if !ok || c.OrgID != orgID {
		s.mu.Unlock()
		return nil, ErrCampaignNotFound
	}
	if c.Status != "draft" {
		s.mu.Unlock()
		return nil, ErrCampaignNotDraft
	}
	// Mark as sending while we hold the lock so concurrent sends are rejected.
	c.Status = "sending"
	c.UpdatedAt = time.Now().UTC()
	c.UpdatedBy = inp.ActorID
	s.mu.Unlock()

	var (
		sentCount       int
		failedCount     int
		suppressedCount int
		processedCount  int
	)

	for _, r := range inp.Recipients {
		email := strings.ToLower(strings.TrimSpace(r.Email))
		if email == "" {
			continue
		}
		processedCount++

		d := &EmailDelivery{
			ID:              s.newID(),
			OrgID:           orgID,
			CampaignID:      campaignID,
			RecipientEmail:  email,
			RecipientUserID: r.UserID,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		}

		s.mu.RLock()
		suppressed := s.isSuppressed(orgID, email)
		s.mu.RUnlock()

		if suppressed {
			d.Status = "suppressed"
			suppressedCount++
		} else {
			resp, err := s.postmarkClient.SendEmail(ctx, postmark.SendEmailRequest{
				To:        email,
				Subject:   c.Subject,
				HTMLBody:  c.BodyHTML,
				TextBody:  c.BodyText,
				FromName:  c.FromName,
				FromEmail: c.FromEmail,
				Metadata: map[string]string{
					"org_id":      orgID,
					"campaign_id": campaignID,
				},
			})
			now := time.Now().UTC()
			if err != nil {
				d.Status = "failed"
				d.ErrorMessage = err.Error()
				failedCount++
			} else {
				d.Status = "sent"
				d.ProviderMessageID = resp.MessageID
				d.SentAt = &now
				sentCount++
			}
			d.UpdatedAt = now
		}

		s.mu.Lock()
		s.deliveries[d.ID] = d
		if s.repo != nil {
			if err := s.repo.UpsertDelivery(context.Background(), d); err != nil {
				s.mu.Unlock()
				return nil, s.revertCampaignSendFailure(c, inp.ActorID, fmt.Errorf("persist email delivery: %w", err))
			}
		}
		s.mu.Unlock()
	}

	// Determine terminal status: "failed" when all non-suppressed sends failed
	// (i.e. nothing was actually delivered and nothing was suppressed to account
	// for the zero sent_count).
	finalStatus := "sent"
	if sentCount == 0 && suppressedCount == 0 && processedCount > 0 {
		finalStatus = "failed"
	}

	now := time.Now().UTC()
	s.mu.Lock()
	c.Status = finalStatus
	c.SentAt = &now
	c.TotalRecipients = processedCount
	c.SentCount = sentCount
	c.FailedCount = failedCount
	c.SuppressedCount = suppressedCount
	c.UpdatedAt = now
	c.UpdatedBy = inp.ActorID
	result := *c // hand back a copy; the cached campaign stays under the service lock
	s.mu.Unlock()

	if s.repo != nil {
		if err := s.repo.UpsertCampaign(context.Background(), &result); err != nil {
			return nil, fmt.Errorf("persist sent campaign: %w", err)
		}
	}
	return &result, nil
}

// ListDeliveries returns all delivery records for a campaign.
func (s *Service) ListDeliveries(orgID, campaignID string) ([]*EmailDelivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Verify campaign ownership.
	c, ok := s.campaigns[campaignID]
	if !ok || c.OrgID != orgID {
		return nil, ErrCampaignNotFound
	}

	out := make([]*EmailDelivery, 0)
	for _, d := range s.deliveries {
		if d.CampaignID == campaignID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ApplyPostmarkEvent updates delivery and suppression state from a Postmark webhook.
func (s *Service) ApplyPostmarkEvent(inp PostmarkEventInput) error {
	email := strings.ToLower(strings.TrimSpace(inp.RecipientEmail))
	recordType := strings.TrimSpace(inp.RecordType)
	orgID := strings.TrimSpace(inp.OrgID)

	s.mu.Lock()
	defer s.mu.Unlock()

	var delivery *EmailDelivery
	if strings.TrimSpace(inp.MessageID) != "" {
		for _, candidate := range s.deliveries {
			if candidate.ProviderMessageID != inp.MessageID {
				continue
			}
			if email != "" && !strings.EqualFold(candidate.RecipientEmail, email) {
				continue
			}
			delivery = candidate
			if orgID == "" {
				orgID = candidate.OrgID
			}
			break
		}
	}

	switch recordType {
	case "Delivery":
		if delivery == nil {
			return ErrDeliveryNotFound
		}
		now := occurredAtOrNow(inp.OccurredAt)
		delivery.Status = "delivered"
		delivery.DeliveredAt = &now
		delivery.UpdatedAt = now
		delivery.ErrorMessage = inp.Details
		if s.repo != nil {
			if err := s.repo.UpsertDelivery(context.Background(), delivery); err != nil {
				return fmt.Errorf("persist delivered email delivery: %w", err)
			}
		}
		return nil
	case "Bounce", "SpamComplaint":
		if delivery == nil && (orgID == "" || email == "") {
			return ErrDeliveryNotFound
		}
		reason, label := "bounce", "bounced"
		if recordType == "SpamComplaint" {
			reason, label = "complaint", "complained"
		}
		now := occurredAtOrNow(inp.OccurredAt)
		if delivery != nil {
			delivery.Status = "bounced"
			delivery.BouncedAt = &now
			delivery.UpdatedAt = now
			delivery.ErrorMessage = inp.Details
			if s.repo != nil {
				if err := s.repo.UpsertDelivery(context.Background(), delivery); err != nil {
					return fmt.Errorf("persist %s email delivery: %w", label, err)
				}
			}
		}
		if orgID != "" && email != "" {
			s.upsertSuppressionLocked(orgID, email, reason, "postmark-webhook")
		}
		return nil
	case "SubscriptionChange":
		if orgID == "" || email == "" {
			return nil
		}
		if inp.SuppressSending {
			s.upsertSuppressionLocked(orgID, email, normalizeSuppressionReason(inp.SuppressionReason), "postmark-webhook")
		} else {
			s.removeSuppressionByEmailLocked(orgID, email)
		}
		return nil
	default:
		return nil
	}
}

// ─── Suppression operations ───────────────────────────────────────────────────

// AddSuppression adds an email to the suppression list for the org.
func (s *Service) AddSuppression(inp AddSuppressionInput) (*SuppressionEntry, error) {
	email := strings.ToLower(strings.TrimSpace(inp.Email))

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addSuppressionLocked(inp.OrgID, email, inp.Reason, inp.CreatedBy)
}

// RemoveSuppression removes an entry from the suppression list by ID.
func (s *Service) RemoveSuppression(orgID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, entry := range s.suppression {
		if entry.ID == id && entry.OrgID == orgID {
			if s.repo != nil {
				if err := s.repo.DeleteSuppressionEntry(context.Background(), orgID, entry.Email); err != nil {
					return fmt.Errorf("delete suppression entry: %w", err)
				}
			}
			delete(s.suppression, key)
			return nil
		}
	}
	return ErrSuppressionNotFound
}

// ListSuppression returns all suppression entries for an org.
func (s *Service) ListSuppression(orgID string) ([]*SuppressionEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*SuppressionEntry, 0)
	for _, e := range s.suppression {
		if e.OrgID == orgID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Service) addSuppressionLocked(orgID, email, reason, createdBy string) (*SuppressionEntry, error) {
	key := suppressionKey(orgID, email)
	if _, exists := s.suppression[key]; exists {
		return nil, ErrDuplicateSuppression
	}
	entry := &SuppressionEntry{
		ID:        s.newID(),
		OrgID:     orgID,
		Email:     email,
		Reason:    reason,
		CreatedAt: time.Now().UTC(),
		CreatedBy: createdBy,
	}
	if s.repo != nil {
		if err := s.repo.UpsertSuppressionEntry(context.Background(), entry); err != nil {
			return nil, fmt.Errorf("persist suppression entry: %w", err)
		}
	}
	s.suppression[key] = entry
	return entry, nil
}

func (s *Service) upsertSuppressionLocked(orgID, email, reason, createdBy string) {
	key := suppressionKey(orgID, email)
	if existing, ok := s.suppression[key]; ok {
		existing.Reason = reason
		if s.repo != nil {
			_ = s.repo.UpsertSuppressionEntry(context.Background(), existing)
		}
		return
	}
	_, _ = s.addSuppressionLocked(orgID, email, reason, createdBy)
}

func (s *Service) removeSuppressionByEmailLocked(orgID, email string) {
	if s.repo != nil {
		_ = s.repo.DeleteSuppressionEntry(context.Background(), orgID, email)
	}
	delete(s.suppression, suppressionKey(orgID, email))
}

func occurredAtOrNow(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now().UTC()
	}
	return ts.UTC()
}

func normalizeSuppressionReason(reason string) string {
	switch strings.TrimSpace(strings.ToLower(reason)) {
	case "hardbounce", "bounce":
		return "bounce"
	case "spamcomplaint", "complaint":
		return "complaint"
	default:
		return "manual"
	}
}

// EraseHouseholdData permanently deletes every comms record tied to a
// household as part of an approved compliance right-to-erasure request:
// message threads (and their edits/reactions/attachments), read watermarks,
// and notification preferences. email_deliveries has no household_id column
// at all, so it is matched by the household's contact emails instead, which
// the caller must capture before member records are anonymized. Returns the
// message-attachment object keys so the caller can purge them from object
// storage.
func (s *Service) EraseHouseholdData(ctx context.Context, orgID, householdID string, contactEmails []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	messageIDs := make(map[string]bool)
	for id, m := range s.messages {
		if m.OrgID == orgID && m.HouseholdID == householdID {
			messageIDs[id] = true
		}
	}

	var objectKeys []string
	for id, a := range s.messageAttachments {
		if !messageIDs[a.MessageID] {
			continue
		}
		if a.ObjectKey != "" {
			objectKeys = append(objectKeys, a.ObjectKey)
		}
		delete(s.messageAttachments, id)
	}
	for id, e := range s.messageEdits {
		if messageIDs[e.MessageID] {
			delete(s.messageEdits, id)
		}
	}
	for id, react := range s.messageReactions {
		if messageIDs[react.MessageID] {
			delete(s.messageReactions, id)
		}
	}
	for id := range messageIDs {
		delete(s.messages, id)
	}
	for id, read := range s.messageReads {
		if read.OrgID == orgID && read.HouseholdID == householdID {
			delete(s.messageReads, id)
		}
	}
	for id, pref := range s.preferences {
		if pref.OrgID == orgID && pref.HouseholdID == householdID {
			delete(s.preferences, id)
		}
	}

	emailSet := make(map[string]bool, len(contactEmails))
	for _, e := range contactEmails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			emailSet[e] = true
		}
	}
	for id, d := range s.deliveries {
		if d.OrgID == orgID && emailSet[strings.ToLower(d.RecipientEmail)] {
			delete(s.deliveries, id)
		}
	}

	if s.repo != nil {
		// message_edits/reactions/attachments cascade-delete via FK once
		// their parent message row is removed.
		if err := s.repo.DeleteMessagesByHousehold(ctx, orgID, householdID); err != nil {
			return objectKeys, fmt.Errorf("erase household messages: %w", err)
		}
		if err := s.repo.DeleteMessageReadsByHousehold(ctx, orgID, householdID); err != nil {
			return objectKeys, fmt.Errorf("erase household message reads: %w", err)
		}
		if err := s.repo.DeleteNotificationPreferencesByHousehold(ctx, orgID, householdID); err != nil {
			return objectKeys, fmt.Errorf("erase household notification preferences: %w", err)
		}
		if err := s.repo.DeleteEmailDeliveriesByRecipientEmails(ctx, orgID, slices.Collect(maps.Keys(emailSet))); err != nil {
			return objectKeys, fmt.Errorf("erase household email deliveries: %w", err)
		}
	}

	return objectKeys, nil
}
