package entries

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// File is one file of an entry bundle.
type File struct {
	Name    string
	Content []byte
}

// Bundle is a complete set of entry files for one meet.
type Bundle struct {
	// BaseName is the shared file-name stem, e.g.
	// "DARE-Entries-Fall Classic-09Nov2026-1538".
	BaseName string
	// Files are in delivery order: SD3, HY3, by-event report, by-swimmer
	// report, entry log.
	Files []File
	// Athletes and IndividualEntries are the totals reported in the log.
	Athletes          int
	IndividualEntries int
}

// Build validates the input and renders every file of the bundle.
func Build(in Input) (*Bundle, error) {
	p, err := prepare(in)
	if err != nil {
		return nil, err
	}
	base := BaseName(p.in)
	names := []string{
		base + ".sd3",
		base + ".hy3",
		base + "-EntryReport-ByEvent.pdf",
		base + "-EntryReport-BySwimmer.pdf",
		base + "-EntryLog.txt",
	}
	return &Bundle{
		BaseName: base,
		Files: []File{
			{Name: names[0], Content: p.sd3()},
			{Name: names[1], Content: p.hy3()},
			{Name: names[2], Content: p.reportByEvent()},
			{Name: names[3], Content: p.reportBySwimmer()},
			{Name: names[4], Content: p.entryLog(names)},
		},
		Athletes:          len(p.byName),
		IndividualEntries: p.individualEntryCount(),
	}, nil
}

// BaseName is "<club code>-Entries-<meet name>-<DDMonYYYY>-<HHMM>" using the
// export time; characters that are unsafe in file names are dropped.
func BaseName(in Input) string {
	return fmt.Sprintf("%s-Entries-%s-%s",
		fileNamePart(strings.ToUpper(in.Club.Code)),
		fileNamePart(in.Meet.Name),
		in.GeneratedAt.Format("02Jan2006-1504"))
}

func fileNamePart(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r < 0x20 || r == 0x7f:
		case strings.ContainsRune(`/\:*?"<>|`, r):
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), ". ")
	if out == "" {
		return "Meet"
	}
	return out
}

// Zip packs the bundle into a ZIP archive with the files at the root.
func (b *Bundle) Zip() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range b.Files {
		w, err := zw.Create(f.Name)
		if err != nil {
			return nil, fmt.Errorf("entries: zip %s: %w", f.Name, err)
		}
		if _, err := w.Write(f.Content); err != nil {
			return nil, fmt.Errorf("entries: zip %s: %w", f.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("entries: zip: %w", err)
	}
	return buf.Bytes(), nil
}

// File returns the bundle file whose name ends in suffix, or nil.
func (b *Bundle) File(suffix string) *File {
	for i := range b.Files {
		if strings.HasSuffix(b.Files[i].Name, suffix) {
			return &b.Files[i]
		}
	}
	return nil
}
