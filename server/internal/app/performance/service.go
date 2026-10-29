// Package performance provides the in-memory service for swimmer results,
// best-time management, time standards, and cut qualification evaluation.
//
// Production note: all data is held in-memory maps protected by a RWMutex.
// Replace with sqlc/pgx queries when the DB layer is wired.
package performance

import (
	"context"
	"errors"
	"fmt"
	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrForbidden              = errors.New("forbidden")
	ErrResultNotFound         = errors.New("result not found")
	ErrBestTimeNotFound       = errors.New("best time record not found")
	ErrStandardNotFound       = errors.New("time standard not found")
	ErrOverrideRequiresReason = errors.New("a reason is required for manual best-time overrides")
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// Result holds a single swimmer-event result record.
type Result struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	SwimmerID        string    `json:"swimmer_id"`
	MeetID           string    `json:"meet_id,omitempty"`
	MeetEventID      string    `json:"meet_event_id,omitempty"`
	ImportID         string    `json:"import_id,omitempty"`
	EventNumber      string    `json:"event_number,omitempty"`
	Distance         int       `json:"distance"`
	Stroke           string    `json:"stroke"`
	Course           string    `json:"course"`
	Gender           string    `json:"gender"`
	Heat             *int      `json:"heat,omitempty"`
	Lane             *int      `json:"lane,omitempty"`
	Place            *int      `json:"place,omitempty"`
	SeedTimeMs       *int      `json:"seed_time_ms,omitempty"`
	FinTimeMs        *int      `json:"fin_time_ms,omitempty"` // nil = NT or DQ
	DQ               bool      `json:"dq"`
	DQCode           string    `json:"dq_code,omitempty"`
	DQNote           string    `json:"dq_note,omitempty"`
	IsOfficial       bool      `json:"is_official"`
	IsManualOverride bool      `json:"is_manual_override"`
	OverrideReason   string    `json:"override_reason,omitempty"`
	OverrideBy       string    `json:"override_by,omitempty"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedBy        string    `json:"created_by"`
	UpdatedBy        string    `json:"updated_by"`
}

// Split holds one split record for a result.
type Split struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	ResultID         string    `json:"result_id"`
	SplitNumber      int       `json:"split_number"`
	SplitDistance    int       `json:"split_distance"`
	SplitTimeMs      int       `json:"split_time_ms"`
	CumulativeTimeMs int       `json:"cumulative_time_ms"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedBy        string    `json:"created_by"`
	UpdatedBy        string    `json:"updated_by"`
}

// BestTime holds the canonical best time for a swimmer/event/course combination.
type BestTime struct {
	ID               string     `json:"id"`
	OrgID            string     `json:"org_id"`
	SwimmerID        string     `json:"swimmer_id"`
	EventDistance    int        `json:"event_distance"`
	Stroke           string     `json:"stroke"`
	Course           string     `json:"course"`
	BestTimeMs       *int       `json:"best_time_ms"` // nil = NT
	ResultID         string     `json:"result_id,omitempty"`
	RecordedAt       *time.Time `json:"recorded_at,omitempty"`
	IsManualOverride bool       `json:"is_manual_override"`
	OverrideReason   string     `json:"override_reason,omitempty"`
	OverrideBy       string     `json:"override_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CreatedBy        string     `json:"created_by"`
	UpdatedBy        string     `json:"updated_by"`
}

// TimeStandard holds a qualifying cut time for an event/age/gender/course combination.
type TimeStandard struct {
	ID            string     `json:"id"`
	OrgID         string     `json:"org_id"`
	Name          string     `json:"name"`
	SeasonYear    int        `json:"season_year"`
	StandardCode  string     `json:"standard_code"`
	Description   string     `json:"description,omitempty"`
	EventDistance int        `json:"event_distance"`
	Stroke        string     `json:"stroke"`
	Course        string     `json:"course"`
	Gender        string     `json:"gender"`
	AgeMin        *int       `json:"age_min,omitempty"`
	AgeMax        *int       `json:"age_max,omitempty"`
	TimeMs        int        `json:"time_ms"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	CreatedBy     string     `json:"created_by"`
	UpdatedBy     string     `json:"updated_by"`
}

