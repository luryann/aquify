// Package imports provides the in-memory service for file import/export
// job management, conflict resolution, and import report retrieval.
package imports

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/meetfiles/cl2"
	"github.com/dareaquatics/aquify/meetfiles/sdif"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrForbidden           = errors.New("forbidden")
	ErrImportNotFound      = errors.New("import job not found")
	ErrConflictNotFound    = errors.New("match conflict not found")
	ErrInvalidFileType     = errors.New("unsupported file type; expected cl2, sd3, zip, hy3, hyv, or ev3")
	ErrInvalidTransition   = errors.New("invalid state transition")
	ErrConflictNotOpen     = errors.New("conflict is not in open state; cannot be resolved")
	ErrInvalidFilename     = errors.New("invalid filename")
	ErrFileTooLarge        = errors.New("file exceeds upload size cap")
	ErrInvalidMIMEType     = errors.New("invalid file mime type")
	ErrMalwareDetected     = errors.New("uploaded file failed malware scan")
	ErrMalwareScanRequired = errors.New("malware scan is required but no compliant scanner is configured")
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
	StorageDownloadURL string        `json:"storage_download_url,omitempty"`
	UploadURL          string        `json:"upload_url,omitempty"`
	UploadURLExpiresAt *time.Time    `json:"upload_url_expires_at,omitempty"`
	DeclaredSizeBytes  int64         `json:"declared_size_bytes,omitempty"`
	DeclaredMIMEType   string        `json:"declared_mime_type,omitempty"`
	ScanStatus         string        `json:"scan_status,omitempty"`
	ScanProvider       string        `json:"scan_provider,omitempty"`
	ScannedAt          *time.Time    `json:"scanned_at,omitempty"`
	RowCount           int           `json:"row_count"`
	ErrorCount         int           `json:"error_count"`
	ConflictCount      int           `json:"conflict_count"`
	ImportReport       *ImportReport `json:"import_report,omitempty"`
	ArtifactExpiresAt  *time.Time    `json:"artifact_expires_at,omitempty"`
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
	Conflicts    []ImportErrorEntry `json:"conflicts,omitempty"`
}

