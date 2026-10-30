package handlers

import "strconv"

// csvSafeCell neutralises spreadsheet formula injection for report cells.
// Unlike csvSafe it leaves genuine numbers (for example a negative balance
// such as "-500") untouched so numeric columns stay numeric.
func csvSafeCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return s
		}
		return "'" + s
	}
	return s
}

func csvSafeRows(rows [][]string) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		cp := make([]string, len(row))
		for j, cell := range row {
			cp[j] = csvSafeCell(cell)
		}
		out[i] = cp
	}
	return out
}
