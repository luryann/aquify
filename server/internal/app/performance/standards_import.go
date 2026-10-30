package performance

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// Time-standards file import.
//
// Format: UTF-8 CSV with a header row (column order is free, names are
// case-insensitive, spaces/dashes are treated as underscores). Columns:
//
//	standard_name   required  display name, e.g. "2026 Senior Cuts"
//	standard_code   optional  level, e.g. "A", "AA"; defaults to standard_name
//	season_year     required  4-digit year, or "2025-2026" (the end year is used);
//	                          header alias: season
//	course          required  SCY | SCM | LCM
//	gender          required  M | F | X (also boys/girls/male/female/mixed)
//	age_group       optional  "11-12", "10&U", "15&O", "13" or "OPEN"/blank = all ages;
//	                          alternatively age_min and/or age_max
//	event_distance  required* distance; with stroke. Alias: distance
//	stroke          required* free|back|breast|fly|im (also freestyle, butterfly, ...)
//	event           alternative to event_distance+stroke, e.g. "100 Free"
//	cut_time        required  "58.12" or "1:02.45"; alias: time
//	description     optional
//
// Only individual events are supported because time standards model individual
// strokes. XLSX is intentionally not supported (no zip/xml spreadsheet reader
// in the dependency set); export the sheet as CSV instead.

// Import limits.
const (
	MaxStandardsImportRows = 5000
)

// Import actions assigned to each row by PlanTimeStandardsImport.
const (
	ImportActionCreate    = "create"
	ImportActionUpdate    = "update"
	ImportActionUnchanged = "unchanged"
)

// ErrImportHasErrors is returned by commit when any row is invalid; nothing is
// written in that case.
var ErrImportHasErrors = errors.New("time standards import has row errors")

// StandardsImportRow is one parsed CSV row plus its planned action.
type StandardsImportRow struct {
	Line          int      `json:"line"`
	Name          string   `json:"standard_name"`
	StandardCode  string   `json:"standard_code"`
	SeasonYear    int      `json:"season_year"`
	Course        string   `json:"course"`
	Gender        string   `json:"gender"`
	AgeMin        *int     `json:"age_min,omitempty"`
	AgeMax        *int     `json:"age_max,omitempty"`
	EventDistance int      `json:"event_distance"`
	Stroke        string   `json:"stroke"`
	TimeMs        int      `json:"time_ms"`
	Description   string   `json:"description,omitempty"`
	Action        string   `json:"action,omitempty"`
	ExistingID    string   `json:"existing_id,omitempty"`
	Errors        []string `json:"errors"`
}

// StandardsImportCounts summarises a plan.
type StandardsImportCounts struct {
	Total     int `json:"total"`
	Valid     int `json:"valid"`
	Errors    int `json:"errors"`
	Create    int `json:"create"`
	Update    int `json:"update"`
	Unchanged int `json:"unchanged"`
}

// Counts tallies rows by validity and planned action.
func CountStandardsImport(rows []StandardsImportRow) StandardsImportCounts {
	c := StandardsImportCounts{Total: len(rows)}
	for _, r := range rows {
		if len(r.Errors) > 0 {
			c.Errors++
			continue
		}
		c.Valid++
		switch r.Action {
		case ImportActionCreate:
			c.Create++
		case ImportActionUpdate:
			c.Update++
		case ImportActionUnchanged:
			c.Unchanged++
		}
	}
	return c
}

var headerAliases = map[string]string{
	"standard_name": "standard_name", "name": "standard_name", "standard": "standard_name",
	"standard_code": "standard_code", "code": "standard_code", "level": "standard_code",
	"season_year": "season_year", "season": "season_year", "year": "season_year",
	"course":    "course",
	"gender":    "gender",
	"age_group": "age_group", "age": "age_group",
	"age_min": "age_min", "age_max": "age_max",
	"event_distance": "event_distance", "distance": "event_distance",
	"stroke": "stroke", "event": "event",
	"cut_time": "cut_time", "time": "cut_time", "cut": "cut_time",
	"description": "description",
}

func normalizeHeader(h string) string {
	h = strings.TrimPrefix(h, "\ufeff")
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.NewReplacer(" ", "_", "-", "_").Replace(h)
	return headerAliases[h]
}

