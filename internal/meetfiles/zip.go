package meetfiles

import (
	"archive/zip"
	"bytes"
	"fmt"
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
	entries, err := readZipEntriesWithExt(data, ".sd3", ".cl2")
	if err != nil {
		return nil, fmt.Errorf("meetfiles/zip: %w", err)
	}
	return entries, nil
}

// readZipEntriesWithExt reads, under the shared size caps, every non-directory
// entry whose name ends in one of exts (lowercase, case-insensitive match).
func readZipEntriesWithExt(data []byte, exts ...string) ([]ZipEntry, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open reader: %w", err)
	}

	var entries []ZipEntry
	var budget int64
	for _, f := range r.File {
		if !hasAnySuffixFold(f.Name, exts) {
			continue
		}
		cleanName, safe := normalizeZipEntryName(f.Name)
		if !safe {
			return nil, fmt.Errorf("unsafe entry name %q", f.Name)
		}
		content, nextBudget, err := readZipEntryBounded(f, budget)
		if err != nil {
			return nil, fmt.Errorf("read entry %q: %w", f.Name, err)
		}
		budget = nextBudget
		entries = append(entries, ZipEntry{Name: cleanName, Content: content})
	}
	return entries, nil
}

func hasAnySuffixFold(name string, exts []string) bool {
	lower := strings.ToLower(name)
	for _, ext := range exts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
