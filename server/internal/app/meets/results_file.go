package meets

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/meetfiles/cl2"
	"github.com/dareaquatics/aquify/meetfiles/sdif"
)

// ParseResultsFile parses a meet results file (SD3/CL2, or an HY3/ZIP archive
// wrapping one) into individual and relay result records. Event-definition
// formats (HYV/EV3) carry no results and are rejected.
func ParseResultsFile(filename string, data []byte) (*meetfiles.ParseResult, []string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || len(data) == 0 {
		return nil, nil, fmt.Errorf("%w: a non-empty results file is required", ErrInvalidMeetFile)
	}
	switch fileType := meetfiles.DetectFileType(filename, data); fileType {
	case meetfiles.FileTypeSD3:
		result, err := sdif.Parse(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse SD3: %v", ErrInvalidMeetFile, err)
		}
		return result, parseWarnings(result.ParseErrors), nil
	case meetfiles.FileTypeCL2:
		result, err := cl2.Parse(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse CL2: %v", ErrInvalidMeetFile, err)
		}
		return result, parseWarnings(result.ParseErrors), nil
	case meetfiles.FileTypeHY3:
		ingest := meetfiles.ReadHY3(filename, data)
		if ingest.SelectedPayload == nil {
			if ingest.Error != nil {
				return nil, nil, fmt.Errorf("%w: %s", ErrInvalidMeetFile, ingest.Error.Message)
			}
			return nil, nil, fmt.Errorf("%w: hy3 payload missing", ErrInvalidMeetFile)
		}
		result, warnings, err := ParseResultsFile(ingest.SelectedPayload.Name, ingest.SelectedPayload.Content)
		if err != nil {
			return nil, nil, err
		}
		if ingest.Error != nil {
			warnings = append([]string{ingest.Error.Message}, warnings...)
		}
		return result, warnings, nil
	case meetfiles.FileTypeZIP:
		entries, err := meetfiles.ReadMeetZip(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse ZIP: %v", ErrInvalidMeetFile, err)
		}
		var results []meetfiles.ZipEntry
		for _, e := range entries {
			if t := meetfiles.DetectFileType(e.Name, e.Content); t == meetfiles.FileTypeSD3 || t == meetfiles.FileTypeCL2 {
				results = append(results, e)
			}
		}
		if len(results) == 0 {
			return nil, nil, fmt.Errorf("%w: zip archive does not contain an sd3 or cl2 results file", ErrInvalidMeetFile)
		}
		sort.Slice(results, func(i, j int) bool {
			iType := meetfiles.DetectFileType(results[i].Name, results[i].Content)
			jType := meetfiles.DetectFileType(results[j].Name, results[j].Content)
			if iType != jType {
				return meetFileTypePriority(iType) < meetFileTypePriority(jType)
			}
			return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
		})
		result, warnings, err := ParseResultsFile(results[0].Name, results[0].Content)
		if err != nil {
			return nil, nil, err
		}
		if len(results) > 1 {
			warnings = append([]string{fmt.Sprintf("zip archive contained %d results files; imported %s", len(results), results[0].Name)}, warnings...)
		}
		return result, warnings, nil
	default:
		return nil, nil, fmt.Errorf("%w: unsupported results file type (expected .sd3, .cl2, .hy3, or .zip)", ErrInvalidMeetFile)
	}
}
