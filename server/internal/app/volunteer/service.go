// Package volunteer manages volunteer job slots (meet and club-event shifts),
// household signups, and per-season service-hour requirements with buyouts.
//
// Persistence follows the domain pattern: NewService() is in-memory only and
// NewServiceWithPool(pool) is Postgres-backed. In DB mode every read and
// write goes straight to Postgres (no process-local cache), so multiple API
// instances always agree, and signup capacity is enforced with a row lock on
// the job so concurrent requests can never overbook a slot.
package volunteer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Errors ───────────────────────────────────────────────────────────────────

var (
	ErrForbidden             = errors.New("volunteer: forbidden")
	ErrJobNotFound           = errors.New("volunteer job not found")
	ErrSignupNotFound        = errors.New("volunteer signup not found")
	ErrRequirementNotFound   = errors.New("volunteer requirement not found")
	ErrSeasonNotFound        = errors.New("season not found")
	ErrMeetNotFound          = errors.New("meet not found")
	ErrHouseholdNotFound     = errors.New("household not found")
	ErrJobFull               = errors.New("volunteer job is full")
	ErrDuplicateSignup       = errors.New("this volunteer already holds a spot on this job")
	ErrJobStarted            = errors.New("volunteer job has already started")
	ErrCancellationClosed    = errors.New("cancellation window has closed for this job")
	ErrSignupNotActive       = errors.New("signup is already cancelled")
	ErrInvalidTransition     = errors.New("invalid signup status transition")
	ErrJobHasWorkedSignups   = errors.New("job has volunteers marked worked; hours already credited")
	ErrCapacityBelowFilled   = errors.New("capacity cannot be lower than the number of filled spots")
	ErrCertificationRequired = errors.New("this job requires a certified volunteer")
	ErrAgeRequirement        = errors.New("volunteer does not meet the minimum age for this job")
	ErrBuyoutExists          = errors.New("a buyout has already been billed for this household and season")
	ErrBuyoutNotFound        = errors.New("buyout not found")
	ErrNothingOwed           = errors.New("household has no remaining hours to buy out")
	ErrNoBuyoutRate          = errors.New("no buyout rate is configured for this season")
	ErrBillingUnavailable    = errors.New("billing service is not configured")
)

// ValidationError reports a bad input value; handlers map it to 422.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// ─── Vocabulary ───────────────────────────────────────────────────────────────

// Roles is the closed set of job roles.
var Roles = []string{"timer", "concessions", "marshal", "official", "hospitality", "setup", "admin", "other"}

var validRoles = func() map[string]bool {
	m := make(map[string]bool, len(Roles))
	for _, r := range Roles {
		m[r] = true
	}
	return m
}()

const (
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
	StatusWorked    = "worked"
	StatusNoShow    = "no_show"
)

// occupiesSpot reports whether a signup in this status counts toward capacity.
func occupiesSpot(status string) bool {
	return status == StatusConfirmed || status == StatusWorked || status == StatusNoShow
}

