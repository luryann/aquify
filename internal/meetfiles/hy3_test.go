package meetfiles_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/dareaquatics/aquify/meetfiles"
)

// ─── ReadHY3: valid SD3 payload ───────────────────────────────────────────────

func TestReadHY3_ValidSD3Payload(t *testing.T) {
	sd3Content := []byte("A0 sd3 data")
	archive := buildZIP(t, map[string][]byte{
		"results.sd3": sd3Content,
	})

	got := meetfiles.ReadHY3("meet.hy3", archive)

	if got.Error != nil && got.Error.Code != meetfiles.ErrCodeHY3AmbiguousPayload {
		t.Fatalf("unexpected error: %v", got.Error)
	}
	if got.SelectedPayload == nil {
		t.Fatal("SelectedPayload is nil; want results.sd3")
	}
	if got.SelectedPayload.Name != "results.sd3" {
		t.Errorf("SelectedPayload.Name = %q, want %q", got.SelectedPayload.Name, "results.sd3")
	}
	if string(got.SelectedPayload.Content) != string(sd3Content) {
		t.Errorf("SelectedPayload.Content mismatch")
	}
	if got.SourceFile != "meet.hy3" {
		t.Errorf("SourceFile = %q, want %q", got.SourceFile, "meet.hy3")
	}
	if len(got.AllPayloads) != 1 {
		t.Errorf("AllPayloads len = %d, want 1", len(got.AllPayloads))
	}
	if got.Error != nil {
		t.Errorf("expected no error for single SD3 payload, got %v", got.Error)
	}
}

// ─── ReadHY3: valid CL2 payload ───────────────────────────────────────────────

func TestReadHY3_ValidCL2Payload(t *testing.T) {
	cl2Content := []byte("CL2 entries data")
	archive := buildZIP(t, map[string][]byte{
		"entries.cl2": cl2Content,
	})

	got := meetfiles.ReadHY3("meet.hy3", archive)

	if got.Error != nil {
		t.Fatalf("unexpected error: %v", got.Error)
	}
	if got.SelectedPayload == nil {
		t.Fatal("SelectedPayload is nil; want entries.cl2")
	}
	if got.SelectedPayload.Name != "entries.cl2" {
		t.Errorf("SelectedPayload.Name = %q, want entries.cl2", got.SelectedPayload.Name)
	}
}

// ─── ReadHY3: malformed / corrupt archive ────────────────────────────────────

func TestReadHY3_CorruptArchive(t *testing.T) {
	got := meetfiles.ReadHY3("meet.hy3", []byte("not a zip file at all"))

	if got.Error == nil {
		t.Fatal("expected error for corrupt archive, got nil")
	}
	if got.Error.Code != meetfiles.ErrCodeMalformedRecord {
		t.Errorf("Error.Code = %q, want %q", got.Error.Code, meetfiles.ErrCodeMalformedRecord)
	}
	if got.SelectedPayload != nil {
		t.Error("SelectedPayload should be nil for corrupt archive")
	}
}

func TestReadHY3_EmptyBytes(t *testing.T) {
	got := meetfiles.ReadHY3("meet.hy3", []byte{})

	if got.Error == nil {
		t.Fatal("expected error for empty data, got nil")
	}
	if got.Error.Code != meetfiles.ErrCodeMalformedRecord {
		t.Errorf("Error.Code = %q, want %q", got.Error.Code, meetfiles.ErrCodeMalformedRecord)
	}
}

// ─── ReadHY3: empty archive ───────────────────────────────────────────────────

func TestReadHY3_EmptyArchive(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{})

	got := meetfiles.ReadHY3("empty.hy3", archive)

	if got.Error == nil {
		t.Fatal("expected error for empty archive, got nil")
	}
	if got.Error.Code != meetfiles.ErrCodeHY3EmptyArchive {
		t.Errorf("Error.Code = %q, want %q", got.Error.Code, meetfiles.ErrCodeHY3EmptyArchive)
	}
	if got.SelectedPayload != nil {
		t.Error("SelectedPayload should be nil for empty archive")
	}
}

