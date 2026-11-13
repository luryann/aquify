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

// DetectFileType infers a FileType from the filename extension and, when the
// extension is missing or unknown, from the first bytes of the content.
//
// Recognised extensions: .hyv and .ev3 (meet event files a host publishes),
// .sd3 and .hy3 (entry and results files) and .zip. Without a known
// extension, ZIP magic bytes mean FileTypeZIP, an SDIF "A0" leader means
// FileTypeSD3 and a Hy-Tek "A1" leader means FileTypeHY3.
//
// Returns an empty FileType if the format cannot be determined.
func DetectFileType(filename string, header []byte) FileType {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".sd3":
		return FileTypeSD3
	case ".zip":
		return FileTypeZIP
	case ".hy3":
		return FileTypeHY3
	case ".hyv":
		return FileTypeHYV
	case ".ev3":
		return FileTypeEV3
	}
	if len(header) >= 4 && header[0] == 'P' && header[1] == 'K' && header[2] == 0x03 && header[3] == 0x04 {
		return FileTypeZIP
	}
	if len(header) >= 2 && header[0] == 'A' && header[1] == '0' {
		return FileTypeSD3
	}
	if len(header) >= 2 && header[0] == 'A' && header[1] == '1' {
		return FileTypeHY3
	}
	return ""
}
