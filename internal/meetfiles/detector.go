//  PROPRIETARY AND CONFIDENTIAL
//  COPYRIGHT (C) 2026 RYAN LU. ALL RIGHTS RESERVED.
//
//  THIS SOURCE FILE IS PART OF A PROPRIETARY, CLOSED-SOURCE SOFTWARE
//  SYSTEM OWNED EXCLUSIVELY BY RYAN LU. THIS FILE AND ALL ASSOCIATED
//  SOURCE CODE, OBJECT CODE, MODULES, COMPONENTS, DOCUMENTATION, AND
//  RELATED MATERIALS ARE CONFIDENTIAL AND PROPRIETARY INTELLECTUAL
//  PROPERTY OF RYAN LU.
//
//  USE OF THIS SOFTWARE IS PERMITTED ONLY FOR PERSONS OR ENTITIES
//  EXPRESSLY AUTHORIZED BY RYAN LU AND ONLY AFTER ACCEPTANCE OF THE
//  APPLICABLE LICENSE TERMS. DARE AQUATICS AND OTHER TEAMS OR
//  ORGANIZATIONS MAY USE THIS SOFTWARE ONLY IF SEPARATELY APPROVED BY
//  RYAN LU.
//
//  NO COPYING, MODIFICATION, DISTRIBUTION, HOSTING, DISCLOSURE,
//  REVERSE ENGINEERING, DECOMPILATION, OR OTHER USE IS PERMITTED
//  EXCEPT AS EXPRESSLY AUTHORIZED IN WRITING BY RYAN LU.
//
//  ALL RIGHTS NOT EXPRESSLY GRANTED ARE RESERVED BY RYAN LU.

package meetfiles

import (
	"path/filepath"
	"strings"
)

// DetectFileType infers a FileType from the filename extension and, optionally,
// the first few bytes of file content (magic-byte sniff).
//
// Priority:
//  1. ".sd3" extension → FileTypeSD3
//  2. ".cl2" extension → FileTypeCL2
//  3. ".zip" extension → FileTypeZIP
//  4. ".hy3" extension → FileTypeHY3
//  5. ".hyv" extension → FileTypeHYV
//  6. ".ev3" extension → FileTypeEV3
//  7. ZIP magic bytes (PK\x03\x04) → FileTypeZIP
//  8. SDIF A0 leader ("A0") at offset 0 → FileTypeSD3
//
// Note: .hy3 files are ZIP containers with a proprietary extension.  The
// extension check must appear before the magic-byte ZIP fallback so that
// .hy3 files are routed to the HY3 ingest path rather than treated as
// generic ZIPs.
//
// Returns an empty FileType if the format cannot be determined.
func DetectFileType(filename string, header []byte) FileType {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".sd3":
		return FileTypeSD3
	case ".cl2":
		return FileTypeCL2
	case ".zip":
		return FileTypeZIP
	case ".hy3":
		return FileTypeHY3
	case ".hyv":
		return FileTypeHYV
	case ".ev3":
		return FileTypeEV3
	}

	// Magic-byte sniff when extension is missing or unrecognised.
	if len(header) >= 4 &&
		header[0] == 'P' && header[1] == 'K' &&
		header[2] == 0x03 && header[3] == 0x04 {
		return FileTypeZIP
	}
	if len(header) >= 2 && header[0] == 'A' && header[1] == '0' {
		return FileTypeSD3
	}
	return ""
}