// ParseTimeStandardsCSV parses a CSV file into rows. Structural problems (no
// header, missing required columns, too many rows) return an error; per-row
// problems are collected on the row's Errors. Duplicate rows within the file
// are flagged.
func ParseTimeStandardsCSV(data []byte) ([]StandardsImportRow, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	rd := csv.NewReader(bytes.NewReader(data))
	rd.FieldsPerRecord = -1
	rd.TrimLeadingSpace = true
	rd.LazyQuotes = true

	header, err := rd.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("file is empty")
		}
		return nil, fmt.Errorf("could not read CSV header: %w", err)
	}
	cols := map[string]int{}
	for i, h := range header {
		if key := normalizeHeader(h); key != "" {
			if _, dup := cols[key]; !dup {
				cols[key] = i
			}
		}
	}
	var missing []string
	for _, req := range []string{"standard_name", "season_year", "course", "gender", "cut_time"} {
		if _, ok := cols[req]; !ok {
			missing = append(missing, req)
		}
	}
	_, hasEvent := cols["event"]
	_, hasDist := cols["event_distance"]
	_, hasStroke := cols["stroke"]
	if !hasEvent && !(hasDist && hasStroke) {
		missing = append(missing, "event_distance+stroke (or event)")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required column(s): %s", strings.Join(missing, ", "))
	}

	get := func(rec []string, key string) string {
		i, ok := cols[key]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	var rows []StandardsImportRow
	seen := map[string]int{}
	for {
		rec, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line, _ := rd.FieldPos(0)
		if err != nil {
			rows = append(rows, StandardsImportRow{Line: line, Errors: []string{"malformed CSV row: " + err.Error()}})
			continue
		}
		blank := true
		for _, f := range rec {
			if strings.TrimSpace(f) != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		if len(rows) >= MaxStandardsImportRows {
			return nil, fmt.Errorf("file has more than %d rows", MaxStandardsImportRows)
		}
		row := parseStandardsRow(line, get, rec)
		if len(row.Errors) == 0 {
			k := standardIdentity(row.StandardCode, row.SeasonYear, row.EventDistance, row.Stroke, row.Course, row.Gender, row.AgeMin, row.AgeMax)
			if first, dup := seen[k]; dup {
				row.Errors = append(row.Errors, fmt.Sprintf("duplicate of line %d (same standard, event, course, gender and age group)", first))
			} else {
				seen[k] = line
			}
		}
		if row.Errors == nil {
			row.Errors = []string{}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, errors.New("file has a header but no data rows")
	}
	return rows, nil
}

func parseStandardsRow(line int, get func([]string, string) string, rec []string) StandardsImportRow {
	row := StandardsImportRow{Line: line, Errors: []string{}}
	fail := func(format string, a ...any) { row.Errors = append(row.Errors, fmt.Sprintf(format, a...)) }

	row.Name = get(rec, "standard_name")
	if row.Name == "" {
		fail("standard_name is required")
	}
	row.StandardCode = get(rec, "standard_code")
	if row.StandardCode == "" {
		row.StandardCode = row.Name
	}
	row.Description = get(rec, "description")

	if y, err := parseSeasonYear(get(rec, "season_year")); err != nil {
		fail("%v", err)
	} else {
		row.SeasonYear = y
	}

	switch c := strings.ToUpper(strings.ReplaceAll(get(rec, "course"), " ", "")); c {
	case "SCY", "SCM", "LCM":
		row.Course = c
	default:
		fail("course %q must be SCY, SCM or LCM", get(rec, "course"))
	}

	if g, ok := normalizeStandardGender(get(rec, "gender")); ok {
		row.Gender = g
	} else {
		fail("gender %q must be M, F or X", get(rec, "gender"))
	}

	// Age group: explicit min/max columns win over age_group.
	minRaw, maxRaw := get(rec, "age_min"), get(rec, "age_max")
	if minRaw != "" || maxRaw != "" {
		for _, p := range []struct {
			raw string
			dst **int
			nm  string
		}{{minRaw, &row.AgeMin, "age_min"}, {maxRaw, &row.AgeMax, "age_max"}} {
			if p.raw == "" {
				continue
			}
			n, err := strconv.Atoi(p.raw)
			if err != nil || n < 0 || n > 120 {
				fail("%s %q is not a valid age", p.nm, p.raw)
				continue
			}
			*p.dst = &n
		}
	} else if lo, hi, err := parseAgeGroup(get(rec, "age_group")); err != nil {
		fail("%v", err)
	} else {
		row.AgeMin, row.AgeMax = lo, hi
	}
	if row.AgeMin != nil && row.AgeMax != nil && *row.AgeMin > *row.AgeMax {
		fail("age_min (%d) is greater than age_max (%d)", *row.AgeMin, *row.AgeMax)
	}

	dist, stroke := get(rec, "event_distance"), get(rec, "stroke")
	if ev := get(rec, "event"); ev != "" && (dist == "" || stroke == "") {
		parts := strings.Fields(ev)
		if len(parts) >= 2 {
			dist, stroke = parts[0], strings.Join(parts[1:], " ")
		} else {
			fail("event %q must look like \"100 Free\"", ev)
		}
	}
	if n, err := strconv.Atoi(strings.TrimSpace(dist)); err != nil || n <= 0 || n > 5000 {
		fail("event distance %q must be a positive whole number", dist)
	} else {
		row.EventDistance = n
	}
	if s, ok := normalizeStroke(stroke); ok {
		row.Stroke = s
	} else {
		fail("stroke %q must be free, back, breast, fly or im", stroke)
	}

	if ms, err := ParseSwimTime(get(rec, "cut_time")); err != nil {
		fail("cut_time: %v", err)
	} else {
		row.TimeMs = ms
	}
	return row
}

var seasonRange = regexp.MustCompile(`^(\d{4})\s*[-/]\s*(\d{2,4})$`)

func parseSeasonYear(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	year := 0
	if m := seasonRange.FindStringSubmatch(raw); m != nil {
		start, _ := strconv.Atoi(m[1])
		if len(m[2]) == 2 {
			year = start/100*100 + atoiOr0(m[2])
			if year < start {
				year += 100
			}
		} else {
			year = atoiOr0(m[2])
		}
	} else if n, err := strconv.Atoi(raw); err == nil {
		year = n
	} else {
		return 0, fmt.Errorf("season_year %q must be a 4-digit year or a range like 2025-2026", raw)
	}
	if year <= 2000 || year > 2200 {
		return 0, fmt.Errorf("season_year %d is out of range", year)
	}
	return year, nil
}

func atoiOr0(s string) int { n, _ := strconv.Atoi(s); return n }

func normalizeStandardGender(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "m", "male", "boy", "boys", "men", "man":
		return "M", true
	case "f", "female", "girl", "girls", "women", "woman":
		return "F", true
	case "x", "mixed", "mix", "open", "coed":
		return "X", true
	}
	return "", false
}

func normalizeStroke(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "free", "freestyle", "fr":
		return "free", true
	case "back", "backstroke", "bk":
		return "back", true
	case "breast", "breaststroke", "br":
		return "breast", true
	case "fly", "butterfly", "fl":
		return "fly", true
	case "im", "individual medley", "medley", "i.m.":
		return "im", true
	}
	return "", false
}

