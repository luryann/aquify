package members

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
	ErrForbidden              = errors.New("forbidden")
	ErrHouseholdNotFound      = errors.New("household not found")
	ErrHouseholdHasSwimmers   = errors.New("household has swimmers")
	ErrSwimmerNotFound        = errors.New("swimmer not found")
	ErrSwimmerHasDependencies = errors.New("swimmer has related records")
	ErrMedicalFlagsRestricted = errors.New("medical flags can only be changed by admin/coach")
	ErrInvalidDateOfBirth     = errors.New("date_of_birth must use YYYY-MM-DD format")
	ErrParentHouseholdScope   = errors.New("parent can only access swimmers in their own household")
	ErrSwimmerBioTooLong      = errors.New("swimmer bio must be 400 characters or fewer")
	ErrAllergiesTooLong       = errors.New("allergies must be 400 characters or fewer")
)

const maxSwimmerBioLength = 400

type Service struct {
	mu                   sync.RWMutex
	repo                 *Repository // nil = in-memory only
	households           map[string]Household
	contacts             map[string]Contact
	swimmers             map[string]Swimmer
	swimmerGroupLinks    map[string]SwimmerGroupLink
	parentHouseholdIndex map[string]map[string]struct{}
	auditEvents          []SwimmerAuditEvent
	linkRequests         map[string]SwimmerLinkRequest
}

func NewService() *Service {
	return &Service{
		households:           make(map[string]Household),
		contacts:             make(map[string]Contact),
		swimmers:             make(map[string]Swimmer),
		swimmerGroupLinks:    make(map[string]SwimmerGroupLink),
		parentHouseholdIndex: make(map[string]map[string]struct{}),
		auditEvents:          []SwimmerAuditEvent{},
		linkRequests:         make(map[string]SwimmerLinkRequest),
	}
}

// NewServiceWithPool creates a DB-backed members service. On construction it
// loads all existing records from the database into the in-memory maps, then
// every subsequent write is persisted to the database before updating memory.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("members: failed to load initial state from DB", "error", err)
	}
	return s
}

// loadFromDB populates the in-memory maps from the database.
func (s *Service) loadFromDB(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	households, contacts, swimmers, links, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetDBBackedStateLocked()
	s.populateDBBackedStateLocked(households, contacts, swimmers, links)
	return nil
}

func (s *Service) syncFromDB(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	households, contacts, swimmers, links, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetDBBackedStateLocked()
	s.populateDBBackedStateLocked(households, contacts, swimmers, links)
	return nil
}

func (s *Service) resetDBBackedStateLocked() {
	s.households = make(map[string]Household)
	s.contacts = make(map[string]Contact)
	s.swimmers = make(map[string]Swimmer)
	s.swimmerGroupLinks = make(map[string]SwimmerGroupLink)
}

func (s *Service) populateDBBackedStateLocked(households []Household, contacts []Contact, swimmers []Swimmer, links []SwimmerGroupLink) {
	for _, h := range households {
		s.households[h.ID] = h
	}
	for _, c := range contacts {
		s.contacts[c.ID] = c
	}
	for _, sw := range swimmers {
		s.swimmers[sw.ID] = sw
	}
	for _, link := range links {
		s.swimmerGroupLinks[link.ID] = link
	}
}

// newID generates an ID for a new record. When a DB repository is configured
// the ID is a proper UUID (required by the DB schema). In in-memory mode the
// existing prefix-token format is preserved so legacy tests are unaffected.
func (s *Service) newID(prefix string) string {
	if s.repo != nil {
		return uuid.New().String()
	}
	return prefix + randomToken(12)
}

