package meetfiles

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ParseHYV(filename string, data []byte) (*ParsedMeetDefinition, error) {
	lines := splitHytekLines(data)
	if len(lines) == 0 {
		return nil, fmt.Errorf("hyv: file is empty")
	}

	header := splitHytekFields(lines[0])
	if len(header) < 6 {
		return nil, fmt.Errorf("hyv: header has %d fields; want at least 6", len(header))
	}

	startDate, err := parseHytekDate(header[1])
	if err != nil {
		return nil, fmt.Errorf("hyv: parse start date: %w", err)
	}
	endDate, err := parseHytekDate(header[2])
	if err != nil {
		return nil, fmt.Errorf("hyv: parse end date: %w", err)
	}

	result := &ParsedMeetDefinition{
		FileType:      FileTypeHYV,
		SourceFile:    filename,
		MeetName:      cleanHytekField(header[0]),
		Location:      cleanHytekField(header[5]),
		MeetStartDate: startDate,
		MeetEndDate:   endDate,
		Course:        parseHytekCourse(cleanHytekField(header[4])),
	}

	for lineNumber, raw := range lines[1:] {
		fields := splitHytekFields(raw)
		if len(fields) < 8 {
			result.ParseErrors = append(result.ParseErrors, ParseError{
				LineNumber: lineNumber + 2,
				Raw:        raw,
				Code:       ErrCodeMalformedRecord,
				Message:    fmt.Sprintf("hyv: expected at least 8 fields, got %d", len(fields)),
			})
			continue
		}

		event, ok, parseErr := parseHYVEvent(fields, result.Course)
		if parseErr != nil {
			result.ParseErrors = append(result.ParseErrors, ParseError{
				LineNumber: lineNumber + 2,
				Raw:        raw,
				Code:       ErrCodeMalformedRecord,
				Message:    parseErr.Error(),
			})
			continue
		}
		if ok {
			result.Events = append(result.Events, event)
		}
	}

	result.Sessions = deriveDailyMeetSessions(startDate, endDate)
	return result, nil
}

func ParseEV3(filename string, data []byte) (*ParsedMeetDefinition, error) {
	lines := splitHytekLines(data)
	if len(lines) == 0 {
		return nil, fmt.Errorf("ev3: file is empty")
	}

	header := splitHytekFields(lines[0])
	if len(header) < 6 {
		return nil, fmt.Errorf("ev3: header has %d fields; want at least 6", len(header))
	}

	startDate, err := parseHytekDate(header[2])
	if err != nil {
		return nil, fmt.Errorf("ev3: parse start date: %w", err)
	}
	endDate, err := parseHytekDate(header[3])
	if err != nil {
		return nil, fmt.Errorf("ev3: parse end date: %w", err)
	}

	result := &ParsedMeetDefinition{
		FileType:      FileTypeEV3,
		SourceFile:    filename,
		MeetName:      cleanHytekField(header[0]),
		Location:      cleanHytekField(header[1]),
		MeetStartDate: startDate,
		MeetEndDate:   endDate,
		Course:        parseHytekCourse(cleanHytekField(header[5])),
	}

	sessionByNumber := map[int]sessionMeta{}

	for lineNumber, raw := range lines[1:] {
		fields := splitHytekFields(raw)
		if len(fields) < 25 {
			result.ParseErrors = append(result.ParseErrors, ParseError{
				LineNumber: lineNumber + 2,
				Raw:        raw,
				Code:       ErrCodeMalformedRecord,
				Message:    fmt.Sprintf("ev3: expected at least 25 fields, got %d", len(fields)),
			})
			continue
		}

		event, ok, meta, parseErr := parseEV3Event(fields, result.Course)
		if parseErr != nil {
			result.ParseErrors = append(result.ParseErrors, ParseError{
				LineNumber: lineNumber + 2,
				Raw:        raw,
				Code:       ErrCodeMalformedRecord,
				Message:    parseErr.Error(),
			})
			continue
		}
		if !ok {
			continue
		}
		result.Events = append(result.Events, event)
		if meta.sessionNumber > 0 {
			existing, found := sessionByNumber[meta.sessionNumber]
			if !found || existing.startTime == "" {
				sessionByNumber[meta.sessionNumber] = meta
			}
		}
	}

	result.Sessions = buildEV3Sessions(startDate, sessionByNumber)
	if len(result.Sessions) == 0 {
		result.Sessions = deriveDailyMeetSessions(startDate, endDate)
	}
	return result, nil
}

