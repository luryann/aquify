// Package exports provides the in-memory service for SD3/SDIF export job
// management. In production, generation would be delegated to an Asynq
// background worker that calls the internal/meetfiles/sdif exporter and stores
// the result in object storage. For the scaffold, generation is synchronous and
// the file content is kept in memory.
package exports

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// ─── Sentinel errors ─────────────────────────────────────────────────────────

var (
	ErrForbidden       = errors.New("forbidden")
	ErrExportNotFound  = errors.New("export job not found")
	ErrExportNotReady  = errors.New("export job is not ready")
	ErrInvalidExportType = errors.New("invalid export type; expected standard_sd3, extended_sd3, or sdif_roster")
)

// ─── Export types ─────────────────────────────────────────────────────────────

const (
	ExportTypeStandardSD3  = "standard_sd3"
	ExportTypeExtendedSD3  = "extended_sd3"
	ExportTypeSDIFRoster   = "sdif_roster"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// FileExport represents one export job.
//
// State machine: pending → generating → ready | failed → expired
// TODO(production): add a background sweeper that transitions ready/failed jobs
// to "expired" after a configurable TTL and evicts their content from memory.
// The current scaffold never transitions to "expired" and retains all content
// indefinitely — this must be addressed before production deployment.
type FileExport struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	MeetID     string    `json:"meet_id"`
	ExportType string    `json:"export_type"`
	Status     string    `json:"status"` // pending → generating → ready | failed → expired
	Filename   string    `json:"filename"`
	SizeBytes  int       `json:"size_bytes,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"` // zero until a TTL sweeper is wired in
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	CreatedBy  string    `json:"created_by"`
	UpdatedBy  string    `json:"updated_by"`

	// content holds the generated file bytes in memory (not serialised).
	content []byte
}

// ExportAuditEvent records privileged actions in the export domain.
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

// ─── Input types ─────────────────────────────────────────────────────────────

// ExportSwimmer holds swimmer identity fields required for SD3 export.
type ExportSwimmer struct {
	LastName  string `json:"last_name"`
	FirstName string `json:"first_name"`
	USSNumber string `json:"uss_number"`
	DOBRaw    string `json:"dob"` // MMDDYYYY — SDIF format; callers must pre-convert from other formats
	Gender    string `json:"gender"` // "M" or "F"
	TeamCode  string `json:"team_code"`
}

// ExportEntry is one swimmer-event entry for inclusion in the SD3 file.
type ExportEntry struct {
	Swimmer     ExportSwimmer `json:"swimmer"`
	EventNumber string        `json:"event_number"`
	Distance    int           `json:"distance"`
	Stroke      string        `json:"stroke"`      // free, back, breast, fly, im, free_relay, medley_relay
	EventGender string        `json:"event_gender"` // M, F, or X
	AgeGroup    string        `json:"age_group"`
	Course      string        `json:"course"` // SCY, SCM, LCM
	SeedTimeMs  int           `json:"seed_time_ms"` // 0 = NT
	Heat        int           `json:"heat"`
	Lane        int           `json:"lane"`
}

