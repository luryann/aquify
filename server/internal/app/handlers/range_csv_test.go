package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseRangeBoundDateOnlyUpperIsInclusiveOfDay(t *testing.T) {
	got, ok := parseRangeBound("2026-11-05", true)
	if !ok || !got.Equal(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date-only upper = %v, %v", got, ok)
	}
	if got, _ := parseRangeBound("2026-11-05", false); !got.Equal(time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("lower bound must stay at midnight: %v", got)
	}
	if got, _ := parseRangeBound("2026-11-05T10:00:00Z", true); !got.Equal(time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("timestamp upper must not shift: %v", got)
	}
}

func TestCSVSafeRowsGuardsFormulasButKeepsNumbers(t *testing.T) {
	rows := csvSafeRows([][]string{{"=HYPERLINK(\"x\")", "-500", "+1.5", "@cmd", "Smith", "-evil"}})
	want := []string{"'=HYPERLINK(\"x\")", "-500", "+1.5", "'@cmd", "Smith", "'-evil"}
	for i, w := range want {
		if rows[0][i] != w {
			t.Errorf("cell %d = %q, want %q", i, rows[0][i], w)
		}
	}
}

func TestWriteCSVNeutralisesFormulas(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCSV(rec, "x.csv", [][]string{{"name", "balance"}, {"=cmd|' /C calc'!A0", "-500"}})
	body := rec.Body.String()
	if !strings.Contains(body, "'=cmd") || !strings.Contains(body, ",-500") {
		t.Fatalf("unexpected csv: %q", body)
	}
}
