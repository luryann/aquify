// Package orgsettings manages org-level export identity used to populate
// USA-Swimming meet reports: the club's team code, display name, and LSC
// (Local Swimming Committee) code. There is exactly one settings row per org.
package orgsettings

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OrgSettings holds the export identity fields for a single org.
type OrgSettings struct {
	OrgID     string    `json:"org_id"`
	TeamCode  string    `json:"team_code"`
	TeamName  string    `json:"team_name"`
	LSCCode   string    `json:"lsc_code"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// Updates carries the mutable fields that may be patched. A nil pointer leaves
// the existing value unchanged.
type Updates struct {
	TeamCode *string `json:"team_code,omitempty"`
	TeamName *string `json:"team_name,omitempty"`
	LSCCode  *string `json:"lsc_code,omitempty"`
}

// Service manages org settings with an in-memory write-through cache.
type Service struct {
	mu       sync.RWMutex
	repo     *Repository
	settings map[string]*OrgSettings
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
	current.UpdatedAt = now
	current.UpdatedBy = updatedBy

	if s.repo != nil {
		if err := s.repo.Upsert(context.Background(), current); err != nil {
			return nil, err
		}
		return s.repo.Get(context.Background(), orgID)
	}
	s.mu.Lock()
	s.settings[orgID] = current
	s.mu.Unlock()
	cp := *current
	return &cp, nil
}