// GenerateExportInput contains everything needed to produce an SD3 export file.
type GenerateExportInput struct {
	ExportType    string        `json:"export_type"`
	MeetName      string        `json:"meet_name"`
	MeetStartDate string        `json:"meet_start_date"` // MMDDYYYY
	MeetEndDate   string        `json:"meet_end_date"`   // MMDDYYYY
	Course        string        `json:"course"`
	HostTeamCode  string        `json:"host_team_code"`
	HostTeamName  string        `json:"host_team_name"`
	Entries       []ExportEntry `json:"entries"`
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service manages export jobs in memory.
type Service struct {
	mu      sync.RWMutex
	exports map[string]*FileExport
}

// NewService returns a new in-memory exports Service.
func NewService() *Service {
	return &Service{
		exports: make(map[string]*FileExport),
	}
}

// ─── GenerateExport ───────────────────────────────────────────────────────────

// GenerateExport creates a new export job, generates the SD3/SDIF content
// synchronously, and returns the completed job. Only owner, admin, and coach
// may generate exports.
func (s *Service) GenerateExport(principal *appmiddleware.Principal, orgID, meetID string, input GenerateExportInput) (*FileExport, *ExportAuditEvent, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}
	if err := validateGenerateInput(input); err != nil {
		return nil, nil, err
	}

	content, filename, err := buildFileContent(input)
	if err != nil {
		return nil, nil, fmt.Errorf("generate export: %w", err)
	}

	id := "export_" + randomToken(12)
	now := time.Now().UTC()

	job := &FileExport{
		ID:         id,
		OrgID:      orgID,
		MeetID:     meetID,
		ExportType: input.ExportType,
		Status:     "ready",
		Filename:   filename,
		SizeBytes:  len(content),
		CreatedAt:  now,
		UpdatedAt:  now,
		CreatedBy:  principal.UserID,
		UpdatedBy:  principal.UserID,
		content:    content,
	}

	s.mu.Lock()
	s.exports[id] = job
	s.mu.Unlock()

	audit := newAuditEvent(orgID, principal.UserID, "export.generated", "file_export", id, map[string]string{
		"export_type": input.ExportType,
		"meet_id":     meetID,
		"filename":    filename,
	})

	cp := *job
	cp.content = nil // don't include raw bytes in the domain response
	return &cp, audit, nil
}

// ─── ListExports ──────────────────────────────────────────────────────────────

// ListExports returns all export jobs for a given org and meet.
// Only owner, admin, and coach may list exports.
func (s *Service) ListExports(principal *appmiddleware.Principal, orgID, meetID string) ([]FileExport, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, ErrForbidden
	}

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
		cp := *ex
		cp.content = nil
		result = append(result, cp)
	}
	// Sort by created_at descending for stable ordering.
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

// ─── GetExport ────────────────────────────────────────────────────────────────

// GetExport returns the export job metadata for the given ID.
// Only owner, admin, and coach may get exports.
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
	cp := *ex
	cp.content = nil
	return &cp, nil
}

// ─── DownloadExport ───────────────────────────────────────────────────────────

