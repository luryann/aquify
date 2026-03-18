package members

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

var (
	ErrForbidden              = errors.New("forbidden")
	ErrHouseholdNotFound      = errors.New("household not found")
	ErrSwimmerNotFound        = errors.New("swimmer not found")
	ErrMedicalFlagsRestricted = errors.New("medical flags can only be changed by owner/admin/coach")
	ErrInvalidDateOfBirth     = errors.New("date_of_birth must use YYYY-MM-DD format")
	ErrParentHouseholdScope   = errors.New("parent can only access swimmers in their own household")
)

type Service struct {
	mu                   sync.RWMutex
	households           map[string]Household
	contacts             map[string]Contact
	swimmers             map[string]Swimmer
	swimmerGroupLinks    map[string]SwimmerGroupLink
	parentHouseholdIndex map[string]map[string]struct{}
	auditEvents          []SwimmerAuditEvent
}

func NewService() *Service {
	return &Service{
		households:           make(map[string]Household),
		contacts:             make(map[string]Contact),
		swimmers:             make(map[string]Swimmer),
		swimmerGroupLinks:    make(map[string]SwimmerGroupLink),
		parentHouseholdIndex: make(map[string]map[string]struct{}),
		auditEvents:          []SwimmerAuditEvent{},
	}
}

type Household struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	HouseholdName    string    `json:"household_name"`
	PrimaryEmail     string    `json:"primary_email,omitempty"`
	PrimaryPhoneE164 string    `json:"primary_phone_e164,omitempty"`
	Status           string    `json:"status"`
	Notes            string    `json:"notes,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedBy        string    `json:"created_by"`
	UpdatedBy        string    `json:"updated_by"`
}

type CreateHouseholdInput struct {
	OrgID            string
	HouseholdName    string
	PrimaryEmail     string
	PrimaryPhoneE164 string
	Status           string
	Notes            string
}

type UpdateHouseholdInput struct {
	HouseholdName    *string
	PrimaryEmail     *string
	PrimaryPhoneE164 *string
	Status           *string
	Notes            *string
}

type Contact struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"org_id"`
	HouseholdID        string    `json:"household_id"`
	Relationship       string    `json:"relationship"`
	FirstName          string    `json:"first_name"`
	LastName           string    `json:"last_name"`
	Email              string    `json:"email,omitempty"`
	PhoneE164          string    `json:"phone_e164,omitempty"`
	IsPrimary          bool      `json:"is_primary"`
	IsEmergencyContact bool      `json:"is_emergency_contact"`
	CanPickUp          bool      `json:"can_pick_up"`
	EmergencyPriority  *int      `json:"emergency_priority,omitempty"`
	Notes              string    `json:"notes,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	CreatedBy          string    `json:"created_by"`
	UpdatedBy          string    `json:"updated_by"`
}

type AddContactInput struct {
	Relationship       string
	FirstName          string
	LastName           string
	Email              string
	PhoneE164          string
	IsPrimary          bool
	IsEmergencyContact bool
	CanPickUp          bool
	EmergencyPriority  *int
	Notes              string
}

type Swimmer struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"org_id"`
	HouseholdID        string    `json:"household_id"`
	FirstName          string    `json:"first_name"`
	LastName           string    `json:"last_name"`
	PreferredName      string    `json:"preferred_name,omitempty"`
	DateOfBirth        string    `json:"date_of_birth"`
	Gender             string    `json:"gender"`
	SwimmerExternalID  string    `json:"swimmer_external_id,omitempty"`
	RegistrationStatus string    `json:"registration_status"`
	MedicalFlags       []string  `json:"medical_flags,omitempty"`
	Notes              string    `json:"notes,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	CreatedBy          string    `json:"created_by"`
	UpdatedBy          string    `json:"updated_by"`
}

type SwimmerView struct {
	Swimmer
	MedicalFlagsRedacted bool `json:"medical_flags_redacted"`
}

type CreateSwimmerInput struct {
	HouseholdID        string
	FirstName          string
	LastName           string
	PreferredName      string
	DateOfBirth        string
	Gender             string
	SwimmerExternalID  string
	RegistrationStatus string
	MedicalFlags       []string
	Notes              string
}

type UpdateSwimmerInput struct {
	FirstName          *string
	LastName           *string
	PreferredName      *string
	DateOfBirth        *string
	Gender             *string
	SwimmerExternalID  *string
	RegistrationStatus *string
	MedicalFlags       *[]string
	Notes              *string
}

type SwimmerGroupLink struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	SwimmerID  string    `json:"swimmer_id"`
	GroupCode  string    `json:"group_code"`
	IsPrimary  bool      `json:"is_primary"`
	AssignedAt time.Time `json:"assigned_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	CreatedBy  string    `json:"created_by"`
	UpdatedBy  string    `json:"updated_by"`
}