// BestTimeAuditEvent records privileged actions in the performance domain.
type BestTimeAuditEvent struct {
	ID         string         `json:"id"`
	OrgID      string         `json:"org_id"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	ResourceID string         `json:"resource_id"`
	Meta       map[string]any `json:"meta,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// CutQualification holds the result of evaluating a swimmer's best time against a time standard.
type CutQualification struct {
	SwimmerID     string `json:"swimmer_id"`
	StandardID    string `json:"standard_id"`
	StandardCode  string `json:"standard_code"`
	EventDistance int    `json:"event_distance"`
	Stroke        string `json:"stroke"`
	Course        string `json:"course"`
	Gender        string `json:"gender"`
	CutTimeMs     int    `json:"cut_time_ms"`
	BestTimeMs    *int   `json:"best_time_ms"` // nil = NT
	Qualified     bool   `json:"qualified"`
	MarginMs      *int   `json:"margin_ms,omitempty"` // positive = under the cut (qualified); negative = over
}

// ─── Input types ─────────────────────────────────────────────────────────────

// UpsertResultInput contains all fields needed to create or update a result.
type UpsertResultInput struct {
	SwimmerID   string       `json:"swimmer_id" validate:"required"`
	MeetID      string       `json:"meet_id"`
	MeetEventID string       `json:"meet_event_id"`
	ImportID    string       `json:"import_id"`
	EventNumber string       `json:"event_number"`
	Distance    int          `json:"distance" validate:"required,gt=0"`
	Stroke      string       `json:"stroke" validate:"required,oneof=free back breast fly im free_relay medley_relay"`
	Course      string       `json:"course" validate:"required,oneof=SCY SCM LCM"`
	Gender      string       `json:"gender" validate:"required,oneof=M F X"`
	Heat        *int         `json:"heat"`
	Lane        *int         `json:"lane"`
	Place       *int         `json:"place"`
	SeedTimeMs  *int         `json:"seed_time_ms"`
	FinTimeMs   *int         `json:"fin_time_ms"`
	DQ          bool         `json:"dq"`
	DQCode      string       `json:"dq_code"`
	DQNote      string       `json:"dq_note"`
	IsOfficial  bool         `json:"is_official"`
	Splits      []SplitInput `json:"splits"`
}

// SplitInput describes one split for a result.
type SplitInput struct {
	SplitNumber      int `json:"split_number" validate:"required,gt=0"`
	SplitDistance    int `json:"split_distance" validate:"required,gt=0"`
	SplitTimeMs      int `json:"split_time_ms" validate:"required,gte=0"`
	CumulativeTimeMs int `json:"cumulative_time_ms" validate:"required,gte=0"`
}

// OverrideBestTimeInput contains the fields for a manual best-time override.
type OverrideBestTimeInput struct {
	SwimmerID     string `json:"swimmer_id" validate:"required"`
	EventDistance int    `json:"event_distance" validate:"required,gt=0"`
	Stroke        string `json:"stroke" validate:"required,oneof=free back breast fly im"`
	Course        string `json:"course" validate:"required,oneof=SCY SCM LCM"`
	BestTimeMs    *int   `json:"best_time_ms"` // nil clears the time (NT)
	Reason        string `json:"reason" validate:"required"`
	RecordedAt    string `json:"recorded_at"` // optional ISO-8601 date
}

// CreateTimeStandardInput holds fields for creating a time standard.
type CreateTimeStandardInput struct {
	Name          string `json:"name" validate:"required"`
	SeasonYear    int    `json:"season_year" validate:"required,gt=2000"`
	StandardCode  string `json:"standard_code" validate:"required"`
	Description   string `json:"description"`
	EventDistance int    `json:"event_distance" validate:"required,gt=0"`
	Stroke        string `json:"stroke" validate:"required,oneof=free back breast fly im"`
	Course        string `json:"course" validate:"required,oneof=SCY SCM LCM"`
	Gender        string `json:"gender" validate:"required,oneof=M F X"`
	AgeMin        *int   `json:"age_min"`
	AgeMax        *int   `json:"age_max"`
	TimeMs        int    `json:"time_ms" validate:"required,gt=0"`
	IsActive      bool   `json:"is_active"`
}

// UpdateTimeStandardInput holds fields for updating a time standard.
type UpdateTimeStandardInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	TimeMs      *int    `json:"time_ms"`
	IsActive    *bool   `json:"is_active"`
}

// ListBestTimesFilters supports optional filtering for best-time queries.
type ListBestTimesFilters struct {
	SwimmerID string
	Course    string
	Stroke    string
}

// ListResultsFilters supports optional filtering for progression queries.
type ListResultsFilters struct {
	SwimmerID     string
	EventDistance int
	Stroke        string
	Course        string
}

// ListTimeStandardsFilters supports optional filtering for time standard queries.
type ListTimeStandardsFilters struct {
	SeasonYear    int
	StandardCode  string
	EventDistance int
	Stroke        string
	Course        string
	Gender        string
	ActiveOnly    bool
}

// ─── Cross-domain interface ───────────────────────────────────────────────────

// EntrySeedTimeUpdater is implemented by services that can back-fill seed
// times on future meet entries after a swimmer's best time is recomputed.
// meets.Service satisfies this interface.
type EntrySeedTimeUpdater interface {
	UpdateSeedTimesForSwimmer(orgID, swimmerID string, distance int, stroke, course string, newBestTimeMs *int) int
}