// DownloadExport returns the generated file content for the given export job.
// Only owner, admin, and coach may download exports.
func (s *Service) DownloadExport(principal *appmiddleware.Principal, orgID, exportID string) (*FileExport, []byte, error) {
	if !hasAnyRole(principal, "owner", "admin", "coach") {
		return nil, nil, ErrForbidden
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ex, ok := s.exports[exportID]
	if !ok || ex.OrgID != orgID {
		return nil, nil, ErrExportNotFound
	}
	if ex.Status != "ready" {
		return nil, nil, ErrExportNotReady
	}

	cp := *ex
	cp.content = nil
	return &cp, ex.content, nil
}

// ─── SD3 file generator ───────────────────────────────────────────────────────

// buildFileContent generates the SD3/SDIF file bytes and returns them along
// with a suggested filename. This is an inline implementation of the SD3
// format (mirroring internal/meetfiles/sdif/exporter.go) so the server module
// remains independent of the meetfiles module in the scaffold phase.
//
// TODO(production): replace this inline implementation by wiring the server
// module to call internal/meetfiles/sdif.ExportStandardSD3 / ExportExtendedSD3 /
// ExportSDIFRoster directly. The duplication means bug fixes in the exporter
// library do not automatically apply here.
func buildFileContent(input GenerateExportInput) ([]byte, string, error) {
	var sb strings.Builder

	switch input.ExportType {
	case ExportTypeStandardSD3:
		writeSD3(&sb, input, false)
		return []byte(sb.String()), sanitizeFilename(input.MeetName) + "_standard.sd3", nil

	case ExportTypeExtendedSD3:
		writeSD3(&sb, input, true)
		return []byte(sb.String()), sanitizeFilename(input.MeetName) + "_extended.sd3", nil

	case ExportTypeSDIFRoster:
		writeRoster(&sb, input)
		return []byte(sb.String()), sanitizeFilename(input.MeetName) + "_roster.sd3", nil

	default:
		return nil, "", ErrInvalidExportType
	}
}

// ─── SD3 writer ───────────────────────────────────────────────────────────────

func writeSD3(sb *strings.Builder, input GenerateExportInput, extended bool) {
	// A0: file description (type 1 = entries)
	sb.WriteString(sd3Line("A0", func(buf []byte) {
		placeStr(buf, 4, 1, "1")
	}))

	// B1: meet info
	sb.WriteString(sd3Line("B1", func(buf []byte) {
		placeStr(buf, 12, 30, input.MeetName)
		placeStr(buf, 122, 8, input.MeetStartDate)
		placeStr(buf, 130, 8, input.MeetEndDate)
		placeStr(buf, 150, 1, encodeCourse(input.Course))
	}))

	// C1: team
	if input.HostTeamCode != "" || input.HostTeamName != "" {
		sb.WriteString(sd3Line("C1", func(buf []byte) {
			placeStr(buf, 12, 6, input.HostTeamCode)
			placeStr(buf, 18, 30, input.HostTeamName)
		}))
	}

	// Sort entries by event number, then swimmer last name
	sorted := make([]ExportEntry, len(input.Entries))
	copy(sorted, input.Entries)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].EventNumber != sorted[j].EventNumber {
			return eventNumberLess(sorted[i].EventNumber, sorted[j].EventNumber)
		}
		return sorted[i].Swimmer.LastName < sorted[j].Swimmer.LastName
	})

	// D0: individual entries
	for _, e := range sorted {
		sb.WriteString(sd3Line("D0", func(buf []byte) {
			placeStr(buf, 12, 28, formatName(e.Swimmer.LastName, e.Swimmer.FirstName))
			placeStr(buf, 40, 12, e.Swimmer.USSNumber)
			placeStr(buf, 52, 1, "A")
			placeStr(buf, 56, 8, e.Swimmer.DOBRaw)
			placeStr(buf, 66, 1, e.Swimmer.Gender)
			placeStr(buf, 67, 1, e.EventGender)
			placeRight(buf, 68, 4, fmt.Sprintf("%d", e.Distance))
			placeStr(buf, 72, 1, encodeStroke(e.Stroke))
			placeStr(buf, 73, 4, e.EventNumber)
			placeStr(buf, 77, 4, e.AgeGroup)
			if extended && e.SeedTimeMs > 0 {
				placeStr(buf, 89, 8, msToSDIFTime(e.SeedTimeMs))
				placeStr(buf, 97, 1, encodeCourse(e.Course))
			}
			if e.Heat > 0 {
				placeRight(buf, 129, 2, fmt.Sprintf("%d", e.Heat))
			}
			if e.Lane > 0 {
				placeRight(buf, 131, 2, fmt.Sprintf("%d", e.Lane))
			}
		}))
	}

	// Z0: file terminator
	sb.WriteString(sd3Line("Z0", func(_ []byte) {}))
}

// writeRoster writes an SDIF roster file (file type 5) with D1 records.
func writeRoster(sb *strings.Builder, input GenerateExportInput) {
	sb.WriteString(sd3Line("A0", func(buf []byte) {
		placeStr(buf, 4, 1, "5")
	}))
	sb.WriteString(sd3Line("B1", func(buf []byte) {
		placeStr(buf, 12, 30, input.MeetName)
		placeStr(buf, 122, 8, input.MeetStartDate)
		placeStr(buf, 130, 8, input.MeetEndDate)
		placeStr(buf, 150, 1, encodeCourse(input.Course))
	}))
	if input.HostTeamCode != "" || input.HostTeamName != "" {
		sb.WriteString(sd3Line("C1", func(buf []byte) {
			placeStr(buf, 12, 6, input.HostTeamCode)
			placeStr(buf, 18, 30, input.HostTeamName)
		}))
	}
	for _, e := range input.Entries {
		sb.WriteString(sd3Line("D1", func(buf []byte) {
			placeStr(buf, 12, 28, formatName(e.Swimmer.LastName, e.Swimmer.FirstName))
			placeStr(buf, 40, 12, e.Swimmer.USSNumber)
			placeStr(buf, 52, 1, "A")
			placeStr(buf, 56, 8, e.Swimmer.DOBRaw)
			placeStr(buf, 66, 1, e.Swimmer.Gender)
		}))
	}
	sb.WriteString(sd3Line("Z0", func(_ []byte) {}))
}

// ─── SD3 formatting helpers ───────────────────────────────────────────────────

