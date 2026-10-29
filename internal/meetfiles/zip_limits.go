package meetfiles

import (
	"archive/zip"
	"errors"
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

// errZipTooLarge marks a size-cap violation, as opposed to a corrupt stream or
// an I/O failure, so callers can report the right problem.
var errZipTooLarge = errors.New("meetfiles: zip size limit exceeded")

// readZipEntryBounded reads a zip.File's content while enforcing a per-entry
// cap (checked against the declared uncompressed size and against actual
// bytes read) and a cumulative-total cap tracked via budget. It returns the
// updated budget alongside the content.
func readZipEntryBounded(f *zip.File, budget int64) ([]byte, int64, error) {
	if f.UncompressedSize64 > uint64(maxZipEntryUncompressedBytes) {
		return nil, budget, fmt.Errorf("%w: entry %q declares %d bytes uncompressed, exceeds per-entry cap of %d", errZipTooLarge, f.Name, f.UncompressedSize64, maxZipEntryUncompressedBytes)
	}

	rc, err := f.Open()
	if err != nil {
		return nil, budget, err
	}
	defer rc.Close()

	// Enforce both caps while reading: the entry may use at most the smaller of
	// the per-entry cap and what is left of the archive budget, plus one byte so
	// truncated/lying archives (which can under-declare UncompressedSize64) are
	// still caught. Peak memory is therefore bounded by the archive cap itself.
	remaining := maxZipTotalUncompressedBytes - budget
	limit := int64(maxZipEntryUncompressedBytes)
	capErr := fmt.Errorf("%w: entry %q exceeds per-entry decompressed cap of %d bytes", errZipTooLarge, f.Name, maxZipEntryUncompressedBytes)
	if remaining < limit {
		limit = remaining
		capErr = fmt.Errorf("%w: archive exceeds cumulative decompressed cap of %d bytes", errZipTooLarge, maxZipTotalUncompressedBytes)
	}
	content, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, budget, err
	}
	if int64(len(content)) > limit {
		return nil, budget, capErr
	}

	budget += int64(len(content))

	return content, budget, nil
}