// ImportErrorEntry is a single parse/match error captured in the report.
type ImportErrorEntry struct {
	RowNumber    int    `json:"row_number,omitempty"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	RawData      string `json:"raw_data,omitempty"`
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
	Filename  string
	FileType  string // cl2, sd3, zip, hy3, hyv, ev3 (or inferred from filename)
	MeetID    string // optional
	SizeBytes int64
	MIMEType  string
}

type FinalizeImportInput struct{}

// ResolveConflictInput is the input for manually resolving a match conflict.
type ResolveConflictInput struct {
	ResolvedSwimmerID string
	ResolutionNote    string
}

type RosterLoader func(orgID string) []meetfiles.RosterSwimmer
type KnownEventsLoader func(orgID, meetID string) []meetfiles.KnownEvent

// ─── Service ─────────────────────────────────────────────────────────────────

// Service manages file import jobs and match conflicts in memory.
type Service struct {
	mu             sync.RWMutex
	repo           *Repository
	imports        map[string]*FileImport
	conflicts      map[string]*MatchConflict
	store          storage.ObjectStorage
	securityConfig SecurityConfig
	scanner        MalwareScanner
	rosterLoader   RosterLoader
	eventsLoader   KnownEventsLoader
}

type SecurityConfig struct {
	MaxBytes               int64
	AllowedMIMEs           []string
	RequireMalwareScanHook bool
}

type FileUploadMetadata struct {
	Filename  string
	FileType  string
	SizeBytes int64
	MIMEType  string
}

type MalwareScanner interface {
	Scan(storageKey string, metadata FileUploadMetadata) MalwareScanResult
}

type MalwareScanResult struct {
	Status   string
	Provider string
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
		securityConfig: SecurityConfig{
			MaxBytes:     25 << 20,
			AllowedMIMEs: []string{"application/octet-stream", "application/zip", "application/x-zip-compressed", "text/plain", "text/csv"},
		},
		scanner: stubMalwareScanner{},
	}
}

// NewServiceWithPool creates a DB-backed imports Service and loads existing
// data from Postgres into memory on startup.
func NewServiceWithPool(pool *pgxpool.Pool, store storage.ObjectStorage) *Service {
	s := NewServiceWithStorage(store)
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("imports: failed to load from DB", "err", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	fileImports, conflicts, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, imp := range fileImports {
		s.imports[imp.ID] = imp
	}
	for _, c := range conflicts {
		s.conflicts[c.ID] = c
	}
	return nil
}

func (s *Service) newID(prefix string) string {
	if s.repo != nil {
		return uuid.New().String()
	}
	return prefix + randomToken(12)
}

func (s *Service) ConfigureSecurity(cfg SecurityConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.MaxBytes > 0 {
		s.securityConfig.MaxBytes = cfg.MaxBytes
	}
	if len(cfg.AllowedMIMEs) > 0 {
		s.securityConfig.AllowedMIMEs = append([]string{}, cfg.AllowedMIMEs...)
	}
	s.securityConfig.RequireMalwareScanHook = cfg.RequireMalwareScanHook
}

func (s *Service) SetMalwareScanner(scanner MalwareScanner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scanner = scanner
}

func (s *Service) SetRosterLoader(loader RosterLoader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rosterLoader = loader
}

func (s *Service) SetKnownEventsLoader(loader KnownEventsLoader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventsLoader = loader
}

// ─── RequestUploadURL ─────────────────────────────────────────────────────────

// RequestUploadURL creates a new file import record and returns a presigned
// upload URL.  Only owner, admin, and coach may request uploads.
func (s *Service) RequestUploadURL(principal *appmiddleware.Principal, orgID string, input RequestUploadURLInput) (*FileImport, *ImportAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}

	if err := validateFilename(input.Filename); err != nil {
		return nil, nil, err
	}
	if input.SizeBytes <= 0 {
		return nil, nil, ErrFileTooLarge
	}

	fileType := strings.ToLower(strings.TrimSpace(input.FileType))
	if fileType == "" {
		fileType = detectFileType(input.Filename)
	}
	if fileType != "cl2" && fileType != "sd3" && fileType != "zip" && fileType != "hy3" && fileType != "hyv" && fileType != "ev3" {
		return nil, nil, ErrInvalidFileType
	}
	if detectFileType(input.Filename) != fileType {
		return nil, nil, ErrInvalidFileType
	}

	s.mu.RLock()
	maxBytes := s.securityConfig.MaxBytes
	allowedMIMEs := append([]string{}, s.securityConfig.AllowedMIMEs...)
	s.mu.RUnlock()
	if maxBytes > 0 && input.SizeBytes > maxBytes {
		return nil, nil, ErrFileTooLarge
	}
	if !isAllowedMIMEType(input.MIMEType, allowedMIMEs) {
		return nil, nil, ErrInvalidMIMEType
	}

	id := s.newID("import_")
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
		DeclaredSizeBytes:  input.SizeBytes,
		DeclaredMIMEType:   strings.TrimSpace(strings.ToLower(input.MIMEType)),
		ScanStatus:         "pending",
		ScanProvider:       "",
		CreatedAt:          now,
		UpdatedAt:          now,
		CreatedBy:          principal.UserID,
		UpdatedBy:          principal.UserID,
	}

	if s.repo != nil {
		if err := s.repo.UpsertFileImport(context.Background(), imp); err != nil {
			return nil, nil, fmt.Errorf("persist file import: %w", err)
		}
	}
	s.mu.Lock()
	s.imports[id] = imp
	s.mu.Unlock()

	audit := newAuditEvent(orgID, principal.UserID, "import.upload_url_requested", "file_import", id, map[string]string{
		"filename":   input.Filename,
		"file_type":  fileType,
		"mime_type":  strings.TrimSpace(strings.ToLower(input.MIMEType)),
		"size_bytes": fmt.Sprintf("%d", input.SizeBytes),
	})
	return imp, audit, nil
}

// ─── FinalizeImport ───────────────────────────────────────────────────────────

// FinalizeImport marks an uploaded artifact as ready for parsing, runs the
// configured malware scan gate, and then processes the import artifact into a
// real report/conflict set. Only owner, admin, and coach may finalize.
func (s *Service) FinalizeImport(principal *appmiddleware.Principal, orgID, importID string, _ FinalizeImportInput) (*FileImport, *ImportAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}

	s.mu.Lock()
	imp, ok := s.imports[importID]
	if !ok || imp.OrgID != orgID {
		s.mu.Unlock()
		return nil, nil, ErrImportNotFound
	}
	if imp.Status != "uploaded" {
		s.mu.Unlock()
		return nil, nil, fmt.Errorf("%w: cannot finalize an import with status %q", ErrInvalidTransition, imp.Status)
	}

	exists, err := s.store.Exists(context.Background(), imp.StorageKey)
	if err != nil {
		s.mu.Unlock()
		return nil, nil, fmt.Errorf("storage: verify upload exists: %w", err)
	}
	if !exists {
		s.mu.Unlock()
		return nil, nil, fmt.Errorf("%w: uploaded object not found in storage", ErrInvalidTransition)
	}

	// The presigned PUT URL is not size-constrained, so the caller-declared
	// size cannot be trusted. Enforce the actual stored object size against
	// the configured cap before ever reading it into memory.
	actualSize, err := s.store.ObjectSize(context.Background(), imp.StorageKey)
	if err != nil {
		s.mu.Unlock()
		return nil, nil, fmt.Errorf("storage: stat uploaded object: %w", err)
	}
	maxBytes := s.securityConfig.MaxBytes
	if maxBytes > 0 && actualSize > maxBytes {
		s.mu.Unlock()
		return nil, nil, ErrFileTooLarge
	}

	now := time.Now().UTC()
	if s.scanner == nil && !s.securityConfig.RequireMalwareScanHook {
		imp.ScanStatus = "clean"
		imp.ScanProvider = ""
		imp.ScannedAt = nil
	} else {
		scanResult := MalwareScanResult{
			Status:   "pending",
			Provider: "",
		}
		if s.scanner != nil {
			scanResult = s.scanner.Scan(imp.StorageKey, FileUploadMetadata{
				Filename:  imp.Filename,
				FileType:  imp.FileType,
				SizeBytes: imp.DeclaredSizeBytes,
				MIMEType:  imp.DeclaredMIMEType,
			})
		}
		provider := strings.TrimSpace(scanResult.Provider)
		if s.securityConfig.RequireMalwareScanHook && !isCompliantScannerProvider(provider) {
			s.mu.Unlock()
			return nil, nil, ErrMalwareScanRequired
		}
		scanStatus := strings.TrimSpace(strings.ToLower(scanResult.Status))
		if scanStatus == "" && provider != "" {
			scanStatus = "clean"
		}
		if scanStatus == "" {
			scanStatus = "pending"
		}
		imp.ScanStatus = scanStatus
		if provider != "" {
			imp.ScanProvider = provider
		} else {
			imp.ScanProvider = ""
		}
		if provider != "" {
			imp.ScannedAt = &now
		} else {
			imp.ScannedAt = nil
		}
		if scanStatus != "clean" {
			imp.Status = "failed"
			imp.UpdatedAt = now
			imp.UpdatedBy = principal.UserID
			if s.repo != nil {
				_ = s.repo.UpsertFileImport(context.Background(), imp)
			}
			s.mu.Unlock()
			return nil, nil, ErrMalwareDetected
		}
	}

	imp.Status = "parsing"
	imp.RowCount = 0
	imp.ErrorCount = 0
	imp.ConflictCount = 0
	imp.ImportReport = nil
	imp.UploadURL = ""
	imp.UploadURLExpiresAt = nil
	imp.UpdatedAt = now
	imp.UpdatedBy = principal.UserID

	if s.repo != nil {
		if err := s.repo.UpsertFileImport(context.Background(), imp); err != nil {
			s.mu.Unlock()
			return nil, nil, fmt.Errorf("persist parsing file import: %w", err)
		}
	}
	audit := newAuditEvent(orgID, principal.UserID, "import.finalized", "file_import", importID, map[string]string{
		"status":      imp.Status,
		"scan_status": imp.ScanStatus,
	})
	s.mu.Unlock()
	processErr := s.ProcessImport(context.Background(), importID)
	if processErr != nil {
		return nil, audit, processErr
	}

	current, err := s.GetImport(principal, orgID, importID)
	if err != nil {
		return nil, audit, err
	}
	return current, audit, nil
}

// ProcessImport reads a finalized artifact from object storage, parses it with
// the meetfiles package, matches parsed rows to roster swimmers when a loader
// is configured, and persists the resulting report/conflicts.
func (s *Service) ProcessImport(ctx context.Context, importID string) error {
	s.mu.RLock()
	imp, ok := s.imports[importID]
	if !ok {
		s.mu.RUnlock()
		return ErrImportNotFound
	}
	storageKey := imp.StorageKey
	filename := imp.Filename
	orgID := imp.OrgID
	meetID := strings.TrimSpace(imp.MeetID)
	status := imp.Status
	rosterLoader := s.rosterLoader
	eventsLoader := s.eventsLoader
	s.mu.RUnlock()

	if status != "parsing" && status != "matching" {
		return fmt.Errorf("%w: cannot process an import with status %q", ErrInvalidTransition, status)
	}

	data, _, err := s.store.GetObject(ctx, storageKey)
	if err != nil {
		s.markImportFailed(importID, fmt.Sprintf("read uploaded artifact: %v", err))
		return fmt.Errorf("read uploaded artifact: %w", err)
	}

	parsed, err := parseImportArtifact(filename, data)
	if err != nil {
		s.markImportFailed(importID, err.Error())
		return err
	}

	s.markImportStatus(importID, "matching")

	roster := []meetfiles.RosterSwimmer{}
	if rosterLoader != nil {
		roster = rosterLoader(orgID)
	}
	withMatches := make([]meetfiles.ResultWithMatch, 0, len(parsed.IndividualResults))
	for _, result := range parsed.IndividualResults {
		match := meetfiles.MatchResult{
			Outcome:  meetfiles.MatchExact,
			Strategy: "unmatched_not_required",
		}
		if rosterLoader != nil {
			match = meetfiles.MatchSwimmer(result.Swimmer, roster)
		}
		withMatches = append(withMatches, meetfiles.ResultWithMatch{
			Result: result,
			Match:  match,
		})
	}

	knownEvents := []meetfiles.KnownEvent{}
	if eventsLoader != nil && meetID != "" {
		knownEvents = eventsLoader(orgID, meetID)
	}
	additionalErrors := append([]meetfiles.ParseError{}, meetfiles.ClassifyEventErrors(parsed.IndividualResults, knownEvents)...)
	additionalErrors = append(additionalErrors, meetfiles.ClassifyDuplicates(withMatches)...)

	report := meetfiles.BuildReport(parsed, withMatches, additionalErrors)
	conflictEntries := meetfiles.BuildConflictQueue(withMatches)
	return s.applyParseOutcome(importID, report, conflictEntries)
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

	if s.repo != nil {
		if err := s.repo.UpsertMatchConflict(context.Background(), conflict); err != nil {
			return nil, nil, fmt.Errorf("persist resolved conflict: %w", err)
		}
		if imp.Status == "applied" {
			if err := s.repo.UpsertFileImport(context.Background(), imp); err != nil {
				return nil, nil, fmt.Errorf("persist applied import: %w", err)
			}
		}
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
	if cp.StorageKey != "" {
		url, err := s.store.PresignedGetURL(cp.StorageKey, 15*time.Minute)
		if err == nil {
			cp.StorageDownloadURL = url
		}
	}
	return &cp, nil
}

type ExpiredUploadArtifact struct {
	ImportID   string
	StorageKey string
	ExpiredAt  time.Time
}

// ExpirePendingUploads revokes stale presigned upload windows and returns the
// object keys that should be deleted from storage.
func (s *Service) ExpirePendingUploads(cutoff time.Time) []ExpiredUploadArtifact {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expired []ExpiredUploadArtifact
	for _, imp := range s.imports {
		if imp == nil || imp.Status != "uploaded" || imp.UploadURLExpiresAt == nil {
			continue
		}
		if imp.UploadURLExpiresAt.After(cutoff) {
			continue
		}
		expired = append(expired, ExpiredUploadArtifact{
			ImportID:   imp.ID,
			StorageKey: imp.StorageKey,
			ExpiredAt:  *imp.UploadURLExpiresAt,
		})
		imp.Status = "expired"
		imp.UploadURL = ""
		imp.UploadURLExpiresAt = nil
		imp.UpdatedAt = cutoff.UTC()
	}
	return expired
}

func (s *Service) DeleteArtifact(storageKey string) error {
	if strings.TrimSpace(storageKey) == "" {
		return nil
	}
	return s.store.Delete(storageKey)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *Service) markImportStatus(importID, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	imp, ok := s.imports[importID]
	if !ok {
		return
	}
	imp.Status = status
	imp.UpdatedAt = time.Now().UTC()
	if s.repo != nil {
		_ = s.repo.UpsertFileImport(context.Background(), imp)
	}
}

func (s *Service) markImportFailed(importID, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	imp, ok := s.imports[importID]
	if !ok {
		return
	}
	now := time.Now().UTC()
	imp.Status = "failed"
	imp.UpdatedAt = now
	imp.ErrorCount = 1
	imp.ImportReport = &ImportReport{
		ErrorRows: 1,
		Errors: []ImportErrorEntry{{
			ErrorCode:    "import_failed",
			ErrorMessage: strings.TrimSpace(message),
		}},
	}
	if s.repo != nil {
		_ = s.repo.UpsertFileImport(context.Background(), imp)
	}
}

func (s *Service) applyParseOutcome(importID string, report meetfiles.ImportReport, conflictEntries []meetfiles.ConflictEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	imp, ok := s.imports[importID]
	if !ok {
		return ErrImportNotFound
	}

	for id, conflict := range s.conflicts {
		if conflict.ImportID == importID {
			delete(s.conflicts, id)
		}
	}

	now := time.Now().UTC()
	imp.RowCount = report.TotalRows
	imp.ErrorCount = report.ErrorRows
	imp.ConflictCount = report.ConflictRows
	imp.ImportReport = &ImportReport{
		TotalRows:    report.TotalRows,
		MatchedRows:  report.MatchedRows,
		ErrorRows:    report.ErrorRows,
		ConflictRows: report.ConflictRows,
		Errors:       translateReportEntries(report.Errors),
		Conflicts:    translateReportEntries(report.Conflicts),
	}
	if len(conflictEntries) > 0 {
		imp.Status = "conflicts_pending"
	} else {
		imp.Status = "applied"
	}
	imp.UpdatedAt = now

	newConflicts := make([]*MatchConflict, 0, len(conflictEntries))
	for _, entry := range conflictEntries {
		conflict := &MatchConflict{
			ID:                  s.newID("conflict_"),
			ImportID:            importID,
			OrgID:               imp.OrgID,
			RowNumber:           entry.LineNumber,
			RawData:             strings.TrimSpace(entry.RawData),
			Status:              "open",
			ConflictType:        string(entry.Code),
			CandidateSwimmerIDs: append([]string{}, entry.CandidateSwimmerIDs...),
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		s.conflicts[conflict.ID] = conflict
		newConflicts = append(newConflicts, conflict)
	}

	if s.repo != nil {
		if err := s.repo.UpsertFileImport(context.Background(), imp); err != nil {
			return fmt.Errorf("persist parsed file import: %w", err)
		}
		for _, conflict := range newConflicts {
			if err := s.repo.UpsertMatchConflict(context.Background(), conflict); err != nil {
				return fmt.Errorf("persist parsed match conflict: %w", err)
			}
		}
	}
	return nil
}

func translateReportEntries(lines []meetfiles.ReportLine) []ImportErrorEntry {
	if len(lines) == 0 {
		return nil
	}
	out := make([]ImportErrorEntry, 0, len(lines))
	for _, line := range lines {
		out = append(out, ImportErrorEntry{
			RowNumber:    line.LineNumber,
			ErrorCode:    string(line.ErrorCode),
			ErrorMessage: line.Message,
			RawData:      strings.TrimSpace(line.RawData),
		})
	}
	return out
}

func parseImportArtifact(filename string, data []byte) (*meetfiles.ParseResult, error) {
	switch meetfiles.DetectFileType(filename, data) {
	case meetfiles.FileTypeSD3:
		return sdif.Parse(data)
	case meetfiles.FileTypeCL2:
		return cl2.Parse(data)
	case meetfiles.FileTypeHY3:
		ingest := meetfiles.ReadHY3(filename, data)
		if ingest.SelectedPayload == nil {
			if ingest.Error != nil {
				return nil, fmt.Errorf("parse hy3: %s", ingest.Error.Message)
			}
			return nil, fmt.Errorf("parse hy3: archive did not contain a parseable payload")
		}
		result, err := parseImportArtifact(ingest.SelectedPayload.Name, ingest.SelectedPayload.Content)
		if err != nil {
			return nil, err
		}
		if ingest.Error != nil {
			result.ParseErrors = append([]meetfiles.ParseError{*ingest.Error}, result.ParseErrors...)
		}
		return result, nil
	case meetfiles.FileTypeZIP:
		entries, err := meetfiles.ReadMeetZip(data)
		if err != nil {
			return nil, fmt.Errorf("parse zip: %w", err)
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("parse zip: archive did not contain a parseable payload")
		}
		merged := &meetfiles.ParseResult{FileType: meetfiles.FileTypeZIP}
		for _, entry := range entries {
			result, err := parseImportArtifact(entry.Name, entry.Content)
			if err != nil {
				return nil, err
			}
			mergeParseResult(merged, result)
		}
		return merged, nil
	default:
		return nil, fmt.Errorf("unsupported import artifact type for %s", filename)
	}
}

func mergeParseResult(dst, src *meetfiles.ParseResult) {
	if dst == nil || src == nil {
		return
	}
	if dst.MeetName == "" {
		dst.MeetName = src.MeetName
	}
	if dst.MeetStartDate == nil {
		dst.MeetStartDate = src.MeetStartDate
	}
	if dst.MeetEndDate == nil {
		dst.MeetEndDate = src.MeetEndDate
	}
	if dst.Course == meetfiles.CourseUnknown {
		dst.Course = src.Course
	}
	dst.IndividualResults = append(dst.IndividualResults, src.IndividualResults...)
	dst.RelayResults = append(dst.RelayResults, src.RelayResults...)
	dst.ParseErrors = append(dst.ParseErrors, src.ParseErrors...)
}

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
		panic(fmt.Errorf("secure random unavailable for import token generation: %w", err))
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
	case strings.HasSuffix(lower, ".hy3"):
		return "hy3"
	case strings.HasSuffix(lower, ".hyv"):
		return "hyv"
	case strings.HasSuffix(lower, ".ev3"):
		return "ev3"
	default:
		return "unknown"
	}
}

func validateFilename(filename string) error {
	trimmed := strings.TrimSpace(filename)
	if trimmed == "" {
		return ErrInvalidFilename
	}
	if trimmed != filepath.Base(trimmed) {
		return ErrInvalidFilename
	}
	if strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, `/\`) {
		return ErrInvalidFilename
	}
	return nil
}

func isAllowedMIMEType(value string, allowed []string) bool {
	mimeType := strings.TrimSpace(strings.ToLower(value))
	if mimeType == "" {
		return false
	}
	for _, item := range allowed {
		if mimeType == strings.TrimSpace(strings.ToLower(item)) {
			return true
		}
	}
	return false
}

func isCompliantScannerProvider(provider string) bool {
	normalized := strings.TrimSpace(strings.ToLower(provider))
	return normalized != "" && normalized != "stub"
}

type stubMalwareScanner struct{}

func (stubMalwareScanner) Scan(_ string, _ FileUploadMetadata) MalwareScanResult {
	return MalwareScanResult{
		Status:   "clean",
		Provider: "stub",
	}
}
