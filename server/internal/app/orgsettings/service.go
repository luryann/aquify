// Package orgsettings manages org-level export identity used to populate
// USA-Swimming meet reports: the club's team code, display name, and LSC
// (Local Swimming Committee) code. There is exactly one settings row per org.
package orgsettings

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidTimezone  = errors.New("timezone must be an IANA name such as America/Los_Angeles")
	ErrInvalidCurrency  = errors.New("currency must be a 3-letter ISO 4217 code such as usd")
	ErrInvalidLegalName = errors.New("legal name must be at most 120 characters")
	ErrInvalidAddress   = errors.New("address fields are too long (lines 60, city 40, state 3, postal code 10 characters)")
	currencyPattern     = regexp.MustCompile(`^[a-z]{3}$`)
	// ErrInvalidSlug means the public slug is malformed or required but empty.
	ErrInvalidSlug = errors.New("public slug must be 3-40 lowercase letters, digits or hyphens")
	// ErrSlugTaken means another club already uses the slug.
	ErrSlugTaken = errors.New("public slug is already in use")

	slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,38})[a-z0-9]$`)
)

// SetSlugReserved installs the cross-namespace check used when a club sets its
// public slug: it reports whether another org already holds slug elsewhere.
func (s *Service) SetSlugReserved(fn func(slug, orgID string) bool) {
	s.mu.Lock()
	s.slugReserved = fn
	s.mu.Unlock()
}

// FindBySlug returns the settings whose public_slug matches, or (nil, nil).
func (s *Service) FindBySlug(slug string) (*OrgSettings, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return nil, nil
	}
	if s.repo != nil {
		return s.repo.FindBySlug(context.Background(), slug)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, row := range s.settings {
		if row.PublicSlug == slug {
			cp := *row
			return &cp, nil
		}
	}
	return nil, nil
}

// OrgSettings holds the export identity fields for a single org.
type OrgSettings struct {
	OrgID              string `json:"org_id"`
	TeamCode           string `json:"team_code"`
	TeamName           string `json:"team_name"`
	LSCCode            string `json:"lsc_code"`
	EmailSenderName    string `json:"email_sender_name"`
	EmailSenderAddress string `json:"email_sender_address"`
	PublicSlug         string `json:"public_slug"`
	// Timezone is an IANA zone name ("" = UTC); Currency an ISO 4217 code in lower case
	// ("" = the deployment default); LegalName the entity name shown on invoices.
	Timezone  string `json:"timezone"`
	Currency  string `json:"currency"`
	LegalName string `json:"legal_name"`
	// Mailing address, written to meet entry files and the entry log.
	AddressLine1       string `json:"address_line1"`
	AddressLine2       string `json:"address_line2"`
	AddressCity        string `json:"address_city"`
	AddressState       string `json:"address_state"`
	AddressPostalCode  string `json:"address_postal_code"`
	PublicSiteEnabled  bool   `json:"public_site_enabled"`
	PublicShowCoaches  bool   `json:"public_show_coaches"`
	PublicAbout        string `json:"public_about"`
	PublicContactEmail string `json:"public_contact_email"`
	PublicContactPhone string `json:"public_contact_phone"`
	// SwimmerAccountsEnabled is filled by WithSwimmerAccounts (default true).
	SwimmerAccountsEnabled bool      `json:"swimmer_accounts_enabled"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	UpdatedBy              string    `json:"updated_by"`
}

// Updates carries the mutable fields that may be patched. A nil pointer leaves
// the existing value unchanged.
type Updates struct {
	TeamCode           *string `json:"team_code,omitempty"`
	TeamName           *string `json:"team_name,omitempty"`
	LSCCode            *string `json:"lsc_code,omitempty"`
	EmailSenderName    *string `json:"email_sender_name,omitempty"`
	EmailSenderAddress *string `json:"email_sender_address,omitempty"`
	PublicSlug         *string `json:"public_slug,omitempty"`
	Timezone           *string `json:"timezone,omitempty"`
	Currency           *string `json:"currency,omitempty"`
	LegalName          *string `json:"legal_name,omitempty"`
	AddressLine1       *string `json:"address_line1,omitempty"`
	AddressLine2       *string `json:"address_line2,omitempty"`
	AddressCity        *string `json:"address_city,omitempty"`
	AddressState       *string `json:"address_state,omitempty"`
	AddressPostalCode  *string `json:"address_postal_code,omitempty"`
	PublicSiteEnabled  *bool   `json:"public_site_enabled,omitempty"`
	PublicShowCoaches  *bool   `json:"public_show_coaches,omitempty"`
	PublicAbout        *string `json:"public_about,omitempty"`
	PublicContactEmail *string `json:"public_contact_email,omitempty"`
	PublicContactPhone *string `json:"public_contact_phone,omitempty"`
	// SwimmerAccountsEnabled is written in the same statement/critical section
	// as every other field so a combined update is all-or-nothing.
	SwimmerAccountsEnabled *bool `json:"swimmer_accounts_enabled,omitempty"`
}

// Service manages org settings with an in-memory write-through cache.
type Service struct {
	// updateMu serialises Update's read-modify-write so concurrent patches to
	// different fields cannot overwrite each other with a stale snapshot.
	updateMu     sync.Mutex
	mu           sync.RWMutex
	repo         *Repository
	settings     map[string]*OrgSettings
	slugReserved func(slug, orgID string) bool
	// swimmerOff holds orgs with swimmer sign-in disabled (in-memory mode).
	swimmerOff map[string]bool
}