// SwimmerHouseholdChecker can look up which household a swimmer belongs to.
// It is implemented by the members service and injected into the performance
// service to enforce parent-scoped data isolation on per-swimmer reads.
type SwimmerHouseholdChecker interface {
	SwimmerHouseholdID(orgID, swimmerID string) (householdID string, ok bool)
}

// ─── Service ─────────────────────────────────────────────────────────────────

// Service is the performance/best-times domain service.
type Service struct {
	mu               sync.RWMutex
	repo             *Repository              // nil = in-memory only
	results          map[string]*Result       // id → Result
	splits           map[string][]*Split      // resultID → []Split
	bestTimes        map[string]*BestTime     // id → BestTime
	btByKey          map[string]*BestTime     // composite key → BestTime (secondary index for O(1) lookups)
	standards        map[string]*TimeStandard // id → TimeStandard
	auditEvents      []*BestTimeAuditEvent
	entrySeedUpdater EntrySeedTimeUpdater    // optional; wired after construction
	householdChecker SwimmerHouseholdChecker // optional; enforces parent household scoping
	directory        Directory               // optional; swimmer/meet facts for records, top lists, relays

	recordEntries map[string]*RecordEntry // id → team record entry
	goals         map[string]*Goal        // id → swimmer goal
	lineups       map[string]*RelayLineup // id → saved relay lineup
}

// AuditEvents returns a snapshot of all audit events recorded by the service.
// Intended for use in tests; in production, audit events would be persisted to
// the database.
func (s *Service) AuditEvents() []*BestTimeAuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*BestTimeAuditEvent, len(s.auditEvents))
	copy(out, s.auditEvents)
	return out
}

// NewService creates a new in-memory performance Service.
func NewService() *Service {
	return &Service{
		results:   make(map[string]*Result),
		splits:    make(map[string][]*Split),
		bestTimes: make(map[string]*BestTime),
		btByKey:   make(map[string]*BestTime),
		standards: make(map[string]*TimeStandard),

		recordEntries: make(map[string]*RecordEntry),
		goals:         make(map[string]*Goal),
		lineups:       make(map[string]*RelayLineup),
	}
}

// NewServiceWithPool creates a DB-backed performance service.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("performance: failed to load initial state from DB", "error", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	results, bestTimes, standards, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range results {
		s.results[r.ID] = r
	}
	for _, bt := range bestTimes {
		s.bestTimes[bt.ID] = bt
		key := btKey(bt.OrgID, bt.SwimmerID, bt.EventDistance, bt.Stroke, bt.Course)
		s.btByKey[key] = bt
	}
	for _, ts := range standards {
		s.standards[ts.ID] = ts
	}
	return s.loadExtensionsLocked(ctx)
}

func (s *Service) newID() string {
	if s.repo != nil {
		return uuid.New().String()
	}
	return dbutil.NewHexID()
}

// SetEntrySeedTimeUpdater injects the cross-domain updater used to back-fill
// seed times on future meet entries when a best time is recomputed.
// Must be called before the service handles requests (e.g. during server wiring).
func (s *Service) SetEntrySeedTimeUpdater(u EntrySeedTimeUpdater) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entrySeedUpdater = u
}

// SetHouseholdChecker injects the SwimmerHouseholdChecker used to enforce
// parent-scoped isolation on per-swimmer reads. Must be called before the
// service handles requests (e.g. during server wiring).
func (s *Service) SetHouseholdChecker(checker SwimmerHouseholdChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.householdChecker = checker
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) requireAdminOrCoach(principal *appmiddleware.Principal) error {
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "admin", "coach":
			return nil
		}
	}
	return ErrForbidden
}

func (s *Service) requireAdmin(principal *appmiddleware.Principal) error {
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "admin":
			return nil
		}
	}
	return ErrForbidden
}

// requireReadAccess allows admin, coach, parent, and swimmer roles to read
// swimmer performance data. For per-swimmer endpoints, call
// checkParentHouseholdForSwimmer after this check to enforce household
// isolation for the parent and swimmer roles.
func (s *Service) requireReadAccess(principal *appmiddleware.Principal) error {
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "admin", "coach", "parent", "swimmer":
			return nil
		}
	}
	return ErrForbidden
}

// checkParentHouseholdForSwimmer returns ErrForbidden when the principal has
// the parent or swimmer role but the requested swimmerID does not belong to
// the principal's household. Other roles pass through unchecked.
// This enforces household-scoped isolation on per-swimmer read paths.
func (s *Service) checkParentHouseholdForSwimmer(principal *appmiddleware.Principal, orgID, swimmerID string) error {
	if principal == nil {
		return ErrForbidden
	}
	requiresHouseholdScope := false
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "parent", "swimmer":
			requiresHouseholdScope = true
		}
	}
	if !requiresHouseholdScope {
		return nil
	}
	// Household checker is required; deny if not wired.
	if s.householdChecker == nil {
		return ErrForbidden
	}
	swimmerHouseholdID, ok := s.householdChecker.SwimmerHouseholdID(orgID, swimmerID)
	if !ok {
		return ErrForbidden
	}
	if principal.HouseholdID == "" || swimmerHouseholdID != principal.HouseholdID {
		return ErrForbidden
	}
	return nil
}

