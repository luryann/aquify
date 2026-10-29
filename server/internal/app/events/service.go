// Package events manages non-meet club events (socials, fundraisers, team
// meetings, clinics, banquets, dryland sessions), their RSVPs, optional RSVP
// fees billed through the billing domain, and per-household calendar feeds.
package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound         = errors.New("club event not found")
	ErrInvalidEvent     = errors.New("invalid club event")
	ErrRSVPClosed       = errors.New("rsvp is closed for this event")
	ErrRSVPDisabled     = errors.New("rsvp is not enabled for this event")
	ErrEventCancelled   = errors.New("event is cancelled")
	ErrEventFull        = errors.New("event is at capacity")
	ErrInvalidRSVP      = errors.New("invalid rsvp")
	ErrRSVPModeLocked   = errors.New("rsvp mode cannot change once responses exist")
	ErrFeedTokenMissing = errors.New("calendar feed token not found")
)

// Categories lists the supported event categories.
var Categories = []string{"social", "fundraiser", "team_meeting", "clinic", "banquet", "dryland", "other"}

var validCategories = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range Categories {
		m[c] = true
	}
	return m
}()

const (
	maxTitleLen       = 200
	maxDescriptionLen = 5000
	maxLocationLen    = 300
	maxNoteLen        = 500
	maxGuestCount     = 50
	maxFeeCents       = 1_000_000
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// Event is a non-meet club calendar event.
type Event struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	Title        string     `json:"title"`
	Category     string     `json:"category"`
	Description  string     `json:"description"`
	Location     string     `json:"location"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at,omitempty"`
	AllDay       bool       `json:"all_day"`
	Audience     string     `json:"audience"` // all | groups
	GroupCodes   []string   `json:"group_codes"`
	RSVPEnabled  bool       `json:"rsvp_enabled"`
	RSVPMode     string     `json:"rsvp_mode"` // swimmer | household
	RSVPDeadline *time.Time `json:"rsvp_deadline,omitempty"`
	Capacity     *int       `json:"capacity,omitempty"`
	FeeCents     int        `json:"fee_cents"`
	Status       string     `json:"status"` // scheduled | cancelled
	Public       bool       `json:"public"` // shown on the public team website
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CreatedBy    string     `json:"created_by"`
	UpdatedBy    string     `json:"updated_by"`
}

// RSVPOpen reports whether non-staff may still respond at time now.
func (e *Event) RSVPOpen(now time.Time) bool {
	if e == nil || !e.RSVPEnabled || e.Status != "scheduled" {
		return false
	}
	if e.RSVPDeadline != nil && now.After(*e.RSVPDeadline) {
		return false
	}
	end := e.StartsAt
	if e.EndsAt != nil {
		end = *e.EndsAt
	}
	return !now.After(end)
}

