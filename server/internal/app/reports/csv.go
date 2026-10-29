package reports

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type CSVDocument struct {
	Filename string
	Data     []byte
}

// unsafeFilenameChars are replaced so a report name built from user data (an
// invoice number, say) cannot add path segments to the storage key or break the
// Content-Disposition header.
var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeFilename(name string) string {
	name = strings.Trim(unsafeFilenameChars.ReplaceAllString(name, "_"), "._")
	if name == "" {
		return "report.csv"
	}
	return name
}

// neutralizeCell stops spreadsheet apps from running a cell as a formula: text
// starting with = + - @ tab or CR gets a leading apostrophe. Plain numbers
// (including negative amounts) are left alone.
func neutralizeCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			return value
		}
		return "'" + value
	}
	return value
}

// BuildCSV renders a CSV report. Cells are formula-neutralised and the
// filename is reduced to safe characters.
func BuildCSV(filename string, headers []string, rows [][]string) (*CSVDocument, error) {
	filename = safeFilename(filename)
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("write csv headers: %w", err)
	}
	for _, row := range rows {
		safe := make([]string, len(row))
		for i, cell := range row {
			safe[i] = neutralizeCell(cell)
		}
		if err := writer.Write(safe); err != nil {
			return nil, fmt.Errorf("write csv row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush csv: %w", err)
	}
	return &CSVDocument{Filename: filename, Data: buf.Bytes()}, nil
}
