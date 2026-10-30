// Package orgsettings manages org-level export identity used to populate
// USA-Swimming meet reports: the club's team code, display name, and LSC
// (Local Swimming Committee) code. There is exactly one settings row per org.
package orgsettings

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrInvalidSlug means the public slug is malformed or required but empty.
	ErrInvalidSlug = errors.New("public slug must be 3-40 lowercase letters, digits or hyphens")
	// ErrSlugTaken means another club already uses the slug.
	ErrSlugTaken = errors.New("public slug is already in use")

	slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,38})[a-z0-9]$`)
)

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
	updateMu sync.Mutex
	mu       sync.RWMutex
	repo     *Repository
	settings map[string]*OrgSettings
	// swimmerOff holds orgs with swimmer sign-in disabled (in-memory mode).
	swimmerOff map[string]bool
}

// NewService returns an in-memory Service (no DB backing).
func NewService() *Service {
	return &Service{settings: make(map[string]*OrgSettings)}
}

// NewServiceWithPool returns a DB-backed Service and loads existing rows on
// startup.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	rows, err := s.repo.LoadAll(context.Background())
	if err != nil {
		slog.Error("orgsettings: failed to load from DB", "err", err)
		return s
	}
	s.mu.Lock()
	for _, row := range rows {
		s.settings[row.OrgID] = row
	}
	s.mu.Unlock()
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