// sd3Line builds a single 162-byte SD3 line (160 data chars + CRLF).
// The caller receives the data buffer and may place fields into it.
func sd3Line(recordType string, fill func(buf []byte)) string {
	buf := make([]byte, 160)
	for i := range buf {
		buf[i] = ' '
	}
	buf[0] = recordType[0]
	buf[1] = recordType[1]
	buf[2] = '1' // org code
	fill(buf)
	return string(buf) + "\r\n"
}

// placeStr places a left-justified, space-padded string at 1-indexed col start.
func placeStr(buf []byte, startOneBased, width int, value string) {
	begin := startOneBased - 1
	for i := 0; i < width; i++ {
		if begin+i >= len(buf) {
			break
		}
		if i < len(value) {
			buf[begin+i] = value[i]
		}
	}
}

// placeRight places a right-justified, space-padded string at 1-indexed col.
func placeRight(buf []byte, startOneBased, width int, value string) {
	begin := startOneBased - 1
	if len(value) > width {
		value = value[len(value)-width:]
	}
	pad := width - len(value)
	for i := 0; i < width; i++ {
		if begin+i >= len(buf) {
			break
		}
		if i >= pad {
			buf[begin+i] = value[i-pad]
		}
	}
}

// encodeCourse maps a course string to its SDIF single-char code.
func encodeCourse(c string) string {
	switch strings.ToUpper(strings.TrimSpace(c)) {
	case "SCY":
		return "Y"
	case "SCM":
		return "S"
	case "LCM":
		return "L"
	default:
		return " "
	}
}

// encodeStroke maps a stroke name to its SDIF single-char code.
func encodeStroke(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "free", "freestyle":
		return "1"
	case "back", "backstroke":
		return "2"
	case "breast", "breaststroke":
		return "3"
	case "fly", "butterfly":
		return "4"
	case "im", "individual_medley":
		return "5"
	case "free_relay":
		return "6"
	case "medley_relay":
		return "7"
	default:
		return " "
	}
}

// formatName returns "Last, First" for use in SDIF name fields.
func formatName(last, first string) string {
	if first == "" {
		return last
	}
	return last + ", " + first
}

// msToSDIFTime converts milliseconds to an 8-char SDIF time field string.
//
//   - Sub-minute:  "  ss.hh "
//   - 1–9 minutes: "m:ss.hh "
//   - 10+ minutes: "mm:ss.hh"
func msToSDIFTime(ms int) string {
	if ms <= 0 {
		return "NT      "
	}
	totalHundredths := int(math.Round(float64(ms) / 10.0))
	minutes := totalHundredths / 6000
	remaining := totalHundredths % 6000
	secs := remaining / 100
	hundredths := remaining % 100

	switch {
	case minutes >= 10:
		return fmt.Sprintf("%02d:%02d.%02d", minutes, secs, hundredths)
	case minutes >= 1:
		return fmt.Sprintf("%d:%02d.%02d ", minutes, secs, hundredths)
	default:
		return fmt.Sprintf("  %02d.%02d ", secs, hundredths)
	}
}

// eventNumberLess orders event number strings numerically when possible.
func eventNumberLess(a, b string) bool {
	var ai, bi int
	_, errA := fmt.Sscanf(strings.TrimSpace(a), "%d", &ai)
	_, errB := fmt.Sscanf(strings.TrimSpace(b), "%d", &bi)
	if errA == nil && errB == nil {
		return ai < bi
	}
	return a < b
}

// sanitizeFilename returns a filename-safe version of a meet name.
// Replaces any character that is not alphanumeric, a hyphen, or an underscore
// with an underscore. This also prevents broken Content-Disposition headers
// caused by characters like `"`, `?`, `*`, `<`, `>`, `|`, etc.
func sanitizeFilename(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" {
		return "export"
	}
	return s
}

// ─── Validation ───────────────────────────────────────────────────────────────

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

// ─── Helpers ──────────────────────────────────────────────────────────────────

func hasAnyRole(p *appmiddleware.Principal, roles ...string) bool {
	if p == nil {
		return false
	}
	for _, role := range p.Roles {
		if slices.Contains(roles, role) {
			return true
		}
	}
	return false
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("exports: rand.Read: " + err.Error())
	}
	return hex.EncodeToString(b)
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