// principalRequiresHouseholdScope reports whether the principal's roles
// require household-scoped filtering (parent/swimmer) on org-wide reads.
func principalRequiresHouseholdScope(principal *appmiddleware.Principal) bool {
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "parent", "swimmer":
			return true
		}
	}
	return false
}

// householdScopedSwimmerAllowed reports whether swimmerID belongs to the
// principal's household. Used to restrict org-wide performance reads for
// parent/swimmer callers, who must never see other households' data.
func (s *Service) householdScopedSwimmerAllowed(principal *appmiddleware.Principal, orgID, swimmerID string) bool {
	if s.householdChecker == nil || principal.HouseholdID == "" {
		return false
	}
	swimmerHouseholdID, ok := s.householdChecker.SwimmerHouseholdID(orgID, swimmerID)
	if !ok {
		return false
	}
	return swimmerHouseholdID == principal.HouseholdID
}

// btKey returns the composite map key for a best-time record.
func btKey(orgID, swimmerID string, distance int, stroke, course string) string {
	return strings.Join([]string{orgID, swimmerID, fmt.Sprintf("%d", distance), stroke, course}, "|") //nolint:perfsprint
}

// isRelayStroke reports whether a stroke value represents a relay event.
// Relay results must not update the individual best-time table because the
// best_times table only allows individual strokes (free, back, breast, fly, im).
func isRelayStroke(stroke string) bool {
	return stroke == "free_relay" || stroke == "medley_relay"
}

// ─── Results ─────────────────────────────────────────────────────────────────

// UpsertResult creates or replaces a result for the given swimmer-event.
// When fin_time_ms is non-nil, is_official is true, and the stroke is not a
// relay stroke, it also triggers a best-time recompute for the swimmer and
// calls the injected EntrySeedTimeUpdater (if set) to back-fill seed times on
// future draft/submitted meet entries for the swimmer.
// HasMeetResult reports whether a result is already recorded for this swimmer
// and event at the given meet — used to make results-file imports idempotent.
func (s *Service) HasMeetResult(orgID, swimmerID, meetID string, distance int, stroke, course string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.results {
		if r.OrgID == orgID && r.SwimmerID == swimmerID && r.MeetID == meetID &&
			r.Distance == distance && r.Stroke == stroke && r.Course == course && r.Status == "active" {
			return true
		}
	}
	return false
}

func (s *Service) UpsertResult(orgID, actorID string, principal *appmiddleware.Principal, input UpsertResultInput) (*Result, error) {
	result, _, err := s.UpsertResultWithRecords(orgID, actorID, principal, input)
	return result, err
}

