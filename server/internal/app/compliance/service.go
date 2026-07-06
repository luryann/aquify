//  PROPRIETARY AND CONFIDENTIAL
//  COPYRIGHT (C) 2026 RYAN LU. ALL RIGHTS RESERVED.
//
//  THIS SOURCE FILE IS PART OF A PROPRIETARY, CLOSED-SOURCE SOFTWARE
//  SYSTEM OWNED EXCLUSIVELY BY RYAN LU. THIS FILE AND ALL ASSOCIATED
//  SOURCE CODE, OBJECT CODE, MODULES, COMPONENTS, DOCUMENTATION, AND
//  RELATED MATERIALS ARE CONFIDENTIAL AND PROPRIETARY INTELLECTUAL
//  PROPERTY OF RYAN LU.
//
//  USE OF THIS SOFTWARE IS PERMITTED ONLY FOR PERSONS OR ENTITIES
//  EXPRESSLY AUTHORIZED BY RYAN LU AND ONLY AFTER ACCEPTANCE OF THE
//  APPLICABLE LICENSE TERMS. DARE AQUATICS AND OTHER TEAMS OR
//  ORGANIZATIONS MAY USE THIS SOFTWARE ONLY IF SEPARATELY APPROVED BY
//  RYAN LU.
//
//  NO COPYING, MODIFICATION, DISTRIBUTION, HOSTING, DISCLOSURE,
//  REVERSE ENGINEERING, DECOMPILATION, OR OTHER USE IS PERMITTED
//  EXCEPT AS EXPRESSLY AUTHORIZED IN WRITING BY RYAN LU.
//
//  ALL RIGHTS NOT EXPRESSLY GRANTED ARE RESERVED BY RYAN LU.

