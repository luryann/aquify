//  PROPRIETARY AND CONFIDENTIAL
//  COPYRIGHT (C) 2026 RYAN LU. ALL RIGHTS RESERVED.
//
//  THIS SOURCE FILE IS PART OF A PROPRIETARY, CLOSED-SOURCE SOFTWARE
//  SYSTEM OWNED EXCLUSIVELY BY RYAN LU. THIS FILE AND ALL ASSOCIATED
//  SOURCE CODE, OBJECT CODE, MODULES, COMPONENTS, DOCUMENTATION, AND
//  RELATED MATERIALS ARE CONFIDENTIAL AND PROPRIETARY INTELLECTUAL
//  PROPERTY OF RYAN LU.
//
//  USE OF THIS SOFTWARE IS PERMITTED ONLY FOR PERSONS OR ENTITIES
//  EXPRESSLY AUTHORIZED BY RYAN LU AND ONLY AFTER ACCEPTANCE OF THE
//  APPLICABLE LICENSE TERMS. DARE AQUATICS AND OTHER TEAMS OR
//  ORGANIZATIONS MAY USE THIS SOFTWARE ONLY IF SEPARATELY APPROVED BY
//  RYAN LU.
//
//  NO COPYING, MODIFICATION, DISTRIBUTION, HOSTING, DISCLOSURE,
//  REVERSE ENGINEERING, DECOMPILATION, OR OTHER USE IS PERMITTED
//  EXCEPT AS EXPRESSLY AUTHORIZED IN WRITING BY RYAN LU.
//
//  ALL RIGHTS NOT EXPRESSLY GRANTED ARE RESERVED BY RYAN LU.

package meetfiles

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ReadHY3 opens a .hy3 archive (which is a ZIP container used by HyTek Meet
// Manager) and selects the best parseable payload file for subsequent parsing.
//
// Payload selection uses deterministic precedence:
//  1. SD3 files are preferred over CL2 files.
//  2. When multiple files of the same preferred type exist, the
//     lexicographically earliest filename is chosen.
//
// All parseable entries (sorted by precedence) and any unrecognized filenames
// are captured in HY3IngestResult.AllPayloads and HY3IngestResult.UnknownFiles
// respectively, to support audit and metadata requirements.
//
// Error conditions (set on result.Error; no Go error is returned):
//   - malformed_record: the data is not a valid ZIP archive.
//   - hy3_empty_archive: the ZIP contains no files at all.
//   - hy3_no_parseable_payload: no .sd3 or .cl2 files found.
//   - hy3_ambiguous_payload: multiple parseable files exist; one was selected
//     by precedence but the condition is flagged so callers can surface it.
//
// When hy3_ambiguous_payload is set, SelectedPayload is still populated with
// the winning file — callers must check SelectedPayload != nil independently.
func ReadHY3(filename string, data []byte) HY3IngestResult {
	result := HY3IngestResult{SourceFile: filename}

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		result.Error = &ParseError{
			Code:    ErrCodeMalformedRecord,
			Message: fmt.Sprintf("hy3: cannot open ZIP container: %v", err),
		}
		return result
	}

	if len(r.File) == 0 {
		result.Error = &ParseError{
			Code:    ErrCodeHY3EmptyArchive,
			Message: "hy3: archive contains no files",
		}
		return result
	}

	var budget int64
	for _, f := range r.File {
		lower := strings.ToLower(f.Name)
		isParseable := strings.HasSuffix(lower, ".sd3") || strings.HasSuffix(lower, ".cl2")
		if !isParseable {
			result.UnknownFiles = append(result.UnknownFiles, f.Name)
			continue
		}
		cleanName, safe := normalizeZipEntryName(f.Name)
		if !safe {
			// Absolute / path-traversal names are never surfaced as payloads.
			result.UnknownFiles = append(result.UnknownFiles, f.Name)
			continue
		}

		content, newBudget, readErr := readZipEntryBounded(f, budget)
		if readErr != nil {
			code := ErrCodeMalformedRecord // corrupt stream or I/O failure
			if errors.Is(readErr, errZipTooLarge) {
				code = ErrCodeArchiveTooLarge
			}
			result.Error = &ParseError{
				Code:    code,
				Message: readErr.Error(),
			}
			return result
		}
		budget = newBudget
		result.AllPayloads = append(result.AllPayloads, ZipEntry{Name: cleanName, Content: content})
	}

	if len(result.AllPayloads) == 0 {
		result.Error = &ParseError{
			Code:    ErrCodeHY3NoPayload,
			Message: "hy3: archive contains no parseable .sd3 or .cl2 payload",
		}
		return result
	}

	// Sort payloads: SD3 before CL2, then lexicographic within each group.
	sort.Slice(result.AllPayloads, func(i, j int) bool {
		iIsSD3 := strings.HasSuffix(strings.ToLower(result.AllPayloads[i].Name), ".sd3")
		jIsSD3 := strings.HasSuffix(strings.ToLower(result.AllPayloads[j].Name), ".sd3")
		if iIsSD3 != jIsSD3 {
			return iIsSD3 // SD3 before CL2
		}
		return result.AllPayloads[i].Name < result.AllPayloads[j].Name
	})

	selected := result.AllPayloads[0]
	result.SelectedPayload = &selected

	if len(result.AllPayloads) > 1 {
		result.Error = &ParseError{
			Code: ErrCodeHY3AmbiguousPayload,
			Message: fmt.Sprintf(
				"hy3: archive contains %d parseable files; selected %q by precedence (SD3 > CL2, then lexicographic)",
				len(result.AllPayloads), selected.Name,
			),
		}
	}

	return result
}