func parseHYVEvent(fields []string, course Course) (ParsedMeetEvent, bool, error) {
	eventNumber := cleanHytekField(fields[0])
	if eventNumber == "" {
		return ParsedMeetEvent{}, false, nil
	}

	isRelay := strings.EqualFold(cleanHytekField(fields[3]), "R")
	minAge := parseHytekAge(cleanHytekField(fields[4]), false)
	maxAge := parseHytekAge(cleanHytekField(fields[5]), true)

	distance, err := strconv.Atoi(cleanHytekField(fields[6]))
	if err != nil || distance <= 0 {
		return ParsedMeetEvent{}, false, fmt.Errorf("hyv: invalid distance %q", cleanHytekField(fields[6]))
	}

	strokeCode, err := strconv.Atoi(cleanHytekField(fields[7]))
	if err != nil {
		return ParsedMeetEvent{}, false, fmt.Errorf("hyv: invalid stroke code %q", cleanHytekField(fields[7]))
	}

	return ParsedMeetEvent{
		EventNumber: eventNumber,
		Distance:    distance,
		Stroke:      parseHYVStroke(strokeCode, isRelay),
		Gender:      parseHYVGender(cleanHytekField(fields[2])),
		MinAge:      minAge,
		MaxAge:      maxAge,
		Course:      course,
		IsRelay:     isRelay,
	}, true, nil
}

func parseEV3Event(fields []string, course Course) (ParsedMeetEvent, bool, sessionMeta, error) {
	eventNumber := cleanHytekField(fields[1])
	if eventNumber == "" {
		return ParsedMeetEvent{}, false, sessionMeta{}, nil
	}

	isRelay := strings.EqualFold(cleanHytekField(fields[4]), "R")
	minAge := parseHytekAge(cleanHytekField(fields[6]), false)
	maxAge := parseHytekAge(cleanHytekField(fields[7]), true)

	distance, err := strconv.Atoi(cleanHytekField(fields[8]))
	if err != nil || distance <= 0 {
		return ParsedMeetEvent{}, false, sessionMeta{}, fmt.Errorf("ev3: invalid distance %q", cleanHytekField(fields[8]))
	}

	sessionNumber, err := strconv.Atoi(cleanHytekField(fields[21]))
	if err != nil || sessionNumber <= 0 {
		return ParsedMeetEvent{}, false, sessionMeta{}, fmt.Errorf("ev3: invalid session number %q", cleanHytekField(fields[21]))
	}

	dayIndex, err := strconv.Atoi(cleanHytekField(fields[23]))
	if err != nil || dayIndex <= 0 {
		dayIndex = 1
	}

	event := ParsedMeetEvent{
		SessionNumber: sessionNumber,
		EventNumber:   eventNumber,
		Distance:      distance,
		Stroke:        parseEV3Stroke(cleanHytekField(fields[9]), isRelay),
		Gender:        parseEV3Gender(cleanHytekField(fields[5])),
		MinAge:        minAge,
		MaxAge:        maxAge,
		Course:        course,
		IsRelay:       isRelay,
	}
	meta := sessionMeta{
		sessionNumber: sessionNumber,
		dayIndex:      dayIndex,
		startTime:     normalizeHytekClock(cleanHytekField(fields[24])),
	}
	return event, true, meta, nil
}

type sessionMeta struct {
	sessionNumber int
	dayIndex      int
	startTime     string
}

func buildEV3Sessions(startDate *time.Time, sessions map[int]sessionMeta) []ParsedMeetSession {
	if len(sessions) == 0 {
		return nil
	}

	keys := make([]int, 0, len(sessions))
	for sessionNumber := range sessions {
		keys = append(keys, sessionNumber)
	}
	sortInts(keys)

	out := make([]ParsedMeetSession, 0, len(keys))
	for _, sessionNumber := range keys {
		meta := sessions[sessionNumber]
		session := ParsedMeetSession{
			SessionNumber: sessionNumber,
			Section:       fmt.Sprintf("Day %d", meta.dayIndex),
			Name:          fmt.Sprintf("Session %d", sessionNumber),
			StartTime:     meta.startTime,
		}
		if startDate != nil {
			date := startDate.AddDate(0, 0, meta.dayIndex-1)
			session.SessionDate = &date
		}
		out = append(out, session)
	}
	return out
}