// RSVP is one party's response to an event: a swimmer (swimmer mode) or a
// whole household (household mode, SwimmerID empty).
type RSVP struct {
	ID             string    `json:"id"`
	OrgID          string    `json:"org_id"`
	EventID        string    `json:"event_id"`
	HouseholdID    string    `json:"household_id"`
	SwimmerID      string    `json:"swimmer_id,omitempty"`
	Response       string    `json:"response"` // yes | no | maybe
	GuestCount     int       `json:"guest_count"`
	Note           string    `json:"note,omitempty"`
	InvoiceID      string    `json:"invoice_id,omitempty"`
	BilledQuantity int       `json:"billed_quantity"`
	RespondedBy    string    `json:"responded_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Headcount is the number of people this RSVP brings when attending.
func (r *RSVP) Headcount() int {
	return 1 + r.GuestCount
}

// Summary aggregates RSVP counts for an event.
type Summary struct {
	Yes          int  `json:"yes"`
	No           int  `json:"no"`
	Maybe        int  `json:"maybe"`
	YesHeadcount int  `json:"yes_headcount"`
	SpotsLeft    *int `json:"spots_left,omitempty"`
}

// Summarize computes a Summary for one event's RSVPs.
func Summarize(event *Event, rsvps []*RSVP) Summary {
	var s Summary
	for _, r := range rsvps {
		switch r.Response {
		case "yes":
			s.Yes++
			s.YesHeadcount += r.Headcount()
		case "no":
			s.No++
		case "maybe":
			s.Maybe++
		}
	}
	if event != nil && event.Capacity != nil {
		left := *event.Capacity - s.YesHeadcount
		if left < 0 {
			left = 0
		}
		s.SpotsLeft = &left
	}
	return s
}

// EventInput carries the full mutable field set for create/replace.
type EventInput struct {
	Title        string
	Category     string
	Description  string
	Location     string
	StartsAt     time.Time
	EndsAt       *time.Time
	AllDay       bool
	Audience     string
	GroupCodes   []string
	RSVPEnabled  bool
	RSVPMode     string
	RSVPDeadline *time.Time
	Capacity     *int
	FeeCents     int
	Status       string
	Public       bool
}

// RSVPInput is a response submission. For household-mode events SwimmerID
// must be empty; for swimmer-mode events it is required.
type RSVPInput struct {
	HouseholdID string
	SwimmerID   string
	Response    string
	GuestCount  int
	Note        string
}

// ValidationError explains which field failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Unwrap lets errors.Is match ErrInvalidEvent.
func (e *ValidationError) Unwrap() error { return ErrInvalidEvent }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// normalize validates input and fills defaults.
func normalize(in EventInput) (EventInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Category = strings.ToLower(strings.TrimSpace(in.Category))
	in.Description = strings.TrimSpace(in.Description)
	in.Location = strings.TrimSpace(in.Location)
	in.Audience = strings.ToLower(strings.TrimSpace(in.Audience))
	in.RSVPMode = strings.ToLower(strings.TrimSpace(in.RSVPMode))
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	if in.Audience == "" {
		in.Audience = "all"
	}
	if in.RSVPMode == "" {
		in.RSVPMode = "household"
	}
	if in.Status == "" {
		in.Status = "scheduled"
	}
	switch {
	case in.Title == "":
		return in, invalid("title", "is required")
	case len(in.Title) > maxTitleLen:
		return in, invalid("title", fmt.Sprintf("must be at most %d characters", maxTitleLen))
	case !validCategories[in.Category]:
		return in, invalid("category", "must be one of "+strings.Join(Categories, ", "))
	case len(in.Description) > maxDescriptionLen:
		return in, invalid("description", fmt.Sprintf("must be at most %d characters", maxDescriptionLen))
	case len(in.Location) > maxLocationLen:
		return in, invalid("location", fmt.Sprintf("must be at most %d characters", maxLocationLen))
	case in.StartsAt.IsZero():
		return in, invalid("starts_at", "is required")
	case in.EndsAt != nil && in.EndsAt.Before(in.StartsAt):
		return in, invalid("ends_at", "must not be before starts_at")
	case in.Audience != "all" && in.Audience != "groups":
		return in, invalid("audience", "must be all or groups")
	case in.RSVPMode != "swimmer" && in.RSVPMode != "household":
		return in, invalid("rsvp_mode", "must be swimmer or household")
	case in.Status != "scheduled" && in.Status != "cancelled":
		return in, invalid("status", "must be scheduled or cancelled")
	case in.Capacity != nil && *in.Capacity <= 0:
		return in, invalid("capacity", "must be greater than zero")
	case in.FeeCents < 0 || in.FeeCents > maxFeeCents:
		return in, invalid("fee_cents", "must be between 0 and 1000000")
	case in.FeeCents > 0 && !in.RSVPEnabled:
		return in, invalid("fee_cents", "an RSVP fee requires RSVP to be enabled")
	case in.RSVPDeadline != nil && in.RSVPDeadline.After(in.StartsAt):
		return in, invalid("rsvp_deadline", "must not be after the event start")
	}
	in.StartsAt = in.StartsAt.UTC()
	if in.EndsAt != nil {
		t := in.EndsAt.UTC()
		in.EndsAt = &t
	}
	if in.RSVPDeadline != nil {
		t := in.RSVPDeadline.UTC()
		in.RSVPDeadline = &t
	}
	codes := []string{}
	seen := map[string]bool{}
	for _, c := range in.GroupCodes {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c != "" && !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	sort.Strings(codes)
	if in.Audience == "groups" && len(codes) == 0 {
		return in, invalid("group_codes", "at least one group is required when audience is groups")
	}
	if in.Audience == "all" {
		codes = []string{}
	}
	in.GroupCodes = codes
	if !in.RSVPEnabled {
		in.RSVPDeadline = nil
		in.Capacity = nil
	}
	return in, nil
}

// ─── Billing seam ─────────────────────────────────────────────────────────────

// Charge describes an RSVP fee to bill to a household.
type Charge struct {
	EventID         string
	HouseholdID     string
	SwimmerID       string
	Description     string
	DueDate         string // YYYY-MM-DD
	Quantity        int
	UnitAmountCents int
	ActorUserID     string
}

// Biller creates and releases RSVP fee invoices. The production
// implementation wraps the billing service; nil disables fees.
type Biller interface {
	// ChargeRSVP creates an issued invoice for the charge and returns its ID.
	ChargeRSVP(orgID string, charge Charge) (string, error)
	// ReleaseCharge voids the invoice if it is still unpaid. It returns
	// released=false (and no error) when the invoice has payments and must
	// be refunded through billing instead.
	ReleaseCharge(orgID, invoiceID, actorUserID string) (bool, error)
}

// BillingOutcome describes what happened to the RSVP fee on a response.
type BillingOutcome string

const (
	BillingNone         BillingOutcome = "none"
	BillingCharged      BillingOutcome = "charged"
	BillingUnchanged    BillingOutcome = "unchanged"
	BillingReleased     BillingOutcome = "released"
	BillingRetainedPaid BillingOutcome = "retained_paid"
	BillingFailed       BillingOutcome = "failed"
)

// RSVPResult is returned from Respond.
type RSVPResult struct {
	RSVP    *RSVP          `json:"rsvp"`
	Billing BillingOutcome `json:"billing"`
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service manages club events with an in-memory store or a Postgres-backed
// repository (DB is canonical when configured, so multiple API instances
// observe the same state).
type Service struct {
	mu      sync.Mutex
	repo    *Repository
	biller  Biller
	logger  *slog.Logger
	now     func() time.Time
	events  map[string]*Event
	rsvps   map[string]*RSVP // by RSVP ID
	tokens  map[string]*FeedToken
	hashIdx map[string]string // token hash -> token ID
}

// NewService returns an in-memory Service.
func NewService() *Service {
	return &Service{
		logger:  slog.Default(),
		now:     func() time.Time { return time.Now().UTC() },
		events:  map[string]*Event{},
		rsvps:   map[string]*RSVP{},
		tokens:  map[string]*FeedToken{},
		hashIdx: map[string]string{},
	}
}

// NewServiceWithPool returns a DB-backed Service (in-memory when pool is nil).
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool != nil {
		s.repo = newRepository(pool)
	}
	return s
}

// SetBiller wires RSVP fee billing.
func (s *Service) SetBiller(b Biller) { s.biller = b }

// SetLogger overrides the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// SetClock overrides the time source (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Now returns the service clock time.
func (s *Service) Now() time.Time { return s.now() }

func copyEvent(e *Event) *Event {
	cp := *e
	cp.GroupCodes = append([]string{}, e.GroupCodes...)
	return &cp
}

func applyInput(e *Event, in EventInput) {
	e.Title = in.Title
	e.Category = in.Category
	e.Description = in.Description
	e.Location = in.Location
	e.StartsAt = in.StartsAt
	e.EndsAt = in.EndsAt
	e.AllDay = in.AllDay
	e.Public = in.Public
	e.Audience = in.Audience
	e.GroupCodes = in.GroupCodes
	e.RSVPEnabled = in.RSVPEnabled
	e.RSVPMode = in.RSVPMode
	e.RSVPDeadline = in.RSVPDeadline
	e.Capacity = in.Capacity
	e.FeeCents = in.FeeCents
	e.Status = in.Status
}

// Create validates and stores a new event.
func (s *Service) Create(orgID, actorID string, in EventInput) (*Event, error) {
	in, err := normalize(in)
	if err != nil {
		return nil, err
	}
	now := s.now()
	e := &Event{ID: uuid.NewString(), OrgID: orgID, CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID}
	applyInput(e, in)
	if s.repo != nil {
		if err := s.repo.UpsertEvent(context.Background(), e); err != nil {
			return nil, err
		}
		return copyEvent(e), nil
	}
	s.mu.Lock()
	s.events[e.ID] = e
	s.mu.Unlock()
	return copyEvent(e), nil
}

// Get returns one event scoped to org.
func (s *Service) Get(orgID, id string) (*Event, error) {
	if s.repo != nil {
		return s.repo.GetEvent(context.Background(), orgID, id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.events[strings.TrimSpace(id)]
	if !ok || e.OrgID != orgID {
		return nil, ErrNotFound
	}
	return copyEvent(e), nil
}

// ListFilter bounds List by event time.
type ListFilter struct {
	From *time.Time // events ending at/after From
	To   *time.Time // events starting before To
}

func eventEnd(e *Event) time.Time {
	if e.EndsAt != nil {
		return *e.EndsAt
	}
	return e.StartsAt
}

// List returns events in org ordered by start time.
func (s *Service) List(orgID string, f ListFilter) ([]*Event, error) {
	var all []*Event
	if s.repo != nil {
		list, err := s.repo.ListEvents(context.Background(), orgID)
		if err != nil {
			return nil, err
		}
		all = list
	} else {
		s.mu.Lock()
		for _, e := range s.events {
			if e.OrgID == orgID {
				all = append(all, copyEvent(e))
			}
		}
		s.mu.Unlock()
	}
	out := []*Event{}
	for _, e := range all {
		if f.From != nil && eventEnd(e).Before(*f.From) {
			continue
		}
		if f.To != nil && !e.StartsAt.Before(*f.To) {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].StartsAt.Equal(out[j].StartsAt) {
			return out[i].StartsAt.Before(out[j].StartsAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Update replaces an event's mutable fields. Cancelling an event releases
// any unpaid RSVP fee invoices.
func (s *Service) Update(orgID, id, actorID string, in EventInput) (*Event, error) {
	in, err := normalize(in)
	if err != nil {
		return nil, err
	}
	existing, err := s.Get(orgID, id)
	if err != nil {
		return nil, err
	}
	rsvps, err := s.ListRSVPs(orgID, id)
	if err != nil {
		return nil, err
	}
	if len(rsvps) > 0 && existing.RSVPMode != in.RSVPMode {
		return nil, ErrRSVPModeLocked
	}
	updated := copyEvent(existing)
	applyInput(updated, in)
	updated.UpdatedAt = s.now()
	updated.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.UpsertEvent(context.Background(), updated); err != nil {
			return nil, err
		}
	} else {
		s.mu.Lock()
		s.events[id] = copyEvent(updated)
		s.mu.Unlock()
	}
	if existing.Status != "cancelled" && updated.Status == "cancelled" {
		s.releaseAll(orgID, actorID, rsvps)
	}
	return updated, nil
}

// Delete removes an event (and its RSVPs), releasing unpaid fee invoices.
func (s *Service) Delete(orgID, id, actorID string) error {
	if _, err := s.Get(orgID, id); err != nil {
		return err
	}
	rsvps, err := s.ListRSVPs(orgID, id)
	if err != nil {
		return err
	}
	if s.repo != nil {
		if err := s.repo.DeleteEvent(context.Background(), orgID, id); err != nil {
			return err
		}
	} else {
		s.mu.Lock()
		delete(s.events, id)
		for rid, r := range s.rsvps {
			if r.EventID == id {
				delete(s.rsvps, rid)
			}
		}
		s.mu.Unlock()
	}
	s.releaseAll(orgID, actorID, rsvps)
	return nil
}

func (s *Service) releaseAll(orgID, actorID string, rsvps []*RSVP) {
	if s.biller == nil {
		return
	}
	for _, r := range rsvps {
		if r.InvoiceID == "" {
			continue
		}
		released, err := s.biller.ReleaseCharge(orgID, r.InvoiceID, actorID)
		if err != nil {
			s.logger.Warn("club events: release rsvp invoice failed", "invoice_id", r.InvoiceID, "rsvp_id", r.ID, "error", err)
			continue
		}
		if released {
			r.InvoiceID = ""
			r.BilledQuantity = 0
			_ = s.saveBilling(orgID, r)
		}
	}
}

// ListRSVPs returns all RSVPs for an event.
func (s *Service) ListRSVPs(orgID, eventID string) ([]*RSVP, error) {
	if s.repo != nil {
		return s.repo.ListRSVPs(context.Background(), orgID, eventID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []*RSVP{}
	for _, r := range s.rsvps {
		if r.OrgID == orgID && r.EventID == eventID {
			cp := *r
			out = append(out, &cp)
		}
	}
	sortRSVPs(out)
	return out, nil
}

// ListRSVPsForEvents returns RSVPs grouped by event ID.
func (s *Service) ListRSVPsForEvents(orgID string, eventIDs []string) (map[string][]*RSVP, error) {
	out := map[string][]*RSVP{}
	if len(eventIDs) == 0 {
		return out, nil
	}
	if s.repo != nil {
		list, err := s.repo.ListRSVPsForEvents(context.Background(), orgID, eventIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range list {
			out[r.EventID] = append(out[r.EventID], r)
		}
		return out, nil
	}
	want := map[string]bool{}
	for _, id := range eventIDs {
		want[id] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rsvps {
		if r.OrgID == orgID && want[r.EventID] {
			cp := *r
			out[r.EventID] = append(out[r.EventID], &cp)
		}
	}
	for k := range out {
		sortRSVPs(out[k])
	}
	return out, nil
}

func sortRSVPs(list []*RSVP) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].HouseholdID != list[j].HouseholdID {
			return list[i].HouseholdID < list[j].HouseholdID
		}
		return list[i].SwimmerID < list[j].SwimmerID
	})
}

// buildRSVP validates a response against capacity and returns the row to
// persist. othersYes is the yes-headcount of every other party.
func (s *Service) buildRSVP(event *Event, existing *RSVP, othersYes int, in RSVPInput, actorID string) (*RSVP, error) {
	now := s.now()
	r := &RSVP{}
	if existing != nil {
		cp := *existing
		r = &cp
	} else {
		r.ID = uuid.NewString()
		r.OrgID = event.OrgID
		r.EventID = event.ID
		r.HouseholdID = in.HouseholdID
		r.SwimmerID = in.SwimmerID
		r.CreatedAt = now
	}
	r.Response = in.Response
	r.GuestCount = in.GuestCount
	r.Note = in.Note
	r.RespondedBy = actorID
	r.UpdatedAt = now
	if r.Response == "yes" && event.Capacity != nil && othersYes+r.Headcount() > *event.Capacity {
		return nil, ErrEventFull
	}
	return r, nil
}

func normalizeRSVPInput(event *Event, in RSVPInput) (RSVPInput, error) {
	in.HouseholdID = strings.TrimSpace(in.HouseholdID)
	in.SwimmerID = strings.TrimSpace(in.SwimmerID)
	in.Response = strings.ToLower(strings.TrimSpace(in.Response))
	in.Note = strings.TrimSpace(in.Note)
	if in.HouseholdID == "" {
		return in, &ValidationError{Field: "household_id", Message: "is required"}
	}
	switch in.Response {
	case "yes", "no", "maybe":
	default:
		return in, &ValidationError{Field: "response", Message: "must be yes, no, or maybe"}
	}
	if in.GuestCount < 0 || in.GuestCount > maxGuestCount {
		return in, &ValidationError{Field: "guest_count", Message: fmt.Sprintf("must be between 0 and %d", maxGuestCount)}
	}
	if len(in.Note) > maxNoteLen {
		return in, &ValidationError{Field: "note", Message: fmt.Sprintf("must be at most %d characters", maxNoteLen)}
	}
	if in.Response == "no" {
		in.GuestCount = 0
	}
	if event.RSVPMode == "swimmer" && in.SwimmerID == "" {
		return in, &ValidationError{Field: "swimmer_id", Message: "is required for per-swimmer RSVP events"}
	}
	if event.RSVPMode == "household" {
		in.SwimmerID = ""
	}
	return in, nil
}

// Respond upserts an RSVP. enforceWindow applies the RSVP deadline (staff
// edits on behalf of a family bypass it). Fee billing runs after the RSVP is
// stored; billing failures are reported in the result, never silently lost.
func (s *Service) Respond(orgID, eventID, actorID string, in RSVPInput, enforceWindow bool) (*RSVPResult, error) {
	event, err := s.Get(orgID, eventID)
	if err != nil {
		return nil, err
	}
	if event.Status == "cancelled" {
		return nil, ErrEventCancelled
	}
	if !event.RSVPEnabled {
		return nil, ErrRSVPDisabled
	}
	if enforceWindow && !event.RSVPOpen(s.now()) {
		return nil, ErrRSVPClosed
	}
	in, err = normalizeRSVPInput(event, in)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRSVP, err)
	}

	var saved *RSVP
	if s.repo != nil {
		saved, err = s.repo.RespondTx(context.Background(), orgID, eventID, in.HouseholdID, in.SwimmerID,
			func(existing *RSVP, othersYes int) (*RSVP, error) {
				return s.buildRSVP(event, existing, othersYes, in, actorID)
			})
		if err != nil {
			return nil, err
		}
	} else {
		s.mu.Lock()
		var existing *RSVP
		othersYes := 0
		for _, r := range s.rsvps {
			if r.OrgID != orgID || r.EventID != eventID {
				continue
			}
			if r.HouseholdID == in.HouseholdID && r.SwimmerID == in.SwimmerID {
				existing = r
				continue
			}
			if r.Response == "yes" {
				othersYes += r.Headcount()
			}
		}
		saved, err = s.buildRSVP(event, existing, othersYes, in, actorID)
		if err == nil {
			cp := *saved
			s.rsvps[saved.ID] = &cp
		}
		s.mu.Unlock()
		if err != nil {
			return nil, err
		}
	}

	outcome := s.reconcileFee(event, saved, actorID)
	return &RSVPResult{RSVP: saved, Billing: outcome}, nil
}

// reconcileFee keeps the RSVP's fee invoice aligned with its response.
func (s *Service) reconcileFee(event *Event, r *RSVP, actorID string) BillingOutcome {
	if s.biller == nil || (event.FeeCents <= 0 && r.InvoiceID == "") {
		return BillingNone
	}
	wantQty := 0
	if r.Response == "yes" && event.FeeCents > 0 {
		wantQty = r.Headcount()
	}
	if r.InvoiceID != "" && r.BilledQuantity == wantQty {
		return BillingUnchanged
	}
	if r.InvoiceID != "" {
		released, err := s.biller.ReleaseCharge(event.OrgID, r.InvoiceID, actorID)
		if err != nil {
			s.logger.Warn("club events: release rsvp invoice failed", "invoice_id", r.InvoiceID, "error", err)
			return BillingFailed
		}
		if !released {
			// Already paid (fully or partly): keep the invoice; any refund
			// goes through billing so the ledger stays authoritative.
			return BillingRetainedPaid
		}
		r.InvoiceID = ""
		r.BilledQuantity = 0
		if wantQty == 0 {
			if err := s.saveBilling(event.OrgID, r); err != nil {
				return BillingFailed
			}
			return BillingReleased
		}
	}
	if wantQty == 0 {
		return BillingNone
	}
	desc := fmt.Sprintf("%s — RSVP fee", event.Title)
	invoiceID, err := s.biller.ChargeRSVP(event.OrgID, Charge{
		EventID:         event.ID,
		HouseholdID:     r.HouseholdID,
		SwimmerID:       r.SwimmerID,
		Description:     desc,
		DueDate:         event.StartsAt.Format("2006-01-02"),
		Quantity:        wantQty,
		UnitAmountCents: event.FeeCents,
		ActorUserID:     actorID,
	})
	if err != nil {
		s.logger.Warn("club events: charge rsvp fee failed", "event_id", event.ID, "household_id", r.HouseholdID, "error", err)
		_ = s.saveBilling(event.OrgID, r)
		return BillingFailed
	}
	r.InvoiceID = invoiceID
	r.BilledQuantity = wantQty
	if err := s.saveBilling(event.OrgID, r); err != nil {
		return BillingFailed
	}
	return BillingCharged
}

func (s *Service) saveBilling(orgID string, r *RSVP) error {
	if s.repo != nil {
		if err := s.repo.UpdateRSVPBilling(context.Background(), orgID, r.ID, r.InvoiceID, r.BilledQuantity); err != nil {
			s.logger.Error("club events: persist rsvp billing failed", "rsvp_id", r.ID, "error", err)
			return err
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if stored, ok := s.rsvps[r.ID]; ok {
		stored.InvoiceID = r.InvoiceID
		stored.BilledQuantity = r.BilledQuantity
	}
	return nil
}