type Household struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	HouseholdName    string    `json:"household_name"`
	PrimaryEmail     string    `json:"primary_email,omitempty"`
	PrimaryPhoneE164 string    `json:"primary_phone_e164,omitempty"`
	Status           string    `json:"status"`
	Notes            string    `json:"notes,omitempty"`
	KeycloakUserID   string    `json:"keycloak_user_id,omitempty"`
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
	KeycloakUserID   string
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
	ID                           string    `json:"id"`
	OrgID                        string    `json:"org_id"`
	HouseholdID                  string    `json:"household_id"`
	FirstName                    string    `json:"first_name"`
	LastName                     string    `json:"last_name"`
	PreferredName                string    `json:"preferred_name,omitempty"`
	DateOfBirth                  string    `json:"date_of_birth"`
	Gender                       string    `json:"gender"`
	SwimmerExternalID            string    `json:"swimmer_external_id,omitempty"`
	USASwimmingID                string    `json:"usa_swimming_id,omitempty"`
	RegistrationStatus           string    `json:"registration_status"`
	MedicalFlags                 []string  `json:"medical_flags,omitempty"`
	Notes                        string    `json:"notes,omitempty"`
	EmergencyContactName         string    `json:"emergency_contact_name,omitempty"`
	EmergencyContactPhone        string    `json:"emergency_contact_phone,omitempty"`
	EmergencyContactRelationship string    `json:"emergency_contact_relationship,omitempty"`
	Allergies                    string    `json:"allergies,omitempty"`
	KeycloakUserID               string    `json:"keycloak_user_id,omitempty"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
	CreatedBy                    string    `json:"created_by"`
	UpdatedBy                    string    `json:"updated_by"`
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
	USASwimmingID      string
	RegistrationStatus string
	MedicalFlags       []string
	Notes              string
}

type UpdateSwimmerInput struct {
	FirstName                    *string
	LastName                     *string
	PreferredName                *string
	DateOfBirth                  *string
	Gender                       *string
	SwimmerExternalID            *string
	USASwimmingID                *string
	RegistrationStatus           *string
	MedicalFlags                 *[]string
	Notes                        *string
	EmergencyContactName         *string
	EmergencyContactPhone        *string
	EmergencyContactRelationship *string
	Allergies                    *string
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

type SwimmerDeleteDependency struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Count int    `json:"count"`
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
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		return Household{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Household{}, err
	}

	household := Household{
		ID:               s.newID("hsh_"),
		OrgID:            strings.TrimSpace(input.OrgID),
		HouseholdName:    strings.TrimSpace(input.HouseholdName),
		PrimaryEmail:     strings.TrimSpace(strings.ToLower(input.PrimaryEmail)),
		PrimaryPhoneE164: strings.TrimSpace(input.PrimaryPhoneE164),
		Status:           strings.TrimSpace(strings.ToLower(input.Status)),
		Notes:            strings.TrimSpace(input.Notes),
		KeycloakUserID:   strings.TrimSpace(input.KeycloakUserID),
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
	defer s.mu.Unlock()

	if s.repo != nil {
		if err := s.repo.UpsertHousehold(context.Background(), household); err != nil {
			return Household{}, fmt.Errorf("persist household: %w", err)
		}
	}

	s.households[household.ID] = household
	if hasAnyRole(principal, "parent") {
		s.associateParentHouseholdLocked(principal.UserID, household.ID)
	}

	return household, nil
}

func (s *Service) UpdateHousehold(principal *appmiddleware.Principal, orgID, householdID string, input UpdateHouseholdInput) (Household, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		return Household{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Household{}, err
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

	if s.repo != nil {
		if err := s.repo.UpsertHousehold(context.Background(), household); err != nil {
			return Household{}, fmt.Errorf("persist household: %w", err)
		}
	}
	s.households[household.ID] = household
	return household, nil
}

func (s *Service) DeleteHousehold(principal *appmiddleware.Principal, orgID, householdID string) (Household, error) {
	if !hasAnyRole(principal, "admin") {
		return Household{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Household{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	household, ok := s.households[strings.TrimSpace(householdID)]
	if !ok || household.OrgID != orgID {
		return Household{}, ErrHouseholdNotFound
	}
	for _, swimmer := range s.swimmers {
		if swimmer.OrgID == orgID && swimmer.HouseholdID == household.ID {
			return Household{}, ErrHouseholdHasSwimmers
		}
	}

	if s.repo != nil {
		if err := s.repo.DeleteHousehold(context.Background(), householdID, principal.UserID); err != nil {
			return Household{}, fmt.Errorf("delete household: %w", err)
		}
	}
	delete(s.households, householdID)
	return household, nil
}

// AnonymizeHousehold implements the actual erasure step of a compliance
// right-to-erasure request: it scrubs directly identifying PII from the
// household, its contacts, and its swimmers in place (rather than hard
// deleting rows, which would break referential integrity with billing/audit/
// meet-entry history that must be retained). It is invoked by the compliance
// deletion worker and does not take a principal — the caller (an approved,
// audited deletion request) has already been authorized.
func (s *Service) AnonymizeHousehold(orgID, householdID string) error {
	if err := s.syncFromDB(context.Background()); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	household, ok := s.households[strings.TrimSpace(householdID)]
	if !ok || household.OrgID != orgID {
		return ErrHouseholdNotFound
	}

	now := time.Now().UTC()

	household.HouseholdName = "Deleted Household"
	household.PrimaryEmail = ""
	household.PrimaryPhoneE164 = ""
	household.Notes = ""
	household.KeycloakUserID = ""
	household.UpdatedAt = now
	household.UpdatedBy = "compliance_erasure"
	if s.repo != nil {
		if err := s.repo.UpsertHousehold(context.Background(), household); err != nil {
			return fmt.Errorf("anonymize household: %w", err)
		}
	}
	s.households[household.ID] = household

	for id, contact := range s.contacts {
		if contact.OrgID != orgID || contact.HouseholdID != household.ID {
			continue
		}
		contact.FirstName = "Deleted"
		contact.LastName = "Contact"
		contact.Email = ""
		contact.PhoneE164 = ""
		contact.Notes = ""
		contact.UpdatedAt = now
		contact.UpdatedBy = "compliance_erasure"
		if s.repo != nil {
			if err := s.repo.UpsertContact(context.Background(), contact); err != nil {
				return fmt.Errorf("anonymize contact: %w", err)
			}
		}
		s.contacts[id] = contact
	}

	for id, swimmer := range s.swimmers {
		if swimmer.OrgID != orgID || swimmer.HouseholdID != household.ID {
			continue
		}
		swimmer.FirstName = "Deleted"
		swimmer.LastName = "Swimmer"
		swimmer.PreferredName = ""
		swimmer.SwimmerExternalID = ""
		swimmer.USASwimmingID = ""
		swimmer.MedicalFlags = nil
		swimmer.Notes = ""
		swimmer.UpdatedAt = now
		swimmer.UpdatedBy = "compliance_erasure"
		if s.repo != nil {
			if err := s.repo.UpsertSwimmer(context.Background(), swimmer); err != nil {
				return fmt.Errorf("anonymize swimmer: %w", err)
			}
		}
		s.swimmers[id] = swimmer
	}

	return nil
}

func (s *Service) GetHousehold(principal *appmiddleware.Principal, orgID, householdID string) (Household, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		return Household{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Household{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	household, ok := s.households[strings.TrimSpace(householdID)]
	if !ok || household.OrgID != orgID {
		return Household{}, ErrHouseholdNotFound
	}
	if err := s.enforceParentHouseholdAccess(principal, household.ID); err != nil {
		return Household{}, err
	}

	return household, nil
}

// ListHouseholds returns the households visible to the principal.
// Parent and swimmer roles are strictly limited to their own household scope.
func (s *Service) ListHouseholds(principal *appmiddleware.Principal, orgID string) ([]Household, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	swimmerScopedHouseholdID := ""
	if hasAnyRole(principal, "swimmer") && !hasAnyRole(principal, "admin", "coach", "parent") {
		swimmerScopedHouseholdID = strings.TrimSpace(principal.HouseholdID)
		if swimmerScopedHouseholdID == "" {
			return nil, ErrForbidden
		}
	}

	allowedHouseholds := map[string]struct{}{}
	if hasAnyRole(principal, "parent") {
		allowedHouseholds = s.parentAllowedHouseholdsLocked(principal)
	}

	out := make([]Household, 0, len(s.households))
	for _, household := range s.households {
		if household.OrgID != orgID {
			continue
		}
		if swimmerScopedHouseholdID != "" && household.ID != swimmerScopedHouseholdID {
			continue
		}
		if hasAnyRole(principal, "parent") {
			if _, ok := allowedHouseholds[household.ID]; !ok {
				continue
			}
		}
		out = append(out, household)
	}

	slices.SortFunc(out, func(a, b Household) int {
		if cmp := strings.Compare(a.HouseholdName, b.HouseholdName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (s *Service) AddContact(principal *appmiddleware.Principal, orgID, householdID string, input AddContactInput) (Contact, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		return Contact{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Contact{}, err
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
		ID:                 s.newID("ctc_"),
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

	if s.repo != nil {
		if err := s.repo.UpsertContact(context.Background(), contact); err != nil {
			return Contact{}, fmt.Errorf("persist contact: %w", err)
		}
	}
	s.contacts[contact.ID] = contact
	return contact, nil
}

func (s *Service) CreateSwimmer(principal *appmiddleware.Principal, orgID string, input CreateSwimmerInput) (Swimmer, SwimmerAuditEvent, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		return Swimmer{}, SwimmerAuditEvent{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Swimmer{}, SwimmerAuditEvent{}, err
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
		ID:                 s.newID("swm_"),
		OrgID:              orgID,
		HouseholdID:        household.ID,
		FirstName:          strings.TrimSpace(input.FirstName),
		LastName:           strings.TrimSpace(input.LastName),
		PreferredName:      strings.TrimSpace(input.PreferredName),
		DateOfBirth:        strings.TrimSpace(input.DateOfBirth),
		Gender:             strings.TrimSpace(strings.ToLower(input.Gender)),
		SwimmerExternalID:  strings.TrimSpace(input.SwimmerExternalID),
		USASwimmingID:      strings.TrimSpace(input.USASwimmingID),
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
	if swimmer.USASwimmingID != "" {
		for _, existing := range s.swimmers {
			if existing.OrgID == orgID && existing.USASwimmingID != "" && strings.EqualFold(existing.USASwimmingID, swimmer.USASwimmingID) {
				return Swimmer{}, SwimmerAuditEvent{}, fmt.Errorf("usa_swimming_id %q already exists in org", swimmer.USASwimmingID)
			}
		}
	}

	if s.repo != nil {
		if err := s.repo.UpsertSwimmer(context.Background(), swimmer); err != nil {
			return Swimmer{}, SwimmerAuditEvent{}, fmt.Errorf("persist swimmer: %w", err)
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
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		return Swimmer{}, SwimmerAuditEvent{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Swimmer{}, SwimmerAuditEvent{}, err
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
	swimmerOnly := hasAnyRole(principal, "swimmer") && !hasAnyRole(principal, "admin", "coach", "parent")
	if swimmerOnly {
		if strings.TrimSpace(principal.HouseholdID) == "" || strings.TrimSpace(principal.HouseholdID) != strings.TrimSpace(swimmer.HouseholdID) {
			return Swimmer{}, SwimmerAuditEvent{}, ErrForbidden
		}
		if input.FirstName != nil || input.LastName != nil || input.PreferredName != nil || input.DateOfBirth != nil || input.Gender != nil || input.SwimmerExternalID != nil || input.USASwimmingID != nil || input.RegistrationStatus != nil || input.MedicalFlags != nil ||
			input.EmergencyContactName != nil || input.EmergencyContactPhone != nil || input.EmergencyContactRelationship != nil || input.Allergies != nil {
			return Swimmer{}, SwimmerAuditEvent{}, ErrForbidden
		}
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
	if input.USASwimmingID != nil {
		nextUSAID := strings.TrimSpace(*input.USASwimmingID)
		if nextUSAID != "" && !strings.EqualFold(nextUSAID, swimmer.USASwimmingID) {
			for _, existing := range s.swimmers {
				if existing.ID == swimmer.ID {
					continue
				}
				if existing.OrgID == orgID && existing.USASwimmingID != "" && strings.EqualFold(existing.USASwimmingID, nextUSAID) {
					return Swimmer{}, SwimmerAuditEvent{}, fmt.Errorf("usa_swimming_id %q already exists in org", nextUSAID)
				}
			}
		}
		swimmer.USASwimmingID = nextUSAID
	}
	if input.RegistrationStatus != nil {
		nextStatus := strings.TrimSpace(strings.ToLower(*input.RegistrationStatus))
		if hasAnyRole(principal, "parent") {
			return Swimmer{}, SwimmerAuditEvent{}, ErrForbidden
		}
		swimmer.RegistrationStatus = nextStatus
	}
	if input.MedicalFlags != nil {
		if !hasAnyRole(principal, "admin", "coach") {
			return Swimmer{}, SwimmerAuditEvent{}, ErrMedicalFlagsRestricted
		}
		swimmer.MedicalFlags = normalizeFlags(*input.MedicalFlags)
	}
	if input.Notes != nil {
		nextNotes := strings.TrimSpace(*input.Notes)
		if len([]rune(nextNotes)) > maxSwimmerBioLength {
			return Swimmer{}, SwimmerAuditEvent{}, ErrSwimmerBioTooLong
		}
		swimmer.Notes = nextNotes
	}
	if input.EmergencyContactName != nil {
		swimmer.EmergencyContactName = strings.TrimSpace(*input.EmergencyContactName)
	}
	if input.EmergencyContactPhone != nil {
		swimmer.EmergencyContactPhone = strings.TrimSpace(*input.EmergencyContactPhone)
	}
	if input.EmergencyContactRelationship != nil {
		swimmer.EmergencyContactRelationship = strings.TrimSpace(*input.EmergencyContactRelationship)
	}
	if input.Allergies != nil {
		nextAllergies := strings.TrimSpace(*input.Allergies)
		if len([]rune(nextAllergies)) > maxSwimmerBioLength {
			return Swimmer{}, SwimmerAuditEvent{}, ErrAllergiesTooLong
		}
		swimmer.Allergies = nextAllergies
	}

	if swimmer.FirstName == "" || swimmer.LastName == "" {
		return Swimmer{}, SwimmerAuditEvent{}, errors.New("first_name and last_name cannot be empty")
	}
	swimmer.UpdatedAt = time.Now().UTC()
	swimmer.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertSwimmer(context.Background(), swimmer); err != nil {
			return Swimmer{}, SwimmerAuditEvent{}, fmt.Errorf("persist swimmer: %w", err)
		}
	}
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

func (s *Service) DeleteSwimmer(principal *appmiddleware.Principal, orgID, swimmerID string) (Swimmer, []SwimmerDeleteDependency, error) {
	if !hasAnyRole(principal, "admin") {
		return Swimmer{}, nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Swimmer{}, nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	swimmer, ok := s.swimmers[strings.TrimSpace(swimmerID)]
	if !ok || swimmer.OrgID != orgID {
		return Swimmer{}, nil, ErrSwimmerNotFound
	}

	var dependencies []SwimmerDeleteDependency
	if s.repo != nil {
		blockers, err := s.repo.ListSwimmerDeleteDependencies(context.Background(), orgID, swimmer.ID)
		if err != nil {
			return Swimmer{}, nil, err
		}
		dependencies = make([]SwimmerDeleteDependency, 0, len(blockers))
		for _, blocker := range blockers {
			dependencies = append(dependencies, SwimmerDeleteDependency{
				Code:  blocker.Code,
				Label: blocker.Label,
				Count: blocker.Count,
			})
		}
	}
	if len(dependencies) > 0 {
		return Swimmer{}, dependencies, ErrSwimmerHasDependencies
	}

	if s.repo != nil {
		if err := s.repo.DeleteSwimmer(context.Background(), swimmer.ID, principal.UserID); err != nil {
			return Swimmer{}, nil, fmt.Errorf("delete swimmer: %w", err)
		}
	}

	delete(s.swimmers, swimmer.ID)
	for id, link := range s.swimmerGroupLinks {
		if link.OrgID != orgID || link.SwimmerID != swimmer.ID {
			continue
		}
		delete(s.swimmerGroupLinks, id)
	}

	s.recordSwimmerAuditEventLocked(SwimmerAuditEvent{
		OrgID:       orgID,
		SwimmerID:   swimmer.ID,
		ActorUserID: principal.UserID,
		Action:      "swimmer.deleted",
		Before:      swimmerSnapshot(swimmer),
		Metadata: map[string]any{
			"reason": "manual_delete",
		},
	})

	return swimmer, nil, nil
}

func (s *Service) GetSwimmer(principal *appmiddleware.Principal, orgID, swimmerID string, includeMedical bool) (SwimmerView, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		return SwimmerView{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return SwimmerView{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	swimmer, ok := s.swimmers[strings.TrimSpace(swimmerID)]
	if !ok || swimmer.OrgID != orgID {
		return SwimmerView{}, ErrSwimmerNotFound
	}
	if err := s.enforceParentHouseholdAccess(principal, swimmer.HouseholdID); err != nil {
		return SwimmerView{}, err
	}

	view := SwimmerView{Swimmer: swimmer}
	if includeMedical && s.canViewMedicalLocked(principal, swimmer.HouseholdID) {
		view.MedicalFlagsRedacted = false
		return view, nil
	}

	view.MedicalFlags = nil
	view.MedicalFlagsRedacted = true
	return view, nil
}

func (s *Service) AssignSwimmerToGroup(principal *appmiddleware.Principal, orgID, swimmerID string, input AssignSwimmerGroupInput) (SwimmerGroupLink, error) {
	if !hasAnyRole(principal, "admin", "coach") {
		return SwimmerGroupLink{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return SwimmerGroupLink{}, err
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
		ID:         s.newID("swg_"),
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
	if s.repo != nil {
		if err := s.repo.UpsertSwimmerGroupLink(context.Background(), link); err != nil {
			return SwimmerGroupLink{}, fmt.Errorf("persist swimmer group link: %w", err)
		}
	}
	s.swimmerGroupLinks[link.ID] = link
	return link, nil
}

func (s *Service) ListSwimmers(principal *appmiddleware.Principal, orgID string, input ListSwimmersInput) ([]SwimmerView, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	householdFilter := strings.TrimSpace(input.HouseholdID)
	// Swimmer-only accounts can only list swimmers in their own household.
	if hasAnyRole(principal, "swimmer") && !hasAnyRole(principal, "admin", "coach", "parent") {
		swimmerHouseholdID := strings.TrimSpace(principal.HouseholdID)
		if swimmerHouseholdID == "" {
			return nil, ErrForbidden
		}
		if householdFilter != "" && householdFilter != swimmerHouseholdID {
			return nil, ErrParentHouseholdScope
		}
		householdFilter = swimmerHouseholdID
	}
	allowedHouseholds := map[string]struct{}{}
	if hasAnyRole(principal, "parent") {
		allowedHouseholds = s.parentAllowedHouseholdsLocked(principal)
		if householdFilter != "" {
			if _, ok := allowedHouseholds[householdFilter]; !ok {
				return nil, ErrParentHouseholdScope
			}
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

	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	return results, nil
}

// ListSwimmerGroupLinks returns swimmer-group links visible to the principal.
// Parent and swimmer accounts are strictly limited to their own household.
func (s *Service) ListSwimmerGroupLinks(principal *appmiddleware.Principal, orgID string) ([]SwimmerGroupLink, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent", "swimmer") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	swimmerScopedHouseholdID := ""
	if hasAnyRole(principal, "swimmer") && !hasAnyRole(principal, "admin", "coach", "parent") {
		swimmerScopedHouseholdID = strings.TrimSpace(principal.HouseholdID)
		if swimmerScopedHouseholdID == "" {
			return nil, ErrForbidden
		}
	}

	allowedHouseholds := map[string]struct{}{}
	if hasAnyRole(principal, "parent") {
		allowedHouseholds = s.parentAllowedHouseholdsLocked(principal)
	}

	out := make([]SwimmerGroupLink, 0, len(s.swimmerGroupLinks))
	for _, link := range s.swimmerGroupLinks {
		if link.OrgID != orgID {
			continue
		}
		swimmer, ok := s.swimmers[link.SwimmerID]
		if !ok || swimmer.OrgID != orgID {
			continue
		}
		if swimmerScopedHouseholdID != "" && swimmer.HouseholdID != swimmerScopedHouseholdID {
			continue
		}
		if hasAnyRole(principal, "parent") {
			if _, ok := allowedHouseholds[swimmer.HouseholdID]; !ok {
				continue
			}
		}
		out = append(out, link)
	}

	slices.SortFunc(out, func(a, b SwimmerGroupLink) int {
		if a.SwimmerID != b.SwimmerID {
			return strings.Compare(a.SwimmerID, b.SwimmerID)
		}
		if a.IsPrimary != b.IsPrimary {
			if a.IsPrimary {
				return -1
			}
			return 1
		}
		return strings.Compare(a.GroupCode, b.GroupCode)
	})
	return out, nil
}

// parentAllowedHouseholdsLocked returns the set of household IDs a parent
// principal may access: the explicit in-memory association index plus the
// household_id attached to the principal at OIDC login (resolved from the
// household's keycloak_user_id link). Both must be honored — login populates
// only principal.HouseholdID, while in-app "parent creates household" and admin
// AssociateParentToHousehold populate only the index. Callers must hold s.mu.
func (s *Service) parentAllowedHouseholdsLocked(principal *appmiddleware.Principal) map[string]struct{} {
	out := make(map[string]struct{}, len(s.parentHouseholdIndex[principal.UserID])+1)
	for householdID := range s.parentHouseholdIndex[principal.UserID] {
		out[householdID] = struct{}{}
	}
	if hid := strings.TrimSpace(principal.HouseholdID); hid != "" {
		out[hid] = struct{}{}
	}
	return out
}

func (s *Service) enforceParentHouseholdAccess(principal *appmiddleware.Principal, householdID string) error {
	if principal == nil {
		return ErrForbidden
	}
	if !hasAnyRole(principal, "parent") {
		return nil
	}
	if _, ok := s.parentAllowedHouseholdsLocked(principal)[householdID]; !ok {
		return ErrParentHouseholdScope
	}
	return nil
}

// AssociateParentToHousehold creates explicit parent-household access links,
// allowing imported/admin-created households to be accessible to a parent.
func (s *Service) AssociateParentToHousehold(principal *appmiddleware.Principal, orgID, parentUserID, householdID string) error {
	if !hasAnyRole(principal, "admin", "coach") {
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
	if hasAnyRole(principal, "admin", "coach") {
		return true
	}
	if hasAnyRole(principal, "parent") {
		_, ok := s.parentAllowedHouseholdsLocked(principal)[householdID]
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
	event.ID = s.newID("aud_")
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

// GetHouseholdByKeycloakUserID returns the household linked to the given Keycloak
// user ID across all orgs. Used during OIDC login to attach household_id to the
// session principal.
func (s *Service) GetHouseholdByKeycloakUserID(keycloakUserID string) (Household, bool) {
	if keycloakUserID == "" {
		return Household{}, false
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Household{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, h := range s.households {
		if h.KeycloakUserID == keycloakUserID {
			return h, true
		}
	}
	return Household{}, false
}

// LinkHouseholdToUser sets the keycloak_user_id on a household record.
// The household must belong to the specified org.
func (s *Service) LinkHouseholdToUser(orgID, householdID, keycloakUserID string) error {
	if err := s.syncFromDB(context.Background()); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	h, ok := s.households[householdID]
	if !ok || h.OrgID != orgID {
		return ErrHouseholdNotFound
	}

	// Verify keycloakUserID is not already taken by another household.
	for _, existing := range s.households {
		if existing.KeycloakUserID == keycloakUserID && existing.ID != householdID {
			return errors.New("keycloak_user_id is already linked to another household")
		}
	}

	h.KeycloakUserID = keycloakUserID
	h.UpdatedAt = time.Now().UTC()

	if s.repo != nil {
		if err := s.repo.SetHouseholdKeycloakUserID(context.Background(), householdID, keycloakUserID); err != nil {
			return fmt.Errorf("persist household link: %w", err)
		}
	}
	s.households[householdID] = h
	return nil
}

// UnlinkHouseholdFromUser clears the keycloak_user_id on a household record.
func (s *Service) UnlinkHouseholdFromUser(orgID, householdID string) error {
	if err := s.syncFromDB(context.Background()); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	h, ok := s.households[householdID]
	if !ok || h.OrgID != orgID {
		return ErrHouseholdNotFound
	}

	h.KeycloakUserID = ""
	h.UpdatedAt = time.Now().UTC()

	if s.repo != nil {
		if err := s.repo.SetHouseholdKeycloakUserID(context.Background(), householdID, ""); err != nil {
			return fmt.Errorf("persist household unlink: %w", err)
		}
	}
	s.households[householdID] = h
	return nil
}

// GetSwimmerByKeycloakUserID returns the swimmer linked to the given Keycloak
// user ID across all orgs. Used during OIDC login (and dev-stub login) to
// attach a stable swimmer_id to a swimmer-role session principal, so the
// portal never has to guess which household roster entry is "self."
func (s *Service) GetSwimmerByKeycloakUserID(keycloakUserID string) (Swimmer, bool) {
	if keycloakUserID == "" {
		return Swimmer{}, false
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Swimmer{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sw := range s.swimmers {
		if sw.KeycloakUserID == keycloakUserID {
			return sw, true
		}
	}
	return Swimmer{}, false
}

// LinkSwimmerToUser sets the keycloak_user_id on a swimmer record, so that
// user's future logins resolve to this specific swimmer instead of any
// roster entry that merely shares a household. The swimmer must belong to
// the specified org.
func (s *Service) LinkSwimmerToUser(orgID, swimmerID, keycloakUserID string) error {
	if err := s.syncFromDB(context.Background()); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sw, ok := s.swimmers[swimmerID]
	if !ok || sw.OrgID != orgID {
		return ErrSwimmerNotFound
	}

	for _, existing := range s.swimmers {
		if existing.KeycloakUserID == keycloakUserID && existing.ID != swimmerID {
			return errors.New("keycloak_user_id is already linked to another swimmer")
		}
	}

	sw.KeycloakUserID = keycloakUserID
	sw.UpdatedAt = time.Now().UTC()

	if s.repo != nil {
		if err := s.repo.SetSwimmerKeycloakUserID(context.Background(), swimmerID, keycloakUserID); err != nil {
			return fmt.Errorf("persist swimmer link: %w", err)
		}
	}
	s.swimmers[swimmerID] = sw
	return nil
}

// UnlinkSwimmerFromUser clears the keycloak_user_id on a swimmer record.
func (s *Service) UnlinkSwimmerFromUser(orgID, swimmerID string) error {
	if err := s.syncFromDB(context.Background()); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sw, ok := s.swimmers[swimmerID]
	if !ok || sw.OrgID != orgID {
		return ErrSwimmerNotFound
	}

	sw.KeycloakUserID = ""
	sw.UpdatedAt = time.Now().UTC()

	if s.repo != nil {
		if err := s.repo.SetSwimmerKeycloakUserID(context.Background(), swimmerID, ""); err != nil {
			return fmt.Errorf("persist swimmer unlink: %w", err)
		}
	}
	s.swimmers[swimmerID] = sw
	return nil
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

func (s *Service) ListContacts(principal *appmiddleware.Principal, orgID, householdID string) ([]Contact, error) {
	if !hasAnyRole(principal, "admin", "coach", "parent") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	household, ok := s.households[strings.TrimSpace(householdID)]
	if !ok || household.OrgID != orgID {
		return nil, ErrHouseholdNotFound
	}
	if err := s.enforceParentHouseholdAccess(principal, household.ID); err != nil {
		return nil, err
	}

	out := make([]Contact, 0)
	for _, contact := range s.contacts {
		if contact.OrgID != orgID || contact.HouseholdID != household.ID {
			continue
		}
		out = append(out, contact)
	}
	slices.SortFunc(out, func(a, b Contact) int {
		if a.IsPrimary != b.IsPrimary {
			if a.IsPrimary {
				return -1
			}
			return 1
		}
		if cmp := strings.Compare(a.LastName, b.LastName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.FirstName, b.FirstName)
	})
	return out, nil
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

// SwimmerHouseholdID returns the household ID for the given swimmer within the
// given org. Returns ("", false) when the swimmer does not exist or does not
// belong to the specified org.
// Implements performance.SwimmerHouseholdChecker.
func (s *Service) SwimmerHouseholdID(orgID, swimmerID string) (string, bool) {
	if err := s.syncFromDB(context.Background()); err != nil {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sw, ok := s.swimmers[strings.TrimSpace(swimmerID)]
	if !ok || sw.OrgID != orgID {
		return "", false
	}
	return sw.HouseholdID, true
}

// SwimmerEligibilityInfo carries the minimal swimmer facts other domains need
// to validate meet-event eligibility (gender + age).
type SwimmerEligibilityInfo struct {
	Gender      string // swimmer vocabulary: female|male|non_binary|undisclosed
	DateOfBirth string // YYYY-MM-DD
	FirstName   string
	LastName    string
}

// SwimmerEligibility returns the gender/DOB for a swimmer, scoped to org. Used
// by the meets domain to gate entry creation. Unscoped by role on purpose —
// callers are trusted server-side services, not end-user requests.
func (s *Service) SwimmerEligibility(orgID, swimmerID string) (SwimmerEligibilityInfo, bool) {
	if err := s.syncFromDB(context.Background()); err != nil {
		return SwimmerEligibilityInfo{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sw, ok := s.swimmers[strings.TrimSpace(swimmerID)]
	if !ok || sw.OrgID != orgID {
		return SwimmerEligibilityInfo{}, false
	}
	return SwimmerEligibilityInfo{
		Gender:      sw.Gender,
		DateOfBirth: sw.DateOfBirth,
		FirstName:   sw.FirstName,
		LastName:    sw.LastName,
	}, true
}

func swimmerSnapshot(swimmer Swimmer) map[string]any {
	return map[string]any{
		"first_name":          swimmer.FirstName,
		"last_name":           swimmer.LastName,
		"preferred_name":      swimmer.PreferredName,
		"date_of_birth":       swimmer.DateOfBirth,
		"gender":              swimmer.Gender,
		"swimmer_external_id": swimmer.SwimmerExternalID,
		"usa_swimming_id":     swimmer.USASwimmingID,
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
		panic(fmt.Errorf("secure random unavailable for member token generation: %w", err))
	}
	return hex.EncodeToString(raw)
}
