package meetfiles

import "fmt"

// ReadMeetZip extracts all recognized meet setup payloads from a ZIP archive.
// Meet setup archives may contain EV3/HYV event exports or SD3/CL2 files.
func ReadMeetZip(data []byte) ([]ZipEntry, error) {
	entries, err := readZipEntriesWithExt(data, ".ev3", ".hyv", ".sd3", ".cl2")
	if err != nil {
		return nil, fmt.Errorf("meetfiles/meet_zip: %w", err)
	}
	return entries, nil
}
