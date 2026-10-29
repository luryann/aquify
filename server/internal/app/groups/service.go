package groups

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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

var (
	ErrForbidden             = errors.New("forbidden")
	ErrGroupNotFound         = errors.New("group not found")
	ErrGroupCodeConflict     = errors.New("group code already exists in org")
	ErrRegistrationNotFound  = errors.New("registration not found")
	ErrWaiverAlreadyAccepted = errors.New("waiver already accepted for this registration")
	ErrInvalidTransition     = errors.New("invalid registration status transition")
	ErrSwimmerNotFound       = errors.New("swimmer not found")
	ErrHouseholdNotFound     = errors.New("household not found")
	ErrDuplicateRegistration = errors.New("swimmer already registered to this group for this season")
)

// Group defines a swim training group within an org.
type Group struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Type        string    `json:"type"`
	MinAge      *int      `json:"min_age,omitempty"`
	MaxAge      *int      `json:"max_age,omitempty"`
	CoachUserID string    `json:"coach_user_id,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by"`
	UpdatedBy   string    `json:"updated_by"`
}

// CreateGroupInput holds validated fields for group creation.
type CreateGroupInput struct {
	Code        string
	Name        string
	Description string
	Type        string
	MinAge      *int
	MaxAge      *int
	CoachUserID string
}

// UpdateGroupInput holds optional fields for group updates (nil = no change).
type UpdateGroupInput struct {
	Name        *string
	Description *string
	Type        *string
	MinAge      *int
	MaxAge      *int
	CoachUserID *string
	IsActive    *bool
}

// EligibilityRule defines a single rule that must pass for a swimmer to be eligible for a group.
type EligibilityRule struct {
	ID         string         `json:"id"`
	OrgID      string         `json:"org_id"`
	GroupID    string         `json:"group_id"`
	RuleType   string         `json:"rule_type"`
	Parameters map[string]any `json:"parameters"`
	IsActive   bool           `json:"is_active"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	CreatedBy  string         `json:"created_by"`
	UpdatedBy  string         `json:"updated_by"`
}

// AttachEligibilityRuleInput holds fields for attaching a rule to a group.
type AttachEligibilityRuleInput struct {
	RuleType   string
	Parameters map[string]any
}

// EligibilityResult summarizes swimmer eligibility evaluation against a group.
type EligibilityResult struct {
	GroupID     string           `json:"group_id"`
	SwimmerID   string           `json:"swimmer_id"`
	Eligible    bool             `json:"eligible"`
	RuleResults []RuleEvaluation `json:"rule_results"`
	EvaluatedAt time.Time        `json:"evaluated_at"`
}

// RuleEvaluation captures the pass/fail result for a single rule.
type RuleEvaluation struct {
	RuleID   string `json:"rule_id"`
	RuleType string `json:"rule_type"`
	Passed   bool   `json:"passed"`
	Reason   string `json:"reason,omitempty"`
}

// Registration links a swimmer to a group for a season.
type Registration struct {
	ID               string     `json:"id"`
	OrgID            string     `json:"org_id"`
	HouseholdID      string     `json:"household_id"`
	SwimmerID        string     `json:"swimmer_id"`
	GroupID          string     `json:"group_id"`
	Season           string     `json:"season"`
	Status           string     `json:"status"`
	RegistrationDate string     `json:"registration_date"`
	Notes            string     `json:"notes,omitempty"`
	WaiverAccepted   bool       `json:"waiver_accepted"`
	WaiverCount      int        `json:"waiver_count"`
	WaiverTypes      []string   `json:"waiver_types,omitempty"`
	LastWaiverAt     *time.Time `json:"last_waiver_accepted_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CreatedBy        string     `json:"created_by"`
	UpdatedBy        string     `json:"updated_by"`
}

// CreateRegistrationInput holds fields for registration creation.
type CreateRegistrationInput struct {
	HouseholdID      string
	SwimmerID        string
	GroupID          string
	Season           string
	RegistrationDate string
	Notes            string
}

