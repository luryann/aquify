// Package training manages practice sessions and the team training schedule.
package training

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrNotFound          = errors.New("practice not found")
	ErrInvalidType       = errors.New("practice type must be water, dryland, video, or rest")
	ErrInvalidAttendance = errors.New("attendance status must be present, absent, or excused")
)

var validTypes = map[string]bool{
	"water":   true,
	"dryland": true,
	"video":   true,
	"rest":    true,
}

var validAttendanceStatuses = map[string]bool{
	"present": true,
	"absent":  true,
	"excused": true,
}

// ─── Domain types ─────────────────────────────────────────────────────────────

// PracticeStrokes holds per-stroke yardage breakdown for a water practice.
type PracticeStrokes struct {
	Free   int `json:"free"`
	Back   int `json:"back"`
	Breast int `json:"breast"`
	Fly    int `json:"fly"`
	IM     int `json:"im"`
}

// Practice represents a single logged training session.
type Practice struct {
	ID            string          `json:"id"`
	OrgID         string          `json:"org_id"`
	Date          string          `json:"date"` // YYYY-MM-DD
	GroupID       string          `json:"group_id,omitempty"`
	Type          string          `json:"type"` // water | dryland | video | rest
	Description   string          `json:"description"`
	YardageTarget int             `json:"yardage_target"`
	Attendance    *int            `json:"attendance,omitempty"`
	Strokes       PracticeStrokes `json:"strokes"`
	Notes         string          `json:"notes,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CreatedBy     string          `json:"created_by"`
	UpdatedBy     string          `json:"updated_by"`
}

// PracticeAttendance records one swimmer's attendance status for a practice.
type PracticeAttendance struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	PracticeID string    `json:"practice_id"`
	SwimmerID  string    `json:"swimmer_id"`
	Status     string    `json:"status"` // present | absent | excused
	MarkedAt   time.Time `json:"marked_at"`
	MarkedBy   string    `json:"marked_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PracticeUpdates carries the mutable fields that may be patched on a Practice.
type PracticeUpdates struct {
	Date          *string          `json:"date,omitempty"`
	GroupID       *string          `json:"group_id,omitempty"`
	Type          *string          `json:"type,omitempty"`
	Description   *string          `json:"description,omitempty"`
	YardageTarget *int             `json:"yardage_target,omitempty"`
	Attendance    *int             `json:"attendance,omitempty"`
	Strokes       *PracticeStrokes `json:"strokes,omitempty"`
	Notes         *string          `json:"notes,omitempty"`
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service manages practice records with an in-memory write-through cache.
type Service struct {
	mu         sync.RWMutex
	repo       *Repository
	practices  map[string]*Practice
	attendance map[string]map[string]*PracticeAttendance // practiceID -> swimmerID -> record
}

// NewService returns an empty in-memory Service (no DB backing).
func NewService() *Service {
	return &Service{
		practices:  make(map[string]*Practice),
		attendance: make(map[string]map[string]*PracticeAttendance),
	}
}

// NewServiceWithPool creates a DB-backed Service and loads existing records
// from Postgres into memory on startup.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("training: failed to load from DB", "err", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	practices, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.practices = make(map[string]*Practice, len(practices))
	for _, p := range practices {
		s.practices[p.ID] = p
	}
	return nil
}

func newID() string {
	return uuid.New().String()
}

// ─── CRUD ─────────────────────────────────────────────────────────────────────

// Create persists a new practice record.
func (s *Service) Create(
	orgID, date, groupID, practiceType, description string,
	yardageTarget int,
	attendance *int,
	strokes PracticeStrokes,
	notes, createdBy string,
) (*Practice, error) {
	if !validTypes[practiceType] {
		return nil, ErrInvalidType
	}
	id := newID()
	now := time.Now().UTC()
	p := &Practice{
		ID:            id,
		OrgID:         orgID,
		Date:          date,
		GroupID:       groupID,
		Type:          practiceType,
		Description:   description,
		YardageTarget: yardageTarget,
		Attendance:    attendance,
		Strokes:       strokes,
		Notes:         notes,
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     createdBy,
		UpdatedBy:     createdBy,
	}
	if s.repo != nil {
		if err := s.repo.Upsert(context.Background(), p); err != nil {
			return nil, err
		}
		return s.repo.GetByID(context.Background(), orgID, id)
	}
	s.mu.Lock()
	s.practices[id] = p
	s.mu.Unlock()
	return p, nil
}

// List returns all practices for an org, sorted newest-first by date.
func (s *Service) List(orgID string) ([]*Practice, error) {
	if s.repo != nil {
		practices, err := s.repo.ListByOrg(context.Background(), orgID)
		if err != nil {
			return nil, err
		}
		if practices == nil {
			practices = []*Practice{}
		}
		return practices, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Practice
	for _, p := range s.practices {
		if p.OrgID != orgID {
			continue
		}
		cp := *p
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if out == nil {
		out = []*Practice{}
	}
	return out, nil
}

// Get returns a single practice by ID, scoped to an org.
func (s *Service) Get(orgID, id string) (*Practice, error) {
	if s.repo != nil {
		return s.repo.GetByID(context.Background(), orgID, id)
	}

	s.mu.RLock()
	p, ok := s.practices[id]
	s.mu.RUnlock()
	if !ok || p.OrgID != orgID {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

// Update applies PracticeUpdates to an existing practice.
func (s *Service) Update(orgID, id string, updates PracticeUpdates, updatedBy string) (*Practice, error) {
	if s.repo != nil {
		p, err := s.repo.GetByID(context.Background(), orgID, id)
		if err != nil {
			return nil, err
		}
		if updates.Date != nil {
			p.Date = *updates.Date
		}
		if updates.GroupID != nil {
			p.GroupID = *updates.GroupID
		}
		if updates.Type != nil {
			if !validTypes[*updates.Type] {
				return nil, ErrInvalidType
			}
			p.Type = *updates.Type
		}
		if updates.Description != nil {
			p.Description = *updates.Description
		}
		if updates.YardageTarget != nil {
			p.YardageTarget = *updates.YardageTarget
		}
		if updates.Attendance != nil {
			p.Attendance = updates.Attendance
		}
		if updates.Strokes != nil {
			p.Strokes = *updates.Strokes
		}
		if updates.Notes != nil {
			p.Notes = *updates.Notes
		}
		p.UpdatedAt = time.Now().UTC()
		p.UpdatedBy = updatedBy
		if err := s.repo.Upsert(context.Background(), p); err != nil {
			return nil, err
		}
		return s.repo.GetByID(context.Background(), orgID, id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.practices[id]
	if !ok || p.OrgID != orgID {
		return nil, ErrNotFound
	}
	cp := *p
	if updates.Date != nil {
		cp.Date = *updates.Date
	}
	if updates.GroupID != nil {
		cp.GroupID = *updates.GroupID
	}
	if updates.Type != nil {
		if !validTypes[*updates.Type] {
			return nil, ErrInvalidType
		}
		cp.Type = *updates.Type
	}
	if updates.Description != nil {
		cp.Description = *updates.Description
	}
	if updates.YardageTarget != nil {
		cp.YardageTarget = *updates.YardageTarget
	}
	if updates.Attendance != nil {
		cp.Attendance = updates.Attendance
	}
	if updates.Strokes != nil {
		cp.Strokes = *updates.Strokes
	}
	if updates.Notes != nil {
		cp.Notes = *updates.Notes
	}
	cp.UpdatedAt = time.Now().UTC()
	cp.UpdatedBy = updatedBy

	if s.repo != nil {
		if err := s.repo.Upsert(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.practices[id] = &cp
	return &cp, nil
}

// Delete permanently removes a practice record. The delete is always scoped
// to orgID (both in the DB predicate and the in-memory cache lookup) so a
// caller cannot delete another org's practice by ID alone.
func (s *Service) Delete(orgID, id string) error {
	if s.repo != nil {
		if err := s.repo.Delete(context.Background(), orgID, id); err != nil {
			return err
		}
		s.mu.Lock()
		delete(s.practices, id)
		s.mu.Unlock()
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.practices[id]
	if !ok || p.OrgID != orgID {
		return ErrNotFound
	}
	delete(s.practices, id)
	return nil
}

// ─── Attendance ───────────────────────────────────────────────────────────────

// SetAttendance upserts one swimmer's attendance status for a practice.
func (s *Service) SetAttendance(orgID, practiceID, swimmerID, status, markedBy string) (*PracticeAttendance, error) {
	if !validAttendanceStatuses[status] {
		return nil, ErrInvalidAttendance
	}
	if _, err := s.Get(orgID, practiceID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if s.repo != nil {
		rec, err := s.repo.UpsertAttendance(context.Background(), orgID, practiceID, swimmerID, status, markedBy, now)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		if s.attendance[practiceID] == nil {
			s.attendance[practiceID] = make(map[string]*PracticeAttendance)
		}
		s.attendance[practiceID][swimmerID] = rec
		s.mu.Unlock()
		return rec, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attendance[practiceID] == nil {
		s.attendance[practiceID] = make(map[string]*PracticeAttendance)
	}
	existing, ok := s.attendance[practiceID][swimmerID]
	rec := &PracticeAttendance{
		ID:         newID(),
		OrgID:      orgID,
		PracticeID: practiceID,
		SwimmerID:  swimmerID,
		Status:     status,
		MarkedAt:   now,
		MarkedBy:   markedBy,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if ok {
		rec.ID = existing.ID
		rec.CreatedAt = existing.CreatedAt
	}
	s.attendance[practiceID][swimmerID] = rec
	return rec, nil
}

// ListAttendance returns all recorded attendance rows for a practice.
func (s *Service) ListAttendance(orgID, practiceID string) ([]*PracticeAttendance, error) {
	if _, err := s.Get(orgID, practiceID); err != nil {
		return nil, err
	}
	if s.repo != nil {
		return s.repo.ListAttendanceForPractice(context.Background(), orgID, practiceID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*PracticeAttendance{}
	for _, rec := range s.attendance[practiceID] {
		cp := *rec
		out = append(out, &cp)
	}
	return out, nil
}
