package meets

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

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/meetfiles/cl2"
	"github.com/dareaquatics/aquify/meetfiles/sdif"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Sentinel errors ─────────────────────────────────────────────────────────

var (
	ErrForbidden             = errors.New("forbidden")
	ErrMeetNotFound          = errors.New("meet not found")
	ErrSessionNotFound       = errors.New("meet session not found")
	ErrEventNotFound         = errors.New("meet event not found")
	ErrCommitmentNotFound    = errors.New("commitment not found")
	ErrSelectionNotFound     = errors.New("event selection not found")
	ErrEntryNotFound         = errors.New("entry not found")
	ErrInvalidTransition     = errors.New("invalid state transition")
	ErrCommitmentLocked      = errors.New("commitment is locked; selections cannot be modified")
	ErrNotCommittedYes       = errors.New("parent must respond committed_yes before selecting events")
	ErrDuplicateEntry        = errors.New("entry already exists for this swimmer and event")
	ErrIneligibleEntry       = errors.New("swimmer is not eligible for this event")
	ErrEntryNotEditable      = errors.New("entry is not in an editable state")
	ErrEntryAlreadyScratched = errors.New("entry is already scratched")
	ErrDuplicateSelection    = errors.New("event already selected for this commitment")
	ErrEventNumberConflict   = errors.New("event number already exists in this meet")
	ErrInvalidMeetFile       = errors.New("invalid meet file")
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// Meet represents a swim meet within an org.
type Meet struct {
	ID                   string     `json:"id"`
	OrgID                string     `json:"org_id"`
	Name                 string     `json:"name"`
	MeetType             string     `json:"meet_type"`
	Course               string     `json:"course"`
	StartDate            string     `json:"start_date,omitempty"`
	EndDate              string     `json:"end_date,omitempty"`
	EntryDeadline        string     `json:"entry_deadline,omitempty"`
	CommitmentDeadline   string     `json:"commitment_deadline,omitempty"`
	HostOrg              string     `json:"host_org,omitempty"`
	Location             string     `json:"location,omitempty"`
	SelfSignupEnabled    bool       `json:"self_signup_enabled"`
	MaxEntriesPerSwimmer int        `json:"max_entries_per_swimmer"`
	Status               string     `json:"status"`
	CommitmentOpenAt     *time.Time `json:"commitment_open_at,omitempty"`
	CommitmentClosedAt   *time.Time `json:"commitment_closed_at,omitempty"`
	EntryFinalizedAt     *time.Time `json:"entry_finalized_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CreatedBy            string     `json:"created_by"`
	UpdatedBy            string     `json:"updated_by"`
}

// MeetSession represents a single session within a meet.
type MeetSession struct {
	ID            string    `json:"id"`
	OrgID         string    `json:"org_id"`
	MeetID        string    `json:"meet_id"`
	SessionNumber int       `json:"session_number"`
	Name          string    `json:"name"`
	SessionDate   string    `json:"session_date,omitempty"`
	WarmupTime    string    `json:"warmup_time,omitempty"`
	StartTime     string    `json:"start_time,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedBy     string    `json:"created_by"`
	UpdatedBy     string    `json:"updated_by"`
}

// MeetEvent represents a single event offered at a meet.
type MeetEvent struct {
	ID            string    `json:"id"`
	OrgID         string    `json:"org_id"`
	MeetID        string    `json:"meet_id"`
	SessionID     string    `json:"session_id,omitempty"`
	EventNumber   string    `json:"event_number"`
	Stroke        string    `json:"stroke"`
	Distance      int       `json:"distance"`
	Course        string    `json:"course"`
	Gender        string    `json:"gender"`
	MinAge        *int      `json:"min_age,omitempty"`
	MaxAge        *int      `json:"max_age,omitempty"`
	EntryFeeCents int       `json:"entry_fee_cents"`
	IsRelay       bool      `json:"is_relay"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedBy     string    `json:"created_by"`
	UpdatedBy     string    `json:"updated_by"`
}

// Commitment tracks a swimmer's response to a meet commitment request.
type Commitment struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"org_id"`
	MeetID          string     `json:"meet_id"`
	HouseholdID     string     `json:"household_id"`
	SwimmerID       string     `json:"swimmer_id"`
	Status          string     `json:"status"`
	Notes           string     `json:"notes,omitempty"`
	CommittedAt     *time.Time `json:"committed_at,omitempty"`
	CoachReviewedAt *time.Time `json:"coach_reviewed_at,omitempty"`
	LockedAt        *time.Time `json:"locked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CreatedBy       string     `json:"created_by"`
	UpdatedBy       string     `json:"updated_by"`
}

// EventSelection records a parent-selected event for a swimmer's commitment.
type EventSelection struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	CommitmentID string    `json:"commitment_id"`
	MeetEventID  string    `json:"meet_event_id"`
	SeedTimeMs   *int      `json:"seed_time_ms,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
}

// Entry represents a swimmer's entry into a specific meet event.
type Entry struct {
	ID             string    `json:"id"`
	OrgID          string    `json:"org_id"`
	MeetID         string    `json:"meet_id"`
	SwimmerID      string    `json:"swimmer_id"`
	MeetEventID    string    `json:"meet_event_id"`
	CommitmentID   string    `json:"commitment_id,omitempty"`
	SeedTimeMs     *int      `json:"seed_time_ms,omitempty"`
	IsRelay        bool      `json:"is_relay"`
	Status         string    `json:"status"`
	OverrideReason string    `json:"override_reason,omitempty"`
	ProposedBy     string    `json:"proposed_by,omitempty"`
	ApprovedBy     string    `json:"approved_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedBy      string    `json:"created_by"`
	UpdatedBy      string    `json:"updated_by"`
}

// EntryLeg is a relay leg in a relay entry.
type EntryLeg struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	EntryID    string    `json:"entry_id"`
	LegNumber  int       `json:"leg_number"`
	SwimmerID  string    `json:"swimmer_id"`
	SeedTimeMs *int      `json:"seed_time_ms,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	CreatedBy  string    `json:"created_by"`
	UpdatedBy  string    `json:"updated_by"`
}

// MeetAuditEvent records privileged actions taken in the meet/commitment/entry domain.
type MeetAuditEvent struct {
	ID          string         `json:"id"`
	OrgID       string         `json:"org_id"`
	ActorUserID string         `json:"actor_user_id"`
	Action      string         `json:"action"`
	Before      map[string]any `json:"before,omitempty"`
	After       map[string]any `json:"after,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// ─── Input types ──────────────────────────────────────────────────────────────

type CreateMeetInput struct {
	Name               string
	MeetType           string
	Course             string
	StartDate          string
	EndDate            string
	EntryDeadline      string
	CommitmentDeadline string
	HostOrg            string
	Location           string
	Sessions           []CreateMeetSessionInput
	Events             []CreateMeetEventDraftInput
	SignupSettings     *CreateMeetSignupSettingsInput
}

type CreateMeetSessionInput struct {
	SessionNumber int
	Name          string
	SessionDate   string
	WarmupTime    string
	StartTime     string
	Notes         string
}

type CreateMeetEventInput struct {
	SessionID     string
	EventNumber   string
	Stroke        string
	Distance      int
	Course        string
	Gender        string
	MinAge        *int
	MaxAge        *int
	EntryFeeCents int
	IsRelay       bool
}

type CommitmentRespondInput struct {
	Response string // "committed_yes" or "committed_no"
	Notes    string
}

type SelectEventInput struct {
	MeetEventID string
	SeedTimeMs  *int
}

type ProposeEntryInput struct {
	SwimmerID    string
	MeetEventID  string
	CommitmentID string
	SeedTimeMs   *int
	IsRelay      bool
	// OverrideEligibility lets a coach knowingly enter a swimmer who fails the
	// gender/age eligibility check (e.g. non-binary swimmers, bona-fide
	// exceptions). Requires OverrideReason; the override is audit-logged.
	OverrideEligibility bool
	OverrideReason      string
}

type OverrideEntryInput struct {
	NewMeetEventID string
	SeedTimeMs     *int
	Reason         string
}

type ParsedSessionDraft struct {
	SessionNumber int    `json:"session_number"`
	Section       string `json:"section"`
	Name          string `json:"name"`
	SessionDate   string `json:"session_date,omitempty"`
	WarmupTime    string `json:"warmup_time,omitempty"`
	StartTime     string `json:"start_time,omitempty"`
}

type ParsedEventDraft struct {
	SessionNumber int    `json:"session_number,omitempty"`
	EventNumber   string `json:"event_number"`
	Stroke        string `json:"stroke"`
	Distance      int    `json:"distance"`
	Course        string `json:"course,omitempty"`
	Gender        string `json:"gender,omitempty"`
	MinAge        *int   `json:"min_age,omitempty"`
	MaxAge        *int   `json:"max_age,omitempty"`
	IsRelay       bool   `json:"is_relay"`
}

type ParsedSignupSettings struct {
	SelfSignupEnabled    bool `json:"self_signup_enabled"`
	MaxEntriesPerSwimmer int  `json:"max_entries_per_swimmer"`
}

type CreateMeetEventDraftInput struct {
	SessionNumber int
	EventNumber   string
	Stroke        string
	Distance      int
	Course        string
	Gender        string
	MinAge        *int
	MaxAge        *int
	EntryFeeCents int
	IsRelay       bool
}

type CreateMeetSignupSettingsInput struct {
	SelfSignupEnabled    bool
	MaxEntriesPerSwimmer int
}

type ParseMeetFileInput struct {
	Filename string
	Data     []byte
}

type ParseMeetFileResult struct {
	FileType       string               `json:"file_type"`
	SourceFile     string               `json:"source_file"`
	MeetName       string               `json:"meet_name,omitempty"`
	Course         string               `json:"course,omitempty"`
	Location       string               `json:"location,omitempty"`
	StartDate      string               `json:"start_date,omitempty"`
	EndDate        string               `json:"end_date,omitempty"`
	Sessions       []ParsedSessionDraft `json:"sessions"`
	Events         []ParsedEventDraft   `json:"events"`
	SignupSettings ParsedSignupSettings `json:"signup_settings"`
	ParseWarnings  []string             `json:"parse_warnings,omitempty"`
}

// ─── Internal seed refs ───────────────────────────────────────────────────────

type swimmerRef struct {
	ID          string
	OrgID       string
	HouseholdID string
}

type householdRef struct {
	ID    string
	OrgID string
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service is the in-memory meets/commitments/entries service.
type Service struct {
	mu              sync.RWMutex
	repo            *Repository // nil = in-memory only
	meets           map[string]Meet
	sessions        map[string]MeetSession
	events          map[string]MeetEvent
	commitments     map[string]Commitment
	selections      map[string]EventSelection
	entries         map[string]Entry
	entryLegs       map[string]EntryLeg
	swimmers        map[string]swimmerRef
	households      map[string]householdRef
	parentHousehold map[string]map[string]struct{}
	auditEvents     []MeetAuditEvent

	// swimmerEligibility resolves a swimmer's gender/DOB for event-eligibility
	// checks. Injected from the members service (see SetSwimmerEligibilityLookup)
	// so the meets domain can gate entries without importing members. Nil in
	// tests that don't exercise eligibility.
	swimmerEligibility func(orgID, swimmerID string) (SwimmerEligibilityView, bool)
}

// SwimmerEligibilityView is the minimal swimmer data needed to validate meet
// event eligibility. Gender uses the swimmer vocabulary (female|male|
// non_binary|undisclosed); DateOfBirth is YYYY-MM-DD.
type SwimmerEligibilityView struct {
	Gender      string
	DateOfBirth string
}

// SetSwimmerEligibilityLookup wires the swimmer gender/DOB accessor used to gate
// entry creation. Safe to leave unset (eligibility checks are then skipped).
func (s *Service) SetSwimmerEligibilityLookup(fn func(orgID, swimmerID string) (SwimmerEligibilityView, bool)) {
	s.swimmerEligibility = fn
}

// swimmerAgeAsOf returns the swimmer's integer age on the given reference date
// (the meet's first day, per USA-Swimming convention). Falls back to today when
// the reference date is empty/unparseable.
func swimmerAgeAsOf(dobRaw, refDateRaw string) (int, bool) {
	dob, err := time.Parse("2006-01-02", strings.TrimSpace(dobRaw))
	if err != nil {
		return 0, false
	}
	ref := time.Now().UTC()
	if d, err := time.Parse("2006-01-02", strings.TrimSpace(refDateRaw)); err == nil {
		ref = d
	}
	age := ref.Year() - dob.Year()
	if ref.YearDay() < dob.YearDay() {
		age--
	}
	return age, true
}

// eventGenderForSwimmer maps swimmer gender vocabulary to the event gender
// vocabulary (M|F|X|O). Returns "" for unmapped values.
func eventGenderForSwimmer(swimmerGender string) string {
	switch strings.ToLower(strings.TrimSpace(swimmerGender)) {
	case "female":
		return "F"
	case "male":
		return "M"
	default:
		return "" // non_binary / undisclosed / unknown → only open/mixed events
	}
}

// validateSwimmerEventEligibility rejects entries where the swimmer's gender or
// age (as of the meet start date) does not match the event. Open/mixed events
// (gender X or O, or unspecified) accept any swimmer; unset age bounds are not
// enforced.
func validateSwimmerEventEligibility(sw SwimmerEligibilityView, ev MeetEvent, meetStartDate string) error {
	evGender := strings.ToUpper(strings.TrimSpace(ev.Gender))
	if evGender == "M" || evGender == "F" {
		if eventGenderForSwimmer(sw.Gender) != evGender {
			return fmt.Errorf("%w: swimmer gender %q is not eligible for a %s event", ErrIneligibleEntry, sw.Gender, evGender)
		}
	}
	if ev.MinAge != nil || ev.MaxAge != nil {
		age, ok := swimmerAgeAsOf(sw.DateOfBirth, meetStartDate)
		if !ok {
			return fmt.Errorf("%w: swimmer date of birth is required to check age eligibility", ErrIneligibleEntry)
		}
		if ev.MinAge != nil && age < *ev.MinAge {
			return fmt.Errorf("%w: swimmer age %d is below the event minimum %d", ErrIneligibleEntry, age, *ev.MinAge)
		}
		if ev.MaxAge != nil && age > *ev.MaxAge {
			return fmt.Errorf("%w: swimmer age %d is above the event maximum %d", ErrIneligibleEntry, age, *ev.MaxAge)
		}
	}
	return nil
}

// NewService returns a new in-memory meets service.
func NewService() *Service {
	return &Service{
		meets:           make(map[string]Meet),
		sessions:        make(map[string]MeetSession),
		events:          make(map[string]MeetEvent),
		commitments:     make(map[string]Commitment),
		selections:      make(map[string]EventSelection),
		entries:         make(map[string]Entry),
		entryLegs:       make(map[string]EntryLeg),
		swimmers:        make(map[string]swimmerRef),
		households:      make(map[string]householdRef),
		parentHousehold: make(map[string]map[string]struct{}),
		auditEvents:     []MeetAuditEvent{},
	}
}

// NewServiceWithPool creates a DB-backed meets service.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("meets: failed to load initial state from DB", "error", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	ms, sessions, events, commitments, entries, swimmers, households, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetDBBackedStateLocked()
	s.populateDBBackedStateLocked(ms, sessions, events, commitments, entries, swimmers, households)
	return nil
}

func (s *Service) syncFromDB(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	ms, sessions, events, commitments, entries, swimmers, households, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetDBBackedStateLocked()
	s.populateDBBackedStateLocked(ms, sessions, events, commitments, entries, swimmers, households)
	return nil
}

func (s *Service) resetDBBackedStateLocked() {
	s.meets = make(map[string]Meet)
	s.sessions = make(map[string]MeetSession)
	s.events = make(map[string]MeetEvent)
	s.commitments = make(map[string]Commitment)
	s.entries = make(map[string]Entry)
	s.swimmers = make(map[string]swimmerRef)
	s.households = make(map[string]householdRef)
}

func (s *Service) populateDBBackedStateLocked(
	ms []Meet,
	sessions []MeetSession,
	events []MeetEvent,
	commitments []Commitment,
	entries []Entry,
	swimmers []swimmerRef,
	households []householdRef,
) {
	for _, m := range ms {
		s.meets[m.ID] = m
	}
	for _, sess := range sessions {
		s.sessions[sess.ID] = sess
	}
	for _, e := range events {
		s.events[e.ID] = e
	}
	for _, c := range commitments {
		s.commitments[c.ID] = c
	}
	for _, e := range entries {
		s.entries[e.ID] = e
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

func (s *Service) ParseMeetFile(principal *appmiddleware.Principal, orgID string, input ParseMeetFileInput) (*ParseMeetFileResult, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(orgID) == "" {
		return nil, fmt.Errorf("%w: org id is required", ErrInvalidMeetFile)
	}
	filename := strings.TrimSpace(input.Filename)
	if filename == "" {
		return nil, fmt.Errorf("%w: filename is required", ErrInvalidMeetFile)
	}
	if len(input.Data) == 0 {
		return nil, fmt.Errorf("%w: file is empty", ErrInvalidMeetFile)
	}

	result, warnings, err := parseMeetDefinition(filename, input.Data)
	if err != nil {
		return nil, err
	}

	startDate := ""
	if result.MeetStartDate != nil {
		startDate = result.MeetStartDate.UTC().Format("2006-01-02")
	}
	endDate := ""
	if result.MeetEndDate != nil {
		endDate = result.MeetEndDate.UTC().Format("2006-01-02")
	}

	return &ParseMeetFileResult{
		FileType:       string(result.FileType),
		SourceFile:     strings.TrimSpace(result.SourceFile),
		MeetName:       strings.TrimSpace(result.MeetName),
		Course:         strings.TrimSpace(string(result.Course)),
		Location:       strings.TrimSpace(result.Location),
		StartDate:      startDate,
		EndDate:        endDate,
		Sessions:       deriveSessionDrafts(result),
		Events:         deriveEventDrafts(result),
		SignupSettings: deriveSignupSettings(result),
		ParseWarnings:  warnings,
	}, nil
}

func parseMeetDefinition(filename string, data []byte) (*meetfiles.ParsedMeetDefinition, []string, error) {
	fileType := meetfiles.DetectFileType(filename, data)
	switch fileType {
	case meetfiles.FileTypeSD3:
		result, err := sdif.Parse(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse SD3: %v", ErrInvalidMeetFile, err)
		}
		return parsedMeetDefinitionFromResult(filename, result), parseWarnings(result.ParseErrors), nil
	case meetfiles.FileTypeCL2:
		result, err := cl2.Parse(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse CL2: %v", ErrInvalidMeetFile, err)
		}
		return parsedMeetDefinitionFromResult(filename, result), parseWarnings(result.ParseErrors), nil
	case meetfiles.FileTypeHY3:
		ingest := meetfiles.ReadHY3(filename, data)
		if ingest.SelectedPayload == nil {
			if ingest.Error != nil {
				return nil, nil, fmt.Errorf("%w: %s", ErrInvalidMeetFile, ingest.Error.Message)
			}
			return nil, nil, fmt.Errorf("%w: hy3 payload missing", ErrInvalidMeetFile)
		}
		result, warnings, err := parseMeetDefinition(ingest.SelectedPayload.Name, ingest.SelectedPayload.Content)
		if err != nil {
			return nil, nil, err
		}
		if ingest.Error != nil {
			warnings = append([]string{ingest.Error.Message}, warnings...)
		}
		return result, warnings, nil
	case meetfiles.FileTypeHYV:
		result, err := meetfiles.ParseHYV(filename, data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse HYV: %v", ErrInvalidMeetFile, err)
		}
		return result, parseWarnings(result.ParseErrors), nil
	case meetfiles.FileTypeEV3:
		result, err := meetfiles.ParseEV3(filename, data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse EV3: %v", ErrInvalidMeetFile, err)
		}
		return result, parseWarnings(result.ParseErrors), nil
	case meetfiles.FileTypeZIP:
		entries, err := meetfiles.ReadMeetZip(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse ZIP: %v", ErrInvalidMeetFile, err)
		}
		if len(entries) == 0 {
			return nil, nil, fmt.Errorf("%w: zip archive does not contain a parseable ev3/hyv/sd3/cl2 payload", ErrInvalidMeetFile)
		}
		sort.Slice(entries, func(i, j int) bool {
			iType := meetfiles.DetectFileType(entries[i].Name, entries[i].Content)
			jType := meetfiles.DetectFileType(entries[j].Name, entries[j].Content)
			if iType != jType {
				return meetFileTypePriority(iType) < meetFileTypePriority(jType)
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		result, warnings, err := parseMeetDefinition(entries[0].Name, entries[0].Content)
		if err != nil {
			return nil, nil, err
		}
		if len(entries) > 1 {
			warnings = append([]string{fmt.Sprintf("zip archive contained %d parseable files; selected %s", len(entries), entries[0].Name)}, warnings...)
		}
		return result, warnings, nil
	default:
		return nil, nil, fmt.Errorf("%w: unsupported file type", ErrInvalidMeetFile)
	}
}

func parseWarnings(errs []meetfiles.ParseError) []string {
	if len(errs) == 0 {
		return nil
	}
	limit := min(len(errs), 5)
	out := make([]string, 0, limit)
	for _, parseErr := range errs[:limit] {
		if strings.TrimSpace(parseErr.Message) != "" {
			out = append(out, parseErr.Message)
		}
	}
	return out
}

func parsedMeetDefinitionFromResult(sourceFile string, result *meetfiles.ParseResult) *meetfiles.ParsedMeetDefinition {
	if result == nil {
		return &meetfiles.ParsedMeetDefinition{SourceFile: sourceFile}
	}
	return &meetfiles.ParsedMeetDefinition{
		FileType:      result.FileType,
		SourceFile:    sourceFile,
		MeetName:      result.MeetName,
		MeetStartDate: result.MeetStartDate,
		MeetEndDate:   result.MeetEndDate,
		Course:        result.Course,
		ParseErrors:   append([]meetfiles.ParseError{}, result.ParseErrors...),
	}
}

func meetFileTypePriority(fileType meetfiles.FileType) int {
	switch fileType {
	case meetfiles.FileTypeEV3:
		return 1
	case meetfiles.FileTypeHYV:
		return 2
	case meetfiles.FileTypeSD3:
		return 3
	case meetfiles.FileTypeCL2:
		return 4
	default:
		return 100
	}
}

func deriveSessionDrafts(result *meetfiles.ParsedMeetDefinition) []ParsedSessionDraft {
	if result == nil {
		return []ParsedSessionDraft{{SessionNumber: 1, Section: "Day 1", Name: "Session 1"}}
	}
	if len(result.Sessions) > 0 {
		drafts := make([]ParsedSessionDraft, 0, len(result.Sessions))
		for _, session := range result.Sessions {
			sessionDate := ""
			if session.SessionDate != nil {
				sessionDate = session.SessionDate.UTC().Format("2006-01-02")
			}
			drafts = append(drafts, ParsedSessionDraft{
				SessionNumber: session.SessionNumber,
				Section:       strings.TrimSpace(session.Section),
				Name:          strings.TrimSpace(session.Name),
				SessionDate:   sessionDate,
				WarmupTime:    strings.TrimSpace(session.WarmupTime),
				StartTime:     strings.TrimSpace(session.StartTime),
			})
		}
		return drafts
	}

	start := result.MeetStartDate
	end := result.MeetEndDate
	if start == nil && end != nil {
		start = end
	}
	if end == nil && start != nil {
		end = start
	}
	if start == nil || end == nil {
		return []ParsedSessionDraft{{SessionNumber: 1, Section: "Day 1", Name: "Session 1"}}
	}

	startDay := start.UTC().Truncate(24 * time.Hour)
	endDay := end.UTC().Truncate(24 * time.Hour)
	if endDay.Before(startDay) {
		endDay = startDay
	}

	var drafts []ParsedSessionDraft
	for day := startDay; !day.After(endDay); day = day.Add(24 * time.Hour) {
		number := len(drafts) + 1
		drafts = append(drafts, ParsedSessionDraft{
			SessionNumber: number,
			Section:       fmt.Sprintf("Day %d", number),
			Name:          fmt.Sprintf("Session %d", number),
			SessionDate:   day.Format("2006-01-02"),
		})
	}
	if len(drafts) == 0 {
		return []ParsedSessionDraft{{SessionNumber: 1, Section: "Day 1", Name: "Session 1"}}
	}
	return drafts
}

func deriveEventDrafts(result *meetfiles.ParsedMeetDefinition) []ParsedEventDraft {
	if result == nil || len(result.Events) == 0 {
		return nil
	}
	out := make([]ParsedEventDraft, 0, len(result.Events))
	for _, event := range result.Events {
		out = append(out, ParsedEventDraft{
			SessionNumber: event.SessionNumber,
			EventNumber:   strings.TrimSpace(event.EventNumber),
			Stroke:        meetEventStrokeFromParsed(event.Stroke),
			Distance:      event.Distance,
			Course:        strings.TrimSpace(string(event.Course)),
			Gender:        strings.TrimSpace(event.Gender),
			MinAge:        event.MinAge,
			MaxAge:        event.MaxAge,
			IsRelay:       event.IsRelay,
		})
	}
	return out
}

func deriveSignupSettings(result *meetfiles.ParsedMeetDefinition) ParsedSignupSettings {
	return ParsedSignupSettings{
		SelfSignupEnabled:    true,
		MaxEntriesPerSwimmer: 3,
	}
}

func meetEventStrokeFromParsed(stroke meetfiles.Stroke) string {
	switch stroke {
	case meetfiles.StrokeFreestyle:
		return "freestyle"
	case meetfiles.StrokeBackstroke:
		return "backstroke"
	case meetfiles.StrokeBreaststroke:
		return "breaststroke"
	case meetfiles.StrokeButterfly:
		return "butterfly"
	case meetfiles.StrokeIM:
		return "individual_medley"
	case meetfiles.StrokeFreestyleRelay:
		return "freestyle_relay"
	case meetfiles.StrokeMedleyRelay:
		return "medley_relay"
	default:
		return "other"
	}
}

// SeedEntry directly inserts a meet event and entry into the service for test
// setup. It bypasses the normal lifecycle validation so that tests can
// pre-populate entries without walking the full meet-lifecycle state machine.
// Returns the generated entry ID.
// Not intended for production use.
func (s *Service) SeedEntry(orgID, swimmerID, meetID, eventStroke, eventCourse string, eventDistance int, seedTimeMs *int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	entryID := "entry_" + randomToken(12)
	eventID := "event_" + randomToken(12)

	// Register the meet event so lookups work.
	s.events[eventID] = MeetEvent{
		ID:       eventID,
		OrgID:    orgID,
		MeetID:   meetID,
		Stroke:   eventStroke,
		Distance: eventDistance,
		Course:   eventCourse,
	}
	// Register a stub meet if not already present.
	if _, ok := s.meets[meetID]; !ok {
		s.meets[meetID] = Meet{
			ID:    meetID,
			OrgID: orgID,
		}
	}
	s.entries[entryID] = Entry{
		ID:          entryID,
		OrgID:       orgID,
		MeetID:      meetID,
		SwimmerID:   swimmerID,
		MeetEventID: eventID,
		SeedTimeMs:  seedTimeMs,
		Status:      "submitted",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return entryID
}

// GetEntrySeedTime returns the current SeedTimeMs for an entry by ID.
// Returns nil, false if the entry is not found.
// Intended for use in tests.
func (s *Service) GetEntrySeedTime(entryID string) (*int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[entryID]
	if !ok {
		return nil, false
	}
	return e.SeedTimeMs, true
}

// SetEntryStatusForTest directly mutates an entry's status for test setup.
// Not intended for production use.
func (s *Service) SetEntryStatusForTest(entryID, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[entryID]; ok {
		e.Status = status
		s.entries[entryID] = e
	}
}

// SeedSwimmer registers a swimmer for scope enforcement. In production this would be a DB read.
func (s *Service) SeedSwimmer(orgID, swimmerID, householdID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.swimmers[swimmerID] = swimmerRef{ID: swimmerID, OrgID: orgID, HouseholdID: householdID}
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

// ─── Meets ────────────────────────────────────────────────────────────────────

// CreateMeet creates a new meet record.
func (s *Service) CreateMeet(principal *appmiddleware.Principal, orgID string, input CreateMeetInput) (Meet, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return Meet{}, MeetAuditEvent{}, ErrForbidden
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Meet{}, MeetAuditEvent{}, errors.New("name is required")
	}

	meetType := normalizeChoice(input.MeetType, "invitational")
	if err := validateMeetType(meetType); err != nil {
		return Meet{}, MeetAuditEvent{}, err
	}

	course := normalizeChoice(input.Course, "SCY")
	if err := validateCourse(course); err != nil {
		return Meet{}, MeetAuditEvent{}, err
	}

	signupSettings := CreateMeetSignupSettingsInput{
		SelfSignupEnabled:    true,
		MaxEntriesPerSwimmer: 3,
	}
	if input.SignupSettings != nil {
		signupSettings = *input.SignupSettings
		if signupSettings.MaxEntriesPerSwimmer <= 0 {
			signupSettings.MaxEntriesPerSwimmer = 3
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	meet := Meet{
		ID:                   s.newID("meet_"),
		OrgID:                orgID,
		Name:                 name,
		MeetType:             meetType,
		Course:               course,
		StartDate:            strings.TrimSpace(input.StartDate),
		EndDate:              strings.TrimSpace(input.EndDate),
		EntryDeadline:        strings.TrimSpace(input.EntryDeadline),
		CommitmentDeadline:   strings.TrimSpace(input.CommitmentDeadline),
		HostOrg:              strings.TrimSpace(input.HostOrg),
		Location:             strings.TrimSpace(input.Location),
		SelfSignupEnabled:    signupSettings.SelfSignupEnabled,
		MaxEntriesPerSwimmer: signupSettings.MaxEntriesPerSwimmer,
		Status:               "draft",
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            principal.UserID,
		UpdatedBy:            principal.UserID,
	}

	sessionByNumber := map[int]string{}
	sessionsToCreate := make([]MeetSession, 0, len(input.Sessions))
	for _, item := range input.Sessions {
		sessionName := strings.TrimSpace(item.Name)
		if sessionName == "" {
			return Meet{}, MeetAuditEvent{}, errors.New("session name is required")
		}
		if item.SessionNumber <= 0 {
			return Meet{}, MeetAuditEvent{}, errors.New("session_number must be a positive integer")
		}
		if _, exists := sessionByNumber[item.SessionNumber]; exists {
			return Meet{}, MeetAuditEvent{}, fmt.Errorf("session_number %d already exists in this meet", item.SessionNumber)
		}
		sessionID := s.newID("sess_")
		sessionByNumber[item.SessionNumber] = sessionID
		sessionsToCreate = append(sessionsToCreate, MeetSession{
			ID:            sessionID,
			OrgID:         orgID,
			MeetID:        meet.ID,
			SessionNumber: item.SessionNumber,
			Name:          sessionName,
			SessionDate:   strings.TrimSpace(item.SessionDate),
			WarmupTime:    strings.TrimSpace(item.WarmupTime),
			StartTime:     strings.TrimSpace(item.StartTime),
			Notes:         strings.TrimSpace(item.Notes),
			CreatedAt:     now,
			UpdatedAt:     now,
			CreatedBy:     principal.UserID,
			UpdatedBy:     principal.UserID,
		})
	}

	eventNumbers := map[string]struct{}{}
	eventsToCreate := make([]MeetEvent, 0, len(input.Events))
	for _, item := range input.Events {
		eventNumber := strings.TrimSpace(item.EventNumber)
		if eventNumber == "" {
			return Meet{}, MeetAuditEvent{}, errors.New("event_number is required")
		}
		if _, exists := eventNumbers[eventNumber]; exists {
			return Meet{}, MeetAuditEvent{}, ErrEventNumberConflict
		}
		eventNumbers[eventNumber] = struct{}{}

		stroke := strings.TrimSpace(strings.ToLower(item.Stroke))
		if stroke == "" {
			return Meet{}, MeetAuditEvent{}, errors.New("stroke is required")
		}
		if err := validateStroke(stroke); err != nil {
			return Meet{}, MeetAuditEvent{}, err
		}
		if item.Distance <= 0 {
			return Meet{}, MeetAuditEvent{}, errors.New("distance must be a positive integer")
		}
		eventCourse := normalizeChoice(item.Course, meet.Course)
		if err := validateCourse(eventCourse); err != nil {
			return Meet{}, MeetAuditEvent{}, err
		}
		gender := normalizeChoice(item.Gender, "O")
		if err := validateGender(gender); err != nil {
			return Meet{}, MeetAuditEvent{}, err
		}

		sessionID := ""
		if item.SessionNumber > 0 {
			mapped, ok := sessionByNumber[item.SessionNumber]
			if !ok {
				return Meet{}, MeetAuditEvent{}, ErrSessionNotFound
			}
			sessionID = mapped
		}

		eventsToCreate = append(eventsToCreate, MeetEvent{
			ID:            s.newID("evt_"),
			OrgID:         orgID,
			MeetID:        meet.ID,
			SessionID:     sessionID,
			EventNumber:   eventNumber,
			Stroke:        stroke,
			Distance:      item.Distance,
			Course:        eventCourse,
			Gender:        gender,
			MinAge:        item.MinAge,
			MaxAge:        item.MaxAge,
			EntryFeeCents: item.EntryFeeCents,
			IsRelay:       item.IsRelay,
			CreatedAt:     now,
			UpdatedAt:     now,
			CreatedBy:     principal.UserID,
			UpdatedBy:     principal.UserID,
		})
	}

	if s.repo != nil {
		if err := s.repo.CreateMeetBundle(context.Background(), meet, sessionsToCreate, eventsToCreate); err != nil {
			return Meet{}, MeetAuditEvent{}, fmt.Errorf("persist meet bundle: %w", err)
		}
	}
	s.meets[meet.ID] = meet
	for _, session := range sessionsToCreate {
		s.sessions[session.ID] = session
	}
	for _, event := range eventsToCreate {
		s.events[event.ID] = event
	}

	event := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "meet.created",
		After:       meetSnapshot(meet),
		Metadata:    map[string]any{"meet_id": meet.ID},
	})
	for _, session := range sessionsToCreate {
		s.recordAuditEventLocked(MeetAuditEvent{
			OrgID:       orgID,
			ActorUserID: principal.UserID,
			Action:      "meet.session_created",
			After:       map[string]any{"session_id": session.ID, "session_number": session.SessionNumber, "name": session.Name},
			Metadata:    map[string]any{"meet_id": meet.ID},
		})
	}
	for _, meetEvent := range eventsToCreate {
		s.recordAuditEventLocked(MeetAuditEvent{
			OrgID:       orgID,
			ActorUserID: principal.UserID,
			Action:      "meet.event_created",
			After:       map[string]any{"event_id": meetEvent.ID, "event_number": meetEvent.EventNumber, "stroke": meetEvent.Stroke, "distance": meetEvent.Distance},
			Metadata:    map[string]any{"meet_id": meet.ID},
		})
	}
	return meet, event, nil
}

// ListMeets returns all meets in an org.
func (s *Service) ListMeets(principal *appmiddleware.Principal, orgID string) ([]Meet, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent", "billing", "read-only", "swimmer") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Meet
	for _, m := range s.meets {
		if m.OrgID == orgID {
			result = append(result, m)
		}
	}
	return result, nil
}

// GetMeet retrieves a single meet by ID.
func (s *Service) GetMeet(principal *appmiddleware.Principal, orgID, meetID string) (Meet, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent", "billing", "read-only", "swimmer") {
		return Meet{}, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Meet{}, ErrMeetNotFound
	}
	return meet, nil
}

// TransitionMeetStatus moves a meet through its lifecycle state machine.
func (s *Service) TransitionMeetStatus(principal *appmiddleware.Principal, orgID, meetID, toStatus string) (Meet, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return Meet{}, MeetAuditEvent{}, ErrForbidden
	}

	toStatus = strings.TrimSpace(strings.ToLower(toStatus))

	// commitment_open and commitment_closed are exclusively managed by their dedicated endpoints.
	if toStatus == "commitment_open" || toStatus == "commitment_closed" {
		return Meet{}, MeetAuditEvent{}, fmt.Errorf("%w: use /commitment-request to open commitments and /lock-commitments to close them", ErrInvalidTransition)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Meet{}, MeetAuditEvent{}, ErrMeetNotFound
	}

	if err := validateMeetTransition(meet.Status, toStatus); err != nil {
		return Meet{}, MeetAuditEvent{}, err
	}

	fromStatus := meet.Status
	now := time.Now().UTC()
	meet.Status = toStatus
	meet.UpdatedAt = now
	meet.UpdatedBy = principal.UserID

	switch toStatus {
	case "commitment_open":
		meet.CommitmentOpenAt = &now
	case "commitment_closed":
		meet.CommitmentClosedAt = &now
	case "entry_finalized":
		meet.EntryFinalizedAt = &now
	}

	if s.repo != nil {
		if err := s.repo.UpsertMeet(context.Background(), meet); err != nil {
			return Meet{}, MeetAuditEvent{}, fmt.Errorf("persist meet transition: %w", err)
		}
	}
	s.meets[meet.ID] = meet

	event := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "meet.status_transitioned",
		Before:      map[string]any{"status": fromStatus},
		After:       map[string]any{"status": toStatus},
		Metadata:    map[string]any{"meet_id": meet.ID},
	})
	return meet, event, nil
}

// ─── Meet Sessions ────────────────────────────────────────────────────────────

// CreateMeetSession adds a session to a meet.
func (s *Service) CreateMeetSession(principal *appmiddleware.Principal, orgID, meetID string, input CreateMeetSessionInput) (MeetSession, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return MeetSession{}, MeetAuditEvent{}, ErrForbidden
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return MeetSession{}, MeetAuditEvent{}, errors.New("name is required")
	}
	if input.SessionNumber <= 0 {
		return MeetSession{}, MeetAuditEvent{}, errors.New("session_number must be a positive integer")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return MeetSession{}, MeetAuditEvent{}, ErrMeetNotFound
	}

	// Check for duplicate session number in this meet.
	for _, sess := range s.sessions {
		if sess.MeetID == meet.ID && sess.SessionNumber == input.SessionNumber {
			return MeetSession{}, MeetAuditEvent{}, fmt.Errorf("session_number %d already exists in this meet", input.SessionNumber)
		}
	}

	now := time.Now().UTC()
	session := MeetSession{
		ID:            s.newID("sess_"),
		OrgID:         orgID,
		MeetID:        meet.ID,
		SessionNumber: input.SessionNumber,
		Name:          name,
		SessionDate:   strings.TrimSpace(input.SessionDate),
		WarmupTime:    strings.TrimSpace(input.WarmupTime),
		StartTime:     strings.TrimSpace(input.StartTime),
		Notes:         strings.TrimSpace(input.Notes),
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     principal.UserID,
		UpdatedBy:     principal.UserID,
	}
	if s.repo != nil {
		if err := s.repo.UpsertMeetSession(context.Background(), session); err != nil {
			return MeetSession{}, MeetAuditEvent{}, fmt.Errorf("persist meet session: %w", err)
		}
	}
	s.sessions[session.ID] = session

	event := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "meet.session_created",
		After:       map[string]any{"session_id": session.ID, "session_number": session.SessionNumber, "name": session.Name},
		Metadata:    map[string]any{"meet_id": meet.ID},
	})
	return session, event, nil
}

// ListMeetSessions returns all sessions for a meet.
func (s *Service) ListMeetSessions(principal *appmiddleware.Principal, orgID, meetID string) ([]MeetSession, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent", "billing", "read-only", "swimmer") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return nil, ErrMeetNotFound
	}

	var result []MeetSession
	for _, sess := range s.sessions {
		if sess.MeetID == meet.ID {
			result = append(result, sess)
		}
	}
	return result, nil
}

// ─── Meet Events ──────────────────────────────────────────────────────────────

// CreateMeetEvent adds an event to a meet.
func (s *Service) CreateMeetEvent(principal *appmiddleware.Principal, orgID, meetID string, input CreateMeetEventInput) (MeetEvent, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return MeetEvent{}, MeetAuditEvent{}, ErrForbidden
	}

	eventNumber := strings.TrimSpace(input.EventNumber)
	stroke := strings.TrimSpace(strings.ToLower(input.Stroke))
	if eventNumber == "" {
		return MeetEvent{}, MeetAuditEvent{}, errors.New("event_number is required")
	}
	if stroke == "" {
		return MeetEvent{}, MeetAuditEvent{}, errors.New("stroke is required")
	}
	if err := validateStroke(stroke); err != nil {
		return MeetEvent{}, MeetAuditEvent{}, err
	}
	if input.Distance <= 0 {
		return MeetEvent{}, MeetAuditEvent{}, errors.New("distance must be a positive integer")
	}

	course := normalizeChoice(input.Course, "SCY")
	if err := validateCourse(course); err != nil {
		return MeetEvent{}, MeetAuditEvent{}, err
	}

	gender := normalizeChoice(input.Gender, "O")
	if err := validateGender(gender); err != nil {
		return MeetEvent{}, MeetAuditEvent{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return MeetEvent{}, MeetAuditEvent{}, ErrMeetNotFound
	}

	// Validate optional session_id.
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID != "" {
		sess, ok := s.sessions[sessionID]
		if !ok || sess.MeetID != meet.ID {
			return MeetEvent{}, MeetAuditEvent{}, ErrSessionNotFound
		}
	}

	// Enforce unique event_number within meet.
	for _, ev := range s.events {
		if ev.MeetID == meet.ID && ev.EventNumber == eventNumber {
			return MeetEvent{}, MeetAuditEvent{}, ErrEventNumberConflict
		}
	}

	now := time.Now().UTC()
	meetEvent := MeetEvent{
		ID:            s.newID("evt_"),
		OrgID:         orgID,
		MeetID:        meet.ID,
		SessionID:     sessionID,
		EventNumber:   eventNumber,
		Stroke:        stroke,
		Distance:      input.Distance,
		Course:        course,
		Gender:        gender,
		MinAge:        input.MinAge,
		MaxAge:        input.MaxAge,
		EntryFeeCents: input.EntryFeeCents,
		IsRelay:       input.IsRelay,
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     principal.UserID,
		UpdatedBy:     principal.UserID,
	}
	if s.repo != nil {
		if err := s.repo.UpsertMeetEvent(context.Background(), meetEvent); err != nil {
			return MeetEvent{}, MeetAuditEvent{}, fmt.Errorf("persist meet event: %w", err)
		}
	}
	s.events[meetEvent.ID] = meetEvent

	event := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "meet.event_created",
		After:       map[string]any{"event_id": meetEvent.ID, "event_number": eventNumber, "stroke": stroke, "distance": input.Distance},
		Metadata:    map[string]any{"meet_id": meet.ID},
	})
	return meetEvent, event, nil
}

// ListMeetEvents returns all events for a meet.
func (s *Service) ListMeetEvents(principal *appmiddleware.Principal, orgID, meetID string) ([]MeetEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent", "billing", "read-only", "swimmer") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return nil, ErrMeetNotFound
	}

	var result []MeetEvent
	for _, ev := range s.events {
		if ev.MeetID == meet.ID {
			result = append(result, ev)
		}
	}
	return result, nil
}

// ─── Commitment request ───────────────────────────────────────────────────────

// PublishCommitmentRequest opens the commitment window for a meet and creates commitment
// records for all seeded swimmers in the org.
func (s *Service) PublishCommitmentRequest(principal *appmiddleware.Principal, orgID, meetID string) (Meet, []Commitment, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return Meet{}, nil, MeetAuditEvent{}, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return Meet{}, nil, MeetAuditEvent{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Meet{}, nil, MeetAuditEvent{}, ErrMeetNotFound
	}

	// Must be published to open commitments; draft is not sufficient.
	if err := validateMeetTransition(meet.Status, "commitment_open"); err != nil {
		return Meet{}, nil, MeetAuditEvent{}, err
	}

	now := time.Now().UTC()
	meet.Status = "commitment_open"
	meet.CommitmentOpenAt = &now
	meet.UpdatedAt = now
	meet.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertMeet(context.Background(), meet); err != nil {
			return Meet{}, nil, MeetAuditEvent{}, fmt.Errorf("persist meet (commitment_open): %w", err)
		}
	}
	s.meets[meet.ID] = meet

	// Create commitment records for all swimmers in the org that don't already have one.
	var created []Commitment
	for _, swimmer := range s.swimmers {
		if swimmer.OrgID != orgID {
			continue
		}
		// Skip if commitment already exists.
		alreadyExists := false
		for _, c := range s.commitments {
			if c.MeetID == meet.ID && c.SwimmerID == swimmer.ID {
				alreadyExists = true
				break
			}
		}
		if alreadyExists {
			continue
		}
		c := Commitment{
			ID:          s.newID("cmt_"),
			OrgID:       orgID,
			MeetID:      meet.ID,
			HouseholdID: swimmer.HouseholdID,
			SwimmerID:   swimmer.ID,
			Status:      "invited",
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   principal.UserID,
			UpdatedBy:   principal.UserID,
		}
		if s.repo != nil {
			if err := s.repo.UpsertCommitment(context.Background(), c); err != nil {
				return Meet{}, nil, MeetAuditEvent{}, fmt.Errorf("persist commitment (publish): %w", err)
			}
		}
		s.commitments[c.ID] = c
		created = append(created, c)
	}

	event := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "meet.commitment_request_published",
		After:       map[string]any{"status": "commitment_open", "commitments_created": len(created)},
		Metadata:    map[string]any{"meet_id": meet.ID},
	})
	return meet, created, event, nil
}

// ─── Commitments ─────────────────────────────────────────────────────────────

// ListCommitments returns commitments for a meet. Parents only see their own household's commitments.
func (s *Service) ListCommitments(principal *appmiddleware.Principal, orgID, meetID string) ([]Commitment, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent", "swimmer") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return nil, ErrMeetNotFound
	}

	var result []Commitment
	for _, c := range s.commitments {
		if c.MeetID != meet.ID {
			continue
		}
		// Parent scope: only see their own household's commitments.
		if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin", "coach") {
			if !s.principalOwnsHouseholdLocked(principal, c.HouseholdID) {
				continue
			}
		}
		// Swimmer-only scope: only commitments for their own household.
		if hasAnyRole(principal, "swimmer") && !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
			if strings.TrimSpace(principal.HouseholdID) == "" || strings.TrimSpace(c.HouseholdID) != strings.TrimSpace(principal.HouseholdID) {
				continue
			}
		}
		result = append(result, c)
	}
	return result, nil
}

// RespondToCommitment records a parent's yes/no commitment response.
func (s *Service) RespondToCommitment(principal *appmiddleware.Principal, orgID, meetID, commitmentID string, input CommitmentRespondInput) (Commitment, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return Commitment{}, MeetAuditEvent{}, ErrForbidden
	}
	// Coach role is not permitted to respond to commitments per RBAC table.
	if hasAnyRole(principal, "coach") && !hasAnyRole(principal, "owner", "admin") {
		return Commitment{}, MeetAuditEvent{}, ErrForbidden
	}

	response := strings.TrimSpace(strings.ToLower(input.Response))
	if response != "committed_yes" && response != "committed_no" {
		return Commitment{}, MeetAuditEvent{}, errors.New("response must be 'committed_yes' or 'committed_no'")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Commitment{}, MeetAuditEvent{}, ErrMeetNotFound
	}

	if meet.Status != "commitment_open" {
		return Commitment{}, MeetAuditEvent{}, fmt.Errorf("%w: commitments are not open for this meet (status: %s)", ErrInvalidTransition, meet.Status)
	}

	cmt, ok := s.commitments[strings.TrimSpace(commitmentID)]
	if !ok || cmt.MeetID != meet.ID || cmt.OrgID != orgID {
		return Commitment{}, MeetAuditEvent{}, ErrCommitmentNotFound
	}

	// Parent scope: can only respond for their own household.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin", "coach") {
		if !s.principalOwnsHouseholdLocked(principal, cmt.HouseholdID) {
			return Commitment{}, MeetAuditEvent{}, ErrForbidden
		}
	}

	if cmt.Status != "invited" && cmt.Status != "committed_yes" && cmt.Status != "committed_no" {
		return Commitment{}, MeetAuditEvent{}, fmt.Errorf("%w: cannot respond to commitment in status %q", ErrInvalidTransition, cmt.Status)
	}

	fromStatus := cmt.Status
	now := time.Now().UTC()
	cmt.Status = response
	cmt.Notes = strings.TrimSpace(input.Notes)
	cmt.CommittedAt = &now
	cmt.UpdatedAt = now
	cmt.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertCommitment(context.Background(), cmt); err != nil {
			return Commitment{}, MeetAuditEvent{}, fmt.Errorf("persist commitment (respond): %w", err)
		}
	}
	s.commitments[cmt.ID] = cmt

	// If switching to committed_no, remove any existing event selections atomically.
	if response == "committed_no" {
		for selID, sel := range s.selections {
			if sel.CommitmentID == cmt.ID {
				delete(s.selections, selID)
			}
		}
	}

	event := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "commitment.responded",
		Before:      map[string]any{"status": fromStatus},
		After:       map[string]any{"status": cmt.Status},
		Metadata:    map[string]any{"meet_id": meet.ID, "commitment_id": cmt.ID, "swimmer_id": cmt.SwimmerID},
	})
	return cmt, event, nil
}

// ─── Event selections (parent) ────────────────────────────────────────────────

// SelectCommitmentEvent adds an event selection for a parent-committed swimmer.
// Only allowed when commitment status is committed_yes and meet is commitment_open.
func (s *Service) SelectCommitmentEvent(principal *appmiddleware.Principal, orgID, meetID, commitmentID string, input SelectEventInput) (EventSelection, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return EventSelection{}, ErrForbidden
	}

	meetEventID := strings.TrimSpace(input.MeetEventID)
	if meetEventID == "" {
		return EventSelection{}, errors.New("meet_event_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return EventSelection{}, ErrMeetNotFound
	}

	// Event selections are only allowed when commitments are open.
	if meet.Status != "commitment_open" {
		return EventSelection{}, ErrCommitmentLocked
	}

	cmt, ok := s.commitments[strings.TrimSpace(commitmentID)]
	if !ok || cmt.MeetID != meet.ID || cmt.OrgID != orgID {
		return EventSelection{}, ErrCommitmentNotFound
	}

	// Parent scope: can only select events for their own household.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin", "coach") {
		if !s.principalOwnsHouseholdLocked(principal, cmt.HouseholdID) {
			return EventSelection{}, ErrForbidden
		}
	}

	if cmt.Status != "committed_yes" {
		return EventSelection{}, ErrNotCommittedYes
	}

	// Validate the meet event exists and belongs to this meet.
	ev, ok := s.events[meetEventID]
	if !ok || ev.MeetID != meet.ID {
		return EventSelection{}, ErrEventNotFound
	}

	// Idempotency: one selection per commitment+event.
	for _, sel := range s.selections {
		if sel.CommitmentID == cmt.ID && sel.MeetEventID == meetEventID {
			return EventSelection{}, ErrDuplicateSelection
		}
	}

	now := time.Now().UTC()
	sel := EventSelection{
		ID:           s.newID("sel_"),
		OrgID:        orgID,
		CommitmentID: cmt.ID,
		MeetEventID:  meetEventID,
		SeedTimeMs:   input.SeedTimeMs,
		CreatedAt:    now,
		CreatedBy:    principal.UserID,
	}
	s.selections[sel.ID] = sel
	return sel, nil
}

// RemoveCommitmentEventSelection removes a parent's event selection before lock.
func (s *Service) RemoveCommitmentEventSelection(principal *appmiddleware.Principal, orgID, meetID, commitmentID, selectionID string) error {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return ErrMeetNotFound
	}

	if meet.Status != "commitment_open" {
		return ErrCommitmentLocked
	}

	cmt, ok := s.commitments[strings.TrimSpace(commitmentID)]
	if !ok || cmt.MeetID != meet.ID || cmt.OrgID != orgID {
		return ErrCommitmentNotFound
	}

	// Parent scope.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin", "coach") {
		if !s.principalOwnsHouseholdLocked(principal, cmt.HouseholdID) {
			return ErrForbidden
		}
	}

	sel, ok := s.selections[strings.TrimSpace(selectionID)]
	if !ok || sel.CommitmentID != cmt.ID {
		return ErrSelectionNotFound
	}

	delete(s.selections, sel.ID)
	return nil
}

// ListCommitmentEventSelections returns the event selections for a commitment.
func (s *Service) ListCommitmentEventSelections(principal *appmiddleware.Principal, orgID, meetID, commitmentID string) ([]EventSelection, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return nil, ErrMeetNotFound
	}

	cmt, ok := s.commitments[strings.TrimSpace(commitmentID)]
	if !ok || cmt.MeetID != meet.ID || cmt.OrgID != orgID {
		return nil, ErrCommitmentNotFound
	}

	// Parent scope.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin", "coach") {
		if !s.principalOwnsHouseholdLocked(principal, cmt.HouseholdID) {
			return nil, ErrForbidden
		}
	}

	var result []EventSelection
	for _, sel := range s.selections {
		if sel.CommitmentID == cmt.ID {
			result = append(result, sel)
		}
	}
	return result, nil
}

// ─── Lock commitments ─────────────────────────────────────────────────────────

// LockCommitments closes the commitment window and locks all commitments.
// Transitions meet to commitment_closed and all commitments to locked.
func (s *Service) LockCommitments(principal *appmiddleware.Principal, orgID, meetID string) (Meet, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return Meet{}, MeetAuditEvent{}, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Meet{}, MeetAuditEvent{}, ErrMeetNotFound
	}

	if err := validateMeetTransition(meet.Status, "commitment_closed"); err != nil {
		return Meet{}, MeetAuditEvent{}, err
	}

	now := time.Now().UTC()
	meet.Status = "commitment_closed"
	meet.CommitmentClosedAt = &now
	meet.UpdatedAt = now
	meet.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertMeet(context.Background(), meet); err != nil {
			return Meet{}, MeetAuditEvent{}, fmt.Errorf("persist meet (lock commitments): %w", err)
		}
	}
	s.meets[meet.ID] = meet

	// Lock all commitments for this meet.
	lockedCount := 0
	for id, cmt := range s.commitments {
		if cmt.MeetID != meet.ID {
			continue
		}
		cmt.Status = "locked"
		cmt.LockedAt = &now
		cmt.UpdatedAt = now
		cmt.UpdatedBy = principal.UserID
		if s.repo != nil {
			if err := s.repo.UpsertCommitment(context.Background(), cmt); err != nil {
				return Meet{}, MeetAuditEvent{}, fmt.Errorf("persist commitment (lock): %w", err)
			}
		}
		s.commitments[id] = cmt
		lockedCount++
	}

	event := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "meet.commitments_locked",
		After:       map[string]any{"status": "commitment_closed", "locked_count": lockedCount},
		Metadata:    map[string]any{"meet_id": meet.ID},
	})
	return meet, event, nil
}

// ─── Entries ──────────────────────────────────────────────────────────────────

// ListEntries returns all entries for a meet.
func (s *Service) ListEntries(principal *appmiddleware.Principal, orgID, meetID string) ([]Entry, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach", "parent", "read-only") {
		return nil, ErrForbidden
	}
	if err := s.syncFromDB(context.Background()); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return nil, ErrMeetNotFound
	}

	var result []Entry
	for _, e := range s.entries {
		if e.MeetID == meet.ID {
			result = append(result, e)
		}
	}

	// Parent scope: only see entries for swimmers in their own household.
	if hasAnyRole(principal, "parent") && !hasAnyRole(principal, "owner", "admin", "coach") {
		var filtered []Entry
		for _, e := range result {
			swimmer, ok := s.swimmers[e.SwimmerID]
			if ok && s.principalOwnsHouseholdLocked(principal, swimmer.HouseholdID) {
				filtered = append(filtered, e)
			}
		}
		result = filtered
	}

	return result, nil
}

// ProposeEntry creates a new entry for a swimmer in a meet event.
// Entries are created with status "submitted".
func (s *Service) ProposeEntry(principal *appmiddleware.Principal, orgID, meetID string, input ProposeEntryInput) (Entry, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return Entry{}, MeetAuditEvent{}, ErrForbidden
	}

	swimmerID := strings.TrimSpace(input.SwimmerID)
	meetEventID := strings.TrimSpace(input.MeetEventID)
	if swimmerID == "" || meetEventID == "" {
		return Entry{}, MeetAuditEvent{}, errors.New("swimmer_id and meet_event_id are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Entry{}, MeetAuditEvent{}, ErrMeetNotFound
	}

	// Entries can be proposed once commitments are closed or finalized; not during the open window.
	if meet.Status != "commitment_closed" && meet.Status != "entry_finalized" {
		return Entry{}, MeetAuditEvent{}, fmt.Errorf("%w: entries cannot be proposed when meet status is %q", ErrInvalidTransition, meet.Status)
	}

	// Validate meet event.
	ev, ok := s.events[meetEventID]
	if !ok || ev.MeetID != meet.ID {
		return Entry{}, MeetAuditEvent{}, ErrEventNotFound
	}

	// Enforce gender/age eligibility (relays are seeded per-leg and skip this).
	// A coach may knowingly override with a required reason (audit-logged below).
	eligibilityOverridden := false
	if !input.IsRelay && s.swimmerEligibility != nil {
		if sw, ok := s.swimmerEligibility(orgID, swimmerID); ok {
			if err := validateSwimmerEventEligibility(sw, ev, meet.StartDate); err != nil {
				if !input.OverrideEligibility {
					return Entry{}, MeetAuditEvent{}, err
				}
				if strings.TrimSpace(input.OverrideReason) == "" {
					return Entry{}, MeetAuditEvent{}, fmt.Errorf("%w: a reason is required to override eligibility", ErrIneligibleEntry)
				}
				eligibilityOverridden = true
			}
		}
	}

	// Validate optional commitment.
	commitmentID := strings.TrimSpace(input.CommitmentID)
	if commitmentID != "" {
		cmt, ok := s.commitments[commitmentID]
		if !ok || cmt.MeetID != meet.ID {
			return Entry{}, MeetAuditEvent{}, ErrCommitmentNotFound
		}
	}

	// Enforce unique entry per (meet, swimmer, event).
	for _, e := range s.entries {
		if e.MeetID == meet.ID && e.SwimmerID == swimmerID && e.MeetEventID == meetEventID {
			return Entry{}, MeetAuditEvent{}, ErrDuplicateEntry
		}
	}

	now := time.Now().UTC()
	entry := Entry{
		ID:           s.newID("ent_"),
		OrgID:        orgID,
		MeetID:       meet.ID,
		SwimmerID:    swimmerID,
		MeetEventID:  meetEventID,
		CommitmentID: commitmentID,
		SeedTimeMs:   input.SeedTimeMs,
		IsRelay:      input.IsRelay,
		Status:       "submitted",
		ProposedBy:   principal.UserID,
		CreatedAt:    now,
		UpdatedAt:    now,
		CreatedBy:    principal.UserID,
		UpdatedBy:    principal.UserID,
	}
	if s.repo != nil {
		if err := s.repo.UpsertEntry(context.Background(), entry); err != nil {
			return Entry{}, MeetAuditEvent{}, fmt.Errorf("persist entry (propose): %w", err)
		}
	}
	s.entries[entry.ID] = entry

	metadata := map[string]any{"meet_id": meet.ID, "entry_id": entry.ID}
	if eligibilityOverridden {
		metadata["eligibility_override"] = true
		metadata["eligibility_override_reason"] = strings.TrimSpace(input.OverrideReason)
	}
	event := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "entry.proposed",
		After:       entrySnapshot(entry),
		Metadata:    metadata,
	})
	return entry, event, nil
}

// OverrideEntry allows a coach to change the event on an existing entry.
// Captures a full before/after audit event with a required reason.
func (s *Service) OverrideEntry(principal *appmiddleware.Principal, orgID, meetID, entryID string, input OverrideEntryInput) (Entry, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return Entry{}, MeetAuditEvent{}, ErrForbidden
	}

	newMeetEventID := strings.TrimSpace(input.NewMeetEventID)
	reason := strings.TrimSpace(input.Reason)
	if newMeetEventID == "" {
		return Entry{}, MeetAuditEvent{}, errors.New("new_meet_event_id is required")
	}
	if reason == "" {
		return Entry{}, MeetAuditEvent{}, errors.New("reason is required for coach override")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Entry{}, MeetAuditEvent{}, ErrMeetNotFound
	}

	// Overrides are not permitted after the meet is finalized or closed.
	if meet.Status == "entry_finalized" || meet.Status == "completed" || meet.Status == "archived" {
		return Entry{}, MeetAuditEvent{}, fmt.Errorf("%w: entries cannot be overridden when meet status is %q", ErrInvalidTransition, meet.Status)
	}

	entry, ok := s.entries[strings.TrimSpace(entryID)]
	if !ok || entry.MeetID != meet.ID || entry.OrgID != orgID {
		return Entry{}, MeetAuditEvent{}, ErrEntryNotFound
	}

	// Only submitted entries can be overridden.
	if entry.Status != "submitted" && entry.Status != "draft" {
		return Entry{}, MeetAuditEvent{}, fmt.Errorf("%w: only draft or submitted entries can be overridden (current: %s)", ErrEntryNotEditable, entry.Status)
	}

	// Validate new meet event.
	newEvent, ok := s.events[newMeetEventID]
	if !ok || newEvent.MeetID != meet.ID {
		return Entry{}, MeetAuditEvent{}, ErrEventNotFound
	}

	// Re-check eligibility against the new event (skip relays).
	if !entry.IsRelay && s.swimmerEligibility != nil {
		if sw, ok := s.swimmerEligibility(orgID, entry.SwimmerID); ok {
			if err := validateSwimmerEventEligibility(sw, newEvent, meet.StartDate); err != nil {
				return Entry{}, MeetAuditEvent{}, err
			}
		}
	}

	// Check for duplicate: another entry for the same swimmer and new event already exists.
	for _, e := range s.entries {
		if e.MeetID == meet.ID && e.SwimmerID == entry.SwimmerID && e.MeetEventID == newMeetEventID && e.ID != entry.ID {
			return Entry{}, MeetAuditEvent{}, ErrDuplicateEntry
		}
	}

	// Capture before state for audit.
	before := entrySnapshot(entry)

	// Apply override.
	oldMeetEventID := entry.MeetEventID
	entry.MeetEventID = newMeetEventID
	entry.OverrideReason = reason
	if input.SeedTimeMs != nil {
		entry.SeedTimeMs = input.SeedTimeMs
	}
	entry.UpdatedAt = time.Now().UTC()
	entry.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertEntry(context.Background(), entry); err != nil {
			return Entry{}, MeetAuditEvent{}, fmt.Errorf("persist entry (override): %w", err)
		}
	}
	s.entries[entry.ID] = entry

	after := entrySnapshot(entry)

	auditEvent := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "entry.coach_override",
		Before:      before,
		After:       after,
		Metadata: map[string]any{
			"meet_id":           meet.ID,
			"entry_id":          entry.ID,
			"swimmer_id":        entry.SwimmerID,
			"old_meet_event_id": oldMeetEventID,
			"new_meet_event_id": newMeetEventID,
			"reason":            reason,
		},
	})
	return entry, auditEvent, nil
}

// ScratchEntry soft-scratches an entry: it is marked "scratched" (retained for
// audit/history) rather than deleted, so it is excluded from reports/exports but
// the record of it ever existing is preserved. Coaches scratch entries when a
// swimmer withdraws or a duplicate was created. Blocked once the meet is
// finalized/closed or the entry has already been exported.
func (s *Service) ScratchEntry(principal *appmiddleware.Principal, orgID, meetID, entryID, reason string) (Entry, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return Entry{}, MeetAuditEvent{}, ErrForbidden
	}
	reason = strings.TrimSpace(reason)

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Entry{}, MeetAuditEvent{}, ErrMeetNotFound
	}
	// Scratches are not permitted after the meet is finalized or closed.
	if meet.Status == "entry_finalized" || meet.Status == "completed" || meet.Status == "archived" {
		return Entry{}, MeetAuditEvent{}, fmt.Errorf("%w: entries cannot be scratched when meet status is %q", ErrInvalidTransition, meet.Status)
	}

	entry, ok := s.entries[strings.TrimSpace(entryID)]
	if !ok || entry.MeetID != meet.ID || entry.OrgID != orgID {
		return Entry{}, MeetAuditEvent{}, ErrEntryNotFound
	}
	if entry.Status == "scratched" {
		return Entry{}, MeetAuditEvent{}, ErrEntryAlreadyScratched
	}
	// Only entries that have not yet been exported/locked can be scratched.
	if entry.Status != "draft" && entry.Status != "submitted" && entry.Status != "approved" {
		return Entry{}, MeetAuditEvent{}, fmt.Errorf("%w: entries in status %q cannot be scratched", ErrEntryNotEditable, entry.Status)
	}

	before := entrySnapshot(entry)
	entry.Status = "scratched"
	entry.UpdatedAt = time.Now().UTC()
	entry.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertEntry(context.Background(), entry); err != nil {
			return Entry{}, MeetAuditEvent{}, fmt.Errorf("persist entry (scratch): %w", err)
		}
	}
	s.entries[entry.ID] = entry

	auditEvent := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "entry.scratched",
		Before:      before,
		After:       entrySnapshot(entry),
		Metadata: map[string]any{
			"meet_id":    meet.ID,
			"entry_id":   entry.ID,
			"swimmer_id": entry.SwimmerID,
			"reason":     reason,
		},
	})
	return entry, auditEvent, nil
}

// ApproveEntry moves an entry from submitted to approved.
func (s *Service) ApproveEntry(principal *appmiddleware.Principal, orgID, meetID, entryID string) (Entry, MeetAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return Entry{}, MeetAuditEvent{}, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meet, ok := s.meets[strings.TrimSpace(meetID)]
	if !ok || meet.OrgID != orgID {
		return Entry{}, MeetAuditEvent{}, ErrMeetNotFound
	}

	// Entries can only be approved when commitments are closed or entries are being finalized.
	if meet.Status != "commitment_closed" && meet.Status != "entry_finalized" {
		return Entry{}, MeetAuditEvent{}, fmt.Errorf("%w: entries can only be approved when meet is in commitment_closed or entry_finalized status (current: %s)", ErrInvalidTransition, meet.Status)
	}

	entry, ok := s.entries[strings.TrimSpace(entryID)]
	if !ok || entry.MeetID != meet.ID || entry.OrgID != orgID {
		return Entry{}, MeetAuditEvent{}, ErrEntryNotFound
	}

	if entry.Status != "submitted" {
		return Entry{}, MeetAuditEvent{}, fmt.Errorf("%w: only submitted entries can be approved (current: %s)", ErrEntryNotEditable, entry.Status)
	}

	now := time.Now().UTC()
	entry.Status = "approved"
	entry.ApprovedBy = principal.UserID
	entry.UpdatedAt = now
	entry.UpdatedBy = principal.UserID
	if s.repo != nil {
		if err := s.repo.UpsertEntry(context.Background(), entry); err != nil {
			return Entry{}, MeetAuditEvent{}, fmt.Errorf("persist entry (approve): %w", err)
		}
	}
	s.entries[entry.ID] = entry

	auditEvent := s.recordAuditEventLocked(MeetAuditEvent{
		OrgID:       orgID,
		ActorUserID: principal.UserID,
		Action:      "entry.approved",
		After:       entrySnapshot(entry),
		Metadata: map[string]any{
			"meet_id":       meet.ID,
			"entry_id":      entry.ID,
			"swimmer_id":    entry.SwimmerID,
			"meet_event_id": entry.MeetEventID,
			"approved_by":   principal.UserID,
		},
	})
	return entry, auditEvent, nil
}

// ─── Seed-time back-fill ──────────────────────────────────────────────────────

// UpdateSeedTimesForSwimmer updates SeedTimeMs on every draft or submitted
// entry for the given swimmer whose meet event matches the specified
// distance/stroke/course. It is called by the performance domain after a
// best-time recompute to keep future entry seed times in sync.
//
// stroke is supplied in performance-domain notation (free, back, breast, fly,
// im, free_relay, medley_relay); it is normalized to meets-domain notation
// before comparison with MeetEvent.Stroke values.
//
// Returns the number of entries updated.
func (s *Service) UpdateSeedTimesForSwimmer(orgID, swimmerID string, distance int, stroke, course string, newBestTimeMs *int) int {
	meetsStroke := normalizePerfStroke(stroke)

	s.mu.Lock()
	defer s.mu.Unlock()

	updated := 0
	now := time.Now().UTC()
	for id, e := range s.entries {
		if e.OrgID != orgID || e.SwimmerID != swimmerID {
			continue
		}
		if e.Status != "draft" && e.Status != "submitted" {
			continue
		}
		ev, ok := s.events[e.MeetEventID]
		if !ok {
			continue
		}
		if ev.Distance != distance || ev.Stroke != meetsStroke || ev.Course != course {
			continue
		}
		e.SeedTimeMs = newBestTimeMs
		e.UpdatedAt = now
		s.entries[id] = e
		updated++
	}
	return updated
}

// normalizePerfStroke maps performance-domain stroke abbreviations to the
// full stroke names used in the meets domain.
// Unknown values are returned unchanged.
func normalizePerfStroke(s string) string {
	switch s {
	case "free":
		return "freestyle"
	case "back":
		return "backstroke"
	case "breast":
		return "breaststroke"
	case "fly":
		return "butterfly"
	case "im":
		return "individual_medley"
	case "free_relay":
		return "freestyle_relay"
	case "medley_relay":
		return "medley_relay"
	default:
		return s
	}
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (s *Service) parentOwnsHouseholdLocked(parentUserID, householdID string) bool {
	households := s.parentHousehold[parentUserID]
	_, ok := households[householdID]
	return ok
}

func (s *Service) principalOwnsHouseholdLocked(principal *appmiddleware.Principal, householdID string) bool {
	if principal == nil {
		return false
	}
	if strings.TrimSpace(principal.HouseholdID) != "" && strings.TrimSpace(principal.HouseholdID) == strings.TrimSpace(householdID) {
		return true
	}
	return s.parentOwnsHouseholdLocked(principal.UserID, householdID)
}

func (s *Service) recordAuditEventLocked(event MeetAuditEvent) MeetAuditEvent {
	event.ID = "aud_" + randomToken(12)
	event.CreatedAt = time.Now().UTC()
	s.auditEvents = append(s.auditEvents, event)
	return event
}

// SnapshotMeets returns a copy of all meets for the given org.
func (s *Service) SnapshotMeets(orgID string) []Meet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Meet, 0, len(s.meets))
	for _, meet := range s.meets {
		if meet.OrgID != orgID {
			continue
		}
		out = append(out, meet)
	}
	return out
}

// ─── Validation helpers ───────────────────────────────────────────────────────

var meetTransitions = map[string][]string{
	"draft":             {"published"},
	"published":         {"commitment_open", "draft"},
	"commitment_open":   {"commitment_closed"},
	"commitment_closed": {"entry_finalized"},
	"entry_finalized":   {"completed"},
	"completed":         {"archived"},
}

func validateMeetTransition(from, to string) error {
	allowed, ok := meetTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown current status %q", ErrInvalidTransition, from)
	}
	if !slices.Contains(allowed, to) {
		return fmt.Errorf("%w: %s → %s is not allowed", ErrInvalidTransition, from, to)
	}
	return nil
}

func validateMeetType(t string) error {
	allowed := []string{"invitational", "dual_meet", "time_trial", "championship", "other"}
	if !slices.Contains(allowed, t) {
		return fmt.Errorf("meet_type must be one of: %s", strings.Join(allowed, ", "))
	}
	return nil
}

func validateCourse(c string) error {
	allowed := []string{"SCY", "SCM", "LCM"}
	if !slices.Contains(allowed, c) {
		return fmt.Errorf("course must be one of: %s", strings.Join(allowed, ", "))
	}
	return nil
}

func validateStroke(s string) error {
	allowed := []string{"freestyle", "backstroke", "breaststroke", "butterfly", "individual_medley", "medley_relay", "freestyle_relay", "other"}
	if !slices.Contains(allowed, s) {
		return fmt.Errorf("stroke must be one of: %s", strings.Join(allowed, ", "))
	}
	return nil
}

func validateGender(g string) error {
	allowed := []string{"M", "F", "X", "O"}
	if !slices.Contains(allowed, g) {
		return fmt.Errorf("gender must be one of: M, F, X, O")
	}
	return nil
}

func normalizeChoice(input, defaultVal string) string {
	v := strings.TrimSpace(input)
	if v == "" {
		return defaultVal
	}
	return v
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

func randomToken(byteLen int) string {
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Errorf("secure random unavailable for meet token generation: %w", err))
	}
	return hex.EncodeToString(raw)
}

func meetSnapshot(m Meet) map[string]any {
	return map[string]any{
		"name":      m.Name,
		"meet_type": m.MeetType,
		"course":    m.Course,
		"status":    m.Status,
		"location":  m.Location,
	}
}

func entrySnapshot(e Entry) map[string]any {
	return map[string]any{
		"swimmer_id":      e.SwimmerID,
		"meet_event_id":   e.MeetEventID,
		"seed_time_ms":    e.SeedTimeMs,
		"status":          e.Status,
		"override_reason": e.OverrideReason,
	}
}
