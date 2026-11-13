package meets

import (
	"fmt"
	"sort"
	"strings"

	"github.com/luryann/aquify/meetfiles"
	"github.com/luryann/aquify/meetfiles/sdif"
)

// ParseResultsFile parses a meet results file: an SDIF .sd3, or a ZIP holding
// one. Meet event files (.hyv/.ev3) and Hy-Tek entry files (.hy3) carry no
// results and are rejected.
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
	case meetfiles.FileTypeZIP:
		results, err := meetfiles.ReadResultsZip(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse ZIP: %v", ErrInvalidMeetFile, err)
		}
		if len(results) == 0 {
			return nil, nil, fmt.Errorf("%w: zip archive does not contain an .sd3 results file", ErrInvalidMeetFile)
		}
		sort.Slice(results, func(i, j int) bool {
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
		return nil, nil, fmt.Errorf("%w: unsupported results file type %q; upload an .sd3 results file or a .zip containing one", ErrInvalidMeetFile, fileType)
	}
}
