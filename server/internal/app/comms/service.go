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
// Production note: all data is held in-memory maps protected by a RWMutex.
// Replace with sqlc/pgx queries when the DB layer is wired.
package comms

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dareaquatics/aquify/server/internal/postmark"
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
	PublishedAt *time.Time `json:"published_at,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CreatedBy   string     `json:"created_by"`
	UpdatedBy   string     `json:"updated_by"`
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

type CreateAnnouncementInput struct {
	OrgID     string `json:"org_id"    validate:"required"`
	Title     string `json:"title"     validate:"required,max=255"`
	Body      string `json:"body"      validate:"required"`
	Audience  string `json:"audience"  validate:"required,oneof=all group"`
	GroupID   string `json:"group_id"`
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
	mu            sync.RWMutex
	repo          *Repository
	announcements map[string]*Announcement
	campaigns     map[string]*EmailCampaign
	deliveries    map[string]*EmailDelivery
	suppression   map[string]*SuppressionEntry // key: orgID|email (lowercase)
	preferences   map[string]*NotificationPreference

	postmarkClient postmark.Client
}

// NewService returns a new in-memory Service with a stub Postmark client.
func NewService() *Service {
	return &Service{
		announcements:  make(map[string]*Announcement),
		campaigns:      make(map[string]*EmailCampaign),
		deliveries:     make(map[string]*EmailDelivery),
		suppression:    make(map[string]*SuppressionEntry),
		preferences:    make(map[string]*NotificationPreference),
		postmarkClient: postmark.NewStubClient(),
	}
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
	announcements, campaigns, deliveries, suppressions, preferences, err := s.repo.LoadAll(ctx)
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
	return nil
}

func (s *Service) newID() string {
	if s.repo != nil {
		return uuid.New().String()
	}
	return newID()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("secure random unavailable for communications ID generation: %w", err))
	}
	return hex.EncodeToString(b)
}

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

	seen := make(map[string]struct{}, len(settings))
	now := time.Now().UTC()
	for _, setting := range settings {
		definition, ok := notificationPreferenceDefinitionByID(setting.EventID)
		if !ok {
			return nil, fmt.Errorf("unsupported notification event %q", setting.EventID)
		}
		if _, exists := seen[definition.EventID]; exists {
			return nil, fmt.Errorf("duplicate notification event %q", definition.EventID)
		}
		seen[definition.EventID] = struct{}{}

		key := notificationPreferenceKey(orgID, userID, definition.EventID)
		pref, exists := s.preferences[key]
		if !exists || pref == nil {
			pref = &NotificationPreference{
				ID:        s.newID(),
				OrgID:     orgID,
				UserID:    userID,
				CreatedAt: now,
			}
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

// PublishAnnouncement transitions an announcement from draft → published.
func (s *Service) PublishAnnouncement(orgID, id, actorID string) (*Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	a, ok := s.announcements[id]
	if !ok || a.OrgID != orgID {
		return nil, ErrAnnouncementNotFound
	}
	if a.Status != "draft" {
		return nil, ErrInvalidTransition
	}

	now := time.Now().UTC()
	a.Status = "published"
	a.PublishedAt = &now
	a.UpdatedAt = now
	a.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.UpsertAnnouncement(context.Background(), a); err != nil {
			return nil, fmt.Errorf("persist published announcement: %w", err)
		}
	}
	return a, nil
}

// ArchiveAnnouncement transitions an announcement to archived.
func (s *Service) ArchiveAnnouncement(orgID, id, actorID string) (*Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	a, ok := s.announcements[id]
	if !ok || a.OrgID != orgID {
		return nil, ErrAnnouncementNotFound
	}
	if a.Status == "archived" {
		return nil, ErrInvalidTransition
	}

	now := time.Now().UTC()
	a.Status = "archived"
	a.ArchivedAt = &now
	a.UpdatedAt = now
	a.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.UpsertAnnouncement(context.Background(), a); err != nil {
			return nil, fmt.Errorf("persist archived announcement: %w", err)
		}
	}
	return a, nil
}

// GetAnnouncement returns a single announcement.
func (s *Service) GetAnnouncement(orgID, id string) (*Announcement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.announcements[id]
	if !ok || a.OrgID != orgID {
		return nil, ErrAnnouncementNotFound
	}
	return a, nil
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
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
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
	return c, nil
}

// ListCampaigns returns all campaigns for an org, newest first.
func (s *Service) ListCampaigns(orgID string) ([]*EmailCampaign, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*EmailCampaign, 0)
	for _, c := range s.campaigns {
		if c.OrgID == orgID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
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
				return nil, fmt.Errorf("persist email delivery: %w", err)
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
	s.mu.Unlock()

	if s.repo != nil {
		if err := s.repo.UpsertCampaign(context.Background(), c); err != nil {
			return nil, fmt.Errorf("persist sent campaign: %w", err)
		}
	}
	return c, nil
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
	case "Bounce":
		if delivery == nil && (orgID == "" || email == "") {
			return ErrDeliveryNotFound
		}
		now := occurredAtOrNow(inp.OccurredAt)
		if delivery != nil {
			delivery.Status = "bounced"
			delivery.BouncedAt = &now
			delivery.UpdatedAt = now
			delivery.ErrorMessage = inp.Details
			if s.repo != nil {
				if err := s.repo.UpsertDelivery(context.Background(), delivery); err != nil {
					return fmt.Errorf("persist bounced email delivery: %w", err)
				}
			}
		}
		if orgID != "" && email != "" {
			s.upsertSuppressionLocked(orgID, email, "bounce", "postmark-webhook")
		}
		return nil
	case "SpamComplaint":
		if delivery == nil && (orgID == "" || email == "") {
			return ErrDeliveryNotFound
		}
		now := occurredAtOrNow(inp.OccurredAt)
		if delivery != nil {
			delivery.Status = "bounced"
			delivery.BouncedAt = &now
			delivery.UpdatedAt = now
			delivery.ErrorMessage = inp.Details
			if s.repo != nil {
				if err := s.repo.UpsertDelivery(context.Background(), delivery); err != nil {
					return fmt.Errorf("persist complained email delivery: %w", err)
				}
			}
		}
		if orgID != "" && email != "" {
			s.upsertSuppressionLocked(orgID, email, "complaint", "postmark-webhook")
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
		ID:        newID(),
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