var (
	ageRange   = regexp.MustCompile(`^(\d{1,3})\s*-\s*(\d{1,3})$`)
	ageUnder   = regexp.MustCompile(`^(\d{1,3})\s*(?:&|and)?\s*(?:u|under)$`)
	ageOver    = regexp.MustCompile(`^(\d{1,3})\s*(?:&\s*o|and\s*over|o|over|\+)$`)
	ageSingle  = regexp.MustCompile(`^\d{1,3}$`)
	ageOpenSet = map[string]bool{"": true, "open": true, "all": true, "any": true}
)

// parseAgeGroup understands "11-12", "10&U", "15&O"/"15+", "13" and OPEN/blank.
func parseAgeGroup(raw string) (lo, hi *int, err error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if ageOpenSet[s] {
		return nil, nil, nil
	}
	ptr := func(v string) *int { n := atoiOr0(v); return &n }
	switch {
	case ageRange.MatchString(s):
		m := ageRange.FindStringSubmatch(s)
		lo, hi = ptr(m[1]), ptr(m[2])
		if *lo > *hi {
			return nil, nil, fmt.Errorf("age_group %q has min greater than max", raw)
		}
		return lo, hi, nil
	case ageUnder.MatchString(s):
		return nil, ptr(ageUnder.FindStringSubmatch(s)[1]), nil
	case ageOver.MatchString(s):
		return ptr(ageOver.FindStringSubmatch(s)[1]), nil, nil
	case ageSingle.MatchString(s):
		return ptr(s), ptr(s), nil
	}
	return nil, nil, fmt.Errorf("age_group %q must look like 11-12, 10&U, 15&O or OPEN", raw)
}

var secondsText = regexp.MustCompile(`^\d*\.?\d*$`)

// ParseSwimTime converts "58.12", "1:02.45" or "1:02:45" (min:sec:hundredths)
// into milliseconds. Times must be positive and under 60 minutes.
func ParseSwimTime(raw string) (int, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, errors.New("a time is required")
	}
	minutes := 0
	rest := s
	if strings.Count(s, ":") == 2 {
		// m:ss:hh legacy form
		p := strings.Split(s, ":")
		s = p[0] + ":" + p[1] + "." + p[2]
		rest = s
	}
	if i := strings.Index(rest, ":"); i >= 0 {
		m, err := strconv.Atoi(rest[:i])
		if err != nil || m < 0 || m >= 60 {
			return 0, fmt.Errorf("%q is not a valid time", raw)
		}
		minutes = m
		rest = rest[i+1:]
	}
	// Digits and one optional decimal point only: ParseFloat would also accept
	// "Inf", "NaN" and hex floats like "0x1p3".
	secs, err := strconv.ParseFloat(rest, 64)
	if err != nil || !secondsText.MatchString(rest) || secs < 0 || secs >= 3600 || (minutes > 0 && secs >= 60) {
		return 0, fmt.Errorf("%q is not a valid time (use 58.12 or 1:02.45)", raw)
	}
	if i := strings.Index(rest, "."); i >= 0 && len(rest)-i-1 > 3 {
		return 0, fmt.Errorf("%q has too many decimal places", raw)
	}
	ms := minutes*60000 + int(secs*1000+0.5)
	if ms <= 0 {
		return 0, fmt.Errorf("%q must be greater than zero", raw)
	}
	return ms, nil
}

