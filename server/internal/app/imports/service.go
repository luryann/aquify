// Package imports provides the in-memory service for file import/export
// job management, conflict resolution, and import report retrieval.
package imports

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrForbidden         = errors.New("forbidden")
	ErrImportNotFound    = errors.New("import job not found")
	ErrConflictNotFound  = errors.New("match conflict not found")
	ErrInvalidFileType   = errors.New("unsupported file type; expected cl2, sd3, zip, or hy3")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrConflictNotOpen   = errors.New("conflict is not in open state; cannot be resolved")
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// FileImport represents a file import job.
type FileImport struct {
	ID                 string        `json:"id"`
	OrgID              string        `json:"org_id"`
	MeetID             string        `json:"meet_id,omitempty"`
	Filename           string        `json:"filename"`
	FileType           string        `json:"file_type"`
	Status             string        `json:"status"`
	StorageKey         string        `json:"storage_key,omitempty"`
	UploadURL          string        `json:"upload_url,omitempty"`
	UploadURLExpiresAt *time.Time    `json:"upload_url_expires_at,omitempty"`
	RowCount           int           `json:"row_count"`
	ErrorCount         int           `json:"error_count"`
	ConflictCount      int           `json:"conflict_count"`
	ImportReport       *ImportReport `json:"import_report,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	CreatedBy          string        `json:"created_by"`
	UpdatedBy          string        `json:"updated_by"`
}

// ImportReport is the summary stored on a completed import.
type ImportReport struct {
	TotalRows    int                `json:"total_rows"`
	MatchedRows  int                `json:"matched_rows"`
	ErrorRows    int                `json:"error_rows"`
	ConflictRows int                `json:"conflict_rows"`
	Errors       []ImportErrorEntry `json:"errors,omitempty"`
}

// ImportErrorEntry is a single parse/match error captured in the report.
type ImportErrorEntry struct {
	RowNumber    int    `json:"row_number,omitempty"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

// MatchConflict represents an identity-matching ambiguity found during import.
type MatchConflict struct {
	ID                  string     `json:"id"`
	ImportID            string     `json:"import_id"`
	OrgID               string     `json:"org_id"`
	RowNumber           int        `json:"row_number,omitempty"`
	RawData             string     `json:"raw_data,omitempty"`
	Status              string     `json:"status"`
	ConflictType        string     `json:"conflict_type"`
	CandidateSwimmerIDs []string   `json:"candidate_swimmer_ids"`
	ResolvedSwimmerID   string     `json:"resolved_swimmer_id,omitempty"`
	ResolverID          string     `json:"resolver_id,omitempty"`
	ResolutionNote      string     `json:"resolution_note,omitempty"`
	ResolvedAt          *time.Time `json:"resolved_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// ImportAuditEvent records privileged actions taken in the import domain.
type ImportAuditEvent struct {
	ID         string            `json:"id"`
	OrgID      string            `json:"org_id"`
	ActorID    string            `json:"actor_id"`
	Action     string            `json:"action"`
	Resource   string            `json:"resource"`
	ResourceID string            `json:"resource_id"`
	Meta       map[string]string `json:"meta,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// ─── Input types ─────────────────────────────────────────────────────────────

// RequestUploadURLInput is the input for requesting a presigned upload URL.
type RequestUploadURLInput struct {
	Filename string
	FileType string // cl2, sd3, zip (or inferred from filename)
	MeetID   string // optional
}

// ResolveConflictInput is the input for manually resolving a match conflict.
type ResolveConflictInput struct {
	ResolvedSwimmerID string
	ResolutionNote    string
}

// ─── Service ─────────────────────────────────────────────────────────────────

// Service manages file import jobs and match conflicts in memory.
type Service struct {
	mu        sync.RWMutex
	imports   map[string]*FileImport
	conflicts map[string]*MatchConflict
	store     storage.ObjectStorage
}

// NewService returns a new Service backed by a stub object storage client.
func NewService() *Service {
	return NewServiceWithStorage(storage.NewStubStorage(""))
}

// NewServiceWithStorage returns a new Service backed by the given storage client.
func NewServiceWithStorage(store storage.ObjectStorage) *Service {
	return &Service{
		imports:   make(map[string]*FileImport),
		conflicts: make(map[string]*MatchConflict),
		store:     store,
	}
}

// ─── RequestUploadURL ─────────────────────────────────────────────────────────

// RequestUploadURL creates a new file import record and returns a presigned
// upload URL.  Only owner, admin, and coach may request uploads.
func (s *Service) RequestUploadURL(principal *appmiddleware.Principal, orgID string, input RequestUploadURLInput) (*FileImport, *ImportAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}

	fileType := strings.ToLower(strings.TrimSpace(input.FileType))
	if fileType == "" {
		fileType = detectFileType(input.Filename)
	}
	if fileType != "cl2" && fileType != "sd3" && fileType != "zip" && fileType != "hy3" {
		return nil, nil, ErrInvalidFileType
	}

	id := "import_" + randomToken(12)
	now := time.Now().UTC()
	storageKey := fmt.Sprintf("imports/%s/%s/%s", orgID, id, input.Filename)

	const uploadTTL = 15 * time.Minute
	uploadURL, err := s.store.PresignedPutURL(storageKey, uploadTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("storage: presign put url: %w", err)
	}
	expiresAt := now.Add(uploadTTL)

	imp := &FileImport{
		ID:                 id,
		OrgID:              orgID,
		MeetID:             strings.TrimSpace(input.MeetID),
		Filename:           input.Filename,
		FileType:           fileType,
		Status:             "uploaded",
		StorageKey:         storageKey,
		UploadURL:          uploadURL,
		UploadURLExpiresAt: &expiresAt,
		CreatedAt:          now,
		UpdatedAt:          now,
		CreatedBy:          principal.UserID,
		UpdatedBy:          principal.UserID,
	}

	s.mu.Lock()
	s.imports[id] = imp
	s.mu.Unlock()

	audit := newAuditEvent(orgID, principal.UserID, "import.upload_url_requested", "file_import", id, map[string]string{
		"filename":  input.Filename,
		"file_type": fileType,
	})
	return imp, audit, nil
}

// ─── FinalizeImport ───────────────────────────────────────────────────────────

// FinalizeImport marks an import as received and simulates processing through
// the state machine.  In the stub, it immediately advances to
// conflicts_pending with synthetic data.  Only owner, admin, and coach may
// finalize.
func (s *Service) FinalizeImport(principal *appmiddleware.Principal, orgID, importID string) (*FileImport, *ImportAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	imp, ok := s.imports[importID]
	if !ok || imp.OrgID != orgID {
		return nil, nil, ErrImportNotFound
	}
	if imp.Status != "uploaded" {
		return nil, nil, fmt.Errorf("%w: cannot finalize an import with status %q", ErrInvalidTransition, imp.Status)
	}

	now := time.Now().UTC()

	// Stub: immediately advance to conflicts_pending with synthetic rows.
	imp.Status = "conflicts_pending"
	imp.RowCount = 10
	imp.ErrorCount = 1
	imp.ConflictCount = 1
	imp.UploadURL = "" // revoke the presigned URL reference once finalized
	imp.UploadURLExpiresAt = nil
	imp.UpdatedAt = now
	imp.UpdatedBy = principal.UserID
	imp.ImportReport = &ImportReport{
		TotalRows:    10,
		MatchedRows:  8,
		ErrorRows:    1,
		ConflictRows: 1,
		Errors: []ImportErrorEntry{
			{
				RowNumber:    3,
				ErrorCode:    "bad_event_code",
				ErrorMessage: "event code '999' is not defined in the meet",
			},
		},
	}

	// Seed one synthetic conflict.
	conflictID := "conflict_" + randomToken(12)
	conflict := &MatchConflict{
		ID:                  conflictID,
		ImportID:            importID,
		OrgID:               orgID,
		RowNumber:           7,
		RawData:             "SMITH,JOHN,2010-01-15,M,100FR,58.23",
		Status:              "open",
		ConflictType:        "ambiguous_match",
		CandidateSwimmerIDs: []string{},
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	s.conflicts[conflictID] = conflict

	audit := newAuditEvent(orgID, principal.UserID, "import.finalized", "file_import", importID, map[string]string{
		"status": imp.Status,
	})

	cp := *imp
	return &cp, audit, nil
}

// ─── ListImports ──────────────────────────────────────────────────────────────

// ListImports returns all import jobs for an org.
// Only owner, admin, and coach may list.
func (s *Service) ListImports(principal *appmiddleware.Principal, orgID string) ([]FileImport, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []FileImport
	for _, imp := range s.imports {
		if imp.OrgID == orgID {
			result = append(result, *imp)
		}
	}
	return result, nil
}

// ─── GetImport ────────────────────────────────────────────────────────────────

// GetImport returns a single import job by ID.
// Only owner, admin, and coach may get.
func (s *Service) GetImport(principal *appmiddleware.Principal, orgID, importID string) (*FileImport, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	imp, ok := s.imports[importID]
	if !ok || imp.OrgID != orgID {
		return nil, ErrImportNotFound
	}
	cp := *imp
	return &cp, nil
}

// ─── ListConflicts ────────────────────────────────────────────────────────────

// ListConflicts returns all match conflicts for a given import.
// Only owner, admin, and coach may list.
func (s *Service) ListConflicts(principal *appmiddleware.Principal, orgID, importID string) ([]MatchConflict, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	imp, ok := s.imports[importID]
	if !ok || imp.OrgID != orgID {
		return nil, ErrImportNotFound
	}
	_ = imp

	var result []MatchConflict
	for _, c := range s.conflicts {
		if c.ImportID == importID {
			result = append(result, *c)
		}
	}
	return result, nil
}

// ─── ResolveConflict ─────────────────────────────────────────────────────────

// ResolveConflict manually resolves a match conflict by assigning a swimmer.
// Only owner and admin may resolve conflicts.
func (s *Service) ResolveConflict(principal *appmiddleware.Principal, orgID, importID, conflictID string, input ResolveConflictInput) (*MatchConflict, *ImportAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin") {
		return nil, nil, ErrForbidden
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	imp, ok := s.imports[importID]
	if !ok || imp.OrgID != orgID {
		return nil, nil, ErrImportNotFound
	}

	conflict, ok := s.conflicts[conflictID]
	if !ok || conflict.ImportID != importID {
		return nil, nil, ErrConflictNotFound
	}
	if conflict.Status != "open" {
		return nil, nil, ErrConflictNotOpen
	}

	now := time.Now().UTC()
	conflict.Status = "resolved_manual"
	conflict.ResolvedSwimmerID = strings.TrimSpace(input.ResolvedSwimmerID)
	conflict.ResolverID = principal.UserID
	conflict.ResolutionNote = strings.TrimSpace(input.ResolutionNote)
	conflict.ResolvedAt = &now
	conflict.UpdatedAt = now

	// If all conflicts for this import are resolved, advance to applied.
	if s.allConflictsResolvedLocked(importID) {
		imp.Status = "applied"
		imp.UpdatedAt = now
		imp.UpdatedBy = principal.UserID
	}

	audit := newAuditEvent(orgID, principal.UserID, "import.conflict_resolved", "match_conflict", conflictID, map[string]string{
		"import_id":           importID,
		"resolved_swimmer_id": input.ResolvedSwimmerID,
	})

	cp := *conflict
	return &cp, audit, nil
}

// allConflictsResolvedLocked returns true if every conflict for importID is
// non-open.  Caller must hold s.mu (write or read lock).
func (s *Service) allConflictsResolvedLocked(importID string) bool {
	for _, c := range s.conflicts {
		if c.ImportID == importID && c.Status == "open" {
			return false
		}
	}
	return true
}

// ─── GetImportReport ─────────────────────────────────────────────────────────

// GetImportReport returns the full import record (including the report JSON)
// for a given import.  Only owner, admin, and coach may download.
func (s *Service) GetImportReport(principal *appmiddleware.Principal, orgID, importID string) (*FileImport, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	imp, ok := s.imports[importID]
	if !ok || imp.OrgID != orgID {
		return nil, ErrImportNotFound
	}
	cp := *imp
	return &cp, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

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
		return fmt.Sprintf("fallback%x", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(raw)
}

func newAuditEvent(orgID, actorID, action, resource, resourceID string, meta map[string]string) *ImportAuditEvent {
	return &ImportAuditEvent{
		ID:         "audit_" + randomToken(12),
		OrgID:      orgID,
		ActorID:    actorID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Meta:       meta,
		CreatedAt:  time.Now().UTC(),
	}
}

// detectFileType infers the file type from a filename extension.
func detectFileType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".cl2"):
		return "cl2"
	case strings.HasSuffix(lower, ".sd3"):
		return "sd3"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return "unknown"
	}
}
