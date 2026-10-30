// Package seasons manages club season records (dates, status, and goals).
package seasons

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound      = errors.New("season not found")
	ErrInvalidStatus = errors.New("season status must be active, upcoming, or completed")
)

var validStatuses = map[string]bool{
	"active":    true,
	"upcoming":  true,
	"completed": true,
}

// ─── Domain types ─────────────────────────────────────────────────────────────

// Season represents a single club season.
type Season struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Name      string    `json:"name"`
	StartDate string    `json:"start_date"` // YYYY-MM-DD, "" when unset
	EndDate   string    `json:"end_date"`   // YYYY-MM-DD, "" when unset
	Status    string    `json:"status"`     // active | upcoming | completed
	Goals     string    `json:"goals"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`
}

// SeasonUpdates carries the mutable fields that may be patched on a Season.
type SeasonUpdates struct {
	Name      *string `json:"name,omitempty"`
	StartDate *string `json:"start_date,omitempty"`
	EndDate   *string `json:"end_date,omitempty"`
	Status    *string `json:"status,omitempty"`
	Goals     *string `json:"goals,omitempty"`
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service manages season records with an in-memory write-through cache.
type Service struct {
	mu      sync.RWMutex
	repo    *Repository
	seasons map[string]*Season
}

// NewService returns an empty in-memory Service (no DB backing).
func NewService() *Service {
	return &Service{
		seasons: make(map[string]*Season),
	}
}

// NewServiceWithPool creates a DB-backed Service. Postgres is canonical: every
// read and write goes to the database, so the in-memory map is unused.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	return s
}

// ─── CRUD ─────────────────────────────────────────────────────────────────────

// Create persists a new season record.
func (s *Service) Create(orgID, name, startDate, endDate, status, goals, createdBy string) (*Season, error) {
	if !validStatuses[status] {
		return nil, ErrInvalidStatus
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	season := &Season{
		ID:        id,
		OrgID:     orgID,
		Name:      name,
		StartDate: startDate,
		EndDate:   endDate,
		Status:    status,
		Goals:     goals,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: createdBy,
		UpdatedBy: createdBy,
	}
	if s.repo != nil {
		if err := s.repo.Upsert(context.Background(), season); err != nil {
			return nil, err
		}
		return s.repo.GetByID(context.Background(), orgID, id)
	}
	s.mu.Lock()
	s.seasons[id] = season
	s.mu.Unlock()
	cp := *season
	return &cp, nil
}

// List returns all seasons for an org, sorted newest-first by start date.
func (s *Service) List(orgID string) ([]*Season, error) {
	if s.repo != nil {
		seasons, err := s.repo.ListByOrg(context.Background(), orgID)
		if err != nil {
			return nil, err
		}
		if seasons == nil {
			seasons = []*Season{}
		}
		return seasons, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Season
	for _, season := range s.seasons {
		if season.OrgID != orgID {
			continue
		}
		cp := *season
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartDate != out[j].StartDate {
			return out[i].StartDate > out[j].StartDate
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if out == nil {
		out = []*Season{}
	}
	return out, nil
}

// Get returns a single season by ID, scoped to an org.
func (s *Service) Get(orgID, id string) (*Season, error) {
	if s.repo != nil {
		return s.repo.GetByID(context.Background(), orgID, id)
	}

	s.mu.RLock()
	season, ok := s.seasons[id]
	s.mu.RUnlock()
	if !ok || season.OrgID != orgID {
		return nil, ErrNotFound
	}
	cp := *season
	return &cp, nil
}

func applyUpdates(season *Season, updates SeasonUpdates) error {
	if updates.Name != nil {
		season.Name = *updates.Name
	}
	if updates.StartDate != nil {
		season.StartDate = *updates.StartDate
	}
	if updates.EndDate != nil {
		season.EndDate = *updates.EndDate
	}
	if updates.Status != nil {
		if !validStatuses[*updates.Status] {
			return ErrInvalidStatus
		}
		season.Status = *updates.Status
	}
	if updates.Goals != nil {
		season.Goals = *updates.Goals
	}
	return nil
}

// Update applies SeasonUpdates to an existing season.
func (s *Service) Update(orgID, id string, updates SeasonUpdates, updatedBy string) (*Season, error) {
	if s.repo != nil {
		return s.repo.UpdateTx(context.Background(), orgID, id, func(season *Season) error {
			if err := applyUpdates(season, updates); err != nil {
				return err
			}
			season.UpdatedAt = time.Now().UTC()
			season.UpdatedBy = updatedBy
			return nil
		})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	season, ok := s.seasons[id]
	if !ok || season.OrgID != orgID {
		return nil, ErrNotFound
	}
	cp := *season
	if err := applyUpdates(&cp, updates); err != nil {
		return nil, err
	}
	cp.UpdatedAt = time.Now().UTC()
	cp.UpdatedBy = updatedBy
	s.seasons[id] = &cp
	return &cp, nil
}

// Delete permanently removes a season record.
func (s *Service) Delete(orgID, id string) error {
	if s.repo != nil {
		return s.repo.Delete(context.Background(), orgID, id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	season, ok := s.seasons[id]
	if !ok || season.OrgID != orgID {
		return ErrNotFound
	}
	delete(s.seasons, id)
	return nil
}