func standardIdentity(code string, year, dist int, stroke, course, gender string, ageMin, ageMax *int) string {
	a := func(p *int) string {
		if p == nil {
			return "-"
		}
		return strconv.Itoa(*p)
	}
	return strings.ToLower(strings.TrimSpace(code)) + "|" + strconv.Itoa(year) + "|" + strconv.Itoa(dist) + "|" + stroke + "|" + course + "|" + gender + "|" + a(ageMin) + "|" + a(ageMax)
}

// PlanTimeStandardsImport classifies each valid row as create/update/unchanged
// against the org's current standards. It never writes. Admin only.
func (s *Service) PlanTimeStandardsImport(orgID string, principal *appmiddleware.Principal, rows []StandardsImportRow) ([]StandardsImportRow, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, ErrForbidden
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.planStandardsImportLocked(orgID, rows), nil
}

func (s *Service) planStandardsImportLocked(orgID string, rows []StandardsImportRow) []StandardsImportRow {
	existing := map[string]*TimeStandard{}
	for _, ts := range s.standards {
		if ts.OrgID != orgID || ts.DeletedAt != nil {
			continue
		}
		k := standardIdentity(ts.StandardCode, ts.SeasonYear, ts.EventDistance, ts.Stroke, ts.Course, ts.Gender, ts.AgeMin, ts.AgeMax)
		if cur, ok := existing[k]; !ok || ts.CreatedAt.Before(cur.CreatedAt) {
			existing[k] = ts
		}
	}
	out := make([]StandardsImportRow, len(rows))
	copy(out, rows)
	for i := range out {
		r := &out[i]
		if len(r.Errors) > 0 {
			continue
		}
		k := standardIdentity(r.StandardCode, r.SeasonYear, r.EventDistance, r.Stroke, r.Course, r.Gender, r.AgeMin, r.AgeMax)
		cur, ok := existing[k]
		switch {
		case !ok:
			r.Action = ImportActionCreate
		case cur.TimeMs == r.TimeMs && cur.Name == r.Name && (r.Description == "" || cur.Description == r.Description):
			r.Action, r.ExistingID = ImportActionUnchanged, cur.ID
		default:
			r.Action, r.ExistingID = ImportActionUpdate, cur.ID
		}
	}
	return out
}

// CommitTimeStandardsImport creates/updates standards for every row atomically:
// with a database every write happens in one transaction, and the in-memory
// cache is only updated after it commits. Any invalid row aborts the whole
// import with ErrImportHasErrors. Admin only.
func (s *Service) CommitTimeStandardsImport(orgID, actorID string, principal *appmiddleware.Principal, rows []StandardsImportRow) ([]StandardsImportRow, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, ErrForbidden
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	planned := s.planStandardsImportLocked(orgID, rows)
	for _, r := range planned {
		if len(r.Errors) > 0 {
			return planned, ErrImportHasErrors
		}
	}
	now := time.Now().UTC()
	var writes []*TimeStandard
	for _, r := range planned {
		switch r.Action {
		case ImportActionCreate:
			writes = append(writes, &TimeStandard{
				ID: s.newID(), OrgID: orgID, Name: r.Name, SeasonYear: r.SeasonYear, StandardCode: r.StandardCode,
				Description: r.Description, EventDistance: r.EventDistance, Stroke: r.Stroke, Course: r.Course,
				Gender: r.Gender, AgeMin: r.AgeMin, AgeMax: r.AgeMax, TimeMs: r.TimeMs, IsActive: true,
				CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
			})
		case ImportActionUpdate:
			cur := s.standards[r.ExistingID]
			cp := *cur
			cp.Name, cp.TimeMs, cp.UpdatedAt, cp.UpdatedBy = r.Name, r.TimeMs, now, actorID
			if r.Description != "" {
				cp.Description = r.Description
			}
			writes = append(writes, &cp)
		}
	}
	if s.repo != nil && len(writes) > 0 {
		if err := s.repo.UpsertTimeStandardsTx(context.Background(), writes); err != nil {
			return nil, fmt.Errorf("persist time standards import: %w", err)
		}
	}
	for _, ts := range writes {
		s.standards[ts.ID] = ts
	}
	return planned, nil
}
