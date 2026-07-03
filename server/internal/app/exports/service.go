// Package exports provides SD3/SDIF export job management backed by object
// storage. Export creation is queue-first: the request path creates a pending
// job and the background worker produces the artifact.
package exports

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

var (
	ErrForbidden         = errors.New("forbidden")
	ErrExportNotFound    = errors.New("export job not found")
	ErrExportNotReady    = errors.New("export job is not ready")
	ErrExportExpired     = errors.New("export job has expired")
	ErrInvalidExportType = errors.New("invalid report type; expected eligibility_report, entry_report, or event_report")
)

type FileExport struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	MeetID      string    `json:"meet_id"`
	ExportType  string    `json:"export_type"`
	Status      string    `json:"status"`
	Filename    string    `json:"filename"`
	SizeBytes   int       `json:"size_bytes,omitempty"`
	StorageKey  string    `json:"storage_key,omitempty"`
	DownloadURL string    `json:"download_url,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by"`
	UpdatedBy   string    `json:"updated_by"`

	pendingInput *ReportInput
}

type ExportAuditEvent struct {
	ID         string            `json:"id"`
	OrgID      string            `json:"org_id"`
	ActorID    string            `json:"actor_id"`
	Action     string            `json:"action"`
	Resource   string            `json:"resource"`
	ResourceID string            `json:"resource_id"`
	Meta       map[string]string `json:"meta,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

type Service struct {
	mu             sync.RWMutex
	repo           *Repository
	exports        map[string]*FileExport
	store          storage.ObjectStorage
	queue          workers.Queue
	artifactTTL    time.Duration
	downloadURLTTL time.Duration
	nowFunc        func() time.Time
}

func NewService() *Service {
	return NewServiceWithStorage(storage.NewStubStorage(""))
}

func NewServiceWithStorage(store storage.ObjectStorage) *Service {
	return &Service{
		exports:        make(map[string]*FileExport),
		store:          store,
		artifactTTL:    7 * 24 * time.Hour,
		downloadURLTTL: 15 * time.Minute,
		nowFunc:        func() time.Time { return time.Now().UTC() },
	}
}

// NewServiceWithPool creates a DB-backed exports Service and loads existing
// data from Postgres into memory on startup.
func NewServiceWithPool(pool *pgxpool.Pool, store storage.ObjectStorage) *Service {
	s := NewServiceWithStorage(store)
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("exports: failed to load from DB", "err", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	fileExports, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range fileExports {
		s.exports[ex.ID] = ex
	}
	return nil
}

func (s *Service) newID(prefix string) string {
	if s.repo != nil {
		return uuid.New().String()
	}
	return prefix + randomToken(12)
}

func (s *Service) SetQueue(queue workers.Queue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = queue
}

func (s *Service) ConfigureArtifactTTL(ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifactTTL = ttl
}

func (s *Service) GenerateExport(principal *appmiddleware.Principal, orgID, meetID string, input ReportInput) (*FileExport, *ExportAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}
	if err := validateReportInput(input); err != nil {
		return nil, nil, err
	}

	id := s.newID("export_")
	now := s.nowFunc()
	filename := reportFilename(input)
	storageKey := fmt.Sprintf("exports/%s/%s/%s", orgID, id, filename)

	cpInput := input
	job := &FileExport{
		ID:           id,
		OrgID:        orgID,
		MeetID:       meetID,
		ExportType:   input.ReportType,
		Status:       "pending",
		Filename:     filename,
		StorageKey:   storageKey,
		ExpiresAt:    now.Add(s.artifactTTL),
		CreatedAt:    now,
		UpdatedAt:    now,
		CreatedBy:    principal.UserID,
		UpdatedBy:    principal.UserID,
		pendingInput: &cpInput,
	}

	if s.repo != nil {
		if err := s.repo.UpsertFileExport(context.Background(), job); err != nil {
			slog.Error("exports: persist file export", "err", err)
		}
	}
	s.mu.Lock()
	s.exports[id] = job
	s.mu.Unlock()

	// PDF reports are small and fast to render; generate synchronously in the
	// request path so the artifact is ready on return (and so generation never
	// depends on in-memory pending input surviving into a separate worker process).
	if err := s.ProcessExport(context.Background(), id); err != nil {
		return nil, nil, err
	}

	audit := newAuditEvent(orgID, principal.UserID, "report.generated", "file_export", id, map[string]string{
		"report_type": input.ReportType,
		"meet_id":     meetID,
		"filename":    filename,
	})
	return s.cloneForResponse(job), audit, nil
}

func (s *Service) ProcessExport(ctx context.Context, exportID string) error {
	s.mu.Lock()
	job, ok := s.exports[exportID]
	if !ok {
		s.mu.Unlock()
		return ErrExportNotFound
	}
	if job.Status == "expired" {
		s.mu.Unlock()
		return ErrExportExpired
	}
	if job.pendingInput == nil {
		s.mu.Unlock()
		return fmt.Errorf("export %s has no pending input", exportID)
	}
	input := *job.pendingInput
	job.Status = "generating"
	job.LastError = ""
	job.UpdatedAt = s.nowFunc()
	s.mu.Unlock()

	content, filename, err := buildReportPDF(input)
	if err != nil {
		s.FailExport(exportID, err)
		return fmt.Errorf("generate report content: %w", err)
	}

	if err := s.store.PutObject(ctx, job.StorageKey, "application/pdf", content); err != nil {
		s.FailExport(exportID, err)
		return fmt.Errorf("store export artifact: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok = s.exports[exportID]
	if !ok {
		return ErrExportNotFound
	}
	job.Status = "ready"
	job.Filename = filename
	job.SizeBytes = len(content)
	job.LastError = ""
	job.UpdatedAt = s.nowFunc()
	job.pendingInput = nil
	if s.repo != nil {
		if err := s.repo.UpsertFileExport(ctx, job); err != nil {
			slog.Error("exports: persist ready file export", "err", err)
		}
	}
	return nil
}

func (s *Service) FailExport(exportID string, cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.exports[exportID]
	if !ok {
		return
	}
	job.Status = "failed"
	job.LastError = strings.TrimSpace(cause.Error())
	job.UpdatedAt = s.nowFunc()
	if s.repo != nil {
		if err := s.repo.UpsertFileExport(context.Background(), job); err != nil {
			slog.Error("exports: persist failed file export", "err", err)
		}
	}
}

func (s *Service) ListExports(principal *appmiddleware.Principal, orgID, meetID string) ([]FileExport, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, ErrForbidden
	}
	return s.ListExportsForAuthorizedMeet(orgID, meetID), nil
}

func (s *Service) ListExportsForAuthorizedMeet(orgID, meetID string) []FileExport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []FileExport
	for _, ex := range s.exports {
		if ex.OrgID != orgID {
			continue
		}
		if meetID != "" && ex.MeetID != meetID {
			continue
		}
		result = append(result, *s.cloneForResponse(ex))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (s *Service) GetExport(principal *appmiddleware.Principal, orgID, exportID string) (*FileExport, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, ErrForbidden
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	ex, ok := s.exports[exportID]
	if !ok || ex.OrgID != orgID {
		return nil, ErrExportNotFound
	}
	return s.cloneForResponse(ex), nil
}

func (s *Service) DownloadExport(principal *appmiddleware.Principal, orgID, exportID string) (*FileExport, []byte, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}

	s.mu.RLock()
	ex, ok := s.exports[exportID]
	if !ok || ex.OrgID != orgID {
		s.mu.RUnlock()
		return nil, nil, ErrExportNotFound
	}
	if ex.Status != "ready" {
		s.mu.RUnlock()
		return nil, nil, ErrExportNotReady
	}
	if !ex.ExpiresAt.IsZero() && !ex.ExpiresAt.After(s.nowFunc()) {
		s.mu.RUnlock()
		return nil, nil, ErrExportExpired
	}
	storageKey := ex.StorageKey
	s.mu.RUnlock()

	content, _, err := s.store.GetObject(context.Background(), storageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, nil, ErrExportNotFound
		}
		return nil, nil, fmt.Errorf("download export artifact: %w", err)
	}
	job := s.cloneForResponse(ex)
	return job, content, nil
}

func (s *Service) ExpireArtifacts(cutoff time.Time) []FileExport {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expired []FileExport
	for _, ex := range s.exports {
		if ex == nil || ex.StorageKey == "" || ex.Status == "expired" {
			continue
		}
		if ex.ExpiresAt.IsZero() || ex.ExpiresAt.After(cutoff) {
			continue
		}
		ex.Status = "expired"
		ex.DownloadURL = ""
		ex.UpdatedAt = cutoff.UTC()
		expired = append(expired, *ex)
	}
	return expired
}

func (s *Service) DeleteArtifact(storageKey string) error {
	if strings.TrimSpace(storageKey) == "" {
		return nil
	}
	return s.store.Delete(storageKey)
}

func (s *Service) cloneForResponse(ex *FileExport) *FileExport {
	if ex == nil {
		return nil
	}
	cp := *ex
	cp.pendingInput = nil
	if cp.Status == "ready" && cp.StorageKey != "" && (cp.ExpiresAt.IsZero() || cp.ExpiresAt.After(s.nowFunc())) {
		if url, err := s.store.PresignedGetURL(cp.StorageKey, s.downloadURLTTL); err == nil {
			cp.DownloadURL = url
		}
	}
	return &cp
}

type GenerateExportTaskPayload struct {
	ExportID string `json:"export_id"`
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
		panic(fmt.Errorf("secure random unavailable for export token generation: %w", err))
	}
	return hex.EncodeToString(raw)
}

func newAuditEvent(orgID, actorID, action, resource, resourceID string, meta map[string]string) *ExportAuditEvent {
	return &ExportAuditEvent{
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

func sanitizeFilename(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return "export"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "export"
	}
	return out
}