// ─── ReadHY3: no parseable payload ───────────────────────────────────────────

func TestReadHY3_NoParsablePayload(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"readme.txt": []byte("some instructions"),
		"report.pdf": []byte("%PDF-1.4"),
	})

	got := meetfiles.ReadHY3("meet.hy3", archive)

	if got.Error == nil {
		t.Fatal("expected error for no parseable payload, got nil")
	}
	if got.Error.Code != meetfiles.ErrCodeHY3NoPayload {
		t.Errorf("Error.Code = %q, want %q", got.Error.Code, meetfiles.ErrCodeHY3NoPayload)
	}
	if got.SelectedPayload != nil {
		t.Error("SelectedPayload should be nil when no parseable files exist")
	}
	if len(got.UnknownFiles) != 2 {
		t.Errorf("UnknownFiles len = %d, want 2", len(got.UnknownFiles))
	}
}

// ─── ReadHY3: ambiguous multi-payload (SD3 + CL2) ───────────────────────────

func TestReadHY3_AmbiguousPayload_SD3TakesPrecedenceOverCL2(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"entries.cl2": []byte("CL2 data"),
		"results.sd3": []byte("A0 sd3 data"),
	})

	got := meetfiles.ReadHY3("meet.hy3", archive)

	// Ambiguous error should be set even though a payload was selected.
	if got.Error == nil {
		t.Fatal("expected ambiguous payload error, got nil")
	}
	if got.Error.Code != meetfiles.ErrCodeHY3AmbiguousPayload {
		t.Errorf("Error.Code = %q, want %q", got.Error.Code, meetfiles.ErrCodeHY3AmbiguousPayload)
	}

	// SD3 must be selected over CL2.
	if got.SelectedPayload == nil {
		t.Fatal("SelectedPayload is nil; SD3 should have been selected")
	}
	if got.SelectedPayload.Name != "results.sd3" {
		t.Errorf("SelectedPayload.Name = %q; want results.sd3 (SD3 > CL2 precedence)", got.SelectedPayload.Name)
	}
	if len(got.AllPayloads) != 2 {
		t.Errorf("AllPayloads len = %d, want 2", len(got.AllPayloads))
	}
}

func TestReadHY3_AmbiguousPayload_MultipleSD3LexicographicOrder(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"beta.sd3":  []byte("A0 beta"),
		"alpha.sd3": []byte("A0 alpha"),
	})

	got := meetfiles.ReadHY3("meet.hy3", archive)

	if got.Error == nil || got.Error.Code != meetfiles.ErrCodeHY3AmbiguousPayload {
		t.Fatalf("expected ErrCodeHY3AmbiguousPayload, got %v", got.Error)
	}
	if got.SelectedPayload == nil {
		t.Fatal("SelectedPayload is nil")
	}
	// Lexicographically first: "alpha.sd3"
	if got.SelectedPayload.Name != "alpha.sd3" {
		t.Errorf("SelectedPayload.Name = %q; want alpha.sd3 (lexicographic order)", got.SelectedPayload.Name)
	}
}

func TestReadHY3_AmbiguousPayload_MultipleCL2LexicographicOrder(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"zebra.cl2": []byte("CL2 z"),
		"apple.cl2": []byte("CL2 a"),
	})

	got := meetfiles.ReadHY3("meet.hy3", archive)

	if got.Error == nil || got.Error.Code != meetfiles.ErrCodeHY3AmbiguousPayload {
		t.Fatalf("expected ErrCodeHY3AmbiguousPayload, got %v", got.Error)
	}
	if got.SelectedPayload == nil {
		t.Fatal("SelectedPayload is nil")
	}
	if got.SelectedPayload.Name != "apple.cl2" {
		t.Errorf("SelectedPayload.Name = %q; want apple.cl2 (lexicographic order)", got.SelectedPayload.Name)
	}
}

