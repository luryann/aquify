// Package meetfiles reads and writes swim meet interchange files: Hy-Tek meet
// event files (.hyv/.ev3), SDIF results (.sd3) and the entry bundle for swim
// meet interoperability.  It produces a normalized ParseResult that the import
// service uses to match swimmers, queue conflicts, and build reports.
package meetfiles

import "time"

// ─── File type ────────────────────────────────────────────────────────────────

// FileType indicates the format of a meet file.
type FileType string

const (
	FileTypeSD3 FileType = "sd3"
	FileTypeZIP FileType = "zip"
	FileTypeHY3 FileType = "hy3"
	FileTypeHYV FileType = "hyv"
	FileTypeEV3 FileType = "ev3"
)

// ─── Domain enums ─────────────────────────────────────────────────────────────

// Course identifies the pool course.
type Course string

const (
	CourseShortCourseYards  Course = "SCY"
	CourseShortCourseMeters Course = "SCM"
	CourseLongCourseMeters  Course = "LCM"
	CourseUnknown           Course = ""
)

// Stroke identifies a swimming stroke.
type Stroke string

const (
	StrokeFreestyle      Stroke = "free"
	StrokeBackstroke     Stroke = "back"
	StrokeBreaststroke   Stroke = "breast"
	StrokeButterfly      Stroke = "fly"
	StrokeIM             Stroke = "im"
	StrokeFreestyleRelay Stroke = "free_relay"
	StrokeMedleyRelay    Stroke = "medley_relay"
	StrokeUnknown        Stroke = ""
)

// ─── Parsed identity ──────────────────────────────────────────────────────────

// ParsedSwimmer holds swimmer identity fields extracted from a file record.
type ParsedSwimmer struct {
	LastName  string
	FirstName string
	DOB       *time.Time // nil when absent or unparseable
	Gender    string     // "M" or "F"
	USSNumber string     // raw USAS registration number; may be blank
	TeamCode  string
}

// ─── Parsed event ─────────────────────────────────────────────────────────────

// ParsedEvent identifies an event as extracted from the file.
type ParsedEvent struct {
	EventNumber string // original event number string (e.g. "1", "12A")
	Distance    int    // in meters or yards depending on course
	Stroke      Stroke
	EventGender string // "M", "F", or "X" (mixed relay)
	AgeGroup    string // e.g. "1824", "OPEN", ""
	Course      Course
}

// ─── Time value ───────────────────────────────────────────────────────────────

// Time represents a swim time value that may be numeric or a special code
// (NT, NS, DNF, DQ, SCR).
type Time struct {
	Raw     string  // raw ASCII string trimmed from file, e.g. "1:23.45" or "NT"
	Valid   bool    // true when a numeric time was successfully parsed
	Seconds float64 // total seconds; meaningful only when Valid == true
}

// ─── Parsed results ───────────────────────────────────────────────────────────

// ParsedIndividualResult corresponds to one D0 record (SDIF) or equivalent
// per-swimmer result row from another format.
type ParsedIndividualResult struct {
	LineNumber int
	Swimmer    ParsedSwimmer
	Event      ParsedEvent
	SeedTime   Time
	PrelimTime Time
	FinalsTime Time
	// PrelimCourse is the course code of the prelim swim, when given.
	PrelimCourse Course
	Heat         int
	Lane         int
	Place        int
}

// ParsedRelayResult corresponds to one E0 record plus its F0 legs (SDIF).
type ParsedRelayResult struct {
	LineNumber int
	TeamCode   string
	Event      ParsedEvent
	SeedTime   Time
	FinalsTime Time
	Legs       []ParsedRelayLeg
}

// ParsedRelayLeg is one F0 record — a single swimmer's contribution to a relay.
type ParsedRelayLeg struct {
	LineNumber int
	Swimmer    ParsedSwimmer
	LegOrder   int // 1-4; 0 when absent
	LegTime    Time
}

// ParsedMeetSession captures schedule/session metadata from meet entry files.
type ParsedMeetSession struct {
	SessionNumber int
	Section       string
	Name          string
	SessionDate   *time.Time
	WarmupTime    string
	StartTime     string
}

// ParsedMeetEvent captures event definitions from meet entry files.
type ParsedMeetEvent struct {
	SessionNumber int
	EventNumber   string
	Distance      int
	Stroke        Stroke
	Gender        string
	MinAge        *int
	MaxAge        *int
	Course        Course
	IsRelay       bool
	// EntryFeeCents is the per-entry fee the host charges for this event.
	EntryFeeCents int
	// TimeStandards holds the event's entry time standard in milliseconds,
	// keyed by the course it is expressed in. Hosts publish the same standard
	// converted for each course they accept times in.
	TimeStandards map[Course]int
}

// ParsedMeetDefinition is the top-level output for meet setup file parsing.
type ParsedMeetDefinition struct {
	FileType      FileType
	SourceFile    string
	MeetName      string
	Location      string
	MeetStartDate *time.Time
	MeetEndDate   *time.Time
	// AgeUpDate is the date swimmer ages are computed on.
	AgeUpDate   *time.Time
	Course      Course
	Sessions    []ParsedMeetSession
	Events      []ParsedMeetEvent
	ParseErrors []ParseError
}

// ─── Top-level result ─────────────────────────────────────────────────────────

// ParseResult is the top-level output of any parser.
type ParseResult struct {
	FileType          FileType
	MeetName          string
	MeetStartDate     *time.Time
	MeetEndDate       *time.Time
	Course            Course
	IndividualResults []ParsedIndividualResult
	RelayResults      []ParsedRelayResult
	ParseErrors       []ParseError
	// TotalParseErrors counts every parse error seen; ParseErrors itself may be
	// capped to bound memory on junk input.
	TotalParseErrors int
}

// ─── Error types ──────────────────────────────────────────────────────────────

// ErrorCode classifies a parse or match error.
type ErrorCode string

const (
	// ErrCodeUnknownAthlete means no swimmer was found for the record.
	ErrCodeUnknownAthlete ErrorCode = "unknown_athlete"
	// ErrCodeBadEventCode means the event code is not defined in the meet.
	ErrCodeBadEventCode ErrorCode = "bad_event_code"
	// ErrCodeDuplicateRow means the same swimmer+event already appeared.
	ErrCodeDuplicateRow ErrorCode = "duplicate_row"
	// ErrCodeMalformedRecord means the line could not be parsed at all.
	ErrCodeMalformedRecord ErrorCode = "malformed_record"
	// ErrCodeAmbiguousMatch means multiple swimmers match the identity fields.
	ErrCodeAmbiguousMatch ErrorCode = "ambiguous_match"
	// ErrCodeArchiveTooLarge means a ZIP archive exceeded the per-entry or
	// cumulative decompressed-size cap (zip-bomb protection).
	ErrCodeArchiveTooLarge ErrorCode = "archive_too_large"
)

// ParseError captures a line-level parse or match error.
type ParseError struct {
	LineNumber int
	Raw        string // raw line content (may be truncated for large files)
	Code       ErrorCode
	Message    string
}
