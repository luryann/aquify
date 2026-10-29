package meetfiles_test

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/meetfiles"
)

// buildZIP is a test helper that creates an in-memory ZIP archive containing
// the provided (name, content) pairs.
func buildZIP(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", name, err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("zip write(%q): %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

func TestReadZip_ExtractsSD3AndCL2(t *testing.T) {
	sd3Content := []byte("A0 sample sd3 content")
	cl2Content := []byte("CL2 sample cl2 content")
	pdfContent := []byte("%PDF-1.4 ignored")

	archive := buildZIP(t, map[string][]byte{
		"results.sd3": sd3Content,
		"entries.CL2": cl2Content, // uppercase extension
		"readme.pdf":  pdfContent,
	})

	entries, err := meetfiles.ReadZip(archive)
	if err != nil {
		t.Fatalf("ReadZip error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	// Collect names.
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["results.sd3"] {
		t.Error("expected results.sd3 in entries")
	}
	if !names["entries.CL2"] {
		t.Error("expected entries.CL2 in entries")
	}
	if names["readme.pdf"] {
		t.Error("readme.pdf should have been filtered out")
	}
}

func TestReadZip_ContentPreserved(t *testing.T) {
	want := []byte("A0 this is the content 12345")
	archive := buildZIP(t, map[string][]byte{"file.sd3": want})

	entries, err := meetfiles.ReadZip(archive)
	if err != nil {
		t.Fatalf("ReadZip error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !bytes.Equal(entries[0].Content, want) {
		t.Errorf("content mismatch: got %q, want %q", entries[0].Content, want)
	}
}

func TestReadZip_EmptyArchive(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{})
	entries, err := meetfiles.ReadZip(archive)
	if err != nil {
		t.Fatalf("ReadZip error on empty archive: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries from empty archive, got %d", len(entries))
	}
}

func TestReadZip_NoRelevantFiles(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"readme.txt": []byte("text"),
		"image.png":  []byte("binary"),
	})
	entries, err := meetfiles.ReadZip(archive)
	if err != nil {
		t.Fatalf("ReadZip error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (no sd3/cl2), got %d", len(entries))
	}
}

func TestReadZip_InvalidData(t *testing.T) {
	_, err := meetfiles.ReadZip([]byte("not a zip file"))
	if err == nil {
		t.Error("expected error for invalid ZIP data")
	}
}

func TestReadZip_RejectsOversizedEntry(t *testing.T) {
	big := bytes.Repeat([]byte("A"), 101<<20) // compresses to a few KB, expands past the cap
	data := buildZIP(t, map[string][]byte{"huge.sd3": big})
	if len(data) > 1<<20 {
		t.Fatalf("test archive unexpectedly large: %d bytes", len(data))
	}
	if _, err := meetfiles.ReadZip(data); err == nil {
		t.Fatal("expected an error for an entry larger than the size limit")
	}
}

// The 250 MiB archive cap must hold while reading, not after: the third 90 MiB
// entry only gets the 70 MiB that is left of the budget, so it fails without
// being read in full.
func TestReadZip_EnforcesCumulativeCapWhileReading(t *testing.T) {
	chunk := bytes.Repeat([]byte("A"), 90<<20)
	data := buildZIP(t, map[string][]byte{"a.sd3": chunk, "b.sd3": chunk, "c.sd3": chunk})
	if _, err := meetfiles.ReadZip(data); err == nil || !strings.Contains(err.Error(), "cumulative") {
		t.Fatalf("expected a cumulative-cap error, got %v", err)
	}
}
