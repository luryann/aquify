package meetfiles_test

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/luryann/aquify/meetfiles"
)

func TestCov1xReadEventsZipRejectsCorruptArchive(t *testing.T) {
	entries, err := meetfiles.ReadEventsZip([]byte("definitely not a zip"))
	if err == nil || !strings.Contains(err.Error(), "meetfiles/zip") || !strings.Contains(err.Error(), "open reader") {
		t.Fatalf("err = %v", err)
	}
	if entries != nil {
		t.Fatalf("entries = %v", entries)
	}
}

func TestCov1xReadResultsZipRejectsUnsafeEntryNames(t *testing.T) {
	for _, name := range []string{"bad\x00name.sd3", "../escape.sd3", "/abs.sd3", "C:evil.sd3", "..\\up.sd3"} {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		fw, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
		_, _ = fw.Write([]byte("A0"))
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		_, err = meetfiles.ReadResultsZip(buf.Bytes())
		if err == nil || !strings.Contains(err.Error(), "unsafe entry name") {
			t.Fatalf("name %q: err = %v", name, err)
		}
	}
}

func TestCov1xReadResultsZipUnsupportedCompressionMethod(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, err := w.CreateRaw(&zip.FileHeader{
		Name:               "x.sd3",
		Method:             99, // no registered decompressor
		CompressedSize64:   4,
		UncompressedSize64: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("abcd"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = meetfiles.ReadResultsZip(buf.Bytes())
	if err == nil || !strings.Contains(err.Error(), `read entry "x.sd3"`) || !strings.Contains(err.Error(), "algorithm") {
		t.Fatalf("err = %v", err)
	}
}
