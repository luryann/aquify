// Package documents manages the document center: document type definitions and
// household/swimmer document submission records with verification workflows.
package documents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrTypeNotFound       = errors.New("document type not found")
	ErrSubmissionNotFound = errors.New("document submission not found")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrDuplicateName      = errors.New("a document type with this name already exists for the org")
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// DocumentType defines a category of document that households must provide.
type DocumentType struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	Name         string    `json:"name"`
	Category     string    `json:"category"` // waiver | medical | insurance | release | registration | other
	Description  string    `json:"description,omitempty"`
	IsRequired   bool      `json:"is_required"`
	AppliesTo    string    `json:"applies_to"`              // swimmer | household | all
	ExpiryMonths *int      `json:"expiry_months,omitempty"` // nil = never expires
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    string    `json:"created_by"`
	UpdatedBy    string    `json:"updated_by"`
}

// DocumentSubmission is a single household/swimmer document record.
type DocumentSubmission struct {
	ID               string `json:"id"`
	OrgID            string `json:"org_id"`
	DocumentTypeID   string `json:"document_type_id"`
	DocumentTypeName string `json:"document_type_name,omitempty"` // denormalized for UI
	HouseholdID      string `json:"household_id"`
	SwimmerID        string `json:"swimmer_id,omitempty"` // empty when applies_to=household
	// Status: pending | submitted | verified | rejected | expired
	Status         string     `json:"status"`
	FileObjectKey  string     `json:"file_object_key,omitempty"`
	FileName       string     `json:"file_name,omitempty"`
	FileSizeBytes  int64      `json:"file_size_bytes,omitempty"`
	FileMIME       string     `json:"file_mime,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	ReviewNotes    string     `json:"review_notes,omitempty"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty"`
	VerifiedAt     *time.Time `json:"verified_at,omitempty"`
	VerifiedBy     string     `json:"verified_by,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	ReminderSentAt *time.Time `json:"reminder_sent_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CreatedBy      string     `json:"created_by"`
	UpdatedBy      string     `json:"updated_by"`
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service manages document types and submissions.
type Service struct {
	mu          sync.RWMutex
	repo        *Repository
	docTypes    map[string]*DocumentType
	submissions map[string]*DocumentSubmission
}

// NewService returns an empty in-memory Service.
func NewService() *Service {
	return &Service{
		docTypes:    make(map[string]*DocumentType),
		submissions: make(map[string]*DocumentSubmission),
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
		slog.Error("documents: failed to load from DB", "err", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	types, subs, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range types {
		s.docTypes[t.ID] = t
	}
	for _, sub := range subs {
		s.submissions[sub.ID] = sub
	}
	return nil
}

// ─── Document Types ───────────────────────────────────────────────────────────

// CreateDocumentType creates a new document type definition.
func (s *Service) CreateDocumentType(orgID, name, category, description, appliesTo string, isRequired, isActive bool, expiryMonths *int, createdBy string) (*DocumentType, error) {
	trimmedName := strings.TrimSpace(name)

	// In-memory duplicate name check (org-scoped)
	s.mu.RLock()
	for _, dt := range s.docTypes {
		if dt.OrgID == orgID && strings.EqualFold(dt.Name, trimmedName) {
			s.mu.RUnlock()
			return nil, ErrDuplicateName
		}
	}
	s.mu.RUnlock()

	id := uuid.NewString()
	now := time.Now().UTC()
	if category == "" {
		category = "other"
	}
	if appliesTo == "" {
		appliesTo = "swimmer"
	}
	dt := &DocumentType{
		ID:           id,
		OrgID:        orgID,
		Name:         trimmedName,
		Category:     category,
		Description:  strings.TrimSpace(description),
		IsRequired:   isRequired,
		AppliesTo:    appliesTo,
		ExpiryMonths: expiryMonths,
		IsActive:     isActive,
		CreatedAt:    now,
		UpdatedAt:    now,
		CreatedBy:    createdBy,
		UpdatedBy:    createdBy,
	}
	if s.repo != nil {
		if err := s.repo.UpsertDocumentType(context.Background(), dt); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	s.docTypes[id] = dt
	s.mu.Unlock()
	return dt, nil
}

// UpdateDocumentType updates mutable fields on a document type.
func (s *Service) UpdateDocumentType(orgID, typeID string, updates map[string]interface{}, updatedBy string) (*DocumentType, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dt, ok := s.docTypes[typeID]
	if !ok || dt.OrgID != orgID {
		return nil, ErrTypeNotFound
	}
	cp := *dt
	if v, ok := updates["name"].(string); ok && v != "" {
		newName := strings.TrimSpace(v)
		// Check for duplicate name within the org (excluding the current record).
		for _, existing := range s.docTypes {
			if existing.ID != typeID && existing.OrgID == orgID && strings.EqualFold(existing.Name, newName) {
				return nil, ErrDuplicateName
			}
		}
		cp.Name = newName
	}
	if v, ok := updates["category"].(string); ok && v != "" {
		cp.Category = v
	}
	if v, ok := updates["description"].(string); ok {
		cp.Description = strings.TrimSpace(v)
	}
	if v, ok := updates["is_required"].(bool); ok {
		cp.IsRequired = v
	}
	if v, ok := updates["applies_to"].(string); ok && v != "" {
		cp.AppliesTo = v
	}
	if v, ok := updates["expiry_months"].(*int); ok {
		cp.ExpiryMonths = v
	}
	if v, ok := updates["is_active"].(bool); ok {
		cp.IsActive = v
	}
	cp.UpdatedAt = time.Now().UTC()
	cp.UpdatedBy = updatedBy

	if s.repo != nil {
		if err := s.repo.UpsertDocumentType(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.docTypes[typeID] = &cp
	return &cp, nil
}

// DeleteDocumentType soft-deletes a document type.
func (s *Service) DeleteDocumentType(orgID, typeID, deletedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dt, ok := s.docTypes[typeID]
	if !ok || dt.OrgID != orgID {
		return ErrTypeNotFound
	}
	if s.repo != nil {
		if err := s.repo.SoftDeleteDocumentType(context.Background(), typeID, deletedBy); err != nil {
			return err
		}
	}
	delete(s.docTypes, typeID)
	return nil
}

// ListDocumentTypes returns all (active) document types for an org.
// Pass activeOnly=true to filter to is_active=true records.
func (s *Service) ListDocumentTypes(orgID string, activeOnly bool) ([]DocumentType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []DocumentType
	for _, dt := range s.docTypes {
		if dt.OrgID != orgID {
			continue
		}
		if activeOnly && !dt.IsActive {
			continue
		}
		out = append(out, *dt)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	if out == nil {
		out = []DocumentType{}
	}
	return out, nil
}

// ─── Document Submissions ─────────────────────────────────────────────────────

// CreateSubmission creates a pending document submission record.
func (s *Service) CreateSubmission(orgID, docTypeID, householdID, swimmerID, createdBy string) (*DocumentSubmission, error) {
	s.mu.RLock()
	dt, ok := s.docTypes[docTypeID]
	s.mu.RUnlock()
	if !ok || dt.OrgID != orgID {
		return nil, ErrTypeNotFound
	}

	id := uuid.NewString()
	now := time.Now().UTC()

	var expiresAt *time.Time
	if dt.ExpiryMonths != nil && *dt.ExpiryMonths > 0 {
		t := now.AddDate(0, *dt.ExpiryMonths, 0)
		expiresAt = &t
	}

	sub := &DocumentSubmission{
		ID:               id,
		OrgID:            orgID,
		DocumentTypeID:   docTypeID,
		DocumentTypeName: dt.Name,
		HouseholdID:      householdID,
		SwimmerID:        swimmerID,
		Status:           "pending",
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
		UpdatedAt:        now,
		CreatedBy:        createdBy,
		UpdatedBy:        createdBy,
	}
	if s.repo != nil {
		if err := s.repo.UpsertSubmission(context.Background(), sub); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	s.submissions[id] = sub
	s.mu.Unlock()
	return sub, nil
}

// AttachFile records the uploaded file metadata on a submission and transitions
// the status to "submitted".
func (s *Service) AttachFile(orgID, submissionID, fileObjectKey, fileName, fileMIME string, fileSizeBytes int64, updatedBy string) (*DocumentSubmission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, ok := s.submissions[submissionID]
	if !ok || sub.OrgID != orgID {
		return nil, ErrSubmissionNotFound
	}
	if sub.Status != "pending" && sub.Status != "rejected" {
		return nil, ErrInvalidTransition
	}
	cp := *sub
	cp.Status = "submitted"
	cp.FileObjectKey = fileObjectKey
	cp.FileName = fileName
	cp.FileMIME = fileMIME
	cp.FileSizeBytes = fileSizeBytes
	now := time.Now().UTC()
	cp.SubmittedAt = &now
	cp.UpdatedAt = now
	cp.UpdatedBy = updatedBy

	if s.repo != nil {
		if err := s.repo.UpsertSubmission(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.submissions[submissionID] = &cp
	return &cp, nil
}

// VerifySubmission transitions a submission to "verified".
func (s *Service) VerifySubmission(orgID, submissionID, verifiedBy, reviewNotes string) (*DocumentSubmission, error) {
	return s.reviewSubmission(orgID, submissionID, verifiedBy, "verified", reviewNotes)
}

// RejectSubmission records a reviewer rejecting a submitted document.
func (s *Service) RejectSubmission(orgID, submissionID, rejectedBy, reviewNotes string) (*DocumentSubmission, error) {
	return s.reviewSubmission(orgID, submissionID, rejectedBy, "rejected", reviewNotes)
}

// reviewSubmission records a reviewer's decision on a submitted document;
// verifying also stamps who verified it and when.
func (s *Service) reviewSubmission(orgID, submissionID, reviewer, status, reviewNotes string) (*DocumentSubmission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, ok := s.submissions[submissionID]
	if !ok || sub.OrgID != orgID {
		return nil, ErrSubmissionNotFound
	}
	if sub.Status != "submitted" {
		return nil, ErrInvalidTransition
	}
	now := time.Now().UTC()
	cp := *sub
	cp.Status = status
	cp.ReviewNotes = strings.TrimSpace(reviewNotes)
	if status == "verified" {
		cp.VerifiedAt = &now
		cp.VerifiedBy = reviewer
	}
	cp.UpdatedAt = now
	cp.UpdatedBy = reviewer

	if s.repo != nil {
		if err := s.repo.UpsertSubmission(context.Background(), &cp); err != nil {
			return nil, err
		}
	}
	s.submissions[submissionID] = &cp
	return &cp, nil
}

// GetSubmission returns a single document submission.
func (s *Service) GetSubmission(orgID, submissionID string) (*DocumentSubmission, error) {
	s.mu.RLock()
	sub, ok := s.submissions[submissionID]
	s.mu.RUnlock()
	if !ok || sub.OrgID != orgID {
		return nil, ErrSubmissionNotFound
	}
	cp := *sub
	// Denormalize type name.
	s.mu.RLock()
	if dt, ok := s.docTypes[sub.DocumentTypeID]; ok {
		cp.DocumentTypeName = dt.Name
	}
	s.mu.RUnlock()
	return &cp, nil
}

// ListSubmissions returns submissions filtered by orgID and optionally householdID.
func (s *Service) ListSubmissions(orgID, householdID string) ([]DocumentSubmission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []DocumentSubmission
	for _, sub := range s.submissions {
		if sub.OrgID != orgID {
			continue
		}
		if householdID != "" && sub.HouseholdID != householdID {
			continue
		}
		cp := *sub
		if dt, ok := s.docTypes[sub.DocumentTypeID]; ok {
			cp.DocumentTypeName = dt.Name
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if out == nil {
		out = []DocumentSubmission{}
	}
	return out, nil
}

// EraseHouseholdData hard-deletes every document submission for a household
// as part of an approved compliance right-to-erasure request (a soft delete
// would leave file names/notes/reviewer notes queryable). Returns the
// non-empty file object keys so the caller can purge them from object
// storage.
func (s *Service) EraseHouseholdData(ctx context.Context, orgID, householdID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var objectKeys []string
	for id, sub := range s.submissions {
		if sub.OrgID != orgID || sub.HouseholdID != householdID {
			continue
		}
		if sub.FileObjectKey != "" {
			objectKeys = append(objectKeys, sub.FileObjectKey)
		}
		delete(s.submissions, id)
	}

	if s.repo != nil {
		if err := s.repo.DeleteSubmissionsByHousehold(ctx, orgID, householdID); err != nil {
			return objectKeys, fmt.Errorf("erase household document submissions: %w", err)
		}
	}

	return objectKeys, nil
}
