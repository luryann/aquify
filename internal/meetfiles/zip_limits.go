package meetfiles

import (
	"archive/zip"
	"fmt"
	"io"
)

// Meet entry files (SD3/CL2/EV3/HYV) are line-oriented text; these caps are
// generous for legitimate files while bounding decompression-bomb exposure
// from a hostile ZIP/HY3 upload.
const (
	maxZipEntryUncompressedBytes = 100 << 20 // 100 MiB per entry
	maxZipTotalUncompressedBytes = 250 << 20 // 250 MiB per archive
)

// readZipEntryBounded reads a zip.File's content while enforcing a per-entry
// cap (checked against the declared uncompressed size and against actual
// bytes read) and a cumulative-total cap tracked via budget. It returns the
// updated budget alongside the content.
func readZipEntryBounded(f *zip.File, budget int64) ([]byte, int64, error) {
	if f.UncompressedSize64 > uint64(maxZipEntryUncompressedBytes) {
		return nil, budget, fmt.Errorf("meetfiles: entry %q declares %d bytes uncompressed, exceeds per-entry cap of %d", f.Name, f.UncompressedSize64, maxZipEntryUncompressedBytes)
	}

	rc, err := f.Open()
	if err != nil {
		return nil, budget, err
	}
	defer rc.Close()

	// Read one byte past the per-entry cap so truncated/lying archives (which
	// can under-declare UncompressedSize64) are still caught.
	limited := io.LimitReader(rc, maxZipEntryUncompressedBytes+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, budget, err
	}
	if int64(len(content)) > maxZipEntryUncompressedBytes {
		return nil, budget, fmt.Errorf("meetfiles: entry %q exceeds per-entry decompressed cap of %d bytes", f.Name, maxZipEntryUncompressedBytes)
	}

	budget += int64(len(content))
	if budget > maxZipTotalUncompressedBytes {
		return nil, budget, fmt.Errorf("meetfiles: archive exceeds cumulative decompressed cap of %d bytes", maxZipTotalUncompressedBytes)
	}

	return content, budget, nil
}