// ─── ReadHY3: unknown files in archive ───────────────────────────────────────

func TestReadHY3_UnknownFilesTracked(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"results.sd3": []byte("A0 data"),
		"logo.png":    []byte("\x89PNG"),
		"notes.txt":   []byte("coach notes"),
	})

	got := meetfiles.ReadHY3("meet.hy3", archive)

	if got.Error != nil {
		t.Fatalf("unexpected error: %v", got.Error)
	}
	if got.SelectedPayload == nil {
		t.Fatal("SelectedPayload is nil")
	}
	if len(got.UnknownFiles) != 2 {
		t.Errorf("UnknownFiles len = %d, want 2; got %v", len(got.UnknownFiles), got.UnknownFiles)
	}
}

// ─── ReadHY3: source file metadata ───────────────────────────────────────────

func TestReadHY3_SourceFileMetadataCaptured(t *testing.T) {
	archive := buildZIP(t, map[string][]byte{
		"data.sd3": []byte("A0 content"),
	})

	got := meetfiles.ReadHY3("2026_champs.hy3", archive)

	if got.SourceFile != "2026_champs.hy3" {
		t.Errorf("SourceFile = %q, want %q", got.SourceFile, "2026_champs.hy3")
	}
	if got.SelectedPayload == nil {
		t.Fatal("SelectedPayload is nil")
	}
	if got.SelectedPayload.Name != "data.sd3" {
		t.Errorf("SelectedPayload.Name = %q, want data.sd3", got.SelectedPayload.Name)
	}
}

// ─── DetectFileType: HY3 extension ───────────────────────────────────────────

func TestDetectFileType_HY3Extension(t *testing.T) {
	cases := []struct {
		filename string
	}{
		{"meet.hy3"},
		{"MEET.HY3"},
		{"2026_champs.hy3"},
	}
	for _, tc := range cases {
		got := meetfiles.DetectFileType(tc.filename, nil)
		if got != meetfiles.FileTypeHY3 {
			t.Errorf("DetectFileType(%q, nil) = %q, want %q", tc.filename, got, meetfiles.FileTypeHY3)
		}
	}
}

func TestDetectFileType_HY3TakesPriorityOverZIPMagic(t *testing.T) {
	// .hy3 IS a ZIP internally, so extension must win over magic-byte fallback.
	zipMagic := []byte{'P', 'K', 0x03, 0x04}
	got := meetfiles.DetectFileType("meet.hy3", zipMagic)
	if got != meetfiles.FileTypeHY3 {
		t.Errorf("DetectFileType(.hy3 with ZIP magic) = %q; want hy3 (extension priority)", got)
	}
}

// A size-cap violation and a corrupt compressed stream are different problems
// and must be reported as such.
func TestReadHY3_ReportsTooLargeAndCorruptEntriesDistinctly(t *testing.T) {
	tooLarge := buildZIP(t, map[string][]byte{"big.sd3": bytes.Repeat([]byte("A"), 101<<20)})
	if got := meetfiles.ReadHY3("meet.hy3", tooLarge); got.Error == nil || got.Error.Code != meetfiles.ErrCodeArchiveTooLarge {
		t.Fatalf("oversized entry: error = %+v, want code %q", got.Error, meetfiles.ErrCodeArchiveTooLarge)
	}

	// Store (no compression) an entry, then damage its CRC so reading it fails
	// with a checksum error rather than a size error.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, err := w.CreateHeader(&zip.FileHeader{Name: "meet.sd3", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("A0 payload that will be damaged")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	damaged := buf.Bytes()
	i := bytes.Index(damaged, []byte("A0 payload"))
	damaged[i+3] ^= 0xFF
	got := meetfiles.ReadHY3("meet.hy3", damaged)
	if got.Error == nil || got.Error.Code != meetfiles.ErrCodeMalformedRecord {
		t.Fatalf("corrupt entry: error = %+v, want code %q", got.Error, meetfiles.ErrCodeMalformedRecord)
	}
}