// NewService returns an in-memory Service (no DB backing).
func NewService() *Service {
	return &Service{settings: make(map[string]*OrgSettings)}
}

// NewServiceWithPool returns a DB-backed Service. Postgres is canonical: every read and
// write goes to the database, so nothing is loaded at startup.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	return s
}

// Get returns the settings for an org, or an empty (all-blank) default when the
// org has not configured any yet.
func (s *Service) Get(orgID string) (*OrgSettings, error) {
	orgID = strings.TrimSpace(orgID)
	if s.repo != nil {
		row, err := s.repo.Get(context.Background(), orgID)
		if err != nil {
			return nil, err
		}
		if row != nil {
			return row, nil
		}
		return &OrgSettings{OrgID: orgID}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if row, ok := s.settings[orgID]; ok {
		cp := *row
		return &cp, nil
	}
	return &OrgSettings{OrgID: orgID}, nil
}

// Update applies the provided field changes, creating the row if absent.
func (s *Service) Update(orgID string, updates Updates, updatedBy string) (*OrgSettings, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	current, err := s.Get(orgID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if current.CreatedAt.IsZero() {
		current.CreatedAt = now
	}
	if updates.TeamCode != nil {
		current.TeamCode = strings.TrimSpace(*updates.TeamCode)
	}
	if updates.TeamName != nil {
		current.TeamName = strings.TrimSpace(*updates.TeamName)
	}
	if updates.LSCCode != nil {
		current.LSCCode = strings.ToUpper(strings.TrimSpace(*updates.LSCCode))
	}
	if updates.EmailSenderName != nil {
		current.EmailSenderName = strings.TrimSpace(*updates.EmailSenderName)
	}
	if updates.EmailSenderAddress != nil {
		current.EmailSenderAddress = strings.TrimSpace(*updates.EmailSenderAddress)
	}
	if updates.Timezone != nil {
		tz := strings.TrimSpace(*updates.Timezone)
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				return nil, ErrInvalidTimezone
			}
		}
		current.Timezone = tz
	}
	if updates.Currency != nil {
		cur := strings.ToLower(strings.TrimSpace(*updates.Currency))
		if cur != "" && !currencyPattern.MatchString(cur) {
			return nil, ErrInvalidCurrency
		}
		current.Currency = cur
	}
	if updates.LegalName != nil {
		name := strings.TrimSpace(*updates.LegalName)
		if len([]rune(name)) > 120 {
			return nil, ErrInvalidLegalName
		}
		current.LegalName = name
	}
	for _, field := range []struct {
		update *string
		target *string
		max    int
		upper  bool
	}{
		{updates.AddressLine1, &current.AddressLine1, 60, false},
		{updates.AddressLine2, &current.AddressLine2, 60, false},
		{updates.AddressCity, &current.AddressCity, 40, false},
		{updates.AddressState, &current.AddressState, 3, true},
		{updates.AddressPostalCode, &current.AddressPostalCode, 10, false},
	} {
		if field.update == nil {
			continue
		}
		value := strings.TrimSpace(*field.update)
		if field.upper {
			value = strings.ToUpper(value)
		}
		if len([]rune(value)) > field.max {
			return nil, ErrInvalidAddress
		}
		*field.target = value
	}
	if updates.PublicSlug != nil {
		slug := strings.ToLower(strings.TrimSpace(*updates.PublicSlug))
		if slug != "" && !slugPattern.MatchString(slug) {
			return nil, ErrInvalidSlug
		}
		if slug != "" {
			if other, err := s.FindBySlug(slug); err != nil {
				return nil, err
			} else if other != nil && other.OrgID != current.OrgID {
				return nil, ErrSlugTaken
			}
			// The tenant registry owns a second slug namespace (orgs.slug); a slug
			// held there by another org must never also address this org's site.
			if s.slugReserved != nil && s.slugReserved(slug, current.OrgID) {
				return nil, ErrSlugTaken
			}
		}
		current.PublicSlug = slug
	}
	if updates.PublicSiteEnabled != nil {
		current.PublicSiteEnabled = *updates.PublicSiteEnabled
	}
	if updates.PublicShowCoaches != nil {
		current.PublicShowCoaches = *updates.PublicShowCoaches
	}
	if updates.PublicAbout != nil {
		current.PublicAbout = strings.TrimSpace(*updates.PublicAbout)
	}
	if updates.PublicContactEmail != nil {
		current.PublicContactEmail = strings.TrimSpace(*updates.PublicContactEmail)
	}
	if updates.PublicContactPhone != nil {
		current.PublicContactPhone = strings.TrimSpace(*updates.PublicContactPhone)
	}
	if current.PublicSiteEnabled && current.PublicSlug == "" {
		return nil, ErrInvalidSlug
	}
	current.UpdatedAt = now
	current.UpdatedBy = updatedBy

	if s.repo != nil {
		if err := s.repo.Upsert(context.Background(), current, updates.SwimmerAccountsEnabled); err != nil {
			// Another instance can claim the slug between the check above and
			// this write; the unique index is the real guard.
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, ErrSlugTaken
			}
			return nil, err
		}
		return s.repo.Get(context.Background(), orgID)
	}
	s.mu.Lock()
	s.settings[orgID] = current
	if updates.SwimmerAccountsEnabled != nil {
		if *updates.SwimmerAccountsEnabled {
			delete(s.swimmerOff, orgID)
		} else {
			if s.swimmerOff == nil {
				s.swimmerOff = make(map[string]bool)
			}
			s.swimmerOff[orgID] = true
		}
	}
	s.mu.Unlock()
	cp := *current
	return &cp, nil
}