// UpsertResultWithRecords behaves like UpsertResult and additionally reports
// any team records the result breaks (one per age group it counts toward:
// the swimmer's age group and Open).
func (s *Service) UpsertResultWithRecords(orgID, actorID string, principal *appmiddleware.Principal, input UpsertResultInput) (*Result, []RecordBreak, error) {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return nil, nil, err
	}
	var dir directorySnapshot
	recordCandidate := input.IsOfficial && !input.DQ && input.FinTimeMs != nil && isIndividualStroke(input.Stroke)
	if recordCandidate {
		dir = s.snapshotDirectory(orgID)
	}

	s.mu.Lock()

	now := time.Now().UTC()
	result := &Result{
		ID:          s.newID(),
		OrgID:       orgID,
		SwimmerID:   input.SwimmerID,
		MeetID:      input.MeetID,
		MeetEventID: input.MeetEventID,
		ImportID:    input.ImportID,
		EventNumber: input.EventNumber,
		Distance:    input.Distance,
		Stroke:      input.Stroke,
		Course:      input.Course,
		Gender:      input.Gender,
		Heat:        input.Heat,
		Lane:        input.Lane,
		Place:       input.Place,
		SeedTimeMs:  input.SeedTimeMs,
		FinTimeMs:   input.FinTimeMs,
		DQ:          input.DQ,
		DQCode:      input.DQCode,
		DQNote:      input.DQNote,
		IsOfficial:  input.IsOfficial,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   actorID,
		UpdatedBy:   actorID,
	}
	// Postgres is canonical: an unpersisted result would vanish on restart and
	// skew best times across instances, so surface the failure.
	if s.repo != nil {
		if err := s.repo.UpsertResult(context.Background(), result); err != nil {
			slog.Error("performance: upsert result", "error", err)
			s.mu.Unlock()
			return nil, nil, fmt.Errorf("persist result: %w", err)
		}
	}
	s.results[result.ID] = result

	// Persist splits
	if len(input.Splits) > 0 {
		ss := make([]*Split, 0, len(input.Splits))
		for _, sp := range input.Splits {
			ss = append(ss, &Split{
				ID:               s.newID(),
				OrgID:            orgID,
				ResultID:         result.ID,
				SplitNumber:      sp.SplitNumber,
				SplitDistance:    sp.SplitDistance,
				SplitTimeMs:      sp.SplitTimeMs,
				CumulativeTimeMs: sp.CumulativeTimeMs,
				CreatedAt:        now,
				UpdatedAt:        now,
				CreatedBy:        actorID,
				UpdatedBy:        actorID,
			})
		}
		if s.repo != nil {
			if err := s.repo.UpsertSplits(context.Background(), ss); err != nil {
				slog.Error("performance: upsert splits", "error", err)
				s.mu.Unlock()
				return nil, nil, fmt.Errorf("persist splits: %w", err)
			}
		}
		s.splits[result.ID] = ss
	}

	// Recompute best time only for official, non-DQ, individual-stroke results
	// with a finish time. Relay strokes (free_relay, medley_relay) are excluded
	// because best_times enforces a CHECK constraint that only allows individual
	// strokes.
	var (
		triggerSeedUpdate bool
		newBestTimeMs     *int
	)
	if result.IsOfficial && !result.DQ && result.FinTimeMs != nil && !isRelayStroke(input.Stroke) {
		if err := s.recomputeBestTimeLocked(orgID, actorID, input.SwimmerID, input.Distance, input.Stroke, input.Course); err != nil {
			s.mu.Unlock()
			return nil, nil, err
		}
		triggerSeedUpdate = true
		key := btKey(orgID, input.SwimmerID, input.Distance, input.Stroke, input.Course)
		if bt := s.btByKey[key]; bt != nil {
			newBestTimeMs = bt.BestTimeMs
		}
	}

	var (
		breaks       []RecordBreak
		detectionErr error
	)
	if recordCandidate {
		breaks, detectionErr = s.detectRecordBreaksLocked(orgID, actorID, result, dir)
	}

	// Emit result.upserted audit event.
	s.auditEvents = append(s.auditEvents, &BestTimeAuditEvent{
		ID:         s.newID(),
		OrgID:      orgID,
		ActorID:    actorID,
		Action:     "result.upserted",
		Resource:   "result",
		ResourceID: result.ID,
		Meta: map[string]any{
			"swimmer_id":  result.SwimmerID,
			"distance":    result.Distance,
			"stroke":      result.Stroke,
			"course":      result.Course,
			"fin_time_ms": result.FinTimeMs,
			"is_official": result.IsOfficial,
		},
		CreatedAt: now,
	})

	// Capture the updater reference before releasing the lock so we can call
	// it outside the performance lock, avoiding cross-package lock ordering issues.
	updater := s.entrySeedUpdater

	s.mu.Unlock()

	// Back-fill seed times on future draft/submitted entries for this swimmer.
	// Called outside the performance lock to avoid potential deadlock with the
	// meets service lock.
	if triggerSeedUpdate && updater != nil {
		updater.UpdateSeedTimesForSwimmer(orgID, input.SwimmerID, input.Distance, input.Stroke, input.Course, newBestTimeMs)
	}

	// The result is saved either way; surface an incomplete record detection so
	// callers can report it instead of silently losing a break.
	return result, breaks, detectionErr
}

// recomputeBestTimeLocked scans all active results for the swimmer/event/course
// and updates (or creates) the best_time record. Caller must hold s.mu.Lock().
// RecordedAt is always set to the timestamp of the result that produced the BT.
// The record is persisted before the cache is replaced, so a failed write
// leaves memory unchanged and is returned to the caller.
func (s *Service) recomputeBestTimeLocked(orgID, actorID, swimmerID string, distance int, stroke, course string) error {
	var best *Result
	for _, r := range s.results {
		if r.OrgID != orgID || r.SwimmerID != swimmerID {
			continue
		}
		if r.Distance != distance || r.Stroke != stroke || r.Course != course {
			continue
		}
		if r.Status != "active" || r.DQ || !r.IsOfficial || r.FinTimeMs == nil {
			continue
		}
		if best == nil || *r.FinTimeMs < *best.FinTimeMs {
			best = r
		}
	}

	key := btKey(orgID, swimmerID, distance, stroke, course)
	existing := s.btByKey[key] // O(1) lookup via secondary index
	now := time.Now().UTC()

	var next *BestTime
	switch {
	case existing != nil && existing.IsManualOverride:
		return nil
	case best == nil && existing == nil:
		return nil
	case existing != nil:
		cp := *existing
		next = &cp
	default:
		next = &BestTime{
			ID:            s.newID(),
			OrgID:         orgID,
			SwimmerID:     swimmerID,
			EventDistance: distance,
			Stroke:        stroke,
			Course:        course,
			CreatedAt:     now,
			CreatedBy:     actorID,
		}
	}
	if best == nil {
		// No qualifying result found; clear the time.
		next.BestTimeMs, next.ResultID, next.RecordedAt = nil, "", nil
	} else {
		recordedAt := best.CreatedAt
		next.BestTimeMs, next.ResultID, next.RecordedAt = best.FinTimeMs, best.ID, &recordedAt
	}
	next.UpdatedAt = now
	next.UpdatedBy = actorID

	if s.repo != nil {
		if err := s.repo.UpsertBestTime(context.Background(), next); err != nil {
			return fmt.Errorf("persist best time: %w", err)
		}
	}
	s.bestTimes[next.ID] = next
	s.btByKey[key] = next
	return nil
}

