// Package audit provides the audit-log service.
//
// Every privileged or sensitive action in the platform must emit an AuditEvent
// via Service.Append. Events are immutable once written.
//
// When SetPool is given a DB pool, events are persisted to the append-only
// audit_events table (canonical) in addition to the in-memory read cache used
// to serve queries; without a pool the service is in-memory-only (tests).
package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrEventNotFound = errors.New("audit event not found")
	ErrForbidden     = errors.New("forbidden")
)

// ─── Well-known action constants ──────────────────────────────────────────────
// Naming convention: <domain>.<resource>.<verb>

const (
	ActionAuthUserSelfRegister               = "auth.user.self_register"
	ActionAuthUserApprove                    = "auth.user.approve"
	ActionAuthUserReject                     = "auth.user.reject"
	ActionAuthRoleAssign                     = "auth.role.assign"
	ActionAuthRoleRemove                     = "auth.role.remove"
	ActionAuthUserInvite                     = "auth.user.invite"
	ActionAuthUserUpdate                     = "auth.user.update"
	ActionAuthUserDisable                    = "auth.user.disable"
	ActionAuthUserEnable                     = "auth.user.enable"
	ActionAuthUserDelete                     = "auth.user.delete"
	ActionAuthUserPasswordReset              = "auth.user.password_reset"
	ActionCommsNotificationPreferencesUpdate = "comms.notification_preferences.update"
	ActionCommsMessageSend                   = "comms.message.send"
	ActionCommsMessageEdit                   = "comms.message.edit"
	ActionCommsCampaignCreate                = "comms.campaign.create"
	ActionCommsCampaignSend                  = "comms.campaign.send"
	ActionCommsSuppressionAdd                = "comms.suppression.add"
	ActionCommsSuppressionRemove             = "comms.suppression.remove"
	ActionCommsAnnouncementCreate            = "comms.announcement.create"
	ActionCommsAnnouncementPublish           = "comms.announcement.publish"
	ActionCommsAnnouncementArchive           = "comms.announcement.archive"
	ActionCommsAnnouncementVisibility        = "comms.announcement.visibility"
	ActionBillingInvoiceIssue                = "billing.invoice.issue"
	ActionBillingInvoiceVoid                 = "billing.invoice.void"
	ActionBillingInvoiceWriteOff             = "billing.invoice.write_off"
	ActionBillingRefundCreate                = "billing.refund.create"
	ActionBillingAutopayEnroll               = "billing.autopay.enroll"
	ActionBillingAutopayOptOut               = "billing.autopay.opt_out"
	ActionMeetEntryOverride                  = "meet.entry.override"
	ActionMeetEntryApprove                   = "meet.entry.approve"
	ActionMeetEntryScratch                   = "meet.entry.scratch"
	ActionMeetCommitmentsLock                = "meet.commitments.lock"
	ActionPerformanceBestTimeOverride        = "performance.best_time.override"
	ActionMembersHouseholdDelete             = "members.household.delete"
	ActionMembersSwimmerDelete               = "members.swimmer.delete"
	ActionMembersSwimmerUpdate               = "members.swimmer.update"
	ActionComplianceDataExportRequest        = "compliance.data_export.request"
	ActionComplianceDataDeletionRequest      = "compliance.data_deletion.request"
	ActionReportMembershipStatusRun          = "report.membership_status.run"
	ActionReportMeetFeesRun                  = "report.meet_fees.run"
	ActionReportInvoiceAgingRun              = "report.invoice_aging.run"
	ActionReportPaymentReconciliationRun     = "report.payment_reconciliation.run"
	ActionReportRegistrationStatusRun        = "report.registration_status.run"
	ActionReportInvoiceDetailRun             = "report.invoice_detail.run"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// AuditEvent is an immutable record of a privileged or sensitive action.
type AuditEvent struct {
	ID           string            `json:"id"`
	OrgID        string            `json:"org_id"`
	ActorUserID  string            `json:"actor_user_id"`
	ActorRoles   []string          `json:"actor_roles"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	BeforeState  json.RawMessage   `json:"before_state,omitempty"`
	AfterState   json.RawMessage   `json:"after_state,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	OccurredAt   time.Time         `json:"occurred_at"`
}

// AppendRequest carries all fields required to create a new AuditEvent.
type AppendRequest struct {
	OrgID        string
	ActorUserID  string
	ActorRoles   []string
	Action       string
	ResourceType string
	ResourceID   string
	// Before is an optional snapshot of the resource state before the action.
	Before interface{}
	// After is an optional snapshot of the resource state after the action.
	After interface{}
	// Metadata holds arbitrary key-value context (reason, request_id, etc.).
	Metadata map[string]string
}

// Filter specifies which audit events to return.
type Filter struct {
	OrgID        string
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	// From and To bound the occurred_at range (both inclusive).
	From  *time.Time
	To    *time.Time
	Limit int
	// Cursor is an opaque value (event ID) for cursor-based pagination.
	Cursor string
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service is the audit log store. It always keeps an in-memory read cache,
// and additionally persists to Postgres (append-only) when a pool is wired
// via SetPool, so the audit trail survives process restarts/crashes.
type Service struct {
	mu     sync.RWMutex
	events []AuditEvent
	repo   *repository
}

// NewService returns an empty, in-memory-only Service.
func NewService() *Service {
	return &Service{}
}

// SetPool wires a DB pool into the service and loads existing audit events
// from Postgres into the in-memory read cache so restart-time state is
// restored before the service handles requests. A nil pool leaves the
// service in-memory-only (used by tests).
func (s *Service) SetPool(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	repo := newRepository(pool)

	events, err := repo.ListAll(context.Background())
	if err != nil {
		slog.Error("audit: failed to load events from DB at startup", "error", err)
	}

	s.mu.Lock()
	s.repo = repo
	if events != nil {
		s.events = events
	}
	s.mu.Unlock()
}

// Append adds an immutable audit event. Returns the created event.
func (s *Service) Append(req AppendRequest) (*AuditEvent, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	var beforeRaw, afterRaw json.RawMessage
	if req.Before != nil {
		beforeRaw, err = json.Marshal(req.Before)
		if err != nil {
			return nil, err
		}
	}
	if req.After != nil {
		afterRaw, err = json.Marshal(req.After)
		if err != nil {
			return nil, err
		}
	}

	roles := req.ActorRoles
	if roles == nil {
		roles = []string{}
	}
	meta := req.Metadata
	if meta == nil {
		meta = map[string]string{}
	}

	ev := AuditEvent{
		ID:           id,
		OrgID:        req.OrgID,
		ActorUserID:  req.ActorUserID,
		ActorRoles:   roles,
		Action:       req.Action,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		BeforeState:  beforeRaw,
		AfterState:   afterRaw,
		Metadata:     meta,
		OccurredAt:   time.Now().UTC(),
	}

	s.mu.Lock()
	repo := s.repo
	s.mu.Unlock()
	if repo != nil {
		if err := repo.Insert(context.Background(), ev); err != nil {
			// The audit trail is a compliance record; a failed DB write must
			// not be silently swallowed as if the event were durably recorded.
			return nil, err
		}
	}

	s.mu.Lock()
	s.events = append(s.events, ev)
	s.mu.Unlock()

	return &ev, nil
}

// Query returns audit events matching the filter, sorted newest-first.
func (s *Service) Query(f Filter) ([]AuditEvent, error) {
	s.mu.RLock()
	all := make([]AuditEvent, len(s.events))
	copy(all, s.events)
	s.mu.RUnlock()

	// s.events is not guaranteed to already be newest-first: in-memory Append
	// calls append in insertion order, but a DB-backed resync reloads via
	// ListAll's ORDER BY occurred_at DESC. Sort explicitly rather than assume
	// either ordering.
	sort.Slice(all, func(i, j int) bool {
		return all[i].OccurredAt.After(all[j].OccurredAt)
	})

	var out []AuditEvent
	pastCursor := f.Cursor == ""
	for _, ev := range all {
		if !pastCursor {
			if ev.ID == f.Cursor {
				pastCursor = true
			}
			continue
		}
		if f.OrgID != "" && ev.OrgID != f.OrgID {
			continue
		}
		if f.ActorUserID != "" && ev.ActorUserID != f.ActorUserID {
			continue
		}
		if f.Action != "" && !strings.EqualFold(ev.Action, f.Action) {
			continue
		}
		if f.ResourceType != "" && !strings.EqualFold(ev.ResourceType, f.ResourceType) {
			continue
		}
		if f.ResourceID != "" && ev.ResourceID != f.ResourceID {
			continue
		}
		if f.From != nil && ev.OccurredAt.Before(*f.From) {
			continue
		}
		if f.To != nil && ev.OccurredAt.After(*f.To) {
			continue
		}
		out = append(out, ev)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	if out == nil {
		out = []AuditEvent{}
	}
	return out, nil
}

// ─── Diff helper ──────────────────────────────────────────────────────────────

// Diff computes a field-level diff between before and after.
// Both values must be JSON-marshallable flat objects (i.e., a struct or map
// whose top-level values are scalars or simple types). Nested objects and
// slices are compared as opaque JSON strings — if the nested structure changes
// at all, the entire key is reported as changed rather than recursively diffed.
// Callers that pass structs with deeply nested slices (e.g. []string fields)
// may see whole-key diffs rather than element-level diffs; this is by design.
// Returns a map of field names to DiffEntry{Before, After}.
// Fields that are identical in both snapshots are omitted.
func Diff(before, after interface{}) (map[string]DiffEntry, error) {
	beforeMap, err := toStringMap(before)
	if err != nil {
		return nil, err
	}
	afterMap, err := toStringMap(after)
	if err != nil {
		return nil, err
	}

	out := make(map[string]DiffEntry)

	// Keys in before.
	for k, bv := range beforeMap {
		av, exists := afterMap[k]
		if !exists {
			out[k] = DiffEntry{Before: bv, After: nil}
		} else if string(bv) != string(av) {
			out[k] = DiffEntry{Before: bv, After: av}
		}
	}

	// Keys only in after (added fields).
	for k, av := range afterMap {
		if _, exists := beforeMap[k]; !exists {
			out[k] = DiffEntry{Before: nil, After: av}
		}
	}

	return out, nil
}

// DiffEntry represents the before and after values of a single field.
type DiffEntry struct {
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}

// toStringMap marshals v to JSON then unmarshals into a flat map[string]json.RawMessage.
func toStringMap(v interface{}) (map[string]json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