func deriveDailyMeetSessions(startDate, endDate *time.Time) []ParsedMeetSession {
	if startDate == nil && endDate == nil {
		return []ParsedMeetSession{{
			SessionNumber: 1,
			Section:       "Day 1",
			Name:          "Session 1",
		}}
	}

	start := startDate
	if start == nil {
		start = endDate
	}
	end := endDate
	if end == nil {
		end = start
	}
	startDay := start.UTC().Truncate(24 * time.Hour)
	endDay := end.UTC().Truncate(24 * time.Hour)
	if endDay.Before(startDay) {
		endDay = startDay
	}

	var sessions []ParsedMeetSession
	for day := startDay; !day.After(endDay); day = day.Add(24 * time.Hour) {
		number := len(sessions) + 1
		date := day
		sessions = append(sessions, ParsedMeetSession{
			SessionNumber: number,
			Section:       fmt.Sprintf("Day %d", number),
			Name:          fmt.Sprintf("Session %d", number),
			SessionDate:   &date,
		})
	}
	return sessions
}

func splitHytekLines(data []byte) []string {
	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, line := range parts {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func splitHytekFields(line string) []string {
	parts := strings.Split(line, ";")
	for i, part := range parts {
		parts[i] = cleanHytekField(part)
	}
	return parts
}

func cleanHytekField(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
	value = strings.TrimSuffix(value, "*>")
	return strings.TrimSpace(value)
}

func parseHytekDate(value string) (*time.Time, error) {
	value = cleanHytekField(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("01/02/2006", value)
	if err != nil {
		return nil, err
	}
	utc := parsed.UTC()
	return &utc, nil
}

func parseHytekCourse(value string) Course {
	if value == "" {
		return CourseUnknown
	}
	switch strings.ToUpper(string(value[0])) {
	case "Y":
		return CourseShortCourseYards
	case "S", "M":
		return CourseShortCourseMeters
	case "L":
		return CourseLongCourseMeters
	default:
		return CourseUnknown
	}
}

func parseHytekAge(value string, treatSpecialMaxAsOpen bool) *int {
	value = cleanHytekField(value)
	if value == "" {
		return nil
	}
	age, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	if age == 0 && treatSpecialMaxAsOpen {
		return nil
	}
	if age >= 99 && treatSpecialMaxAsOpen {
		return nil
	}
	return &age
}

func parseHYVGender(value string) string {
	switch strings.ToUpper(value) {
	case "F":
		return "F"
	case "M":
		return "M"
	case "X":
		return "X"
	default:
		return "O"
	}
}

func parseEV3Gender(value string) string {
	switch strings.ToUpper(value) {
	case "G", "F":
		return "F"
	case "B", "M":
		return "M"
	case "X":
		return "X"
	default:
		return "O"
	}
}

func parseHYVStroke(code int, relay bool) Stroke {
	switch code {
	case 1:
		if relay {
			return StrokeFreestyleRelay
		}
		return StrokeFreestyle
	case 2:
		return StrokeBackstroke
	case 3:
		return StrokeBreaststroke
	case 4:
		return StrokeButterfly
	case 5:
		if relay {
			return StrokeMedleyRelay
		}
		return StrokeIM
	default:
		return StrokeUnknown
	}
}

func parseEV3Stroke(code string, relay bool) Stroke {
	switch strings.ToUpper(code) {
	case "A":
		if relay {
			return StrokeFreestyleRelay
		}
		return StrokeFreestyle
	case "B":
		return StrokeBackstroke
	case "C":
		return StrokeBreaststroke
	case "D":
		return StrokeButterfly
	case "E":
		if relay {
			return StrokeMedleyRelay
		}
		return StrokeIM
	default:
		return StrokeUnknown
	}
}

func normalizeHytekClock(value string) string {
	value = strings.ToUpper(cleanHytekField(value))
	if value == "" {
		return ""
	}
	parsed, err := time.Parse("03:04PM", value)
	if err != nil {
		return ""
	}
	return parsed.Format("15:04")
}

func sortInts(values []int) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