type AssignSwimmerGroupInput struct {
	GroupCode string
	IsPrimary bool
}

type ListSwimmersInput struct {
	HouseholdID        string
	GroupCode          string
	RegistrationStatus string
	IncludeMedical     bool
}

type SwimmerAuditEvent struct {
	ID          string         `json:"id"`
	OrgID       string         `json:"org_id"`
	SwimmerID   string         `json:"swimmer_id"`
	ActorUserID string         `json:"actor_user_id"`
	Action      string         `json:"action"`
	Before      map[string]any `json:"before,omitempty"`
	After       map[string]any `json:"after,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (s *Service) CreateHousehold(principal *appmiddleware.Principal, input CreateHouseholdInput) (Household, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return Household{}, ErrForbidden
	}

	household := Household{
		ID:               "hsh_" + randomToken(12),
		OrgID:            strings.TrimSpace(input.OrgID),
		HouseholdName:    strings.TrimSpace(input.HouseholdName),
		PrimaryEmail:     strings.TrimSpace(strings.ToLower(input.PrimaryEmail)),
		PrimaryPhoneE164: strings.TrimSpace(input.PrimaryPhoneE164),
		Status:           strings.TrimSpace(strings.ToLower(input.Status)),
		Notes:            strings.TrimSpace(input.Notes),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		CreatedBy:        principal.UserID,
		UpdatedBy:        principal.UserID,
	}
	if household.Status == "" {
		household.Status = "active"
	}
	if household.OrgID == "" || household.HouseholdName == "" {
		return Household{}, errors.New("org_id and household_name are required")
	}

	s.mu.Lock()
	s.households[household.ID] = household
	if hasAnyRole(principal, "parent") {
		s.associateParentHouseholdLocked(principal.UserID, household.ID)
	}
	s.mu.Unlock()

	return household, nil
}

func (s *Service) UpdateHousehold(principal *appmiddleware.Principal, orgID, householdID string, input UpdateHouseholdInput) (Household, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return Household{}, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	household, ok := s.households[householdID]
	if !ok || household.OrgID != orgID {
		return Household{}, ErrHouseholdNotFound
	}
	if err := s.enforceParentHouseholdAccess(principal, householdID); err != nil {
		return Household{}, err
	}

	if input.HouseholdName != nil {
		household.HouseholdName = strings.TrimSpace(*input.HouseholdName)
	}
	if input.PrimaryEmail != nil {
		household.PrimaryEmail = strings.TrimSpace(strings.ToLower(*input.PrimaryEmail))
	}
	if input.PrimaryPhoneE164 != nil {
		household.PrimaryPhoneE164 = strings.TrimSpace(*input.PrimaryPhoneE164)
	}
	if input.Status != nil {
		household.Status = strings.TrimSpace(strings.ToLower(*input.Status))
	}
	if input.Notes != nil {
		household.Notes = strings.TrimSpace(*input.Notes)
	}
	household.UpdatedAt = time.Now().UTC()
	household.UpdatedBy = principal.UserID
	if household.HouseholdName == "" {
		return Household{}, errors.New("household_name cannot be empty")
	}

	s.households[household.ID] = household
	return household, nil
}

func (s *Service) AddContact(principal *appmiddleware.Principal, orgID, householdID string, input AddContactInput) (Contact, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return Contact{}, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	household, ok := s.households[householdID]
	if !ok || household.OrgID != orgID {
		return Contact{}, ErrHouseholdNotFound
	}
	if err := s.enforceParentHouseholdAccess(principal, householdID); err != nil {
		return Contact{}, err
	}

	contact := Contact{
		ID:                 "ctc_" + randomToken(12),
		OrgID:              orgID,
		HouseholdID:        householdID,
		Relationship:       strings.TrimSpace(strings.ToLower(input.Relationship)),
		FirstName:          strings.TrimSpace(input.FirstName),
		LastName:           strings.TrimSpace(input.LastName),
		Email:              strings.TrimSpace(strings.ToLower(input.Email)),
		PhoneE164:          strings.TrimSpace(input.PhoneE164),
		IsPrimary:          input.IsPrimary,
		IsEmergencyContact: input.IsEmergencyContact,
		CanPickUp:          input.CanPickUp,
		EmergencyPriority:  input.EmergencyPriority,
		Notes:              strings.TrimSpace(input.Notes),
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
		CreatedBy:          principal.UserID,
		UpdatedBy:          principal.UserID,
	}
	if contact.Relationship == "" || contact.FirstName == "" || contact.LastName == "" {
		return Contact{}, errors.New("relationship, first_name, and last_name are required")
	}

	s.contacts[contact.ID] = contact
	return contact, nil
}

func (s *Service) CreateSwimmer(principal *appmiddleware.Principal, orgID string, input CreateSwimmerInput) (Swimmer, SwimmerAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return Swimmer{}, SwimmerAuditEvent{}, ErrForbidden
	}

	if _, err := time.Parse("2006-01-02", strings.TrimSpace(input.DateOfBirth)); err != nil {
		return Swimmer{}, SwimmerAuditEvent{}, ErrInvalidDateOfBirth
	}
	if !slices.Contains([]string{"female", "male", "non_binary", "undisclosed"}, strings.TrimSpace(strings.ToLower(input.Gender))) {
		return Swimmer{}, SwimmerAuditEvent{}, errors.New("gender must be one of: female, male, non_binary, undisclosed")
	}
	if strings.TrimSpace(input.RegistrationStatus) == "" {
		input.RegistrationStatus = "active"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	household, ok := s.households[strings.TrimSpace(input.HouseholdID)]
	if !ok || household.OrgID != orgID {
		return Swimmer{}, SwimmerAuditEvent{}, ErrHouseholdNotFound
	}
	if err := s.enforceParentHouseholdAccess(principal, household.ID); err != nil {
		return Swimmer{}, SwimmerAuditEvent{}, err
	}

	swimmer := Swimmer{
		ID:                 "swm_" + randomToken(12),
		OrgID:              orgID,
		HouseholdID:        household.ID,
		FirstName:          strings.TrimSpace(input.FirstName),
		LastName:           strings.TrimSpace(input.LastName),
		PreferredName:      strings.TrimSpace(input.PreferredName),
		DateOfBirth:        strings.TrimSpace(input.DateOfBirth),
		Gender:             strings.TrimSpace(strings.ToLower(input.Gender)),
		SwimmerExternalID:  strings.TrimSpace(input.SwimmerExternalID),
		RegistrationStatus: strings.TrimSpace(strings.ToLower(input.RegistrationStatus)),
		MedicalFlags:       normalizeFlags(input.MedicalFlags),
		Notes:              strings.TrimSpace(input.Notes),
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
		CreatedBy:          principal.UserID,
		UpdatedBy:          principal.UserID,
	}
	if swimmer.FirstName == "" || swimmer.LastName == "" || swimmer.HouseholdID == "" {
		return Swimmer{}, SwimmerAuditEvent{}, errors.New("household_id, first_name, and last_name are required")
	}
	if hasAnyRole(principal, "parent") && len(swimmer.MedicalFlags) > 0 {
		return Swimmer{}, SwimmerAuditEvent{}, ErrMedicalFlagsRestricted
	}

	if swimmer.SwimmerExternalID != "" {
		for _, existing := range s.swimmers {
			if existing.OrgID == orgID && existing.SwimmerExternalID != "" && strings.EqualFold(existing.SwimmerExternalID, swimmer.SwimmerExternalID) {
				return Swimmer{}, SwimmerAuditEvent{}, fmt.Errorf("swimmer_external_id %q already exists in org", swimmer.SwimmerExternalID)
			}
		}
	}

	s.swimmers[swimmer.ID] = swimmer
	event := s.recordSwimmerAuditEventLocked(SwimmerAuditEvent{
		OrgID:       orgID,
		SwimmerID:   swimmer.ID,
		ActorUserID: principal.UserID,
		Action:      "swimmer.profile_created",
		After:       swimmerSnapshot(swimmer),
		Metadata: map[string]any{
			"reason": "initial_profile_creation",
		},
	})

	return swimmer, event, nil
}

func (s *Service) UpdateSwimmer(principal *appmiddleware.Principal, orgID, swimmerID string, input UpdateSwimmerInput) (Swimmer, SwimmerAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return Swimmer{}, SwimmerAuditEvent{}, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	swimmer, ok := s.swimmers[strings.TrimSpace(swimmerID)]
	if !ok || swimmer.OrgID != orgID {
		return Swimmer{}, SwimmerAuditEvent{}, ErrSwimmerNotFound
	}
	if err := s.enforceParentHouseholdAccess(principal, swimmer.HouseholdID); err != nil {
		return Swimmer{}, SwimmerAuditEvent{}, err
	}

	before := swimmerSnapshot(swimmer)
	if input.FirstName != nil {
		swimmer.FirstName = strings.TrimSpace(*input.FirstName)
	}
	if input.LastName != nil {
		swimmer.LastName = strings.TrimSpace(*input.LastName)
	}
	if input.PreferredName != nil {
		swimmer.PreferredName = strings.TrimSpace(*input.PreferredName)
	}
	if input.DateOfBirth != nil {
		nextDOB := strings.TrimSpace(*input.DateOfBirth)
		if _, err := time.Parse("2006-01-02", nextDOB); err != nil {
			return Swimmer{}, SwimmerAuditEvent{}, ErrInvalidDateOfBirth
		}
		swimmer.DateOfBirth = nextDOB
	}
	if input.Gender != nil {
		nextGender := strings.TrimSpace(strings.ToLower(*input.Gender))
		if !slices.Contains([]string{"female", "male", "non_binary", "undisclosed"}, nextGender) {
			return Swimmer{}, SwimmerAuditEvent{}, errors.New("gender must be one of: female, male, non_binary, undisclosed")
		}
		swimmer.Gender = nextGender
	}
	if input.SwimmerExternalID != nil {
		nextExternalID := strings.TrimSpace(*input.SwimmerExternalID)
		if nextExternalID != "" && nextExternalID != swimmer.SwimmerExternalID {
			for _, existing := range s.swimmers {
				if existing.ID == swimmer.ID {
					continue
				}
				if existing.OrgID == orgID && strings.EqualFold(existing.SwimmerExternalID, nextExternalID) {
					return Swimmer{}, SwimmerAuditEvent{}, fmt.Errorf("swimmer_external_id %q already exists in org", nextExternalID)
				}
			}
		}
		swimmer.SwimmerExternalID = nextExternalID
	}
	if input.RegistrationStatus != nil {
		nextStatus := strings.TrimSpace(strings.ToLower(*input.RegistrationStatus))
		if hasAnyRole(principal, "parent") {
			return Swimmer{}, SwimmerAuditEvent{}, ErrForbidden
		}
		swimmer.RegistrationStatus = nextStatus
	}
	if input.MedicalFlags != nil {
		if !hasAnyRole(principal, "owner", "admin", "coach") {
			return Swimmer{}, SwimmerAuditEvent{}, ErrMedicalFlagsRestricted
		}
		swimmer.MedicalFlags = normalizeFlags(*input.MedicalFlags)
	}
	if input.Notes != nil {
		swimmer.Notes = strings.TrimSpace(*input.Notes)
	}

	if swimmer.FirstName == "" || swimmer.LastName == "" {
		return Swimmer{}, SwimmerAuditEvent{}, errors.New("first_name and last_name cannot be empty")
	}
	swimmer.UpdatedAt = time.Now().UTC()
	swimmer.UpdatedBy = principal.UserID
	s.swimmers[swimmer.ID] = swimmer

	event := s.recordSwimmerAuditEventLocked(SwimmerAuditEvent{
		OrgID:       orgID,
		SwimmerID:   swimmer.ID,
		ActorUserID: principal.UserID,
		Action:      "swimmer.profile_updated",
		Before:      before,
		After:       swimmerSnapshot(swimmer),
		Metadata: map[string]any{
			"reason": "profile_edit",
		},
	})

	return swimmer, event, nil
}

func (s *Service) AssignSwimmerToGroup(principal *appmiddleware.Principal, orgID, swimmerID string, input AssignSwimmerGroupInput) (SwimmerGroupLink, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return SwimmerGroupLink{}, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	swimmer, ok := s.swimmers[strings.TrimSpace(swimmerID)]
	if !ok || swimmer.OrgID != orgID {
		return SwimmerGroupLink{}, ErrSwimmerNotFound
	}

	groupCode := strings.TrimSpace(strings.ToLower(input.GroupCode))
	if groupCode == "" {
		return SwimmerGroupLink{}, errors.New("group_code is required")
	}

	for _, link := range s.swimmerGroupLinks {
		if link.OrgID == orgID && link.SwimmerID == swimmer.ID && link.GroupCode == groupCode {
			return link, nil
		}
	}

	now := time.Now().UTC()
	if input.IsPrimary {
		for id, link := range s.swimmerGroupLinks {
			if link.OrgID == orgID && link.SwimmerID == swimmer.ID && link.IsPrimary {
				link.IsPrimary = false
				link.UpdatedAt = now
				link.UpdatedBy = principal.UserID
				s.swimmerGroupLinks[id] = link
			}
		}
	}

	link := SwimmerGroupLink{
		ID:         "swg_" + randomToken(12),
		OrgID:      orgID,
		SwimmerID:  swimmer.ID,
		GroupCode:  groupCode,
		IsPrimary:  input.IsPrimary,
		AssignedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
		CreatedBy:  principal.UserID,
		UpdatedBy:  principal.UserID,
	}
	s.swimmerGroupLinks[link.ID] = link
	return link, nil
}

func (s *Service) ListSwimmers(principal *appmiddleware.Principal, orgID string, input ListSwimmersInput) ([]SwimmerView, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent", "billing", "read-only") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	householdFilter := strings.TrimSpace(input.HouseholdID)
	if hasAnyRole(principal, "parent") {
		if householdFilter != "" {
			if _, ok := s.parentHouseholdIndex[principal.UserID][householdFilter]; !ok {
				return nil, ErrParentHouseholdScope
			}
		}
	}

	allowedHouseholds := map[string]struct{}{}
	if hasAnyRole(principal, "parent") {
		for householdID := range s.parentHouseholdIndex[principal.UserID] {
			allowedHouseholds[householdID] = struct{}{}
		}
	}

	groupFilter := strings.TrimSpace(strings.ToLower(input.GroupCode))
	statusFilter := strings.TrimSpace(strings.ToLower(input.RegistrationStatus))
	results := []SwimmerView{}

	for _, swimmer := range s.swimmers {
		if swimmer.OrgID != orgID {
			continue
		}
		if householdFilter != "" && swimmer.HouseholdID != householdFilter {
			continue
		}
		if statusFilter != "" && swimmer.RegistrationStatus != statusFilter {
			continue
		}
		if hasAnyRole(principal, "parent") {
			if _, ok := allowedHouseholds[swimmer.HouseholdID]; !ok {
				continue
			}
		}
		if groupFilter != "" && !s.swimmerInGroupLocked(orgID, swimmer.ID, groupFilter) {
			continue
		}

		view := SwimmerView{Swimmer: swimmer}
		if input.IncludeMedical {
			if s.canViewMedicalLocked(principal, swimmer.HouseholdID) {
				view.MedicalFlagsRedacted = false
			} else {
				view.MedicalFlags = nil
				view.MedicalFlagsRedacted = true
			}
		} else {
			view.MedicalFlags = nil
			view.MedicalFlagsRedacted = true
		}
		results = append(results, view)
	}

	return results, nil
}

func (s *Service) enforceParentHouseholdAccess(principal *appmiddleware.Principal, householdID string) error {
	if principal == nil {
		return ErrForbidden
	}
	if !hasAnyRole(principal, "parent") {
		return nil
	}
	households := s.parentHouseholdIndex[principal.UserID]
	if _, ok := households[householdID]; !ok {
		return ErrParentHouseholdScope
	}
	return nil
}

// AssociateParentToHousehold creates explicit parent-household access links,
// allowing imported/admin-created households to be accessible to a parent.
func (s *Service) AssociateParentToHousehold(principal *appmiddleware.Principal, orgID, parentUserID, householdID string) error {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return ErrForbidden
	}
	if strings.TrimSpace(principal.OrgID) != strings.TrimSpace(orgID) {
		return ErrForbidden
	}

	parentUserID = strings.TrimSpace(parentUserID)
	householdID = strings.TrimSpace(householdID)
	if parentUserID == "" || householdID == "" {
		return errors.New("parent_user_id and household_id are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	household, ok := s.households[householdID]
	if !ok || household.OrgID != strings.TrimSpace(orgID) {
		return ErrHouseholdNotFound
	}

	s.associateParentHouseholdLocked(parentUserID, householdID)
	return nil
}

func (s *Service) canViewMedicalLocked(principal *appmiddleware.Principal, householdID string) bool {
	if principal == nil {
		return false
	}
	if hasAnyRole(principal, "owner", "admin", "coach") {
		return true
	}
	if hasAnyRole(principal, "parent") {
		households := s.parentHouseholdIndex[principal.UserID]
		_, ok := households[householdID]
		return ok
	}
	return false
}

func (s *Service) associateParentHouseholdLocked(parentUserID, householdID string) {
	if _, ok := s.parentHouseholdIndex[parentUserID]; !ok {
		s.parentHouseholdIndex[parentUserID] = make(map[string]struct{})
	}
	s.parentHouseholdIndex[parentUserID][householdID] = struct{}{}
}

func (s *Service) swimmerInGroupLocked(orgID, swimmerID, groupCode string) bool {
	for _, link := range s.swimmerGroupLinks {
		if link.OrgID == orgID && link.SwimmerID == swimmerID && link.GroupCode == groupCode {
			return true
		}
	}
	return false
}

func (s *Service) recordSwimmerAuditEventLocked(event SwimmerAuditEvent) SwimmerAuditEvent {
	event.ID = "aud_" + randomToken(12)
	event.CreatedAt = time.Now().UTC()
	s.auditEvents = append(s.auditEvents, event)
	return event
}

// SnapshotHouseholds returns a copy of all households for the given org.
func (s *Service) SnapshotHouseholds(orgID string) []Household {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Household, 0, len(s.households))
	for _, household := range s.households {
		if household.OrgID != orgID {
			continue
		}
		out = append(out, household)
	}
	return out
}

// SnapshotContacts returns a copy of all contacts for the given org.
func (s *Service) SnapshotContacts(orgID string) []Contact {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Contact, 0, len(s.contacts))
	for _, contact := range s.contacts {
		if contact.OrgID != orgID {
			continue
		}
		out = append(out, contact)
	}
	return out
}

// SnapshotSwimmers returns a copy of all swimmers for the given org.
func (s *Service) SnapshotSwimmers(orgID string) []Swimmer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Swimmer, 0, len(s.swimmers))
	for _, swimmer := range s.swimmers {
		if swimmer.OrgID != orgID {
			continue
		}
		out = append(out, swimmer)
	}
	return out
}

// SnapshotSwimmerGroupLinks returns a copy of all swimmer-group links for the given org.
func (s *Service) SnapshotSwimmerGroupLinks(orgID string) []SwimmerGroupLink {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]SwimmerGroupLink, 0, len(s.swimmerGroupLinks))
	for _, link := range s.swimmerGroupLinks {
		if link.OrgID != orgID {
			continue
		}
		out = append(out, link)
	}
	return out
}

func swimmerSnapshot(swimmer Swimmer) map[string]any {
	return map[string]any{
		"first_name":          swimmer.FirstName,
		"last_name":           swimmer.LastName,
		"preferred_name":      swimmer.PreferredName,
		"date_of_birth":       swimmer.DateOfBirth,
		"gender":              swimmer.Gender,
		"swimmer_external_id": swimmer.SwimmerExternalID,
		"registration_status": swimmer.RegistrationStatus,
		"medical_flags":       append([]string{}, swimmer.MedicalFlags...),
		"notes":               swimmer.Notes,
		"household_id":        swimmer.HouseholdID,
	}
}

func hasAnyRole(principal *appmiddleware.Principal, required ...string) bool {
	if principal == nil {
		return false
	}
	for _, role := range principal.Roles {
		if slices.Contains(required, role) {
			return true
		}
	}
	return false
}

func normalizeFlags(flags []string) []string {
	dedup := make(map[string]struct{}, len(flags))
	out := make([]string, 0, len(flags))
	for _, raw := range flags {
		flag := strings.TrimSpace(strings.ToLower(raw))
		if flag == "" {
			continue
		}
		if _, exists := dedup[flag]; exists {
			continue
		}
		dedup[flag] = struct{}{}
		out = append(out, flag)
	}
	return out
}

func randomToken(byteLen int) string {
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		now := time.Now().UTC().UnixNano()
		return fmt.Sprintf("fallback%x", now)
	}
	return hex.EncodeToString(raw)
}
