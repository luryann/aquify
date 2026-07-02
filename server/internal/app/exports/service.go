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

	"github.com/dareaquatics/aquify/meetfiles"
	sdifexport "github.com/dareaquatics/aquify/meetfiles/sdif"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

var (
	ErrForbidden         = errors.New("forbidden")
	ErrExportNotFound    = errors.New("export job not found")
	ErrExportNotReady    = errors.New("export job is not ready")
	ErrExportExpired     = errors.New("export job has expired")
	ErrInvalidExportType = errors.New("invalid export type; expected standard_sd3, extended_sd3, or sdif_roster")
)

const (
	ExportTypeStandardSD3 = "standard_sd3"
	ExportTypeExtendedSD3 = "extended_sd3"
	ExportTypeSDIFRoster  = "sdif_roster"
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

	pendingInput *GenerateExportInput
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

type ExportSwimmer struct {
	LastName  string `json:"last_name"`
	FirstName string `json:"first_name"`
	USSNumber string `json:"uss_number"`
	DOBRaw    string `json:"dob"`
	Gender    string `json:"gender"`
	TeamCode  string `json:"team_code"`
}

type ExportEntry struct {
	Swimmer     ExportSwimmer `json:"swimmer"`
	EventNumber string        `json:"event_number"`
	Distance    int           `json:"distance"`
	Stroke      string        `json:"stroke"`
	EventGender string        `json:"event_gender"`
	AgeGroup    string        `json:"age_group"`
	Course      string        `json:"course"`
	SeedTimeMs  int           `json:"seed_time_ms"`
	Heat        int           `json:"heat"`
	Lane        int           `json:"lane"`
}

type GenerateExportInput struct {
	ExportType    string        `json:"export_type"`
	MeetName      string        `json:"meet_name"`
	MeetStartDate string        `json:"meet_start_date"`
	MeetEndDate   string        `json:"meet_end_date"`
	Course        string        `json:"course"`
	HostTeamCode  string        `json:"host_team_code"`
	HostTeamName  string        `json:"host_team_name"`
	Entries       []ExportEntry `json:"entries"`
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

func (s *Service) GenerateExport(principal *appmiddleware.Principal, orgID, meetID string, input GenerateExportInput) (*FileExport, *ExportAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}
	if err := validateGenerateInput(input); err != nil {
		return nil, nil, err
	}

	id := s.newID("export_")
	now := s.nowFunc()
	filename := suggestedFilename(input)
	storageKey := fmt.Sprintf("exports/%s/%s/%s", orgID, id, filename)

	cpInput := input
	job := &FileExport{
		ID:           id,
		OrgID:        orgID,
		MeetID:       meetID,
		ExportType:   input.ExportType,
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
	queue := s.queue
	s.mu.Unlock()

	if queue != nil {
		task, err := workers.NewTask(workers.TypeGenerateExport, GenerateExportTaskPayload{
			ExportID: id,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("build export task: %w", err)
		}
		if _, err := queue.Enqueue(context.Background(), task, workers.WithQueue(workers.QueueDefault), workers.WithUniqueKey("export:"+id)); err != nil {
			s.FailExport(id, fmt.Errorf("enqueue export task: %w", err))
			return nil, nil, fmt.Errorf("enqueue export task: %w", err)
		}
	} else {
		if err := s.ProcessExport(context.Background(), id); err != nil {
			return nil, nil, err
		}
	}

	audit := newAuditEvent(orgID, principal.UserID, "export.requested", "file_export", id, map[string]string{
		"export_type": input.ExportType,
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

	content, filename, err := buildFileContent(input)
	if err != nil {
		s.FailExport(exportID, err)
		return fmt.Errorf("generate export content: %w", err)
	}

	if err := s.store.PutObject(ctx, job.StorageKey, "application/octet-stream", content); err != nil {
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

func suggestedFilename(input GenerateExportInput) string {
	switch input.ExportType {
	case ExportTypeStandardSD3:
		return sanitizeFilename(input.MeetName) + "_standard.sd3"
	case ExportTypeExtendedSD3:
		return sanitizeFilename(input.MeetName) + "_extended.sd3"
	case ExportTypeSDIFRoster:
		return sanitizeFilename(input.MeetName) + "_roster.sd3"
	default:
		return sanitizeFilename(input.MeetName) + ".dat"
	}
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

func validateGenerateInput(input GenerateExportInput) error {
	switch input.ExportType {
	case ExportTypeStandardSD3, ExportTypeExtendedSD3, ExportTypeSDIFRoster:
	default:
		return ErrInvalidExportType
	}
	if strings.TrimSpace(input.MeetName) == "" {
		return fmt.Errorf("meet_name is required")
	}
	return nil
}

// buildFileContent generates the SD3/SDIF file bytes and returns them along
// with a suggested filename.
func buildFileContent(input GenerateExportInput) ([]byte, string, error) {
	filename := suggestedFilename(input)
	switch input.ExportType {
	case ExportTypeStandardSD3:
		canonical, err := toCanonicalExportInput(input)
		if err != nil {
			return nil, "", err
		}
		content, err := sdifexport.ExportStandardSD3(canonical)
		if err != nil {
			return nil, "", fmt.Errorf("generate standard sd3: %w", err)
		}
		return content, filename, nil
	case ExportTypeExtendedSD3:
		canonical, err := toCanonicalExportInput(input)
		if err != nil {
			return nil, "", err
		}
		content, err := sdifexport.ExportExtendedSD3(canonical)
		if err != nil {
			return nil, "", fmt.Errorf("generate extended sd3: %w", err)
		}
		return content, filename, nil
	case ExportTypeSDIFRoster:
		canonical, err := toCanonicalRosterInput(input)
		if err != nil {
			return nil, "", err
		}
		content, err := sdifexport.ExportSDIFRoster(canonical)
		if err != nil {
			return nil, "", fmt.Errorf("generate sdif roster: %w", err)
		}
		return content, filename, nil
	default:
		return nil, "", ErrInvalidExportType
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func toCanonicalExportInput(input GenerateExportInput) (sdifexport.ExportInput, error) {
	startDate, err := parseMMDDYYYYDate(input.MeetStartDate)
	if err != nil {
		return sdifexport.ExportInput{}, fmt.Errorf("meet_start_date: %w", err)
	}
	endDate, err := parseMMDDYYYYDate(input.MeetEndDate)
	if err != nil {
		return sdifexport.ExportInput{}, fmt.Errorf("meet_end_date: %w", err)
	}

	out := sdifexport.ExportInput{
		MeetName:      strings.TrimSpace(input.MeetName),
		MeetStartDate: startDate,
		MeetEndDate:   endDate,
		Course:        canonicalCourse(input.Course),
		HostTeamCode:  strings.TrimSpace(input.HostTeamCode),
		HostTeamName:  strings.TrimSpace(input.HostTeamName),
	}
	for _, entry := range input.Entries {
		dob, err := parseMMDDYYYYDate(entry.Swimmer.DOBRaw)
		if err != nil {
			return sdifexport.ExportInput{}, fmt.Errorf("entry swimmer dob for event %q: %w", entry.EventNumber, err)
		}
		out.IndividualEntries = append(out.IndividualEntries, sdifexport.ExportIndividualEntry{
			Swimmer: sdifexport.ExportSwimmer{
				LastName:  strings.TrimSpace(entry.Swimmer.LastName),
				FirstName: strings.TrimSpace(entry.Swimmer.FirstName),
				USSNumber: strings.TrimSpace(entry.Swimmer.USSNumber),
				DOB:       dob,
				Gender:    strings.TrimSpace(entry.Swimmer.Gender),
				TeamCode:  strings.TrimSpace(entry.Swimmer.TeamCode),
			},
			EventNumber: strings.TrimSpace(entry.EventNumber),
			Distance:    entry.Distance,
			Stroke:      canonicalStroke(entry.Stroke),
			EventGender: strings.TrimSpace(entry.EventGender),
			AgeGroup:    strings.TrimSpace(entry.AgeGroup),
			Course:      canonicalCourse(entry.Course),
			SeedTime:    canonicalTime(entry.SeedTimeMs),
			Heat:        max(entry.Heat, 0),
			Lane:        max(entry.Lane, 0),
		})
	}
	return out, nil
}

func toCanonicalRosterInput(input GenerateExportInput) (sdifexport.RosterExportInput, error) {
	startDate, err := parseMMDDYYYYDate(input.MeetStartDate)
	if err != nil {
		return sdifexport.RosterExportInput{}, fmt.Errorf("meet_start_date: %w", err)
	}
	endDate, err := parseMMDDYYYYDate(input.MeetEndDate)
	if err != nil {
		return sdifexport.RosterExportInput{}, fmt.Errorf("meet_end_date: %w", err)
	}

	out := sdifexport.RosterExportInput{
		MeetName:  strings.TrimSpace(input.MeetName),
		StartDate: startDate,
		EndDate:   endDate,
		Course:    canonicalCourse(input.Course),
		TeamCode:  strings.TrimSpace(input.HostTeamCode),
		TeamName:  strings.TrimSpace(input.HostTeamName),
	}

	seen := make(map[string]struct{})
	for _, entry := range input.Entries {
		key := strings.Join([]string{
			strings.TrimSpace(entry.Swimmer.USSNumber),
			strings.TrimSpace(entry.Swimmer.LastName),
			strings.TrimSpace(entry.Swimmer.FirstName),
			strings.TrimSpace(entry.Swimmer.DOBRaw),
			strings.TrimSpace(entry.Swimmer.Gender),
			strings.TrimSpace(entry.Swimmer.TeamCode),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dob, err := parseMMDDYYYYDate(entry.Swimmer.DOBRaw)
		if err != nil {
			return sdifexport.RosterExportInput{}, fmt.Errorf("roster swimmer dob for %q: %w", entry.Swimmer.LastName, err)
		}
		out.Swimmers = append(out.Swimmers, sdifexport.RosterExportSwimmer{
			LastName:  strings.TrimSpace(entry.Swimmer.LastName),
			FirstName: strings.TrimSpace(entry.Swimmer.FirstName),
			USSNumber: strings.TrimSpace(entry.Swimmer.USSNumber),
			DOB:       dob,
			Gender:    strings.TrimSpace(entry.Swimmer.Gender),
			TeamCode:  strings.TrimSpace(entry.Swimmer.TeamCode),
			TeamName:  strings.TrimSpace(input.HostTeamName),
		})
	}
	return out, nil
}

func parseMMDDYYYYDate(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("01022006", value)
	if err != nil {
		return nil, fmt.Errorf("must use MMDDYYYY")
	}
	utc := parsed.UTC()
	return &utc, nil
}

func canonicalCourse(raw string) meetfiles.Course {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "SCM":
		return meetfiles.CourseShortCourseMeters
	case "LCM":
		return meetfiles.CourseLongCourseMeters
	case "SCY":
		return meetfiles.CourseShortCourseYards
	default:
		return meetfiles.CourseUnknown
	}
}

func canonicalStroke(raw string) meetfiles.Stroke {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "free":
		return meetfiles.StrokeFreestyle
	case "back":
		return meetfiles.StrokeBackstroke
	case "breast":
		return meetfiles.StrokeBreaststroke
	case "fly":
		return meetfiles.StrokeButterfly
	case "im":
		return meetfiles.StrokeIM
	case "free_relay":
		return meetfiles.StrokeFreestyleRelay
	case "medley_relay":
		return meetfiles.StrokeMedleyRelay
	default:
		return meetfiles.StrokeUnknown
	}
}

func canonicalTime(ms int) meetfiles.Time {
	if ms <= 0 {
		return meetfiles.Time{Raw: "NT"}
	}
	return meetfiles.Time{
		Valid:   true,
		Seconds: float64(ms) / 1000,
	}
}
