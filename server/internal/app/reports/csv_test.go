package reports

import (
	"strings"
	"testing"
)

func TestBuildCSVQuotesHeadersAndRows(t *testing.T) {
	t.Parallel()

	doc, err := BuildCSV("export.csv", []string{"Name", "Note"}, [][]string{
		{"Lane, Sky", "Line one\nLine two"},
	})
	if err != nil {
		t.Fatalf("BuildCSV() error = %v", err)
	}
	if doc.Filename != "export.csv" {
		t.Fatalf("BuildCSV() filename = %q, want export.csv", doc.Filename)
	}
	got := string(doc.Data)
	if !strings.Contains(got, "\"Lane, Sky\"") || !strings.Contains(got, "\"Line one\nLine two\"") {
		t.Fatalf("BuildCSV() data = %q, want quoted comma and newline fields", got)
	}
}

func TestBuildCSVNeutralizesFormulasAndSanitizesFilename(t *testing.T) {
	t.Parallel()

	doc, err := BuildCSV("../invoice INV/1.csv", []string{"Name", "Amount"}, [][]string{
		{"=HYPERLINK(\"http://x\")", "-500"},
		{"@cmd", "+12.5"},
	})
	if err != nil {
		t.Fatalf("BuildCSV() error = %v", err)
	}
	if strings.ContainsAny(doc.Filename, "/ \\") || strings.HasPrefix(doc.Filename, ".") {
		t.Fatalf("filename not sanitized: %q", doc.Filename)
	}
	got := string(doc.Data)
	if !strings.Contains(got, "'=HYPERLINK") || !strings.Contains(got, "'@cmd") {
		t.Fatalf("formula cells not neutralized: %q", got)
	}
	if !strings.Contains(got, ",-500") || !strings.Contains(got, ",+12.5") {
		t.Fatalf("numeric cells should be untouched: %q", got)
	}
}
