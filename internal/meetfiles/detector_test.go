package meetfiles_test

import (
	"testing"

	"github.com/dareaquatics/aquify/meetfiles"
)

func TestDetectFileType_ByExtension(t *testing.T) {
	cases := []struct {
		filename string
		want     meetfiles.FileType
	}{
		{"results.sd3", meetfiles.FileTypeSD3},
		{"RESULTS.SD3", meetfiles.FileTypeSD3},
		{"entries.cl2", meetfiles.FileTypeCL2},
		{"ENTRIES.CL2", meetfiles.FileTypeCL2},
		{"meet.zip", meetfiles.FileTypeZIP},
		{"MEET.ZIP", meetfiles.FileTypeZIP},
		{"events.hyv", meetfiles.FileTypeHYV},
		{"EVENTS.HYV", meetfiles.FileTypeHYV},
		{"events.ev3", meetfiles.FileTypeEV3},
		{"EVENTS.EV3", meetfiles.FileTypeEV3},
		{"roster.csv", ""},
		{"noextension", ""},
	}

	for _, tc := range cases {
		got := meetfiles.DetectFileType(tc.filename, nil)
		if got != tc.want {
			t.Errorf("DetectFileType(%q, nil) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

func TestDetectFileType_ZIPMagic(t *testing.T) {
	// ZIP magic bytes even with unknown extension.
	header := []byte{'P', 'K', 0x03, 0x04, 0x00}
	got := meetfiles.DetectFileType("mystery.bin", header)
	if got != meetfiles.FileTypeZIP {
		t.Errorf("DetectFileType by ZIP magic = %q, want zip", got)
	}
}

func TestDetectFileType_SDIFMagic(t *testing.T) {
	// A0 leader at byte 0 should detect as SD3.
	header := []byte{'A', '0', ' ', '1'}
	got := meetfiles.DetectFileType("mystery.bin", header)
	if got != meetfiles.FileTypeSD3 {
		t.Errorf("DetectFileType by SDIF magic = %q, want sd3", got)
	}
}

func TestDetectFileType_ExtensionTakesPriority(t *testing.T) {
	// If extension says .sd3 but magic bytes look like ZIP, extension wins.
	zipHeader := []byte{'P', 'K', 0x03, 0x04}
	got := meetfiles.DetectFileType("file.sd3", zipHeader)
	if got != meetfiles.FileTypeSD3 {
		t.Errorf("extension should take priority: got %q, want sd3", got)
	}
}
