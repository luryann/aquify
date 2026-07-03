package meetfiles

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// ReadMeetZip extracts all recognized meet setup payloads from a ZIP archive.
// Meet setup archives may contain EV3/HYV event exports or SD3/CL2 files.
func ReadMeetZip(data []byte) ([]ZipEntry, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("meetfiles/meet_zip: open reader: %w", err)
	}

	var entries []ZipEntry
	var budget int64
	for _, f := range r.File {
		lower := strings.ToLower(f.Name)
		if !strings.HasSuffix(lower, ".ev3") &&
			!strings.HasSuffix(lower, ".hyv") &&
			!strings.HasSuffix(lower, ".sd3") &&
			!strings.HasSuffix(lower, ".cl2") {
			continue
		}

		content, newBudget, err := readZipEntryBounded(f, budget)
		if err != nil {
			return nil, fmt.Errorf("meetfiles/meet_zip: %w", err)
		}
		budget = newBudget
		entries = append(entries, ZipEntry{Name: f.Name, Content: content})
	}
	return entries, nil
}