// WaiverAcceptance records a waiver acceptance for a registration.
type WaiverAcceptance struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	RegistrationID   string    `json:"registration_id"`
	WaiverType       string    `json:"waiver_type"`
	AcceptedAt       time.Time `json:"accepted_at"`
	AcceptedByUserID string    `json:"accepted_by_user_id"`
	IPAddress        string    `json:"ip_address,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// AcceptWaiverInput holds fields for recording waiver acceptance.
type AcceptWaiverInput struct {
	WaiverType string
	IPAddress  string
}

// GroupAuditEvent records a privileged action taken on a group, eligibility rule, or registration.
type GroupAuditEvent struct {
	ID          string         `json:"id"`
	OrgID       string         `json:"org_id"`
	ActorUserID string         `json:"actor_user_id"`
	Action      string         `json:"action"`
	Before      map[string]any `json:"before,omitempty"`
	After       map[string]any `json:"after,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// swimmerRef is a minimal swimmer representation used for eligibility evaluation.
type swimmerRef struct {
	ID                 string
	OrgID              string
	HouseholdID        string
	DateOfBirth        string
	Gender             string
	RegistrationStatus string
}

// householdRef is a minimal household representation for scope checks.
type householdRef struct {
	ID    string
	OrgID string
}

// Service is the in-memory groups/eligibility/registrations service.
type Service struct {
	// syncHook runs after a reload has read Postgres and before it replaces the
	// in-memory state; tests use it to land a write inside that window.
	syncHook        func()
	mu              sync.RWMutex
	repo            *Repository // nil = in-memory only
	groups          map[string]Group
	rules           map[string]EligibilityRule
	registrations   map[string]Registration
	waivers         map[string]WaiverAcceptance
	swimmers        map[string]swimmerRef
	households      map[string]householdRef
	parentHousehold map[string]map[string]struct{}
	auditEvents     []GroupAuditEvent
	// onRegistrationActivated runs (outside the lock) when a registration
	// becomes active, so the swimmer joins the group's roster.
	onRegistrationActivated func(reg Registration, groupCode, actorUserID string)
}

// SetRegistrationActivatedHook registers a callback fired after a registration
// transitions to "active". Wire it once at startup.
func (s *Service) SetRegistrationActivatedHook(fn func(reg Registration, groupCode, actorUserID string)) {
	s.onRegistrationActivated = fn
}

// NewService returns a new in-memory groups service.
func NewService() *Service {
	return &Service{
		groups:          make(map[string]Group),
		rules:           make(map[string]EligibilityRule),
		registrations:   make(map[string]Registration),
		waivers:         make(map[string]WaiverAcceptance),
		swimmers:        make(map[string]swimmerRef),
		households:      make(map[string]householdRef),
		parentHousehold: make(map[string]map[string]struct{}),
		auditEvents:     []GroupAuditEvent{},
	}
}

// NewServiceWithPool creates a DB-backed groups service.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	if err := s.syncFromDB(context.Background()); err != nil {
		slog.Error("groups: failed to load initial state from DB", "error", err)
	}
	return s
}

// SetSyncHookForTest installs a hook run inside syncFromDB between reading
// Postgres and replacing memory. Set it before the service is used concurrently.
func (s *Service) SetSyncHookForTest(fn func()) { s.syncHook = fn }

// syncFromDB reloads the in-memory maps from the database (no-op without a repository).
func (s *Service) syncFromDB(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	// Load under the write lock so a concurrent write cannot be overwritten by an
	// older snapshot (writers persist and update memory while holding it).
	s.mu.Lock()
	defer s.mu.Unlock()
	grps, rules, regs, waivers, swimmers, households, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	if s.syncHook != nil {
		s.syncHook()
	}
	s.resetDBBackedStateLocked()
	s.populateDBBackedStateLocked(grps, rules, regs, waivers, swimmers, households)
	return nil
}

func (s *Service) resetDBBackedStateLocked() {
	s.groups = make(map[string]Group)
	s.rules = make(map[string]EligibilityRule)
	s.registrations = make(map[string]Registration)
	s.waivers = make(map[string]WaiverAcceptance)
	s.swimmers = make(map[string]swimmerRef)
	s.households = make(map[string]householdRef)
}

func (s *Service) populateDBBackedStateLocked(grps []Group, rules []EligibilityRule, regs []Registration, waivers []WaiverAcceptance, swimmers []swimmerRef, households []householdRef) {
	for _, g := range grps {
		s.groups[g.ID] = g
	}
	for _, r := range rules {
		s.rules[r.ID] = r
	}
	for _, r := range regs {
		s.registrations[r.ID] = r
	}
	for _, w := range waivers {
		s.waivers[w.ID] = w
	}
	for _, swimmer := range swimmers {
		s.swimmers[swimmer.ID] = swimmer
	}
	for _, household := range households {
		s.households[household.ID] = household
	}
}