// Package compliance provides the in-memory service for GDPR/compliance
// household data-export and data-deletion request management.
//
// Production note: all data is held in-memory maps protected by a RWMutex.
// Replace with sqlc/pgx queries when the DB layer is wired.
package compliance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	ErrExportNotFound    = errors.New("data export request not found")
	ErrDeletionNotFound  = errors.New("data deletion request not found")
	ErrForbidden         = errors.New("forbidden")
	ErrInvalidTransition = errors.New("invalid status transition")
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// DataExportRequest represents a household's request for a copy of their data.
type DataExportRequest struct {
	ID          string `json:"id"`
	OrgID       string `json:"org_id"`
	HouseholdID string `json:"household_id"`
	RequestedBy string `json:"requested_by"`
	// Status values: pending | processing | ready | failed | expired
	Status      string     `json:"status"`
	DownloadURL string     `json:"download_url,omitempty"`
	RequestedAt time.Time  `json:"requested_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// DataDeletionRequest represents a household's right-to-erasure request.
type DataDeletionRequest struct {
	ID          string `json:"id"`
	OrgID       string `json:"org_id"`
	HouseholdID string `json:"household_id"`
	RequestedBy string `json:"requested_by"`
	// Status values: pending | reviewing | approved | processing | completed | rejected | failed
	Status      string     `json:"status"`
	Reason      string     `json:"reason,omitempty"`
	ReviewNotes string     `json:"review_notes,omitempty"`
	RequestedAt time.Time  `json:"requested_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// ExpiresAt is the review deadline (now + deletionReviewDays). Admin must act
	// on the request before this date.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service manages compliance data-export and data-deletion requests.
type Service struct {
	mu        sync.RWMutex
	repo      *Repository
	exports   map[string]*DataExportRequest
	deletions map[string]*DataDeletionRequest

	// exportTTLDays is how many days until an export download link expires.
	exportTTLDays int
	// deletionReviewDays is how many days an admin has to review a deletion request.
	deletionReviewDays int
}

// NewService returns an empty Service with default settings.
func NewService() *Service {
	return NewServiceWithConfig(30, 30)
}

// NewServiceWithConfig returns a Service configured with custom TTL values.
// exportTTLDays controls how long a data-export download link is valid.
// deletionReviewDays controls the review deadline for data-deletion requests.
func NewServiceWithConfig(exportTTLDays, deletionReviewDays int) *Service {
	return &Service{
		exports:            make(map[string]*DataExportRequest),
		deletions:          make(map[string]*DataDeletionRequest),
		exportTTLDays:      exportTTLDays,
		deletionReviewDays: deletionReviewDays,
	}
}

// NewServiceWithPool creates a DB-backed compliance Service and loads existing
// data from Postgres into memory on startup.
func NewServiceWithPool(pool *pgxpool.Pool, exportTTLDays, deletionReviewDays int) *Service {
	s := NewServiceWithConfig(exportTTLDays, deletionReviewDays)
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("compliance: failed to load from DB", "err", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	exports, deletions, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range exports {
		s.exports[e.ID] = e
	}
	for _, d := range deletions {
		s.deletions[d.ID] = d
	}
	return nil
}

func (s *Service) genID() string {
	if s.repo != nil {
		return uuid.New().String()
	}
	id, err := newID()
	if err != nil {
		panic(fmt.Errorf("secure random unavailable for compliance ID generation: %w", err))
	}
	return id
}

// ─── Data Export Requests ─────────────────────────────────────────────────────

// CreateDataExportRequest opens a new pending data-export request for a household.
func (s *Service) CreateDataExportRequest(orgID, householdID, requestedBy string) (*DataExportRequest, error) {
	id := s.genID()
	now := time.Now().UTC()
	expiresAt := now.AddDate(0, 0, s.exportTTLDays)
	req := &DataExportRequest{
		ID:          id,
		OrgID:       orgID,
		HouseholdID: householdID,
		RequestedBy: requestedBy,
		Status:      "pending",
		RequestedAt: now,
		ExpiresAt:   &expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if s.repo != nil {
		if err := s.repo.UpsertDataExportRequest(context.Background(), req); err != nil {
			slog.Error("compliance: persist data export request", "err", err)
		}
	}
	s.mu.Lock()
	s.exports[id] = req
	s.mu.Unlock()
	return req, nil
}

// GetDataExportRequest returns the export request identified by requestID.
// Callers must enforce org-scope and ownership checks before calling.
func (s *Service) GetDataExportRequest(orgID, requestID string) (*DataExportRequest, error) {
	s.mu.RLock()
	req, ok := s.exports[requestID]
	s.mu.RUnlock()
	if !ok || req.OrgID != orgID {
		return nil, ErrExportNotFound
	}
	cp := *req
	return &cp, nil
}

// ListDataExportRequests returns all export requests for an org (or a specific household).
// Pass householdID="" to list all requests for the org.
func (s *Service) ListDataExportRequests(orgID, householdID string) ([]DataExportRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []DataExportRequest
	for _, req := range s.exports {
		if req.OrgID != orgID {
			continue
		}
		if householdID != "" && req.HouseholdID != householdID {
			continue
		}
		out = append(out, *req)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RequestedAt.After(out[j].RequestedAt)
	})
	if out == nil {
		out = []DataExportRequest{}
	}
	return out, nil
}

func (s *Service) MarkDataExportProcessing(orgID, requestID string) (*DataExportRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.exports[requestID]
	if !ok || req.OrgID != orgID {
		return nil, ErrExportNotFound
	}
	if req.Status != "pending" && req.Status != "failed" {
		return nil, ErrInvalidTransition
	}
	cp := *req
	cp.Status = "processing"
	cp.UpdatedAt = time.Now().UTC()
	if s.repo != nil {
		if err := s.repo.UpsertDataExportRequest(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.exports[requestID] = &cp
	return &cp, nil
}

func (s *Service) CompleteDataExport(orgID, requestID, downloadURL string, expiresAt time.Time) (*DataExportRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.exports[requestID]
	if !ok || req.OrgID != orgID {
		return nil, ErrExportNotFound
	}
	if req.Status != "processing" {
		return nil, ErrInvalidTransition
	}
	now := time.Now().UTC()
	cp := *req
	cp.Status = "ready"
	cp.DownloadURL = strings.TrimSpace(downloadURL)
	cp.CompletedAt = &now
	cp.ExpiresAt = &expiresAt
	cp.UpdatedAt = now
	if s.repo != nil {
		if err := s.repo.UpsertDataExportRequest(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.exports[requestID] = &cp
	return &cp, nil
}

func (s *Service) FailDataExport(orgID, requestID string) (*DataExportRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.exports[requestID]
	if !ok || req.OrgID != orgID {
		return nil, ErrExportNotFound
	}
	if req.Status != "processing" {
		return nil, ErrInvalidTransition
	}
	cp := *req
	cp.Status = "failed"
	cp.UpdatedAt = time.Now().UTC()
	if s.repo != nil {
		if err := s.repo.UpsertDataExportRequest(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.exports[requestID] = &cp
	return &cp, nil
}

// ─── Data Deletion Requests ───────────────────────────────────────────────────

// CreateDataDeletionRequest opens a new pending data-deletion request.
func (s *Service) CreateDataDeletionRequest(orgID, householdID, requestedBy, reason string) (*DataDeletionRequest, error) {
	id := s.genID()
	now := time.Now().UTC()
	reviewDeadline := now.AddDate(0, 0, s.deletionReviewDays)
	req := &DataDeletionRequest{
		ID:          id,
		OrgID:       orgID,
		HouseholdID: householdID,
		RequestedBy: requestedBy,
		Status:      "pending",
		Reason:      strings.TrimSpace(reason),
		RequestedAt: now,
		ExpiresAt:   &reviewDeadline,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if s.repo != nil {
		if err := s.repo.UpsertDataDeletionRequest(context.Background(), req); err != nil {
			slog.Error("compliance: persist data deletion request", "err", err)
		}
	}
	s.mu.Lock()
	s.deletions[id] = req
	s.mu.Unlock()
	return req, nil
}

// GetDataDeletionRequest returns the deletion request identified by requestID.
func (s *Service) GetDataDeletionRequest(orgID, requestID string) (*DataDeletionRequest, error) {
	s.mu.RLock()
	req, ok := s.deletions[requestID]
	s.mu.RUnlock()
	if !ok || req.OrgID != orgID {
		return nil, ErrDeletionNotFound
	}
	cp := *req
	return &cp, nil
}

// ListDataDeletionRequests returns all deletion requests for an org.
// Pass householdID="" to list all.
func (s *Service) ListDataDeletionRequests(orgID, householdID string) ([]DataDeletionRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []DataDeletionRequest
	for _, req := range s.deletions {
		if req.OrgID != orgID {
			continue
		}
		if householdID != "" && req.HouseholdID != householdID {
			continue
		}
		out = append(out, *req)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RequestedAt.After(out[j].RequestedAt)
	})
	if out == nil {
		out = []DataDeletionRequest{}
	}
	return out, nil
}

func (s *Service) MarkDataDeletionProcessing(orgID, requestID string) (*DataDeletionRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.deletions[requestID]
	if !ok || req.OrgID != orgID {
		return nil, ErrDeletionNotFound
	}
	if req.Status != "pending" && req.Status != "approved" && req.Status != "failed" {
		return nil, ErrInvalidTransition
	}
	cp := *req
	cp.Status = "processing"
	cp.UpdatedAt = time.Now().UTC()
	if s.repo != nil {
		if err := s.repo.UpsertDataDeletionRequest(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.deletions[requestID] = &cp
	return &cp, nil
}

func (s *Service) CompleteDataDeletion(orgID, requestID, reviewNotes string) (*DataDeletionRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.deletions[requestID]
	if !ok || req.OrgID != orgID {
		return nil, ErrDeletionNotFound
	}
	if req.Status != "processing" {
		return nil, ErrInvalidTransition
	}
	now := time.Now().UTC()
	cp := *req
	cp.Status = "completed"
	cp.ReviewNotes = strings.TrimSpace(reviewNotes)
	cp.CompletedAt = &now
	cp.UpdatedAt = now
	if s.repo != nil {
		if err := s.repo.UpsertDataDeletionRequest(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.deletions[requestID] = &cp
	return &cp, nil
}

func (s *Service) FailDataDeletion(orgID, requestID, reviewNotes string) (*DataDeletionRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.deletions[requestID]
	if !ok || req.OrgID != orgID {
		return nil, ErrDeletionNotFound
	}
	if req.Status != "processing" {
		return nil, ErrInvalidTransition
	}
	cp := *req
	cp.Status = "failed"
	cp.ReviewNotes = strings.TrimSpace(reviewNotes)
	cp.UpdatedAt = time.Now().UTC()
	if s.repo != nil {
		if err := s.repo.UpsertDataDeletionRequest(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.deletions[requestID] = &cp
	return &cp, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
