package performance

import (
	"context"
	"errors"
	"fmt"
	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"log/slog"
	"sort"
	"strings"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// Team records.
//
// Every record-worthy swim is stored as a RecordEntry keyed by
// course × gender × age group × event. The current record for a key is the
// fastest *accepted* entry (ties go to the earlier swim); the record history
// is the chronological progression of accepted entries that were faster than
// everything before them. Entries come from official results (source=result,
// detected live on result upsert/import or by a rebuild) or are entered by an
// admin for pre-platform history (source=manual). When the current record is
// locked, a faster swim is stored as a pending entry that an admin must
// approve before it becomes the record.

var (
	ErrRecordNotFound   = errors.New("record entry not found")
	ErrRecordInvalid    = errors.New("invalid record entry")
	ErrRecordNotPending = errors.New("record entry is not pending")
)

// Record entry statuses and sources.
const (
	RecordStatusAccepted = "accepted"
	RecordStatusPending  = "pending"
	RecordStatusRejected = "rejected"
	RecordSourceResult   = "result"
	RecordSourceManual   = "manual"
)

// RecordEntry is one swim in a team record's history.
type RecordEntry struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	Course      string     `json:"course"`
	Gender      string     `json:"gender"`
	AgeGroup    string     `json:"age_group"`
	Distance    int        `json:"distance"`
	Stroke      string     `json:"stroke"`
	TimeMs      int        `json:"time_ms"`
	SwimmerID   string     `json:"swimmer_id,omitempty"`
	SwimmerName string     `json:"swimmer_name"`
	SwimmerAge  *int       `json:"swimmer_age,omitempty"`
	SwimDate    string     `json:"swim_date"`
	MeetID      string     `json:"meet_id,omitempty"`
	MeetName    string     `json:"meet_name,omitempty"`
	ResultID    string     `json:"result_id,omitempty"`
	Source      string     `json:"source"`
	Status      string     `json:"status"`
	Locked      bool       `json:"locked"`
	Verified    bool       `json:"verified"`
	VerifiedBy  string     `json:"verified_by,omitempty"`
	VerifiedAt  *time.Time `json:"verified_at,omitempty"`
	Notes       string     `json:"notes,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CreatedBy   string     `json:"created_by"`
	UpdatedBy   string     `json:"updated_by"`
}

// RecordKey identifies one record slot.
type RecordKey struct {
	Course   string `json:"course"`
	Gender   string `json:"gender"`
	AgeGroup string `json:"age_group"`
	Distance int    `json:"distance"`
	Stroke   string `json:"stroke"`
}

func (e *RecordEntry) key() RecordKey {
	return RecordKey{Course: e.Course, Gender: e.Gender, AgeGroup: e.AgeGroup, Distance: e.Distance, Stroke: e.Stroke}
}

// Validate checks that a key names a real record slot.
func (k RecordKey) Validate() error {
	switch {
	case k.Course != "SCY" && k.Course != "SCM" && k.Course != "LCM":
		return fmt.Errorf("%w: course must be SCY, SCM or LCM", ErrRecordInvalid)
	case k.Gender != "M" && k.Gender != "F":
		return fmt.Errorf("%w: gender must be M or F", ErrRecordInvalid)
	case !ValidAgeGroup(k.AgeGroup):
		return fmt.Errorf("%w: unknown age group %q", ErrRecordInvalid, k.AgeGroup)
	case !isIndividualStroke(k.Stroke):
		return fmt.Errorf("%w: stroke must be free, back, breast, fly or im", ErrRecordInvalid)
	case k.Distance <= 0 || k.Distance > 1650:
		return fmt.Errorf("%w: distance out of range", ErrRecordInvalid)
	}
	return nil
}

// Label renders e.g. "Girls 11-12 100 SCY Freestyle".
func (k RecordKey) Label() string {
	who := map[string]string{"F": "Girls", "M": "Boys"}[k.Gender]
	group := k.AgeGroup
	switch k.AgeGroup {
	case AgeGroupOpen:
		who = map[string]string{"F": "Women", "M": "Men"}[k.Gender]
		group = "Open"
	case "8U":
		group = "8 & Under"
	}
	return fmt.Sprintf("%s %s %d %s %s", who, group, k.Distance, k.Course, strokeLabel(k.Stroke))
}

func strokeLabel(stroke string) string {
	switch stroke {
	case "free":
		return "Freestyle"
	case "back":
		return "Backstroke"
	case "breast":
		return "Breaststroke"
	case "fly":
		return "Butterfly"
	case "im":
		return "IM"
	}
	return stroke
}

// RecordBreak reports a team record broken by a newly recorded swim.
type RecordBreak struct {
	EntryID string `json:"entry_id"`
	RecordKey
	Label          string `json:"label"`
	TimeMs         int    `json:"time_ms"`
	PreviousTimeMs *int   `json:"previous_time_ms,omitempty"`
	PreviousHolder string `json:"previous_holder,omitempty"`
	SwimmerID      string `json:"swimmer_id"`
	SwimmerName    string `json:"swimmer_name"`
	SwimDate       string `json:"swim_date"`
	// Status is "accepted" (the swim is now the record) or "pending" (the
	// standing record is locked and an admin must approve the new one).
	Status string `json:"status"`
}

// TeamRecord is the current record for one slot plus the record it replaced.
type TeamRecord struct {
	RecordKey
	Label        string       `json:"label"`
	Record       *RecordEntry `json:"record"`
	Previous     *RecordEntry `json:"previous,omitempty"`
	HistoryCount int          `json:"history_count"`
	PendingCount int          `json:"pending_count"`
}

// RecordHistory is the full history for one slot.
type RecordHistory struct {
	RecordKey
	Label       string         `json:"label"`
	Current     *RecordEntry   `json:"current,omitempty"`
	Progression []*RecordEntry `json:"progression"` // newest first; first item is the current record
	Pending     []*RecordEntry `json:"pending"`
	Rejected    []*RecordEntry `json:"rejected"`
}

// ListRecordsFilters narrows the record board.
type ListRecordsFilters struct {
	Course   string
	Gender   string
	AgeGroup string
	Distance int
	Stroke   string
}

// CreateManualRecordInput adds a historical (pre-platform) record entry.
type CreateManualRecordInput struct {
	Course      string `json:"course"`
	Gender      string `json:"gender"`
	AgeGroup    string `json:"age_group"`
	Distance    int    `json:"distance"`
	Stroke      string `json:"stroke"`
	TimeMs      int    `json:"time_ms"`
	SwimmerID   string `json:"swimmer_id"`
	SwimmerName string `json:"swimmer_name"`
	SwimmerAge  *int   `json:"swimmer_age"`
	SwimDate    string `json:"swim_date"`
	MeetName    string `json:"meet_name"`
	Notes       string `json:"notes"`
	Locked      bool   `json:"locked"`
	Verified    bool   `json:"verified"`
}

// UpdateRecordEntryInput patches a record entry. Time, name, date and meet
// edits are allowed only on manual entries; result entries mirror a swim.
type UpdateRecordEntryInput struct {
	Locked      *bool   `json:"locked"`
	Verified    *bool   `json:"verified"`
	Notes       *string `json:"notes"`
	SwimmerName *string `json:"swimmer_name"`
	SwimDate    *string `json:"swim_date"`
	MeetName    *string `json:"meet_name"`
	TimeMs      *int    `json:"time_ms"`
}

// RebuildRecordsSummary describes a rebuild-from-results pass.
type RebuildRecordsSummary struct {
	SlotsEvaluated     int `json:"slots_evaluated"`
	SlotsSkippedLocked int `json:"slots_skipped_locked"`
	EntriesRemoved     int `json:"entries_removed"`
	EntriesCreated     int `json:"entries_created"`
}

// ─── Ordering helpers ────────────────────────────────────────────────────────

var strokeOrder = map[string]int{"free": 0, "back": 1, "breast": 2, "fly": 3, "im": 4}

func ageGroupIndex(code string) int {
	for i, g := range AgeGroups {
		if g == code {
			return i
		}
	}
	return len(AgeGroups)
}

func lessRecordKey(a, b RecordKey) bool {
	if a.Course != b.Course {
		return courseIndex(a.Course) < courseIndex(b.Course)
	}
	if a.Gender != b.Gender {
		return a.Gender < b.Gender
	}
	if a.AgeGroup != b.AgeGroup {
		return ageGroupIndex(a.AgeGroup) < ageGroupIndex(b.AgeGroup)
	}
	if a.Stroke != b.Stroke {
		return strokeOrder[a.Stroke] < strokeOrder[b.Stroke]
	}
	return a.Distance < b.Distance
}

func courseIndex(c string) int {
	switch c {
	case "SCY":
		return 0
	case "SCM":
		return 1
	case "LCM":
		return 2
	}
	return 3
}

// fasterEntry orders by time, then earlier swim, then creation.
func fasterEntry(a, b *RecordEntry) bool {
	if a.TimeMs != b.TimeMs {
		return a.TimeMs < b.TimeMs
	}
	if a.SwimDate != b.SwimDate {
		return a.SwimDate < b.SwimDate
	}
	return a.CreatedAt.Before(b.CreatedAt)
}

func currentRecordOf(entries []*RecordEntry) *RecordEntry {
	var cur *RecordEntry
	for _, e := range entries {
		if e.Status != RecordStatusAccepted {
			continue
		}
		if cur == nil || fasterEntry(e, cur) {
			cur = e
		}
	}
	return cur
}

// progressionOf returns accepted entries that were faster than every earlier
// accepted entry, newest first.
func progressionOf(entries []*RecordEntry) []*RecordEntry {
	accepted := make([]*RecordEntry, 0, len(entries))
	for _, e := range entries {
		if e.Status == RecordStatusAccepted {
			accepted = append(accepted, e)
		}
	}
	sort.Slice(accepted, func(i, j int) bool {
		a, b := accepted[i], accepted[j]
		if a.SwimDate != b.SwimDate {
			return a.SwimDate < b.SwimDate
		}
		// Same day (e.g. prelims then finals): the order the swims were
		// recorded is the order the record changed hands.
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.TimeMs < b.TimeMs
	})
	var prog []*RecordEntry
	best := 0
	for _, e := range accepted {
		if best == 0 || e.TimeMs < best {
			prog = append(prog, e)
			best = e.TimeMs
		}
	}
	for i, j := 0, len(prog)-1; i < j; i, j = i+1, j-1 {
		prog[i], prog[j] = prog[j], prog[i]
	}
	return prog
}

func (s *Service) entriesForKeyLocked(orgID string, key RecordKey) []*RecordEntry {
	var out []*RecordEntry
	for _, e := range s.recordEntries {
		if e.OrgID == orgID && e.key() == key {
			out = append(out, e)
		}
	}
	return out
}

func copyEntry(e *RecordEntry) *RecordEntry {
	if e == nil {
		return nil
	}
	cp := *e
	return &cp
}

func copyEntries(in []*RecordEntry) []*RecordEntry {
	out := make([]*RecordEntry, 0, len(in))
	for _, e := range in {
		out = append(out, copyEntry(e))
	}
	return out
}

func (s *Service) persistRecordEntryLocked(e *RecordEntry) error {
	if s.repo == nil {
		return nil
	}
	return s.repo.UpsertRecordEntry(context.Background(), e)
}

func (s *Service) appendAuditLocked(orgID, actorID, action, resource, resourceID string, meta map[string]any) {
	s.auditEvents = append(s.auditEvents, &BestTimeAuditEvent{
		ID:         dbutil.NewHexID(),
		OrgID:      orgID,
		ActorID:    actorID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Meta:       meta,
		CreatedAt:  time.Now().UTC(),
	})
}

// ─── Live detection ──────────────────────────────────────────────────────────

// detectRecordBreaksLocked checks a freshly stored result against the
// current records for its age group and Open. Caller holds s.mu.
func (s *Service) detectRecordBreaksLocked(orgID, actorID string, result *Result, dir directorySnapshot) []RecordBreak {
	if !qualifyingSwim(result) {
		return nil
	}
	facts := factsFor(result, dir)
	if facts.gender == "" {
		return nil
	}
	groups := []string{}
	if facts.age != nil {
		if g := AgeGroupForAge(*facts.age); g != "" {
			groups = append(groups, g)
		}
	}
	groups = append(groups, AgeGroupOpen)

	var breaks []RecordBreak
	now := time.Now().UTC()
	for _, group := range groups {
		key := RecordKey{Course: result.Course, Gender: facts.gender, AgeGroup: group, Distance: result.Distance, Stroke: result.Stroke}
		entries := s.entriesForKeyLocked(orgID, key)
		duplicate := false
		for _, e := range entries {
			if e.ResultID == result.ID {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		cur := currentRecordOf(entries)
		timeMs := *result.FinTimeMs
		if cur != nil && timeMs >= cur.TimeMs {
			continue
		}
		status := RecordStatusAccepted
		if cur != nil && cur.Locked {
			status = RecordStatusPending
		}
		entry := &RecordEntry{
			ID: s.newID(), OrgID: orgID,
			Course: key.Course, Gender: key.Gender, AgeGroup: key.AgeGroup, Distance: key.Distance, Stroke: key.Stroke,
			TimeMs: timeMs, SwimmerID: result.SwimmerID, SwimmerName: facts.swimmerName, SwimmerAge: facts.age,
			SwimDate: facts.date, MeetID: result.MeetID, MeetName: facts.meetName, ResultID: result.ID,
			Source: RecordSourceResult, Status: status,
			CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
		}
		if err := s.persistRecordEntryLocked(entry); err != nil {
			slog.Error("performance: persist record entry", "error", err, "result_id", result.ID)
			continue
		}
		s.recordEntries[entry.ID] = entry
		rb := RecordBreak{
			EntryID: entry.ID, RecordKey: key, Label: key.Label(), TimeMs: timeMs,
			SwimmerID: result.SwimmerID, SwimmerName: facts.swimmerName, SwimDate: facts.date, Status: status,
		}
		if cur != nil {
			prev := cur.TimeMs
			rb.PreviousTimeMs = &prev
			rb.PreviousHolder = cur.SwimmerName
		}
		breaks = append(breaks, rb)
		s.appendAuditLocked(orgID, actorID, "team_record.broken", "team_record_entry", entry.ID, map[string]any{
			"label": rb.Label, "time_ms": timeMs, "status": status, "result_id": result.ID,
		})
	}
	return breaks
}

// ─── Reads ───────────────────────────────────────────────────────────────────

// ListRecords returns the current team record for each slot matching filters.
// Readable by every club role; pending counts are only shown to staff.
func (s *Service) ListRecords(orgID string, principal *appmiddleware.Principal, filters ListRecordsFilters) ([]TeamRecord, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	staff := s.requireAdminOrCoach(principal) == nil
	s.mu.RLock()
	defer s.mu.RUnlock()

	byKey := map[RecordKey][]*RecordEntry{}
	for _, e := range s.recordEntries {
		if e.OrgID != orgID {
			continue
		}
		k := e.key()
		if filters.Course != "" && k.Course != filters.Course ||
			filters.Gender != "" && k.Gender != filters.Gender ||
			filters.AgeGroup != "" && k.AgeGroup != filters.AgeGroup ||
			filters.Stroke != "" && k.Stroke != filters.Stroke ||
			filters.Distance > 0 && k.Distance != filters.Distance {
			continue
		}
		byKey[k] = append(byKey[k], e)
	}
	out := make([]TeamRecord, 0, len(byKey))
	for k, entries := range byKey {
		cur := currentRecordOf(entries)
		pending := 0
		for _, e := range entries {
			if e.Status == RecordStatusPending {
				pending++
			}
		}
		if cur == nil && (!staff || pending == 0) {
			continue
		}
		prog := progressionOf(entries)
		tr := TeamRecord{RecordKey: k, Label: k.Label(), Record: copyEntry(cur), HistoryCount: len(prog)}
		if len(prog) > 1 {
			tr.Previous = copyEntry(prog[1])
		}
		if staff {
			tr.PendingCount = pending
		} else {
			redactEntry(tr.Record)
			redactEntry(tr.Previous)
		}
		out = append(out, tr)
	}
	sort.Slice(out, func(i, j int) bool { return lessRecordKey(out[i].RecordKey, out[j].RecordKey) })
	return out, nil
}

// redactEntry strips staff-only bookkeeping from entries shown to families.
func redactEntry(e *RecordEntry) {
	if e == nil {
		return
	}
	e.Notes = ""
	e.VerifiedBy = ""
	e.CreatedBy = ""
	e.UpdatedBy = ""
}

// GetRecordHistory returns the progression (and, for staff, pending and
// rejected entries) for one slot.
func (s *Service) GetRecordHistory(orgID string, principal *appmiddleware.Principal, key RecordKey) (*RecordHistory, error) {
	if err := s.requireReadAccess(principal); err != nil {
		return nil, err
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	staff := s.requireAdminOrCoach(principal) == nil
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := s.entriesForKeyLocked(orgID, key)
	h := &RecordHistory{
		RecordKey:   key,
		Label:       key.Label(),
		Current:     copyEntry(currentRecordOf(entries)),
		Progression: copyEntries(progressionOf(entries)),
		Pending:     []*RecordEntry{},
		Rejected:    []*RecordEntry{},
	}
	if staff {
		for _, e := range entries {
			switch e.Status {
			case RecordStatusPending:
				h.Pending = append(h.Pending, copyEntry(e))
			case RecordStatusRejected:
				h.Rejected = append(h.Rejected, copyEntry(e))
			}
		}
		sort.Slice(h.Pending, func(i, j int) bool { return fasterEntry(h.Pending[i], h.Pending[j]) })
		sort.Slice(h.Rejected, func(i, j int) bool { return h.Rejected[i].SwimDate > h.Rejected[j].SwimDate })
	} else {
		redactEntry(h.Current)
		for _, e := range h.Progression {
			redactEntry(e)
		}
	}
	return h, nil
}

// ListPendingRecords returns pending record entries awaiting admin approval.
func (s *Service) ListPendingRecords(orgID string, principal *appmiddleware.Principal) ([]*RecordEntry, error) {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*RecordEntry{}
	for _, e := range s.recordEntries {
		if e.OrgID == orgID && e.Status == RecordStatusPending {
			out = append(out, copyEntry(e))
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessRecordKey(out[i].key(), out[j].key()) })
	return out, nil
}

// GetRecordEntry returns one entry (staff only).
func (s *Service) GetRecordEntry(orgID string, principal *appmiddleware.Principal, entryID string) (*RecordEntry, error) {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.recordEntries[entryID]
	if !ok || e.OrgID != orgID {
		return nil, ErrRecordNotFound
	}
	return copyEntry(e), nil
}

// ─── Writes (admin) ──────────────────────────────────────────────────────────

func validSwimDate(v string) (string, error) {
	t, err := parseDateOnly(v)
	if err != nil {
		return "", fmt.Errorf("%w: swim_date must be YYYY-MM-DD", ErrRecordInvalid)
	}
	if t.After(time.Now().UTC().Add(24 * time.Hour)) {
		return "", fmt.Errorf("%w: swim_date cannot be in the future", ErrRecordInvalid)
	}
	if t.Year() < 1900 {
		return "", fmt.Errorf("%w: swim_date is too early", ErrRecordInvalid)
	}
	return t.Format("2006-01-02"), nil
}

// CreateManualRecord adds a historical record entry (admin only).
func (s *Service) CreateManualRecord(orgID, actorID string, principal *appmiddleware.Principal, input CreateManualRecordInput) (*RecordEntry, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	key := RecordKey{Course: strings.ToUpper(strings.TrimSpace(input.Course)), Gender: strings.ToUpper(strings.TrimSpace(input.Gender)), AgeGroup: strings.ToUpper(strings.TrimSpace(input.AgeGroup)), Distance: input.Distance, Stroke: strings.ToLower(strings.TrimSpace(input.Stroke))}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if input.TimeMs <= 0 || input.TimeMs > 3_600_000 {
		return nil, fmt.Errorf("%w: time_ms must be between 1 and 3600000", ErrRecordInvalid)
	}
	name := strings.TrimSpace(input.SwimmerName)
	swimmerID := strings.TrimSpace(input.SwimmerID)
	var dir directorySnapshot
	if swimmerID != "" {
		dir = s.snapshotDirectory(orgID)
		p, ok := dir.swimmers[swimmerID]
		if !ok {
			s.mu.RLock()
			hasDir := s.directory != nil
			s.mu.RUnlock()
			if hasDir {
				return nil, fmt.Errorf("%w: swimmer_id is not a club swimmer", ErrRecordInvalid)
			}
		} else if name == "" {
			name = p.DisplayName()
		}
	}
	if name == "" || len(name) > 120 {
		return nil, fmt.Errorf("%w: swimmer_name is required (max 120 chars)", ErrRecordInvalid)
	}
	date, err := validSwimDate(input.SwimDate)
	if err != nil {
		return nil, err
	}
	if len(input.Notes) > 1000 || len(input.MeetName) > 200 {
		return nil, fmt.Errorf("%w: notes/meet_name too long", ErrRecordInvalid)
	}
	if input.SwimmerAge != nil && (*input.SwimmerAge < 3 || *input.SwimmerAge > 120) {
		return nil, fmt.Errorf("%w: swimmer_age out of range", ErrRecordInvalid)
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := &RecordEntry{
		ID: s.newID(), OrgID: orgID,
		Course: key.Course, Gender: key.Gender, AgeGroup: key.AgeGroup, Distance: key.Distance, Stroke: key.Stroke,
		TimeMs: input.TimeMs, SwimmerID: swimmerID, SwimmerName: name, SwimmerAge: input.SwimmerAge,
		SwimDate: date, MeetName: strings.TrimSpace(input.MeetName), Notes: strings.TrimSpace(input.Notes),
		Source: RecordSourceManual, Status: RecordStatusAccepted, Locked: input.Locked, Verified: input.Verified,
		CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
	}
	if input.Verified {
		e.VerifiedBy = actorID
		e.VerifiedAt = &now
	}
	if err := s.persistRecordEntryLocked(e); err != nil {
		return nil, fmt.Errorf("persist record entry: %w", err)
	}
	s.recordEntries[e.ID] = e
	s.appendAuditLocked(orgID, actorID, "team_record.manual_created", "team_record_entry", e.ID, map[string]any{"label": key.Label(), "time_ms": e.TimeMs})
	return copyEntry(e), nil
}

// UpdateRecordEntry patches lock/verify/notes, and for manual entries the swim
// facts (admin only).
func (s *Service) UpdateRecordEntry(orgID, actorID, entryID string, principal *appmiddleware.Principal, input UpdateRecordEntryInput) (*RecordEntry, *RecordEntry, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.recordEntries[entryID]
	if !ok || e.OrgID != orgID {
		return nil, nil, ErrRecordNotFound
	}
	before := copyEntry(e)
	next := copyEntry(e)
	now := time.Now().UTC()
	factEdit := input.TimeMs != nil || input.SwimmerName != nil || input.SwimDate != nil || input.MeetName != nil
	if factEdit && e.Source != RecordSourceManual {
		return nil, nil, fmt.Errorf("%w: only manual entries can change time, swimmer, date or meet", ErrRecordInvalid)
	}
	if input.TimeMs != nil {
		if *input.TimeMs <= 0 || *input.TimeMs > 3_600_000 {
			return nil, nil, fmt.Errorf("%w: time_ms out of range", ErrRecordInvalid)
		}
		next.TimeMs = *input.TimeMs
	}
	if input.SwimmerName != nil {
		n := strings.TrimSpace(*input.SwimmerName)
		if n == "" || len(n) > 120 {
			return nil, nil, fmt.Errorf("%w: swimmer_name is required (max 120 chars)", ErrRecordInvalid)
		}
		next.SwimmerName = n
	}
	if input.SwimDate != nil {
		d, err := validSwimDate(*input.SwimDate)
		if err != nil {
			return nil, nil, err
		}
		next.SwimDate = d
	}
	if input.MeetName != nil {
		if len(*input.MeetName) > 200 {
			return nil, nil, fmt.Errorf("%w: meet_name too long", ErrRecordInvalid)
		}
		next.MeetName = strings.TrimSpace(*input.MeetName)
	}
	if input.Notes != nil {
		if len(*input.Notes) > 1000 {
			return nil, nil, fmt.Errorf("%w: notes too long", ErrRecordInvalid)
		}
		next.Notes = strings.TrimSpace(*input.Notes)
	}
	if input.Locked != nil {
		if *input.Locked && e.Status != RecordStatusAccepted {
			return nil, nil, fmt.Errorf("%w: only accepted entries can be locked", ErrRecordInvalid)
		}
		next.Locked = *input.Locked
	}
	if input.Verified != nil && *input.Verified != e.Verified {
		next.Verified = *input.Verified
		if next.Verified {
			next.VerifiedBy = actorID
			next.VerifiedAt = &now
		} else {
			next.VerifiedBy = ""
			next.VerifiedAt = nil
		}
	}
	next.UpdatedAt = now
	next.UpdatedBy = actorID
	if err := s.persistRecordEntryLocked(next); err != nil {
		return nil, nil, fmt.Errorf("persist record entry: %w", err)
	}
	s.recordEntries[e.ID] = next
	s.appendAuditLocked(orgID, actorID, "team_record.updated", "team_record_entry", e.ID, map[string]any{"locked": next.Locked, "verified": next.Verified})
	return before, copyEntry(next), nil
}

// DecideRecordEntry approves (pending → accepted) or rejects (pending or
// accepted result entry → rejected) a record entry. Admin only.
func (s *Service) DecideRecordEntry(orgID, actorID, entryID string, principal *appmiddleware.Principal, approve bool, notes string) (*RecordEntry, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.recordEntries[entryID]
	if !ok || e.OrgID != orgID {
		return nil, ErrRecordNotFound
	}
	next := copyEntry(e)
	now := time.Now().UTC()
	if approve {
		if e.Status != RecordStatusPending {
			return nil, ErrRecordNotPending
		}
		next.Status = RecordStatusAccepted
		next.Verified = true
		next.VerifiedBy = actorID
		next.VerifiedAt = &now
	} else {
		if e.Status == RecordStatusRejected {
			return nil, ErrRecordNotPending
		}
		if e.Source == RecordSourceManual {
			return nil, fmt.Errorf("%w: delete manual entries instead of rejecting them", ErrRecordInvalid)
		}
		next.Status = RecordStatusRejected
		next.Locked = false
	}
	if n := strings.TrimSpace(notes); n != "" {
		if len(n) > 1000 {
			return nil, fmt.Errorf("%w: notes too long", ErrRecordInvalid)
		}
		next.Notes = n
	}
	next.UpdatedAt = now
	next.UpdatedBy = actorID
	if err := s.persistRecordEntryLocked(next); err != nil {
		return nil, fmt.Errorf("persist record entry: %w", err)
	}
	s.recordEntries[e.ID] = next
	action := "team_record.rejected"
	if approve {
		action = "team_record.approved"
	}
	s.appendAuditLocked(orgID, actorID, action, "team_record_entry", e.ID, map[string]any{"label": e.key().Label()})
	return copyEntry(next), nil
}

// DeleteManualRecord removes a manual entry (admin only). Result-sourced
// entries must be rejected so a rebuild does not re-create them.
func (s *Service) DeleteManualRecord(orgID, actorID, entryID string, principal *appmiddleware.Principal) (*RecordEntry, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.recordEntries[entryID]
	if !ok || e.OrgID != orgID {
		return nil, ErrRecordNotFound
	}
	if e.Source != RecordSourceManual {
		return nil, fmt.Errorf("%w: only manual entries can be deleted; reject result entries instead", ErrRecordInvalid)
	}
	if s.repo != nil {
		if err := s.repo.DeleteRecordEntry(context.Background(), e.OrgID, e.ID); err != nil {
			return nil, fmt.Errorf("delete record entry: %w", err)
		}
	}
	delete(s.recordEntries, e.ID)
	s.appendAuditLocked(orgID, actorID, "team_record.deleted", "team_record_entry", e.ID, map[string]any{"label": e.key().Label()})
	return copyEntry(e), nil
}

// RebuildRecords recomputes result-sourced record entries from every official
// result. Slots whose current record is locked are left untouched; manual,
// verified and rejected entries are preserved (rejected swims stay excluded).
func (s *Service) RebuildRecords(orgID, actorID string, principal *appmiddleware.Principal) (RebuildRecordsSummary, error) {
	var summary RebuildRecordsSummary
	if err := s.requireAdmin(principal); err != nil {
		return summary, err
	}
	dir := s.snapshotDirectory(orgID)
	s.mu.Lock()
	defer s.mu.Unlock()

	type cand struct {
		facts swimFacts
		key   RecordKey
	}
	candidates := map[RecordKey][]cand{}
	for _, r := range s.results {
		if r.OrgID != orgID || !qualifyingSwim(r) {
			continue
		}
		f := factsFor(r, dir)
		if f.gender == "" {
			continue
		}
		groups := []string{AgeGroupOpen}
		if f.age != nil {
			if g := AgeGroupForAge(*f.age); g != "" {
				groups = append(groups, g)
			}
		}
		for _, g := range groups {
			k := RecordKey{Course: r.Course, Gender: f.gender, AgeGroup: g, Distance: r.Distance, Stroke: r.Stroke}
			candidates[k] = append(candidates[k], cand{facts: f, key: k})
		}
	}
	existing := map[RecordKey][]*RecordEntry{}
	for _, e := range s.recordEntries {
		if e.OrgID == orgID {
			existing[e.key()] = append(existing[e.key()], e)
		}
	}
	keys := map[RecordKey]bool{}
	for k := range candidates {
		keys[k] = true
	}
	for k := range existing {
		keys[k] = true
	}
	now := time.Now().UTC()
	for k := range keys {
		summary.SlotsEvaluated++
		entries := existing[k]
		if cur := currentRecordOf(entries); cur != nil && cur.Locked {
			summary.SlotsSkippedLocked++
			continue
		}
		var kept []*RecordEntry
		excluded := map[string]bool{}
		for _, e := range entries {
			removable := e.Source == RecordSourceResult && !e.Verified && !e.Locked && e.Status != RecordStatusRejected
			if !removable {
				kept = append(kept, e)
				if e.ResultID != "" {
					excluded[e.ResultID] = true
				}
				continue
			}
			if s.repo != nil {
				if err := s.repo.DeleteRecordEntry(context.Background(), e.OrgID, e.ID); err != nil {
					return summary, fmt.Errorf("delete record entry: %w", err)
				}
			}
			delete(s.recordEntries, e.ID)
			summary.EntriesRemoved++
		}
		cs := candidates[k]
		sort.Slice(cs, func(i, j int) bool {
			a, b := cs[i].facts, cs[j].facts
			if a.date != b.date {
				return a.date < b.date
			}
			if *a.result.FinTimeMs != *b.result.FinTimeMs {
				return *a.result.FinTimeMs < *b.result.FinTimeMs
			}
			return a.result.ID < b.result.ID
		})
		var added []*RecordEntry
		for _, c := range cs {
			r := c.facts.result
			if excluded[r.ID] {
				continue
			}
			best := 0
			for _, e := range kept {
				if e.Status == RecordStatusAccepted && e.SwimDate <= c.facts.date && (best == 0 || e.TimeMs < best) {
					best = e.TimeMs
				}
			}
			for _, e := range added {
				if best == 0 || e.TimeMs < best {
					best = e.TimeMs
				}
			}
			if best != 0 && *r.FinTimeMs >= best {
				continue
			}
			e := &RecordEntry{
				ID: s.newID(), OrgID: orgID,
				Course: k.Course, Gender: k.Gender, AgeGroup: k.AgeGroup, Distance: k.Distance, Stroke: k.Stroke,
				TimeMs: *r.FinTimeMs, SwimmerID: r.SwimmerID, SwimmerName: c.facts.swimmerName, SwimmerAge: c.facts.age,
				SwimDate: c.facts.date, MeetID: r.MeetID, MeetName: c.facts.meetName, ResultID: r.ID,
				Source: RecordSourceResult, Status: RecordStatusAccepted,
				CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
			}
			if err := s.persistRecordEntryLocked(e); err != nil {
				return summary, fmt.Errorf("persist record entry: %w", err)
			}
			s.recordEntries[e.ID] = e
			added = append(added, e)
			summary.EntriesCreated++
		}
	}
	s.appendAuditLocked(orgID, actorID, "team_record.rebuilt", "team_record", orgID, map[string]any{
		"entries_created": summary.EntriesCreated, "entries_removed": summary.EntriesRemoved,
	})
	return summary, nil
}

// FormatSwimTime renders milliseconds as M:SS.hh or SS.hh.
func FormatSwimTime(ms int) string {
	hundredths := (ms + 5) / 10
	minutes := hundredths / 6000
	seconds := (hundredths % 6000) / 100
	frac := hundredths % 100
	if minutes > 0 {
		return fmt.Sprintf("%d:%02d.%02d", minutes, seconds, frac)
	}
	return fmt.Sprintf("%d.%02d", seconds, frac)
}

// RecordAnnouncementDraft builds an announcement title/body celebrating a
// record entry (staff only). The entry must be accepted.
func (s *Service) RecordAnnouncementDraft(orgID string, principal *appmiddleware.Principal, entryID string) (string, string, error) {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return "", "", err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.recordEntries[entryID]
	if !ok || e.OrgID != orgID {
		return "", "", ErrRecordNotFound
	}
	if e.Status != RecordStatusAccepted {
		return "", "", fmt.Errorf("%w: only accepted records can be announced", ErrRecordInvalid)
	}
	key := e.key()
	prog := progressionOf(s.entriesForKeyLocked(orgID, key))
	var prev *RecordEntry
	for i, p := range prog {
		if p.ID == e.ID && i+1 < len(prog) {
			prev = prog[i+1]
		}
	}
	title := fmt.Sprintf("New team record: %s, %s", e.SwimmerName, key.Label())
	var b strings.Builder
	fmt.Fprintf(&b, "%s set a new team record in the %s with a time of %s", e.SwimmerName, key.Label(), FormatSwimTime(e.TimeMs))
	if e.MeetName != "" {
		fmt.Fprintf(&b, " at %s", e.MeetName)
	}
	if e.SwimDate != "" {
		fmt.Fprintf(&b, " on %s", e.SwimDate)
	}
	b.WriteString(".")
	if prev != nil {
		fmt.Fprintf(&b, " The previous record of %s was held by %s", FormatSwimTime(prev.TimeMs), prev.SwimmerName)
		if len(prev.SwimDate) >= 4 {
			fmt.Fprintf(&b, " since %s", prev.SwimDate[:4])
		}
		b.WriteString(".")
	}
	b.WriteString(" Congratulations!")
	return title, b.String(), nil
}
