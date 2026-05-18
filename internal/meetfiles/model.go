// Package meetfiles implements SD3/SDIF, CL2, and ZIP file parsing for swim
// meet interoperability.  It produces a normalized ParseResult that the import
// service uses to match swimmers, queue conflicts, and build reports.
package meetfiles

import "time"

// ─── File type ────────────────────────────────────────────────────────────────

// FileType indicates the format of a meet file.
type FileType string

const (
	FileTypeSD3 FileType = "sd3"
	FileTypeCL2 FileType = "cl2"
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
	FinalsTime Time
	Heat       int
	Lane       int
	Place      int
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
}

// ParsedMeetDefinition is the top-level output for meet setup file parsing.
type ParsedMeetDefinition struct {
	FileType      FileType
	SourceFile    string
	MeetName      string
	Location      string
	MeetStartDate *time.Time
	MeetEndDate   *time.Time
	Course        Course
	Sessions      []ParsedMeetSession
	Events        []ParsedMeetEvent
	ParseErrors   []ParseError
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
	// ErrCodeHY3EmptyArchive means the .hy3 ZIP container has no files at all.
	ErrCodeHY3EmptyArchive ErrorCode = "hy3_empty_archive"
	// ErrCodeHY3NoPayload means the .hy3 archive has no parseable .sd3 or .cl2 files.
	ErrCodeHY3NoPayload ErrorCode = "hy3_no_parseable_payload"
	// ErrCodeHY3AmbiguousPayload means the .hy3 archive contains multiple parseable
	// files; one was selected by deterministic precedence but the conflict is flagged.
	ErrCodeHY3AmbiguousPayload ErrorCode = "hy3_ambiguous_payload"
)

// HY3 ingest result

// HY3IngestResult holds the outcome of reading a .hy3 container archive.
// The SelectedPayload is the single file chosen for parsing; callers should
// pass its Content to the appropriate SD3 or CL2 parser.
type HY3IngestResult struct {
	// SourceFile is the original .hy3 filename.
	SourceFile string
	// SelectedPayload is the inner file chosen for parsing.  Nil when no
	// parseable payload could be selected (see Error).
	SelectedPayload *ZipEntry
	// AllPayloads contains every .sd3 and .cl2 entry found in the archive,
	// sorted by deterministic precedence (SD3 before CL2, then lexicographic).
	AllPayloads []ZipEntry
	// UnknownFiles lists filenames inside the archive that are neither .sd3
	// nor .cl2 and were therefore skipped.
	UnknownFiles []string
	// Error is non-nil when a packaging failure prevents payload selection
	// (hy3_empty_archive, hy3_no_parseable_payload, hy3_ambiguous_payload, or
	// malformed_record when the archive itself is corrupt).
	// Note: ErrCodeHY3AmbiguousPayload is set even when a payload was selected,
	// so callers must check SelectedPayload independently of Error.
	Error *ParseError
}

// ParseError captures a line-level parse or match error.
type ParseError struct {
	LineNumber int
	Raw        string // raw line content (may be truncated for large files)
	Code       ErrorCode
	Message    string
}