func (s *Service) newID(prefix string) string {
	if s.repo != nil {
		return uuid.New().String()
	}
	return prefix + randomToken(12)
}

// SeedSwimmer allows test harnesses (or the members service) to register a swimmer
// for eligibility evaluation. In production this would be a shared DB read.
func (s *Service) SeedSwimmer(orgID, swimmerID, householdID, dob, gender, regStatus string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.swimmers[swimmerID] = swimmerRef{
		ID:                 swimmerID,
		OrgID:              orgID,
		HouseholdID:        householdID,
		DateOfBirth:        dob,
		Gender:             gender,
		RegistrationStatus: regStatus,
	}
	s.households[householdID] = householdRef{ID: householdID, OrgID: orgID}
}

// SeedParentHousehold links a parent user to a household for scope enforcement.
func (s *Service) SeedParentHousehold(parentUserID, householdID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parentHousehold[parentUserID] == nil {
		s.parentHousehold[parentUserID] = make(map[string]struct{})
	}
	s.parentHousehold[parentUserID][householdID] = struct{}{}
}

// ─── Groups ──────────────────────────────────────────────────────────────────

// CreateGroup creates a new training group.
func (s *Service) CreateGroup(principal *appmiddleware.Principal, orgID string, input CreateGroupInput) (Group, GroupAuditEvent, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach") {
		return Group{}, GroupAuditEvent{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Group{}, GroupAuditEvent{}, err
	}

	code := strings.TrimSpace(strings.ToUpper(input.Code))
	name := strings.TrimSpace(input.Name)
	if code == "" || name == "" {
		return Group{}, GroupAuditEvent{}, errors.New("code and name are required")
	}
	if err := validateGroupType(input.Type); err != nil {
		return Group{}, GroupAuditEvent{}, err
	}
	if input.MinAge != nil && input.MaxAge != nil && *input.MinAge > *input.MaxAge {
		return Group{}, GroupAuditEvent{}, errors.New("min_age cannot exceed max_age")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, g := range s.groups {
		if g.OrgID == orgID && g.Code == code {
			return Group{}, GroupAuditEvent{}, ErrGroupCodeConflict
		}
	}

	now := time.Now().UTC()
	groupType := strings.TrimSpace(strings.ToLower(input.Type))
	if groupType == "" {
		groupType = "competitive"
	}
	group := Group{
		ID:          s.newID("grp_"),
		OrgID:       orgID,
		Code:        code,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Type:        groupType,
		MinAge:      input.MinAge,
		MaxAge:      input.MaxAge,
		CoachUserID: strings.TrimSpace(input.CoachUserID),
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   principal.UserID,
		UpdatedBy:   principal.UserID,
	}
	if s.repo != nil {
		if err := s.repo.UpsertGroup(context.Background(), group); err != nil {
			return Group{}, GroupAuditEvent{}, mapPersistenceError(err)
		}
	}
	s.groups[group.ID] = group

	event := s.recordAuditEventLocked(GroupAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "group.created",
		After:       groupSnapshot(group),
	})
	return group, event, nil
}

// UpdateGroup updates a group's mutable fields.
func (s *Service) UpdateGroup(principal *appmiddleware.Principal, orgID, groupID string, input UpdateGroupInput) (Group, GroupAuditEvent, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach") {
		return Group{}, GroupAuditEvent{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Group{}, GroupAuditEvent{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.groupLocked(orgID, groupID)
	if !ok {
		return Group{}, GroupAuditEvent{}, ErrGroupNotFound
	}

	before := groupSnapshot(group)

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return Group{}, GroupAuditEvent{}, errors.New("name cannot be empty")
		}
		group.Name = name
	}
	if input.Description != nil {
		group.Description = strings.TrimSpace(*input.Description)
	}
	if input.Type != nil {
		t := strings.TrimSpace(strings.ToLower(*input.Type))
		if err := validateGroupType(t); err != nil {
			return Group{}, GroupAuditEvent{}, err
		}
		group.Type = t
	}
	if input.MinAge != nil {
		group.MinAge = input.MinAge
	}
	if input.MaxAge != nil {
		group.MaxAge = input.MaxAge
	}
	if group.MinAge != nil && group.MaxAge != nil && *group.MinAge > *group.MaxAge {
		return Group{}, GroupAuditEvent{}, errors.New("min_age cannot exceed max_age")
	}
	if input.CoachUserID != nil {
		group.CoachUserID = strings.TrimSpace(*input.CoachUserID)
	}
	if input.IsActive != nil {
		group.IsActive = *input.IsActive
	}

	group.UpdatedAt = time.Now().UTC()
	group.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertGroup(context.Background(), group); err != nil {
			return Group{}, GroupAuditEvent{}, mapPersistenceError(err)
		}
	}
	s.groups[group.ID] = group

	event := s.recordAuditEventLocked(GroupAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "group.updated",
		Before:      before,
		After:       groupSnapshot(group),
		Metadata:    map[string]any{"group_id": group.ID},
	})
	return group, event, nil
}