// Ledger compliance statuses.
const (
	ComplianceExempt    = "exempt"
	ComplianceMet       = "met"
	ComplianceOnTrack   = "on_track"
	ComplianceShort     = "short"
	ComplianceBoughtOut = "bought_out"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// Settings are the org-level volunteer defaults.
type Settings struct {
	OrgID                     string     `json:"org_id"`
	DefaultRequiredHours      float64    `json:"default_required_hours"`
	DefaultBuyoutCentsPerHour int        `json:"default_buyout_cents_per_hour"`
	CancellationCutoffHours   int        `json:"cancellation_cutoff_hours"`
	RemindersEnabled          bool       `json:"reminders_enabled"`
	Timezone                  string     `json:"timezone"` // IANA name used to render reminder times
	UpdatedAt                 *time.Time `json:"updated_at,omitempty"`
	UpdatedBy                 string     `json:"updated_by,omitempty"`
}

// DefaultSettings returns the settings used before an admin configures any.
func DefaultSettings(orgID string) *Settings {
	return &Settings{OrgID: orgID, CancellationCutoffHours: 48, RemindersEnabled: true, Timezone: "UTC"}
}

// Job is one volunteer slot with Capacity spots.
type Job struct {
	ID                    string    `json:"id"`
	OrgID                 string    `json:"org_id"`
	MeetID                string    `json:"meet_id,omitempty"`
	EventName             string    `json:"event_name,omitempty"`
	SeasonID              string    `json:"season_id,omitempty"`
	Role                  string    `json:"role"`
	Title                 string    `json:"title"`
	Description           string    `json:"description"`
	Location              string    `json:"location"`
	StartsAt              time.Time `json:"starts_at"`
	EndsAt                time.Time `json:"ends_at"`
	Capacity              int       `json:"capacity"`
	HourCredit            float64   `json:"hour_credit"`
	MinAge                int       `json:"min_age,omitempty"`
	RequiresCertification bool      `json:"requires_certification"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	CreatedBy             string    `json:"created_by"`
	UpdatedBy             string    `json:"updated_by"`
}

// JobView decorates a Job with fill state and display context.
type JobView struct {
	Job
	MeetName          string       `json:"meet_name,omitempty"`
	EffectiveSeasonID string       `json:"effective_season_id,omitempty"`
	FilledCount       int          `json:"filled_count"`
	OpenSpots         int          `json:"open_spots"`
	Signups           []SignupView `json:"signups,omitempty"`
}

// Signup is one household member holding one spot on a job.
type Signup struct {
	ID            string     `json:"id"`
	OrgID         string     `json:"org_id"`
	JobID         string     `json:"job_id"`
	HouseholdID   string     `json:"household_id"`
	VolunteerName string     `json:"volunteer_name"`
	SwimmerID     string     `json:"swimmer_id,omitempty"`
	Status        string     `json:"status"`
	HoursCredited *float64   `json:"hours_credited,omitempty"`
	Notes         string     `json:"notes"`
	SignedUpBy    string     `json:"signed_up_by"`
	CancelledAt   *time.Time `json:"cancelled_at,omitempty"`
	CancelledBy   string     `json:"cancelled_by,omitempty"`
	CheckedInAt   *time.Time `json:"checked_in_at,omitempty"`
	CheckedInBy   string     `json:"checked_in_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// SignupView decorates a Signup with its job and credit.
type SignupView struct {
	Signup
	HouseholdName  string  `json:"household_name,omitempty"`
	EffectiveHours float64 `json:"effective_hours"`
	CanCancel      bool    `json:"can_cancel"`
	Job            *Job    `json:"job,omitempty"`
}

// Requirement is a season default (HouseholdID == "") or a household override.
type Requirement struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"org_id"`
	SeasonID           string    `json:"season_id"`
	HouseholdID        string    `json:"household_id,omitempty"`
	RequiredHours      float64   `json:"required_hours"`
	BuyoutCentsPerHour int       `json:"buyout_cents_per_hour"`
	Notes              string    `json:"notes"`
	UpdatedAt          time.Time `json:"updated_at"`
	UpdatedBy          string    `json:"updated_by"`
}

// Credit is a manual hour adjustment made by staff.
type Credit struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	SeasonID    string    `json:"season_id"`
	HouseholdID string    `json:"household_id"`
	Hours       float64   `json:"hours"`
	Reason      string    `json:"reason"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// Buyout records that a household's missing hours were billed.
type Buyout struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	SeasonID         string    `json:"season_id"`
	HouseholdID      string    `json:"household_id"`
	Status           string    `json:"status"` // pending | billed
	HoursShort       float64   `json:"hours_short"`
	RateCentsPerHour int       `json:"rate_cents_per_hour"`
	AmountCents      int       `json:"amount_cents"`
	InvoiceID        string    `json:"invoice_id,omitempty"`
	InvoiceItemID    string    `json:"invoice_item_id,omitempty"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// LedgerRow is one household's hours position for one season.
type LedgerRow struct {
	SeasonID           string  `json:"season_id"`
	SeasonName         string  `json:"season_name"`
	SeasonStatus       string  `json:"season_status"`
	SeasonStartDate    string  `json:"season_start_date,omitempty"`
	SeasonEndDate      string  `json:"season_end_date,omitempty"`
	HouseholdID        string  `json:"household_id"`
	HouseholdName      string  `json:"household_name"`
	RequiredHours      float64 `json:"required_hours"`
	WorkedHours        float64 `json:"worked_hours"`
	CreditedHours      float64 `json:"credited_hours"`
	EarnedHours        float64 `json:"earned_hours"`
	ScheduledHours     float64 `json:"scheduled_hours"`
	RemainingHours     float64 `json:"remaining_hours"`
	BuyoutCentsPerHour int     `json:"buyout_cents_per_hour"`
	BuyoutDueCents     int     `json:"buyout_due_cents"`
	RequirementSource  string  `json:"requirement_source"` // household | season | org_default
	Status             string  `json:"status"`
	Buyout             *Buyout `json:"buyout,omitempty"`
}

// ─── Cross-domain refs (wired in server.go) ───────────────────────────────────

// SeasonRef is the minimal season data needed for hour attribution.
type SeasonRef struct {
	ID        string
	Name      string
	Status    string
	StartDate string // YYYY-MM-DD or ""
	EndDate   string
}

// HouseholdRef is the minimal household data needed for ledgers/reminders.
type HouseholdRef struct {
	ID     string
	Name   string
	Email  string
	Status string
}

// SwimmerRef is used to validate swimmer volunteers (teen helpers).
type SwimmerRef struct {
	HouseholdID string
	DateOfBirth string
	Name        string
}

// BuyoutInvoiceRequest asks the billing domain to bill a buyout.
type BuyoutInvoiceRequest struct {
	OrgID       string
	HouseholdID string
	Description string
	AmountCents int
	DueDate     string
	Issue       bool
	Principal   *appmiddleware.Principal
}

// BuyoutInvoiceResult identifies the billing records that were created.
type BuyoutInvoiceResult struct {
	InvoiceID     string `json:"invoice_id"`
	InvoiceItemID string `json:"invoice_item_id"`
	InvoiceNumber string `json:"invoice_number"`
	InvoiceStatus string `json:"invoice_status"`
}

// Biller creates buyout invoices through the existing billing service.
type Biller interface {
	CreateBuyoutInvoice(ctx context.Context, req BuyoutInvoiceRequest) (BuyoutInvoiceResult, error)
}

// ReminderEmail is a rendered shift reminder.
type ReminderEmail struct {
	OrgID    string
	To       string
	Subject  string
	TextBody string
}

// Enqueuer is the subset of workers.Queue used for reminders.
type Enqueuer interface {
	EnqueueIn(ctx context.Context, delay time.Duration, task *workers.Task, opts ...workers.Option) (*workers.TaskInfo, error)
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service implements the volunteer domain.
type Service struct {
	store  store
	logger *slog.Logger
	now    func() time.Time

	seasons    func(orgID string) ([]SeasonRef, error)
	households func(orgID string) ([]HouseholdRef, error)
	swimmer    func(orgID, swimmerID string) (SwimmerRef, bool)
	meetName   func(orgID, meetID string) (string, bool)
	biller     Biller
	queue      Enqueuer
	sendEmail  func(ctx context.Context, email ReminderEmail) error
}

// NewService returns an in-memory Service.
func NewService() *Service {
	return &Service{store: newMemoryStore(), logger: slog.Default(), now: func() time.Time { return time.Now().UTC() }}
}

// NewServiceWithPool returns a Postgres-backed Service.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool != nil {
		s.store = newRepository(pool)
	}
	return s
}

// SetLogger overrides the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// SetClock overrides the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// SetSeasonLookup wires the seasons domain.
func (s *Service) SetSeasonLookup(fn func(orgID string) ([]SeasonRef, error)) { s.seasons = fn }

// SetHouseholdDirectory wires the members domain household list.
func (s *Service) SetHouseholdDirectory(fn func(orgID string) ([]HouseholdRef, error)) {
	s.households = fn
}

// SetSwimmerLookup wires swimmer household/DOB lookup.
func (s *Service) SetSwimmerLookup(fn func(orgID, swimmerID string) (SwimmerRef, bool)) {
	s.swimmer = fn
}

// SetMeetLookup wires meet existence/name lookup.
func (s *Service) SetMeetLookup(fn func(orgID, meetID string) (string, bool)) { s.meetName = fn }

// SetBiller wires the billing adapter used for buyouts.
func (s *Service) SetBiller(b Biller) { s.biller = b }

// SetReminderQueue wires the background job queue for shift reminders.
func (s *Service) SetReminderQueue(q Enqueuer) { s.queue = q }

// SetReminderSender wires the email transport used by the reminder job.
func (s *Service) SetReminderSender(fn func(ctx context.Context, email ReminderEmail) error) {
	s.sendEmail = fn
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ─── Role helpers ─────────────────────────────────────────────────────────────

func hasRole(p *appmiddleware.Principal, roles ...string) bool {
	if p == nil {
		return false
	}
	for _, have := range p.Roles {
		for _, want := range roles {
			if strings.EqualFold(strings.TrimSpace(have), want) {
				return true
			}
		}
	}
	return false
}

func isStaff(p *appmiddleware.Principal) bool { return hasRole(p, "admin", "coach") }

// householdScope returns the household a non-staff principal is limited to.
// Staff get ("", true). Unknown roles or a missing household are denied.
func householdScope(p *appmiddleware.Principal) (string, error) {
	if isStaff(p) {
		return "", nil
	}
	if hasRole(p, "parent", "swimmer") {
		hh := strings.TrimSpace(p.HouseholdID)
		if hh == "" {
			return "", ErrForbidden
		}
		return hh, nil
	}
	return "", ErrForbidden
}

// ─── Settings ─────────────────────────────────────────────────────────────────

// GetSettings returns org settings (defaults when unset). Staff only.
func (s *Service) GetSettings(ctx context.Context, p *appmiddleware.Principal, orgID string) (*Settings, error) {
	if !isStaff(p) {
		return nil, ErrForbidden
	}
	return s.settings(ctx, orgID)
}

func (s *Service) settings(ctx context.Context, orgID string) (*Settings, error) {
	st, found, err := s.store.getSettings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if !found {
		return DefaultSettings(orgID), nil
	}
	return st, nil
}

// SettingsUpdate carries optional settings fields.
type SettingsUpdate struct {
	DefaultRequiredHours      *float64
	DefaultBuyoutCentsPerHour *int
	CancellationCutoffHours   *int
	RemindersEnabled          *bool
	Timezone                  *string
}

// UpdateSettings patches org settings. Admin only.
func (s *Service) UpdateSettings(ctx context.Context, p *appmiddleware.Principal, orgID string, in SettingsUpdate) (*Settings, error) {
	if !hasRole(p, "admin") {
		return nil, ErrForbidden
	}
	st, err := s.settings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if in.DefaultRequiredHours != nil {
		if *in.DefaultRequiredHours < 0 || *in.DefaultRequiredHours > 1000 {
			return nil, invalid("default_required_hours", "must be between 0 and 1000")
		}
		st.DefaultRequiredHours = round2(*in.DefaultRequiredHours)
	}
	if in.DefaultBuyoutCentsPerHour != nil {
		if *in.DefaultBuyoutCentsPerHour < 0 || *in.DefaultBuyoutCentsPerHour > 100000 {
			return nil, invalid("default_buyout_cents_per_hour", "must be between 0 and 100000")
		}
		st.DefaultBuyoutCentsPerHour = *in.DefaultBuyoutCentsPerHour
	}
	if in.CancellationCutoffHours != nil {
		if *in.CancellationCutoffHours < 0 || *in.CancellationCutoffHours > 720 {
			return nil, invalid("cancellation_cutoff_hours", "must be between 0 and 720")
		}
		st.CancellationCutoffHours = *in.CancellationCutoffHours
	}
	if in.RemindersEnabled != nil {
		st.RemindersEnabled = *in.RemindersEnabled
	}
	if in.Timezone != nil {
		tz := strings.TrimSpace(*in.Timezone)
		if _, err := time.LoadLocation(tz); err != nil || tz == "" || tz == "Local" {
			return nil, invalid("timezone", "must be an IANA time zone such as America/Los_Angeles")
		}
		st.Timezone = tz
	}
	now := s.now()
	st.OrgID = orgID
	st.UpdatedAt = &now
	st.UpdatedBy = p.UserID
	if err := s.store.upsertSettings(ctx, st); err != nil {
		return nil, err
	}
	return st, nil
}

// ─── Jobs ─────────────────────────────────────────────────────────────────────

// JobInput is the full set of writable job fields.
type JobInput struct {
	MeetID                string
	EventName             string
	SeasonID              string
	Role                  string
	Title                 string
	Description           string
	Location              string
	StartsAt              time.Time
	EndsAt                time.Time
	Capacity              int
	HourCredit            *float64 // nil → derived from shift length
	MinAge                int
	RequiresCertification bool
}

func (s *Service) validateJob(orgID string, in *JobInput, checkMeet bool) error {
	in.MeetID = strings.TrimSpace(in.MeetID)
	in.EventName = strings.TrimSpace(in.EventName)
	in.SeasonID = strings.TrimSpace(in.SeasonID)
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Location = strings.TrimSpace(in.Location)
	if in.MeetID == "" && in.EventName == "" {
		return invalid("event_name", "a job needs a meet_id or an event_name")
	}
	if len(in.EventName) > 160 {
		return invalid("event_name", "must be at most 160 characters")
	}
	if !validRoles[in.Role] {
		return invalid("role", "must be one of "+strings.Join(Roles, ", "))
	}
	if in.Title == "" || len(in.Title) > 120 {
		return invalid("title", "is required (max 120 characters)")
	}
	if len(in.Description) > 2000 {
		return invalid("description", "must be at most 2000 characters")
	}
	if len(in.Location) > 200 {
		return invalid("location", "must be at most 200 characters")
	}
	if in.StartsAt.IsZero() || in.EndsAt.IsZero() {
		return invalid("starts_at", "starts_at and ends_at are required")
	}
	if !in.EndsAt.After(in.StartsAt) {
		return invalid("ends_at", "must be after starts_at")
	}
	if in.EndsAt.Sub(in.StartsAt) > 24*time.Hour {
		return invalid("ends_at", "a shift cannot be longer than 24 hours")
	}
	if in.Capacity < 1 || in.Capacity > 500 {
		return invalid("capacity", "must be between 1 and 500")
	}
	if in.HourCredit == nil {
		h := shiftHours(in.StartsAt, in.EndsAt)
		in.HourCredit = &h
	}
	if *in.HourCredit < 0 || *in.HourCredit > 24 {
		return invalid("hour_credit", "must be between 0 and 24")
	}
	if in.MinAge < 0 || in.MinAge > 99 {
		return invalid("min_age", "must be between 0 and 99")
	}
	if checkMeet && in.MeetID != "" && s.meetName != nil {
		if _, ok := s.meetName(orgID, in.MeetID); !ok {
			return ErrMeetNotFound
		}
	}
	if in.SeasonID != "" && s.seasons != nil {
		if _, ok := s.findSeason(orgID, in.SeasonID); !ok {
			return ErrSeasonNotFound
		}
	}
	return nil
}

// shiftHours returns the shift length rounded to the nearest quarter hour.
func shiftHours(start, end time.Time) float64 {
	return math.Round(end.Sub(start).Hours()*4) / 4
}

func (s *Service) findSeason(orgID, seasonID string) (SeasonRef, bool) {
	if s.seasons == nil {
		return SeasonRef{ID: seasonID, Name: "Season"}, true
	}
	list, err := s.seasons(orgID)
	if err != nil {
		return SeasonRef{}, false
	}
	for _, se := range list {
		if se.ID == seasonID {
			return se, true
		}
	}
	return SeasonRef{}, false
}

// CreateJob creates one job. Admin/coach.
func (s *Service) CreateJob(ctx context.Context, p *appmiddleware.Principal, orgID string, in JobInput) (*Job, error) {
	if !isStaff(p) {
		return nil, ErrForbidden
	}
	if err := s.validateJob(orgID, &in, true); err != nil {
		return nil, err
	}
	job := s.newJob(p, orgID, in)
	if err := s.store.insertJobs(ctx, []*Job{job}); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Service) newJob(p *appmiddleware.Principal, orgID string, in JobInput) *Job {
	now := s.now()
	return &Job{
		ID:                    uuid.NewString(),
		OrgID:                 orgID,
		MeetID:                in.MeetID,
		EventName:             in.EventName,
		SeasonID:              in.SeasonID,
		Role:                  in.Role,
		Title:                 in.Title,
		Description:           in.Description,
		Location:              in.Location,
		StartsAt:              in.StartsAt.UTC(),
		EndsAt:                in.EndsAt.UTC(),
		Capacity:              in.Capacity,
		HourCredit:            round2(*in.HourCredit),
		MinAge:                in.MinAge,
		RequiresCertification: in.RequiresCertification,
		CreatedAt:             now,
		UpdatedAt:             now,
		CreatedBy:             p.UserID,
		UpdatedBy:             p.UserID,
	}
}

// JobUpdate carries optional job fields.
type JobUpdate struct {
	EventName             *string
	SeasonID              *string
	Role                  *string
	Title                 *string
	Description           *string
	Location              *string
	StartsAt              *time.Time
	EndsAt                *time.Time
	Capacity              *int
	HourCredit            *float64
	MinAge                *int
	RequiresCertification *bool
}

// UpdateJob patches a job. Admin/coach. Capacity cannot drop below filled
// spots; a time change reschedules reminders for confirmed volunteers.
func (s *Service) UpdateJob(ctx context.Context, p *appmiddleware.Principal, orgID, jobID string, up JobUpdate) (*Job, *Job, error) {
	if !isStaff(p) {
		return nil, nil, ErrForbidden
	}
	job, err := s.store.getJob(ctx, orgID, jobID)
	if err != nil {
		return nil, nil, err
	}
	before := *job
	in := JobInput{
		MeetID: job.MeetID, EventName: job.EventName, SeasonID: job.SeasonID, Role: job.Role, Title: job.Title,
		Description: job.Description, Location: job.Location, StartsAt: job.StartsAt, EndsAt: job.EndsAt,
		Capacity: job.Capacity, MinAge: job.MinAge, RequiresCertification: job.RequiresCertification,
	}
	hc := job.HourCredit
	in.HourCredit = &hc
	if up.EventName != nil {
		in.EventName = *up.EventName
	}
	if up.SeasonID != nil {
		in.SeasonID = *up.SeasonID
	}
	if up.Role != nil {
		in.Role = *up.Role
	}
	if up.Title != nil {
		in.Title = *up.Title
	}
	if up.Description != nil {
		in.Description = *up.Description
	}
	if up.Location != nil {
		in.Location = *up.Location
	}
	if up.StartsAt != nil {
		in.StartsAt = *up.StartsAt
	}
	if up.EndsAt != nil {
		in.EndsAt = *up.EndsAt
	}
	if up.Capacity != nil {
		in.Capacity = *up.Capacity
	}
	if up.HourCredit != nil {
		in.HourCredit = up.HourCredit
	}
	if up.MinAge != nil {
		in.MinAge = *up.MinAge
	}
	if up.RequiresCertification != nil {
		in.RequiresCertification = *up.RequiresCertification
	}
	// The meet anchor is immutable; skip the meet lookup on update.
	if err := s.validateJob(orgID, &in, false); err != nil {
		return nil, nil, err
	}
	signups, err := s.store.listSignups(ctx, orgID, SignupFilter{JobID: jobID})
	if err != nil {
		return nil, nil, err
	}
	filled := 0
	for _, su := range signups {
		if occupiesSpot(su.Status) {
			filled++
		}
	}
	if in.Capacity < filled {
		return nil, nil, ErrCapacityBelowFilled
	}
	job.EventName, job.SeasonID, job.Role, job.Title = in.EventName, in.SeasonID, in.Role, in.Title
	job.Description, job.Location = in.Description, in.Location
	job.StartsAt, job.EndsAt = in.StartsAt.UTC(), in.EndsAt.UTC()
	job.Capacity, job.HourCredit, job.MinAge = in.Capacity, round2(*in.HourCredit), in.MinAge
	job.RequiresCertification = in.RequiresCertification
	job.UpdatedAt = s.now()
	job.UpdatedBy = p.UserID
	if err := s.store.updateJob(ctx, job); err != nil {
		return nil, nil, err
	}
	if !before.StartsAt.Equal(job.StartsAt) {
		for _, su := range signups {
			if su.Status == StatusConfirmed {
				s.scheduleReminder(ctx, job, su)
			}
		}
	}
	return &before, job, nil
}

// DeleteJob removes a job and its signups. Refused when hours were credited.
func (s *Service) DeleteJob(ctx context.Context, p *appmiddleware.Principal, orgID, jobID string) (*Job, error) {
	if !isStaff(p) {
		return nil, ErrForbidden
	}
	job, err := s.store.getJob(ctx, orgID, jobID)
	if err != nil {
		return nil, err
	}
	signups, err := s.store.listSignups(ctx, orgID, SignupFilter{JobID: jobID})
	if err != nil {
		return nil, err
	}
	for _, su := range signups {
		if su.Status == StatusWorked {
			return nil, ErrJobHasWorkedSignups
		}
	}
	if err := s.store.deleteJob(ctx, orgID, jobID); err != nil {
		return nil, err
	}
	return job, nil
}

// JobFilter narrows job listings.
type JobFilter struct {
	MeetID   string
	SeasonID string // effective season (explicit or by date)
	From     time.Time
	To       time.Time
	Upcoming bool // ends_at >= now
}

// ListJobs lists jobs with fill counts. Every role may browse jobs; only staff
// see the per-job volunteer roster (via GetJob).
func (s *Service) ListJobs(ctx context.Context, p *appmiddleware.Principal, orgID string, f JobFilter) ([]JobView, error) {
	if _, err := householdScope(p); err != nil {
		return nil, err
	}
	jobs, err := s.store.listJobs(ctx, orgID, f.MeetID)
	if err != nil {
		return nil, err
	}
	seasons := s.seasonList(orgID)
	signups, err := s.store.listSignups(ctx, orgID, SignupFilter{})
	if err != nil {
		return nil, err
	}
	filled := map[string]int{}
	for _, su := range signups {
		if occupiesSpot(su.Status) {
			filled[su.JobID]++
		}
	}
	now := s.now()
	meetNames := map[string]string{}
	out := make([]JobView, 0, len(jobs))
	for _, job := range jobs {
		if f.Upcoming && job.EndsAt.Before(now) {
			continue
		}
		if !f.From.IsZero() && job.StartsAt.Before(f.From) {
			continue
		}
		if !f.To.IsZero() && !job.StartsAt.Before(f.To) {
			continue
		}
		eff := effectiveSeason(job, seasons)
		if f.SeasonID != "" && eff != f.SeasonID {
			continue
		}
		out = append(out, s.jobView(job, eff, filled[job.ID], meetNames))
	}
	return out, nil
}

func (s *Service) jobView(job *Job, effSeason string, filled int, meetNames map[string]string) JobView {
	v := JobView{Job: *job, EffectiveSeasonID: effSeason, FilledCount: filled}
	v.OpenSpots = job.Capacity - filled
	if v.OpenSpots < 0 {
		v.OpenSpots = 0
	}
	if job.MeetID != "" && s.meetName != nil {
		name, cached := meetNames[job.MeetID]
		if !cached {
			name, _ = s.meetName(job.OrgID, job.MeetID)
			meetNames[job.MeetID] = name
		}
		v.MeetName = name
	}
	return v
}

// GetJob returns one job. Staff also receive the roster of live and
// cancelled signups.
func (s *Service) GetJob(ctx context.Context, p *appmiddleware.Principal, orgID, jobID string) (*JobView, error) {
	if _, err := householdScope(p); err != nil {
		return nil, err
	}
	job, err := s.store.getJob(ctx, orgID, jobID)
	if err != nil {
		return nil, err
	}
	signups, err := s.store.listSignups(ctx, orgID, SignupFilter{JobID: jobID})
	if err != nil {
		return nil, err
	}
	filled := 0
	for _, su := range signups {
		if occupiesSpot(su.Status) {
			filled++
		}
	}
	v := s.jobView(job, effectiveSeason(job, s.seasonList(orgID)), filled, map[string]string{})
	if isStaff(p) {
		names := s.householdNames(orgID)
		settings, err := s.settings(ctx, orgID)
		if err != nil {
			return nil, err
		}
		v.Signups = make([]SignupView, 0, len(signups))
		for _, su := range signups {
			v.Signups = append(v.Signups, s.signupView(su, job, names, settings, p))
		}
	}
	return &v, nil
}

// ─── Templates / bulk create ──────────────────────────────────────────────────

// TemplateRole is one role line in a job template.
type TemplateRole struct {
	Role                  string   `json:"role"`
	Title                 string   `json:"title"`
	Description           string   `json:"description"`
	Capacity              int      `json:"capacity"`
	HourCredit            *float64 `json:"hour_credit,omitempty"`
	MinAge                int      `json:"min_age,omitempty"`
	RequiresCertification bool     `json:"requires_certification"`
}

// Template is a named set of roles stamped onto each shift.
type Template struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Roles []TemplateRole `json:"roles"`
}

// Templates returns the built-in job templates.
func Templates() []Template {
	return []Template{
		{
			ID:   "standard_meet",
			Name: "Standard home meet (per session)",
			Roles: []TemplateRole{
				{Role: "timer", Title: "Timer", Capacity: 16, Description: "Run a lane stopwatch and backup button. Two timers per lane; briefing 15 minutes before warm-up ends."},
				{Role: "official", Title: "Stroke & Turn Official", Capacity: 4, MinAge: 18, RequiresCertification: true, Description: "USA Swimming certified officials only."},
				{Role: "marshal", Title: "Deck Marshal", Capacity: 2, MinAge: 18, Description: "Keep warm-up lanes and the deck safe and orderly."},
				{Role: "admin", Title: "Clerk of Course / Runner", Capacity: 2, Description: "Collect heat sheets, run results to the computer table, post results."},
				{Role: "concessions", Title: "Concessions", Capacity: 4, Description: "Staff the snack stand and handle cash/card sales."},
				{Role: "hospitality", Title: "Hospitality", Capacity: 2, Description: "Keep the officials' and coaches' room stocked."},
				{Role: "setup", Title: "Setup & Teardown", Capacity: 4, Description: "Lane lines, backstroke flags, timing system, tents."},
			},
		},
		{
			ID:   "club_event",
			Name: "Club event / fundraiser",
			Roles: []TemplateRole{
				{Role: "setup", Title: "Setup crew", Capacity: 4, Description: "Arrive early to set up tables, signage, and supplies."},
				{Role: "hospitality", Title: "Check-in table", Capacity: 2, Description: "Greet guests and manage the sign-in sheet."},
				{Role: "other", Title: "Event helper", Capacity: 6, Description: "General help wherever the event lead needs it."},
				{Role: "setup", Title: "Cleanup crew", Capacity: 4, Description: "Stay after to break down and clean up."},
			},
		},
	}
}

// Shift is one time window that a template is stamped onto.
type Shift struct {
	StartsAt time.Time
	EndsAt   time.Time
	Label    string
}

// BulkInput describes a bulk job creation.
type BulkInput struct {
	MeetID     string
	EventName  string
	SeasonID   string
	Location   string
	TemplateID string
	Roles      []TemplateRole // overrides the template when non-empty
	Shifts     []Shift
}

// BulkCreateJobs stamps template roles onto every shift atomically.
func (s *Service) BulkCreateJobs(ctx context.Context, p *appmiddleware.Principal, orgID string, in BulkInput) ([]*Job, error) {
	if !isStaff(p) {
		return nil, ErrForbidden
	}
	roles := in.Roles
	if len(roles) == 0 {
		tid := strings.TrimSpace(in.TemplateID)
		if tid == "" {
			tid = "standard_meet"
		}
		for _, t := range Templates() {
			if t.ID == tid {
				roles = t.Roles
			}
		}
		if len(roles) == 0 {
			return nil, invalid("template", "unknown template")
		}
	}
	if len(in.Shifts) == 0 || len(in.Shifts) > 40 {
		return nil, invalid("shifts", "provide between 1 and 40 shifts")
	}
	if len(roles)*len(in.Shifts) > 500 {
		return nil, invalid("roles", "at most 500 jobs can be created at once")
	}
	jobs := make([]*Job, 0, len(roles)*len(in.Shifts))
	for i, sh := range in.Shifts {
		for _, r := range roles {
			title := strings.TrimSpace(r.Title)
			if label := strings.TrimSpace(sh.Label); label != "" && title != "" {
				title = title + " · " + label
			}
			ji := JobInput{
				MeetID: in.MeetID, EventName: in.EventName, SeasonID: in.SeasonID, Role: r.Role, Title: title,
				Description: r.Description, Location: in.Location, StartsAt: sh.StartsAt, EndsAt: sh.EndsAt,
				Capacity: r.Capacity, HourCredit: r.HourCredit, MinAge: r.MinAge, RequiresCertification: r.RequiresCertification,
			}
			if err := s.validateJob(orgID, &ji, i == 0); err != nil {
				var ve *ValidationError
				if errors.As(err, &ve) {
					return nil, invalid(fmt.Sprintf("shifts[%d].%s", i, ve.Field), ve.Message)
				}
				return nil, err
			}
			jobs = append(jobs, s.newJob(p, orgID, ji))
		}
	}
	if err := s.store.insertJobs(ctx, jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// ─── Signups ──────────────────────────────────────────────────────────────────

// SignupInput is a request to take one spot.
type SignupInput struct {
	HouseholdID   string // required for staff; ignored (forced) for parents
	VolunteerName string
	SwimmerID     string
	Certified     bool
	Notes         string
}

// SignUp takes a spot. Parents sign up their own household; staff may sign up
// any household. Capacity and duplicate checks are atomic in the store.
func (s *Service) SignUp(ctx context.Context, p *appmiddleware.Principal, orgID, jobID string, in SignupInput) (*SignupView, error) {
	var householdID string
	switch {
	case isStaff(p):
		householdID = strings.TrimSpace(in.HouseholdID)
		if householdID == "" {
			return nil, invalid("household_id", "is required when staff sign up a volunteer")
		}
		if err := s.ensureHousehold(orgID, householdID); err != nil {
			return nil, err
		}
	case hasRole(p, "parent"):
		householdID = strings.TrimSpace(p.HouseholdID)
		if householdID == "" {
			return nil, ErrForbidden
		}
		if in.HouseholdID != "" && strings.TrimSpace(in.HouseholdID) != householdID {
			return nil, ErrForbidden
		}
	default:
		return nil, ErrForbidden
	}
	name := strings.Join(strings.Fields(in.VolunteerName), " ")
	if name == "" || len(name) > 120 {
		return nil, invalid("volunteer_name", "is required (max 120 characters)")
	}
	notes := strings.TrimSpace(in.Notes)
	if len(notes) > 500 {
		return nil, invalid("notes", "must be at most 500 characters")
	}
	job, err := s.store.getJob(ctx, orgID, jobID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if !job.StartsAt.After(now) {
		return nil, ErrJobStarted
	}
	if job.RequiresCertification && !in.Certified {
		return nil, ErrCertificationRequired
	}
	swimmerID := strings.TrimSpace(in.SwimmerID)
	if swimmerID != "" {
		if s.swimmer == nil {
			return nil, invalid("swimmer_id", "swimmer volunteers are not supported")
		}
		ref, ok := s.swimmer(orgID, swimmerID)
		if !ok || ref.HouseholdID != householdID {
			return nil, invalid("swimmer_id", "swimmer is not in this household")
		}
		if job.MinAge > 0 {
			age, ok := ageOn(ref.DateOfBirth, job.StartsAt)
			if !ok || age < job.MinAge {
				return nil, ErrAgeRequirement
			}
		}
	}
	su := &Signup{
		ID:            uuid.NewString(),
		OrgID:         orgID,
		JobID:         jobID,
		HouseholdID:   householdID,
		VolunteerName: name,
		SwimmerID:     swimmerID,
		Status:        StatusConfirmed,
		Notes:         notes,
		SignedUpBy:    p.UserID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.insertSignup(ctx, su); err != nil {
		return nil, err
	}
	s.scheduleReminder(ctx, job, su)
	// The signup is already committed, so a settings read failure must not
	// fail the request (a client retry would hit the duplicate guard). Log it
	// and render with the default cutoff instead of hiding the problem.
	settings, err := s.settings(ctx, orgID)
	if err != nil {
		s.logger.Warn("volunteer: settings lookup failed after signup; using default cancellation cutoff",
			"org_id", orgID, "signup_id", su.ID, "job_id", job.ID, "error", err)
		settings = DefaultSettings(orgID)
	}
	v := s.signupView(su, job, s.householdNames(orgID), settings, p)
	return &v, nil
}

func (s *Service) ensureHousehold(orgID, householdID string) error {
	if s.households == nil {
		return nil
	}
	list, err := s.households(orgID)
	if err != nil {
		return err
	}
	for _, h := range list {
		if h.ID == householdID {
			return nil
		}
	}
	return ErrHouseholdNotFound
}

// ageOn returns the whole-year age on the given instant.
func ageOn(dob string, at time.Time) (int, bool) {
	d, err := time.Parse("2006-01-02", strings.TrimSpace(dob))
	if err != nil {
		return 0, false
	}
	age := at.Year() - d.Year()
	if at.Month() < d.Month() || (at.Month() == d.Month() && at.Day() < d.Day()) {
		age--
	}
	return age, true
}

// Cancel releases a spot. Parents may cancel their own household's spots
// until the org's cancellation cutoff; staff may cancel any time.
func (s *Service) Cancel(ctx context.Context, p *appmiddleware.Principal, orgID, signupID, reason string) (*Signup, *Signup, error) {
	if !isStaff(p) && !hasRole(p, "parent") {
		return nil, nil, ErrForbidden
	}
	su, err := s.store.getSignup(ctx, orgID, signupID)
	if err != nil {
		return nil, nil, err
	}
	if !isStaff(p) {
		if strings.TrimSpace(p.HouseholdID) == "" || su.HouseholdID != strings.TrimSpace(p.HouseholdID) {
			return nil, nil, ErrSignupNotFound
		}
	}
	if su.Status != StatusConfirmed {
		if su.Status == StatusCancelled {
			return nil, nil, ErrSignupNotActive
		}
		return nil, nil, ErrInvalidTransition
	}
	job, err := s.store.getJob(ctx, orgID, su.JobID)
	if err != nil {
		return nil, nil, err
	}
	if !isStaff(p) {
		settings, err := s.settings(ctx, orgID)
		if err != nil {
			return nil, nil, err
		}
		if !s.canCancel(job, settings) {
			return nil, nil, ErrCancellationClosed
		}
	}
	before := *su
	now := s.now()
	su.Status = StatusCancelled
	su.CancelledAt = &now
	su.CancelledBy = p.UserID
	if r := strings.TrimSpace(reason); r != "" {
		if runes := []rune(r); len(runes) > 500 {
			r = string(runes[:500])
		}
		su.Notes = strings.TrimSpace(su.Notes + "\nCancelled: " + r)
	}
	su.UpdatedAt = now
	if err := s.store.updateSignup(ctx, su, before.Status); err != nil {
		return nil, nil, err
	}
	return &before, su, nil
}

func (s *Service) canCancel(job *Job, settings *Settings) bool {
	cutoff := 48
	if settings != nil {
		cutoff = settings.CancellationCutoffHours
	}
	return s.now().Before(job.StartsAt.Add(-time.Duration(cutoff) * time.Hour))
}

// CheckIn records attendance: worked, no_show, or back to confirmed.
// hours overrides the job's hour credit for a worked signup.
func (s *Service) CheckIn(ctx context.Context, p *appmiddleware.Principal, orgID, signupID, status string, hours *float64) (*Signup, *Signup, error) {
	if !isStaff(p) {
		return nil, nil, ErrForbidden
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != StatusWorked && status != StatusNoShow && status != StatusConfirmed {
		return nil, nil, invalid("status", "must be worked, no_show, or confirmed")
	}
	su, err := s.store.getSignup(ctx, orgID, signupID)
	if err != nil {
		return nil, nil, err
	}
	if su.Status == StatusCancelled {
		return nil, nil, ErrInvalidTransition
	}
	if hours != nil {
		if status != StatusWorked {
			return nil, nil, invalid("hours", "may only be set when marking worked")
		}
		if *hours < 0 || *hours > 24 {
			return nil, nil, invalid("hours", "must be between 0 and 24")
		}
	}
	before := *su
	now := s.now()
	su.Status = status
	su.HoursCredited = nil
	if status == StatusWorked && hours != nil {
		h := round2(*hours)
		su.HoursCredited = &h
	}
	if status == StatusConfirmed {
		su.CheckedInAt = nil
		su.CheckedInBy = ""
	} else {
		su.CheckedInAt = &now
		su.CheckedInBy = p.UserID
	}
	su.UpdatedAt = now
	if err := s.store.updateSignup(ctx, su, before.Status); err != nil {
		return nil, nil, err
	}
	return &before, su, nil
}

// SignupFilter narrows signup listings.
type SignupFilter struct {
	JobID       string
	HouseholdID string
	MeetID      string
	Status      string
}

// ListSignups lists signups with their jobs. Parents and swimmers are pinned
// to their own household regardless of the requested filter.
func (s *Service) ListSignups(ctx context.Context, p *appmiddleware.Principal, orgID string, f SignupFilter) ([]SignupView, error) {
	scope, err := householdScope(p)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		if f.HouseholdID != "" && f.HouseholdID != scope {
			return nil, ErrForbidden
		}
		f.HouseholdID = scope
	}
	signups, err := s.store.listSignups(ctx, orgID, SignupFilter{JobID: f.JobID, HouseholdID: f.HouseholdID, Status: f.Status})
	if err != nil {
		return nil, err
	}
	jobs, err := s.store.listJobs(ctx, orgID, f.MeetID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*Job, len(jobs))
	for _, j := range jobs {
		byID[j.ID] = j
	}
	settings, err := s.settings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	names := s.householdNames(orgID)
	out := make([]SignupView, 0, len(signups))
	for _, su := range signups {
		job, ok := byID[su.JobID]
		if !ok {
			continue // filtered out by meet
		}
		v := s.signupView(su, job, names, settings, p)
		jc := *job
		v.Job = &jc
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Job.StartsAt.Before(out[j].Job.StartsAt)
	})
	return out, nil
}

func effectiveHours(su *Signup, job *Job) float64 {
	if su.Status != StatusWorked {
		return 0
	}
	if su.HoursCredited != nil {
		return *su.HoursCredited
	}
	return job.HourCredit
}

func (s *Service) signupView(su *Signup, job *Job, names map[string]string, settings *Settings, p *appmiddleware.Principal) SignupView {
	v := SignupView{Signup: *su, HouseholdName: names[su.HouseholdID], EffectiveHours: effectiveHours(su, job)}
	if su.Status == StatusConfirmed {
		if isStaff(p) {
			v.CanCancel = true
		} else if hasRole(p, "parent") {
			v.CanCancel = s.canCancel(job, settings)
		}
	}
	return v
}

// ─── Requirements & credits ───────────────────────────────────────────────────

// ListRequirements lists season/household requirements. Staff only.
func (s *Service) ListRequirements(ctx context.Context, p *appmiddleware.Principal, orgID, seasonID string) ([]*Requirement, error) {
	if !isStaff(p) {
		return nil, ErrForbidden
	}
	return s.store.listRequirements(ctx, orgID, seasonID)
}

// RequirementInput sets a season default or household override.
type RequirementInput struct {
	SeasonID           string
	HouseholdID        string
	RequiredHours      float64
	BuyoutCentsPerHour int
	Notes              string
}

// UpsertRequirement creates or replaces a requirement. Admin only.
func (s *Service) UpsertRequirement(ctx context.Context, p *appmiddleware.Principal, orgID string, in RequirementInput) (*Requirement, error) {
	if !hasRole(p, "admin") {
		return nil, ErrForbidden
	}
	in.SeasonID = strings.TrimSpace(in.SeasonID)
	in.HouseholdID = strings.TrimSpace(in.HouseholdID)
	if in.SeasonID == "" {
		return nil, invalid("season_id", "is required")
	}
	if _, ok := s.findSeason(orgID, in.SeasonID); !ok {
		return nil, ErrSeasonNotFound
	}
	if in.HouseholdID != "" {
		if err := s.ensureHousehold(orgID, in.HouseholdID); err != nil {
			return nil, err
		}
	}
	if in.RequiredHours < 0 || in.RequiredHours > 1000 {
		return nil, invalid("required_hours", "must be between 0 and 1000")
	}
	if in.BuyoutCentsPerHour < 0 || in.BuyoutCentsPerHour > 100000 {
		return nil, invalid("buyout_cents_per_hour", "must be between 0 and 100000")
	}
	if len(strings.TrimSpace(in.Notes)) > 500 {
		return nil, invalid("notes", "must be at most 500 characters")
	}
	req := &Requirement{
		ID:                 uuid.NewString(),
		OrgID:              orgID,
		SeasonID:           in.SeasonID,
		HouseholdID:        in.HouseholdID,
		RequiredHours:      round2(in.RequiredHours),
		BuyoutCentsPerHour: in.BuyoutCentsPerHour,
		Notes:              strings.TrimSpace(in.Notes),
		UpdatedAt:          s.now(),
		UpdatedBy:          p.UserID,
	}
	return s.store.upsertRequirement(ctx, req)
}

// DeleteRequirement removes a requirement. Admin only.
func (s *Service) DeleteRequirement(ctx context.Context, p *appmiddleware.Principal, orgID, id string) error {
	if !hasRole(p, "admin") {
		return ErrForbidden
	}
	return s.store.deleteRequirement(ctx, orgID, id)
}

// CreditInput is a manual hour adjustment.
type CreditInput struct {
	SeasonID    string
	HouseholdID string
	Hours       float64
	Reason      string
}

// AddCredit records a manual hour adjustment. Admin/coach.
func (s *Service) AddCredit(ctx context.Context, p *appmiddleware.Principal, orgID string, in CreditInput) (*Credit, error) {
	if !isStaff(p) {
		return nil, ErrForbidden
	}
	in.SeasonID = strings.TrimSpace(in.SeasonID)
	in.HouseholdID = strings.TrimSpace(in.HouseholdID)
	in.Reason = strings.TrimSpace(in.Reason)
	if in.SeasonID == "" {
		return nil, invalid("season_id", "is required")
	}
	if _, ok := s.findSeason(orgID, in.SeasonID); !ok {
		return nil, ErrSeasonNotFound
	}
	if in.HouseholdID == "" {
		return nil, invalid("household_id", "is required")
	}
	if err := s.ensureHousehold(orgID, in.HouseholdID); err != nil {
		return nil, err
	}
	h := round2(in.Hours)
	if h == 0 || h < -200 || h > 200 {
		return nil, invalid("hours", "must be non-zero and between -200 and 200")
	}
	if in.Reason == "" || len(in.Reason) > 500 {
		return nil, invalid("reason", "is required (max 500 characters)")
	}
	c := &Credit{
		ID:          uuid.NewString(),
		OrgID:       orgID,
		SeasonID:    in.SeasonID,
		HouseholdID: in.HouseholdID,
		Hours:       h,
		Reason:      in.Reason,
		CreatedBy:   p.UserID,
		CreatedAt:   s.now(),
	}
	if err := s.store.insertCredit(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ListCredits lists manual credits; parents/swimmers see their own household.
func (s *Service) ListCredits(ctx context.Context, p *appmiddleware.Principal, orgID, seasonID, householdID string) ([]*Credit, error) {
	scope, err := householdScope(p)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		if householdID != "" && householdID != scope {
			return nil, ErrForbidden
		}
		householdID = scope
	}
	return s.store.listCredits(ctx, orgID, seasonID, householdID)
}

// ─── Ledger ───────────────────────────────────────────────────────────────────

func (s *Service) seasonList(orgID string) []SeasonRef {
	if s.seasons == nil {
		return nil
	}
	list, err := s.seasons(orgID)
	if err != nil {
		s.logger.Warn("volunteer: season lookup failed", "org_id", orgID, "error", err)
		return nil
	}
	return list
}

func (s *Service) householdList(orgID string) []HouseholdRef {
	if s.households == nil {
		return nil
	}
	list, err := s.households(orgID)
	if err != nil {
		s.logger.Warn("volunteer: household lookup failed", "org_id", orgID, "error", err)
		return nil
	}
	return list
}

func (s *Service) householdNames(orgID string) map[string]string {
	out := map[string]string{}
	for _, h := range s.householdList(orgID) {
		out[h.ID] = h.Name
	}
	return out
}

// effectiveSeason returns the job's explicit season or the season whose date
// range contains the shift's (UTC) start date. Active seasons win ties.
func effectiveSeason(job *Job, seasons []SeasonRef) string {
	if job.SeasonID != "" {
		return job.SeasonID
	}
	day := job.StartsAt.UTC().Format("2006-01-02")
	best := ""
	for _, se := range seasons {
		if se.StartDate == "" || se.EndDate == "" {
			continue
		}
		if day < se.StartDate || day > se.EndDate {
			continue
		}
		if best == "" || se.Status == "active" {
			best = se.ID
		}
	}
	return best
}

type ledgerInputs struct {
	seasons  []SeasonRef
	jobs     map[string]*Job
	jobSeas  map[string]string
	signups  []*Signup
	credits  []*Credit
	reqs     []*Requirement
	buyouts  []*Buyout
	settings *Settings
}

func (s *Service) loadLedgerInputs(ctx context.Context, orgID, seasonID, householdID string) (*ledgerInputs, error) {
	in := &ledgerInputs{seasons: s.seasonList(orgID)}
	var err error
	if in.settings, err = s.settings(ctx, orgID); err != nil {
		return nil, err
	}
	jobs, err := s.store.listJobs(ctx, orgID, "")
	if err != nil {
		return nil, err
	}
	in.jobs = make(map[string]*Job, len(jobs))
	in.jobSeas = make(map[string]string, len(jobs))
	for _, j := range jobs {
		in.jobs[j.ID] = j
		in.jobSeas[j.ID] = effectiveSeason(j, in.seasons)
	}
	if in.signups, err = s.store.listSignups(ctx, orgID, SignupFilter{HouseholdID: householdID}); err != nil {
		return nil, err
	}
	if in.credits, err = s.store.listCredits(ctx, orgID, seasonID, householdID); err != nil {
		return nil, err
	}
	if in.reqs, err = s.store.listRequirements(ctx, orgID, seasonID); err != nil {
		return nil, err
	}
	if in.buyouts, err = s.store.listBuyouts(ctx, orgID, seasonID); err != nil {
		return nil, err
	}
	return in, nil
}

func (in *ledgerInputs) row(season SeasonRef, hh HouseholdRef) LedgerRow {
	row := LedgerRow{
		SeasonID: season.ID, SeasonName: season.Name, SeasonStatus: season.Status,
		SeasonStartDate: season.StartDate, SeasonEndDate: season.EndDate,
		HouseholdID: hh.ID, HouseholdName: hh.Name,
		RequiredHours:      in.settings.DefaultRequiredHours,
		BuyoutCentsPerHour: in.settings.DefaultBuyoutCentsPerHour,
		RequirementSource:  "org_default",
	}
	for _, r := range in.reqs {
		if r.SeasonID == season.ID && r.HouseholdID == "" && row.RequirementSource == "org_default" {
			row.RequiredHours, row.BuyoutCentsPerHour, row.RequirementSource = r.RequiredHours, r.BuyoutCentsPerHour, "season"
		}
	}
	for _, r := range in.reqs {
		if r.SeasonID == season.ID && r.HouseholdID == hh.ID {
			row.RequiredHours, row.BuyoutCentsPerHour, row.RequirementSource = r.RequiredHours, r.BuyoutCentsPerHour, "household"
		}
	}
	for _, su := range in.signups {
		if su.HouseholdID != hh.ID || in.jobSeas[su.JobID] != season.ID {
			continue
		}
		job := in.jobs[su.JobID]
		if job == nil {
			continue
		}
		switch su.Status {
		case StatusWorked:
			row.WorkedHours += effectiveHours(su, job)
		case StatusConfirmed:
			row.ScheduledHours += job.HourCredit
		}
	}
	for _, c := range in.credits {
		if c.HouseholdID == hh.ID && c.SeasonID == season.ID {
			row.CreditedHours += c.Hours
		}
	}
	for _, b := range in.buyouts {
		if b.HouseholdID == hh.ID && b.SeasonID == season.ID {
			bc := *b
			row.Buyout = &bc
		}
	}
	row.WorkedHours = round2(row.WorkedHours)
	row.CreditedHours = round2(row.CreditedHours)
	row.ScheduledHours = round2(row.ScheduledHours)
	row.EarnedHours = round2(row.WorkedHours + row.CreditedHours)
	row.RemainingHours = math.Max(0, round2(row.RequiredHours-row.EarnedHours))
	switch {
	case row.Buyout != nil && row.Buyout.Status == "billed":
		row.Status = ComplianceBoughtOut
	case row.RequiredHours <= 0:
		row.Status = ComplianceExempt
	case row.RemainingHours <= 0:
		row.Status = ComplianceMet
	case row.EarnedHours+row.ScheduledHours >= row.RequiredHours:
		row.Status = ComplianceOnTrack
	default:
		row.Status = ComplianceShort
	}
	if row.Status != ComplianceBoughtOut && row.Buyout == nil {
		row.BuyoutDueCents = int(math.Round(row.RemainingHours * float64(row.BuyoutCentsPerHour)))
	}
	return row
}

// SeasonLedger returns every household's position for one season. Staff only.
func (s *Service) SeasonLedger(ctx context.Context, p *appmiddleware.Principal, orgID, seasonID string) ([]LedgerRow, error) {
	if !isStaff(p) {
		return nil, ErrForbidden
	}
	season, ok := s.findSeason(orgID, strings.TrimSpace(seasonID))
	if !ok || seasonID == "" {
		return nil, ErrSeasonNotFound
	}
	in, err := s.loadLedgerInputs(ctx, orgID, season.ID, "")
	if err != nil {
		return nil, err
	}
	households := s.householdList(orgID)
	out := make([]LedgerRow, 0, len(households))
	for _, hh := range households {
		if hh.Status != "" && hh.Status != "active" {
			continue
		}
		out = append(out, in.row(season, hh))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RemainingHours != out[j].RemainingHours {
			return out[i].RemainingHours > out[j].RemainingHours
		}
		return strings.ToLower(out[i].HouseholdName) < strings.ToLower(out[j].HouseholdName)
	})
	return out, nil
}

// HouseholdLedger returns one household's position for every season (newest
// first). Parents/swimmers may only read their own household.
func (s *Service) HouseholdLedger(ctx context.Context, p *appmiddleware.Principal, orgID, householdID string) ([]LedgerRow, error) {
	scope, err := householdScope(p)
	if err != nil {
		return nil, err
	}
	householdID = strings.TrimSpace(householdID)
	if scope != "" && householdID != scope {
		return nil, ErrForbidden
	}
	hh := HouseholdRef{ID: householdID}
	found := s.households == nil
	for _, h := range s.householdList(orgID) {
		if h.ID == householdID {
			hh, found = h, true
		}
	}
	if !found {
		return nil, ErrHouseholdNotFound
	}
	in, err := s.loadLedgerInputs(ctx, orgID, "", householdID)
	if err != nil {
		return nil, err
	}
	seasons := append([]SeasonRef(nil), in.seasons...)
	sort.SliceStable(seasons, func(i, j int) bool { return seasons[i].StartDate > seasons[j].StartDate })
	out := make([]LedgerRow, 0, len(seasons))
	for _, se := range seasons {
		out = append(out, in.row(se, hh))
	}
	return out, nil
}

// ─── Buyouts ──────────────────────────────────────────────────────────────────

// BuyoutInput requests billing a household's missing hours.
type BuyoutInput struct {
	SeasonID    string
	HouseholdID string
	DueDate     string
	Issue       bool
}

// BillBuyout bills the remaining hours × rate through the billing service.
// A buyout row is reserved first so concurrent requests cannot double-bill;
// the reservation is released when billing fails. Admin only.
func (s *Service) BillBuyout(ctx context.Context, p *appmiddleware.Principal, orgID string, in BuyoutInput) (*Buyout, *BuyoutInvoiceResult, error) {
	if !hasRole(p, "admin") {
		return nil, nil, ErrForbidden
	}
	if s.biller == nil {
		return nil, nil, ErrBillingUnavailable
	}
	in.SeasonID = strings.TrimSpace(in.SeasonID)
	in.HouseholdID = strings.TrimSpace(in.HouseholdID)
	if in.DueDate != "" {
		if _, err := time.Parse("2006-01-02", in.DueDate); err != nil {
			return nil, nil, invalid("due_date", "must be YYYY-MM-DD")
		}
	}
	season, ok := s.findSeason(orgID, in.SeasonID)
	if !ok || in.SeasonID == "" {
		return nil, nil, ErrSeasonNotFound
	}
	var hh HouseholdRef
	found := false
	for _, h := range s.householdList(orgID) {
		if h.ID == in.HouseholdID {
			hh, found = h, true
		}
	}
	if !found {
		return nil, nil, ErrHouseholdNotFound
	}
	ledger, err := s.loadLedgerInputs(ctx, orgID, season.ID, hh.ID)
	if err != nil {
		return nil, nil, err
	}
	row := ledger.row(season, hh)
	if row.Buyout != nil {
		return nil, nil, ErrBuyoutExists
	}
	if row.RemainingHours <= 0 {
		return nil, nil, ErrNothingOwed
	}
	if row.BuyoutCentsPerHour <= 0 {
		return nil, nil, ErrNoBuyoutRate
	}
	now := s.now()
	b := &Buyout{
		ID:               uuid.NewString(),
		OrgID:            orgID,
		SeasonID:         season.ID,
		HouseholdID:      hh.ID,
		Status:           "pending",
		HoursShort:       row.RemainingHours,
		RateCentsPerHour: row.BuyoutCentsPerHour,
		AmountCents:      row.BuyoutDueCents,
		CreatedBy:        p.UserID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.reserveBuyout(ctx, b); err != nil {
		return nil, nil, err
	}
	due := in.DueDate
	if due == "" {
		due = now.AddDate(0, 0, 30).Format("2006-01-02")
	}
	res, err := s.biller.CreateBuyoutInvoice(ctx, BuyoutInvoiceRequest{
		OrgID:       orgID,
		HouseholdID: hh.ID,
		Description: fmt.Sprintf("Volunteer hours buyout — %s: %s of %s hours short at $%.2f/hr",
			season.Name, formatHours(row.RemainingHours), formatHours(row.RequiredHours), float64(row.BuyoutCentsPerHour)/100),
		AmountCents: b.AmountCents,
		DueDate:     due,
		Issue:       in.Issue,
		Principal:   p,
	})
	if err != nil {
		if delErr := s.store.deleteBuyout(ctx, orgID, b.ID); delErr != nil {
			s.logger.Error("volunteer: release buyout reservation failed", "buyout_id", b.ID, "error", delErr)
		}
		return nil, nil, err
	}
	b.Status = "billed"
	b.InvoiceID = res.InvoiceID
	b.InvoiceItemID = res.InvoiceItemID
	b.UpdatedAt = s.now()
	if err := s.store.updateBuyout(ctx, b); err != nil {
		return nil, nil, err
	}
	return b, &res, nil
}

func formatHours(h float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", h), "0"), ".")
}

// ListBuyouts lists buyouts for a season. Admin only.
func (s *Service) ListBuyouts(ctx context.Context, p *appmiddleware.Principal, orgID, seasonID string) ([]*Buyout, error) {
	if !hasRole(p, "admin") {
		return nil, ErrForbidden
	}
	return s.store.listBuyouts(ctx, orgID, seasonID)
}