// RecomputeBestTime forces a best-time recompute for the given swimmer/event/course.
func (s *Service) RecomputeBestTime(orgID, actorID string, principal *appmiddleware.Principal, swimmerID string, distance int, stroke, course string) (*BestTime, error) {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.recomputeBestTimeLocked(orgID, actorID, swimmerID, distance, stroke, course); err != nil {
		return nil, err
	}

	key := btKey(orgID, swimmerID, distance, stroke, course)
	return s.btByKey[key], nil
}

// ─── Best Times ───────────────────────────────────────────────────────────────

// ListBestTimes returns all best-time records for the given swimmer.
// Allowed roles: admin, coach, parent.
// For the parent role, household ownership is enforced via checkParentHouseholdForSwimmer.
func (s *Service) ListBestTimes(orgID string, principal *appmiddleware.Principal, swimmerID string, filters ListBestTimesFilters) ([]*BestTime, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	if err := s.checkParentHouseholdForSwimmer(principal, orgID, swimmerID); err != nil {
		return nil, err
	}

	filters.SwimmerID = swimmerID
	return s.filterBestTimes(orgID, filters, nil), nil
}

// ListBestTimesForOrg returns all best-time records visible within the org.
func (s *Service) ListBestTimesForOrg(orgID string, principal *appmiddleware.Principal, filters ListBestTimesFilters) ([]*BestTime, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	var allow func(swimmerID string) bool
	if principalRequiresHouseholdScope(principal) {
		allow = func(swimmerID string) bool { return s.householdScopedSwimmerAllowed(principal, orgID, swimmerID) }
	}
	return s.filterBestTimes(orgID, filters, allow), nil
}

// filterBestTimes returns copies of the org's best times matching the filters
// (and, when set, the allow predicate), ordered by swimmer, stroke, distance, course.
func (s *Service) filterBestTimes(orgID string, filters ListBestTimesFilters, allow func(swimmerID string) bool) []*BestTime {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*BestTime
	for _, bt := range s.bestTimes {
		if bt.OrgID != orgID {
			continue
		}
		if filters.SwimmerID != "" && bt.SwimmerID != filters.SwimmerID {
			continue
		}
		if filters.Course != "" && bt.Course != filters.Course {
			continue
		}
		if filters.Stroke != "" && bt.Stroke != filters.Stroke {
			continue
		}
		if allow != nil && !allow(bt.SwimmerID) {
			continue
		}
		cp := *bt
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SwimmerID != out[j].SwimmerID {
			return out[i].SwimmerID < out[j].SwimmerID
		}
		if out[i].Stroke != out[j].Stroke {
			return out[i].Stroke < out[j].Stroke
		}
		if out[i].EventDistance != out[j].EventDistance {
			return out[i].EventDistance < out[j].EventDistance
		}
		return out[i].Course < out[j].Course
	})
	return out
}

// OverrideBestTime applies a manual best-time override and emits an audit event.
func (s *Service) OverrideBestTime(orgID, actorID string, principal *appmiddleware.Principal, input OverrideBestTimeInput) (*BestTime, *BestTimeAuditEvent, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, nil, ErrForbidden
	}
	if strings.TrimSpace(input.Reason) == "" {
		return nil, nil, ErrOverrideRequiresReason
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	key := btKey(orgID, input.SwimmerID, input.EventDistance, input.Stroke, input.Course)

	// O(1) lookup via secondary index.
	existing := s.btByKey[key]

	var beforeMs *int
	if existing != nil {
		beforeMs = existing.BestTimeMs
	}

	// Edit a copy so a failed write leaves the cached record untouched.
	var next BestTime
	if existing != nil {
		next = *existing
	} else {
		next = BestTime{
			ID:            s.newID(),
			OrgID:         orgID,
			SwimmerID:     input.SwimmerID,
			EventDistance: input.EventDistance,
			Stroke:        input.Stroke,
			Course:        input.Course,
			CreatedAt:     now,
			CreatedBy:     actorID,
		}
	}
	next.BestTimeMs = input.BestTimeMs
	next.IsManualOverride = true
	next.OverrideReason = input.Reason
	next.OverrideBy = actorID
	next.UpdatedAt = now
	next.UpdatedBy = actorID

	if input.RecordedAt != "" {
		if t, err := time.Parse("2006-01-02", input.RecordedAt); err == nil {
			next.RecordedAt = &t
		}
	}
	if s.repo != nil {
		if err := s.repo.UpsertBestTime(context.Background(), &next); err != nil {
			return nil, nil, fmt.Errorf("persist best time override: %w", err)
		}
	}
	s.bestTimes[next.ID] = &next
	s.btByKey[key] = &next
	existing = &next

	meta := map[string]any{
		"swimmer_id":     input.SwimmerID,
		"event_distance": input.EventDistance,
		"stroke":         input.Stroke,
		"course":         input.Course,
		"reason":         input.Reason,
	}
	if beforeMs != nil {
		meta["before_ms"] = *beforeMs
	} else {
		meta["before_ms"] = nil
	}
	if input.BestTimeMs != nil {
		meta["after_ms"] = *input.BestTimeMs
	} else {
		meta["after_ms"] = nil
	}

	audit := &BestTimeAuditEvent{
		ID:         s.newID(),
		OrgID:      orgID,
		ActorID:    actorID,
		Action:     "best_time.manual_override",
		Resource:   "best_time",
		ResourceID: existing.ID,
		Meta:       meta,
		CreatedAt:  now,
	}
	s.auditEvents = append(s.auditEvents, audit)

	return existing, audit, nil
}

