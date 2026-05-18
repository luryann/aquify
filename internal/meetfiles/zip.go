package meetfiles

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// ZipEntry is a single file extracted from a ZIP archive.
type ZipEntry struct {
	Name    string
	Content []byte
}

// ReadZip extracts all relevant meet files from a ZIP archive.
// Only entries with .sd3 or .cl2 extensions (case-insensitive) are returned;
// all other files (images, PDFs, etc.) are ignored.
func ReadZip(data []byte) ([]ZipEntry, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("meetfiles/zip: open reader: %w", err)
	}

	var entries []ZipEntry
	for _, f := range r.File {
		lower := strings.ToLower(f.Name)
		if !strings.HasSuffix(lower, ".sd3") && !strings.HasSuffix(lower, ".cl2") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("meetfiles/zip: open entry %q: %w", f.Name, err)
		}
		content, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("meetfiles/zip: read entry %q: %w", f.Name, readErr)
		}
		entries = append(entries, ZipEntry{Name: f.Name, Content: content})
	}
	return entries, nil
}