// GetGroup retrieves a single group by ID.
func (s *Service) GetGroup(principal *appmiddleware.Principal, orgID, groupID string) (Group, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		return Group{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Group{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.groupLocked(orgID, groupID)
	if !ok {
		return Group{}, ErrGroupNotFound
	}
	return group, nil
}

// ListGroups returns groups in an org. Coaches without admin access only see their own groups.
func (s *Service) ListGroups(principal *appmiddleware.Principal, orgID string, activeOnly bool) ([]Group, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Group
	for _, g := range s.groups {
		if g.OrgID != orgID {
			continue
		}
		if activeOnly && !g.IsActive {
			continue
		}
		// Group-scoped coach visibility: coaches see only groups they coach,
		// unless they're also admin.
		if appmiddleware.HasAnyRole(principal, "coach") && !appmiddleware.HasAnyRole(principal, "admin") {
			if g.CoachUserID != principal.UserID {
				continue
			}
		}
		result = append(result, g)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Code != result[j].Code {
			return result[i].Code < result[j].Code
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

// ─── Eligibility Rules ───────────────────────────────────────────────────────

// AttachEligibilityRule attaches a new eligibility rule to a group.
func (s *Service) AttachEligibilityRule(principal *appmiddleware.Principal, orgID, groupID string, input AttachEligibilityRuleInput) (EligibilityRule, GroupAuditEvent, error) {
	if !appmiddleware.HasAnyRole(principal, "admin") {
		return EligibilityRule{}, GroupAuditEvent{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return EligibilityRule{}, GroupAuditEvent{}, err
	}

	ruleType := strings.TrimSpace(strings.ToLower(input.RuleType))
	if err := validateRuleType(ruleType); err != nil {
		return EligibilityRule{}, GroupAuditEvent{}, err
	}
	if input.Parameters == nil {
		input.Parameters = map[string]any{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.groupLocked(orgID, groupID)
	if !ok {
		return EligibilityRule{}, GroupAuditEvent{}, ErrGroupNotFound
	}

	now := time.Now().UTC()
	rule := EligibilityRule{
		ID:         s.newID("rul_"),
		OrgID:      orgID,
		GroupID:    group.ID,
		RuleType:   ruleType,
		Parameters: input.Parameters,
		IsActive:   true,
		CreatedAt:  now,
		UpdatedAt:  now,
		CreatedBy:  principal.UserID,
		UpdatedBy:  principal.UserID,
	}
	if s.repo != nil {
		if err := s.repo.UpsertEligibilityRule(context.Background(), rule); err != nil {
			return EligibilityRule{}, GroupAuditEvent{}, mapPersistenceError(err)
		}
	}
	s.rules[rule.ID] = rule

	event := s.recordAuditEventLocked(GroupAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "group.eligibility_rule_attached",
		After:       map[string]any{"rule_id": rule.ID, "rule_type": rule.RuleType, "parameters": rule.Parameters},
		Metadata:    map[string]any{"group_id": group.ID},
	})
	return rule, event, nil
}

// EvaluateSwimmerEligibility evaluates a swimmer against all active rules for a group.
func (s *Service) EvaluateSwimmerEligibility(principal *appmiddleware.Principal, orgID, groupID, swimmerID string) (EligibilityResult, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach", "parent") {
		return EligibilityResult{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return EligibilityResult{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.groupLocked(orgID, groupID)
	if !ok {
		return EligibilityResult{}, ErrGroupNotFound
	}

	swimmer, ok := s.swimmers[strings.TrimSpace(swimmerID)]
	if !ok || swimmer.OrgID != orgID {
		return EligibilityResult{}, ErrSwimmerNotFound
	}

	// Parent scope: can only evaluate own household swimmers.
	if appmiddleware.IsParentOnly(principal) {
		if !s.principalOwnsHouseholdLocked(principal, swimmer.HouseholdID) {
			return EligibilityResult{}, ErrForbidden
		}
	}

	var ruleResults []RuleEvaluation
	allPassed := true

	for _, rule := range s.rules {
		if rule.GroupID != group.ID || !rule.IsActive {
			continue
		}
		eval := s.evalRule(rule, swimmer)
		ruleResults = append(ruleResults, eval)
		if !eval.Passed {
			allPassed = false
		}
	}

	return EligibilityResult{
		GroupID:     group.ID,
		SwimmerID:   swimmer.ID,
		Eligible:    allPassed,
		RuleResults: ruleResults,
		EvaluatedAt: time.Now().UTC(),
	}, nil
}

func (s *Service) ListEligibilityRules(principal *appmiddleware.Principal, orgID, groupID string) ([]EligibilityRule, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach", "parent") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.groupLocked(orgID, groupID)
	if !ok {
		return nil, ErrGroupNotFound
	}

	out := make([]EligibilityRule, 0)
	for _, rule := range s.rules {
		if rule.OrgID != orgID || rule.GroupID != group.ID {
			continue
		}
		out = append(out, rule)
	}
	slices.SortFunc(out, func(a, b EligibilityRule) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	return out, nil
}

// evalRule evaluates a single rule against a swimmer.
func (s *Service) evalRule(rule EligibilityRule, swimmer swimmerRef) RuleEvaluation {
	eval := RuleEvaluation{
		RuleID:   rule.ID,
		RuleType: rule.RuleType,
		Passed:   true,
	}

	switch rule.RuleType {
	case "age_range":
		age := computeAge(swimmer.DateOfBirth)
		if age < 0 {
			eval.Passed = false
			eval.Reason = "could not compute swimmer age from date_of_birth"
			break
		}
		if minAge, ok := rule.Parameters["min_age"]; ok {
			min := toInt(minAge)
			if age < min {
				eval.Passed = false
				eval.Reason = fmt.Sprintf("swimmer age %d is below min_age %d", age, min)
			}
		}
		if maxAge, ok := rule.Parameters["max_age"]; ok {
			max := toInt(maxAge)
			if age > max {
				eval.Passed = false
				eval.Reason = fmt.Sprintf("swimmer age %d exceeds max_age %d", age, max)
			}
		}

	case "gender":
		allowed := toStringSlice(rule.Parameters["allowed"])
		if len(allowed) > 0 && !slices.Contains(allowed, swimmer.Gender) {
			eval.Passed = false
			eval.Reason = fmt.Sprintf("swimmer gender %q not in allowed list", swimmer.Gender)
		}

	case "registration_status":
		allowed := toStringSlice(rule.Parameters["allowed"])
		if len(allowed) > 0 && !slices.Contains(allowed, swimmer.RegistrationStatus) {
			eval.Passed = false
			eval.Reason = fmt.Sprintf("swimmer registration_status %q not in allowed list", swimmer.RegistrationStatus)
		}

	default:
		eval.Reason = fmt.Sprintf("unknown rule type %q — treated as pass", rule.RuleType)
	}

	return eval
}

// ─── Registrations ───────────────────────────────────────────────────────────

// CreateRegistration creates a new registration for a swimmer in a group.
func (s *Service) CreateRegistration(principal *appmiddleware.Principal, orgID string, input CreateRegistrationInput) (Registration, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach", "parent") {
		return Registration{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Registration{}, err
	}

	swimmerID := strings.TrimSpace(input.SwimmerID)
	householdID := strings.TrimSpace(input.HouseholdID)
	groupID := strings.TrimSpace(input.GroupID)
	season := strings.TrimSpace(input.Season)

	if swimmerID == "" || householdID == "" || groupID == "" || season == "" {
		return Registration{}, errors.New("swimmer_id, household_id, group_id, and season are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.groups[groupID]
	if !ok || group.OrgID != orgID {
		return Registration{}, ErrGroupNotFound
	}

	household, ok := s.households[householdID]
	if !ok || household.OrgID != orgID {
		return Registration{}, ErrHouseholdNotFound
	}

	swimmer, ok := s.swimmers[swimmerID]
	if !ok || swimmer.OrgID != orgID {
		return Registration{}, ErrSwimmerNotFound
	}

	if swimmer.HouseholdID != householdID {
		return Registration{}, errors.New("swimmer does not belong to the specified household")
	}

	// Parent scope: can only register swimmers from their own household.
	if appmiddleware.IsParentOnly(principal) {
		if !s.principalOwnsHouseholdLocked(principal, householdID) {
			return Registration{}, ErrForbidden
		}
	}

	// Duplicate check: swimmer+group+season must be unique per org.
	for _, reg := range s.registrations {
		if reg.OrgID == orgID && reg.SwimmerID == swimmerID && reg.GroupID == groupID && reg.Season == season {
			return Registration{}, ErrDuplicateRegistration
		}
	}

	regDate := strings.TrimSpace(input.RegistrationDate)
	if regDate == "" {
		regDate = time.Now().UTC().Format("2006-01-02")
	}

	now := time.Now().UTC()
	reg := Registration{
		ID:               s.newID("reg_"),
		OrgID:            orgID,
		HouseholdID:      householdID,
		SwimmerID:        swimmerID,
		GroupID:          groupID,
		Season:           season,
		Status:           "pending",
		RegistrationDate: regDate,
		Notes:            strings.TrimSpace(input.Notes),
		CreatedAt:        now,
		UpdatedAt:        now,
		CreatedBy:        principal.UserID,
		UpdatedBy:        principal.UserID,
	}
	if s.repo != nil {
		if err := s.repo.UpsertRegistration(context.Background(), reg); err != nil {
			return Registration{}, mapPersistenceError(err)
		}
	}
	s.registrations[reg.ID] = reg
	return reg, nil
}

// TransitionRegistrationStatus moves a registration through its status state machine.
func (s *Service) TransitionRegistrationStatus(principal *appmiddleware.Principal, orgID, registrationID, toStatus string) (Registration, GroupAuditEvent, error) {
	reg, event, err := s.transitionRegistrationStatus(principal, orgID, registrationID, toStatus)
	if err == nil && reg.Status == "active" && s.onRegistrationActivated != nil {
		s.mu.RLock()
		groupCode := s.groups[reg.GroupID].Code
		s.mu.RUnlock()
		if groupCode != "" {
			s.onRegistrationActivated(reg, groupCode, principal.UserID)
		}
	}
	return reg, event, err
}

func (s *Service) transitionRegistrationStatus(principal *appmiddleware.Principal, orgID, registrationID, toStatus string) (Registration, GroupAuditEvent, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach", "parent") {
		return Registration{}, GroupAuditEvent{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Registration{}, GroupAuditEvent{}, err
	}

	toStatus = strings.TrimSpace(strings.ToLower(toStatus))

	s.mu.Lock()
	defer s.mu.Unlock()

	reg, ok := s.registrations[strings.TrimSpace(registrationID)]
	if !ok || reg.OrgID != orgID {
		return Registration{}, GroupAuditEvent{}, ErrRegistrationNotFound
	}

	// Parents may only transition registrations belonging to their own
	// household — otherwise a parent who learns another household's
	// registration ID could withdraw it (cross-household IDOR).
	if appmiddleware.IsParentOnly(principal) {
		if !s.principalOwnsHouseholdLocked(principal, reg.HouseholdID) {
			return Registration{}, GroupAuditEvent{}, ErrRegistrationNotFound
		}
	}

	// Validate state machine transition.
	if err := validateTransition(reg.Status, toStatus, principal); err != nil {
		return Registration{}, GroupAuditEvent{}, err
	}

	fromStatus := reg.Status
	reg.Status = toStatus
	reg.UpdatedAt = time.Now().UTC()
	reg.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertRegistration(context.Background(), reg); err != nil {
			return Registration{}, GroupAuditEvent{}, mapPersistenceError(err)
		}
	}
	s.registrations[reg.ID] = reg

	event := s.recordAuditEventLocked(GroupAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "registration.status_changed",
		Before:      map[string]any{"status": fromStatus},
		After:       map[string]any{"status": toStatus},
		Metadata:    map[string]any{"registration_id": reg.ID, "swimmer_id": reg.SwimmerID},
	})
	return reg, event, nil
}

// ─── Waivers ─────────────────────────────────────────────────────────────────

// AcceptWaiver records a waiver acceptance for a registration.
func (s *Service) AcceptWaiver(principal *appmiddleware.Principal, orgID, registrationID string, input AcceptWaiverInput) (WaiverAcceptance, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "parent") {
		return WaiverAcceptance{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return WaiverAcceptance{}, err
	}

	waiverType := strings.TrimSpace(strings.ToLower(input.WaiverType))
	if waiverType == "" {
		return WaiverAcceptance{}, errors.New("waiver_type is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	reg, ok := s.registrations[strings.TrimSpace(registrationID)]
	if !ok || reg.OrgID != orgID {
		return WaiverAcceptance{}, ErrRegistrationNotFound
	}

	// Parent scope: can only accept waivers for their own household registrations.
	if appmiddleware.HasAnyRole(principal, "parent") && !appmiddleware.HasAnyRole(principal, "admin") {
		if !s.principalOwnsHouseholdLocked(principal, reg.HouseholdID) {
			return WaiverAcceptance{}, ErrForbidden
		}
	}

	// Idempotency: one acceptance per (registration, waiver_type).
	for _, w := range s.waivers {
		if w.RegistrationID == reg.ID && w.WaiverType == waiverType {
			return WaiverAcceptance{}, ErrWaiverAlreadyAccepted
		}
	}

	now := time.Now().UTC()
	waiver := WaiverAcceptance{
		ID:               s.newID("wvr_"),
		OrgID:            orgID,
		RegistrationID:   reg.ID,
		WaiverType:       waiverType,
		AcceptedAt:       now,
		AcceptedByUserID: principal.UserID,
		IPAddress:        strings.TrimSpace(input.IPAddress),
		CreatedAt:        now,
	}
	if s.repo != nil {
		if err := s.repo.UpsertWaiverAcceptance(context.Background(), waiver); err != nil {
			return WaiverAcceptance{}, mapPersistenceError(err)
		}
	}
	s.waivers[waiver.ID] = waiver
	return waiver, nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// groupLocked looks up a group in the org (callers hold s.mu).
func (s *Service) groupLocked(orgID, groupID string) (Group, bool) {
	group, ok := s.groups[strings.TrimSpace(groupID)]
	return group, ok && group.OrgID == orgID
}

// principalOwnsHouseholdLocked reports whether the principal's login household or
// an explicit parent link covers householdID.
func (s *Service) principalOwnsHouseholdLocked(principal *appmiddleware.Principal, householdID string) bool {
	return strings.TrimSpace(principal.HouseholdID) == householdID || s.parentOwnsHouseholdLocked(principal.UserID, householdID)
}

func (s *Service) parentOwnsHouseholdLocked(parentUserID, householdID string) bool {
	households := s.parentHousehold[parentUserID]
	_, ok := households[householdID]
	return ok
}

func validateGroupType(t string) error {
	allowed := []string{"competitive", "recreational", "developmental", "masters", "other"}
	t = strings.TrimSpace(strings.ToLower(t))
	if t == "" {
		return nil // defaults to "competitive"
	}
	if !slices.Contains(allowed, t) {
		return fmt.Errorf("type must be one of: %s", strings.Join(allowed, ", "))
	}
	return nil
}

func validateRuleType(t string) error {
	allowed := []string{"age_range", "gender", "registration_status"}
	if !slices.Contains(allowed, t) {
		return fmt.Errorf("rule_type must be one of: %s", strings.Join(allowed, ", "))
	}
	return nil
}

// validTransitions defines allowed state machine moves.
var validTransitions = map[string][]string{
	"pending":  {"active", "withdrawn"},
	"active":   {"inactive", "withdrawn"},
	"inactive": {"active", "withdrawn"},
}

func validateTransition(from, to string, principal *appmiddleware.Principal) error {
	allowed, ok := validTransitions[from]
	if !ok {
		return ErrInvalidTransition
	}
	if !slices.Contains(allowed, to) {
		return fmt.Errorf("%w: %s -> %s is not allowed", ErrInvalidTransition, from, to)
	}
	// Parents can only withdraw; they cannot activate or deactivate.
	if appmiddleware.IsParentOnly(principal) {
		if to != "withdrawn" {
			return fmt.Errorf("%w: parent role can only transition to withdrawn", ErrForbidden)
		}
	}
	return nil
}

func computeAge(dob string) int {
	t, err := time.Parse("2006-01-02", dob)
	if err != nil {
		return -1
	}
	now := time.Now().UTC()
	age := now.Year() - t.Year()
	// Subtract 1 if the birthday has not yet occurred this year.
	// Month/day comparison correctly handles Feb 29 leap-day birthdays.
	if now.Month() < t.Month() || (now.Month() == t.Month() && now.Day() < t.Day()) {
		age--
	}
	return age
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	}
	return 0
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func randomToken(byteLen int) string {
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Errorf("secure random unavailable for group token generation: %w", err))
	}
	return hex.EncodeToString(raw)
}

func (s *Service) recordAuditEventLocked(event GroupAuditEvent) GroupAuditEvent {
	event.ID = "aud_" + randomToken(12)
	event.CreatedAt = time.Now().UTC()
	s.auditEvents = append(s.auditEvents, event)
	return event
}

// SnapshotGroups returns a copy of all groups for the given org.
func (s *Service) SnapshotGroups(orgID string) []Group {
	if err := s.syncFromDB(context.Background()); err != nil {
		return []Group{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Group, 0, len(s.groups))
	for _, group := range s.groups {
		if group.OrgID != orgID {
			continue
		}
		out = append(out, group)
	}
	return out
}

// SnapshotRegistrations returns a copy of all registrations for the given org.
func (s *Service) SnapshotRegistrations(orgID string) []Registration {
	if err := s.syncFromDB(context.Background()); err != nil {
		return []Registration{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Registration, 0, len(s.registrations))
	for _, registration := range s.registrations {
		if registration.OrgID != orgID {
			continue
		}
		out = append(out, registration)
	}
	return out
}

// ListRegistrations returns registration rows visible to the principal. Parent
// access is strictly limited to households linked to the parent principal.
func (s *Service) ListRegistrations(principal *appmiddleware.Principal, orgID string) ([]Registration, error) {
	if !appmiddleware.HasAnyRole(principal, "admin", "coach", "parent") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return []Registration{}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	parentScoped := appmiddleware.IsParentOnly(principal)
	allowedHouseholds := make(map[string]struct{})
	if parentScoped {
		if householdID := strings.TrimSpace(principal.HouseholdID); householdID != "" {
			allowedHouseholds[householdID] = struct{}{}
		}
		for householdID := range s.parentHousehold[principal.UserID] {
			allowedHouseholds[householdID] = struct{}{}
		}
	}

	waiverTypesByRegistration := make(map[string][]string)
	lastWaiverByRegistration := make(map[string]time.Time)
	for _, waiver := range s.waivers {
		if waiver.OrgID != orgID {
			continue
		}
		waiverTypesByRegistration[waiver.RegistrationID] = append(waiverTypesByRegistration[waiver.RegistrationID], waiver.WaiverType)
		if waiver.AcceptedAt.After(lastWaiverByRegistration[waiver.RegistrationID]) {
			lastWaiverByRegistration[waiver.RegistrationID] = waiver.AcceptedAt
		}
	}

	out := make([]Registration, 0, len(s.registrations))
	for _, registration := range s.registrations {
		if registration.OrgID != orgID {
			continue
		}
		if parentScoped {
			if _, ok := allowedHouseholds[registration.HouseholdID]; !ok {
				continue
			}
		}
		cp := registration
		cp.WaiverTypes = slices.Clone(waiverTypesByRegistration[registration.ID])
		slices.Sort(cp.WaiverTypes)
		cp.WaiverCount = len(cp.WaiverTypes)
		cp.WaiverAccepted = cp.WaiverCount > 0
		if lastWaiverAt, ok := lastWaiverByRegistration[registration.ID]; ok {
			timestamp := lastWaiverAt
			cp.LastWaiverAt = &timestamp
		}
		out = append(out, cp)
	}

	slices.SortFunc(out, func(a, b Registration) int {
		if a.RegistrationDate != b.RegistrationDate {
			return strings.Compare(b.RegistrationDate, a.RegistrationDate)
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			if a.CreatedAt.After(b.CreatedAt) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

// SnapshotWaivers returns a copy of all waiver acceptances for the given org.
func (s *Service) SnapshotWaivers(orgID string) []WaiverAcceptance {
	if err := s.syncFromDB(context.Background()); err != nil {
		return []WaiverAcceptance{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]WaiverAcceptance, 0, len(s.waivers))
	for _, waiver := range s.waivers {
		if waiver.OrgID != orgID {
			continue
		}
		out = append(out, waiver)
	}
	return out
}

func groupSnapshot(g Group) map[string]any {
	return map[string]any{
		"name":          g.Name,
		"description":   g.Description,
		"type":          g.Type,
		"min_age":       g.MinAge,
		"max_age":       g.MaxAge,
		"coach_user_id": g.CoachUserID,
		"is_active":     g.IsActive,
	}
}

func mapPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "groups_code_org_unique"):
		return ErrGroupCodeConflict
	case strings.Contains(msg, "registrations_swimmer_group_season_unique"):
		return ErrDuplicateRegistration
	case strings.Contains(msg, "waiver_acceptances_registration_type_unique"):
		return ErrWaiverAlreadyAccepted
	default:
		return err
	}
}