// ─── Progression ─────────────────────────────────────────────────────────────

// ListProgression returns all official results for a swimmer, optionally
// filtered by event/stroke/course, sorted chronologically (oldest first).
// Allowed roles: admin, coach, parent.
// For the parent role, household ownership is enforced via checkParentHouseholdForSwimmer.
func (s *Service) ListProgression(orgID string, principal *appmiddleware.Principal, swimmerID string, filters ListResultsFilters) ([]*Result, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	if err := s.checkParentHouseholdForSwimmer(principal, orgID, swimmerID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Result
	for _, r := range s.results {
		if r.OrgID != orgID || r.SwimmerID != swimmerID {
			continue
		}
		if r.Status != "active" {
			continue
		}
		if filters.EventDistance > 0 && r.Distance != filters.EventDistance {
			continue
		}
		if filters.Stroke != "" && r.Stroke != filters.Stroke {
			continue
		}
		if filters.Course != "" && r.Course != filters.Course {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ListProgressionForOrg returns all visible progression rows within the org.
func (s *Service) ListProgressionForOrg(orgID string, principal *appmiddleware.Principal, filters ListResultsFilters) ([]*Result, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	householdScoped := principalRequiresHouseholdScope(principal)

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Result
	for _, result := range s.results {
		if result.OrgID != orgID {
			continue
		}
		if filters.SwimmerID != "" && result.SwimmerID != filters.SwimmerID {
			continue
		}
		if result.Status != "active" {
			continue
		}
		if filters.EventDistance > 0 && result.Distance != filters.EventDistance {
			continue
		}
		if filters.Stroke != "" && result.Stroke != filters.Stroke {
			continue
		}
		if filters.Course != "" && result.Course != filters.Course {
			continue
		}
		if householdScoped && !s.householdScopedSwimmerAllowed(principal, orgID, result.SwimmerID) {
			continue
		}
		cp := *result
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SwimmerID != out[j].SwimmerID {
			return out[i].SwimmerID < out[j].SwimmerID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ─── Time Standards ───────────────────────────────────────────────────────────

// CreateTimeStandard creates a new time standard.
func (s *Service) CreateTimeStandard(orgID, actorID string, principal *appmiddleware.Principal, input CreateTimeStandardInput) (*TimeStandard, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	ts := &TimeStandard{
		ID:            s.newID(),
		OrgID:         orgID,
		Name:          input.Name,
		SeasonYear:    input.SeasonYear,
		StandardCode:  input.StandardCode,
		Description:   input.Description,
		EventDistance: input.EventDistance,
		Stroke:        input.Stroke,
		Course:        input.Course,
		Gender:        input.Gender,
		AgeMin:        input.AgeMin,
		AgeMax:        input.AgeMax,
		TimeMs:        input.TimeMs,
		IsActive:      input.IsActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     actorID,
		UpdatedBy:     actorID,
	}
	if s.repo != nil {
		if err := s.repo.UpsertTimeStandard(context.Background(), ts); err != nil {
			return nil, fmt.Errorf("persist time standard: %w", err)
		}
	}
	s.standards[ts.ID] = ts
	return ts, nil
}

// ListTimeStandards returns time standards for the org, with optional filtering.
func (s *Service) ListTimeStandards(orgID string, filters ListTimeStandardsFilters) ([]*TimeStandard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*TimeStandard
	for _, ts := range s.standards {
		if ts.OrgID != orgID {
			continue
		}
		if ts.DeletedAt != nil {
			continue
		}
		if filters.ActiveOnly && !ts.IsActive {
			continue
		}
		if filters.SeasonYear > 0 && ts.SeasonYear != filters.SeasonYear {
			continue
		}
		if filters.StandardCode != "" && !strings.EqualFold(ts.StandardCode, filters.StandardCode) {
			continue
		}
		if filters.EventDistance > 0 && ts.EventDistance != filters.EventDistance {
			continue
		}
		if filters.Stroke != "" && ts.Stroke != filters.Stroke {
			continue
		}
		if filters.Course != "" && ts.Course != filters.Course {
			continue
		}
		if filters.Gender != "" && ts.Gender != filters.Gender {
			continue
		}
		out = append(out, ts)
	}
	slices.SortFunc(out, func(a, b *TimeStandard) int {
		if a.SeasonYear != b.SeasonYear {
			if a.SeasonYear > b.SeasonYear {
				return -1
			}
			return 1
		}
		return strings.Compare(a.StandardCode, b.StandardCode)
	})
	return out, nil
}

// GetTimeStandard fetches a single time standard by ID.
func (s *Service) GetTimeStandard(orgID, standardID string) (*TimeStandard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ts, ok := s.standards[standardID]
	if !ok || ts.OrgID != orgID || ts.DeletedAt != nil {
		return nil, ErrStandardNotFound
	}
	return ts, nil
}

// UpdateTimeStandard applies partial updates to a time standard.
func (s *Service) UpdateTimeStandard(orgID, actorID, standardID string, principal *appmiddleware.Principal, input UpdateTimeStandardInput) (*TimeStandard, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.standards[standardID]
	if !ok || current.OrgID != orgID || current.DeletedAt != nil {
		return nil, ErrStandardNotFound
	}

	ts := new(TimeStandard)
	*ts = *current
	if input.Name != nil {
		ts.Name = *input.Name
	}
	if input.Description != nil {
		ts.Description = *input.Description
	}
	if input.TimeMs != nil {
		ts.TimeMs = *input.TimeMs
	}
	if input.IsActive != nil {
		ts.IsActive = *input.IsActive
	}
	ts.UpdatedAt = time.Now().UTC()
	ts.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.UpsertTimeStandard(context.Background(), ts); err != nil {
			return nil, fmt.Errorf("persist time standard: %w", err)
		}
	}
	s.standards[standardID] = ts
	return ts, nil
}

// DeleteTimeStandard soft-deletes a time standard.
func (s *Service) DeleteTimeStandard(orgID, actorID, standardID string, principal *appmiddleware.Principal) error {
	if err := s.requireAdmin(principal); err != nil {
		return ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ts, ok := s.standards[standardID]
	if !ok || ts.OrgID != orgID || ts.DeletedAt != nil {
		return ErrStandardNotFound
	}

	now := time.Now().UTC()
	if s.repo != nil {
		if err := s.repo.SoftDeleteTimeStandard(context.Background(), orgID, standardID, now); err != nil {
			return fmt.Errorf("persist time standard delete: %w", err)
		}
	}
	// Replace rather than mutate: readers may still hold the old pointer.
	deleted := *ts
	deleted.DeletedAt = &now
	deleted.UpdatedAt = now
	deleted.UpdatedBy = actorID
	s.standards[standardID] = &deleted
	return nil
}

// ─── Cut Qualification ────────────────────────────────────────────────────────

// EvaluateCutQualification evaluates whether a swimmer's best time qualifies
// for a given time standard.
// Allowed roles: admin, coach, parent, swimmer.
// Household-scoped roles must match the swimmer's household via the injected checker.
func (s *Service) EvaluateCutQualification(orgID string, principal *appmiddleware.Principal, swimmerID, standardID string) (*CutQualification, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	if err := s.checkParentHouseholdForSwimmer(principal, orgID, swimmerID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ts, ok := s.standards[standardID]
	if !ok || ts.OrgID != orgID || ts.DeletedAt != nil {
		return nil, ErrStandardNotFound
	}

	// O(1) lookup via secondary index.
	key := btKey(orgID, swimmerID, ts.EventDistance, ts.Stroke, ts.Course)
	bt := s.btByKey[key]

	result := &CutQualification{
		SwimmerID:     swimmerID,
		StandardID:    standardID,
		StandardCode:  ts.StandardCode,
		EventDistance: ts.EventDistance,
		Stroke:        ts.Stroke,
		Course:        ts.Course,
		Gender:        ts.Gender,
		CutTimeMs:     ts.TimeMs,
	}

	if bt != nil && bt.BestTimeMs != nil {
		btMs := *bt.BestTimeMs
		result.BestTimeMs = bt.BestTimeMs
		result.Qualified = btMs <= ts.TimeMs
		margin := ts.TimeMs - btMs // positive = swimmer is under the cut
		result.MarginMs = &margin
	}
	// If no BT on record, Qualified stays false, BestTimeMs stays nil.

	return result, nil
}
